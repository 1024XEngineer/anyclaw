package main

import (
	"context"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Config struct {
	AppID          string `json:"app_id"`
	AppSecret      string `json:"app_secret"`
	Token          string `json:"token,omitempty"`
	EncodingAESKey string `json:"encoding_aes_key,omitempty"`
	Port           int    `json:"port"`
}

type WeChatExtension struct {
	config      Config
	client      *http.Client
	accessToken string
	tokenMu     sync.RWMutex
	tokenExpire time.Time
}

func NewWeChatExtension(cfg Config) *WeChatExtension {
	if cfg.Port <= 0 {
		cfg.Port = 8080
	}
	return &WeChatExtension{
		config: cfg,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (e *WeChatExtension) Run(ctx context.Context) error {
	go e.refreshTokenLoop(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("/wechat", e.handleWebhook)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", e.config.Port),
		Handler: mux,
	}

	errCh := make(chan error, 1)
	go func() {
		fmt.Fprintf(os.Stderr, "wechat webhook listening on :%d\n", e.config.Port)
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

func (e *WeChatExtension) handleWebhook(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	signature := query.Get("signature")
	timestamp := query.Get("timestamp")
	nonce := query.Get("nonce")
	echoStr := query.Get("echostr")

	if r.Method == http.MethodGet && echoStr != "" {
		if e.verifySignature(signature, timestamp, nonce) {
			w.Write([]byte(echoStr))
		} else {
			http.Error(w, "invalid signature", http.StatusForbidden)
		}
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !e.verifySignature(signature, timestamp, nonce) {
		http.Error(w, "invalid signature", http.StatusForbidden)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	msg := e.parseMessage(body)
	if msg == nil {
		w.Write([]byte("success"))
		return
	}

	if msg.MsgType != "text" || msg.Content == "" {
		w.Write([]byte("success"))
		return
	}

	input := map[string]any{
		"action":   "message",
		"channel":  "wechat",
		"chat_id":  msg.FromUserName,
		"text":     msg.Content,
		"user_id":  msg.FromUserName,
		"msg_id":   msg.MsgID,
		"msg_type": msg.MsgType,
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
		if err := e.sendTextMessage(msg.FromUserName, reply); err != nil {
			fmt.Fprintf(os.Stderr, "wechat send message error: %v\n", err)
		}
	}

	w.Write([]byte("success"))
}

type wechatMessage struct {
	ToUserName   string `json:"ToUserName" xml:"ToUserName"`
	FromUserName string `json:"FromUserName" xml:"FromUserName"`
	CreateTime   int64  `json:"CreateTime" xml:"CreateTime"`
	MsgType      string `json:"MsgType" xml:"MsgType"`
	Content      string `json:"Content" xml:"Content"`
	MsgID        string `json:"MsgId" xml:"MsgId"`
}

func (e *WeChatExtension) parseMessage(body []byte) *wechatMessage {
	contentType := ""
	if strings.Contains(string(body), "<xml>") {
		contentType = "xml"
	} else {
		contentType = "json"
	}

	if contentType == "json" {
		var msg wechatMessage
		if err := json.Unmarshal(body, &msg); err != nil {
			return nil
		}
		return &msg
	}

	return e.parseXMLMessage(string(body))
}

func (e *WeChatExtension) parseXMLMessage(rawXML string) *wechatMessage {
	var msg wechatMessage
	if err := xml.Unmarshal([]byte(rawXML), &msg); err != nil {
		return nil
	}
	return &msg
}

func (e *WeChatExtension) verifySignature(signature, timestamp, nonce string) bool {
	if strings.TrimSpace(e.config.Token) == "" ||
		strings.TrimSpace(signature) == "" ||
		strings.TrimSpace(timestamp) == "" ||
		strings.TrimSpace(nonce) == "" {
		return false
	}
	arr := []string{e.config.Token, timestamp, nonce}
	sort.Strings(arr)
	combined := strings.Join(arr, "")
	h := sha1.Sum([]byte(combined))
	computed := fmt.Sprintf("%x", h)
	return subtle.ConstantTimeCompare([]byte(computed), []byte(signature)) == 1
}

func (e *WeChatExtension) getAccessToken() (string, error) {
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

	u := fmt.Sprintf("https://api.weixin.qq.com/cgi-bin/token?grant_type=client_credential&appid=%s&secret=%s",
		e.config.AppID, e.config.AppSecret)

	resp, err := e.client.Get(u)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if result.ErrCode != 0 {
		return "", fmt.Errorf("wechat token error: %d %s", result.ErrCode, result.ErrMsg)
	}

	e.accessToken = result.AccessToken
	e.tokenExpire = time.Now().Add(time.Duration(result.ExpiresIn-300) * time.Second)
	return e.accessToken, nil
}

func (e *WeChatExtension) refreshTokenLoop(ctx context.Context) {
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

func (e *WeChatExtension) sendTextMessage(openID, text string) error {
	token, err := e.getAccessToken()
	if err != nil {
		return err
	}

	u := fmt.Sprintf("https://api.weixin.qq.com/cgi-bin/message/custom/send?access_token=%s", token)

	payload := map[string]any{
		"touser":  openID,
		"msgtype": "text",
		"text": map[string]string{
			"content": text,
		},
	}
	body, _ := json.Marshal(payload)

	resp, err := e.client.Post(u, "application/json", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	if result.ErrCode != 0 {
		return fmt.Errorf("wechat send error: %d %s", result.ErrCode, result.ErrMsg)
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

	ext := NewWeChatExtension(cfg)
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
