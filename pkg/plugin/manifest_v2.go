package plugin

import (
	"encoding/json"
	"fmt"
	"time"

	appstore "github.com/anyclaw/anyclaw/pkg/apps"
)

// ManifestV2 插件清单 V2
type ManifestV2 struct {
	// 基础字段
	APIVersion  string    `json:"api_version"`
	PluginID    string    `json:"plugin_id"`
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	Description string    `json:"description"`
	Author      string    `json:"author,omitempty"`
	License     string    `json:"license,omitempty"`
	Homepage    string    `json:"homepage,omitempty"`
	Repository  string    `json:"repository,omitempty"`
	PublishedAt time.Time `json:"published_at,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`

	// 插件类型和配置
	Kinds          []string `json:"kinds"`
	Builtin        bool     `json:"builtin"`
	Enabled        bool     `json:"enabled"`
	Entrypoint     string   `json:"entrypoint,omitempty"`
	ExecPolicy     string   `json:"exec_policy,omitempty"`
	TimeoutSeconds int      `json:"timeout_seconds,omitempty"`

	// 平台兼容性
	Platforms     []string `json:"platforms"`
	Architectures []string `json:"architectures,omitempty"`
	MinOSVersion  string   `json:"min_os_version,omitempty"`
	MaxOSVersion  string   `json:"max_os_version,omitempty"`

	// 能力标签
	CapabilityTags []string        `json:"capability_tags,omitempty"`
	TriggerWords   []string        `json:"trigger_words,omitempty"`
	DefaultParams  json.RawMessage `json:"default_params,omitempty"`

	// 风险和安全
	RiskLevel     string   `json:"risk_level"`               // low|medium|high
	DataAccess    []string `json:"data_access,omitempty"`    // file-system, network, clipboard, etc.
	ApprovalScope string   `json:"approval_scope,omitempty"` // none|tool|action|always
	RequiresHost  bool     `json:"requires_host,omitempty"`

	// 执行策略
	RetryPolicy    *RetryPolicy    `json:"retry_policy,omitempty"`
	FallbackPolicy *FallbackPolicy `json:"fallback_policy,omitempty"`
	RollbackPolicy *RollbackPolicy `json:"rollback_policy,omitempty"`

	// 验证模板
	VerificationTemplates []VerificationTemplate `json:"verification_templates,omitempty"`

	// 恢复支持
	ResumeSupport *ResumeSupport `json:"resume_support,omitempty"`

	// 健康检查
	HealthCheck *HealthCheck `json:"health_check,omitempty"`

	// 生命周期钩子
	LifecycleHooks *LifecycleHooks `json:"lifecycle_hooks,omitempty"`

	// 插件特定配置
	Tool         *ToolSpecV2       `json:"tool,omitempty"`
	Ingress      *IngressSpecV2    `json:"ingress,omitempty"`
	Channel      *ChannelSpecV2    `json:"channel,omitempty"`
	App          *AppSpecV2        `json:"app,omitempty"`
	Node         *NodeSpecV2       `json:"node,omitempty"`
	Surface      *SurfaceSpecV2    `json:"surface,omitempty"`
	WorkflowPack *WorkflowPackSpec `json:"workflow_pack,omitempty"`

	// 签名和安全
	Signer    string `json:"signer,omitempty"`
	Signature string `json:"signature,omitempty"`
	Trust     string `json:"trust,omitempty"`
	Verified  bool   `json:"verified,omitempty"`

	// 内部字段
	sourceDir    string
	manifestPath string
}

// ToolSpecV2 工具规范 V2
type ToolSpecV2 struct {
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	Category      string         `json:"category,omitempty"`
	InputSchema   map[string]any `json:"input_schema"`
	OutputSchema  map[string]any `json:"output_schema,omitempty"`
	Examples      []ToolExample  `json:"examples,omitempty"`
	Documentation string         `json:"documentation,omitempty"`
}

type ToolExample struct {
	Input   map[string]any `json:"input"`
	Output  map[string]any `json:"output"`
	Context string         `json:"context,omitempty"`
}

