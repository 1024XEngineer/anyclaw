package inkscape

import (
	"context"
	"fmt"
	"os/exec"
)

type Adapter struct {
	inkscapePath string
}

type Config struct {
	InkscapePath string
}

func New(cfg Config) *Adapter {
	path := cfg.InkscapePath
	if path == "" {
		path = "inkscape"
	}
	return &Adapter{inkscapePath: path}
}

func (a *Adapter) Name() string {
	return "inkscape"
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
		return "", fmt.Errorf("export requires <input.svg> <output.png/pdf/eps>")
	}
	input := args[0]
	output := args[1]

	cmd := exec.Command(a.inkscapePath, "--export-filename", output, input)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("export failed: %w", err)
	}

	return fmt.Sprintf("Exported to %s", output), nil
}

func (a *Adapter) convert(args []string) (string, error) {
	return a.export(args)
}

func (a *Adapter) help() (string, error) {
	return `Inkscape CLI adapter
Commands:
  export <input.svg> <output.png/pdf/eps> - Export to format
  convert <input> <output>               - Convert (alias)
  help                                    - Show this help`, nil
}
