package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/config"
	inputlayer "github.com/1024XEngineer/anyclaw/pkg/input"
)

type SignalAdapter struct {
	base        inputlayer.BaseAdapter
	config      config.SignalChannelConfig
	client      *http.Client
	appendEvent func(eventType string, sessionID string, payload map[string]any)
	latestTS    int64
	processed   map[string]time.Time
}

func NewSignalAdapter(cfg config.SignalChannelConfig, appendEvent func(eventType string, sessionID string, payload map[string]any)) *SignalAdapter {
	return &SignalAdapter{
		base:        inputlayer.NewBaseAdapter("signal", cfg.Enabled && cfg.BaseURL != ""),
		config:      cfg,
		client:      &http.Client{Timeout: 20 * time.Second},
		appendEvent: appendEvent,
		processed:   make(map[string]time.Time),
	}
}

func (a *SignalAdapter) Name() string { return "signal" }

func (a *SignalAdapter) Enabled() bool {
	return a.config.Enabled && strings.TrimSpace(a.config.BaseURL) != "" && strings.TrimSpace(a.config.Number) != ""
}

func (a *SignalAdapter) Status() inputlayer.Status {
	status := a.base.Status()
	status.Enabled = a.Enabled()
	return status
}

func (a *SignalAdapter) Run(ctx context.Context, handle inputlayer.InboundHandler) error {
	a.base.SetRunning(true)
	defer a.base.SetRunning(false)
	interval := time.Duration(a.config.PollEvery) * time.Second
	if interval <= 0 {
		interval = 3 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := a.pollOnce(ctx, handle); err != nil {
			a.base.SetError(err)
			a.append("channel.signal.error", "", map[string]any{"error": err.Error()})
		} else {
			a.base.SetError(nil)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (a *SignalAdapter) pollOnce(ctx context.Context, handle inputlayer.InboundHandler) error {
	baseURL := strings.TrimRight(strings.TrimSpace(a.config.BaseURL), "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/v1/receive/"+url.PathEscape(a.config.Number), nil)
	if err != nil {
		return err
	}
	if strings.TrimSpace(a.config.BearerToken) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(a.config.BearerToken))
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("signal receive failed: %s", resp.Status)
	}
	var payload []struct {
		Envelope struct {
			Timestamp  int64  `json:"timestamp"`
			Source     string `json:"source"`
			SourceName string `json:"sourceName"`
			GroupInfo  struct {
				GroupID string `json:"groupId"`
			} `json:"groupInfo"`
			DataMessage struct {
				Message     string `json:"message"`
				Attachments []struct {
					ContentType string `json:"contentType"`
					Filename    string `json:"filename"`
				} `json:"attachments"`
			} `json:"dataMessage"`
		} `json:"envelope"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return err
	}
	for _, item := range payload {
		threadID := strings.TrimSpace(item.Envelope.GroupInfo.GroupID)
		messageID := signalMessageID(
			item.Envelope.Source,
			threadID,
			item.Envelope.Timestamp,
			item.Envelope.DataMessage.Message,
			item.Envelope.DataMessage.Attachments,
		)
		if item.Envelope.Timestamp < a.latestTS || a.hasSeen(messageID) {
			continue
		}

		msg := strings.TrimSpace(item.Envelope.DataMessage.Message)
		replyTarget := item.Envelope.Source
		if threadID != "" {
			replyTarget = threadID
		}

		meta := map[string]string{
			"channel":      "signal",
			"user_id":      item.Envelope.Source,
			"username":     item.Envelope.SourceName,
			"reply_target": replyTarget,
			"thread_id":    threadID,
			"message_id":   messageID,
			"sender":       item.Envelope.SourceName,
			"channel_type": signalChannelType(threadID),
			"is_group":     boolString(threadID != ""),
		}

		audioURL, audioMIME, hasAudio := a.findAudioAttachment(item.Envelope.DataMessage.Attachments)
		if hasAudio {
			meta["message_type"] = "voice_note"
			meta["audio_mime"] = audioMIME
			if audioURL != "" {
				meta["audio_url"] = audioURL
			}
			if msg != "" {
				meta["caption"] = msg
			}

			sessionID, response, err := handle(ctx, "", audioURL, meta)
			if err != nil {
				return err
			}
			if err := a.sendMessage(ctx, replyTarget, response); err != nil {
				return err
			}
			a.advanceTimestamp(item.Envelope.Timestamp)
			a.markSeen(messageID)
			a.base.MarkActivity()
			a.append("channel.signal.voice", sessionID, map[string]any{
				"source":       item.Envelope.Source,
				"source_name":  item.Envelope.SourceName,
				"group_id":     threadID,
				"message_type": "voice_note",
				"audio_url":    audioURL,
				"audio_mime":   audioMIME,
			})
			continue
		}

		if msg == "" {
			a.advanceTimestamp(item.Envelope.Timestamp)
			a.markSeen(messageID)
			continue
		}

		if len(item.Envelope.DataMessage.Attachments) > 0 {
			meta["attachment_count"] = fmt.Sprintf("%d", len(item.Envelope.DataMessage.Attachments))
		}
		sessionID, response, err := handle(ctx, "", msg, meta)
		if err != nil {
			return err
		}
		if err := a.sendMessage(ctx, replyTarget, response); err != nil {
			return err
		}
		a.advanceTimestamp(item.Envelope.Timestamp)
		a.markSeen(messageID)
		a.base.MarkActivity()
		a.append("channel.signal.message", sessionID, map[string]any{
			"source":      item.Envelope.Source,
			"source_name": item.Envelope.SourceName,
			"group_id":    threadID,
			"attachments": len(item.Envelope.DataMessage.Attachments),
			"text":        msg,
		})
	}
	return nil
}

func signalChannelType(threadID string) string {
	if strings.TrimSpace(threadID) != "" {
		return "group"
	}
	return "private"
}

func (a *SignalAdapter) sendMessage(ctx context.Context, recipient string, text string) error {
	recipient = strings.TrimSpace(recipient)
	if recipient == "" {
		recipient = strings.TrimSpace(a.config.DefaultRecipient)
	}
	if recipient == "" {
		return nil
	}
	body, _ := json.Marshal(map[string]any{
		"message":    text,
		"number":     a.config.Number,
		"recipients": []string{recipient},
	})
	baseURL := strings.TrimRight(strings.TrimSpace(a.config.BaseURL), "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v2/send", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(a.config.BearerToken) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(a.config.BearerToken))
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("signal send failed: %s", resp.Status)
	}
	return nil
}

func (a *SignalAdapter) append(eventType string, sessionID string, payload map[string]any) {
	if a.appendEvent != nil {
		a.appendEvent(eventType, sessionID, payload)
	}
}

func (a *SignalAdapter) advanceTimestamp(ts int64) {
	if ts > a.latestTS {
		a.latestTS = ts
	}
}

func (a *SignalAdapter) pruneSeen() {
	for key, ts := range a.processed {
		if time.Since(ts) > 30*time.Minute {
			delete(a.processed, key)
		}
	}
}

func (a *SignalAdapter) hasSeen(id string) bool {
	a.pruneSeen()
	_, ok := a.processed[id]
	return ok
}

func (a *SignalAdapter) markSeen(id string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	a.pruneSeen()
	a.processed[id] = time.Now().UTC()
}

func (a *SignalAdapter) seen(id string) bool {
	if a.hasSeen(id) {
		return true
	}
	a.markSeen(id)
	return false
}

func signalMessageID(source string, threadID string, timestamp int64, message string, attachments []struct {
	ContentType string `json:"contentType"`
	Filename    string `json:"filename"`
}) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(source))
	b.WriteString("|")
	b.WriteString(strings.TrimSpace(threadID))
	b.WriteString("|")
	b.WriteString(fmt.Sprintf("%d", timestamp))
	b.WriteString("|")
	b.WriteString(strings.TrimSpace(message))
	for _, attachment := range attachments {
		b.WriteString("|")
		b.WriteString(strings.TrimSpace(attachment.ContentType))
		b.WriteString(":")
		b.WriteString(strings.TrimSpace(attachment.Filename))
	}
	return b.String()
}

func (a *SignalAdapter) findAudioAttachment(attachments []struct {
	ContentType string `json:"contentType"`
	Filename    string `json:"filename"`
}) (string, string, bool) {
	for _, att := range attachments {
		mime := strings.ToLower(att.ContentType)
		fn := strings.ToLower(att.Filename)
		if strings.HasPrefix(mime, "audio/") {
			return strings.TrimSpace(att.Filename), att.ContentType, true
		}
		if strings.HasSuffix(fn, ".ogg") || strings.HasSuffix(fn, ".mp3") || strings.HasSuffix(fn, ".wav") || strings.HasSuffix(fn, ".flac") || strings.HasSuffix(fn, ".m4a") || strings.HasSuffix(fn, ".webm") {
			mimeType := "audio/unknown"
			switch {
			case strings.HasSuffix(fn, ".ogg"):
				mimeType = "audio/ogg"
			case strings.HasSuffix(fn, ".mp3"):
				mimeType = "audio/mpeg"
			case strings.HasSuffix(fn, ".wav"):
				mimeType = "audio/wav"
			case strings.HasSuffix(fn, ".flac"):
				mimeType = "audio/flac"
			case strings.HasSuffix(fn, ".m4a"):
				mimeType = "audio/mp4"
			case strings.HasSuffix(fn, ".webm"):
				mimeType = "audio/webm"
			}
			return strings.TrimSpace(att.Filename), mimeType, true
		}
	}
	return "", "", false
}
