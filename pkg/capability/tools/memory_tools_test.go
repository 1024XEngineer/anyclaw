package tools

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	appmemory "github.com/1024XEngineer/anyclaw/pkg/state/memory"
)

type stubMemoryBackend struct {
	searchResults []appmemory.MemoryEntry
	searchErr     error
}

func (s *stubMemoryBackend) Init() error                                { return nil }
func (s *stubMemoryBackend) Add(appmemory.MemoryEntry) error            { return nil }
func (s *stubMemoryBackend) Get(string) (*appmemory.MemoryEntry, error) { return nil, nil }
func (s *stubMemoryBackend) Delete(string) error                        { return nil }
func (s *stubMemoryBackend) List() ([]appmemory.MemoryEntry, error)     { return nil, nil }
func (s *stubMemoryBackend) Close() error                               { return nil }
func (s *stubMemoryBackend) Search(string, int) ([]appmemory.MemoryEntry, error) {
	if s.searchErr != nil {
		return nil, s.searchErr
	}
	return s.searchResults, nil
}

type stubVectorBackend struct {
	vectorResults []appmemory.VectorEntry
	hybridResults []appmemory.HybridSearchResult
	vectorErr     error
	hybridErr     error
}

func (s *stubVectorBackend) VectorSearch([]float64, int, float64) ([]appmemory.VectorEntry, error) {
	if s.vectorErr != nil {
		return nil, s.vectorErr
	}
	return s.vectorResults, nil
}

func (s *stubVectorBackend) HybridSearch(string, []float64, int, float64) ([]appmemory.HybridSearchResult, error) {
	if s.hybridErr != nil {
		return nil, s.hybridErr
	}
	return s.hybridResults, nil
}

func (s *stubVectorBackend) StoreEmbedding(string, []float64) error { return nil }

func TestMemorySearchToolWithBackendUsesBackend(t *testing.T) {
	now := time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC)
	mem := &stubMemoryBackend{
		searchResults: []appmemory.MemoryEntry{{
			ID:        "1",
			Type:      "fact",
			Content:   "demo memory",
			Timestamp: now,
		}},
	}

	result, err := MemorySearchToolWithBackend(context.Background(), map[string]any{"query": "demo"}, t.TempDir(), mem)
	if err != nil {
		t.Fatalf("MemorySearchToolWithBackend: %v", err)
	}
	if !strings.Contains(result, "demo memory") || !strings.Contains(result, "2026-04-22 10:00") {
		t.Fatalf("unexpected memory search output: %q", result)
	}
}

func TestMemoryVectorSearchToolBranches(t *testing.T) {
	mem := &stubMemoryBackend{
		searchResults: []appmemory.MemoryEntry{{
			ID:        "text",
			Type:      "fact",
			Content:   "text fallback",
			Timestamp: time.Date(2026, 4, 22, 9, 0, 0, 0, time.UTC),
		}},
	}
	vec := &stubVectorBackend{
		vectorResults: []appmemory.VectorEntry{{
			ID:        "vec",
			Type:      "fact",
			Content:   "vector result",
			Timestamp: time.Date(2026, 4, 22, 9, 30, 0, 0, time.UTC),
			Score:     0.99,
		}},
	}

	textResult, err := MemoryVectorSearchTool(context.Background(), map[string]any{
		"query": "demo",
	}, mem, vec)
	if err != nil {
		t.Fatalf("MemoryVectorSearchTool text fallback: %v", err)
	}
	if !strings.Contains(textResult, "text fallback") {
		t.Fatalf("expected text fallback output, got %q", textResult)
	}

	vectorResult, err := MemoryVectorSearchTool(context.Background(), map[string]any{
		"query":     "demo",
		"embedding": []any{0.1, 0.2},
	}, mem, vec)
	if err != nil {
		t.Fatalf("MemoryVectorSearchTool vector search: %v", err)
	}
	if !strings.Contains(vectorResult, "vector result") || !strings.Contains(vectorResult, "0.9900") {
		t.Fatalf("unexpected vector search output: %q", vectorResult)
	}
}

