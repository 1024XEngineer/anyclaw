package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/anyclaw/anyclaw/pkg/llm"
	"github.com/anyclaw/anyclaw/pkg/prompt"
)

// CompactionConfig controls context window management
type CompactionConfig struct {
	Enabled          bool    `json:"enabled"`
	MaxTokens        int     `json:"max_tokens"`
	SummaryThreshold int     `json:"summary_threshold"`
	SafetyMargin     float64 `json:"safety_margin"`
	KeepLastN        int     `json:"keep_last_n"`
	SummaryModel     string  `json:"summary_model,omitempty"`
}

// DefaultCompactionConfig returns sensible defaults
func DefaultCompactionConfig() CompactionConfig {
	return CompactionConfig{
		Enabled:          true,
		MaxTokens:        8000,
		SummaryThreshold: 6000,
		SafetyMargin:     0.8,
		KeepLastN:        4,
	}
}

// Compactor handles context window compaction
type Compactor struct {
	config     CompactionConfig
	llm        LLMCaller
	mu         sync.RWMutex
	summaries  []string
	totalSaved int
}

// NewCompactor creates a new compactor
func NewCompactor(config CompactionConfig, llm LLMCaller) *Compactor {
	return &Compactor{
		config: config,
		llm:    llm,
	}
}

// CompactHistory compacts history if it exceeds thresholds
func (c *Compactor) CompactHistory(ctx context.Context, history []prompt.Message) ([]prompt.Message, error) {
	if !c.config.Enabled || len(history) == 0 {
		return history, nil
	}

	// Estimate token count
	tokenCount := estimateTokens(history)

	// Check if compaction is needed
	threshold := int(float64(c.config.SummaryThreshold) * c.config.SafetyMargin)
	if tokenCount < threshold {
		return history, nil
	}

	// Keep last N messages untouched
	keepN := c.config.KeepLastN
	if keepN <= 0 {
		keepN = 4
	}
	if len(history) <= keepN {
		return history, nil
	}

	// Split: older messages to summarize, recent to keep
	toSummarize := history[:len(history)-keepN]
	toKeep := history[len(history)-keepN:]

	// Generate summary
	summary, err := c.summarizeMessages(ctx, toSummarize)
	if err != nil {
		// Fallback: just truncate
		return toKeep, nil
	}

	// Build compacted history
	compacted := []prompt.Message{
		{Role: "system", Content: fmt.Sprintf("[Conversation Summary]\n%s", summary)},
	}
	compacted = append(compacted, toKeep...)

	c.mu.Lock()
	c.summaries = append(c.summaries, summary)
	c.totalSaved += len(toSummarize)
	c.mu.Unlock()

	return compacted, nil
}

// summarizeMessages generates a summary of messages
func (c *Compactor) summarizeMessages(ctx context.Context, messages []prompt.Message) (string, error) {
	if c.llm == nil {
		return c.fallbackSummary(messages), nil
	}

	// Build summarization prompt
	var conversation strings.Builder
	for _, msg := range messages {
		role := msg.Role
		if role == "" {
			role = "user"
		}
		conversation.WriteString(fmt.Sprintf("%s: %s\n", role, msg.Content))
	}

	summarizePrompt := []llm.Message{
		{Role: "system", Content: "You are a conversation summarizer. Summarize the following conversation concisely, preserving key facts, decisions, and context. Be brief but comprehensive."},
		{Role: "user", Content: fmt.Sprintf("Summarize this conversation:\n\n%s", conversation.String())},
	}

	response, err := c.llm.Chat(ctx, summarizePrompt, nil)
	if err != nil {
		return c.fallbackSummary(messages), nil
	}

	return response.Content, nil
}

// fallbackSummary generates a simple summary without LLM
func (c *Compactor) fallbackSummary(messages []prompt.Message) string {
	var parts []string
	for _, msg := range messages {
		content := msg.Content
		if len(content) > 200 {
			content = content[:200] + "..."
		}
		parts = append(parts, fmt.Sprintf("[%s] %s", msg.Role, content))
	}
	return fmt.Sprintf("Previous conversation (%d messages): %s", len(messages), strings.Join(parts, "\n"))
}

// estimateTokens estimates token count from messages
func estimateTokens(messages []prompt.Message) int {
	total := 0
	for _, msg := range messages {
		// Rough estimate: 1 token per 4 characters
		total += len(msg.Content) / 4
		total += len(msg.Role) / 4
		total += 4 // overhead per message
	}
	return total
}

// GetStats returns compaction statistics
func (c *Compactor) GetStats() map[string]any {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return map[string]any{
		"summaries_count": len(c.summaries),
		"total_saved":     c.totalSaved,
	}
}

// SessionWriteLock provides concurrency control for session writes
type SessionWriteLock struct {
	mu       sync.Mutex
	sessions map[string]*sync.Mutex
}

// NewSessionWriteLock creates a new session write lock manager
func NewSessionWriteLock() *SessionWriteLock {
	return &SessionWriteLock{
		sessions: make(map[string]*sync.Mutex),
	}
}

// Lock acquires a write lock for a session
func (swl *SessionWriteLock) Lock(sessionID string) {
	swl.mu.Lock()
	lock, ok := swl.sessions[sessionID]
	if !ok {
		lock = &sync.Mutex{}
		swl.sessions[sessionID] = lock
	}
	swl.mu.Unlock()
	lock.Lock()
}

// Unlock releases a write lock for a session
func (swl *SessionWriteLock) Unlock(sessionID string) {
	swl.mu.Lock()
	lock, ok := swl.sessions[sessionID]
	swl.mu.Unlock()
	if ok {
		lock.Unlock()
	}
}

// ToolLoopDetector detects and prevents infinite tool call loops
type ToolLoopDetector struct {
	mu         sync.RWMutex
	maxRepeats int
	history    map[string][]string // sessionID -> recent tool calls
}

// NewToolLoopDetector creates a new loop detector
func NewToolLoopDetector(maxRepeats int) *ToolLoopDetector {
	if maxRepeats <= 0 {
		maxRepeats = 3
	}
	return &ToolLoopDetector{
		maxRepeats: maxRepeats,
		history:    make(map[string][]string),
	}
}

// Check detects if a tool call would create a loop
func (tld *ToolLoopDetector) Check(sessionID string, toolName string, argsHash string) bool {
	tld.mu.Lock()
	defer tld.mu.Unlock()

	key := toolName + ":" + argsHash
	recent := tld.history[sessionID]

	// Count occurrences
	count := 0
	for _, call := range recent {
		if call == key {
			count++
		}
	}

	// Add to history
	tld.history[sessionID] = append(recent, key)

	// Keep only last 20 entries
	if len(tld.history[sessionID]) > 20 {
		tld.history[sessionID] = tld.history[sessionID][len(tld.history[sessionID])-20:]
	}

	return count >= tld.maxRepeats
}

// Reset clears detection history for a session
func (tld *ToolLoopDetector) Reset(sessionID string) {
	tld.mu.Lock()
	defer tld.mu.Unlock()
	delete(tld.history, sessionID)
}
