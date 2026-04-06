package gateway

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/anyclaw/anyclaw/pkg/agent"
	"github.com/anyclaw/anyclaw/pkg/agentstore"
	"github.com/anyclaw/anyclaw/pkg/channel"
	"github.com/anyclaw/anyclaw/pkg/chat"
	"github.com/anyclaw/anyclaw/pkg/config"
	"github.com/anyclaw/anyclaw/pkg/cron"
	"github.com/anyclaw/anyclaw/pkg/discovery"
	"github.com/anyclaw/anyclaw/pkg/llm"
	"github.com/anyclaw/anyclaw/pkg/mcp"
	"github.com/anyclaw/anyclaw/pkg/observability"
	"github.com/anyclaw/anyclaw/pkg/plugin"
	"github.com/anyclaw/anyclaw/pkg/routing"
	"github.com/anyclaw/anyclaw/pkg/runtime"
	"github.com/anyclaw/anyclaw/pkg/speech"
	taskModule "github.com/anyclaw/anyclaw/pkg/task"
	"github.com/anyclaw/anyclaw/pkg/tools"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

type llmPlannerAdapter struct {
	client *llm.ClientWrapper
}

func (a *llmPlannerAdapter) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition) (*llm.Response, error) {
	return a.client.Chat(ctx, messages, tools)
}

func (a *llmPlannerAdapter) Name() string {
	return "llm-planner"
}

type Server struct {
	app            *runtime.App
	httpServer     *http.Server
	startedAt      time.Time
	store          *Store
	sessions       *SessionManager
	bus            *Bus
	channels       *channel.Manager
	telegram       *channel.TelegramAdapter
	slack          *channel.SlackAdapter
	discord        *channel.DiscordAdapter
	whatsapp       *channel.WhatsAppAdapter
	signal         *channel.SignalAdapter
	router         *channel.Router
	runtimePool    *RuntimePool
	tasks          *TaskManager
	taskModule     taskModule.TaskManager
	chatModule     chat.ChatManager
	storeModule    agentstore.StoreManager
	approvals      *approvalManager
	auth           *authMiddleware
	rateLimit      *rateLimiter
	plugins        *plugin.Registry
	ingressPlugins []plugin.IngressRunner
	jobQueue       chan func()
	jobCancel      map[string]bool
	jobMaxAttempts int
	webhooks       *WebhookHandler
	nodes          *NodeManager
	sttPipeline    *speech.STTPipeline
	sttIntegration *speech.STTIntegration
	sttManager     *speech.STTManager
	ttsPipeline    *speech.TTSPipeline
	ttsIntegration *speech.Integration
	ttsManager     *speech.Manager
	mcpRegistry    *mcp.Registry
	mcpServer      *mcp.Server
	marketStore    *plugin.Store
	discoverySvc   *discovery.Service
	mentionGate    *channel.MentionGate
	groupSecurity  *channel.GroupSecurity
	channelCmds    *channel.ChannelCommands
	channelPairing *channel.ChannelPairing
	channelPolicy  *channel.ChannelPolicy
	presenceMgr    *channel.PresenceManager
	contactDir     *channel.ContactDirectory
	devicePairing  *DevicePairing
}

var titleCase = cases.Title(language.English)

type controlPlaneSnapshot struct {
	Status         Status                `json:"status"`
	Channels       []channel.Status      `json:"channels"`
	Runtimes       []RuntimeInfo         `json:"runtimes"`
	RuntimeMetrics RuntimeMetrics        `json:"runtime_metrics"`
	RecentEvents   []*Event              `json:"recent_events"`
	RecentTools    []*ToolActivityRecord `json:"recent_tools"`
	RecentJobs     []*Job                `json:"recent_jobs"`
	UpdatedAt      string                `json:"updated_at"`
}

type Status struct {
	OK         bool   `json:"ok"`
	Status     string `json:"status"`
	Version    string `json:"version"`
	Provider   string `json:"provider"`
	Model      string `json:"model"`
	Address    string `json:"address"`
	StartedAt  string `json:"started_at,omitempty"`
	WorkingDir string `json:"working_dir"`
	WorkDir    string `json:"work_dir"`
	Sessions   int    `json:"sessions"`
	Events     int    `json:"events"`
	Skills     int    `json:"skills"`
	Tools      int    `json:"tools"`
	Secured    bool   `json:"secured"`
	Users      int    `json:"users"`
}

func New(app *runtime.App) *Server {
	store, err := NewStore(app.WorkDir)
	if err != nil {
		panic(err)
	}
	server := &Server{
		app:            app,
		store:          store,
		sessions:       NewSessionManager(store, app.Agent),
		bus:            NewBus(),
		runtimePool:    NewRuntimePool(app.ConfigPath, store, app.Config.Gateway.RuntimeMaxInstances, time.Duration(app.Config.Gateway.RuntimeIdleSeconds)*time.Second),
		auth:           newAuthMiddleware(&app.Config.Security),
		rateLimit:      newRateLimiter(&app.Config.Security),
		plugins:        app.Plugins,
		telegram:       nil,
		jobQueue:       make(chan func(), 64),
		jobCancel:      map[string]bool{},
		jobMaxAttempts: app.Config.Gateway.JobMaxAttempts,
		webhooks:       NewWebhookHandler(),
		nodes:          NewNodeManager(),
		devicePairing:  NewDevicePairing(app.Config.Security.PairingTTLHours),
	}
	if app.Config.Security.PairingEnabled {
		server.devicePairing.SetEnabled(true)
	}
	server.approvals = newApprovalManager(store)

	// Initialize routing layer for low-token path optimization
	var router *routing.Router
	var registry *plugin.Registry
	if app.Plugins != nil {
		registry = app.Plugins
		cfg := config.DefaultConfig()
		var planner routing.PlannerClient
		if app.LLM != nil {
			planner = &llmPlannerAdapter{client: app.LLM}
		}
		router = routing.NewRouter(app.Plugins, cfg, app.LLM, planner)
	}

	server.tasks = NewTaskManager(store, server.sessions, server.runtimePool, taskAppInfo{Name: app.Config.Agent.Name, WorkingDir: app.WorkingDir, ConfigPath: app.ConfigPath}, app.LLM, server.approvals, router, registry)

	if sm, err := agentstore.NewStoreManager(app.WorkDir, app.ConfigPath); err == nil {
		server.storeModule = sm
	}

	return server
}

func (s *Server) initChannels() {
	s.initSTT()
	s.initTTS()

	s.router = channel.NewRouter(s.app.Config.Channels.Routing)
	if s.plugins != nil {
		s.ingressPlugins = s.plugins.IngressRunners(s.app.Config.Plugins.Dir)
	}
	builders := map[string]func() channel.Adapter{
		"telegram-channel": func() channel.Adapter {
			s.telegram = channel.NewTelegramAdapter(s.app.Config.Channels.Telegram, s.router, s.appendEvent)
			return s.telegram
		},
		"slack-channel": func() channel.Adapter {
			s.slack = channel.NewSlackAdapter(s.app.Config.Channels.Slack, s.router, s.appendEvent)
			return s.slack
		},
		"discord-channel": func() channel.Adapter {
			s.discord = channel.NewDiscordAdapter(s.app.Config.Channels.Discord, s.router, s.appendEvent)
			return s.discord
		},
		"whatsapp-channel": func() channel.Adapter {
			s.whatsapp = channel.NewWhatsAppAdapter(s.app.Config.Channels.WhatsApp, s.router, s.appendEvent)
			return s.whatsapp
		},
		"signal-channel": func() channel.Adapter {
			s.signal = channel.NewSignalAdapter(s.app.Config.Channels.Signal, s.router, s.appendEvent)
			return s.signal
		},
	}
	var adapters []channel.Adapter
	if s.plugins != nil {
		for _, name := range s.plugins.EnabledPluginNames() {
			if builder, ok := builders[name]; ok {
				adapters = append(adapters, builder())
			}
		}
		for _, runner := range s.plugins.ChannelRunners(s.app.Config.Plugins.Dir) {
			adapters = append(adapters, newPluginChannelAdapter(runner, s.router, s.appendEvent))
		}
	}
	if len(adapters) == 0 {
		adapters = []channel.Adapter{
			builders["telegram-channel"](),
			builders["slack-channel"](),
			builders["discord-channel"](),
			builders["whatsapp-channel"](),
			builders["signal-channel"](),
		}
	}
	s.channels = channel.NewManager(adapters...)
	s.initChannelAdvanced()
}

func (s *Server) initChannelAdvanced() {
	botName := s.app.Config.Agent.Name
	if botName == "" {
		botName = "AnyClaw"
	}

	botUserID := ""
	if s.telegram != nil {
		botUserID = s.app.Config.Channels.Telegram.BotToken
	}

	secCfg := s.app.Config.Channels.Security
	channelPolicy := channel.ChannelPolicyFromConfig(config.ChannelSecurityConfig{
		DMPolicy:         secCfg.DMPolicy,
		GroupPolicy:      secCfg.GroupPolicy,
		AllowFrom:        secCfg.AllowFrom,
		PairingEnabled:   secCfg.PairingEnabled,
		PairingTTLHours:  secCfg.PairingTTLHours,
		MentionGate:      secCfg.MentionGate,
		RiskAcknowledged: s.app.Config.Security.RiskAcknowledged,
		DefaultDenyDM:    secCfg.DefaultDenyDM,
	})

	s.mentionGate = channel.NewMentionGate(secCfg.MentionGate, botUserID, nil)
	s.groupSecurity = channel.NewGroupSecurity()
	s.channelCmds = channel.NewChannelCommands(botName)
	s.channelPairing = channel.NewChannelPairing()
	if secCfg.PairingEnabled {
		s.channelPairing.SetEnabled(true)
	}
	for _, userID := range secCfg.AllowFrom {
		if userID = strings.TrimSpace(userID); userID != "" {
			channelPolicy.AddAllowedUser(userID)
		}
	}

	s.presenceMgr = channel.NewPresenceManager(func(ch, userID string, presence channel.PresenceInfo) {
		s.appendEvent("channel.presence", "", map[string]any{
			"channel": ch,
			"user_id": userID,
			"status":  presence.Status,
		})
	})
	s.contactDir = channel.NewContactDirectory()

	s.channelPolicy = channelPolicy
	s.appendEvent("security.init", "", map[string]any{
		"dm_policy":         secCfg.DMPolicy,
		"group_policy":      secCfg.GroupPolicy,
		"mention_gate":      secCfg.MentionGate,
		"pairing_enabled":   secCfg.PairingEnabled,
		"allow_from":        len(secCfg.AllowFrom),
		"risk_acknowledged": s.app.Config.Security.RiskAcknowledged,
	})
}

func (s *Server) initSTT() {
	sttCfg := s.app.Config.Speech.STT
	if !sttCfg.Enabled {
		return
	}

	s.sttManager = speech.NewSTTManager()

	if sttCfg.Provider != "" && sttCfg.APIKey != "" {
		providerType := speech.STTProviderType(sttCfg.Provider)
		sttProviderCfg := speech.STTConfig{
			Type:     providerType,
			APIKey:   sttCfg.APIKey,
			BaseURL:  sttCfg.BaseURL,
			Model:    sttCfg.Model,
			Language: sttCfg.DefaultLang,
			Timeout:  time.Duration(sttCfg.TimeoutSec) * time.Second,
		}
		if sttCfg.TimeoutSec <= 0 {
			sttProviderCfg.Timeout = 120 * time.Second
		}

		provider, err := speech.NewSTTProvider(sttProviderCfg)
		if err != nil {
			s.appendEvent("stt.init.error", "", map[string]any{"error": err.Error(), "provider": sttCfg.Provider})
			return
		}

		if err := s.sttManager.Register(sttCfg.Provider, provider); err != nil {
			s.appendEvent("stt.init.error", "", map[string]any{"error": err.Error(), "provider": sttCfg.Provider})
			return
		}
	}

	pipelineCfg := speech.STTPipelineConfig{
		Provider:      sttCfg.Provider,
		DefaultLang:   sttCfg.DefaultLang,
		AutoDetect:    sttCfg.DefaultLang == "auto",
		MaxDuration:   time.Duration(sttCfg.MaxDurationSec) * time.Second,
		MinConfidence: sttCfg.MinConfidence,
		Timeout:       time.Duration(sttCfg.TimeoutSec) * time.Second,
	}
	if sttCfg.MaxDurationSec <= 0 {
		pipelineCfg.MaxDuration = 10 * time.Minute
	}
	if sttCfg.TimeoutSec <= 0 {
		pipelineCfg.Timeout = 120 * time.Second
	}

	s.sttPipeline = speech.NewSTTPipeline(s.sttManager, pipelineCfg)

	integrationCfg := speech.STTIntegrationConfig{
		Enabled:          sttCfg.Enabled,
		AutoSTT:          sttCfg.AutoSTT,
		TriggerPrefix:    sttCfg.TriggerPrefix,
		Provider:         sttCfg.Provider,
		DefaultLang:      sttCfg.DefaultLang,
		MaxDuration:      pipelineCfg.MaxDuration,
		MinConfidence:    sttCfg.MinConfidence,
		Timeout:          pipelineCfg.Timeout,
		Channels:         sttCfg.Channels,
		ExcludeChannels:  sttCfg.ExcludeChannels,
		FallbackToVoice:  sttCfg.FallbackToVoice,
		AppendTranscript: sttCfg.AppendTranscript,
	}
	if integrationCfg.TriggerPrefix == "" {
		integrationCfg.TriggerPrefix = "/transcribe"
	}

	s.sttIntegration = speech.NewSTTIntegration(s.sttPipeline, integrationCfg)

	s.appendEvent("stt.init.ok", "", map[string]any{
		"provider": sttCfg.Provider,
		"auto_stt": sttCfg.AutoSTT,
		"language": sttCfg.DefaultLang,
		"channels": len(sttCfg.Channels),
		"excluded": len(sttCfg.ExcludeChannels),
	})
}

func (s *Server) initTTS() {
	ttsCfg := s.app.Config.Speech.TTS
	if !ttsCfg.Enabled {
		return
	}

	s.ttsManager = speech.NewManager()

	if ttsCfg.Provider != "" {
		providerType := speech.ProviderType(ttsCfg.Provider)
		ttsProviderCfg := speech.Config{
			Type:        providerType,
			APIKey:      ttsCfg.APIKey,
			BaseURL:     ttsCfg.BaseURL,
			Voice:       ttsCfg.Voice,
			AudioFormat: speech.AudioFormat(ttsCfg.Format),
			Timeout:     time.Duration(ttsCfg.TimeoutSec) * time.Second,
		}
		if ttsProviderCfg.AudioFormat == "" {
			ttsProviderCfg.AudioFormat = speech.FormatMP3
		}
		if ttsCfg.TimeoutSec <= 0 {
			ttsProviderCfg.Timeout = 30 * time.Second
		}

		provider, err := speech.NewProvider(ttsProviderCfg)
		if err != nil {
			s.appendEvent("tts.init.error", "", map[string]any{"error": err.Error(), "provider": ttsCfg.Provider})
			return
		}

		if err := s.ttsManager.Register(ttsCfg.Provider, provider); err != nil {
			s.appendEvent("tts.init.error", "", map[string]any{"error": err.Error(), "provider": ttsCfg.Provider})
			return
		}
	}

	pipelineCfg := speech.PipelineConfig{
		Enabled:         ttsCfg.Enabled,
		AutoTrigger:     ttsCfg.AutoTTS,
		TriggerKeywords: []string{ttsCfg.TriggerPrefix},
		DefaultProvider: ttsCfg.Provider,
		DefaultVoice:    ttsCfg.Voice,
		DefaultSpeed:    ttsCfg.Speed,
		DefaultFormat:   speech.AudioFormat(ttsCfg.Format),
		FallbackToText:  ttsCfg.FallbackToText,
		Timeout:         time.Duration(ttsCfg.TimeoutSec) * time.Second,
	}
	if pipelineCfg.TriggerKeywords[0] == "" {
		pipelineCfg.TriggerKeywords = []string{"/speak", "/voice", "/tts"}
	}
	if pipelineCfg.DefaultFormat == "" {
		pipelineCfg.DefaultFormat = speech.FormatMP3
	}
	if pipelineCfg.DefaultSpeed <= 0 {
		pipelineCfg.DefaultSpeed = 1.0
	}
	if pipelineCfg.Timeout <= 0 {
		pipelineCfg.Timeout = 30 * time.Second
	}

	s.ttsPipeline = speech.NewTTSPipeline(s.ttsManager, pipelineCfg)

	s.registerAudioSenders()

	integrationCfg := speech.IntegrationConfig{
		Enabled:          ttsCfg.Enabled,
		AutoTTS:          ttsCfg.AutoTTS,
		TTSTriggerPrefix: ttsCfg.TriggerPrefix,
		VoiceProvider:    ttsCfg.Provider,
		Voice:            ttsCfg.Voice,
		Speed:            ttsCfg.Speed,
		Format:           speech.AudioFormat(ttsCfg.Format),
		FallbackToText:   ttsCfg.FallbackToText,
		Timeout:          pipelineCfg.Timeout,
		Channels:         ttsCfg.Channels,
		ExcludeChannels:  ttsCfg.ExcludeChannels,
	}
	if integrationCfg.TTSTriggerPrefix == "" {
		integrationCfg.TTSTriggerPrefix = "/speak"
	}
	if integrationCfg.Format == "" {
		integrationCfg.Format = speech.FormatMP3
	}
	if integrationCfg.Speed <= 0 {
		integrationCfg.Speed = 1.0
	}

	s.ttsIntegration = speech.NewIntegration(s.ttsPipeline, nil, nil, integrationCfg)

	s.appendEvent("tts.init.ok", "", map[string]any{
		"provider": ttsCfg.Provider,
		"auto_tts": ttsCfg.AutoTTS,
		"voice":    ttsCfg.Voice,
		"channels": len(ttsCfg.Channels),
		"excluded": len(ttsCfg.ExcludeChannels),
	})
}

func (s *Server) registerAudioSenders() {
	if s.ttsPipeline == nil {
		return
	}

	chCfg := s.app.Config.Channels

	if chCfg.Telegram.Enabled && chCfg.Telegram.BotToken != "" {
		s.ttsPipeline.RegisterAudioSender("telegram", speech.NewTelegramAudioSender(chCfg.Telegram.BotToken))
	}
	if chCfg.Discord.Enabled && chCfg.Discord.BotToken != "" {
		s.ttsPipeline.RegisterAudioSender("discord", speech.NewDiscordAudioSender(chCfg.Discord.BotToken))
	}
	if chCfg.Slack.Enabled && chCfg.Slack.BotToken != "" {
		s.ttsPipeline.RegisterAudioSender("slack", speech.NewSlackAudioSender(chCfg.Slack.BotToken))
	}
	if chCfg.WhatsApp.Enabled && chCfg.WhatsApp.PhoneNumberID != "" && chCfg.WhatsApp.AccessToken != "" {
		s.ttsPipeline.RegisterAudioSender("whatsapp", speech.NewWhatsAppAudioSender(chCfg.WhatsApp.PhoneNumberID, chCfg.WhatsApp.AccessToken))
	}
	if chCfg.Signal.Enabled && chCfg.Signal.Number != "" {
		s.ttsPipeline.RegisterAudioSender("signal", speech.NewSignalAudioSender(chCfg.Signal.Number, ""))
	}
}

func (s *Server) startWorkers(ctx context.Context) {
	workerCount := s.app.Config.Gateway.JobWorkerCount
	if workerCount <= 0 {
		workerCount = 1
	}
	for i := 0; i < workerCount; i++ {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case job := <-s.jobQueue:
					if job != nil {
						job()
					}
				}
			}
		}()
	}
}

