package plugin

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Store struct {
	mu             sync.RWMutex
	pluginDir      string
	marketDir      string
	cacheDir       string
	sources        []PluginSource
	httpClient     *http.Client
	trustStore     *TrustStore
	registry       *Registry
	installHistory []InstallRecord
}

type InstallRecord struct {
	PluginID    string    `json:"plugin_id"`
	Version     string    `json:"version"`
	InstalledAt time.Time `json:"installed_at"`
	Source      string    `json:"source"`
	Checksum    string    `json:"checksum"`
}

type InstallResult struct {
	PluginID     string   `json:"plugin_id"`
	Version      string   `json:"version"`
	Path         string   `json:"path"`
	Dependencies []string `json:"dependencies"`
	Warnings     []string `json:"warnings,omitempty"`
}

type UninstallResult struct {
	PluginID string  `json:"plugin_id"`
	Version  string  `json:"version"`
	Rollback *string `json:"rollback,omitempty"`
}

func NewStore(pluginDir, marketDir, cacheDir string, sources []PluginSource, trustStore *TrustStore, registry *Registry) *Store {
	return &Store{
		pluginDir:  pluginDir,
		marketDir:  marketDir,
		cacheDir:   cacheDir,
		sources:    sources,
		httpClient: &http.Client{Timeout: 60 * time.Second},
		trustStore: trustStore,
		registry:   registry,
	}
}

