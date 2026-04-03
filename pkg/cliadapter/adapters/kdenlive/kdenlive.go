package kdenlive

import (
	"context"
	"fmt"
	"os/exec"
)

type Adapter struct {
	meltPath string
}

type Config struct {
	MeltPath string
}

func New(cfg Config) *Adapter {
	path := cfg.MeltPath
	if path == "" {
		path = "melt"
	}
	return &Adapter{meltPath: path}
}

func (a *Adapter) Name() string {
	return "kdenlive"
}

func (a *Adapter) Execute(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return a.help()
	}

	cmd := args[0]
	subArgs := args[1:]

	switch cmd {
	case "render":
		return a.render(subArgs)
	case "info":
		return a.info(subArgs)
	case "help":
		return a.help()
	default:
		return "", fmt.Errorf("unknown command: %s", cmd)
	}
}

func (a *Adapter) render(args []string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("render requires <profile> <output>")
	}
	profile := args[0]
	output := args[1]

	cmd := exec.Command(a.meltPath, profile, "-", "out", output)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("render failed: %w", err)
	}

	return fmt.Sprintf("Rendered to %s", output), nil
}

func (a *Adapter) info(args []string) (string, error) {
	cmd := exec.Command(a.meltPath, "-query", "profiles")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func (a *Adapter) help() (string, error) {
	return `Kdenlive CLI adapter (via melt)
Commands:
  render <profile> <output> - Render video
  info                      - List profiles
  help                      - Show this help`, nil
}
