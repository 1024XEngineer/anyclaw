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

type BaiduProvider struct {
	apiKey      string
	secretKey   string
	accessToken string
	model       string
	dimension   int
	httpClient  *http.Client
	tokenExpiry time.Time
}

type BaiduOption func(*BaiduProvider)

func WithBaiduModel(model string) BaiduOption {
	return func(p *BaiduProvider) { p.model = model }
}

func WithBaiduSecretKey(secret string) BaiduOption {
	return func(p *BaiduProvider) { p.secretKey = secret }
}

func NewBaiduProvider(apiKey string, opts ...BaiduOption) (*BaiduProvider, error) {
	p := &BaiduProvider{
		apiKey:     apiKey,
		model:      "embedding-v1",
		dimension:  384,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
	for _, opt := range opts {
		opt(p)
	}
	if p.apiKey == "" {
		return nil, fmt.Errorf("baidu: api key is required")
	}
	return p, nil
}

func (p *BaiduProvider) Name() string   { return "baidu" }
func (p *BaiduProvider) Dimension() int { return p.dimension }

func (p *BaiduProvider) getAccessToken(ctx context.Context) (string, error) {
	if p.accessToken != "" && time.Now().Before(p.tokenExpiry) {
		return p.accessToken, nil
	}

	url := fmt.Sprintf("https://aip.baidubce.com/oauth/2.0/token?grant_type=client_credentials&client_id=%s&client_secret=%s",
		p.apiKey, p.secretKey)

	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return "", fmt.Errorf("baidu: create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("baidu: token request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("baidu: token error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("baidu: parse token response: %w", err)
	}

	p.accessToken = result.AccessToken
	p.tokenExpiry = time.Now().Add(time.Duration(result.ExpiresIn-300) * time.Second)
	return p.accessToken, nil
}

func (p *BaiduProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	results, err := p.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("baidu: no embedding returned")
	}
	return results[0], nil
}

func (p *BaiduProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	token, err := p.getAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	payload := map[string]any{
		"input": texts,
	}
	body, _ := json.Marshal(payload)

	modelPath := p.model
	if strings.HasPrefix(p.model, "bge-large-zh") {
		modelPath = "bge_large_zh"
	}

	url := fmt.Sprintf("https://aip.baidubce.com/rpc/2.0/ai_custom/v1/wenxinworkshop/embeddings/%s?access_token=%s",
		modelPath, token)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("baidu: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("baidu: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("baidu: API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("baidu: parse response: %w", err)
	}

	embeddings := make([][]float32, len(result.Data))
	for _, d := range result.Data {
		embeddings[d.Index] = d.Embedding
	}
	return embeddings, nil
}
