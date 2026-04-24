package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/gateway"
	inputlayer "github.com/1024XEngineer/anyclaw/pkg/input"
)

func TestRunAnyClawCLIRoutesStatusHealthSessionsAndApprovalsCommands(t *testing.T) {
	clearModelsCLIEnv(t)

	now := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/status":
			_ = json.NewEncoder(w).Encode(gateway.Status{
				Status:     "running",
				Address:    "127.0.0.1:18789",
				Provider:   "openai",
				Model:      "gpt-4o-mini",
				Sessions:   1,
				Events:     2,
				Tools:      3,
				Skills:     4,
				WorkingDir: "workflows",
			})
		case "/healthz":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		case "/sessions":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id":            "session-1",
					"title":         "Demo Session",
					"agent":         "main",
					"message_count": 2,
					"updated_at":    now.Format(time.RFC3339),
				},
			})
		case "/approvals":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id":           "ap-1",
					"tool_name":    "run_command",
					"action":       "exec",
					"status":       "pending",
					"requested_at": now.Format(time.RFC3339),
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	configPath := writeStatusCLIConfig(t, server.URL, "secret-token")

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "status",
			args: []string{"status", "--config", configPath},
			want: "Gateway is running",
		},
		{
			name: "health",
			args: []string{"health", "--config", configPath},
			want: "Gateway health: ok",
		},
		{
			name: "sessions",
			args: []string{"sessions", "--config", configPath},
			want: "Found 1 session(s)",
		},
		{
			name: "approvals",
			args: []string{"approvals", "--config", configPath},
			want: "Found 1 approval(s)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stdout, _, err := captureCLIOutput(t, func() error {
				return runAnyClawCLI(tc.args)
			})
			if err != nil {
				t.Fatalf("runAnyClawCLI %s: %v", tc.name, err)
			}
			if !strings.Contains(stdout, tc.want) {
				t.Fatalf("expected %q in output, got %q", tc.want, stdout)
			}
		})
	}
}

func TestCLIUsageIncludesStatusCommands(t *testing.T) {
	stdout, _, err := captureCLIOutput(t, func() error {
		return runAnyClawCLI([]string{"help"})
	})
	if err != nil {
		t.Fatalf("runAnyClawCLI help: %v", err)
	}
	for _, want := range []string{
		"anyclaw status [options]",
		"anyclaw health [options]",
		"anyclaw sessions [options]",
		"anyclaw approvals <subcommand>",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected %q in CLI usage, got %q", want, stdout)
		}
	}
}

func TestRunStatusCommandUsesGatewayToken(t *testing.T) {
	clearModelsCLIEnv(t)

	const token = "test-token"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+token {
			t.Fatalf("unexpected auth header: %q", got)
		}
		_ = json.NewEncoder(w).Encode(gateway.Status{
			Status:     "running",
			Address:    "127.0.0.1:18789",
			Provider:   "openai",
			Model:      "gpt-4o-mini",
			Sessions:   2,
			Events:     3,
			Tools:      5,
			Skills:     4,
			WorkingDir: "workflows",
		})
	}))
	defer server.Close()

	configPath := writeStatusCLIConfig(t, server.URL, token)
	stdout, _, err := captureCLIOutput(t, func() error {
		return runStatusCommand([]string{"--config", configPath})
	})
	if err != nil {
		t.Fatalf("runStatusCommand: %v", err)
	}
	if !strings.Contains(stdout, "Gateway is running") || !strings.Contains(stdout, "Provider: openai") {
		t.Fatalf("unexpected status output: %q", stdout)
	}
}

