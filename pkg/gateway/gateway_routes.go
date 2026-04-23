package gateway

import (
	"context"
	"net/http"
	"time"

	gatewaytransport "github.com/1024XEngineer/anyclaw/pkg/gateway/transport"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

type Status = gatewaytransport.Status
type GatewayStatus = gatewaytransport.GatewayStatus
type HealthStatus = gatewaytransport.HealthStatus
type PresenceStatus = gatewaytransport.PresenceStatus
type TypingStatus = gatewaytransport.TypingStatus
type ApprovalStatus = gatewaytransport.ApprovalStatus
type SessionStatus = gatewaytransport.SessionStatus
type ChannelStatus = gatewaytransport.ChannelStatus
type AdapterStatus = gatewaytransport.AdapterStatus
type SecurityStatus = gatewaytransport.SecurityStatus
type RuntimeStatus = gatewaytransport.RuntimeStatus

const typingSessionStaleAfter = gatewaytransport.TypingSessionStaleAfter

func Probe(ctx context.Context, baseURL string) (*Status, error) {
	return gatewaytransport.Probe(ctx, baseURL)
}

func typingSessionActive(session *state.Session, now time.Time, maxAge time.Duration) bool {
	return gatewaytransport.TypingSessionActive(session, now, maxAge)
}

func (s *Server) statusDeps() gatewaytransport.StatusDeps {
	deps := gatewaytransport.StatusDeps{
		MainRuntime: s.mainRuntime,
		StartedAt:   s.startedAt,
		Store:       s.store,
	}
	if s.channels != nil {
		deps.Channels = s.channels
	}
	if s.runtimePool != nil {
		deps.RuntimePool = s.runtimePool
	}
	return deps
}

func (s *Server) status() Status {
	return gatewaytransport.StatusSnapshot(s.statusDeps())
}

func (s *Server) GatewayStatus() GatewayStatus {
	return gatewaytransport.GatewaySnapshot(s.statusDeps())
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.controlPlaneStatusAPI().HandleHealth(w, r)
}

func (s *Server) handleRootAPI(w http.ResponseWriter, r *http.Request) {
	s.controlPlaneStatusAPI().HandleRoot(w, r)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.controlPlaneStatusAPI().HandleStatus(w, r)
}

func (s *Server) registerGatewayRoutes(mux *http.ServeMux) {
	if mux == nil {
		return
	}

	mux.HandleFunc("/healthz", s.wrap("/healthz", s.handleHealth))
	mux.HandleFunc("/status", s.wrap("/status", requirePermission("status.read", s.handleStatus)))
	mux.HandleFunc("/events", s.wrap("/events", requirePermission("events.read", s.handleEvents)))
	mux.HandleFunc("/events/stream", s.wrap("/events/stream", requirePermission("events.read", s.handleEventStream)))
	mux.HandleFunc("/control-plane", s.wrap("/control-plane", requirePermission("status.read", s.controlPlaneRuntimeAPI().HandleControlPlane)))

	mux.HandleFunc("/resources", s.wrap("/resources", s.resourcesAPI().HandleCollection))

	mux.HandleFunc("/runtimes", s.wrap("/runtimes", requirePermission("runtimes.read", requireHierarchyAccess(s.resolveHierarchyFromQuery, s.controlPlaneRuntimeAPI().HandleList))))
	mux.HandleFunc("/runtimes/refresh", s.wrap("/runtimes/refresh", requirePermission("runtimes.write", s.controlPlaneRuntimeAPI().HandleRefresh)))
	mux.HandleFunc("/runtimes/refresh-batch", s.wrap("/runtimes/refresh-batch", requirePermission("runtimes.write", s.controlPlaneRuntimeAPI().HandleRefreshBatch)))
	mux.HandleFunc("/runtimes/metrics", s.wrap("/runtimes/metrics", requirePermission("runtimes.read", s.controlPlaneRuntimeAPI().HandleMetrics)))

	mux.HandleFunc("/approvals", s.wrap("/approvals", requirePermission("approvals.read", s.handleApprovals)))
	mux.HandleFunc("/approvals/", s.wrap("/approvals/", requirePermission("approvals.write", s.handleApprovalByID)))

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

	mux.HandleFunc("/tasks", s.wrap("/tasks", requirePermissionByMethod(map[string]string{
		http.MethodGet:  "tasks.read",
		http.MethodPost: "tasks.write",
	}, "tasks.read", requireHierarchyAccess(s.resolveHierarchyFromQuery, s.taskCommandsAPI().HandleCollection))))
	mux.HandleFunc("/tasks/", s.wrap("/tasks/", s.taskCommandsAPI().HandleByID))

	mux.HandleFunc("/nodes", s.wrap("/nodes", requirePermission("nodes.read", s.nodesAPI().HandleList)))
	mux.HandleFunc("/nodes/", s.wrap("/nodes/", s.nodesAPI().HandleByID))
	mux.HandleFunc("/nodes/invoke", s.wrap("/nodes/invoke", requirePermission("nodes.write", s.nodesAPI().HandleInvoke)))

	mux.HandleFunc("/discovery/instances", s.wrap("/discovery/instances", s.discoveryAPI().HandleInstances))
	mux.HandleFunc("/discovery/query", s.wrap("/discovery/query", s.discoveryAPI().HandleQuery))

	if s.openAICompat != nil {
		mux.HandleFunc("/v1/chat/completions", s.wrap("/v1/chat/completions", s.openAICompat.HandleChatCompletions))
		mux.HandleFunc("/v1/models", s.wrap("/v1/models", s.openAICompat.HandleModels))
		mux.HandleFunc("/v1/responses", s.wrap("/v1/responses", s.openAICompat.HandleResponses))
	}

	mux.HandleFunc("/", s.handleRootAPI)
}
