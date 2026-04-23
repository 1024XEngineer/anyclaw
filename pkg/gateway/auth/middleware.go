package auth

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/config"
)

type Middleware struct {
	cfg *config.SecurityConfig
}

func NewMiddleware(cfg *config.SecurityConfig) *Middleware {
	return &Middleware{cfg: cfg}
}

func (m *Middleware) Wrap(path string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminCtx := func(req *http.Request) *http.Request {
			admin := &User{Name: "local-admin", Role: "admin", Permissions: []string{"*"}}
			return req.WithContext(WithUser(req.Context(), admin))
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
		next(w, r.WithContext(WithUser(r.Context(), user)))
	}
}

func (m *Middleware) authenticate(provided string, fallbackToken string) (*User, bool) {
	if fallbackToken != "" && subtle.ConstantTimeCompare([]byte(provided), []byte(fallbackToken)) == 1 {
		return &User{Name: "admin", Role: "admin", Permissions: []string{"*"}}, true
	}
	for _, user := range m.cfg.Users {
		if user.Token == "" {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(provided), []byte(user.Token)) == 1 {
			permissions := resolveRolePermissions(m.cfg, user.Role)
			permissions = append(permissions, user.PermissionOverrides...)
			return &User{Name: user.Name, Role: user.Role, Permissions: permissions, PermissionOverrides: user.PermissionOverrides, Scopes: user.Scopes, Orgs: user.Orgs, Projects: user.Projects, Workspaces: user.Workspaces}, true
		}
	}
	return nil, false
}

func (m *Middleware) requiresAuth(path string) bool {
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