func (s *Store) Search(ctx context.Context, filter SearchFilter) ([]PluginListing, error) {
	var allResults []PluginListing

	for _, source := range s.sources {
		results, err := s.searchSource(ctx, source, filter)
		if err != nil {
			continue
		}
		allResults = append(allResults, results...)
	}

	localResults := s.searchLocal(filter)
	allResults = append(allResults, localResults...)

	allResults = deduplicateListings(allResults)

	if filter.SortBy == "stars" {
		sort.Slice(allResults, func(i, j int) bool {
			if filter.SortOrder == "asc" {
				return allResults[i].Stars < allResults[j].Stars
			}
			return allResults[i].Stars > allResults[j].Stars
		})
	} else if filter.SortBy == "downloads" {
		sort.Slice(allResults, func(i, j int) bool {
			if filter.SortOrder == "asc" {
				return allResults[i].Downloads < allResults[j].Downloads
			}
			return allResults[i].Downloads > allResults[j].Downloads
		})
	} else if filter.SortBy == "name" {
		sort.Slice(allResults, func(i, j int) bool {
			if filter.SortOrder == "asc" {
				return allResults[i].Name < allResults[j].Name
			}
			return allResults[i].Name > allResults[j].Name
		})
	} else {
		sort.Slice(allResults, func(i, j int) bool {
			return allResults[i].Downloads > allResults[j].Downloads
		})
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := filter.Offset
	if offset >= len(allResults) {
		return nil, nil
	}
	end := offset + limit
	if end > len(allResults) {
		end = len(allResults)
	}
	return allResults[offset:end], nil
}

func (s *Store) searchSource(ctx context.Context, source PluginSource, filter SearchFilter) ([]PluginListing, error) {
	if source.Type == "http" || source.Type == "github" {
		return s.searchHTTP(ctx, source, filter)
	}
	return nil, nil
}

func (s *Store) searchHTTP(ctx context.Context, source PluginSource, filter SearchFilter) ([]PluginListing, error) {
	baseURL := strings.TrimRight(source.URL, "/")
	url := fmt.Sprintf("%s/api/v1/plugins/search?q=%s&limit=%d&offset=%d",
		baseURL, filter.Query, filter.Limit+filter.Offset, filter.Offset)
	if filter.Author != "" {
		url += fmt.Sprintf("&author=%s", filter.Author)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if source.Auth != "" {
		req.Header.Set("Authorization", "Bearer "+source.Auth)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("search failed: %s", resp.Status)
	}

	var result struct {
		Plugins []PluginListing `json:"plugins"`
		Total   int             `json:"total"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Plugins, nil
}

func (s *Store) searchLocal(filter SearchFilter) []PluginListing {
	entries, err := os.ReadDir(s.pluginDir)
	if err != nil {
		return nil
	}

	var results []PluginListing
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifest, err := s.loadManifest(filepath.Join(s.pluginDir, entry.Name()))
		if err != nil {
			continue
		}

		if filter.Query != "" {
			q := strings.ToLower(filter.Query)
			if !strings.Contains(strings.ToLower(manifest.Name), q) &&
				!strings.Contains(strings.ToLower(manifest.Description), q) {
				continue
			}
		}

		results = append(results, PluginListing{
			PluginID:    manifest.Name,
			Name:        manifest.Name,
			Version:     manifest.Version,
			Description: manifest.Description,
			Tags:        manifest.CapabilityTags,
			Verified:    manifest.Verified,
			TrustLevel:  manifest.Trust,
		})
	}
	return results
}

func (s *Store) GetPlugin(ctx context.Context, id string) (*PluginListing, error) {
	for _, source := range s.sources {
		listing, err := s.getPluginFromSource(ctx, source, id)
		if err == nil && listing != nil {
			return listing, nil
		}
	}

	manifest, err := s.loadManifest(filepath.Join(s.pluginDir, id))
	if err == nil {
		return &PluginListing{
			PluginID:    manifest.Name,
			Name:        manifest.Name,
			Version:     manifest.Version,
			Description: manifest.Description,
			Verified:    manifest.Verified,
		}, nil
	}

	return nil, fmt.Errorf("plugin not found: %s", id)
}

func (s *Store) getPluginFromSource(ctx context.Context, source PluginSource, id string) (*PluginListing, error) {
	baseURL := strings.TrimRight(source.URL, "/")
	url := fmt.Sprintf("%s/api/v1/plugins/%s", baseURL, id)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if source.Auth != "" {
		req.Header.Set("Authorization", "Bearer "+source.Auth)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, nil
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("get plugin failed: %s", resp.Status)
	}

	var listing PluginListing
	if err := json.NewDecoder(resp.Body).Decode(&listing); err != nil {
		return nil, err
	}
	return &listing, nil
}

func (s *Store) GetVersions(ctx context.Context, id string) ([]string, error) {
	for _, source := range s.sources {
		baseURL := strings.TrimRight(source.URL, "/")
		url := fmt.Sprintf("%s/api/v1/plugins/%s/versions", baseURL, id)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			continue
		}
		if source.Auth != "" {
			req.Header.Set("Authorization", "Bearer "+source.Auth)
		}

		resp, err := s.httpClient.Do(req)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 300 {
			continue
		}

		var result struct {
			Versions []string `json:"versions"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			continue
		}
		return result.Versions, nil
	}
	return nil, fmt.Errorf("no versions found for: %s", id)
}

func (s *Store) Install(ctx context.Context, id, version string) (*InstallResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var listing *PluginListing
	var source PluginSource

	for _, src := range s.sources {
		l, err := s.getPluginFromSource(ctx, src, id)
		if err == nil && l != nil {
			listing = l
			source = src
			break
		}
	}

	if listing == nil {
		return nil, fmt.Errorf("plugin not found in any source: %s", id)
	}

	if version == "" {
		version = listing.Version
	}

	if err := s.downloadAndExtract(ctx, source, listing, version); err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}

	pluginPath := filepath.Join(s.pluginDir, id)
	if err := s.verifyInstalledPlugin(pluginPath); err != nil {
		os.RemoveAll(pluginPath)
		return nil, fmt.Errorf("verify: %w", err)
	}

	record := InstallRecord{
		PluginID:    id,
		Version:     version,
		InstalledAt: time.Now().UTC(),
		Source:      source.Name,
		Checksum:    listing.SHA256,
	}
	s.installHistory = append(s.installHistory, record)
	s.saveInstallHistory()

	result := &InstallResult{
		PluginID: id,
		Version:  version,
		Path:     pluginPath,
	}

	if len(listing.Dependencies) > 0 {
		for _, dep := range listing.Dependencies {
			if _, err := s.installWithoutLock(ctx, source, &PluginListing{PluginID: dep}, ""); err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("dependency %s: %v", dep, err))
			}
			result.Dependencies = append(result.Dependencies, dep)
		}
	}

	return result, nil
}

