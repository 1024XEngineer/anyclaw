package resources

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/state"
)

type RuntimePool interface {
	InvalidateByProject(projectID string)
	InvalidateByWorkspace(workspaceID string)
}

type PermissionChecker func(r *http.Request, permission string) bool
type AuditAppender func(r *http.Request, action string, target string, meta map[string]any)
type JSONWriter func(w http.ResponseWriter, status int, payload any)

type API struct {
	Store           *state.Store
	RuntimePool     RuntimePool
	CheckPermission PermissionChecker
	AppendAudit     AuditAppender
	WriteJSON       JSONWriter
}

func (api API) HandleCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		api.handleRead(w, r)
	case http.MethodPost:
		api.handleCreate(w, r)
	case http.MethodPatch:
		api.handleUpdate(w, r)
	case http.MethodDelete:
		api.handleDelete(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (api API) handleRead(w http.ResponseWriter, r *http.Request) {
	if !api.allowed(r, "resources.read") {
		api.writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "resources.read"})
		return
	}
	api.audit(r, "resources.read", "resources", nil)
	api.writeJSON(w, http.StatusOK, map[string]any{
		"orgs":       api.Store.ListOrgs(),
		"projects":   api.Store.ListProjects(),
		"workspaces": api.Store.ListWorkspaces(),
	})
}

func (api API) handleCreate(w http.ResponseWriter, r *http.Request) {
	if !api.allowed(r, "resources.write") {
		api.writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "resources.write"})
		return
	}
	req, ok := decodeResourceMutation(w, r, api.writeJSON)
	if !ok {
		return
	}
	if req.Org != nil {
		if err := api.Store.UpsertOrg(req.Org); err != nil {
			api.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}
	if req.Project != nil {
		if err := api.Store.UpsertProject(req.Project); err != nil {
			api.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}
	if req.Workspace != nil {
		if err := api.Store.UpsertWorkspace(req.Workspace); err != nil {
			api.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}
	api.audit(r, "resources.write", "resources", nil)
	api.writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (api API) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if !api.allowed(r, "resources.write") {
		api.writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "resources.write"})
		return
	}
	req, ok := decodeResourceMutation(w, r, api.writeJSON)
	if !ok {
		return
	}
	if req.Org != nil {
		if err := api.Store.UpsertOrg(req.Org); err != nil {
			api.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
	}
	if req.Project != nil {
		if err := api.Store.UpsertProject(req.Project); err != nil {
			api.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := api.Store.RebindSessionsForProject(req.Project.ID, req.Project.OrgID); err != nil {
			api.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if api.RuntimePool != nil {
			api.RuntimePool.InvalidateByProject(req.Project.ID)
		}
		api.audit(r, "runtimes.invalidate", req.Project.ID, map[string]any{"reason": "project update"})
	}
	if req.Workspace != nil {
		if err := api.Store.UpsertWorkspace(req.Workspace); err != nil {
			api.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		project, ok := api.Store.GetProject(req.Workspace.ProjectID)
		if ok {
			if err := api.Store.RebindSessionsForWorkspace(req.Workspace.ID, project.ID, project.OrgID); err != nil {
				api.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
		}
		if api.RuntimePool != nil {
			api.RuntimePool.InvalidateByWorkspace(req.Workspace.ID)
		}
		api.audit(r, "runtimes.invalidate", req.Workspace.ID, map[string]any{"reason": "workspace update"})
	}
	api.audit(r, "resources.update", "resources", nil)
	api.writeJSON(w, http.StatusOK, map[string]any{"status": "updated"})
}

func (api API) handleDelete(w http.ResponseWriter, r *http.Request) {
	if !api.allowed(r, "resources.write") {
		api.writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "resources.write"})
		return
	}
	kind := strings.TrimSpace(r.URL.Query().Get("kind"))
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if kind == "" || id == "" {
		api.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "kind and id are required"})
		return
	}
	var err error
	switch kind {
	case "org":
		err = api.Store.DeleteOrg(id)
	case "project":
		err = api.Store.DeleteProject(id)
	case "workspace":
		err = api.Store.DeleteWorkspace(id)
	default:
		api.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported resource kind"})
		return
	}
	if err != nil {
		api.writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	api.audit(r, "resources.delete", kind+":"+id, nil)
	api.writeJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
}

type resourceMutationRequest struct {
	Org       *state.Org       `json:"org"`
	Project   *state.Project   `json:"project"`
	Workspace *state.Workspace `json:"workspace"`
}

func decodeResourceMutation(w http.ResponseWriter, r *http.Request, writeJSON JSONWriter) (resourceMutationRequest, bool) {
	var req resourceMutationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return resourceMutationRequest{}, false
	}
	return req, true
}

func (api API) allowed(r *http.Request, permission string) bool {
	if api.CheckPermission == nil {
		return true
	}
	return api.CheckPermission(r, permission)
}

func (api API) audit(r *http.Request, action string, target string, meta map[string]any) {
	if api.AppendAudit == nil {
		return
	}
	api.AppendAudit(r, action, target, meta)
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
