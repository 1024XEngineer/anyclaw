package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Config struct {
	AppID        string `json:"app_id"`
	SecretKey    string `json:"secret_key"`
	OaID         string `json:"oa_id"`
	WebhookToken string `json:"webhook_token,omitempty"`
	Port         int    `json:"port"`
}

type ZaloExtension struct {
	config      Config
	client      *http.Client
	accessToken string
	tokenMu     sync.RWMutex
	tokenExpire time.Time
}

func NewZaloExtension(cfg Config) *ZaloExtension {
	if cfg.Port <= 0 {
		cfg.Port = 8080
	}
	return &ZaloExtension{
		config: cfg,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (e *ZaloExtension) Run(ctx context.Context) error {
	go e.refreshTokenLoop(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("/zalo", e.handleWebhook)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", e.config.Port),
		Handler: mux,
	}

	errCh := make(chan error, 1)
	go func() {
		fmt.Fprintf(os.Stderr, "zalo webhook listening on :%d\n", e.config.Port)
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

func (e *ZaloExtension) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		e.handleVerification(w, r)
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

	var rawEvent map[string]any
	if err := json.Unmarshal(body, &rawEvent); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	if e.config.WebhookToken != "" {
		if !e.verifyWebhookSignature(rawEvent, r) {
			http.Error(w, "invalid signature", http.StatusForbidden)
			return
		}
	}

	eventType, _ := rawEvent["event"].(string)
	if eventType != "user_send_msg" {
		w.Write([]byte("success"))
		return
	}

	message, _ := rawEvent["message"].(map[string]any)
	if message == nil {
		w.Write([]byte("success"))
		return
	}

	msgType, _ := message["msg_type"].(string)
	if msgType != "text" {
		w.Write([]byte("success"))
		return
	}

	text, _ := message["text"].(string)
	if text == "" {
		w.Write([]byte("success"))
		return
	}

	sender, _ := rawEvent["sender"].(map[string]any)
	senderID, _ := sender["id"].(string)
	if senderID == "" {
		if fromUser, ok := rawEvent["from_user"].(map[string]any); ok {
			senderID, _ = fromUser["id"].(string)
		}
	}
	if senderID == "" {
		senderID, _ = rawEvent["user_id"].(string)
	}

	chatID := senderID
	if chatID == "" {
		chatID, _ = rawEvent["conversation_id"].(string)
	}

	input := map[string]any{
		"action":   "message",
		"channel":  "zalo",
		"chat_id":  chatID,
		"text":     text,
		"user_id":  senderID,
		"msg_type": msgType,
	}
	data, _ := json.Marshal(input)
	fmt.Println(string(data))

	var response map[string]any
	if err := json.NewDecoder(os.Stdin).Decode(&response); err != nil {
		fmt.Fprintf(os.Stderr, "failed to read response: %v\n", err)
		w.Write([]byte("success"))
		return
	}

	if reply, ok := response["text"].(string); ok && reply != "" {
		if err := e.sendTextMessage(senderID, reply); err != nil {
			fmt.Fprintf(os.Stderr, "zalo send message error: %v\n", err)
		}
	}

	w.Write([]byte("success"))
}

func (e *ZaloExtension) handleVerification(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	verifyToken := query.Get("verify_token")
	echoStr := query.Get("echostr")

	if e.config.WebhookToken != "" && verifyToken != e.config.WebhookToken {
		http.Error(w, "invalid verify_token", http.StatusForbidden)
		return
	}

	if echoStr != "" {
		w.Write([]byte(echoStr))
		return
	}

	w.Write([]byte("verified"))
}

func (e *ZaloExtension) verifyWebhookSignature(rawEvent map[string]any, r *http.Request) bool {
	query := r.URL.Query()
	sig := query.Get("sig")
	if sig == "" {
		return true
	}

	timestamp := query.Get("timestamp")
	body, _ := json.Marshal(rawEvent)

	h := sha256.New()
	h.Write([]byte(timestamp + e.config.WebhookToken + string(body)))
	computed := fmt.Sprintf("%x", h.Sum(nil))

	return strings.EqualFold(computed, sig)
}

func (e *ZaloExtension) getAccessToken() (string, error) {
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

	u := "https://oauth.zalo.me/v4/oa/access_token"
	payload := map[string]string{
		"app_id":     e.config.AppID,
		"secret_key": e.config.SecretKey,
		"grant_type": "client_credentials",
	}
	body, _ := json.Marshal(payload)

	resp, err := e.client.Post(u, "application/json", strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		ErrCode     int    `json:"error"`
		ErrMsg      string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if result.ErrCode != 0 {
		return "", fmt.Errorf("zalo token error: %d %s", result.ErrCode, result.ErrMsg)
	}

	e.accessToken = result.AccessToken
	e.tokenExpire = time.Now().Add(time.Duration(result.ExpiresIn-300) * time.Second)
	return e.accessToken, nil
}

func (e *ZaloExtension) refreshTokenLoop(ctx context.Context) {
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

func (e *ZaloExtension) sendTextMessage(userID, text string) error {
	token, err := e.getAccessToken()
	if err != nil {
		return err
	}

	u := fmt.Sprintf("https://openapi.zalo.me/v2.0/oa/message?access_token=%s", token)

	payload := map[string]any{
		"recipient": map[string]string{
			"user_id": userID,
		},
		"message": map[string]any{
			"text": text,
		},
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

	var result struct {
		ErrCode int    `json:"error"`
		ErrMsg  string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	if result.ErrCode != 0 {
		return fmt.Errorf("zalo send error: %d %s", result.ErrCode, result.ErrMsg)
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

	ext := NewZaloExtension(cfg)
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
