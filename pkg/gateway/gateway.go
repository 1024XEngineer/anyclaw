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
	"github.com/anyclaw/anyclaw/pkg/plugin"
	"github.com/anyclaw/anyclaw/pkg/runtime"
	taskModule "github.com/anyclaw/anyclaw/pkg/task"
	"github.com/anyclaw/anyclaw/pkg/tools"
)

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
}

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
	}
	server.approvals = newApprovalManager(store)
	server.tasks = NewTaskManager(store, server.sessions, server.runtimePool, taskAppInfo{Name: app.Config.Agent.Name, WorkingDir: app.WorkingDir}, app.LLM, server.approvals)

	if app.Config.Orchestrator.Enabled && app.Orchestrator != nil {
		server.taskModule = taskModule.NewTaskManager(app.Orchestrator)
		server.chatModule = chat.NewChatManager(app.Orchestrator)
	} else {
		server.chatModule = chat.NewChatManager(nil)
	}

	if sm, err := agentstore.NewStoreManager(app.WorkDir, app.ConfigPath); err == nil {
		server.storeModule = sm
	}

	return server
}

func (s *Server) initChannels() {
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
		adapters = []channel.Adapter{builders["telegram-channel"](), builders["slack-channel"]()}
	}
	s.channels = channel.NewManager(adapters...)
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

func (s *Server) resolveAssistantName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return s.app.Config.Agent.Name, nil
	}
	profile, ok := s.app.Config.FindAgentProfile(name)
	if !ok {
		return "", fmt.Errorf("assistant not found: %s", name)
	}
	if !profile.IsEnabled() {
		return "", fmt.Errorf("assistant is disabled: %s", name)
	}
	return profile.Name, nil
}

func (s *Server) Run(ctx context.Context) error {
	addr := runtime.GatewayAddress(s.app.Config)
	mux := http.NewServeMux()
	s.initChannels()
	if err := s.ensureDefaultWorkspace(); err != nil {
		return err
	}
	s.startWorkers(ctx)
	mux.HandleFunc("/healthz", s.wrap("/healthz", s.handleHealth))
	mux.HandleFunc("/status", s.wrap("/status", requirePermission("status.read", s.handleStatus)))
	mux.HandleFunc("/chat", s.wrap("/chat", requirePermission("chat.send", requireHierarchyAccess(func(r *http.Request) (string, string, string) {
		return s.resolveHierarchyFromQuery(r)
	}, s.handleChat))))
	mux.HandleFunc("/channels", s.wrap("/channels", requirePermission("channels.read", s.handleChannels)))
	mux.HandleFunc("/plugins", s.wrap("/plugins", requirePermission("plugins.read", s.handlePlugins)))
	mux.HandleFunc("/routing", s.wrap("/routing", requirePermission("routing.read", s.handleRouting)))
	mux.HandleFunc("/routing/analysis", s.wrap("/routing/analysis", requirePermission("routing.read", s.handleRoutingAnalysis)))
	mux.HandleFunc("/assistants", s.wrap("/assistants", s.handleAssistants))
	mux.HandleFunc("/assistants/personality-templates", s.wrap("/assistants/personality-templates", requirePermission("config.read", s.handlePersonalityTemplates)))
	mux.HandleFunc("/assistants/skill-catalog", s.wrap("/assistants/skill-catalog", requirePermission("skills.read", s.handleAssistantSkillCatalog)))
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
	mux.HandleFunc("/config", s.wrap("/config", requirePermission("config.write", s.handleConfigAPI)))
	mux.HandleFunc("/memory", s.wrap("/memory", requirePermission("memory.read", requireHierarchyAccess(s.resolveHierarchyFromQuery, s.handleMemory))))
	mux.HandleFunc("/events", s.wrap("/events", requirePermission("events.read", s.handleEvents)))
	mux.HandleFunc("/events/stream", s.wrap("/events/stream", requirePermission("events.read", s.handleEventStream)))
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
	mux.HandleFunc("/channels/whatsapp/webhook", s.rateLimit.Wrap(s.handleWhatsAppWebhook))
	mux.HandleFunc("/channels/discord/interactions", s.rateLimit.Wrap(s.handleDiscordInteractions))
	mux.HandleFunc("/ingress/web", s.rateLimit.Wrap(s.handleSignedIngress))
	mux.HandleFunc("/ingress/plugins/", s.rateLimit.Wrap(s.handlePluginIngress))

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
	s.channels.Run(ctx, s.processChannelMessage)
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
	return session.ID, response, nil
}

