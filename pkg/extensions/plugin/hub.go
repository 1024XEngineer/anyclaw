package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type HubClient struct {
	baseURL    string
	httpClient *http.Client
}

type HubPlugin struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	Description string    `json:"description"`
	Author      string    `json:"author"`
	Tags        []string  `json:"tags"`
	Downloads   int       `json:"downloads"`
	Stars       int       `json:"stars"`
	Category    string    `json:"category"`
	License     string    `json:"license"`
	Repository  string    `json:"repository"`
	Homepage    string    `json:"homepage"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type HubSearchResult struct {
	Plugins []HubPlugin `json:"plugins"`
	Total   int         `json:"total"`
	Page    int         `json:"page"`
	Limit   int         `json:"limit"`
}

func NewHubClient(baseURL string) *HubClient {
	return &HubClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *HubClient) Search(ctx context.Context, query string, category string, limit int, offset int) (*HubSearchResult, error) {
	url := fmt.Sprintf("%s/api/v1/plugins/search?q=%s&limit=%d&offset=%d", c.baseURL, query, limit, offset)
	if category != "" {
		url += "&category=" + category
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result HubSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

func (c *HubClient) GetPlugin(ctx context.Context, pluginID string) (*HubPlugin, error) {
	url := fmt.Sprintf("%s/api/v1/plugins/%s", c.baseURL, pluginID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("plugin not found: %s", pluginID)
	}

	var plugin HubPlugin
	if err := json.NewDecoder(resp.Body).Decode(&plugin); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &plugin, nil
}

func (c *HubClient) Download(ctx context.Context, pluginID string, version string, dest string) error {
	url := fmt.Sprintf("%s/api/v1/plugins/%s/download?version=%s", c.baseURL, pluginID, version)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	return writeFileAtomic(dest, data)
}

func writeFileAtomic(path string, data []byte) error {
	return nil
}

type HubCategories struct {
	Categories []HubCategory `json:"categories"`
}

type HubCategory struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Count       int    `json:"count"`
}

func (c *HubClient) GetCategories(ctx context.Context) ([]HubCategory, error) {
	url := fmt.Sprintf("%s/api/v1/categories", c.baseURL)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var categories HubCategories
	if err := json.NewDecoder(resp.Body).Decode(&categories); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return categories.Categories, nil
}

func (c *HubClient) GetVersions(ctx context.Context, pluginID string) ([]string, error) {
	url := fmt.Sprintf("%s/api/v1/plugins/%s/versions", c.baseURL, pluginID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var versions []string
	if err := json.NewDecoder(resp.Body).Decode(&versions); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return versions, nil
}

type HubStats struct {
	TotalPlugins   int `json:"total_plugins"`
	TotalDownloads int `json:"total_downloads"`
	TotalStars     int `json:"total_stars"`
	TotalSigners   int `json:"total_signers"`
}

func (c *HubClient) GetStats(ctx context.Context) (*HubStats, error) {
	url := fmt.Sprintf("%s/api/v1/stats", c.baseURL)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var stats HubStats
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &stats, nil
}

type HubManager struct {
	hubClient  *HubClient
	localCache *LocalCache
}

type LocalCache struct {
	dir string
}

func (lc *LocalCache) GetPlugin(pluginID string) (*HubPlugin, bool) {
	return nil, false
}

func NewHubManager(baseURL string, cacheDir string) *HubManager {
	return &HubManager{
		hubClient:  NewHubClient(baseURL),
		localCache: &LocalCache{dir: cacheDir},
	}
}

func (hm *HubManager) SearchPlugins(ctx context.Context, query string, category string, limit int) ([]HubPlugin, error) {
	result, err := hm.hubClient.Search(ctx, query, category, limit, 0)
	if err != nil {
		return nil, err
	}
	return result.Plugins, nil
}

func (hm *HubManager) InstallPlugin(ctx context.Context, pluginID string, version string, installDir string) error {
	versions, err := hm.hubClient.GetVersions(ctx, pluginID)
	if err != nil {
		return err
	}

	targetVersion := version
	if targetVersion == "" && len(versions) > 0 {
		targetVersion = versions[0]
	}

	dest := installDir + "/" + pluginID + ".tar.gz"
	return hm.hubClient.Download(ctx, pluginID, targetVersion, dest)
}

func (hm *HubManager) UpdatePlugin(ctx context.Context, pluginID string, installDir string) error {
	versions, err := hm.hubClient.GetVersions(ctx, pluginID)
	if err != nil {
		return err
	}

	if len(versions) == 0 {
		return fmt.Errorf("no versions available for plugin %s", pluginID)
	}

	dest := installDir + "/" + pluginID + ".tar.gz"
	return hm.hubClient.Download(ctx, pluginID, versions[0], dest)
}
