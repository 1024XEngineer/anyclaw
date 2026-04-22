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

type VoyageProvider struct {
	apiKey     string
	model      string
	dimension  int
	httpClient *http.Client
}

type VoyageOption func(*VoyageProvider)

func WithVoyageModel(model string) VoyageOption {
	return func(p *VoyageProvider) { p.model = model }
}

func NewVoyageProvider(apiKey string, opts ...VoyageOption) (*VoyageProvider, error) {
	p := &VoyageProvider{
		apiKey:     apiKey,
		model:      "voyage-3",
		dimension:  1024,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
	for _, opt := range opts {
		opt(p)
	}
	if p.apiKey == "" {
		return nil, fmt.Errorf("voyage: api key is required")
	}
	return p, nil
}

func (p *VoyageProvider) Name() string   { return "voyage" }
func (p *VoyageProvider) Dimension() int { return p.dimension }

func (p *VoyageProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	results, err := p.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("voyage: no embedding returned")
	}
	return results[0], nil
}

func (p *VoyageProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	payload := map[string]any{
		"model": p.model,
		"input": texts,
	}
	body, _ := json.Marshal(payload)

	url := "https://api.voyageai.com/v1/embeddings"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("voyage: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("voyage: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("voyage: API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("voyage: parse response: %w", err)
	}

	embeddings := make([][]float32, len(result.Data))
	for _, d := range result.Data {
		embeddings[d.Index] = d.Embedding
	}
	return embeddings, nil
}
