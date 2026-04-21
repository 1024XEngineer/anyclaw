package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/config"
	"github.com/gorilla/websocket"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonResponse(status int, payload any) *http.Response {
	body, _ := json.Marshal(payload)
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
}

func readJSONBody(t *testing.T, req *http.Request) map[string]any {
	t.Helper()
	data, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	if len(data) == 0 {
		return map[string]any{}
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	return payload
}

func TestReadBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/body", strings.NewReader("payload"))
	body, err := ReadBody(req)
	if err != nil {
		t.Fatalf("ReadBody returned error: %v", err)
	}
	if string(body) != "payload" {
		t.Fatalf("expected payload body, got %q", string(body))
	}
}

func TestDiscordAdapterHelpers(t *testing.T) {
	adapter := NewDiscordAdapter(config.DiscordChannelConfig{
		Enabled:   true,
		BotToken:  "token",
		GuildID:   "guild",
		PublicKey: "not-hex",
	}, nil)

	if adapter.Name() != "discord" {
		t.Fatalf("expected adapter name discord, got %q", adapter.Name())
	}
	if adapter.Enabled() {
		t.Fatal("expected adapter without default channel to be disabled")
	}
	if adapter.apiBaseURL != "https://discord.com/api/v10" {
		t.Fatalf("expected default api base url, got %q", adapter.apiBaseURL)
	}

	req := httptest.NewRequest(http.MethodPost, "/interactions", nil)
	if adapter.VerifyInteraction(req, []byte(`{}`)) {
		t.Fatal("expected invalid public key config to fail verification")
	}

	audioURL, audioMIME := adapter.findAudioAttachment([]struct {
		ID          string "json:\"id\""
		URL         string "json:\"url\""
		ProxyURL    string "json:\"proxy_url\""
		Filename    string "json:\"filename\""
		ContentType string "json:\"content_type\""
		Size        int    "json:\"size\""
	}{
		{Filename: "voice.ogg", ProxyURL: "https://audio.example/voice"},
	})
	if audioURL != "https://audio.example/voice" || audioMIME != "audio/ogg" {
		t.Fatalf("unexpected filename-based audio detection: %q %q", audioURL, audioMIME)
	}

	adapter.processed["stale"] = time.Now().UTC().Add(-31 * time.Minute)
	if adapter.seen("fresh") {
		t.Fatal("expected first message id to be unseen")
	}
	if _, ok := adapter.processed["stale"]; ok {
		t.Fatal("expected stale processed entry to be cleaned up")
	}
	if !adapter.seen("fresh") {
		t.Fatal("expected repeated message id to be marked as seen")
	}
}

func TestDiscordAdapterHandleInteractionPingAndError(t *testing.T) {
	adapter := NewDiscordAdapter(config.DiscordChannelConfig{
		Enabled:        true,
		BotToken:       "token",
		DefaultChannel: "123",
	}, nil)

	resp, err := adapter.HandleInteraction(context.Background(), []byte(`{"type":1}`), func(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error) {
		t.Fatal("ping interaction should not invoke handler")
		return "", "", nil
	})
	if err != nil {
		t.Fatalf("ping interaction returned error: %v", err)
	}
	if resp["type"] != 1 {
		t.Fatalf("expected ping response type 1, got %#v", resp["type"])
	}

	if _, err := adapter.HandleInteraction(context.Background(), []byte(`{bad json}`), nil); err == nil {
		t.Fatal("expected invalid interaction payload to fail")
	}
}

