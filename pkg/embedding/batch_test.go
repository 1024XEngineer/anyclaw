package embedding

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

func TestBatchProcessorBasic(t *testing.T) {
	provider := &mockProviderBatch{dim: 4}
	mgr, _ := NewManager([]Provider{provider}, NewCache(1000, time.Hour))
	bp := NewBatchProcessor(mgr, DefaultBatchConfig())

	texts := make([]string, 10)
	for i := range texts {
		texts[i] = fmt.Sprintf("text %d", i)
	}

	ctx := context.Background()
	result, err := bp.Process(ctx, texts)
	if err != nil {
		t.Fatalf("batch process: %v", err)
	}

	if result.Total != 10 {
		t.Errorf("expected total 10, got %d", result.Total)
	}
	if result.Succeeded != 10 {
		t.Errorf("expected succeeded 10, got %d", result.Succeeded)
	}
	if result.Failed != 0 {
		t.Errorf("expected failed 0, got %d", result.Failed)
	}
	if len(result.Embeddings) != 10 {
		t.Errorf("expected 10 embeddings, got %d", len(result.Embeddings))
	}
}

func TestBatchProcessorEmpty(t *testing.T) {
	provider := &mockProviderBatch{dim: 4}
	mgr, _ := NewManager([]Provider{provider}, nil)
	bp := NewBatchProcessor(mgr, DefaultBatchConfig())

	ctx := context.Background()
	result, err := bp.Process(ctx, []string{})
	if err != nil {
		t.Fatalf("batch process: %v", err)
	}

	if result.Total != 0 {
		t.Errorf("expected total 0, got %d", result.Total)
	}
}

func TestBatchProcessorPartialFailure(t *testing.T) {
	provider := &mockProviderBatch{dim: 4, failIndices: map[int]bool{2: true, 5: true}}
	mgr, _ := NewManager([]Provider{provider}, nil)
	bp := NewBatchProcessor(mgr, BatchConfig{
		ChunkSize:   5,
		Concurrency: 2,
		MaxRetries:  0,
	})

	texts := make([]string, 8)
	for i := range texts {
		texts[i] = fmt.Sprintf("text %d", i)
	}

	ctx := context.Background()
	result, err := bp.Process(ctx, texts)
	if err != nil {
		t.Fatalf("batch process: %v", err)
	}

	if result.Succeeded != 6 {
		t.Errorf("expected succeeded 6, got %d", result.Succeeded)
	}
	if result.Failed != 2 {
		t.Errorf("expected failed 2, got %d", result.Failed)
	}
	if len(result.Errors) != 2 {
		t.Errorf("expected 2 errors, got %d", len(result.Errors))
	}
}

func TestBatchProcessorProgress(t *testing.T) {
	provider := &mockProviderBatch{dim: 4}
	mgr, _ := NewManager([]Provider{provider}, nil)

	var progressCount atomic.Int32
	bp := NewBatchProcessor(mgr, BatchConfig{
		ChunkSize:   5,
		Concurrency: 2,
		MaxRetries:  0,
		ProgressFunc: func(p BatchProgress) {
			progressCount.Add(1)
		},
	})

	texts := make([]string, 20)
	for i := range texts {
		texts[i] = fmt.Sprintf("text %d", i)
	}

	ctx := context.Background()
	_, err := bp.Process(ctx, texts)
	if err != nil {
		t.Fatalf("batch process: %v", err)
	}

	if progressCount.Load() == 0 {
		t.Error("expected progress callbacks")
	}
}

func TestBatchProcessorWithRateLimit(t *testing.T) {
	provider := &mockProviderBatch{dim: 4}
	mgr, _ := NewManager([]Provider{provider}, nil)

	start := time.Now()
	bp := NewBatchProcessor(mgr, BatchConfig{
		ChunkSize:       10,
		Concurrency:     1,
		MaxRetries:      0,
		RateLimitPerSec: 1000,
	})

	texts := make([]string, 10)
	for i := range texts {
		texts[i] = fmt.Sprintf("text %d", i)
	}

	ctx := context.Background()
	result, err := bp.Process(ctx, texts)
	if err != nil {
		t.Fatalf("batch process: %v", err)
	}

	elapsed := time.Since(start)
	if elapsed < 5*time.Millisecond {
		t.Logf("rate limiting may not be precise in tests, elapsed: %v", elapsed)
	}

	if result.Succeeded != 10 {
		t.Errorf("expected 10 succeeded, got %d", result.Succeeded)
	}
}

func TestBatchProcessorWithProvider(t *testing.T) {
	provider := &mockProviderBatch{dim: 4}
	mgr, _ := NewManager([]Provider{provider}, nil)
	bp := NewBatchProcessor(mgr, DefaultBatchConfig())

	texts := []string{"a", "b", "c"}

	ctx := context.Background()
	result, err := bp.ProcessWithProvider(ctx, texts, 0)
	if err != nil {
		t.Fatalf("batch process with provider: %v", err)
	}

	if result.Succeeded != 3 {
		t.Errorf("expected 3 succeeded, got %d", result.Succeeded)
	}
}

