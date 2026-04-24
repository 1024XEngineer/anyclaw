package channelbridge

import (
	"context"
	"fmt"
	"strings"

	routeingress "github.com/1024XEngineer/anyclaw/pkg/route/ingress"
	sessionrunner "github.com/1024XEngineer/anyclaw/pkg/runtime/sessionrunner"
	"github.com/1024XEngineer/anyclaw/pkg/state"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var titleCase = cases.Title(language.English)

type ResourceSelectionValidator func(orgID string, projectID string, workspaceID string) (*state.Org, *state.Project, *state.Workspace, error)

type DefaultResourceResolver func() (string, string, string)

type EventAppender func(eventType string, sessionID string, payload map[string]any)

type ChunkFunc func(string) error

// Service bridges routed channel requests into persisted sessions before runtime execution.
type Service struct {
	Sessions                  *state.SessionManager
	Runner                    *sessionrunner.Manager
	ResolveMainAgentName      func() string
	ResolveDefaultResources   DefaultResourceResolver
	ValidateResourceSelection ResourceSelectionValidator
	AppendEvent               EventAppender
}

// EnsureSession reuses the routed session when present, otherwise creates the concrete session needed by runtime execution.
func (s Service) EnsureSession(source string, sessionID string, routed routeingress.RouteOutput, meta map[string]string, streaming bool) (string, error) {
	sessionResolution := routed.Request.Route.Session
	if routedSessionID := strings.TrimSpace(sessionResolution.SessionID); routedSessionID != "" {
		if sessionResolution.Created {
			s.appendChannelSessionCreatedEvent(source, routedSessionID, sessionResolution.TitleHint, meta, streaming)
		}
		return routedSessionID, nil
	}

	createOpts, err := s.buildChannelSessionCreateOptions(source, sessionID, routed.Request.Route, meta)
	if err != nil {
		return "", err
	}
	session, err := s.Sessions.CreateWithOptions(createOpts)
	if err != nil {
		return "", err
	}
	if session == nil {
		return "", nil
	}

	if sessionResolution.NeedsCreate {
		s.appendChannelSessionCreatedEvent(source, session.ID, session.Title, meta, streaming)
	}
	return session.ID, nil
}

// RunRouted executes a non-streaming channel message after routing and session materialization.
func (s Service) RunRouted(ctx context.Context, source string, sessionID string, message string, routed routeingress.RouteOutput, meta map[string]string) (string, *state.Session, error) {
	normalizedMeta := normalizedChannelRunMeta(meta, routed.Request.Route.Delivery)
	resolvedSessionID, err := s.EnsureSession(source, sessionID, routed, normalizedMeta, false)
	if err != nil {
		return "", nil, err
	}
	if s.Runner == nil {
		return "", nil, fmt.Errorf("session runner not initialized")
	}
	result, err := s.Runner.RunChannel(ctx, sessionrunner.ChannelRunRequest{
		Source:    source,
		SessionID: resolvedSessionID,
		Message:   message,
		QueueMode: routed.Request.Route.Session.QueueMode,
		Meta:      normalizedMeta,
		Streaming: false,
	})
	if err != nil {
		if result != nil {
			return result.Response, result.Session, err
		}
		return "", nil, err
	}
	if result == nil {
		return "", nil, nil
	}
	return result.Response, result.Session, nil
}

// RunRoutedStream executes a streaming channel message after routing and session materialization.
func (s Service) RunRoutedStream(ctx context.Context, source string, sessionID string, message string, routed routeingress.RouteOutput, meta map[string]string, onChunk ChunkFunc) (string, *state.Session, error) {
	normalizedMeta := normalizedChannelRunMeta(meta, routed.Request.Route.Delivery)
	resolvedSessionID, err := s.EnsureSession(source, sessionID, routed, normalizedMeta, true)
	if err != nil {
		return "", nil, err
	}
	if s.Runner == nil {
		return "", nil, fmt.Errorf("session runner not initialized")
	}
	result, err := s.Runner.RunChannel(ctx, sessionrunner.ChannelRunRequest{
		Source:    source,
		SessionID: resolvedSessionID,
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
		if result != nil {
			return result.Response, result.Session, err
		}
		return "", nil, err
	}
	if result == nil {
		return "", nil, nil
	}
	return result.Response, result.Session, nil
}

func (s Service) appendChannelSessionCreatedEvent(source string, sessionID string, title string, meta map[string]string, streaming bool) {
	if s.AppendEvent == nil {
		return
	}
	sessionTitle := strings.TrimSpace(title)
	if sessionTitle == "" && s.Sessions != nil {
		if session, ok := s.Sessions.Get(sessionID); ok && session != nil {
			sessionTitle = strings.TrimSpace(session.Title)
		}
	}
	if sessionTitle == "" {
		sessionTitle = titleCase.String(source) + " session"
	}
	payload := channelMetaPayload(map[string]any{
		"title":  sessionTitle,
		"source": source,
	}, meta)
	if streaming {
		payload["streaming"] = true
	}
	s.AppendEvent("session.created", sessionID, payload)
}

func (s Service) buildChannelSessionCreateOptions(source string, sessionID string, resolution routeingress.RouteResolution, meta map[string]string) (state.SessionCreateOptions, error) {
	agentName := strings.TrimSpace(resolution.Agent.AgentName)
	if agentName == "" && s.ResolveMainAgentName != nil {
		agentName = strings.TrimSpace(s.ResolveMainAgentName())
	}

	orgID, projectID, workspaceID := "", "", ""
	if s.ResolveDefaultResources != nil {
		orgID, projectID, workspaceID = s.ResolveDefaultResources()
	}
	org, project, workspace, err := s.ValidateResourceSelection(orgID, projectID, workspaceID)
	if err != nil {
		return state.SessionCreateOptions{}, err
	}

	title := strings.TrimSpace(resolution.Session.TitleHint)
	if title == "" {
		title = titleCase.String(source) + " session"
	}

	createOpts := state.SessionCreateOptions{
		Title:           title,
		AgentName:       agentName,
		Org:             org.ID,
		Project:         project.ID,
		Workspace:       workspace.ID,
		SessionMode:     normalizeSingleAgentSessionMode(resolution.Session.SessionMode, "channel-dm"),
		QueueMode:       resolution.Session.QueueMode,
		ReplyBack:       resolution.Session.ReplyBack,
		SourceChannel:   source,
		SourceID:        channelSourceID(resolution.Delivery.TransportMeta, sessionID),
		UserID:          strings.TrimSpace(meta["user_id"]),
		UserName:        firstNonEmpty(strings.TrimSpace(meta["username"]), strings.TrimSpace(meta["user_name"])),
		ReplyTarget:     strings.TrimSpace(resolution.Delivery.ReplyTo),
		ThreadID:        strings.TrimSpace(resolution.Delivery.ThreadID),
		ConversationKey: resolution.Session.SessionKey,
		TransportMeta:   channelSessionTransportMeta(resolution.Delivery.TransportMeta),
	}
	if createOpts.SessionMode == "" {
		createOpts.SessionMode = "main"
	}
	return createOpts, nil
}

func normalizeSingleAgentSessionMode(mode string, fallback string) string {
	mode = strings.TrimSpace(strings.ToLower(mode))
	if mode == "" {
		return strings.TrimSpace(fallback)
	}
	if isGroupSessionMode(mode) {
		return strings.TrimSpace(fallback)
	}
	return mode
}

func isGroupSessionMode(mode string) bool {
	switch strings.TrimSpace(strings.ToLower(mode)) {
	case "group", "group-shared", "channel-group":
		return true
	default:
		return false
	}
}

func channelMetaPayload(base map[string]any, meta map[string]string) map[string]any {
	payload := make(map[string]any, len(base)+len(meta))
	for k, v := range base {
		payload[k] = v
	}
	for k, v := range meta {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			payload[k] = trimmed
		}
	}
	return payload
}

func channelSourceID(meta map[string]string, fallback string) string {
	return firstNonEmpty(strings.TrimSpace(meta["user_id"]), strings.TrimSpace(meta["reply_target"]), fallback)
}

func channelSessionTransportMeta(meta map[string]string) map[string]string {
	transportMeta := map[string]string{}
	for _, key := range []string{"channel_id", "chat_id", "guild_id", "attachment_count"} {
		if v := strings.TrimSpace(meta[key]); v != "" {
			transportMeta[key] = v
		}
	}
	return transportMeta
}

func normalizedChannelRunMeta(meta map[string]string, target routeingress.DeliveryTarget) map[string]string {
	normalized := cloneStringMap(meta)
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

func cloneStringMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return map[string]string{}
	}
	cloned := make(map[string]string, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
