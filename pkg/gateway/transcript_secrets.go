package gateway

import (
	"fmt"
	"strings"

	"github.com/anyclaw/anyclaw/pkg/llm"
	"github.com/anyclaw/anyclaw/pkg/prompt"
)

// TranscriptRepair repairs corrupted or malformed session transcripts
type TranscriptRepair struct {
	strictMode bool
}

// NewTranscriptRepair creates a new transcript repair handler
func NewTranscriptRepair(strictMode bool) *TranscriptRepair {
	return &TranscriptRepair{strictMode: strictMode}
}

// RepairMessage repairs a single message
func (tr *TranscriptRepair) RepairMessage(msg prompt.Message) prompt.Message {
	// Ensure role is valid
	if msg.Role == "" {
		msg.Role = "user"
	}
	validRoles := map[string]bool{"system": true, "user": true, "assistant": true, "tool": true}
	if !validRoles[msg.Role] {
		msg.Role = "user"
	}

	// Clean content
	msg.Content = strings.TrimSpace(msg.Content)

	return msg
}

// RepairHistory repairs a conversation history
func (tr *TranscriptRepair) RepairHistory(history []prompt.Message) []prompt.Message {
	if len(history) == 0 {
		return history
	}

	var repaired []prompt.Message

	for i, msg := range history {
		repaired = append(repaired, tr.RepairMessage(msg))

		// Ensure alternating user/assistant pattern (except system messages)
		if i > 0 && msg.Role != "system" {
			prevRole := repaired[i-1].Role
			if prevRole != "system" && prevRole == msg.Role {
				// Insert missing role
				if msg.Role == "user" {
					repaired = append(repaired, prompt.Message{Role: "assistant", Content: "[continuation]"})
				} else {
					repaired = append(repaired, prompt.Message{Role: "user", Content: "[continuation]"})
				}
			}
		}
	}

	return repaired
}

// RepairToolCalls repairs tool_use/tool_result pairing
func (tr *TranscriptRepair) RepairToolCalls(messages []llm.Message) []llm.Message {
	if len(messages) == 0 {
		return messages
	}

	var repaired []llm.Message
	var pendingToolCalls []llm.ToolCall

	for _, msg := range messages {
		// If assistant has tool calls, store them
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			pendingToolCalls = msg.ToolCalls
			repaired = append(repaired, msg)
			continue
		}

		// If we have a tool result, match it with pending tool calls
		if msg.Role == "tool" && msg.ToolCallID != "" {
			matched := false
			for i, tc := range pendingToolCalls {
				if tc.ID == msg.ToolCallID {
					matched = true
					pendingToolCalls = append(pendingToolCalls[:i], pendingToolCalls[i+1:]...)
					break
				}
			}

			if !matched && tr.strictMode {
				// Skip orphaned tool results in strict mode
				continue
			}

			repaired = append(repaired, msg)
			continue
		}

		// If we have pending tool calls but got a user message, add placeholder tool results
		if msg.Role == "user" && len(pendingToolCalls) > 0 {
			for _, tc := range pendingToolCalls {
				repaired = append(repaired, llm.Message{
					Role:       "tool",
					ToolCallID: tc.ID,
					Content:    "[tool call not completed]",
				})
			}
			pendingToolCalls = nil
		}

		repaired = append(repaired, msg)
	}

	// Handle any remaining pending tool calls
	if len(pendingToolCalls) > 0 {
		for _, tc := range pendingToolCalls {
			repaired = append(repaired, llm.Message{
				Role:       "tool",
				ToolCallID: tc.ID,
				Content:    "[tool call not completed]",
			})
		}
	}

	return repaired
}

// StripDetails strips detailed tool results for context compaction
func (tr *TranscriptRepair) StripDetails(messages []llm.Message, maxContentLen int) []llm.Message {
	var stripped []llm.Message

	for _, msg := range messages {
		if msg.Role == "tool" && len(msg.Content) > maxContentLen {
			msg.Content = msg.Content[:maxContentLen] + "\n[...truncated]"
		}
		stripped = append(stripped, msg)
	}

	return stripped
}

