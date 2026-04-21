package ingress

import "time"

// RouteRequest is the lightweight routing input consumed by the M2 router.
type RouteRequest struct {
	Channel  string
	Source   string
	Text     string
	ThreadID string
	IsGroup  bool
	GroupID  string
}

// MessageActor captures the sender facts needed by the route layer.
type MessageActor struct {
	UserID      string
	DisplayName string
}

// MessageScope carries the transport coordinates for one ingress message.
type MessageScope struct {
	EntryPoint     string
	ChannelID      string
	ConversationID string
	ThreadID       string
	GroupID        string
	IsGroup        bool
	Metadata       map[string]string
}

// DeliveryHint stores the inbound delivery facts observed before delivery routing.
type DeliveryHint struct {
	ChannelID      string
	ConversationID string
	ReplyTo        string
	ThreadID       string
	Metadata       map[string]string
}

// RouteHint carries optional caller hints into the routing flow.
type RouteHint struct {
	RequestedAgentName string
	RequestedSessionID string
}

// IngressRoutingEntry is the trusted route-layer input passed from gateway to route.
type IngressRoutingEntry struct {
	MessageID  string
	Text       string
	Actor      MessageActor
	Scope      MessageScope
	Delivery   DeliveryHint
	Hint       RouteHint
	ReceivedAt time.Time
}

// MainRouteRequest is the normalized request emitted by the M1 projector.
type MainRouteRequest struct {
	MessageID    string
	Text         string
	Actor        MessageActor
	Scope        MessageScope
	DeliveryHint DeliveryHint
	Hint         RouteHint
	ReceivedAt   time.Time
}

// AgentResolution is the M2 output for agent selection.
type AgentResolution struct {
	AgentName string
	MatchedBy string
}

// RouteDecision is the lightweight M2 -> M3 session policy output.
type RouteDecision struct {
	RouteKey        string
	ForcedSessionID string
	SessionMode     string
	QueueMode       string
	ReplyBack       bool
	TitleHint       string
	MatchedRule     string
	ThreadID        string
}

// SessionResolution is the M3 output for session selection.
type SessionResolution struct {
	SessionID   string
	SessionKey  string
	SessionMode string
	QueueMode   string
	ReplyBack   bool
	TitleHint   string
	MatchedBy   string
	MatchedRule string
	ThreadID    string
	Created     bool
	NeedsCreate bool
}

// SessionSnapshot is the route-layer view of one persisted session.
type SessionSnapshot struct {
	ID              string
	AgentName       string
	WorkspaceID     string
	ConversationKey string
	SessionMode     string
	QueueMode       string
	ReplyBack       bool
	ReplyTarget     string
	ThreadID        string
	TransportMeta   map[string]string
}

// SessionCreateOptions is the route-layer request used when M3 creates a new session.
type SessionCreateOptions struct {
	Title           string
	AgentName       string
	SessionMode     string
	QueueMode       string
	ReplyBack       bool
	SourceChannel   string
	SourceID        string
	UserID          string
	UserName        string
	ReplyTarget     string
	ThreadID        string
	ConversationKey string
	TransportMeta   map[string]string
	IsGroup         bool
	GroupKey        string
}

// SessionStore is the session interface required by the route-layer M3 resolver.
type SessionStore interface {
	GetSession(sessionID string) (SessionSnapshot, bool, error)
	FindByConversationKey(conversationKey string) (SessionSnapshot, bool, error)
	BindConversationKey(sessionID string, conversationKey string) (SessionSnapshot, error)
	Create(opts SessionCreateOptions) (SessionSnapshot, error)
}
