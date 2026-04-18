package gateway

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/config"
	gatewayauth "github.com/1024XEngineer/anyclaw/pkg/gateway/auth"
)

func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if !HasPermission(UserFromContext(r.Context()), "auth.users.read") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "auth.users.read"})
			return
		}
		s.appendAudit(UserFromContext(r.Context()), "auth.users.read", "users", nil)
		writeJSON(w, http.StatusOK, s.listUserViews())
	case http.MethodPost:
		s.handleUserUpsert(w, r)
	case http.MethodDelete:
		s.handleUserDelete(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) listUserViews() []map[string]any {
	rolesIndex := map[string]config.SecurityRole{}
	for _, role := range s.mainRuntime.Config.Security.Roles {
		rolesIndex[role.Name] = role
	}
	for _, role := range gatewayauth.BuiltinRoleTemplates() {
		rolesIndex[role.Name] = role
	}
	items := make([]map[string]any, 0, len(s.mainRuntime.Config.Security.Users))
	for _, user := range s.mainRuntime.Config.Security.Users {
		effective := append([]string{}, user.PermissionOverrides...)
		if role, ok := rolesIndex[user.Role]; ok {
			effective = append(append([]string{}, role.Permissions...), user.PermissionOverrides...)
		}
		items = append(items, map[string]any{
			"name":                 user.Name,
			"role":                 user.Role,
			"permissions":          effective,
			"permission_overrides": user.PermissionOverrides,
			"scopes":               user.Scopes,
			"orgs":                 user.Orgs,
			"projects":             user.Projects,
			"workspaces":           user.Workspaces,
		})
	}
	return items
}

func (s *Server) handleUserUpsert(w http.ResponseWriter, r *http.Request) {
	if !HasPermission(UserFromContext(r.Context()), "auth.users.read") {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "auth.users.read"})
		return
	}
	var user config.SecurityUser
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if strings.TrimSpace(user.Name) == "" || strings.TrimSpace(user.Token) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and token are required"})
		return
	}
	for _, permission := range user.PermissionOverrides {
		if !allowedSecurityPermissions()[permission] {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown permission", "permission": permission})
			return
		}
	}
	for _, existing := range s.mainRuntime.Config.Security.Users {
		if existing.Name != user.Name && existing.Token == user.Token {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "token already in use"})
			return
		}
	}
	updated := false
	for i := range s.mainRuntime.Config.Security.Users {
		if s.mainRuntime.Config.Security.Users[i].Name == user.Name {
			s.mainRuntime.Config.Security.Users[i] = user
			updated = true
			break
		}
	}
	if !updated {
		s.mainRuntime.Config.Security.Users = append(s.mainRuntime.Config.Security.Users, user)
	}
	if err := s.mainRuntime.Config.Save(s.mainRuntime.ConfigPath); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.appendAudit(UserFromContext(r.Context()), "auth.users.write", user.Name, nil)
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) handleUserDelete(w http.ResponseWriter, r *http.Request) {
	if !HasPermission(UserFromContext(r.Context()), "auth.users.read") {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "auth.users.read"})
		return
	}
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	filtered := make([]config.SecurityUser, 0, len(s.mainRuntime.Config.Security.Users))
	removed := false
	for _, user := range s.mainRuntime.Config.Security.Users {
		if user.Name == name {
			removed = true
			continue
		}
		filtered = append(filtered, user)
	}
	if !removed {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}
	s.mainRuntime.Config.Security.Users = filtered
	if err := s.mainRuntime.Config.Save(s.mainRuntime.ConfigPath); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.appendAudit(UserFromContext(r.Context()), "auth.users.delete", name, nil)
	writeJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
}

func allowedSecurityPermissions() map[string]bool {
	return map[string]bool{
		"*":               true,
		"status.read":     true,
		"chat.send":       true,
		"tasks.read":      true,
		"tasks.write":     true,
		"approvals.read":  true,
		"approvals.write": true,
		"sessions.read":   true,
		"sessions.write":  true,
		"memory.read":     true,
		"events.read":     true,
		"tools.read":      true,
		"plugins.read":    true,
		"channels.read":   true,
		"routing.read":    true,
		"runtimes.read":   true,
		"runtimes.write":  true,
		"resources.read":  true,
		"resources.write": true,
		"config.read":     true,
		"config.write":    true,
		"audit.read":      true,
		"auth.users.read": true,
	}
}