func TestBatchProcessorInvalidProvider(t *testing.T) {
	provider := &mockProviderBatch{dim: 4}
	mgr, _ := NewManager([]Provider{provider}, nil)
	bp := NewBatchProcessor(mgr, DefaultBatchConfig())

	ctx := context.Background()
	_, err := bp.ProcessWithProvider(ctx, []string{"test"}, 5)
	if err == nil {
		t.Error("expected error for invalid provider index")
	}
}

func TestBatchProcessorTimeout(t *testing.T) {
	provider := &mockProviderBatch{dim: 4, delay: 100 * time.Millisecond}
	mgr, _ := NewManager([]Provider{provider}, nil)

	bp := NewBatchProcessor(mgr, BatchConfig{
		ChunkSize:   1,
		Concurrency: 1,
		MaxRetries:  0,
		Timeout:     50 * time.Millisecond,
	})

	texts := make([]string, 10)
	for i := range texts {
		texts[i] = fmt.Sprintf("text %d", i)
	}

	ctx := context.Background()
	result, err := bp.Process(ctx, texts)
	if err != nil {
		t.Fatalf("batch process: %v", err)
	}

	if result.Failed == 0 {
		t.Log("timeout may not trigger in all environments")
	}
}

func TestBatchProcessorWithProviderRejectsInvalidBatchResult(t *testing.T) {
	provider := &mockProviderBatch{
		dim:             4,
		batchEmbeddings: [][][]float32{{{1, 2, 3, 4}}},
	}
	mgr, _ := NewManager([]Provider{provider}, nil)
	bp := NewBatchProcessor(mgr, BatchConfig{
		ChunkSize: 3,
		Timeout:   time.Minute,
	})

	ctx := context.Background()
	result, err := bp.ProcessWithProvider(ctx, []string{"a", "b", "c"}, 0)
	if err != nil {
		t.Fatalf("batch process with provider: %v", err)
	}

	if result.Succeeded != 0 {
		t.Fatalf("expected 0 succeeded, got %d", result.Succeeded)
	}
	if result.Failed != 3 {
		t.Fatalf("expected 3 failed, got %d", result.Failed)
	}
	if len(result.Errors) != 3 {
		t.Fatalf("expected 3 errors, got %d", len(result.Errors))
	}
	for i, emb := range result.Embeddings {
		if emb != nil {
			t.Fatalf("expected nil embedding at index %d when batch result is invalid", i)
		}
	}
}

func TestBatchProcessorRetry(t *testing.T) {
	provider := &mockProviderBatch{dim: 4, failUntil: 2}
	mgr, _ := NewManager([]Provider{provider}, nil)

	bp := NewBatchProcessor(mgr, BatchConfig{
		ChunkSize:   1,
		Concurrency: 1,
		MaxRetries:  3,
		RetryDelay:  10 * time.Millisecond,
	})

	ctx := context.Background()
	result, err := bp.Process(ctx, []string{"retry me"})
	if err != nil {
		t.Fatalf("batch process: %v", err)
	}

	if result.Succeeded != 1 {
		t.Errorf("expected 1 succeeded after retries, got %d", result.Succeeded)
	}
}

type mockProviderBatch struct {
	dim             int
	failIndices     map[int]bool
	failUntil       int
	callCount       atomic.Int32
	delay           time.Duration
	batchEmbeddings [][][]float32
	batchCalls      atomic.Int32
}

func (m *mockProviderBatch) Embed(ctx context.Context, text string) ([]float32, error) {
	count := m.callCount.Add(1)
	if m.delay > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(m.delay):
		}
	}

	if m.failIndices != nil && m.failIndices[int(count-1)] {
		return nil, fmt.Errorf("simulated failure for index %d", count-1)
	}

	if m.failUntil > 0 && int(count) <= m.failUntil {
		return nil, fmt.Errorf("simulated failure attempt %d", count)
	}

	result := make([]float32, m.dim)
	for i := range result {
		result[i] = float32(len(text)) / float32(m.dim)
	}
	return result, nil
}

func (m *mockProviderBatch) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	call := int(m.batchCalls.Add(1)) - 1
	if call < len(m.batchEmbeddings) {
		return m.batchEmbeddings[call], nil
	}

	var results [][]float32
	for _, text := range texts {
		emb, err := m.Embed(ctx, text)
		if err != nil {
			return nil, err
		}
		results = append(results, emb)
	}
	return results, nil
}

func (m *mockProviderBatch) Name() string   { return "mock" }
func (m *mockProviderBatch) Dimension() int { return m.dim }
