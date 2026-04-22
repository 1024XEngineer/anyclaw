package channels

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/config"
	inputlayer "github.com/1024XEngineer/anyclaw/pkg/input"
)

type TelegramAdapter struct {
	base        inputlayer.BaseAdapter
	config      config.TelegramChannelConfig
	baseURL     string
	client      *http.Client
	offset      int64
	appendEvent func(eventType string, sessionID string, payload map[string]any)
}

func NewTelegramAdapter(cfg config.TelegramChannelConfig, appendEvent func(eventType string, sessionID string, payload map[string]any)) *TelegramAdapter {
	return &TelegramAdapter{
		base:        inputlayer.NewBaseAdapter("telegram", cfg.Enabled && cfg.BotToken != ""),
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
	return a.config.Enabled && strings.TrimSpace(a.config.BotToken) != ""
}

func (a *TelegramAdapter) Status() inputlayer.Status {
	status := a.base.Status()
	status.Enabled = a.Enabled()
	return status
}

func (a *TelegramAdapter) Run(ctx context.Context, runMessage inputlayer.InboundHandler) error {
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

func (a *TelegramAdapter) pollOnce(ctx context.Context, runMessage inputlayer.InboundHandler) error {
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
		OK          bool   `json:"ok"`
		Description string `json:"description"`
		Result      []struct {
			UpdateID int64 `json:"update_id"`
			Message  struct {
				Text    string `json:"text"`
				Caption string `json:"caption"`
				Chat    struct {
					ID   int64  `json:"id"`
					Type string `json:"type"`
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
	if !payload.OK {
		if strings.TrimSpace(payload.Description) == "" {
			return fmt.Errorf("telegram getUpdates failed: api returned ok=false")
		}
		return fmt.Errorf("telegram getUpdates failed: %s", payload.Description)
	}

	for _, update := range payload.Result {
		nextOffset := update.UpdateID + 1
		chatID := strconv.FormatInt(update.Message.Chat.ID, 10)
		if strings.TrimSpace(a.config.ChatID) != "" && chatID != strings.TrimSpace(a.config.ChatID) {
			a.offset = nextOffset
			continue
		}

		text := strings.TrimSpace(update.Message.Text)
		caption := strings.TrimSpace(update.Message.Caption)
		if text == "" {
			text = caption
		}
		username := update.Message.From.Username
		userID := strconv.FormatInt(update.Message.From.ID, 10)
		messageID := strconv.FormatInt(update.UpdateID, 10)
		channelType, isGroup := telegramConversationMetadata(update.Message.Chat.Type, update.Message.Chat.ID, update.Message.From.ID)

		meta := map[string]string{
			"channel":      "telegram",
			"chat_id":      chatID,
			"chat_type":    update.Message.Chat.Type,
			"channel_type": channelType,
			"is_group":     boolString(isGroup),
			"user_id":      userID,
			"username":     username,
			"reply_target": chatID,
			"message_id":   messageID,
			"sender":       username,
		}

		var messageType string
		var audioURL string
		var audioRef string
		var audioFileID string
		var audioMIME string
		var cleanupAudio func()

		if update.Message.Voice != nil {
			messageType = "voice_note"
			audioFileID = strings.TrimSpace(update.Message.Voice.FileID)
			audioMIME = update.Message.Voice.MimeType
			if audioMIME == "" {
				audioMIME = "audio/ogg"
			}
			audioRef = telegramFileRef(audioFileID)
			localPath, cleanup, err := a.downloadFile(ctx, audioFileID)
			if err != nil {
				return err
			}
			audioURL = localPath
			cleanupAudio = cleanup
		} else if update.Message.Audio != nil {
			messageType = "audio_file"
			audioFileID = strings.TrimSpace(update.Message.Audio.FileID)
			audioMIME = update.Message.Audio.MimeType
			audioRef = telegramFileRef(audioFileID)
			localPath, cleanup, err := a.downloadFile(ctx, audioFileID)
			if err != nil {
				return err
			}
			audioURL = localPath
			cleanupAudio = cleanup
		} else if update.Message.Document != nil && strings.HasPrefix(update.Message.Document.MimeType, "audio/") {
			messageType = "audio_file"
			audioFileID = strings.TrimSpace(update.Message.Document.FileID)
			audioMIME = update.Message.Document.MimeType
			audioRef = telegramFileRef(audioFileID)
			localPath, cleanup, err := a.downloadFile(ctx, audioFileID)
			if err != nil {
				return err
			}
			audioURL = localPath
			cleanupAudio = cleanup
		}

		if messageType != "" {
			meta["message_type"] = messageType
			meta["audio_url"] = audioURL
			meta["audio_ref"] = audioRef
			meta["audio_file_id"] = audioFileID
			meta["audio_mime"] = audioMIME
			if caption != "" {
				meta["caption"] = caption
			}

			sessionID, response, err := func() (string, string, error) {
				if cleanupAudio != nil {
					defer cleanupAudio()
				}
				return runMessage(ctx, "", audioURL, meta)
			}()
			if err != nil {
				return err
			}
			if err := a.sendMessage(ctx, chatID, response); err != nil {
				return err
			}
			a.offset = nextOffset
			a.base.MarkActivity()
			a.append("channel.telegram.voice", sessionID, map[string]any{
				"chat_id":       chatID,
				"message_type":  messageType,
				"audio_ref":     audioRef,
				"audio_file_id": audioFileID,
				"audio_mime":    audioMIME,
			})
			continue
		}

		if text == "" {
			a.offset = nextOffset
			continue
		}

		sessionID, response, err := runMessage(ctx, "", text, meta)
		if err != nil {
			return err
		}
		if err := a.sendMessage(ctx, chatID, response); err != nil {
			return err
		}
		a.offset = nextOffset
		a.base.MarkActivity()
		a.append("channel.telegram.message", sessionID, map[string]any{
			"chat_id": chatID,
			"text":    text,
		})
	}
	return nil
}

func telegramFileRef(fileID string) string {
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return ""
	}
	return "telegram-file:" + fileID
}

func (a *TelegramAdapter) resolveFileDownloadURL(ctx context.Context, fileID string) (string, error) {
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return "", fmt.Errorf("telegram: missing file id")
	}

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
		OK          bool   `json:"ok"`
		Description string `json:"description"`
		Result      struct {
			FileID   string `json:"file_id"`
			FilePath string `json:"file_path"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if !result.OK || strings.TrimSpace(result.Result.FilePath) == "" {
		if strings.TrimSpace(result.Description) == "" {
			return "", fmt.Errorf("telegram: failed to resolve file path for %s", fileID)
		}
		return "", fmt.Errorf("telegram: %s", result.Description)
	}
	return fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", a.config.BotToken, strings.TrimPrefix(result.Result.FilePath, "/")), nil
}

func (a *TelegramAdapter) downloadFile(ctx context.Context, fileID string) (string, func(), error) {
	remoteURL, err := a.resolveFileDownloadURL(ctx, fileID)
	if err != nil {
		return "", nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, remoteURL, nil)
	if err != nil {
		return "", nil, err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", nil, fmt.Errorf("telegram file download failed: %s", resp.Status)
	}

	suffix := path.Ext(req.URL.Path)
	tmpFile, err := os.CreateTemp("", "anyclaw-telegram-*"+suffix)
	if err != nil {
		return "", nil, err
	}
	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		_ = os.Remove(tmpFile.Name())
		return "", nil, err
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpFile.Name())
		return "", nil, err
	}

	cleanup := func() {
		_ = os.Remove(tmpFile.Name())
	}
	return tmpFile.Name(), cleanup, nil
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

	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if !result.OK {
		if strings.TrimSpace(result.Description) == "" {
			return fmt.Errorf("telegram send failed: api returned ok=false")
		}
		return fmt.Errorf("telegram send failed: %s", result.Description)
	}
	return nil
}

func (a *TelegramAdapter) sendStreamingMessage(ctx context.Context, chatID string, streamFn func(onChunk func(chunk string) error) error) error {
	streamInterval := time.Duration(a.config.StreamInterval) * time.Millisecond
	if streamInterval <= 0 {
		streamInterval = 500 * time.Millisecond
	}

	initialMsgID, err := a.sendMessageWithResult(ctx, chatID, "\u200B")
	if err != nil {
		return err
	}
	if initialMsgID == "" {
		return streamWithMessageFallback(func(onChunk func(chunk string)) error {
			return streamFn(func(chunk string) error {
				onChunk(chunk)
				return nil
			})
		}, func(final string) error {
			return a.sendMessage(ctx, chatID, final)
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
			if err := a.editMessage(ctx, chatID, initialMsgID, text); err != nil {
				return err
			}
		}
		return nil
	}

	if err := streamFn(onChunk); err != nil {
		mu.Lock()
		final := accumulated.String()
		mu.Unlock()
		if strings.TrimSpace(final) == "" {
			if deleteErr := a.deleteMessage(ctx, chatID, initialMsgID); deleteErr != nil {
				return errors.Join(err, deleteErr)
			}
			return err
		}
		if editErr := a.editMessage(ctx, chatID, initialMsgID, final+"\n\n[Error: "+err.Error()+"]"); editErr != nil {
			return errors.Join(err, editErr)
		}
		return err
	}

	mu.Lock()
	final := accumulated.String()
	mu.Unlock()
	if strings.TrimSpace(final) == "" {
		return a.deleteMessage(ctx, chatID, initialMsgID)
	}
	return a.editMessage(ctx, chatID, initialMsgID, final)
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
		OK          bool   `json:"ok"`
		Description string `json:"description"`
		Result      struct {
			MessageID int `json:"message_id"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if !result.OK {
		if strings.TrimSpace(result.Description) == "" {
			return "", fmt.Errorf("telegram send failed: api returned ok=false")
		}
		return "", fmt.Errorf("telegram send failed: %s", result.Description)
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
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("telegram edit failed: %s", resp.Status)
	}
	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if !result.OK {
		if strings.TrimSpace(result.Description) == "" {
			return fmt.Errorf("telegram edit failed: api returned ok=false")
		}
		return fmt.Errorf("telegram edit failed: %s", result.Description)
	}
	return nil
}

func (a *TelegramAdapter) deleteMessage(ctx context.Context, chatID string, messageID string) error {
	if strings.TrimSpace(messageID) == "" {
		return nil
	}
	values := url.Values{}
	values.Set("chat_id", chatID)
	values.Set("message_id", messageID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/deleteMessage", strings.NewReader(values.Encode()))
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
		return fmt.Errorf("telegram delete failed: %s", resp.Status)
	}
	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if !result.OK {
		if strings.TrimSpace(result.Description) == "" {
			return fmt.Errorf("telegram delete failed: api returned ok=false")
		}
		return fmt.Errorf("telegram delete failed: %s", result.Description)
	}
	return nil
}

func (a *TelegramAdapter) RunStream(ctx context.Context, handle inputlayer.StreamChunkHandler) error {
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

func (a *TelegramAdapter) pollOnceStream(ctx context.Context, handle inputlayer.StreamChunkHandler) error {
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
		OK          bool   `json:"ok"`
		Description string `json:"description"`
		Result      []struct {
			UpdateID int64 `json:"update_id"`
			Message  struct {
				Text    string `json:"text"`
				Caption string `json:"caption"`
				Chat    struct {
					ID   int64  `json:"id"`
					Type string `json:"type"`
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
	if !payload.OK {
		if strings.TrimSpace(payload.Description) == "" {
			return fmt.Errorf("telegram getUpdates failed: api returned ok=false")
		}
		return fmt.Errorf("telegram getUpdates failed: %s", payload.Description)
	}

	for _, update := range payload.Result {
		nextOffset := update.UpdateID + 1
		chatID := strconv.FormatInt(update.Message.Chat.ID, 10)
		if strings.TrimSpace(a.config.ChatID) != "" && chatID != strings.TrimSpace(a.config.ChatID) {
			a.offset = nextOffset
			continue
		}

		text := strings.TrimSpace(update.Message.Text)
		caption := strings.TrimSpace(update.Message.Caption)
		if text == "" {
			text = caption
		}

		username := update.Message.From.Username
		userID := strconv.FormatInt(update.Message.From.ID, 10)
		messageID := strconv.FormatInt(update.UpdateID, 10)
		channelType, isGroup := telegramConversationMetadata(update.Message.Chat.Type, update.Message.Chat.ID, update.Message.From.ID)

		meta := map[string]string{
			"channel":      "telegram",
			"chat_id":      chatID,
			"chat_type":    update.Message.Chat.Type,
			"channel_type": channelType,
			"is_group":     boolString(isGroup),
			"user_id":      userID,
			"username":     username,
			"reply_target": chatID,
			"message_id":   messageID,
			"sender":       username,
		}

		var messageType string
		var audioURL string
		var audioRef string
		var audioFileID string
		var audioMIME string
		var cleanupAudio func()
		if update.Message.Voice != nil {
			messageType = "voice_note"
			audioFileID = strings.TrimSpace(update.Message.Voice.FileID)
			audioMIME = update.Message.Voice.MimeType
			if audioMIME == "" {
				audioMIME = "audio/ogg"
			}
			audioRef = telegramFileRef(audioFileID)
			localPath, cleanup, err := a.downloadFile(ctx, audioFileID)
			if err != nil {
				return err
			}
			audioURL = localPath
			cleanupAudio = cleanup
		} else if update.Message.Audio != nil {
			messageType = "audio_file"
			audioFileID = strings.TrimSpace(update.Message.Audio.FileID)
			audioMIME = update.Message.Audio.MimeType
			audioRef = telegramFileRef(audioFileID)
			localPath, cleanup, err := a.downloadFile(ctx, audioFileID)
			if err != nil {
				return err
			}
			audioURL = localPath
			cleanupAudio = cleanup
		} else if update.Message.Document != nil && strings.HasPrefix(update.Message.Document.MimeType, "audio/") {
			messageType = "audio_file"
			audioFileID = strings.TrimSpace(update.Message.Document.FileID)
			audioMIME = update.Message.Document.MimeType
			audioRef = telegramFileRef(audioFileID)
			localPath, cleanup, err := a.downloadFile(ctx, audioFileID)
			if err != nil {
				return err
			}
			audioURL = localPath
			cleanupAudio = cleanup
		}

		if messageType != "" {
			meta["message_type"] = messageType
			meta["audio_url"] = audioURL
			meta["audio_ref"] = audioRef
			meta["audio_file_id"] = audioFileID
			meta["audio_mime"] = audioMIME
			if caption != "" {
				meta["caption"] = caption
			}

			var response strings.Builder
			sessionID, err := func() (string, error) {
				if cleanupAudio != nil {
					defer cleanupAudio()
				}
				return handle(ctx, "", audioURL, meta, func(chunk string) error {
					response.WriteString(chunk)
					return nil
				})
			}()
			if err != nil {
				return err
			}
			if err := a.sendMessage(ctx, chatID, response.String()); err != nil {
				return err
			}
			a.offset = nextOffset
			a.base.MarkActivity()
			a.append("channel.telegram.voice", sessionID, map[string]any{
				"chat_id":       chatID,
				"message_type":  messageType,
				"audio_ref":     audioRef,
				"audio_file_id": audioFileID,
				"audio_mime":    audioMIME,
				"streaming":     true,
			})
			continue
		}

		if text == "" {
			a.offset = nextOffset
			continue
		}

		sessionID := ""
		err := a.sendStreamingMessage(ctx, chatID, func(onChunk func(chunk string) error) error {
			var err error
			sessionID, err = handle(ctx, sessionID, text, meta, func(chunk string) error {
				return onChunk(chunk)
			})
			return err
		})
		if err != nil {
			return err
		}
		a.offset = nextOffset
		a.base.MarkActivity()
		a.append("channel.telegram.message", sessionID, map[string]any{
			"chat_id":   chatID,
			"text":      text,
			"streaming": true,
		})
	}
	return nil
}

func telegramConversationMetadata(chatType string, chatID int64, userID int64) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(chatType))
	switch normalized {
	case "private":
		return "private", false
	case "group", "supergroup", "channel":
		return normalized, true
	}
	if chatID < 0 {
		return "group", true
	}
	if userID != 0 && chatID == userID {
		return "private", false
	}
	return "", false
}
