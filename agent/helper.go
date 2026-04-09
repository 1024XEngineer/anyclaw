package agent

import (
	"time"

	"anyclaw/internal/logger"
	"anyclaw/session"
	"go.uber.org/zap"
)

// AgentHelper provides helper functions for agent message processing
type AgentHelper struct {
	sessionMgr *session.Manager
}

// NewAgentHelper creates a new agent helper
func NewAgentHelper(sessionMgr *session.Manager) *AgentHelper {
	return &AgentHelper{
		sessionMgr: sessionMgr,
	}
}

// UpdateSessionWithOptions updates the session with new messages with options
type UpdateSessionOptions struct {
	SaveImmediately bool // Save to disk after updating
}

// UpdateSession updates the session with new messages
// This function is shared between Agent and AgentManager to avoid code duplication
func (h *AgentHelper) UpdateSession(sess *session.Session, messages []AgentMessage, opts *UpdateSessionOptions) error {
	if opts == nil {
		opts = &UpdateSessionOptions{SaveImmediately: true}
	}

	for _, msg := range messages {
		sessMsg := session.Message{
			Role:      string(msg.Role),
			Content:   extractTextContent(msg),
			Timestamp: time.Unix(extractTimestamp(msg)/1000, 0),
		}

		// Handle tool calls
		if msg.Role == RoleAssistant {
			for _, block := range msg.Content {
				if tc, ok := block.(ToolCallContent); ok {
					sessMsg.ToolCalls = append(sessMsg.ToolCalls, session.ToolCall{
						ID:     tc.ID,
						Name:   tc.Name,
						Params: tc.Arguments,
					})
				}
			}
		}

		// Handle tool results
		if msg.Role == RoleToolResult {
			if id, ok := msg.Metadata["tool_call_id"].(string); ok {
				sessMsg.ToolCallID = id
			}
			// Preserve tool_name in metadata for validation
			if toolName, ok := msg.Metadata["tool_name"].(string); ok {
				if sessMsg.Metadata == nil {
					sessMsg.Metadata = make(map[string]interface{})
				}
				sessMsg.Metadata["tool_name"] = toolName
			}
		}

		sess.AddMessage(sessMsg)
	}

	if opts.SaveImmediately {
		if err := h.sessionMgr.Save(sess); err != nil {
			logger.Error("Failed to save session", zap.Error(err))
			return err
		}
	}

	return nil
}
