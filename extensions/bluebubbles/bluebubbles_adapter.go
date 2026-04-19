package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Config struct {
	ServerAddress   string `json:"server_address"`
	Password        string `json:"password"`
	UseWebSocket    bool   `json:"use_websocket"`
	PollInterval    int    `json:"poll_interval"`
	GroupNamePrefix string `json:"group_name_prefix"`
	Port            int    `json:"port"`
}

type BlueBubblesExtension struct {
	config      Config
	client      *http.Client
	lastMessage time.Time
	mu          sync.RWMutex
}

func NewBlueBubblesExtension(cfg Config) *BlueBubblesExtension {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 5
	}
	if cfg.Port <= 0 {
		cfg.Port = 8080
	}
	if !strings.HasPrefix(cfg.ServerAddress, "http") {
		cfg.ServerAddress = "http://" + cfg.ServerAddress
	}
	cfg.ServerAddress = strings.TrimRight(cfg.ServerAddress, "/")
	return &BlueBubblesExtension{
		config: cfg,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (e *BlueBubblesExtension) Run(ctx context.Context) error {
	if e.config.UseWebSocket {
		return e.runWebSocketMode(ctx)
	}
	return e.runPollMode(ctx)
}

func (e *BlueBubblesExtension) runPollMode(ctx context.Context) error {
	interval := time.Duration(e.config.PollInterval) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	e.lastMessage = time.Now()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}

		if err := e.pollMessages(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "bluebubbles poll error: %v\n", err)
		}
	}
}

