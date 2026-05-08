package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/config"
	"github.com/1024XEngineer/anyclaw/pkg/marketplace"
)

var (
	ErrRemoteDisabled = errors.New("marketplace remote registry is disabled")
	ErrNotConfigured  = errors.New("marketplace registry endpoint is not configured")
)

type Client struct {
	endpoint        string
	token           string
	protocolVersion string
	httpClient      *http.Client
	retryCount      int
	cacheTTL        time.Duration

	mu    sync.Mutex
	cache map[string]cacheEntry
}

type ClientConfig struct {
	Endpoint        string
	Token           string
	ProtocolVersion string
	Timeout         time.Duration
	RetryCount      int
	CacheTTL        time.Duration
}

type cacheEntry struct {
	expiresAt time.Time
	data      []byte
}

func NewClient(cfg ClientConfig) *Client {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	protocolVersion := strings.TrimSpace(cfg.ProtocolVersion)
	if protocolVersion == "" {
		protocolVersion = "1.0"
	}
	if cfg.RetryCount < 0 {
		cfg.RetryCount = 0
	}
	return &Client{
		endpoint:        normalizeEndpoint(cfg.Endpoint),
		token:           strings.TrimSpace(cfg.Token),
		protocolVersion: protocolVersion,
		httpClient:      &http.Client{Timeout: timeout},
		retryCount:      cfg.RetryCount,
		cacheTTL:        cfg.CacheTTL,
		cache:           map[string]cacheEntry{},
	}
}

func normalizeEndpoint(endpoint string) string {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if strings.HasSuffix(endpoint, "/v1") {
		return strings.TrimSuffix(endpoint, "/v1")
	}
	return endpoint
}

func NewClientFromConfig(cfg config.MarketplaceConfig) *Client {
	return NewClient(ClientConfig{
		Endpoint:        cfg.RegistryEndpoint,
		Token:           cfg.RegistryToken,
		ProtocolVersion: cfg.ProtocolVersion,
		Timeout:         time.Duration(cfg.RequestTimeoutSeconds) * time.Second,
		RetryCount:      cfg.RetryCount,
		CacheTTL:        time.Duration(cfg.CacheTTLSeconds) * time.Second,
	})
}

func IsEnabled(cfg config.MarketplaceConfig) bool {
	return !cfg.DisableRemote && strings.TrimSpace(cfg.RegistryEndpoint) != ""
}

func (c *Client) List(ctx context.Context, filter marketplace.Filter) (marketplace.ListResult, error) {
	values := url.Values{}
	if filter.Kind != "" {
		values.Set("kind", string(filter.Kind))
	}
	if filter.Query != "" {
		values.Set("q", filter.Query)
	}
	if filter.Risk != "" {
		values.Set("risk", filter.Risk)
	}
	if filter.Trust != "" {
		values.Set("trust", filter.Trust)
	}
	if filter.Tag != "" {
		values.Set("tag", filter.Tag)
	}
	if filter.Permission != "" {
		values.Set("permission", filter.Permission)
	}
	if filter.Publisher != "" {
		values.Set("publisher", filter.Publisher)
	}
	if filter.OS != "" {
		values.Set("os", filter.OS)
	}
	if filter.Arch != "" {
		values.Set("arch", filter.Arch)
	}
	if filter.Sort != "" {
		values.Set("sort", filter.Sort)
	}
	if filter.Limit > 0 {
		values.Set("limit", fmt.Sprint(filter.Limit))
	}
	if filter.Offset > 0 {
		values.Set("offset", fmt.Sprint(filter.Offset))
	}
	var envelope listEnvelope
	if err := c.get(ctx, "/v1/artifacts?"+values.Encode(), &envelope); err != nil {
		return marketplace.ListResult{}, err
	}
	items := make([]marketplace.Artifact, 0, len(envelope.Data.Items))
	for _, item := range envelope.Data.Items {
		items = append(items, convertArtifact(item))
	}
	return marketplace.ListResult{
		Items:  items,
		Total:  envelope.Data.Total,
		Limit:  envelope.Data.Limit,
		Offset: envelope.Data.Offset,
	}, nil
}

