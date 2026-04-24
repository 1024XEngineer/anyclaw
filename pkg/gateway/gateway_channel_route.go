package gateway

import (
	"context"

	gatewaysurface "github.com/1024XEngineer/anyclaw/pkg/gateway/surface"
	routeingress "github.com/1024XEngineer/anyclaw/pkg/route/ingress"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

func (s *Server) resolveChannelRoute(source string, sessionID string, message string, meta map[string]string) (routeingress.RouteOutput, error) {
	raw := s.surfaceService().BuildChannelRawRequest(gatewaysurface.ChannelInput{
		Source:    source,
		SessionID: sessionID,
		Message:   message,
		Meta:      meta,
	})
	normalized, err := s.governanceService().Accept(context.Background(), raw)
	if err != nil {
		return routeingress.RouteOutput{}, err
	}
	return s.ingressHandoffService().RouteNormalized(context.Background(), normalized)
}

func (s *Server) resolveChannelRouteDecision(source string, sessionID string, message string, meta map[string]string) routeingress.SessionRoute {
	routed, err := s.resolveChannelRoute(source, sessionID, message, meta)
	if err != nil {
		return routeingress.SessionRoute{}
	}
	return routed.Request.Route.Session.LegacySessionRoute()
}

func (s *Server) ingressService() *routeingress.Service {
	if s == nil {
		return nil
	}
	return s.ingress
}

func (s *Server) runOrCreateChannelSession(ctx context.Context, source string, sessionID string, message string, meta map[string]string) (string, *state.Session, error) {
	routed, err := s.resolveChannelRoute(source, sessionID, message, meta)
	if err != nil {
		return "", nil, err
	}
	return s.channelBridgeService().RunRouted(ctx, source, sessionID, message, routed, meta)
}

func (s *Server) runOrCreateChannelSessionStream(ctx context.Context, source string, sessionID string, message string, meta map[string]string, onChunk func(chunk string) error) (string, *state.Session, error) {
	routed, err := s.resolveChannelRoute(source, sessionID, message, meta)
	if err != nil {
		return "", nil, err
	}
	return s.channelBridgeService().RunRoutedStream(ctx, source, sessionID, message, routed, meta, onChunk)
}
