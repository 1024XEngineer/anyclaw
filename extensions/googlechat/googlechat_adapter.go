package main

import (
	"context"
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
	ProjectID         string `json:"project_id"`
	ServiceAccountKey string `json:"service_account_key,omitempty"`
	WebhookURL        string `json:"webhook_url,omitempty"`
	Port              int    `json:"port"`
}

type GoogleChatExtension struct {
	config Config
	client *http.Client
}

func NewGoogleChatExtension(cfg Config) *GoogleChatExtension {
	if cfg.Port <= 0 {
		cfg.Port = 8080
	}
	return &GoogleChatExtension{
		config: cfg,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (e *GoogleChatExtension) Run(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/googlechat", e.handleWebhook)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", e.config.Port),
		Handler: mux,
	}

	errCh := make(chan error, 1)
	go func() {
		fmt.Fprintf(os.Stderr, "google chat webhook listening on :%d\n", e.config.Port)
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

func (e *GoogleChatExtension) handleWebhook(w http.ResponseWriter, r *http.Request) {
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
		Type      string `json:"type"`
		EventTime string `json:"eventTime"`
		Action    struct {
			ActionMethodName string `json:"actionMethodName"`
			Parameters       []struct {
				Name  string `json:"name"`
				Value string `json:"value"`
			} `json:"parameters"`
		} `json:"action"`
		Message struct {
			Name   string `json:"name"`
			Sender struct {
				Name        string `json:"name"`
				DisplayName string `json:"displayName"`
				Type        string `json:"type"`
				Domain      struct {
					Name string `json:"name"`
				} `json:"domain"`
			} `json:"sender"`
			Space struct {
				Name        string `json:"name"`
				Type        string `json:"type"`
				DisplayName string `json:"displayName"`
				SpaceType   string `json:"spaceType"`
			} `json:"space"`
			Thread struct {
				Name string `json:"name"`
			} `json:"thread"`
			ArgumentText string `json:"argumentText"`
			Annotations  []struct {
				Type        string `json:"type"`
				UserMention struct {
					User struct {
						DisplayName string `json:"displayName"`
					} `json:"user"`
					Type string `json:"type"`
				} `json:"userMention"`
			} `json:"annotations"`
			Text             string `json:"text"`
			FallbackText     string `json:"fallbackText"`
			CreateTime       string `json:"createTime"`
			LastUpdateTime   string `json:"lastUpdateTime"`
			LastActiveThread struct {
				Name string `json:"name"`
			} `json:"lastActiveThread"`
		} `json:"message"`
		Space struct {
			Name        string `json:"name"`
			Type        string `json:"type"`
			DisplayName string `json:"displayName"`
		} `json:"space"`
		User struct {
			Name        string `json:"name"`
			DisplayName string `json:"displayName"`
		} `json:"user"`
		ConfigCompleteRequested bool `json:"configCompleteRequested"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	if payload.Type == "MESSAGE" {
		e.handleMessage(w, &payload)
		return
	}

	if payload.Type == "ADDED_TO_SPACE" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"text": fmt.Sprintf("Thank you for adding me to %s!", payload.Space.DisplayName),
		})
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (e *GoogleChatExtension) handleMessage(w http.ResponseWriter, payload *struct {
	Type      string `json:"type"`
	EventTime string `json:"eventTime"`
	Action    struct {
		ActionMethodName string `json:"actionMethodName"`
		Parameters       []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"parameters"`
	} `json:"action"`
	Message struct {
		Name   string `json:"name"`
		Sender struct {
			Name        string `json:"name"`
			DisplayName string `json:"displayName"`
			Type        string `json:"type"`
			Domain      struct {
				Name string `json:"name"`
			} `json:"domain"`
		} `json:"sender"`
		Space struct {
			Name        string `json:"name"`
			Type        string `json:"type"`
			DisplayName string `json:"displayName"`
			SpaceType   string `json:"spaceType"`
		} `json:"space"`
		Thread struct {
			Name string `json:"name"`
		} `json:"thread"`
		ArgumentText string `json:"argumentText"`
		Annotations  []struct {
			Type        string `json:"type"`
			UserMention struct {
				User struct {
					DisplayName string `json:"displayName"`
				} `json:"user"`
				Type string `json:"type"`
			} `json:"userMention"`
		} `json:"annotations"`
		Text             string `json:"text"`
		FallbackText     string `json:"fallbackText"`
		CreateTime       string `json:"createTime"`
		LastUpdateTime   string `json:"lastUpdateTime"`
		LastActiveThread struct {
			Name string `json:"name"`
		} `json:"lastActiveThread"`
	} `json:"message"`
	Space struct {
		Name        string `json:"name"`
		Type        string `json:"type"`
		DisplayName string `json:"displayName"`
	} `json:"space"`
	User struct {
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
	} `json:"user"`
	ConfigCompleteRequested bool `json:"configCompleteRequested"`
}) {
	text := strings.TrimSpace(payload.Message.Text)
	if text == "" {
		text = strings.TrimSpace(payload.Message.ArgumentText)
	}
	if text == "" {
		w.WriteHeader(http.StatusOK)
		return
	}

	senderID := payload.Message.Sender.Name
	senderName := payload.Message.Sender.DisplayName
	if senderName == "" {
		senderName = payload.User.DisplayName
	}
	spaceID := payload.Message.Space.Name
	if spaceID == "" {
		spaceID = payload.Space.Name
	}
	threadName := payload.Message.Thread.Name

	input := map[string]any{
		"action":     "message",
		"channel":    "googlechat",
		"chat_id":    spaceID,
		"text":       text,
		"user_id":    senderID,
		"username":   senderName,
		"thread":     threadName,
		"space_type": payload.Message.Space.SpaceType,
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
		if err := e.sendReply(payload.Message.Name, reply); err != nil {
			fmt.Fprintf(os.Stderr, "google chat send message error: %v\n", err)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"text": ""})
}

func (e *GoogleChatExtension) sendReply(messageName, text string) error {
	if e.config.WebhookURL != "" {
		return e.sendViaWebhook(text)
	}

	u := fmt.Sprintf("https://chat.googleapis.com/v1/%s/replies", messageName)
	payload := map[string]any{
		"text": text,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest(http.MethodPost, u, strings.NewReader(string(body)))
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
		return fmt.Errorf("google chat reply error: %s: %s", resp.Status, string(respBody))
	}
	return nil
}

func (e *GoogleChatExtension) sendViaWebhook(text string) error {
	payload := map[string]any{
		"text": text,
	}
	body, _ := json.Marshal(payload)

	resp, err := e.client.Post(e.config.WebhookURL, "application/json", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("google chat webhook error: %s: %s", resp.Status, string(respBody))
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

	ext := NewGoogleChatExtension(cfg)
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
