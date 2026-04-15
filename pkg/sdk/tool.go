package sdk

import "context"

// ToolSpec describes a tool's static contract.
type ToolSpec struct {
	Name        string
	Description string
	Schema      map[string]any
}

// ToolCall is one structured tool invocation request.
type ToolCall struct {
	ID   string
	Name string
	Args map[string]any
}

// ToolResult is the normalized tool execution result.
type ToolResult struct {
	Output  any
	Error   string
	Meta    map[string]any
	Visible bool
}

// Tool executes one named capability for the agent runtime.
type Tool interface {
	Name() string
	Spec() ToolSpec
	Invoke(ctx context.Context, call ToolCall) (ToolResult, error)
}

// ToolRegistry stores the available tools for a running app.
type ToolRegistry interface {
	Register(tool Tool)
	Get(name string) (Tool, bool)
	List() []Tool
}
