package canvas

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type CanvasEntry struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Content   string    `json:"content"`
	Type      string    `json:"type"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Version   int       `json:"version"`
}

type CanvasVersion struct {
	Version   int       `json:"version"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
	Agent     string    `json:"agent,omitempty"`
}

type CanvasStore struct {
	mu          sync.RWMutex
	baseDir     string
	entries     map[string]*CanvasEntry
	versions    map[string][]*CanvasVersion
	maxVersions int
}

const (
	EntryTypeHTML = "html"
	EntryTypeA2UI = "a2ui"
	EntryTypeMD   = "markdown"
	EntryTypeJSON = "json"
	EntryTypeText = "text"
)

func NewStore(baseDir string, maxVersions int) (*CanvasStore, error) {
	if maxVersions <= 0 {
		maxVersions = 20
	}
	canvasDir := filepath.Join(baseDir, "canvas")
	if err := os.MkdirAll(canvasDir, 0o755); err != nil {
		return nil, fmt.Errorf("create canvas dir: %w", err)
	}

	store := &CanvasStore{
		baseDir:     canvasDir,
		entries:     make(map[string]*CanvasEntry),
		versions:    make(map[string][]*CanvasVersion),
		maxVersions: maxVersions,
	}

	if err := store.load(); err != nil {
		return nil, fmt.Errorf("load canvas store: %w", err)
	}

	return store, nil
}

func (s *CanvasStore) entryDir() string {
	return filepath.Join(s.baseDir, "entries")
}

func (s *CanvasStore) versionDir(id string) string {
	return filepath.Join(s.baseDir, "versions", id)
}

func (s *CanvasStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entryDir := s.entryDir()
	if _, err := os.Stat(entryDir); os.IsNotExist(err) {
		return nil
	}

	entries, err := os.ReadDir(entryDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(entryDir, entry.Name()))
		if err != nil {
			continue
		}
		var ce CanvasEntry
		if err := json.Unmarshal(data, &ce); err != nil {
			continue
		}
		s.entries[ce.ID] = &ce
		s.loadVersions(ce.ID)
	}

	return nil
}

func (s *CanvasStore) loadVersions(id string) {
	vDir := s.versionDir(id)
	entries, err := os.ReadDir(vDir)
	if err != nil {
		return
	}

	versions := make([]*CanvasVersion, 0)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(vDir, e.Name()))
		if err != nil {
			continue
		}
		var v CanvasVersion
		if err := json.Unmarshal(data, &v); err != nil {
			continue
		}
		versions = append(versions, &v)
	}

	sort.Slice(versions, func(i, j int) bool {
		return versions[i].Version < versions[j].Version
	})
	s.versions[id] = versions
}

func (s *CanvasStore) Push(id string, name string, content string, contentType string, agent string) (*CanvasEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()

	if strings.TrimSpace(id) == "" {
		id = fmt.Sprintf("canvas-%d", now.UnixMilli())
	}

	if strings.TrimSpace(contentType) == "" {
		contentType = EntryTypeHTML
	}

	entry, exists := s.entries[id]
	if exists {
		s.saveVersionLocked(entry, agent)
		entry.Content = content
		entry.UpdatedAt = now
		entry.Version++
	} else {
		if strings.TrimSpace(name) == "" {
			name = id
		}
		entry = &CanvasEntry{
			ID:        id,
			Name:      name,
			Content:   content,
			Type:      contentType,
			CreatedAt: now,
			UpdatedAt: now,
			Version:   1,
		}
		s.entries[id] = entry
	}

	if err := s.saveEntry(entry); err != nil {
		return nil, err
	}

	return cloneEntry(entry), nil
}

func (s *CanvasStore) saveVersionLocked(entry *CanvasEntry, agent string) {
	vDir := s.versionDir(entry.ID)
	if err := os.MkdirAll(vDir, 0o755); err != nil {
		return
	}

	version := &CanvasVersion{
		Version:   entry.Version,
		Content:   entry.Content,
		Timestamp: time.Now().UTC(),
		Agent:     agent,
	}

	data, err := json.MarshalIndent(version, "", "  ")
	if err != nil {
		return
	}

	versionFile := filepath.Join(vDir, fmt.Sprintf("v%d.json", entry.Version))
	_ = os.WriteFile(versionFile, data, 0o644)

	if s.versions[entry.ID] == nil {
		s.versions[entry.ID] = make([]*CanvasVersion, 0)
	}
	s.versions[entry.ID] = append(s.versions[entry.ID], version)

	s.pruneVersions(entry.ID)
}

func (s *CanvasStore) pruneVersions(id string) {
	versions := s.versions[id]
	if len(versions) <= s.maxVersions {
		return
	}

	vDir := s.versionDir(id)
	toRemove := versions[:len(versions)-s.maxVersions]
	for _, v := range toRemove {
		_ = os.Remove(filepath.Join(vDir, fmt.Sprintf("v%d.json", v.Version)))
	}
	s.versions[id] = versions[len(toRemove):]
}

func (s *CanvasStore) saveEntry(entry *CanvasEntry) error {
	entryDir := s.entryDir()
	if err := os.MkdirAll(entryDir, 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(entryDir, entry.ID+".json"), data, 0o644)
}

func (s *CanvasStore) Get(id string) (*CanvasEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.entries[id]
	if !ok {
		return nil, false
	}
	return cloneEntry(entry), true
}

func (s *CanvasStore) List() []*CanvasEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]*CanvasEntry, 0, len(s.entries))
	for _, entry := range s.entries {
		items = append(items, cloneEntry(entry))
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})

	return items
}

func (s *CanvasStore) GetVersions(id string, limit int) []*CanvasVersion {
	s.mu.RLock()
	defer s.mu.RUnlock()

	versions := s.versions[id]
	if len(versions) == 0 {
		return nil
	}

	result := make([]*CanvasVersion, len(versions))
	for i, v := range versions {
		result[i] = cloneVersion(v)
	}

	if limit > 0 && len(result) > limit {
		result = result[len(result)-limit:]
	}

	return result
}

func (s *CanvasStore) GetVersion(id string, version int) (*CanvasVersion, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	versions := s.versions[id]
	for _, v := range versions {
		if v.Version == version {
			return cloneVersion(v), true
		}
	}
	return nil, false
}

func (s *CanvasStore) Reset(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.entries[id]
	if !ok {
		return fmt.Errorf("canvas entry not found: %s", id)
	}

	s.saveVersionLocked(entry, "system")

	entry.Content = ""
	entry.UpdatedAt = time.Now().UTC()
	entry.Version++

	return s.saveEntry(entry)
}

func (s *CanvasStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.entries, id)

	entryDir := s.entryDir()
	_ = os.Remove(filepath.Join(entryDir, id+".json"))

	vDir := s.versionDir(id)
	_ = os.RemoveAll(vDir)
	delete(s.versions, id)

	return nil
}

func (s *CanvasStore) BaseDir() string {
	return s.baseDir
}

func cloneEntry(e *CanvasEntry) *CanvasEntry {
	if e == nil {
		return nil
	}
	clone := *e
	return &clone
}

func cloneVersion(v *CanvasVersion) *CanvasVersion {
	if v == nil {
		return nil
	}
	clone := *v
	return &clone
}
