package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anyclaw/anyclaw/pkg/config"
)

func TestRunConfigSetGetAndUnset(t *testing.T) {
	cfg := config.DefaultConfig()
	configPath := filepath.Join(t.TempDir(), "anyclaw.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save config: %v", err)
	}

	if err := runConfigCommand([]string{"set", "--config", configPath, "plugins.enabled[0]", "demo-plugin"}); err != nil {
		t.Fatalf("runConfigCommand set: %v", err)
	}

	output := captureStdout(t, func() {
		if err := runConfigCommand([]string{"get", "--config", configPath, "plugins.enabled[0]"}); err != nil {
			t.Fatalf("runConfigCommand get: %v", err)
		}
	})
	if strings.TrimSpace(output) != "demo-plugin" {
		t.Fatalf("unexpected get output: %q", output)
	}

	if err := runConfigCommand([]string{"unset", "--config", configPath, "plugins.enabled[0]"}); err != nil {
		t.Fatalf("runConfigCommand unset: %v", err)
	}

	if err := runConfigCommand([]string{"get", "--config", configPath, "plugins.enabled[0]"}); err == nil {
		t.Fatalf("expected get on removed path to fail")
	}
}

func TestRunConfigValidateJSON(t *testing.T) {
	cfg := config.DefaultConfig()
	configPath := filepath.Join(t.TempDir(), "anyclaw.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save config: %v", err)
	}

	output := captureStdout(t, func() {
		if err := runConfigCommand([]string{"validate", "--config", configPath, "--json"}); err != nil {
			t.Fatalf("runConfigCommand validate: %v", err)
		}
	})

	var payload struct {
		OK   bool   `json:"ok"`
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("Unmarshal output: %v\noutput=%s", err, output)
	}
	if !payload.OK {
		t.Fatalf("expected ok=true, got %#v", payload)
	}
	if strings.TrimSpace(payload.Path) == "" {
		t.Fatalf("expected path in payload, got %#v", payload)
	}
}
