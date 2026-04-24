package gateway

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/config"
	gatewayauth "github.com/1024XEngineer/anyclaw/pkg/gateway/auth"
)

func (s *Server) handleRoles(w http.ResponseWriter, r *http.Request) {
	builtinRoles := make([]map[string]any, 0, len(gatewayauth.BuiltinRoleTemplates()))
	for _, role := range gatewayauth.BuiltinRoleTemplates() {
		builtinRoles = append(builtinRoles, map[string]any{
			"name":        role.Name,
			"description": role.Description,
			"permissions": role.Permissions,
		})
	}
	switch r.Method {
	case http.MethodGet:
		if !HasPermission(UserFromContext(r.Context()), "auth.users.read") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "auth.users.read"})
			return
		}
		roles := append([]map[string]any{}, builtinRoles...)
		for _, role := range s.mainRuntime.Config.Security.Roles {
			roles = append(roles, map[string]any{"name": role.Name, "description": role.Description, "permissions": role.Permissions, "custom": true})
		}
		writeJSON(w, http.StatusOK, roles)
	case http.MethodPost:
		if !HasPermission(UserFromContext(r.Context()), "auth.users.read") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "auth.users.read"})
			return
		}
		var role config.SecurityRole
		if err := json.NewDecoder(r.Body).Decode(&role); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		if strings.TrimSpace(role.Name) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "role name is required"})
			return
		}
		updated := false
		for i := range s.mainRuntime.Config.Security.Roles {
			if s.mainRuntime.Config.Security.Roles[i].Name == role.Name {
				s.mainRuntime.Config.Security.Roles[i] = role
				updated = true
				break
			}
		}
		if !updated {
			s.mainRuntime.Config.Security.Roles = append(s.mainRuntime.Config.Security.Roles, role)
		}
		if err := s.mainRuntime.Config.Save(s.mainRuntime.ConfigPath); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		s.appendAudit(UserFromContext(r.Context()), "auth.roles.write", role.Name, nil)
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	case http.MethodDelete:
		if !HasPermission(UserFromContext(r.Context()), "auth.users.read") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": "auth.users.read"})
			return
		}
		name := strings.TrimSpace(r.URL.Query().Get("name"))
		if name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
			return
		}
		filtered := make([]config.SecurityRole, 0, len(s.mainRuntime.Config.Security.Roles))
		removed := false
		for _, role := range s.mainRuntime.Config.Security.Roles {
			if role.Name == name {
				removed = true
				continue
			}
			filtered = append(filtered, role)
		}
		if !removed {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "role not found"})
			return
		}
		s.mainRuntime.Config.Security.Roles = filtered
		if err := s.mainRuntime.Config.Save(s.mainRuntime.ConfigPath); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		s.appendAudit(UserFromContext(r.Context()), "auth.roles.delete", name, nil)
		writeJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleRoleImpact(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	roles := []config.SecurityRole{}
	roles = append(roles, gatewayauth.BuiltinRoleTemplates()...)
	roles = append(roles, s.mainRuntime.Config.Security.Roles...)
	impact := make([]map[string]any, 0, len(roles))
	for _, role := range roles {
		users := []string{}
		for _, user := range s.mainRuntime.Config.Security.Users {
			if user.Role == role.Name {
				users = append(users, user.Name)
			}
		}
		impact = append(impact, map[string]any{
			"name":        role.Name,
			"description": role.Description,
			"permissions": role.Permissions,
			"user_count":  len(users),
			"users":       users,
		})
	}
	writeJSON(w, http.StatusOK, impact)
}
