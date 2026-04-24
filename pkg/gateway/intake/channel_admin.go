package intake

import (
	"encoding/json"
	"net/http"
	"time"

	inputlayer "github.com/1024XEngineer/anyclaw/pkg/input"
)

type MentionGate interface {
	IsEnabled() bool
	SetEnabled(enabled bool)
}

type GroupSecurity interface {
	AllowGroup(groupID string)
	DenyGroup(groupID string)
	AllowUser(userID string)
	DenyUser(userID string)
}

type ChannelPairing interface {
	IsEnabled() bool
	ListPaired() []inputlayer.PairingInfo
	Pair(userID, deviceID, channel, displayName string, ttl time.Duration) inputlayer.PairingInfo
	Unpair(userID, deviceID, channel string)
}

type ContactDirectory interface {
	List(channel string) []inputlayer.ContactInfo
	Search(query string) []inputlayer.ContactInfo
}

type MentionGateAPI struct {
	Gate MentionGate
}

func (api MentionGateAPI) Handle(w http.ResponseWriter, r *http.Request) {
	if api.Gate == nil {
		http.Error(w, "mention gate not initialized", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{
			"enabled": api.Gate.IsEnabled(),
		})
	case http.MethodPost:
		var req struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		api.Gate.SetEnabled(req.Enabled)
		writeJSON(w, http.StatusOK, map[string]any{"status": "updated", "enabled": req.Enabled})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

type GroupSecurityAPI struct {
	Security GroupSecurity
}

func (api GroupSecurityAPI) Handle(w http.ResponseWriter, r *http.Request) {
	if api.Security == nil {
		http.Error(w, "group security not initialized", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	case http.MethodPost:
		var req struct {
			Action  string `json:"action"`
			UserID  string `json:"user_id"`
			GroupID string `json:"group_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		switch req.Action {
		case "allow_group":
			api.Security.AllowGroup(req.GroupID)
		case "deny_group":
			api.Security.DenyGroup(req.GroupID)
		case "allow_user":
			api.Security.AllowUser(req.UserID)
		case "deny_user":
			api.Security.DenyUser(req.UserID)
		default:
			http.Error(w, "unknown action", http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "updated"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

type ChannelPairingAPI struct {
	Pairing ChannelPairing
}

func (api ChannelPairingAPI) Handle(w http.ResponseWriter, r *http.Request) {
	if api.Pairing == nil {
		http.Error(w, "channel pairing not initialized", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{
			"enabled": api.Pairing.IsEnabled(),
			"paired":  api.Pairing.ListPaired(),
		})
	case http.MethodPost:
		var req struct {
			Action      string `json:"action"`
			UserID      string `json:"user_id"`
			DeviceID    string `json:"device_id"`
			Channel     string `json:"channel"`
			DisplayName string `json:"display_name"`
			TTL         int    `json:"ttl_seconds"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		switch req.Action {
		case "pair":
			ttl := time.Duration(req.TTL) * time.Second
			if ttl <= 0 {
				ttl = 24 * time.Hour
			}
			api.Pairing.Pair(req.UserID, req.DeviceID, req.Channel, req.DisplayName, ttl)
		case "unpair":
			api.Pairing.Unpair(req.UserID, req.DeviceID, req.Channel)
		default:
			http.Error(w, "unknown action", http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "updated"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

type ContactsAPI struct {
	Directory ContactDirectory
}

func (api ContactsAPI) Handle(w http.ResponseWriter, r *http.Request) {
	if api.Directory == nil {
		http.Error(w, "contact directory not initialized", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		channel := r.URL.Query().Get("channel")
		query := r.URL.Query().Get("q")
		if query != "" {
			writeJSON(w, http.StatusOK, api.Directory.Search(query))
			return
		}
		writeJSON(w, http.StatusOK, api.Directory.List(channel))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
