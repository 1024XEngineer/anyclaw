package gateway

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/anyclaw/anyclaw/pkg/config"
)

type authMiddleware struct {
	cfg *config.SecurityConfig
}

type authContextKey string

const authUserKey authContextKey = "auth-user"

type AuthUser struct {
	Name                string   `json:"name"`
	Role                string   `json:"role"`
	Permissions         []string `json:"permissions"`
	PermissionOverrides []string `json:"permission_overrides"`
	Scopes              []string `json:"scopes"`
	Orgs                []string `json:"orgs"`
	Projects            []string `json:"projects"`
	Workspaces          []string `json:"workspaces"`
}

func newAuthMiddleware(cfg *config.SecurityConfig) *authMiddleware {
	return &authMiddleware{cfg: cfg}
}

func (m *authMiddleware) Wrap(path string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminCtx := func(req *http.Request) *http.Request {
			admin := &AuthUser{Name: "local-admin", Role: "admin", Permissions: []string{"*"}}
			return req.WithContext(context.WithValue(req.Context(), authUserKey, admin))
		}
		if !m.requiresAuth(path) {
			next(w, adminCtx(r))
			return
		}
		token := strings.TrimSpace(m.cfg.APIToken)
		if token == "" && len(m.cfg.Users) == 0 {
			next(w, adminCtx(r))
			return
		}
		provided := bearerToken(r.Header.Get("Authorization"))
		if provided == "" && r.URL.Query().Get("token") != "" {
			provided = strings.TrimSpace(r.URL.Query().Get("token"))
		}
		user, ok := m.authenticate(provided, token)
		if !ok {
			w.Header().Set("WWW-Authenticate", `Bearer realm="anyclaw"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), authUserKey, user)))
	}
}

func (m *authMiddleware) authenticate(provided string, fallbackToken string) (*AuthUser, bool) {
	if fallbackToken != "" && subtle.ConstantTimeCompare([]byte(provided), []byte(fallbackToken)) == 1 {
		return &AuthUser{Name: "admin", Role: "admin", Permissions: []string{"*"}}, true
	}
	for _, user := range m.cfg.Users {
		if user.Token == "" {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(provided), []byte(user.Token)) == 1 {
			permissions := resolveRolePermissions(m.cfg, user.Role)
			permissions = append(permissions, user.PermissionOverrides...)
			return &AuthUser{Name: user.Name, Role: user.Role, Permissions: permissions, PermissionOverrides: user.PermissionOverrides, Scopes: user.Scopes, Orgs: user.Orgs, Projects: user.Projects, Workspaces: user.Workspaces}, true
		}
	}
	return nil, false
}

func resolveRolePermissions(cfg *config.SecurityConfig, roleName string) []string {
	if roleName == "admin" {
		return []string{"*"}
	}
	if cfg != nil {
		for _, role := range cfg.Roles {
			if role.Name == roleName {
				return append([]string{}, role.Permissions...)
			}
		}
	}
	for _, role := range builtinRoleTemplates() {
		if role.Name == roleName {
			return append([]string{}, role.Permissions...)
		}
	}
	switch roleName {
	case "viewer":
		return []string{"status.read", "sessions.read", "events.read", "audit.read", "plugins.read", "channels.read", "routing.read", "runtimes.read", "resources.read"}
	default:
		return nil
	}
}

func (m *authMiddleware) requiresAuth(path string) bool {
	if m == nil || m.cfg == nil {
		return false
	}
	for _, publicPath := range m.cfg.PublicPaths {
		if strings.TrimSpace(publicPath) == path {
			return false
		}
	}
	if strings.HasPrefix(path, "/events") && !m.cfg.ProtectEvents {
		return false
	}
	return true
}

func bearerToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) >= 7 && strings.EqualFold(value[:7], "Bearer ") {
		return strings.TrimSpace(value[7:])
	}
	return ""
}

func UserFromContext(ctx context.Context) *AuthUser {
	user, _ := ctx.Value(authUserKey).(*AuthUser)
	return user
}

func HasPermission(user *AuthUser, permission string) bool {
	if permission == "" {
		return true
	}
	if user == nil {
		return false
	}
	if user.Role == "admin" {
		return true
	}
	for _, granted := range user.Permissions {
		if granted == "*" || granted == permission {
			return true
		}
	}
	return false
}

func HasScope(user *AuthUser, workspace string) bool {
	if workspace == "" {
		return true
	}
	if user == nil {
		return false
	}
	if user.Role == "admin" {
		return true
	}
	if len(user.Scopes) == 0 {
		return false
	}
	for _, scope := range user.Scopes {
		if scope == "*" || scope == workspace {
			return true
		}
	}
	return false
}

func HasHierarchyAccess(user *AuthUser, org string, project string, workspace string) bool {
	if user == nil {
		return false
	}
	if user.Role == "admin" {
		return true
	}
	if workspace != "" {
		for _, item := range user.Workspaces {
			if item == "*" || item == workspace {
				return true
			}
		}
	}
	if project != "" {
		for _, item := range user.Projects {
			if item == "*" || item == project {
				return true
			}
		}
	}
	if org != "" {
		for _, item := range user.Orgs {
			if item == "*" || item == org {
				return true
			}
		}
		return false
	}
	return HasScope(user, workspace)
}
