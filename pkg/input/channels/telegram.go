package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/config"
)

type TelegramAdapter struct {
	base        BaseAdapter
	config      config.TelegramChannelConfig
	baseURL     string
	client      *http.Client
	offset      int64
	appendEvent func(eventType string, sessionID string, payload map[string]any)
}

func NewTelegramAdapter(cfg config.TelegramChannelConfig, appendEvent func(eventType string, sessionID string, payload map[string]any)) *TelegramAdapter {
	return &TelegramAdapter{
		base:        NewBaseAdapter("telegram", cfg.Enabled && cfg.BotToken != ""),
		config:      cfg,
		baseURL:     "https://api.telegram.org/bot" + cfg.BotToken,
		client:      &http.Client{Timeout: 20 * time.Second},
		appendEvent: appendEvent,
	}
}

func (a *TelegramAdapter) Name() string {
	return "telegram"
}

func (a *TelegramAdapter) Enabled() bool {
	return a.config.Enabled && a.config.BotToken != "" && (strings.TrimSpace(a.config.ChatID) != "" || true)
}

func (a *TelegramAdapter) Status() Status {
	status := a.base.Status()
	status.Enabled = a.Enabled()
	return status
}

func (a *TelegramAdapter) Run(ctx context.Context, runMessage InboundHandler) error {
	a.base.SetRunning(true)
	defer a.base.SetRunning(false)
	interval := time.Duration(a.config.PollEvery) * time.Second
	if interval <= 0 {
		interval = 3 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if err := a.pollOnce(ctx, runMessage); err != nil {
			a.base.SetError(err)
			a.append("channel.telegram.error", "", map[string]any{"error": err.Error()})
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

func (a *TelegramAdapter) pollOnce(ctx context.Context, runMessage InboundHandler) error {
	u := fmt.Sprintf("%s/getUpdates?timeout=1&offset=%d", a.baseURL, a.offset)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var payload struct {
		OK     bool `json:"ok"`
		Result []struct {
			UpdateID int64 `json:"update_id"`
			Message  struct {
				Text    string `json:"text"`
				Caption string `json:"caption"`
				Chat    struct {
					ID int64 `json:"id"`
				} `json:"chat"`
				From struct {
					Username string `json:"username"`
					ID       int64  `json:"id"`
				} `json:"from"`
				Voice *struct {
					FileID   string `json:"file_id"`
					Duration int    `json:"duration"`
					MimeType string `json:"mime_type"`
					FileSize int    `json:"file_size"`
				} `json:"voice"`
				Audio *struct {
					FileID   string `json:"file_id"`
					Duration int    `json:"duration"`
					MimeType string `json:"mime_type"`
					FileSize int    `json:"file_size"`
					FileName string `json:"file_name"`
				} `json:"audio"`
				Document *struct {
					FileID   string `json:"file_id"`
					MimeType string `json:"mime_type"`
					FileSize int    `json:"file_size"`
					FileName string `json:"file_name"`
				} `json:"document"`
			} `json:"message"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return err
	}

	for _, update := range payload.Result {
		a.offset = update.UpdateID + 1
		chatID := strconv.FormatInt(update.Message.Chat.ID, 10)
		if strings.TrimSpace(a.config.ChatID) != "" && chatID != strings.TrimSpace(a.config.ChatID) {
			continue
		}

		text := strings.TrimSpace(update.Message.Text)
		caption := strings.TrimSpace(update.Message.Caption)
		username := update.Message.From.Username
		messageID := strconv.FormatInt(update.UpdateID, 10)

		meta := map[string]string{
			"channel":      "telegram",
			"chat_id":      chatID,
			"username":     username,
			"reply_target": chatID,
			"message_id":   messageID,
			"sender":       username,
		}

		var messageType string
		var audioURL string
		var audioMIME string

		if update.Message.Voice != nil {
			messageType = "voice_note"
			audioMIME = update.Message.Voice.MimeType
			if audioMIME == "" {
				audioMIME = "audio/ogg"
			}
			fileURL, err := a.getFileURL(ctx, update.Message.Voice.FileID)
			if err != nil {
				a.append("channel.telegram.error", "", map[string]any{"error": err.Error(), "type": "voice_download_failed"})
				continue
			}
			audioURL = fileURL
		} else if update.Message.Audio != nil {
			messageType = "audio_file"
			audioMIME = update.Message.Audio.MimeType
			fileURL, err := a.getFileURL(ctx, update.Message.Audio.FileID)
			if err != nil {
				a.append("channel.telegram.error", "", map[string]any{"error": err.Error(), "type": "audio_download_failed"})
				continue
			}
			audioURL = fileURL
		} else if update.Message.Document != nil && strings.HasPrefix(update.Message.Document.MimeType, "audio/") {
			messageType = "audio_file"
			audioMIME = update.Message.Document.MimeType
			fileURL, err := a.getFileURL(ctx, update.Message.Document.FileID)
			if err != nil {
				a.append("channel.telegram.error", "", map[string]any{"error": err.Error(), "type": "document_audio_download_failed"})
				continue
			}
			audioURL = fileURL
		}

		if messageType != "" {
			meta["message_type"] = messageType
			meta["audio_url"] = audioURL
			meta["audio_mime"] = audioMIME
			if caption != "" {
				meta["caption"] = caption
			}

			sessionID, response, err := runMessage(ctx, "", audioURL, meta)
			if err != nil {
				return err
			}
			if err := a.sendMessage(ctx, chatID, response); err != nil {
				return err
			}
			a.base.MarkActivity()
			a.append("channel.telegram.voice", sessionID, map[string]any{
				"chat_id":      chatID,
				"message_type": messageType,
				"audio_url":    audioURL,
				"audio_mime":   audioMIME,
			})
			continue
		}

		if text == "" {
			continue
		}

		sessionID, response, err := runMessage(ctx, "", text, meta)
		if err != nil {
			return err
		}
		if err := a.sendMessage(ctx, chatID, response); err != nil {
			return err
		}
		a.base.MarkActivity()
		a.append("channel.telegram.message", sessionID, map[string]any{
			"chat_id": chatID,
			"text":    text,
		})
	}
	return nil
}

func (a *TelegramAdapter) getFileURL(ctx context.Context, fileID string) (string, error) {
	u := fmt.Sprintf("%s/getFile?file_id=%s", a.baseURL, url.QueryEscape(fileID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		OK     bool `json:"ok"`
		Result struct {
			FileID   string `json:"file_id"`
			FileSize int    `json:"file_size"`
			FilePath string `json:"file_path"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if !result.OK || result.Result.FilePath == "" {
		return "", fmt.Errorf("telegram: failed to get file URL for %s", fileID)
	}

	return fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", a.config.BotToken, result.Result.FilePath), nil
}

func (a *TelegramAdapter) append(eventType string, sessionID string, payload map[string]any) {
	if a.appendEvent != nil {
		a.appendEvent(eventType, sessionID, payload)
	}
}

func (a *TelegramAdapter) sendMessage(ctx context.Context, chatID string, text string) error {
	values := url.Values{}
	values.Set("chat_id", chatID)
	values.Set("text", text)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/sendMessage", strings.NewReader(values.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("telegram send failed: %s", resp.Status)
	}
	return nil
}

func (a *TelegramAdapter) sendStreamingMessage(ctx context.Context, chatID string, streamFn func(onChunk func(chunk string)) error) error {
	streamInterval := time.Duration(a.config.StreamInterval) * time.Millisecond
	if streamInterval <= 0 {
		streamInterval = 500 * time.Millisecond
	}

	initialMsgID, err := a.sendMessageWithResult(ctx, chatID, "\u200B")
	if err != nil {
		return err
	}
	if initialMsgID == "" {
		return streamFn(func(chunk string) {})
	}

	var accumulated strings.Builder
	var mu sync.Mutex
	lastEdit := time.Now()

	onChunk := func(chunk string) {
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
			a.editMessage(ctx, chatID, initialMsgID, text)
		}
	}

	if err := streamFn(onChunk); err != nil {
		mu.Lock()
		final := accumulated.String()
		mu.Unlock()
		a.editMessage(ctx, chatID, initialMsgID, final+"\n\n[Error: "+err.Error()+"]")
		return err
	}

	mu.Lock()
	final := accumulated.String()
	mu.Unlock()
	a.editMessage(ctx, chatID, initialMsgID, final)
	return nil
}

func (a *TelegramAdapter) sendMessageWithResult(ctx context.Context, chatID string, text string) (string, error) {
	values := url.Values{}
	values.Set("chat_id", chatID)
	values.Set("text", text)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/sendMessage", strings.NewReader(values.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := a.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("telegram send failed: %s", resp.Status)
	}

	var result struct {
		OK     bool `json:"ok"`
		Result struct {
			MessageID int `json:"message_id"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if !result.OK {
		return "", nil
	}
	return strconv.Itoa(result.Result.MessageID), nil
}

func (a *TelegramAdapter) editMessage(ctx context.Context, chatID string, messageID string, text string) error {
	if messageID == "" {
		return nil
	}
	values := url.Values{}
	values.Set("chat_id", chatID)
	values.Set("message_id", messageID)
	truncated := text
	if len([]rune(truncated)) > 4096 {
		truncated = string([]rune(truncated)[:4093]) + "..."
	}
	values.Set("text", truncated)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/editMessageText", strings.NewReader(values.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := a.client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	return nil
}

func (a *TelegramAdapter) RunStream(ctx context.Context, handle StreamChunkHandler) error {
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
			a.append("channel.telegram.error", "", map[string]any{"error": err.Error()})
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

func (a *TelegramAdapter) pollOnceStream(ctx context.Context, handle StreamChunkHandler) error {
	u := fmt.Sprintf("%s/getUpdates?timeout=1&offset=%d", a.baseURL, a.offset)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var payload struct {
		OK     bool `json:"ok"`
		Result []struct {
			UpdateID int64 `json:"update_id"`
			Message  struct {
				Text string `json:"text"`
				Chat struct {
					ID int64 `json:"id"`
				} `json:"chat"`
				From struct {
					Username string `json:"username"`
					ID       int64  `json:"id"`
				} `json:"from"`
			} `json:"message"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return err
	}

	for _, update := range payload.Result {
		a.offset = update.UpdateID + 1
		chatID := strconv.FormatInt(update.Message.Chat.ID, 10)
		if strings.TrimSpace(a.config.ChatID) != "" && chatID != strings.TrimSpace(a.config.ChatID) {
			continue
		}

		text := strings.TrimSpace(update.Message.Text)
		if text == "" {
			continue
		}

		username := update.Message.From.Username
		messageID := strconv.FormatInt(update.UpdateID, 10)

		meta := map[string]string{
			"channel":      "telegram",
			"chat_id":      chatID,
			"username":     username,
			"reply_target": chatID,
			"message_id":   messageID,
			"sender":       username,
		}

		sessionID := ""
		var responseText string
		err := a.sendStreamingMessage(ctx, chatID, func(onChunk func(chunk string)) error {
			var err error
			sessionID, err = handle(ctx, sessionID, text, meta, func(chunk string) error {
				onChunk(chunk)
				responseText += chunk
				return nil
			})
			return err
		})
		if err != nil {
			return err
		}
		_ = responseText
		a.base.MarkActivity()
		a.append("channel.telegram.message", sessionID, map[string]any{
			"chat_id":   chatID,
			"text":      text,
			"streaming": true,
		})
	}
	return nil
}
