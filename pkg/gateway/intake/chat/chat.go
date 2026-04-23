package chat

import "time"

// Message is the protocol-neutral chat message shape used by gateway intake.
type Message struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	AgentName string    `json:"agent_name,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// Session is the lightweight chat session snapshot accepted by gateway intake.
type Session struct {
	ID        string    `json:"id"`
	AgentName string    `json:"agent_name"`
	Title     string    `json:"title"`
	Messages  []Message `json:"messages"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ChatRequest is the stable /v2/chat request contract consumed by command intake.
type ChatRequest struct {
	AgentName string `json:"agent_name"`
	SessionID string `json:"session_id,omitempty"`
	Message   string `json:"message"`
}

// ChatResponse is the stable /v2/chat response contract returned by gateway intake.
type ChatResponse struct {
	SessionID string    `json:"session_id"`
	AgentName string    `json:"agent_name"`
	Message   Message   `json:"message"`
	History   []Message `json:"history,omitempty"`
}
