package memory

import (
	cryptorand "crypto/rand"
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
		suffix, err := randomID(8)
		if err != nil {
			return fmt.Errorf("generate memory id: %w", err)
		}
		entry.ID = fmt.Sprintf("%d-%s", time.Now().UnixMilli(), suffix)
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

func randomID(length int) (string, error) {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"

	if length <= 0 {
		return "", nil
	}

	result := make([]byte, length)
	randomBytes := make([]byte, length)
	maxRandomByte := byte(256 - (256 % len(chars)))

	for i := 0; i < length; {
		if _, err := cryptorand.Read(randomBytes); err != nil {
			return "", err
		}
		for _, b := range randomBytes {
			if b >= maxRandomByte {
				continue
			}
			result[i] = chars[int(b)%len(chars)]
			i++
			if i == length {
				break
			}
		}
	}

	return string(result), nil
}