func (s *Store) downloadAndExtract(ctx context.Context, source PluginSource, listing *PluginListing, version string) error {
	baseURL := strings.TrimRight(source.URL, "/")
	downloadURL := listing.DownloadURL
	if downloadURL == "" {
		downloadURL = fmt.Sprintf("%s/api/v1/plugins/%s/download?version=%s", baseURL, listing.PluginID, version)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return err
	}
	if source.Auth != "" {
		req.Header.Set("Authorization", "Bearer "+source.Auth)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("download failed: %s", resp.Status)
	}

	pluginPath := filepath.Join(s.pluginDir, listing.PluginID)
	tempPath := pluginPath + ".tmp"
	if err := os.RemoveAll(tempPath); err != nil {
		return err
	}
	if err := os.MkdirAll(tempPath, 0755); err != nil {
		return err
	}

	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "zip") || strings.HasSuffix(downloadURL, ".zip") {
		if err := extractZip(resp.Body, tempPath); err != nil {
			os.RemoveAll(tempPath)
			return err
		}
	} else {
		if err := extractTarGz(resp.Body, tempPath); err != nil {
			os.RemoveAll(tempPath)
			return err
		}
	}

	if listing.SHA256 != "" {
		checksum, err := computeDirChecksum(tempPath)
		if err == nil && checksum != listing.SHA256 {
			os.RemoveAll(tempPath)
			return fmt.Errorf("checksum mismatch: expected %s, got %s", listing.SHA256, checksum)
		}
	}

	if listing.Signature != "" && s.trustStore != nil {
		if err := verifyPluginSignature(tempPath, listing.Signature); err != nil {
			os.RemoveAll(tempPath)
			return fmt.Errorf("signature verification failed: %w", err)
		}
	}

	if err := os.RemoveAll(pluginPath); err != nil {
		os.RemoveAll(tempPath)
		return err
	}
	if err := os.Rename(tempPath, pluginPath); err != nil {
		copyDirRecursive(tempPath, pluginPath)
		os.RemoveAll(tempPath)
	}

	return nil
}

func (s *Store) verifyInstalledPlugin(pluginPath string) error {
	manifest, err := s.loadManifest(pluginPath)
	if err != nil {
		return fmt.Errorf("invalid plugin: %w", err)
	}
	if manifest.Name == "" {
		return fmt.Errorf("plugin manifest missing name")
	}
	return nil
}

func (s *Store) Update(ctx context.Context, id string) (*InstallResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	currentManifest, err := s.loadManifest(filepath.Join(s.pluginDir, id))
	if err != nil {
		return nil, fmt.Errorf("plugin not installed: %s", id)
	}

	var listing *PluginListing
	var source PluginSource
	for _, src := range s.sources {
		l, err := s.getPluginFromSource(ctx, src, id)
		if err == nil && l != nil {
			listing = l
			source = src
			break
		}
	}

	if listing == nil {
		return nil, fmt.Errorf("plugin not found in any source: %s", id)
	}

	if compareVersions(listing.Version, currentManifest.Version) <= 0 {
		return nil, fmt.Errorf("already at latest version: %s", currentManifest.Version)
	}

	backupPath := filepath.Join(s.pluginDir, id+".backup")
	if err := copyDirRecursive(filepath.Join(s.pluginDir, id), backupPath); err != nil {
		return nil, fmt.Errorf("backup: %w", err)
	}

	result, err := s.installWithoutLock(ctx, source, listing, listing.Version)
	if err != nil {
		os.RemoveAll(filepath.Join(s.pluginDir, id))
		copyDirRecursive(backupPath, filepath.Join(s.pluginDir, id))
		os.RemoveAll(backupPath)
		return nil, fmt.Errorf("update failed, rolled back: %w", err)
	}

	os.RemoveAll(backupPath)
	return result, nil
}

func (s *Store) installWithoutLock(ctx context.Context, source PluginSource, listing *PluginListing, version string) (*InstallResult, error) {
	if err := s.downloadAndExtract(ctx, source, listing, version); err != nil {
		return nil, err
	}

	pluginPath := filepath.Join(s.pluginDir, listing.PluginID)
	if err := s.verifyInstalledPlugin(pluginPath); err != nil {
		os.RemoveAll(pluginPath)
		return nil, fmt.Errorf("verify: %w", err)
	}

	record := InstallRecord{
		PluginID:    listing.PluginID,
		Version:     version,
		InstalledAt: time.Now().UTC(),
		Source:      source.Name,
		Checksum:    listing.SHA256,
	}
	s.installHistory = append(s.installHistory, record)
	s.saveInstallHistory()

	return &InstallResult{
		PluginID: listing.PluginID,
		Version:  version,
		Path:     pluginPath,
	}, nil
}

