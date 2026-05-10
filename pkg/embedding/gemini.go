package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type GeminiProvider struct {
	apiKey     string
	model      string
	dimension  int
	httpClient *http.Client
}

type GeminiOption func(*GeminiProvider)

func WithGeminiModel(model string) GeminiOption {
	return func(p *GeminiProvider) {
		p.model = normalizeGeminiModel(model)
		if dim := geminiModelDimension(p.model); dim > 0 {
			p.dimension = dim
		}
	}
}

func NewGeminiProvider(apiKey string, opts ...GeminiOption) (*GeminiProvider, error) {
	p := &GeminiProvider{
		apiKey:     apiKey,
		model:      "models/text-embedding-004",
		dimension:  768,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
	for _, opt := range opts {
		opt(p)
	}
	if p.apiKey == "" {
		return nil, fmt.Errorf("gemini: api key is required")
	}
	return p, nil
}

func (p *GeminiProvider) Name() string   { return "gemini" }
func (p *GeminiProvider) Dimension() int { return p.dimension }

func (p *GeminiProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	results, err := p.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("gemini: no embedding returned")
	}
	return results[0], nil
}

func (p *GeminiProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	var embeddings [][]float32
	for _, text := range texts {
		emb, err := p.embedSingle(ctx, text)
		if err != nil {
			return nil, err
		}
		embeddings = append(embeddings, emb)
	}
	return embeddings, nil
}

func (p *GeminiProvider) embedSingle(ctx context.Context, text string) ([]float32, error) {
	payload := map[string]any{
		"model": p.model,
		"content": map[string]any{
			"parts": []any{
				map[string]string{"text": text},
			},
		},
	}
	body, _ := json.Marshal(payload)

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/%s:embedContent?key=%s", p.model, p.apiKey)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("gemini: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gemini: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini: API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Embedding struct {
			Values []float32 `json:"values"`
		} `json:"embedding"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("gemini: parse response: %w", err)
	}

	return result.Embedding.Values, nil
}

func normalizeGeminiModel(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return "models/text-embedding-004"
	}
	if !strings.Contains(model, "/") {
		return "models/" + model
	}
	return model
}

func geminiModelDimension(model string) int {
	switch normalizeGeminiModel(model) {
	case "models/gemini-embedding-001":
		return 3072
	case "models/text-embedding-004":
		return 768
	default:
		return 0
	}
}
