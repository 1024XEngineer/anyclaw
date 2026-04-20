package qmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

type Protocol string

const (
	ProtocolHTTP      Protocol = "http"
	ProtocolUnix      Protocol = "unix"
	DefaultTimeout             = 10 * time.Second
	DefaultRetryCount          = 3
	DefaultRetryDelay          = 100 * time.Millisecond
)

type ClientConfig struct {
	Address    string
	Protocol   Protocol
	Timeout    time.Duration
	RetryCount int
	RetryDelay time.Duration
}

func DefaultClientConfig() ClientConfig {
	return ClientConfig{
		Address:    "http://localhost:9876",
		Protocol:   ProtocolHTTP,
		Timeout:    DefaultTimeout,
		RetryCount: DefaultRetryCount,
		RetryDelay: DefaultRetryDelay,
	}
}

func UnixClientConfig(socketPath string) ClientConfig {
	return ClientConfig{
		Address:    socketPath,
		Protocol:   ProtocolUnix,
		Timeout:    DefaultTimeout,
		RetryCount: DefaultRetryCount,
		RetryDelay: DefaultRetryDelay,
	}
}

type Client struct {
	config ClientConfig
	client *http.Client
}

func NewClient(cfg ClientConfig) *Client {
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	if cfg.RetryCount < 0 {
		cfg.RetryCount = DefaultRetryCount
	}
	if cfg.RetryDelay <= 0 {
		cfg.RetryDelay = DefaultRetryDelay
	}

	httpClient := &http.Client{
		Timeout: cfg.Timeout,
	}

	if cfg.Protocol == ProtocolUnix {
		httpClient.Transport = &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return net.DialTimeout("unix", cfg.Address, cfg.Timeout)
			},
		}
	}

	return &Client{
		config: cfg,
		client: httpClient,
	}
}

func (c *Client) Ping(ctx context.Context) error {
	_, err := c.do(ctx, "GET", "/v1/health", nil)
	return err
}

func (c *Client) CreateTable(ctx context.Context, name string, columns []string) error {
	reqBody := map[string]any{
		"name":    name,
		"columns": columns,
	}
	_, err := c.do(ctx, "POST", "/v1/tables", reqBody)
	return err
}

func (c *Client) DropTable(ctx context.Context, name string) error {
	_, err := c.do(ctx, "DELETE", fmt.Sprintf("/v1/tables/%s", name), nil)
	return err
}

func (c *Client) ListTables(ctx context.Context) ([]TableStat, error) {
	body, err := c.do(ctx, "GET", "/v1/tables", nil)
	if err != nil {
		return nil, err
	}

	var tables []TableStat
	if err := json.Unmarshal(body, &tables); err != nil {
		return nil, fmt.Errorf("qmd: parse tables: %w", err)
	}
	return tables, nil
}

func (c *Client) Insert(ctx context.Context, table string, record *Record) error {
	reqBody := map[string]any{
		"id":   record.ID,
		"data": record.Data,
	}
	_, err := c.do(ctx, "POST", fmt.Sprintf("/v1/tables/%s/records", table), reqBody)
	return err
}

func (c *Client) InsertBatch(ctx context.Context, table string, records []*Record) error {
	if len(records) == 0 {
		return nil
	}

	for _, r := range records {
		if err := c.Insert(ctx, table, r); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) Get(ctx context.Context, table, id string) (*Record, error) {
	body, err := c.do(ctx, "GET", fmt.Sprintf("/v1/tables/%s/records/%s", table, id), nil)
	if err != nil {
		return nil, err
	}

	var record Record
	if err := json.Unmarshal(body, &record); err != nil {
		return nil, fmt.Errorf("qmd: parse record: %w", err)
	}
	return &record, nil
}

func (c *Client) Update(ctx context.Context, table string, record *Record) error {
	reqBody := map[string]any{
		"data": record.Data,
	}
	_, err := c.do(ctx, "PUT", fmt.Sprintf("/v1/tables/%s/records/%s", table, record.ID), reqBody)
	return err
}

func (c *Client) Delete(ctx context.Context, table, id string) error {
	_, err := c.do(ctx, "DELETE", fmt.Sprintf("/v1/tables/%s/records/%s", table, id), nil)
	return err
}

func (c *Client) List(ctx context.Context, table string, limit int) ([]*Record, error) {
	path := fmt.Sprintf("/v1/tables/%s/records", table)
	if limit > 0 {
		path += fmt.Sprintf("?limit=%d", limit)
	}

	body, err := c.do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var records []*Record
	if err := json.Unmarshal(body, &records); err != nil {
		return nil, fmt.Errorf("qmd: parse records: %w", err)
	}
	return records, nil
}

func (c *Client) Query(ctx context.Context, table, field string, value any, limit int) ([]*Record, error) {
	path := fmt.Sprintf("/v1/tables/%s/query?field=%s&value=%v", table, field, value)
	if limit > 0 {
		path += fmt.Sprintf("&limit=%d", limit)
	}

	body, err := c.do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var records []*Record
	if err := json.Unmarshal(body, &records); err != nil {
		return nil, fmt.Errorf("qmd: parse query results: %w", err)
	}
	return records, nil
}

func (c *Client) Count(ctx context.Context, table string) (int, error) {
	body, err := c.do(ctx, "GET", fmt.Sprintf("/v1/tables/%s/count", table), nil)
	if err != nil {
		return 0, err
	}

	var resp struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return 0, fmt.Errorf("qmd: parse count: %w", err)
	}
	return resp.Count, nil
}

func (c *Client) Stats(ctx context.Context) (Stats, error) {
	body, err := c.do(ctx, "GET", "/v1/stats", nil)
	if err != nil {
		return Stats{}, err
	}

	var stats Stats
	if err := json.Unmarshal(body, &stats); err != nil {
		return Stats{}, fmt.Errorf("qmd: parse stats: %w", err)
	}
	return stats, nil
}

func (c *Client) WAL(ctx context.Context, since string) ([]*WALEntry, error) {
	path := "/v1/wal"
	if since != "" {
		path += fmt.Sprintf("?since=%s", since)
	}

	body, err := c.do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var entries []*WALEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("qmd: parse WAL: %w", err)
	}
	return entries, nil
}

func (c *Client) TruncateWAL(ctx context.Context) error {
	_, err := c.do(ctx, "POST", "/v1/wal/truncate", nil)
	return err
}

func (c *Client) Clear(ctx context.Context) error {
	_, err := c.do(ctx, "POST", "/v1/clear", nil)
	return err
}

func (c *Client) do(ctx context.Context, method, path string, body any) ([]byte, error) {
	var lastErr error

	for attempt := 0; attempt <= c.config.RetryCount; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(c.config.RetryDelay * time.Duration(attempt)):
			}
		}

		result, err := c.doOnce(ctx, method, path, body)
		if err == nil {
			return result, nil
		}
		lastErr = err
	}

	return nil, fmt.Errorf("qmd: request failed after %d retries: %w", c.config.RetryCount, lastErr)
}

func (c *Client) doOnce(ctx context.Context, method, path string, body any) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("qmd: marshal request: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	var url string
	if c.config.Protocol == ProtocolUnix {
		url = "http://unix" + path
	} else {
		url = c.config.Address + path
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("qmd: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("qmd: request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("qmd: read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != "" {
			return nil, fmt.Errorf("qmd: %s", errResp.Error)
		}
		return nil, fmt.Errorf("qmd: HTTP %d", resp.StatusCode)
	}

	return respBody, nil
}
