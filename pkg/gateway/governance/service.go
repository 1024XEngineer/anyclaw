package governance

import (
	"context"
	"encoding/json"
	"net/http"

	gatewayauth "github.com/1024XEngineer/anyclaw/pkg/gateway/auth"
	gatewaymiddleware "github.com/1024XEngineer/anyclaw/pkg/gateway/middleware"
)

type UserResolver func(context.Context) *gatewayauth.User

type HierarchyResolver func(*http.Request) (string, string, string)

// Service owns reusable gateway governance helpers such as auth, rate-limit and permission gates.
type Service struct {
	Auth        *gatewayauth.Middleware
	RateLimit   *gatewaymiddleware.RateLimiter
	CurrentUser UserResolver
}

// Wrap applies rate limit and auth middleware in the canonical gateway order.
func (s Service) Wrap(path string, next http.HandlerFunc) http.HandlerFunc {
	if s.RateLimit != nil {
		next = s.RateLimit.Wrap(next)
	}
	if s.Auth != nil {
		return s.Auth.Wrap(path, next)
	}
	return next
}

// RequirePermission checks a single named permission before invoking the next handler.
func (s Service) RequirePermission(permission string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if permission == "" {
			next(w, r)
			return
		}
		if _, err := s.AuthorizeCommand(r.Context(), CommandRequest{
			Method:             permission,
			RequiredPermission: permission,
		}); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": permission})
			return
		}
		next(w, r)
	}
}

// RequirePermissionByMethod checks request-specific permissions before invoking the next handler.
func (s Service) RequirePermissionByMethod(methodPermissions map[string]string, defaultPermission string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		permission := defaultPermission
		if methodPermissions != nil {
			if mapped, ok := methodPermissions[r.Method]; ok {
				permission = mapped
			}
		}
		if permission == "" {
			next(w, r)
			return
		}
		if _, err := s.AuthorizeCommand(r.Context(), CommandRequest{
			Method:             permission,
			RequiredPermission: permission,
		}); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_permission": permission})
			return
		}
		next(w, r)
	}
}

// RequireHierarchyAccess checks org/project/workspace visibility before invoking the next handler.
func (s Service) RequireHierarchyAccess(resolve HierarchyResolver, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		org, project, workspace := "", "", ""
		if resolve != nil {
			org, project, workspace = resolve(r)
		}
		if org == "" && project == "" && workspace == "" {
			next(w, r)
			return
		}
		if !gatewayauth.HasHierarchyAccess(s.currentUser(r.Context()), org, project, workspace) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "required_org": org, "required_project": project, "required_workspace": workspace})
			return
		}
		next(w, r)
	}
}

func (s Service) currentUser(ctx context.Context) *gatewayauth.User {
	if s.CurrentUser == nil {
		return nil
	}
	return s.CurrentUser(ctx)
}

func (s Service) hasPermission(ctx context.Context, permission string) bool {
	return gatewayauth.HasPermission(s.currentUser(ctx), permission)
}

func writeJSON(w http.ResponseWriter, statusCode int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(value)
}