func (s *Server) shouldCancelJob(id string) bool {
	return s.jobCancel[id]
}

func (s *Server) wrap(path string, next http.HandlerFunc) http.HandlerFunc {
	if s.rateLimit != nil {
		next = s.rateLimit.Wrap(next)
	}
	if s.auth != nil {
		return s.auth.Wrap(path, next)
	}
	return next
}

func requirePermission(permission string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if permission == "" {
			next(w, r)
			return
		}
		user := UserFromContext(r.Context())
		if !HasPermission(user, permission) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": permission})
			return
		}
		next(w, r)
	}
}

func requireWorkspaceAccess(resolveWorkspace func(*http.Request) string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workspace := ""
		if resolveWorkspace != nil {
			workspace = resolveWorkspace(r)
		}
		if workspace == "" {
			next(w, r)
			return
		}
		if !HasScope(UserFromContext(r.Context()), workspace) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_scope": workspace})
			return
		}
		next(w, r)
	}
}

func requireHierarchyAccess(resolve func(*http.Request) (string, string, string), next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org, project, workspace := "", "", ""
		if resolve != nil {
			org, project, workspace = resolve(r)
		}
		if org == "" && project == "" && workspace == "" {
			next(w, r)
			return
		}
		if !HasHierarchyAccess(UserFromContext(r.Context()), org, project, workspace) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_org": org, "required_project": project, "required_workspace": workspace})
			return
		}
		next(w, r)
	}
}

func (s *Server) resolveWorkspaceFromQuery(r *http.Request) string {
	return strings.TrimSpace(r.URL.Query().Get("workspace"))
}

func (s *Server) resolveHierarchyFromQuery(r *http.Request) (string, string, string) {
	return strings.TrimSpace(r.URL.Query().Get("org")), strings.TrimSpace(r.URL.Query().Get("project")), strings.TrimSpace(r.URL.Query().Get("workspace"))
}

func (s *Server) resolveWorkspaceFromSessionPath(r *http.Request) string {
	id := strings.TrimPrefix(r.URL.Path, "/sessions/")
	if id == "" {
		return ""
	}
	session, ok := s.sessions.Get(id)
	if !ok {
		return ""
	}
	return session.Workspace
}

func (s *Server) resolveHierarchyFromSessionPath(r *http.Request) (string, string, string) {
	id := strings.TrimPrefix(r.URL.Path, "/sessions/")
	if id == "" {
		return "", "", ""
	}
	session, ok := s.sessions.Get(id)
	if !ok {
		return "", "", ""
	}
	return session.Org, session.Project, session.Workspace
}

func (s *Server) resolveSessionWorkspaceFromChat(r *http.Request) string {
	if r.Method != http.MethodPost {
		return ""
	}
	var req struct {
		SessionID string `json:"session_id"`
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return ""
	}
	r.Body = io.NopCloser(strings.NewReader(string(body)))
	if err := json.Unmarshal(body, &req); err != nil {
		return ""
	}
	if strings.TrimSpace(req.SessionID) == "" {
		return strings.TrimSpace(r.URL.Query().Get("workspace"))
	}
	session, ok := s.sessions.Get(strings.TrimSpace(req.SessionID))
	if !ok {
		return ""
	}
	return session.Workspace
}

func (s *Server) resolveResourceSelection(r *http.Request) (string, string, string) {
	org := strings.TrimSpace(r.URL.Query().Get("org"))
	project := strings.TrimSpace(r.URL.Query().Get("project"))
	workspace := strings.TrimSpace(r.URL.Query().Get("workspace"))
	return org, project, workspace
}

func (s *Server) validateResourceSelection(orgID string, projectID string, workspaceID string) (*Org, *Project, *Workspace, error) {
	var org *Org
	var project *Project
	var workspace *Workspace
	var ok bool
	if workspaceID == "" {
		return nil, nil, nil, fmt.Errorf("workspace is required")
	}
	workspace, ok = s.store.GetWorkspace(workspaceID)
	if !ok {
		return nil, nil, nil, fmt.Errorf("workspace not found: %s", workspaceID)
	}
	project, ok = s.store.GetProject(workspace.ProjectID)
	if !ok {
		return nil, nil, nil, fmt.Errorf("project not found: %s", workspace.ProjectID)
	}
	org, ok = s.store.GetOrg(project.OrgID)
	if !ok {
		return nil, nil, nil, fmt.Errorf("org not found: %s", project.OrgID)
	}
	if projectID != "" && project.ID != projectID {
		return nil, nil, nil, fmt.Errorf("workspace %s does not belong to project %s", workspaceID, projectID)
	}
	if orgID != "" && org.ID != orgID {
		return nil, nil, nil, fmt.Errorf("workspace %s does not belong to org %s", workspaceID, orgID)
	}
	return org, project, workspace, nil
}

func defaultResourceIDs(workingDir string) (string, string, string) {
	workspaceID := "workspace-default"
	clean := strings.TrimSpace(strings.ToLower(workingDir))
	if clean != "" {
		replacer := strings.NewReplacer(":", "-", "\\", "-", "/", "-", " ", "-")
		clean = replacer.Replace(clean)
		clean = strings.Trim(clean, "-.")
		if clean != "" {
			workspaceID = "ws-" + clean
		}
	}
	return "org-local", "project-local", workspaceID
}

func normalizeWorkspacePath(path string) string {
	clean := filepath.Clean(strings.TrimSpace(path))
	if os.PathSeparator == '\\' {
		return strings.ToLower(clean)
	}
	return clean
}

func (s *Server) ensureDefaultWorkspace() error {
	orgID, projectID, workspaceID := defaultResourceIDs(s.app.WorkingDir)
	if err := s.store.UpsertOrg(&Org{ID: orgID, Name: "Local Org"}); err != nil {
		return err
	}
	if err := s.store.UpsertProject(&Project{ID: projectID, OrgID: orgID, Name: "Local Project"}); err != nil {
		return err
	}
	desired := &Workspace{
		ID:        workspaceID,
		ProjectID: projectID,
		Name:      filepath.Base(s.app.WorkingDir),
		Path:      s.app.WorkingDir,
	}
	if existing, ok := s.store.GetWorkspace(workspaceID); ok {
		if existing.ProjectID == desired.ProjectID &&
			existing.Name == desired.Name &&
			normalizeWorkspacePath(existing.Path) == normalizeWorkspacePath(desired.Path) {
			return nil
		}
		existing.ProjectID = desired.ProjectID
		existing.Name = desired.Name
		existing.Path = desired.Path
		return s.store.UpsertWorkspace(existing)
	}
	for _, existing := range s.store.ListWorkspaces() {
		if existing.ProjectID != projectID {
			continue
		}
		samePath := normalizeWorkspacePath(existing.Path) == normalizeWorkspacePath(desired.Path)
		sameName := existing.Name == desired.Name
		if !samePath && !sameName {
			continue
		}
		if existing.ID != desired.ID {
			if err := s.store.RebindWorkspaceID(existing.ID, desired.ID); err != nil {
				return err
			}
		}
		existing.ID = desired.ID
		existing.ProjectID = desired.ProjectID
		existing.Name = desired.Name
		existing.Path = desired.Path
		return s.store.UpsertWorkspace(existing)
	}
	return s.store.UpsertWorkspace(desired)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

type agentProfileView struct {
	Name            string                 `json:"name"`
	Description     string                 `json:"description"`
	Role            string                 `json:"role,omitempty"`
	Persona         string                 `json:"persona,omitempty"`
	AvatarPreset    string                 `json:"avatar_preset,omitempty"`
	AvatarDataURL   string                 `json:"avatar_data_url,omitempty"`
	WorkingDir      string                 `json:"working_dir,omitempty"`
	PermissionLevel string                 `json:"permission_level,omitempty"`
	ProviderRef     string                 `json:"provider_ref,omitempty"`
	ProviderName    string                 `json:"provider_name,omitempty"`
	ProviderType    string                 `json:"provider_type,omitempty"`
	Provider        string                 `json:"provider,omitempty"`
	DefaultModel    string                 `json:"default_model,omitempty"`
	Enabled         bool                   `json:"enabled"`
	Active          bool                   `json:"active"`
	Personality     config.PersonalitySpec `json:"personality,omitempty"`
	Skills          []config.AgentSkillRef `json:"skills,omitempty"`
}

func (s *Server) buildAgentProfileView(profile config.AgentProfile) agentProfileView {
	personality := profile.Personality
	if strings.TrimSpace(personality.Template) == "" &&
		len(personality.Traits) == 0 &&
		strings.TrimSpace(personality.Tone) == "" &&
		strings.TrimSpace(personality.Style) == "" {
		personality = defaultPersonalitySpec()
	}
	providerName := ""
	providerType := ""
	providerRuntime := ""
	if provider, ok := s.app.Config.FindProviderProfile(profile.ProviderRef); ok {
		providerName = provider.Name
		providerType = provider.Type
		providerRuntime = provider.Provider
	}
	return agentProfileView{
		Name:            profile.Name,
		Description:     profile.Description,
		Role:            profile.Role,
		Persona:         profile.Persona,
		AvatarPreset:    profile.AvatarPreset,
		AvatarDataURL:   profile.AvatarDataURL,
		WorkingDir:      profile.WorkingDir,
		PermissionLevel: profile.PermissionLevel,
		ProviderRef:     profile.ProviderRef,
		ProviderName:    providerName,
		ProviderType:    providerType,
		Provider:        firstNonEmpty(providerRuntime, s.app.Config.LLM.Provider),
		DefaultModel:    profile.DefaultModel,
		Enabled:         profile.IsEnabled(),
		Active:          s.app.Config.IsCurrentAgentProfile(profile.Name),
		Personality:     personality,
		Skills:          append([]config.AgentSkillRef{}, profile.Skills...),
	}
}

func (s *Server) listAgentViews() []agentProfileView {
	items := make([]agentProfileView, 0, len(s.app.Config.Agent.Profiles))
	for _, profile := range s.app.Config.Agent.Profiles {
		items = append(items, s.buildAgentProfileView(profile))
	}
	return items
}

func (s *Server) getAgentView(name string) (agentProfileView, bool) {
	for _, profile := range s.app.Config.Agent.Profiles {
		if profile.Name == name {
			return s.buildAgentProfileView(profile), true
		}
	}
	return agentProfileView{}, false
}

func requestedAgentName(agentName string, assistantName string) string {
	return firstNonEmpty(agentName, assistantName)
}

func (s *Server) resolveAgentName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		if resolved := s.app.Config.ResolveMainAgentName(); resolved != "" {
			return resolved, nil
		}
		return "", fmt.Errorf("agent not configured")
	}
	if config.IsMainAgentAlias(name) {
		if resolved := s.app.Config.ResolveMainAgentName(); resolved != "" {
			return resolved, nil
		}
		return "", fmt.Errorf("agent not configured")
	}
	profile, ok := s.app.Config.FindAgentProfile(name)
	if !ok {
		if strings.EqualFold(name, strings.TrimSpace(s.app.Config.ResolveMainAgentName())) {
			return s.app.Config.ResolveMainAgentName(), nil
		}
		return "", fmt.Errorf("agent not found: %s", name)
	}
	if !profile.IsEnabled() {
		return "", fmt.Errorf("agent is disabled: %s", name)
	}
	return profile.Name, nil
}

func (s *Server) resolveAssistantName(name string) (string, error) {
	return s.resolveAgentName(name)
}

func (s *Server) Run(ctx context.Context) error {
	addr := runtime.GatewayAddress(s.app.Config)
	mux := http.NewServeMux()
	s.initChannels()
	s.initMCP(ctx)
	s.initMarketStore()
	s.initDiscovery(ctx)
	if err := s.ensureDefaultWorkspace(); err != nil {
		return err
	}
	s.startWorkers(ctx)

	// Observability middleware
	obs := newObservabilityMiddleware(runtime.Version)
	obs.RegisterHealthChecks(s.app)

	mux.HandleFunc("/health", obs.handleHealth)
	mux.HandleFunc("/ready", obs.handleReady)
	mux.HandleFunc("/live", obs.handleLive)
	mux.HandleFunc("/metrics", obs.handleMetrics)
	mux.HandleFunc("/metrics.json", obs.handleMetricsJSON)
	observability.RegisterPprof(mux, "/debug/pprof/")

	mux.HandleFunc("/healthz", s.wrap("/healthz", s.handleHealth))
	mux.HandleFunc("/status", s.wrap("/status", requirePermission("status.read", s.handleStatus)))
	mux.HandleFunc("/chat", s.wrap("/chat", requirePermission("chat.send", requireHierarchyAccess(func(r *http.Request) (string, string, string) {
		return s.resolveHierarchyFromQuery(r)
	}, s.handleChat))))
	mux.HandleFunc("/channels", s.wrap("/channels", requirePermission("channels.read", s.handleChannels)))
	mux.HandleFunc("/plugins", s.wrap("/plugins", requirePermission("plugins.read", s.handlePlugins)))
	mux.HandleFunc("/apps", s.wrap("/apps", requirePermission("apps.read", s.handleApps)))
	mux.HandleFunc("/app-workflows/resolve", s.wrap("/app-workflows/resolve", requirePermission("apps.read", s.handleAppWorkflowResolve)))
	mux.HandleFunc("/app-bindings", s.wrap("/app-bindings", s.handleAppBindings))
	mux.HandleFunc("/app-pairings", s.wrap("/app-pairings", s.handleAppPairings))
	mux.HandleFunc("/routing", s.wrap("/routing", requirePermission("routing.read", s.handleRouting)))
	mux.HandleFunc("/routing/analysis", s.wrap("/routing/analysis", requirePermission("routing.read", s.handleRoutingAnalysis)))
	mux.HandleFunc("/agents", s.wrap("/agents", s.handleAgents))
	mux.HandleFunc("/agents/personality-templates", s.wrap("/agents/personality-templates", requirePermission("config.read", s.handlePersonalityTemplates)))
	mux.HandleFunc("/agents/skill-catalog", s.wrap("/agents/skill-catalog", requirePermission("skills.read", s.handleAssistantSkillCatalog)))
	mux.HandleFunc("/assistants", s.wrap("/assistants", s.handleAssistants))
	mux.HandleFunc("/assistants/personality-templates", s.wrap("/assistants/personality-templates", requirePermission("config.read", s.handlePersonalityTemplates)))
	mux.HandleFunc("/assistants/skill-catalog", s.wrap("/assistants/skill-catalog", requirePermission("skills.read", s.handleAssistantSkillCatalog)))
	mux.HandleFunc("/providers", s.wrap("/providers", s.handleProviders))
	mux.HandleFunc("/providers/test", s.wrap("/providers/test", s.handleProviderTest))
	mux.HandleFunc("/providers/default", s.wrap("/providers/default", s.handleDefaultProvider))
	mux.HandleFunc("/agent-bindings", s.wrap("/agent-bindings", s.handleAgentBindings))
	mux.HandleFunc("/runtimes", s.wrap("/runtimes", requirePermission("runtimes.read", requireHierarchyAccess(s.resolveHierarchyFromQuery, s.handleRuntimes))))
	mux.HandleFunc("/runtimes/refresh", s.wrap("/runtimes/refresh", requirePermission("runtimes.write", s.handleRefreshRuntime)))
	mux.HandleFunc("/runtimes/refresh-batch", s.wrap("/runtimes/refresh-batch", requirePermission("runtimes.write", s.handleRefreshRuntimesBatch)))
	mux.HandleFunc("/runtimes/metrics", s.wrap("/runtimes/metrics", requirePermission("runtimes.read", s.handleRuntimeMetrics)))
	mux.HandleFunc("/resources", s.wrap("/resources", s.handleResources))
	mux.HandleFunc("/auth/users", s.wrap("/auth/users", s.handleUsers))
	mux.HandleFunc("/auth/roles", s.wrap("/auth/roles", s.handleRoles))
	mux.HandleFunc("/auth/roles/impact", s.wrap("/auth/roles/impact", requirePermission("auth.users.read", s.handleRoleImpact)))
	mux.HandleFunc("/audit", s.wrap("/audit", requirePermission("audit.read", s.handleAudit)))
	mux.HandleFunc("/jobs", s.wrap("/jobs", requirePermission("audit.read", s.handleJobs)))
	mux.HandleFunc("/jobs/", s.wrap("/jobs/", requirePermission("audit.read", s.handleJobByID)))
	mux.HandleFunc("/jobs/retry", s.wrap("/jobs/retry", requirePermission("audit.read", s.handleRetryJob)))
	mux.HandleFunc("/jobs/cancel", s.wrap("/jobs/cancel", requirePermission("audit.read", s.handleCancelJob)))
	mux.HandleFunc("/config", s.wrap("/config", s.handleConfigAPI))
	mux.HandleFunc("/memory", s.wrap("/memory", requirePermission("memory.read", requireHierarchyAccess(s.resolveHierarchyFromQuery, s.handleMemory))))
	mux.HandleFunc("/events", s.wrap("/events", requirePermission("events.read", s.handleEvents)))
	mux.HandleFunc("/events/stream", s.wrap("/events/stream", requirePermission("events.read", s.handleEventStream)))
	mux.HandleFunc("/ws", s.wrap("/ws", s.handleOpenClawWS))
	mux.HandleFunc("/control-plane", s.wrap("/control-plane", requirePermission("status.read", s.handleControlPlane)))
	mux.HandleFunc("/sessions", s.wrap("/sessions", requirePermission("sessions.read", requireHierarchyAccess(s.resolveHierarchyFromQuery, s.handleSessions))))
	mux.HandleFunc("/sessions/", s.wrap("/sessions/", requirePermission("sessions.read", requireHierarchyAccess(s.resolveHierarchyFromSessionPath, s.handleSessionByID))))
	mux.HandleFunc("/sessions/move", s.wrap("/sessions/move", requirePermission("sessions.write", s.handleMoveSession)))
	mux.HandleFunc("/sessions/move-batch", s.wrap("/sessions/move-batch", requirePermission("sessions.write", s.handleMoveSessionsBatch)))
	mux.HandleFunc("/tasks", s.wrap("/tasks", requirePermission("tasks.write", requireHierarchyAccess(s.resolveHierarchyFromQuery, s.handleTasks))))
	mux.HandleFunc("/tasks/", s.wrap("/tasks/", s.handleTaskByID))
	mux.HandleFunc("/v2/tasks", s.wrap("/v2/tasks", requirePermission("tasks.write", s.handleV2Tasks)))
	mux.HandleFunc("/v2/tasks/", s.wrap("/v2/tasks/", requirePermission("tasks.read", s.handleV2TaskByID)))
	mux.HandleFunc("/v2/agents", s.wrap("/v2/agents", requirePermission("tasks.read", s.handleV2Agents)))
	mux.HandleFunc("/v2/chat", s.wrap("/v2/chat", requirePermission("tasks.write", s.handleV2Chat)))
	mux.HandleFunc("/v2/chat/sessions", s.wrap("/v2/chat/sessions", requirePermission("tasks.read", s.handleV2ChatSessions)))
	mux.HandleFunc("/v2/chat/sessions/", s.wrap("/v2/chat/sessions/", requirePermission("tasks.read", s.handleV2ChatSessionByID)))
	mux.HandleFunc("/v2/store", s.wrap("/v2/store", requirePermission("tasks.read", s.handleV2Store)))
	mux.HandleFunc("/v2/store/", s.wrap("/v2/store/", requirePermission("tasks.read", s.handleV2StoreByID)))
	mux.HandleFunc("/approvals", s.wrap("/approvals", requirePermission("approvals.read", s.handleApprovals)))
	mux.HandleFunc("/approvals/", s.wrap("/approvals/", requirePermission("approvals.write", s.handleApprovalByID)))
	mux.HandleFunc("/skills", s.wrap("/skills", requirePermission("skills.read", s.handleSkills)))
	mux.HandleFunc("/tools/activity", s.wrap("/tools/activity", requirePermission("tools.read", s.handleToolActivity)))
	mux.HandleFunc("/tools", s.wrap("/tools", requirePermission("tools.read", s.handleTools)))
	mux.HandleFunc("/mcp/servers", s.wrap("/mcp/servers", requirePermission("mcp.read", s.handleMCPServers)))
	mux.HandleFunc("/mcp/tools", s.wrap("/mcp/tools", requirePermission("mcp.read", s.handleMCPTools)))
	mux.HandleFunc("/mcp/resources", s.wrap("/mcp/resources", requirePermission("mcp.read", s.handleMCPResources)))
	mux.HandleFunc("/mcp/prompts", s.wrap("/mcp/prompts", requirePermission("mcp.read", s.handleMCPPrompts)))
	mux.HandleFunc("/mcp/call", s.wrap("/mcp/call", requirePermission("mcp.write", s.handleMCPCall)))
	mux.HandleFunc("/mcp/servers/", s.wrap("/mcp/servers/", s.handleMCPServerAction))
	mux.HandleFunc("/market/search", s.wrap("/market/search", requirePermission("market.read", s.handleMarketSearch)))
	mux.HandleFunc("/market/plugins", s.wrap("/market/plugins", requirePermission("market.read", s.handleMarketPlugins)))
	mux.HandleFunc("/market/plugins/", s.wrap("/market/plugins/", s.handleMarketPluginAction))
	mux.HandleFunc("/market/installed", s.wrap("/market/installed", requirePermission("market.read", s.handleMarketInstalled)))
	mux.HandleFunc("/market/categories", s.wrap("/market/categories", requirePermission("market.read", s.handleMarketCategories)))
	mux.HandleFunc("/channel/mention-gate", s.wrap("/channel/mention-gate", s.handleMentionGate))
	mux.HandleFunc("/channel/group-security", s.wrap("/channel/group-security", s.handleGroupSecurity))
	mux.HandleFunc("/channel/pairing", s.wrap("/channel/pairing", s.handleChannelPairing))
	mux.HandleFunc("/channel/presence", s.wrap("/channel/presence", s.handlePresence))
	mux.HandleFunc("/channel/contacts", s.wrap("/channel/contacts", s.handleContacts))
	mux.HandleFunc("/channels/whatsapp/webhook", s.rateLimit.Wrap(s.handleWhatsAppWebhook))
	mux.HandleFunc("/channels/discord/interactions", s.rateLimit.Wrap(s.handleDiscordInteractions))
	mux.HandleFunc("/ingress/web", s.rateLimit.Wrap(s.handleSignedIngress))
	mux.HandleFunc("/ingress/plugins/", s.rateLimit.Wrap(s.handlePluginIngress))

	// OpenAI-compatible API endpoints
	mux.HandleFunc("/v1/chat/completions", s.wrap("/v1/chat/completions", s.handleOpenAIChatCompletions))
	mux.HandleFunc("/v1/models", s.wrap("/v1/models", s.handleOpenAIModels))
	mux.HandleFunc("/v1/responses", s.wrap("/v1/responses", s.handleOpenAIResponses))

	// Webhook endpoints
	mux.HandleFunc("/webhooks/", s.rateLimit.Wrap(s.handleWebhookIncoming))

	// Device nodes
	mux.HandleFunc("/nodes", s.wrap("/nodes", requirePermission("nodes.read", s.handleNodesList)))
	mux.HandleFunc("/nodes/", s.wrap("/nodes/", s.handleNodeByID))
	mux.HandleFunc("/nodes/invoke", s.wrap("/nodes/invoke", requirePermission("nodes.write", s.handleNodeInvoke)))

	// Device pairing
	mux.HandleFunc("/device/pairing", s.wrap("/device/pairing", s.handleDevicePairing))
	mux.HandleFunc("/device/pairing/code", s.wrap("/device/pairing/code", s.handleDevicePairingCode))

	// LAN Discovery
	mux.HandleFunc("/discovery/instances", s.wrap("/discovery/instances", s.handleDiscoveryInstances))
	mux.HandleFunc("/discovery/query", s.wrap("/discovery/query", s.handleDiscoveryQuery))

	// Cron jobs
	cron.RegisterUIHandler(mux, cronScheduler, "/cron")

	mux.HandleFunc("/", s.handleRootAPI)

	s.startedAt = time.Now().UTC()
	s.httpServer = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go s.runChannels(ctx)
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.httpServer.Shutdown(shutdownCtx)
	case err := <-errCh:
		return fmt.Errorf("gateway server failed: %w", err)
	}
}

