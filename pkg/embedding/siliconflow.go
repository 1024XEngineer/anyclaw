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

type SiliconFlowProvider struct {
	apiKey     string
	model      string
	dimension  int
	httpClient *http.Client
}

type SiliconFlowOption func(*SiliconFlowProvider)

func WithSiliconFlowModel(model string) SiliconFlowOption {
	return func(p *SiliconFlowProvider) { p.model = model }
}

func NewSiliconFlowProvider(apiKey string, opts ...SiliconFlowOption) (*SiliconFlowProvider, error) {
	p := &SiliconFlowProvider{
		apiKey:     apiKey,
		model:      "BAAI/bge-m3",
		dimension:  1024,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
	for _, opt := range opts {
		opt(p)
	}
	if p.apiKey == "" {
		return nil, fmt.Errorf("siliconflow: api key is required")
	}
	return p, nil
}

func (p *SiliconFlowProvider) Name() string   { return "siliconflow" }
func (p *SiliconFlowProvider) Dimension() int { return p.dimension }

func (p *SiliconFlowProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	results, err := p.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("siliconflow: no embedding returned")
	}
	return results[0], nil
}

func (p *SiliconFlowProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	payload := map[string]any{
		"model":           p.model,
		"input":           texts,
		"encoding_format": "float",
	}
	body, _ := json.Marshal(payload)

	url := "https://api.siliconflow.cn/v1/embeddings"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("siliconflow: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("siliconflow: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("siliconflow: API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("siliconflow: parse response: %w", err)
	}

	embeddings := make([][]float32, len(result.Data))
	for _, d := range result.Data {
		embeddings[d.Index] = d.Embedding
	}
	return embeddings, nil
}
