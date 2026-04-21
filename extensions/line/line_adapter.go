package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

type Config struct {
	ChannelAccessToken string `json:"channel_access_token"`
	ChannelSecret      string `json:"channel_secret"`
	Port               int    `json:"port"`
}

type LINEExtension struct {
	config Config
	client *http.Client
}

func NewLINEExtension(cfg Config) *LINEExtension {
	if cfg.Port <= 0 {
		cfg.Port = 8080
	}
	return &LINEExtension{
		config: cfg,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (e *LINEExtension) Run(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/line", e.handleWebhook)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", e.config.Port),
		Handler: mux,
	}

	errCh := make(chan error, 1)
	go func() {
		fmt.Fprintf(os.Stderr, "line webhook listening on :%d\n", e.config.Port)
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

func (e *LINEExtension) handleWebhook(w http.ResponseWriter, r *http.Request) {
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

	if !e.verifySignature(body, r.Header.Get("X-Line-Signature")) {
		http.Error(w, "invalid signature", http.StatusForbidden)
		return
	}

	var payload struct {
		Events []struct {
			Type       string `json:"type"`
			ReplyToken string `json:"replyToken"`
			Source     struct {
				Type    string `json:"type"`
				UserID  string `json:"userId"`
				GroupID string `json:"groupId"`
				RoomID  string `json:"roomId"`
			} `json:"source"`
			Message struct {
				Type string `json:"type"`
				ID   string `json:"id"`
				Text string `json:"text"`
			} `json:"message"`
			Timestamp int64 `json:"timestamp"`
		} `json:"events"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	for _, event := range payload.Events {
		if event.Type != "message" {
			continue
		}
		if event.Message.Type != "text" || event.Message.Text == "" {
			continue
		}

		senderID := event.Source.UserID
		chatID := event.Source.UserID
		if event.Source.GroupID != "" {
			chatID = event.Source.GroupID
		} else if event.Source.RoomID != "" {
			chatID = event.Source.RoomID
		}

		input := map[string]any{
			"action":      "message",
			"channel":     "line",
			"chat_id":     chatID,
			"text":        event.Message.Text,
			"user_id":     senderID,
			"reply_token": event.ReplyToken,
			"msg_type":    event.Message.Type,
		}
		data, _ := json.Marshal(input)
		fmt.Println(string(data))

		var response map[string]any
		if err := json.NewDecoder(os.Stdin).Decode(&response); err != nil {
			fmt.Fprintf(os.Stderr, "failed to read response: %v\n", err)
			continue
		}

		if reply, ok := response["text"].(string); ok && reply != "" {
			if err := e.replyTextMessage(event.ReplyToken, reply); err != nil {
				fmt.Fprintf(os.Stderr, "line send message error: %v\n", err)
			}
		}
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("success"))
}

func (e *LINEExtension) verifySignature(body []byte, signature string) bool {
	if e.config.ChannelSecret == "" {
		return true
	}
	mac := hmac.New(sha256.New, []byte(e.config.ChannelSecret))
	mac.Write(body)
	computed := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(computed), []byte(signature))
}

func (e *LINEExtension) replyTextMessage(replyToken, text string) error {
	u := "https://api.line.me/v2/bot/message/reply"
	payload := map[string]any{
		"replyToken": replyToken,
		"messages": []map[string]any{
			{"type": "text", "text": text},
		},
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest(http.MethodPost, u, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+e.config.ChannelAccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("line reply error: %s: %s", resp.Status, string(respBody))
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

	ext := NewLINEExtension(cfg)
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
