package gateway

import (
	"context"

	gatewaycommands "github.com/1024XEngineer/anyclaw/pkg/gateway/commands"
	gatewaycontrolplane "github.com/1024XEngineer/anyclaw/pkg/gateway/controlplane"
	gatewayevents "github.com/1024XEngineer/anyclaw/pkg/gateway/events"
	gatewaygovernance "github.com/1024XEngineer/anyclaw/pkg/gateway/governance"
	gatewayingress "github.com/1024XEngineer/anyclaw/pkg/gateway/ingress"
	gatewayintake "github.com/1024XEngineer/anyclaw/pkg/gateway/intake"
	gatewaysurface "github.com/1024XEngineer/anyclaw/pkg/gateway/surface"
	gatewaytransport "github.com/1024XEngineer/anyclaw/pkg/gateway/transport"
	routeingress "github.com/1024XEngineer/anyclaw/pkg/route/ingress"
	runtimechannelbridge "github.com/1024XEngineer/anyclaw/pkg/runtime/channelbridge"
	runtimesessionbridge "github.com/1024XEngineer/anyclaw/pkg/runtime/sessionbridge"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

func (s *Server) controlPlaneService() gatewaycontrolplane.Service {
	return gatewaycontrolplane.NewService("openclaw.gateway.v1", openClawWSMethods)
}

func (s *Server) controlPlaneStatusAPI() gatewaytransport.PublicAPI {
	return gatewaytransport.PublicAPI{
		Status: s.statusDeps(),
		OnStatusRead: func(ctx context.Context) {
			s.appendAudit(UserFromContext(ctx), "status.read", "status", nil)
		},
	}
}

func (s *Server) controlPlaneRuntimeAPI() gatewaytransport.RuntimeGovernanceAPI {
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

func (s *Server) controlPlanePresenceAPI() gatewaycontrolplane.PresenceAPI {
	if s.presenceMgr == nil {
		return gatewaycontrolplane.PresenceAPI{}
	}
	return gatewaycontrolplane.PresenceAPI{
		Get: func(channel string, userID string) (any, bool) {
			info, ok := s.presenceMgr.GetPresence(channel, userID)
			if !ok {
				return nil, false
			}
			return info, true
		},
		List: func() any {
			return s.presenceMgr.ListActive()
		},
	}
}

func (s *Server) eventsService() gatewayevents.Service {
	return gatewayevents.NewService(s.store, s.bus)
}

func (s *Server) surfaceService() gatewaysurface.Service {
	return gatewaysurface.Service{}
}

func (s *Server) governanceService() gatewaygovernance.Service {
	return gatewaygovernance.Service{
		Auth:        s.auth,
		RateLimit:   s.rateLimit,
		CurrentUser: UserFromContext,
	}
}

func (s *Server) commandIntakeService() gatewaycommands.Service {
	return gatewaycommands.Service{}
}

func (s *Server) sessionCommandsAPI() gatewaytransport.SessionAPI {
	return s.sessionAPI()
}

func (s *Server) sessionMoveCommandsAPI() gatewaytransport.SessionMoveAPI {
	return s.sessionMoveAPI()
}

func (s *Server) taskCommandsAPI() gatewaytransport.TaskAPI {
	return s.taskAPI()
}

func (s *Server) ingressHandoffService() gatewayingress.Service {
	return gatewayingress.Service{
		Routes:    s.ensureIngressRoutingService(),
		CloneMeta: cloneBindingConfig,
	}
}

func (s *Server) ensureIngressRoutingService() *routeingress.Service {
	if s == nil {
		return nil
	}
	if s.ingress != nil {
		return s.ingress
	}
	s.ingress = routeingress.NewService(
		routeingress.NewRouter(s.mainRuntime.Config.Channels.Routing),
		routeingress.WithMainAgentNameResolver(s.mainRuntime.Config.ResolveMainAgentName),
		routeingress.WithSessionStore(ingressSessionStore{server: s, manager: s.sessions}),
	)
	return s.ingress
}

func (s *Server) channelBridgeService() runtimechannelbridge.Service {
	return runtimechannelbridge.Service{
		Sessions:             s.sessions,
		Runner:               s.ensureSessionRunner(),
		ResolveMainAgentName: s.mainRuntime.Config.ResolveMainAgentName,
		ResolveDefaultResources: func() (string, string, string) {
			return defaultResourceIDs(s.mainRuntime.WorkingDir)
		},
		ValidateResourceSelection: s.validateResourceSelection,
		AppendEvent:               s.appendEvent,
	}
}

func (s *Server) sessionBridgeService() runtimesessionbridge.Service {
	return runtimesessionbridge.Service{Runner: s.ensureSessionRunner()}
}

func (s *Server) signedIngressAPI() gatewayintake.SignedIngressAPI {
	return gatewayintake.SignedIngressAPI{
		Secret: s.mainRuntime.Config.Security.WebhookSecret,
		RunSessionMessage: func(ctx context.Context, sessionID string, title string, message string) (string, *state.Session, error) {
			raw := s.surfaceService().BuildSignedWebhookRawRequest(gatewaysurface.SignedWebhookInput{
				SessionID: sessionID,
				Title:     title,
				Message:   message,
				Meta: map[string]string{
					"title_hint": title,
				},
			})
			normalized, err := s.governanceService().Accept(ctx, raw)
			if err != nil {
				return "", nil, err
			}
			routed, err := s.ingressHandoffService().RouteNormalized(ctx, normalized)
			if err != nil {
				return "", nil, err
			}
			return s.channelBridgeService().RunRouted(ctx, "webhook", sessionID, message, routed, map[string]string{
				"title_hint": title,
			})
		},
		SessionApprovalResponse: s.sessionApprovalResponse,
		CurrentUser:             UserFromContext,
		AppendAudit:             s.appendAudit,
		AppendEvent:             s.appendEvent,
	}
}

func (s *Server) pluginIngressAPI() gatewayintake.PluginIngressAPI {
	return gatewayintake.PluginIngressAPI{
		IngressPlugins: s.ingressPlugins,
		CurrentUser:    UserFromContext,
		AppendAudit:    s.appendAudit,
		AppendEvent:    s.appendEvent,
	}
}