func (s *Store) Uninstall(id string) (*UninstallResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	pluginPath := filepath.Join(s.pluginDir, id)
	if _, err := os.Stat(pluginPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("plugin not installed: %s", id)
	}

	manifest, err := s.loadManifest(pluginPath)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	result := &UninstallResult{
		PluginID: id,
		Version:  manifest.Version,
	}

	history := s.loadInstallHistory()
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].PluginID == id {
			if i > 0 {
				prev := history[i-1]
				result.Rollback = &prev.Version
			}
			break
		}
	}

	if err := os.RemoveAll(pluginPath); err != nil {
		return nil, fmt.Errorf("remove: %w", err)
	}

	return result, nil
}

type RollbackResult struct {
	PluginID    string `json:"plugin_id"`
	FromVersion string `json:"from_version"`
	ToVersion   string `json:"to_version"`
}

func (s *Store) Rollback(ctx context.Context, id, targetVersion string) (*RollbackResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	pluginPath := filepath.Join(s.pluginDir, id)
	currentManifest, err := s.loadManifest(pluginPath)
	if err != nil {
		return nil, fmt.Errorf("plugin not installed: %s", id)
	}

	history := s.loadInstallHistory()
	var targetRecord *InstallRecord
	var targetSource PluginSource
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].PluginID == id && history[i].Version == targetVersion {
			targetRecord = &history[i]
			for _, src := range s.sources {
				if src.Name == history[i].Source {
					targetSource = src
					break
				}
			}
			break
		}
	}

	if targetRecord == nil {
		return nil, fmt.Errorf("version %s not found in install history for plugin %s", targetVersion, id)
	}

	if targetVersion == currentManifest.Version {
		return nil, fmt.Errorf("already at version %s", targetVersion)
	}

	backupPath := pluginPath + ".rollback-backup"
	if err := copyDirRecursive(pluginPath, backupPath); err != nil {
		return nil, fmt.Errorf("backup current version failed: %w", err)
	}

	if targetSource.URL != "" && targetRecord.Checksum != "" {
		listing := &PluginListing{
			PluginID: id,
			Version:  targetVersion,
			SHA256:   targetRecord.Checksum,
		}
		if err := s.downloadAndExtract(ctx, targetSource, listing, targetVersion); err != nil {
			os.RemoveAll(pluginPath)
			copyDirRecursive(backupPath, pluginPath)
			os.RemoveAll(backupPath)
			return nil, fmt.Errorf("rollback download failed: %w", err)
		}
	} else {
		prevPath := filepath.Join(s.cacheDir, id, targetVersion)
		if _, err := os.Stat(prevPath); err == nil {
			os.RemoveAll(pluginPath)
			if err := copyDirRecursive(prevPath, pluginPath); err != nil {
				os.RemoveAll(pluginPath)
				copyDirRecursive(backupPath, pluginPath)
				os.RemoveAll(backupPath)
				return nil, fmt.Errorf("restore from cache failed: %w", err)
			}
		} else {
			os.RemoveAll(pluginPath)
			copyDirRecursive(backupPath, pluginPath)
			os.RemoveAll(backupPath)
			return nil, fmt.Errorf("version %s not available offline, re-download source not configured", targetVersion)
		}
	}

	os.RemoveAll(backupPath)

	record := InstallRecord{
		PluginID:    id,
		Version:     targetVersion,
		InstalledAt: time.Now().UTC(),
		Source:      targetSource.Name,
		Checksum:    targetRecord.Checksum,
	}
	s.installHistory = append(s.installHistory, record)
	s.saveInstallHistory()

	return &RollbackResult{
		PluginID:    id,
		FromVersion: currentManifest.Version,
		ToVersion:   targetVersion,
	}, nil
}

