package ingress

type RouteRequest struct {
	Channel  string
	Source   string
	Text     string
	ThreadID string
	IsGroup  bool
	GroupID  string
}

// SessionRoute describes where an inbound message should land. It is limited
// to session-scoped routing concerns and must not select agents or resources.
type SessionRoute struct {
	Key         string `json:"key"`
	SessionMode string `json:"session_mode"`
	SessionID   string `json:"session_id,omitempty"`
	QueueMode   string `json:"queue_mode,omitempty"`
	ReplyBack   bool   `json:"reply_back,omitempty"`
	Title       string `json:"title,omitempty"`
	MatchedRule string `json:"matched_rule,omitempty"`
	IsThread    bool   `json:"is_thread,omitempty"`
	ThreadID    string `json:"thread_id,omitempty"`
}
