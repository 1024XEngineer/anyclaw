package channels

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/config"
)

func TestBuildAdaptersIncludesTelegramAndSignal(t *testing.T) {
	adapters := BuildAdapters(config.ChannelsConfig{
		Telegram: config.TelegramChannelConfig{
			Enabled:  true,
			BotToken: "telegram-token",
		},
		Slack: config.SlackChannelConfig{
			Enabled:        true,
			BotToken:       "slack-token",
			AppToken:       "app-token",
			DefaultChannel: "C123",
		},
		Discord: config.DiscordChannelConfig{
			Enabled:        true,
			BotToken:       "discord-token",
			DefaultChannel: "123",
		},
		Signal: config.SignalChannelConfig{
			Enabled: true,
			BaseURL: "https://signal.example",
			Number:  "+1000",
		},
	}, nil)

	if len(adapters) != 4 {
		t.Fatalf("expected 4 adapters, got %d", len(adapters))
	}

	gotNames := make([]string, 0, len(adapters))
	for _, adapter := range adapters {
		gotNames = append(gotNames, adapter.Name())
	}

	wantNames := []string{"telegram", "slack", "discord", "signal"}
	for i, want := range wantNames {
		if gotNames[i] != want {
			t.Fatalf("expected adapter %d to be %q, got %q", i, want, gotNames[i])
		}
	}
}

