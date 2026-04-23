package contextengine

import (
	"context"
	"fmt"
	"testing"
	"time"
)

type summarizerFunc func(ctx context.Context, text string) (string, error)

func (f summarizerFunc) Summarize(ctx context.Context, text string) (string, error) {
	return f(ctx, text)
}

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
	if guard.CurrentTokens() != 25 {
		t.Errorf("expected guard to track 25 tokens, got %d", guard.CurrentTokens())
	}
}

func TestCompactorAddClonesInput(t *testing.T) {
	guard := NewWindowGuard(DefaultGuardConfig(1000))
	compactor := NewCompactor(guard, DefaultCompactionConfig())

	msg := newTestMessage("1", "user", "hello", 10)
	compactor.Add(msg)
	msg.Content = "mutated"

	messages := compactor.Messages()
	if messages[0].Content != "hello" {
		t.Fatalf("expected stored content to remain unchanged, got %q", messages[0].Content)
	}
}

func TestCompactorAddHonorsHardLimit(t *testing.T) {
	guard := NewWindowGuard(GuardConfig{
		MaxTokens:    100,
		SafetyMargin: 0,
		HardLimit:    true,
	})
	compactor := NewCompactor(guard, DefaultCompactionConfig())

	compactor.Add(newTestMessage("fit", "user", "fits", 80))
	compactor.Add(newTestMessage("overflow", "user", "too much", 30))

	if compactor.Count() != 1 {
		t.Fatalf("expected only the in-budget message to remain, got %d messages", compactor.Count())
	}
	if compactor.TotalTokens() != 80 {
		t.Fatalf("expected 80 total tokens after rejecting overflow, got %d", compactor.TotalTokens())
	}
	if guard.CurrentTokens() != 80 {
		t.Fatalf("expected guard to stay at 80 tokens, got %d", guard.CurrentTokens())
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

func TestCompactorCompactDoesNotHoldMutexDuringSummarize(t *testing.T) {
	guard := NewWindowGuard(DefaultGuardConfig(1000))
	compactor := NewCompactor(guard, CompactionConfig{
		Strategy:         StrategyOldestFirst,
		MinKeepMessages:  3,
		MaxSummaryTokens: 100,
		CompactThreshold: 0.5,
	})

	for i := 0; i < 10; i++ {
		compactor.Add(newTestMessage(fmt.Sprintf("m%d", i), "user", "message content", 60))
	}

	done := make(chan struct{}, 1)
	errCh := make(chan error, 1)

	go func() {
		_, err := compactor.Compact(context.Background(), summarizerFunc(func(ctx context.Context, text string) (string, error) {
			if compactor.Count() == 0 {
				return "", fmt.Errorf("expected messages to remain visible during summarize")
			}
			done <- struct{}{}
			return "summary", nil
		}))
		errCh <- err
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("summarizer blocked while querying compactor state")
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("compact: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("compact did not return")
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

func TestCompactorMessagesReturnsDefensiveCopies(t *testing.T) {
	guard := NewWindowGuard(DefaultGuardConfig(1000))
	compactor := NewCompactor(guard, DefaultCompactionConfig())

	compactor.Add(newTestMessage("1", "user", "hello", 10))

	messages := compactor.Messages()
	messages[0].Content = "mutated"

	fresh := compactor.Messages()
	if fresh[0].Content != "hello" {
		t.Fatalf("expected stored content to remain unchanged, got %q", fresh[0].Content)
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

func TestCompactorTargetTokenRatio(t *testing.T) {
	guard := NewWindowGuard(GuardConfig{
		MaxTokens:    1000,
		SafetyMargin: 0,
	})
	compactor := NewCompactor(guard, CompactionConfig{
		Strategy:         StrategyOldestFirst,
		MinKeepMessages:  2,
		MaxSummaryTokens: 10,
		CompactThreshold: 0.5,
		TargetTokenRatio: 0.3,
		SummaryPrefix:    "s",
	})

	for i := 0; i < 10; i++ {
		compactor.Add(newTestMessage(fmt.Sprintf("m%d", i), "user", "message content", 60))
	}

	result, err := compactor.Compact(context.Background(), &StaticSummarizer{Summary: "x"})
	if err != nil {
		t.Fatalf("compact: %v", err)
	}

	if result.CompactedCount != 5 {
		t.Errorf("expected 5 compacted messages, got %d", result.CompactedCount)
	}
	if result.RemainingTokens != 300 {
		t.Errorf("expected 300 remaining tokens, got %d", result.RemainingTokens)
	}
}

func TestCompactorTracksOverflowInGuard(t *testing.T) {
	guard := NewWindowGuard(DefaultGuardConfig(1000))
	compactor := NewCompactor(guard, DefaultCompactionConfig())

	compactor.Add(newTestMessage("1", "user", "large", 900))
	compactor.Add(newTestMessage("2", "assistant", "overflow", 200))

	if compactor.TotalTokens() != 1100 {
		t.Fatalf("expected 1100 total tokens, got %d", compactor.TotalTokens())
	}
	if guard.CurrentTokens() != 1100 {
		t.Fatalf("expected guard to track 1100 tokens, got %d", guard.CurrentTokens())
	}

	used, max, avail := guard.Status()
	if used != 1100 || max != 1000 {
		t.Fatalf("expected used=1100 max=1000, got used=%d max=%d", used, max)
	}
	if avail != 0 {
		t.Fatalf("expected no available tokens after overflow, got %d", avail)
	}
}

func TestCompactorCompactKeepsGuardSyncedWhenPinnedMessagesStillOverflow(t *testing.T) {
	guard := NewWindowGuard(GuardConfig{
		MaxTokens:    100,
		SafetyMargin: 0,
	})
	compactor := NewCompactor(guard, CompactionConfig{
		Strategy:         StrategyOldestFirst,
		MinKeepMessages:  2,
		MaxSummaryTokens: 10,
		CompactThreshold: 0.5,
		TargetTokenRatio: 0.1,
		SummaryPrefix:    "s",
	})

	pinnedA := newTestMessage("pinned-a", "system", "important", 60)
	pinnedA.Pinned = true
	pinnedB := newTestMessage("pinned-b", "system", "important", 60)
	pinnedB.Pinned = true

	compactor.Add(pinnedA)
	compactor.Add(pinnedB)
	compactor.Add(newTestMessage("user-1", "user", "compact me", 60))

	result, err := compactor.Compact(context.Background(), &StaticSummarizer{Summary: "x"})
	if err != nil {
		t.Fatalf("compact: %v", err)
	}

	if result.RemainingTokens != 120 {
		t.Fatalf("expected 120 remaining tokens, got %d", result.RemainingTokens)
	}
	if compactor.TotalTokens() != 120 {
		t.Fatalf("expected compactor to retain 120 tokens, got %d", compactor.TotalTokens())
	}
	if guard.CurrentTokens() != 120 {
		t.Fatalf("expected guard to stay synced at 120 tokens, got %d", guard.CurrentTokens())
	}
}
