package runtime

import (
	"context"
	"fmt"
	"time"

	"github.com/anyclaw/anyclaw/pkg/agent"
	"github.com/anyclaw/anyclaw/pkg/audit"
	"github.com/anyclaw/anyclaw/pkg/config"
	"github.com/anyclaw/anyclaw/pkg/cron"
	"github.com/anyclaw/anyclaw/pkg/llm"
	"github.com/anyclaw/anyclaw/pkg/memory"
	"github.com/anyclaw/anyclaw/pkg/orchestrator"
	"github.com/anyclaw/anyclaw/pkg/plugin"
	"github.com/anyclaw/anyclaw/pkg/prompt"
	"github.com/anyclaw/anyclaw/pkg/qmd"
	"github.com/anyclaw/anyclaw/pkg/secrets"
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
	Delegation     *DelegationService
	QMD            *qmd.Client
	SecretsManager *secrets.ActivationManager
	SecretsStore   *secrets.Store
	WorkDir        string
	WorkingDir     string
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

func (a *App) GetHistory() []prompt.Message {
	if a == nil || a.Agent == nil {
		return nil
	}
	return a.Agent.GetHistory()
}

func (a *App) SetHistory(history []prompt.Message) {
	if a == nil || a.Agent == nil {
		return
	}
	a.Agent.SetHistory(history)
}

func (a *App) ListTools() []tools.ToolInfo {
	if a == nil || a.Agent == nil {
		return nil
	}
	return a.Agent.ListTools()
}

func (a *App) ListSkills() []skills.SkillInfo {
	if a == nil || a.Agent == nil {
		return nil
	}
	return a.Agent.ListSkills()
}

func (a *App) ShowMemory() (string, error) {
	if a == nil || a.Agent == nil {
		return "", fmt.Errorf("runtime memory is unavailable: agent is not initialized")
	}
	return a.Agent.ShowMemory()
}

func (a *App) CallTool(ctx context.Context, name string, input map[string]any) (string, error) {
	if a == nil || a.Tools == nil {
		return "", fmt.Errorf("runtime tool registry is unavailable")
	}
	return a.Tools.Call(ctx, name, input)
}

func (a *App) ToolRegistry() *tools.Registry {
	if a == nil {
		return nil
	}
	return a.Tools
}

func (a *App) PluginRegistry() *plugin.Registry {
	if a == nil {
		return nil
	}
	return a.Plugins
}

func (a *App) ListPlugins() []plugin.Manifest {
	if a == nil || a.Plugins == nil {
		return nil
	}
	return a.Plugins.List()
}

func (a *App) Chat(ctx context.Context, messages []llm.Message, toolDefs []llm.ToolDefinition) (*llm.Response, error) {
	if a == nil || a.LLM == nil {
		return nil, fmt.Errorf("runtime llm is unavailable")
	}
	return a.LLM.Chat(ctx, messages, toolDefs)
}

func (a *App) StreamChat(ctx context.Context, messages []llm.Message, toolDefs []llm.ToolDefinition, onChunk func(string)) error {
	if a == nil || a.LLM == nil {
		return fmt.Errorf("runtime llm is unavailable")
	}
	return a.LLM.StreamChat(ctx, messages, toolDefs, onChunk)
}

func (a *App) LLMName() string {
	if a == nil || a.LLM == nil {
		return ""
	}
	return a.LLM.Name()
}

func (a *App) Name() string {
	return a.LLMName()
}

func (a *App) HasLLM() bool {
	return a != nil && a.LLM != nil
}

func (a *App) SetLLMClient(client *llm.ClientWrapper) {
	if a == nil {
		return
	}
	a.LLM = client
}

func (a *App) LLMClient() *llm.ClientWrapper {
	if a == nil {
		return nil
	}
	return a.LLM
}

func (a *App) NewCronExecutor() *cron.AgentExecutor {
	if a == nil {
		return nil
	}
	return cron.NewAgentExecutor(a.Agent, a.Orchestrator)
}
