package agent

import (
	"context"

	"anyclaw/internal/tools"
	"anyclaw/pkg/sdk"
)

// ToolExecutor isolates tool lookup and tool invocation.
type ToolExecutor struct {
	Tools *tools.Registry
}

// Invoke resolves and executes one tool call.
func (t ToolExecutor) Invoke(ctx context.Context, call sdk.ToolCall) (sdk.ToolResult, error) {
	tool, ok := t.Tools.Get(call.Name)
	if !ok {
		return sdk.ToolResult{Error: "tool not found", Visible: true}, nil
	}
	return tool.Invoke(ctx, call)
}
