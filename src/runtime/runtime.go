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
	"github.com/anyclaw/anyclaw/pkg/workspace"
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
	// WorkingDirOverride preserves an explicit target workspace while still
	// allowing the selected agent profile to apply provider/model defaults.
	WorkingDirOverride string
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

func resolveRuntimePaths(cfg *config.Config, configPath string) {
	if cfg == nil {
		return
	}
	if resolved := config.ResolvePath(configPath, cfg.Agent.WorkDir); resolved != "" {
		cfg.Agent.WorkDir = resolved
	}
	if resolved := config.ResolvePath(configPath, cfg.Agent.WorkingDir); resolved != "" {
		cfg.Agent.WorkingDir = resolved
	}
	if resolved := config.ResolvePath(configPath, cfg.Skills.Dir); resolved != "" {
		cfg.Skills.Dir = resolved
	}
	if resolved := config.ResolvePath(configPath, cfg.Plugins.Dir); resolved != "" {
		cfg.Plugins.Dir = resolved
	}
	if resolved := config.ResolvePath(configPath, cfg.Memory.Dir); resolved != "" {
		cfg.Memory.Dir = resolved
	}
	if resolved := config.ResolvePath(configPath, cfg.Security.AuditLog); resolved != "" {
		cfg.Security.AuditLog = resolved
	}
	if resolved := config.ResolvePath(configPath, cfg.Sandbox.BaseDir); resolved != "" {
		cfg.Sandbox.BaseDir = resolved
	}
	if resolved := config.ResolvePath(configPath, cfg.Daemon.PIDFile); resolved != "" {
		cfg.Daemon.PIDFile = resolved
	}
	if resolved := config.ResolvePath(configPath, cfg.Daemon.LogFile); resolved != "" {
		cfg.Daemon.LogFile = resolved
	}
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
	agentName = strings.TrimSpace(agentName)
	if agentName != "" {
		if profile, ok := cfg.ResolveAgentProfile(agentName); ok {
			_ = cfg.ApplyAgentRuntimeProfile(profile.Name)
		} else {
			cfg.Agent.Name = agentName
			cfg.Agent.ActiveProfile = ""
		}
	} else if profile, ok := cfg.ResolveMainAgentProfile(); ok {
		_ = cfg.ApplyAgentRuntimeProfile(profile.Name)
	}
	workingDir = strings.TrimSpace(workingDir)
	if workingDir != "" {
		cfg.Agent.WorkingDir = workingDir
	}
	baseWorkDir := config.ResolvePath(configPath, cfg.Agent.WorkDir)
	if baseWorkDir == "" {
		baseWorkDir = config.ResolvePath(configPath, ".anyclaw")
	}
	targetName := sanitizeTargetName(cfg.Agent.Name + "-" + cfg.Agent.WorkingDir)
	cfg.Agent.WorkDir = filepath.Join(baseWorkDir, "runtimes", targetName)
	return Bootstrap(BootstrapOptions{ConfigPath: configPath, Config: cfg, WorkingDirOverride: workingDir})
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
	_ = app.Config.ApplyDefaultProviderProfile()
	app.ConfigPath = config.ResolveConfigPath(app.ConfigPath)
	resolveRuntimePaths(app.Config, app.ConfigPath)
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
	if profile, ok := app.Config.ResolveMainAgentProfile(); ok {
		_ = app.Config.ApplyAgentProfile(profile.Name)
		if override := strings.TrimSpace(opts.WorkingDirOverride); override != "" {
			app.Config.Agent.WorkingDir = override
		}
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
	if err := workspace.EnsureBootstrap(workingDir, workspace.BootstrapOptions{
		AgentName:        app.Config.Agent.Name,
		AgentDescription: app.Config.Agent.Description,
	}); err != nil {
		progress(BootEvent{Phase: PhaseStorage, Status: "fail", Message: "workspace bootstrap failed", Err: err, Dur: time.Since(t)})
		return nil, fmt.Errorf("storage: bootstrap workspace %q: %w", workingDir, err)
	}

	mem := memory.NewFileMemory(workDir)
	mem.SetDailyDir(filepath.Join(workingDir, "memory"))
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
	configuredSkillNames := configuredAgentSkillNames(app.Config)
	missingSkillNames := []string{}
	if len(configuredSkillNames) > 0 {
		sk, missingSkillNames = filterConfiguredSkills(sk, configuredSkillNames)
	}
	app.Skills = sk
	skillCount := len(sk.List())
	switch {
	case skillCount == 0 && len(missingSkillNames) > 0:
		progress(BootEvent{Phase: PhaseSkills, Status: "warn", Message: fmt.Sprintf("no configured skills loaded; missing: %s", strings.Join(missingSkillNames, ", ")), Dur: time.Since(t)})
	case skillCount == 0:
		progress(BootEvent{Phase: PhaseSkills, Status: "warn", Message: "no skills loaded", Dur: time.Since(t)})
	case len(missingSkillNames) > 0:
		progress(BootEvent{Phase: PhaseSkills, Status: "warn", Message: fmt.Sprintf("%d skill(s) loaded; missing configured skills: %s", skillCount, strings.Join(missingSkillNames, ", ")), Dur: time.Since(t)})
	default:
		progress(BootEvent{Phase: PhaseSkills, Status: "ok", Message: fmt.Sprintf("%d skill(s) loaded", skillCount), Dur: time.Since(t)})
	}

	// ── Phase 5: Tools ───────────────────────────────────────────────
	progress(BootEvent{Phase: PhaseTools, Status: "start", Message: "registering tools"})
	t = time.Now()

	registry := tools.NewRegistry()
	sandboxManager := tools.NewSandboxManager(app.Config.Sandbox, workingDir)
	policyEngine := tools.NewPolicyEngine(tools.PolicyOptions{
		WorkingDir:           workingDir,
		PermissionLevel:      app.Config.Agent.PermissionLevel,
		ProtectedPaths:       app.Config.Security.ProtectedPaths,
		AllowedReadPaths:     app.Config.Security.AllowedReadPaths,
		AllowedWritePaths:    app.Config.Security.AllowedWritePaths,
		AllowedEgressDomains: app.Config.Security.AllowedEgressDomains,
	})
	tools.RegisterBuiltins(registry, tools.BuiltinOptions{
		WorkingDir:            workingDir,
		PermissionLevel:       app.Config.Agent.PermissionLevel,
		ExecutionMode:         app.Config.Sandbox.ExecutionMode,
		DangerousPatterns:     app.Config.Security.DangerousCommandPatterns,
		ProtectedPaths:        app.Config.Security.ProtectedPaths,
		Policy:                policyEngine,
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
	plugRegistry.SetPolicyEngine(policyEngine)
	plugRegistry.RegisterToolPlugins(registry, app.Config.Plugins.Dir)
	plugRegistry.RegisterAppPlugins(registry, app.Config.Plugins.Dir, app.ConfigPath)
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
		Personality: agent.BuildPersonalityPrompt(resolveMainAgentPersonality(app.Config)),
		LLM:         llmWrapper,
		Memory:      mem,
		Skills:      sk,
		Tools:       registry,
		WorkDir:     workDir,
		WorkingDir:  workingDir,
	})
	app.Agent = ag
	progress(BootEvent{Phase: PhaseAgent, Status: "ok", Message: fmt.Sprintf("permission=%s", app.Config.Agent.PermissionLevel), Dur: time.Since(t)})

	// ── Phase 8.5: Orchestrator (multi-agent coordination) ──────────
	if app.Config.Orchestrator.Enabled || len(app.Config.Orchestrator.AgentNames) > 0 || len(app.Config.Orchestrator.SubAgents) > 0 {
		progress(BootEvent{Phase: PhaseOrchestrator, Status: "skip", Message: "multi-agent orchestrator removed; running in single-agent mode", Dur: 0})
	} else {
		progress(BootEvent{Phase: PhaseOrchestrator, Status: "skip", Message: "single-agent runtime", Dur: 0})
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

func resolveMainAgentPersonality(cfg *config.Config) config.PersonalitySpec {
	if cfg == nil {
		return config.PersonalitySpec{}
	}
	if profile, ok := cfg.ResolveMainAgentProfile(); ok {
		return profile.Personality
	}
	return config.PersonalitySpec{}
}

func configuredAgentSkillNames(cfg *config.Config) []string {
	if cfg == nil {
		return nil
	}
	if profile, ok := cfg.ResolveMainAgentProfile(); ok {
		return enabledSkillNames(profile.Skills)
	}
	return enabledSkillNames(cfg.Agent.Skills)
}

func enabledSkillNames(skills []config.AgentSkillRef) []string {
	if len(skills) == 0 {
		return nil
	}
	items := make([]string, 0, len(skills))
	seen := make(map[string]struct{}, len(skills))
	for _, skill := range skills {
		if !skill.Enabled {
			continue
		}
		name := strings.TrimSpace(skill.Name)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		items = append(items, name)
	}
	return items
}

func filterConfiguredSkills(manager *skills.SkillsManager, configured []string) (*skills.SkillsManager, []string) {
	if manager == nil || len(configured) == 0 {
		return manager, nil
	}
	filtered := manager.FilterEnabled(configured)
	loaded := make(map[string]struct{}, len(filtered.List()))
	for _, skill := range filtered.List() {
		if skill == nil {
			continue
		}
		name := strings.TrimSpace(strings.ToLower(skill.Name))
		if name != "" {
			loaded[name] = struct{}{}
		}
	}
	missing := make([]string, 0)
	for _, name := range configured {
		key := strings.TrimSpace(strings.ToLower(name))
		if key == "" {
			continue
		}
		if _, ok := loaded[key]; ok {
			continue
		}
		missing = append(missing, name)
	}
	return filtered, missing
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
