package gimp

import (
	"context"
	"fmt"
	"os/exec"
)

type Adapter struct {
	gimpPath string
}

type Config struct {
	GimpPath string
}

func New(cfg Config) *Adapter {
	path := cfg.GimpPath
	if path == "" {
		path = "gimp"
	}
	return &Adapter{gimpPath: path}
}

func (a *Adapter) Name() string {
	return "gimp"
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
	case "resize":
		return a.resize(subArgs)
	case "crop":
		return a.crop(subArgs)
	case "help":
		return a.help()
	default:
		return "", fmt.Errorf("unknown command: %s", cmd)
	}
}

func (a *Adapter) convert(args []string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("convert requires <input> <output>")
	}
	input := args[0]
	output := args[1]

	script := fmt.Sprintf("(gimp-file-load RUN-NONINTERACTIVE \"%s\" \"%s\") (gimp-file-save RUN-NONINTERACTIVE (car (gimp-image-get-active-layer 1)) (car (gimp-image-get-active-drawable 1)) \"%s\" \"%s\") (gimp-quit 1)", input, input, output, output)

	cmd := exec.Command(a.gimpPath, "-i", "-b", script)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("conversion failed: %w", err)
	}

	return fmt.Sprintf("Converted to %s", output), nil
}

func (a *Adapter) resize(args []string) (string, error) {
	if len(args) < 3 {
		return "", fmt.Errorf("resize requires <input> <width> <height>")
	}
	input := args[0]
	width := args[1]
	height := args[2]

	script := fmt.Sprintf("(gimp-file-load RUN-NONINTERACTIVE \"%s\" \"%s\") (gimp-image-scale 1 %s %s) (gimp-file-save RUN-NONINTERACTIVE (car (gimp-image-get-active-layer 1)) (car (gimp-image-get-active-drawable 1)) \"%s\" \"%s\") (gimp-quit 1)", input, input, width, height, input, input)

	cmd := exec.Command(a.gimpPath, "-i", "-b", script)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("resize failed: %w", err)
	}

	return fmt.Sprintf("Resized to %sx%s", width, height), nil
}

func (a *Adapter) crop(args []string) (string, error) {
	if len(args) < 5 {
		return "", fmt.Errorf("crop requires <input> <x> <y> <width> <height>")
	}
	input := args[0]
	x, y, w, h := args[1], args[2], args[3], args[4]

	script := fmt.Sprintf("(gimp-file-load RUN-NONINTERACTIVE \"%s\" \"%s\") (gimp-image-crop 1 %s %s %s %s) (gimp-file-save RUN-NONINTERACTIVE (car (gimp-image-get-active-layer 1)) (car (gimp-image-get-active-drawable 1)) \"%s\" \"%s\") (gimp-quit 1)", input, input, w, h, x, y, input, input)

	cmd := exec.Command(a.gimpPath, "-i", "-b", script)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("crop failed: %w", err)
	}

	return "Cropped successfully", nil
}

func (a *Adapter) help() (string, error) {
	return `GIMP CLI adapter (batch mode)
Commands:
  convert <input> <output>           - Convert format
  resize <input> <width> <height>   - Resize image
  crop <input> <x> <y> <w> <h>      - Crop image
  help                               - Show this help`, nil
}
