package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type atomicTempFile interface {
	Name() string
	io.Writer
	Sync() error
	Close() error
}

type atomicFileWriter struct {
	mkdirAll   func(string, os.FileMode) error
	createTemp func(string, string) (atomicTempFile, error)
	remove     func(string) error
	rename     func(string, string) error
}

func newAtomicFileWriter() atomicFileWriter {
	return atomicFileWriter{
		mkdirAll: os.MkdirAll,
		createTemp: func(dir, pattern string) (atomicTempFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		remove: os.Remove,
		rename: os.Rename,
	}
}

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

type HubCategories struct {
	Categories []HubCategory `json:"categories"`
}

type HubCategory struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Count       int    `json:"count"`
}

type HubStats struct {
	TotalPlugins   int `json:"total_plugins"`
	TotalDownloads int `json:"total_downloads"`
	TotalStars     int `json:"total_stars"`
	TotalSigners   int `json:"total_signers"`
}

type HubManager struct {
	hubClient  *HubClient
	localCache *LocalCache
}

type LocalCache struct {
	dir string
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
	values := url.Values{
		"q":      []string{query},
		"limit":  []string{fmt.Sprintf("%d", limit)},
		"offset": []string{fmt.Sprintf("%d", offset)},
	}
	if category != "" {
		values.Set("category", category)
	}

	requestURL, err := c.buildURL([]string{"plugins", "search"}, values)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", requestURL, nil)
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
	requestURL, err := c.buildURL([]string{"plugins", pluginID}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", requestURL, nil)
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
	requestURL, err := c.buildURL([]string{"plugins", pluginID, "download"}, url.Values{
		"version": []string{version},
	})
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", requestURL, nil)
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

	return writeReaderAtomic(dest, resp.Body)
}

func writeFileAtomic(path string, data []byte) error {
	return newAtomicFileWriter().write(path, bytes.NewReader(data))
}

func writeReaderAtomic(path string, src io.Reader) error {
	return newAtomicFileWriter().write(path, src)
}

func (w atomicFileWriter) write(path string, src io.Reader) error {
	if err := w.mkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create destination dir: %w", err)
	}

	tempFile, err := w.createTemp(filepath.Dir(path), ".hub-download-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tempPath := tempFile.Name()
	success := false
	defer func() {
		_ = tempFile.Close()
		if !success {
			_ = w.remove(tempPath)
		}
	}()

	if _, err := io.Copy(tempFile, src); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tempFile.Sync(); err != nil {
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := w.rename(tempPath, path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}

	success = true
	return nil
}

func (c *HubClient) GetCategories(ctx context.Context) ([]HubCategory, error) {
	requestURL, err := c.buildURL([]string{"categories"}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", requestURL, nil)
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
	requestURL, err := c.buildURL([]string{"plugins", pluginID, "versions"}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", requestURL, nil)
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

func (c *HubClient) GetStats(ctx context.Context) (*HubStats, error) {
	requestURL, err := c.buildURL([]string{"stats"}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", requestURL, nil)
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

func (c *HubClient) buildURL(segments []string, query url.Values) (string, error) {
	baseURL, err := url.Parse(c.baseURL)
	if err != nil {
		return "", err
	}
	baseURL.RawQuery = ""
	baseURL.Fragment = ""

	apiPath := append([]string{"api", "v1"}, segments...)
	escapedSegments := make([]string, 0, len(apiPath))
	for _, segment := range apiPath {
		escapedSegments = append(escapedSegments, url.PathEscape(segment))
	}

	base := strings.TrimRight(baseURL.String(), "/")
	requestURL := base + "/" + strings.Join(escapedSegments, "/")
	if query != nil {
		requestURL += "?" + query.Encode()
	}

	return requestURL, nil
}

func (lc *LocalCache) GetPlugin(pluginID string) (*HubPlugin, bool) {
	if lc == nil || lc.dir == "" || pluginID == "" {
		return nil, false
	}

	data, err := os.ReadFile(filepath.Join(lc.dir, pluginID+".json"))
	if err != nil {
		return nil, false
	}

	var plugin HubPlugin
	if err := json.Unmarshal(data, &plugin); err != nil {
		return nil, false
	}

	return &plugin, true
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
	if targetVersion == "" {
		return fmt.Errorf("no versions available for plugin %s", pluginID)
	}

	dest := filepath.Join(installDir, pluginID+".tar.gz")
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

	dest := filepath.Join(installDir, pluginID+".tar.gz")
	return hm.hubClient.Download(ctx, pluginID, versions[0], dest)
}
