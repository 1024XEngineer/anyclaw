package mermaid

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Adapter struct{}

func New() *Adapter {
	return &Adapter{}
}

func (a *Adapter) Name() string {
	return "mermaid"
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
	case "serve":
		return a.serve(subArgs)
	case "validate":
		return a.validate(subArgs)
	case "help":
		return a.help()
	default:
		return "", fmt.Errorf("unknown command: %s", cmd)
	}
}

func (a *Adapter) render(args []string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("render requires <input.mmd> <output.png/pdf/svg>")
	}
	input := args[0]
	output := args[1]

	ext := strings.ToLower(filepath.Ext(output))
	format := strings.TrimPrefix(ext, ".")

	outputDir := filepath.Dir(output)
	if outputDir == "." {
		outputDir = "./dist"
		os.MkdirAll(outputDir, 0755)
	}

	cmd := exec.Command("npx", "-y", "@mermaid-js/mermaid-cli", "-i", input, "-o", outputDir, "-t", format)
	cmd.Dir = filepath.Dir(input)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("render failed: %s", string(out))
	}

	return fmt.Sprintf("Rendered to %s", output), nil
}

func (a *Adapter) serve(args []string) (string, error) {
	port := "8080"
	if len(args) > 0 {
		port = args[0]
	}

	cmd := exec.Command("npx", "-y", "mermaid", "-p", port)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return "", err
	}

	return fmt.Sprintf("Mermaid server started on port %s", port), nil
}

func (a *Adapter) validate(args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("validate requires <input.mmd>")
	}
	input := args[0]

	cmd := exec.Command("npx", "-y", "@mermaid-js/mermaid-cli", "-i", input, "-o", "/dev/null")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("validation failed: %s", string(output))
	}

	return "Valid Mermaid diagram", nil
}

func (a *Adapter) help() (string, error) {
	return `Mermaid CLI adapter
Commands:
  render <input.mmd> <output>  - Render diagram to file
  serve [port]                - Start local server
  validate <input.mmd>        - Validate diagram syntax
  help                        - Show this help`, nil
}
