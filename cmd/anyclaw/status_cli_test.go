package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/anyclaw/anyclaw/pkg/config"
)

func TestRunStatusCommandUsesGatewayToken(t *testing.T) {
	const token = "test-token"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+token {
			t.Fatalf("unexpected auth header: %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"status":      "running",
			"version":     "2026.3.13",
			"provider":    "openai",
			"model":       "gpt-4o-mini",
			"address":     "127.0.0.1:18789",
			"working_dir": "workflows",
			"work_dir":    ".anyclaw",
			"sessions":    2,
			"events":      3,
			"skills":      4,
			"tools":       5,
		})
	}))
	defer server.Close()

	configPath := writeGatewayTestConfig(t, server.URL, token)
	output := captureStdout(t, func() {
		if err := runStatusCommand([]string{"--config", configPath}); err != nil {
			t.Fatalf("runStatusCommand: %v", err)
		}
	})

	if !strings.Contains(output, "Gateway is running") {
		t.Fatalf("expected running output, got %q", output)
	}
	if !strings.Contains(output, "Provider: openai") {
		t.Fatalf("expected provider in output, got %q", output)
	}
}

func TestRunSessionsCommandFiltersActiveSessions(t *testing.T) {
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

	configPath := writeGatewayTestConfig(t, server.URL, "")
	output := captureStdout(t, func() {
		if err := runSessionsCommand([]string{"--config", configPath, "--active", "30", "--json"}); err != nil {
			t.Fatalf("runSessionsCommand: %v", err)
		}
	})

	var payload struct {
		Count    int               `json:"count"`
		Sessions []sessionListItem `json:"sessions"`
	}
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("Unmarshal output: %v\noutput=%s", err, output)
	}
	if payload.Count != 1 {
		t.Fatalf("expected 1 active session, got %d", payload.Count)
	}
	if len(payload.Sessions) != 1 || payload.Sessions[0].ID != "sess-recent" {
		t.Fatalf("unexpected sessions payload: %#v", payload.Sessions)
	}
}

func TestRunApprovalsApprovePostsResolution(t *testing.T) {
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
			"id":          "ap-1",
			"tool_name":   "run_command",
			"status":      "approved",
			"requested_at": time.Now().UTC().Format(time.RFC3339),
		})
	}))
	defer server.Close()

	configPath := writeGatewayTestConfig(t, server.URL, token)
	if err := runApprovalsCommand([]string{"approve", "--config", configPath, "--comment", "ship it", "ap-1"}); err != nil {
		t.Fatalf("runApprovalsCommand: %v", err)
	}
}

func writeGatewayTestConfig(t *testing.T, serverURL string, token string) string {
	t.Helper()

	parsed, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatalf("Atoi: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Gateway.Host = parsed.Hostname()
	cfg.Gateway.Port = port
	cfg.Security.APIToken = token

	configPath := filepath.Join(t.TempDir(), "anyclaw.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save config: %v", err)
	}
	return configPath
}
