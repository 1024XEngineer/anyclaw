package agent

import (
	"anyclaw/internal/session"
	"anyclaw/pkg/sdk"
)

// BuildPrompt converts session history plus the current user message into provider input.
func BuildPrompt(history []session.ChatMessage, current session.ChatMessage) []sdk.Message {
	messages := make([]sdk.Message, 0, len(history)+1)
	for _, item := range history {
		messages = append(messages, sdk.Message{Role: item.Role, Content: item.Text})
	}
	messages = append(messages, sdk.Message{Role: current.Role, Content: current.Text})
	return messages
}
