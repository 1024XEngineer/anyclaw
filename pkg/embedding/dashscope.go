package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type DashScopeProvider struct {
	apiKey     string
	model      string
	dimension  int
	httpClient *http.Client
}

type DashScopeOption func(*DashScopeProvider)

func WithDashScopeModel(model string) DashScopeOption {
	return func(p *DashScopeProvider) { p.model = model }
}

func NewDashScopeProvider(apiKey string, opts ...DashScopeOption) (*DashScopeProvider, error) {
	p := &DashScopeProvider{
		apiKey:     apiKey,
		model:      "text-embedding-v3",
		dimension:  1024,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
	for _, opt := range opts {
		opt(p)
	}
	if p.apiKey == "" {
		return nil, fmt.Errorf("dashscope: api key is required")
	}
	return p, nil
}

func (p *DashScopeProvider) Name() string   { return "dashscope" }
func (p *DashScopeProvider) Dimension() int { return p.dimension }

func (p *DashScopeProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	results, err := p.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("dashscope: no embedding returned")
	}
	return results[0], nil
}

func (p *DashScopeProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	payload := map[string]any{
		"model": p.model,
		"input": map[string]any{
			"texts": texts,
		},
		"parameters": map[string]any{
			"text_type": "document",
		},
	}
	body, _ := json.Marshal(payload)

	url := "https://dashscope.aliyuncs.com/api/v1/services/embeddings/text-embedding/text-embedding"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("dashscope: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-DashScope-Async", "disable")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("dashscope: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dashscope: API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Output struct {
			Embeddings []struct {
				Index     int       `json:"text_index"`
				Embedding []float32 `json:"embedding"`
			} `json:"embeddings"`
		} `json:"output"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("dashscope: parse response: %w", err)
	}

	embeddings := make([][]float32, len(result.Output.Embeddings))
	for _, d := range result.Output.Embeddings {
		embeddings[d.Index] = d.Embedding
	}
	return embeddings, nil
}
