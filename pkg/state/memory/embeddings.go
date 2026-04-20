package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"
)

// EmbeddingProvider generates vector embeddings for text
type EmbeddingProvider interface {
	Embed(ctx context.Context, text string) ([]float64, error)
	EmbedBatch(ctx context.Context, texts []string) ([][]float64, error)
	Name() string
	Dimension() int
}

// OpenAIEmbeddingProvider uses OpenAI's embedding API
type OpenAIEmbeddingProvider struct {
	apiKey     string
	model      string
	baseURL    string
	dimension  int
	httpClient *http.Client
}

func NewOpenAIEmbeddingProvider(apiKey, model string) *OpenAIEmbeddingProvider {
	if model == "" {
		model = "text-embedding-3-small"
	}
	dim := 1536
	if strings.Contains(model, "large") {
		dim = 3072
	}
	return &OpenAIEmbeddingProvider{
		apiKey:     apiKey,
		model:      model,
		baseURL:    "https://api.openai.com/v1",
		dimension:  dim,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *OpenAIEmbeddingProvider) Name() string   { return "openai" }
func (p *OpenAIEmbeddingProvider) Dimension() int { return p.dimension }

func (p *OpenAIEmbeddingProvider) Embed(ctx context.Context, text string) ([]float64, error) {
	results, err := p.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}
	return results[0], nil
}

func (p *OpenAIEmbeddingProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float64, error) {
	payload := map[string]any{
		"model": p.model,
		"input": texts,
	}
	body, _ := json.Marshal(payload)

	url := p.baseURL + "/embeddings"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("embedding API error: %s", string(respBody))
	}

	var result struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	embeddings := make([][]float64, len(result.Data))
	for i, d := range result.Data {
		embeddings[i] = d.Embedding
	}
	return embeddings, nil
}

// OllamaEmbeddingProvider uses Ollama's embedding API
type OllamaEmbeddingProvider struct {
	baseURL    string
	model      string
	dimension  int
	httpClient *http.Client
}

func NewOllamaEmbeddingProvider(baseURL, model string) *OllamaEmbeddingProvider {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	if model == "" {
		model = "nomic-embed-text"
	}
	return &OllamaEmbeddingProvider{
		baseURL:    baseURL,
		model:      model,
		dimension:  768,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

func (p *OllamaEmbeddingProvider) Name() string   { return "ollama" }
func (p *OllamaEmbeddingProvider) Dimension() int { return p.dimension }

func (p *OllamaEmbeddingProvider) Embed(ctx context.Context, text string) ([]float64, error) {
	results, err := p.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}
	return results[0], nil
}

func (p *OllamaEmbeddingProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float64, error) {
	var embeddings [][]float64
	for _, text := range texts {
		payload := map[string]any{
			"model":  p.model,
			"prompt": text,
		}
		body, _ := json.Marshal(payload)

		url := p.baseURL + "/api/embeddings"
		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := p.httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var result struct {
			Embedding []float64 `json:"embedding"`
		}
		if err := json.Unmarshal(respBody, &result); err != nil {
			return nil, err
		}
		embeddings = append(embeddings, result.Embedding)
	}
	return embeddings, nil
}

// EmbeddingManager manages embedding providers with fallback
type EmbeddingManager struct {
	providers []EmbeddingProvider
	current   int
	cache     map[string][]float64
	cacheMu   sync.RWMutex
	maxCache  int
}

func NewEmbeddingManager(providers ...EmbeddingProvider) *EmbeddingManager {
	return &EmbeddingManager{
		providers: providers,
		cache:     make(map[string][]float64),
		maxCache:  10000,
	}
}

func (em *EmbeddingManager) Embed(ctx context.Context, text string) ([]float64, error) {
	// Check cache
	em.cacheMu.RLock()
	if cached, ok := em.cache[text]; ok {
		em.cacheMu.RUnlock()
		return cached, nil
	}
	em.cacheMu.RUnlock()

	// Try providers in order
	var lastErr error
	for i, provider := range em.providers {
		embedding, err := provider.Embed(ctx, text)
		if err == nil {
			em.current = i
			// Cache result
			em.cacheMu.Lock()
			if len(em.cache) >= em.maxCache {
				// Simple eviction: clear half
				for k := range em.cache {
					delete(em.cache, k)
					if len(em.cache) < em.maxCache/2 {
						break
					}
				}
			}
			em.cache[text] = embedding
			em.cacheMu.Unlock()
			return embedding, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("all embedding providers failed: %w", lastErr)
}

func (em *EmbeddingManager) EmbedBatch(ctx context.Context, texts []string) ([][]float64, error) {
	var results [][]float64
	for _, text := range texts {
		embedding, err := em.Embed(ctx, text)
		if err != nil {
			return nil, err
		}
		results = append(results, embedding)
	}
	return results, nil
}

func (em *EmbeddingManager) GetCurrentProvider() EmbeddingProvider {
	if len(em.providers) == 0 {
		return nil
	}
	return em.providers[em.current]
}

// MMR performs Maximal Marginal Relevance re-ranking for diversity
func MMR(query []float64, candidates []VectorEntry, lambda float64, limit int) []VectorEntry {
	if len(candidates) <= limit {
		return candidates
	}

	selected := make([]VectorEntry, 0, limit)
	remaining := make([]VectorEntry, len(candidates))
	copy(remaining, candidates)

	for len(selected) < limit && len(remaining) > 0 {
		bestIdx := 0
		bestScore := -math.MaxFloat64

		for i, candidate := range remaining {
			// Relevance to query
			relevance := CosineSimilarity(query, candidate.Embedding)

			// Max similarity to already selected
			maxSim := 0.0
			for _, sel := range selected {
				sim := CosineSimilarity(candidate.Embedding, sel.Embedding)
				if sim > maxSim {
					maxSim = sim
				}
			}

			// MMR score
			score := lambda*relevance - (1-lambda)*maxSim
			if score > bestScore {
				bestScore = score
				bestIdx = i
			}
		}

		selected = append(selected, remaining[bestIdx])
		remaining = append(remaining[:bestIdx], remaining[bestIdx+1:]...)
	}

	return selected
}

// TemporalDecay applies time-based decay to search scores
func TemporalDecay(entryTime time.Time, halfLifeDays float64) float64 {
	if halfLifeDays <= 0 {
		return 1.0
	}
	daysSince := time.Since(entryTime).Hours() / 24.0
	return math.Pow(0.5, daysSince/halfLifeDays)
}

// CosineSimilarity calculates cosine similarity
func CosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