func (s *Server) runChannels(ctx context.Context) {
	if s.channels == nil {
		return
	}
	handler := s.processChannelMessage
	handler = s.mentionGate.Wrap(handler)
	handler = s.groupSecurity.Wrap(handler)
	handler = s.channelPairing.Wrap(handler)
	handler = s.channelCmds.Wrap(handler)
	handler = s.contactDir.Wrap(handler)
	handler = s.presenceMgr.Wrap(handler)
	if s.sttIntegration != nil {
		handler = s.sttIntegration.WrapInboundHandler(handler)
	}
	s.channels.Run(ctx, handler)
	s.runStreamChannels(ctx)
}

func (s *Server) runStreamChannels(ctx context.Context) {
	for _, adapter := range s.getStreamAdapters() {
		if !adapter.Enabled() {
			continue
		}
		go func(sa channel.StreamAdapter) {
			handler := s.processChannelMessageStream
			handler = s.mentionGate.WrapStream(handler)
			handler = s.groupSecurity.WrapStream(handler)
			handler = s.channelPairing.WrapStream(handler)
			handler = s.channelCmds.WrapStream(handler)
			handler = s.contactDir.WrapStream(handler)
			handler = s.presenceMgr.WrapStream(handler)
			if s.sttIntegration != nil {
				handler = s.sttIntegration.WrapStreamInboundHandler(handler)
			}
			_ = sa.RunStream(ctx, handler)
		}(adapter)
	}
}

func (s *Server) getStreamAdapters() []channel.StreamAdapter {
	var adapters []channel.StreamAdapter
	if s.telegram != nil && s.app.Config.Channels.Telegram.StreamReply {
		adapters = append(adapters, s.telegram)
	}
	if s.discord != nil && s.app.Config.Channels.Discord.StreamReply {
		adapters = append(adapters, s.discord)
	}
	if s.slack != nil && s.app.Config.Channels.Slack.StreamReply {
		adapters = append(adapters, s.slack)
	}
	return adapters
}

func (s *Server) processChannelMessage(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error) {
	source := strings.TrimSpace(meta["channel"])
	if source == "" {
		source = "telegram"
	}
	response, session, err := s.runOrCreateChannelSession(ctx, source, sessionID, message, meta)
	if err != nil {
		return "", "", err
	}

	if s.ttsIntegration != nil && response != "" {
		recipient := firstNonEmpty(strings.TrimSpace(meta["chat_id"]), strings.TrimSpace(meta["reply_target"]), strings.TrimSpace(meta["user_id"]), sessionID)
		metadata := make(map[string]any, len(meta))
		for k, v := range meta {
			metadata[k] = v
		}
		if err := s.ttsIntegration.ProcessMessage(ctx, source, recipient, response, metadata); err != nil {
			s.appendEvent("tts.process.error", sessionID, map[string]any{"error": err.Error(), "channel": source})
		}
	}

	return session.ID, response, nil
}

func (s *Server) processChannelMessageStream(ctx context.Context, sessionID string, message string, meta map[string]string, onChunk func(chunk string) error) (string, error) {
	source := strings.TrimSpace(meta["channel"])
	if source == "" {
		source = "telegram"
	}
	_, session, err := s.runOrCreateChannelSessionStream(ctx, source, sessionID, message, meta, onChunk)
	if err != nil {
		return "", err
	}

	if s.ttsIntegration != nil {
		recipient := firstNonEmpty(strings.TrimSpace(meta["chat_id"]), strings.TrimSpace(meta["reply_target"]), strings.TrimSpace(meta["user_id"]), sessionID)
		metadata := make(map[string]any, len(meta))
		for k, v := range meta {
			metadata[k] = v
		}
		if err := s.ttsIntegration.ProcessMessage(ctx, source, recipient, session.ID, metadata); err != nil {
			s.appendEvent("tts.process.error", sessionID, map[string]any{"error": err.Error(), "channel": source})
		}
	}

	return session.ID, nil
}

func (s *Server) resolveChannelRouteDecision(source string, sessionID string, message string, meta map[string]string) channel.RouteDecision {
	decision := channel.RouteDecision{}
	routeSource := sessionID
	if replyTarget := strings.TrimSpace(meta["reply_target"]); replyTarget != "" {
		routeSource = replyTarget
	}
	if s.router != nil {
		decision = s.router.Decide(channel.RouteRequest{
			Channel:  source,
			Source:   routeSource,
			Text:     message,
			ThreadID: meta["thread_id"],
			IsGroup:  meta["is_group"] == "true",
			GroupID:  meta["guild_id"],
		})
	}
	return decision
}

func (s *Server) ensureChannelSession(source string, sessionID string, decision channel.RouteDecision, meta map[string]string, streaming bool) (string, error) {
	if strings.TrimSpace(sessionID) != "" {
		return sessionID, nil
	}

	agentName := s.app.Config.ResolveMainAgentName()
	orgID, projectID, workspaceID := defaultResourceIDs(s.app.WorkingDir)
	if decision.Agent != "" {
		agentName = decision.Agent
	}
	if decision.Org != "" {
		orgID = decision.Org
	}
	if decision.Project != "" {
		projectID = decision.Project
	}
	if decision.Workspace != "" {
		workspaceID = decision.Workspace
	}

	org, project, workspace, err := s.validateResourceSelection(orgID, projectID, workspaceID)
	if err != nil {
		return "", err
	}

	title := strings.TrimSpace(decision.Title)
	if title == "" {
		title = titleCase.String(source) + " session"
	}

	createOpts := SessionCreateOptions{
		Title:         title,
		AgentName:     agentName,
		Org:           org.ID,
		Project:       project.ID,
		Workspace:     workspace.ID,
		SessionMode:   normalizeSingleAgentSessionMode(decision.SessionMode, "channel-dm"),
		QueueMode:     decision.QueueMode,
		ReplyBack:     decision.ReplyBack,
		SourceChannel: source,
		SourceID:      channelSourceID(meta, sessionID),
	}
	if createOpts.SessionMode == "" {
		createOpts.SessionMode = "main"
	}

	session, err := s.sessions.CreateWithOptions(createOpts)
	if err != nil {
		return "", err
	}

	sessionID = session.ID
	payload := channelMetaPayload(map[string]any{
		"title":  session.Title,
		"source": source,
	}, meta)
	if streaming {
		payload["streaming"] = true
	}
	s.appendEvent("session.created", sessionID, payload)
	return sessionID, nil
}

func (s *Server) prepareChannelExecution(ctx context.Context, source string, sessionID string, message string, decision channel.RouteDecision, meta map[string]string, streaming bool) (*Session, *runtime.App, context.Context, error) {
	if _, err := s.sessions.EnqueueTurn(sessionID); err == nil {
		s.appendEvent("session.queue.updated", sessionID, map[string]any{
			"queue_mode":   decision.QueueMode,
			"source":       source,
			"reply_target": meta["reply_target"],
		})
	}

	if _, err := s.sessions.SetUserMapping(
		sessionID,
		meta["user_id"],
		firstNonEmpty(meta["username"], meta["user_name"]),
		meta["reply_target"],
		meta["thread_id"],
		channelTransportMeta(meta),
	); err == nil {
		s.appendEvent("session.user_mapped", sessionID, map[string]any{
			"source":       source,
			"user_id":      meta["user_id"],
			"user_name":    firstNonEmpty(meta["username"], meta["user_name"]),
			"reply_target": meta["reply_target"],
		})
	}

	if _, err := s.sessions.SetPresence(sessionID, "typing", true); err == nil {
		s.appendEvent("session.typing", sessionID, map[string]any{
			"typing":  true,
			"source":  source,
			"user_id": meta["user_id"],
		})
	}

	startedPayload := channelMetaPayload(map[string]any{
		"message": message,
		"source":  source,
	}, meta)
	if streaming {
		startedPayload["streaming"] = true
	}
	s.appendEvent("chat.started", sessionID, startedPayload)

	session, ok := s.sessions.Get(sessionID)
	if !ok {
		return nil, nil, nil, fmt.Errorf("session not found: %s", sessionID)
	}
	if s.runtimePool == nil {
		return nil, nil, nil, fmt.Errorf("runtime pool not initialized")
	}

	targetApp, err := s.runtimePool.GetOrCreate(session.Agent, session.Org, session.Project, session.Workspace)
	if err != nil {
		return nil, nil, nil, err
	}
	targetApp.Agent.SetHistory(session.History)

	execCtx := tools.WithBrowserSession(ctx, sessionID)
	execCtx = tools.WithSandboxScope(execCtx, tools.SandboxScope{SessionID: sessionID, Channel: source})
	return session, targetApp, execCtx, nil
}

func (s *Server) finalizeChannelExecution(sessionID string, source string, message string, response string, meta map[string]string, targetApp *runtime.App, streaming bool) (*Session, error) {
	updatedSession, err := s.sessions.AddExchange(sessionID, message, response)
	if err != nil {
		return nil, err
	}
	if updatedSession.ReplyBack {
		s.appendEvent("session.reply_back", sessionID, map[string]any{
			"enabled":      true,
			"source":       source,
			"reply_target": meta["reply_target"],
		})
	}
	if _, err := s.sessions.SetPresence(sessionID, "idle", false); err == nil {
		s.appendEvent("session.presence", sessionID, map[string]any{
			"presence": "idle",
			"source":   source,
			"user_id":  meta["user_id"],
		})
	}

	if targetApp != nil {
		s.recordSessionToolActivities(updatedSession, targetApp.Agent.GetLastToolActivities())
	}

	completedPayload := channelMetaPayload(map[string]any{
		"message":         message,
		"response_length": len(response),
		"source":          source,
	}, meta)
	if streaming {
		completedPayload["streaming"] = true
	}
	s.appendEvent("chat.completed", sessionID, completedPayload)
	return updatedSession, nil
}

func channelMetaPayload(base map[string]any, meta map[string]string) map[string]any {
	payload := make(map[string]any, len(base)+len(meta))
	for k, v := range base {
		payload[k] = v
	}
	for k, v := range meta {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			payload[k] = trimmed
		}
	}
	return payload
}

func channelTransportMeta(meta map[string]string) map[string]string {
	transportMeta := map[string]string{}
	for _, key := range []string{"channel_id", "chat_id", "guild_id", "attachment_count"} {
		if v := strings.TrimSpace(meta[key]); v != "" {
			transportMeta[key] = v
		}
	}
	return transportMeta
}

func channelSourceID(meta map[string]string, fallback string) string {
	return firstNonEmpty(strings.TrimSpace(meta["user_id"]), strings.TrimSpace(meta["reply_target"]), fallback)
}

func (s *Server) runOrCreateChannelSession(ctx context.Context, source string, sessionID string, message string, meta map[string]string) (string, *Session, error) {
	decision := s.resolveChannelRouteDecision(source, sessionID, message, meta)
	sessionID, err := s.ensureChannelSession(source, sessionID, decision, meta, false)
	if err != nil {
		return "", nil, err
	}
	_, targetApp, execCtx, err := s.prepareChannelExecution(ctx, source, sessionID, message, decision, meta, false)
	if err != nil {
		return "", nil, err
	}
	response, err := targetApp.Agent.Run(execCtx, message)
	if err != nil {
		return "", nil, err
	}
	updatedSession, err := s.finalizeChannelExecution(sessionID, source, message, response, meta, targetApp, false)
	if err != nil {
		return "", nil, err
	}
	return response, updatedSession, nil
}

func (s *Server) runOrCreateChannelSessionStream(ctx context.Context, source string, sessionID string, message string, meta map[string]string, onChunk func(chunk string) error) (string, *Session, error) {
	decision := s.resolveChannelRouteDecision(source, sessionID, message, meta)
	sessionID, err := s.ensureChannelSession(source, sessionID, decision, meta, true)
	if err != nil {
		return "", nil, err
	}
	_, targetApp, execCtx, err := s.prepareChannelExecution(ctx, source, sessionID, message, decision, meta, true)
	if err != nil {
		return "", nil, err
	}

	var responseText strings.Builder
	err = targetApp.Agent.RunStream(execCtx, message, func(chunk string) {
		responseText.WriteString(chunk)
		onChunk(chunk)
	})
	if err != nil {
		return "", nil, err
	}

	response := responseText.String()
	updatedSession, err := s.finalizeChannelExecution(sessionID, source, message, response, meta, targetApp, true)
	if err != nil {
		return "", nil, err
	}
	return response, updatedSession, nil
}

func (s *Server) appendEvent(eventType string, sessionID string, payload map[string]any) {
	if s == nil {
		return
	}
	event := NewEvent(eventType, sessionID, payload)
	if s.store != nil {
		_ = s.store.AppendEvent(event)
	}
	if s.bus != nil {
		s.bus.Publish(event)
	}
}

func (s *Server) appendToolActivity(sessionID string, activity ToolActivityRecord) {
	activity.ID = uniqueID("tool")
	activity.SessionID = sessionID
	if activity.Timestamp.IsZero() {
		activity.Timestamp = time.Now().UTC()
	}
	_ = s.store.AppendToolActivity(&activity)
	s.appendEvent("tool.activity", sessionID, map[string]any{
		"tool_name": activity.ToolName,
		"args":      activity.Args,
		"error":     activity.Error,
		"agent":     activity.Agent,
		"workspace": activity.Workspace,
	})
}

func (s *Server) recordSessionToolActivities(session *Session, activities []agent.ToolActivity) {
	for _, activity := range activities {
		s.appendToolActivity(session.ID, ToolActivityRecord{
			ToolName:  activity.ToolName,
			Args:      activity.Args,
			Result:    activity.Result,
			Error:     activity.Error,
			Agent:     session.Agent,
			Workspace: session.Workspace,
		})
	}
}

func (s *Server) controlPlaneSnapshot() controlPlaneSnapshot {
	channels := []channel.Status{}
	if s.channels != nil {
		channels = s.channels.Statuses()
	}
	runtimes := []RuntimeInfo{}
	metrics := RuntimeMetrics{}
	if s.runtimePool != nil {
		runtimes = s.runtimePool.List()
		metrics = s.runtimePool.Metrics()
	}
	return controlPlaneSnapshot{
		Status:         s.status(),
		Channels:       channels,
		Runtimes:       runtimes,
		RuntimeMetrics: metrics,
		RecentEvents:   s.store.ListEvents(24),
		RecentTools:    s.store.ListToolActivities(24, ""),
		RecentJobs:     s.store.ListJobs(12),
		UpdatedAt:      time.Now().UTC().Format(time.RFC3339),
	}
}

func (s *Server) appendAudit(user *AuthUser, action string, target string, meta map[string]any) {
	actor := "anonymous"
	role := ""
	if user != nil {
		actor = user.Name
		role = user.Role
	}
	_ = s.store.AppendAudit(&AuditEvent{
		ID:        uniqueID("aud"),
		Actor:     actor,
		Role:      role,
		Action:    action,
		Target:    target,
		Timestamp: time.Now().UTC(),
		Meta:      meta,
	})
}

func Probe(ctx context.Context, baseURL string) (*Status, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/status", nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	var status Status
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, err
	}
	return &status, nil
}

