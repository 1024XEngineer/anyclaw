package gateway

import (
	"encoding/json"
	"net/http"

	appsecurity "github.com/1024XEngineer/anyclaw/pkg/gateway/auth/security"
)

func (s *Server) handleDevicePairing(w http.ResponseWriter, r *http.Request) {
	if s.devicePairing == nil {
		http.Error(w, "device pairing not initialized", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		action := r.URL.Query().Get("action")
		if action == "list" || action == "" {
			devices := s.devicePairing.ListPaired()
			writeJSON(w, http.StatusOK, map[string]any{"devices": devices, "status": s.devicePairing.GetStatus()})
			return
		}
		if action == "status" {
			writeJSON(w, http.StatusOK, s.devicePairing.GetStatus())
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid action"})
	case http.MethodPost:
		var req appsecurity.PairingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}

		resp, err := s.devicePairing.HandleRequest(r.Context(), req)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !resp.OK {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": resp.Error})
			return
		}
		writeJSON(w, http.StatusOK, resp)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleDevicePairingCode(w http.ResponseWriter, r *http.Request) {
	if s.devicePairing == nil {
		http.Error(w, "device pairing not initialized", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		DeviceName string `json:"device_name"`
		DeviceType string `json:"device_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	code, err := s.devicePairing.GeneratePairingCode(req.DeviceName, req.DeviceType)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"code":    code.Code,
		"expires": code.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
		"device":  code.DeviceName,
		"type":    code.DeviceType,
	})
}
