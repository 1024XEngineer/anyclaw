package renderdoc

import (
	"context"
	"fmt"
	"os/exec"
)

type Adapter struct {
	renderdocPath string
}

type Config struct {
	RenderdocPath string
}

func New(cfg Config) *Adapter {
	path := cfg.RenderdocPath
	if path == "" {
		path = "renderdoc"
	}
	return &Adapter{renderdocPath: path}
}

func (a *Adapter) Name() string {
	return "renderdoc"
}

func (a *Adapter) Execute(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return a.help()
	}

	cmd := args[0]
	subArgs := args[1:]

	switch cmd {
	case "capture":
		return a.capture(subArgs)
	case "info":
		return a.info(subArgs)
	case "help":
		return a.help()
	default:
		return "", fmt.Errorf("unknown command: %s", cmd)
	}
}

func (a *Adapter) capture(args []string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("capture requires <executable> <output.rdc>")
	}
	exe := args[0]
	output := args[1]

	cmd := exec.Command(a.renderdocPath, "--capture", "--exe", exe, "--output", output)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("capture failed: %w", err)
	}

	return fmt.Sprintf("Captured to %s", output), nil
}

func (a *Adapter) info(args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("info requires <capture.rdc>")
	}
	input := args[0]

	cmd := exec.Command(a.renderdocPath, "--info", input)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func (a *Adapter) help() (string, error) {
	return `RenderDoc CLI adapter (GPU frame capture)
Commands:
  capture <exe> <output.rdc> - Capture frames
  info <capture.rdc>         - Show capture info
  help                       - Show this help`, nil
}
