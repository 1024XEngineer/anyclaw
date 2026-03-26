package tools

import (
	"context"
	"fmt"
)

type AuditLogger interface {
	LogTool(toolName string, input map[string]any, output string, err error)
}

type DangerousCommandConfirmer func(command string) bool

type BuiltinOptions struct {
	WorkingDir              string
	PermissionLevel         string
	ExecutionMode           string
	DangerousPatterns       []string
	ProtectedPaths          []string
	CommandTimeoutSeconds   int
	ConfirmDangerousCommand DangerousCommandConfirmer
	AuditLogger             AuditLogger
	Sandbox                 *SandboxManager
}

type ToolFunc func(ctx context.Context, input map[string]any) (string, error)

type Tool struct {
	Name        string
	Description string
	InputSchema map[string]any
	Handler     ToolFunc
}

type Registry struct {
	tools map[string]*Tool
}

func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]*Tool),
	}
}

func (r *Registry) RegisterTool(name string, desc string, schema map[string]any, handler ToolFunc) {
	r.tools[name] = &Tool{
		Name:        name,
		Description: desc,
		InputSchema: schema,
		Handler:     handler,
	}
}

func (r *Registry) Register(t *Tool) {
	r.tools[t.Name] = t
}

func (r *Registry) Get(name string) (*Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

func (r *Registry) ListTools() []*Tool {
	var list []*Tool
	for _, t := range r.tools {
		list = append(list, t)
	}
	return list
}

func (r *Registry) Call(ctx context.Context, name string, input map[string]any) (string, error) {
	t, ok := r.tools[name]
	if !ok {
		return "", fmt.Errorf("tool not found: %s", name)
	}

	if t.Handler == nil {
		return "", fmt.Errorf("tool handler not implemented: %s", name)
	}

	return t.Handler(ctx, input)
}

type ToolInfo struct {
	Name        string
	Description string
	InputSchema map[string]any
}

func (r *Registry) List() []ToolInfo {
	var list []ToolInfo
	for _, t := range r.tools {
		list = append(list, ToolInfo{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		})
	}
	return list
}
