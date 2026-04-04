// Package memory provides the enhanced memory system with hybrid search,
// MMR ranking, temporal decay, and multiple storage backends.
// This mirrors OpenClaw's memory architecture with SQLite + vector search.
package memory

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"
)

// SearchResult represents a memory search result with scoring.
type SearchResult struct {
	Entry          MemoryEntry
	Score          float64
	MatchType      string // "vector", "keyword", "hybrid"
	TemporalWeight float64
}

// SearchOptions configures memory search behavior.
type SearchOptions struct {
	Limit           int
	Types           []string // Filter by entry types
	MaxAge          time.Duration
	MinScore        float64
	UseVector       bool
	UseKeyword      bool
	VectorWeight    float64 // 0.0-1.0, weight for vector results in hybrid search
	ApplyMMR        bool
	MMRLambda       float64 // MMR diversity parameter (0.0-1.0, default 0.7)
	ApplyTemporal   bool
	TemporalDecay   float64 // Half-life in hours for temporal decay
	Context         context.Context
	Embedder        EmbeddingProvider
	QueryEmbedding  []float64
	EntryEmbeddings map[string][]float64
}

// DefaultSearchOptions returns sensible defaults matching OpenClaw's behavior.
func DefaultSearchOptions() SearchOptions {
	return SearchOptions{
		Limit:         10,
		UseVector:     false,
		UseKeyword:    true,
		VectorWeight:  0.6,
		ApplyMMR:      false,
		MMRLambda:     0.7,
		ApplyTemporal: true,
		TemporalDecay: 168.0, // 7 days half-life
		Context:       context.Background(),
	}
}

