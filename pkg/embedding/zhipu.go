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

type ZhipuProvider struct {
	apiKey     string
	model      string
	dimension  int
	httpClient *http.Client
}

type ZhipuOption func(*ZhipuProvider)

func WithZhipuModel(model string) ZhipuOption {
	return func(p *ZhipuProvider) { p.model = model }
}

func NewZhipuProvider(apiKey string, opts ...ZhipuOption) (*ZhipuProvider, error) {
	p := &ZhipuProvider{
		apiKey:     apiKey,
		model:      "embedding-3",
		dimension:  2048,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
	for _, opt := range opts {
		opt(p)
	}
	if p.apiKey == "" {
		return nil, fmt.Errorf("zhipu: api key is required")
	}
	return p, nil
}

func (p *ZhipuProvider) Name() string   { return "zhipu" }
func (p *ZhipuProvider) Dimension() int { return p.dimension }

func (p *ZhipuProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	results, err := p.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("zhipu: no embedding returned")
	}
	return results[0], nil
}

func (p *ZhipuProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	payload := map[string]any{
		"model": p.model,
		"input": texts,
	}
	body, _ := json.Marshal(payload)

	url := "https://open.bigmodel.cn/api/paas/v4/embeddings"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("zhipu: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("zhipu: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("zhipu: API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("zhipu: parse response: %w", err)
	}

	embeddings := make([][]float32, len(result.Data))
	for _, d := range result.Data {
		embeddings[d.Index] = d.Embedding
	}
	return embeddings, nil
}
