package gateway

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/1024XEngineer/anyclaw/pkg/config"
	"github.com/1024XEngineer/anyclaw/pkg/extensions/mcp"
	"github.com/1024XEngineer/anyclaw/pkg/extensions/plugin"
	gatewayauth "github.com/1024XEngineer/anyclaw/pkg/gateway/auth"
)

func newAdminRequest(method string, target string, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	user := &gatewayauth.User{Name: "tester", Permissions: []string{"*"}}
	return req.WithContext(gatewayauth.WithUser(req.Context(), user))
}

func decodeBodyMap(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, rec.Body.String())
	}
	return payload
}

func newSplitAPITestServer(t *testing.T) *Server {
	t.Helper()
	server := New(newTestMainRuntime(t))
	if err := server.ensureDefaultWorkspace(); err != nil {
		t.Fatalf("ensure default workspace: %v", err)
	}
	server.marketStore = plugin.NewStore(
		filepath.Join(t.TempDir(), "plugins"),
		filepath.Join(t.TempDir(), "market"),
		filepath.Join(t.TempDir(), "cache"),
		nil,
		nil,
		server.plugins,
	)
	server.mainRuntime.Config.Security.Users = []config.SecurityUser{{Name: "tester", Token: "token-1", Role: "admin"}}
	server.mainRuntime.Config.Security.Roles = []config.SecurityRole{{Name: "custom", Permissions: []string{"config.read"}}}
	server.mainRuntime.Config.Agent.Profiles = []config.AgentProfile{{Name: "helper", Enabled: config.BoolPtr(true)}}
	server.mainRuntime.Config.Providers = []config.ProviderProfile{{ID: "provider-1", Name: "Provider 1", Type: "openai", Provider: "openai", Enabled: config.BoolPtr(true)}}
	server.mainRuntime.Config.LLM.DefaultProviderRef = "provider-1"
	return server
}