func (s *Server) status() Status {
	secured := strings.TrimSpace(s.app.Config.Security.APIToken) != ""
	return Status{
		OK:         true,
		Status:     "running",
		Version:    runtime.Version,
		Provider:   s.app.Config.LLM.Provider,
		Model:      s.app.Config.LLM.Model,
		Address:    runtime.GatewayAddress(s.app.Config),
		StartedAt:  s.startedAt.Format(time.RFC3339),
		WorkingDir: s.app.WorkingDir,
		WorkDir:    s.app.WorkDir,
		Sessions:   len(s.store.ListSessions()),
		Events:     len(s.store.ListEvents(0)),
		Skills:     len(s.app.Agent.ListSkills()),
		Tools:      len(s.app.Agent.ListTools()),
		Secured:    secured,
		Users:      len(s.app.Config.Security.Users),
	}
}

type GatewayStatus struct {
	Status    Status         `json:"status"`
	Health    HealthStatus   `json:"health"`
	Presence  PresenceStatus `json:"presence"`
	Typing    TypingStatus   `json:"typing"`
	Approvals ApprovalStatus `json:"approvals"`
	Sessions  SessionStatus  `json:"sessions"`
	Channels  ChannelStatus  `json:"channels"`
	Security  SecurityStatus `json:"security"`
	Runtime   RuntimeStatus  `json:"runtime"`
	UpdatedAt string         `json:"updated_at"`
}

type HealthStatus struct {
	OK            bool   `json:"ok"`
	Uptime        string `json:"uptime"`
	ChannelsUp    int    `json:"channels_up"`
	ChannelsTotal int    `json:"channels_total"`
	LLMConnected  bool   `json:"llm_connected"`
	LastError     string `json:"last_error,omitempty"`
}

type PresenceStatus struct {
	ActiveUsers int            `json:"active_users"`
	ByChannel   map[string]int `json:"by_channel"`
}

type TypingStatus struct {
	ActiveSessions int            `json:"active_sessions"`
	ByChannel      map[string]int `json:"by_channel"`
}

type ApprovalStatus struct {
	Pending  int `json:"pending"`
	Approved int `json:"approved"`
	Denied   int `json:"denied"`
	Total    int `json:"total"`
}

type SessionStatus struct {
	Total     int            `json:"total"`
	Active    int            `json:"active"`
	Idle      int            `json:"idle"`
	Queued    int            `json:"queued"`
	ByChannel map[string]int `json:"by_channel"`
}

type ChannelStatus struct {
	Total  int                      `json:"total"`
	ByName map[string]AdapterStatus `json:"by_name"`
}

type AdapterStatus struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Running bool   `json:"running"`
	Healthy bool   `json:"healthy"`
	Error   string `json:"error,omitempty"`
}

type SecurityStatus struct {
	DMPolicy         string `json:"dm_policy"`
	GroupPolicy      string `json:"group_policy"`
	MentionGate      bool   `json:"mention_gate"`
	PairingEnabled   bool   `json:"pairing_enabled"`
	RiskAcknowledged bool   `json:"risk_acknowledged"`
	AllowFromCount   int    `json:"allow_from_count"`
}

type RuntimeStatus struct {
	Pooled int `json:"pooled"`
	Active int `json:"active"`
	Idle   int `json:"idle"`
	Max    int `json:"max"`
}

func (s *Server) GatewayStatus() GatewayStatus {
	sessions := s.store.ListSessions()
	activeUsers := make(map[string]bool)
	typingSessions := 0
	queuedSessions := 0
	channelSessions := make(map[string]int)
	for _, sess := range sessions {
		if sess.UserID != "" {
			activeUsers[sess.UserID] = true
		}
		if sess.Typing {
			typingSessions++
		}
		if sess.Presence == "queued" {
			queuedSessions++
		}
		if sess.SourceChannel != "" {
			channelSessions[sess.SourceChannel]++
		}
	}

	approvals := s.store.ListApprovals("")
	pendingApprovals := 0
	approvedApprovals := 0
	deniedApprovals := 0
	for _, a := range approvals {
		switch a.Status {
		case "pending":
			pendingApprovals++
		case "approved":
			approvedApprovals++
		case "denied":
			deniedApprovals++
		}
	}

	channelStatuses := s.channels.Statuses()
	channelsUp := 0
	channelByName := make(map[string]AdapterStatus)
	for _, st := range channelStatuses {
		if st.Running && st.Healthy {
			channelsUp++
		}
		channelByName[st.Name] = AdapterStatus{
			Name:    st.Name,
			Enabled: st.Enabled,
			Running: st.Running,
			Healthy: st.Healthy,
			Error:   st.LastError,
		}
	}

	secCfg := s.app.Config.Channels.Security
	securityStatus := SecurityStatus{
		DMPolicy:         secCfg.DMPolicy,
		GroupPolicy:      secCfg.GroupPolicy,
		MentionGate:      secCfg.MentionGate,
		PairingEnabled:   secCfg.PairingEnabled,
		RiskAcknowledged: s.app.Config.Security.RiskAcknowledged,
		AllowFromCount:   len(secCfg.AllowFrom),
	}

	poolStatus := s.runtimePool.Status()
	runtimeStatus := RuntimeStatus{
		Pooled: poolStatus.Pooled,
		Active: poolStatus.Active,
		Idle:   poolStatus.Idle,
		Max:    poolStatus.Max,
	}

	return GatewayStatus{
		Status: s.status(),
		Health: HealthStatus{
			OK:            true,
			Uptime:        time.Since(s.startedAt).Round(time.Second).String(),
			ChannelsUp:    channelsUp,
			ChannelsTotal: len(channelStatuses),
			LLMConnected:  s.app.LLM != nil,
		},
		Presence: PresenceStatus{
			ActiveUsers: len(activeUsers),
			ByChannel:   nil,
		},
		Typing: TypingStatus{
			ActiveSessions: typingSessions,
			ByChannel:      nil,
		},
		Approvals: ApprovalStatus{
			Pending:  pendingApprovals,
			Approved: approvedApprovals,
			Denied:   deniedApprovals,
			Total:    len(approvals),
		},
		Sessions: SessionStatus{
			Total:     len(sessions),
			Active:    len(sessions) - queuedSessions,
			Idle:      0,
			Queued:    queuedSessions,
			ByChannel: channelSessions,
		},
		Channels: ChannelStatus{
			Total:  len(channelStatuses),
			ByName: channelByName,
		},
		Security:  securityStatus,
		Runtime:   runtimeStatus,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}
}

func writeJSON(w http.ResponseWriter, statusCode int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(value)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleRootAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"name":    "AnyClaw Gateway",
		"version": runtime.Version,
		"status":  "running",
		"endpoints": map[string]string{
			"health":     "/healthz",
			"status":     "/status",
			"chat":       "/chat",
			"agents":     "/agents",
			"tasks":      "/tasks",
			"sessions":   "/sessions",
			"channels":   "/channels",
			"plugins":    "/plugins",
			"skills":     "/skills",
			"tools":      "/tools",
			"websocket":  "/ws",
			"openai_api": "/v1/chat/completions",
			"models":     "/v1/models",
			"responses":  "/v1/responses",
			"webhooks":   "/webhooks/",
			"nodes":      "/nodes",
			"cron":       "/cron",
			"pairing":    "/device/pairing",
		},
	})
}

func (s *Server) handleDiscordInteractions(w http.ResponseWriter, r *http.Request) {
	if s.discord == nil || !s.discord.Enabled() {
		http.NotFound(w, r)
		return
	}
	body, err := channel.ReadBody(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if !s.discord.VerifyInteraction(r, body) {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}
	response, err := s.discord.HandleInteraction(r.Context(), body, s.processChannelMessage)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleWhatsAppWebhook(w http.ResponseWriter, r *http.Request) {
	if s.whatsapp == nil || !s.whatsapp.Enabled() {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		verifyToken := strings.TrimSpace(s.app.Config.Channels.WhatsApp.VerifyToken)
		if verifyToken == "" || r.URL.Query().Get("hub.verify_token") != verifyToken {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte(r.URL.Query().Get("hub.challenge")))
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if secret := strings.TrimSpace(s.app.Config.Channels.WhatsApp.AppSecret); secret != "" {
			provided := strings.TrimSpace(r.Header.Get("X-Hub-Signature-256"))
			if !verifySignature(secret, body, provided) {
				http.Error(w, "invalid signature", http.StatusUnauthorized)
				return
			}
		}
		var payload struct {
			Entry []struct {
				Changes []struct {
					Value struct {
						Statuses []struct {
							ID          string `json:"id"`
							Status      string `json:"status"`
							RecipientID string `json:"recipient_id"`
						} `json:"statuses"`
						Messages []struct {
							ID      string `json:"id"`
							From    string `json:"from"`
							Profile struct {
								Name string `json:"name"`
							} `json:"profile"`
							Text struct {
								Body string `json:"body"`
							} `json:"text"`
						} `json:"messages"`
					} `json:"value"`
				} `json:"changes"`
			} `json:"entry"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		for _, entry := range payload.Entry {
			for _, change := range entry.Changes {
				for _, status := range change.Value.Statuses {
					s.whatsapp.HandleStatus("", status.Status, status.ID, status.RecipientID)
				}
				for _, msg := range change.Value.Messages {
					text := strings.TrimSpace(msg.Text.Body)
					if text == "" {
						continue
					}
					if _, _, err := s.whatsapp.HandleInbound(r.Context(), msg.From, text, msg.ID, msg.Profile.Name, s.processChannelMessage); err != nil {
						writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
						return
					}
				}
			}
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.appendAudit(UserFromContext(r.Context()), "status.read", "status", nil)

	if r.URL.Query().Get("extended") == "true" {
		writeJSON(w, http.StatusOK, s.GatewayStatus())
		return
	}
	writeJSON(w, http.StatusOK, s.status())
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Message   string `json:"message"`
		SessionID string `json:"session_id"`
		Title     string `json:"title"`
		Agent     string `json:"agent"`
		Assistant string `json:"assistant"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message is required"})
		return
	}
	agentName, err := s.resolveAgentName(requestedAgentName(req.Agent, req.Assistant))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if strings.TrimSpace(req.SessionID) == "" {
		orgID, projectID, workspaceID := s.resolveResourceSelection(r)
		org, project, workspace, err := s.validateResourceSelection(orgID, projectID, workspaceID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if !HasHierarchyAccess(UserFromContext(r.Context()), org.ID, project.ID, workspace.ID) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_org": org.ID, "required_project": project.ID, "required_workspace": workspace.ID})
			return
		}
		createOpts := SessionCreateOptions{
			Title:       req.Title,
			AgentName:   agentName,
			Org:         org.ID,
			Project:     project.ID,
			Workspace:   workspace.ID,
			SessionMode: "main",
			QueueMode:   "fifo",
		}
		session, err := s.sessions.CreateWithOptions(createOpts)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		req.SessionID = session.ID
		s.appendEvent("session.created", session.ID, map[string]any{"title": session.Title, "org": session.Org, "project": session.Project, "workspace": session.Workspace})
	}

	response, updatedSession, err := s.runSessionMessage(r.Context(), req.SessionID, req.Title, req.Message)
	if err != nil {
		if errors.Is(err, ErrTaskWaitingApproval) {
			s.appendAudit(UserFromContext(r.Context()), "chat.send", req.SessionID, map[string]any{"message_length": len(req.Message), "status": "waiting_approval"})
			writeJSON(w, http.StatusAccepted, s.sessionApprovalResponse(req.SessionID))
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.appendAudit(UserFromContext(r.Context()), "chat.send", updatedSession.ID, map[string]any{"message_length": len(req.Message)})
	writeJSON(w, http.StatusOK, map[string]any{"response": response, "session": updatedSession})
}

func (s *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if !HasPermission(UserFromContext(r.Context()), "tasks.read") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "tasks.read"})
			return
		}
		items := s.store.ListTasks()
		workspace := strings.TrimSpace(r.URL.Query().Get("workspace"))
		status := strings.TrimSpace(r.URL.Query().Get("status"))
		filtered := make([]*Task, 0, len(items))
		for _, task := range items {
			if workspace != "" && task.Workspace != workspace {
				continue
			}
			if status != "" && !strings.EqualFold(task.Status, status) {
				continue
			}
			filtered = append(filtered, task)
		}
		s.appendAudit(UserFromContext(r.Context()), "tasks.read", "tasks", map[string]any{"count": len(filtered)})
		writeJSON(w, http.StatusOK, filtered)
	case http.MethodPost:
		if !HasPermission(UserFromContext(r.Context()), "tasks.write") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "tasks.write"})
			return
		}
		var req struct {
			Title     string `json:"title"`
			Input     string `json:"input"`
			Agent     string `json:"agent"`
			Assistant string `json:"assistant"`
			SessionID string `json:"session_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		if strings.TrimSpace(req.Input) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "input is required"})
			return
		}
		assistantName, err := s.resolveAgentName(requestedAgentName(req.Agent, req.Assistant))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		var orgID, projectID, workspaceID string
		if strings.TrimSpace(req.SessionID) != "" {
			session, ok := s.sessions.Get(strings.TrimSpace(req.SessionID))
			if !ok {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
				return
			}
			orgID, projectID, workspaceID = session.Org, session.Project, session.Workspace
		} else {
			queryOrg, queryProject, queryWorkspace := s.resolveHierarchyFromQuery(r)
			org, project, workspace, err := s.validateResourceSelection(queryOrg, queryProject, queryWorkspace)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			orgID, projectID, workspaceID = org.ID, project.ID, workspace.ID
		}
		task, err := s.tasks.Create(TaskCreateOptions{
			Title:     req.Title,
			Input:     req.Input,
			Assistant: assistantName,
			Org:       orgID,
			Project:   projectID,
			Workspace: workspaceID,
			SessionID: req.SessionID,
		})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		result, err := s.tasks.Execute(r.Context(), task.ID)
		if err != nil {
			if errors.Is(err, ErrTaskWaitingApproval) {
				s.appendAudit(UserFromContext(r.Context()), "tasks.write", task.ID, map[string]any{"status": "waiting_approval"})
				response := s.taskResponse(result.Task, result.Session)
				response["status"] = "waiting_approval"
				writeJSON(w, http.StatusAccepted, response)
				return
			}
			s.appendAudit(UserFromContext(r.Context()), "tasks.write", task.ID, map[string]any{"status": "failed"})
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error(), "task": task})
			return
		}
		s.recordTaskCompletion(result, "task_api")
		s.appendAudit(UserFromContext(r.Context()), "tasks.write", task.ID, map[string]any{"status": result.Task.Status})
		writeJSON(w, http.StatusCreated, s.taskResponse(result.Task, result.Session))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleTaskByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/tasks/")
	path = strings.TrimSpace(path)
	if path == "" {
		http.Error(w, "task id required", http.StatusBadRequest)
		return
	}
	parts := strings.Split(path, "/")
	taskID := strings.TrimSpace(parts[0])
	task, ok := s.tasks.Get(taskID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
		return
	}
	if len(parts) > 1 && parts[1] == "steps" {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !HasPermission(UserFromContext(r.Context()), "tasks.read") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "tasks.read"})
			return
		}
		writeJSON(w, http.StatusOK, s.tasks.Steps(taskID))
		return
	}
	if len(parts) > 1 && parts[1] == "execute" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !HasPermission(UserFromContext(r.Context()), "tasks.write") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "tasks.write"})
			return
		}
		result, err := s.tasks.Execute(r.Context(), taskID)
		if err != nil {
			if errors.Is(err, ErrTaskWaitingApproval) {
				s.appendAudit(UserFromContext(r.Context()), "tasks.write", taskID, map[string]any{"status": "waiting_approval", "resume": true})
				response := s.taskResponse(result.Task, result.Session)
				response["status"] = "waiting_approval"
				writeJSON(w, http.StatusAccepted, response)
				return
			}
			s.appendAudit(UserFromContext(r.Context()), "tasks.write", taskID, map[string]any{"status": "failed", "resume": true})
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error(), "task": task})
			return
		}
		s.recordTaskCompletion(result, "task_resume")
		s.appendAudit(UserFromContext(r.Context()), "tasks.write", taskID, map[string]any{"status": result.Task.Status, "resume": true})
		writeJSON(w, http.StatusOK, s.taskResponse(result.Task, result.Session))
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !HasPermission(UserFromContext(r.Context()), "tasks.read") {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "tasks.read"})
		return
	}
	response := s.taskResponse(task, nil)
	s.appendAudit(UserFromContext(r.Context()), "tasks.read", taskID, nil)
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleApprovals(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		status := strings.TrimSpace(r.URL.Query().Get("status"))
		items := s.store.ListApprovals(status)
		s.appendAudit(UserFromContext(r.Context()), "approvals.read", "approvals", map[string]any{"count": len(items), "status": status})
		writeJSON(w, http.StatusOK, items)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleApprovalByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/approvals/"))
	if path == "" {
		http.Error(w, "approval id required", http.StatusBadRequest)
		return
	}
	parts := strings.Split(path, "/")
	id := strings.TrimSpace(parts[0])
	approval, ok := s.store.GetApproval(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "approval not found"})
		return
	}
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, http.StatusOK, approval)
		return
	}
	if len(parts) == 2 && parts[1] == "resolve" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Approved bool   `json:"approved"`
			Comment  string `json:"comment"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		actor := "anonymous"
		if user := UserFromContext(r.Context()); user != nil {
			actor = user.Name
		}
		updated, err := s.approvals.Resolve(id, req.Approved, actor, req.Comment)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		s.handleResolvedApproval(updated, req.Approved, req.Comment)
		s.appendAudit(UserFromContext(r.Context()), "approvals.write", id, map[string]any{"approved": req.Approved})
		if updated.TaskID != "" || updated.SessionID != "" {
			payload := map[string]any{"approval_id": updated.ID, "status": updated.Status}
			if updated.TaskID != "" {
				payload["task_id"] = updated.TaskID
			}
			if updated.ToolName != "" {
				payload["tool_name"] = updated.ToolName
			}
			s.appendEvent("approval.resolved", updated.SessionID, payload)
		}
		writeJSON(w, http.StatusOK, updated)
		return
	}
	http.NotFound(w, r)
}

func (s *Server) taskResponse(task *Task, session *Session) map[string]any {
	response := map[string]any{
		"task":      task,
		"steps":     s.tasks.Steps(task.ID),
		"approvals": s.store.ListTaskApprovals(task.ID),
	}
	if session != nil {
		response["session"] = session
	} else if strings.TrimSpace(task.SessionID) != "" {
		if linkedSession, ok := s.sessions.Get(task.SessionID); ok {
			response["session"] = linkedSession
		}
	}
	return response
}

func (s *Server) recordTaskCompletion(result *TaskExecutionResult, source string) {
	if result == nil || result.Task == nil || result.Session == nil {
		return
	}
	s.appendEvent("task.completed", result.Session.ID, map[string]any{"task_id": result.Task.ID, "status": result.Task.Status, "source": source})
	app, getErr := s.runtimePool.GetOrCreate(result.Task.Assistant, result.Task.Org, result.Task.Project, result.Task.Workspace)
	if getErr != nil {
		return
	}
	freshSession, ok := s.sessions.Get(result.Session.ID)
	if !ok {
		return
	}
	s.recordSessionToolActivities(freshSession, app.Agent.GetLastToolActivities())
}

func (s *Server) runSessionMessage(ctx context.Context, sessionID string, title string, message string) (string, *Session, error) {
	return s.runSessionMessageWithOptions(ctx, sessionID, title, message, sessionRunOptions{Source: "api"})
}

