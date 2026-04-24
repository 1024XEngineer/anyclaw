package gateway

import (
	"context"

	gatewaycommands "github.com/1024XEngineer/anyclaw/pkg/gateway/commands"
	gatewaysurface "github.com/1024XEngineer/anyclaw/pkg/gateway/surface"
	sessionrunner "github.com/1024XEngineer/anyclaw/pkg/runtime/sessionrunner"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

func (s *Server) runChatIngressMessage(ctx context.Context, source string, req gatewaycommands.ChatSendRequest, requestedAgentName string) (string, *state.Session, error) {
	raw := s.surfaceService().BuildChatRawRequest(gatewaysurface.ChatIngressInput{
		Source:             source,
		Request:            req,
		RequestedAgentName: requestedAgentName,
	})
	normalized, err := s.governanceService().Accept(ctx, raw)
	if err != nil {
		return "", nil, err
	}
	routed, err := s.ingressHandoffService().RouteNormalized(ctx, normalized)
	if err != nil {
		return "", nil, err
	}
	sessionID := routed.Request.Route.Session.SessionID
	if sessionID == "" {
		sessionID = req.SessionID
	}
	if sessionID == "" {
		return "", nil, nil
	}
	if routed.Request.Route.Session.Created {
		if session, ok := s.sessions.Get(sessionID); ok && session != nil {
			s.appendEvent("session.created", session.ID, sessionCreatedEventPayload(session))
		}
	}
	return s.sessionBridgeService().RunMessage(ctx, sessionID, req.Title, req.Message, sessionrunner.RunOptions{
		Source:  source,
		Channel: source,
	})
}
