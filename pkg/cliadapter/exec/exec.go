package exec

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	cr "github.com/anyclaw/anyclaw/pkg/cliadapter/registry"
)

type Executor struct {
	mu       sync.RWMutex
	reg      *cr.Registry
	handlers map[string]CommandHandler
}

type CommandHandler func(ctx context.Context, args []string) (string, error)

type ExecResult struct {
	Name      string   `json:"name"`
	Args      []string `json:"args"`
	Output    string   `json:"output"`
	Error     string   `json:"error,omitempty"`
	ExitCode  int      `json:"exit_code"`
	Installed bool     `json:"installed"`
}

func NewExecutor(reg *cr.Registry) *Executor {
	return &Executor{
		reg:      reg,
		handlers: make(map[string]CommandHandler),
	}
}

func (e *Executor) RegisterHandler(name string, handler CommandHandler) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.handlers[name] = handler
}

func (e *Executor) Exec(ctx context.Context, name string, args []string) *ExecResult {
	result := &ExecResult{
		Name: name,
		Args: args,
	}

	entry, ok := e.reg.Find(name)
	if !ok {
		result.Error = fmt.Sprintf("CLI not found: %s", name)
		return result
	}

	e.mu.RLock()
	if handler, exists := e.handlers[name]; exists {
		e.mu.RUnlock()
		output, err := handler(ctx, args)
		result.Output = output
		if err != nil {
			result.Error = err.Error()
			result.ExitCode = 1
		} else {
			result.ExitCode = 0
		}
		result.Installed = true
		return result
	}
	e.mu.RUnlock()

	if entry.Installed && entry.ExecutablePath != "" {
		return e.execBinary(ctx, entry.ExecutablePath, args, result)
	}

	result.Error = fmt.Sprintf("CLI not installed: %s (install with: %s)", name, entry.InstallCmd)
	result.Installed = false
	return result
}

func (e *Executor) execBinary(ctx context.Context, path string, args []string, result *ExecResult) *ExecResult {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil

	output, err := cmd.CombinedOutput()
	result.Output = string(output)
	result.Installed = true

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = 1
		}
		result.Error = err.Error()
	} else {
		result.ExitCode = 0
	}

	return result
}

func (e *Executor) AutoInstall(ctx context.Context, name string) *ExecResult {
	result := &ExecResult{Name: name}

	entry, ok := e.reg.Get(name)
	if !ok {
		result.Error = "CLI not found in registry"
		return result
	}

	if entry.InstallCmd == "" {
		result.Error = "No install command available"
		return result
	}

	parts := strings.Fields(entry.InstallCmd)
	if len(parts) < 2 {
		result.Error = "Invalid install command"
		return result
	}

	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		result.Error = fmt.Sprintf("Install failed: %v", err)
		result.ExitCode = 1
		return result
	}

	result.Output = "Installation completed"
	result.ExitCode = 0
	result.Installed = true
	e.reg.MarkInstalled(name, entry.EntryPoint)

	return result
}

func (e *Executor) List() []*cr.EntryStatus {
	return e.reg.List()
}

func (e *Executor) Search(query, category string, limit int) []*cr.EntryStatus {
	return e.reg.Search(query, category, limit)
}

func (e *Executor) Categories() map[string]int {
	return e.reg.Categories()
}

func (e *Executor) JSON() (string, error) {
	return e.reg.JSON()
}
