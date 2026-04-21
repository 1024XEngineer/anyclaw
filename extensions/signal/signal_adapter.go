package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type Config struct {
	SignalURL string `json:"signal_url"`
	Number    string `json:"number"`
	PollEvery int    `json:"poll_every"`
}

type SignalExtension struct {
	config  Config
	client  *http.Client
	baseURL string
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
}

func NewSignalExtension(cfg Config) *SignalExtension {
	baseURL := strings.TrimRight(cfg.SignalURL, "/")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	return &SignalExtension{
		config:  cfg,
		client:  &http.Client{Timeout: 15 * time.Second},
		baseURL: baseURL,
		stdin:   os.Stdin,
		stdout:  os.Stdout,
		stderr:  os.Stderr,
	}
}

func (e *SignalExtension) Run(ctx context.Context) error {
	if strings.TrimSpace(e.config.Number) == "" {
		return fmt.Errorf("number is required")
	}

	interval := time.Duration(e.config.PollEvery) * time.Second
	if interval <= 0 {
		interval = 3 * time.Second
	}

	if err := e.pollOnce(ctx); err != nil {
		fmt.Fprintf(e.stderr, "signal poll error: %v\n", err)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := e.pollOnce(ctx); err != nil {
				fmt.Fprintf(e.stderr, "signal poll error: %v\n", err)
			}
		}
	}
}

type signalEnvelope struct {
	Envelope struct {
		SourceNumber string `json:"sourceNumber"`
		Timestamp    int64  `json:"timestamp"`
		DataMessage  struct {
			Message string `json:"message"`
		} `json:"dataMessage"`
	} `json:"envelope"`
}

func (e *SignalExtension) pollOnce(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.baseURL+"/v1/receive/"+url.PathEscape(e.config.Number), nil)
	if err != nil {
		return err
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("signal receive failed: %s", resp.Status)
	}

	var envelopes []signalEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&envelopes); err != nil {
		return err
	}

	for _, envelope := range envelopes {
		text := strings.TrimSpace(envelope.Envelope.DataMessage.Message)
		if text == "" {
			continue
		}

		payload := map[string]any{
			"action":    "message",
			"channel":   "signal",
			"chat_id":   envelope.Envelope.SourceNumber,
			"text":      text,
			"user_id":   envelope.Envelope.SourceNumber,
			"timestamp": envelope.Envelope.Timestamp,
		}
		if err := json.NewEncoder(e.stdout).Encode(payload); err != nil {
			return err
		}

		reply, err := e.readReply()
		if err != nil {
			return fmt.Errorf("failed to read response: %w", err)
		}
		if reply != "" {
			if err := e.sendMessage(ctx, envelope.Envelope.SourceNumber, reply); err != nil {
				return err
			}
		}
	}

	return nil
}

func (e *SignalExtension) readReply() (string, error) {
	var response map[string]any
	if err := json.NewDecoder(e.stdin).Decode(&response); err != nil {
		return "", err
	}

	reply, _ := response["text"].(string)
	return strings.TrimSpace(reply), nil
}

func (e *SignalExtension) sendMessage(ctx context.Context, recipient, text string) error {
	body, _ := json.Marshal(map[string]any{
		"message":    text,
		"number":     e.config.Number,
		"recipients": []string{recipient},
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/v2/send", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("signal send failed: %s: %s", resp.Status, strings.TrimSpace(string(respBody)))
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

	ext := NewSignalExtension(cfg)
	if err := ext.Run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "extension error: %v\n", err)
		os.Exit(1)
	}
}