func (c *Client) Get(ctx context.Context, id string) (*marketplace.Artifact, error) {
	var envelope artifactEnvelope
	if err := c.get(ctx, "/v1/artifacts/"+url.PathEscape(strings.TrimSpace(id)), &envelope); err != nil {
		return nil, err
	}
	item := convertArtifact(envelope.Data)
	return &item, nil
}

func (c *Client) Versions(ctx context.Context, id string) ([]marketplace.ArtifactVersion, error) {
	var envelope versionsEnvelope
	if err := c.get(ctx, "/v1/artifacts/"+url.PathEscape(strings.TrimSpace(id))+"/versions", &envelope); err != nil {
		return nil, err
	}
	items := make([]marketplace.ArtifactVersion, 0, len(envelope.Data.Items))
	for _, item := range envelope.Data.Items {
		items = append(items, convertVersion(item))
	}
	return items, nil
}

func (c *Client) Resolve(ctx context.Context, id string, req ResolveRequest) (ResolvedArtifact, error) {
	var envelope resolveEnvelope
	if err := c.post(ctx, "/v1/artifacts/"+url.PathEscape(strings.TrimSpace(id))+"/resolve", req, &envelope); err != nil {
		return ResolvedArtifact{}, err
	}
	return envelope.Data, nil
}

func (c *Client) Download(ctx context.Context, rawURL string) ([]byte, error) {
	if c == nil {
		return nil, ErrNotConfigured
	}
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, fmt.Errorf("download url is required")
	}
	var lastErr error
	attempts := c.retryCount + 1
	for attempt := 0; attempt < attempts; attempt++ {
		data, err := c.downloadOnce(ctx, rawURL)
		if err == nil {
			return data, nil
		}
		lastErr = err
		if !retryable(err) || attempt == attempts-1 {
			break
		}
	}
	return nil, lastErr
}

func (c *Client) get(ctx context.Context, path string, dst any) error {
	if c == nil {
		return ErrNotConfigured
	}
	if c.endpoint == "" {
		return ErrNotConfigured
	}
	cacheKey := "GET " + path
	if data, ok := c.cached(cacheKey); ok {
		return json.Unmarshal(data, dst)
	}
	data, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	c.storeCache(cacheKey, data)
	return json.Unmarshal(data, dst)
}

func (c *Client) post(ctx context.Context, path string, body any, dst any) error {
	if c == nil {
		return ErrNotConfigured
	}
	if c.endpoint == "" {
		return ErrNotConfigured
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	data, err := c.do(ctx, http.MethodPost, path, payload)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}

func (c *Client) do(ctx context.Context, method, path string, payload []byte) ([]byte, error) {
	var lastErr error
	attempts := c.retryCount + 1
	for attempt := 0; attempt < attempts; attempt++ {
		data, err := c.doOnce(ctx, method, path, payload)
		if err == nil {
			return data, nil
		}
		lastErr = err
		if !retryable(err) || attempt == attempts-1 {
			break
		}
	}
	return nil, lastErr
}

func (c *Client) doOnce(ctx context.Context, method, path string, payload []byte) ([]byte, error) {
	body := io.Reader(nil)
	if payload != nil {
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.endpoint+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-AnyClaw-Protocol-Version", c.protocolVersion)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, remoteStatusError{StatusCode: resp.StatusCode, Body: string(data)}
	}
	return data, nil
}

func (c *Client) downloadOnce(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 512<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, remoteStatusError{StatusCode: resp.StatusCode, Body: string(data)}
	}
	return data, nil
}

func (c *Client) cached(key string) ([]byte, bool) {
	if c.cacheTTL <= 0 {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.cache[key]
	if !ok || time.Now().After(entry.expiresAt) {
		delete(c.cache, key)
		return nil, false
	}
	return append([]byte(nil), entry.data...), true
}

func (c *Client) storeCache(key string, data []byte) {
	if c.cacheTTL <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[key] = cacheEntry{
		expiresAt: time.Now().Add(c.cacheTTL),
		data:      append([]byte(nil), data...),
	}
}

func retryable(err error) bool {
	var status remoteStatusError
	if errors.As(err, &status) {
		return status.StatusCode == http.StatusTooManyRequests || status.StatusCode >= 500
	}
	return true
}

type remoteStatusError struct {
	StatusCode int
	Body       string
}

func (e remoteStatusError) Error() string {
	return fmt.Sprintf("marketplace registry returned HTTP %d", e.StatusCode)
}
