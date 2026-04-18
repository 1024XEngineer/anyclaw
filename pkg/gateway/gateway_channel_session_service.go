package gateway

import (
	"strings"

	gatewayintake "github.com/anyclaw/anyclaw/pkg/gateway/intake"
	routeingress "github.com/anyclaw/anyclaw/pkg/route/ingress"
	"github.com/anyclaw/anyclaw/pkg/state"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var titleCase = cases.Title(language.English)

func (s *Server) ensureChannelSession(source string, sessionID string, routed routeingress.RouteOutput, meta map[string]string, streaming bool) (string, error) {
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
	session, err := s.sessions.CreateWithOptions(createOpts)
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

func (s *Server) appendChannelSessionCreatedEvent(source string, sessionID string, title string, meta map[string]string, streaming bool) {
	sessionTitle := strings.TrimSpace(title)
	if sessionTitle == "" && s.sessions != nil {
		if session, ok := s.sessions.Get(sessionID); ok && session != nil {
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
	s.appendEvent("session.created", sessionID, payload)
}

func (s *Server) buildChannelSessionCreateOptions(source string, sessionID string, resolution routeingress.RouteResolution, meta map[string]string) (state.SessionCreateOptions, error) {
	agentName := strings.TrimSpace(resolution.Agent.AgentName)
	if agentName == "" {
		agentName = s.mainRuntime.Config.ResolveMainAgentName()
	}
	orgID, projectID, workspaceID := defaultResourceIDs(s.mainRuntime.WorkingDir)

	org, project, workspace, err := s.validateResourceSelection(orgID, projectID, workspaceID)
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
		SessionMode:     gatewayintake.NormalizeSingleAgentSessionMode(resolution.Session.SessionMode, "channel-dm"),
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