// AppSpecV2 App连接器规范 V2
type AppSpecV2 struct {
	Name           string                `json:"name"`
	Description    string                `json:"description"`
	Transport      string                `json:"transport,omitempty"`
	Platforms      []string              `json:"platforms"`
	Capabilities   []string              `json:"capabilities"`
	Desktop        *appstore.DesktopSpec `json:"desktop,omitempty"`
	Actions        []AppActionSpecV2     `json:"actions"`
	Workflows      []AppWorkflowSpecV2   `json:"workflows"`
	ProbeCommand   string                `json:"probe_command,omitempty"`
	LaunchCommand  string                `json:"launch_command,omitempty"`
	VersionCommand string                `json:"version_command,omitempty"`
}

// AppActionSpecV2 App动作规范 V2
type AppActionSpecV2 struct {
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	Kind          string          `json:"kind"`
	InputSchema   map[string]any  `json:"input_schema"`
	OutputSchema  map[string]any  `json:"output_schema,omitempty"`
	RequiresFocus bool            `json:"requires_focus,omitempty"`
	TimeoutSec    int             `json:"timeout_sec,omitempty"`
	Examples      []ActionExample `json:"examples,omitempty"`
}

type ActionExample struct {
	Input   map[string]any `json:"input"`
	Output  map[string]any `json:"output"`
	Context string         `json:"context,omitempty"`
}

// AppWorkflowSpecV2 App工作流规范 V2
type AppWorkflowSpecV2 struct {
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	Action        string            `json:"action"`
	Tags          []string          `json:"tags"`
	TriggerWords  []string          `json:"trigger_words,omitempty"`
	InputSchema   map[string]any    `json:"input_schema,omitempty"`
	Defaults      map[string]any    `json:"defaults,omitempty"`
	Examples      []WorkflowExample `json:"examples,omitempty"`
	EstimatedTime int               `json:"estimated_time,omitempty"` // 秒
}

type WorkflowExample struct {
	UserRequest string         `json:"user_request"`
	Parameters  map[string]any `json:"parameters"`
	Output      string         `json:"output,omitempty"`
}

// WorkflowPackSpec 工作流包规范
type WorkflowPackSpec struct {
	Name         string           `json:"name"`
	Description  string           `json:"description"`
	Workflows    []WorkflowSpec   `json:"workflows"`
	Dependencies []DependencySpec `json:"dependencies,omitempty"`
}

type WorkflowSpec struct {
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	Steps        []WorkflowStep `json:"steps"`
	InputSchema  map[string]any `json:"input_schema,omitempty"`
	OutputSchema map[string]any `json:"output_schema,omitempty"`
	Tags         []string       `json:"tags,omitempty"`
}

type WorkflowStep struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Plugin      string         `json:"plugin"`
	Action      string         `json:"action"`
	Inputs      map[string]any `json:"inputs"`
	RetryCount  int            `json:"retry_count,omitempty"`
	TimeoutSec  int            `json:"timeout_sec,omitempty"`
}

type DependencySpec struct {
	PluginID string `json:"plugin_id"`
	Version  string `json:"version,omitempty"`
}

// 其他规范 V2
type IngressSpecV2 struct {
	Name        string   `json:"name"`
	Path        string   `json:"path"`
	Description string   `json:"description"`
	Methods     []string `json:"methods,omitempty"`
}

type ChannelSpecV2 struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Protocol    string `json:"protocol,omitempty"`
}

type NodeSpecV2 struct {
	Name         string             `json:"name"`
	Description  string             `json:"description"`
	Platforms    []string           `json:"platforms"`
	Capabilities []string           `json:"capabilities"`
	Actions      []NodeActionSpecV2 `json:"actions"`
}

type NodeActionSpecV2 struct {
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	InputSchema  map[string]any `json:"input_schema"`
	OutputSchema map[string]any `json:"output_schema,omitempty"`
}

type SurfaceSpecV2 struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Path         string   `json:"path"`
	Capabilities []string `json:"capabilities"`
}

// 策略定义
type RetryPolicy struct {
	MaxAttempts   int     `json:"max_attempts"`
	InitialDelay  int     `json:"initial_delay"` // 毫秒
	MaxDelay      int     `json:"max_delay"`     // 毫秒
	BackoffFactor float64 `json:"backoff_factor"`
}

type FallbackPolicy struct {
	AlternativeAction string `json:"alternative_action,omitempty"`
	AlternativePlugin string `json:"alternative_plugin,omitempty"`
	ManualFallback    bool   `json:"manual_fallback,omitempty"`
}