func TestDiscordAdapterSendMethods(t *testing.T) {
	var (
		postBodies  []map[string]any
		patchBodies []map[string]any
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			postBodies = append(postBodies, readJSONBody(t, r))
			_, _ = w.Write([]byte(`{"id":"sent-1"}`))
		case http.MethodPatch:
			patchBodies = append(patchBodies, readJSONBody(t, r))
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	}))
	defer server.Close()

	adapter := NewDiscordAdapter(config.DiscordChannelConfig{
		Enabled:        true,
		BotToken:       "token",
		DefaultChannel: "default-chan",
		APIBaseURL:     server.URL,
	}, nil)
	adapter.client = server.Client()

	if err := adapter.sendMessage(context.Background(), "", "reply-1", "hello"); err != nil {
		t.Fatalf("sendMessage returned error: %v", err)
	}
	messageID, err := adapter.sendMessageWithResult(context.Background(), "chan-2", "", "hello 2")
	if err != nil {
		t.Fatalf("sendMessageWithResult returned error: %v", err)
	}
	if messageID != "sent-1" {
		t.Fatalf("expected sent message id, got %q", messageID)
	}

	longText := strings.Repeat("x", 2105)
	if err := adapter.editMessage(context.Background(), "", "msg-1", longText); err != nil {
		t.Fatalf("editMessage returned error: %v", err)
	}

	if len(postBodies) != 2 {
		t.Fatalf("expected 2 post requests, got %d", len(postBodies))
	}
	if postBodies[0]["content"] != "hello" {
		t.Fatalf("unexpected first post body: %+v", postBodies[0])
	}
	if _, ok := postBodies[0]["message_reference"]; !ok {
		t.Fatalf("expected reply message reference in first post body: %+v", postBodies[0])
	}
	if postBodies[1]["content"] != "hello 2" {
		t.Fatalf("unexpected second post body: %+v", postBodies[1])
	}

	if len(patchBodies) != 1 {
		t.Fatalf("expected 1 patch request, got %d", len(patchBodies))
	}
	patched := patchBodies[0]["content"].(string)
	if len([]rune(patched)) != 2000 || !strings.HasSuffix(patched, "...") {
		t.Fatalf("expected truncated patch body, got len=%d content suffix=%q", len([]rune(patched)), patched[len(patched)-3:])
	}
}

