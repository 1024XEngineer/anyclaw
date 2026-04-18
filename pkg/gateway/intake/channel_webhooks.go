package intake

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	inputlayer "github.com/1024XEngineer/anyclaw/pkg/input"
	inputchannels "github.com/1024XEngineer/anyclaw/pkg/input/channels"
)

type DiscordInteractionAdapter interface {
	Enabled() bool
	VerifyInteraction(r *http.Request, body []byte) bool
	HandleInteraction(ctx context.Context, body []byte, handle inputchannels.InboundHandler) (map[string]any, error)
}

type WhatsAppWebhookAdapter interface {
	Enabled() bool
	HandleInbound(ctx context.Context, source string, text string, messageID string, profileName string, handle inputchannels.InboundHandler) (string, string, error)
	HandleStatus(sessionID string, status string, messageID string, recipient string)
}

type DiscordInteractionAPI struct {
	Adapter       DiscordInteractionAdapter
	HandleMessage inputlayer.InboundHandler
}

func (api DiscordInteractionAPI) Handle(w http.ResponseWriter, r *http.Request) {
	if api.Adapter == nil || !api.Adapter.Enabled() {
		http.NotFound(w, r)
		return
	}
	body, err := inputchannels.ReadBody(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if !api.Adapter.VerifyInteraction(r, body) {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}
	response, err := api.Adapter.HandleInteraction(r.Context(), body, api.HandleMessage)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, response)
}

type WhatsAppWebhookAPI struct {
	Adapter       WhatsAppWebhookAdapter
	HandleMessage inputlayer.InboundHandler
	VerifyToken   string
	AppSecret     string
}

func (api WhatsAppWebhookAPI) Handle(w http.ResponseWriter, r *http.Request) {
	if api.Adapter == nil || !api.Adapter.Enabled() {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		verifyToken := strings.TrimSpace(api.VerifyToken)
		if verifyToken == "" || r.URL.Query().Get("hub.verify_token") != verifyToken {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte(r.URL.Query().Get("hub.challenge")))
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if secret := strings.TrimSpace(api.AppSecret); secret != "" {
			provided := strings.TrimSpace(r.Header.Get("X-Hub-Signature-256"))
			if !VerifySignature(secret, body, provided) {
				http.Error(w, "invalid signature", http.StatusUnauthorized)
				return
			}
		}
		var payload struct {
			Entry []struct {
				Changes []struct {
					Value struct {
						Statuses []struct {
							ID          string `json:"id"`
							Status      string `json:"status"`
							RecipientID string `json:"recipient_id"`
						} `json:"statuses"`
						Messages []struct {
							ID      string `json:"id"`
							From    string `json:"from"`
							Profile struct {
								Name string `json:"name"`
							} `json:"profile"`
							Text struct {
								Body string `json:"body"`
							} `json:"text"`
						} `json:"messages"`
					} `json:"value"`
				} `json:"changes"`
			} `json:"entry"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		for _, entry := range payload.Entry {
			for _, change := range entry.Changes {
				for _, status := range change.Value.Statuses {
					api.Adapter.HandleStatus("", status.Status, status.ID, status.RecipientID)
				}
				for _, msg := range change.Value.Messages {
					text := strings.TrimSpace(msg.Text.Body)
					if text == "" {
						continue
					}
					if _, _, err := api.Adapter.HandleInbound(r.Context(), msg.From, text, msg.ID, msg.Profile.Name, api.HandleMessage); err != nil {
						writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
						return
					}
				}
			}
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
