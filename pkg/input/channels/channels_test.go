package channels

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

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

func TestSlackSendStreamingMessageFallsBackToThreadedFinalPost(t *testing.T) {
	var bodies []string
	adapter := &SlackAdapter{
		config: config.SlackChannelConfig{
			DefaultChannel: "C123",
		},
		client: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				payload, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatalf("read request body: %v", err)
				}
				bodies = append(bodies, string(payload))
				switch len(bodies) {
				case 1:
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(`{"ok":true,"ts":""}`)),
						Header:     make(http.Header),
					}, nil
				case 2:
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(`{"ok":true,"ts":"123.456"}`)),
						Header:     make(http.Header),
					}, nil
				default:
					t.Fatalf("unexpected additional request: %s", req.URL.String())
					return nil, nil
				}
			}),
		},
	}

	err := adapter.sendStreamingMessage(context.Background(), "thread-1", func(onChunk func(chunk string)) error {
		onChunk("hello from stream")
		return nil
	})
	if err != nil {
		t.Fatalf("send streaming message: %v", err)
	}
	if len(bodies) != 2 {
		t.Fatalf("expected 2 postMessage calls, got %d", len(bodies))
	}
	if !strings.Contains(bodies[1], `"thread_ts":"thread-1"`) {
		t.Fatalf("expected fallback post to preserve thread_ts, got %s", bodies[1])
	}
	if !strings.Contains(bodies[1], `"text":"hello from stream"`) {
		t.Fatalf("expected fallback post to include final text, got %s", bodies[1])
	}
}

func TestTelegramSendMessageReturnsAPIErrorWhenOKFalse(t *testing.T) {
	adapter := &TelegramAdapter{
		baseURL: "https://telegram.example/bot-token",
		client: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if !strings.HasSuffix(req.URL.String(), "/sendMessage") {
					t.Fatalf("unexpected request URL: %s", req.URL.String())
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"ok":false,"description":"chat not found"}`)),
					Header:     make(http.Header),
				}, nil
			}),
		},
	}

	err := adapter.sendMessage(context.Background(), "42", "hello")
	if err == nil {
		t.Fatal("expected telegram API error")
	}
	if !strings.Contains(err.Error(), "chat not found") {
		t.Fatalf("expected chat not found error, got %v", err)
	}
}

func TestDiscordPollOnceRepliesToChannelInsteadOfParentMessageID(t *testing.T) {
	var postedURL string
	var postedBody string
	adapter := &DiscordAdapter{
		config: config.DiscordChannelConfig{
			DefaultChannel: "c1",
		},
		apiBaseURL: "https://discord.example/api/v10",
		client: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				switch req.Method {
				case http.MethodGet:
					return &http.Response{
						StatusCode: http.StatusOK,
						Body: io.NopCloser(strings.NewReader(`[
							{
								"id":"m1",
								"channel_id":"c1",
								"content":"hello",
								"guild_id":"g1",
								"author":{"id":"u1","username":"alice","bot":false},
								"message_reference":{"message_id":"parent-123","channel_id":"c1"}
							}
						]`)),
						Header: make(http.Header),
					}, nil
				case http.MethodPost:
					body, err := io.ReadAll(req.Body)
					if err != nil {
						t.Fatalf("read request body: %v", err)
					}
					postedURL = req.URL.String()
					postedBody = string(body)
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(`{"id":"reply-1"}`)),
						Header:     make(http.Header),
					}, nil
				default:
					t.Fatalf("unexpected method: %s", req.Method)
					return nil, nil
				}
			}),
		},
		processed: make(map[string]time.Time),
	}

	err := adapter.pollOnce(context.Background(), func(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error) {
		return "", "reply", nil
	})
	if err != nil {
		t.Fatalf("pollOnce failed: %v", err)
	}
	if !strings.Contains(postedURL, "/channels/c1/messages") {
		t.Fatalf("expected reply to post to channel c1, got %s", postedURL)
	}
	if !strings.Contains(postedBody, `"message_reference":{"message_id":"parent-123"}`) {
		t.Fatalf("expected reply to preserve parent message reference, got %s", postedBody)
	}
}
