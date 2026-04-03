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

type OpenAIProvider struct {
	apiKey     string
	model      string
	baseURL    string
	dimension  int
	httpClient *http.Client
}

type OpenAIOption func(*OpenAIProvider)

func WithOpenAIBaseURL(url string) OpenAIOption {
	return func(p *OpenAIProvider) { p.baseURL = url }
}

func WithOpenAIModel(model string) OpenAIOption {
	return func(p *OpenAIProvider) { p.model = model }
}

func WithOpenAIDimension(dim int) OpenAIOption {
	return func(p *OpenAIProvider) { p.dimension = dim }
}

func NewOpenAIProvider(apiKey string, opts ...OpenAIOption) (*OpenAIProvider, error) {
	p := &OpenAIProvider{
		apiKey:     apiKey,
		model:      "text-embedding-3-small",
		baseURL:    "https://api.openai.com/v1",
		dimension:  1536,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
	for _, opt := range opts {
		opt(p)
	}
	if p.apiKey == "" {
		return nil, fmt.Errorf("openai: api key is required")
	}
	return p, nil
}

func (p *OpenAIProvider) Name() string   { return "openai" }
func (p *OpenAIProvider) Dimension() int { return p.dimension }

func (p *OpenAIProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	results, err := p.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("openai: no embedding returned")
	}
	return results[0], nil
}

func (p *OpenAIProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	payload := map[string]any{
		"model":           p.model,
		"input":           texts,
		"encoding_format": "float",
	}
	if p.dimension > 0 && p.model != "text-embedding-ada-002" {
		payload["dimensions"] = p.dimension
	}
	body, _ := json.Marshal(payload)

	url := p.baseURL + "/embeddings"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai: API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
		Usage struct {
			PromptTokens int `json:"prompt_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("openai: parse response: %w", err)
	}

	embeddings := make([][]float32, len(result.Data))
	for _, d := range result.Data {
		embeddings[d.Index] = d.Embedding
	}
	return embeddings, nil
}
