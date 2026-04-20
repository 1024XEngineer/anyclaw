package memory

import (
	"fmt"
	"sort"
	"strings"
)

// MemoryService composes higher-level memory workflows on top of a storage backend.
type MemoryService struct {
	backend MemoryBackend
}

func NewMemoryService(backend MemoryBackend) *MemoryService {
	return &MemoryService{backend: backend}
}

func (s *MemoryService) AddReflection(content string, metadata map[string]string) error {
	return s.addTypedEntry(TypeReflection, content, metadata)
}

func (s *MemoryService) AddFact(content string, metadata map[string]string) error {
	return s.addTypedEntry(TypeFact, content, metadata)
}

func (s *MemoryService) GetConversationHistory(limit int) ([]MemoryEntry, error) {
	entries, err := s.listEntries()
	if err != nil {
		return nil, err
	}

	history := make([]MemoryEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Type != TypeConversation {
			continue
		}
		history = append(history, entry)
	}

	sort.Slice(history, func(i, j int) bool {
		return history[i].Timestamp.After(history[j].Timestamp)
	})
	if limit > 0 && len(history) > limit {
		history = history[:limit]
	}
	sort.Slice(history, func(i, j int) bool {
		return history[i].Timestamp.Before(history[j].Timestamp)
	})

	return history, nil
}

func (s *MemoryService) FormatAsMarkdown() (string, error) {
	entries, err := s.listEntries()
	if err != nil {
		return "", err
	}

	if len(entries) == 0 {
		return "# Memory\n\n(No entries)", nil
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.After(entries[j].Timestamp)
	})

	var sb strings.Builder
	sb.WriteString("# Memory\n\n")
	for _, entry := range entries {
		sb.WriteString(fmt.Sprintf("## [%s] %s - %s\n\n%s\n\n",
			entry.Type, entry.ID, entry.Timestamp.Format("2006-01-02 15:04"), entry.Content))
	}

	return sb.String(), nil
}

func (s *MemoryService) GetStats() (map[string]int, error) {
	entries, err := s.listEntries()
	if err != nil {
		return nil, err
	}

	stats := map[string]int{"total": len(entries)}
	for _, entry := range entries {
		stats[entry.Type]++
	}

	return stats, nil
}

func (s *MemoryService) addTypedEntry(entryType string, content string, metadata map[string]string) error {
	if s.backend == nil {
		return fmt.Errorf("memory backend is nil")
	}

	return s.backend.Add(MemoryEntry{
		Type:     entryType,
		Content:  content,
		Metadata: metadata,
	})
}

func (s *MemoryService) listEntries() ([]MemoryEntry, error) {
	if s.backend == nil {
		return nil, fmt.Errorf("memory backend is nil")
	}

	return s.backend.List()
}
