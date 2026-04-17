package plugin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type PluginSource struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Type   string `json:"type"` // github, http, local
	Auth   string `json:"auth,omitempty"`
	Branch string `json:"branch,omitempty"`
}

type PluginListing struct {
	PluginID     string            `json:"plugin_id"`
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	Description  string            `json:"description"`
	Author       string            `json:"author"`
	Homepage     string            `json:"homepage"`
	Repository   string            `json:"repository"`
	DownloadURL  string            `json:"download_url"`
	SignatureURL string            `json:"signature_url,omitempty"`
	Checksums    map[string]string `json:"checksums,omitempty"`
	PublishedAt  time.Time         `json:"published_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
	Stars        int               `json:"stars"`
	Downloads    int               `json:"downloads"`
	License      string            `json:"license"`
	Tags         []string          `json:"tags"`
	Platforms    []string          `json:"platforms"`
	MinVersion   string            `json:"min_version,omitempty"`
	Dependencies []string          `json:"dependencies,omitempty"`
	FileSize     int64             `json:"file_size"`
	SHA256       string            `json:"sha256"`
	Signature    string            `json:"signature,omitempty"`
	Verified     bool              `json:"verified"`
	TrustLevel   string            `json:"trust_level"`
}

type SearchFilter struct {
	Query     string
	Tags      []string
	Platforms []string
	Author    string
	SortBy    string // stars, downloads, updated, name
	SortOrder string // asc, desc
	Limit     int
	Offset    int
}

type MarketClient struct {
	sources    []*PluginSource
	cacheDir   string
	httpClient *http.Client
}

func NewMarketClient(cacheDir string) *MarketClient {
	return &MarketClient{
		cacheDir:   cacheDir,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (mc *MarketClient) AddSource(source *PluginSource) {
	mc.sources = append(mc.sources, source)
}

func (mc *MarketClient) Search(filter SearchFilter) ([]*PluginListing, error) {
	var results []*PluginListing

	for _, source := range mc.sources {
		items, err := mc.searchSource(source, filter)
		if err != nil {
			continue
		}
		results = append(results, items...)
	}

	results = mc.applyFilter(results, filter)
	mc.sortResults(results, filter)

	if filter.Limit > 0 && len(results) > filter.Limit {
		results = results[:filter.Limit]
	}
	if filter.Offset > 0 && len(results) > filter.Offset {
		results = results[filter.Offset:]
	}

	return results, nil
}

func (mc *MarketClient) searchSource(source *PluginSource, filter SearchFilter) ([]*PluginListing, error) {
	switch source.Type {
	case "github":
		return mc.searchGitHub(source, filter)
	case "http":
		return mc.searchHTTP(source, filter)
	default:
		return nil, fmt.Errorf("unsupported source type: %s", source.Type)
	}
}

func (mc *MarketClient) searchGitHub(source *PluginSource, filter SearchFilter) ([]*PluginListing, error) {
	url := fmt.Sprintf("%s/plugins.json", strings.TrimSuffix(source.URL, "/"))
	resp, err := mc.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var listings []*PluginListing
	if err := json.NewDecoder(resp.Body).Decode(&listings); err != nil {
		return nil, err
	}

	return listings, nil
}

func (mc *MarketClient) searchHTTP(source *PluginSource, filter SearchFilter) ([]*PluginListing, error) {
	url := source.URL
	resp, err := mc.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var listings []*PluginListing
	if err := json.NewDecoder(resp.Body).Decode(&listings); err != nil {
		return nil, err
	}

	return listings, nil
}

func (mc *MarketClient) applyFilter(listings []*PluginListing, filter SearchFilter) []*PluginListing {
	var filtered []*PluginListing

	for _, listing := range listings {
		if filter.Query != "" {
			query := strings.ToLower(filter.Query)
			match := strings.Contains(strings.ToLower(listing.Name), query) ||
				strings.Contains(strings.ToLower(listing.Description), query) ||
				strings.Contains(strings.ToLower(listing.Author), query)
			if !match {
				continue
			}
		}

		if len(filter.Tags) > 0 {
			match := false
			for _, tag := range filter.Tags {
				for _, listingTag := range listing.Tags {
					if strings.EqualFold(tag, listingTag) {
						match = true
						break
					}
				}
			}
			if !match {
				continue
			}
		}

		if len(filter.Platforms) > 0 {
			match := false
			for _, p := range filter.Platforms {
				for _, lp := range listing.Platforms {
					if strings.EqualFold(p, lp) {
						match = true
						break
					}
				}
			}
			if !match {
				continue
			}
		}

		if filter.Author != "" && !strings.Contains(strings.ToLower(listing.Author), strings.ToLower(filter.Author)) {
			continue
		}

		filtered = append(filtered, listing)
	}

	return filtered
}

func (mc *MarketClient) sortResults(listings []*PluginListing, filter SearchFilter) {
	sort.Slice(listings, func(i, j int) bool {
		var cmp bool
		switch filter.SortBy {
		case "stars":
			cmp = listings[i].Stars > listings[j].Stars
		case "downloads":
			cmp = listings[i].Downloads > listings[j].Downloads
		case "updated", "updated_at":
			cmp = listings[i].UpdatedAt.After(listings[j].UpdatedAt)
		case "name":
			cmp = listings[i].Name < listings[j].Name
		default:
			cmp = listings[i].Name < listings[j].Name
		}
		if filter.SortOrder == "asc" {
			return !cmp
		}
		return cmp
	})
}

func (mc *MarketClient) GetPlugin(pluginID string) (*PluginListing, error) {
	filter := SearchFilter{Query: pluginID, Limit: 1}
	results, err := mc.Search(filter)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("plugin not found: %s", pluginID)
	}
	return results[0], nil
}

func (mc *MarketClient) DownloadPlugin(listing *PluginListing, destDir string) (string, error) {
	os.MkdirAll(destDir, 0755)

	resp, err := mc.httpClient.Get(listing.DownloadURL)
	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	filename := fmt.Sprintf("%s-%s.zip", listing.Name, listing.Version)
	destPath := filepath.Join(destDir, filename)

	out, err := os.Create(destPath)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	if listing.SignatureURL != "" {
		sigResp, err := mc.httpClient.Get(listing.SignatureURL)
		if err == nil {
			defer sigResp.Body.Close()
			sigData, _ := io.ReadAll(sigResp.Body)
			os.WriteFile(destPath+".sig", sigData, 0644)
		}
	}

	return destPath, nil
}

func (mc *MarketClient) InstallPlugin(listing *PluginListing, pluginsDir string, trustStore *TrustStore) error {
	downloadPath, err := mc.DownloadPlugin(listing, mc.cacheDir)
	if err != nil {
		return err
	}

	installDir := filepath.Join(pluginsDir, listing.Name)
	if err := mc.extractPlugin(downloadPath, installDir); err != nil {
		return err
	}

	sigPath := downloadPath + ".sig"
	if _, err := os.Stat(sigPath); err == nil {
		sigData, _ := os.ReadFile(sigPath)
		var sig Signature
		if json.Unmarshal(sigData, &sig) == nil {
			if trustStore.IsTrusted(sig.KeyID) {
				listing.Verified = true
				listing.TrustLevel = "verified"
			}
		}
	}

	return nil
}

func (mc *MarketClient) extractPlugin(zipPath, destDir string) error {
	return fmt.Errorf("extract not implemented: use archive package")
}

func (mc *MarketClient) UpdatePlugin(pluginID string, pluginsDir string) error {
	listing, err := mc.GetPlugin(pluginID)
	if err != nil {
		return err
	}

	pluginDir := filepath.Join(pluginsDir, pluginID)
	if _, err := os.Stat(pluginDir); os.IsNotExist(err) {
		return fmt.Errorf("plugin not installed: %s", pluginID)
	}

	return mc.InstallPlugin(listing, pluginsDir, nil)
}

type PluginIndex struct {
	UpdatedAt time.Time                 `json:"updated_at"`
	Plugins   map[string]*PluginListing `json:"plugins"`
}

func (mc *MarketClient) BuildLocalIndex(pluginsDir string) (*PluginIndex, error) {
	index := &PluginIndex{
		UpdatedAt: time.Now(),
		Plugins:   make(map[string]*PluginListing),
	}

	entries, err := os.ReadDir(pluginsDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifest, err := loadManifestFromFile(filepath.Join(pluginsDir, entry.Name(), "plugin.json"))
		if err != nil {
			continue
		}
		sig, _ := LoadSignature(filepath.Join(pluginsDir, entry.Name()))

		listing := &PluginListing{
			PluginID:    entry.Name(),
			Name:        manifest.Name,
			Version:     manifest.Version,
			Description: manifest.Description,
			Author:      manifest.Signer,
			Platforms:   []string{"windows", "linux", "darwin"},
			TrustLevel:  "unknown",
		}
		if sig != nil {
			listing.Verified = true
			listing.TrustLevel = "signed"
		}
		index.Plugins[entry.Name()] = listing
	}

	return index, nil
}

type Version struct {
	Major int
	Minor int
	Patch int
}

func ParseVersion(v string) (*Version, error) {
	re := regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)`)
	matches := re.FindStringSubmatch(v)
	if len(matches) < 4 {
		return nil, fmt.Errorf("invalid version: %s", v)
	}
	return &Version{
		Major: parseInt(matches[1]),
		Minor: parseInt(matches[2]),
		Patch: parseInt(matches[3]),
	}, nil
}

func parseInt(s string) int {
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
}

func (v *Version) Compare(other *Version) int {
	if v.Major != other.Major {
		return v.Major - other.Major
	}
	if v.Minor != other.Minor {
		return v.Minor - other.Minor
	}
	return v.Patch - other.Patch
}

func (v *Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

func IsCompatible(current, required string) bool {
	cv, err := ParseVersion(current)
	if err != nil {
		return false
	}
	rv, err := ParseVersion(required)
	if err != nil {
		return false
	}

	if cv.Major != rv.Major {
		return cv.Major > rv.Major
	}
	if cv.Minor < rv.Minor {
		return false
	}
	return true
}
