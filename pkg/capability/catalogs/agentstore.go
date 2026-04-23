package agentstore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type AgentPackage struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	DisplayName  string        `json:"display_name"`
	Description  string        `json:"description"`
	Author       string        `json:"author"`
	Version      string        `json:"version"`
	Category     string        `json:"category"`
	Tags         []string      `json:"tags"`
	Icon         string        `json:"icon,omitempty"`
	Persona      string        `json:"persona"`
	Domain       string        `json:"domain"`
	Expertise    []string      `json:"expertise"`
	SystemPrompt string        `json:"system_prompt"`
	Tone         string        `json:"tone"`
	Style        string        `json:"style"`
	Skills       []string      `json:"skills"`
	Permission   string        `json:"permission"`
	Downloads    int           `json:"downloads"`
	Rating       float64       `json:"rating"`
	RatingCount  int           `json:"rating_count"`
	IsBuiltin    bool          `json:"is_builtin"`
	InstalledAt  string        `json:"installed_at,omitempty"`
	UpdatedAt    string        `json:"updated_at"`
	Install      *InstallSpec  `json:"install,omitempty"`
	Bundle       PackageBundle `json:"bundle"`

	sourcePath string
}

type StoreFilter struct {
	Category  string `json:"category,omitempty"`
	Tag       string `json:"tag,omitempty"`
	Keyword   string `json:"keyword,omitempty"`
	Installed *bool  `json:"installed,omitempty"`
}

type StoreManager interface {
	List(filter StoreFilter) []AgentPackage
	Get(id string) (*AgentPackage, error)
	Search(keyword string) []AgentPackage
	Install(id string) error
	Uninstall(id string) error
	Installed() []AgentPackage
	IsInstalled(id string) bool
	GetCategories() []string
	GetTags() []string
}

type storeManager struct {
	mu           sync.RWMutex
	packages     map[string]*AgentPackage
	installedIDs map[string]bool
	installDir   string
	configPath   string
}

func NewStoreManager(installDir string, configPath string) (StoreManager, error) {
	sm := &storeManager{
		packages:     make(map[string]*AgentPackage),
		installedIDs: make(map[string]bool),
		installDir:   installDir,
		configPath:   configPath,
	}

	// Load built-in packages
	sm.loadBuiltinPackages()

	// Load packages from store directory
	if err := sm.loadStorePackages(); err != nil {
		// Non-fatal: store directory might not exist yet
		_ = err
	}

	// Load installed markers
	sm.loadInstalledMarkers()

	return sm, nil
}

func (sm *storeManager) loadBuiltinPackages() {
	for _, pkg := range builtinPackages() {
		pkg.IsBuiltin = true
		pkg.UpdatedAt = time.Now().Format(time.RFC3339)
		sm.packages[pkg.ID] = &pkg
	}
}

func (sm *storeManager) loadStorePackages() error {
	storeDir := filepath.Join(sm.installDir, "store")
	if _, err := os.Stat(storeDir); os.IsNotExist(err) {
		os.MkdirAll(storeDir, 0o755)
		return nil
	}

	entries, err := os.ReadDir(storeDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(storeDir, entry.Name()))
		if err != nil {
			continue
		}
		var pkg AgentPackage
		if err := json.Unmarshal(data, &pkg); err != nil {
			continue
		}
		if pkg.ID != "" {
			pkg.UpdatedAt = time.Now().Format(time.RFC3339)
			pkg.sourcePath = filepath.Join(storeDir, entry.Name())
			sm.packages[pkg.ID] = &pkg
		}
	}
	return nil
}

func (sm *storeManager) loadInstalledMarkers() {
	markerFile := filepath.Join(sm.installDir, "installed_agents.json")
	data, err := os.ReadFile(markerFile)
	if err != nil {
		return
	}
	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil {
		return
	}
	for _, id := range ids {
		sm.installedIDs[id] = true
	}
}

func (sm *storeManager) saveInstalledMarkers() {
	markerFile := filepath.Join(sm.installDir, "installed_agents.json")
	ids := make([]string, 0, len(sm.installedIDs))
	for id := range sm.installedIDs {
		ids = append(ids, id)
	}
	data, _ := json.MarshalIndent(ids, "", "  ")
	os.WriteFile(markerFile, data, 0o644)
}

