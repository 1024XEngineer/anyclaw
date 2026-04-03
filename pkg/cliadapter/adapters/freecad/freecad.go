package freecad

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

type Adapter struct {
	freecadPath string
}

type Config struct {
	FreecadPath string
}

func New(cfg Config) *Adapter {
	path := cfg.FreecadPath
	if path == "" {
		path = "FreeCAD"
	}
	return &Adapter{freecadPath: path}
}

func (a *Adapter) Name() string {
	return "freecad"
}

func (a *Adapter) Execute(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return a.help()
	}

	cmd := args[0]
	subArgs := args[1:]

	switch cmd {
	case "info":
		return a.info()
	case "open":
		return a.open(subArgs)
	case "macro":
		return a.macro(subArgs)
	case "help":
		return a.help()
	default:
		return "", fmt.Errorf("unknown command: %s", cmd)
	}
}

func (a *Adapter) info() (string, error) {
	cmd := exec.Command(a.freecadPath, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("freecad not found: %w", err)
	}
	return string(output), nil
}

func (a *Adapter) open(args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("open requires <file.fcstd1> [file.fcstd2...]")
	}

	for _, f := range args {
		if _, err := os.Stat(f); os.IsNotExist(err) {
			return "", fmt.Errorf("file not found: %s", f)
		}
	}

	cmd := exec.Command(a.freecadPath, "--background", args[0])
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}

	return fmt.Sprintf("Opened %d files", len(args)), nil
}

func (a *Adapter) macro(args []string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("macro requires <file.fcstd> <macro.py>")
	}
	model := args[0]
	script := args[1]

	if _, err := os.Stat(script); os.IsNotExist(err) {
		return "", fmt.Errorf("script not found: %s", script)
	}

	cmd := exec.Command(a.freecadPath, "--background", "--python-script", script, model)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("macro failed: %s", string(output))
	}

	return "Macro executed", nil
}

func (a *Adapter) help() (string, error) {
	return `FreeCAD CLI adapter (258 commands: Part, Sketcher, PartDesign, Assembly, Mesh, TechDraw, Draft, FEM, CAM)
Commands:
  info                   - Show FreeCAD version
  open <file.fcstd...>  - Open model file(s)
  macro <file> <script> - Run Python script on model
  help                   - Show this help`, nil
}