func (s *Server) runSessionMessageWithOptions(ctx context.Context, sessionID string, title string, message string, opts sessionRunOptions) (string, *Session, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", nil, fmt.Errorf("session creation now requires registered org/project/workspace via request path")
	}
	source := firstNonEmpty(strings.TrimSpace(opts.Source), "api")

	if !opts.Resume {
		if _, err := s.sessions.EnqueueTurn(sessionID); err == nil {
			s.appendEvent("session.queue.updated", sessionID, map[string]any{"queue_mode": "fifo", "source": source})
		}
	}
	if _, err := s.sessions.SetPresence(sessionID, "typing", true); err == nil {
		s.appendEvent("session.typing", sessionID, map[string]any{"typing": true, "source": source})
	}
	eventName := "chat.started"
	if opts.Resume {
		eventName = "chat.resumed"
	}
	s.appendEvent(eventName, sessionID, map[string]any{"message": message, "source": source})
	session, ok := s.sessions.Get(sessionID)
	if !ok {
		return "", nil, fmt.Errorf("session not found: %s", sessionID)
	}
	targetApp, err := s.runtimePool.GetOrCreate(session.Agent, session.Org, session.Project, session.Workspace)
	if err != nil {
		return "", session, err
	}
	targetApp.Agent.SetHistory(session.History)
	execCtx := tools.WithBrowserSession(ctx, sessionID)
	execCtx = tools.WithSandboxScope(execCtx, tools.SandboxScope{SessionID: sessionID, Channel: "api"})
	execCtx = agent.WithToolApprovalHook(execCtx, s.sessionToolApprovalHook(session, targetApp.Config, title, message, source))
	execCtx = tools.WithToolApprovalHook(execCtx, s.sessionProtocolApprovalHook(session, targetApp.Config, title, message, source))
	response, err := targetApp.Agent.Run(execCtx, message)
	if err != nil {
		if errors.Is(err, ErrTaskWaitingApproval) {
			s.updateSessionApprovalPresence(sessionID, "")
		} else {
			s.updateSessionPresence(sessionID, "idle", false)
		}
		return "", session, err
	}
	updatedSession, err := s.sessions.AddExchange(sessionID, message, response)
	if err != nil {
		return "", session, err
	}
	if _, err := s.sessions.SetPresence(sessionID, "idle", false); err == nil {
		s.appendEvent("session.presence", sessionID, map[string]any{"presence": "idle", "source": source})
	}
	s.recordSessionToolActivities(updatedSession, targetApp.Agent.GetLastToolActivities())
	s.appendEvent("chat.completed", sessionID, map[string]any{"message": message, "response_length": len(response), "source": source})
	return response, updatedSession, nil
	/*
		session, err := s.sessions.Create(title, s.app.Config.Agent.Name, org.ID, project.ID, workspace.ID)
		if err != nil {
			return "", nil, err
		}
		sessionID = session.ID
		s.appendEvent("session.created", sessionID, map[string]any{"title": session.Title})
	*/
}

func (s *Server) handleResolvedApproval(updated *Approval, approved bool, comment string) {
	if updated == nil {
		return
	}
	if updated.TaskID != "" {
		if approved {
			go func(taskID string) {
				result, runErr := s.tasks.Execute(context.Background(), taskID)
				if runErr != nil {
					if errors.Is(runErr, ErrTaskWaitingApproval) {
						return
					}
					return
				}
				s.recordTaskCompletion(result, "approval_resume")
			}(updated.TaskID)
			return
		}
		_ = s.tasks.MarkRejected(updated.TaskID, updated.StepIndex, firstNonEmpty(strings.TrimSpace(comment), "task execution rejected by approver"))
		return
	}
	if updated.SessionID == "" {
		return
	}
	if approved {
		approval := cloneApproval(updated)
		go func(item *Approval) {
			if item == nil {
				return
			}
			_ = s.resumeApprovedSessionApproval(context.Background(), item)
		}(approval)
		return
	}
	s.updateSessionPresence(updated.SessionID, "idle", false)
	s.appendEvent("chat.cancelled", updated.SessionID, map[string]any{
		"approval_id": updated.ID,
		"reason":      firstNonEmpty(strings.TrimSpace(comment), "approval rejected"),
		"source":      "approval",
	})
}

func (s *Server) handleMemory(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		mem, err := s.app.Agent.ShowMemory()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"memory": mem})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleConfigAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if !HasPermission(UserFromContext(r.Context()), "config.read") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "config.read"})
			return
		}
		s.appendAudit(UserFromContext(r.Context()), "config.read", "config", nil)
		writeJSON(w, http.StatusOK, s.app.Config)
	case http.MethodPost:
		var cfg map[string]any
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		if llm, ok := cfg["llm"].(map[string]any); ok {
			if provider, ok := llm["provider"].(string); ok {
				s.app.Config.LLM.Provider = provider
			}
			if model, ok := llm["model"].(string); ok {
				s.app.Config.LLM.Model = model
			}
		}
		if channels, ok := cfg["channels"].(map[string]any); ok {
			if routing, ok := channels["routing"].(map[string]any); ok {
				if mode, ok := routing["mode"].(string); ok {
					s.app.Config.Channels.Routing.Mode = mode
				}
				if rawRules, ok := routing["rules"].([]any); ok {
					rules := make([]config.ChannelRoutingRule, 0, len(rawRules))
					seen := map[string]bool{}
					for _, item := range rawRules {
						ruleMap, ok := item.(map[string]any)
						if !ok {
							continue
						}
						rule := config.ChannelRoutingRule{}
						if v, ok := ruleMap["channel"].(string); ok {
							rule.Channel = v
						}
						if v, ok := ruleMap["match"].(string); ok {
							rule.Match = v
						}
						if v, ok := ruleMap["session_mode"].(string); ok {
							rule.SessionMode = v
						}
						if v, ok := ruleMap["session_id"].(string); ok {
							rule.SessionID = v
						}
						if v, ok := ruleMap["queue_mode"].(string); ok {
							rule.QueueMode = v
						}
						if v, ok := ruleMap["reply_back"].(bool); ok {
							replyBack := v
							rule.ReplyBack = &replyBack
						}
						if v, ok := ruleMap["title_prefix"].(string); ok {
							rule.TitlePrefix = v
						}
						if v, ok := ruleMap["agent"].(string); ok {
							rule.Agent = v
						}
						if v, ok := ruleMap["org"].(string); ok {
							rule.Org = v
						}
						if v, ok := ruleMap["project"].(string); ok {
							rule.Project = v
						}
						if v, ok := ruleMap["workspace"].(string); ok {
							rule.Workspace = v
						}
						if v, ok := ruleMap["workspace_ref"].(string); ok {
							rule.WorkspaceRef = v
						}
						if rule.WorkspaceRef != "" || rule.Workspace != "" {
							workspaceID := rule.WorkspaceRef
							if workspaceID == "" {
								workspaceID = rule.Workspace
							}
							if _, _, _, err := s.validateResourceSelection(rule.Org, rule.Project, workspaceID); err != nil {
								writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid routing resource reference", "details": err.Error()})
								return
							}
						}
						conflictKey := strings.Join([]string{rule.Channel, rule.Match, rule.SessionMode, rule.Agent, rule.Org, rule.Project, firstNonEmpty(rule.WorkspaceRef, rule.Workspace)}, "|")
						if seen[conflictKey] {
							writeJSON(w, http.StatusBadRequest, map[string]string{"error": "duplicate routing rule", "details": conflictKey})
							return
						}
						seen[conflictKey] = true
						rules = append(rules, rule)
					}
					s.app.Config.Channels.Routing.Rules = rules
				}
			}
		}
		if err := s.app.Config.Save(s.app.ConfigPath); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		s.appendAudit(UserFromContext(r.Context()), "config.write", "config", nil)
		writeJSON(w, http.StatusOK, s.app.Config)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleToolActivity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
	writeJSON(w, http.StatusOK, s.store.ListToolActivities(limit, sessionID))
}

func (s *Server) handleChannels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.channels == nil {
		writeJSON(w, http.StatusOK, []channel.Status{})
		return
	}
	writeJSON(w, http.StatusOK, s.channels.Statuses())
}

func (s *Server) handlePlugins(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.plugins == nil {
		writeJSON(w, http.StatusOK, []plugin.Manifest{})
		return
	}
	s.appendAudit(UserFromContext(r.Context()), "plugins.read", "plugins", nil)
	writeJSON(w, http.StatusOK, s.plugins.List())
}

func (s *Server) handleRouting(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.app.Config.Channels.Routing)
}

func (s *Server) handleRoutingAnalysis(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, channel.AnalyzeRouting(s.app.Config.Channels.Routing))
}

func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if !HasPermission(UserFromContext(r.Context()), "config.read") && !HasPermission(UserFromContext(r.Context()), "config.write") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "config.read"})
			return
		}
		s.appendAudit(UserFromContext(r.Context()), "agents.read", "agents", nil)
		writeJSON(w, http.StatusOK, s.listAgentViews())
	case http.MethodPost:
		if !HasPermission(UserFromContext(r.Context()), "config.write") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "config.write"})
			return
		}
		var req struct {
			Name            string                 `json:"name"`
			Description     string                 `json:"description"`
			Role            string                 `json:"role"`
			Persona         string                 `json:"persona"`
			AvatarPreset    *string                `json:"avatar_preset"`
			AvatarDataURL   *string                `json:"avatar_data_url"`
			WorkingDir      string                 `json:"working_dir"`
			PermissionLevel string                 `json:"permission_level"`
			ProviderRef     string                 `json:"provider_ref"`
			DefaultModel    string                 `json:"default_model"`
			Enabled         *bool                  `json:"enabled"`
			Personality     config.PersonalitySpec `json:"personality"`
			Skills          []config.AgentSkillRef `json:"skills"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		profile := config.AgentProfile{
			Name:            req.Name,
			Description:     req.Description,
			Role:            req.Role,
			Persona:         req.Persona,
			WorkingDir:      req.WorkingDir,
			PermissionLevel: req.PermissionLevel,
			ProviderRef:     req.ProviderRef,
			DefaultModel:    req.DefaultModel,
			Enabled:         req.Enabled,
			Personality:     req.Personality,
			Skills:          req.Skills,
		}
		if req.AvatarPreset != nil {
			profile.AvatarPreset = *req.AvatarPreset
		}
		if req.AvatarDataURL != nil {
			profile.AvatarDataURL = *req.AvatarDataURL
		}
		if existing, ok := s.app.Config.FindAgentProfile(profile.Name); ok {
			if req.AvatarPreset == nil {
				profile.AvatarPreset = existing.AvatarPreset
			}
			if req.AvatarDataURL == nil {
				profile.AvatarDataURL = existing.AvatarDataURL
			}
			profile.Domain = existing.Domain
			profile.Expertise = append([]string{}, existing.Expertise...)
			profile.SystemPrompt = existing.SystemPrompt
			if strings.TrimSpace(profile.Personality.Template) == "" &&
				strings.TrimSpace(profile.Personality.Tone) == "" &&
				strings.TrimSpace(profile.Personality.Style) == "" &&
				strings.TrimSpace(profile.Personality.GoalOrientation) == "" &&
				strings.TrimSpace(profile.Personality.ConstraintMode) == "" &&
				strings.TrimSpace(profile.Personality.ResponseVerbosity) == "" &&
				strings.TrimSpace(profile.Personality.CustomInstructions) == "" &&
				len(profile.Personality.Traits) == 0 {
				profile.Personality = existing.Personality
			}
		}
		if profile.Enabled == nil {
			profile.Enabled = config.BoolPtr(true)
		}
		if strings.TrimSpace(profile.PermissionLevel) == "" {
			profile.PermissionLevel = "limited"
		}
		if strings.TrimSpace(profile.ProviderRef) != "" {
			if _, ok := s.app.Config.FindProviderProfile(profile.ProviderRef); !ok {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider not found"})
				return
			}
		}
		if err := s.app.Config.UpsertAgentProfile(profile); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := s.app.Config.Save(s.app.ConfigPath); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		s.appendAudit(UserFromContext(r.Context()), "agents.write", profile.Name, map[string]any{"enabled": profile.IsEnabled()})
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	case http.MethodDelete:
		if !HasPermission(UserFromContext(r.Context()), "config.write") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "config.write"})
			return
		}
		name := strings.TrimSpace(r.URL.Query().Get("name"))
		if name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
			return
		}
		if !s.app.Config.DeleteAgentProfile(name) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
			return
		}
		if err := s.app.Config.Save(s.app.ConfigPath); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		s.appendAudit(UserFromContext(r.Context()), "agents.delete", name, nil)
		writeJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAssistants(w http.ResponseWriter, r *http.Request) {
	s.handleAgents(w, r)
}

func (s *Server) handleRuntimes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.runtimePool == nil {
		writeJSON(w, http.StatusOK, []RuntimeInfo{})
		return
	}
	s.appendAudit(UserFromContext(r.Context()), "runtimes.read", "runtimes", nil)
	writeJSON(w, http.StatusOK, s.runtimePool.List())
}

func (s *Server) handlePersonalityTemplates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, builtinPersonalityTemplates)
}

func (s *Server) handleAssistantSkillCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	entries := s.app.Skills.Catalog()
	writeJSON(w, http.StatusOK, entries)
}

func (s *Server) handleRefreshRuntime(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Agent     string `json:"agent"`
		Org       string `json:"org"`
		Project   string `json:"project"`
		Workspace string `json:"workspace"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	s.runtimePool.Refresh(req.Agent, req.Org, req.Project, req.Workspace)
	s.appendAudit(UserFromContext(r.Context()), "runtimes.refresh", req.Workspace, map[string]any{"agent": req.Agent, "org": req.Org, "project": req.Project})
	writeJSON(w, http.StatusOK, map[string]any{"status": "refreshed"})
}

func (s *Server) handleRefreshRuntimesBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Items []struct {
			Agent     string `json:"agent"`
			Org       string `json:"org"`
			Project   string `json:"project"`
			Workspace string `json:"workspace"`
		} `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	payload := map[string]any{"items": req.Items}
	job := &Job{ID: uniqueID("job"), Kind: "runtimes.refresh.batch", Status: "queued", Summary: fmt.Sprintf("Refreshing %d runtimes", len(req.Items)), CreatedAt: time.Now().UTC(), Payload: payload, MaxAttempts: s.jobMaxAttempts}
	job.Cancellable = true
	job.Retriable = true
	_ = s.store.AppendJob(job)
	s.jobQueue <- func() {
		if s.shouldCancelJob(job.ID) {
			return
		}
		job.Attempts++
		job.Status = "running"
		job.StartedAt = time.Now().UTC().Format(time.RFC3339)
		_ = s.store.UpdateJob(job)
		results := make([]map[string]any, 0, len(req.Items))
		failedCount := 0
		for _, item := range req.Items {
			if s.shouldCancelJob(job.ID) {
				job.Status = "cancelled"
				job.CompletedAt = time.Now().UTC().Format(time.RFC3339)
				job.Cancellable = false
				job.Retriable = true
				job.Details = map[string]any{"results": results}
				_ = s.store.UpdateJob(job)
				return
			}
			status := map[string]any{"agent": item.Agent, "org": item.Org, "project": item.Project, "workspace": item.Workspace, "status": "refreshed"}
			if strings.TrimSpace(item.Workspace) == "" {
				status["status"] = "failed"
				status["error"] = "workspace is required"
				failedCount++
			} else {
				s.runtimePool.Refresh(item.Agent, item.Org, item.Project, item.Workspace)
			}
			results = append(results, status)
		}
		if failedCount == len(req.Items) && len(req.Items) > 0 {
			job.Status = "failed"
			job.Error = "all runtime refresh items failed"
			if job.Attempts < job.MaxAttempts {
				job.Status = "queued"
			}
		} else {
			job.Status = "completed"
		}
		job.CompletedAt = time.Now().UTC().Format(time.RFC3339)
		job.Cancellable = false
		job.Retriable = true
		job.Details = map[string]any{"results": results, "failed_count": failedCount}
		_ = s.store.UpdateJob(job)
	}
	s.appendAudit(UserFromContext(r.Context()), "runtimes.refresh.batch", "runtimes", map[string]any{"count": len(req.Items)})
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "queued", "job_id": job.ID, "count": len(req.Items)})
}

func (s *Server) handleRuntimeMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.runtimePool.Metrics())
}

func (s *Server) handleControlPlane(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.appendAudit(UserFromContext(r.Context()), "control-plane.read", "control-plane", nil)
	writeJSON(w, http.StatusOK, s.controlPlaneSnapshot())
}

func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if !HasPermission(UserFromContext(r.Context()), "auth.users.read") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "auth.users.read"})
			return
		}
		rolesIndex := map[string]config.SecurityRole{}
		for _, role := range s.app.Config.Security.Roles {
			rolesIndex[role.Name] = role
		}
		for _, role := range builtinRoleTemplates() {
			rolesIndex[role.Name] = role
		}
		type view struct {
			Name                string   `json:"name"`
			Role                string   `json:"role"`
			Permissions         []string `json:"permissions"`
			PermissionOverrides []string `json:"permission_overrides"`
			Scopes              []string `json:"scopes"`
			Orgs                []string `json:"orgs"`
			Projects            []string `json:"projects"`
			Workspaces          []string `json:"workspaces"`
		}
		items := make([]view, 0, len(s.app.Config.Security.Users))
		for _, user := range s.app.Config.Security.Users {
			effective := append([]string{}, user.PermissionOverrides...)
			if role, ok := rolesIndex[user.Role]; ok {
				effective = append(append([]string{}, role.Permissions...), user.PermissionOverrides...)
			}
			items = append(items, view{Name: user.Name, Role: user.Role, Permissions: effective, PermissionOverrides: user.PermissionOverrides, Scopes: user.Scopes, Orgs: user.Orgs, Projects: user.Projects, Workspaces: user.Workspaces})
		}
		s.appendAudit(UserFromContext(r.Context()), "auth.users.read", "users", nil)
		writeJSON(w, http.StatusOK, items)
	case http.MethodPost:
		if !HasPermission(UserFromContext(r.Context()), "auth.users.read") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "auth.users.read"})
			return
		}
		var user config.SecurityUser
		if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		if strings.TrimSpace(user.Name) == "" || strings.TrimSpace(user.Token) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and token are required"})
			return
		}
		allowedPermissions := map[string]bool{
			"*":               true,
			"status.read":     true,
			"chat.send":       true,
			"tasks.read":      true,
			"tasks.write":     true,
			"approvals.read":  true,
			"approvals.write": true,
			"sessions.read":   true,
			"sessions.write":  true,
			"memory.read":     true,
			"events.read":     true,
			"tools.read":      true,
			"plugins.read":    true,
			"apps.read":       true,
			"apps.write":      true,
			"channels.read":   true,
			"routing.read":    true,
			"runtimes.read":   true,
			"runtimes.write":  true,
			"resources.read":  true,
			"resources.write": true,
			"config.read":     true,
			"config.write":    true,
			"audit.read":      true,
			"auth.users.read": true,
		}
		for _, permission := range user.Permissions {
			_ = permission
		}
		for _, permission := range user.PermissionOverrides {
			if !allowedPermissions[permission] {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown permission", "permission": permission})
				return
			}
		}
		for _, existing := range s.app.Config.Security.Users {
			if existing.Name != user.Name && existing.Token == user.Token {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "token already in use"})
				return
			}
		}
		updated := false
		for i := range s.app.Config.Security.Users {
			if s.app.Config.Security.Users[i].Name == user.Name {
				s.app.Config.Security.Users[i] = user
				updated = true
				break
			}
		}
		if !updated {
			s.app.Config.Security.Users = append(s.app.Config.Security.Users, user)
		}
		if err := s.app.Config.Save(s.app.ConfigPath); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		s.appendAudit(UserFromContext(r.Context()), "auth.users.write", user.Name, nil)
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	case http.MethodDelete:
		if !HasPermission(UserFromContext(r.Context()), "auth.users.read") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "auth.users.read"})
			return
		}
		name := strings.TrimSpace(r.URL.Query().Get("name"))
		if name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
			return
		}
		filtered := make([]config.SecurityUser, 0, len(s.app.Config.Security.Users))
		removed := false
		for _, user := range s.app.Config.Security.Users {
			if user.Name == name {
				removed = true
				continue
			}
			filtered = append(filtered, user)
		}
		if !removed {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
			return
		}
		s.app.Config.Security.Users = filtered
		if err := s.app.Config.Save(s.app.ConfigPath); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		s.appendAudit(UserFromContext(r.Context()), "auth.users.delete", name, nil)
		writeJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleRoles(w http.ResponseWriter, r *http.Request) {
	builtinRoles := make([]map[string]any, 0, len(builtinRoleTemplates()))
	for _, role := range builtinRoleTemplates() {
		builtinRoles = append(builtinRoles, map[string]any{
			"name":        role.Name,
			"description": role.Description,
			"permissions": role.Permissions,
		})
	}
	switch r.Method {
	case http.MethodGet:
		if !HasPermission(UserFromContext(r.Context()), "auth.users.read") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "auth.users.read"})
			return
		}
		roles := append([]map[string]any{}, builtinRoles...)
		for _, role := range s.app.Config.Security.Roles {
			roles = append(roles, map[string]any{"name": role.Name, "description": role.Description, "permissions": role.Permissions, "custom": true})
		}
		writeJSON(w, http.StatusOK, roles)
	case http.MethodPost:
		if !HasPermission(UserFromContext(r.Context()), "auth.users.read") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "auth.users.read"})
			return
		}
		var role config.SecurityRole
		if err := json.NewDecoder(r.Body).Decode(&role); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		if strings.TrimSpace(role.Name) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "role name is required"})
			return
		}
		updated := false
		for i := range s.app.Config.Security.Roles {
			if s.app.Config.Security.Roles[i].Name == role.Name {
				s.app.Config.Security.Roles[i] = role
				updated = true
				break
			}
		}
		if !updated {
			s.app.Config.Security.Roles = append(s.app.Config.Security.Roles, role)
		}
		if err := s.app.Config.Save(s.app.ConfigPath); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		s.appendAudit(UserFromContext(r.Context()), "auth.roles.write", role.Name, nil)
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	case http.MethodDelete:
		if !HasPermission(UserFromContext(r.Context()), "auth.users.read") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "auth.users.read"})
			return
		}
		name := strings.TrimSpace(r.URL.Query().Get("name"))
		if name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
			return
		}
		filtered := make([]config.SecurityRole, 0, len(s.app.Config.Security.Roles))
		removed := false
		for _, role := range s.app.Config.Security.Roles {
			if role.Name == name {
				removed = true
				continue
			}
			filtered = append(filtered, role)
		}
		if !removed {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "role not found"})
			return
		}
		s.app.Config.Security.Roles = filtered
		if err := s.app.Config.Save(s.app.ConfigPath); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		s.appendAudit(UserFromContext(r.Context()), "auth.roles.delete", name, nil)
		writeJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func builtinRoleTemplates() []config.SecurityRole {
	return []config.SecurityRole{
		{Name: "admin", Description: "Full platform access", Permissions: []string{"*"}},
		{Name: "operator", Description: "Operate sessions, runtimes, and workspace resources", Permissions: []string{"status.read", "chat.send", "tasks.read", "tasks.write", "approvals.read", "approvals.write", "sessions.read", "sessions.write", "memory.read", "runtimes.read", "runtimes.write", "events.read", "tools.read", "resources.read", "resources.write", "apps.read", "apps.write"}},
		{Name: "viewer", Description: "Read-only governance and monitoring", Permissions: []string{"status.read", "sessions.read", "events.read", "audit.read", "plugins.read", "apps.read", "channels.read", "routing.read", "runtimes.read", "resources.read"}},
	}
}

func (s *Server) handleRoleImpact(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	roles := []config.SecurityRole{}
	roles = append(roles, builtinRoleTemplates()...)
	roles = append(roles, s.app.Config.Security.Roles...)
	impact := make([]map[string]any, 0, len(roles))
	for _, role := range roles {
		users := []string{}
		for _, user := range s.app.Config.Security.Users {
			if user.Role == role.Name {
				users = append(users, user.Name)
			}
		}
		impact = append(impact, map[string]any{
			"name":        role.Name,
			"description": role.Description,
			"permissions": role.Permissions,
			"user_count":  len(users),
			"users":       users,
		})
	}
	writeJSON(w, http.StatusOK, impact)
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.appendAudit(UserFromContext(r.Context()), "audit.read", "audit", nil)
	writeJSON(w, http.StatusOK, s.store.ListAudit(100))
}

func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.appendAudit(UserFromContext(r.Context()), "jobs.read", "jobs", nil)
	writeJSON(w, http.StatusOK, s.store.ListJobs(100))
}

func (s *Server) handleJobByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/jobs/")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	job, ok := s.store.GetJob(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
		return
	}
	s.appendAudit(UserFromContext(r.Context()), "jobs.detail.read", id, nil)
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) handleCancelJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		JobID string `json:"job_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	job, ok := s.store.GetJob(req.JobID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
		return
	}
	if job.Status == "completed" || job.Status == "failed" || job.Status == "cancelled" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "job is not cancellable"})
		return
	}
	s.jobCancel[job.ID] = true
	job.Status = "cancelled"
	job.CompletedAt = time.Now().UTC().Format(time.RFC3339)
	job.Cancellable = false
	job.Retriable = true
	_ = s.store.UpdateJob(job)
	s.appendAudit(UserFromContext(r.Context()), "jobs.cancel", job.ID, nil)
	writeJSON(w, http.StatusOK, map[string]any{"status": "cancelled"})
}

