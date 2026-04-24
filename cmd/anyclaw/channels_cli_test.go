package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/config"
	inputlayer "github.com/1024XEngineer/anyclaw/pkg/input"
)

func TestRunAnyClawCLIRoutesChannelsCommand(t *testing.T) {
	clearModelsCLIEnv(t)

	configPath := writeDefaultCLIConfig(t)
	stdout, _, err := captureCLIOutput(t, func() error {
		return runAnyClawCLI([]string{"channels", "list", "--config", configPath, "--json"})
	})
	if err != nil {
		t.Fatalf("runAnyClawCLI channels: %v", err)
	}

	var payload struct {
		Count    int                 `json:"count"`
		Channels []inputlayer.Status `json:"channels"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("Unmarshal output: %v\noutput=%s", err, stdout)
	}
	if payload.Count != 5 || len(payload.Channels) != 5 {
		t.Fatalf("unexpected channels payload: %#v", payload)
	}
}

func TestCLIUsageIncludesChannelsCommand(t *testing.T) {
	stdout, _, err := captureCLIOutput(t, func() error {
		return runAnyClawCLI([]string{"help"})
	})
	if err != nil {
		t.Fatalf("runAnyClawCLI help: %v", err)
	}
	if !strings.Contains(stdout, "anyclaw channels <subcommand>") {
		t.Fatalf("expected channels help entry, got %q", stdout)
	}
}

func TestRunChannelsCommandUnknownSubcommandPrintsUsage(t *testing.T) {
	clearModelsCLIEnv(t)

	stdout, _, err := captureCLIOutput(t, func() error {
		return runChannelsCommand([]string{"unknown"})
	})
	if err == nil || !strings.Contains(err.Error(), "unknown channels command") {
		t.Fatalf("expected unknown channels command error, got %v", err)
	}
	if !strings.Contains(stdout, "AnyClaw channels commands:") {
		t.Fatalf("expected channels usage output, got %q", stdout)
	}
}

func TestRunChannelsListFallsBackToConfiguredChannels(t *testing.T) {
	clearModelsCLIEnv(t)

	cfg := config.DefaultConfig()
	cfg.Channels.Telegram.Enabled = true
	cfg.Channels.Signal.Enabled = true
	cfg.Gateway.Host = "127.0.0.1"
	cfg.Gateway.Port = 1

	configPath := filepath.Join(t.TempDir(), "anyclaw.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("save config: %v", err)
	}

	stdout, _, err := captureCLIOutput(t, func() error {
		return runChannelsList([]string{"--config", configPath})
	})
	if err != nil {
		t.Fatalf("runChannelsList: %v", err)
	}
	for _, want := range []string{
		"Gateway not reachable at http://127.0.0.1:1; showing configured channels only",
		"Found 5 channel(s)",
		"telegram: enabled",
		"signal: enabled",
		"discord: disabled",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected %q in output, got %q", want, stdout)
		}
	}
}

func TestRunChannelsStatusUsesGatewayToken(t *testing.T) {
	clearModelsCLIEnv(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/channels" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret-token" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"name":          "telegram",
				"enabled":       true,
				"running":       true,
				"healthy":       true,
				"last_activity": time.Now().UTC().Format(time.RFC3339),
			},
		})
	}))
	defer server.Close()

	configPath := writeGatewayServerConfig(t, server.URL)
	stdout, _, err := captureCLIOutput(t, func() error {
		return runChannelsCommand([]string{"status", "--config", configPath, "--json"})
	})
	if err != nil {
		t.Fatalf("runChannelsCommand status: %v", err)
	}

	var payload struct {
		GatewayReachable bool                `json:"gateway_reachable"`
		Count            int                 `json:"count"`
		Channels         []inputlayer.Status `json:"channels"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("Unmarshal output: %v\noutput=%s", err, stdout)
	}
	if !payload.GatewayReachable || payload.Count != 5 {
		t.Fatalf("unexpected payload header: %#v", payload)
	}
	foundTelegram := false
	for _, item := range payload.Channels {
		if item.Name == "telegram" && item.Healthy && item.Running {
			foundTelegram = true
			break
		}
	}
	if !foundTelegram {
		t.Fatalf("expected telegram channel in payload: %#v", payload.Channels)
	}
}

func TestCollectChannelStatusesRequiresReachableGatewayWhenRequested(t *testing.T) {
	clearModelsCLIEnv(t)

	cfg := config.DefaultConfig()
	cfg.Gateway.Host = "127.0.0.1"
	cfg.Gateway.Port = 1

	configPath := filepath.Join(t.TempDir(), "anyclaw.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("save config: %v", err)
	}

	_, _, reachable, err := collectChannelStatuses(configPath, true)
	if err == nil {
		t.Fatal("expected collectChannelStatuses to require a reachable gateway")
	}
	if reachable {
		t.Fatalf("expected gateway to be unreachable, got reachable=%v", reachable)
	}
}

func TestConfiguredChannelStatusesIncludesPluginChannels(t *testing.T) {
	clearModelsCLIEnv(t)

	pluginsDir := filepath.Join(t.TempDir(), "plugins")
	pluginDir := filepath.Join(pluginsDir, "matrix-channel")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("mkdir plugin dir: %v", err)
	}
	manifest := `{
  "name": "matrix-channel",
  "entrypoint": "run.py",
  "enabled": true,
  "channel": {
    "name": "matrix",
    "description": "Matrix channel"
  }
}`
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write plugin manifest: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Channels.Discord.Enabled = true
	cfg.Plugins.Dir = pluginsDir
	cfg.Plugins.AllowExec = true
	cfg.Plugins.RequireTrust = false

	items := configuredChannelStatuses(cfg)
	if len(items) != 6 {
		t.Fatalf("expected 6 channels including plugin channel, got %#v", items)
	}
	foundMatrix := false
	for _, item := range items {
		if item.Name == "matrix-channel" && item.Enabled {
			foundMatrix = true
			break
		}
	}
	if !foundMatrix {
		t.Fatalf("expected plugin-provided manifest name in status output, got %#v", items)
	}
}

