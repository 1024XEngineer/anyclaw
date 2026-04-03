package signal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/anyclaw/anyclaw/pkg/channel"
	"github.com/anyclaw/anyclaw/pkg/config"
)

type Adapter struct {
	base     channel.BaseAdapter
	config   config.SignalChannelConfig
	client   *http.Client
	handler  channel.InboundHandler
	mu       sync.Mutex
	sessions map[string]string
}

func NewAdapter(cfg config.SignalChannelConfig) *Adapter {
	return &Adapter{
		base:     channel.NewBaseAdapter("signal", cfg.Enabled && cfg.Number != ""),
		config:   cfg,
		client:   &http.Client{Timeout: 20 * time.Second},
		sessions: make(map[string]string),
	}
}

func (a *Adapter) SetHandler(handler channel.InboundHandler) {
	a.handler = handler
}

func (a *Adapter) Name() string {
	return "signal"
}

func (a *Adapter) Enabled() bool {
	return a.config.Enabled && a.config.Number != ""
}

func (a *Adapter) Run(ctx context.Context, handler channel.InboundHandler) error {
	a.handler = handler
	a.base.SetRunning(true)
	defer a.base.SetRunning(false)

	a.base.MarkActivity()
	<-ctx.Done()
	return nil
}

func (a *Adapter) HandleWebhook(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload map[string]any
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	envelope, _ := payload["envelope"].(map[string]any)
	source, _ := envelope["source"].(string)
	message, _ := envelope["message"].(string)

	if source == "" || message == "" {
		w.WriteHeader(http.StatusOK)
		return
	}

	meta := map[string]string{
		"channel": "signal",
		"source":  source,
	}

	_, _, _ = a.handler(context.Background(), "", message, meta)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
}

func (a *Adapter) SendMessage(ctx context.Context, recipient, message string) error {
	if a.config.BaseURL == "" {
		return nil
	}

	url := a.config.BaseURL + "/send"
	body, _ := json.Marshal(map[string]string{
		"recipient": recipient,
		"message":   message,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("signal API error: %s", resp.Status)
	}

	return nil
}

func (a *Adapter) Status() channel.Status {
	return a.base.Status()
}