func TestNewManagerBuildsStatusesForChannelAdapters(t *testing.T) {
	manager := NewManager(config.ChannelsConfig{
		Telegram: config.TelegramChannelConfig{
			Enabled:  true,
			BotToken: "telegram-token",
		},
		Slack: config.SlackChannelConfig{
			Enabled:        true,
			BotToken:       "slack-token",
			AppToken:       "app-token",
			DefaultChannel: "C123",
		},
		Discord: config.DiscordChannelConfig{
			Enabled:        true,
			BotToken:       "discord-token",
			DefaultChannel: "123",
		},
		Signal: config.SignalChannelConfig{
			Enabled: true,
			BaseURL: "https://signal.example",
			Number:  "+1000",
		},
	}, nil)

	if manager == nil {
		t.Fatal("expected channel manager to be created")
	}

	statuses := manager.Statuses()
	if len(statuses) != 4 {
		t.Fatalf("expected 4 adapter statuses, got %d", len(statuses))
	}

	wantNames := []string{"telegram", "slack", "discord", "signal"}
	for i, want := range wantNames {
		if statuses[i].Name != want {
			t.Fatalf("expected status %d to be %q, got %+v", i, want, statuses[i])
		}
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

func TestSignalAdapterSendMessageUsesDefaultRecipientAndBearerToken(t *testing.T) {
	var (
		gotAuth string
		gotBody string
		calls   int
	)

	adapter := NewSignalAdapter(config.SignalChannelConfig{
		Enabled:          true,
		BaseURL:          "https://signal.example/",
		Number:           "+1000",
		DefaultRecipient: "+2000",
		BearerToken:      "secret",
	}, nil)

	if adapter.Name() != "signal" {
		t.Fatalf("expected signal adapter name, got %q", adapter.Name())
	}
	if !adapter.Enabled() {
		t.Fatal("expected signal adapter to be enabled")
	}
	status := adapter.Status()
	if !status.Enabled || status.Name != "signal" {
		t.Fatalf("unexpected signal adapter status: %+v", status)
	}
	if signalChannelType("group-1") != "group" || signalChannelType("") != "private" {
		t.Fatal("expected signal channel type helper to distinguish group/private chats")
	}

	adapter.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			calls++
			gotAuth = req.Header.Get("Authorization")
			body, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("read signal send body: %v", err)
			}
			gotBody = string(body)
			return jsonResponse(http.StatusOK, map[string]any{"timestamp": 1}), nil
		}),
	}

	if err := adapter.sendMessage(context.Background(), "", "hello"); err != nil {
		t.Fatalf("sendMessage returned error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected one outbound signal send, got %d", calls)
	}
	if gotAuth != "Bearer secret" {
		t.Fatalf("expected bearer token header, got %q", gotAuth)
	}
	if !strings.Contains(gotBody, `"+2000"`) || !strings.Contains(gotBody, `"+1000"`) || !strings.Contains(gotBody, `"hello"`) {
		t.Fatalf("unexpected signal send payload: %s", gotBody)
	}

	adapter.config.DefaultRecipient = ""
	if err := adapter.sendMessage(context.Background(), "", "ignored"); err != nil {
		t.Fatalf("expected empty recipient branch to return nil, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected empty recipient branch to skip HTTP call, got %d calls", calls)
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
				return jsonResponse(http.StatusOK, map[string]any{"ok": false, "description": "chat not found"}), nil
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

func TestTelegramAdapterLifecycleAndHelperMethods(t *testing.T) {
	var (
		cancelRun       context.CancelFunc
		cancelRunStream context.CancelFunc
		editBody        string
		deleteCalls     int32
		sendCalls       int32
	)

	adapter := NewTelegramAdapter(config.TelegramChannelConfig{
		Enabled:   true,
		BotToken:  "token",
		PollEvery: 1,
	}, nil)

	if adapter.Name() != "telegram" {
		t.Fatalf("expected telegram adapter name, got %q", adapter.Name())
	}
	if !adapter.Enabled() {
		t.Fatal("expected telegram adapter to be enabled")
	}
	status := adapter.Status()
	if !status.Enabled || status.Name != "telegram" {
		t.Fatalf("unexpected telegram adapter status: %+v", status)
	}
	if got := telegramFileRef(" file-1 "); got != "telegram-file:file-1" {
		t.Fatalf("expected telegram file ref helper to trim file id, got %q", got)
	}
	if got := telegramFileRef("   "); got != "" {
		t.Fatalf("expected empty telegram file ref for blank id, got %q", got)
	}
	if channelType, isGroup := telegramConversationMetadata("private", 42, 42); channelType != "private" || isGroup {
		t.Fatalf("expected private telegram conversation metadata, got %q %v", channelType, isGroup)
	}
	if channelType, isGroup := telegramConversationMetadata("supergroup", -100, 42); channelType != "supergroup" || !isGroup {
		t.Fatalf("expected group telegram conversation metadata, got %q %v", channelType, isGroup)
	}

	adapter.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch {
			case strings.Contains(req.URL.String(), "/getUpdates"):
				return jsonResponse(http.StatusOK, map[string]any{
					"ok": true,
					"result": []map[string]any{
						{
							"update_id": 12,
							"message": map[string]any{
								"text": "hello",
								"chat": map[string]any{"id": 42, "type": "private"},
								"from": map[string]any{"id": 99, "username": "alice"},
							},
						},
					},
				}), nil
			case strings.Contains(req.URL.String(), "/sendMessage"):
				atomic.AddInt32(&sendCalls, 1)
				body, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatalf("read telegram send body: %v", err)
				}
				values, err := url.ParseQuery(string(body))
				if err != nil {
					t.Fatalf("parse telegram send body: %v", err)
				}
				switch values.Get("text") {
				case "hello":
					if cancelRun != nil {
						cancelRun()
					}
					return jsonResponse(http.StatusOK, map[string]any{"ok": true}), nil
				case "\u200B", "hello again":
					return jsonResponse(http.StatusOK, map[string]any{"ok": true, "result": map[string]any{"message_id": 77}}), nil
				default:
					return jsonResponse(http.StatusOK, map[string]any{"ok": true}), nil
				}
			case strings.Contains(req.URL.String(), "/editMessageText"):
				body, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatalf("read telegram edit body: %v", err)
				}
				editBody = string(body)
				if cancelRunStream != nil {
					cancelRunStream()
				}
				return jsonResponse(http.StatusOK, map[string]any{"ok": true}), nil
			case strings.Contains(req.URL.String(), "/deleteMessage"):
				atomic.AddInt32(&deleteCalls, 1)
				return jsonResponse(http.StatusOK, map[string]any{"ok": true}), nil
			default:
				return nil, fmt.Errorf("unexpected telegram lifecycle request: %s", req.URL.String())
			}
		}),
	}

	runCtx, runCancel := context.WithCancel(context.Background())
	cancelRun = runCancel
	if err := adapter.Run(runCtx, func(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error) {
		if message != "hello" {
			t.Fatalf("expected telegram Run to pass text message, got %q", message)
		}
		return "session-run", "hello", nil
	}); err != nil {
		t.Fatalf("telegram Run returned error: %v", err)
	}

	streamCtx, streamCancel := context.WithCancel(context.Background())
	cancelRunStream = streamCancel
	if err := adapter.RunStream(streamCtx, func(ctx context.Context, sessionID string, message string, meta map[string]string, onChunk func(chunk string) error) (string, error) {
		if message != "hello" {
			t.Fatalf("expected telegram RunStream to pass text message, got %q", message)
		}
		if err := onChunk(strings.Repeat("x", 4105)); err != nil {
			return "", err
		}
		return "session-stream", nil
	}); err != nil {
		t.Fatalf("telegram RunStream returned error: %v", err)
	}

	if ts, err := adapter.sendMessageWithResult(context.Background(), "42", "hello again"); err != nil || ts != "77" {
		t.Fatalf("expected sendMessageWithResult to return message id, got %q %v", ts, err)
	}
	if err := adapter.editMessage(context.Background(), "42", "77", strings.Repeat("x", 4105)); err != nil {
		t.Fatalf("editMessage returned error: %v", err)
	}
	values, err := url.ParseQuery(editBody)
	if err != nil {
		t.Fatalf("parse telegram edit body: %v", err)
	}
	if got := values.Get("text"); len([]rune(got)) != 4096 || !strings.HasSuffix(got, "...") {
		t.Fatalf("expected telegram edit text to be truncated to 4096 runes, got len=%d text=%q", len([]rune(got)), got)
	}
	if err := adapter.deleteMessage(context.Background(), "42", "77"); err != nil {
		t.Fatalf("deleteMessage returned error: %v", err)
	}
	if err := adapter.deleteMessage(context.Background(), "42", ""); err != nil {
		t.Fatalf("expected blank deleteMessage to be a no-op, got %v", err)
	}
	if atomic.LoadInt32(&sendCalls) < 3 {
		t.Fatalf("expected lifecycle test to send multiple telegram requests, got %d", sendCalls)
	}
	if atomic.LoadInt32(&deleteCalls) != 1 {
		t.Fatalf("expected one explicit telegram delete call, got %d", deleteCalls)
	}
}