func (s *Server) handleRetryJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		JobID string `json:"job_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	job, ok := s.store.GetJob(req.JobID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
		return
	}
	if !job.Retriable {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "job is not retriable"})
		return
	}
	clone := &Job{ID: uniqueID("job"), Kind: job.Kind, Status: "queued", Summary: job.Summary + " (retry)", CreatedAt: time.Now().UTC(), RetryOf: job.ID, Cancellable: true, Retriable: true, Payload: job.Payload}
	_ = s.store.AppendJob(clone)
	s.enqueueJobFromPayload(clone)
	s.appendAudit(UserFromContext(r.Context()), "jobs.retry", job.ID, map[string]any{"new_job": clone.ID})
	writeJSON(w, http.StatusOK, map[string]any{"status": "queued", "job_id": clone.ID})
}

func (s *Server) enqueueJobFromPayload(job *Job) {
	if job == nil {
		return
	}
	switch job.Kind {
	case "runtimes.refresh.batch":
		rawItems, _ := job.Payload["items"].([]any)
		items := make([]struct {
			Agent     string `json:"agent"`
			Org       string `json:"org"`
			Project   string `json:"project"`
			Workspace string `json:"workspace"`
		}, 0, len(rawItems))
		for _, raw := range rawItems {
			m, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			items = append(items, struct {
				Agent     string `json:"agent"`
				Org       string `json:"org"`
				Project   string `json:"project"`
				Workspace string `json:"workspace"`
			}{Agent: fmt.Sprint(m["Agent"], m["agent"]), Org: fmt.Sprint(m["Org"], m["org"]), Project: fmt.Sprint(m["Project"], m["project"]), Workspace: fmt.Sprint(m["Workspace"], m["workspace"])})
		}
		s.jobQueue <- func() {
			job.Status = "running"
			job.StartedAt = time.Now().UTC().Format(time.RFC3339)
			_ = s.store.UpdateJob(job)
			results := make([]map[string]any, 0, len(items))
			failedCount := 0
			for _, item := range items {
				if strings.TrimSpace(item.Workspace) == "" {
					results = append(results, map[string]any{"agent": item.Agent, "org": item.Org, "project": item.Project, "workspace": item.Workspace, "status": "failed", "error": "workspace is required"})
					failedCount++
					continue
				}
				s.runtimePool.Refresh(item.Agent, item.Org, item.Project, item.Workspace)
				results = append(results, map[string]any{"agent": item.Agent, "org": item.Org, "project": item.Project, "workspace": item.Workspace, "status": "refreshed"})
			}
			if failedCount == len(items) && len(items) > 0 {
				job.Status = "failed"
				job.Error = "all runtime refresh items failed"
			} else {
				job.Status = "completed"
			}
			job.CompletedAt = time.Now().UTC().Format(time.RFC3339)
			job.Cancellable = false
			job.Details = map[string]any{"results": results, "failed_count": failedCount}
			_ = s.store.UpdateJob(job)
		}
	case "sessions.move.batch":
		rawIDs, _ := job.Payload["session_ids"].([]any)
		sessionIDs := make([]string, 0, len(rawIDs))
		for _, raw := range rawIDs {
			sessionIDs = append(sessionIDs, fmt.Sprint(raw))
		}
		orgID := fmt.Sprint(job.Payload["org"])
		projectID := fmt.Sprint(job.Payload["project"])
		workspaceID := fmt.Sprint(job.Payload["workspace"])
		agent := fmt.Sprint(job.Payload["agent"])
		org, project, workspace, err := s.validateResourceSelection(orgID, projectID, workspaceID)
		if err != nil {
			job.Status = "failed"
			job.CompletedAt = time.Now().UTC().Format(time.RFC3339)
			job.Error = err.Error()
			_ = s.store.UpdateJob(job)
			return
		}
		s.jobQueue <- func() {
			job.Status = "running"
			job.StartedAt = time.Now().UTC().Format(time.RFC3339)
			_ = s.store.UpdateJob(job)
			updatedCount := 0
			failedCount := 0
			results := make([]map[string]any, 0, len(sessionIDs))
			for _, sessionID := range sessionIDs {
				if _, err := s.sessions.MoveSession(sessionID, org.ID, project.ID, workspace.ID, agent); err == nil {
					updatedCount++
					results = append(results, map[string]any{"session_id": sessionID, "status": "moved"})
				} else {
					failedCount++
					results = append(results, map[string]any{"session_id": sessionID, "status": "failed", "error": err.Error()})
				}
			}
			if updatedCount > 0 {
				s.runtimePool.InvalidateByWorkspace(workspace.ID)
			}
			if failedCount == len(sessionIDs) && len(sessionIDs) > 0 {
				job.Status = "failed"
				job.Error = "all session move items failed"
			} else {
				job.Status = "completed"
			}
			job.CompletedAt = time.Now().UTC().Format(time.RFC3339)
			job.Cancellable = false
			job.Details = map[string]any{"results": results, "target_workspace": workspace.ID, "failed_count": failedCount}
			_ = s.store.UpdateJob(job)
		}
	}
}

func (s *Server) handleResources(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		if !HasPermission(UserFromContext(r.Context()), "resources.read") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "resources.read"})
			return
		}
		s.appendAudit(UserFromContext(r.Context()), "resources.read", "resources", nil)
		writeJSON(w, http.StatusOK, map[string]any{
			"orgs":       s.store.ListOrgs(),
			"projects":   s.store.ListProjects(),
			"workspaces": s.store.ListWorkspaces(),
		})
		return
	}
	if r.Method == http.MethodPost {
		if !HasPermission(UserFromContext(r.Context()), "resources.write") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "resources.write"})
			return
		}
		var req struct {
			Org       *Org       `json:"org"`
			Project   *Project   `json:"project"`
			Workspace *Workspace `json:"workspace"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		if req.Org != nil {
			if err := s.store.UpsertOrg(req.Org); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
		}
		if req.Project != nil {
			if err := s.store.UpsertProject(req.Project); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
		}
		if req.Workspace != nil {
			if err := s.store.UpsertWorkspace(req.Workspace); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
		}
		s.appendAudit(UserFromContext(r.Context()), "resources.write", "resources", nil)
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
		return
	}
	if r.Method == http.MethodPatch {
		if !HasPermission(UserFromContext(r.Context()), "resources.write") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "resources.write"})
			return
		}
		var req struct {
			Org       *Org       `json:"org"`
			Project   *Project   `json:"project"`
			Workspace *Workspace `json:"workspace"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		if req.Org != nil {
			if err := s.store.UpsertOrg(req.Org); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
		}
		if req.Project != nil {
			if err := s.store.UpsertProject(req.Project); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			if err := s.store.RebindSessionsForProject(req.Project.ID, req.Project.OrgID); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			s.runtimePool.InvalidateByProject(req.Project.ID)
			s.appendAudit(UserFromContext(r.Context()), "runtimes.invalidate", req.Project.ID, map[string]any{"reason": "project update"})
		}
		if req.Workspace != nil {
			if err := s.store.UpsertWorkspace(req.Workspace); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			project, ok := s.store.GetProject(req.Workspace.ProjectID)
			if ok {
				if err := s.store.RebindSessionsForWorkspace(req.Workspace.ID, project.ID, project.OrgID); err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
					return
				}
			}
			s.runtimePool.InvalidateByWorkspace(req.Workspace.ID)
			s.appendAudit(UserFromContext(r.Context()), "runtimes.invalidate", req.Workspace.ID, map[string]any{"reason": "workspace update"})
		}
		s.appendAudit(UserFromContext(r.Context()), "resources.update", "resources", nil)
		writeJSON(w, http.StatusOK, map[string]any{"status": "updated"})
		return
	}
	if r.Method == http.MethodDelete {
		if !HasPermission(UserFromContext(r.Context()), "resources.write") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "resources.write"})
			return
		}
		kind := strings.TrimSpace(r.URL.Query().Get("kind"))
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		if kind == "" || id == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "kind and id are required"})
			return
		}
		var err error
		switch kind {
		case "org":
			err = s.store.DeleteOrg(id)
		case "project":
			err = s.store.DeleteProject(id)
		case "workspace":
			err = s.store.DeleteWorkspace(id)
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported resource kind"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		s.appendAudit(UserFromContext(r.Context()), "resources.delete", kind+":"+id, nil)
		writeJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
		return
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func (s *Server) handleSignedIngress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	secret := strings.TrimSpace(s.app.Config.Security.WebhookSecret)
	if secret == "" {
		http.Error(w, "webhook secret not configured", http.StatusForbidden)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	provided := strings.TrimSpace(r.Header.Get("X-AnyClaw-Signature"))
	if !verifySignature(secret, body, provided) {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}
	var req struct {
		Message   string `json:"message"`
		SessionID string `json:"session_id"`
		Title     string `json:"title"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message is required"})
		return
	}
	response, session, err := s.runSessionMessage(r.Context(), req.SessionID, req.Title, req.Message)
	if err != nil {
		if errors.Is(err, ErrTaskWaitingApproval) {
			s.appendAudit(UserFromContext(r.Context()), "ingress.web.accepted", req.SessionID, map[string]any{"status": "waiting_approval"})
			writeJSON(w, http.StatusAccepted, s.sessionApprovalResponse(req.SessionID))
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.appendEvent("ingress.web.accepted", session.ID, map[string]any{"signed": true})
	s.appendAudit(UserFromContext(r.Context()), "ingress.web.accepted", session.ID, nil)
	writeJSON(w, http.StatusOK, map[string]any{"response": response, "session": session})
}

func (s *Server) handlePluginIngress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	pluginName := strings.TrimPrefix(r.URL.Path, "/ingress/plugins/")
	if pluginName == "" {
		http.NotFound(w, r)
		return
	}
	var runner *plugin.IngressRunner
	for i := range s.ingressPlugins {
		if s.ingressPlugins[i].Manifest.Name == pluginName {
			runner = &s.ingressPlugins[i]
			break
		}
	}
	if runner == nil {
		http.NotFound(w, r)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), runner.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, runner.Entrypoint)
	pluginDir := filepath.Dir(runner.Entrypoint)
	cmd.Dir = pluginDir
	cmd.Env = append(os.Environ(),
		"ANYCLAW_PLUGIN_INPUT="+string(body),
		"ANYCLAW_PLUGIN_DIR="+pluginDir,
		"ANYCLAW_PLUGIN_TIMEOUT_SECONDS="+fmt.Sprintf("%d", int(runner.Timeout/time.Second)),
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			writeJSON(w, http.StatusGatewayTimeout, map[string]string{"error": "plugin ingress timed out"})
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("plugin ingress failed: %s", string(output))})
		return
	}
	s.appendEvent("ingress.plugin.accepted", "", map[string]any{"plugin": runner.Manifest.Name})
	s.appendAudit(UserFromContext(r.Context()), "ingress.plugin.accepted", runner.Manifest.Name, nil)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(output)
}

func verifySignature(secret string, body []byte, provided string) bool {
	if strings.TrimSpace(provided) == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	expected := fmt.Sprintf("sha256=%x", mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(strings.TrimSpace(provided)))
}

func StartDetached(app *runtime.App) error {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if _, err := Probe(ctx, runtime.GatewayURL(app.Config)); err == nil {
		return fmt.Errorf("gateway already running at %s", runtime.GatewayURL(app.Config))
	}
	logPath := app.Config.Daemon.LogFile
	if logPath == "" {
		logPath = filepath.Join(app.WorkDir, "gateway.log")
	}
	pidPath := app.Config.Daemon.PIDFile
	if pidPath == "" {
		pidPath = filepath.Join(app.WorkDir, "gateway.pid")
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(pidPath), 0o755); err != nil {
		return err
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	cmd := exec.Command(os.Args[0], "gateway", "run", "--config", app.ConfigPath)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		return err
	}
	startCtx, startCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer startCancel()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		probeCtx, probeCancel := context.WithTimeout(startCtx, time.Second)
		_, err := Probe(probeCtx, runtime.GatewayURL(app.Config))
		probeCancel()
		if err == nil {
			pidData := []byte(strconv.Itoa(cmd.Process.Pid))
			return os.WriteFile(pidPath, pidData, 0o644)
		}
		select {
		case <-startCtx.Done():
			return fmt.Errorf("gateway daemon failed to start within 5s; see %s", logPath)
		case <-ticker.C:
		}
	}
}

func StopDetached(app *runtime.App) error {
	pidPath := app.Config.Daemon.PIDFile
	if pidPath == "" {
		pidPath = filepath.Join(app.WorkDir, "gateway.pid")
	}
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return err
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := process.Kill(); err != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if _, probeErr := Probe(ctx, runtime.GatewayURL(app.Config)); probeErr != nil {
			_ = os.Remove(pidPath)
			return nil
		}
		return err
	}
	_ = os.Remove(pidPath)
	return nil
}

func (s *Server) handleSkills(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.app.Agent.ListSkills())
}

