package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type DailyMemoryMatch struct {
	Date    string `json:"date"`
	Path    string `json:"path"`
	Snippet string `json:"snippet"`
}

type DailyMemoryFile struct {
	Date    string `json:"date"`
	Path    string `json:"path"`
	Content string `json:"content"`
}

func (m *FileMemory) SetDailyDir(dir string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	dir = strings.TrimSpace(dir)
	if dir == "" {
		m.dailyDir = m.baseDir
		return
	}
	m.dailyDir = filepath.Clean(dir)
}

func (m *FileMemory) DailyDir() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if strings.TrimSpace(m.dailyDir) == "" {
		return m.baseDir
	}
	return m.dailyDir
}

func (m *FileMemory) SearchDaily(query string, limit int, dayRef string) ([]DailyMemoryMatch, error) {
	return SearchDailyMarkdown(m.DailyDir(), query, limit, dayRef)
}

func (m *FileMemory) GetDaily(dayRef string) (*DailyMemoryFile, error) {
	return GetDailyMarkdown(m.DailyDir(), dayRef)
}

func SearchDailyMarkdown(memoryDir string, query string, limit int, dayRef string) ([]DailyMemoryMatch, error) {
	return searchDailyMarkdownAt(memoryDir, query, limit, dayRef, time.Now())
}

func GetDailyMarkdown(memoryDir string, dayRef string) (*DailyMemoryFile, error) {
	return getDailyMarkdownAt(memoryDir, dayRef, time.Now())
}

func searchDailyMarkdownAt(memoryDir string, query string, limit int, dayRef string, now time.Time) ([]DailyMemoryMatch, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}

	files, err := selectDailyFiles(memoryDir, dayRef, now)
	if err != nil {
		return nil, err
	}

	if limit <= 0 {
		limit = 5
	}

	results := make([]DailyMemoryMatch, 0, limit)
	for _, item := range files {
		data, err := os.ReadFile(item.Path)
		if err != nil {
			return nil, err
		}
		content := normalizeMemoryText(string(data))
		if !strings.Contains(strings.ToLower(content), strings.ToLower(query)) {
			continue
		}
		results = append(results, DailyMemoryMatch{
			Date:    item.Date,
			Path:    item.Path,
			Snippet: snippetAround(content, query, 80),
		})
		if len(results) >= limit {
			break
		}
	}

	return results, nil
}

func getDailyMarkdownAt(memoryDir string, dayRef string, now time.Time) (*DailyMemoryFile, error) {
	files, err := selectDailyFiles(memoryDir, dayRef, now)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("daily memory not found")
	}

	data, err := os.ReadFile(files[0].Path)
	if err != nil {
		return nil, err
	}

	return &DailyMemoryFile{
		Date:    files[0].Date,
		Path:    files[0].Path,
		Content: normalizeMemoryText(string(data)),
	}, nil
}

func (m *FileMemory) appendDailyMarkdownLocked(entry MemoryEntry) error {
	dailyDir := strings.TrimSpace(m.dailyDir)
	if dailyDir == "" {
		dailyDir = m.baseDir
	}
	if err := os.MkdirAll(dailyDir, 0o755); err != nil {
		return err
	}

	day := entry.Timestamp.Format("2006-01-02")
	path := filepath.Join(dailyDir, day+".md")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Size() == 0 {
		if _, err := file.WriteString(fmt.Sprintf("# Daily Memory %s\n\n", day)); err != nil {
			return err
		}
	}

	_, err = file.WriteString(formatDailyEntry(entry))
	return err
}

