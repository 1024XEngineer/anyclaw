package sdk

import "context"

// HookPoint identifies a lifecycle stage where extension logic can run.
type HookPoint string

const (
	HookBeforeModelResolve HookPoint = "before_model_resolve"
	HookBeforePromptBuild  HookPoint = "before_prompt_build"
	HookBeforeToolCall     HookPoint = "before_tool_call"
	HookAfterToolCall      HookPoint = "after_tool_call"
	HookAgentEnd           HookPoint = "agent_end"
)

// HookContext is the shared context passed to hook implementations.
type HookContext struct {
	RunID      string
	AgentID    string
	SessionKey string
	Data       map[string]any
}

// Hook allows extensions to participate in lifecycle events.
type Hook interface {
	Point() HookPoint
	Run(ctx context.Context, hc HookContext) error
}

// HookRegistry stores installed hooks.
type HookRegistry interface {
	Register(hook Hook)
	List(point HookPoint) []Hook
}
