package runtime

import (
	"context"
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
	"github.com/anyclaw/anyclaw/pkg/qmd"
	"github.com/anyclaw/anyclaw/pkg/secrets"
	"github.com/anyclaw/anyclaw/pkg/skills"
	"github.com/anyclaw/anyclaw/pkg/tools"
	"github.com/anyclaw/anyclaw/pkg/workspace"
)

const Version = "2026.3.13"

const defaultAgentContextTokenFloor = 16384

// BootPhase represents an initialization phase name.
type BootPhase string

const (
	PhaseConfig       BootPhase = "config"
	PhaseStorage      BootPhase = "storage"
	PhaseSecurity     BootPhase = "security"
	PhaseQMD          BootPhase = "qmd"
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
	ConfigPath     string
	Config         *config.Config
	Agent          *agent.Agent
	LLM            *llm.ClientWrapper
	Memory         memory.MemoryBackend
	Skills         *skills.SkillsManager
	Tools          *tools.Registry
	Plugins        *plugin.Registry
	Audit          *audit.Logger
	Orchestrator   *orchestrator.Orchestrator
	QMD            *qmd.Client
	SecretsManager *secrets.ActivationManager
	SecretsStore   *secrets.Store
	WorkDir        string
	WorkingDir     string
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

	// ── Phase 1.5: Secrets ───────────────────────────────────────────
	progress(BootEvent{Phase: PhaseSecurity, Status: "start", Message: "initializing secrets"})
	t = time.Now()

	secretsConfigDir := filepath.Dir(app.ConfigPath)
	if secretsConfigDir == "." {
		secretsConfigDir = "."
	}
	secretsStorePath := filepath.Join(secretsConfigDir, ".anyclaw", "secrets", "store.json")
	if err := os.MkdirAll(filepath.Dir(secretsStorePath), 0o700); err != nil {
		progress(BootEvent{Phase: PhaseSecurity, Status: "warn", Message: fmt.Sprintf("secrets dir creation failed, continuing without secrets store: %v", err), Dur: time.Since(t)})
	} else {
		encKey := os.Getenv("ANYCLAW_SECRETS_KEY")
		storeCfg := secrets.DefaultStoreConfig()
		storeCfg.Path = secretsStorePath
		if encKey != "" {
			storeCfg.EncryptionKey = encKey
		}
		store, err := secrets.NewStore(storeCfg)
		if err != nil {
			progress(BootEvent{Phase: PhaseSecurity, Status: "warn", Message: fmt.Sprintf("secrets store init failed, continuing without persistence: %v", err), Dur: time.Since(t)})
		} else {
			app.SecretsStore = store

			snap := buildInitialSecretsSnapshot(store, app.Config)
			fbCfg := secrets.DefaultFallbackConfig()
			fbCfg.EnvPrefix = "ANYCLAW_SECRET_"
			am := secrets.NewActivationManagerWithFallback(store, snap, fbCfg)

			startupCfg := secrets.DefaultStartupConfig()
			startupCfg.ValidationMode = secrets.ValidationWarn
			startupCfg.FailFast = false
			if err := am.ValidateStartup(startupCfg); err != nil {
				progress(BootEvent{Phase: PhaseSecurity, Status: "warn", Message: fmt.Sprintf("secrets startup validation warning: %v", err), Dur: time.Since(t)})
			}

			app.SecretsManager = am
			progress(BootEvent{Phase: PhaseSecurity, Status: "ok", Message: "secrets manager initialized", Dur: time.Since(t)})
		}
	}

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

	memCfg := memory.DefaultConfig(workDir)
	var secretsSnap *secrets.RuntimeSnapshot
	if app.SecretsManager != nil {
		secretsSnap = app.SecretsManager.GetActiveSnapshot()
	}
	if embedder := resolveEmbedder(app.Config, secretsSnap); embedder != nil {
		memCfg.Embedder = embedder
	}
	mem, err := memory.NewMemoryBackend(memCfg)
	if err != nil {
		progress(BootEvent{Phase: PhaseStorage, Status: "fail", Message: "memory backend creation failed", Err: err, Dur: time.Since(t)})
		return nil, fmt.Errorf("storage: create memory backend: %w", err)
	}
	if err := mem.Init(); err != nil {
		progress(BootEvent{Phase: PhaseStorage, Status: "fail", Message: "memory init failed", Err: err, Dur: time.Since(t)})
		return nil, fmt.Errorf("storage: init memory: %w", err)
	}
	if db, ok := mem.(interface{ SetDailyDir(string) }); ok {
		db.SetDailyDir(filepath.Join(workingDir, "memory"))
	}

	if warmupper, ok := mem.(interface {
		Warmup([]string, int) memory.WarmupProgress
	}); ok {
		warmupCfg := memCfg.Warmup
		if warmupCfg.Enabled && len(warmupCfg.Queries) > 0 {
			go func() {
				result := warmupper.Warmup(warmupCfg.Queries, 4)
				_ = result
			}()
		}
	}

	if sqliteMem, ok := mem.(interface {
		StartAutoBackup(string, time.Duration, int) error
	}); ok {
		backupDir := filepath.Join(workDir, "backups")
		if err := sqliteMem.StartAutoBackup(backupDir, 1*time.Hour, 10); err != nil {
			progress(BootEvent{Phase: PhaseStorage, Status: "warn", Message: fmt.Sprintf("auto-backup init failed: %v", err), Dur: time.Since(t)})
		}
	}

	app.Memory = mem
	progress(BootEvent{Phase: PhaseStorage, Status: "ok", Message: fmt.Sprintf("work_dir=%s working_dir=%s", workDir, workingDir), Dur: time.Since(t)})

	// ── Phase 3: Security (audit logger) ─────────────────────────────
	progress(BootEvent{Phase: PhaseSecurity, Status: "start", Message: "initializing security"})
	t = time.Now()

	auditLogger := audit.New(app.Config.Security.AuditLog, app.Config.Agent.Name)
	app.Audit = auditLogger

	// Resolve security tokens through secrets manager
	if app.SecretsManager != nil {
		secretsSnap := app.SecretsManager.GetActiveSnapshot()
		app.Config.Security.APIToken = resolveSecret(secretsSnap, app.Config.Security.APIToken, "security_api_token")
		app.Config.Security.WebhookSecret = resolveSecret(secretsSnap, app.Config.Security.WebhookSecret, "security_webhook_secret")
	}

	secured := strings.TrimSpace(app.Config.Security.APIToken) != ""
	progress(BootEvent{Phase: PhaseSecurity, Status: "ok", Message: fmt.Sprintf("audit_log=%s secured=%v", app.Config.Security.AuditLog, secured), Dur: time.Since(t)})

	// ── Phase 3.5: QMD (in-memory data store) ────────────────────────
	progress(BootEvent{Phase: PhaseQMD, Status: "start", Message: "initializing QMD"})
	t = time.Now()

	qmdServer := qmd.NewServer(qmd.ServerConfig{})
	if err := qmdServer.Start(); err != nil {
		progress(BootEvent{Phase: PhaseQMD, Status: "warn", Message: fmt.Sprintf("QMD server failed to start, running without structured data store: %v", err), Dur: time.Since(t)})
	} else {
		qmdClient := qmd.NewClient(qmd.DefaultClientConfig())
		ctx := context.Background()
		if err := qmdClient.Ping(ctx); err != nil {
			progress(BootEvent{Phase: PhaseQMD, Status: "warn", Message: fmt.Sprintf("QMD server not reachable: %v", err), Dur: time.Since(t)})
		} else {
			app.QMD = qmdClient
			progress(BootEvent{Phase: PhaseQMD, Status: "ok", Message: "QMD in-memory data store ready", Dur: time.Since(t)})
		}
	}

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
	var qmdClient tools.QMDClient
	if app.QMD != nil {
		qmdClient = &qmdAdapter{client: app.QMD}
	}

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
		MemoryBackend:         mem,
		QMDClient:             qmdClient,
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

	llmAPIKey := resolveSecret(secretsSnap, app.Config.LLM.APIKey, "llm_api_key")
	llmWrapper, err := llm.NewClientWrapper(llm.Config{
		Provider:    app.Config.LLM.Provider,
		Model:       app.Config.LLM.Model,
		APIKey:      llmAPIKey,
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
		Name:             app.Config.Agent.Name,
		Description:      app.Config.Agent.Description,
		Personality:      agent.BuildPersonalityPrompt(resolveMainAgentPersonality(app.Config)),
		LLM:              llmWrapper,
		Memory:           mem,
		Skills:           sk,
		Tools:            registry,
		WorkDir:          workDir,
		WorkingDir:       workingDir,
		MaxContextTokens: deriveAgentContextTokenBudget(app.Config.LLM.MaxTokens),
	})
	app.Agent = ag
	progress(BootEvent{Phase: PhaseAgent, Status: "ok", Message: fmt.Sprintf("permission=%s", app.Config.Agent.PermissionLevel), Dur: time.Since(t)})

	// ── Phase 8.5: Orchestrator (multi-agent coordination) ──────────
	t = time.Now()
	if app.Config.Orchestrator.Enabled || len(app.Config.Orchestrator.AgentNames) > 0 || len(app.Config.Orchestrator.SubAgents) > 0 {
		orchCfg := buildOrchestratorConfig(app.Config, workDir, workingDir)
		if len(orchCfg.AgentDefinitions) > 0 {
			orch, err := orchestrator.NewOrchestrator(orchCfg, app.LLM, app.Skills, registry, app.Memory)
			if err != nil {
				progress(BootEvent{Phase: PhaseOrchestrator, Status: "warn", Message: fmt.Sprintf("orchestrator init failed: %v; running in single-agent mode", err), Dur: 0})
			} else {
				app.Orchestrator = orch
				progress(BootEvent{Phase: PhaseOrchestrator, Status: "ok", Message: fmt.Sprintf("multi-agent orchestrator enabled (%d agents)", len(orchCfg.AgentDefinitions)), Dur: time.Since(t)})
			}
		} else {
			progress(BootEvent{Phase: PhaseOrchestrator, Status: "warn", Message: "orchestrator enabled but no agent definitions found", Dur: 0})
		}
	} else {
		progress(BootEvent{Phase: PhaseOrchestrator, Status: "skip", Message: "single-agent runtime", Dur: 0})
	}

	// ── Done ─────────────────────────────────────────────────────────
	progress(BootEvent{Phase: PhaseReady, Status: "ok", Message: fmt.Sprintf("bootstrap complete in %s", time.Since(start).Round(time.Millisecond))})
	return app, nil
}

