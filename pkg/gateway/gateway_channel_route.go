package gateway

import (
	"context"
	"fmt"
	"strings"

	routeingress "github.com/anyclaw/anyclaw/pkg/route/ingress"
	sessionrunner "github.com/anyclaw/anyclaw/pkg/runtime/sessionrunner"
	"github.com/anyclaw/anyclaw/pkg/state"
)

func (s *Server) resolveChannelRoute(source string, sessionID string, message string, meta map[string]string) (routeingress.RouteOutput, error) {
	service := s.ingressService()
	if service == nil {
		return routeingress.RouteOutput{}, fmt.Errorf("ingress service not initialized")
	}
	return service.Route(context.Background(), routeingress.RouteInput{
		Entry: routeingress.IngressRoutingEntry{
			Text: message,
			Actor: routeingress.MessageActor{
				UserID:      strings.TrimSpace(meta["user_id"]),
				DisplayName: firstNonEmpty(strings.TrimSpace(meta["username"]), strings.TrimSpace(meta["user_name"])),
			},
			Scope: routeingress.MessageScope{
				EntryPoint:     "channel",
				ChannelID:      source,
				ConversationID: firstNonEmpty(strings.TrimSpace(meta["reply_target"]), strings.TrimSpace(sessionID)),
				ThreadID:       strings.TrimSpace(meta["thread_id"]),
				GroupID:        strings.TrimSpace(meta["guild_id"]),
				IsGroup:        meta["is_group"] == "true",
				Metadata:       cloneBindingConfig(meta),
			},
			Delivery: routeingress.DeliveryHint{
				ChannelID:      source,
				ConversationID: firstNonEmpty(strings.TrimSpace(meta["reply_target"]), strings.TrimSpace(sessionID)),
				ReplyTo:        strings.TrimSpace(meta["reply_target"]),
				ThreadID:       strings.TrimSpace(meta["thread_id"]),
				Metadata:       cloneBindingConfig(meta),
			},
			Hint: routeingress.RouteHint{
				RequestedAgentName: firstNonEmpty(
					strings.TrimSpace(meta["agent_name"]),
					strings.TrimSpace(meta["assistant_name"]),
					strings.TrimSpace(meta["agent"]),
					strings.TrimSpace(meta["assistant"]),
				),
				RequestedSessionID: strings.TrimSpace(sessionID),
			},
		},
	})
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

func normalizedChannelRunMeta(meta map[string]string, target routeingress.DeliveryTarget) map[string]string {
	normalized := cloneBindingConfig(meta)
	for key, value := range target.TransportMeta {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			normalized[key] = trimmed
		}
	}
	if trimmed := strings.TrimSpace(target.ChannelID); trimmed != "" {
		normalized["channel"] = trimmed
		normalized["channel_id"] = trimmed
	}
	if trimmed := strings.TrimSpace(target.ConversationID); trimmed != "" {
		if strings.TrimSpace(normalized["chat_id"]) == "" {
			normalized["chat_id"] = trimmed
		}
		if strings.TrimSpace(normalized["conversation_id"]) == "" {
			normalized["conversation_id"] = trimmed
		}
	}
	if trimmed := strings.TrimSpace(target.ReplyTo); trimmed != "" {
		normalized["reply_target"] = trimmed
	}
	if trimmed := strings.TrimSpace(target.ThreadID); trimmed != "" {
		normalized["thread_id"] = trimmed
	}
	return normalized
}

func (s *Server) runOrCreateChannelSession(ctx context.Context, source string, sessionID string, message string, meta map[string]string) (string, *state.Session, error) {
	routed, err := s.resolveChannelRoute(source, sessionID, message, meta)
	if err != nil {
		return "", nil, err
	}
	normalizedMeta := normalizedChannelRunMeta(meta, routed.Request.Route.Delivery)
	sessionID, err = s.ensureChannelSession(source, sessionID, routed, normalizedMeta, false)
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
		QueueMode: routed.Request.Route.Session.QueueMode,
		Meta:      normalizedMeta,
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
	routed, err := s.resolveChannelRoute(source, sessionID, message, meta)
	if err != nil {
		return "", nil, err
	}
	normalizedMeta := normalizedChannelRunMeta(meta, routed.Request.Route.Delivery)
	sessionID, err = s.ensureChannelSession(source, sessionID, routed, normalizedMeta, true)
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
		QueueMode: routed.Request.Route.Session.QueueMode,
		Meta:      normalizedMeta,
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