func TestTelegramResolveAndDownloadFileErrors(t *testing.T) {
	adapter := NewTelegramAdapter(config.TelegramChannelConfig{
		Enabled:  true,
		BotToken: "token",
	}, nil)

	if _, err := adapter.resolveFileDownloadURL(context.Background(), "   "); err == nil {
		t.Fatal("expected missing telegram file id to fail")
	}

	adapter.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch {
			case strings.Contains(req.URL.String(), "/getFile") && strings.Contains(req.URL.RawQuery, "file_id=missing"):
				return jsonResponse(http.StatusOK, map[string]any{"ok": false, "description": "missing file"}), nil
			case strings.Contains(req.URL.String(), "/getFile") && strings.Contains(req.URL.RawQuery, "file_id=download-fail"):
				return jsonResponse(http.StatusOK, map[string]any{
					"ok":     true,
					"result": map[string]any{"file_id": "download-fail", "file_path": "voice/fail.ogg"},
				}), nil
			case strings.Contains(req.URL.String(), "/file/bottoken/voice/fail.ogg"):
				return &http.Response{
					StatusCode: http.StatusBadGateway,
					Body:       io.NopCloser(strings.NewReader("bad gateway")),
					Header:     make(http.Header),
				}, nil
			default:
				return nil, fmt.Errorf("unexpected telegram helper request: %s", req.URL.String())
			}
		}),
	}

	if _, err := adapter.resolveFileDownloadURL(context.Background(), "missing"); err == nil || !strings.Contains(err.Error(), "missing file") {
		t.Fatalf("expected telegram getFile error, got %v", err)
	}
	if _, _, err := adapter.downloadFile(context.Background(), "download-fail"); err == nil || !strings.Contains(err.Error(), "download failed") {
		t.Fatalf("expected telegram download error, got %v", err)
	}
}

func TestTelegramPollOncePreservesOffsetOnHandlerFailure(t *testing.T) {
	adapter := NewTelegramAdapter(config.TelegramChannelConfig{
		Enabled:   true,
		BotToken:  "token",
		PollEvery: 1,
	}, nil)

	adapter.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch {
			case strings.Contains(req.URL.String(), "/getUpdates"):
				return jsonResponse(http.StatusOK, map[string]any{
					"ok": true,
					"result": []map[string]any{
						{
							"update_id": 7,
							"message": map[string]any{
								"text": "hello",
								"chat": map[string]any{"id": 42, "type": "private"},
								"from": map[string]any{"id": 99, "username": "alice"},
							},
						},
					},
				}), nil
			case strings.Contains(req.URL.String(), "/sendMessage"):
				return jsonResponse(http.StatusOK, map[string]any{"ok": true}), nil
			default:
				return nil, fmt.Errorf("unexpected request URL: %s", req.URL.String())
			}
		}),
	}

	calls := 0
	wantErr := fmt.Errorf("boom")
	err := adapter.pollOnce(context.Background(), func(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error) {
		calls++
		return "", "", wantErr
	})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected handler error, got %v", err)
	}
	if adapter.offset != 0 {
		t.Fatalf("expected offset to remain unchanged after failure, got %d", adapter.offset)
	}

	err = adapter.pollOnce(context.Background(), func(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error) {
		calls++
		return "session-1", "ok", nil
	})
	if err != nil {
		t.Fatalf("expected second poll to succeed, got %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected failed update to be retried, got %d calls", calls)
	}
	if adapter.offset != 8 {
		t.Fatalf("expected offset to advance after success, got %d", adapter.offset)
	}
}

func TestTelegramPollOnceFallsBackToCaptionWhenTextMissing(t *testing.T) {
	adapter := NewTelegramAdapter(config.TelegramChannelConfig{
		Enabled:   true,
		BotToken:  "token",
		PollEvery: 1,
	}, nil)

	adapter.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch {
			case strings.Contains(req.URL.String(), "/getUpdates"):
				return jsonResponse(http.StatusOK, map[string]any{
					"ok": true,
					"result": []map[string]any{
						{
							"update_id": 9,
							"message": map[string]any{
								"caption": "photo caption",
								"chat":    map[string]any{"id": 42, "type": "private"},
								"from":    map[string]any{"id": 99, "username": "alice"},
								"document": map[string]any{
									"file_id":   "doc-1",
									"mime_type": "image/png",
								},
							},
						},
					},
				}), nil
			case strings.Contains(req.URL.String(), "/sendMessage"):
				return jsonResponse(http.StatusOK, map[string]any{"ok": true}), nil
			default:
				return nil, fmt.Errorf("unexpected request URL: %s", req.URL.String())
			}
		}),
	}

	calls := 0
	err := adapter.pollOnce(context.Background(), func(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error) {
		calls++
		if message != "photo caption" {
			t.Fatalf("expected caption fallback message, got %q", message)
		}
		if meta["message_type"] != "" {
			t.Fatalf("expected plain text caption flow, got %+v", meta)
		}
		return "session-1", "ok", nil
	})
	if err != nil {
		t.Fatalf("pollOnce failed: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected caption-only update to reach handler once, got %d", calls)
	}
	if adapter.offset != 10 {
		t.Fatalf("expected offset to advance after caption fallback, got %d", adapter.offset)
	}
}

