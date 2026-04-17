package memory

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"
)

type SearchResult struct {
	Entry          MemoryEntry
	Score          float64
	KeywordScore   float64
	VectorScore    float64
	MatchType      string
	TemporalWeight float64
}

type SearchOptions struct {
	Limit           int
	Types           []string
	MaxAge          time.Duration
	MinScore        float64
	UseVector       bool
	UseKeyword      bool
	VectorWeight    float64
	KeywordWeight   float64
	ApplyMMR        bool
	MMRLambda       float64
	ApplyTemporal   bool
	TemporalDecay   float64
	Context         context.Context
	Embedder        EmbeddingProvider
	QueryEmbedding  []float64
	EntryEmbeddings map[string][]float64
	NormalizeScores bool
}

func DefaultSearchOptions() SearchOptions {
	return SearchOptions{
		Limit:           10,
		UseVector:       false,
		UseKeyword:      true,
		VectorWeight:    0.6,
		KeywordWeight:   0.4,
		ApplyMMR:        false,
		MMRLambda:       0.7,
		ApplyTemporal:   true,
		TemporalDecay:   168.0,
		Context:         context.Background(),
		NormalizeScores: true,
	}
}

func (opts *SearchOptions) effectiveWeights() (kwW, vecW float64) {
	if opts.UseVector && opts.UseKeyword {
		if opts.KeywordWeight > 0 && opts.VectorWeight > 0 {
			total := opts.KeywordWeight + opts.VectorWeight
			return opts.KeywordWeight / total, opts.VectorWeight / total
		}
		vecW = opts.VectorWeight
		kwW = 1.0 - vecW
		if kwW < 0 {
			kwW = 0
		}
		return kwW, vecW
	}
	if opts.UseVector {
		return 0, 1.0
	}
	return 1.0, 0
}