func TestDiscordAdapterPollOnceProcessesMessages(t *testing.T) {
	var (
		sentBodies []map[string]any
		events     []string
		calls      []map[string]string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/messages"):
			_, _ = w.Write([]byte(`[
				{
					"id":"2",
					"channel_id":"chan-1",
					"content":"hello discord",
					"guild_id":"guild-1",
					"author":{"id":"user-1","username":"alice","bot":false},
					"message_reference":{"message_id":"reply-1"}
				},
				{
					"id":"1",
					"channel_id":"chan-1",
					"content":"voice caption",
					"guild_id":"guild-1",
					"author":{"id":"user-2","username":"bob","bot":false},
					"attachments":[{"filename":"voice.ogg","proxy_url":"https://audio.example/1"}]
				}
			]`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/messages"):
			sentBodies = append(sentBodies, readJSONBody(t, r))
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	adapter := NewDiscordAdapter(config.DiscordChannelConfig{
		Enabled:        true,
		BotToken:       "token",
		DefaultChannel: "default-chan",
		APIBaseURL:     server.URL,
	}, func(eventType string, sessionID string, payload map[string]any) {
		events = append(events, eventType)
	})
	adapter.client = server.Client()

	err := adapter.pollOnce(context.Background(), func(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error) {
		calls = append(calls, map[string]string{
			"message":      message,
			"message_type": meta["message_type"],
			"audio_url":    meta["audio_url"],
			"channel_type": meta["channel_type"],
			"is_group":     meta["is_group"],
		})
		if meta["message_type"] == "voice_note" {
			return "session-voice", "voice response", nil
		}
		return "session-text", "text response", nil
	})
	if err != nil {
		t.Fatalf("pollOnce returned error: %v", err)
	}

	if len(calls) != 2 {
		t.Fatalf("expected 2 handler calls, got %d", len(calls))
	}
	if calls[0]["message_type"] != "voice_note" || calls[0]["audio_url"] != "https://audio.example/1" {
		t.Fatalf("unexpected voice call: %+v", calls[0])
	}
	if calls[1]["message"] != "hello discord" {
		t.Fatalf("unexpected text call: %+v", calls[1])
	}
	if calls[1]["channel_type"] != "guild" || calls[1]["is_group"] != "true" {
		t.Fatalf("unexpected channel metadata: %+v", calls[1])
	}
	if len(sentBodies) != 2 {
		t.Fatalf("expected 2 outbound sends, got %d", len(sentBodies))
	}
	if sentBodies[0]["content"] != "voice response" || sentBodies[1]["content"] != "text response" {
		t.Fatalf("unexpected outbound messages: %+v", sentBodies)
	}
	if adapter.Status().LastActivity.IsZero() {
		t.Fatal("expected pollOnce to mark adapter activity")
	}
	if len(events) < 2 {
		t.Fatalf("expected append event callbacks, got %v", events)
	}
}

func TestDiscordAdapterPollOnceStream(t *testing.T) {
	var (
		postBodies  []map[string]any
		patchBodies []map[string]any
		events      []string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/messages"):
			_, _ = w.Write([]byte(`[
				{
					"id":"1",
					"channel_id":"chan-1",
					"content":"stream hello",
					"guild_id":"guild-1",
					"author":{"id":"user-1","username":"alice","bot":false}
				}
			]`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/messages"):
			postBodies = append(postBodies, readJSONBody(t, r))
			_, _ = w.Write([]byte(`{"id":"out-1"}`))
		case r.Method == http.MethodPatch:
			patchBodies = append(patchBodies, readJSONBody(t, r))
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	adapter := NewDiscordAdapter(config.DiscordChannelConfig{
		Enabled:        true,
		BotToken:       "token",
		DefaultChannel: "default-chan",
		APIBaseURL:     server.URL,
	}, func(eventType string, sessionID string, payload map[string]any) {
		events = append(events, eventType)
	})
	adapter.client = server.Client()

	err := adapter.pollOnceStream(context.Background(), func(ctx context.Context, sessionID string, message string, meta map[string]string, onChunk func(chunk string) error) (string, error) {
		if message != "stream hello" {
			t.Fatalf("unexpected stream message: %q", message)
		}
		if err := onChunk("part-1"); err != nil {
			return "", err
		}
		if err := onChunk("part-2"); err != nil {
			return "", err
		}
		return "session-stream", nil
	})
	if err != nil {
		t.Fatalf("pollOnceStream returned error: %v", err)
	}

	if len(postBodies) != 1 || postBodies[0]["content"] != "\u200b" {
		t.Fatalf("expected placeholder message to be posted first, got %+v", postBodies)
	}
	if len(patchBodies) == 0 || patchBodies[len(patchBodies)-1]["content"] != "part-1part-2" {
		t.Fatalf("expected final patch content, got %+v", patchBodies)
	}
	if len(events) == 0 {
		t.Fatal("expected streaming poll to append events")
	}
}

func TestDiscordAdapterRunGatewayWS(t *testing.T) {
	var upgrader websocket.Upgrader
	var receivedMessages []map[string]string

	var cancel context.CancelFunc
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/messages") {
			t.Fatalf("unexpected discord api request: %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{}`))
		if cancel != nil {
			go func() {
				time.Sleep(20 * time.Millisecond)
				cancel()
			}()
		}
	}))
	defer apiServer.Close()

	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade websocket: %v", err)
		}
		defer conn.Close()

		if err := conn.WriteJSON(map[string]any{
			"op": 10,
			"d":  map[string]any{"heartbeat_interval": 10},
		}); err != nil {
			t.Fatalf("write hello packet: %v", err)
		}

		for i := 0; i < 2; i++ {
			if _, _, err := conn.ReadMessage(); err != nil {
				t.Fatalf("read client packet: %v", err)
			}
		}

		messagePayload := map[string]any{
			"id":         "1",
			"content":    "gateway hello",
			"channel_id": "chan-1",
			"guild_id":   "guild-1",
			"author": map[string]any{
				"id":       "user-1",
				"username": "alice",
				"bot":      false,
			},
		}
		raw, _ := json.Marshal(messagePayload)
		if err := conn.WriteJSON(map[string]any{
			"op": 0,
			"t":  "MESSAGE_CREATE",
			"d":  json.RawMessage(raw),
		}); err != nil {
			t.Fatalf("write gateway event: %v", err)
		}

		time.Sleep(20 * time.Millisecond)
	}))
	defer wsServer.Close()

	wsURL := "ws" + strings.TrimPrefix(wsServer.URL, "http")
	adapter := NewDiscordAdapter(config.DiscordChannelConfig{
		Enabled:        true,
		BotToken:       "token",
		DefaultChannel: "default-chan",
		APIBaseURL:     apiServer.URL,
	}, nil)
	adapter.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() == "https://discord.com/api/gateway/bot" {
				return jsonResponse(http.StatusOK, map[string]any{"url": wsURL}), nil
			}
			return apiServer.Client().Transport.RoundTrip(req)
		}),
	}

	ctx, cancelFn := context.WithCancel(context.Background())
	cancel = cancelFn
	err := adapter.runGatewayWS(ctx, func(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error) {
		receivedMessages = append(receivedMessages, map[string]string{
			"message": message,
			"user":    meta["username"],
		})
		return "session-1", "ok", nil
	})
	if err != nil && len(receivedMessages) == 0 {
		t.Fatalf("runGatewayWS returned error: %v", err)
	}
	if len(receivedMessages) != 1 || receivedMessages[0]["message"] != "gateway hello" || receivedMessages[0]["user"] != "alice" {
		t.Fatalf("unexpected gateway handler calls: %+v", receivedMessages)
	}
}

