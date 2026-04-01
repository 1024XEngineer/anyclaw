package gateway

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	appstore "github.com/anyclaw/anyclaw/pkg/apps"
)

type appBindingView struct {
	ID          string            `json:"id"`
	App         string            `json:"app"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Enabled     bool              `json:"enabled"`
	Org         string            `json:"org,omitempty"`
	Project     string            `json:"project,omitempty"`
	Workspace   string            `json:"workspace,omitempty"`
	Target      string            `json:"target,omitempty"`
	Config      map[string]string `json:"config,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	HasSecrets  bool              `json:"has_secrets"`
	SecretKeys  []string          `json:"secret_keys,omitempty"`
	UpdatedAt   string            `json:"updated_at,omitempty"`
	CreatedAt   string            `json:"created_at,omitempty"`
}

func bindingToView(binding *appstore.Binding) appBindingView {
	if binding == nil {
		return appBindingView{}
	}
	view := appBindingView{
		ID:          binding.ID,
		App:         binding.App,
		Name:        binding.Name,
		Description: binding.Description,
		Enabled:     binding.Enabled,
		Org:         binding.Org,
		Project:     binding.Project,
		Workspace:   binding.Workspace,
		Target:      binding.Target,
		Config:      cloneBindingConfig(binding.Config),
		Metadata:    cloneBindingConfig(binding.Metadata),
		HasSecrets:  len(binding.Secrets) > 0,
	}
	if !binding.CreatedAt.IsZero() {
		view.CreatedAt = binding.CreatedAt.Format(time.RFC3339)
	}
	if !binding.UpdatedAt.IsZero() {
		view.UpdatedAt = binding.UpdatedAt.Format(time.RFC3339)
	}
	if len(binding.Secrets) > 0 {
		keys := make([]string, 0, len(binding.Secrets))
		for key := range binding.Secrets {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		view.SecretKeys = keys
	}
	return view
}

func cloneBindingConfig(items map[string]string) map[string]string {
	if len(items) == 0 {
		return map[string]string{}
	}
	cloned := make(map[string]string, len(items))
	for key, value := range items {
		cloned[key] = value
	}
	return cloned
}

func (s *Server) listAppBindingViews(appFilter string) ([]appBindingView, error) {
	store, err := newAppStore(s.app.ConfigPath)
	if err != nil {
		return nil, err
	}
	appFilter = strings.TrimSpace(appFilter)
	items := store.List()
	views := make([]appBindingView, 0, len(items))
	for _, item := range items {
		if appFilter != "" && !strings.EqualFold(strings.TrimSpace(item.App), appFilter) {
			continue
		}
		views = append(views, bindingToView(item))
	}
	return views, nil
}

func (s *Server) handleAppBindings(w http.ResponseWriter, r *http.Request) {
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
		views, err := s.listAppBindingViews(appFilter)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		s.appendAudit(UserFromContext(r.Context()), "apps.read", "app-bindings", map[string]any{"count": len(views), "app": appFilter})
		writeJSON(w, http.StatusOK, views)
	case http.MethodPost, http.MethodPatch:
		if !HasPermission(UserFromContext(r.Context()), "apps.write") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "apps.write"})
			return
		}
		var req appstore.Binding
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		if err := store.Upsert(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		var saved *appstore.Binding
		if strings.TrimSpace(req.ID) != "" {
			saved, _ = store.Get(req.ID)
		}
		if saved == nil {
			for _, item := range store.ListByApp(req.App) {
				if strings.EqualFold(strings.TrimSpace(item.Name), strings.TrimSpace(req.Name)) {
					saved = item
					break
				}
			}
		}
		s.appendAudit(UserFromContext(r.Context()), "apps.write", req.App+":"+req.Name, map[string]any{"enabled": req.Enabled})
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "binding": bindingToView(saved)})
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
		if err := store.Delete(id); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		s.appendAudit(UserFromContext(r.Context()), "apps.write", "app-binding:"+id, map[string]any{"deleted": true})
		writeJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
