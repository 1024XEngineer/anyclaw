package speech

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAudioSenders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll(%s): %v", r.URL.Path, err)
		}

		switch r.URL.Path {
		case "/botbot-token/sendAudio":
			if !strings.Contains(string(body), "chat-1") {
				t.Fatalf("telegram body missing recipient: %s", string(body))
			}
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":42}}`))
		case "/api/v10/channels/channel-1/messages":
			if got := r.Header.Get("Authorization"); got != "Bot discord-token" {
				t.Fatalf("unexpected discord auth header: %s", got)
			}
			_, _ = w.Write([]byte(`{"id":"discord-msg"}`))
		case "/api/files.upload":
			if got := r.Header.Get("Authorization"); got != "Bearer slack-token" {
				t.Fatalf("unexpected slack auth header: %s", got)
			}
			_, _ = w.Write([]byte(`{"ok":true,"file":{"id":"slack-file","name":"tts.mp3"}}`))
		case "/wa-123/media":
			if got := r.Header.Get("Authorization"); got != "Bearer wa-token" {
				t.Fatalf("unexpected whatsapp auth header: %s", got)
			}
			_, _ = w.Write([]byte(`{"id":"media-1"}`))
		case "/wa-123/messages":
			if got := r.Header.Get("Authorization"); got != "Bearer wa-token" {
				t.Fatalf("unexpected whatsapp send auth header: %s", got)
			}
			_, _ = w.Write([]byte(`{"messages":[{"id":"wa-message"}]}`))
		case "/signal/v2/send":
			if !strings.Contains(string(body), "base64_audio") {
				t.Fatalf("signal body missing base64 audio: %s", string(body))
			}
			_, _ = w.Write([]byte(`{"timestamp":12345}`))
		case "/hook":
			if got := r.Header.Get("X-Test"); got != "ok" {
				t.Fatalf("unexpected webhook header: %s", got)
			}
			_, _ = w.Write([]byte(`hook-id`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	audio := &AudioResult{
		Data:       []byte("audio-bytes"),
		Format:     FormatMP3,
		SampleRate: 24000,
	}

	telegram := NewTelegramAudioSender("bot-token")
	telegram.baseURL = server.URL + "/botbot-token"
	telegram.client = server.Client()
	if _, err := telegram.SendAudio(context.Background(), "telegram", "", audio, "hello"); err == nil {
		t.Fatal("expected telegram sender to require recipient")
	}
	telegramID, err := telegram.SendAudio(context.Background(), "telegram", "chat-1", audio, "hello")
	if err != nil {
		t.Fatalf("Telegram SendAudio: %v", err)
	}
	if telegramID != "42" || !telegram.CanSend("telegram") {
		t.Fatalf("unexpected telegram result: id=%s", telegramID)
	}

	discord := NewDiscordAudioSender("discord-token")
	discord.apiBase = server.URL + "/api/v10"
	discord.client = server.Client()
	if _, err := discord.SendAudio(context.Background(), "discord", "", audio, "hello"); err == nil {
		t.Fatal("expected discord sender to require recipient")
	}
	discordID, err := discord.SendAudio(context.Background(), "discord", "channel-1", audio, "hello")
	if err != nil {
		t.Fatalf("Discord SendAudio: %v", err)
	}
	if discordID != "discord-msg" || !discord.CanSend("discord") {
		t.Fatalf("unexpected discord result: id=%s", discordID)
	}

	slack := NewSlackAudioSender("slack-token")
	slack.baseURL = server.URL + "/api"
	slack.client = server.Client()
	if _, err := slack.SendAudio(context.Background(), "slack", "", audio, "hello"); err == nil {
		t.Fatal("expected slack sender to require recipient")
	}
	slackID, err := slack.SendAudio(context.Background(), "slack", "channel-1", audio, "hello")
	if err != nil {
		t.Fatalf("Slack SendAudio: %v", err)
	}
	if slackID != "slack-file" || !slack.CanSend("slack") {
		t.Fatalf("unexpected slack result: id=%s", slackID)
	}

	whatsApp := NewWhatsAppAudioSender("wa-123", "wa-token")
	whatsApp.baseURL = server.URL
	whatsApp.client = server.Client()
	if _, err := whatsApp.SendAudio(context.Background(), "whatsapp", "", audio, "hello"); err == nil {
		t.Fatal("expected whatsapp sender to require recipient")
	}
	waID, err := whatsApp.SendAudio(context.Background(), "whatsapp", "13800138000", audio, "hello")
	if err != nil {
		t.Fatalf("WhatsApp SendAudio: %v", err)
	}
	if waID != "wa-message" || !whatsApp.CanSend("whatsapp") {
		t.Fatalf("unexpected whatsapp result: id=%s", waID)
	}

	signal := NewSignalAudioSender(server.URL+"/signal", "+10086")
	signal.client = server.Client()
	if _, err := signal.SendAudio(context.Background(), "signal", "", audio, "hello"); err == nil {
		t.Fatal("expected signal sender to require recipient")
	}
	signalID, err := signal.SendAudio(context.Background(), "signal", "+10010", audio, "hello")
	if err != nil {
		t.Fatalf("Signal SendAudio: %v", err)
	}
	if signalID != "12345" || !signal.CanSend("signal") {
		t.Fatalf("unexpected signal result: id=%s", signalID)
	}

	webhook := NewGenericWebhookAudioSender(server.URL+"/hook", map[string]string{"X-Test": "ok"})
	webhook.client = server.Client()
	webhookID, err := webhook.SendAudio(context.Background(), "custom", "recipient-1", audio, "hello")
	if err != nil {
		t.Fatalf("GenericWebhook SendAudio: %v", err)
	}
	if webhookID != "hook-id" || !webhook.CanSend("anything") {
		t.Fatalf("unexpected webhook result: id=%s", webhookID)
	}
}