func (s *Server) handleTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.app.Agent.ListTools())
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		workspace := strings.TrimSpace(r.URL.Query().Get("workspace"))
		sessions := s.store.ListSessions()
		if workspace != "" {
			filtered := make([]*Session, 0, len(sessions))
			for _, session := range sessions {
				if session.Workspace == workspace {
					filtered = append(filtered, session)
				}
			}
			sessions = filtered
		}
		writeJSON(w, http.StatusOK, sessions)
	case http.MethodPost:
		var req struct {
			Title        string   `json:"title"`
			Agent        string   `json:"agent"`
			Assistant    string   `json:"assistant"`
			Participants []string `json:"participants"`
			SessionMode  string   `json:"session_mode"`
			QueueMode    string   `json:"queue_mode"`
			ReplyBack    bool     `json:"reply_back"`
			IsGroup      bool     `json:"is_group"`
			GroupKey     string   `json:"group_key"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, context.Canceled) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		agentName, err := s.resolveAgentName(requestedAgentName(req.Agent, req.Assistant))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		resolvedParticipants := make([]string, 0, len(req.Participants))
		seenParticipants := map[string]bool{}
		for _, name := range req.Participants {
			resolvedName, err := s.resolveAgentName(name)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			if resolvedName == "" || seenParticipants[resolvedName] {
				continue
			}
			seenParticipants[resolvedName] = true
			resolvedParticipants = append(resolvedParticipants, resolvedName)
		}
		resolvedParticipants = normalizeParticipants(agentName, resolvedParticipants)
		if req.IsGroup || strings.TrimSpace(req.GroupKey) != "" || isGroupSessionMode(req.SessionMode) || len(resolvedParticipants) > 1 {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "multi-agent session creation is not supported on /sessions; use single-agent sessions only",
			})
			return
		}
		orgID, projectID, workspaceID := s.resolveResourceSelection(r)
		org, project, workspace, err := s.validateResourceSelection(orgID, projectID, workspaceID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		session, err := s.sessions.CreateWithOptions(SessionCreateOptions{
			Title:       req.Title,
			AgentName:   agentName,
			Org:         org.ID,
			Project:     project.ID,
			Workspace:   workspace.ID,
			SessionMode: normalizeSingleAgentSessionMode(req.SessionMode, "main"),
			QueueMode:   req.QueueMode,
			ReplyBack:   req.ReplyBack,
		})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		s.appendEvent("session.created", session.ID, map[string]any{"title": session.Title})
		writeJSON(w, http.StatusCreated, session)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func normalizeSingleAgentSessionMode(mode string, fallback string) string {
	mode = strings.TrimSpace(strings.ToLower(mode))
	if mode == "" {
		return fallback
	}
	if isGroupSessionMode(mode) {
		return fallback
	}
	return mode
}

func isGroupSessionMode(mode string) bool {
	switch strings.TrimSpace(strings.ToLower(mode)) {
	case "group", "group-shared", "channel-group":
		return true
	default:
		return false
	}
}

func (s *Server) handleSessionByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/sessions/")
	if id == "" {
		http.Error(w, "session id required", http.StatusBadRequest)
		return
	}
	session, ok := s.sessions.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
		return
	}
	writeJSON(w, http.StatusOK, session)
}

func (s *Server) handleMoveSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		SessionID string `json:"session_id"`
		Org       string `json:"org"`
		Project   string `json:"project"`
		Workspace string `json:"workspace"`
		Agent     string `json:"agent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	org, project, workspace, err := s.validateResourceSelection(req.Org, req.Project, req.Workspace)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	updated, err := s.sessions.MoveSession(req.SessionID, org.ID, project.ID, workspace.ID, req.Agent)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.runtimePool.InvalidateByWorkspace(workspace.ID)
	s.appendAudit(UserFromContext(r.Context()), "runtimes.invalidate", workspace.ID, map[string]any{"reason": "session move"})
	s.appendAudit(UserFromContext(r.Context()), "sessions.move", req.SessionID, map[string]any{"org": org.ID, "project": project.ID, "workspace": workspace.ID, "agent": req.Agent})
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleMoveSessionsBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		SessionIDs []string `json:"session_ids"`
		Org        string   `json:"org"`
		Project    string   `json:"project"`
		Workspace  string   `json:"workspace"`
		Agent      string   `json:"agent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	org, project, workspace, err := s.validateResourceSelection(req.Org, req.Project, req.Workspace)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	payload := map[string]any{"session_ids": req.SessionIDs, "org": org.ID, "project": project.ID, "workspace": workspace.ID, "agent": req.Agent}
	job := &Job{ID: uniqueID("job"), Kind: "sessions.move.batch", Status: "queued", Summary: fmt.Sprintf("Moving %d sessions", len(req.SessionIDs)), CreatedAt: time.Now().UTC(), Payload: payload, MaxAttempts: s.jobMaxAttempts}
	job.Cancellable = true
	job.Retriable = true
	_ = s.store.AppendJob(job)
	s.jobQueue <- func() {
		if s.shouldCancelJob(job.ID) {
			return
		}
		job.Attempts++
		job.Status = "running"
		job.StartedAt = time.Now().UTC().Format(time.RFC3339)
		_ = s.store.UpdateJob(job)
		updatedCount := 0
		failedCount := 0
		results := make([]map[string]any, 0, len(req.SessionIDs))
		for _, sessionID := range req.SessionIDs {
			if s.shouldCancelJob(job.ID) {
				job.Status = "cancelled"
				job.CompletedAt = time.Now().UTC().Format(time.RFC3339)
				job.Cancellable = false
				job.Retriable = true
				job.Details = map[string]any{"results": results, "target_workspace": workspace.ID}
				_ = s.store.UpdateJob(job)
				return
			}
			if _, err := s.sessions.MoveSession(sessionID, org.ID, project.ID, workspace.ID, req.Agent); err == nil {
				updatedCount++
				results = append(results, map[string]any{"session_id": sessionID, "status": "moved"})
			} else {
				failedCount++
				results = append(results, map[string]any{"session_id": sessionID, "status": "failed", "error": err.Error()})
			}
		}
		if updatedCount > 0 {
			s.runtimePool.InvalidateByWorkspace(workspace.ID)
		}
		if failedCount == len(req.SessionIDs) && len(req.SessionIDs) > 0 {
			job.Status = "failed"
			job.Error = "all session move items failed"
			if job.Attempts < job.MaxAttempts {
				job.Status = "queued"
			}
		} else {
			job.Status = "completed"
		}
		job.CompletedAt = time.Now().UTC().Format(time.RFC3339)
		job.Cancellable = false
		job.Retriable = true
		job.Details = map[string]any{"results": results, "target_workspace": workspace.ID, "failed_count": failedCount}
		_ = s.store.UpdateJob(job)
		s.appendAudit(UserFromContext(r.Context()), "sessions.move.batch", workspace.ID, map[string]any{"count": updatedCount, "agent": req.Agent})
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "queued", "job_id": job.ID, "count": len(req.SessionIDs)})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	writeJSON(w, http.StatusOK, s.store.ListEvents(limit))
}

func (s *Server) handleEventStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	limit := 20
	if raw := strings.TrimSpace(r.URL.Query().Get("replay")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed >= 0 {
			limit = parsed
		}
	}
	filterSessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))

	for _, event := range s.store.ListEvents(limit) {
		if filterSessionID != "" && event.SessionID != filterSessionID {
			continue
		}
		if err := writeSSEEvent(w, event); err != nil {
			return
		}
	}
	flusher.Flush()

	ch := s.bus.Subscribe(32)
	defer s.bus.Unsubscribe(ch)

	pingTicker := time.NewTicker(15 * time.Second)
	defer pingTicker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-pingTicker.C:
			_, _ = fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		case event := <-ch:
			if event == nil {
				continue
			}
			if filterSessionID != "" && event.SessionID != filterSessionID {
				continue
			}
			if err := writeSSEEvent(w, event); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func writeSSEEvent(w http.ResponseWriter, event *Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "id: %s\n", event.ID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\n", event.Type); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		return err
	}
	return nil
}

func (s *Server) handleV2Agents(w http.ResponseWriter, r *http.Request) {
	if s.taskModule == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "task module not available"})
		return
	}

	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	agents := s.taskModule.ListAgents()
	writeJSON(w, http.StatusOK, agents)
}

func (s *Server) handleV2Tasks(w http.ResponseWriter, r *http.Request) {
	if s.taskModule == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "task module not available"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		tasks := s.taskModule.ListTasks()
		writeJSON(w, http.StatusOK, tasks)

	case http.MethodPost:
		var req struct {
			Title          string   `json:"title"`
			Input          string   `json:"input"`
			Mode           string   `json:"mode"`
			SelectedAgent  string   `json:"selected_agent"`
			SelectedAgents []string `json:"selected_agents"`
			Sync           bool     `json:"sync"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}

		if strings.TrimSpace(req.Input) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "input is required"})
			return
		}

		mode := taskModule.ExecutionMode(req.Mode)
		if mode == "" {
			mode = taskModule.ModeSingle
		}
		if mode != taskModule.ModeSingle && mode != taskModule.ModeMulti {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "mode must be 'single' or 'multi'"})
			return
		}

		taskReq := taskModule.TaskRequest{
			Title:          req.Title,
			Input:          req.Input,
			Mode:           mode,
			SelectedAgent:  req.SelectedAgent,
			SelectedAgents: req.SelectedAgents,
		}

		taskResp, err := s.taskModule.CreateTask(taskReq)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		if req.Sync {
			result, err := s.taskModule.ExecuteTask(r.Context(), taskResp.ID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
					"task":  result,
					"error": err.Error(),
				})
				return
			}
			writeJSON(w, http.StatusOK, result)
			return
		}

		go func() {
			ctx := context.Background()
			_, _ = s.taskModule.ExecuteTask(ctx, taskResp.ID)
		}()

		writeJSON(w, http.StatusAccepted, taskResp)

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (s *Server) handleV2TaskByID(w http.ResponseWriter, r *http.Request) {
	if s.taskModule == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "task module not available"})
		return
	}

	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	taskID := strings.TrimPrefix(r.URL.Path, "/v2/tasks/")
	if taskID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "task id required"})
		return
	}

	taskResp, err := s.taskModule.GetTask(taskID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, taskResp)
}

func (s *Server) handleV2Chat(w http.ResponseWriter, r *http.Request) {
	if s.chatModule == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "chat not available"})
		return
	}

	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	var req chat.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	if strings.TrimSpace(req.AgentName) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent_name is required"})
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message is required"})
		return
	}

	resp, err := s.chatModule.Chat(r.Context(), req)
	if err != nil {
		code := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			code = http.StatusNotFound
		}
		writeJSON(w, code, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleV2ChatSessions(w http.ResponseWriter, r *http.Request) {
	if s.chatModule == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "chat not available"})
		return
	}

	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	sessions := s.chatModule.ListSessions()
	writeJSON(w, http.StatusOK, sessions)
}

func (s *Server) handleV2ChatSessionByID(w http.ResponseWriter, r *http.Request) {
	if s.chatModule == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "chat not available"})
		return
	}

	sessionID := strings.TrimPrefix(r.URL.Path, "/v2/chat/sessions/")
	if sessionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session id required"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		history, err := s.chatModule.GetSessionHistory(sessionID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, history)

	case http.MethodDelete:
		if err := s.chatModule.DeleteSession(sessionID); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (s *Server) handleV2Store(w http.ResponseWriter, r *http.Request) {
	if s.storeModule == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "store not available"})
		return
	}

	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	filter := agentstore.StoreFilter{
		Category: r.URL.Query().Get("category"),
		Tag:      r.URL.Query().Get("tag"),
		Keyword:  r.URL.Query().Get("q"),
	}

	if installedStr := r.URL.Query().Get("installed"); installedStr != "" {
		installed := installedStr == "true"
		filter.Installed = &installed
	}

	packages := s.storeModule.List(filter)
	writeJSON(w, http.StatusOK, packages)
}

func (s *Server) handleV2StoreByID(w http.ResponseWriter, r *http.Request) {
	if s.storeModule == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "store not available"})
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/v2/store/")
	if id == "" {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"categories": s.storeModule.GetCategories(),
			"tags":       s.storeModule.GetTags(),
		})
		return
	}

	parts := strings.SplitN(id, "/", 2)
	pkgID := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	switch {
	case action == "install" && r.Method == http.MethodPost:
		if err := s.storeModule.Install(pkgID); err != nil {
			code := http.StatusInternalServerError
			if strings.Contains(err.Error(), "not found") {
				code = http.StatusNotFound
			}
			writeJSON(w, code, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "installed", "id": pkgID})

	case action == "uninstall" && r.Method == http.MethodPost:
		if err := s.storeModule.Uninstall(pkgID); err != nil {
			code := http.StatusInternalServerError
			if strings.Contains(err.Error(), "not found") {
				code = http.StatusNotFound
			}
			writeJSON(w, code, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "uninstalled", "id": pkgID})

	case action == "" && r.Method == http.MethodGet:
		pkg, err := s.storeModule.Get(pkgID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, pkg)

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func RunWithWorkers(ctx context.Context, app *runtime.App) error {
	workerCount := app.Config.Gateway.WorkerCount
	if workerCount <= 0 {
		workerCount = 4
	}
	if workerCount > 64 {
		workerCount = 64
	}

	if os.Getenv("ANYCLAW_WORKER_MODE") == "1" {
		return runWorker(ctx, app)
	}

	return runMaster(ctx, app, workerCount)
}

func runMaster(ctx context.Context, app *runtime.App, workerCount int) error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	basePort := app.Config.Gateway.Port
	workerPIDs := make([]int, workerCount)
	workerPorts := make([]int, workerCount)

	for i := 0; i < workerCount; i++ {
		workerPort := basePort + i
		workerPorts[i] = workerPort
		cmd := exec.Command(execPath, "gateway", "run",
			"--config", app.ConfigPath,
			"--host", app.Config.Gateway.Host,
			"--port", strconv.Itoa(workerPort),
			"--workers", "1")
		cmd.Env = append(os.Environ(),
			"ANYCLAW_WORKER_MODE=1",
			fmt.Sprintf("ANYCLAW_WORKER_ID=%d", i),
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Start(); err != nil {
			for _, pid := range workerPIDs[:i] {
				killProcess(pid)
			}
			return fmt.Errorf("start worker %d: %w", i, err)
		}
		workerPIDs[i] = cmd.Process.Pid
	}

	printWorkerStatus(workerPIDs, workerPorts, basePort)

	<-ctx.Done()

	for _, pid := range workerPIDs {
		killProcess(pid)
	}

	return nil
}

func killProcess(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Kill()
}

func runWorker(ctx context.Context, app *runtime.App) error {
	workerID := os.Getenv("ANYCLAW_WORKER_ID")
	addr := runtime.GatewayAddress(app.Config)

	server := New(app)
	mux := http.NewServeMux()

	server.initChannels()
	if err := server.ensureDefaultWorkspace(); err != nil {
		return err
	}
	server.startWorkers(ctx)

	mux.HandleFunc("/healthz", server.wrap("/healthz", server.handleHealth))
	mux.HandleFunc("/status", server.wrap("/status", requirePermission("status.read", server.handleStatus)))
	mux.HandleFunc("/chat", server.wrap("/chat", requirePermission("chat.send", requireHierarchyAccess(func(r *http.Request) (string, string, string) {
		return server.resolveHierarchyFromQuery(r)
	}, server.handleChat))))
	mux.HandleFunc("/channels", server.wrap("/channels", requirePermission("channels.read", server.handleChannels)))
	mux.HandleFunc("/plugins", server.wrap("/plugins", requirePermission("plugins.read", server.handlePlugins)))
	mux.HandleFunc("/apps", server.wrap("/apps", requirePermission("apps.read", server.handleApps)))
	mux.HandleFunc("/app-workflows/resolve", server.wrap("/app-workflows/resolve", requirePermission("apps.read", server.handleAppWorkflowResolve)))
	mux.HandleFunc("/app-bindings", server.wrap("/app-bindings", server.handleAppBindings))
	mux.HandleFunc("/app-pairings", server.wrap("/app-pairings", server.handleAppPairings))
	mux.HandleFunc("/routing", server.wrap("/routing", requirePermission("routing.read", server.handleRouting)))
	mux.HandleFunc("/routing/analysis", server.wrap("/routing/analysis", requirePermission("routing.read", server.handleRoutingAnalysis)))
	mux.HandleFunc("/agents", server.wrap("/agents", server.handleAgents))
	mux.HandleFunc("/agents/personality-templates", server.wrap("/agents/personality-templates", requirePermission("config.read", server.handlePersonalityTemplates)))
	mux.HandleFunc("/agents/skill-catalog", server.wrap("/agents/skill-catalog", requirePermission("skills.read", server.handleAssistantSkillCatalog)))
	mux.HandleFunc("/assistants", server.wrap("/assistants", server.handleAssistants))
	mux.HandleFunc("/assistants/personality-templates", server.wrap("/assistants/personality-templates", requirePermission("config.read", server.handlePersonalityTemplates)))
	mux.HandleFunc("/assistants/skill-catalog", server.wrap("/assistants/skill-catalog", requirePermission("skills.read", server.handleAssistantSkillCatalog)))
	mux.HandleFunc("/providers", server.wrap("/providers", server.handleProviders))
	mux.HandleFunc("/providers/test", server.wrap("/providers/test", server.handleProviderTest))
	mux.HandleFunc("/providers/default", server.wrap("/providers/default", server.handleDefaultProvider))
	mux.HandleFunc("/agent-bindings", server.wrap("/agent-bindings", server.handleAgentBindings))
	mux.HandleFunc("/runtimes", server.wrap("/runtimes", requirePermission("runtimes.read", requireHierarchyAccess(server.resolveHierarchyFromQuery, server.handleRuntimes))))
	mux.HandleFunc("/runtimes/refresh", server.wrap("/runtimes/refresh", requirePermission("runtimes.write", server.handleRefreshRuntime)))
	mux.HandleFunc("/runtimes/refresh-batch", server.wrap("/runtimes/refresh-batch", requirePermission("runtimes.write", server.handleRefreshRuntimesBatch)))
	mux.HandleFunc("/runtimes/metrics", server.wrap("/runtimes/metrics", requirePermission("runtimes.read", server.handleRuntimeMetrics)))
	mux.HandleFunc("/resources", server.wrap("/resources", server.handleResources))
	mux.HandleFunc("/auth/users", server.wrap("/auth/users", server.handleUsers))
	mux.HandleFunc("/auth/roles", server.wrap("/auth/roles", server.handleRoles))
	mux.HandleFunc("/auth/roles/impact", server.wrap("/auth/roles/impact", requirePermission("auth.users.read", server.handleRoleImpact)))
	mux.HandleFunc("/audit", server.wrap("/audit", requirePermission("audit.read", server.handleAudit)))
	mux.HandleFunc("/jobs", server.wrap("/jobs", requirePermission("audit.read", server.handleJobs)))
	mux.HandleFunc("/jobs/", server.wrap("/jobs/", requirePermission("audit.read", server.handleJobByID)))
	mux.HandleFunc("/jobs/retry", server.wrap("/jobs/retry", requirePermission("audit.read", server.handleRetryJob)))
	mux.HandleFunc("/jobs/cancel", server.wrap("/jobs/cancel", requirePermission("audit.read", server.handleCancelJob)))
	mux.HandleFunc("/config", server.wrap("/config", server.handleConfigAPI))
	mux.HandleFunc("/memory", server.wrap("/memory", requirePermission("memory.read", requireHierarchyAccess(server.resolveHierarchyFromQuery, server.handleMemory))))
	mux.HandleFunc("/events", server.wrap("/events", requirePermission("events.read", server.handleEvents)))
	mux.HandleFunc("/events/stream", server.wrap("/events/stream", requirePermission("events.read", server.handleEventStream)))
	mux.HandleFunc("/ws", server.wrap("/ws", server.handleOpenClawWS))
	mux.HandleFunc("/control-plane", server.wrap("/control-plane", requirePermission("status.read", server.handleControlPlane)))
	mux.HandleFunc("/sessions", server.wrap("/sessions", requirePermission("sessions.read", requireHierarchyAccess(server.resolveHierarchyFromQuery, server.handleSessions))))
	mux.HandleFunc("/sessions/", server.wrap("/sessions/", requirePermission("sessions.read", requireHierarchyAccess(server.resolveHierarchyFromSessionPath, server.handleSessionByID))))
	mux.HandleFunc("/sessions/move", server.wrap("/sessions/move", requirePermission("sessions.write", server.handleMoveSession)))
	mux.HandleFunc("/sessions/move-batch", server.wrap("/sessions/move-batch", requirePermission("sessions.write", server.handleMoveSessionsBatch)))
	mux.HandleFunc("/tasks", server.wrap("/tasks", requirePermission("tasks.write", requireHierarchyAccess(server.resolveHierarchyFromQuery, server.handleTasks))))
	mux.HandleFunc("/tasks/", server.wrap("/tasks/", server.handleTaskByID))
	mux.HandleFunc("/v2/tasks", server.wrap("/v2/tasks", requirePermission("tasks.write", server.handleV2Tasks)))
	mux.HandleFunc("/v2/tasks/", server.wrap("/v2/tasks/", requirePermission("tasks.read", server.handleV2TaskByID)))
	mux.HandleFunc("/v2/agents", server.wrap("/v2/agents", requirePermission("tasks.read", server.handleV2Agents)))
	mux.HandleFunc("/v2/chat", server.wrap("/v2/chat", requirePermission("tasks.write", server.handleV2Chat)))
	mux.HandleFunc("/v2/chat/sessions", server.wrap("/v2/chat/sessions", requirePermission("tasks.read", server.handleV2ChatSessions)))
	mux.HandleFunc("/v2/chat/sessions/", server.wrap("/v2/chat/sessions/", requirePermission("tasks.read", server.handleV2ChatSessionByID)))
	mux.HandleFunc("/v2/store", server.wrap("/v2/store", requirePermission("tasks.read", server.handleV2Store)))
	mux.HandleFunc("/v2/store/", server.wrap("/v2/store/", requirePermission("tasks.read", server.handleV2StoreByID)))
	mux.HandleFunc("/approvals", server.wrap("/approvals", requirePermission("approvals.read", server.handleApprovals)))
	mux.HandleFunc("/approvals/", server.wrap("/approvals/", requirePermission("approvals.write", server.handleApprovalByID)))
	mux.HandleFunc("/skills", server.wrap("/skills", requirePermission("skills.read", server.handleSkills)))
	mux.HandleFunc("/tools/activity", server.wrap("/tools/activity", requirePermission("tools.read", server.handleToolActivity)))
	mux.HandleFunc("/tools", server.wrap("/tools", requirePermission("tools.read", server.handleTools)))

	// Device pairing
	mux.HandleFunc("/device/pairing", server.wrap("/device/pairing", server.handleDevicePairing))
	mux.HandleFunc("/device/pairing/code", server.wrap("/device/pairing/code", server.handleDevicePairingCode))

	mux.HandleFunc("/channels/whatsapp/webhook", server.rateLimit.Wrap(server.handleWhatsAppWebhook))
	mux.HandleFunc("/channels/discord/interactions", server.rateLimit.Wrap(server.handleDiscordInteractions))
	mux.HandleFunc("/ingress/web", server.rateLimit.Wrap(server.handleSignedIngress))
	mux.HandleFunc("/ingress/plugins/", server.rateLimit.Wrap(server.handlePluginIngress))
	mux.HandleFunc("/", server.handleRootAPI)

	server.startedAt = time.Now().UTC()
	server.httpServer = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go server.runChannels(ctx)
	go func() {
		if err := server.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.httpServer.Shutdown(shutdownCtx)
	case err := <-errCh:
		return fmt.Errorf("worker %s server failed: %w", workerID, err)
	}
}

func printWorkerStatus(pids []int, ports []int, basePort int) {
	if len(pids) == 0 {
		return
	}
	fmt.Printf("Gateway workers started:\n")
	for i, pid := range pids {
		addr := fmt.Sprintf("127.0.0.1:%d", ports[i])
		fmt.Printf("  Worker %d: PID=%d, addr=%s\n", i, pid, addr)
	}
	fmt.Printf("Main Gateway: 127.0.0.1:%d (load balancer)\n", basePort)
}

func (s *Server) initMCP(ctx context.Context) {
	s.mcpRegistry = mcp.NewRegistry()

	cfg := s.app.Config.MCP
	if !cfg.Enabled || len(cfg.Servers) == 0 {
		return
	}

	for _, srvCfg := range cfg.Servers {
		if !srvCfg.Enabled {
			continue
		}
		if srvCfg.Command == "" {
			continue
		}
		client := mcp.NewClient(srvCfg.Name, srvCfg.Command, srvCfg.Args, srvCfg.Env)
		if err := s.mcpRegistry.Register(srvCfg.Name, client); err != nil {
			fmt.Fprintf(os.Stderr, "MCP register %s: %v\n", srvCfg.Name, err)
			continue
		}
	}

	if errs := s.mcpRegistry.ConnectAll(ctx); len(errs) > 0 {
		for _, err := range errs {
			fmt.Fprintf(os.Stderr, "MCP connect: %v\n", err)
		}
	}

	if s.mcpRegistry != nil {
		if err := mcp.BridgeToToolRegistry(s.app.Tools, s.mcpRegistry); err != nil {
			fmt.Fprintf(os.Stderr, "MCP bridge: %v\n", err)
		}
	}

	s.mcpServer = mcp.NewServer("anyclaw", "1.0.0")
	s.registerBuiltinMCPTools()
}

func (s *Server) initMarketStore() {
	pluginDir := s.app.Config.Plugins.Dir
	if pluginDir == "" {
		pluginDir = "plugins"
	}
	marketDir := filepath.Join(pluginDir, ".market")
	cacheDir := filepath.Join(pluginDir, ".cache")

	os.MkdirAll(marketDir, 0755)
	os.MkdirAll(cacheDir, 0755)

	sources := []plugin.PluginSource{
		{Name: "default", URL: "https://market.anyclaw.github.io", Type: "http"},
	}

	trustStore := plugin.NewTrustStore()
	s.marketStore = plugin.NewStore(pluginDir, marketDir, cacheDir, sources, trustStore, s.plugins)
}

func (s *Server) initDiscovery(ctx context.Context) {
	port := s.app.Config.Gateway.Port
	if port <= 0 {
		port = 18789
	}

	caps := []string{"gateway", "chat", "agents"}
	for _, profile := range s.app.Config.Agent.Profiles {
		caps = append(caps, "agent:"+profile.Name)
	}

	svc := discovery.NewService(discovery.Config{
		ServiceName:  s.app.Config.Agent.Name,
		ServicePort:  port,
		InstanceID:   fmt.Sprintf("anyclaw-%s", s.app.Config.Agent.Name),
		Version:      "1.0.0",
		Capabilities: caps,
		Metadata: map[string]string{
			"provider": s.app.Config.LLM.Provider,
			"model":    s.app.Config.LLM.Model,
		},
	})

	svc.OnDiscover(func(inst *discovery.Instance) {
		s.appendEvent("discovery.instance", "", map[string]any{
			"event":   "discovered",
			"id":      inst.ID,
			"name":    inst.Name,
			"url":     inst.URL,
			"version": inst.Version,
			"caps":    inst.Caps,
		})

		if inst.Address != "" && !discovery.IsLocalhost(inst.Address) {
			node := &DeviceNode{
				ID:           inst.ID,
				Name:         inst.Name,
				Type:         "anyclaw",
				Capabilities: inst.Caps,
				Status:       "online",
				ConnectedAt:  inst.LastSeen,
				LastSeen:     inst.LastSeen,
				Metadata: map[string]string{
					"url":     inst.URL,
					"version": inst.Version,
				},
			}
			s.nodes.Register(node)
		}
	})

	if err := svc.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "discovery start error: %v\n", err)
		return
	}

	s.discoverySvc = svc

	go func() {
		<-ctx.Done()
		svc.Stop()
	}()
}