func TestTelegramPollOnceProcessesVoiceMessagesWithoutTokenLeak(t *testing.T) {
	var (
		outbound      []string
		eventType     string
		eventSession  string
		eventPayload  map[string]any
		getFileCalls  int32
		downloadCalls int32
		audioPath     string
	)

	adapter := NewTelegramAdapter(config.TelegramChannelConfig{
		Enabled:   true,
		BotToken:  "secret-token",
		PollEvery: 1,
	}, func(kind string, sessionID string, payload map[string]any) {
		eventType = kind
		eventSession = sessionID
		eventPayload = payload
	})

	adapter.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch {
			case strings.Contains(req.URL.String(), "/getUpdates"):
				return jsonResponse(http.StatusOK, map[string]any{
					"ok": true,
					"result": []map[string]any{
						{
							"update_id": 8,
							"message": map[string]any{
								"caption": "voice caption",
								"chat":    map[string]any{"id": 42, "type": "private"},
								"from":    map[string]any{"id": 99, "username": "alice"},
								"voice":   map[string]any{"file_id": "voice-1", "mime_type": "audio/ogg"},
							},
						},
					},
				}), nil
			case strings.Contains(req.URL.String(), "/getFile"):
				atomic.AddInt32(&getFileCalls, 1)
				return jsonResponse(http.StatusOK, map[string]any{
					"ok":     true,
					"result": map[string]any{"file_id": "voice-1", "file_path": "voice/path.ogg"},
				}), nil
			case strings.Contains(req.URL.String(), "/file/botsecret-token/voice/path.ogg"):
				atomic.AddInt32(&downloadCalls, 1)
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("voice-bytes")),
					Header:     make(http.Header),
				}, nil
			case strings.Contains(req.URL.String(), "/sendMessage"):
				body, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatalf("read request body: %v", err)
				}
				outbound = append(outbound, string(body))
				return jsonResponse(http.StatusOK, map[string]any{"ok": true}), nil
			default:
				return nil, fmt.Errorf("unexpected request URL: %s", req.URL.String())
			}
		}),
	}

	err := adapter.pollOnce(context.Background(), func(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error) {
		if meta["message_type"] != "voice_note" {
			t.Fatalf("expected voice_note meta, got %+v", meta)
		}
		if got := meta["audio_ref"]; got != "telegram-file:voice-1" {
			t.Fatalf("expected opaque audio ref, got %+v", meta)
		}
		if got := meta["audio_file_id"]; got != "voice-1" {
			t.Fatalf("expected audio file id, got %+v", meta)
		}
		if got := meta["audio_mime"]; got != "audio/ogg" {
			t.Fatalf("expected audio mime, got %+v", meta)
		}
		if got := meta["audio_url"]; got == "" {
			t.Fatalf("expected retrievable audio path, got %+v", meta)
		} else {
			audioPath = got
			if strings.Contains(got, "secret-token") {
				t.Fatalf("expected audio path to hide bot token, got %+v", meta)
			}
			data, err := os.ReadFile(got)
			if err != nil {
				t.Fatalf("expected audio path to be readable: %v", err)
			}
			if string(data) != "voice-bytes" {
				t.Fatalf("expected downloaded audio bytes, got %q", string(data))
			}
		}
		if strings.Contains(meta["audio_ref"], "secret-token") {
			t.Fatalf("expected audio ref to hide bot token, got %+v", meta)
		}
		if message != meta["audio_url"] {
			t.Fatalf("expected message payload to use audio path, got %q meta=%+v", message, meta)
		}
		return "session-voice", "voice reply", nil
	})
	if err != nil {
		t.Fatalf("pollOnce failed: %v", err)
	}
	if len(outbound) != 1 || !strings.Contains(outbound[0], "text=voice+reply") {
		t.Fatalf("expected voice reply to be sent once, got %+v", outbound)
	}
	if adapter.offset != 9 {
		t.Fatalf("expected offset to advance after voice success, got %d", adapter.offset)
	}
	if atomic.LoadInt32(&getFileCalls) != 1 {
		t.Fatalf("expected one getFile call, got %d", getFileCalls)
	}
	if atomic.LoadInt32(&downloadCalls) != 1 {
		t.Fatalf("expected one file download call, got %d", downloadCalls)
	}
	if eventType != "channel.telegram.voice" || eventSession != "session-voice" {
		t.Fatalf("unexpected event info: %q %q", eventType, eventSession)
	}
	if got := eventPayload["audio_ref"]; got != "telegram-file:voice-1" {
		t.Fatalf("expected opaque event audio ref, got %+v", eventPayload)
	}
	if got := eventPayload["audio_file_id"]; got != "voice-1" {
		t.Fatalf("expected event audio file id, got %+v", eventPayload)
	}
	if got := eventPayload["audio_mime"]; got != "audio/ogg" {
		t.Fatalf("expected event audio mime, got %+v", eventPayload)
	}
	if _, ok := eventPayload["audio_url"]; ok {
		t.Fatalf("expected event payload to omit audio_url, got %+v", eventPayload)
	}
	if audioPath == "" {
		t.Fatal("expected audio path to be captured during handler execution")
	}
	if _, err := os.Stat(audioPath); !os.IsNotExist(err) {
		t.Fatalf("expected temporary audio file to be cleaned up, stat err=%v", err)
	}
}

