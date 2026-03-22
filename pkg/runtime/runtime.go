package runtime

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/anyclaw/anyclaw/pkg/agent"
	"github.com/anyclaw/anyclaw/pkg/audit"
	"github.com/anyclaw/anyclaw/pkg/config"
	"github.com/anyclaw/anyclaw/pkg/llm"
	"github.com/anyclaw/anyclaw/pkg/memory"
	"github.com/anyclaw/anyclaw/pkg/plugin"
	"github.com/anyclaw/anyclaw/pkg/skills"
	"github.com/anyclaw/anyclaw/pkg/tools"
)

const Version = "2026.3.13"

type App struct {
	ConfigPath string
	Config     *config.Config
	Agent      *agent.Agent
	LLM        *llm.ClientWrapper
	Memory     *memory.FileMemory
	Skills     *skills.SkillsManager
	Tools      *tools.Registry
	Plugins    *plugin.Registry
	Audit      *audit.Logger
	WorkDir    string
	WorkingDir string
}

func LoadConfig(configPath string) (*config.Config, error) {
	if configPath == "" {
		configPath = "anyclaw.json"
	}
	return config.Load(configPath)
}

func NewApp(configPath string) (*App, error) {
	if configPath == "" {
		configPath = "anyclaw.json"
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}
	return NewAppFromConfig(configPath, cfg)
}

func NewAppFromConfig(configPath string, cfg *config.Config) (*App, error) {
	return newAppFromConfig(configPath, cfg)
}

func NewTargetApp(configPath string, agentName string, workingDir string) (*App, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
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
	return newAppFromConfig(configPath, cfg)
}

func newAppFromConfig(configPath string, cfg *config.Config) (*App, error) {

	workDir := cfg.Agent.WorkDir
	if workDir == "" {
		workDir = ".anyclaw"
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create work dir: %w", err)
	}

	workingDir := cfg.Agent.WorkingDir
	if workingDir == "" {
		workingDir = "workflows"
	}
	if cfg.Agent.ActiveProfile != "" {
		_ = cfg.ApplyAgentProfile(cfg.Agent.ActiveProfile)
		if cfg.Agent.WorkingDir != "" {
			workingDir = cfg.Agent.WorkingDir
		}
	}

	absWorkingDir, err := filepath.Abs(workingDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve working dir: %w", err)
	}
	workingDir = absWorkingDir
	if err := os.MkdirAll(workingDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create working dir: %w", err)
	}

	mem := memory.NewFileMemory(workDir)
	if err := mem.Init(); err != nil {
		return nil, fmt.Errorf("failed to initialize memory: %w", err)
	}

	sk := skills.NewSkillsManager(cfg.Skills.Dir)
	if err := sk.Load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load skills: %w", err)
	}
	boundSkillNames := enabledProfileSkills(cfg.Agent.Profiles, cfg.Agent.ActiveProfile)
	if len(boundSkillNames) > 0 {
		sk = sk.FilterEnabled(boundSkillNames)
	}

	registry := tools.NewRegistry()
	auditLogger := audit.New(cfg.Security.AuditLog, cfg.Agent.Name)
	sandboxManager := tools.NewSandboxManager(cfg.Sandbox, workingDir)
	tools.RegisterBuiltins(registry, tools.BuiltinOptions{
		WorkingDir:            workingDir,
		PermissionLevel:       cfg.Agent.PermissionLevel,
		DangerousPatterns:     cfg.Security.DangerousCommandPatterns,
		CommandTimeoutSeconds: cfg.Security.CommandTimeoutSeconds,
		AuditLogger:           auditLogger,
		Sandbox:               sandboxManager,
	})
	sk.RegisterTools(registry, skills.ExecutionOptions{AllowExec: cfg.Plugins.AllowExec, ExecTimeoutSeconds: cfg.Plugins.ExecTimeoutSeconds})

	plugRegistry, err := plugin.NewRegistry(cfg.Plugins)
	if err != nil {
		return nil, fmt.Errorf("failed to load plugins: %w", err)
	}
	plugRegistry.RegisterToolPlugins(registry, cfg.Plugins.Dir)

	llmWrapper, err := llm.NewClientWrapper(llm.Config{
		Provider:    cfg.LLM.Provider,
		Model:       cfg.LLM.Model,
		APIKey:      cfg.LLM.APIKey,
		BaseURL:     cfg.LLM.BaseURL,
		Proxy:       cfg.LLM.Proxy,
		MaxTokens:   cfg.LLM.MaxTokens,
		Temperature: cfg.LLM.Temperature,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize LLM client: %w", err)
	}

	ag := agent.New(agent.Config{
		Name:        cfg.Agent.Name,
		Description: cfg.Agent.Description,
		Personality: agent.BuildPersonalityPrompt(firstEnabledProfilePersonality(cfg.Agent.Profiles, cfg.Agent.ActiveProfile)),
		LLM:         llmWrapper,
		Memory:      mem,
		Skills:      sk,
		Tools:       registry,
		WorkDir:     workDir,
	})

	return &App{
		ConfigPath: configPath,
		Config:     cfg,
		Agent:      ag,
		LLM:        llmWrapper,
		Memory:     mem,
		Skills:     sk,
		Tools:      registry,
		Plugins:    plugRegistry,
		Audit:      auditLogger,
		WorkDir:    workDir,
		WorkingDir: workingDir,
	}, nil
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
