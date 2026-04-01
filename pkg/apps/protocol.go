package apps

const DesktopProtocolVersion = "anyclaw.app.desktop.v1"

type DesktopSpec struct {
	LaunchCommand        string   `json:"launch_command,omitempty"`
	WindowTitle          string   `json:"window_title,omitempty"`
	WindowClass          string   `json:"window_class,omitempty"`
	FocusStrategy        string   `json:"focus_strategy,omitempty"`
	DetectionHints       []string `json:"detection_hints,omitempty"`
	RequiresHostReviewed bool     `json:"requires_host_reviewed,omitempty"`
}

type ProtocolContext struct {
	Version      string          `json:"version"`
	Transport    string          `json:"transport,omitempty"`
	Platforms    []string        `json:"platforms,omitempty"`
	Capabilities []string        `json:"capabilities,omitempty"`
	Host         HostContext     `json:"host"`
	Action       ActionContext   `json:"action"`
	Desktop      *DesktopContext `json:"desktop,omitempty"`
}

type HostContext struct {
	OS             string   `json:"os"`
	AvailableTools []string `json:"available_tools,omitempty"`
}

type ActionContext struct {
	Name string `json:"name"`
	Kind string `json:"kind,omitempty"`
}

type DesktopContext struct {
	LaunchCommand        string   `json:"launch_command,omitempty"`
	WindowTitle          string   `json:"window_title,omitempty"`
	WindowClass          string   `json:"window_class,omitempty"`
	FocusStrategy        string   `json:"focus_strategy,omitempty"`
	DetectionHints       []string `json:"detection_hints,omitempty"`
	RequiresHostReviewed bool     `json:"requires_host_reviewed,omitempty"`
}

type DesktopPlan struct {
	Protocol string            `json:"protocol"`
	Summary  string            `json:"summary,omitempty"`
	Result   string            `json:"result,omitempty"`
	Steps    []DesktopPlanStep `json:"steps,omitempty"`
}

type DesktopPlanStep struct {
	Tool            string            `json:"tool"`
	Label           string            `json:"label,omitempty"`
	Target          map[string]any    `json:"target,omitempty"`
	Action          string            `json:"action,omitempty"`
	Input           map[string]any    `json:"input,omitempty"`
	Value           *string           `json:"value,omitempty"`
	Append          *bool             `json:"append,omitempty"`
	Submit          *bool             `json:"submit,omitempty"`
	Retry           int               `json:"retry,omitempty"`
	RetryDelayMS    int               `json:"retry_delay_ms,omitempty"`
	WaitAfterMS     int               `json:"wait_after_ms,omitempty"`
	ContinueOnError bool              `json:"continue_on_error,omitempty"`
	Verify          *DesktopPlanCheck `json:"verify,omitempty"`
	OnFailure       []DesktopPlanStep `json:"on_failure,omitempty"`
}

type DesktopPlanCheck struct {
	Tool         string         `json:"tool"`
	Target       map[string]any `json:"target,omitempty"`
	Input        map[string]any `json:"input,omitempty"`
	Retry        int            `json:"retry,omitempty"`
	RetryDelayMS int            `json:"retry_delay_ms,omitempty"`
}

type ProbeResult struct {
	Installed bool   `json:"installed"`
	Running   bool   `json:"running,omitempty"`
	Version   string `json:"version,omitempty"`
	Path      string `json:"path,omitempty"`
	Error     string `json:"error,omitempty"`
}

type ResumeState struct {
	PlanID         string         `json:"plan_id"`
	StepIndex      int            `json:"step_index"`
	CompletedSteps []int          `json:"completed_steps,omitempty"`
	AppState       map[string]any `json:"app_state,omitempty"`
	WindowState    *WindowState   `json:"window_state,omitempty"`
	Data           map[string]any `json:"data,omitempty"`
	Checkpoint     string         `json:"checkpoint,omitempty"`
}

type WindowState struct {
	Title   string `json:"title,omitempty"`
	Class   string `json:"class,omitempty"`
	Visible bool   `json:"visible,omitempty"`
	Focused bool   `json:"focused,omitempty"`
	BoundsX int    `json:"bounds_x,omitempty"`
	BoundsY int    `json:"bounds_y,omitempty"`
	BoundsW int    `json:"bounds_w,omitempty"`
	BoundsH int    `json:"bounds_h,omitempty"`
}

type CleanupSpec struct {
	Enabled       bool            `json:"enabled,omitempty"`
	CloseApps     bool            `json:"close_apps,omitempty"`
	RestoreWindow bool            `json:"restore_window,omitempty"`
	TempFiles     []string        `json:"temp_files,omitempty"`
	CustomActions []CleanupAction `json:"custom_actions,omitempty"`
}

type CleanupAction struct {
	Type  string         `json:"type"`
	Input map[string]any `json:"input,omitempty"`
}

type VerificationResult struct {
	Passed    bool           `json:"passed"`
	Type      string         `json:"type"`
	Evidence  map[string]any `json:"evidence,omitempty"`
	Message   string         `json:"message,omitempty"`
	Timestamp string         `json:"timestamp"`
}

type AppConnector interface {
	Probe(ctx interface{}) (*ProbeResult, error)
	Bootstrap(ctx interface{}) error
	Execute(ctx interface{}, action string, params map[string]any) (map[string]any, error)
	Verify(ctx interface{}, spec *VerificationSpec) (*VerificationResult, error)
	Resume(ctx interface{}, state *ResumeState) error
	Cleanup(ctx interface{}, spec *CleanupSpec) error
	GetCapabilities() []string
	GetActions() []string
}

type VerificationSpec struct {
	Type       string         `json:"type"`
	Parameters map[string]any `json:"parameters"`
	TimeoutSec int            `json:"timeout_sec,omitempty"`
	RetryCount int            `json:"retry_count,omitempty"`
	Evidence   bool           `json:"evidence,omitempty"`
}