func HybridSearch(entries []MemoryEntry, query string, opts SearchOptions) []SearchResult {
	if opts.Limit <= 0 {
		opts.Limit = 10
	}

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

	keywordScores := map[string]float64{}
	if opts.UseKeyword || !opts.UseVector {
		keywordScores = bm25Search(entries, query)
	}

	var vectorScores map[string]float64
	if opts.UseVector {
		vectorScores = vectorSearch(entries, query, opts)
	}

	kwWeight, vecWeight := opts.effectiveWeights()

	if opts.NormalizeScores && opts.UseVector && opts.UseKeyword {
		keywordScores = normalizeScoresMinmax(keywordScores)
		vectorScores = normalizeScoresMinmax(vectorScores)
	}

	results := make([]SearchResult, 0, len(entries))
	for _, entry := range entries {
		kwScore := keywordScores[entry.ID]
		vecScore := vectorScores[entry.ID]

		var combinedScore float64
		var matchType string

		hasVec := opts.UseVector && vectorScores != nil
		hasKw := (opts.UseKeyword || !opts.UseVector) && keywordScores != nil

		switch {
		case hasVec && hasKw && vecScore > 0 && kwScore > 0:
			combinedScore = vecWeight*vecScore + kwWeight*kwScore
			matchType = "hybrid"
		case hasVec && vecScore > 0:
			combinedScore = vecScore
			matchType = "vector"
		case hasKw && kwScore > 0:
			combinedScore = kwScore
			matchType = "keyword"
		case hasVec && vecScore == 0 && hasKw:
			combinedScore = kwWeight * kwScore
			matchType = "keyword"
		case hasKw && kwScore == 0 && hasVec:
			combinedScore = vecWeight * vecScore
			matchType = "vector"
		case hasVec && hasKw:
			combinedScore = vecWeight*vecScore + kwWeight*kwScore
			matchType = "hybrid"
		case hasVec:
			combinedScore = vecScore
			matchType = "vector"
		case hasKw:
			combinedScore = kwScore
			matchType = "keyword"
		default:
			continue
		}

		if combinedScore < opts.MinScore && opts.MinScore > 0 {
			continue
		}

		result := SearchResult{
			Entry:        entry,
			Score:        combinedScore,
			KeywordScore: kwScore,
			VectorScore:  vecScore,
			MatchType:    matchType,
		}

		if opts.ApplyTemporal {
			decay := temporalDecay(entry.Timestamp, opts.TemporalDecay)
			result.TemporalWeight = decay
			result.Score *= decay
		}

		results = append(results, result)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if opts.ApplyMMR && len(results) > opts.Limit {
		results = mmrSelect(results, opts.Limit, opts.MMRLambda)
	}

	if len(results) > opts.Limit {
		results = results[:opts.Limit]
	}

	return results
}

func normalizeScoresMinmax(scores map[string]float64) map[string]float64 {
	if len(scores) == 0 {
		return scores
	}

	minVal := math.MaxFloat64
	maxVal := -math.MaxFloat64
	for _, s := range scores {
		if s < minVal {
			minVal = s
		}
		if s > maxVal {
			maxVal = s
		}
	}

	if maxVal == minVal {
		normalized := make(map[string]float64, len(scores))
		for k := range scores {
			normalized[k] = 0.5
		}
		return normalized
	}

	rangeVal := maxVal - minVal
	normalized := make(map[string]float64, len(scores))
	for k, s := range scores {
		normalized[k] = (s - minVal) / rangeVal
	}
	return normalized
}

func bm25Search(entries []MemoryEntry, query string) map[string]float64 {
	scores := make(map[string]float64)
	queryTerms := tokenize(query)

	if len(queryTerms) == 0 {
		return scores
	}

	k1 := 1.5
	b := 0.75

	docTerms := make([]map[string]int, len(entries))
	docLengths := make([]int, len(entries))
	totalLength := 0

	for i, entry := range entries {
		terms := tokenize(entry.Content)
		termFreq := make(map[string]int)
		for _, term := range terms {
			termFreq[term]++
		}
		docTerms[i] = termFreq
		docLengths[i] = len(terms)
		totalLength += len(terms)
	}

	avgDocLength := float64(totalLength) / float64(len(entries))
	if avgDocLength == 0 {
		avgDocLength = 1
	}

	docFreq := make(map[string]int)
	for _, termFreq := range docTerms {
		seen := make(map[string]bool)
		for term := range termFreq {
			if !seen[term] {
				docFreq[term]++
				seen[term] = true
			}
		}
	}

	n := float64(len(entries))

	for i, entry := range entries {
		var score float64
		docLen := float64(docLengths[i])

		for _, qt := range queryTerms {
			df := float64(docFreq[qt])
			if df == 0 {
				continue
			}

			idf := math.Log((n-df+0.5)/(df+0.5) + 1.0)
			tf := float64(docTerms[i][qt])

			numerator := tf * (k1 + 1.0)
			denominator := tf + k1*(1.0-b+b*(docLen/avgDocLength))

			score += idf * (numerator / denominator)
		}

		lowerContent := strings.ToLower(entry.Content)
		lowerQuery := strings.ToLower(query)
		if strings.Contains(lowerContent, lowerQuery) {
			score *= 1.5
		}

		for _, val := range entry.Metadata {
			if strings.Contains(strings.ToLower(val), lowerQuery) {
				score += 0.3
			}
		}

		scores[entry.ID] = score
	}

	return scores
}

func keywordSearch(entries []MemoryEntry, query string) map[string]float64 {
	return bm25Search(entries, query)
}

func vectorSearch(entries []MemoryEntry, query string, opts SearchOptions) map[string]float64 {
	queryEmbedding, ok := resolveQueryEmbedding(query, opts)
	if !ok {
		return nil
	}

	entryEmbeddings, ok := resolveEntryEmbeddings(entries, opts)
	if !ok {
		return nil
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

func temporalDecay(timestamp time.Time, halfLifeHours float64) float64 {
	age := time.Since(timestamp).Hours()
	return math.Exp(-math.Ln2 * age / halfLifeHours)
}

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