func TestTelegramPollOnceStreamProcessesVoiceMessages(t *testing.T) {
	var (
		outbound      []string
		eventType     string
		eventSession  string
		eventPayload  map[string]any
		getFileCalls  int32
		downloadCalls int32
		audioPath     string
	)

	adapter := NewTelegramAdapter(config.TelegramChannelConfig{
		Enabled:   true,
		BotToken:  "secret-token",
		PollEvery: 1,
	}, func(kind string, sessionID string, payload map[string]any) {
		eventType = kind
		eventSession = sessionID
		eventPayload = payload
	})

	adapter.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch {
			case strings.Contains(req.URL.String(), "/getUpdates"):
				return jsonResponse(http.StatusOK, map[string]any{
					"ok": true,
					"result": []map[string]any{
						{
							"update_id": 8,
							"message": map[string]any{
								"caption": "voice caption",
								"chat":    map[string]any{"id": 42, "type": "private"},
								"from":    map[string]any{"id": 99, "username": "alice"},
								"voice":   map[string]any{"file_id": "voice-1", "mime_type": "audio/ogg"},
							},
						},
					},
				}), nil
			case strings.Contains(req.URL.String(), "/getFile"):
				atomic.AddInt32(&getFileCalls, 1)
				return jsonResponse(http.StatusOK, map[string]any{
					"ok":     true,
					"result": map[string]any{"file_id": "voice-1", "file_path": "voice/path.ogg"},
				}), nil
			case strings.Contains(req.URL.String(), "/file/botsecret-token/voice/path.ogg"):
				atomic.AddInt32(&downloadCalls, 1)
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("voice-bytes")),
					Header:     make(http.Header),
				}, nil
			case strings.Contains(req.URL.String(), "/sendMessage"):
				body, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatalf("read request body: %v", err)
				}
				outbound = append(outbound, string(body))
				return jsonResponse(http.StatusOK, map[string]any{"ok": true}), nil
			default:
				return nil, fmt.Errorf("unexpected request URL: %s", req.URL.String())
			}
		}),
	}

	err := adapter.pollOnceStream(context.Background(), func(ctx context.Context, sessionID string, message string, meta map[string]string, onChunk func(chunk string) error) (string, error) {
		if meta["message_type"] != "voice_note" {
			t.Fatalf("expected voice_note meta, got %+v", meta)
		}
		if got := meta["audio_ref"]; got != "telegram-file:voice-1" {
			t.Fatalf("expected opaque audio ref in meta, got %+v", meta)
		}
		if got := meta["audio_file_id"]; got != "voice-1" {
			t.Fatalf("expected audio file id in meta, got %+v", meta)
		}
		if got := meta["audio_mime"]; got != "audio/ogg" {
			t.Fatalf("expected audio mime in meta, got %+v", meta)
		}
		if got := meta["audio_url"]; got == "" {
			t.Fatalf("expected retrievable audio path in meta, got %+v", meta)
		} else {
			audioPath = got
			if strings.Contains(got, "secret-token") {
				t.Fatalf("expected audio path to hide bot token, got %+v", meta)
			}
			data, err := os.ReadFile(got)
			if err != nil {
				t.Fatalf("expected audio path to be readable: %v", err)
			}
			if string(data) != "voice-bytes" {
				t.Fatalf("expected downloaded audio bytes, got %q", string(data))
			}
		}
		if strings.Contains(meta["audio_ref"], "secret-token") {
			t.Fatalf("expected audio ref to hide bot token, got %+v", meta)
		}
		if meta["caption"] != "voice caption" {
			t.Fatalf("expected caption to be preserved, got %+v", meta)
		}
		if message != meta["audio_url"] {
			t.Fatalf("expected streaming payload to use audio path, got %q meta=%+v", message, meta)
		}
		if err := onChunk("voice reply"); err != nil {
			return "", err
		}
		return "session-1", nil
	})
	if err != nil {
		t.Fatalf("pollOnceStream failed: %v", err)
	}
	if len(outbound) != 1 || !strings.Contains(outbound[0], "text=voice+reply") {
		t.Fatalf("expected voice reply to be sent once, got %+v", outbound)
	}
	if adapter.offset != 9 {
		t.Fatalf("expected offset to advance after streaming voice success, got %d", adapter.offset)
	}
	if atomic.LoadInt32(&getFileCalls) != 1 {
		t.Fatalf("expected one getFile call, got %d", getFileCalls)
	}
	if atomic.LoadInt32(&downloadCalls) != 1 {
		t.Fatalf("expected one file download call, got %d", downloadCalls)
	}
	if eventType != "channel.telegram.voice" || eventSession != "session-1" {
		t.Fatalf("unexpected event info: %q %q", eventType, eventSession)
	}
	if got := eventPayload["audio_ref"]; got != "telegram-file:voice-1" {
		t.Fatalf("expected opaque event audio ref, got %+v", eventPayload)
	}
	if got := eventPayload["audio_file_id"]; got != "voice-1" {
		t.Fatalf("expected event audio file id, got %+v", eventPayload)
	}
	if got := eventPayload["audio_mime"]; got != "audio/ogg" {
		t.Fatalf("expected event audio mime, got %+v", eventPayload)
	}
	if _, ok := eventPayload["audio_url"]; ok {
		t.Fatalf("expected event payload to omit audio_url, got %+v", eventPayload)
	}
	if got := eventPayload["streaming"]; got != true {
		t.Fatalf("expected streaming voice event, got %+v", eventPayload)
	}
	if audioPath == "" {
		t.Fatal("expected audio path to be captured during handler execution")
	}
	if _, err := os.Stat(audioPath); !os.IsNotExist(err) {
		t.Fatalf("expected temporary audio file to be cleaned up, stat err=%v", err)
	}
}

