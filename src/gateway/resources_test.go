package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anyclaw/anyclaw/pkg/config"
)

func TestResolveRolePermissionsUsesBuiltinTemplates(t *testing.T) {
	perms := resolveRolePermissions(&config.SecurityConfig{}, "operator")
	required := map[string]bool{
		"tasks.read":      false,
		"tasks.write":     false,
		"approvals.read":  false,
		"approvals.write": false,
		"resources.read":  false,
		"resources.write": false,
	}
	for _, perm := range perms {
		if _, ok := required[perm]; ok {
			required[perm] = true
		}
	}
	for perm, found := range required {
		if !found {
			t.Fatalf("operator permissions should include %s, got %v", perm, perms)
		}
	}
}

func TestResolveRolePermissionsPrefersCustomRoleOverrides(t *testing.T) {
	perms := resolveRolePermissions(&config.SecurityConfig{
		Roles: []config.SecurityRole{
			{Name: "operator", Permissions: []string{"custom.permission"}},
		},
	}, "operator")
	if len(perms) != 1 || perms[0] != "custom.permission" {
		t.Fatalf("expected custom role permissions to override builtin template, got %v", perms)
	}
}

func TestHandleResourcesWriteRequiresWritePermission(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	server := &Server{store: store}

	req := httptest.NewRequest(http.MethodPost, "/resources", strings.NewReader(`{"org":{"id":"org-1","name":"Org 1"}}`))
	req = req.WithContext(context.WithValue(req.Context(), authUserKey, &AuthUser{
		Name:        "viewer",
		Permissions: []string{"resources.read"},
	}))
	rec := httptest.NewRecorder()

	server.handleResources(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
	var payload map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["required_permission"] != "resources.write" {
		t.Fatalf("expected required_permission resources.write, got %q", payload["required_permission"])
	}
}

func TestHandleResourcesWriteAllowsOrgCreation(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	server := &Server{store: store}

	req := httptest.NewRequest(http.MethodPost, "/resources", strings.NewReader(`{"org":{"id":"org-1","name":"Org 1"}}`))
	req = req.WithContext(context.WithValue(req.Context(), authUserKey, &AuthUser{
		Name:        "operator",
		Permissions: []string{"resources.write"},
	}))
	rec := httptest.NewRecorder()

	server.handleResources(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	org, ok := store.GetOrg("org-1")
	if !ok {
		t.Fatalf("expected org to be created")
	}
	if org.Name != "Org 1" {
		t.Fatalf("expected org name Org 1, got %q", org.Name)
	}
}