func (s *Server) runOrCreateChannelSession(ctx context.Context, source string, sessionID string, message string, meta map[string]string) (string, *Session, error) {
	decision := channel.RouteDecision{}
	routeSource := sessionID
	if strings.TrimSpace(meta["reply_target"]) != "" {
		routeSource = strings.TrimSpace(meta["reply_target"])
	}
	if s.router != nil {
		decision = s.router.Decide(channel.RouteRequest{Channel: source, Source: routeSource, Text: message})
	}
	if strings.TrimSpace(sessionID) == "" {
		agentName := s.app.Config.Agent.Name
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
			return "", nil, err
		}
		title := strings.TrimSpace(decision.Title)
		if title == "" {
			title = strings.Title(source) + " session"
		}
		createOpts := SessionCreateOptions{
			Title:         title,
			AgentName:     agentName,
			Org:           org.ID,
			Project:       project.ID,
			Workspace:     workspace.ID,
			SessionMode:   decision.SessionMode,
			QueueMode:     decision.QueueMode,
			ReplyBack:     decision.ReplyBack,
			SourceChannel: source,
			SourceID:      firstNonEmpty(strings.TrimSpace(meta["user_id"]), strings.TrimSpace(meta["reply_target"]), sessionID),
			GroupKey:      decision.Key,
			IsGroup:       decision.SessionMode == "group" || decision.SessionMode == "group-shared",
		}
		if createOpts.SessionMode == "" {
			createOpts.SessionMode = "main"
		}
		session, err := s.sessions.CreateWithOptions(createOpts)
		if err != nil {
			return "", nil, err
		}
		sessionID = session.ID
		payload := map[string]any{"title": session.Title, "source": source}
		for k, v := range meta {
			if strings.TrimSpace(v) != "" {
				payload[k] = v
			}
		}
		s.appendEvent("session.created", sessionID, payload)
	}
	if _, err := s.sessions.EnqueueTurn(sessionID); err == nil {
		s.appendEvent("session.queue.updated", sessionID, map[string]any{"queue_mode": decision.QueueMode, "source": source, "reply_target": meta["reply_target"]})
	}
	transportMeta := map[string]string{}
	for _, key := range []string{"channel_id", "chat_id", "guild_id", "attachment_count"} {
		if v := strings.TrimSpace(meta[key]); v != "" {
			transportMeta[key] = v
		}
	}
	if _, err := s.sessions.SetUserMapping(sessionID, meta["user_id"], firstNonEmpty(meta["username"], meta["user_name"]), meta["reply_target"], meta["thread_id"], transportMeta); err == nil {
		s.appendEvent("session.user_mapped", sessionID, map[string]any{"source": source, "user_id": meta["user_id"], "user_name": firstNonEmpty(meta["username"], meta["user_name"]), "reply_target": meta["reply_target"]})
	}
	if _, err := s.sessions.SetPresence(sessionID, "typing", true); err == nil {
		s.appendEvent("session.typing", sessionID, map[string]any{"typing": true, "source": source, "user_id": meta["user_id"]})
	}
	startedPayload := map[string]any{"message": message, "source": source}
	for k, v := range meta {
		if strings.TrimSpace(v) != "" {
			startedPayload[k] = v
		}
	}
	startedEvent := NewEvent("chat.started", sessionID, startedPayload)
	_ = s.store.AppendEvent(startedEvent)
	s.bus.Publish(startedEvent)
	session, ok := s.sessions.Get(sessionID)
	if !ok {
		return "", nil, fmt.Errorf("session not found: %s", sessionID)
	}
	targetApp, err := s.runtimePool.GetOrCreate(session.Agent, session.Org, session.Project, session.Workspace)
	if err != nil {
		return "", nil, err
	}
	targetApp.Agent.SetHistory(session.History)
	execCtx := tools.WithBrowserSession(ctx, sessionID)
	execCtx = tools.WithSandboxScope(execCtx, tools.SandboxScope{SessionID: sessionID, Channel: source})
	response, err := targetApp.Agent.Run(execCtx, message)
	if err != nil {
		return "", nil, err
	}
	updatedSession, err := s.sessions.AddExchange(sessionID, message, response)
	if err != nil {
		return "", nil, err
	}
	if updatedSession.ReplyBack {
		s.appendEvent("session.reply_back", sessionID, map[string]any{"enabled": true, "source": source, "reply_target": meta["reply_target"]})
	}
	if _, err := s.sessions.SetPresence(sessionID, "idle", false); err == nil {
		s.appendEvent("session.presence", sessionID, map[string]any{"presence": "idle", "source": source, "user_id": meta["user_id"]})
	}
	s.recordSessionToolActivities(updatedSession, targetApp.Agent.GetLastToolActivities())
	completedPayload := map[string]any{"message": message, "response_length": len(response), "source": source}
	for k, v := range meta {
		if strings.TrimSpace(v) != "" {
			completedPayload[k] = v
		}
	}
	s.appendEvent("chat.completed", sessionID, completedPayload)
	return response, updatedSession, nil
}

