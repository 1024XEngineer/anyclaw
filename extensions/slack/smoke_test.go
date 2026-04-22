package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSlackHandleWebhookRespondsToVerification(t *testing.T) {
	ext := NewSlackExtension(Config{})

	req := httptest.NewRequest(http.MethodPost, "/slack/events", strings.NewReader(`{"type":"url_verification","challenge":"abc123"}`))
	rec := httptest.NewRecorder()

	ext.handleWebhook(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if strings.TrimSpace(rec.Body.String()) != "abc123" {
		t.Fatalf("unexpected verification response: %q", rec.Body.String())
	}
}

func TestSlackSendMessageReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat.postMessage" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"ok":false,"error":"channel_not_found"}`)
	}))
	defer server.Close()

	ext := NewSlackExtension(Config{BotToken: "token"})
	ext.baseURL = server.URL
	ext.client = server.Client()

	err := ext.sendMessage("C123", "pong")
	if err == nil {
		t.Fatal("expected send failure")
	}
	if !strings.Contains(err.Error(), "channel_not_found") {
		t.Fatalf("unexpected error: %v", err)
	}
}
