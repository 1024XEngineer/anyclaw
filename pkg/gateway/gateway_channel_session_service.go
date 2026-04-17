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

func (s *Server) ensureChannelSession(source string, sessionID string, decision routeingress.SessionRoute, meta map[string]string, streaming bool) (string, error) {
	createOpts, err := s.buildChannelSessionCreateOptions(source, sessionID, decision, meta)
	if err != nil {
		return "", err
	}
	resolution, err := s.sessions.ResolveConversationSession(state.ConversationSessionResolveOptions{
		SessionID:       sessionID,
		RoutedSessionID: decision.SessionID,
		ConversationKey: decision.Key,
		CreateOptions:   createOpts,
	})
	if err != nil {
		return "", err
	}
	if resolution == nil || resolution.Session == nil {
		return "", nil
	}

	session := resolution.Session
	if resolution.Created {
		payload := channelMetaPayload(map[string]any{
			"title":  session.Title,
			"source": source,
		}, meta)
		if streaming {
			payload["streaming"] = true
		}
		s.appendEvent("session.created", session.ID, payload)
	}
	return session.ID, nil
}

func (s *Server) buildChannelSessionCreateOptions(source string, sessionID string, decision routeingress.SessionRoute, meta map[string]string) (state.SessionCreateOptions, error) {
	agentName := s.mainRuntime.Config.ResolveMainAgentName()
	orgID, projectID, workspaceID := defaultResourceIDs(s.mainRuntime.WorkingDir)

	org, project, workspace, err := s.validateResourceSelection(orgID, projectID, workspaceID)
	if err != nil {
		return state.SessionCreateOptions{}, err
	}

	title := strings.TrimSpace(decision.Title)
	if title == "" {
		title = titleCase.String(source) + " session"
	}

	createOpts := state.SessionCreateOptions{
		Title:           title,
		AgentName:       agentName,
		Org:             org.ID,
		Project:         project.ID,
		Workspace:       workspace.ID,
		SessionMode:     gatewayintake.NormalizeSingleAgentSessionMode(decision.SessionMode, "channel-dm"),
		QueueMode:       decision.QueueMode,
		ReplyBack:       decision.ReplyBack,
		SourceChannel:   source,
		SourceID:        channelSourceID(meta, sessionID),
		UserID:          strings.TrimSpace(meta["user_id"]),
		UserName:        firstNonEmpty(strings.TrimSpace(meta["username"]), strings.TrimSpace(meta["user_name"])),
		ReplyTarget:     strings.TrimSpace(meta["reply_target"]),
		ThreadID:        strings.TrimSpace(meta["thread_id"]),
		ConversationKey: decision.Key,
		TransportMeta:   channelSessionTransportMeta(meta),
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
