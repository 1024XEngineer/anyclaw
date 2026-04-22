package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type Config struct {
	AccessToken   string `json:"access_token"`
	PhoneNumberID string `json:"phone_number_id"`
	VerifyToken   string `json:"verify_token,omitempty"`
	AppSecret     string `json:"app_secret,omitempty"`
	Port          int    `json:"port,omitempty"`
}

type WhatsAppExtension struct {
	config     Config
	client     *http.Client
	apiBaseURL string
	stdin      io.Reader
	stdout     io.Writer
	stderr     io.Writer
}

func NewWhatsAppExtension(cfg Config) *WhatsAppExtension {
	if cfg.Port <= 0 {
		cfg.Port = 8080
	}
	return &WhatsAppExtension{
		config:     cfg,
		client:     &http.Client{Timeout: 10 * time.Second},
		apiBaseURL: "https://graph.facebook.com/v19.0",
		stdin:      os.Stdin,
		stdout:     os.Stdout,
		stderr:     os.Stderr,
	}
}

func (e *WhatsAppExtension) Run(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/whatsapp", e.handleWebhook)

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

func (e *WhatsAppExtension) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		mode := r.URL.Query().Get("hub.mode")
		verifyToken := r.URL.Query().Get("hub.verify_token")
		challenge := r.URL.Query().Get("hub.challenge")
		if mode == "subscribe" && verifyToken == e.config.VerifyToken {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(challenge))
			return
		}
		http.Error(w, "invalid verification", http.StatusForbidden)
		return
	}

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
		Entry []struct {
			Changes []struct {
				Value struct {
					Messages []struct {
						From string `json:"from"`
						ID   string `json:"id"`
						Type string `json:"type"`
						Text struct {
							Body string `json:"body"`
						} `json:"text"`
					} `json:"messages"`
				} `json:"value"`
			} `json:"changes"`
		} `json:"entry"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			for _, message := range change.Value.Messages {
				if message.Type != "text" || strings.TrimSpace(message.Text.Body) == "" {
					continue
				}

				input := map[string]any{
					"action":  "message",
					"channel": "whatsapp",
					"chat_id": message.From,
					"text":    message.Text.Body,
					"user_id": message.From,
					"msg_id":  message.ID,
				}
				if err := json.NewEncoder(e.stdout).Encode(input); err != nil {
					http.Error(w, "failed to emit message", http.StatusInternalServerError)
					return
				}

				reply, err := e.readReply()
				if err != nil {
					fmt.Fprintf(e.stderr, "failed to read response: %v\n", err)
					continue
				}
				if reply != "" {
					if err := e.sendTextMessage(message.From, reply); err != nil {
						fmt.Fprintf(e.stderr, "whatsapp send error: %v\n", err)
					}
				}
			}
		}
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (e *WhatsAppExtension) readReply() (string, error) {
	var response map[string]any
	if err := json.NewDecoder(e.stdin).Decode(&response); err != nil {
		return "", err
	}
	reply, _ := response["text"].(string)
	return strings.TrimSpace(reply), nil
}

func (e *WhatsAppExtension) sendTextMessage(recipient, text string) error {
	body, _ := json.Marshal(map[string]any{
		"messaging_product": "whatsapp",
		"to":                recipient,
		"type":              "text",
		"text": map[string]string{
			"body": text,
		},
	})

	req, err := http.NewRequest(http.MethodPost, e.apiBaseURL+"/"+e.config.PhoneNumberID+"/messages", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+e.config.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("whatsapp send failed: %s: %s", resp.Status, strings.TrimSpace(string(respBody)))
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

	ext := NewWhatsAppExtension(cfg)
	if err := ext.Run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "extension error: %v\n", err)
		os.Exit(1)
	}
}
