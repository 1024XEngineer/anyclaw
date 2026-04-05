package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Config struct {
	Mode           string `json:"mode"`
	BridgeURL      string `json:"bridge_url,omitempty"`
	BridgePassword string `json:"bridge_password,omitempty"`
	ChatDBPath     string `json:"chat_db_path,omitempty"`
	PollInterval   int    `json:"poll_interval"`
	Port           int    `json:"port"`
}

type IMessageExtension struct {
	config    Config
	client    *http.Client
	lastRowID int64
	mu        sync.Mutex
}

func NewIMessageExtension(cfg Config) *IMessageExtension {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 5
	}
	if cfg.Port <= 0 {
		cfg.Port = 8080
	}
	if cfg.BridgeURL == "" {
		cfg.BridgeURL = "http://localhost:12345"
	}
	return &IMessageExtension{
		config: cfg,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (e *IMessageExtension) Run(ctx context.Context) error {
	switch e.config.Mode {
	case "bridge":
		return e.runBridgeMode(ctx)
	case "database":
		return e.runDatabaseMode(ctx)
	default:
		return fmt.Errorf("unknown mode: %s (use 'bridge' or 'database')", e.config.Mode)
	}
}

func (e *IMessageExtension) runBridgeMode(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/imessage", e.handleBridgeWebhook)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", e.config.Port),
		Handler: mux,
	}

	errCh := make(chan error, 1)
	go func() {
		fmt.Fprintf(os.Stderr, "imessage bridge webhook listening on :%d\n", e.config.Port)
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

func (e *IMessageExtension) handleBridgeWebhook(w http.ResponseWriter, r *http.Request) {
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
			GUID                  string `json:"guid"`
			RowID                 int64  `json:"rowid"`
			Handle                string `json:"handle"`
			HandleID              int    `json:"handle_id"`
			IsFromMe              bool   `json:"is_from_me"`
			Text                  string `json:"text"`
			Subject               string `json:"subject"`
			Date                  int64  `json:"date"`
			DateRead              int64  `json:"date_read"`
			ChatGUID              string `json:"chat_guid"`
			GroupTitle            string `json:"group_title"`
			AssociatedMessageGUID string `json:"associated_message_guid"`
			ExpressiveSend        struct {
				Type string `json:"type"`
			} `json:"expressive_send"`
		} `json:"message"`
		Chat struct {
			GUID        string `json:"guid"`
			DisplayName string `json:"display_name"`
			IsGroup     bool   `json:"is_group"`
			Members     []struct {
				Address string `json:"address"`
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
		w.WriteHeader(http.StatusOK)
		return
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

	input := map[string]any{
		"action":    "message",
		"channel":   "imessage",
		"chat_id":   chatID,
		"text":      text,
		"user_id":   senderID,
		"chat_name": chatName,
		"is_group":  payload.Chat.IsGroup,
		"guid":      payload.Message.GUID,
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
		if err := e.sendBridgeMessage(chatID, senderID, reply); err != nil {
			fmt.Fprintf(os.Stderr, "imessage send error: %v\n", err)
		}
	}

	w.WriteHeader(http.StatusOK)
}

func (e *IMessageExtension) sendBridgeMessage(chatGUID, recipient, text string) error {
	u := fmt.Sprintf("%s/api/v1/message", strings.TrimRight(e.config.BridgeURL, "/"))

	payload := map[string]any{
		"method": "text",
		"text":   text,
	}
	if chatGUID != "" {
		payload["chatGuid"] = chatGUID
	} else if recipient != "" {
		payload["address"] = recipient
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest(http.MethodPost, u, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if e.config.BridgePassword != "" {
		req.Header.Set("Authorization", "Bearer "+e.config.BridgePassword)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("bridge send error: %s: %s", resp.Status, string(respBody))
	}
	return nil
}

func (e *IMessageExtension) runDatabaseMode(ctx context.Context) error {
	if e.config.ChatDBPath == "" {
		e.config.ChatDBPath = os.Getenv("HOME") + "/Library/Messages/chat.db"
	}

	interval := time.Duration(e.config.PollInterval) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	e.lastRowID = e.getLastRowID()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}

		if err := e.pollDatabase(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "imessage poll error: %v\n", err)
		}
	}
}

func (e *IMessageExtension) getLastRowID() int64 {
	cmd := exec.Command("sqlite3", e.config.ChatDBPath, "SELECT MAX(rowid) FROM message;")
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	var rowID int64
	fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &rowID)
	return rowID
}

