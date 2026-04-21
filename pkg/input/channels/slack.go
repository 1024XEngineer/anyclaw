package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/config"
	inputlayer "github.com/1024XEngineer/anyclaw/pkg/input"
)

type SlackAdapter struct {
	base        inputlayer.BaseAdapter
	config      config.SlackChannelConfig
	client      *http.Client
	appendEvent func(eventType string, sessionID string, payload map[string]any)
	latestTS    string
}

func NewSlackAdapter(cfg config.SlackChannelConfig, appendEvent func(eventType string, sessionID string, payload map[string]any)) *SlackAdapter {
	return &SlackAdapter{
		base:        inputlayer.NewBaseAdapter("slack", cfg.Enabled && cfg.BotToken != ""),
		config:      cfg,
		client:      &http.Client{Timeout: 20 * time.Second},
		appendEvent: appendEvent,
	}
}

func (a *SlackAdapter) Name() string {
	return "slack"
}

func (a *SlackAdapter) Enabled() bool {
	return a.config.Enabled && strings.TrimSpace(a.config.BotToken) != "" && strings.TrimSpace(a.config.DefaultChannel) != ""
}

func (a *SlackAdapter) Status() inputlayer.Status {
	status := a.base.Status()
	status.Enabled = a.Enabled()
	return status
}