func deriveAgentContextTokenBudget(llmMaxTokens int) int {
	if llmMaxTokens <= 0 {
		return defaultAgentContextTokenFloor
	}

	// `llm.max_tokens` is the completion budget sent to the provider, not the
	// total context window. Keep a larger local guard so the system prompt and
	// chat history do not get rejected before the first request.
	budget := llmMaxTokens * 2
	if budget < defaultAgentContextTokenFloor {
		budget = defaultAgentContextTokenFloor
	}
	return budget
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
		Role:            saCfg.Role,
		ParentRef:       saCfg.ParentRef,
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

func buildOrchestratorConfig(cfg *config.Config, workDir string, workingDir string) orchestrator.OrchestratorConfig {
	orchCfg := cfg.Orchestrator

	timeout := time.Duration(orchCfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}

	defs := make([]orchestrator.AgentDefinition, 0)

	// Build definitions from agent_names (references to agent profiles)
	for _, agentName := range orchCfg.AgentNames {
		if strings.TrimSpace(agentName) == "" {
			continue
		}
		profile, ok := cfg.FindAgentProfile(agentName)
		if !ok {
			continue
		}
		def := orchestrator.AgentDefinition{
			Name:            profile.Name,
			Description:     profile.Description,
			Persona:         profile.Persona,
			Domain:          profile.Domain,
			Expertise:       profile.Expertise,
			SystemPrompt:    profile.SystemPrompt,
			PrivateSkills:   make([]string, len(profile.Skills)),
			PermissionLevel: profile.PermissionLevel,
			WorkingDir:      profile.WorkingDir,
		}
		for i, skill := range profile.Skills {
			def.PrivateSkills[i] = skill.Name
		}
		if profile.ProviderRef != "" {
			if provider, ok := cfg.FindProviderProfile(profile.ProviderRef); ok {
				def.LLMProvider = provider.Provider
				def.LLMModel = provider.DefaultModel
				def.LLMAPIKey = provider.APIKey
				def.LLMBaseURL = provider.BaseURL
			}
		}
		if def.WorkingDir == "" {
			def.WorkingDir = workingDir
		}
		defs = append(defs, def)
	}

	// Build definitions from sub_agents (explicit sub-agent configs)
	for _, saCfg := range orchCfg.SubAgents {
		if strings.TrimSpace(saCfg.Name) == "" {
			continue
		}
		def := resolveSubAgentDefinition(saCfg, cfg.LLM)
		if def.WorkingDir == "" {
			def.WorkingDir = workingDir
		}
		defs = append(defs, def)
	}

	return orchestrator.OrchestratorConfig{
		MaxConcurrentAgents: orchCfg.MaxConcurrentAgents,
		MaxRetries:          orchCfg.MaxRetries,
		Timeout:             timeout,
		AgentDefinitions:    defs,
		EnableDecomposition: orchCfg.EnableDecomposition,
	}
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

func resolveEmbedder(cfg *config.Config, secretsSnap *secrets.RuntimeSnapshot) memory.EmbeddingProvider {
	if cfg == nil {
		return nil
	}
	if embedModel := strings.TrimSpace(cfg.LLM.Extra["embed_model"]); embedModel != "" {
		baseURL := cfg.LLM.BaseURL
		if v := strings.TrimSpace(cfg.LLM.Extra["embed_base_url"]); v != "" {
			baseURL = v
		}
		apiKey := resolveSecret(secretsSnap, cfg.LLM.APIKey, "llm_api_key")
		if v := strings.TrimSpace(cfg.LLM.Extra["embed_api_key"]); v != "" {
			apiKey = resolveSecret(secretsSnap, v, "embed_api_key")
		}
		switch strings.ToLower(cfg.LLM.Provider) {
		case "ollama":
			return memory.NewOllamaEmbeddingProvider(baseURL, embedModel)
		default:
			return memory.NewOpenAIEmbeddingProvider(apiKey, embedModel)
		}
	}
	if strings.ToLower(cfg.LLM.Provider) == "ollama" {
		return memory.NewOllamaEmbeddingProvider(cfg.LLM.BaseURL, "")
	}
	apiKey := resolveSecret(secretsSnap, cfg.LLM.APIKey, "llm_api_key")
	if strings.TrimSpace(apiKey) != "" {
		return memory.NewOpenAIEmbeddingProvider(apiKey, "")
	}
	return nil
}

type qmdAdapter struct {
	client *qmd.Client
}

func (a *qmdAdapter) CreateTable(ctx context.Context, name string, columns []string) error {
	return a.client.CreateTable(ctx, name, columns)
}

func (a *qmdAdapter) Insert(ctx context.Context, table string, record map[string]any) error {
	id, _ := record["id"].(string)
	r := &qmd.Record{ID: id, Data: record}
	return a.client.Insert(ctx, table, r)
}

func (a *qmdAdapter) Get(ctx context.Context, table, id string) (map[string]any, error) {
	r, err := a.client.Get(ctx, table, id)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"id": r.ID}
	for k, v := range r.Data {
		out[k] = v
	}
	return out, nil
}

func (a *qmdAdapter) Update(ctx context.Context, table string, record map[string]any) error {
	id, _ := record["id"].(string)
	r := &qmd.Record{ID: id, Data: record}
	return a.client.Update(ctx, table, r)
}

func (a *qmdAdapter) Delete(ctx context.Context, table, id string) error {
	return a.client.Delete(ctx, table, id)
}

func (a *qmdAdapter) List(ctx context.Context, table string, limit int) ([]map[string]any, error) {
	records, err := a.client.List(ctx, table, limit)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, len(records))
	for i, r := range records {
		m := map[string]any{"id": r.ID}
		for k, v := range r.Data {
			m[k] = v
		}
		out[i] = m
	}
	return out, nil
}