func (s *Store) GetInstallHistory(id string) []InstallRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	history := s.loadInstallHistory()
	var filtered []InstallRecord
	for _, r := range history {
		if r.PluginID == id {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

func (s *Store) ListInstalled() []InstallRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]InstallRecord(nil), s.installHistory...)
}

func (s *Store) loadManifest(pluginDir string) (*Manifest, error) {
	candidates := []string{
		"openclaw.plugin.json",
		"plugin.json",
		"anyclaw.plugin.json",
	}
	for _, name := range candidates {
		path := filepath.Join(pluginDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var m Manifest
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}
		return &m, nil
	}
	return nil, fmt.Errorf("no manifest found")
}

func (s *Store) loadInstallHistory() []InstallRecord {
	path := filepath.Join(s.marketDir, "install_history.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var records []InstallRecord
	json.Unmarshal(data, &records)
	return records
}

func (s *Store) saveInstallHistory() {
	path := filepath.Join(s.marketDir, "install_history.json")
	os.MkdirAll(filepath.Dir(path), 0755)
	data, _ := json.MarshalIndent(s.installHistory, "", "  ")
	os.WriteFile(path, data, 0644)
}

func extractZip(body io.Reader, dest string) error {
	tempFile, err := os.CreateTemp("", "plugin-*.zip")
	if err != nil {
		return err
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	if _, err := io.Copy(tempFile, body); err != nil {
		return err
	}
	tempFile.Close()

	r, err := zip.OpenReader(tempFile.Name())
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		path := filepath.Join(dest, f.Name)
		if f.FileInfo().IsDir() {
			os.MkdirAll(path, 0755)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		dst, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}
		src, err := f.Open()
		if err != nil {
			dst.Close()
			return err
		}
		io.Copy(dst, src)
		dst.Close()
		src.Close()
	}
	return nil
}

func extractTarGz(body io.Reader, dest string) error {
	gzr, err := gzip.NewReader(body)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		path := filepath.Join(dest, header.Name)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return err
			}
			f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
			if err != nil {
				return err
			}
			io.Copy(f, tr)
			f.Close()
		}
	}
	return nil
}

func copyDirRecursive(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := copyDirRecursive(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			data, err := os.ReadFile(srcPath)
			if err != nil {
				return err
			}
			if err := os.WriteFile(dstPath, data, 0644); err != nil {
				return err
			}
		}
	}
	return nil
}

func computeDirChecksum(dir string) (string, error) {
	var files []string
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		files = append(files, rel)
		return nil
	})
	sort.Strings(files)

	hash := ""
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			continue
		}
		h := sha256.Sum256(data)
		hash += fmt.Sprintf("%s:%x\n", f, h)
	}
	h := sha256.Sum256([]byte(hash))
	return fmt.Sprintf("%x", h), nil
}

func verifyPluginSignature(pluginDir, signature string) error {
	manifestPath := filepath.Join(pluginDir, "plugin.json")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		manifestPath = filepath.Join(pluginDir, "openclaw.plugin.json")
		manifestData, err = os.ReadFile(manifestPath)
		if err != nil {
			return fmt.Errorf("no manifest: %w", err)
		}
	}

	var sig Signature
	if err := json.Unmarshal([]byte(signature), &sig); err != nil {
		return fmt.Errorf("invalid signature: %w", err)
	}

	var m Manifest
	if err := json.Unmarshal(manifestData, &m); err != nil {
		return fmt.Errorf("invalid manifest: %w", err)
	}

	ok, err := VerifySignature(&m, &sig, "")
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("signature verification failed")
	}
	return nil
}

func deduplicateListings(listings []PluginListing) []PluginListing {
	seen := make(map[string]int)
	var result []PluginListing
	for i, l := range listings {
		if idx, ok := seen[l.PluginID]; ok {
			if l.Downloads > result[idx].Downloads {
				result[idx] = l
			}
			continue
		}
		seen[l.PluginID] = len(result)
		result = append(result, listings[i])
	}
	return result
}

func compareVersions(a, b string) int {
	va, errA := ParseVersion(a)
	vb, errB := ParseVersion(b)
	if errA != nil || errB != nil {
		if a == b {
			return 0
		}
		if a > b {
			return 1
		}
		return -1
	}
	return va.Compare(vb)
}