func TestCollectChannelStatusesMergesPluginStatusByManifestName(t *testing.T) {
	clearModelsCLIEnv(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/channels" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"name":    "matrix-channel",
				"enabled": true,
				"running": true,
				"healthy": true,
			},
		})
	}))
	defer server.Close()

	pluginsDir := filepath.Join(t.TempDir(), "plugins")
	pluginDir := filepath.Join(pluginsDir, "matrix-channel")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("mkdir plugin dir: %v", err)
	}
	manifest := `{
  "name": "matrix-channel",
  "entrypoint": "run.py",
  "enabled": true,
  "channel": {
    "name": "matrix",
    "description": "Matrix channel"
  }
}`
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write plugin manifest: %v", err)
	}

	cfg := gatewayTestConfigFromURL(t, server.URL)
	cfg.Plugins.Dir = pluginsDir
	cfg.Plugins.AllowExec = true
	cfg.Plugins.RequireTrust = false

	configPath := filepath.Join(t.TempDir(), "anyclaw.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("save config: %v", err)
	}

	_, items, reachable, err := collectChannelStatuses(configPath, true)
	if err != nil {
		t.Fatalf("collectChannelStatuses: %v", err)
	}
	if !reachable {
		t.Fatal("expected gateway to be reachable")
	}
	if len(items) != 6 {
		t.Fatalf("expected merged plugin status to keep 6 items, got %#v", items)
	}

	foundMatrix := false
	for _, item := range items {
		if item.Name == "matrix-channel" && item.Enabled && item.Running && item.Healthy {
			foundMatrix = true
			break
		}
	}
	if !foundMatrix {
		t.Fatalf("expected merged matrix-channel status, got %#v", items)
	}
}

func TestConfiguredChannelStatusesMarksNonRunnablePluginChannelsUnavailable(t *testing.T) {
	clearModelsCLIEnv(t)

	pluginsDir := filepath.Join(t.TempDir(), "plugins")
	pluginDir := filepath.Join(pluginsDir, "matrix-channel")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("mkdir plugin dir: %v", err)
	}
	manifest := `{
  "name": "matrix-channel",
  "entrypoint": "run.py",
  "enabled": true,
  "channel": {
    "name": "matrix",
    "description": "Matrix channel"
  }
}`
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write plugin manifest: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Plugins.Dir = pluginsDir
	cfg.Plugins.AllowExec = false
	cfg.Plugins.RequireTrust = false

	items := configuredChannelStatuses(cfg)

	foundMatrix := false
	for _, item := range items {
		if item.Name == "matrix-channel" {
			foundMatrix = true
			if item.Enabled {
				t.Fatalf("expected non-runnable plugin channel to be unavailable, got %#v", item)
			}
			if !strings.Contains(item.LastError, "plugin execution policy") {
				t.Fatalf("expected unavailability note in plugin channel status, got %#v", item)
			}
			break
		}
	}
	if !foundMatrix {
		t.Fatalf("expected matrix-channel to be listed as unavailable, got %#v", items)
	}
}

func TestMergeChannelStatusesPreservesConfiguredEnabledState(t *testing.T) {
	local := []inputlayer.Status{
		{Name: "telegram", Enabled: true},
		{Name: "discord", Enabled: false},
	}
	remote := []inputlayer.Status{
		{Name: "telegram", Running: true, Healthy: true},
		{Name: "custom", Enabled: true, Running: true},
	}

	items := mergeChannelStatuses(local, remote)
	if len(items) != 3 {
		t.Fatalf("expected 3 merged items, got %#v", items)
	}

	statuses := map[string]inputlayer.Status{}
	for _, item := range items {
		statuses[item.Name] = item
	}
	if !statuses["telegram"].Enabled || !statuses["telegram"].Running || !statuses["telegram"].Healthy {
		t.Fatalf("expected telegram status to merge local enabled with remote health, got %#v", statuses["telegram"])
	}
	if !statuses["custom"].Enabled || !statuses["custom"].Running {
		t.Fatalf("expected custom remote channel to be preserved, got %#v", statuses["custom"])
	}
}

func TestPrintChannelStatusLinesIncludesActivityAndErrors(t *testing.T) {
	now := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)
	items := []inputlayer.Status{
		{Name: "telegram", Enabled: true, Running: true, Healthy: true, LastActivity: now},
		{Name: "discord", LastError: "gateway offline"},
	}

	stdout, _, err := captureCLIOutput(t, func() error {
		printChannelStatusLines(items, channelStatusPrintOptions{
			DisabledLabel:       "disabled",
			IncludeLastActivity: true,
			LastActivityLabel:   "last_activity=",
			ErrorLabel:          "error=",
		})
		return nil
	})
	if err != nil {
		t.Fatalf("captureCLIOutput: %v", err)
	}
	for _, want := range []string{
		"telegram: healthy",
		"last_activity=2026-04-24T12:00:00Z",
		"discord: disabled",
		"error=gateway offline",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected %q in output, got %q", want, stdout)
		}
	}
}
