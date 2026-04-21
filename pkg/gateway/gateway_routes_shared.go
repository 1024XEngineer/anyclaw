package gateway

import (
	"net/http"

	gatewayintake "github.com/1024XEngineer/anyclaw/pkg/gateway/intake"
	controlui "github.com/1024XEngineer/anyclaw/pkg/gateway/transport/controlui"
)

func (s *Server) registerSharedRoutes(mux *http.ServeMux) {
	s.registerStatusRoutes(mux)
	s.registerCatalogRoutes(mux)
	s.registerRuntimeGovernanceRoutes(mux)
	s.registerSessionTaskRoutes(mux)
	s.registerChannelRoutes(mux)

	controlui.RegisterRoutes(mux, controlui.Options{
		BasePath: s.mainRuntime.Config.Gateway.ControlUI.BasePath,
		Root:     s.mainRuntime.Config.Gateway.ControlUI.Root,
	})
	mux.HandleFunc("/", s.handleRootAPI)
}

func (s *Server) registerStatusRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/healthz", s.wrap("/healthz", s.handleHealth))
	mux.HandleFunc("/status", s.wrap("/status", requirePermission("status.read", s.handleStatus)))
	mux.HandleFunc("/events", s.wrap("/events", requirePermission("events.read", s.handleEvents)))
	mux.HandleFunc("/events/stream", s.wrap("/events/stream", requirePermission("events.read", s.handleEventStream)))
	mux.HandleFunc("/ws", s.wrap("/ws", s.handleOpenClawWS))
	mux.HandleFunc("/control-plane", s.wrap("/control-plane", requirePermission("status.read", s.controlPlaneRuntimeAPI().HandleControlPlane)))
}

func (s *Server) registerCatalogRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/chat", s.wrap("/chat", requirePermission("chat.send", requireHierarchyAccess(func(r *http.Request) (string, string, string) {
		return s.resolveHierarchyFromQuery(r)
	}, s.handleChat))))
	mux.HandleFunc("/channels", s.wrap("/channels", requirePermission("channels.read", s.handleChannels)))
	mux.HandleFunc("/plugins", s.wrap("/plugins", requirePermission("plugins.read", s.handlePlugins)))
	mux.HandleFunc("/routing", s.wrap("/routing", requirePermission("routing.read", s.handleRouting)))
	mux.HandleFunc("/routing/analysis", s.wrap("/routing/analysis", requirePermission("routing.read", s.handleRoutingAnalysis)))
	mux.HandleFunc("/agents", s.wrap("/agents", s.handleAgents))
	mux.HandleFunc("/agents/personality-templates", s.wrap("/agents/personality-templates", requirePermission("config.read", s.handlePersonalityTemplates)))
	mux.HandleFunc("/agents/skill-catalog", s.wrap("/agents/skill-catalog", requirePermission("skills.read", s.handleAssistantSkillCatalog)))
	mux.HandleFunc("/assistants", s.wrap("/assistants", s.handleAssistants))
	mux.HandleFunc("/assistants/personality-templates", s.wrap("/assistants/personality-templates", requirePermission("config.read", s.handlePersonalityTemplates)))
	mux.HandleFunc("/assistants/skill-catalog", s.wrap("/assistants/skill-catalog", requirePermission("skills.read", s.handleAssistantSkillCatalog)))
	mux.HandleFunc("/providers", s.wrap("/providers", s.handleProviders))
	mux.HandleFunc("/providers/test", s.wrap("/providers/test", s.handleProviderTest))
	mux.HandleFunc("/providers/default", s.wrap("/providers/default", s.handleDefaultProvider))
	mux.HandleFunc("/agent-bindings", s.wrap("/agent-bindings", s.handleAgentBindings))
	mux.HandleFunc("/resources", s.wrap("/resources", s.resourcesAPI().HandleCollection))
	mux.HandleFunc("/skills", s.wrap("/skills", s.handleSkills))
	mux.HandleFunc("/tools/activity", s.wrap("/tools/activity", requirePermission("tools.read", s.handleToolActivity)))
	mux.HandleFunc("/tools", s.wrap("/tools", requirePermission("tools.read", s.handleTools)))
}

func (s *Server) registerRuntimeGovernanceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/runtimes", s.wrap("/runtimes", requirePermission("runtimes.read", requireHierarchyAccess(s.resolveHierarchyFromQuery, s.controlPlaneRuntimeAPI().HandleList))))
	mux.HandleFunc("/runtimes/refresh", s.wrap("/runtimes/refresh", requirePermission("runtimes.write", s.controlPlaneRuntimeAPI().HandleRefresh)))
	mux.HandleFunc("/runtimes/refresh-batch", s.wrap("/runtimes/refresh-batch", requirePermission("runtimes.write", s.controlPlaneRuntimeAPI().HandleRefreshBatch)))
	mux.HandleFunc("/runtimes/metrics", s.wrap("/runtimes/metrics", requirePermission("runtimes.read", s.controlPlaneRuntimeAPI().HandleMetrics)))
	mux.HandleFunc("/auth/users", s.wrap("/auth/users", s.handleUsers))
	mux.HandleFunc("/auth/roles", s.wrap("/auth/roles", s.handleRoles))
	mux.HandleFunc("/auth/roles/impact", s.wrap("/auth/roles/impact", requirePermission("auth.users.read", s.handleRoleImpact)))
	mux.HandleFunc("/audit", s.wrap("/audit", requirePermission("audit.read", s.handleAudit)))
	mux.HandleFunc("/jobs", s.wrap("/jobs", requirePermission("audit.read", s.handleJobs)))
	mux.HandleFunc("/jobs/", s.wrap("/jobs/", requirePermission("audit.read", s.handleJobByID)))
	mux.HandleFunc("/jobs/retry", s.wrap("/jobs/retry", requirePermission("audit.read", s.handleRetryJob)))
	mux.HandleFunc("/jobs/cancel", s.wrap("/jobs/cancel", requirePermission("audit.read", s.handleCancelJob)))
	mux.HandleFunc("/config", s.wrap("/config", s.handleConfigAPI))
	mux.HandleFunc("/memory", s.wrap("/memory", requirePermission("memory.read", requireHierarchyAccess(s.resolveHierarchyFromQuery, s.handleMemory))))
	mux.HandleFunc("/approvals", s.wrap("/approvals", requirePermission("approvals.read", s.handleApprovals)))
	mux.HandleFunc("/approvals/", s.wrap("/approvals/", requirePermission("approvals.write", s.handleApprovalByID)))
}