func TestMemoryHybridSearchToolBranches(t *testing.T) {
	mem := &stubMemoryBackend{
		searchResults: []appmemory.MemoryEntry{{
			ID:        "text",
			Type:      "fact",
			Content:   "hybrid text fallback",
			Timestamp: time.Date(2026, 4, 22, 9, 0, 0, 0, time.UTC),
		}},
	}
	vec := &stubVectorBackend{
		hybridResults: []appmemory.HybridSearchResult{{
			Entry: appmemory.VectorEntry{
				ID:        "hybrid",
				Type:      "fact",
				Content:   "hybrid result",
				Timestamp: time.Date(2026, 4, 22, 11, 0, 0, 0, time.UTC),
			},
			FTSScore:    0.8,
			VectorScore: 0.9,
			FinalScore:  0.85,
		}},
	}

	textResult, err := MemoryHybridSearchTool(context.Background(), map[string]any{
		"query": "demo",
	}, mem, vec)
	if err != nil {
		t.Fatalf("MemoryHybridSearchTool text fallback: %v", err)
	}
	if !strings.Contains(textResult, "hybrid text fallback") {
		t.Fatalf("expected text fallback output, got %q", textResult)
	}

	hybridResult, err := MemoryHybridSearchTool(context.Background(), map[string]any{
		"query":     "demo",
		"embedding": []any{0.2, 0.3},
	}, mem, vec)
	if err != nil {
		t.Fatalf("MemoryHybridSearchTool hybrid search: %v", err)
	}
	if !strings.Contains(hybridResult, "hybrid result") || !strings.Contains(hybridResult, "final: 0.8500") {
		t.Fatalf("unexpected hybrid search output: %q", hybridResult)
	}
}

func TestMemoryToolErrors(t *testing.T) {
	if _, err := MemorySearchToolWithBackend(context.Background(), map[string]any{}, t.TempDir(), nil); err == nil {
		t.Fatal("expected missing query error")
	}
	if _, err := MemoryVectorSearchTool(context.Background(), map[string]any{"query": "demo"}, nil, nil); err == nil {
		t.Fatal("expected missing vector backend error")
	}
	if _, err := MemoryHybridSearchTool(context.Background(), map[string]any{"query": "demo"}, nil, nil); err == nil {
		t.Fatal("expected missing hybrid backend error")
	}

	mem := &stubMemoryBackend{searchErr: errors.New("backend failed")}
	vec := &stubVectorBackend{vectorErr: errors.New("vector failed"), hybridErr: errors.New("hybrid failed")}
	if _, err := MemorySearchToolWithBackend(context.Background(), map[string]any{"query": "demo"}, t.TempDir(), mem); err == nil {
		t.Fatal("expected backend search error")
	}
	if _, err := MemoryVectorSearchTool(context.Background(), map[string]any{
		"query":     "demo",
		"embedding": []any{1.0},
	}, mem, vec); err == nil {
		t.Fatal("expected vector backend error")
	}
	if _, err := MemoryHybridSearchTool(context.Background(), map[string]any{
		"query":     "demo",
		"embedding": []any{1.0},
	}, mem, vec); err == nil {
		t.Fatal("expected hybrid backend error")
	}
}

func TestMemorySearchToolWithCwdAndFormatTimestamp(t *testing.T) {
	workspace := t.TempDir()
	if _, err := MemorySearchToolWithCwd(context.Background(), map[string]any{}, workspace); err == nil {
		t.Fatal("expected missing query error from cwd search wrapper")
	}
	if got := formatMemoryTimestamp(time.Date(2026, 4, 22, 12, 34, 0, 0, time.UTC)); got != "2026-04-22 12:34" {
		t.Fatalf("unexpected formatted timestamp %q", got)
	}
}
