package main

import (
	"bufio"
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestIRCHandleLineProcessesIncomingMessage(t *testing.T) {
	stdout := &bytes.Buffer{}
	raw := &bytes.Buffer{}

	ext := NewIRCExtension(Config{Nick: "bot", Channels: []string{"#room"}})
	ext.stdin = strings.NewReader(`{"text":"pong"}`)
	ext.stdout = stdout
	ext.writer = bufio.NewWriter(raw)

	if err := ext.handleLine(context.Background(), ":alice!u@h PRIVMSG #room :hello"); err != nil {
		t.Fatalf("handleLine: %v", err)
	}
	if !strings.Contains(stdout.String(), `"channel":"irc"`) {
		t.Fatalf("expected irc event, got %q", stdout.String())
	}
	if !strings.Contains(raw.String(), "PRIVMSG #room :pong") {
		t.Fatalf("expected reply to be written, got %q", raw.String())
	}
}

func TestIRCRunRequiresChannels(t *testing.T) {
	ext := NewIRCExtension(Config{Nick: "bot"})
	err := ext.Run(context.Background())
	if err == nil {
		t.Fatal("expected missing channels to fail")
	}
	if !strings.Contains(err.Error(), "channels are required") {
		t.Fatalf("unexpected error: %v", err)
	}
}
