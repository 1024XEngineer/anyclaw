package routing

import "context"

// RouteInput describes the metadata used to route an inbound message.
type RouteInput struct {
	Channel       string
	AccountID     string
	PeerID        string
	PeerKind      string
	ParentPeerID  string
	GuildID       string
	TeamID        string
	MemberRoleIDs []string
}

// RouteResult is the canonical route decision for one inbound message.
type RouteResult struct {
	AgentID        string
	Channel        string
	AccountID      string
	SessionKey     string
	MainSessionKey string
	MatchedBy      string
}

// Router resolves inbound metadata to an agent and session.
type Router interface {
	Resolve(ctx context.Context, in RouteInput) (RouteResult, error)
}

// StaticRouter is a temporary implementation that always returns the default route.
type StaticRouter struct {
	DefaultAgentID string
}

// Resolve returns a deterministic default route for the current input.
func (r StaticRouter) Resolve(_ context.Context, in RouteInput) (RouteResult, error) {
	agentID := r.DefaultAgentID
	if agentID == "" {
		agentID = "default-agent"
	}
	mainKey := BuildMainSessionKey(agentID, in.AccountID)
	return RouteResult{
		AgentID:        agentID,
		Channel:        in.Channel,
		AccountID:      normalizeAccountID(in.AccountID),
		SessionKey:     BuildPeerSessionKey(agentID, in.AccountID, in.PeerKind, in.PeerID),
		MainSessionKey: mainKey,
		MatchedBy:      "default",
	}, nil
}
