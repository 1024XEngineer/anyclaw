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
	"sync"
	"syscall"
	"time"
)

type Config struct {
	AppID             string `json:"app_id"`
	AppSecret         string `json:"app_secret"`
	VerificationToken string `json:"verification_token,omitempty"`
	EncryptKey        string `json:"encrypt_key,omitempty"`
	Port              int    `json:"port"`
}

type FeishuExtension struct {
	config            Config
	client            *http.Client
	tenantAccessToken string
	tokenMu           sync.RWMutex
	tokenExpire       time.Time
}

func NewFeishuExtension(cfg Config) *FeishuExtension {
	if cfg.Port <= 0 {
		cfg.Port = 8080
	}
	return &FeishuExtension{
		config: cfg,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (e *FeishuExtension) Run(ctx context.Context) error {
	go e.refreshTokenLoop(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("/feishu", e.handleWebhook)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", e.config.Port),
		Handler: mux,
	}

	errCh := make(chan error, 1)
	go func() {
		fmt.Fprintf(os.Stderr, "feishu webhook listening on :%d\n", e.config.Port)
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

func (e *FeishuExtension) handleWebhook(w http.ResponseWriter, r *http.Request) {
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

	if challenge, ok := rawEvent["challenge"].(string); ok {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"challenge": challenge})
		return
	}

	header, _ := rawEvent["header"].(map[string]any)
	event, _ := rawEvent["event"].(map[string]any)

	if header != nil {
		if e.config.VerificationToken != "" {
			token, _ := header["token"].(string)
			if token != e.config.VerificationToken {
				http.Error(w, "invalid token", http.StatusForbidden)
				return
			}
		}
	}

	eventType, _ := header["event_type"].(string)
	if eventType != "im.message.receive_v1" {
		w.Write([]byte("success"))
		return
	}

	message, _ := event["message"].(map[string]any)
	sender, _ := event["sender"].(map[string]any)

	msgType, _ := message["message_type"].(string)
	if msgType != "text" {
		w.Write([]byte("success"))
		return
	}

	content, _ := message["content"].(string)
	var textContent struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(content), &textContent); err != nil {
		w.Write([]byte("success"))
		return
	}

	if textContent.Text == "" {
		w.Write([]byte("success"))
		return
	}

	chatID, _ := message["chat_id"].(string)
	senderID, _ := sender["sender_id"].(map[string]any)
	openID, _ := senderID["open_id"].(string)
	messageID, _ := message["message_id"].(string)

	if openID == "" {
		openID, _ = event["open_id"].(string)
	}

	input := map[string]any{
		"action":     "message",
		"channel":    "feishu",
		"chat_id":    chatID,
		"text":       textContent.Text,
		"user_id":    openID,
		"message_id": messageID,
		"msg_type":   msgType,
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
		if err := e.sendTextMessage(chatID, reply); err != nil {
			fmt.Fprintf(os.Stderr, "feishu send message error: %v\n", err)
		}
	}

	w.Write([]byte("success"))
}

func (e *FeishuExtension) getTenantAccessToken() (string, error) {
	e.tokenMu.RLock()
	if e.tenantAccessToken != "" && time.Now().Before(e.tokenExpire) {
		token := e.tenantAccessToken
		e.tokenMu.RUnlock()
		return token, nil
	}
	e.tokenMu.RUnlock()

	e.tokenMu.Lock()
	defer e.tokenMu.Unlock()

	if e.tenantAccessToken != "" && time.Now().Before(e.tokenExpire) {
		return e.tenantAccessToken, nil
	}

	u := "https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal"
	payload := map[string]string{
		"app_id":     e.config.AppID,
		"app_secret": e.config.AppSecret,
	}
	body, _ := json.Marshal(payload)

	resp, err := e.client.Post(u, "application/json", strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Code              int    `json:"code"`
		Msg               string `json:"msg"`
		TenantAccessToken string `json:"tenant_access_token"`
		Expire            int    `json:"expire"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if result.Code != 0 {
		return "", fmt.Errorf("feishu token error: %d %s", result.Code, result.Msg)
	}

	e.tenantAccessToken = result.TenantAccessToken
	e.tokenExpire = time.Now().Add(time.Duration(result.Expire-300) * time.Second)
	return e.tenantAccessToken, nil
}

func (e *FeishuExtension) refreshTokenLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	e.getTenantAccessToken()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.getTenantAccessToken()
		}
	}
}

func (e *FeishuExtension) sendTextMessage(chatID, text string) error {
	token, err := e.getTenantAccessToken()
	if err != nil {
		return err
	}

	u := "https://open.feishu.cn/open-apis/im/v1/messages"
	content, _ := json.Marshal(map[string]string{"text": text})

	payload := map[string]any{
		"receive_id": chatID,
		"msg_type":   "text",
		"content":    string(content),
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s?receive_id_type=chat_id", u), strings.NewReader(string(body)))
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

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	if result.Code != 0 {
		return fmt.Errorf("feishu send error: %d %s", result.Code, result.Msg)
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

	ext := NewFeishuExtension(cfg)
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
