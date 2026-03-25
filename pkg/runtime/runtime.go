package runtime

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/anyclaw/anyclaw/pkg/agent"
	"github.com/anyclaw/anyclaw/pkg/audit"
	"github.com/anyclaw/anyclaw/pkg/config"
	"github.com/anyclaw/anyclaw/pkg/llm"
	"github.com/anyclaw/anyclaw/pkg/memory"
	"github.com/anyclaw/anyclaw/pkg/orchestrator"
	"github.com/anyclaw/anyclaw/pkg/plugin"
	"github.com/anyclaw/anyclaw/pkg/skills"
	"github.com/anyclaw/anyclaw/pkg/tools"
)

const Version = "2026.3.13"

// BootPhase represents an initialization phase name.
type BootPhase string

const (
	PhaseConfig       BootPhase = "config"
	PhaseStorage      BootPhase = "storage"
	PhaseSecurity     BootPhase = "security"
	PhaseSkills       BootPhase = "skills"
	PhaseTools        BootPhase = "tools"
	PhasePlugins      BootPhase = "plugins"
	PhaseLLM          BootPhase = "llm"
	PhaseAgent        BootPhase = "agent"
	PhaseOrchestrator BootPhase = "orchestrator"
	PhaseReady        BootPhase = "ready"
)

// BootEvent is emitted during initialization to report progress.
type BootEvent struct {
	Phase   BootPhase
	Status  string // "start", "ok", "warn", "skip", "fail"
	Message string
	Err     error
	Dur     time.Duration
}

// BootProgress receives boot events for logging or UI display.
type BootProgress func(BootEvent)

// BootstrapOptions controls how the app is initialized.
type BootstrapOptions struct {
	ConfigPath string
	Config     *config.Config // if set, skip loading from file
	Progress   BootProgress   // optional progress callback
}

type App struct {
	ConfigPath   string
	Config       *config.Config
	Agent        *agent.Agent
	LLM          *llm.ClientWrapper
	Memory       *memory.FileMemory
	Skills       *skills.SkillsManager
	Tools        *tools.Registry
	Plugins      *plugin.Registry
	Audit        *audit.Logger
	Orchestrator *orchestrator.Orchestrator
	WorkDir      string
	WorkingDir   string
}

// LoadConfig loads configuration from disk with validation.
func LoadConfig(configPath string) (*config.Config, error) {
	if configPath == "" {
		configPath = "anyclaw.json"
	}
	return config.Load(configPath)
}

// NewApp creates an App from a config file path (legacy API).
func NewApp(configPath string) (*App, error) {
	if configPath == "" {
		configPath = "anyclaw.json"
	}
	return Bootstrap(BootstrapOptions{ConfigPath: configPath})
}

// NewAppFromConfig creates an App from an existing config (legacy API).
func NewAppFromConfig(configPath string, cfg *config.Config) (*App, error) {
	return Bootstrap(BootstrapOptions{ConfigPath: configPath, Config: cfg})
}

// NewTargetApp creates a runtime-targeted App with isolated work dir.
func NewTargetApp(configPath string, agentName string, workingDir string) (*App, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	if strings.TrimSpace(agentName) != "" {
		cfg.Agent.Name = strings.TrimSpace(agentName)
	}
	if strings.TrimSpace(workingDir) != "" {
		cfg.Agent.WorkingDir = strings.TrimSpace(workingDir)
	}
	baseWorkDir := cfg.Agent.WorkDir
	if baseWorkDir == "" {
		baseWorkDir = ".anyclaw"
	}
	targetName := sanitizeTargetName(cfg.Agent.Name + "-" + cfg.Agent.WorkingDir)
	cfg.Agent.WorkDir = filepath.Join(baseWorkDir, "runtimes", targetName)
	return Bootstrap(BootstrapOptions{ConfigPath: configPath, Config: cfg})
}

