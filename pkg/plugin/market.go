package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type LocalMarket struct {
	dir     string
	index   *MarketIndex
	plugins map[string]*MarketPlugin
}

type MarketIndex struct {
	Version   string        `json:"version"`
	UpdatedAt string        `json:"updated_at"`
	Plugins   []MarketEntry `json:"plugins"`
}

type MarketEntry struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Tags        []string `json:"tags"`
	Author      string   `json:"author"`
	Repo        string   `json:"repo"`
	Stars       int      `json:"stars"`
	Downloads   int      `json:"downloads"`
	License     string   `json:"license"`
	Score       int      `json:"score"`
}

type MarketPlugin struct {
	Entry       MarketEntry
	LocalPath   string
	IsInstalled bool
}

func NewLocalMarket(dir string) (*LocalMarket, error) {
	market := &LocalMarket{
		dir:     dir,
		index:   &MarketIndex{Version: "1.0"},
		plugins: make(map[string]*MarketPlugin),
	}

	if err := market.loadIndex(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	return market, nil
}

func (m *LocalMarket) loadIndex() error {
	indexPath := filepath.Join(m.dir, "market.json")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, m.index); err != nil {
		return err
	}
	for _, entry := range m.index.Plugins {
		m.plugins[entry.ID] = &MarketPlugin{
			Entry:       entry,
			LocalPath:   filepath.Join(m.dir, entry.ID),
			IsInstalled: false,
		}
	}
	return nil
}

func (m *LocalMarket) SaveIndex() error {
	m.index.UpdatedAt = "2026-03-31"
	data, err := json.MarshalIndent(m.index, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(m.dir, "market.json"), data, 0644)
}

func (m *LocalMarket) AddPlugin(entry MarketEntry) {
	m.plugins[entry.ID] = &MarketPlugin{
		Entry:       entry,
		LocalPath:   filepath.Join(m.dir, entry.ID),
		IsInstalled: false,
	}
	m.index.Plugins = append(m.index.Plugins, entry)
}

func (m *LocalMarket) Search(query string, category string, tags []string, limit int) []MarketEntry {
	var results []MarketEntry
	query = strings.ToLower(query)

	for _, entry := range m.index.Plugins {
		if category != "" && entry.Category != category {
			continue
		}

		if len(tags) > 0 {
			match := false
			for _, tag := range tags {
				for _, t := range entry.Tags {
					if strings.EqualFold(t, tag) {
						match = true
						break
					}
				}
			}
			if !match {
				continue
			}
		}

		if query != "" {
			score := m.calculateScore(entry, query)
			if score == 0 {
				continue
			}
			entry.Score = score
		}

		results = append(results, entry)
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].Downloads > results[j].Downloads
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results
}

func (m *LocalMarket) calculateScore(entry MarketEntry, query string) int {
	score := 0

	if strings.Contains(strings.ToLower(entry.Name), query) {
		score += 50
	}
	if strings.Contains(strings.ToLower(entry.Description), query) {
		score += 20
	}
	for _, tag := range entry.Tags {
		if strings.Contains(strings.ToLower(tag), query) {
			score += 15
		}
	}
	if strings.Contains(strings.ToLower(entry.Category), query) {
		score += 10
	}

	return score
}

func (m *LocalMarket) GetPlugin(id string) (*MarketPlugin, bool) {
	plugin, exists := m.plugins[id]
	return plugin, exists
}

func (m *LocalMarket) ListByCategory() map[string][]MarketEntry {
	result := make(map[string][]MarketEntry)
	for _, entry := range m.index.Plugins {
		result[entry.Category] = append(result[entry.Category], entry)
	}
	return result
}

func (m *LocalMarket) ListCategories() []string {
	seen := make(map[string]bool)
	var categories []string
	for _, entry := range m.index.Plugins {
		if !seen[entry.Category] {
			seen[entry.Category] = true
			categories = append(categories, entry.Category)
		}
	}
	sort.Strings(categories)
	return categories
}

type MarketManager struct {
	localMarket *LocalMarket
	hubClient   *HubClient
}

func NewMarketManager(localDir string, hubURL string) (*MarketManager, error) {
	market, err := NewLocalMarket(localDir)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize local market: %w", err)
	}

	mm := &MarketManager{
		localMarket: market,
	}

	if hubURL != "" {
		mm.hubClient = NewHubClient(hubURL)
	}

	return mm, nil
}

func (mm *MarketManager) Search(ctx interface{}, query string, category string, limit int) ([]MarketEntry, error) {
	entries := mm.localMarket.Search(query, category, nil, limit)
	return entries, nil
}

func (mm *MarketManager) Install(id string, version string, targetDir string) error {
	plugin, ok := mm.localMarket.GetPlugin(id)
	if !ok {
		return fmt.Errorf("plugin not found in market: %s", id)
	}

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return err
	}

	sourcePath := plugin.LocalPath
	if _, err := os.Stat(sourcePath); err != nil {
		return fmt.Errorf("plugin source not found: %s", sourcePath)
	}

	_ = version

	destPath := filepath.Join(targetDir, id)
	return copyDir(sourcePath, destPath)
}

func copyDir(src, dst string) error {
	return nil
}

func (mm *MarketManager) Update(id string, targetDir string) error {
	plugin, ok := mm.localMarket.GetPlugin(id)
	if !ok {
		return fmt.Errorf("plugin not found: %s", id)
	}

	destPath := filepath.Join(targetDir, id)
	if _, err := os.Stat(destPath); err != nil {
		return fmt.Errorf("plugin not installed: %s", id)
	}

	sourcePath := plugin.LocalPath
	return copyDir(sourcePath, destPath)
}

func (mm *MarketManager) ListInstalled(pluginDir string) []MarketEntry {
	var entries []MarketEntry

	entries_, err := os.ReadDir(pluginDir)
	if err != nil {
		return entries
	}

	for _, entry := range entries_ {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if plugin, ok := mm.localMarket.GetPlugin(name); ok {
			entry := plugin.Entry
			entry.Score = 100
			entries = append(entries, entry)
		}
	}

	return entries
}
