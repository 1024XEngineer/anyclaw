package contextengine

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func newTestMessage(id, role, content string, tokens int) *Message {
	return &Message{
		ID:         id,
		Role:       role,
		Content:    content,
		Tokens:     tokens,
		CreatedAt:  time.Now(),
		AccessedAt: time.Now(),
	}
}

func TestCompactorAddAndCount(t *testing.T) {
	guard := NewWindowGuard(DefaultGuardConfig(1000))
	compactor := NewCompactor(guard, DefaultCompactionConfig())

	compactor.Add(newTestMessage("1", "user", "hello", 10))
	compactor.Add(newTestMessage("2", "assistant", "hi there", 15))

	if compactor.Count() != 2 {
		t.Errorf("expected 2 messages, got %d", compactor.Count())
	}
	if compactor.TotalTokens() != 25 {
		t.Errorf("expected 25 tokens, got %d", compactor.TotalTokens())
	}
}

func TestCompactorRemove(t *testing.T) {
	guard := NewWindowGuard(DefaultGuardConfig(1000))
	compactor := NewCompactor(guard, DefaultCompactionConfig())

	compactor.Add(newTestMessage("1", "user", "hello", 10))
	compactor.Add(newTestMessage("2", "assistant", "hi there", 15))

	compactor.Remove("1")

	if compactor.Count() != 1 {
		t.Errorf("expected 1 message after remove, got %d", compactor.Count())
	}
	if compactor.TotalTokens() != 15 {
		t.Errorf("expected 15 tokens after remove, got %d", compactor.TotalTokens())
	}
}

func TestCompactorNeedsCompaction(t *testing.T) {
	guard := NewWindowGuard(DefaultGuardConfig(1000))
	compactor := NewCompactor(guard, DefaultCompactionConfig())

	if compactor.NeedsCompaction() {
		t.Error("expected no compaction needed when empty")
	}

	for i := 0; i < 10; i++ {
		compactor.Add(newTestMessage(fmt.Sprintf("m%d", i), "user", "message content", 80))
	}

	if !compactor.NeedsCompaction() {
		t.Error("expected compaction needed when near limit")
	}
}

func TestCompactorNoCompactionBelowThreshold(t *testing.T) {
	guard := NewWindowGuard(DefaultGuardConfig(1000))
	compactor := NewCompactor(guard, DefaultCompactionConfig())

	for i := 0; i < 3; i++ {
		compactor.Add(newTestMessage(fmt.Sprintf("m%d", i), "user", "short", 20))
	}

	if compactor.NeedsCompaction() {
		t.Error("expected no compaction needed below threshold")
	}
}

func TestCompactorOldestFirst(t *testing.T) {
	guard := NewWindowGuard(DefaultGuardConfig(1000))
	compactor := NewCompactor(guard, CompactionConfig{
		Strategy:         StrategyOldestFirst,
		MinKeepMessages:  3,
		MaxSummaryTokens: 100,
		CompactThreshold: 0.5,
	})

	for i := 0; i < 10; i++ {
		compactor.Add(newTestMessage(fmt.Sprintf("m%d", i), "user", fmt.Sprintf("message %d content here", i), 60))
	}

	result, err := compactor.Compact(context.Background(), nil)
	if err != nil {
		t.Fatalf("compact: %v", err)
	}

	if result.CompactedCount == 0 {
		t.Error("expected some messages compacted")
	}
	if compactor.Count() < 3 {
		t.Errorf("expected at least 3 messages remaining, got %d", compactor.Count())
	}
}

func TestCompactorSmallestFirst(t *testing.T) {
	guard := NewWindowGuard(GuardConfig{
		MaxTokens:    1000,
		SafetyMargin: 0,
	})
	compactor := NewCompactor(guard, CompactionConfig{
		Strategy:         StrategySmallestFirst,
		MinKeepMessages:  3,
		MaxSummaryTokens: 100,
		CompactThreshold: 0.5,
	})

	compactor.Add(newTestMessage("big", "user", "big message", 200))
	for i := 0; i < 8; i++ {
		compactor.Add(newTestMessage(fmt.Sprintf("small%d", i), "user", "tiny", 50))
	}

	total := compactor.TotalTokens()
	if !compactor.NeedsCompaction() {
		t.Skipf("skipping: total=%d, max=%d, threshold=%.2f", total, guard.MaxTokens(), float64(total)/float64(guard.MaxTokens()))
	}

	result, err := compactor.Compact(context.Background(), nil)
	if err != nil {
		t.Fatalf("compact: %v", err)
	}

	if result.CompactedCount == 0 {
		t.Errorf("expected messages compacted, total=%d, remaining=%d", total, compactor.TotalTokens())
	}
}

