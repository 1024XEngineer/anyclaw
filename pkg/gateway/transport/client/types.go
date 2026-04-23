package sdk

import (
	"encoding/json"
	"time"
)

type (
	MessageID  string
	ChannelID  string
	SessionID  string
	AgentID    string
	ToolCallID string
	NodeID     string
	UserID     string
)

type MessageType string

const (
	MessageTypeText    MessageType = "text"
	MessageTypeImage   MessageType = "image"
	MessageTypeAudio   MessageType = "audio"
	MessageTypeVideo   MessageType = "video"
	MessageTypeFile    MessageType = "file"
	MessageTypeCommand MessageType = "command"
)

type SenderType string

const (
	SenderTypeUser    SenderType = "user"
	SenderTypeAgent   SenderType = "agent"
	SenderTypeChannel SenderType = "channel"
	SenderTypeSystem  SenderType = "system"
)

type OutgoingMessage struct {
	To          string          `json:"to"`
	Content     string          `json:"content"`
	ContentType MessageType     `json:"content_type,omitempty"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
	ReplyTo     MessageID       `json:"reply_to,omitempty"`
}

type IncomingMessage struct {
	ID          MessageID       `json:"id"`
	ChannelID   ChannelID       `json:"channel_id"`
	SessionID   SessionID       `json:"session_id"`
	Sender      Sender          `json:"sender"`
	Content     string          `json:"content"`
	ContentType MessageType     `json:"content_type"`
	Timestamp   time.Time       `json:"timestamp"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
}

type Sender struct {
	ID       UserID          `json:"id"`
	Name     string          `json:"name"`
	Type     SenderType      `json:"type"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}

type ToolCall struct {
	ID       ToolCallID      `json:"id"`
	Name     string          `json:"name"`
	Input    json.RawMessage `json:"input"`
	Progress float64         `json:"progress,omitempty"`
	Output   json.RawMessage `json:"output,omitempty"`
	Error    string          `json:"error,omitempty"`
}

type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type ToolResult struct {
	ToolCallID ToolCallID      `json:"tool_call_id"`
	Output     json.RawMessage `json:"output,omitempty"`
	Error      string          `json:"error,omitempty"`
	IsError    bool            `json:"is_error"`
}

type Session struct {
	ID        SessionID    `json:"id"`
	Title     string       `json:"title,omitempty"`
	AgentID   AgentID      `json:"agent_id"`
	ChannelID ChannelID    `json:"channel_id"`
	State     SessionState `json:"state"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

type SessionState string

const (
	SessionStateActive  SessionState = "active"
	SessionStateIdle    SessionState = "idle"
	SessionStateWaiting SessionState = "waiting"
	SessionStateError   SessionState = "error"
)

type Agent struct {
	ID           AgentID  `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Model        string   `json:"model"`
	Provider     string   `json:"provider"`
	Tools        []string `json:"tools,omitempty"`
	Skills       []string `json:"skills,omitempty"`
	SystemPrompt string   `json:"system_prompt,omitempty"`
}

type NodeInfo struct {
	ID           NodeID     `json:"id"`
	Name         string     `json:"name"`
	Platform     string     `json:"platform"`
	Status       NodeStatus `json:"status"`
	Capabilities []string   `json:"capabilities"`
	LastSeen     time.Time  `json:"last_seen"`
}

type NodeStatus string

const (
	NodeStatusOnline  NodeStatus = "online"
	NodeStatusOffline NodeStatus = "offline"
	NodeStatusBusy    NodeStatus = "busy"
)

type NodeAction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

type NodeActionResult struct {
	Action  string          `json:"action"`
	Output  json.RawMessage `json:"output,omitempty"`
	Error   string          `json:"error,omitempty"`
	IsError bool            `json:"is_error"`
}

type ChannelInfo struct {
	ID           ChannelID `json:"id"`
	Name         string    `json:"name"`
	Type         string    `json:"type"`
	Status       string    `json:"status"`
	Connected    bool      `json:"connected"`
	MessageCount int64     `json:"message_count"`
}

type Presence struct {
	UserID    UserID    `json:"user_id"`
	Status    string    `json:"status"`
	ChannelID ChannelID `json:"channel_id"`
	UpdatedAt time.Time `json:"updated_at"`
}

type TypingIndicator struct {
	UserID    UserID    `json:"user_id"`
	ChannelID ChannelID `json:"channel_id"`
	Typing    bool      `json:"typing"`
}

type Event struct {
	Type      string         `json:"type"`
	SessionID string         `json:"session_id,omitempty"`
	Payload   map[string]any `json:"payload,omitempty"`
	Source    string         `json:"source,omitempty"`
}
