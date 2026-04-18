package approvals

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	gatewayauth "github.com/1024XEngineer/anyclaw/pkg/gateway/auth"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

type API struct {
	Store       *state.Store
	Approvals   *state.ApprovalManager
	CurrentUser func(context.Context) *gatewayauth.User
	AppendAudit func(user *gatewayauth.User, action string, target string, meta map[string]any)
	AppendEvent func(eventType string, sessionID string, payload map[string]any)
	OnResolved  func(updated *state.Approval, approved bool, comment string)
}

func (api API) HandleList(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		status := strings.TrimSpace(r.URL.Query().Get("status"))
		items := []*state.Approval{}
		if api.Store != nil {
			items = api.Store.ListApprovals(status)
		}
		api.appendAudit(api.currentUser(r.Context()), "approvals.read", "approvals", map[string]any{
			"count":  len(items),
			"status": status,
		})
		writeJSON(w, http.StatusOK, items)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (api API) HandleByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/approvals/"))
	if path == "" {
		http.Error(w, "approval id required", http.StatusBadRequest)
		return
	}
	if api.Store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "approval store not available"})
		return
	}

	parts := strings.Split(path, "/")
	id := strings.TrimSpace(parts[0])
	approval, ok := api.Store.GetApproval(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "approval not found"})
		return
	}
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, http.StatusOK, approval)
		return
	}
	if len(parts) == 2 && parts[1] == "resolve" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if api.Approvals == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "approval manager not available"})
			return
		}

		var req struct {
			Approved bool   `json:"approved"`
			Comment  string `json:"comment"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}

		user := api.currentUser(r.Context())
		actor := "anonymous"
		if user != nil {
			actor = user.Name
		}

		updated, err := api.Approvals.Resolve(id, req.Approved, actor, req.Comment)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		api.resolve(updated, req.Approved, req.Comment)
		api.appendAudit(user, "approvals.write", id, map[string]any{"approved": req.Approved})
		if updated.TaskID != "" || updated.SessionID != "" {
			payload := map[string]any{
				"approval_id": updated.ID,
				"status":      updated.Status,
			}
			if updated.TaskID != "" {
				payload["task_id"] = updated.TaskID
			}
			if updated.ToolName != "" {
				payload["tool_name"] = updated.ToolName
			}
			api.appendEvent("approval.resolved", updated.SessionID, payload)
		}
		writeJSON(w, http.StatusOK, updated)
		return
	}
	http.NotFound(w, r)
}

func (api API) currentUser(ctx context.Context) *gatewayauth.User {
	if api.CurrentUser == nil {
		return nil
	}
	return api.CurrentUser(ctx)
}

func (api API) appendAudit(user *gatewayauth.User, action string, target string, meta map[string]any) {
	if api.AppendAudit == nil {
		return
	}
	api.AppendAudit(user, action, target, meta)
}

func (api API) appendEvent(eventType string, sessionID string, payload map[string]any) {
	if api.AppendEvent == nil {
		return
	}
	api.AppendEvent(eventType, sessionID, payload)
}

func (api API) resolve(updated *state.Approval, approved bool, comment string) {
	if api.OnResolved == nil {
		return
	}
	api.OnResolved(updated, approved, comment)
}

func writeJSON(w http.ResponseWriter, statusCode int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(value)
}
