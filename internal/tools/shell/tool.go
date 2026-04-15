package shell

import (
	"context"

	"anyclaw/pkg/sdk"
)

// Tool is the starter shell tool placeholder.
type Tool struct{}

// Name returns the tool name.
func (Tool) Name() string { return "shell" }

// Spec returns the tool declaration.
func (Tool) Spec() sdk.ToolSpec {
	return sdk.ToolSpec{Name: "shell", Description: "Execute a controlled shell command.", Schema: map[string]any{"type": "object"}}
}

// Invoke executes the tool call.
func (Tool) Invoke(context.Context, sdk.ToolCall) (sdk.ToolResult, error) {
	return sdk.ToolResult{Output: "shell tool placeholder", Visible: true}, nil
}
