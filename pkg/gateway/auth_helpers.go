package gateway

import (
	"context"

	"github.com/1024XEngineer/anyclaw/pkg/config"
	gatewayauth "github.com/1024XEngineer/anyclaw/pkg/gateway/auth"
)

type AuthUser = gatewayauth.User
type authMiddleware = gatewayauth.Middleware

var authUserKey = gatewayauth.UserContextKey

func newAuthMiddleware(cfg *config.SecurityConfig) *authMiddleware {
	return gatewayauth.NewMiddleware(cfg)
}

func UserFromContext(ctx context.Context) *AuthUser {
	return gatewayauth.UserFromContext(ctx)
}

func HasPermission(user *AuthUser, permission string) bool {
	return gatewayauth.HasPermission(user, permission)
}

func HasHierarchyAccess(user *AuthUser, org string, project string, workspace string) bool {
	return gatewayauth.HasHierarchyAccess(user, org, project, workspace)
}

func resolveRolePermissions(cfg *config.SecurityConfig, roleName string) []string {
	return gatewayauth.ResolveRolePermissions(cfg, roleName)
}
