package obsstudio

import (
	"context"
	"fmt"
	"os/exec"
)

type Adapter struct {
	obsPath string
}

type Config struct {
	ObsPath string
}

func New(cfg Config) *Adapter {
	path := cfg.ObsPath
	if path == "" {
		path = "obs"
	}
	return &Adapter{obsPath: path}
}

func (a *Adapter) Name() string {
	return "obs-studio"
}

func (a *Adapter) Execute(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return a.help()
	}

	cmd := args[0]
	subArgs := args[1:]

	switch cmd {
	case "start":
		return a.start(subArgs)
	case "stop":
		return a.stop()
	case "scene":
		return a.scene(subArgs)
	case "help":
		return a.help()
	default:
		return "", fmt.Errorf("unknown command: %s", cmd)
	}
}

func (a *Adapter) start(args []string) (string, error) {
	mode := "recording"
	if len(args) > 0 {
		mode = args[0]
	}

	cmd := exec.Command(a.obsPath, "--start"+mode)
	if err := cmd.Run(); err != nil {
		return "", err
	}

	return fmt.Sprintf("Started %s", mode), nil
}

func (a *Adapter) stop() (string, error) {
	cmd := exec.Command(a.obsPath, "--stop")
	if err := cmd.Run(); err != nil {
		return "", err
	}

	return "Stopped", nil
}

func (a *Adapter) scene(args []string) (string, error) {
	if len(args) == 0 {
		return "Available: list, switch <name>", nil
	}

	action := args[0]
	switch action {
	case "list":
		cmd := exec.Command(a.obsPath, "--scene-list")
		output, err := cmd.CombinedOutput()
		if err != nil {
			return "", err
		}
		return string(output), nil
	case "switch":
		if len(args) < 2 {
			return "", fmt.Errorf("switch requires <scene_name>")
		}
		cmd := exec.Command(a.obsPath, "--scene-switch", args[1])
		if err := cmd.Run(); err != nil {
			return "", err
		}
		return fmt.Sprintf("Switched to %s", args[1]), nil
	default:
		return "", fmt.Errorf("unknown scene action: %s", action)
	}
}

func (a *Adapter) help() (string, error) {
	return `OBS Studio CLI adapter
Commands:
  start [recording/streaming] - Start recording/streaming
  stop                         - Stop
  scene list                   - List scenes
  scene switch <name>          - Switch scene
  help                         - Show this help`, nil
}