func TestTelegramSendStreamingMessageDeletesPlaceholderWhenNoChunksArrive(t *testing.T) {
	var (
		sendCalls   int32
		deleteCalls int32
		editCalls   int32
	)

	adapter := NewTelegramAdapter(config.TelegramChannelConfig{
		Enabled:   true,
		BotToken:  "token",
		PollEvery: 1,
	}, nil)

	adapter.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch {
			case strings.Contains(req.URL.String(), "/sendMessage"):
				atomic.AddInt32(&sendCalls, 1)
				return jsonResponse(http.StatusOK, map[string]any{
					"ok": true,
					"result": map[string]any{
						"message_id": 12,
					},
				}), nil
			case strings.Contains(req.URL.String(), "/deleteMessage"):
				atomic.AddInt32(&deleteCalls, 1)
				return jsonResponse(http.StatusOK, map[string]any{"ok": true}), nil
			case strings.Contains(req.URL.String(), "/editMessageText"):
				atomic.AddInt32(&editCalls, 1)
				return jsonResponse(http.StatusOK, map[string]any{"ok": true}), nil
			default:
				return nil, fmt.Errorf("unexpected request URL: %s", req.URL.String())
			}
		}),
	}

	if err := adapter.sendStreamingMessage(context.Background(), "42", func(onChunk func(chunk string) error) error {
		return nil
	}); err != nil {
		t.Fatalf("sendStreamingMessage failed: %v", err)
	}
	if atomic.LoadInt32(&sendCalls) != 1 {
		t.Fatalf("expected placeholder message to be sent once, got %d", sendCalls)
	}
	if atomic.LoadInt32(&deleteCalls) != 1 {
		t.Fatalf("expected placeholder message to be deleted once, got %d", deleteCalls)
	}
	if atomic.LoadInt32(&editCalls) != 0 {
		t.Fatalf("expected no empty edit request, got %d", editCalls)
	}
}

func TestTelegramSendStreamingMessageDeletesPlaceholderWhenStreamFailsWithoutChunks(t *testing.T) {
	var (
		sendCalls   int32
		deleteCalls int32
		editCalls   int32
	)

	adapter := NewTelegramAdapter(config.TelegramChannelConfig{
		Enabled:   true,
		BotToken:  "token",
		PollEvery: 1,
	}, nil)

	adapter.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch {
			case strings.Contains(req.URL.String(), "/sendMessage"):
				atomic.AddInt32(&sendCalls, 1)
				return jsonResponse(http.StatusOK, map[string]any{
					"ok": true,
					"result": map[string]any{
						"message_id": 15,
					},
				}), nil
			case strings.Contains(req.URL.String(), "/deleteMessage"):
				atomic.AddInt32(&deleteCalls, 1)
				return jsonResponse(http.StatusOK, map[string]any{"ok": true}), nil
			case strings.Contains(req.URL.String(), "/editMessageText"):
				atomic.AddInt32(&editCalls, 1)
				return jsonResponse(http.StatusOK, map[string]any{"ok": true}), nil
			default:
				return nil, fmt.Errorf("unexpected request URL: %s", req.URL.String())
			}
		}),
	}

	wantErr := fmt.Errorf("stream failed")
	err := adapter.sendStreamingMessage(context.Background(), "42", func(onChunk func(chunk string) error) error {
		return wantErr
	})
	if err == nil || !strings.Contains(err.Error(), wantErr.Error()) {
		t.Fatalf("expected original stream error, got %v", err)
	}
	if atomic.LoadInt32(&sendCalls) != 1 {
		t.Fatalf("expected placeholder message to be sent once, got %d", sendCalls)
	}
	if atomic.LoadInt32(&deleteCalls) != 1 {
		t.Fatalf("expected placeholder message to be deleted once, got %d", deleteCalls)
	}
	if atomic.LoadInt32(&editCalls) != 0 {
		t.Fatalf("expected no empty edit request, got %d", editCalls)
	}
}

