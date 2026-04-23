package governance

// DeliveryHint stores the inbound reply facts observed before route delivery resolution.
type DeliveryHint struct {
	ReplyTarget   string
	ThreadID      string
	TransportMeta map[string]string
}

// RouteContext carries the trusted ingress routing facts produced by governance.
type RouteContext struct {
	EntryPoint     string
	SourceType     string
	ChannelID      string
	ConversationID string
	ThreadID       string
	GroupID        string
	IsGroup        bool
	PeerID         string
	PeerKind       string
	Delivery       DeliveryHint
	Metadata       map[string]string
}

// ActorRef is the governance-confirmed caller identity used by business ingress.
type ActorRef struct {
	UserID        string
	AccountID     string
	DisplayName   string
	Roles         []string
	Authenticated bool
}

// GovernanceResult records the governance outcome for one accepted request.
type GovernanceResult struct {
	Authenticated bool
	PermissionSet []string
	RateLimitKey  string
	RiskLevel     string
	DenyReason    string
}

// ConnectionRequest is the governance contract for one control-plane connection.
type ConnectionRequest struct {
	ConnectionID    string
	Protocol        string
	Path            string
	ClientID        string
	RequestedScopes []string
	Metadata        map[string]string
}

// ConnectionAuthorization is the governance outcome for one accepted connection.
type ConnectionAuthorization struct {
	Caller ActorRef
	Result GovernanceResult
}

// CommandRequest is the governance contract for one control-plane command.
type CommandRequest struct {
	RequestID          string
	Method             string
	ResourceID         string
	RequiredPermission string
	Params             map[string]string
}

// CommandAuthorization is the governance outcome for one accepted command.
type CommandAuthorization struct {
	Request CommandRequest
	Caller  ActorRef
	Result  GovernanceResult
}

// RawRequest is the gateway ingress contract before governance normalization.
type RawRequest struct {
	RequestID           string
	SourceType          string
	EntryPoint          string
	ChannelID           string
	SessionID           string
	ConversationID      string
	PeerID              string
	PeerKind            string
	ThreadID            string
	GroupID             string
	IsGroup             bool
	Message             string
	TitleHint           string
	ActorUserID         string
	ActorDisplayName    string
	RequestedAgentName  string
	RequestedSessionID  string
	DeliveryReplyTarget string
	Metadata            map[string]string
}

// NormalizedRequest is the trusted business-ingress contract produced by governance.
type NormalizedRequest struct {
	RequestID          string
	SourceType         string
	Actor              ActorRef
	ContentText        string
	TitleHint          string
	RouteContext       RouteContext
	Governance         GovernanceResult
	RequestedAgentName string
	RequestedSessionID string
}
