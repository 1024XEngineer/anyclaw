package gateway

import (
	"context"
	"fmt"

	routeingress "github.com/anyclaw/anyclaw/pkg/route/ingress"
	sessionrunner "github.com/anyclaw/anyclaw/pkg/runtime/sessionrunner"
	"github.com/anyclaw/anyclaw/pkg/state"
)

func (s *Server) resolveChannelRouteDecision(source string, sessionID string, message string, meta map[string]string) routeingress.SessionRoute {
	service := s.ingressService()
	if service == nil {
		return routeingress.SessionRoute{}
	}
	return service.DecideChannel(routeingress.ChannelRequest{
		Channel:     source,
		SessionID:   sessionID,
		ReplyTarget: meta["reply_target"],
		Message:     message,
		ThreadID:    meta["thread_id"],
		IsGroup:     meta["is_group"] == "true",
		GroupID:     meta["guild_id"],
	})
}

func (s *Server) ingressService() *routeingress.Service {
	if s == nil {
		return nil
	}
	return s.ingress
}

func (s *Server) runOrCreateChannelSession(ctx context.Context, source string, sessionID string, message string, meta map[string]string) (string, *state.Session, error) {
	decision := s.resolveChannelRouteDecision(source, sessionID, message, meta)
	sessionID, err := s.ensureChannelSession(source, sessionID, decision, meta, false)
	if err != nil {
		return "", nil, err
	}
	runner := s.ensureSessionRunner()
	if runner == nil {
		return "", nil, fmt.Errorf("session runner not initialized")
	}
	result, err := runner.RunChannel(ctx, sessionrunner.ChannelRunRequest{
		Source:    source,
		SessionID: sessionID,
		Message:   message,
		QueueMode: decision.QueueMode,
		Meta:      meta,
		Streaming: false,
	})
	if err != nil {
		return "", nil, err
	}
	if result == nil {
		return "", nil, nil
	}
	return result.Response, result.Session, nil
}

func (s *Server) runOrCreateChannelSessionStream(ctx context.Context, source string, sessionID string, message string, meta map[string]string, onChunk func(chunk string) error) (string, *state.Session, error) {
	decision := s.resolveChannelRouteDecision(source, sessionID, message, meta)
	sessionID, err := s.ensureChannelSession(source, sessionID, decision, meta, true)
	if err != nil {
		return "", nil, err
	}
	runner := s.ensureSessionRunner()
	if runner == nil {
		return "", nil, fmt.Errorf("session runner not initialized")
	}
	result, err := runner.RunChannel(ctx, sessionrunner.ChannelRunRequest{
		Source:    source,
		SessionID: sessionID,
		Message:   message,
		QueueMode: decision.QueueMode,
		Meta:      meta,
		Streaming: true,
		OnChunk: func(chunk string) {
			if onChunk != nil {
				_ = onChunk(chunk)
			}
		},
	})
	if err != nil {
		return "", nil, err
	}
	if result == nil {
		return "", nil, nil
	}
	return result.Response, result.Session, nil
}
