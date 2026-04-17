package memory

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

type FileMemory struct {
	baseDir  string
	dailyDir string
	mu       sync.RWMutex
}

type MemoryEntry struct {
	ID        string            `json:"id"`
	Timestamp time.Time         `json:"timestamp"`
	Type      string            `json:"type"`
	Role      string            `json:"role,omitempty"`
	Content   string            `json:"content"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type MemoryIndex struct {
	Entries []MemoryEntry `json:"entries"`
	Updated time.Time     `json:"updated"`
}

const (
	TypeConversation = "conversation"
	TypeReflection   = "reflection"
	TypeFact         = "fact"
)

func NewFileMemory(workDir string) *FileMemory {
	memoryDir := filepath.Join(workDir, "memory")
	return &FileMemory{
		baseDir:  memoryDir,
		dailyDir: memoryDir,
	}
}

func (m *FileMemory) Init() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	dirs := []string{
		filepath.Join(m.baseDir, TypeConversation),
		filepath.Join(m.baseDir, TypeReflection),
		filepath.Join(m.baseDir, TypeFact),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	if strings.TrimSpace(m.dailyDir) != "" {
		if err := os.MkdirAll(m.dailyDir, 0o755); err != nil {
			return err
		}
	}

	return nil
}

func (m *FileMemory) Add(entry MemoryEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if entry.ID == "" {
		entry.ID = fmt.Sprintf("%d-%s", time.Now().UnixMilli(), randomID(8))
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	if err := m.initLocked(); err != nil {
		return err
	}

	dir := m.getDirForType(entry.Type)
	filename := fmt.Sprintf("%s.json", entry.ID)
	path := filepath.Join(dir, filename)

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}

	if err := m.updateIndexLocked(entry); err != nil {
		return err
	}
	return m.appendDailyMarkdownLocked(entry)
}

func (m *FileMemory) initLocked() error {
	dirs := []string{
		filepath.Join(m.baseDir, TypeConversation),
		filepath.Join(m.baseDir, TypeReflection),
		filepath.Join(m.baseDir, TypeFact),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	if strings.TrimSpace(m.dailyDir) != "" {
		if err := os.MkdirAll(m.dailyDir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (m *FileMemory) Get(id string) (*MemoryEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entries, err := m.listLocked()
	if err != nil {
		return nil, err
	}

	for i := range entries {
		if entries[i].ID == id {
			return &entries[i], nil
		}
	}
	return nil, fmt.Errorf("memory entry not found: %s", id)
}

func (m *FileMemory) Search(query string, limit int) ([]MemoryEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entries, err := m.listLocked()
	if err != nil {
		return nil, err
	}

	query = strings.ToLower(query)
	var results []MemoryEntry

	for _, entry := range entries {
		if strings.Contains(strings.ToLower(entry.Content), query) {
			results = append(results, entry)
			if limit > 0 && len(results) >= limit {
				break
			}
		}
	}

	return results, nil
}

func (m *FileMemory) List() ([]MemoryEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.listLocked()
}

func (m *FileMemory) listLocked() ([]MemoryEntry, error) {
	if err := m.initLocked(); err != nil {
		return nil, err
	}

	dirs := []string{
		filepath.Join(m.baseDir, TypeConversation),
		filepath.Join(m.baseDir, TypeReflection),
		filepath.Join(m.baseDir, TypeFact),
	}

	var entries []MemoryEntry
	for _, dir := range dirs {
		files, err := filepath.Glob(filepath.Join(dir, "*.json"))
		if err != nil {
			continue
		}

		for _, file := range files {
			data, err := os.ReadFile(file)
			if err != nil {
				continue
			}

			var entry MemoryEntry
			if err := json.Unmarshal(data, &entry); err != nil {
				continue
			}
			entries = append(entries, entry)
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.After(entries[j].Timestamp)
	})

	return entries, nil
}

func (m *FileMemory) GetConversationHistory(limit int) ([]MemoryEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entries, err := m.listLocked()
	if err != nil {
		return nil, err
	}

	var history []MemoryEntry
	for _, e := range entries {
		if e.Type == TypeConversation {
			history = append(history, e)
			if limit > 0 && len(history) >= limit {
				break
			}
		}
	}

	sort.Slice(history, func(i, j int) bool {
		return history[i].Timestamp.Before(history[j].Timestamp)
	})

	return history, nil
}

func (m *FileMemory) AddReflection(content string, metadata map[string]string) error {
	return m.Add(MemoryEntry{Type: TypeReflection, Content: content, Metadata: metadata})
}

func (m *FileMemory) AddFact(content string, metadata map[string]string) error {
	return m.Add(MemoryEntry{Type: TypeFact, Content: content, Metadata: metadata})
}

func (m *FileMemory) FormatAsMarkdown() (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entries, err := m.listLocked()
	if err != nil {
		return "", err
	}

	if len(entries) == 0 {
		return "# Memory\n\n(No entries)", nil
	}

	var sb strings.Builder
	sb.WriteString("# Memory\n\n")

	for _, entry := range entries {
		sb.WriteString(fmt.Sprintf("## [%s] %s - %s\n\n%s\n\n",
			entry.Type, entry.ID, entry.Timestamp.Format("2006-01-02 15:04"), entry.Content))
	}

	return sb.String(), nil
}

func (m *FileMemory) getDirForType(memType string) string {
	switch memType {
	case TypeConversation:
		return filepath.Join(m.baseDir, TypeConversation)
	case TypeReflection:
		return filepath.Join(m.baseDir, TypeReflection)
	case TypeFact:
		return filepath.Join(m.baseDir, TypeFact)
	default:
		return filepath.Join(m.baseDir, "other")
	}
}

func (m *FileMemory) updateIndexLocked(entry MemoryEntry) error {
	indexPath := filepath.Join(m.baseDir, "index.json")

	var index MemoryIndex
	data, err := os.ReadFile(indexPath)
	if err == nil {
		json.Unmarshal(data, &index)
	}

	index.Entries = append(index.Entries, entry)
	index.Updated = time.Now()

	data, err = json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(indexPath, data, 0o644)
}

func (m *FileMemory) Close() error {
	return nil
}

func (m *FileMemory) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.initLocked(); err != nil {
		return err
	}

	dirs := []string{
		filepath.Join(m.baseDir, TypeConversation),
		filepath.Join(m.baseDir, TypeReflection),
		filepath.Join(m.baseDir, TypeFact),
	}

	for _, dir := range dirs {
		path := filepath.Join(dir, fmt.Sprintf("%s.json", id))
		if err := os.Remove(path); err == nil {
			return nil
		}
	}

	return fmt.Errorf("memory entry not found: %s", id)
}

func (m *FileMemory) GetStats() (map[string]int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entries, err := m.listLocked()
	if err != nil {
		return nil, err
	}

	stats := make(map[string]int)
	stats["total"] = len(entries)

	for _, e := range entries {
		stats[e.Type]++
	}

	return stats, nil
}

func randomID(length int) string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, length)
	for i := range result {
		result[i] = chars[time.Now().UnixNano()%int64(len(chars))]
	}
	return string(result)
}
