package contextengine

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type CompactionStrategy string

const (
	StrategyOldestFirst   CompactionStrategy = "oldest_first"
	StrategySmallestFirst CompactionStrategy = "smallest_first"
	StrategyLRU           CompactionStrategy = "lru"
)

type CompactionConfig struct {
	Strategy         CompactionStrategy
	MinKeepMessages  int
	MaxSummaryTokens int
	CompactThreshold float64
	TargetTokenRatio float64
	SummaryPrefix    string
}

func DefaultCompactionConfig() CompactionConfig {
	return CompactionConfig{
		Strategy:         StrategyOldestFirst,
		MinKeepMessages:  3,
		MaxSummaryTokens: 200,
		CompactThreshold: 0.8,
		TargetTokenRatio: 0.5,
		SummaryPrefix:    "[Summary of previous conversation]",
	}
}

type Message struct {
	ID         string
	Role       string
	Content    string
	Tokens     int
	Pinned     bool
	CreatedAt  time.Time
	AccessedAt time.Time
}

func (m *Message) TokenCount() int {
	return m.Tokens
}

func (m *Message) IsPinned() bool {
	return m.Pinned
}

type SummaryResult struct {
	Summary         string
	SummaryTokens   int
	CompactedCount  int
	FreedTokens     int
	RemainingTokens int
}

type Compactor struct {
	mu       sync.Mutex
	messages []*Message
	guard    *WindowGuard
	config   CompactionConfig
}

func NewCompactor(guard *WindowGuard, cfg CompactionConfig) *Compactor {
	if cfg.MinKeepMessages < 1 {
		cfg.MinKeepMessages = 3
	}
	if cfg.MaxSummaryTokens <= 0 {
		cfg.MaxSummaryTokens = 200
	}
	if cfg.CompactThreshold <= 0 || cfg.CompactThreshold > 1 {
		cfg.CompactThreshold = 0.8
	}
	if cfg.TargetTokenRatio <= 0 || cfg.TargetTokenRatio > 1 {
		cfg.TargetTokenRatio = 0.5
	}
	if cfg.TargetTokenRatio > cfg.CompactThreshold {
		cfg.TargetTokenRatio = cfg.CompactThreshold
	}

	return &Compactor{
		messages: make([]*Message, 0),
		guard:    guard,
		config:   cfg,
	}
}

func (c *Compactor) Add(msg *Message) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if msg.AccessedAt.IsZero() {
		msg.AccessedAt = time.Now()
	}
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now()
	}

	c.messages = append(c.messages, msg)
	_ = c.guard.track(msg.Tokens)
}

func (c *Compactor) Remove(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i, msg := range c.messages {
		if msg.ID == id {
			c.guard.Remove(msg.Tokens)
			c.messages = append(c.messages[:i], c.messages[i+1:]...)
			return
		}
	}
}

func (c *Compactor) Messages() []*Message {
	c.mu.Lock()
	defer c.mu.Unlock()

	result := make([]*Message, len(c.messages))
	copy(result, c.messages)
	return result
}

func (c *Compactor) Count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.messages)
}

func (c *Compactor) TotalTokens() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	total := 0
	for _, msg := range c.messages {
		total += msg.Tokens
	}
	return total
}

func (c *Compactor) NeedsCompaction() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	_, max, _ := c.guard.Status()
	if max == 0 {
		return false
	}

	total := 0
	for _, msg := range c.messages {
		total += msg.Tokens
	}

	return float64(total)/float64(max) >= c.config.CompactThreshold
}

