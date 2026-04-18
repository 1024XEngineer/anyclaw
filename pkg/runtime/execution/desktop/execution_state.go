package desktop

import (
	"context"
	"time"
)

type DesktopPlanStepExecutionState struct {
	Index     int    `json:"index"`
	Tool      string `json:"tool"`
	Label     string `json:"label,omitempty"`
	HasVerify bool   `json:"has_verify,omitempty"`
	Verified  bool   `json:"verified,omitempty"`
	Status    string `json:"status,omitempty"`
	Attempts  int    `json:"attempts,omitempty"`
	Output    string `json:"output,omitempty"`
	Error     string `json:"error,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type DesktopPlanExecutionState struct {
	ToolName          string                          `json:"tool_name,omitempty"`
	Plugin            string                          `json:"plugin,omitempty"`
	App               string                          `json:"app,omitempty"`
	Action            string                          `json:"action,omitempty"`
	Workflow          string                          `json:"workflow,omitempty"`
	Status            string                          `json:"status,omitempty"`
	Summary           string                          `json:"summary,omitempty"`
	Result            string                          `json:"result,omitempty"`
	TotalSteps        int                             `json:"total_steps,omitempty"`
	CurrentStep       int                             `json:"current_step,omitempty"`
	NextStep          int                             `json:"next_step,omitempty"`
	LastCompletedStep int                             `json:"last_completed_step,omitempty"`
	LastOutput        string                          `json:"last_output,omitempty"`
	LastError         string                          `json:"last_error,omitempty"`
	Resumed           bool                            `json:"resumed,omitempty"`
	Steps             []DesktopPlanStepExecutionState `json:"steps,omitempty"`
	UpdatedAt         string                          `json:"updated_at,omitempty"`
}

type DesktopPlanStateHook func(ctx context.Context, state DesktopPlanExecutionState)

type desktopPlanStateHookKey struct{}
type desktopPlanResumeStateKey struct{}

func WithDesktopPlanStateHook(ctx context.Context, hook DesktopPlanStateHook) context.Context {
	if hook == nil {
		return ctx
	}
	return context.WithValue(ctx, desktopPlanStateHookKey{}, hook)
}

func ReportDesktopPlanState(ctx context.Context, state DesktopPlanExecutionState) {
	if ctx == nil {
		return
	}
	hook, _ := ctx.Value(desktopPlanStateHookKey{}).(DesktopPlanStateHook)
	if hook == nil {
		return
	}
	hook(ctx, state)
}

func WithDesktopPlanResumeState(ctx context.Context, state *DesktopPlanExecutionState) context.Context {
	if state == nil {
		return ctx
	}
	return context.WithValue(ctx, desktopPlanResumeStateKey{}, CloneDesktopPlanExecutionState(state))
}

func DesktopPlanResumeStateFromContext(ctx context.Context) *DesktopPlanExecutionState {
	if ctx == nil {
		return nil
	}
	state, _ := ctx.Value(desktopPlanResumeStateKey{}).(*DesktopPlanExecutionState)
	return CloneDesktopPlanExecutionState(state)
}

func CloneDesktopPlanExecutionState(state *DesktopPlanExecutionState) *DesktopPlanExecutionState {
	if state == nil {
		return nil
	}
	cloned := *state
	if len(state.Steps) > 0 {
		cloned.Steps = append([]DesktopPlanStepExecutionState(nil), state.Steps...)
	}
	return &cloned
}

func TimestampNowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}