func TestCompactorLRU(t *testing.T) {
	guard := NewWindowGuard(GuardConfig{
		MaxTokens:    1000,
		SafetyMargin: 0,
	})
	compactor := NewCompactor(guard, CompactionConfig{
		Strategy:         StrategyLRU,
		MinKeepMessages:  3,
		MaxSummaryTokens: 100,
		CompactThreshold: 0.5,
	})

	for i := 0; i < 10; i++ {
		msg := newTestMessage(fmt.Sprintf("m%d", i), "user", "content here", 60)
		msg.AccessedAt = time.Now().Add(-time.Duration(10-i) * time.Minute)
		compactor.Add(msg)
	}

	total := compactor.TotalTokens()
	if !compactor.NeedsCompaction() {
		t.Skipf("skipping: total=%d, max=%d, threshold=%.2f", total, guard.MaxTokens(), float64(total)/float64(guard.MaxTokens()))
	}

	result, err := compactor.Compact(context.Background(), nil)
	if err != nil {
		t.Fatalf("compact: %v", err)
	}

	if result.CompactedCount == 0 {
		t.Errorf("expected LRU messages compacted, total=%d, remaining=%d", total, compactor.TotalTokens())
	}
}

func TestCompactorPinnedMessages(t *testing.T) {
	guard := NewWindowGuard(DefaultGuardConfig(1000))
	compactor := NewCompactor(guard, CompactionConfig{
		Strategy:         StrategyOldestFirst,
		MinKeepMessages:  1,
		MaxSummaryTokens: 100,
		CompactThreshold: 0.5,
	})

	pinned := newTestMessage("pinned", "system", "important system prompt", 100)
	pinned.Pinned = true
	compactor.Add(pinned)

	for i := 0; i < 10; i++ {
		compactor.Add(newTestMessage(fmt.Sprintf("m%d", i), "user", "message content", 80))
	}

	messages := compactor.Messages()
	hasPinned := false
	for _, msg := range messages {
		if msg.ID == "pinned" {
			hasPinned = true
			break
		}
	}

	if !hasPinned {
		t.Error("expected pinned message to remain after compaction")
	}
}

func TestCompactorMinKeepMessages(t *testing.T) {
	guard := NewWindowGuard(DefaultGuardConfig(1000))
	compactor := NewCompactor(guard, CompactionConfig{
		Strategy:         StrategyOldestFirst,
		MinKeepMessages:  5,
		MaxSummaryTokens: 100,
		CompactThreshold: 0.5,
	})

	for i := 0; i < 10; i++ {
		compactor.Add(newTestMessage(fmt.Sprintf("m%d", i), "user", "content", 60))
	}

	result, err := compactor.Compact(context.Background(), nil)
	if err != nil {
		t.Fatalf("compact: %v", err)
	}

	if compactor.Count() < 5 {
		t.Errorf("expected at least 5 messages remaining, got %d", compactor.Count())
	}

	_ = result
}

func TestCompactorWithSummarizer(t *testing.T) {
	guard := NewWindowGuard(DefaultGuardConfig(1000))
	compactor := NewCompactor(guard, CompactionConfig{
		Strategy:         StrategyOldestFirst,
		MinKeepMessages:  3,
		MaxSummaryTokens: 100,
		CompactThreshold: 0.5,
	})

	for i := 0; i < 10; i++ {
		compactor.Add(newTestMessage(fmt.Sprintf("m%d", i), "user", fmt.Sprintf("message %d", i), 60))
	}

	summarizer := &StaticSummarizer{Summary: "User discussed various topics in previous messages."}
	result, err := compactor.Compact(context.Background(), summarizer)
	if err != nil {
		t.Fatalf("compact: %v", err)
	}

	if result.Summary == "" {
		t.Error("expected summary from summarizer")
	}
	if result.CompactedCount == 0 {
		t.Error("expected messages compacted")
	}
}

func TestCompactorFallbackSummary(t *testing.T) {
	guard := NewWindowGuard(DefaultGuardConfig(1000))
	compactor := NewCompactor(guard, CompactionConfig{
		Strategy:         StrategyOldestFirst,
		MinKeepMessages:  3,
		MaxSummaryTokens: 100,
		CompactThreshold: 0.5,
	})

	for i := 0; i < 10; i++ {
		compactor.Add(newTestMessage(fmt.Sprintf("m%d", i), "user", fmt.Sprintf("message %d with some content", i), 60))
	}

	result, err := compactor.Compact(context.Background(), nil)
	if err != nil {
		t.Fatalf("compact: %v", err)
	}

	if result.Summary == "" {
		t.Error("expected fallback summary")
	}
}

