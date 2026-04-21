package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	BotToken      string `json:"bot_token"`
	AppToken      string `json:"app_token,omitempty"`
	SigningSecret string `json:"signing_secret,omitempty"`
	Port          int    `json:"port,omitempty"`
}

type SlackExtension struct {
	config  Config
	client  *http.Client
	baseURL string
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
}

func NewSlackExtension(cfg Config) *SlackExtension {
	if cfg.Port <= 0 {
		cfg.Port = 8080
	}
	return &SlackExtension{
		config:  cfg,
		client:  &http.Client{Timeout: 10 * time.Second},
		baseURL: "https://slack.com/api",
		stdin:   os.Stdin,
		stdout:  os.Stdout,
		stderr:  os.Stderr,
	}
}

func (e *SlackExtension) Run(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/slack/events", e.handleWebhook)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", e.config.Port),
		Handler: mux,
	}

	errCh := make(chan error, 1)
	go func() {
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

func (e *SlackExtension) handleWebhook(w http.ResponseWriter, r *http.Request) {
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

	if !e.verifySignature(body, r.Header.Get("X-Slack-Signature"), r.Header.Get("X-Slack-Request-Timestamp")) {
		http.Error(w, "invalid signature", http.StatusForbidden)
		return
	}

	var payload struct {
		Type      string `json:"type"`
		Challenge string `json:"challenge"`
		Event     struct {
			Type    string `json:"type"`
			Text    string `json:"text"`
			Channel string `json:"channel"`
			User    string `json:"user"`
			BotID   string `json:"bot_id"`
			Subtype string `json:"subtype"`
		} `json:"event"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	if payload.Type == "url_verification" {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(payload.Challenge))
		return
	}

	if payload.Type != "event_callback" || payload.Event.Type != "message" {
		w.WriteHeader(http.StatusOK)
		return
	}
	if payload.Event.BotID != "" || payload.Event.Subtype == "bot_message" || strings.TrimSpace(payload.Event.Text) == "" {
		w.WriteHeader(http.StatusOK)
		return
	}

	input := map[string]any{
		"action":  "message",
		"channel": "slack",
		"chat_id": payload.Event.Channel,
		"text":    payload.Event.Text,
		"user_id": payload.Event.User,
	}
	if err := json.NewEncoder(e.stdout).Encode(input); err != nil {
		http.Error(w, "failed to emit message", http.StatusInternalServerError)
		return
	}

	reply, err := e.readReply()
	if err != nil {
		fmt.Fprintf(e.stderr, "failed to read response: %v\n", err)
		w.WriteHeader(http.StatusOK)
		return
	}
	if reply != "" {
		if err := e.sendMessage(payload.Event.Channel, reply); err != nil {
			fmt.Fprintf(e.stderr, "slack send error: %v\n", err)
		}
	}

	w.WriteHeader(http.StatusOK)
}

func (e *SlackExtension) verifySignature(body []byte, signature, timestamp string) bool {
	if e.config.SigningSecret == "" {
		return true
	}

	if signature == "" || timestamp == "" {
		return false
	}
	if _, err := strconv.ParseInt(timestamp, 10, 64); err != nil {
		return false
	}

	base := "v0:" + timestamp + ":" + string(body)
	mac := hmac.New(sha256.New, []byte(e.config.SigningSecret))
	mac.Write([]byte(base))
	expected := "v0=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

func (e *SlackExtension) readReply() (string, error) {
	var response map[string]any
	if err := json.NewDecoder(e.stdin).Decode(&response); err != nil {
		return "", err
	}
	reply, _ := response["text"].(string)
	return strings.TrimSpace(reply), nil
}

func (e *SlackExtension) sendMessage(channelID, text string) error {
	body, _ := json.Marshal(map[string]string{
		"channel": channelID,
		"text":    text,
	})

	req, err := http.NewRequest(http.MethodPost, e.baseURL+"/chat.postMessage", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+e.config.BotToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if resp.StatusCode >= 300 || !result.OK {
		if result.Error == "" {
			result.Error = resp.Status
		}
		return fmt.Errorf("slack send failed: %s", result.Error)
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

	ext := NewSlackExtension(cfg)
	if err := ext.Run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "extension error: %v\n", err)
		os.Exit(1)
	}
}
