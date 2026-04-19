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

func TestHybridSearchScoreComponents(t *testing.T) {
	entries := []MemoryEntry{
		{ID: "1", Type: "fact", Content: "Go programming", Timestamp: time.Now()},
	}

	opts := DefaultSearchOptions()
	opts.UseVector = true
	opts.UseKeyword = true
	opts.VectorWeight = 0.5
	opts.KeywordWeight = 0.5
	opts.NormalizeScores = true
	opts.ApplyTemporal = false
	opts.QueryEmbedding = []float64{0.1, 0.2}
	opts.EntryEmbeddings = map[string][]float64{
		"1": {0.1, 0.2},
	}

	results := HybridSearch(entries, "programming", opts)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].KeywordScore <= 0 {
		t.Errorf("expected positive keyword score, got %f", results[0].KeywordScore)
	}
	if results[0].VectorScore <= 0 {
		t.Errorf("expected positive vector score, got %f", results[0].VectorScore)
	}
	if results[0].Score <= 0 {
		t.Errorf("expected positive combined score, got %f", results[0].Score)
	}
	if results[0].MatchType != "hybrid" {
		t.Errorf("expected match type hybrid, got %s", results[0].MatchType)
	}
}

func TestHybridSearchWeightNormalization(t *testing.T) {
	entries := []MemoryEntry{
		{ID: "1", Type: "fact", Content: "Go programming", Timestamp: time.Now()},
		{ID: "2", Type: "fact", Content: "Python programming", Timestamp: time.Now()},
	}

	opts := DefaultSearchOptions()
	opts.UseVector = true
	opts.UseKeyword = true
	opts.VectorWeight = 3.0
	opts.KeywordWeight = 1.0
	opts.NormalizeScores = true
	opts.ApplyTemporal = false
	opts.QueryEmbedding = []float64{0.1, 0.2}
	opts.EntryEmbeddings = map[string][]float64{
		"1": {0.1, 0.2},
		"2": {0.9, 0.8},
	}

	results := HybridSearch(entries, "programming", opts)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	kwW, vecW := opts.effectiveWeights()
	if kwW < 0.24 || kwW > 0.26 {
		t.Errorf("expected keyword weight ~0.25, got %f", kwW)
	}
	if vecW < 0.74 || vecW > 0.76 {
		t.Errorf("expected vector weight ~0.75, got %f", vecW)
	}
}

func TestHybridSearchMinScore(t *testing.T) {
	entries := []MemoryEntry{
		{ID: "1", Type: "fact", Content: "Go programming", Timestamp: time.Now()},
		{ID: "2", Type: "fact", Content: "The weather is nice", Timestamp: time.Now()},
	}

	opts := DefaultSearchOptions()
	opts.UseKeyword = true
	opts.UseVector = false
	opts.MinScore = 0.5
	opts.Limit = 10

	results := HybridSearch(entries, "programming", opts)

	if len(results) != 1 {
		t.Errorf("expected 1 result with min score, got %d", len(results))
	}
}

func TestHybridSearchTypeFilter(t *testing.T) {
	entries := []MemoryEntry{
		{ID: "1", Type: "fact", Content: "Go programming", Timestamp: time.Now()},
		{ID: "2", Type: "reflection", Content: "Go programming reflection", Timestamp: time.Now()},
	}

	opts := DefaultSearchOptions()
	opts.UseKeyword = true
	opts.UseVector = false
	opts.Types = []string{"fact"}
	opts.Limit = 10

	results := HybridSearch(entries, "programming", opts)

	if len(results) != 1 {
		t.Errorf("expected 1 result with type filter, got %d", len(results))
	}
	if results[0].Entry.Type != "fact" {
		t.Errorf("expected type fact, got %s", results[0].Entry.Type)
	}
}

func TestHybridSearchMaxAge(t *testing.T) {
	entries := []MemoryEntry{
		{ID: "1", Type: "fact", Content: "Go programming", Timestamp: time.Now()},
		{ID: "2", Type: "fact", Content: "Go programming old", Timestamp: time.Now().Add(-48 * time.Hour)},
	}

	opts := DefaultSearchOptions()
	opts.UseKeyword = true
	opts.UseVector = false
	opts.MaxAge = 24 * time.Hour
	opts.Limit = 10

	results := HybridSearch(entries, "programming", opts)

	if len(results) != 1 {
		t.Errorf("expected 1 result with max age, got %d", len(results))
	}
	if results[0].Entry.ID != "1" {
		t.Errorf("expected ID 1, got %s", results[0].Entry.ID)
	}
}

func TestHybridSearchTemporalDecay(t *testing.T) {
	now := time.Now()
	entries := []MemoryEntry{
		{ID: "1", Type: "fact", Content: "Go programming", Timestamp: now},
		{ID: "2", Type: "fact", Content: "Go programming", Timestamp: now.Add(-24 * time.Hour)},
	}

	opts := DefaultSearchOptions()
	opts.UseKeyword = true
	opts.UseVector = false
	opts.ApplyTemporal = true
	opts.TemporalDecay = 24.0
	opts.Limit = 10

	results := HybridSearch(entries, "programming", opts)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	if results[0].Entry.ID != "1" {
		t.Errorf("expected recent entry first, got %s", results[0].Entry.ID)
	}

	if results[0].TemporalWeight <= results[1].TemporalWeight {
		t.Errorf("expected recent entry to have higher temporal weight")
	}
}