func TestRunStatusCommandAllJSONIncludesChannelsAndRecentSessions(t *testing.T) {
	clearModelsCLIEnv(t)

	now := time.Date(2026, 4, 24, 13, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/status":
			_ = json.NewEncoder(w).Encode(gateway.Status{
				Status:     "running",
				Address:    "127.0.0.1:18789",
				Provider:   "openai",
				Model:      "gpt-4o-mini",
				Sessions:   7,
				Events:     9,
				Tools:      8,
				Skills:     6,
				WorkingDir: "workflows",
			})
		case "/channels":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"name":    "telegram",
					"enabled": true,
					"running": true,
					"healthy": true,
				},
			})
		case "/sessions":
			var sessions []map[string]any
			for i := 0; i < 6; i++ {
				sessions = append(sessions, map[string]any{
					"id":            "session-" + string(rune('A'+i)),
					"title":         "Session " + string(rune('A'+i)),
					"agent":         "main",
					"message_count": i + 1,
					"updated_at":    now.Add(-time.Duration(i) * time.Minute).Format(time.RFC3339),
				})
			}
			_ = json.NewEncoder(w).Encode(sessions)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	configPath := writeStatusCLIConfig(t, server.URL, "secret-token")
	stdout, _, err := captureCLIOutput(t, func() error {
		return runStatusCommand([]string{"--config", configPath, "--all", "--json"})
	})
	if err != nil {
		t.Fatalf("runStatusCommand --all --json: %v", err)
	}

	var payload struct {
		Status   gateway.Status      `json:"status"`
		Channels []inputlayer.Status `json:"channels"`
		Sessions []sessionListItem   `json:"sessions"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("Unmarshal output: %v\noutput=%s", err, stdout)
	}
	if payload.Status.Status != "running" || len(payload.Channels) != 1 {
		t.Fatalf("unexpected status payload: %#v", payload)
	}
	if len(payload.Sessions) != 5 {
		t.Fatalf("expected recent sessions to be trimmed to 5, got %#v", payload.Sessions)
	}
	if payload.Sessions[0].ID != "session-A" {
		t.Fatalf("expected most recent session first, got %#v", payload.Sessions)
	}
}

func TestRunHealthCommandVerboseJSON(t *testing.T) {
	clearModelsCLIEnv(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		case "/status":
			_ = json.NewEncoder(w).Encode(gateway.Status{
				Status:     "running",
				Address:    "127.0.0.1:18789",
				Provider:   "openai",
				Model:      "gpt-4o-mini",
				WorkingDir: "workflows",
			})
		case "/channels":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"name": "telegram", "enabled": true, "healthy": true},
				{"name": "discord", "enabled": true, "healthy": false},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	configPath := writeStatusCLIConfig(t, server.URL, "secret-token")
	stdout, _, err := captureCLIOutput(t, func() error {
		return runHealthCommand([]string{"--config", configPath, "--verbose", "--json"})
	})
	if err != nil {
		t.Fatalf("runHealthCommand: %v", err)
	}

	var payload struct {
		Health   map[string]any      `json:"health"`
		Status   gateway.Status      `json:"status"`
		Channels []inputlayer.Status `json:"channels"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("Unmarshal output: %v\noutput=%s", err, stdout)
	}
	if ok, _ := payload.Health["ok"].(bool); !ok {
		t.Fatalf("expected health.ok=true, got %#v", payload.Health)
	}
	if payload.Status.Provider != "openai" || len(payload.Channels) != 2 {
		t.Fatalf("unexpected verbose health payload: %#v", payload)
	}
}

