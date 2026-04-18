package channels

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/1024XEngineer/anyclaw/pkg/config"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestDiscordPolledMessageDecodesMessageReference(t *testing.T) {
	var msg discordPolledMessage
	raw := []byte(`{
		"id":"m1",
		"content":"hello",
		"guild_id":"g1",
		"author":{"id":"u1","username":"alice"},
		"message_reference":{"message_id":"parent-123","channel_id":"c1"}
	}`)

	if err := json.Unmarshal(raw, &msg); err != nil {
		t.Fatalf("unmarshal discord message: %v", err)
	}
	if msg.MessageReference.MessageID != "parent-123" {
		t.Fatalf("expected parent message id to decode, got %q", msg.MessageReference.MessageID)
	}
}

func TestSignalFindAudioAttachmentMatchesByMIMEWithoutURL(t *testing.T) {
	adapter := &SignalAdapter{}
	attachments := []struct {
		ContentType string `json:"contentType"`
		Filename    string `json:"filename"`
	}{
		{ContentType: "audio/ogg", Filename: ""},
	}

	audioURL, audioMIME, hasAudio := adapter.findAudioAttachment(attachments)
	if !hasAudio {
		t.Fatal("expected audio attachment to be detected")
	}
	if audioMIME != "audio/ogg" {
		t.Fatalf("expected audio MIME to be preserved, got %q", audioMIME)
	}
	if audioURL != "" {
		t.Fatalf("expected empty audio URL when attachment has no filename, got %q", audioURL)
	}
}

func TestSlackPollOnceReturnsAPIErrorWhenOKFalse(t *testing.T) {
	adapter := &SlackAdapter{
		config: config.SlackChannelConfig{
			DefaultChannel: "C123",
		},
		client: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if !strings.Contains(req.URL.String(), "conversations.history") {
					t.Fatalf("unexpected request URL: %s", req.URL.String())
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"ok":false,"error":"invalid_auth"}`)),
					Header:     make(http.Header),
				}, nil
			}),
		},
	}

	err := adapter.pollOnce(context.Background(), func(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error) {
		t.Fatal("handler should not be called on Slack API failure")
		return "", "", nil
	})
	if err == nil {
		t.Fatal("expected slack API error")
	}
	if !strings.Contains(err.Error(), "invalid_auth") {
		t.Fatalf("expected invalid_auth error, got %v", err)
	}
}
