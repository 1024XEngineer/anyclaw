package zotero

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	httpClient  *http.Client
	apiKey      string
	userID      string
	baseURL     string
	libraryPath string
}

type Config struct {
	APIKey      string
	UserID      string
	LibraryPath string
}

func NewClient(cfg Config) *Client {
	baseURL := "https://api.zotero.org"
	if cfg.UserID == "" {
		cfg.UserID = os.Getenv("ZOTERO_USER_ID")
	}
	if cfg.APIKey == "" {
		cfg.APIKey = os.Getenv("ZOTERO_API_KEY")
	}
	if cfg.LibraryPath == "" {
		cfg.LibraryPath = os.Getenv("ZOTERO_LIBRARY_PATH")
	}

	return &Client{
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		apiKey:      cfg.APIKey,
		userID:      cfg.UserID,
		baseURL:     baseURL,
		libraryPath: cfg.LibraryPath,
	}
}

type Item struct {
	Key          string         `json:"key"`
	Version      int            `json:"version"`
	Library      map[string]any `json:"library"`
	Parent       string         `json:"parent,omitempty"`
	ItemType     string         `json:"itemType"`
	Title        string         `json:"title"`
	Creators     []Creator      `json:"creators,omitempty"`
	Tags         []Tag          `json:"tags,omitempty"`
	Collections  []string       `json:"collections,omitempty"`
	Date         string         `json:"date,omitempty"`
	URL          string         `json:"url,omitempty"`
	AbstractNote string         `json:"abstractNote,omitempty"`
	Extra        string         `json:"extra,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

type Creator struct {
	CreatorType string `json:"creatorType"`
	FirstName   string `json:"firstName"`
	LastName    string `json:"lastName"`
	Name        string `json:"name,omitempty"`
}

type Tag struct {
	Tag  string `json:"tag"`
	Type int    `json:"type"`
}

type Collection struct {
	Key      string   `json:"key"`
	Name     string   `json:"name"`
	Parent   string   `json:"parent,omitempty"`
	Children []string `json:"children,omitempty"`
}

type SearchResult struct {
	Items   []Item `json:"items"`
	Total   int    `json:"total"`
	Library string `json:"library"`
}

func (c *Client) ListItems(ctx context.Context, collectionKey string, limit int) ([]Item, error) {
	url := fmt.Sprintf("%s/users/%s/items", c.baseURL, c.userID)
	if collectionKey != "" {
		url = fmt.Sprintf("%s/users/%s/collections/%s/items", c.baseURL, c.userID, collectionKey)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Zotero-API-Key", c.apiKey)
	req.Header.Set("Zotero-API-Version", "3")
	if limit > 0 {
		req.Header.Set("Limit", strconv.Itoa(limit))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("zotero error: %s", string(b))
	}

	var items []Item
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, err
	}

	return items, nil
}

func (c *Client) GetItem(ctx context.Context, itemKey string) (*Item, error) {
	url := fmt.Sprintf("%s/users/%s/items/%s", c.baseURL, c.userID, itemKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Zotero-API-Key", c.apiKey)
	req.Header.Set("Zotero-API-Version", "3")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("zotero error: %s", string(b))
	}

	var items []Item
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, err
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("item not found")
	}

	return &items[0], nil
}

func (c *Client) Search(ctx context.Context, query string, itemType string, limit int) ([]Item, error) {
	url := fmt.Sprintf("%s/users/%s/items", c.baseURL, c.userID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	q := req.URL.Query()
	q.Add("q", query)
	if itemType != "" {
		q.Add("itemType", itemType)
	}
	if limit > 0 {
		q.Add("limit", strconv.Itoa(limit))
	}
	req.URL.RawQuery = q.Encode()

	req.Header.Set("Zotero-API-Key", c.apiKey)
	req.Header.Set("Zotero-API-Version", "3")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("zotero error: %s", string(b))
	}

	var items []Item
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, err
	}

	return items, nil
}

func (c *Client) ListCollections(ctx context.Context) ([]Collection, error) {
	url := fmt.Sprintf("%s/users/%s/collections", c.baseURL, c.userID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Zotero-API-Key", c.apiKey)
	req.Header.Set("Zotero-API-Version", "3")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("zotero error: %s", string(b))
	}

	var collections []Collection
	if err := json.NewDecoder(resp.Body).Decode(&collections); err != nil {
		return nil, err
	}

	return collections, nil
}

func (c *Client) CreateCollection(ctx context.Context, name string, parentKey string) (*Collection, error) {
	url := fmt.Sprintf("%s/users/%s/collections", c.baseURL, c.userID)

	body, _ := json.Marshal(map[string]any{
		"name":   name,
		"parent": parentKey,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Zotero-API-Key", c.apiKey)
	req.Header.Set("Zotero-API-Version", "3")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("zotero error: %s", string(b))
	}

	var collection Collection
	if err := json.NewDecoder(resp.Body).Decode(&collection); err != nil {
		return nil, err
	}

	return &collection, nil
}

func (c *Client) AttachFile(ctx context.Context, itemKey string, filePath string) error {
	if c.libraryPath == "" {
		return fmt.Errorf("library path not configured")
	}

	if _, err := os.Stat(filePath); err != nil {
		return err
	}

	url := fmt.Sprintf("%s/users/%s/items/%s/attachments", c.baseURL, c.userID, itemKey)

	filename := filepath.Base(filePath)
	body, _ := json.Marshal(map[string]any{
		"filename": filename,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		return err
	}

	req.Header.Set("Zotero-API-Key", c.apiKey)
	req.Header.Set("Zotero-API-Version", "3")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("zotero error: %s", string(b))
	}

	return nil
}

func (c *Client) Export(itemKey string, format string) (string, error) {
	url := fmt.Sprintf("%s/users/%s/items/%s?format=%s", c.baseURL, c.userID, itemKey, format)

	cmd := exec.Command("curl", "-s", "-H", "Zotero-API-Key: "+c.apiKey, url)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return string(output), nil
}

func (c *Client) IsConfigured() bool {
	return c.apiKey != "" && c.userID != ""
}

func FormatItem(item *Item) string {
	var parts []string

	if item.Title != "" {
		parts = append(parts, item.Title)
	}

	for _, c := range item.Creators {
		if c.Name != "" {
			parts = append(parts, c.Name)
		} else {
			parts = append(parts, c.LastName+", "+c.FirstName)
		}
	}

	if item.Date != "" {
		parts = append(parts, "("+item.Date+")")
	}

	if item.URL != "" {
		parts = append(parts, item.URL)
	}

	return strings.Join(parts, " - ")
}