func TestRunSessionsCommandFiltersActiveSessions(t *testing.T) {
	clearModelsCLIEnv(t)

	now := time.Now().UTC()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sessions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"id":             "sess-recent",
				"title":          "Recent Session",
				"agent":          "main",
				"message_count":  3,
				"updated_at":     now.Format(time.RFC3339),
				"last_active_at": now.Format(time.RFC3339),
			},
			{
				"id":             "sess-old",
				"title":          "Old Session",
				"agent":          "main",
				"message_count":  1,
				"updated_at":     now.Add(-2 * time.Hour).Format(time.RFC3339),
				"last_active_at": now.Add(-2 * time.Hour).Format(time.RFC3339),
			},
		})
	}))
	defer server.Close()

	configPath := writeStatusCLIConfig(t, server.URL, "")
	stdout, _, err := captureCLIOutput(t, func() error {
		return runSessionsCommand([]string{"--config", configPath, "--active", "30", "--json"})
	})
	if err != nil {
		t.Fatalf("runSessionsCommand: %v", err)
	}

	var payload struct {
		Count    int               `json:"count"`
		Sessions []sessionListItem `json:"sessions"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("Unmarshal output: %v\noutput=%s", err, stdout)
	}
	if payload.Count != 1 {
		t.Fatalf("expected 1 active session, got %d", payload.Count)
	}
	if len(payload.Sessions) != 1 || payload.Sessions[0].ID != "sess-recent" {
		t.Fatalf("unexpected sessions payload: %#v", payload.Sessions)
	}
}

func TestRunSessionsCommandSupportsWorkspaceFilter(t *testing.T) {
	clearModelsCLIEnv(t)

	const workspace = "demo team&A"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sessions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("workspace"); got != workspace {
			t.Fatalf("unexpected workspace query: %q", got)
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"id":            "sess-1",
				"title":         "Demo Session",
				"agent":         "main",
				"workspace":     workspace,
				"message_count": 2,
				"updated_at":    "2026-04-24T10:00:00Z",
			},
		})
	}))
	defer server.Close()

	configPath := writeStatusCLIConfig(t, server.URL, "secret-token")
	stdout, _, err := captureCLIOutput(t, func() error {
		return runSessionsCommand([]string{"--config", configPath, "--workspace", workspace})
	})
	if err != nil {
		t.Fatalf("runSessionsCommand workspace: %v", err)
	}
	if !strings.Contains(stdout, "workspace="+workspace) {
		t.Fatalf("expected workspace in output, got %q", stdout)
	}
}

func TestRunApprovalsCommandDefaultsToGetJSON(t *testing.T) {
	clearModelsCLIEnv(t)

	const approvalStatus = "pending review&ops"
	now := time.Date(2026, 4, 24, 14, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/approvals" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("status"); got != approvalStatus {
			t.Fatalf("unexpected status query: %q", got)
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"id":           "ap-1",
				"tool_name":    "run_command",
				"action":       "exec",
				"status":       approvalStatus,
				"requested_at": now.Format(time.RFC3339),
			},
		})
	}))
	defer server.Close()

	configPath := writeStatusCLIConfig(t, server.URL, "approval-token")
	stdout, _, err := captureCLIOutput(t, func() error {
		return runApprovalsCommand([]string{"--config", configPath, "--json", "--status", approvalStatus})
	})
	if err != nil {
		t.Fatalf("runApprovalsCommand default get: %v", err)
	}

	var payload struct {
		Count     int                `json:"count"`
		Approvals []approvalListItem `json:"approvals"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("Unmarshal output: %v\noutput=%s", err, stdout)
	}
	if payload.Count != 1 || len(payload.Approvals) != 1 || payload.Approvals[0].ID != "ap-1" {
		t.Fatalf("unexpected approvals payload: %#v", payload)
	}
}

func TestRunApprovalsCommandUnknownSubcommandPrintsUsage(t *testing.T) {
	clearModelsCLIEnv(t)

	stdout, _, err := captureCLIOutput(t, func() error {
		return runApprovalsCommand([]string{"unknown"})
	})
	if err == nil || !strings.Contains(err.Error(), "unknown approvals command") {
		t.Fatalf("expected unknown approvals command error, got %v", err)
	}
	if !strings.Contains(stdout, "AnyClaw approvals commands:") {
		t.Fatalf("expected approvals usage output, got %q", stdout)
	}
}

func TestRunApprovalsApprovePostsResolution(t *testing.T) {
	clearModelsCLIEnv(t)

	const token = "approval-token"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/approvals/ap-1/resolve" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+token {
			t.Fatalf("unexpected auth header: %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("Decode body: %v", err)
		}
		if approved, _ := body["approved"].(bool); !approved {
			t.Fatalf("expected approved=true, got %#v", body)
		}
		if comment, _ := body["comment"].(string); comment != "ship it" {
			t.Fatalf("expected comment to round-trip, got %#v", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":           "ap-1",
			"tool_name":    "run_command",
			"status":       "approved",
			"requested_at": time.Now().UTC().Format(time.RFC3339),
		})
	}))
	defer server.Close()

	configPath := writeStatusCLIConfig(t, server.URL, token)
	stdout, _, err := captureCLIOutput(t, func() error {
		return runApprovalsCommand([]string{"approve", "--config", configPath, "--comment", "ship it", "ap-1"})
	})
	if err != nil {
		t.Fatalf("runApprovalsCommand approve: %v", err)
	}
	if !strings.Contains(stdout, "Approved: ap-1") {
		t.Fatalf("expected approval success output, got %q", stdout)
	}
}

func TestRunApprovalsRejectRequiresID(t *testing.T) {
	clearModelsCLIEnv(t)

	err := runApprovalsCommand([]string{"reject"})
	if err == nil || !strings.Contains(err.Error(), "approval id is required") {
		t.Fatalf("expected approval id validation error, got %v", err)
	}
}

func writeStatusCLIConfig(t *testing.T, serverURL string, token string) string {
	t.Helper()

	cfg := gatewayTestConfigFromURL(t, serverURL)
	cfg.Security.APIToken = token

	configPath := filepath.Join(t.TempDir(), "anyclaw.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("save config: %v", err)
	}
	return configPath
}