// HybridSearch performs combined keyword + vector search with MMR and temporal decay.
func HybridSearch(entries []MemoryEntry, query string, opts SearchOptions) []SearchResult {
	if opts.Limit <= 0 {
		opts.Limit = 10
	}

	// Filter by type
	if len(opts.Types) > 0 {
		typeSet := make(map[string]bool, len(opts.Types))
		for _, t := range opts.Types {
			typeSet[t] = true
		}
		filtered := make([]MemoryEntry, 0, len(entries))
		for _, e := range entries {
			if typeSet[e.Type] {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}

	// Filter by age
	if opts.MaxAge > 0 {
		cutoff := time.Now().Add(-opts.MaxAge)
		filtered := make([]MemoryEntry, 0, len(entries))
		for _, e := range entries {
			if e.Timestamp.After(cutoff) {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}

	// Keyword search
	keywordScores := map[string]float64{}
	if opts.UseKeyword || !opts.UseVector {
		keywordScores = keywordSearch(entries, query)
	}

	// Vector search uses configured embeddings when available and degrades
	// to keyword scoring when the caller has not wired a vector provider yet.
	var vectorScores map[string]float64
	if opts.UseVector {
		vectorScores = vectorSearch(entries, query, opts)
	}

	// Combine scores
	results := make([]SearchResult, 0, len(entries))
	for _, entry := range entries {
		kwScore := keywordScores[entry.ID]
		vecScore := vectorScores[entry.ID]

		var combinedScore float64
		matchType := "keyword"

		switch {
		case opts.UseVector && vecScore > 0 && opts.UseKeyword && kwScore > 0:
			combinedScore = opts.VectorWeight*vecScore + (1-opts.VectorWeight)*kwScore
			matchType = "hybrid"
		case opts.UseVector && vecScore > 0:
			combinedScore = vecScore
			matchType = "vector"
		case opts.UseKeyword || !opts.UseVector:
			combinedScore = kwScore
			matchType = "keyword"
		}

		if combinedScore < opts.MinScore && opts.MinScore > 0 {
			continue
		}

		result := SearchResult{
			Entry:     entry,
			Score:     combinedScore,
			MatchType: matchType,
		}

		// Apply temporal decay
		if opts.ApplyTemporal {
			decay := temporalDecay(entry.Timestamp, opts.TemporalDecay)
			result.TemporalWeight = decay
			result.Score *= decay
		}

		results = append(results, result)
	}

	// Sort by score
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Apply MMR for diversity
	if opts.ApplyMMR && len(results) > opts.Limit {
		results = mmrSelect(results, opts.Limit, opts.MMRLambda)
	}

	// Limit results
	if len(results) > opts.Limit {
		results = results[:opts.Limit]
	}

	return results
}

// keywordSearch performs TF-IDF-like keyword matching.
func keywordSearch(entries []MemoryEntry, query string) map[string]float64 {
	scores := make(map[string]float64)
	queryTerms := tokenize(query)

	if len(queryTerms) == 0 {
		return scores
	}

	// Build document frequency
	docFreq := make(map[string]int)
	for _, entry := range entries {
		terms := tokenize(entry.Content)
		seen := make(map[string]bool)
		for _, term := range terms {
			if !seen[term] {
				docFreq[term]++
				seen[term] = true
			}
		}
	}

	n := float64(len(entries))
	if n == 0 {
		return scores
	}

	for _, entry := range entries {
		terms := tokenize(entry.Content)
		termFreq := make(map[string]int)
		for _, term := range terms {
			termFreq[term]++
		}

		var score float64
		for _, qt := range queryTerms {
			tf := float64(termFreq[qt]) / float64(len(terms))
			idf := math.Log(n / float64(docFreq[qt]+1))
			score += tf * idf
		}

		// Bonus for exact phrase match
		if strings.Contains(strings.ToLower(entry.Content), strings.ToLower(query)) {
			score *= 1.5
		}

		// Bonus for metadata match
		for _, val := range entry.Metadata {
			if strings.Contains(strings.ToLower(val), strings.ToLower(query)) {
				score += 0.5
			}
		}

		scores[entry.ID] = score
	}

	return scores
}

// vectorSearch performs provider-backed cosine similarity search and falls back
// to keyword scoring when callers have not configured embeddings yet.
func vectorSearch(entries []MemoryEntry, query string, opts SearchOptions) map[string]float64 {
	queryEmbedding, ok := resolveQueryEmbedding(query, opts)
	if !ok {
		return keywordFallback(entries, query, opts)
	}

	entryEmbeddings, ok := resolveEntryEmbeddings(entries, opts)
	if !ok {
		return keywordFallback(entries, query, opts)
	}

	scores := make(map[string]float64, len(entries))
	for _, entry := range entries {
		embedding := entryEmbeddings[entry.ID]
		if len(embedding) == 0 {
			continue
		}

		score := CosineSimilarity(queryEmbedding, embedding)
		if score < 0 {
			score = 0
		}
		scores[entry.ID] = score
	}

	return scores
}

// temporalDecay applies exponential decay based on entry age.
func temporalDecay(timestamp time.Time, halfLifeHours float64) float64 {
	age := time.Since(timestamp).Hours()
	return math.Exp(-math.Ln2 * age / halfLifeHours)
}

// mmrSelect implements Maximal Marginal Relevance for diverse results.
func mmrSelect(results []SearchResult, k int, lambda float64) []SearchResult {
	if len(results) <= k {
		return results
	}

	selected := make([]SearchResult, 0, k)
	remaining := make([]Result, len(results))
	for i, r := range results {
		remaining[i] = Result{index: i, score: r.Score}
	}

	for len(selected) < k && len(remaining) > 0 {
		bestIdx := -1
		bestMMR := -math.MaxFloat64

		for i, r := range remaining {
			similarity := results[r.index].Score

			// Calculate max similarity to already selected items
			maxSim := 0.0
			for _, s := range selected {
				sim := contentSimilarity(results[r.index].Entry, s.Entry)
				if sim > maxSim {
					maxSim = sim
				}
			}

			mmr := lambda*similarity - (1-lambda)*maxSim
			if mmr > bestMMR {
				bestMMR = mmr
				bestIdx = i
			}
		}

		if bestIdx >= 0 {
			selected = append(selected, results[remaining[bestIdx].index])
			remaining = append(remaining[:bestIdx], remaining[bestIdx+1:]...)
		}
	}

	return selected
}

// contentSimilarity calculates Jaccard similarity between two entries.
func contentSimilarity(a, b MemoryEntry) float64 {
	termsA := tokenize(a.Content)
	termsB := tokenize(b.Content)

	setA := make(map[string]bool)
	setB := make(map[string]bool)
	for _, t := range termsA {
		setA[t] = true
	}
	for _, t := range termsB {
		setB[t] = true
	}

	intersection := 0
	for t := range setA {
		if setB[t] {
			intersection++
		}
	}

	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

// tokenize splits text into lowercase tokens.
func tokenize(text string) []string {
	text = strings.ToLower(text)
	var tokens []string
	var current strings.Builder

	for _, r := range text {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			current.WriteRune(r)
		} else {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens
}

type Result struct {
	index int
	score float64
}

func resolveQueryEmbedding(query string, opts SearchOptions) ([]float64, bool) {
	if len(opts.QueryEmbedding) > 0 {
		return opts.QueryEmbedding, true
	}
	if opts.Embedder == nil {
		return nil, false
	}

	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}

	embedding, err := opts.Embedder.Embed(ctx, query)
	if err != nil || len(embedding) == 0 {
		return nil, false
	}
	return embedding, true
}

func resolveEntryEmbeddings(entries []MemoryEntry, opts SearchOptions) (map[string][]float64, bool) {
	resolved := make(map[string][]float64, len(entries))
	missingIDs := make([]string, 0, len(entries))
	missingTexts := make([]string, 0, len(entries))

	for _, entry := range entries {
		if embedding, ok := opts.EntryEmbeddings[entry.ID]; ok && len(embedding) > 0 {
			resolved[entry.ID] = embedding
			continue
		}
		if opts.Embedder == nil {
			return nil, false
		}
		missingIDs = append(missingIDs, entry.ID)
		missingTexts = append(missingTexts, entry.Content)
	}

	if len(missingTexts) == 0 {
		return resolved, true
	}

	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}

	embeddings, err := opts.Embedder.EmbedBatch(ctx, missingTexts)
	if err != nil || len(embeddings) != len(missingTexts) {
		return nil, false
	}

	for i, id := range missingIDs {
		if len(embeddings[i]) == 0 {
			return nil, false
		}
		resolved[id] = embeddings[i]
	}

	return resolved, true
}

func keywordFallback(entries []MemoryEntry, query string, opts SearchOptions) map[string]float64 {
	if opts.UseKeyword {
		return keywordSearch(entries, query)
	}
	return map[string]float64{}
}
