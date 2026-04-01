package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anyclaw/anyclaw/pkg/apps"
	"github.com/anyclaw/anyclaw/pkg/config"
	"github.com/anyclaw/anyclaw/pkg/plugin"
)

func TestRunAppWorkflowsResolvePrintsMatches(t *testing.T) {
	baseDir := t.TempDir()
	pluginDir := filepath.Join(baseDir, "image-app")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	manifest := plugin.Manifest{
		Name:        "image-app",
		Version:     "1.0.0",
		Enabled:     true,
		Entrypoint:  "app.py",
		Permissions: []string{"tool:exec"},
		App: &plugin.AppSpec{
			Name:         "Image App",
			Description:  "Image workflow connector",
			Transport:    "desktop",
			Platforms:    []string{"windows"},
			Capabilities: []string{"vision"},
			Desktop: &apps.DesktopSpec{
				LaunchCommand: "image-app.exe",
				WindowTitle:   "Image App",
			},
			Actions: []plugin.AppActionSpec{
				{Name: "run"},
			},
			Workflows: []plugin.AppWorkflowSpec{
				{
					Name:        "remove-background",
					Description: "Remove the background and export png",
					Action:      "run",
					Tags:        []string{"background", "png", "image"},
				},
			},
		},
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), data, 0o644); err != nil {
		t.Fatalf("WriteFile manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "app.py"), []byte("print('ok')\n"), 0o644); err != nil {
		t.Fatalf("WriteFile entrypoint: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Plugins.Dir = baseDir
	cfg.Plugins.AllowExec = true
	cfg.Plugins.RequireTrust = false
	cfg.Plugins.Enabled = []string{"image-app"}
	configPath := filepath.Join(t.TempDir(), "anyclaw.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save config: %v", err)
	}

	output := captureStdout(t, func() {
		if err := runAppWorkflowsResolve([]string{"--config", configPath, "--query", "remove background from this image"}); err != nil {
			t.Fatalf("runAppWorkflowsResolve: %v", err)
		}
	})

	if !strings.Contains(output, "app_image_app_workflow_remove_background") {
		t.Fatalf("expected workflow tool name in output, got %q", output)
	}
	if !strings.Contains(output, "Image App") {
		t.Fatalf("expected app name in output, got %q", output)
	}
}

func TestRunAppPairingsSetAndList(t *testing.T) {
	cfg := config.DefaultConfig()
	configPath := filepath.Join(t.TempDir(), "anyclaw.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save config: %v", err)
	}

	if err := runAppPairingsSet([]string{
		"--config", configPath,
		"--app", "qq-local",
		"--workflow", "send-message",
		"--name", "personal-chat",
		"--binding", "primary",
		"--triggers", "给张三发消息,联系张三",
		"--default-values", "contact=张三,human_like=true",
	}); err != nil {
		t.Fatalf("runAppPairingsSet: %v", err)
	}

	output := captureStdout(t, func() {
		if err := runAppPairingsList([]string{"--config", configPath, "qq-local"}); err != nil {
			t.Fatalf("runAppPairingsList: %v", err)
		}
	})

	if !strings.Contains(output, "personal-chat") {
		t.Fatalf("expected pairing name in output, got %q", output)
	}
	if !strings.Contains(output, "send-message") {
		t.Fatalf("expected workflow in output, got %q", output)
	}
	if !strings.Contains(output, "binding:  primary") {
		t.Fatalf("expected binding in output, got %q", output)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	original := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = writer
	defer func() {
		os.Stdout = original
	}()

	fn()

	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close: %v", err)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	return string(data)
}
