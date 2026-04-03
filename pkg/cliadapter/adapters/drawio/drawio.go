package drawio

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

type Adapter struct {
	drawioPath string
}

type Config struct {
	DrawioPath string
}

func New(cfg Config) *Adapter {
	path := cfg.DrawioPath
	if path == "" {
		path = "drawio"
	}
	return &Adapter{drawioPath: path}
}

func (a *Adapter) Name() string {
	return "drawio"
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
	case "convert":
		return a.convert(subArgs)
	case "help":
		return a.help()
	default:
		return "", fmt.Errorf("unknown command: %s", cmd)
	}
}

func (a *Adapter) export(args []string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("export requires <input.drawio> <output.pdf/png/svg>")
	}
	input := args[0]
	output := args[1]

	ext := strings.ToLower(filepath.Ext(output))
	format := strings.TrimPrefix(ext, ".")

	cmd := exec.Command(a.drawioPath, "--export", "--output", output, "--format", format, input)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("export failed: %w", err)
	}

	return fmt.Sprintf("Exported to %s", output), nil
}

func (a *Adapter) convert(args []string) (string, error) {
	return a.export(args)
}

func (a *Adapter) help() (string, error) {
	return `Draw.io CLI adapter
Commands:
  export <input.drawio> <output.pdf/png/svg> - Export diagram
  convert <input> <output>                   - Convert (alias)
  help                                       - Show this help`, nil
}
