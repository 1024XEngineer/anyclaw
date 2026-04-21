package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDiscordPollOnceProcessesIncomingMessage(t *testing.T) {
	var sent bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/channels/chan/messages":
			_, _ = io.WriteString(w, `[{"id":"1","content":"hello","author":{"username":"alice","bot":false}}]`)
		case r.Method == http.MethodPost && r.URL.Path == "/channels/chan/messages":
			sent = true
			w.WriteHeader(http.StatusCreated)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	stdout := &bytes.Buffer{}
	ext := NewDiscordExtension(Config{BotToken: "token", APIBaseURL: server.URL, ChannelIDs: []string{"chan"}})
	ext.client = server.Client()
	ext.stdin = strings.NewReader(`{"text":"pong"}`)
	ext.stdout = stdout

	if err := ext.pollOnce(context.Background()); err != nil {
		t.Fatalf("pollOnce: %v", err)
	}
	if !strings.Contains(stdout.String(), `"channel":"discord"`) {
		t.Fatalf("expected discord event, got %q", stdout.String())
	}
	if !sent {
		t.Fatal("expected reply to be sent")
	}
}

func TestDiscordRunRequiresChannelIDs(t *testing.T) {
	ext := NewDiscordExtension(Config{BotToken: "token"})
	err := ext.Run(context.Background())
	if err == nil {
		t.Fatal("expected missing channel_ids to fail")
	}
	if !strings.Contains(err.Error(), "channel_ids required") {
		t.Fatalf("unexpected error: %v", err)
	}
}