func (s *Server) appendEvent(eventType string, sessionID string, payload map[string]any) {
	event := NewEvent(eventType, sessionID, payload)
	_ = s.store.AppendEvent(event)
	s.bus.Publish(event)
}

func (s *Server) appendToolActivity(sessionID string, activity ToolActivityRecord) {
	activity.ID = fmt.Sprintf("tool_%d", time.Now().UnixNano())
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
		ID:        fmt.Sprintf("aud_%d", time.Now().UnixNano()),
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
	agentName, err := s.resolveAssistantName(req.Assistant)
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
		assistantName, err := s.resolveAssistantName(req.Assistant)
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
		if updated.TaskID != "" {
			if req.Approved {
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
			} else {
				_ = s.tasks.MarkRejected(updated.TaskID, firstNonEmpty(strings.TrimSpace(req.Comment), "task execution rejected by approver"))
			}
		}
		s.appendAudit(UserFromContext(r.Context()), "approvals.write", id, map[string]any{"approved": req.Approved})
		if updated.TaskID != "" {
			s.appendEvent("approval.resolved", updated.SessionID, map[string]any{"approval_id": updated.ID, "task_id": updated.TaskID, "status": updated.Status})
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
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", nil, fmt.Errorf("session creation now requires registered org/project/workspace via request path")
	}

	if _, err := s.sessions.EnqueueTurn(sessionID); err == nil {
		s.appendEvent("session.queue.updated", sessionID, map[string]any{"queue_mode": "fifo", "source": "api"})
	}
	if _, err := s.sessions.SetPresence(sessionID, "typing", true); err == nil {
		s.appendEvent("session.typing", sessionID, map[string]any{"typing": true, "source": "api"})
	}
	s.appendEvent("chat.started", sessionID, map[string]any{"message": message})
	session, ok := s.sessions.Get(sessionID)
	if !ok {
		return "", nil, fmt.Errorf("session not found: %s", sessionID)
	}
	targetApp, err := s.runtimePool.GetOrCreate(session.Agent, session.Org, session.Project, session.Workspace)
	if err != nil {
		return "", nil, err
	}
	targetApp.Agent.SetHistory(session.History)
	execCtx := tools.WithBrowserSession(ctx, sessionID)
	execCtx = tools.WithSandboxScope(execCtx, tools.SandboxScope{SessionID: sessionID, Channel: "api"})
	response, err := targetApp.Agent.Run(execCtx, message)
	if err != nil {
		return "", nil, err
	}
	updatedSession, err := s.sessions.AddExchange(sessionID, message, response)
	if err != nil {
		return "", nil, err
	}
	if _, err := s.sessions.SetPresence(sessionID, "idle", false); err == nil {
		s.appendEvent("session.presence", sessionID, map[string]any{"presence": "idle", "source": "api"})
	}
	s.recordSessionToolActivities(updatedSession, targetApp.Agent.GetLastToolActivities())
	s.appendEvent("chat.completed", sessionID, map[string]any{"message": message, "response_length": len(response)})
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

func (s *Server) handleAssistants(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if !HasPermission(UserFromContext(r.Context()), "config.read") && !HasPermission(UserFromContext(r.Context()), "config.write") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "config.read"})
			return
		}
		items := make([]map[string]any, 0, len(s.app.Config.Agent.Profiles))
		for _, profile := range s.app.Config.Agent.Profiles {
			personality := profile.Personality
			if strings.TrimSpace(personality.Template) == "" && len(personality.Traits) == 0 && strings.TrimSpace(personality.Tone) == "" && strings.TrimSpace(personality.Style) == "" {
				personality = defaultPersonalitySpec()
			}
			items = append(items, map[string]any{
				"name":             profile.Name,
				"description":      profile.Description,
				"role":             profile.Role,
				"persona":          profile.Persona,
				"working_dir":      profile.WorkingDir,
				"permission_level": profile.PermissionLevel,
				"default_model":    profile.DefaultModel,
				"enabled":          profile.IsEnabled(),
				"active":           strings.EqualFold(strings.TrimSpace(s.app.Config.Agent.ActiveProfile), strings.TrimSpace(profile.Name)),
				"personality":      personality,
				"skills":           profile.Skills,
			})
		}
		s.appendAudit(UserFromContext(r.Context()), "assistants.read", "assistants", nil)
		writeJSON(w, http.StatusOK, items)
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
			WorkingDir      string                 `json:"working_dir"`
			PermissionLevel string                 `json:"permission_level"`
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
			DefaultModel:    req.DefaultModel,
			Enabled:         req.Enabled,
			Personality:     req.Personality,
			Skills:          req.Skills,
		}
		if profile.Enabled == nil {
			profile.Enabled = config.BoolPtr(true)
		}
		if strings.TrimSpace(profile.PermissionLevel) == "" {
			profile.PermissionLevel = "limited"
		}
		if err := s.app.Config.UpsertAgentProfile(profile); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "assistant name is required"})
			return
		}
		if err := s.app.Config.Save(s.app.ConfigPath); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		s.appendAudit(UserFromContext(r.Context()), "assistants.write", profile.Name, map[string]any{"enabled": profile.IsEnabled()})
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
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "assistant not found"})
			return
		}
		if err := s.app.Config.Save(s.app.ConfigPath); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		s.appendAudit(UserFromContext(r.Context()), "assistants.delete", name, nil)
		writeJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
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
	job := &Job{ID: fmt.Sprintf("job_%d", time.Now().UnixNano()), Kind: "runtimes.refresh.batch", Status: "queued", Summary: fmt.Sprintf("Refreshing %d runtimes", len(req.Items)), CreatedAt: time.Now().UTC(), Payload: payload, MaxAttempts: s.jobMaxAttempts}
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
			"channels.read":   true,
			"routing.read":    true,
			"runtimes.read":   true,
			"runtimes.write":  true,
			"resources.read":  true,
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
	builtinRoles := []map[string]any{
		{
			"name":        "admin",
			"description": "Full platform access",
			"permissions": []string{"*"},
		},
		{
			"name":        "operator",
			"description": "Operate sessions and runtimes",
			"permissions": []string{"status.read", "chat.send", "tasks.read", "tasks.write", "approvals.read", "approvals.write", "sessions.read", "sessions.write", "memory.read", "runtimes.read", "runtimes.write", "events.read", "tools.read"},
		},
		{
			"name":        "viewer",
			"description": "Read-only governance and monitoring",
			"permissions": []string{"status.read", "sessions.read", "events.read", "audit.read", "plugins.read", "channels.read", "routing.read", "runtimes.read", "resources.read"},
		},
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
		{Name: "operator", Description: "Operate sessions and runtimes", Permissions: []string{"status.read", "chat.send", "tasks.read", "tasks.write", "approvals.read", "approvals.write", "sessions.read", "sessions.write", "memory.read", "runtimes.read", "runtimes.write", "events.read", "tools.read"}},
		{Name: "viewer", Description: "Read-only governance and monitoring", Permissions: []string{"status.read", "sessions.read", "events.read", "audit.read", "plugins.read", "channels.read", "routing.read", "runtimes.read", "resources.read"}},
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
	clone := &Job{ID: fmt.Sprintf("job_%d", time.Now().UnixNano()), Kind: job.Kind, Status: "queued", Summary: job.Summary + " (retry)", CreatedAt: time.Now().UTC(), RetryOf: job.ID, Cancellable: true, Retriable: true, Payload: job.Payload}
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
		if !HasPermission(UserFromContext(r.Context()), "resources.read") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "resources.read"})
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
		if !HasPermission(UserFromContext(r.Context()), "resources.read") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "resources.read"})
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
		if !HasPermission(UserFromContext(r.Context()), "resources.read") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "resources.read"})
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
			Title       string `json:"title"`
			Assistant   string `json:"assistant"`
			SessionMode string `json:"session_mode"`
			QueueMode   string `json:"queue_mode"`
			ReplyBack   bool   `json:"reply_back"`
			IsGroup     bool   `json:"is_group"`
			GroupKey    string `json:"group_key"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, context.Canceled) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		agentName, err := s.resolveAssistantName(req.Assistant)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
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
			SessionMode: req.SessionMode,
			QueueMode:   req.QueueMode,
			ReplyBack:   req.ReplyBack,
			IsGroup:     req.IsGroup,
			GroupKey:    req.GroupKey,
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
	job := &Job{ID: fmt.Sprintf("job_%d", time.Now().UnixNano()), Kind: "sessions.move.batch", Status: "queued", Summary: fmt.Sprintf("Moving %d sessions", len(req.SessionIDs)), CreatedAt: time.Now().UTC(), Payload: payload, MaxAttempts: s.jobMaxAttempts}
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