func TestSplitAPIs_ConfigUsersRolesAndTasks(t *testing.T) {
	server := newSplitAPITestServer(t)

	t.Run("config api", func(t *testing.T) {
		rec := httptest.NewRecorder()
		server.handleConfigAPI(rec, newAdminRequest(http.MethodGet, "/config", ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /config = %d", rec.Code)
		}

		rec = httptest.NewRecorder()
		server.handleConfigAPI(rec, newAdminRequest(http.MethodPost, "/config", `{"llm":{"provider":"x","model":"y"}}`))
		if rec.Code != http.StatusOK {
			t.Fatalf("POST /config = %d body=%s", rec.Code, rec.Body.String())
		}
		if server.mainRuntime.Config.LLM.Provider != "x" || server.mainRuntime.Config.LLM.Model != "y" {
			t.Fatalf("llm patch not applied")
		}

		_, err := parseChannelRoutingRules([]any{
			map[string]any{"channel": "discord", "match": "support"},
			map[string]any{"channel": "discord", "match": "support"},
		})
		if err == nil {
			t.Fatal("expected duplicate routing rule error")
		}
	})

	t.Run("users api", func(t *testing.T) {
		rec := httptest.NewRecorder()
		server.handleUsers(rec, newAdminRequest(http.MethodGet, "/auth/users", ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /auth/users = %d", rec.Code)
		}

		rec = httptest.NewRecorder()
		server.handleUsers(rec, newAdminRequest(http.MethodPost, "/auth/users", `{"name":"new-user","token":"token-2"}`))
		if rec.Code != http.StatusOK {
			t.Fatalf("POST /auth/users = %d body=%s", rec.Code, rec.Body.String())
		}

		rec = httptest.NewRecorder()
		server.handleUsers(rec, newAdminRequest(http.MethodDelete, "/auth/users?name=new-user", ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("DELETE /auth/users = %d body=%s", rec.Code, rec.Body.String())
		}

		rec = httptest.NewRecorder()
		server.handleUsers(rec, newAdminRequest(http.MethodPost, "/auth/users", `{"name":"bad-user","token":"token-3","permission_overrides":["unknown"]}`))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("POST /auth/users invalid permission = %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("roles api", func(t *testing.T) {
		rec := httptest.NewRecorder()
		server.handleRoles(rec, newAdminRequest(http.MethodGet, "/auth/roles", ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /auth/roles = %d", rec.Code)
		}

		rec = httptest.NewRecorder()
		server.handleRoles(rec, newAdminRequest(http.MethodPost, "/auth/roles", `{"name":"writers","permissions":["config.write"]}`))
		if rec.Code != http.StatusOK {
			t.Fatalf("POST /auth/roles = %d body=%s", rec.Code, rec.Body.String())
		}

		rec = httptest.NewRecorder()
		server.handleRoleImpact(rec, newAdminRequest(http.MethodGet, "/auth/roles/impact", ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /auth/roles/impact = %d", rec.Code)
		}

		rec = httptest.NewRecorder()
		server.handleRoles(rec, newAdminRequest(http.MethodDelete, "/auth/roles?name=missing", ""))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("DELETE /auth/roles missing = %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("tasks api", func(t *testing.T) {
		rec := httptest.NewRecorder()
		server.handleV2Tasks(rec, newAdminRequest(http.MethodGet, "/v2/tasks", ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /v2/tasks = %d", rec.Code)
		}

		rec = httptest.NewRecorder()
		server.handleV2Tasks(rec, newAdminRequest(http.MethodPost, "/v2/tasks", `{`))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("POST /v2/tasks = %d body=%s", rec.Code, rec.Body.String())
		}

		rec = httptest.NewRecorder()
		server.handleV2TaskByID(rec, newAdminRequest(http.MethodGet, "/v2/tasks/missing", ""))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("GET missing /v2/tasks/id = %d", rec.Code)
		}

		rec = httptest.NewRecorder()
		server.handleV2Agents(rec, newAdminRequest(http.MethodGet, "/v2/agents", ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /v2/agents = %d", rec.Code)
		}

		server.executeTaskAsync("", "test")
	})
}

func TestSplitAPIs_MarketMCPProvidersAndBindings(t *testing.T) {
	server := newSplitAPITestServer(t)

	t.Run("market api", func(t *testing.T) {
		rec := httptest.NewRecorder()
		server.handleMarketSearch(rec, newAdminRequest(http.MethodGet, "/market/search?q=test", ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /market/search = %d body=%s", rec.Code, rec.Body.String())
		}
		payload := decodeBodyMap(t, rec)
		if payload["total"] == nil {
			t.Fatalf("expected total in market search response")
		}

		rec = httptest.NewRecorder()
		server.handleMarketPlugins(rec, newAdminRequest(http.MethodGet, "/market/plugins", ""))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("GET /market/plugins missing id = %d", rec.Code)
		}

		rec = httptest.NewRecorder()
		server.handleMarketPluginAction(rec, newAdminRequest(http.MethodGet, "/market/plugins/plugin-1/history", ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /market/plugins/history = %d body=%s", rec.Code, rec.Body.String())
		}

		for _, path := range []string{
			"/market/plugins/plugin-1/update",
			"/market/plugins/plugin-1/uninstall",
			"/market/plugins/plugin-1/rollback?version=v1",
			"/market/plugins/plugin-1/versions",
		} {
			method := http.MethodPost
			if strings.Contains(path, "versions") {
				method = http.MethodGet
			}
			rec = httptest.NewRecorder()
			server.handleMarketPluginAction(rec, newAdminRequest(method, path, ""))
			if rec.Code == 0 {
				t.Fatalf("unexpected market action status for %s", path)
			}
		}

		rec = httptest.NewRecorder()
		server.handleMarketInstalled(rec, newAdminRequest(http.MethodGet, "/market/installed", ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /market/installed = %d", rec.Code)
		}

		rec = httptest.NewRecorder()
		server.handleMarketCategories(rec, newAdminRequest(http.MethodGet, "/market/categories", ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /market/categories = %d", rec.Code)
		}
	})

	t.Run("mcp api", func(t *testing.T) {
		for _, handler := range []func(http.ResponseWriter, *http.Request){
			server.handleMCPServers,
			server.handleMCPTools,
			server.handleMCPResources,
			server.handleMCPPrompts,
		} {
			rec := httptest.NewRecorder()
			handler(rec, newAdminRequest(http.MethodGet, "/mcp", ""))
			if rec.Code != http.StatusOK {
				t.Fatalf("mcp GET = %d body=%s", rec.Code, rec.Body.String())
			}
		}

		rec := httptest.NewRecorder()
		server.handleMCPCall(rec, newAdminRequest(http.MethodPost, "/mcp/call", `{`))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("POST /mcp/call invalid json = %d", rec.Code)
		}

		server.mcpRegistry = mcp.NewRegistry()
		rec = httptest.NewRecorder()
		server.handleMCPCall(rec, newAdminRequest(http.MethodPost, "/mcp/call", `{}`))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("POST /mcp/call missing target = %d body=%s", rec.Code, rec.Body.String())
		}

		rec = httptest.NewRecorder()
		server.handleMCPServerAction(rec, newAdminRequest(http.MethodGet, "/mcp/servers/", ""))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("GET /mcp/servers/ missing name = %d", rec.Code)
		}

		rec = httptest.NewRecorder()
		server.handleMCPServerAction(rec, newAdminRequest(http.MethodGet, "/mcp/servers/missing", ""))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("GET /mcp/servers/missing = %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("providers api", func(t *testing.T) {
		rec := httptest.NewRecorder()
		server.handleProviders(rec, newAdminRequest(http.MethodGet, "/providers", ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /providers = %d", rec.Code)
		}

		rec = httptest.NewRecorder()
		server.handleProviderUpsert(rec, newAdminRequest(http.MethodPost, "/providers", `{`))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("POST /providers invalid body = %d body=%s", rec.Code, rec.Body.String())
		}

		rec = httptest.NewRecorder()
		server.handleProviderUpsert(rec, newAdminRequest(http.MethodPost, "/providers", `{"id":"provider-2","name":"Provider 2","type":"openai","provider":"openai"}`))
		if rec.Code != http.StatusOK {
			t.Fatalf("POST /providers = %d body=%s", rec.Code, rec.Body.String())
		}

		rec = httptest.NewRecorder()
		server.handleProviderDelete(rec, newAdminRequest(http.MethodDelete, "/providers?id=provider-2", ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("DELETE /providers = %d body=%s", rec.Code, rec.Body.String())
		}

		rec = httptest.NewRecorder()
		server.handleProviderDelete(rec, newAdminRequest(http.MethodDelete, "/providers?id=missing", ""))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("DELETE /providers missing = %d body=%s", rec.Code, rec.Body.String())
		}

		provider := config.ProviderProfile{ID: "provider-3"}
		mergeProviderSecretsAndExtra(&provider, config.ProviderProfile{APIKey: "secret", Extra: map[string]string{"k": "v"}})
		if provider.APIKey != "secret" || provider.Extra["k"] != "v" {
			t.Fatalf("expected provider secrets to merge")
		}
	})

	t.Run("provider helpers and agent bindings", func(t *testing.T) {
		rec := httptest.NewRecorder()
		server.handleDefaultProvider(rec, newAdminRequest(http.MethodPost, "/providers/default", `{"provider_ref":"provider-1"}`))
		if rec.Code != http.StatusOK {
			t.Fatalf("POST /providers/default = %d body=%s", rec.Code, rec.Body.String())
		}

		rec = httptest.NewRecorder()
		server.handleProviderTest(rec, newAdminRequest(http.MethodPost, "/providers/test", `{"name":"Provider 1"}`))
		if rec.Code != http.StatusOK {
			t.Fatalf("POST /providers/test = %d body=%s", rec.Code, rec.Body.String())
		}

		probe := config.ProviderProfile{Name: "Provider 1"}
		enrichProviderForHealthTest(&probe, server.mainRuntime.Config)
		if probe.ID == "" || probe.Provider == "" {
			t.Fatalf("expected provider health enrichment to fill fields: %+v", probe)
		}

		rec = httptest.NewRecorder()
		server.handleAgentBindings(rec, newAdminRequest(http.MethodGet, "/agent-bindings", ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /agent-bindings = %d", rec.Code)
		}

		model := "model-x"
		body, _ := json.Marshal(map[string]any{"agent": "helper", "provider_ref": "provider-1", "model": model})
		rec = httptest.NewRecorder()
		server.handleAgentBindings(rec, newAdminRequest(http.MethodPost, "/agent-bindings", string(body)))
		if rec.Code != http.StatusOK {
			t.Fatalf("POST /agent-bindings = %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("agents api", func(t *testing.T) {
		rec := httptest.NewRecorder()
		server.handleAgents(rec, newAdminRequest(http.MethodGet, "/agents", ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /agents = %d", rec.Code)
		}

		rec = httptest.NewRecorder()
		server.handleAgents(rec, newAdminRequest(http.MethodDelete, "/agents?name=missing", ""))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("DELETE /agents missing = %d body=%s", rec.Code, rec.Body.String())
		}

		payload := bytes.NewBufferString(`{"name":"helper-2","provider_ref":"provider-1"}`)
		rec = httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/agents", payload)
		req = req.WithContext(gatewayauth.WithUser(req.Context(), &gatewayauth.User{Name: "tester", Permissions: []string{"*"}}))
		server.handleAgents(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("POST /agents = %d body=%s", rec.Code, rec.Body.String())
		}

		rec = httptest.NewRecorder()
		server.handleAssistants(rec, newAdminRequest(http.MethodGet, "/assistants", ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /assistants = %d", rec.Code)
		}
	})
}