func TestDiscordAdapterRunGatewayWSStream(t *testing.T) {
	var upgrader websocket.Upgrader
	var cancel context.CancelFunc
	var patchBodies []map[string]any

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			_, _ = w.Write([]byte(`{"id":"stream-msg"}`))
		case http.MethodPatch:
			patchBodies = append(patchBodies, readJSONBody(t, r))
			_, _ = w.Write([]byte(`{}`))
			if cancel != nil {
				go func() {
					time.Sleep(20 * time.Millisecond)
					cancel()
				}()
			}
		default:
			t.Fatalf("unexpected discord api request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer apiServer.Close()

	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade websocket: %v", err)
		}
		defer conn.Close()

		if err := conn.WriteJSON(map[string]any{"op": 10, "d": map[string]any{"heartbeat_interval": 10}}); err != nil {
			t.Fatalf("write hello packet: %v", err)
		}
		for i := 0; i < 2; i++ {
			if _, _, err := conn.ReadMessage(); err != nil {
				t.Fatalf("read client packet: %v", err)
			}
		}

		messagePayload := map[string]any{
			"id":         "1",
			"content":    "gateway stream",
			"channel_id": "chan-1",
			"guild_id":   "guild-1",
			"author": map[string]any{
				"id":       "user-1",
				"username": "alice",
				"bot":      false,
			},
		}
		raw, _ := json.Marshal(messagePayload)
		if err := conn.WriteJSON(map[string]any{"op": 0, "t": "MESSAGE_CREATE", "d": json.RawMessage(raw)}); err != nil {
			t.Fatalf("write gateway event: %v", err)
		}

		time.Sleep(20 * time.Millisecond)
	}))
	defer wsServer.Close()

	wsURL := "ws" + strings.TrimPrefix(wsServer.URL, "http")
	adapter := NewDiscordAdapter(config.DiscordChannelConfig{
		Enabled:        true,
		BotToken:       "token",
		DefaultChannel: "default-chan",
		APIBaseURL:     apiServer.URL,
	}, nil)
	adapter.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() == "https://discord.com/api/gateway/bot" {
				return jsonResponse(http.StatusOK, map[string]any{"url": wsURL}), nil
			}
			return apiServer.Client().Transport.RoundTrip(req)
		}),
	}

	ctx, cancelFn := context.WithCancel(context.Background())
	cancel = cancelFn
	err := adapter.runGatewayWSStream(ctx, func(ctx context.Context, sessionID string, message string, meta map[string]string, onChunk func(chunk string) error) (string, error) {
		if message != "gateway stream" {
			t.Fatalf("unexpected gateway stream message: %q", message)
		}
		_ = onChunk("chunk-1")
		_ = onChunk("chunk-2")
		return "session-stream", nil
	})
	if err != nil && len(patchBodies) == 0 {
		t.Fatalf("runGatewayWSStream returned error: %v", err)
	}
	if len(patchBodies) == 0 || patchBodies[len(patchBodies)-1]["content"] != "chunk-1chunk-2" {
		t.Fatalf("unexpected gateway stream patches: %+v", patchBodies)
	}
}