func (c *Compactor) Compact(ctx context.Context, summarizer Summarizer) (*SummaryResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.messages) <= c.config.MinKeepMessages {
		return &SummaryResult{}, nil
	}

	_, max, _ := c.guard.Status()
	currentTokens := 0
	for _, msg := range c.messages {
		currentTokens += msg.Tokens
	}

	if float64(currentTokens)/float64(max) < c.config.CompactThreshold {
		return &SummaryResult{}, nil
	}

	targetTokens := int(float64(max) * c.config.TargetTokenRatio)
	needToRemove := currentTokens - targetTokens
	if needToRemove <= 0 {
		return &SummaryResult{}, nil
	}

	candidates := c.selectCandidates()
	if len(candidates) == 0 {
		return &SummaryResult{}, nil
	}

	var toCompact []*Message
	removedTokens := 0
	for _, msg := range candidates {
		if len(c.messages)-len(toCompact) <= c.config.MinKeepMessages {
			break
		}
		toCompact = append(toCompact, msg)
		removedTokens += msg.Tokens
		if removedTokens >= needToRemove {
			break
		}
	}

	if len(toCompact) == 0 {
		return &SummaryResult{}, nil
	}

	summaryText := c.generateSummary(toCompact, summarizer, ctx)
	summaryTokens := estimateTokens(summaryText)
	if summaryTokens > c.config.MaxSummaryTokens {
		summaryTokens = c.config.MaxSummaryTokens
	}

	compactedIDs := make(map[string]bool)
	for _, msg := range toCompact {
		compactedIDs[msg.ID] = true
	}

	remaining := make([]*Message, 0, len(c.messages))
	for _, msg := range c.messages {
		if !compactedIDs[msg.ID] {
			remaining = append(remaining, msg)
		}
	}

	if summaryText != "" {
		summaryMsg := &Message{
			ID:         fmt.Sprintf("summary_%d", time.Now().UnixNano()),
			Role:       "system",
			Content:    summaryText,
			Tokens:     summaryTokens,
			Pinned:     true,
			CreatedAt:  time.Now(),
			AccessedAt: time.Now(),
		}
		remaining = append([]*Message{summaryMsg}, remaining...)
	}

	freedTokens := removedTokens - summaryTokens
	if freedTokens < 0 {
		freedTokens = 0
	}

	c.messages = remaining
	c.guard.Reset()
	for _, msg := range c.messages {
		_ = c.guard.track(msg.Tokens)
	}

	remainingTokens := 0
	for _, msg := range c.messages {
		remainingTokens += msg.Tokens
	}

	return &SummaryResult{
		Summary:         summaryText,
		SummaryTokens:   summaryTokens,
		CompactedCount:  len(toCompact),
		FreedTokens:     freedTokens,
		RemainingTokens: remainingTokens,
	}, nil
}

func (c *Compactor) selectCandidates() []*Message {
	switch c.config.Strategy {
	case StrategyOldestFirst:
		return c.selectOldestFirst()
	case StrategySmallestFirst:
		return c.selectSmallestFirst()
	case StrategyLRU:
		return c.selectLRU()
	default:
		return c.selectOldestFirst()
	}
}

func (c *Compactor) selectOldestFirst() []*Message {
	candidates := make([]*Message, 0, len(c.messages))
	for _, msg := range c.messages {
		if !msg.Pinned {
			candidates = append(candidates, msg)
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].CreatedAt.Before(candidates[j].CreatedAt)
	})

	return candidates
}

func (c *Compactor) selectSmallestFirst() []*Message {
	candidates := make([]*Message, 0, len(c.messages))
	for _, msg := range c.messages {
		if !msg.Pinned {
			candidates = append(candidates, msg)
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Tokens < candidates[j].Tokens
	})

	return candidates
}

func (c *Compactor) selectLRU() []*Message {
	candidates := make([]*Message, 0, len(c.messages))
	for _, msg := range c.messages {
		if !msg.Pinned {
			candidates = append(candidates, msg)
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].AccessedAt.Before(candidates[j].AccessedAt)
	})

	return candidates
}

func (c *Compactor) generateSummary(messages []*Message, summarizer Summarizer, ctx context.Context) string {
	if len(messages) == 0 {
		return ""
	}

	if summarizer != nil {
		contents := make([]string, len(messages))
		for i, msg := range messages {
			contents[i] = fmt.Sprintf("%s: %s", msg.Role, msg.Content)
		}

		summary, err := summarizer.Summarize(ctx, strings.Join(contents, "\n"))
		if err == nil && summary != "" {
			return fmt.Sprintf("%s %s", c.config.SummaryPrefix, summary)
		}
	}

	return c.fallbackSummary(messages)
}

func (c *Compactor) fallbackSummary(messages []*Message) string {
	if len(messages) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(c.config.SummaryPrefix)
	sb.WriteString(fmt.Sprintf(" %d messages were summarized: ", len(messages)))

	truncated := false
	totalLen := 0
	for _, msg := range messages {
		content := msg.Content
		if len(content) > 100 {
			content = content[:97] + "..."
			truncated = true
		}
		sb.WriteString(fmt.Sprintf("[%s] %s; ", msg.Role, content))
		totalLen += len(content)
		if totalLen > c.config.MaxSummaryTokens*4 {
			break
		}
	}

	if truncated {
		sb.WriteString("(truncated)")
	}

	return sb.String()
}

type Summarizer interface {
	Summarize(ctx context.Context, text string) (string, error)
}

type StaticSummarizer struct {
	Summary string
}

func (s *StaticSummarizer) Summarize(ctx context.Context, text string) (string, error) {
	return s.Summary, nil
}

func estimateTokens(text string) int {
	return len([]rune(text)) / 4
}