func formatDailyEntry(entry MemoryEntry) string {
	var sb strings.Builder

	label := strings.TrimSpace(entry.Type)
	if role := strings.TrimSpace(entry.Role); role != "" {
		label = label + ":" + role
	}

	sb.WriteString(fmt.Sprintf("## %s [%s] %s\n\n", entry.Timestamp.Format("15:04:05"), label, entry.ID))
	sb.WriteString(strings.TrimSpace(normalizeMemoryText(entry.Content)))
	sb.WriteString("\n")

	if len(entry.Metadata) > 0 {
		keys := make([]string, 0, len(entry.Metadata))
		for key := range entry.Metadata {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		sb.WriteString("\nMetadata:\n")
		for _, key := range keys {
			sb.WriteString(fmt.Sprintf("- %s: %s\n", key, entry.Metadata[key]))
		}
	}

	sb.WriteString("\n")
	return sb.String()
}

type dailyMemoryPath struct {
	Date string
	Path string
}

func selectDailyFiles(memoryDir string, dayRef string, now time.Time) ([]dailyMemoryPath, error) {
	memoryDir = strings.TrimSpace(memoryDir)
	if memoryDir == "" {
		memoryDir = "memory"
	}
	if err := os.MkdirAll(memoryDir, 0o755); err != nil {
		return nil, err
	}

	normalized, latestOnly, err := normalizeDayRef(dayRef, now)
	if err != nil {
		return nil, err
	}

	all, err := listDailyMarkdownFiles(memoryDir)
	if err != nil {
		return nil, err
	}
	if latestOnly {
		if len(all) == 0 {
			return nil, fmt.Errorf("no daily memory files found")
		}
		return all[:1], nil
	}
	if normalized == "" {
		return all, nil
	}
	for _, item := range all {
		if item.Date == normalized {
			return []dailyMemoryPath{item}, nil
		}
	}
	return nil, fmt.Errorf("daily memory not found for %s", normalized)
}

func listDailyMarkdownFiles(memoryDir string) ([]dailyMemoryPath, error) {
	entries, err := os.ReadDir(memoryDir)
	if err != nil {
		return nil, err
	}

	items := make([]dailyMemoryPath, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.TrimSpace(entry.Name())
		if filepath.Ext(name) != ".md" {
			continue
		}
		date := strings.TrimSuffix(name, filepath.Ext(name))
		if _, err := time.Parse("2006-01-02", date); err != nil {
			continue
		}
		items = append(items, dailyMemoryPath{
			Date: date,
			Path: filepath.Join(memoryDir, name),
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Date > items[j].Date
	})
	return items, nil
}

func normalizeDayRef(dayRef string, now time.Time) (string, bool, error) {
	ref := strings.TrimSpace(strings.ToLower(dayRef))
	switch ref {
	case "":
		return "", false, nil
	case "today":
		return now.Format("2006-01-02"), false, nil
	case "yesterday":
		return now.AddDate(0, 0, -1).Format("2006-01-02"), false, nil
	case "latest", "newest", "recent":
		return "", true, nil
	default:
		ref = strings.TrimSuffix(ref, ".md")
		if _, err := time.Parse("2006-01-02", ref); err != nil {
			return "", false, fmt.Errorf("invalid memory date %q: use YYYY-MM-DD, today, yesterday, or latest", dayRef)
		}
		return ref, false, nil
	}
}

func normalizeMemoryText(input string) string {
	input = strings.ReplaceAll(input, "\r\n", "\n")
	return strings.ReplaceAll(input, "\r", "\n")
}

func snippetAround(content string, query string, radius int) string {
	contentRunes := []rune(content)
	lowerContent := []rune(strings.ToLower(content))
	lowerQuery := []rune(strings.ToLower(strings.TrimSpace(query)))
	index := indexRunes(lowerContent, lowerQuery)
	if index < 0 {
		normalized := strings.Join(strings.Fields(content), " ")
		if len([]rune(normalized)) > radius*2 {
			return truncateRunes(normalized, radius*2) + "..."
		}
		return normalized
	}

	start := index - radius
	if start < 0 {
		start = 0
	}
	end := index + len(lowerQuery) + radius
	if end > len(contentRunes) {
		end = len(contentRunes)
	}

	snippet := strings.Join(strings.Fields(string(contentRunes[start:end])), " ")
	if start > 0 {
		snippet = "..." + snippet
	}
	if end < len(contentRunes) {
		snippet = snippet + "..."
	}
	return snippet
}

func indexRunes(content []rune, query []rune) int {
	if len(query) == 0 || len(content) < len(query) {
		return -1
	}
	for i := 0; i <= len(content)-len(query); i++ {
		matched := true
		for j := range query {
			if content[i+j] != query[j] {
				matched = false
				break
			}
		}
		if matched {
			return i
		}
	}
	return -1
}

func truncateRunes(input string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(input)
	if len(runes) <= limit {
		return input
	}
	return string(runes[:limit])
}