// Validate validates a transcript for consistency
func (tr *TranscriptRepair) Validate(messages []llm.Message) []string {
	var issues []string

	toolCallIDs := make(map[string]bool)
	toolResultIDs := make(map[string]bool)

	for i, msg := range messages {
		// Track tool calls
		if msg.Role == "assistant" {
			for _, tc := range msg.ToolCalls {
				toolCallIDs[tc.ID] = true
			}
		}

		// Track tool results
		if msg.Role == "tool" {
			if msg.ToolCallID == "" {
				issues = append(issues, fmt.Sprintf("Message %d: tool result missing ToolCallID", i))
			} else {
				toolResultIDs[msg.ToolCallID] = true
			}
		}

		// Check for empty content
		if msg.Content == "" && msg.Role != "assistant" {
			issues = append(issues, fmt.Sprintf("Message %d: empty content for role %s", i, msg.Role))
		}
	}

	// Check for unmatched tool calls
	for id := range toolCallIDs {
		if !toolResultIDs[id] {
			issues = append(issues, fmt.Sprintf("Unmatched tool call: %s", id))
		}
	}

	// Check for orphaned tool results
	for id := range toolResultIDs {
		if !toolCallIDs[id] {
			issues = append(issues, fmt.Sprintf("Orphaned tool result: %s", id))
		}
	}

	return issues
}

// SecretsManager manages secrets and sensitive configuration
type SecretsManager struct {
	secrets map[string]string
	envVars map[string]string
}

// NewSecretsManager creates a new secrets manager
func NewSecretsManager() *SecretsManager {
	return &SecretsManager{
		secrets: make(map[string]string),
		envVars: make(map[string]string),
	}
}

// SetSecret sets a secret value
func (sm *SecretsManager) SetSecret(key string, value string) {
	sm.secrets[key] = value
}

// GetSecret gets a secret value
func (sm *SecretsManager) GetSecret(key string) (string, bool) {
	val, ok := sm.secrets[key]
	return val, ok
}

// SetEnvVar sets an environment variable
func (sm *SecretsManager) SetEnvVar(key string, value string) {
	sm.envVars[key] = value
}

// ResolveValue resolves a value that may reference secrets or env vars
// Supports: ${SECRET:key}, ${ENV:var}, ${FILE:path}
func (sm *SecretsManager) ResolveValue(value string) string {
	if !strings.Contains(value, "${") {
		return value
	}

	// Resolve secret references
	for strings.Contains(value, "${SECRET:") {
		start := strings.Index(value, "${SECRET:")
		end := strings.Index(value[start:], "}")
		if end == -1 {
			break
		}
		end += start

		key := value[start+9 : end]
		if secret, ok := sm.secrets[key]; ok {
			value = value[:start] + secret + value[end+1:]
		} else {
			value = value[:start] + value[end+1:]
		}
	}

	// Resolve env var references
	for strings.Contains(value, "${ENV:") {
		start := strings.Index(value, "${ENV:")
		end := strings.Index(value[start:], "}")
		if end == -1 {
			break
		}
		end += start

		key := value[start+6 : end]
		if envVal, ok := sm.envVars[key]; ok {
			value = value[:start] + envVal + value[end+1:]
		} else {
			value = value[:start] + value[end+1:]
		}
	}

	return value
}

// ListSecrets returns a list of secret keys (not values)
func (sm *SecretsManager) ListSecrets() []string {
	var keys []string
	for k := range sm.secrets {
		keys = append(keys, k)
	}
	return keys
}

// DeleteSecret deletes a secret
func (sm *SecretsManager) DeleteSecret(key string) {
	delete(sm.secrets, key)
}

// Redact redacts secrets from text
func (sm *SecretsManager) Redact(text string) string {
	for key, value := range sm.secrets {
		if len(value) > 4 {
			text = strings.ReplaceAll(text, value, "[REDACTED:"+key+"]")
		}
	}
	return text
}