// Bootstrap initializes the application in well-defined phases.
// Each phase emits a BootEvent through opts.Progress (if set).
func Bootstrap(opts BootstrapOptions) (*App, error) {
	start := time.Now()
	progress := opts.Progress
	if progress == nil {
		progress = func(BootEvent) {}
	}

	app := &App{ConfigPath: opts.ConfigPath}

	// ── Phase 1: Config ──────────────────────────────────────────────
	progress(BootEvent{Phase: PhaseConfig, Status: "start", Message: "loading configuration"})
	t := time.Now()

	if opts.Config != nil {
		app.Config = opts.Config
	} else {
		cfgPath := opts.ConfigPath
		if cfgPath == "" {
			cfgPath = "anyclaw.json"
		}
		app.ConfigPath = cfgPath
		cfg, err := config.Load(cfgPath)
		if err != nil {
			progress(BootEvent{Phase: PhaseConfig, Status: "fail", Message: "config load failed", Err: err, Dur: time.Since(t)})
			return nil, fmt.Errorf("config: %w", err)
		}
		app.Config = cfg
	}
	progress(BootEvent{Phase: PhaseConfig, Status: "ok", Message: fmt.Sprintf("provider=%s model=%s", app.Config.LLM.Provider, app.Config.LLM.Model), Dur: time.Since(t)})

	// ── Phase 2: Storage (work dirs + memory) ────────────────────────
	progress(BootEvent{Phase: PhaseStorage, Status: "start", Message: "initializing storage"})
	t = time.Now()

	workDir := app.Config.Agent.WorkDir
	if workDir == "" {
		workDir = ".anyclaw"
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		progress(BootEvent{Phase: PhaseStorage, Status: "fail", Message: "create work dir failed", Err: err, Dur: time.Since(t)})
		return nil, fmt.Errorf("storage: create work dir %q: %w", workDir, err)
	}
	app.WorkDir = workDir

	workingDir := app.Config.Agent.WorkingDir
	if workingDir == "" {
		workingDir = "workflows"
	}
	if app.Config.Agent.ActiveProfile != "" {
		_ = app.Config.ApplyAgentProfile(app.Config.Agent.ActiveProfile)
		if app.Config.Agent.WorkingDir != "" {
			workingDir = app.Config.Agent.WorkingDir
		}
	}
	absWorkingDir, err := filepath.Abs(workingDir)
	if err != nil {
		progress(BootEvent{Phase: PhaseStorage, Status: "fail", Message: "resolve working dir failed", Err: err, Dur: time.Since(t)})
		return nil, fmt.Errorf("storage: resolve working dir %q: %w", workingDir, err)
	}
	workingDir = absWorkingDir
	if err := os.MkdirAll(workingDir, 0o755); err != nil {
		progress(BootEvent{Phase: PhaseStorage, Status: "fail", Message: "create working dir failed", Err: err, Dur: time.Since(t)})
		return nil, fmt.Errorf("storage: create working dir %q: %w", workingDir, err)
	}
	app.WorkingDir = workingDir

	mem := memory.NewFileMemory(workDir)
	if err := mem.Init(); err != nil {
		progress(BootEvent{Phase: PhaseStorage, Status: "fail", Message: "memory init failed", Err: err, Dur: time.Since(t)})
		return nil, fmt.Errorf("storage: init memory: %w", err)
	}
	app.Memory = mem
	progress(BootEvent{Phase: PhaseStorage, Status: "ok", Message: fmt.Sprintf("work_dir=%s working_dir=%s", workDir, workingDir), Dur: time.Since(t)})

	// ── Phase 3: Security (audit logger) ─────────────────────────────
	progress(BootEvent{Phase: PhaseSecurity, Status: "start", Message: "initializing security"})
	t = time.Now()

	auditLogger := audit.New(app.Config.Security.AuditLog, app.Config.Agent.Name)
	app.Audit = auditLogger

	secured := strings.TrimSpace(app.Config.Security.APIToken) != ""
	progress(BootEvent{Phase: PhaseSecurity, Status: "ok", Message: fmt.Sprintf("audit_log=%s secured=%v", app.Config.Security.AuditLog, secured), Dur: time.Since(t)})

	// ── Phase 4: Skills ──────────────────────────────────────────────
	progress(BootEvent{Phase: PhaseSkills, Status: "start", Message: "loading skills"})
	t = time.Now()

	sk := skills.NewSkillsManager(app.Config.Skills.Dir)
	if err := sk.Load(); err != nil && !os.IsNotExist(err) {
		progress(BootEvent{Phase: PhaseSkills, Status: "fail", Message: "skills load failed", Err: err, Dur: time.Since(t)})
		return nil, fmt.Errorf("skills: %w", err)
	}
	boundSkillNames := enabledProfileSkills(app.Config.Agent.Profiles, app.Config.Agent.ActiveProfile)
	if len(boundSkillNames) > 0 {
		sk = sk.FilterEnabled(boundSkillNames)
	}
	app.Skills = sk
	skillCount := len(sk.List())
	if skillCount == 0 {
		progress(BootEvent{Phase: PhaseSkills, Status: "warn", Message: "no skills loaded", Dur: time.Since(t)})
	} else {
		progress(BootEvent{Phase: PhaseSkills, Status: "ok", Message: fmt.Sprintf("%d skill(s) loaded", skillCount), Dur: time.Since(t)})
	}

	// ── Phase 5: Tools ───────────────────────────────────────────────
	progress(BootEvent{Phase: PhaseTools, Status: "start", Message: "registering tools"})
	t = time.Now()

	registry := tools.NewRegistry()
	sandboxManager := tools.NewSandboxManager(app.Config.Sandbox, workingDir)
	tools.RegisterBuiltins(registry, tools.BuiltinOptions{
		WorkingDir:            workingDir,
		PermissionLevel:       app.Config.Agent.PermissionLevel,
		ExecutionMode:         app.Config.Sandbox.ExecutionMode,
		DangerousPatterns:     app.Config.Security.DangerousCommandPatterns,
		ProtectedPaths:        app.Config.Security.ProtectedPaths,
		CommandTimeoutSeconds: app.Config.Security.CommandTimeoutSeconds,
		AuditLogger:           auditLogger,
		Sandbox:               sandboxManager,
	})
	sk.RegisterTools(registry, skills.ExecutionOptions{AllowExec: app.Config.Plugins.AllowExec, ExecTimeoutSeconds: app.Config.Plugins.ExecTimeoutSeconds})
	app.Tools = registry

	toolCount := len(registry.List())
	progress(BootEvent{Phase: PhaseTools, Status: "ok", Message: fmt.Sprintf("%d tool(s) registered", toolCount), Dur: time.Since(t)})

	// ── Phase 6: Plugins ─────────────────────────────────────────────
	progress(BootEvent{Phase: PhasePlugins, Status: "start", Message: "loading plugins"})
	t = time.Now()

	plugRegistry, err := plugin.NewRegistry(app.Config.Plugins)
	if err != nil {
		progress(BootEvent{Phase: PhasePlugins, Status: "fail", Message: "plugin load failed", Err: err, Dur: time.Since(t)})
		return nil, fmt.Errorf("plugins: %w", err)
	}
	plugRegistry.RegisterToolPlugins(registry, app.Config.Plugins.Dir)
	app.Plugins = plugRegistry

	pluginCount := len(plugRegistry.List())
	if pluginCount == 0 {
		progress(BootEvent{Phase: PhasePlugins, Status: "skip", Message: "no plugins found", Dur: time.Since(t)})
	} else {
		progress(BootEvent{Phase: PhasePlugins, Status: "ok", Message: fmt.Sprintf("%d plugin(s) loaded", pluginCount), Dur: time.Since(t)})
	}

	// ── Phase 7: LLM client ─────────────────────────────────────────
	progress(BootEvent{Phase: PhaseLLM, Status: "start", Message: fmt.Sprintf("connecting to %s/%s", app.Config.LLM.Provider, app.Config.LLM.Model)})
	t = time.Now()

	llmWrapper, err := llm.NewClientWrapper(llm.Config{
		Provider:    app.Config.LLM.Provider,
		Model:       app.Config.LLM.Model,
		APIKey:      app.Config.LLM.APIKey,
		BaseURL:     app.Config.LLM.BaseURL,
		Proxy:       app.Config.LLM.Proxy,
		MaxTokens:   app.Config.LLM.MaxTokens,
		Temperature: app.Config.LLM.Temperature,
	})
	if err != nil {
		progress(BootEvent{Phase: PhaseLLM, Status: "fail", Message: "LLM client init failed", Err: err, Dur: time.Since(t)})
		return nil, fmt.Errorf("llm: %w", err)
	}
	app.LLM = llmWrapper
	progress(BootEvent{Phase: PhaseLLM, Status: "ok", Message: "LLM client ready", Dur: time.Since(t)})

	// ── Phase 8: Agent (orchestrator) ────────────────────────────────
	progress(BootEvent{Phase: PhaseAgent, Status: "start", Message: fmt.Sprintf("creating agent %q", app.Config.Agent.Name)})
	t = time.Now()

	ag := agent.New(agent.Config{
		Name:        app.Config.Agent.Name,
		Description: app.Config.Agent.Description,
		Personality: agent.BuildPersonalityPrompt(firstEnabledProfilePersonality(app.Config.Agent.Profiles, app.Config.Agent.ActiveProfile)),
		LLM:         llmWrapper,
		Memory:      mem,
		Skills:      sk,
		Tools:       registry,
		WorkDir:     workDir,
	})
	app.Agent = ag
	progress(BootEvent{Phase: PhaseAgent, Status: "ok", Message: fmt.Sprintf("permission=%s", app.Config.Agent.PermissionLevel), Dur: time.Since(t)})

	// ── Phase 8.5: Orchestrator (multi-agent coordination) ──────────
	if app.Config.Orchestrator.Enabled {
		progress(BootEvent{Phase: PhaseOrchestrator, Status: "start", Message: "initializing orchestrator"})
		t = time.Now()

		// Collect agent definitions from profiles and/or orchestrator config
		agentDefs := make([]orchestrator.AgentDefinition, 0)

		// If agent_names specified, pick those profiles
		if len(app.Config.Orchestrator.AgentNames) > 0 {
			for _, name := range app.Config.Orchestrator.AgentNames {
				if profile, ok := app.Config.FindAgentProfile(name); ok && profile.IsEnabled() {
					skillNames := make([]string, 0, len(profile.Skills))
					for _, s := range profile.Skills {
						if s.Enabled {
							skillNames = append(skillNames, s.Name)
						}
					}
					agentDefs = append(agentDefs, orchestrator.AgentDefinition{
						Name:              profile.Name,
						Description:       profile.Description,
						Persona:           profile.Persona,
						Domain:            profile.Domain,
						Expertise:         profile.Expertise,
						SystemPrompt:      profile.SystemPrompt,
						ConversationTone:  profile.Personality.Tone,
						ConversationStyle: profile.Personality.Style,
						PrivateSkills:     skillNames,
						PermissionLevel:   profile.PermissionLevel,
						WorkingDir:        profile.WorkingDir,
					})
				}
			}
		}

		// Also add from legacy sub_agents config (backward compat)
		for _, saCfg := range app.Config.Orchestrator.SubAgents {
			agentDefs = append(agentDefs, resolveSubAgentDefinition(saCfg, app.Config.LLM))
		}

		// If no agents defined, auto-create from enabled profiles
		if len(agentDefs) == 0 {
			for _, profile := range app.Config.Agent.Profiles {
				if !profile.IsEnabled() {
					continue
				}
				skillNames := make([]string, 0, len(profile.Skills))
				for _, s := range profile.Skills {
					if s.Enabled {
						skillNames = append(skillNames, s.Name)
					}
				}
				agentDefs = append(agentDefs, orchestrator.AgentDefinition{
					Name:              profile.Name,
					Description:       profile.Description,
					Persona:           profile.Persona,
					Domain:            profile.Domain,
					Expertise:         profile.Expertise,
					SystemPrompt:      profile.SystemPrompt,
					ConversationTone:  profile.Personality.Tone,
					ConversationStyle: profile.Personality.Style,
					PrivateSkills:     skillNames,
					PermissionLevel:   profile.PermissionLevel,
					WorkingDir:        profile.WorkingDir,
				})
			}
		}

		orchCfg := orchestrator.OrchestratorConfig{
			MaxConcurrentAgents: app.Config.Orchestrator.MaxConcurrentAgents,
			MaxRetries:          app.Config.Orchestrator.MaxRetries,
			Timeout:             time.Duration(app.Config.Orchestrator.TimeoutSeconds) * time.Second,
			AgentDefinitions:    agentDefs,
			EnableDecomposition: app.Config.Orchestrator.EnableDecomposition,
		}

		if len(agentDefs) == 0 {
			progress(BootEvent{Phase: PhaseOrchestrator, Status: "warn", Message: "no agent definitions found, orchestrator will have no agents", Dur: time.Since(t)})
		}

		orch, err := orchestrator.NewOrchestrator(orchCfg, llmWrapper, sk, registry, mem)
		if err != nil {
			progress(BootEvent{Phase: PhaseOrchestrator, Status: "warn", Message: fmt.Sprintf("orchestrator init failed: %v (continuing with single agent)", err), Dur: time.Since(t)})
		} else if orch.AgentCount() == 0 {
			progress(BootEvent{Phase: PhaseOrchestrator, Status: "warn", Message: "orchestrator initialized but has 0 agents", Dur: time.Since(t)})
			app.Orchestrator = orch
		} else {
			app.Orchestrator = orch
			progress(BootEvent{Phase: PhaseOrchestrator, Status: "ok", Message: fmt.Sprintf("orchestrator ready with %d agents", orch.AgentCount()), Dur: time.Since(t)})
		}
	} else {
		progress(BootEvent{Phase: PhaseOrchestrator, Status: "skip", Message: "orchestrator disabled", Dur: 0})
	}

	// ── Done ─────────────────────────────────────────────────────────
	progress(BootEvent{Phase: PhaseReady, Status: "ok", Message: fmt.Sprintf("bootstrap complete in %s", time.Since(start).Round(time.Millisecond))})
	return app, nil
}

