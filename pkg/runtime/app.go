package runtime

import (
	"context"
	"fmt"
	"time"

	agent "github.com/1024XEngineer/anyclaw/pkg/capability/agents"
	llm "github.com/1024XEngineer/anyclaw/pkg/capability/models"
	"github.com/1024XEngineer/anyclaw/pkg/capability/skills"
	"github.com/1024XEngineer/anyclaw/pkg/capability/tools"
	"github.com/1024XEngineer/anyclaw/pkg/config"
	"github.com/1024XEngineer/anyclaw/pkg/extensions/plugin"
	"github.com/1024XEngineer/anyclaw/pkg/qmd"
	runtimeschedule "github.com/1024XEngineer/anyclaw/pkg/runtime/execution/schedule"
	"github.com/1024XEngineer/anyclaw/pkg/runtime/orchestrator"
	"github.com/1024XEngineer/anyclaw/pkg/state"
	"github.com/1024XEngineer/anyclaw/pkg/state/audit"
	"github.com/1024XEngineer/anyclaw/pkg/state/memory"
	"github.com/1024XEngineer/anyclaw/pkg/state/policy/secrets"
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

// MainRuntime is the main-agent execution container for a specific target.
type MainRuntime struct {
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

// App is kept as a legacy alias while callers migrate to MainRuntime naming.
type App = MainRuntime

// LoadConfig loads configuration from disk with validation.
func LoadConfig(configPath string) (*config.Config, error) {
	if configPath == "" {
		configPath = "anyclaw.json"
	}
	return config.Load(configPath)
}

func NewMainRuntime(configPath string) (*MainRuntime, error) {
	if configPath == "" {
		configPath = "anyclaw.json"
	}
	return Bootstrap(BootstrapOptions{ConfigPath: configPath})
}

func NewMainRuntimeFromConfig(configPath string, cfg *config.Config) (*MainRuntime, error) {
	return Bootstrap(BootstrapOptions{ConfigPath: configPath, Config: cfg})
}

// NewApp creates an App from a config file path (legacy API).
func NewApp(configPath string) (*App, error) {
	return NewMainRuntime(configPath)
}

// NewAppFromConfig creates an App from an existing config (legacy API).
func NewAppFromConfig(configPath string, cfg *config.Config) (*App, error) {
	return NewMainRuntimeFromConfig(configPath, cfg)
}

func (a *MainRuntime) GetHistory() []state.HistoryMessage {
	if a == nil || a.Agent == nil {
		return nil
	}
	return FromPromptMessages(a.Agent.GetHistory())
}

func (a *MainRuntime) SetHistory(history []state.HistoryMessage) {
	if a == nil || a.Agent == nil {
		return
	}
	a.Agent.SetHistory(ToPromptMessages(history))
}

func (a *MainRuntime) ListTools() []tools.ToolInfo {
	if a == nil || a.Agent == nil {
		return nil
	}
	return a.Agent.ListTools()
}

func (a *MainRuntime) ListSkills() []skills.SkillInfo {
	if a == nil || a.Agent == nil {
		return nil
	}
	return a.Agent.ListSkills()
}

func (a *MainRuntime) ShowMemory() (string, error) {
	if a == nil || a.Agent == nil {
		return "", fmt.Errorf("runtime memory is unavailable: agent is not initialized")
	}
	return a.Agent.ShowMemory()
}

func (a *MainRuntime) HasMemory() bool {
	return a != nil && a.Memory != nil
}

func (a *MainRuntime) CallTool(ctx context.Context, name string, input map[string]any) (string, error) {
	if a == nil || a.Tools == nil {
		return "", fmt.Errorf("runtime tool registry is unavailable")
	}
	return a.Tools.Call(ctx, name, input)
}

func (a *MainRuntime) ToolRegistry() *tools.Registry {
	if a == nil {
		return nil
	}
	return a.Tools
}

func (a *MainRuntime) PluginRegistry() *plugin.Registry {
	if a == nil {
		return nil
	}
	return a.Plugins
}

func (a *MainRuntime) ListPlugins() []plugin.Manifest {
	if a == nil || a.Plugins == nil {
		return nil
	}
	return a.Plugins.List()
}

func (a *MainRuntime) Chat(ctx context.Context, messages []llm.Message, toolDefs []llm.ToolDefinition) (*llm.Response, error) {
	if a == nil || a.LLM == nil {
		return nil, fmt.Errorf("runtime llm is unavailable")
	}
	return a.LLM.Chat(ctx, messages, toolDefs)
}

func (a *MainRuntime) StreamChat(ctx context.Context, messages []llm.Message, toolDefs []llm.ToolDefinition, onChunk func(string)) error {
	if a == nil || a.LLM == nil {
		return fmt.Errorf("runtime llm is unavailable")
	}
	return a.LLM.StreamChat(ctx, messages, toolDefs, onChunk)
}

func (a *MainRuntime) LLMName() string {
	if a == nil || a.LLM == nil {
		return ""
	}
	return a.LLM.Name()
}

func (a *MainRuntime) Name() string {
	return a.LLMName()
}

func (a *MainRuntime) HasLLM() bool {
	return a != nil && a.LLM != nil
}

func (a *MainRuntime) SetLLMClient(client *llm.ClientWrapper) {
	if a == nil {
		return
	}
	a.LLM = client
}

func (a *MainRuntime) LLMClient() *llm.ClientWrapper {
	if a == nil {
		return nil
	}
	return a.LLM
}

func (a *MainRuntime) NewCronExecutor() *runtimeschedule.AgentExecutor {
	if a == nil {
		return nil
	}
	return runtimeschedule.NewAgentExecutor(a.Agent, a.Orchestrator)
}
