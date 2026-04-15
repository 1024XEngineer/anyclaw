package nodes

import "context"

// Command is a normalized node invocation request.
type Command struct {
	Name string
	Args map[string]any
}

// Result is a normalized node invocation result.
type Result struct {
	Output any
	Error  string
}

// Node describes a connected capability host.
type Node interface {
	ID() string
	Caps() []string
	Invoke(ctx context.Context, command Command) (Result, error)
}
