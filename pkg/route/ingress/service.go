package ingress

import (
	"context"
	"fmt"
	"strings"
)

// ChannelRequest is the legacy channel-facing route input kept for gateway compatibility.
type ChannelRequest struct {
	Channel     string
	SessionID   string
	ReplyTarget string
	Message     string
	ThreadID    string
	IsGroup     bool
	GroupID     string
}

// Option customizes how the ingress route service is wired.
type Option func(*Service)

// WithMainAgentNameResolver wires the M2 resolver to the configured main agent.
func WithMainAgentNameResolver(resolve func() string) Option {
	return func(s *Service) {
		if s == nil {
			return
		}
		s.agents.ResolveMainAgentName = resolve
	}
}

// WithSessionStore wires the M3 resolver to the current session store.
func WithSessionStore(store SessionStore) Option {
	return func(s *Service) {
		if s == nil {
			return
		}
		s.sessions.Sessions = store
	}
}

// Service wires the ingress route modules together.
type Service struct {
	router    *Router
	projector IngressRouteProjector
	agents    AgentResolver
	sessions  SessionResolver
	delivery  DeliveryResolver
}

// NewService constructs one ingress route service instance.
func NewService(router *Router, options ...Option) *Service {
	service := &Service{
		router:    router,
		projector: IngressRouteProjector{},
		agents: AgentResolver{
			Router: router,
		},
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

// DecideChannel keeps the existing session-only routing behavior for gateway callers.
func (s *Service) DecideChannel(req ChannelRequest) SessionRoute {
	if s == nil || s.router == nil {
		return SessionRoute{}
	}

	request, err := s.projector.Project(channelIngressEntryFromRequest(req))
	if err != nil {
		return SessionRoute{}
	}
	return s.router.Decide(routeRequestFromMainRequest(request)).LegacySessionRoute()
}

// Route executes the current ingress route chain: M1 projector, M2 agent resolution, M3 session resolution, and M4 delivery resolution.
func (s *Service) Route(ctx context.Context, input RouteInput) (RouteOutput, error) {
	_ = ctx

	if s == nil {
		return RouteOutput{}, fmt.Errorf("ingress route service is nil")
	}

	request, err := s.projector.Project(input.Entry)
	if err != nil {
		return RouteOutput{}, err
	}
	agentResolution, sessionDecision, err := s.agents.Resolve(request)
	if err != nil {
		return RouteOutput{}, err
	}
	sessionResolution, sessionSnapshot, resolvedAgent, err := s.sessions.Resolve(request, sessionDecision, agentResolution)
	if err != nil {
		return RouteOutput{}, err
	}
	deliveryResolution := s.delivery.Resolve(request, sessionSnapshot)

	return RouteOutput{
		Request: RoutedRequest{
			Request: request,
			Route: RouteResolution{
				Agent:    resolvedAgent,
				Session:  sessionResolution,
				Delivery: deliveryResolution,
			},
		},
	}, nil
}

func channelIngressEntryFromRequest(req ChannelRequest) IngressRoutingEntry {
	routeSource := strings.TrimSpace(req.SessionID)
	if replyTarget := strings.TrimSpace(req.ReplyTarget); replyTarget != "" {
		routeSource = replyTarget
	}

	return IngressRoutingEntry{
		Text: req.Message,
		Scope: MessageScope{
			EntryPoint:     "channel",
			ChannelID:      strings.TrimSpace(req.Channel),
			ConversationID: routeSource,
			ThreadID:       strings.TrimSpace(req.ThreadID),
			GroupID:        strings.TrimSpace(req.GroupID),
			IsGroup:        req.IsGroup,
		},
		Delivery: DeliveryHint{
			ChannelID:      strings.TrimSpace(req.Channel),
			ConversationID: routeSource,
			ReplyTo:        strings.TrimSpace(req.ReplyTarget),
			ThreadID:       strings.TrimSpace(req.ThreadID),
		},
		Hint: RouteHint{
			RequestedSessionID: strings.TrimSpace(req.SessionID),
		},
	}
}

func routeRequestFromMainRequest(request MainRouteRequest) RouteRequest {
	return RouteRequest{
		Channel:  request.Scope.ChannelID,
		Source:   request.Scope.ConversationID,
		Text:     request.Text,
		ThreadID: request.Scope.ThreadID,
		IsGroup:  request.Scope.IsGroup,
		GroupID:  request.Scope.GroupID,
	}
}