func (sm *storeManager) List(filter StoreFilter) []AgentPackage {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	result := make([]AgentPackage, 0)
	for _, pkg := range sm.packages {
		if !sm.matchesFilter(pkg, filter) {
			continue
		}
		p := *pkg
		p.Bundle = summarizePackageBundle(p)
		if sm.installedIDs[p.ID] {
			p.InstalledAt = time.Now().Format(time.RFC3339)
		}
		result = append(result, p)
	}
	return result
}

func (sm *storeManager) matchesFilter(pkg *AgentPackage, filter StoreFilter) bool {
	if filter.Category != "" && !strings.EqualFold(pkg.Category, filter.Category) {
		return false
	}
	if filter.Tag != "" {
		found := false
		for _, t := range pkg.Tags {
			if strings.EqualFold(t, filter.Tag) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if filter.Keyword != "" {
		kw := strings.ToLower(filter.Keyword)
		if !strings.Contains(strings.ToLower(pkg.Name), kw) &&
			!strings.Contains(strings.ToLower(pkg.DisplayName), kw) &&
			!strings.Contains(strings.ToLower(pkg.Description), kw) &&
			!strings.Contains(strings.ToLower(pkg.Domain), kw) {
			return false
		}
	}
	if filter.Installed != nil {
		isInstalled := sm.installedIDs[pkg.ID]
		if *filter.Installed != isInstalled {
			return false
		}
	}
	return true
}

func (sm *storeManager) Get(id string) (*AgentPackage, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	pkg, ok := sm.packages[id]
	if !ok {
		return nil, fmt.Errorf("agent package not found: %s", id)
	}
	p := *pkg
	p.Bundle = summarizePackageBundle(p)
	if sm.installedIDs[p.ID] {
		p.InstalledAt = time.Now().Format(time.RFC3339)
	}
	return &p, nil
}

func (sm *storeManager) Search(keyword string) []AgentPackage {
	return sm.List(StoreFilter{Keyword: keyword})
}

func (sm *storeManager) Install(id string) error {
	sm.mu.RLock()
	pkg, ok := sm.packages[id]
	if !ok {
		sm.mu.RUnlock()
		return fmt.Errorf("agent package not found: %s", id)
	}
	if sm.installedIDs[id] {
		sm.mu.RUnlock()
		return nil
	}
	packageCopy := *pkg
	sm.mu.RUnlock()

	if err := sm.installPackage(packageCopy); err != nil {
		return err
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.installedIDs[id] = true
	sm.saveInstalledMarkers()

	// Update download count
	pkg.Downloads++

	return nil
}

func (sm *storeManager) Uninstall(id string) error {
	sm.mu.RLock()
	if !sm.installedIDs[id] {
		sm.mu.RUnlock()
		return fmt.Errorf("agent not installed: %s", id)
	}
	packageCopy, _ := sm.packages[id]
	sm.mu.RUnlock()

	if packageCopy != nil {
		if err := sm.uninstallPackage(*packageCopy); err != nil {
			return err
		}
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.installedIDs, id)
	sm.saveInstalledMarkers()
	return nil
}

func (sm *storeManager) Installed() []AgentPackage {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	result := make([]AgentPackage, 0)
	for id := range sm.installedIDs {
		if pkg, ok := sm.packages[id]; ok {
			p := *pkg
			p.Bundle = summarizePackageBundle(p)
			p.InstalledAt = time.Now().Format(time.RFC3339)
			result = append(result, p)
		}
	}
	return result
}

func (sm *storeManager) IsInstalled(id string) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.installedIDs[id]
}

func (sm *storeManager) GetCategories() []string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	catSet := make(map[string]bool)
	for _, pkg := range sm.packages {
		if pkg.Category != "" {
			catSet[pkg.Category] = true
		}
	}
	cats := make([]string, 0, len(catSet))
	for c := range catSet {
		cats = append(cats, c)
	}
	return cats
}

func (sm *storeManager) GetTags() []string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	tagSet := make(map[string]bool)
	for _, pkg := range sm.packages {
		for _, t := range pkg.Tags {
			tagSet[t] = true
		}
	}
	tags := make([]string, 0, len(tagSet))
	for t := range tagSet {
		tags = append(tags, t)
	}
	return tags
}
