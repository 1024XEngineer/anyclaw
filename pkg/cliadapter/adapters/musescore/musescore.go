package musescore

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Adapter struct {
	mscorePath string
}

type Config struct {
	MscorePath string
}

func New(cfg Config) *Adapter {
	path := cfg.MscorePath
	if path == "" {
		path = "mscore"
	}
	return &Adapter{mscorePath: path}
}

func (a *Adapter) Name() string {
	return "musescore"
}

func (a *Adapter) Execute(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return a.help()
	}

	cmd := args[0]
	subArgs := args[1:]

	switch cmd {
	case "convert":
		return a.convert(subArgs)
	case "export":
		return a.export(subArgs)
	case "info":
		return a.info(subArgs)
	case "help":
		return a.help()
	default:
		return "", fmt.Errorf("unknown command: %s", cmd)
	}
}

func (a *Adapter) convert(args []string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("convert requires <input.mscz> <output.pdf/mp3/midi>")
	}
	input := args[0]
	output := args[1]

	cmd := exec.Command(a.mscorePath, "-o", output, input)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("conversion failed: %w", err)
	}

	return fmt.Sprintf("Converted %s to %s", input, output), nil
}

func (a *Adapter) export(args []string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("export requires <input.mscz> <format>")
	}
	input := args[0]
	format := args[1]

	ext := "." + format
	output := strings.TrimSuffix(filepath.Base(input), filepath.Ext(input)) + ext

	cmd := exec.Command(a.mscorePath, "-o", output, input)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("export failed: %w", err)
	}

	return fmt.Sprintf("Exported to %s", output), nil
}

func (a *Adapter) info(args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("info requires <input.mscz>")
	}
	input := args[0]

	cmd := exec.Command(a.mscorePath, "-i", input)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("info failed: %w", err)
	}

	return string(output), nil
}

func (a *Adapter) help() (string, error) {
	return `MuseScore CLI adapter
Commands:
  convert <input.mscz> <output.pdf/mp3/midi>  - Convert score
  export <input.mscz> <format>               - Export to format
  info <input.mscz>                           - Show score info
  help                                        - Show this help`, nil
}