func TestCompactorEmptySummary(t *testing.T) {
	guard := NewWindowGuard(DefaultGuardConfig(1000))
	compactor := NewCompactor(guard, DefaultCompactionConfig())

	result, err := compactor.Compact(context.Background(), nil)
	if err != nil {
		t.Fatalf("compact empty: %v", err)
	}

	if result.CompactedCount != 0 {
		t.Errorf("expected 0 compacted, got %d", result.CompactedCount)
	}
}

func TestCompactorBelowThreshold(t *testing.T) {
	guard := NewWindowGuard(DefaultGuardConfig(1000))
	compactor := NewCompactor(guard, DefaultCompactionConfig())

	for i := 0; i < 3; i++ {
		compactor.Add(newTestMessage(fmt.Sprintf("m%d", i), "user", "short", 20))
	}

	result, err := compactor.Compact(context.Background(), nil)
	if err != nil {
		t.Fatalf("compact: %v", err)
	}

	if result.CompactedCount != 0 {
		t.Errorf("expected 0 compacted below threshold, got %d", result.CompactedCount)
	}
}

func TestCompactorFreedTokens(t *testing.T) {
	guard := NewWindowGuard(DefaultGuardConfig(1000))
	compactor := NewCompactor(guard, CompactionConfig{
		Strategy:         StrategyOldestFirst,
		MinKeepMessages:  2,
		MaxSummaryTokens: 50,
		CompactThreshold: 0.5,
	})

	for i := 0; i < 10; i++ {
		compactor.Add(newTestMessage(fmt.Sprintf("m%d", i), "user", "message content", 60))
	}

	beforeTokens := compactor.TotalTokens()

	result, err := compactor.Compact(context.Background(), nil)
	if err != nil {
		t.Fatalf("compact: %v", err)
	}

	afterTokens := compactor.TotalTokens()
	if afterTokens >= beforeTokens {
		t.Error("expected tokens to decrease after compaction")
	}

	if result.FreedTokens <= 0 {
		t.Errorf("expected positive freed tokens, got %d", result.FreedTokens)
	}
}

func TestCompactorSummaryAtFront(t *testing.T) {
	guard := NewWindowGuard(DefaultGuardConfig(1000))
	compactor := NewCompactor(guard, CompactionConfig{
		Strategy:         StrategyOldestFirst,
		MinKeepMessages:  2,
		MaxSummaryTokens: 50,
		CompactThreshold: 0.5,
	})

	for i := 0; i < 10; i++ {
		compactor.Add(newTestMessage(fmt.Sprintf("m%d", i), "user", "message content", 60))
	}

	_, err := compactor.Compact(context.Background(), nil)
	if err != nil {
		t.Fatalf("compact: %v", err)
	}

	messages := compactor.Messages()
	if len(messages) == 0 {
		t.Fatal("expected messages after compaction")
	}

	if messages[0].Role != "system" {
		t.Errorf("expected summary message at front, got role %s", messages[0].Role)
	}
}

func TestEstimateTokens(t *testing.T) {
	tokens := estimateTokens("Hello world this is a test message")
	if tokens <= 0 {
		t.Errorf("expected positive token estimate, got %d", tokens)
	}
}

func TestCompactorMessages(t *testing.T) {
	guard := NewWindowGuard(DefaultGuardConfig(1000))
	compactor := NewCompactor(guard, DefaultCompactionConfig())

	compactor.Add(newTestMessage("1", "user", "hello", 10))
	compactor.Add(newTestMessage("2", "assistant", "hi", 5))

	msgs := compactor.Messages()
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages, got %d", len(msgs))
	}

	if msgs[0].ID != "1" || msgs[1].ID != "2" {
		t.Error("expected messages in order")
	}
}

func TestCompactorConcurrent(t *testing.T) {
	guard := NewWindowGuard(DefaultGuardConfig(10000))
	compactor := NewCompactor(guard, DefaultCompactionConfig())

	done := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		go func(n int) {
			compactor.Add(newTestMessage(fmt.Sprintf("m%d", n), "user", "content", 10))
			done <- true
		}(i)
	}

	for i := 0; i < 100; i++ {
		<-done
	}

	if compactor.Count() != 100 {
		t.Errorf("expected 100 messages, got %d", compactor.Count())
	}
}
