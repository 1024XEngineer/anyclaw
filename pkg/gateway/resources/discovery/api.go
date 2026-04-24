package discovery

import (
	"encoding/json"
	"net/http"
)

type InstanceSource interface {
	Instances() []*Instance
	SendQuery()
}

type API struct {
	Service InstanceSource
}

func (api API) HandleInstances(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if api.Service == nil {
		writeJSON(w, http.StatusOK, map[string]any{"instances": []any{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"instances": api.Service.Instances()})
}

func (api API) HandleQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if api.Service == nil {
		writeJSON(w, http.StatusOK, map[string]any{"status": "discovery not enabled"})
		return
	}
	api.Service.SendQuery()
	writeJSON(w, http.StatusOK, map[string]any{"status": "query sent"})
}

func writeJSON(w http.ResponseWriter, statusCode int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(value)
}