func (a *SlackAdapter) Run(ctx context.Context, handle inputlayer.InboundHandler) error {
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
			a.append("channel.slack.error", "", map[string]any{"error": err.Error()})
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

func (a *SlackAdapter) pollOnce(ctx context.Context, handle inputlayer.InboundHandler) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://slack.com/api/conversations.history?channel="+a.config.DefaultChannel+"&limit=10", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+a.config.BotToken)
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var payload struct {
		OK       bool   `json:"ok"`
		Error    string `json:"error"`
		Messages []struct {
			Text     string `json:"text"`
			Ts       string `json:"ts"`
			ThreadTS string `json:"thread_ts"`
			User     string `json:"user"`
			BotID    string `json:"bot_id"`
			Files    []struct {
				Mimetype string `json:"mimetype"`
				URL      string `json:"url_private"`
				Title    string `json:"title"`
			} `json:"files"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return err
	}
	if !payload.OK {
		if strings.TrimSpace(payload.Error) == "" {
			return fmt.Errorf("slack history failed: api returned ok=false")
		}
		return fmt.Errorf("slack history failed: %s", payload.Error)
	}

	for i := len(payload.Messages) - 1; i >= 0; i-- {
		msg := payload.Messages[i]
		if msg.Ts == "" || slackTSLessOrEqual(msg.Ts, a.latestTS) || msg.BotID != "" {
			continue
		}
		channelType, isGroup := slackConversationMetadata(a.config.DefaultChannel)

		meta := map[string]string{
			"channel":      "slack",
			"channel_id":   a.config.DefaultChannel,
			"user_id":      msg.User,
			"reply_target": a.config.DefaultChannel,
			"message_id":   msg.Ts,
			"sender":       msg.User,
			"thread_id":    msg.ThreadTS,
			"channel_type": channelType,
			"is_group":     boolString(isGroup),
		}

		audioURL, audioMIME := a.findAudioFile(msg.Files)
		if audioURL != "" {
			meta["message_type"] = "voice_note"
			meta["audio_url"] = audioURL
			meta["audio_mime"] = audioMIME
			if strings.TrimSpace(msg.Text) != "" {
				meta["caption"] = strings.TrimSpace(msg.Text)
			}

			sessionID, response, err := handle(ctx, "", audioURL, meta)
			if err != nil {
				return err
			}
			if err := a.sendMessage(ctx, response, msg.ThreadTS); err != nil {
				return err
			}
			a.latestTS = msg.Ts
			a.base.MarkActivity()
			a.append("channel.slack.voice", sessionID, map[string]any{
				"channel":      a.config.DefaultChannel,
				"user":         msg.User,
				"message_type": "voice_note",
				"audio_url":    audioURL,
				"audio_mime":   audioMIME,
			})
			continue
		}

		if strings.TrimSpace(msg.Text) == "" {
			a.latestTS = msg.Ts
			continue
		}

		sessionID, response, err := handle(ctx, "", msg.Text, meta)
		if err != nil {
			return err
		}
		if err := a.sendMessage(ctx, response, msg.ThreadTS); err != nil {
			return err
		}
		a.latestTS = msg.Ts
		a.base.MarkActivity()
		a.append("channel.slack.message", sessionID, map[string]any{
			"channel": a.config.DefaultChannel,
			"user":    msg.User,
			"text":    msg.Text,
		})
	}
	return nil
}

func (a *SlackAdapter) sendMessage(ctx context.Context, text string, threadTS string) error {
	bodyMap := map[string]any{"channel": a.config.DefaultChannel, "text": text}
	if strings.TrimSpace(threadTS) != "" {
		bodyMap["thread_ts"] = threadTS
	}
	body, _ := json.Marshal(bodyMap)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://slack.com/api/chat.postMessage", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+a.config.BotToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("slack send failed: %s", resp.Status)
	}

	var result struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if !result.OK {
		if strings.TrimSpace(result.Error) == "" {
			return fmt.Errorf("slack send failed: api returned ok=false")
		}
		return fmt.Errorf("slack send failed: %s", result.Error)
	}
	return nil
}

func (a *SlackAdapter) append(eventType string, sessionID string, payload map[string]any) {
	if a.appendEvent != nil {
		a.appendEvent(eventType, sessionID, payload)
	}
}

func (a *SlackAdapter) findAudioFile(files []struct {
	Mimetype string `json:"mimetype"`
	URL      string `json:"url_private"`
	Title    string `json:"title"`
}) (string, string) {
	for _, f := range files {
		mime := strings.ToLower(f.Mimetype)
		if strings.HasPrefix(mime, "audio/") {
			return f.URL, f.Mimetype
		}
	}
	return "", ""
}

func (a *SlackAdapter) sendMessageWithResult(ctx context.Context, text string, threadTS string) (string, error) {
	bodyMap := map[string]any{"channel": a.config.DefaultChannel, "text": text}
	if strings.TrimSpace(threadTS) != "" {
		bodyMap["thread_ts"] = threadTS
	}
	body, _ := json.Marshal(bodyMap)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://slack.com/api/chat.postMessage", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+a.config.BotToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("slack send failed: %s", resp.Status)
	}

	var result struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
		Ts    string `json:"ts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if !result.OK {
		if strings.TrimSpace(result.Error) == "" {
			return "", fmt.Errorf("slack send failed: api returned ok=false")
		}
		return "", fmt.Errorf("slack send failed: %s", result.Error)
	}
	return result.Ts, nil
}

func (a *SlackAdapter) editMessage(ctx context.Context, ts string, text string) error {
	if ts == "" {
		return nil
	}
	body, _ := json.Marshal(map[string]any{"channel": a.config.DefaultChannel, "ts": ts, "text": text})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://slack.com/api/chat.update", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+a.config.BotToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("slack update failed: %s", resp.Status)
	}
	var result struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if !result.OK {
		if strings.TrimSpace(result.Error) == "" {
			return fmt.Errorf("slack update failed: api returned ok=false")
		}
		return fmt.Errorf("slack update failed: %s", result.Error)
	}
	return nil
}

