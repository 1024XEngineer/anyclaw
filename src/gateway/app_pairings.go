package gateway

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	appstore "github.com/anyclaw/anyclaw/pkg/apps"
)

type appPairingView struct {
	ID          string            `json:"id"`
	App         string            `json:"app"`
	Workflow    string            `json:"workflow"`
	Binding     string            `json:"binding,omitempty"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Enabled     bool              `json:"enabled"`
	Org         string            `json:"org,omitempty"`
	Project     string            `json:"project,omitempty"`
	Workspace   string            `json:"workspace,omitempty"`
	Triggers    []string          `json:"triggers,omitempty"`
	Defaults    map[string]any    `json:"defaults,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	UpdatedAt   string            `json:"updated_at,omitempty"`
	CreatedAt   string            `json:"created_at,omitempty"`
}

func pairingToView(pairing *appstore.Pairing) appPairingView {
	if pairing == nil {
		return appPairingView{}
	}
	view := appPairingView{
		ID:          pairing.ID,
		App:         pairing.App,
		Workflow:    pairing.Workflow,
		Binding:     pairing.Binding,
		Name:        pairing.Name,
		Description: pairing.Description,
		Enabled:     pairing.Enabled,
		Org:         pairing.Org,
		Project:     pairing.Project,
		Workspace:   pairing.Workspace,
		Triggers:    append([]string{}, pairing.Triggers...),
		Defaults:    cloneAnyValues(pairing.Defaults),
		Metadata:    cloneBindingConfig(pairing.Metadata),
	}
	if !pairing.CreatedAt.IsZero() {
		view.CreatedAt = pairing.CreatedAt.Format(time.RFC3339)
	}
	if !pairing.UpdatedAt.IsZero() {
		view.UpdatedAt = pairing.UpdatedAt.Format(time.RFC3339)
	}
	return view
}

func cloneAnyValues(items map[string]any) map[string]any {
	if len(items) == 0 {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(items))
	for key, value := range items {
		cloned[key] = value
	}
	return cloned
}

func (s *Server) listAppPairingViews(appFilter string) ([]appPairingView, error) {
	store, err := newAppStore(s.app.ConfigPath)
	if err != nil {
		return nil, err
	}
	appFilter = strings.TrimSpace(appFilter)
	items := store.ListPairings()
	views := make([]appPairingView, 0, len(items))
	for _, item := range items {
		if appFilter != "" && !strings.EqualFold(strings.TrimSpace(item.App), appFilter) {
			continue
		}
		views = append(views, pairingToView(item))
	}
	return views, nil
}

func (s *Server) handleAppPairings(w http.ResponseWriter, r *http.Request) {
	store, err := newAppStore(s.app.ConfigPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	switch r.Method {
	case http.MethodGet:
		if !HasPermission(UserFromContext(r.Context()), "apps.read") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "apps.read"})
			return
		}
		appFilter := strings.TrimSpace(r.URL.Query().Get("app"))
		views, err := s.listAppPairingViews(appFilter)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		s.appendAudit(UserFromContext(r.Context()), "apps.read", "app-pairings", map[string]any{"count": len(views), "app": appFilter})
		writeJSON(w, http.StatusOK, views)
	case http.MethodPost, http.MethodPatch:
		if !HasPermission(UserFromContext(r.Context()), "apps.write") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "apps.write"})
			return
		}
		var req appstore.Pairing
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		if err := store.UpsertPairing(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		var saved *appstore.Pairing
		if strings.TrimSpace(req.ID) != "" {
			saved, _ = store.GetPairing(req.ID)
		}
		if saved == nil {
			for _, item := range store.ListPairingsByApp(req.App) {
				if strings.EqualFold(strings.TrimSpace(item.Name), strings.TrimSpace(req.Name)) &&
					strings.EqualFold(strings.TrimSpace(item.Workflow), strings.TrimSpace(req.Workflow)) {
					saved = item
					break
				}
			}
		}
		s.appendAudit(UserFromContext(r.Context()), "apps.write", req.App+":"+req.Name, map[string]any{"workflow": req.Workflow, "enabled": req.Enabled})
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "pairing": pairingToView(saved)})
	case http.MethodDelete:
		if !HasPermission(UserFromContext(r.Context()), "apps.write") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "apps.write"})
			return
		}
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		if id == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
			return
		}
		if err := store.DeletePairing(id); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		s.appendAudit(UserFromContext(r.Context()), "apps.write", "app-pairing:"+id, map[string]any{"deleted": true})
		writeJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