func (s *Server) registerSessionTaskRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/sessions", s.wrap("/sessions", requirePermissionByMethod(map[string]string{
		http.MethodGet:  "sessions.read",
		http.MethodPost: "sessions.write",
	}, "sessions.read", requireHierarchyAccess(s.resolveHierarchyFromQuery, s.sessionCommandsAPI().HandleCollection))))
	mux.HandleFunc("/sessions/", s.wrap("/sessions/", requirePermissionByMethod(map[string]string{
		http.MethodDelete: "sessions.write",
		http.MethodGet:    "sessions.read",
	}, "sessions.read", requireHierarchyAccess(s.resolveHierarchyFromSessionPath, s.sessionCommandsAPI().HandleByID))))
	mux.HandleFunc("/sessions/move", s.wrap("/sessions/move", requirePermission("sessions.write", s.sessionMoveCommandsAPI().HandleSingle)))
	mux.HandleFunc("/sessions/move-batch", s.wrap("/sessions/move-batch", requirePermission("sessions.write", s.sessionMoveCommandsAPI().HandleBatch)))
	mux.HandleFunc("/tasks", s.wrap("/tasks", requirePermission("tasks.write", requireHierarchyAccess(s.resolveHierarchyFromQuery, s.taskCommandsAPI().HandleCollection))))
	mux.HandleFunc("/tasks/", s.wrap("/tasks/", s.taskCommandsAPI().HandleByID))
	mux.HandleFunc("/v2/tasks", s.wrap("/v2/tasks", requirePermission("tasks.write", s.handleV2Tasks)))
	mux.HandleFunc("/v2/tasks/", s.wrap("/v2/tasks/", requirePermission("tasks.read", s.handleV2TaskByID)))
	mux.HandleFunc("/v2/agents", s.wrap("/v2/agents", requirePermission("tasks.read", s.handleV2Agents)))
	mux.HandleFunc("/v2/chat", s.wrap("/v2/chat", requirePermission("tasks.write", s.handleV2Chat)))
	mux.HandleFunc("/v2/chat/sessions", s.wrap("/v2/chat/sessions", requirePermission("tasks.read", s.handleV2ChatSessions)))
	mux.HandleFunc("/v2/chat/sessions/", s.wrap("/v2/chat/sessions/", requirePermission("tasks.read", s.handleV2ChatSessionByID)))
	mux.HandleFunc("/v2/store", s.wrap("/v2/store", requirePermission("tasks.read", s.handleV2Store)))
	mux.HandleFunc("/v2/store/", s.wrap("/v2/store/", requirePermission("tasks.read", s.handleV2StoreByID)))
}

func (s *Server) registerChannelRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/channel/mention-gate", s.wrap("/channel/mention-gate", gatewayintake.MentionGateAPI{Gate: s.mentionGate}.Handle))
	mux.HandleFunc("/channel/group-security", s.wrap("/channel/group-security", gatewayintake.GroupSecurityAPI{Security: s.groupSecurity}.Handle))
	mux.HandleFunc("/channel/pairing", s.wrap("/channel/pairing", gatewayintake.ChannelPairingAPI{Pairing: s.channelPairing}.Handle))
	mux.HandleFunc("/channel/presence", s.wrap("/channel/presence", s.controlPlanePresenceAPI().Handle))
	mux.HandleFunc("/channel/contacts", s.wrap("/channel/contacts", gatewayintake.ContactsAPI{Directory: s.contactDir}.Handle))
	mux.HandleFunc("/device/pairing", s.wrap("/device/pairing", s.handleDevicePairing))
	mux.HandleFunc("/device/pairing/code", s.wrap("/device/pairing/code", s.handleDevicePairingCode))
	mux.HandleFunc("/channels/whatsapp/webhook", s.rateLimit.Wrap(gatewayintake.WhatsAppWebhookAPI{
		Adapter:       s.whatsapp,
		HandleMessage: s.processChannelMessage,
		VerifyToken:   s.mainRuntime.Config.Channels.WhatsApp.VerifyToken,
		AppSecret:     s.mainRuntime.Config.Channels.WhatsApp.AppSecret,
	}.Handle))
	mux.HandleFunc("/channels/discord/interactions", s.rateLimit.Wrap(gatewayintake.DiscordInteractionAPI{
		Adapter:       s.discord,
		HandleMessage: s.processChannelMessage,
	}.Handle))
	mux.HandleFunc("/ingress/web", s.rateLimit.Wrap(s.signedIngressAPI().Handle))
	mux.HandleFunc("/ingress/plugins/", s.rateLimit.Wrap(s.pluginIngressAPI().Handle))
}
