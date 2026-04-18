package gateway

import (
	"net/http"

	gatewayintake "github.com/1024XEngineer/anyclaw/pkg/gateway/intake"
	scheduleui "github.com/1024XEngineer/anyclaw/pkg/gateway/transport/scheduleui"
)

func (s *Server) registerGatewayPlatformRoutes(mux *http.ServeMux) {
	s.registerOpenAIRoutes(mux)
	s.registerExtensionRoutes(mux)
	s.registerNodeRoutes(mux)
	s.registerDiscoveryRoutes(mux)
	s.registerMCPRoutes(mux)
	s.registerMarketRoutes(mux)
	scheduleui.RegisterUIHandler(mux, cronScheduler, "/cron")
}

func (s *Server) registerOpenAIRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/v1/chat/completions", s.wrap("/v1/chat/completions", s.openAICompat.HandleChatCompletions))
	mux.HandleFunc("/v1/models", s.wrap("/v1/models", s.openAICompat.HandleModels))
	mux.HandleFunc("/v1/responses", s.wrap("/v1/responses", s.openAICompat.HandleResponses))
}

func (s *Server) registerExtensionRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/webhooks/", s.rateLimit.Wrap(gatewayintake.WebhookIngressAPI{
		Webhooks:               s.webhooks,
		MainRuntime:            s.mainRuntime,
		RuntimePool:            s.runtimePool,
		EnsureDefaultWorkspace: s.ensureDefaultWorkspace,
		DefaultResourceIDs:     defaultResourceIDs,
		AppendEvent:            s.appendEvent,
	}.Handle))
}

func (s *Server) registerNodeRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/nodes", s.wrap("/nodes", requirePermission("nodes.read", s.nodesAPI().HandleList)))
	mux.HandleFunc("/nodes/", s.wrap("/nodes/", s.nodesAPI().HandleByID))
	mux.HandleFunc("/nodes/invoke", s.wrap("/nodes/invoke", requirePermission("nodes.write", s.nodesAPI().HandleInvoke)))
}

func (s *Server) registerDiscoveryRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/discovery/instances", s.wrap("/discovery/instances", s.discoveryAPI().HandleInstances))
	mux.HandleFunc("/discovery/query", s.wrap("/discovery/query", s.discoveryAPI().HandleQuery))
}

func (s *Server) registerMCPRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/mcp/servers", s.wrap("/mcp/servers", requirePermission("mcp.read", s.handleMCPServers)))
	mux.HandleFunc("/mcp/tools", s.wrap("/mcp/tools", requirePermission("mcp.read", s.handleMCPTools)))
	mux.HandleFunc("/mcp/resources", s.wrap("/mcp/resources", requirePermission("mcp.read", s.handleMCPResources)))
	mux.HandleFunc("/mcp/prompts", s.wrap("/mcp/prompts", requirePermission("mcp.read", s.handleMCPPrompts)))
	mux.HandleFunc("/mcp/call", s.wrap("/mcp/call", requirePermission("mcp.write", s.handleMCPCall)))
	mux.HandleFunc("/mcp/servers/", s.wrap("/mcp/servers/", s.handleMCPServerAction))
}

func (s *Server) registerMarketRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/market/search", s.wrap("/market/search", requirePermission("market.read", s.handleMarketSearch)))
	mux.HandleFunc("/market/plugins", s.wrap("/market/plugins", requirePermission("market.read", s.handleMarketPlugins)))
	mux.HandleFunc("/market/plugins/", s.wrap("/market/plugins/", s.handleMarketPluginAction))
	mux.HandleFunc("/market/installed", s.wrap("/market/installed", requirePermission("market.read", s.handleMarketInstalled)))
	mux.HandleFunc("/market/categories", s.wrap("/market/categories", requirePermission("market.read", s.handleMarketCategories)))
}
