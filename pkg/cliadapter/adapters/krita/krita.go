package krita

import (
	"context"
	"fmt"
	"os/exec"
)

type Adapter struct {
	kritaPath string
}

type Config struct {
	KritaPath string
}

func New(cfg Config) *Adapter {
	path := cfg.KritaPath
	if path == "" {
		path = "krita"
	}
	return &Adapter{kritaPath: path}
}

func (a *Adapter) Name() string {
	return "krita"
}

func (a *Adapter) Execute(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return a.help()
	}

	cmd := args[0]
	subArgs := args[1:]

	switch cmd {
	case "export":
		return a.export(subArgs)
	case "help":
		return a.help()
	default:
		return "", fmt.Errorf("unknown command: %s", cmd)
	}
}

func (a *Adapter) export(args []string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("export requires <input.kra> <output.png>")
	}
	input := args[0]
	output := args[1]

	cmd := exec.Command(a.kritaPath, "--export", "--export-filename", output, input)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("export failed: %w", err)
	}

	return fmt.Sprintf("Exported to %s", output), nil
}

func (a *Adapter) help() (string, error) {
	return `Krita CLI adapter
Commands:
  export <input.kra> <output.png> - Export to format
  help                           - Show this help`, nil
}
