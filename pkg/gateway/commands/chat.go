package commands

import (
	"fmt"
	"strings"

	chatintake "github.com/1024XEngineer/anyclaw/pkg/gateway/intake/chat"
)

// ChatSendRequest is the normalized command-intake contract for chat.send style requests.
type ChatSendRequest struct {
	Message     string
	SessionID   string
	Title       string
	Agent       string
	Assistant   string
	OrgID       string
	ProjectID   string
	WorkspaceID string
}

// NewChatSendCommandRequest converts a chat.send style payload into the command-intake contract.
func NewChatSendCommandRequest(req ChatSendRequest) Request {
	params := map[string]string{
		"message":       req.Message,
		"session_id":    req.SessionID,
		"title":         req.Title,
		"agent":         req.Agent,
		"assistant":     req.Assistant,
		"org":           req.OrgID,
		"project":       req.ProjectID,
		"workspace":     req.WorkspaceID,
		"has_session":   fmt.Sprintf("%t", strings.TrimSpace(req.SessionID) != ""),
		"has_message":   fmt.Sprintf("%t", strings.TrimSpace(req.Message) != ""),
		"has_agent":     fmt.Sprintf("%t", strings.TrimSpace(req.Agent) != ""),
		"has_assistant": fmt.Sprintf("%t", strings.TrimSpace(req.Assistant) != ""),
	}
	return Request{
		Method:     "chat.send",
		ResourceID: strings.TrimSpace(req.SessionID),
		Params:     params,
	}
}

// ValidateChatSend validates the minimum required fields for chat.send style commands.
func ValidateChatSend(req ChatSendRequest) error {
	if strings.TrimSpace(req.Message) == "" {
		return fmt.Errorf("message is required")
	}
	return nil
}

// ValidateV2Chat validates the minimum required fields for /v2/chat.
func ValidateV2Chat(req chatintake.ChatRequest) error {
	if strings.TrimSpace(req.AgentName) == "" {
		return fmt.Errorf("agent_name is required")
	}
	if strings.TrimSpace(req.Message) == "" {
		return fmt.Errorf("message is required")
	}
	return nil
}

// NewV2ChatCommandRequest converts the /v2/chat payload into the command-intake contract.
func NewV2ChatCommandRequest(req chatintake.ChatRequest) Request {
	return Request{
		Method:     "chat.v2.send",
		ResourceID: strings.TrimSpace(req.SessionID),
		Params: map[string]string{
			"agent_name":  strings.TrimSpace(req.AgentName),
			"session_id":  strings.TrimSpace(req.SessionID),
			"message":     strings.TrimSpace(req.Message),
			"has_session": fmt.Sprintf("%t", strings.TrimSpace(req.SessionID) != ""),
		},
	}
}
