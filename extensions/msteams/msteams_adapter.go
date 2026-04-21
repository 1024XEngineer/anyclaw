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
	AppID      string `json:"app_id"`
	AppSecret  string `json:"app_secret"`
	TenantID   string `json:"tenant_id"`
	WebhookURL string `json:"webhook_url,omitempty"`
	Port       int    `json:"port"`
}

type MSTeamsExtension struct {
	config      Config
	client      *http.Client
	accessToken string
	tokenMu     sync.RWMutex
	tokenExpire time.Time
}

func NewMSTeamsExtension(cfg Config) *MSTeamsExtension {
	if cfg.Port <= 0 {
		cfg.Port = 8080
	}
	return &MSTeamsExtension{
		config: cfg,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (e *MSTeamsExtension) Run(ctx context.Context) error {
	go e.refreshTokenLoop(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("/msteams", e.handleWebhook)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", e.config.Port),
		Handler: mux,
	}

	errCh := make(chan error, 1)
	go func() {
		fmt.Fprintf(os.Stderr, "msteams webhook listening on :%d\n", e.config.Port)
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

func (e *MSTeamsExtension) handleWebhook(w http.ResponseWriter, r *http.Request) {
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
		Type           string `json:"type"`
		ID             string `json:"id"`
		Timestamp      string `json:"timestamp"`
		LocalTimestamp string `json:"localTimestamp"`
		ServiceURL     string `json:"serviceUrl"`
		ChannelID      string `json:"channelId"`
		From           struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			AadID    string `json:"aadObjectId"`
			TenantID string `json:"tenantId"`
		} `json:"from"`
		Conversation struct {
			IsGroup          bool   `json:"isGroup"`
			ConversationType string `json:"conversationType"`
			ID               string `json:"id"`
			TenantID         string `json:"tenantId"`
			Name             string `json:"name"`
		} `json:"conversation"`
		Recipient struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			AadID string `json:"aadObjectId"`
		} `json:"recipient"`
		Text       string `json:"text"`
		TextFormat string `json:"textFormat"`
		Locale     string `json:"locale"`
		ReplyToID  string `json:"replyToId"`
		Entities   []struct {
			Type      string `json:"type"`
			Mentioned struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"mentioned"`
			Text string `json:"text"`
		} `json:"entities"`
		ChannelData struct {
			TeamsChannelID   string `json:"teamsChannelId"`
			TeamsTeamID      string `json:"teamsTeamId"`
			TeamsChannelData struct {
				Channel struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"channel"`
				Team struct {
					ID string `json:"id"`
				} `json:"team"`
				Tenant struct {
					ID string `json:"id"`
				} `json:"tenant"`
			} `json:"teamsChannelData"`
		} `json:"channelData"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	if payload.Type == "message" && payload.Text != "" {
		e.handleMessage(w, &payload)
		return
	}

	if payload.Type == "conversationUpdate" {
		w.WriteHeader(http.StatusOK)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (e *MSTeamsExtension) handleMessage(w http.ResponseWriter, payload *struct {
	Type           string `json:"type"`
	ID             string `json:"id"`
	Timestamp      string `json:"timestamp"`
	LocalTimestamp string `json:"localTimestamp"`
	ServiceURL     string `json:"serviceUrl"`
	ChannelID      string `json:"channelId"`
	From           struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		AadID    string `json:"aadObjectId"`
		TenantID string `json:"tenantId"`
	} `json:"from"`
	Conversation struct {
		IsGroup          bool   `json:"isGroup"`
		ConversationType string `json:"conversationType"`
		ID               string `json:"id"`
		TenantID         string `json:"tenantId"`
		Name             string `json:"name"`
	} `json:"conversation"`
	Recipient struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		AadID string `json:"aadObjectId"`
	} `json:"recipient"`
	Text       string `json:"text"`
	TextFormat string `json:"textFormat"`
	Locale     string `json:"locale"`
	ReplyToID  string `json:"replyToId"`
	Entities   []struct {
		Type      string `json:"type"`
		Mentioned struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"mentioned"`
		Text string `json:"text"`
	} `json:"entities"`
	ChannelData struct {
		TeamsChannelID   string `json:"teamsChannelId"`
		TeamsTeamID      string `json:"teamsTeamId"`
		TeamsChannelData struct {
			Channel struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"channel"`
			Team struct {
				ID string `json:"id"`
			} `json:"team"`
			Tenant struct {
				ID string `json:"id"`
			} `json:"tenant"`
		} `json:"teamsChannelData"`
	} `json:"channelData"`
}) {
	text := strings.TrimSpace(payload.Text)
	if text == "" {
		w.WriteHeader(http.StatusOK)
		return
	}

	senderID := payload.From.ID
	senderName := payload.From.Name
	conversationID := payload.Conversation.ID
	serviceURL := payload.ServiceURL
	isGroup := payload.Conversation.IsGroup

	input := map[string]any{
		"action":      "message",
		"channel":     "msteams",
		"chat_id":     conversationID,
		"text":        text,
		"user_id":     senderID,
		"username":    senderName,
		"service_url": serviceURL,
		"reply_to_id": payload.ReplyToID,
		"activity_id": payload.ID,
		"is_group":    isGroup,
		"conv_type":   payload.Conversation.ConversationType,
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
		if err := e.sendReply(serviceURL, conversationID, payload.ID, reply); err != nil {
			fmt.Fprintf(os.Stderr, "msteams send message error: %v\n", err)
		}
	}

	w.WriteHeader(http.StatusOK)
}

func (e *MSTeamsExtension) getAccessToken() (string, error) {
	e.tokenMu.RLock()
	if e.accessToken != "" && time.Now().Before(e.tokenExpire) {
		token := e.accessToken
		e.tokenMu.RUnlock()
		return token, nil
	}
	e.tokenMu.RUnlock()

	e.tokenMu.Lock()
	defer e.tokenMu.Unlock()

	if e.accessToken != "" && time.Now().Before(e.tokenExpire) {
		return e.accessToken, nil
	}

	tokenURL := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", e.config.TenantID)
	formData := url.Values{}
	formData.Set("grant_type", "client_credentials")
	formData.Set("client_id", e.config.AppID)
	formData.Set("client_secret", e.config.AppSecret)
	formData.Set("scope", "https://api.botframework.com/.default")

	resp, err := e.client.PostForm(tokenURL, formData)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", err
	}

	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("msteams token error: no access_token in response")
	}

	e.accessToken = tokenResp.AccessToken
	if tokenResp.ExpiresIn > 0 {
		e.tokenExpire = time.Now().Add(time.Duration(tokenResp.ExpiresIn-300) * time.Second)
	} else {
		e.tokenExpire = time.Now().Add(30 * time.Minute)
	}
	return e.accessToken, nil
}

func (e *MSTeamsExtension) refreshTokenLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	e.getAccessToken()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.getAccessToken()
		}
	}
}

func (e *MSTeamsExtension) sendReply(serviceURL, conversationID, replyToID, text string) error {
	if e.config.WebhookURL != "" {
		return e.sendViaWebhook(text)
	}

	if serviceURL == "" {
		return fmt.Errorf("no service_url available to send reply")
	}

	u := fmt.Sprintf("%s/v3/conversations/%s/activities/%s", strings.TrimRight(serviceURL, "/"), conversationID, replyToID)
	payload := map[string]any{
		"type": "message",
		"text": text,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest(http.MethodPost, u, strings.NewReader(string(body)))
	if err != nil {
		return err
	}

	token, err := e.getAccessToken()
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("msteams reply error: %s: %s", resp.Status, string(respBody))
	}
	return nil
}

func (e *MSTeamsExtension) sendViaWebhook(text string) error {
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
		return fmt.Errorf("msteams webhook error: %s: %s", resp.Status, string(respBody))
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

	ext := NewMSTeamsExtension(cfg)
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