func (s *Server) registerBuiltinMCPTools() {
	s.mcpServer.RegisterTool(mcp.ServerTool{
		Name:        "chat",
		Description: "Send a message to AnyClaw AI agent",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]any{"type": "string", "description": "User message"},
			},
			"required": []string{"message"},
		},
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			msg, _ := args["message"].(string)
			if msg == "" {
				return nil, fmt.Errorf("message is required")
			}
			return "Chat endpoint available via HTTP POST /chat", nil
		},
	})

	s.mcpServer.RegisterResource(mcp.ServerResource{
		URI:         "status://gateway",
		Name:        "Gateway Status",
		Description: "Current gateway server status",
		MimeType:    "application/json",
		Handler: func(ctx context.Context) (any, error) {
			return map[string]any{
				"started_at": s.startedAt,
				"uptime":     time.Since(s.startedAt).String(),
			}, nil
		},
	})

	s.mcpServer.RegisterPrompt(mcp.ServerPrompt{
		Name:        "code_review",
		Description: "Code review prompt template",
		Arguments: []mcp.PromptArg{
			{Name: "language", Description: "Programming language", Required: false},
			{Name: "focus", Description: "Review focus (security, performance, style)", Required: false},
		},
		Handler: func(ctx context.Context, args map[string]string) ([]mcp.PromptMessage, error) {
			lang := args["language"]
			focus := args["focus"]
			text := "Please review the following code"
			if lang != "" {
				text += " (" + lang + ")"
			}
			if focus != "" {
				text += " with focus on " + focus
			}
			text += ":\n\n"
			return []mcp.PromptMessage{
				{Role: "user", Content: struct {
					Type string `json:"type"`
					Text string `json:"text"`
				}{Type: "text", Text: text}},
			}, nil
		},
	})
}

func (s *Server) handleMCPServers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.mcpRegistry == nil {
		writeJSON(w, http.StatusOK, map[string]any{"servers": []any{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"servers": s.mcpRegistry.Status()})
}

func (s *Server) handleMCPTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.mcpRegistry == nil {
		writeJSON(w, http.StatusOK, map[string]any{"tools": map[string]any{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tools": s.mcpRegistry.AllTools()})
}

func (s *Server) handleMCPResources(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.mcpRegistry == nil {
		writeJSON(w, http.StatusOK, map[string]any{"resources": map[string]any{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"resources": s.mcpRegistry.AllResources()})
}

func (s *Server) handleMCPPrompts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.mcpRegistry == nil {
		writeJSON(w, http.StatusOK, map[string]any{"prompts": map[string]any{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"prompts": s.mcpRegistry.AllPrompts()})
}

func (s *Server) handleMCPCall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Server   string         `json:"server"`
		Tool     string         `json:"tool"`
		Resource string         `json:"resource"`
		Prompt   string         `json:"prompt"`
		Args     map[string]any `json:"args"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	if s.mcpRegistry == nil {
		http.Error(w, "MCP not configured", http.StatusServiceUnavailable)
		return
	}

	ctx := r.Context()
	var result any
	var err error

	if req.Tool != "" {
		result, err = s.mcpRegistry.CallTool(ctx, req.Server, req.Tool, req.Args)
	} else if req.Resource != "" {
		result, err = s.mcpRegistry.ReadResource(ctx, req.Server, req.Resource)
	} else if req.Prompt != "" {
		strArgs := make(map[string]string)
		for k, v := range req.Args {
			strArgs[k] = fmt.Sprintf("%v", v)
		}
		result, err = s.mcpRegistry.GetPrompt(ctx, req.Server, req.Prompt, strArgs)
	} else {
		http.Error(w, "must specify tool, resource, or prompt", http.StatusBadRequest)
		return
	}

	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": result})
}

func (s *Server) handleMCPServerAction(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/mcp/servers/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "server name required", http.StatusBadRequest)
		return
	}
	serverName := parts[0]

	if s.mcpRegistry == nil {
		http.Error(w, "MCP not configured", http.StatusServiceUnavailable)
		return
	}

	client, ok := s.mcpRegistry.Get(serverName)
	if !ok {
		http.Error(w, "server not found", http.StatusNotFound)
		return
	}

	ctx := r.Context()
	switch r.Method {
	case http.MethodPost:
		if len(parts) > 1 {
			switch parts[1] {
			case "connect":
				if err := client.Connect(ctx); err != nil {
					writeJSON(w, http.StatusOK, map[string]any{"error": err.Error()})
					return
				}
				writeJSON(w, http.StatusOK, map[string]any{"status": "connected"})
				return
			case "disconnect":
				client.Close()
				writeJSON(w, http.StatusOK, map[string]any{"status": "disconnected"})
				return
			}
		}
		http.Error(w, "unknown action", http.StatusBadRequest)
	case http.MethodGet:
		status := map[string]any{
			"name":      serverName,
			"connected": client.IsConnected(),
		}
		if client.IsConnected() {
			status["tools"] = len(client.ListTools())
			status["resources"] = len(client.ListResources())
			status["prompts"] = len(client.ListPrompts())
		}
		writeJSON(w, http.StatusOK, status)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleMarketSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	filter := plugin.SearchFilter{
		Query:     r.URL.Query().Get("q"),
		Author:    r.URL.Query().Get("author"),
		SortBy:    r.URL.Query().Get("sort"),
		SortOrder: r.URL.Query().Get("order"),
		Limit:     parseIntParam(r.URL.Query().Get("limit"), 50),
		Offset:    parseIntParam(r.URL.Query().Get("offset"), 0),
	}

	if tags := r.URL.Query().Get("tags"); tags != "" {
		filter.Tags = strings.Split(tags, ",")
	}

	ctx := r.Context()
	results, err := s.marketStore.Search(ctx, filter)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"error": err.Error(), "plugins": []any{}})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"plugins": results,
		"total":   len(results),
	})
}

func (s *Server) handleMarketPlugins(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "plugin id required", http.StatusBadRequest)
		return
	}

	listing, err := s.marketStore.GetPlugin(ctx, id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, listing)
}

func (s *Server) handleMarketPluginAction(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/market/plugins/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "plugin id required", http.StatusBadRequest)
		return
	}
	pluginID := parts[0]

	ctx := r.Context()
	switch r.Method {
	case http.MethodPost:
		if len(parts) > 1 {
			switch parts[1] {
			case "install":
				version := r.URL.Query().Get("version")
				result, err := s.marketStore.Install(ctx, pluginID, version)
				if err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
					return
				}
				writeJSON(w, http.StatusOK, result)
				return
			case "update":
				result, err := s.marketStore.Update(ctx, pluginID)
				if err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
					return
				}
				writeJSON(w, http.StatusOK, result)
				return
			case "uninstall":
				result, err := s.marketStore.Uninstall(pluginID)
				if err != nil {
					writeJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
					return
				}
				writeJSON(w, http.StatusOK, result)
				return
			case "rollback":
				targetVersion := r.URL.Query().Get("version")
				if targetVersion == "" {
					http.Error(w, "version query param required for rollback", http.StatusBadRequest)
					return
				}
				result, err := s.marketStore.Rollback(ctx, pluginID, targetVersion)
				if err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
					return
				}
				writeJSON(w, http.StatusOK, result)
				return
			}
		}
		http.Error(w, "unknown action", http.StatusBadRequest)
	case http.MethodGet:
		if len(parts) > 1 && parts[1] == "versions" {
			versions, err := s.marketStore.GetVersions(ctx, pluginID)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"versions": versions})
			return
		}
		if len(parts) > 1 && parts[1] == "history" {
			history := s.marketStore.GetInstallHistory(pluginID)
			writeJSON(w, http.StatusOK, map[string]any{"history": history})
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleMarketInstalled(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	records := s.marketStore.ListInstalled()
	writeJSON(w, http.StatusOK, map[string]any{"installed": records})
}

func (s *Server) handleMarketCategories(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	categories := []string{
		"channel", "tool", "mcp", "skill", "app",
		"model-provider", "speech", "context-engine",
		"node", "surface", "ingress", "workflow-pack",
	}
	writeJSON(w, http.StatusOK, map[string]any{"categories": categories})
}

func parseIntParam(s string, defaultVal int) int {
	if s == "" {
		return defaultVal
	}
	var n int
	fmt.Sscanf(s, "%d", &n)
	if n <= 0 {
		return defaultVal
	}
	return n
}

func (s *Server) handleMarketUI(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile("ui/market/index.html")
	if err != nil {
		data, err = os.ReadFile(filepath.Join(s.app.Config.Gateway.ControlUI.Root, "market", "index.html"))
	}
	if err != nil {
		http.Error(w, "market UI not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

func (s *Server) handleDiscoveryInstances(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.discoverySvc == nil {
		writeJSON(w, http.StatusOK, map[string]any{"instances": []any{}})
		return
	}

	instances := s.discoverySvc.Instances()
	writeJSON(w, http.StatusOK, map[string]any{"instances": instances})
}

func (s *Server) handleDiscoveryQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.discoverySvc == nil {
		writeJSON(w, http.StatusOK, map[string]any{"status": "discovery not enabled"})
		return
	}

	s.discoverySvc.SendQuery()
	writeJSON(w, http.StatusOK, map[string]any{"status": "query sent"})
}

func (s *Server) handleDiscoveryUI(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile("ui/discovery/index.html")
	if err != nil {
		data, err = os.ReadFile(filepath.Join(s.app.Config.Gateway.ControlUI.Root, "discovery", "index.html"))
	}
	if err != nil {
		http.Error(w, "discovery UI not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

func (s *Server) handleMentionGate(w http.ResponseWriter, r *http.Request) {
	if s.mentionGate == nil {
		http.Error(w, "mention gate not initialized", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{
			"enabled": s.mentionGate.IsEnabled(),
		})
	case http.MethodPost:
		var req struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		s.mentionGate.SetEnabled(req.Enabled)
		writeJSON(w, http.StatusOK, map[string]any{"status": "updated", "enabled": req.Enabled})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleGroupSecurity(w http.ResponseWriter, r *http.Request) {
	if s.groupSecurity == nil {
		http.Error(w, "group security not initialized", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	case http.MethodPost:
		var req struct {
			Action  string `json:"action"`
			UserID  string `json:"user_id"`
			GroupID string `json:"group_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		switch req.Action {
		case "allow_group":
			s.groupSecurity.AllowGroup(req.GroupID)
		case "deny_group":
			s.groupSecurity.DenyGroup(req.GroupID)
		case "allow_user":
			s.groupSecurity.AllowUser(req.UserID)
		case "deny_user":
			s.groupSecurity.DenyUser(req.UserID)
		default:
			http.Error(w, "unknown action", http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "updated"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleChannelPairing(w http.ResponseWriter, r *http.Request) {
	if s.channelPairing == nil {
		http.Error(w, "channel pairing not initialized", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{
			"enabled": s.channelPairing.IsEnabled(),
			"paired":  s.channelPairing.ListPaired(),
		})
	case http.MethodPost:
		var req struct {
			Action      string `json:"action"`
			UserID      string `json:"user_id"`
			DeviceID    string `json:"device_id"`
			Channel     string `json:"channel"`
			DisplayName string `json:"display_name"`
			TTL         int    `json:"ttl_seconds"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		switch req.Action {
		case "pair":
			ttl := time.Duration(req.TTL) * time.Second
			if ttl <= 0 {
				ttl = 24 * time.Hour
			}
			s.channelPairing.Pair(req.UserID, req.DeviceID, req.Channel, req.DisplayName, ttl)
		case "unpair":
			s.channelPairing.Unpair(req.UserID, req.DeviceID, req.Channel)
		default:
			http.Error(w, "unknown action", http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "updated"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handlePresence(w http.ResponseWriter, r *http.Request) {
	if s.presenceMgr == nil {
		http.Error(w, "presence manager not initialized", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		ch := r.URL.Query().Get("channel")
		userID := r.URL.Query().Get("user_id")
		if ch != "" && userID != "" {
			info, ok := s.presenceMgr.GetPresence(ch, userID)
			if !ok {
				writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
				return
			}
			writeJSON(w, http.StatusOK, info)
		} else {
			writeJSON(w, http.StatusOK, s.presenceMgr.ListActive())
		}
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleContacts(w http.ResponseWriter, r *http.Request) {
	if s.contactDir == nil {
		http.Error(w, "contact directory not initialized", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		ch := r.URL.Query().Get("channel")
		query := r.URL.Query().Get("q")
		if query != "" {
			writeJSON(w, http.StatusOK, s.contactDir.Search(query))
		} else {
			writeJSON(w, http.StatusOK, s.contactDir.List(ch))
		}
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
