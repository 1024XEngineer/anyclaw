package bootstrap

import (
	"path/filepath"
	"testing"

	"github.com/1024XEngineer/anyclaw/pkg/config"
)

func TestResolveRuntimePathsResolvesControlUIRoot(t *testing.T) {
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "anyclaw.json")

	cfg := config.DefaultConfig()
	cfg.Gateway.ControlUI.Root = "dist/control-ui"

	ResolveRuntimePaths(cfg, configPath)

	expected := filepath.Join(configDir, "dist", "control-ui")
	if cfg.Gateway.ControlUI.Root != expected {
		t.Fatalf("expected control UI root %q, got %q", expected, cfg.Gateway.ControlUI.Root)
	}
}
