package node

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type AuditAppender func(ctx context.Context, action string, target string, meta map[string]any)
type JSONWriter func(w http.ResponseWriter, status int, payload any)

type API struct {
	Nodes       *DeviceManager
	AppendAudit AuditAppender
	WriteJSON   JSONWriter
}

func (api API) HandleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if api.Nodes == nil {
		api.writeJSON(w, http.StatusOK, []any{})
		return
	}
	nodes := api.Nodes.List()
	api.audit(r.Context(), "nodes.read", "nodes", map[string]any{"count": len(nodes)})
	api.writeJSON(w, http.StatusOK, nodes)
}

func (api API) HandleByID(w http.ResponseWriter, r *http.Request) {
	if api.Nodes == nil {
		http.NotFound(w, r)
		return
	}
	path := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/nodes/"))
	if path == "" {
		http.Error(w, "node id required", http.StatusBadRequest)
		return
	}
	nodeID := strings.Split(path, "/")[0]

	switch r.Method {
	case http.MethodGet:
		node, ok := api.Nodes.Get(nodeID)
		if !ok {
			api.writeJSON(w, http.StatusNotFound, map[string]string{"error": "node not found"})
			return
		}
		api.audit(r.Context(), "nodes.read", nodeID, nil)
		api.writeJSON(w, http.StatusOK, node)
	case http.MethodDelete:
		if err := api.Nodes.Unregister(nodeID); err != nil {
			api.writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		api.audit(r.Context(), "nodes.delete", nodeID, nil)
		api.writeJSON(w, http.StatusOK, map[string]string{"status": "unregistered"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (api API) HandleInvoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if api.Nodes == nil {
		api.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no nodes available"})
		return
	}
	var req struct {
		NodeID string         `json:"node_id"`
		Action string         `json:"action"`
		Params map[string]any `json:"params,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if req.NodeID == "" {
		api.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "node_id is required"})
		return
	}
	if req.Action == "" {
		api.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "action is required"})
		return
	}

	result, err := api.Nodes.Invoke(r.Context(), req.NodeID, req.Action, req.Params)
	if err != nil {
		api.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	api.audit(r.Context(), "nodes.invoke", req.NodeID, map[string]any{"action": req.Action})
	api.writeJSON(w, http.StatusOK, result)
}

func (api API) HandleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if api.Nodes == nil {
		api.writeJSON(w, http.StatusOK, map[string]any{"total": 0, "online": 0, "offline": 0})
		return
	}
	api.writeJSON(w, http.StatusOK, api.Nodes.Health())
}

func (api API) audit(ctx context.Context, action string, target string, meta map[string]any) {
	if api.AppendAudit == nil {
		return
	}
	api.AppendAudit(ctx, action, target, meta)
}

func (api API) writeJSON(w http.ResponseWriter, status int, payload any) {
	if api.WriteJSON != nil {
		api.WriteJSON(w, status, payload)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