func TestDiscordAdapterStreamingFallback(t *testing.T) {
	var postBodies []map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/messages") {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		postBodies = append(postBodies, readJSONBody(t, r))
		if len(postBodies) == 1 {
			_, _ = w.Write([]byte(`{"id":""}`))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	adapter := NewDiscordAdapter(config.DiscordChannelConfig{
		Enabled:        true,
		BotToken:       "token",
		DefaultChannel: "default-chan",
		APIBaseURL:     server.URL,
	}, nil)
	adapter.client = server.Client()

	err := adapter.sendStreamingMessage(context.Background(), "chan-1", "", func(onChunk func(chunk string) error) error {
		if err := onChunk("hello"); err != nil {
			return err
		}
		if err := onChunk(" discord"); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("sendStreamingMessage returned error: %v", err)
	}

	if len(postBodies) != 2 {
		t.Fatalf("expected placeholder and fallback send, got %d", len(postBodies))
	}
	if postBodies[1]["content"] != "hello discord" {
		t.Fatalf("unexpected fallback final message: %+v", postBodies[1])
	}
}

func TestSlackHelpersAndMetadata(t *testing.T) {
	if got, isGroup := slackConversationMetadata("D123"); got != "dm" || isGroup {
		t.Fatalf("expected dm metadata, got %q %v", got, isGroup)
	}
	if got, isGroup := slackConversationMetadata("C123"); got != "group" || !isGroup {
		t.Fatalf("expected group metadata, got %q %v", got, isGroup)
	}
	if got := boolString(true); got != "true" {
		t.Fatalf("expected true string, got %q", got)
	}
	if got := boolString(false); got != "false" {
		t.Fatalf("expected false string, got %q", got)
	}
	if !slackTSLessOrEqual("1710000000.000100", "1710000000.000100") {
		t.Fatal("expected equal slack timestamps to compare as less-or-equal")
	}
	if !slackTSLessOrEqual("1710000000.000099", "1710000000.000100") {
		t.Fatal("expected older slack timestamp to compare as less-or-equal")
	}
	if slackTSLessOrEqual("1710000000.000101", "1710000000.000100") {
		t.Fatal("expected newer slack timestamp to compare as greater")
	}
}

func TestSlackAdapterSendMethods(t *testing.T) {
	var bodies []map[string]any
	adapter := NewSlackAdapter(config.SlackChannelConfig{
		Enabled:        true,
		BotToken:       "token",
		DefaultChannel: "C123",
	}, nil)
	adapter.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			bodies = append(bodies, readJSONBody(t, req))
			switch req.URL.Path {
			case "/api/chat.postMessage":
				return jsonResponse(http.StatusOK, map[string]any{"ok": true, "ts": "ts-1"}), nil
			case "/api/chat.update":
				return jsonResponse(http.StatusOK, map[string]any{"ok": true}), nil
			default:
				return nil, fmt.Errorf("unexpected request path: %s", req.URL.Path)
			}
		}),
	}

	if err := adapter.sendMessage(context.Background(), "hello", "thread-1"); err != nil {
		t.Fatalf("sendMessage returned error: %v", err)
	}
	ts, err := adapter.sendMessageWithResult(context.Background(), "hello again", "")
	if err != nil {
		t.Fatalf("sendMessageWithResult returned error: %v", err)
	}
	if ts != "ts-1" {
		t.Fatalf("expected sent ts, got %q", ts)
	}
	if err := adapter.editMessage(context.Background(), "ts-1", "edited"); err != nil {
		t.Fatalf("editMessage returned error: %v", err)
	}

	if len(bodies) != 3 {
		t.Fatalf("expected 3 outbound requests, got %d", len(bodies))
	}
	if bodies[0]["thread_ts"] != "thread-1" {
		t.Fatalf("expected thread ts in sendMessage body, got %+v", bodies[0])
	}
	if bodies[2]["text"] != "edited" {
		t.Fatalf("expected edit payload, got %+v", bodies[2])
	}
}

