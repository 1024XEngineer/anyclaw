package memory

import (
	"context"

	"anyclaw/pkg/sdk"
)

// Tool is the starter memory tool placeholder.
type Tool struct{}

// Name returns the tool name.
func (Tool) Name() string { return "memory" }

// Spec returns the tool declaration.
func (Tool) Spec() sdk.ToolSpec {
	return sdk.ToolSpec{Name: "memory", Description: "Read and write long-term memory.", Schema: map[string]any{"type": "object"}}
}

// Invoke executes the tool call.
func (Tool) Invoke(context.Context, sdk.ToolCall) (sdk.ToolResult, error) {
	return sdk.ToolResult{Output: "memory tool placeholder", Visible: true}, nil
}