func TestHybridSearchMMR(t *testing.T) {
	entries := []MemoryEntry{
		{ID: "1", Type: "fact", Content: "Go programming language", Timestamp: time.Now()},
		{ID: "2", Type: "fact", Content: "Go programming language tutorial", Timestamp: time.Now()},
		{ID: "3", Type: "fact", Content: "Python programming language", Timestamp: time.Now()},
	}

	opts := DefaultSearchOptions()
	opts.UseKeyword = true
	opts.UseVector = false
	opts.ApplyMMR = true
	opts.MMRLambda = 0.7
	opts.Limit = 2

	results := HybridSearch(entries, "programming language", opts)

	if len(results) != 2 {
		t.Errorf("expected 2 results with MMR, got %d", len(results))
	}

	if results[0].Entry.ID == results[1].Entry.ID {
		t.Error("expected different entries with MMR")
	}
}

func TestHybridSearchLimit(t *testing.T) {
	entries := []MemoryEntry{
		{ID: "1", Type: "fact", Content: "Go programming", Timestamp: time.Now()},
		{ID: "2", Type: "fact", Content: "Python programming", Timestamp: time.Now()},
		{ID: "3", Type: "fact", Content: "Java programming", Timestamp: time.Now()},
	}

	opts := DefaultSearchOptions()
	opts.UseKeyword = true
	opts.UseVector = false
	opts.Limit = 2

	results := HybridSearch(entries, "programming", opts)

	if len(results) != 2 {
		t.Errorf("expected 2 results with limit, got %d", len(results))
	}
}

func TestHybridSearchEmptyEntries(t *testing.T) {
	opts := DefaultSearchOptions()
	opts.UseKeyword = true
	opts.UseVector = false
	opts.Limit = 10

	results := HybridSearch([]MemoryEntry{}, "query", opts)

	if len(results) != 0 {
		t.Errorf("expected 0 results for empty entries, got %d", len(results))
	}
}

func TestBM25Search(t *testing.T) {
	entries := []MemoryEntry{
		{ID: "1", Type: "fact", Content: "Go is a programming language used for systems programming", Timestamp: time.Now()},
		{ID: "2", Type: "fact", Content: "Python is a programming language", Timestamp: time.Now()},
		{ID: "3", Type: "fact", Content: "The weather is sunny", Timestamp: time.Now()},
	}

	scores := bm25Search(entries, "programming language")

	if scores["1"] <= 0 {
		t.Error("expected positive score for entry 1")
	}
	if scores["2"] <= 0 {
		t.Error("expected positive score for entry 2")
	}
	if scores["3"] != 0 {
		t.Errorf("expected zero score for entry 3, got %f", scores["3"])
	}
}

func TestNormalizeScoresMinmax(t *testing.T) {
	scores := map[string]float64{
		"a": 1.0,
		"b": 3.0,
		"c": 5.0,
	}

	normalized := normalizeScoresMinmax(scores)

	if normalized["a"] != 0.0 {
		t.Errorf("expected normalized a = 0.0, got %f", normalized["a"])
	}
	if normalized["c"] != 1.0 {
		t.Errorf("expected normalized c = 1.0, got %f", normalized["c"])
	}
	if normalized["b"] < 0.49 || normalized["b"] > 0.51 {
		t.Errorf("expected normalized b ~0.5, got %f", normalized["b"])
	}
}

func TestNormalizeScoresEmpty(t *testing.T) {
	scores := map[string]float64{}
	normalized := normalizeScoresMinmax(scores)

	if len(normalized) != 0 {
		t.Errorf("expected empty normalized scores, got %d", len(normalized))
	}
}

func TestNormalizeScoresConstant(t *testing.T) {
	scores := map[string]float64{
		"a": 1.0,
		"b": 1.0,
		"c": 1.0,
	}

	normalized := normalizeScoresMinmax(scores)

	for k, v := range normalized {
		if v != 0.5 {
			t.Errorf("expected normalized %s = 0.5, got %f", k, v)
		}
	}
}

func TestEffectiveWeights(t *testing.T) {
	tests := []struct {
		name    string
		opts    SearchOptions
		wantKw  float64
		wantVec float64
	}{
		{
			name:    "both enabled default",
			opts:    SearchOptions{UseVector: true, UseKeyword: true, VectorWeight: 0.6, KeywordWeight: 0.4},
			wantKw:  0.4,
			wantVec: 0.6,
		},
		{
			name:    "both enabled unnormalized",
			opts:    SearchOptions{UseVector: true, UseKeyword: true, VectorWeight: 3.0, KeywordWeight: 1.0},
			wantKw:  0.25,
			wantVec: 0.75,
		},
		{
			name:    "vector only",
			opts:    SearchOptions{UseVector: true, UseKeyword: false},
			wantKw:  0,
			wantVec: 1.0,
		},
		{
			name:    "keyword only",
			opts:    SearchOptions{UseVector: false, UseKeyword: true},
			wantKw:  1.0,
			wantVec: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kwW, vecW := tt.opts.effectiveWeights()
			if kwW < tt.wantKw-0.01 || kwW > tt.wantKw+0.01 {
				t.Errorf("keyword weight: expected %f, got %f", tt.wantKw, kwW)
			}
			if vecW < tt.wantVec-0.01 || vecW > tt.wantVec+0.01 {
				t.Errorf("vector weight: expected %f, got %f", tt.wantVec, vecW)
			}
		})
	}
}