func TestSlackAdapterPollOnceAndStream(t *testing.T) {
	var (
		mu         sync.Mutex
		postBodies []map[string]any
		calls      []map[string]string
	)

	adapter := NewSlackAdapter(config.SlackChannelConfig{
		Enabled:        true,
		BotToken:       "token",
		DefaultChannel: "C123",
		StreamInterval: 1,
	}, func(eventType string, sessionID string, payload map[string]any) {})

	var historyCalls int
	adapter.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/api/conversations.history":
				historyCalls++
				if historyCalls == 1 {
					return jsonResponse(http.StatusOK, map[string]any{
						"ok": true,
						"messages": []map[string]any{
							{"text": "text message", "ts": "2", "thread_ts": "thread-2", "user": "U2"},
							{
								"text": "voice caption",
								"ts":   "1",
								"user": "U1",
								"files": []map[string]any{
									{"mimetype": "audio/mpeg", "url_private": "https://audio.example/slack.mp3", "title": "voice"},
								},
							},
						},
					}), nil
				}
				return jsonResponse(http.StatusOK, map[string]any{
					"ok": true,
					"messages": []map[string]any{
						{"text": "stream text", "ts": "3", "thread_ts": "thread-3", "user": "U3"},
					},
				}), nil
			case "/api/chat.postMessage":
				mu.Lock()
				postBodies = append(postBodies, readJSONBody(t, req))
				current := len(postBodies)
				mu.Unlock()
				if current == 3 {
					return jsonResponse(http.StatusOK, map[string]any{"ok": true, "ts": "stream-ts"}), nil
				}
				return jsonResponse(http.StatusOK, map[string]any{"ok": true, "ts": fmt.Sprintf("ts-%d", current)}), nil
			case "/api/chat.update":
				mu.Lock()
				postBodies = append(postBodies, readJSONBody(t, req))
				mu.Unlock()
				return jsonResponse(http.StatusOK, map[string]any{"ok": true}), nil
			default:
				return nil, fmt.Errorf("unexpected request path: %s", req.URL.Path)
			}
		}),
	}

	err := adapter.pollOnce(context.Background(), func(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error) {
		calls = append(calls, map[string]string{
			"message":      message,
			"message_type": meta["message_type"],
			"audio_url":    meta["audio_url"],
			"channel_type": meta["channel_type"],
			"is_group":     meta["is_group"],
		})
		if meta["message_type"] == "voice_note" {
			return "session-voice", "voice reply", nil
		}
		return "session-text", "text reply", nil
	})
	if err != nil {
		t.Fatalf("pollOnce returned error: %v", err)
	}

	err = adapter.pollOnceStream(context.Background(), func(ctx context.Context, sessionID string, message string, meta map[string]string, onChunk func(chunk string) error) (string, error) {
		if err := onChunk("chunk-a"); err != nil {
			return "", err
		}
		if err := onChunk("chunk-b"); err != nil {
			return "", err
		}
		return "session-stream", nil
	})
	if err != nil {
		t.Fatalf("pollOnceStream returned error: %v", err)
	}

	if len(calls) != 2 {
		t.Fatalf("expected 2 pollOnce handler calls, got %d", len(calls))
	}
	if calls[0]["message_type"] != "voice_note" || calls[0]["audio_url"] != "https://audio.example/slack.mp3" {
		t.Fatalf("unexpected slack voice call: %+v", calls[0])
	}
	if calls[1]["message"] != "text message" || calls[1]["channel_type"] != "group" || calls[1]["is_group"] != "true" {
		t.Fatalf("unexpected slack text call: %+v", calls[1])
	}

	mu.Lock()
	defer mu.Unlock()
	if len(postBodies) < 4 {
		t.Fatalf("expected outbound slack messages and updates, got %d", len(postBodies))
	}
	if postBodies[0]["text"] != "voice reply" || postBodies[1]["text"] != "text reply" {
		t.Fatalf("unexpected first slack outbound bodies: %+v", postBodies[:2])
	}
	last := postBodies[len(postBodies)-1]
	if last["text"] != "chunk-achunk-b" {
		t.Fatalf("unexpected final slack streamed body: %+v", last)
	}
}