func (a *qmdAdapter) Query(ctx context.Context, table, field string, value any, limit int) ([]map[string]any, error) {
	records, err := a.client.Query(ctx, table, field, value, limit)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, len(records))
	for i, r := range records {
		m := map[string]any{"id": r.ID}
		for k, v := range r.Data {
			m[k] = v
		}
		out[i] = m
	}
	return out, nil
}

func (a *qmdAdapter) ListTables(ctx context.Context) ([]tools.TableStat, error) {
	tables, err := a.client.ListTables(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]tools.TableStat, len(tables))
	for i, t := range tables {
		out[i] = tools.TableStat{Name: t.Name, RowCount: t.RowCount, Columns: t.Columns}
	}
	return out, nil
}

func (a *qmdAdapter) Count(ctx context.Context, table string) (int, error) {
	return a.client.Count(ctx, table)
}

func buildInitialSecretsSnapshot(store *secrets.Store, cfg *config.Config) *secrets.RuntimeSnapshot {
	entries := make(map[string]*secrets.SecretEntry)
	now := time.Now().UTC()

	seedSecret := func(key string, value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		entries[key] = &secrets.SecretEntry{
			ID:        fmt.Sprintf("sec-%d", time.Now().UnixNano()),
			Key:       key,
			Value:     value,
			Scope:     secrets.ScopeGlobal,
			Source:    secrets.SourceManual,
			CreatedAt: now,
			UpdatedAt: now,
		}
	}

	seedSecret("llm_api_key", cfg.LLM.APIKey)
	seedSecret("security_api_token", cfg.Security.APIToken)
	seedSecret("security_webhook_secret", cfg.Security.WebhookSecret)

	for _, p := range cfg.Providers {
		if strings.TrimSpace(p.APIKey) != "" {
			seedSecret("provider_"+p.ID+"_api_key", p.APIKey)
		}
	}

	if strings.TrimSpace(cfg.Channels.Telegram.BotToken) != "" {
		seedSecret("channel_telegram_bot_token", cfg.Channels.Telegram.BotToken)
	}
	if strings.TrimSpace(cfg.Channels.Slack.BotToken) != "" {
		seedSecret("channel_slack_bot_token", cfg.Channels.Slack.BotToken)
	}
	if strings.TrimSpace(cfg.Channels.Discord.BotToken) != "" {
		seedSecret("channel_discord_bot_token", cfg.Channels.Discord.BotToken)
	}
	if strings.TrimSpace(cfg.Channels.WhatsApp.AccessToken) != "" {
		seedSecret("channel_whatsapp_access_token", cfg.Channels.WhatsApp.AccessToken)
	}
	if strings.TrimSpace(cfg.Channels.Signal.BearerToken) != "" {
		seedSecret("channel_signal_bearer_token", cfg.Channels.Signal.BearerToken)
	}

	snap := secrets.NewRuntimeSnapshot(entries, "bootstrap")

	for _, entry := range entries {
		_ = store.SetSecret(entry)
	}
	if len(entries) > 0 {
		_, _ = store.CreateSnapshot("bootstrap")
	}

	return snap
}

func resolveSecret(snap *secrets.RuntimeSnapshot, plaintext string, secretKey string) string {
	if snap != nil {
		resolved := snap.ResolveValue(plaintext)
		if resolved != plaintext {
			return resolved
		}
		if entry, ok := snap.Get(secretKey); ok {
			return entry.Value
		}
	}
	return plaintext
}
