package channels

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/config"
	"github.com/gorilla/websocket"
)

type DiscordAdapter struct {
	base        BaseAdapter
	config      config.DiscordChannelConfig
	client      *http.Client
	appendEvent func(eventType string, sessionID string, payload map[string]any)
	latestID    string
	apiBaseURL  string
	processed   map[string]time.Time
}

func (a *DiscordAdapter) VerifyInteraction(r *http.Request, body []byte) bool {
	publicKeyHex := strings.TrimSpace(a.config.PublicKey)
	if publicKeyHex == "" {
		return false
	}
	publicKey, err := hex.DecodeString(publicKeyHex)
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return false
	}
	sigHex := strings.TrimSpace(r.Header.Get("X-Signature-Ed25519"))
	ts := strings.TrimSpace(r.Header.Get("X-Signature-Timestamp"))
	if sigHex == "" || ts == "" {
		return false
	}
	sig, err := hex.DecodeString(sigHex)
	if err != nil {
		return false
	}
	message := append([]byte(ts), body...)
	return ed25519.Verify(ed25519.PublicKey(publicKey), message, sig)
}

func (a *DiscordAdapter) HandleInteraction(ctx context.Context, body []byte, handle InboundHandler) (map[string]any, error) {
	var payload struct {
		Type int `json:"type"`
		Data struct {
			Name    string `json:"name"`
			Options []struct {
				Name  string `json:"name"`
				Value any    `json:"value"`
			} `json:"options"`
		} `json:"data"`
		ChannelID string `json:"channel_id"`
		GuildID   string `json:"guild_id"`
		Member    struct {
			User struct {
				ID       string `json:"id"`
				Username string `json:"username"`
			} `json:"user"`
		} `json:"member"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	if payload.Type == 1 {
		return map[string]any{"type": 1}, nil
	}
	message := payload.Data.Name
	for _, opt := range payload.Data.Options {
		if strings.EqualFold(opt.Name, "message") {
			message = fmt.Sprintf("%v", opt.Value)
		}
	}
	respSession, response, err := handle(ctx, "", message, map[string]string{"channel": "discord", "channel_id": payload.ChannelID, "guild_id": payload.GuildID, "user_id": payload.Member.User.ID, "username": payload.Member.User.Username, "reply_target": payload.ChannelID, "message_id": fmt.Sprintf("interaction:%d", time.Now().UnixNano())})
	if err != nil {
		return nil, err
	}
	_ = respSession
	return map[string]any{"type": 4, "data": map[string]any{"content": response}}, nil
}

func ReadBody(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	return io.ReadAll(r.Body)
}

func NewDiscordAdapter(cfg config.DiscordChannelConfig, appendEvent func(eventType string, sessionID string, payload map[string]any)) *DiscordAdapter {
	apiBaseURL := strings.TrimRight(strings.TrimSpace(cfg.APIBaseURL), "/")
	if apiBaseURL == "" {
		apiBaseURL = "https://discord.com/api/v10"
	}
	return &DiscordAdapter{
		base:        NewBaseAdapter("discord", cfg.Enabled && cfg.BotToken != ""),
		config:      cfg,
		client:      &http.Client{Timeout: 20 * time.Second},
		appendEvent: appendEvent,
		apiBaseURL:  apiBaseURL,
		processed:   make(map[string]time.Time),
	}
}

func (a *DiscordAdapter) Name() string {
	return "discord"
}

func (a *DiscordAdapter) Enabled() bool {
	return a.config.Enabled && strings.TrimSpace(a.config.BotToken) != "" && strings.TrimSpace(a.config.DefaultChannel) != ""
}

func (a *DiscordAdapter) Status() Status {
	status := a.base.Status()
	status.Enabled = a.Enabled()
	return status
}

func (a *DiscordAdapter) Run(ctx context.Context, handle InboundHandler) error {
	a.base.SetRunning(true)
	defer a.base.SetRunning(false)
	if a.config.UseGatewayWS {
		if err := a.runGatewayWS(ctx, handle); err == nil {
			return nil
		} else {
			a.append("channel.discord.gateway.error", "", map[string]any{"error": err.Error()})
		}
	}
	interval := time.Duration(a.config.PollEvery) * time.Second
	if interval <= 0 {
		interval = 3 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if err := a.pollOnce(ctx, handle); err != nil {
			a.base.SetError(err)
			a.append("channel.discord.error", "", map[string]any{"error": err.Error()})
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

func (a *DiscordAdapter) runGatewayWS(ctx context.Context, handle InboundHandler) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://discord.com/api/gateway/bot", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bot "+a.config.BotToken)
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var gateway struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&gateway); err != nil {
		return err
	}
	if strings.TrimSpace(gateway.URL) == "" {
		return fmt.Errorf("discord gateway URL missing")
	}
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, gateway.URL+"?v=10&encoding=json", nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.WriteJSON(map[string]any{"op": 2, "d": map[string]any{"token": a.config.BotToken, "intents": 4609, "properties": map[string]string{"$os": "windows", "$browser": "anyclaw", "$device": "anyclaw"}}})
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		var packet struct {
			Op int             `json:"op"`
			T  string          `json:"t"`
			D  json.RawMessage `json:"d"`
		}
		if err := conn.ReadJSON(&packet); err != nil {
			return err
		}
		if packet.Op == 10 {
			_ = conn.WriteJSON(map[string]any{"op": 1, "d": nil})
			continue
		}
		if packet.T == "MESSAGE_CREATE" || packet.T == "THREAD_CREATE" || packet.T == "TYPING_START" || packet.T == "PRESENCE_UPDATE" || packet.T == "INTERACTION_CREATE" {
			_ = a.handleGatewayEvent(ctx, packet.T, packet.D, handle)
		}
	}
}

func (a *DiscordAdapter) handleGatewayEvent(ctx context.Context, eventType string, raw json.RawMessage, handle InboundHandler) error {
	a.append("channel.discord.gateway.event", "", map[string]any{"event": eventType})
	if eventType == "MESSAGE_CREATE" {
		var msg struct {
			ID        string `json:"id"`
			Content   string `json:"content"`
			ChannelID string `json:"channel_id"`
			GuildID   string `json:"guild_id"`
			Author    struct {
				ID       string `json:"id"`
				Username string `json:"username"`
				Bot      bool   `json:"bot"`
			} `json:"author"`
			Attachments []struct {
				ID          string `json:"id"`
				URL         string `json:"url"`
				ProxyURL    string `json:"proxy_url"`
				Filename    string `json:"filename"`
				ContentType string `json:"content_type"`
				Size        int    `json:"size"`
			} `json:"attachments"`
			MessageReference struct {
				MessageID string `json:"message_id"`
				ChannelID string `json:"channel_id"`
			} `json:"message_reference"`
		}
		if err := json.Unmarshal(raw, &msg); err != nil {
			return err
		}
		if msg.Author.Bot || a.seen(msg.ID) {
			return nil
		}

		meta := map[string]string{
			"channel":      "discord",
			"channel_id":   msg.ChannelID,
			"guild_id":     msg.GuildID,
			"user_id":      msg.Author.ID,
			"username":     msg.Author.Username,
			"reply_target": msg.ChannelID,
			"message_id":   msg.ID,
			"sender":       msg.Author.Username,
		}

		audioURL, audioMIME := a.findAudioAttachment(msg.Attachments)
		if audioURL != "" {
			meta["message_type"] = "voice_note"
			meta["audio_url"] = audioURL
			meta["audio_mime"] = audioMIME
			if strings.TrimSpace(msg.Content) != "" {
				meta["caption"] = strings.TrimSpace(msg.Content)
			}

			respSession, response, err := handle(ctx, "", audioURL, meta)
			if err != nil {
				return err
			}
			_ = respSession
			return a.sendMessage(ctx, msg.ChannelID, "", response)
		}

		if strings.TrimSpace(msg.Content) == "" {
			return nil
		}

		respSession, response, err := handle(ctx, "", msg.Content, meta)
		if err != nil {
			return err
		}
		_ = respSession
		return a.sendMessage(ctx, msg.ChannelID, "", response)
	}
	return nil
}

func (a *DiscordAdapter) findAudioAttachment(attachments []struct {
	ID          string `json:"id"`
	URL         string `json:"url"`
	ProxyURL    string `json:"proxy_url"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int    `json:"size"`
}) (string, string) {
	for _, att := range attachments {
		ct := strings.ToLower(att.ContentType)
		fn := strings.ToLower(att.Filename)
		if strings.HasPrefix(ct, "audio/") {
			url := att.URL
			if url == "" {
				url = att.ProxyURL
			}
			return url, att.ContentType
		}
		if strings.HasSuffix(fn, ".ogg") || strings.HasSuffix(fn, ".mp3") || strings.HasSuffix(fn, ".wav") || strings.HasSuffix(fn, ".flac") || strings.HasSuffix(fn, ".m4a") || strings.HasSuffix(fn, ".webm") {
			url := att.URL
			if url == "" {
				url = att.ProxyURL
			}
			mime := "audio/unknown"
			switch {
			case strings.HasSuffix(fn, ".ogg"):
				mime = "audio/ogg"
			case strings.HasSuffix(fn, ".mp3"):
				mime = "audio/mpeg"
			case strings.HasSuffix(fn, ".wav"):
				mime = "audio/wav"
			case strings.HasSuffix(fn, ".flac"):
				mime = "audio/flac"
			case strings.HasSuffix(fn, ".m4a"):
				mime = "audio/mp4"
			case strings.HasSuffix(fn, ".webm"):
				mime = "audio/webm"
			}
			return url, mime
		}
	}
	return "", ""
}

