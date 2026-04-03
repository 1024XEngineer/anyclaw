package audacity

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/anyclaw/anyclaw/v2/pkg/cliadapter/exec"
)

type Adapter struct {
	exec *exec.Executor
}

func New(executor *exec.Executor) *Adapter {
	return &Adapter{exec: executor}
}

func (a *Adapter) Name() string {
	return "audacity"
}

func (a *Adapter) Execute(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("no command specified")
	}

	cmd := args[0]
	subArgs := args[1:]

	switch cmd {
	case "info":
		return a.info()
	case "convert":
		return a.convert(subArgs)
	case "trim":
		return a.trim(subArgs)
	default:
		return "", fmt.Errorf("unknown command: %s", cmd)
	}
}

func (a *Adapter) info() (string, error) {
	cmd := exec.Command("sox", "--version")
	output, err := a.exec.Run(ctx, cmd)
	if err != nil {
		return "", fmt.Errorf("sox not installed: %w", err)
	}
	return output, nil
}

func (a *Adapter) convert(args []string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("convert requires <input> <output>")
	}
	input := args[0]
	output := args[1]

	ext := strings.ToLower(filepath.Ext(output))
	format := strings.TrimPrefix(ext, ".")

	cmd := exec.Command("sox", input, "-t", format, output)
	_, err := a.exec.Run(ctx, cmd)
	if err != nil {
		return "", fmt.Errorf("conversion failed: %w", err)
	}
	return fmt.Sprintf("Converted %s to %s", input, output), nil
}

func (a *Adapter) trim(args []string) (string, error) {
	if len(args) < 3 {
		return "", fmt.Errorf("trim requires <input> <output> <duration>")
	}
	input := args[0]
	output := args[1]
	duration := args[2]

	cmd := exec.Command("sox", input, output, "trim", "0", duration)
	_, err := a.exec.Run(ctx, cmd)
	if err != nil {
		return "", fmt.Errorf("trim failed: %w", err)
	}
	return fmt.Sprintf("Trimmed %s to %s seconds", output, duration), nil
}