type RollbackPolicy struct {
	Steps         []RollbackStep `json:"steps,omitempty"`
	Automatic     bool           `json:"automatic"`
	OnFailureOnly bool           `json:"on_failure_only"`
}

type RollbackStep struct {
	Name   string         `json:"name"`
	Action string         `json:"action"`
	Inputs map[string]any `json:"inputs"`
}

// 验证模板
type VerificationTemplate struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Type        string         `json:"type"` // file-exists, window-appears, text-contains, etc.
	Parameters  map[string]any `json:"parameters"`
	TimeoutSec  int            `json:"timeout_sec,omitempty"`
}

// 恢复支持
type ResumeSupport struct {
	Checkpointable bool     `json:"checkpointable"`
	StateKeys      []string `json:"state_keys,omitempty"`
	ResumeActions  []string `json:"resume_actions,omitempty"`
}

// 健康检查
type HealthCheck struct {
	Command     string `json:"command,omitempty"`
	TimeoutSec  int    `json:"timeout_sec,omitempty"`
	IntervalSec int    `json:"interval_sec,omitempty"`
	Retries     int    `json:"retries,omitempty"`
}

// 生命周期钩子
type LifecycleHooks struct {
	BeforeLoad    string `json:"before_load,omitempty"`
	AfterLoad     string `json:"after_load,omitempty"`
	BeforeUnload  string `json:"before_unload,omitempty"`
	AfterUnload   string `json:"after_unload,omitempty"`
	BeforeExecute string `json:"before_execute,omitempty"`
	AfterExecute  string `json:"after_execute,omitempty"`
}

// ConvertToV1 将 V2 Manifest 转换为 V1 Manifest
func (m *ManifestV2) ConvertToV1() *Manifest {
	v1 := &Manifest{
		Name:           m.Name,
		Version:        m.Version,
		Description:    m.Description,
		Kinds:          m.Kinds,
		Builtin:        m.Builtin,
		Enabled:        m.Enabled,
		Entrypoint:     m.Entrypoint,
		TimeoutSeconds: m.TimeoutSeconds,
		Signer:         m.Signer,
		Signature:      m.Signature,
		Trust:          m.Trust,
		Verified:       m.Verified,
		sourceDir:      m.sourceDir,
		manifestPath:   m.manifestPath,
	}

	// 转换 ExecPolicy
	if m.ExecPolicy != "" {
		v1.ExecPolicy = m.ExecPolicy
	}

	// 转换 Permissions
	if len(m.DataAccess) > 0 {
		v1.Permissions = make([]string, len(m.DataAccess))
		copy(v1.Permissions, m.DataAccess)
	}

	// 转换 Tool
	if m.Tool != nil {
		v1.Tool = &ToolSpec{
			Name:        m.Tool.Name,
			Description: m.Tool.Description,
			InputSchema: m.Tool.InputSchema,
		}
	}

	// 转换 Ingress
	if m.Ingress != nil {
		v1.Ingress = &IngressSpec{
			Name:        m.Ingress.Name,
			Path:        m.Ingress.Path,
			Description: m.Ingress.Description,
		}
	}

	// 转换 Channel
	if m.Channel != nil {
		v1.Channel = &ChannelSpec{
			Name:        m.Channel.Name,
			Description: m.Channel.Description,
		}
	}

	// 转换 App
	if m.App != nil {
		v1.App = &AppSpec{
			Name:         m.App.Name,
			Description:  m.App.Description,
			Transport:    m.App.Transport,
			Platforms:    m.App.Platforms,
			Capabilities: m.App.Capabilities,
			Desktop:      m.App.Desktop,
		}

		// 转换 Actions
		if len(m.App.Actions) > 0 {
			v1.App.Actions = make([]AppActionSpec, len(m.App.Actions))
			for i, action := range m.App.Actions {
				v1.App.Actions[i] = AppActionSpec{
					Name:        action.Name,
					Description: action.Description,
					Kind:        action.Kind,
					InputSchema: action.InputSchema,
				}
			}
		}

		// 转换 Workflows
		if len(m.App.Workflows) > 0 {
			v1.App.Workflows = make([]AppWorkflowSpec, len(m.App.Workflows))
			for i, workflow := range m.App.Workflows {
				v1.App.Workflows[i] = AppWorkflowSpec{
					Name:        workflow.Name,
					Description: workflow.Description,
					Action:      workflow.Action,
					Tags:        workflow.Tags,
					InputSchema: workflow.InputSchema,
					Defaults:    workflow.Defaults,
				}
			}
		}
	}

	// 转换 Node
	if m.Node != nil {
		v1.Node = &NodeSpec{
			Name:         m.Node.Name,
			Description:  m.Node.Description,
			Platforms:    m.Node.Platforms,
			Capabilities: m.Node.Capabilities,
		}

		if len(m.Node.Actions) > 0 {
			v1.Node.Actions = make([]NodeActionSpec, len(m.Node.Actions))
			for i, action := range m.Node.Actions {
				v1.Node.Actions[i] = NodeActionSpec{
					Name:        action.Name,
					Description: action.Description,
					InputSchema: action.InputSchema,
				}
			}
		}
	}

	// 转换 Surface
	if m.Surface != nil {
		v1.Surface = &SurfaceSpec{
			Name:         m.Surface.Name,
			Description:  m.Surface.Description,
			Path:         m.Surface.Path,
			Capabilities: m.Surface.Capabilities,
		}
	}

	return v1
}

