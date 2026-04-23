package transport

import (
	"context"
	"encoding/json"
	"net/http"

	runtimepkg "github.com/1024XEngineer/anyclaw/pkg/runtime"
)

type PublicAPI struct {
	Status       StatusDeps
	OnStatusRead func(context.Context)
}

func (api PublicAPI) HandleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (api PublicAPI) HandleRoot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"name":    "AnyClaw Gateway",
		"version": runtimepkg.Version,
		"status":  "running",
		"endpoints": map[string]string{
			"health":     "/healthz",
			"status":     "/status",
			"chat":       "/chat",
			"agents":     "/agents",
			"tasks":      "/tasks",
			"sessions":   "/sessions",
			"channels":   "/channels",
			"plugins":    "/plugins",
			"skills":     "/skills",
			"tools":      "/tools",
			"websocket":  "/ws",
			"openai_api": "/v1/chat/completions",
			"models":     "/v1/models",
			"responses":  "/v1/responses",
			"webhooks":   "/webhooks/",
			"nodes":      "/nodes",
			"cron":       "/cron",
			"pairing":    "/device/pairing",
		},
	})
}

func (api PublicAPI) HandleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if api.OnStatusRead != nil {
		api.OnStatusRead(r.Context())
	}
	if r.URL.Query().Get("extended") == "true" {
		writeJSON(w, http.StatusOK, GatewaySnapshot(api.Status))
		return
	}
	writeJSON(w, http.StatusOK, StatusSnapshot(api.Status))
}

func writeJSON(w http.ResponseWriter, statusCode int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(value)
}
