package memory

import (
	"context"
	"errors"
	"testing"
	"time"
)

type stubEmbeddingProvider struct {
	vectors map[string][]float64
	err     error
}

func (s stubEmbeddingProvider) Embed(ctx context.Context, text string) ([]float64, error) {
	if s.err != nil {
		return nil, s.err
	}
	vector, ok := s.vectors[text]
	if !ok {
		return nil, errors.New("missing vector")
	}
	return vector, nil
}

func (s stubEmbeddingProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float64, error) {
	if s.err != nil {
		return nil, s.err
	}
	results := make([][]float64, 0, len(texts))
	for _, text := range texts {
		vector, ok := s.vectors[text]
		if !ok {
			return nil, errors.New("missing vector")
		}
		results = append(results, vector)
	}
	return results, nil
}

func (s stubEmbeddingProvider) Name() string   { return "stub" }
func (s stubEmbeddingProvider) Dimension() int { return 2 }

func TestHybridSearchUsesRealVectorRanking(t *testing.T) {
	entries := []MemoryEntry{
		{ID: "cat", Content: "feline companion", Timestamp: time.Now()},
		{ID: "dog", Content: "canine companion", Timestamp: time.Now()},
	}

	opts := DefaultSearchOptions()
	opts.UseKeyword = false
	opts.UseVector = true
	opts.ApplyTemporal = false
	opts.Embedder = stubEmbeddingProvider{
		vectors: map[string][]float64{
			"kitten":           {1, 0},
			"feline companion": {0.99, 0.01},
			"canine companion": {0.1, 0.9},
		},
	}

	results := HybridSearch(entries, "kitten", opts)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Entry.ID != "cat" {
		t.Fatalf("expected vector match to rank first, got %q", results[0].Entry.ID)
	}
	if results[0].MatchType != "vector" {
		t.Fatalf("expected vector match type, got %q", results[0].MatchType)
	}
	if results[0].Score <= results[1].Score {
		t.Fatalf("expected top result score %f to exceed second %f", results[0].Score, results[1].Score)
	}
}

func TestHybridSearchSupportsPrecomputedEmbeddings(t *testing.T) {
	entries := []MemoryEntry{
		{ID: "alpha", Content: "alpha", Timestamp: time.Now()},
		{ID: "beta", Content: "beta", Timestamp: time.Now()},
	}

	opts := DefaultSearchOptions()
	opts.UseKeyword = false
	opts.UseVector = true
	opts.ApplyTemporal = false
	opts.QueryEmbedding = []float64{1, 0}
	opts.EntryEmbeddings = map[string][]float64{
		"alpha": {1, 0},
		"beta":  {0, 1},
	}

	results := HybridSearch(entries, "ignored", opts)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Entry.ID != "alpha" {
		t.Fatalf("expected precomputed vector match first, got %q", results[0].Entry.ID)
	}
}

func TestHybridSearchFallsBackToKeywordWhenVectorUnavailable(t *testing.T) {
	entries := []MemoryEntry{
		{ID: "hello", Content: "hello world", Timestamp: time.Now()},
		{ID: "other", Content: "goodbye world", Timestamp: time.Now()},
	}

	opts := DefaultSearchOptions()
	opts.UseKeyword = true
	opts.UseVector = true
	opts.ApplyTemporal = false
	opts.Embedder = stubEmbeddingProvider{err: errors.New("embedding offline")}

	results := HybridSearch(entries, "hello", opts)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Entry.ID != "hello" {
		t.Fatalf("expected keyword fallback result first, got %q", results[0].Entry.ID)
	}
	if results[0].MatchType != "keyword" {
		t.Fatalf("expected keyword fallback match type, got %q", results[0].MatchType)
	}
}

func TestHybridSearchCombinesKeywordAndVectorScores(t *testing.T) {
	entries := []MemoryEntry{
		{ID: "semantic", Content: "rust compiler internals", Timestamp: time.Now()},
		{ID: "keyword", Content: "rust ownership basics", Timestamp: time.Now()},
	}

	opts := DefaultSearchOptions()
	opts.UseKeyword = true
	opts.UseVector = true
	opts.VectorWeight = 0.8
	opts.ApplyTemporal = false
	opts.Embedder = stubEmbeddingProvider{
		vectors: map[string][]float64{
			"borrow checker":          {1, 0},
			"rust compiler internals": {0.95, 0.05},
			"rust ownership basics":   {0.55, 0.45},
		},
	}

	results := HybridSearch(entries, "borrow checker", opts)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Entry.ID != "semantic" {
		t.Fatalf("expected semantic vector signal to win hybrid ranking, got %q", results[0].Entry.ID)
	}
	if results[0].MatchType != "vector" && results[0].MatchType != "hybrid" {
		t.Fatalf("expected vector-aware match type, got %q", results[0].MatchType)
	}
}
