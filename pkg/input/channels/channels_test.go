package channels

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/config"
)

func TestStreamWithMessageFallback(t *testing.T) {
	var sent []string
	err := streamWithMessageFallback(func(onChunk func(chunk string)) error {
		onChunk("hello")
		onChunk(" world")
		return nil
	}, func(text string) error {
		sent = append(sent, text)
		return nil
	})
	if err != nil {
		t.Fatalf("expected fallback stream to succeed, got %v", err)
	}
	if len(sent) != 1 || sent[0] != "hello world" {
		t.Fatalf("unexpected sent messages: %v", sent)
	}

	sent = nil
	wantErr := errors.New("boom")
	err = streamWithMessageFallback(func(onChunk func(chunk string)) error {
		onChunk("partial")
		return wantErr
	}, func(text string) error {
		sent = append(sent, text)
		return nil
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected original error, got %v", err)
	}
	if len(sent) != 1 || !strings.Contains(sent[0], "partial") || !strings.Contains(sent[0], "boom") {
		t.Fatalf("unexpected fallback error message: %v", sent)
	}
}

func TestDiscordChannelType(t *testing.T) {
	if got := discordChannelType("guild-1"); got != "guild" {
		t.Fatalf("expected guild channel type, got %q", got)
	}
	if got := discordChannelType("   "); got != "private" {
		t.Fatalf("expected private channel type, got %q", got)
	}
}

func TestDiscordAdapterVerifyInteraction(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	body := []byte(`{"type":1}`)
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	signature := ed25519.Sign(privateKey, append([]byte(timestamp), body...))

	adapter := NewDiscordAdapter(config.DiscordChannelConfig{
		Enabled:   true,
		BotToken:  "token",
		PublicKey: hex.EncodeToString(publicKey),
	}, nil)

	req := httptest.NewRequest("POST", "/interactions", strings.NewReader(string(body)))
	req.Header.Set("X-Signature-Ed25519", hex.EncodeToString(signature))
	req.Header.Set("X-Signature-Timestamp", timestamp)

	if !adapter.VerifyInteraction(req, body) {
		t.Fatal("expected valid discord interaction signature to verify")
	}

	req.Header.Set("X-Signature-Ed25519", strings.Repeat("0", len(signature)*2))
	if adapter.VerifyInteraction(req, body) {
		t.Fatal("expected invalid discord interaction signature to fail verification")
	}

	staleTimestamp := fmt.Sprintf("%d", time.Now().Add(-10*time.Minute).Unix())
	staleSignature := ed25519.Sign(privateKey, append([]byte(staleTimestamp), body...))
	req.Header.Set("X-Signature-Timestamp", staleTimestamp)
	req.Header.Set("X-Signature-Ed25519", hex.EncodeToString(staleSignature))
	if adapter.VerifyInteraction(req, body) {
		t.Fatal("expected stale discord interaction timestamp to fail verification")
	}
}

func TestDiscordAdapterHandleInteraction(t *testing.T) {
	adapter := NewDiscordAdapter(config.DiscordChannelConfig{
		Enabled:        true,
		BotToken:       "token",
		DefaultChannel: "123",
	}, nil)

	body := []byte(`{
		"type": 2,
		"data": {
			"name": "ask",
			"options": [{"name":"message","value":"hello"}]
		},
		"channel_id": "chan-1",
		"guild_id": "guild-1",
		"member": {"user": {"id":"user-1","username":"alice"}}
	}`)

	called := false
	resp, err := adapter.HandleInteraction(context.Background(), body, func(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error) {
		called = true
		if message != "hello" {
			t.Fatalf("expected interaction message hello, got %q", message)
		}
		if meta["channel"] != "discord" || meta["channel_id"] != "chan-1" || meta["guild_id"] != "guild-1" {
			t.Fatalf("unexpected interaction meta: %+v", meta)
		}
		return "session-1", "ok", nil
	})
	if err != nil {
		t.Fatalf("handle interaction returned error: %v", err)
	}
	if !called {
		t.Fatal("expected interaction handler to be called")
	}
	if resp["type"] != 4 {
		t.Fatalf("expected response type 4, got %#v", resp["type"])
	}
}

func TestSlackAdapterHelpers(t *testing.T) {
	adapter := NewSlackAdapter(config.SlackChannelConfig{
		Enabled:        true,
		BotToken:       "token",
		DefaultChannel: "C123",
	}, nil)

	if !adapter.Enabled() {
		t.Fatal("expected slack adapter to be enabled with valid config")
	}
	status := adapter.Status()
	if !status.Enabled || status.Name != "slack" {
		t.Fatalf("unexpected slack adapter status: %+v", status)
	}

	audioURL, audioMIME := adapter.findAudioFile([]struct {
		Mimetype string `json:"mimetype"`
		URL      string `json:"url_private"`
		Title    string `json:"title"`
	}{
		{Mimetype: "image/png", URL: "https://example.com/image"},
		{Mimetype: "audio/mpeg", URL: "https://example.com/audio", Title: "voice"},
	})
	if audioURL != "https://example.com/audio" || audioMIME != "audio/mpeg" {
		t.Fatalf("unexpected audio file detection result: %q %q", audioURL, audioMIME)
	}
}
