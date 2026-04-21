package tools

import "context"

type ToolApprovalCall struct {
	Name string
	Args map[string]any
}

type ToolApprovalHook func(ctx context.Context, call ToolApprovalCall) error

type ToolCallerRole string

const (
	ToolCallerRoleUnknown   ToolCallerRole = ""
	ToolCallerRoleMainAgent ToolCallerRole = "main_agent"
	ToolCallerRoleSubAgent  ToolCallerRole = "sub_agent"
	ToolCallerRoleSystem    ToolCallerRole = "system"
)

type ToolCaller struct {
	Role        ToolCallerRole
	AgentName   string
	ExecutionID string
}

type toolApprovalHookKey struct{}
type toolCallerKey struct{}

func WithToolApprovalHook(ctx context.Context, hook ToolApprovalHook) context.Context {
	if hook == nil {
		return ctx
	}
	return context.WithValue(ctx, toolApprovalHookKey{}, hook)
}

func RequestToolApproval(ctx context.Context, name string, args map[string]any) error {
	if ctx == nil {
		return nil
	}
	hook, _ := ctx.Value(toolApprovalHookKey{}).(ToolApprovalHook)
	if hook == nil {
		return nil
	}
	return hook(ctx, ToolApprovalCall{Name: name, Args: args})
}

func WithToolCaller(ctx context.Context, caller ToolCaller) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, toolCallerKey{}, caller)
}

func ToolCallerFromContext(ctx context.Context) ToolCaller {
	if ctx == nil {
		return ToolCaller{}
	}
	caller, _ := ctx.Value(toolCallerKey{}).(ToolCaller)
	return caller
}