func TestSignalPollOnceRetriesMessageUntilSendSucceeds(t *testing.T) {
	sendCalls := 0
	handleCalls := 0

	adapter := NewSignalAdapter(config.SignalChannelConfig{
		Enabled:   true,
		BaseURL:   "https://signal.example",
		Number:    "+1000",
		PollEvery: 1,
	}, nil)

	adapter.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch {
			case strings.Contains(req.URL.String(), "/v1/receive/"):
				return jsonResponse(http.StatusOK, []map[string]any{
					{
						"envelope": map[string]any{
							"timestamp":  123,
							"source":     "+2000",
							"sourceName": "bob",
							"dataMessage": map[string]any{
								"message": "hello",
							},
						},
					},
				}), nil
			case strings.Contains(req.URL.String(), "/v2/send"):
				sendCalls++
				if sendCalls == 1 {
					return jsonResponse(http.StatusBadGateway, map[string]any{"error": "fail"}), nil
				}
				return jsonResponse(http.StatusOK, map[string]any{"timestamp": 124}), nil
			default:
				return nil, fmt.Errorf("unexpected request URL: %s", req.URL.String())
			}
		}),
	}

	err := adapter.pollOnce(context.Background(), func(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error) {
		handleCalls++
		return "session-1", "ok", nil
	})
	if err == nil {
		t.Fatal("expected first send failure")
	}
	if adapter.latestTS != 0 {
		t.Fatalf("expected latestTS to remain unchanged after send failure, got %d", adapter.latestTS)
	}

	err = adapter.pollOnce(context.Background(), func(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error) {
		handleCalls++
		return "session-1", "ok", nil
	})
	if err != nil {
		t.Fatalf("expected second poll to succeed, got %v", err)
	}
	if handleCalls != 2 {
		t.Fatalf("expected message to be retried, got %d handler calls", handleCalls)
	}
	if adapter.latestTS != 123 {
		t.Fatalf("expected latestTS to advance after success, got %d", adapter.latestTS)
	}
}

func TestSignalPollOnceProcessesDistinctMessagesWithSameTimestamp(t *testing.T) {
	sendCalls := 0
	var handled []string

	adapter := NewSignalAdapter(config.SignalChannelConfig{
		Enabled:   true,
		BaseURL:   "https://signal.example",
		Number:    "+1000",
		PollEvery: 1,
	}, nil)

	adapter.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch {
			case strings.Contains(req.URL.String(), "/v1/receive/"):
				return jsonResponse(http.StatusOK, []map[string]any{
					{
						"envelope": map[string]any{
							"timestamp":  123,
							"source":     "+2000",
							"sourceName": "bob",
							"dataMessage": map[string]any{
								"message": "hello",
							},
						},
					},
					{
						"envelope": map[string]any{
							"timestamp":  123,
							"source":     "+3000",
							"sourceName": "carol",
							"dataMessage": map[string]any{
								"message": "world",
							},
						},
					},
				}), nil
			case strings.Contains(req.URL.String(), "/v2/send"):
				sendCalls++
				return jsonResponse(http.StatusOK, map[string]any{"timestamp": 124}), nil
			default:
				return nil, fmt.Errorf("unexpected request URL: %s", req.URL.String())
			}
		}),
	}

	err := adapter.pollOnce(context.Background(), func(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error) {
		handled = append(handled, message)
		return "session-1", "ok", nil
	})
	if err != nil {
		t.Fatalf("expected poll to succeed, got %v", err)
	}
	if len(handled) != 2 {
		t.Fatalf("expected both same-timestamp messages to be handled, got %d", len(handled))
	}
	if handled[0] != "hello" || handled[1] != "world" {
		t.Fatalf("unexpected handled messages: %v", handled)
	}
	if sendCalls != 2 {
		t.Fatalf("expected 2 send attempts, got %d", sendCalls)
	}
	if adapter.latestTS != 123 {
		t.Fatalf("expected latestTS to advance to 123, got %d", adapter.latestTS)
	}
}

