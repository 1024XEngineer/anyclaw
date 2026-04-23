package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	agent "github.com/1024XEngineer/anyclaw/pkg/capability/agents"
	llm "github.com/1024XEngineer/anyclaw/pkg/capability/models"
	"github.com/1024XEngineer/anyclaw/pkg/capability/skills"
	"github.com/1024XEngineer/anyclaw/pkg/capability/tools"
	"github.com/1024XEngineer/anyclaw/pkg/config"
	"github.com/1024XEngineer/anyclaw/pkg/extensions/plugin"
	"github.com/1024XEngineer/anyclaw/pkg/qmd"
	"github.com/1024XEngineer/anyclaw/pkg/runtime/orchestrator"
	"github.com/1024XEngineer/anyclaw/pkg/state/audit"
	"github.com/1024XEngineer/anyclaw/pkg/state/memory"
	"github.com/1024XEngineer/anyclaw/pkg/state/policy/secrets"
	"github.com/1024XEngineer/anyclaw/pkg/workspace"
)

// NewTargetRuntime creates a main runtime with an isolated work dir for a target agent/workspace.
func NewTargetRuntime(configPath string, agentName string, workingDir string) (*MainRuntime, error) {
	cfg, err := LoadConfig(configPath)
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

// NewTargetApp preserves the legacy constructor name while callers migrate to MainRuntime naming.
func NewTargetApp(configPath string, agentName string, workingDir string) (*App, error) {
	return NewTargetRuntime(configPath, agentName, workingDir)
}

// Bootstrap initializes the application in well-defined phases.
// Each phase emits a BootEvent through opts.Progress (if set).
func Bootstrap(opts BootstrapOptions) (*MainRuntime, error) {
	start := time.Now()
	progress := opts.Progress
	if progress == nil {
		progress = func(BootEvent) {}
	}

	app := &MainRuntime{ConfigPath: opts.ConfigPath}

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
		cfg, err := LoadConfig(cfgPath)
		if err != nil {
			progress(BootEvent{Phase: PhaseConfig, Status: "fail", Message: "config load failed", Err: err, Dur: time.Since(t)})
			return nil, fmt.Errorf("config: %w", err)
		}
		app.Config = cfg
	}
	_ = app.Config.ApplyDefaultProviderProfile()
	app.ConfigPath = ResolveConfigPath(app.ConfigPath)
	resolveRuntimePaths(app.Config, app.ConfigPath)
	progress(BootEvent{Phase: PhaseConfig, Status: "ok", Message: fmt.Sprintf("provider=%s model=%s", app.Config.LLM.Provider, app.Config.LLM.Model), Dur: time.Since(t)})

	progress(BootEvent{Phase: PhaseSecurity, Status: "start", Message: "initializing secrets"})
	t = time.Now()

	secretsConfigDir := filepath.Dir(app.ConfigPath)
	if secretsConfigDir == "" {
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
	if err := workspace.EnsureBootstrap(workingDir, buildWorkspaceBootstrapOptions(app.Config)); err != nil {
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
			_ = warmupper.Warmup(warmupCfg.Queries, 4)
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

	progress(BootEvent{Phase: PhaseSecurity, Status: "start", Message: "initializing security"})
	t = time.Now()

	auditLogger := audit.New(app.Config.Security.AuditLog, app.Config.Agent.Name)
	app.Audit = auditLogger

	if app.SecretsManager != nil {
		secretsSnap := app.SecretsManager.GetActiveSnapshot()
		app.Config.Security.APIToken = resolveSecret(secretsSnap, app.Config.Security.APIToken, "security_api_token")
		app.Config.Security.WebhookSecret = resolveSecret(secretsSnap, app.Config.Security.WebhookSecret, "security_webhook_secret")
	}

	secured := strings.TrimSpace(app.Config.Security.APIToken) != ""
	progress(BootEvent{Phase: PhaseSecurity, Status: "ok", Message: fmt.Sprintf("audit_log=%s secured=%v", app.Config.Security.AuditLog, secured), Dur: time.Since(t)})

	progress(BootEvent{Phase: PhaseQMD, Status: "start", Message: "initializing QMD"})
	t = time.Now()

	qmdServer := qmd.NewServer(qmd.ServerConfig{HTTPAddr: "127.0.0.1:0"})
	if err := qmdServer.Start(); err != nil {
		progress(BootEvent{Phase: PhaseQMD, Status: "warn", Message: fmt.Sprintf("QMD server failed to start, running without structured data store: %v", err), Dur: time.Since(t)})
	} else {
		qmdClient := qmd.NewClient(qmd.ClientConfig{
			Address:    "http://" + qmdServer.HTTPAddr(),
			Protocol:   qmd.ProtocolHTTP,
			Timeout:    qmd.DefaultTimeout,
			RetryCount: qmd.DefaultRetryCount,
			RetryDelay: qmd.DefaultRetryDelay,
		})
		ctx := context.Background()
		if err := qmdClient.Ping(ctx); err != nil {
			progress(BootEvent{Phase: PhaseQMD, Status: "warn", Message: fmt.Sprintf("QMD server not reachable: %v", err), Dur: time.Since(t)})
			_ = qmdServer.Shutdown(context.Background())
		} else {
			app.QMD = qmdClient
			app.qmdServer = qmdServer
			progress(BootEvent{Phase: PhaseQMD, Status: "ok", Message: "QMD in-memory data store ready", Dur: time.Since(t)})
		}
	}

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
		AllowedReadPaths:      app.Config.Security.AllowedReadPaths,
		AllowedWritePaths:     app.Config.Security.AllowedWritePaths,
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

	progress(BootEvent{Phase: PhasePlugins, Status: "start", Message: "loading plugins"})
	t = time.Now()

	plugRegistry, err := plugin.NewRegistry(app.Config.Plugins)
	if err != nil {
		progress(BootEvent{Phase: PhasePlugins, Status: "fail", Message: "plugin load failed", Err: err, Dur: time.Since(t)})
		return nil, fmt.Errorf("plugins: %w", err)
	}
	plugRegistry.SetPolicyEngine(policyEngine)
	plugRegistry.RegisterToolPlugins(registry, app.Config.Plugins.Dir)
	app.Plugins = plugRegistry

	pluginCount := len(plugRegistry.List())
	if pluginCount == 0 {
		progress(BootEvent{Phase: PhasePlugins, Status: "skip", Message: "no plugins found", Dur: time.Since(t)})
	} else {
		progress(BootEvent{Phase: PhasePlugins, Status: "ok", Message: fmt.Sprintf("%d plugin(s) loaded", pluginCount), Dur: time.Since(t)})
	}

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

	t = time.Now()
	if app.Config.Orchestrator.Enabled || len(app.Config.Orchestrator.AgentNames) > 0 || len(app.Config.Orchestrator.SubAgents) > 0 {
		orchCfg := buildOrchestratorConfig(app.Config, workDir, workingDir)
		if len(orchCfg.AgentDefinitions) > 0 {
			orch, err := orchestrator.NewOrchestrator(orchCfg, app.LLM, app.Skills, registry, app.Memory)
			if err != nil {
				progress(BootEvent{Phase: PhaseOrchestrator, Status: "warn", Message: fmt.Sprintf("orchestrator init failed: %v; running in single-agent mode", err), Dur: 0})
			} else {
				app.Orchestrator = orch
				registerDelegationTool(app)
				if warnings := orch.InitWarnings(); len(warnings) > 0 {
					progress(BootEvent{Phase: PhaseOrchestrator, Status: "warn", Message: fmt.Sprintf("multi-agent orchestrator enabled with %d active agent(s); warnings: %s", orch.AgentCount(), strings.Join(warnings, "; ")), Dur: time.Since(t)})
				} else {
					progress(BootEvent{Phase: PhaseOrchestrator, Status: "ok", Message: fmt.Sprintf("multi-agent orchestrator enabled (%d agents)", len(orchCfg.AgentDefinitions)), Dur: time.Since(t)})
				}
			}
		} else {
			progress(BootEvent{Phase: PhaseOrchestrator, Status: "warn", Message: "orchestrator enabled but no agent definitions found", Dur: 0})
		}
	} else {
		progress(BootEvent{Phase: PhaseOrchestrator, Status: "skip", Message: "single-agent runtime", Dur: 0})
	}

	progress(BootEvent{Phase: PhaseReady, Status: "ok", Message: fmt.Sprintf("bootstrap complete in %s", time.Since(start).Round(time.Millisecond))})
	return app, nil
}

func buildWorkspaceBootstrapOptions(cfg *config.Config) workspace.BootstrapOptions {
	opts := workspace.BootstrapOptions{}
	if cfg == nil {
		return opts
	}

	opts.AgentName = cfg.Agent.Name
	opts.AgentDescription = cfg.Agent.Description
	opts.UserProfile = bootstrapUserProfile(cfg)
	opts.WorkspaceFocus = strings.TrimSpace(cfg.Agent.WorkFocus)
	opts.AssistantStyle = strings.TrimSpace(cfg.Agent.BehaviorStyle)
	opts.Constraints = strings.TrimSpace(cfg.Agent.Constraints)
	return opts
}

func bootstrapUserProfile(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}

	parts := []string{}
	if lang := strings.TrimSpace(cfg.Agent.Lang); lang != "" {
		parts = append(parts, "Default language: "+lang)
	}
	return strings.Join(parts, "; ")
}
