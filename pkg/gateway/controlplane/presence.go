package controlplane

import (
	"encoding/json"
	"net/http"
)

type PresenceGetter func(channel string, userID string) (any, bool)
type PresenceLister func() any
type PresenceJSONWriter func(http.ResponseWriter, int, any)

// PresenceAPI exposes channel presence as part of the gateway control plane.
type PresenceAPI struct {
	Get       PresenceGetter
	List      PresenceLister
	WriteJSON PresenceJSONWriter
}

// Handle returns either a single channel presence record or the active set.
func (api PresenceAPI) Handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if api.Get == nil || api.List == nil {
		http.Error(w, "presence manager not initialized", http.StatusServiceUnavailable)
		return
	}

	channelID := r.URL.Query().Get("channel")
	userID := r.URL.Query().Get("user_id")
	if channelID != "" && userID != "" {
		info, ok := api.Get(channelID, userID)
		if !ok {
			api.writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
			return
		}
		api.writeJSON(w, http.StatusOK, info)
		return
	}

	api.writeJSON(w, http.StatusOK, api.List())
}

func (api PresenceAPI) writeJSON(w http.ResponseWriter, statusCode int, value any) {
	if api.WriteJSON != nil {
		api.WriteJSON(w, statusCode, value)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(value)
}