// Validate 验证 Manifest V2
func (m *ManifestV2) Validate() error {
	// 必需字段检查
	if m.APIVersion == "" {
		return &ValidationError{Field: "api_version", Reason: "required"}
	}
	if m.PluginID == "" {
		return &ValidationError{Field: "plugin_id", Reason: "required"}
	}
	if m.Name == "" {
		return &ValidationError{Field: "name", Reason: "required"}
	}
	if m.Version == "" {
		return &ValidationError{Field: "version", Reason: "required"}
	}
	if len(m.Kinds) == 0 {
		return &ValidationError{Field: "kinds", Reason: "must have at least one kind"}
	}

	// 平台检查
	if len(m.Platforms) == 0 {
		return &ValidationError{Field: "platforms", Reason: "must specify at least one platform"}
	}

	// 风险等级检查
	if m.RiskLevel == "" {
		m.RiskLevel = "medium" // 默认值
	} else if m.RiskLevel != "low" && m.RiskLevel != "medium" && m.RiskLevel != "high" {
		return &ValidationError{Field: "risk_level", Reason: "must be low, medium, or high"}
	}

	// 特定类型验证
	if contains(m.Kinds, "app") && m.App == nil {
		return &ValidationError{Field: "app", Reason: "required for app kind"}
	}
	if contains(m.Kinds, "tool") && m.Tool == nil {
		return &ValidationError{Field: "tool", Reason: "required for tool kind"}
	}
	if contains(m.Kinds, "workflow-pack") && m.WorkflowPack == nil {
		return &ValidationError{Field: "workflow_pack", Reason: "required for workflow-pack kind"}
	}

	return nil
}

// ValidationError 验证错误
type ValidationError struct {
	Field  string
	Reason string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error: field %s - %s", e.Field, e.Reason)
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// PluginLifecycle 插件生命周期管理
type PluginLifecycle struct {
	State       PluginState
	Manifest    *ManifestV2
	LoadedAt    time.Time
	LastUsed    time.Time
	Health      PluginHealth
	Metrics     PluginMetrics
	Sessions    map[string]*PluginSession
	SuspendData map[string]any
}

type PluginSession struct {
	SessionID string
	BoundAt   time.Time
	Context   map[string]any
}

type PluginState string

const (
	PluginStateDiscovered PluginState = "discovered"
	PluginStateVerified   PluginState = "verified"
	PluginStateLoaded     PluginState = "loaded"
	PluginStateIndexed    PluginState = "indexed"
	PluginStateBound      PluginState = "bound"
	PluginStateExecuting  PluginState = "executing"
	PluginStateSuspended  PluginState = "suspended"
	PluginStateUnloaded   PluginState = "unloaded"
)

type PluginHealth struct {
	Status    string // healthy, warning, error
	Message   string
	CheckedAt time.Time
}

type PluginMetrics struct {
	ExecutionCount int
	SuccessCount   int
	FailureCount   int
	AverageTime    time.Duration
	LastExecuted   time.Time
}
