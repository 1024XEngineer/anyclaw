package routing

// Binding describes a future explicit route override.
type Binding struct {
	Channel   string
	AccountID string
	PeerID    string
	AgentID   string
}
