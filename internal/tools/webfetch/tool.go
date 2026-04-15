package webfetch

import (
	"context"

	"anyclaw/pkg/sdk"
)

// Tool is the starter web fetch tool placeholder.
type Tool struct{}

// Name returns the tool name.
func (Tool) Name() string { return "webfetch" }

// Spec returns the tool declaration.
func (Tool) Spec() sdk.ToolSpec {
	return sdk.ToolSpec{Name: "webfetch", Description: "Fetch and normalize web content.", Schema: map[string]any{"type": "object"}}
}

// Invoke executes the tool call.
func (Tool) Invoke(context.Context, sdk.ToolCall) (sdk.ToolResult, error) {
	return sdk.ToolResult{Output: "webfetch tool placeholder", Visible: true}, nil
}
