package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/anyclaw/anyclaw/pkg/channel"
)

func TestRunChannelsStatusUsesGatewayToken(t *testing.T) {
	const token = "channel-token"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/channels" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+token {
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

	configPath := writeGatewayTestConfig(t, server.URL, token)
	output := captureStdout(t, func() {
		if err := runChannelsCommand([]string{"status", "--config", configPath, "--json"}); err != nil {
			t.Fatalf("runChannelsCommand status: %v", err)
		}
	})

	var payload struct {
		GatewayReachable bool             `json:"gateway_reachable"`
		Count            int              `json:"count"`
		Channels         []channel.Status `json:"channels"`
	}
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("Unmarshal output: %v\noutput=%s", err, output)
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
