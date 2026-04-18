package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anyclaw/anyclaw/pkg/config"
	routeingress "github.com/anyclaw/anyclaw/pkg/route/ingress"
)

func TestConfigRouteAllowsReadPermission(t *testing.T) {
	server, _ := newAgentManagementTestServer(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/config", server.wrap("/config", server.handleConfigAPI))

	req := httptest.NewRequest(http.MethodGet, "/config", nil)
	req = req.WithContext(context.WithValue(req.Context(), authUserKey, &AuthUser{
		Name:        "reader",
		Permissions: []string{"config.read"},
	}))
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestConfigRouteLegacyResourceFieldsNoLongerRequireValidation(t *testing.T) {
	server, _ := newAgentManagementTestServer(t)

	body := `{
		"channels": {
			"routing": {
				"mode": "per-chat",
				"rules": [
					{
						"channel": "telegram",
						"match": "deploy",
						"session_mode": "shared",
						"agent": "legacy-agent",
						"org": "missing-org",
						"project": "missing-project",
						"workspace_ref": "missing-workspace"
					}
				]
			}
		}
	}`

	req := httptest.NewRequest(http.MethodPost, "/config", strings.NewReader(body))
	rec := httptest.NewRecorder()

	server.handleConfigAPI(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestConfigRouteDuplicateDetectionIgnoresLegacyTargets(t *testing.T) {
	server, _ := newAgentManagementTestServer(t)

	body := `{
		"channels": {
			"routing": {
				"mode": "per-chat",
				"rules": [
					{
						"channel": "slack",
						"match": "deploy",
						"session_mode": "shared",
						"agent": "legacy-a",
						"workspace_ref": "legacy-a"
					},
					{
						"channel": "slack",
						"match": "deploy",
						"session_mode": "shared",
						"agent": "legacy-b",
						"workspace_ref": "legacy-b"
					}
				]
			}
		}
	}`

	req := httptest.NewRequest(http.MethodPost, "/config", strings.NewReader(body))
	rec := httptest.NewRecorder()

	server.handleConfigAPI(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 duplicate rejection, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestEnsureChannelSessionUsesMainAgentAndDefaultWorkspace(t *testing.T) {
	server, workspaceID := newAgentManagementTestServer(t)
	server.mainRuntime.Config.Channels.Routing = config.RoutingConfig{
		Mode: "per-chat",
		Rules: []config.ChannelRoutingRule{
			{
				Channel:      "telegram",
				Match:        "deploy",
				SessionMode:  "shared",
				Agent:        "legacy-specialist",
				Org:          "legacy-org",
				Project:      "legacy-project",
				WorkspaceRef: "legacy-workspace",
			},
		},
	}
	server.ingress = routeingress.NewService(
		routeingress.NewRouter(server.mainRuntime.Config.Channels.Routing),
		routeingress.WithMainAgentNameResolver(server.mainRuntime.Config.ResolveMainAgentName),
		routeingress.WithSessionStore(ingressSessionStore{server: server, manager: server.sessions}),
	)

	routed, err := server.resolveChannelRoute("telegram", "", "please deploy", map[string]string{
		"reply_target": "chat-123",
	})
	if err != nil {
		t.Fatalf("resolveChannelRoute: %v", err)
	}
	if routed.Request.Route.Session.SessionID == "" || !routed.Request.Route.Session.Created {
		t.Fatalf("expected route layer to create session before gateway ensure, got %#v", routed.Request.Route.Session)
	}

	sessionID, err := server.ensureChannelSession("telegram", "", routed, map[string]string{
		"reply_target": "chat-123",
	}, false)
	if err != nil {
		t.Fatalf("ensureChannelSession: %v", err)
	}

	session, ok := server.sessions.Get(sessionID)
	if !ok {
		t.Fatalf("expected session %q to exist", sessionID)
	}
	if session.Agent != server.mainRuntime.Config.ResolveMainAgentName() {
		t.Fatalf("expected main agent %q, got %q", server.mainRuntime.Config.ResolveMainAgentName(), session.Agent)
	}
	if session.Workspace != workspaceID {
		t.Fatalf("expected default workspace %q, got %q", workspaceID, session.Workspace)
	}
	if session.ReplyTarget != "chat-123" {
		t.Fatalf("expected reply target chat-123, got %q", session.ReplyTarget)
	}
	if session.TransportMeta["chat_id"] != "chat-123" {
		t.Fatalf("expected transport meta chat_id chat-123, got %#v", session.TransportMeta)
	}
}

func TestEnsureChannelSessionReusesConversationKeyWithoutAdapterCache(t *testing.T) {
	server, _ := newAgentManagementTestServer(t)
	server.ingress = routeingress.NewService(
		routeingress.NewRouter(config.RoutingConfig{Mode: "per-chat"}),
		routeingress.WithMainAgentNameResolver(server.mainRuntime.Config.ResolveMainAgentName),
		routeingress.WithSessionStore(ingressSessionStore{server: server, manager: server.sessions}),
	)

	firstRoute, err := server.resolveChannelRoute("telegram", "", "hello", map[string]string{
		"reply_target": "chat-123",
	})
	if err != nil {
		t.Fatalf("first resolveChannelRoute: %v", err)
	}
	if firstRoute.Request.Route.Session.SessionID == "" || !firstRoute.Request.Route.Session.Created {
		t.Fatalf("expected first route to create session in route layer, got %#v", firstRoute.Request.Route.Session)
	}

	firstSessionID, err := server.ensureChannelSession("telegram", "", firstRoute, map[string]string{
		"reply_target": "chat-123",
	}, false)
	if err != nil {
		t.Fatalf("first ensureChannelSession: %v", err)
	}

	secondRoute, err := server.resolveChannelRoute("telegram", "", "hello again", map[string]string{
		"reply_target": "chat-123",
	})
	if err != nil {
		t.Fatalf("second resolveChannelRoute: %v", err)
	}
	if secondRoute.Request.Route.Session.Created {
		t.Fatalf("expected second route to reuse existing session, got %#v", secondRoute.Request.Route.Session)
	}

	secondSessionID, err := server.ensureChannelSession("telegram", "", secondRoute, map[string]string{
		"reply_target": "chat-123",
	}, false)
	if err != nil {
		t.Fatalf("second ensureChannelSession: %v", err)
	}

	if secondSessionID != firstSessionID {
		t.Fatalf("expected conversation key to reuse session %q, got %q", firstSessionID, secondSessionID)
	}
}