func sanitizeTargetName(input string) string {
	clean := strings.TrimSpace(strings.ToLower(input))
	if clean == "" {
		return "default"
	}
	re := regexp.MustCompile(`[^a-z0-9._-]+`)
	clean = re.ReplaceAllString(clean, "-")
	clean = strings.Trim(clean, "-.")
	if clean == "" {
		return "default"
	}
	return clean
}

func GatewayAddress(cfg *config.Config) string {
	host := strings.TrimSpace(cfg.Gateway.Host)
	if host == "" {
		host = "127.0.0.1"
	}
	port := cfg.Gateway.Port
	if port <= 0 {
		port = 18789
	}
	return net.JoinHostPort(host, fmt.Sprintf("%d", port))
}

func GatewayURL(cfg *config.Config) string {
	return "http://" + GatewayAddress(cfg)
}

func firstEnabledProfilePersonality(profiles []config.AgentProfile, active string) config.PersonalitySpec {
	active = strings.TrimSpace(active)
	for _, profile := range profiles {
		if active != "" && strings.EqualFold(strings.TrimSpace(profile.Name), active) {
			return profile.Personality
		}
	}
	for _, profile := range profiles {
		if profile.IsEnabled() {
			return profile.Personality
		}
	}
	return config.PersonalitySpec{}
}

