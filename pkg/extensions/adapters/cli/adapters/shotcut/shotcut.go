package shotcut

import (
	"context"
	"fmt"
	"os/exec"
)

type Adapter struct {
	meltPath   string
	ffmpegPath string
}

type Config struct {
	MeltPath   string
	FfmpegPath string
}

func New(cfg Config) *Adapter {
	meltPath := cfg.MeltPath
	if meltPath == "" {
		meltPath = "melt"
	}
	ffmpegPath := cfg.FfmpegPath
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}
	return &Adapter{meltPath: meltPath, ffmpegPath: ffmpegPath}
}

func (a *Adapter) Name() string {
	return "shotcut"
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
	case "convert":
		return a.convert(subArgs)
	case "help":
		return a.help()
	default:
		return "", fmt.Errorf("unknown command: %s", cmd)
	}
}

func (a *Adapter) render(args []string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("render requires <input> <output>")
	}
	input := args[0]
	output := args[1]

	cmd := exec.Command(a.meltPath, input, "-", "out", output)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("render failed: %w", err)
	}

	return fmt.Sprintf("Rendered to %s", output), nil
}

func (a *Adapter) convert(args []string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("convert requires <input> <output>")
	}
	input := args[0]
	output := args[1]

	cmd := exec.Command(a.ffmpegPath, "-i", input, "-c:v", "libx264", "-preset", "fast", "-crf", "23", "-c:a", "aac", "-b:a", "128k", output)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("convert failed: %w", err)
	}

	return fmt.Sprintf("Converted to %s", output), nil
}

func (a *Adapter) help() (string, error) {
	return `Shotcut CLI adapter (via melt/ffmpeg)
Commands:
  render <input> <output> - Render video (melt)
  convert <input> <output> - Convert format (ffmpeg)
  help                    - Show this help`, nil
}
