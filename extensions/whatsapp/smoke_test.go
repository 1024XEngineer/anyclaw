package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWhatsAppHandleWebhookProcessesIncomingMessage(t *testing.T) {
	var sent bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/123/messages" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		sent = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	stdout := &bytes.Buffer{}
	ext := NewWhatsAppExtension(Config{AccessToken: "token", PhoneNumberID: "123"})
	ext.apiBaseURL = server.URL
	ext.client = server.Client()
	ext.stdin = strings.NewReader(`{"text":"pong"}`)
	ext.stdout = stdout

	body := `{"entry":[{"changes":[{"value":{"messages":[{"from":"15551234","id":"wamid","type":"text","text":{"body":"hello"}}]}}]}]}`
	req := httptest.NewRequest(http.MethodPost, "/whatsapp", strings.NewReader(body))
	rec := httptest.NewRecorder()

	ext.handleWebhook(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(stdout.String(), `"channel":"whatsapp"`) {
		t.Fatalf("expected whatsapp event, got %q", stdout.String())
	}
	if !sent {
		t.Fatal("expected reply to be sent")
	}
}

func TestWhatsAppSendTextMessageReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, "upstream failed")
	}))
	defer server.Close()

	ext := NewWhatsAppExtension(Config{AccessToken: "token", PhoneNumberID: "123"})
	ext.apiBaseURL = server.URL
	ext.client = server.Client()

	err := ext.sendTextMessage("15551234", "pong")
	if err == nil {
		t.Fatal("expected send failure")
	}
	if !strings.Contains(err.Error(), "whatsapp send failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}
