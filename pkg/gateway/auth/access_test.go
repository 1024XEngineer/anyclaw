package auth

import (
	"slices"
	"testing"

	"github.com/1024XEngineer/anyclaw/pkg/config"
)

func TestBuiltinOperatorCanUseMutablePlatformRoutes(t *testing.T) {
	permissions := ResolveRolePermissions(&config.SecurityConfig{}, "operator")
	for _, permission := range []string{"market.write", "mcp.write", "nodes.write"} {
		if !slices.Contains(permissions, permission) {
			t.Fatalf("builtin operator missing %q in %v", permission, permissions)
		}
	}
}

func TestBuiltinViewerDoesNotGetPlatformWritePermissions(t *testing.T) {
	permissions := ResolveRolePermissions(&config.SecurityConfig{}, "viewer")
	for _, permission := range []string{"market.write", "mcp.write", "nodes.write"} {
		if slices.Contains(permissions, permission) {
			t.Fatalf("builtin viewer unexpectedly has %q in %v", permission, permissions)
		}
	}
}