func enabledProfileSkills(profiles []config.AgentProfile, active string) []string {
	active = strings.TrimSpace(active)
	resolve := func(profile config.AgentProfile) []string {
		items := make([]string, 0, len(profile.Skills))
		for _, skill := range profile.Skills {
			if !skill.Enabled {
				continue
			}
			name := strings.TrimSpace(skill.Name)
			if name != "" {
				items = append(items, name)
			}
		}
		return items
	}
	for _, profile := range profiles {
		if active != "" && strings.EqualFold(strings.TrimSpace(profile.Name), active) {
			return resolve(profile)
		}
	}
	for _, profile := range profiles {
		if profile.IsEnabled() {
			return resolve(profile)
		}
	}
	return nil
}

func ResolveConfigPath(path string) string {
	if path == "" {
		path = "anyclaw.json"
	}
	if filepath.IsAbs(path) {
		return path
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

func resolveSubAgentDefinition(saCfg config.SubAgentConfig, global config.LLMConfig) orchestrator.AgentDefinition {
	def := orchestrator.AgentDefinition{
		Name:            saCfg.Name,
		Description:     saCfg.Description,
		Persona:         saCfg.Personality,
		PrivateSkills:   saCfg.PrivateSkills,
		PermissionLevel: saCfg.PermissionLevel,
		WorkingDir:      saCfg.WorkingDir,
		LLMProvider:     saCfg.LLMProvider,
		LLMModel:        saCfg.LLMModel,
		LLMAPIKey:       saCfg.LLMAPIKey,
		LLMBaseURL:      saCfg.LLMBaseURL,
		LLMMaxTokens:    copyIntPtr(saCfg.LLMMaxTokens),
		LLMTemperature:  copyFloat64Ptr(saCfg.LLMTemperature),
		LLMProxy:        saCfg.LLMProxy,
	}
	if def.LLMProvider == "" {
		def.LLMProvider = global.Provider
	}
	if def.LLMModel == "" {
		def.LLMModel = global.Model
	}
	if def.LLMAPIKey == "" {
		def.LLMAPIKey = global.APIKey
	}
	if def.LLMBaseURL == "" {
		def.LLMBaseURL = global.BaseURL
	}
	if def.LLMProxy == "" {
		def.LLMProxy = global.Proxy
	}
	if def.LLMMaxTokens == nil {
		def.LLMMaxTokens = copyIntPtr(&global.MaxTokens)
	}
	if def.LLMTemperature == nil {
		def.LLMTemperature = copyFloat64Ptr(&global.Temperature)
	}
	return def
}

func copyIntPtr(value *int) *int {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func copyFloat64Ptr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
