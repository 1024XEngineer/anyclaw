package agent

import "context"

type ToolApprovalHook func(ctx context.Context, tc ToolCall) error

type toolApprovalHookKey struct{}

func WithToolApprovalHook(ctx context.Context, hook ToolApprovalHook) context.Context {
	if hook == nil {
		return ctx
	}
	return context.WithValue(ctx, toolApprovalHookKey{}, hook)
}

func toolApprovalHookFromContext(ctx context.Context) ToolApprovalHook {
	if ctx == nil {
		return nil
	}
	hook, _ := ctx.Value(toolApprovalHookKey{}).(ToolApprovalHook)
	return hook
}