func TestSignalPollOnceProcessesVoiceGroupMessages(t *testing.T) {
	var (
		eventType    string
		eventSession string
		eventPayload map[string]any
		sendBody     string
	)

	adapter := NewSignalAdapter(config.SignalChannelConfig{
		Enabled:     true,
		BaseURL:     "https://signal.example",
		Number:      "+1000",
		PollEvery:   1,
		BearerToken: "secret",
	}, func(kind string, sessionID string, payload map[string]any) {
		eventType = kind
		eventSession = sessionID
		eventPayload = payload
	})

	adapter.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch {
			case strings.Contains(req.URL.String(), "/v1/receive/"):
				if req.Header.Get("Authorization") != "Bearer secret" {
					t.Fatalf("expected bearer token on signal receive request, got %q", req.Header.Get("Authorization"))
				}
				return jsonResponse(http.StatusOK, []map[string]any{
					{
						"envelope": map[string]any{
							"timestamp":  234,
							"source":     "+2000",
							"sourceName": "bob",
							"groupInfo":  map[string]any{"groupId": "group-1"},
							"dataMessage": map[string]any{
								"message": "voice caption",
								"attachments": []map[string]any{
									{"filename": "clip.m4a"},
								},
							},
						},
					},
				}), nil
			case strings.Contains(req.URL.String(), "/v2/send"):
				body, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatalf("read signal send body: %v", err)
				}
				sendBody = string(body)
				if req.Header.Get("Authorization") != "Bearer secret" {
					t.Fatalf("expected bearer token on signal send request, got %q", req.Header.Get("Authorization"))
				}
				return jsonResponse(http.StatusOK, map[string]any{"timestamp": 235}), nil
			default:
				return nil, fmt.Errorf("unexpected signal voice request: %s", req.URL.String())
			}
		}),
	}

	err := adapter.pollOnce(context.Background(), func(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error) {
		if message != "clip.m4a" {
			t.Fatalf("expected voice attachment filename as message payload, got %q", message)
		}
		if meta["message_type"] != "voice_note" || meta["audio_url"] != "clip.m4a" || meta["audio_mime"] != "audio/mp4" {
			t.Fatalf("unexpected signal voice metadata: %+v", meta)
		}
		if meta["caption"] != "voice caption" || meta["reply_target"] != "group-1" {
			t.Fatalf("expected caption and group reply target, got %+v", meta)
		}
		if meta["channel_type"] != "group" || meta["is_group"] != "true" {
			t.Fatalf("expected group metadata, got %+v", meta)
		}
		return "session-voice", "ok", nil
	})
	if err != nil {
		t.Fatalf("pollOnce failed: %v", err)
	}
	if !strings.Contains(sendBody, `"group-1"`) {
		t.Fatalf("expected signal outbound send to target group recipient, got %s", sendBody)
	}
	if adapter.latestTS != 234 {
		t.Fatalf("expected latestTS to advance after voice success, got %d", adapter.latestTS)
	}
	expectedMessageID := signalMessageID(
		"+2000",
		"group-1",
		234,
		"voice caption",
		[]struct {
			ContentType string `json:"contentType"`
			Filename    string `json:"filename"`
		}{
			{Filename: "clip.m4a"},
		},
	)
	if !adapter.hasSeen(expectedMessageID) {
		t.Fatalf("expected signal voice message %q to be marked seen", expectedMessageID)
	}
	if eventType != "channel.signal.voice" || eventSession != "session-voice" {
		t.Fatalf("unexpected signal voice event info: %q %q", eventType, eventSession)
	}
	if got := eventPayload["audio_mime"]; got != "audio/mp4" {
		t.Fatalf("expected signal voice event mime, got %+v", eventPayload)
	}
}

func TestSignalRunRecordsPollErrorsBeforeShutdown(t *testing.T) {
	var eventPayload map[string]any

	adapter := NewSignalAdapter(config.SignalChannelConfig{
		Enabled:   true,
		BaseURL:   "https://signal.example",
		Number:    "+1000",
		PollEvery: 1,
	}, func(kind string, sessionID string, payload map[string]any) {
		if kind == "channel.signal.error" {
			eventPayload = payload
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	adapter.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			cancel()
			return jsonResponse(http.StatusBadGateway, map[string]any{"error": "fail"}), nil
		}),
	}

	if err := adapter.Run(ctx, func(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error) {
		t.Fatal("expected handler not to be called when signal receive fails")
		return "", "", nil
	}); err != nil {
		t.Fatalf("signal Run returned error: %v", err)
	}

	status := adapter.Status()
	if !strings.Contains(status.LastError, "signal receive failed") || status.Healthy {
		t.Fatalf("expected signal status to record poll error, got %+v", status)
	}
	if eventPayload == nil || !strings.Contains(fmt.Sprint(eventPayload["error"]), "signal receive failed") {
		t.Fatalf("expected signal error event payload, got %+v", eventPayload)
	}
}

func TestSignalAdapterStateHelpers(t *testing.T) {
	adapter := &SignalAdapter{
		processed: make(map[string]time.Time),
	}

	if adapter.seen("msg-1") {
		t.Fatal("expected first seen check to report false")
	}
	if !adapter.seen("msg-1") {
		t.Fatal("expected second seen check to report true")
	}

	adapter.processed["stale"] = time.Now().Add(-31 * time.Minute)
	if adapter.hasSeen("stale") {
		t.Fatal("expected stale processed message to be pruned")
	}

	adapter.advanceTimestamp(10)
	adapter.advanceTimestamp(5)
	if adapter.latestTS != 10 {
		t.Fatalf("expected advanceTimestamp to keep max timestamp, got %d", adapter.latestTS)
	}

	messageID := signalMessageID(
		" +2000 ",
		" group-1 ",
		123,
		" hello ",
		[]struct {
			ContentType string `json:"contentType"`
			Filename    string `json:"filename"`
		}{
			{ContentType: "audio/ogg", Filename: " clip.ogg "},
		},
	)
	if messageID != "+2000|group-1|123|hello|audio/ogg:clip.ogg" {
		t.Fatalf("unexpected normalized signal message id: %q", messageID)
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
					return jsonResponse(http.StatusOK, []map[string]any{
						{
							"id":         "m1",
							"channel_id": "c1",
							"content":    "hello",
							"guild_id":   "g1",
							"author":     map[string]any{"id": "u1", "username": "alice", "bot": false},
							"message_reference": map[string]any{
								"message_id": "parent-123",
								"channel_id": "c1",
							},
						},
					}), nil
				case http.MethodPost:
					body, err := io.ReadAll(req.Body)
					if err != nil {
						t.Fatalf("read request body: %v", err)
					}
					postedURL = req.URL.String()
					postedBody = string(body)
					return jsonResponse(http.StatusOK, map[string]any{"id": "reply-1"}), nil
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
