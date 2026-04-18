package gateway

import (
	"context"
	"net/http"
	"time"

	gatewaytransport "github.com/1024XEngineer/anyclaw/pkg/gateway/transport"
	"github.com/1024XEngineer/anyclaw/pkg/runtime"
	"github.com/1024XEngineer/anyclaw/pkg/state"
	"github.com/1024XEngineer/anyclaw/pkg/state/observability"
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

func (s *Server) transportAPI() gatewaytransport.PublicAPI {
	return gatewaytransport.PublicAPI{
		Status: s.statusDeps(),
		OnStatusRead: func(ctx context.Context) {
			s.appendAudit(UserFromContext(ctx), "status.read", "status", nil)
		},
	}
}

func (s *Server) runtimeGovernanceAPI() gatewaytransport.RuntimeGovernanceAPI {
	return gatewaytransport.RuntimeGovernanceAPI{
		Status:      s.statusDeps(),
		RuntimePool: s.runtimePool,
		Store:       s.store,
		AppendAudit: func(ctx context.Context, action string, target string, meta map[string]any) {
			s.appendAudit(UserFromContext(ctx), action, target, meta)
		},
		EnqueueJob: func(job func()) {
			s.jobQueue <- job
		},
		ShouldCancel:   s.shouldCancelJob,
		JobMaxAttempts: s.jobMaxAttempts,
	}
}

func (s *Server) statusDeps() gatewaytransport.StatusDeps {
	deps := gatewaytransport.StatusDeps{
		MainRuntime:       s.mainRuntime,
		StartedAt:         s.startedAt,
		Store:             s.store,
		EnabledSkillCount: s.currentEnabledSkillCount,
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
	s.transportAPI().HandleHealth(w, r)
}

func (s *Server) handleRootAPI(w http.ResponseWriter, r *http.Request) {
	s.transportAPI().HandleRoot(w, r)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.transportAPI().HandleStatus(w, r)
}

func (s *Server) registerGatewayRoutes(mux *http.ServeMux) {
	obs := observability.NewGatewayHTTP(runtime.Version)
	obs.RegisterHealthChecks(s.mainRuntime)

	mux.Handle("/health", obs.HealthHandler())
	mux.Handle("/ready", obs.ReadyHandler())
	mux.Handle("/live", obs.LiveHandler())
	mux.Handle("/metrics", obs.MetricsHandler())
	mux.Handle("/metrics.json", obs.MetricsJSONHandler())
	observability.RegisterPprof(mux, "/debug/pprof/")

	s.registerSharedRoutes(mux)
	s.registerGatewayPlatformRoutes(mux)
}

func (s *Server) registerWorkerRoutes(mux *http.ServeMux) {
	s.registerSharedRoutes(mux)
}
