package sdk

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/config"
)

func TestNewGatewayAppliesOptions(t *testing.T) {
	httpClient := &http.Client{Timeout: time.Second}
	client, err := NewGateway(WithGatewayAddr("127.0.0.1:19999"), WithHTTPClient(httpClient))
	if err != nil {
		t.Fatalf("NewGateway: %v", err)
	}
	if client.addr != "127.0.0.1:19999" {
		t.Fatalf("addr = %q", client.addr)
	}
	if client.httpClient != httpClient {
		t.Fatal("expected custom http client")
	}
	if err := client.Disconnect(); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}

	client.OnMessage(func(IncomingMessage) {})
	client.OnToolCall(func(ToolCall) {})
	client.OnPresence(func(Presence) {})
	client.OnTyping(func(TypingIndicator) {})
	if client.onMessage == nil || client.onToolCall == nil || client.onPresence == nil || client.onTyping == nil {
		t.Fatal("expected callbacks to be registered")
	}
}

func TestWSClientHelpers(t *testing.T) {
	client := NewWSClient("ws://127.0.0.1:18789/ws", "token")
	if client == nil || client.Connected() {
		t.Fatal("expected disconnected client")
	}
	if err := client.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !client.isClosed() {
		t.Fatal("expected closed client")
	}

	url := GatewayURLFromConfig(&config.Config{})
	if url != "ws://127.0.0.1:18789/ws" {
		t.Fatalf("GatewayURLFromConfig = %q", url)
	}
	if got := mapString(map[string]any{" name ": "ignored", "name": " value "}, "name"); got != "value" {
		t.Fatalf("mapString = %q", got)
	}
	if id1, id2 := uniqueID("req"), uniqueID("req"); id1 == id2 {
		t.Fatal("expected unique ids")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.ensureConnected(ctx); err == nil {
		t.Fatal("expected ensureConnected to fail on closed client")
	}
}
