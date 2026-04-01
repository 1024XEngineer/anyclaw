package tools

import "context"

type ToolApprovalCall struct {
	Name string
	Args map[string]any
}

type ToolApprovalHook func(ctx context.Context, call ToolApprovalCall) error

type toolApprovalHookKey struct{}

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
