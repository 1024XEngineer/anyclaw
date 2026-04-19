package sketch

import (
	"context"
	"fmt"
	"os/exec"
)

type Adapter struct {
	nodePath string
}

type Config struct {
	NodePath string
}

func New(cfg Config) *Adapter {
	path := cfg.NodePath
	if path == "" {
		path = "node"
	}
	return &Adapter{nodePath: path}
}

func (a *Adapter) Name() string {
	return "sketch"
}

func (a *Adapter) Execute(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return a.help()
	}

	cmd := args[0]
	subArgs := args[1:]

	switch cmd {
	case "create":
		return a.create(subArgs)
	case "export":
		return a.export(subArgs)
	case "help":
		return a.help()
	default:
		return "", fmt.Errorf("unknown command: %s", cmd)
	}
}

func (a *Adapter) create(args []string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("create requires <output.sketch> <json-spec>")
	}
	output := args[0]
	spec := args[1]

	cmd := exec.Command(a.nodePath, "sketch-cli", "create", "-o", output, spec)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("create failed: %w", err)
	}

	return fmt.Sprintf("Created %s", output), nil
}

func (a *Adapter) export(args []string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("export requires <input.sketch> <output.pdf/png>")
	}
	input := args[0]
	output := args[1]

	cmd := exec.Command(a.nodePath, "sketch-cli", "export", "-o", output, input)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("export failed: %w", err)
	}

	return fmt.Sprintf("Exported to %s", output), nil
}

func (a *Adapter) help() (string, error) {
	return `Sketch CLI adapter (via sketch-constructor)
Commands:
  create <output.sketch> <spec.json> - Create sketch file
  export <input.sketch> <output>      - Export to format
  help                               - Show this help`, nil
}
