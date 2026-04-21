package gateway

import (
	"fmt"
	"strings"

	gatewayintake "github.com/1024XEngineer/anyclaw/pkg/gateway/intake"
	"github.com/1024XEngineer/anyclaw/pkg/route/ingress"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

type ingressSessionStore struct {
	server  *Server
	manager *state.SessionManager
}

func (s ingressSessionStore) GetSession(sessionID string) (ingress.SessionSnapshot, bool, error) {
	if s.manager == nil {
		return ingress.SessionSnapshot{}, false, nil
	}
	session, ok := s.manager.Get(sessionID)
	if !ok || session == nil {
		return ingress.SessionSnapshot{}, false, nil
	}
	return ingress.SessionSnapshot{
		ID:              session.ID,
		AgentName:       state.SessionExecutionAgent(session),
		WorkspaceID:     state.SessionExecutionWorkspace(session),
		ConversationKey: session.ConversationKey,
		SessionMode:     session.SessionMode,
		QueueMode:       session.QueueMode,
		ReplyBack:       session.ReplyBack,
		ReplyTarget:     session.ReplyTarget,
		ThreadID:        session.ThreadID,
		TransportMeta:   cloneBindingConfig(session.TransportMeta),
	}, true, nil
}

func (s ingressSessionStore) FindByConversationKey(conversationKey string) (ingress.SessionSnapshot, bool, error) {
	if s.manager == nil {
		return ingress.SessionSnapshot{}, false, nil
	}
	session, ok := s.manager.FindByConversationKey(conversationKey)
	if !ok || session == nil {
		return ingress.SessionSnapshot{}, false, nil
	}
	return ingress.SessionSnapshot{
		ID:              session.ID,
		AgentName:       state.SessionExecutionAgent(session),
		WorkspaceID:     state.SessionExecutionWorkspace(session),
		ConversationKey: session.ConversationKey,
		SessionMode:     session.SessionMode,
		QueueMode:       session.QueueMode,
		ReplyBack:       session.ReplyBack,
		ReplyTarget:     session.ReplyTarget,
		ThreadID:        session.ThreadID,
		TransportMeta:   cloneBindingConfig(session.TransportMeta),
	}, true, nil
}

func (s ingressSessionStore) BindConversationKey(sessionID string, conversationKey string) (ingress.SessionSnapshot, error) {
	session, err := s.manager.BindConversationKey(sessionID, conversationKey)
	if err != nil {
		return ingress.SessionSnapshot{}, err
	}
	return ingress.SessionSnapshot{
		ID:              session.ID,
		AgentName:       state.SessionExecutionAgent(session),
		WorkspaceID:     state.SessionExecutionWorkspace(session),
		ConversationKey: session.ConversationKey,
		SessionMode:     session.SessionMode,
		QueueMode:       session.QueueMode,
		ReplyBack:       session.ReplyBack,
		ReplyTarget:     session.ReplyTarget,
		ThreadID:        session.ThreadID,
		TransportMeta:   cloneBindingConfig(session.TransportMeta),
	}, nil
}

func (s ingressSessionStore) Create(opts ingress.SessionCreateOptions) (ingress.SessionSnapshot, error) {
	if s.manager == nil {
		return ingress.SessionSnapshot{}, fmt.Errorf("session manager is unavailable")
	}
	createOpts, err := s.buildStateCreateOptions(opts)
	if err != nil {
		return ingress.SessionSnapshot{}, err
	}
	session, err := s.manager.CreateWithOptions(createOpts)
	if err != nil {
		return ingress.SessionSnapshot{}, err
	}
	if session == nil {
		return ingress.SessionSnapshot{}, nil
	}
	return ingress.SessionSnapshot{
		ID:              session.ID,
		AgentName:       state.SessionExecutionAgent(session),
		WorkspaceID:     state.SessionExecutionWorkspace(session),
		ConversationKey: session.ConversationKey,
		SessionMode:     session.SessionMode,
		QueueMode:       session.QueueMode,
		ReplyBack:       session.ReplyBack,
		ReplyTarget:     session.ReplyTarget,
		ThreadID:        session.ThreadID,
		TransportMeta:   cloneBindingConfig(session.TransportMeta),
	}, nil
}

func (s ingressSessionStore) buildStateCreateOptions(opts ingress.SessionCreateOptions) (state.SessionCreateOptions, error) {
	if s.server == nil {
		return state.SessionCreateOptions{}, fmt.Errorf("route session store requires gateway server context")
	}

	agentName := firstNonEmpty(opts.AgentName, s.server.mainRuntime.Config.ResolveMainAgentName())
	orgID, projectID, workspaceID := routeResourceSelection(opts.TransportMeta)
	if orgID == "" && projectID == "" && workspaceID == "" {
		orgID, projectID, workspaceID = defaultResourceIDs(s.server.mainRuntime.WorkingDir)
	}
	org, project, workspace, err := s.server.validateResourceSelection(orgID, projectID, workspaceID)
	if err != nil {
		return state.SessionCreateOptions{}, err
	}
	transportMeta := cloneBindingConfig(opts.TransportMeta)
	if transportMeta == nil {
		transportMeta = map[string]string{}
	}
	if channelID := strings.TrimSpace(opts.SourceChannel); channelID != "" {
		transportMeta["channel_id"] = channelID
	}
	if replyTarget := strings.TrimSpace(opts.ReplyTarget); replyTarget != "" {
		if strings.TrimSpace(transportMeta["chat_id"]) == "" {
			transportMeta["chat_id"] = replyTarget
		}
		if strings.TrimSpace(transportMeta["conversation_id"]) == "" {
			transportMeta["conversation_id"] = replyTarget
		}
		if strings.TrimSpace(transportMeta["reply_target"]) == "" {
			transportMeta["reply_target"] = replyTarget
		}
	}
	if threadID := strings.TrimSpace(opts.ThreadID); threadID != "" {
		transportMeta["thread_id"] = threadID
	}

	createOpts := state.SessionCreateOptions{
		Title:           opts.Title,
		AgentName:       agentName,
		Org:             org.ID,
		Project:         project.ID,
		Workspace:       workspace.ID,
		SessionMode:     gatewayintake.NormalizeSingleAgentSessionMode(opts.SessionMode, "channel-dm"),
		QueueMode:       opts.QueueMode,
		ReplyBack:       opts.ReplyBack,
		SourceChannel:   opts.SourceChannel,
		SourceID:        opts.SourceID,
		UserID:          opts.UserID,
		UserName:        opts.UserName,
		ReplyTarget:     opts.ReplyTarget,
		ThreadID:        opts.ThreadID,
		ConversationKey: opts.ConversationKey,
		TransportMeta:   channelSessionTransportMeta(transportMeta),
		IsGroup:         opts.IsGroup,
		GroupKey:        opts.GroupKey,
	}
	if createOpts.SessionMode == "" {
		createOpts.SessionMode = "main"
	}
	return createOpts, nil
}

func routeResourceSelection(meta map[string]string) (string, string, string) {
	if len(meta) == 0 {
		return "", "", ""
	}
	return firstNonEmpty(meta["org"], meta["org_id"]),
		firstNonEmpty(meta["project"], meta["project_id"]),
		firstNonEmpty(meta["workspace"], meta["workspace_id"])
}