func (e *IMessageExtension) pollDatabase(ctx context.Context) error {
	e.mu.Lock()
	lastRowID := e.lastRowID
	e.mu.Unlock()

	query := fmt.Sprintf(
		"SELECT m.rowid, m.guid, m.text, m.is_from_me, m.date, h.id, c.guid as chat_guid, c.display_name "+
			"FROM message m "+
			"LEFT JOIN handle h ON m.handle_id = h.ROWID "+
			"LEFT JOIN chat_message_join cmj ON m.ROWID = cmj.message_id "+
			"LEFT JOIN chat c ON cmj.chat_id = c.ROWID "+
			"WHERE m.rowid > %d AND m.is_from_me = 0 AND m.text IS NOT NULL AND m.text != '' "+
			"ORDER BY m.rowid ASC;",
		lastRowID,
	)

	cmd := exec.CommandContext(ctx, "sqlite3", "-json", e.config.ChatDBPath, query)
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("sqlite3 query failed: %w", err)
	}

	var messages []struct {
		RowID    int64  `json:"rowid"`
		GUID     string `json:"guid"`
		Text     string `json:"text"`
		IsFromMe int    `json:"is_from_me"`
		Date     int64  `json:"date"`
		ID       string `json:"id"`
		ChatGUID string `json:"chat_guid"`
		ChatName string `json:"display_name"`
	}
	if err := json.Unmarshal(out, &messages); err != nil {
		return fmt.Errorf("failed to parse messages: %w", err)
	}

	for _, msg := range messages {
		text := strings.TrimSpace(msg.Text)
		if text == "" {
			continue
		}

		chatID := msg.ChatGUID
		if chatID == "" {
			chatID = "imessage;-;" + msg.ID
		}
		senderID := msg.ID

		input := map[string]any{
			"action":    "message",
			"channel":   "imessage",
			"chat_id":   chatID,
			"text":      text,
			"user_id":   senderID,
			"chat_name": msg.ChatName,
			"guid":      msg.GUID,
		}
		data, _ := json.Marshal(input)
		fmt.Println(string(data))

		var response map[string]any
		if err := json.NewDecoder(os.Stdin).Decode(&response); err != nil {
			fmt.Fprintf(os.Stderr, "failed to read response: %v\n", err)
			continue
		}

		if reply, ok := response["text"].(string); ok && reply != "" {
			if err := e.sendDatabaseMessage(senderID, reply); err != nil {
				fmt.Fprintf(os.Stderr, "imessage send error: %v\n", err)
			}
		}

		e.mu.Lock()
		if msg.RowID > e.lastRowID {
			e.lastRowID = msg.RowID
		}
		e.mu.Unlock()
	}

	return nil
}

func (e *IMessageExtension) sendDatabaseMessage(recipient, text string) error {
	escapedText := strings.ReplaceAll(text, "'", "'\"'\"'")
	escapedRecipient := strings.ReplaceAll(recipient, "'", "'\"'\"'")

	script := fmt.Sprintf(`
		tell application "Messages"
			set targetService to 1st service whose service type = iMessage
			set targetBuddy to buddy "%s" of targetService
			send "%s" to targetBuddy
		end tell
	`, escapedRecipient, escapedText)

	cmd := exec.Command("osascript", "-e", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("osascript failed: %w: %s", err, string(output))
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

	ext := NewIMessageExtension(cfg)
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