func TestSlackAdapterPollOnceSkipsOlderMessages(t *testing.T) {
	var calls []string

	adapter := NewSlackAdapter(config.SlackChannelConfig{
		Enabled:        true,
		BotToken:       "token",
		DefaultChannel: "C123",
	}, nil)

	historyCalls := 0
	adapter.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/api/conversations.history":
				historyCalls++
				if historyCalls == 1 {
					return jsonResponse(http.StatusOK, map[string]any{
						"ok": true,
						"messages": []map[string]any{
							{"text": "second", "ts": "1710000000.000200", "user": "U2"},
							{"text": "first", "ts": "1710000000.000100", "user": "U1"},
						},
					}), nil
				}
				return jsonResponse(http.StatusOK, map[string]any{
					"ok": true,
					"messages": []map[string]any{
						{"text": "third", "ts": "1710000000.000300", "user": "U3"},
						{"text": "second", "ts": "1710000000.000200", "user": "U2"},
						{"text": "first", "ts": "1710000000.000100", "user": "U1"},
					},
				}), nil
			case "/api/chat.postMessage":
				return jsonResponse(http.StatusOK, map[string]any{"ok": true, "ts": "reply-ts"}), nil
			default:
				return nil, fmt.Errorf("unexpected request path: %s", req.URL.Path)
			}
		}),
	}

	handle := func(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error) {
		calls = append(calls, message)
		return "session", "ok", nil
	}

	if err := adapter.pollOnce(context.Background(), handle); err != nil {
		t.Fatalf("first pollOnce returned error: %v", err)
	}
	if err := adapter.pollOnce(context.Background(), handle); err != nil {
		t.Fatalf("second pollOnce returned error: %v", err)
	}

	if len(calls) != 3 {
		t.Fatalf("expected only new slack message to be processed on second poll, got %v", calls)
	}
	if calls[2] != "third" {
		t.Fatalf("expected final processed message to be third, got %v", calls)
	}
}

func TestSlackAdapterErrorBranchesAndFallback(t *testing.T) {
	adapter := NewSlackAdapter(config.SlackChannelConfig{
		Enabled:        true,
		BotToken:       "token",
		DefaultChannel: "D123",
	}, nil)

	postCalls := 0
	adapter.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/api/conversations.history":
				return jsonResponse(http.StatusOK, map[string]any{"ok": false, "error": "denied"}), nil
			case "/api/chat.postMessage":
				postCalls++
				if postCalls == 1 {
					return jsonResponse(http.StatusOK, map[string]any{"ok": true, "ts": ""}), nil
				}
				return jsonResponse(http.StatusOK, map[string]any{"ok": true, "ts": "ts-2"}), nil
			case "/api/chat.update":
				return jsonResponse(http.StatusOK, map[string]any{"ok": false, "error": "edit_failed"}), nil
			default:
				return nil, fmt.Errorf("unexpected request path: %s", req.URL.Path)
			}
		}),
	}

	if err := adapter.pollOnce(context.Background(), func(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error) {
		return "", "", nil
	}); err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("expected slack history error, got %v", err)
	}

	if err := adapter.sendStreamingMessage(context.Background(), "thread-1", func(onChunk func(chunk string) error) error {
		if err := onChunk("fallback"); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatalf("expected slack streaming fallback to succeed, got %v", err)
	}

	if err := adapter.editMessage(context.Background(), "ts-1", "hello"); err == nil || !strings.Contains(err.Error(), "edit_failed") {
		t.Fatalf("expected slack update failure, got %v", err)
	}
}

func TestSlackAdapterSendStreamingMessageReturnsEditError(t *testing.T) {
	adapter := NewSlackAdapter(config.SlackChannelConfig{
		Enabled:        true,
		BotToken:       "token",
		DefaultChannel: "C123",
		StreamInterval: 1,
	}, nil)

	postCalls := 0
	adapter.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/api/chat.postMessage":
				postCalls++
				return jsonResponse(http.StatusOK, map[string]any{"ok": true, "ts": fmt.Sprintf("ts-%d", postCalls)}), nil
			case "/api/chat.update":
				return jsonResponse(http.StatusOK, map[string]any{"ok": false, "error": "edit_failed"}), nil
			default:
				return nil, fmt.Errorf("unexpected request path: %s", req.URL.Path)
			}
		}),
	}

	err := adapter.sendStreamingMessage(context.Background(), "thread-1", func(onChunk func(chunk string) error) error {
		return onChunk("hello")
	})
	if err == nil || !strings.Contains(err.Error(), "edit_failed") {
		t.Fatalf("expected slack streaming edit failure, got %v", err)
	}
}