func (a *SlackAdapter) sendStreamingMessage(ctx context.Context, threadTS string, streamFn func(onChunk func(chunk string) error) error) error {
	streamInterval := time.Duration(a.config.StreamInterval) * time.Millisecond
	if streamInterval <= 0 {
		streamInterval = 500 * time.Millisecond
	}

	initialTs, err := a.sendMessageWithResult(ctx, "\u200B", threadTS)
	if err != nil {
		return err
	}
	if initialTs == "" {
		return streamWithMessageFallback(func(onChunk func(chunk string)) error {
			return streamFn(func(chunk string) error {
				onChunk(chunk)
				return nil
			})
		}, func(final string) error {
			return a.sendMessage(ctx, final, threadTS)
		})
	}

	var accumulated strings.Builder
	var mu sync.Mutex
	lastEdit := time.Now()

	onChunk := func(chunk string) error {
		mu.Lock()
		accumulated.WriteString(chunk)
		shouldEdit := time.Since(lastEdit) >= streamInterval
		if shouldEdit {
			lastEdit = time.Now()
		}
		mu.Unlock()

		if shouldEdit {
			mu.Lock()
			text := accumulated.String()
			mu.Unlock()
			if err := a.editMessage(ctx, initialTs, text); err != nil {
				return err
			}
		}
		return nil
	}

	if err := streamFn(onChunk); err != nil {
		mu.Lock()
		final := accumulated.String()
		mu.Unlock()
		if editErr := a.editMessage(ctx, initialTs, final+"\n\n[Error: "+err.Error()+"]"); editErr != nil {
			return errors.Join(err, editErr)
		}
		return err
	}

	mu.Lock()
	final := accumulated.String()
	mu.Unlock()
	return a.editMessage(ctx, initialTs, final)
}

func (a *SlackAdapter) RunStream(ctx context.Context, handle inputlayer.StreamChunkHandler) error {
	a.base.SetRunning(true)
	defer a.base.SetRunning(false)
	interval := time.Duration(a.config.PollEvery) * time.Second
	if interval <= 0 {
		interval = 3 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if err := a.pollOnceStream(ctx, handle); err != nil {
			a.base.SetError(err)
			a.append("channel.slack.error", "", map[string]any{"error": err.Error()})
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

func (a *SlackAdapter) pollOnceStream(ctx context.Context, handle inputlayer.StreamChunkHandler) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://slack.com/api/conversations.history?channel="+a.config.DefaultChannel+"&limit=10", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+a.config.BotToken)
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var payload struct {
		OK       bool   `json:"ok"`
		Error    string `json:"error"`
		Messages []struct {
			Text     string `json:"text"`
			Ts       string `json:"ts"`
			ThreadTS string `json:"thread_ts"`
			User     string `json:"user"`
			BotID    string `json:"bot_id"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return err
	}
	if !payload.OK {
		if strings.TrimSpace(payload.Error) == "" {
			return fmt.Errorf("slack history failed: api returned ok=false")
		}
		return fmt.Errorf("slack history failed: %s", payload.Error)
	}

	for i := len(payload.Messages) - 1; i >= 0; i-- {
		msg := payload.Messages[i]
		if msg.Ts == "" || slackTSLessOrEqual(msg.Ts, a.latestTS) || msg.BotID != "" {
			continue
		}

		if strings.TrimSpace(msg.Text) == "" {
			a.latestTS = msg.Ts
			continue
		}
		channelType, isGroup := slackConversationMetadata(a.config.DefaultChannel)

		meta := map[string]string{
			"channel":      "slack",
			"channel_id":   a.config.DefaultChannel,
			"user_id":      msg.User,
			"reply_target": a.config.DefaultChannel,
			"message_id":   msg.Ts,
			"sender":       msg.User,
			"thread_id":    msg.ThreadTS,
			"channel_type": channelType,
			"is_group":     boolString(isGroup),
		}

		sessionID := ""
		err := a.sendStreamingMessage(ctx, msg.ThreadTS, func(onChunk func(chunk string) error) error {
			var err error
			sessionID, err = handle(ctx, sessionID, msg.Text, meta, func(chunk string) error {
				return onChunk(chunk)
			})
			return err
		})
		if err != nil {
			return err
		}
		a.latestTS = msg.Ts
		a.base.MarkActivity()
		a.append("channel.slack.message", sessionID, map[string]any{
			"channel":   a.config.DefaultChannel,
			"user":      msg.User,
			"text":      msg.Text,
			"streaming": true,
		})
	}
	return nil
}

func slackConversationMetadata(channelID string) (string, bool) {
	channelID = strings.ToUpper(strings.TrimSpace(channelID))
	switch {
	case strings.HasPrefix(channelID, "D"):
		return "dm", false
	case channelID != "":
		return "group", true
	default:
		return "", false
	}
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
