package gateway

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/1024XEngineer/anyclaw/pkg/config"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

func newBearerRequest(method string, target string, token string, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

func newPermissionRouteServer(t *testing.T, users []config.SecurityUser) (*Server, *http.ServeMux) {
	t.Helper()
	server := New(newTestMainRuntime(t))
	if err := server.ensureDefaultWorkspace(); err != nil {
		t.Fatalf("ensure default workspace: %v", err)
	}
	server.mainRuntime.Config.Security.Users = users
	mux := http.NewServeMux()
	server.registerGatewayRoutes(mux)
	return server, mux
}

func TestGatewayRoutesUseMethodSpecificTaskPermissions(t *testing.T) {
	_, mux := newPermissionRouteServer(t, []config.SecurityUser{{
		Name:                "task-reader",
		Token:               "task-read-token",
		PermissionOverrides: []string{"tasks.read"},
	}})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, newBearerRequest(http.MethodGet, "/tasks", "task-read-token", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /tasks with tasks.read = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, newBearerRequest(http.MethodPost, "/tasks", "task-read-token", `{"input":"hello"}`))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("POST /tasks without tasks.write = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
}

func TestGatewayJobMutationRoutesRequireWritePermission(t *testing.T) {
	server, mux := newPermissionRouteServer(t, []config.SecurityUser{{
		Name:                "job-reader",
		Token:               "job-read-token",
		PermissionOverrides: []string{"jobs.read"},
	}})
	if err := server.store.AppendJob(&state.Job{ID: "queued-job", Kind: "noop", Status: "queued", Cancellable: true}); err != nil {
		t.Fatalf("append job: %v", err)
	}

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, newBearerRequest(http.MethodGet, "/jobs", "job-read-token", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /jobs with jobs.read = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, newBearerRequest(http.MethodPost, "/jobs/cancel", "job-read-token", `{"job_id":"queued-job"}`))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("POST /jobs/cancel without jobs.write = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
}

func TestGatewayConfigMutationRequiresWritePermission(t *testing.T) {
	server, mux := newPermissionRouteServer(t, []config.SecurityUser{{
		Name:                "config-reader",
		Token:               "config-read-token",
		PermissionOverrides: []string{"config.read"},
	}})
	server.mainRuntime.Config.LLM.Provider = "original-provider"
	server.mainRuntime.Config.LLM.Model = "original-model"

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, newBearerRequest(http.MethodGet, "/config", "config-read-token", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /config with config.read = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, newBearerRequest(http.MethodPost, "/config", "config-read-token", `{"llm":{"provider":"x","model":"y"}}`))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("POST /config without config.write = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
	if server.mainRuntime.Config.LLM.Provider != "original-provider" || server.mainRuntime.Config.LLM.Model != "original-model" {
		t.Fatalf("config mutation was applied without config.write")
	}
}

func TestGatewayMutablePlatformRoutesRequireWritePermission(t *testing.T) {
	_, mux := newPermissionRouteServer(t, []config.SecurityUser{{
		Name:  "platform-reader",
		Token: "platform-read-token",
		PermissionOverrides: []string{
			"market.read",
			"mcp.read",
			"nodes.read",
		},
	}})

	for _, tc := range []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPost, path: "/market/plugins/demo/install"},
		{method: http.MethodPost, path: "/mcp/servers/demo/connect"},
		{method: http.MethodDelete, path: "/nodes/demo"},
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, newBearerRequest(tc.method, tc.path, "platform-read-token", tc.body))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s %s without write permission = %d, want 403; body=%s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
}

func TestGatewayRoutesAllowLocalControlUICORSPreflight(t *testing.T) {
	_, mux := newPermissionRouteServer(t, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/providers", nil)
	req.Header.Set("Origin", "http://127.0.0.1:4173")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "content-type")
	req.Header.Set("Access-Control-Request-Private-Network", "true")
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("OPTIONS /providers = %d, want 204; body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://127.0.0.1:4173" {
		t.Fatalf("unexpected CORS allow origin %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Private-Network"); got != "true" {
		t.Fatalf("unexpected private network CORS header %q", got)
	}
}

func TestGatewayAuthUserMutationsRequireUserWritePermission(t *testing.T) {
	_, mux := newPermissionRouteServer(t, []config.SecurityUser{
		{
			Name:                "user-reader",
			Token:               "user-read-token",
			PermissionOverrides: []string{"auth.users.read"},
		},
		{
			Name:                "user-writer",
			Token:               "user-write-token",
			PermissionOverrides: []string{"auth.users.write"},
		},
		{
			Name:  "target",
			Token: "target-token",
		},
	})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, newBearerRequest(http.MethodGet, "/auth/users", "user-read-token", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /auth/users with auth.users.read = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, newBearerRequest(http.MethodPost, "/auth/users", "user-read-token", `{"name":"new-user","token":"new-token"}`))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("POST /auth/users without auth.users.write = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, newBearerRequest(http.MethodDelete, "/auth/users?name=target", "user-read-token", ""))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("DELETE /auth/users without auth.users.write = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, newBearerRequest(http.MethodPost, "/auth/users", "user-write-token", `{"name":"new-user","token":"new-token"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /auth/users with auth.users.write = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, newBearerRequest(http.MethodDelete, "/auth/users?name=target", "user-write-token", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE /auth/users with auth.users.write = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
}

func TestGatewayAuthRoleMutationsRequireRoleWritePermission(t *testing.T) {
	server, mux := newPermissionRouteServer(t, []config.SecurityUser{
		{
			Name:                "role-reader",
			Token:               "role-read-token",
			PermissionOverrides: []string{"auth.roles.read"},
		},
		{
			Name:                "role-writer",
			Token:               "role-write-token",
			PermissionOverrides: []string{"auth.roles.write"},
		},
	})
	server.mainRuntime.Config.Security.Roles = []config.SecurityRole{{
		Name:        "custom",
		Description: "Custom",
		Permissions: []string{"status.read"},
	}}

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, newBearerRequest(http.MethodGet, "/auth/roles", "role-read-token", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /auth/roles with auth.roles.read = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, newBearerRequest(http.MethodPost, "/auth/roles", "role-read-token", `{"name":"ops","permissions":["status.read"]}`))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("POST /auth/roles without auth.roles.write = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, newBearerRequest(http.MethodDelete, "/auth/roles?name=custom", "role-read-token", ""))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("DELETE /auth/roles without auth.roles.write = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, newBearerRequest(http.MethodPost, "/auth/roles", "role-write-token", `{"name":"ops","permissions":["status.read"]}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /auth/roles with auth.roles.write = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, newBearerRequest(http.MethodDelete, "/auth/roles?name=custom", "role-write-token", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE /auth/roles with auth.roles.write = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, newBearerRequest(http.MethodPost, "/auth/roles", "role-write-token", `{"name":"bad","permissions":["unknown.permission"]}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST /auth/roles with unknown permission = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
}

func TestGatewayCronRoutesInitializeLazilyAndRequireWritePermission(t *testing.T) {
	cronInitOnce = sync.Once{}
	cronScheduler = nil
	defer func() {
		if cronScheduler != nil {
			cronScheduler.Stop()
		}
		cronInitOnce = sync.Once{}
		cronScheduler = nil
	}()

	_, mux := newPermissionRouteServer(t, []config.SecurityUser{{
		Name:                "cron-reader",
		Token:               "cron-read-token",
		PermissionOverrides: []string{"cron.read"},
	}})
	if cronScheduler != nil {
		t.Fatal("expected cron scheduler to remain uninitialized until the first cron request")
	}

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, newBearerRequest(http.MethodGet, "/cron/?json=1", "cron-read-token", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /cron/?json=1 with cron.read = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if cronScheduler == nil {
		t.Fatal("expected cron scheduler to initialize lazily on first cron request")
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, newBearerRequest(http.MethodPost, "/cron/", "cron-read-token", `{"name":"hourly","schedule":"@hourly","command":"echo hi"}`))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("POST /cron/ without cron.write = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
}