func (e *BlueBubblesExtension) runWebSocketMode(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/bluebubbles", e.handleWebhook)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", e.config.Port),
		Handler: mux,
	}

	errCh := make(chan error, 1)
	go func() {
		fmt.Fprintf(os.Stderr, "bluebubbles webhook listening on :%d\n", e.config.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

func (e *BlueBubblesExtension) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var payload struct {
		Type    string `json:"type"`
		Message struct {
			GUID           string `json:"guid"`
			RowID          int64  `json:"rowid"`
			Handle         string `json:"handle"`
			HandleID       int    `json:"handle_id"`
			IsFromMe       bool   `json:"is_from_me"`
			Text           string `json:"text"`
			Subject        string `json:"subject"`
			Date           int64  `json:"date"`
			DateRead       int64  `json:"date_read"`
			ChatGUID       string `json:"chat_guid"`
			GroupTitle     string `json:"group_title"`
			AssociatedGUID string `json:"associated_message_guid"`
			Attachments    []struct {
				GUID       string `json:"guid"`
				TransferID string `json:"transfer_guid"`
				FileName   string `json:"file_name"`
				MimeType   string `json:"mime_type"`
			} `json:"attachments"`
		} `json:"message"`
		Chat struct {
			GUID        string `json:"guid"`
			DisplayName string `json:"display_name"`
			IsGroup     bool   `json:"is_group"`
			Members     []struct {
				Address string `json:"address"`
				Name    string `json:"name"`
			} `json:"members"`
		} `json:"chat"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	if payload.Type != "new-message" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if payload.Message.IsFromMe {
		w.WriteHeader(http.StatusOK)
		return
	}

	text := strings.TrimSpace(payload.Message.Text)
	if text == "" {
		if len(payload.Message.Attachments) > 0 {
			text = fmt.Sprintf("[Attachment: %s]", payload.Message.Attachments[0].FileName)
		} else {
			w.WriteHeader(http.StatusOK)
			return
		}
	}

	chatID := payload.Message.ChatGUID
	if chatID == "" {
		chatID = payload.Chat.GUID
	}
	senderID := payload.Message.Handle
	chatName := payload.Chat.DisplayName
	if chatName == "" {
		chatName = payload.Message.GroupTitle
	}
	if e.config.GroupNamePrefix != "" && payload.Chat.IsGroup {
		chatName = e.config.GroupNamePrefix + chatName
	}

	input := map[string]any{
		"action":    "message",
		"channel":   "bluebubbles",
		"chat_id":   chatID,
		"text":      text,
		"user_id":   senderID,
		"chat_name": chatName,
		"is_group":  payload.Chat.IsGroup,
		"guid":      payload.Message.GUID,
		"rowid":     payload.Message.RowID,
	}
	data, _ := json.Marshal(input)
	fmt.Println(string(data))

	var response map[string]any
	if err := json.NewDecoder(os.Stdin).Decode(&response); err != nil {
		fmt.Fprintf(os.Stderr, "failed to read response: %v\n", err)
		w.WriteHeader(http.StatusOK)
		return
	}

	if reply, ok := response["text"].(string); ok && reply != "" {
		if err := e.sendMessage(chatID, senderID, reply); err != nil {
			fmt.Fprintf(os.Stderr, "bluebubbles send error: %v\n", err)
		}
	}

	w.WriteHeader(http.StatusOK)
}

func (e *BlueBubblesExtension) pollMessages(ctx context.Context) error {
	e.mu.RLock()
	lastMsg := e.lastMessage
	e.mu.RUnlock()

	since := lastMsg.Add(-2*time.Second).UnixNano() / int64(time.Millisecond)

	u := fmt.Sprintf("%s/api/v1/message/query?password=%s&after=%d&limit=50&sort=date DESC",
		e.config.ServerAddress, url.QueryEscape(e.config.Password), since)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("query failed: %s: %s", resp.Status, string(body))
	}

	var result struct {
		Status int `json:"status"`
		Data   struct {
			Messages []struct {
				GUID     string `json:"guid"`
				RowID    int64  `json:"rowid"`
				IsFromMe bool   `json:"is_from_me"`
				Text     string `json:"text"`
				Date     int64  `json:"date"`
				HandleID int    `json:"handle_id"`
				ChatGUID string `json:"chat_guid"`
			} `json:"messages"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	for _, msg := range result.Data.Messages {
		if msg.IsFromMe {
			continue
		}

		text := strings.TrimSpace(msg.Text)
		if text == "" {
			continue
		}

		handle, err := e.getHandle(ctx, msg.HandleID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to get handle: %v\n", err)
			handle = fmt.Sprintf("handle:%d", msg.HandleID)
		}

		chatID := msg.ChatGUID
		if chatID == "" {
			chatID = fmt.Sprintf("imessage;-;+%s", handle)
		}

		input := map[string]any{
			"action":  "message",
			"channel": "bluebubbles",
			"chat_id": chatID,
			"text":    text,
			"user_id": handle,
			"guid":    msg.GUID,
			"rowid":   msg.RowID,
		}
		data, _ := json.Marshal(input)
		fmt.Println(string(data))

		var response map[string]any
		if err := json.NewDecoder(os.Stdin).Decode(&response); err != nil {
			fmt.Fprintf(os.Stderr, "failed to read response: %v\n", err)
			continue
		}

		if reply, ok := response["text"].(string); ok && reply != "" {
			if err := e.sendMessage(chatID, handle, reply); err != nil {
				fmt.Fprintf(os.Stderr, "bluebubbles send error: %v\n", err)
			}
		}

		msgTime := time.Unix(0, msg.Date*int64(time.Millisecond))
		e.mu.Lock()
		if msgTime.After(e.lastMessage) {
			e.lastMessage = msgTime
		}
		e.mu.Unlock()
	}

	return nil
}

func (e *BlueBubblesExtension) getHandle(ctx context.Context, handleID int) (string, error) {
	u := fmt.Sprintf("%s/api/v1/handle/%d?password=%s",
		e.config.ServerAddress, handleID, url.QueryEscape(e.config.Password))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Status int `json:"status"`
		Data   struct {
			Address string `json:"address"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if result.Data.Address == "" {
		return fmt.Sprintf("handle:%d", handleID), nil
	}
	return result.Data.Address, nil
}

func (e *BlueBubblesExtension) sendMessage(chatGUID, recipient, text string) error {
	u := fmt.Sprintf("%s/api/v1/message/text?password=%s",
		e.config.ServerAddress, url.QueryEscape(e.config.Password))

	payload := map[string]any{
		"text": text,
	}
	if chatGUID != "" {
		payload["chatGuid"] = chatGUID
	} else if recipient != "" {
		payload["address"] = recipient
	}
	body, _ := json.Marshal(payload)

	resp, err := e.client.Post(u, "application/json", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("send failed: %s: %s", resp.Status, string(respBody))
	}
	return nil
}

func main() {
	configJSON := os.Getenv("ANYCLAW_EXTENSION_CONFIG")
	if configJSON == "" {
		fmt.Fprintln(os.Stderr, "missing ANYCLAW_EXTENSION_CONFIG")
		os.Exit(1)
	}

	var cfg Config
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "invalid config: %v\n", err)
		os.Exit(1)
	}

	ext := NewBlueBubblesExtension(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	if err := ext.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "extension error: %v\n", err)
		os.Exit(1)
	}
}
