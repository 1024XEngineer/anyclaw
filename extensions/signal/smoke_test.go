package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSignalPollOnceProcessesIncomingMessage(t *testing.T) {
	var sendBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/receive/"):
			_, _ = io.WriteString(w, `[{"envelope":{"sourceNumber":"+10001","timestamp":42,"dataMessage":{"message":"hello"}}}]`)
		case r.Method == http.MethodPost && r.URL.Path == "/v2/send":
			sendBody, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusCreated)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	stdout := &bytes.Buffer{}
	ext := NewSignalExtension(Config{SignalURL: server.URL, Number: "+19999"})
	ext.client = server.Client()
	ext.stdin = strings.NewReader(`{"text":"pong"}`)
	ext.stdout = stdout

	if err := ext.pollOnce(context.Background()); err != nil {
		t.Fatalf("pollOnce: %v", err)
	}

	if !strings.Contains(stdout.String(), `"channel":"signal"`) {
		t.Fatalf("expected signal event, got %q", stdout.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(sendBody, &payload); err != nil {
		t.Fatalf("Unmarshal send payload: %v", err)
	}
	if payload["message"] != "pong" {
		t.Fatalf("unexpected reply payload: %#v", payload)
	}
}

func TestSignalPollOnceReturnsSendError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/receive/"):
			_, _ = io.WriteString(w, `[{"envelope":{"sourceNumber":"+10001","timestamp":42,"dataMessage":{"message":"hello"}}}]`)
		case r.Method == http.MethodPost && r.URL.Path == "/v2/send":
			http.Error(w, "boom", http.StatusBadGateway)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	ext := NewSignalExtension(Config{SignalURL: server.URL, Number: "+19999"})
	ext.client = server.Client()
	ext.stdin = strings.NewReader(`{"text":"pong"}`)
	ext.stdout = &bytes.Buffer{}

	err := ext.pollOnce(context.Background())
	if err == nil {
		t.Fatal("expected send failure")
	}
	if !strings.Contains(err.Error(), "signal send failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}