func TestAdaptersRunAndRunStream(t *testing.T) {
	t.Run("discord run", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/messages"):
				_, _ = w.Write([]byte(`[
					{"id":"1","channel_id":"chan-1","content":"hello","author":{"id":"user-1","username":"alice","bot":false}}
				]`))
			case r.Method == http.MethodPost:
				_, _ = w.Write([]byte(`{}`))
			default:
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
			}
		}))
		defer server.Close()

		adapter := NewDiscordAdapter(config.DiscordChannelConfig{
			Enabled:        true,
			BotToken:       "token",
			DefaultChannel: "default-chan",
			APIBaseURL:     server.URL,
			PollEvery:      1,
		}, nil)
		adapter.client = server.Client()

		ctx, cancel := context.WithCancel(context.Background())
		err := adapter.Run(ctx, func(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error) {
			cancel()
			return "session-1", "ok", nil
		})
		if err != nil {
			t.Fatalf("discord Run returned error: %v", err)
		}
	})

	t.Run("discord run stream", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/messages"):
				_, _ = w.Write([]byte(`[
					{"id":"1","channel_id":"chan-1","content":"hello","author":{"id":"user-1","username":"alice","bot":false}}
				]`))
			case r.Method == http.MethodPost:
				_, _ = w.Write([]byte(`{"id":"stream-1"}`))
			case r.Method == http.MethodPatch:
				_, _ = w.Write([]byte(`{}`))
			default:
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
			}
		}))
		defer server.Close()

		adapter := NewDiscordAdapter(config.DiscordChannelConfig{
			Enabled:        true,
			BotToken:       "token",
			DefaultChannel: "default-chan",
			APIBaseURL:     server.URL,
			PollEvery:      1,
		}, nil)
		adapter.client = server.Client()

		ctx, cancel := context.WithCancel(context.Background())
		err := adapter.RunStream(ctx, func(ctx context.Context, sessionID string, message string, meta map[string]string, onChunk func(chunk string) error) (string, error) {
			cancel()
			_ = onChunk("ok")
			return "session-1", nil
		})
		if err != nil {
			t.Fatalf("discord RunStream returned error: %v", err)
		}
	})

	t.Run("slack run", func(t *testing.T) {
		adapter := NewSlackAdapter(config.SlackChannelConfig{
			Enabled:        true,
			BotToken:       "token",
			DefaultChannel: "C123",
			PollEvery:      1,
		}, nil)
		adapter.client = &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/api/conversations.history":
					return jsonResponse(http.StatusOK, map[string]any{
						"ok": true,
						"messages": []map[string]any{
							{"text": "hello", "ts": "1", "user": "U1"},
						},
					}), nil
				case "/api/chat.postMessage":
					return jsonResponse(http.StatusOK, map[string]any{"ok": true, "ts": "1"}), nil
				default:
					return nil, fmt.Errorf("unexpected request path: %s", req.URL.Path)
				}
			}),
		}

		ctx, cancel := context.WithCancel(context.Background())
		err := adapter.Run(ctx, func(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error) {
			cancel()
			return "session-1", "ok", nil
		})
		if err != nil {
			t.Fatalf("slack Run returned error: %v", err)
		}
	})

	t.Run("slack run stream", func(t *testing.T) {
		adapter := NewSlackAdapter(config.SlackChannelConfig{
			Enabled:        true,
			BotToken:       "token",
			DefaultChannel: "C123",
			PollEvery:      1,
		}, nil)
		adapter.client = &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/api/conversations.history":
					return jsonResponse(http.StatusOK, map[string]any{
						"ok": true,
						"messages": []map[string]any{
							{"text": "hello", "ts": "1", "thread_ts": "thread-1", "user": "U1"},
						},
					}), nil
				case "/api/chat.postMessage":
					return jsonResponse(http.StatusOK, map[string]any{"ok": true, "ts": "tmp-1"}), nil
				case "/api/chat.update":
					return jsonResponse(http.StatusOK, map[string]any{"ok": true}), nil
				default:
					return nil, fmt.Errorf("unexpected request path: %s", req.URL.Path)
				}
			}),
		}

		ctx, cancel := context.WithCancel(context.Background())
		err := adapter.RunStream(ctx, func(ctx context.Context, sessionID string, message string, meta map[string]string, onChunk func(chunk string) error) (string, error) {
			cancel()
			_ = onChunk("ok")
			return "session-1", nil
		})
		if err != nil {
			t.Fatalf("slack RunStream returned error: %v", err)
		}
	})
}
