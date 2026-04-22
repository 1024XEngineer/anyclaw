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

type OllamaProvider struct {
	baseURL    string
	model      string
	dimension  int
	httpClient *http.Client
}

type OllamaOption func(*OllamaProvider)

func WithOllamaBaseURL(url string) OllamaOption {
	return func(p *OllamaProvider) { p.baseURL = url }
}

func WithOllamaModel(model string) OllamaOption {
	return func(p *OllamaProvider) { p.model = model }
}

func NewOllamaProvider(opts ...OllamaOption) *OllamaProvider {
	p := &OllamaProvider{
		baseURL:    "http://localhost:11434",
		model:      "nomic-embed-text",
		dimension:  768,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func (p *OllamaProvider) Name() string   { return "ollama" }
func (p *OllamaProvider) Dimension() int { return p.dimension }

func (p *OllamaProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	results, err := p.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("ollama: no embedding returned")
	}
	return results[0], nil
}

func (p *OllamaProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	var embeddings [][]float32
	for _, text := range texts {
		payload := map[string]any{
			"model": p.model,
			"input": text,
		}
		body, _ := json.Marshal(payload)

		url := p.baseURL + "/api/embed"
		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("ollama: create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := p.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("ollama: request failed: %w", err)
		}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("ollama: API error %d: %s", resp.StatusCode, string(respBody))
		}

		var result struct {
			Embeddings [][]float32 `json:"embeddings"`
		}
		if err := json.Unmarshal(respBody, &result); err != nil {
			return nil, fmt.Errorf("ollama: parse response: %w", err)
		}

		if len(result.Embeddings) > 0 {
			embeddings = append(embeddings, result.Embeddings[0])
		}
	}
	return embeddings, nil
}