func (a *DiscordAdapter) pollOnce(ctx context.Context, handle InboundHandler) error {
	url := fmt.Sprintf("%s/channels/%s/messages?limit=20", a.apiBaseURL, a.config.DefaultChannel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bot "+a.config.BotToken)
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("discord fetch failed: %s", resp.Status)
	}

	var messages []struct {
		ID       string `json:"id"`
		Content  string `json:"content"`
		GuildID  string `json:"guild_id"`
		ParentID string `json:"message_reference.message_id"`
		Author   struct {
			ID       string `json:"id"`
			Username string `json:"username"`
			Bot      bool   `json:"bot"`
		} `json:"author"`
		Attachments []struct {
			ID          string `json:"id"`
			URL         string `json:"url"`
			ProxyURL    string `json:"proxy_url"`
			Filename    string `json:"filename"`
			ContentType string `json:"content_type"`
			Size        int    `json:"size"`
		} `json:"attachments"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&messages); err != nil {
		return err
	}

	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.ID == "" || msg.ID == a.latestID || msg.Author.Bot || a.seen(msg.ID) {
			continue
		}
		a.latestID = msg.ID

		meta := map[string]string{
			"channel":      "discord",
			"channel_id":   a.config.DefaultChannel,
			"guild_id":     msg.GuildID,
			"user_id":      msg.Author.ID,
			"username":     msg.Author.Username,
			"reply_target": a.config.DefaultChannel,
			"message_id":   msg.ID,
			"sender":       msg.Author.Username,
		}

		audioURL, audioMIME := a.findAudioAttachment(msg.Attachments)
		if audioURL != "" {
			meta["message_type"] = "voice_note"
			meta["audio_url"] = audioURL
			meta["audio_mime"] = audioMIME
			if strings.TrimSpace(msg.Content) != "" {
				meta["caption"] = strings.TrimSpace(msg.Content)
			}

			threadID := strings.TrimSpace(msg.ParentID)
			replyTarget := a.config.DefaultChannel
			if threadID != "" {
				replyTarget = threadID
			}

			sessionID, response, err := handle(ctx, "", audioURL, meta)
			if err != nil {
				return err
			}
			if err := a.sendMessage(ctx, replyTarget, threadID, response); err != nil {
				return err
			}
			a.base.MarkActivity()
			a.append("channel.discord.voice", sessionID, map[string]any{
				"channel":      a.config.DefaultChannel,
				"user":         msg.Author.Username,
				"user_id":      msg.Author.ID,
				"message_type": "voice_note",
				"audio_url":    audioURL,
				"audio_mime":   audioMIME,
			})
			continue
		}

		if strings.TrimSpace(msg.Content) == "" {
			continue
		}

		threadID := strings.TrimSpace(msg.ParentID)
		replyTarget := a.config.DefaultChannel
		if threadID != "" {
			replyTarget = threadID
		}
		sessionID, response, err := handle(ctx, "", msg.Content, meta)
		if err != nil {
			return err
		}
		if err := a.sendMessage(ctx, replyTarget, threadID, response); err != nil {
			return err
		}
		a.base.MarkActivity()
		a.append("channel.discord.message", sessionID, map[string]any{
			"channel": a.config.DefaultChannel,
			"user":    msg.Author.Username,
			"user_id": msg.Author.ID,
			"text":    msg.Content,
		})
	}
	return nil
}

func (a *DiscordAdapter) sendMessage(ctx context.Context, target string, threadID string, text string) error {
	target = strings.TrimSpace(target)
	if target == "" {
		target = strings.TrimSpace(a.config.DefaultChannel)
	}
	bodyMap := map[string]any{"content": text}
	if strings.TrimSpace(threadID) != "" && threadID != target {
		bodyMap["message_reference"] = map[string]any{"message_id": threadID}
	}
	body, _ := json.Marshal(bodyMap)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/channels/%s/messages", a.apiBaseURL, target), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bot "+a.config.BotToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("discord send failed: %s", resp.Status)
	}
	return nil
}

func (a *DiscordAdapter) append(eventType string, sessionID string, payload map[string]any) {
	if a.appendEvent != nil {
		a.appendEvent(eventType, sessionID, payload)
	}
}

func (a *DiscordAdapter) seen(id string) bool {
	for key, ts := range a.processed {
		if time.Since(ts) > 30*time.Minute {
			delete(a.processed, key)
		}
	}
	if _, ok := a.processed[id]; ok {
		return true
	}
	a.processed[id] = time.Now().UTC()
	return false
}

func (a *DiscordAdapter) sendStreamingMessage(ctx context.Context, target string, threadID string, streamFn func(onChunk func(chunk string)) error) error {
	streamInterval := time.Duration(a.config.StreamInterval) * time.Millisecond
	if streamInterval <= 0 {
		streamInterval = 500 * time.Millisecond
	}

	initialMsgID, err := a.sendMessageWithResult(ctx, target, threadID, "\u200B")
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
			a.editMessage(ctx, target, initialMsgID, text)
		}
	}

	if err := streamFn(onChunk); err != nil {
		mu.Lock()
		final := accumulated.String()
		mu.Unlock()
		a.editMessage(ctx, target, initialMsgID, final+"\n\n[Error: "+err.Error()+"]")
		return err
	}

	mu.Lock()
	final := accumulated.String()
	mu.Unlock()
	a.editMessage(ctx, target, initialMsgID, final)
	return nil
}

func (a *DiscordAdapter) sendMessageWithResult(ctx context.Context, target string, threadID string, text string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		target = strings.TrimSpace(a.config.DefaultChannel)
	}
	bodyMap := map[string]any{"content": text}
	if strings.TrimSpace(threadID) != "" && threadID != target {
		bodyMap["message_reference"] = map[string]any{"message_id": threadID}
	}
	body, _ := json.Marshal(bodyMap)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/channels/%s/messages", a.apiBaseURL, target), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bot "+a.config.BotToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("discord send failed: %s", resp.Status)
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.ID, nil
}

func (a *DiscordAdapter) editMessage(ctx context.Context, target string, messageID string, text string) error {
	if messageID == "" {
		return nil
	}
	target = strings.TrimSpace(target)
	if target == "" {
		target = strings.TrimSpace(a.config.DefaultChannel)
	}
	truncated := text
	if len([]rune(truncated)) > 2000 {
		truncated = string([]rune(truncated)[:1997]) + "..."
	}
	body := map[string]any{"content": truncated}
	payload, _ := json.Marshal(body)
	url := fmt.Sprintf("%s/channels/%s/messages/%s", a.apiBaseURL, target, messageID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bot "+a.config.BotToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	return nil
}

func (a *DiscordAdapter) RunStream(ctx context.Context, handle StreamChunkHandler) error {
	a.base.SetRunning(true)
	defer a.base.SetRunning(false)
	if a.config.UseGatewayWS {
		if err := a.runGatewayWSStream(ctx, handle); err == nil {
			return nil
		} else {
			a.append("channel.discord.gateway.error", "", map[string]any{"error": err.Error()})
		}
	}
	interval := time.Duration(a.config.PollEvery) * time.Second
	if interval <= 0 {
		interval = 3 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if err := a.pollOnceStream(ctx, handle); err != nil {
			a.base.SetError(err)
			a.append("channel.discord.error", "", map[string]any{"error": err.Error()})
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

func (a *DiscordAdapter) runGatewayWSStream(ctx context.Context, handle StreamChunkHandler) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://discord.com/api/gateway/bot", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bot "+a.config.BotToken)
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var gateway struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&gateway); err != nil {
		return err
	}
	if strings.TrimSpace(gateway.URL) == "" {
		return fmt.Errorf("discord gateway URL missing")
	}
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, gateway.URL+"?v=10&encoding=json", nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.WriteJSON(map[string]any{"op": 2, "d": map[string]any{"token": a.config.BotToken, "intents": 4609, "properties": map[string]string{"$os": "windows", "$browser": "anyclaw", "$device": "anyclaw"}}})
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		var packet struct {
			Op int             `json:"op"`
			T  string          `json:"t"`
			D  json.RawMessage `json:"d"`
		}
		if err := conn.ReadJSON(&packet); err != nil {
			return err
		}
		if packet.Op == 10 {
			_ = conn.WriteJSON(map[string]any{"op": 1, "d": nil})
			continue
		}
		if packet.T == "MESSAGE_CREATE" || packet.T == "THREAD_CREATE" {
			_ = a.handleGatewayEventStream(ctx, packet.T, packet.D, handle)
		}
	}
}

func (a *DiscordAdapter) handleGatewayEventStream(ctx context.Context, eventType string, raw json.RawMessage, handle StreamChunkHandler) error {
	if eventType == "MESSAGE_CREATE" {
		var msg struct {
			ID        string `json:"id"`
			Content   string `json:"content"`
			ChannelID string `json:"channel_id"`
			GuildID   string `json:"guild_id"`
			Author    struct {
				ID       string `json:"id"`
				Username string `json:"username"`
				Bot      bool   `json:"bot"`
			} `json:"author"`
			MessageReference struct {
				MessageID string `json:"message_id"`
				ChannelID string `json:"channel_id"`
			} `json:"message_reference"`
		}
		if err := json.Unmarshal(raw, &msg); err != nil {
			return err
		}
		if msg.Author.Bot || a.seen(msg.ID) {
			return nil
		}
		if strings.TrimSpace(msg.Content) == "" {
			return nil
		}

		meta := map[string]string{
			"channel":      "discord",
			"channel_id":   msg.ChannelID,
			"guild_id":     msg.GuildID,
			"user_id":      msg.Author.ID,
			"username":     msg.Author.Username,
			"reply_target": msg.ChannelID,
			"message_id":   msg.ID,
			"sender":       msg.Author.Username,
		}

		sessionID := ""
		err := a.sendStreamingMessage(ctx, msg.ChannelID, "", func(onChunk func(chunk string)) error {
			var err error
			sessionID, err = handle(ctx, sessionID, msg.Content, meta, func(chunk string) error {
				onChunk(chunk)
				return nil
			})
			return err
		})
		if err != nil {
			return err
		}
		a.base.MarkActivity()
		return nil
	}
	return nil
}

func (a *DiscordAdapter) pollOnceStream(ctx context.Context, handle StreamChunkHandler) error {
	url := fmt.Sprintf("%s/channels/%s/messages?limit=20", a.apiBaseURL, a.config.DefaultChannel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bot "+a.config.BotToken)
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("discord fetch failed: %s", resp.Status)
	}

	var messages []struct {
		ID       string `json:"id"`
		Content  string `json:"content"`
		GuildID  string `json:"guild_id"`
		ParentID string `json:"message_reference.message_id"`
		Author   struct {
			ID       string `json:"id"`
			Username string `json:"username"`
			Bot      bool   `json:"bot"`
		} `json:"author"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&messages); err != nil {
		return err
	}

	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.ID == "" || msg.ID == a.latestID || msg.Author.Bot || a.seen(msg.ID) {
			continue
		}
		a.latestID = msg.ID

		if strings.TrimSpace(msg.Content) == "" {
			continue
		}

		meta := map[string]string{
			"channel":      "discord",
			"channel_id":   a.config.DefaultChannel,
			"guild_id":     msg.GuildID,
			"user_id":      msg.Author.ID,
			"username":     msg.Author.Username,
			"reply_target": a.config.DefaultChannel,
			"message_id":   msg.ID,
			"sender":       msg.Author.Username,
		}

		threadID := strings.TrimSpace(msg.ParentID)
		replyTarget := a.config.DefaultChannel
		if threadID != "" {
			replyTarget = threadID
		}
		sessionID := ""

		err := a.sendStreamingMessage(ctx, replyTarget, threadID, func(onChunk func(chunk string)) error {
			var err error
			sessionID, err = handle(ctx, sessionID, msg.Content, meta, func(chunk string) error {
				onChunk(chunk)
				return nil
			})
			return err
		})
		if err != nil {
			return err
		}
		a.base.MarkActivity()
		a.append("channel.discord.message", sessionID, map[string]any{
			"channel":   a.config.DefaultChannel,
			"user":      msg.Author.Username,
			"user_id":   msg.Author.ID,
			"text":      msg.Content,
			"streaming": true,
		})
	}
	return nil
}
