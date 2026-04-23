package clihub

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const EnvRoot = "ANYCLAW_CLI_ANYTHING_ROOT"

var ErrNotFound = errors.New("cli-anything root not found")

type catalogFile struct {
	Meta struct {
		Repo        string `json:"repo"`
		Description string `json:"description"`
		Updated     string `json:"updated"`
	} `json:"meta"`
	CLIs []Entry `json:"clis"`
}

type Entry struct {
	Name           string `json:"name"`
	DisplayName    string `json:"display_name"`
	Version        string `json:"version"`
	Description    string `json:"description"`
	Requires       string `json:"requires"`
	Homepage       string `json:"homepage"`
	InstallCmd     string `json:"install_cmd"`
	EntryPoint     string `json:"entry_point"`
	SkillMD        string `json:"skill_md"`
	Category       string `json:"category"`
	Contributor    string `json:"contributor"`
	ContributorURL string `json:"contributor_url"`
}

type Catalog struct {
	Root        string  `json:"root"`
	Repo        string  `json:"repo"`
	Description string  `json:"description"`
	Updated     string  `json:"updated"`
	Entries     []Entry `json:"entries"`
}

type EntryStatus struct {
	Entry
	Installed      bool   `json:"installed"`
	Runnable       bool   `json:"runnable"`
	RunMode        string `json:"run_mode,omitempty"`
	ExecutablePath string `json:"executable_path,omitempty"`
	SourcePath     string `json:"source_path,omitempty"`
	DevModule      string `json:"dev_module,omitempty"`
	SkillPath      string `json:"skill_path,omitempty"`
}

type Summary struct {
	Root           string        `json:"root"`
	Updated        string        `json:"updated"`
	EntriesCount   int           `json:"entries_count"`
	Categories     []CategoryHit `json:"categories"`
	RunnableCount  int           `json:"runnable_count"`
	Runnable       []EntryStatus `json:"runnable,omitempty"`
	InstalledCount int           `json:"installed_count"`
	Installed      []EntryStatus `json:"installed,omitempty"`
}

type CategoryHit struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func DiscoverRoot(start string) (string, bool) {
	if envRoot := strings.TrimSpace(os.Getenv(EnvRoot)); envRoot != "" {
		if root, ok := normalizeRoot(envRoot); ok {
			return root, true
		}
	}

	start = strings.TrimSpace(start)
	if start == "" {
		return "", false
	}
	absStart, err := filepath.Abs(start)
	if err != nil {
		return "", false
	}
	info, err := os.Stat(absStart)
	if err == nil && !info.IsDir() {
		absStart = filepath.Dir(absStart)
	}

	for _, dir := range ancestorDirs(absStart) {
		for _, candidate := range []string{dir, filepath.Join(dir, "CLI-Anything-0.2.0"), filepath.Join(dir, "CLI-Anything")} {
			if root, ok := normalizeRoot(candidate); ok {
				return root, true
			}
		}
	}
	return "", false
}

func LoadAuto(start string) (*Catalog, error) {
	root, ok := DiscoverRoot(start)
	if !ok {
		return nil, ErrNotFound
	}
	return Load(root)
}

func Load(root string) (*Catalog, error) {
	root, ok := normalizeRoot(root)
	if !ok {
		return nil, ErrNotFound
	}
	data, err := os.ReadFile(filepath.Join(root, "registry.json"))
	if err != nil {
		return nil, err
	}
	var file catalogFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, err
	}
	entries := append([]Entry(nil), file.CLIs...)
	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
	return &Catalog{
		Root:        root,
		Repo:        file.Meta.Repo,
		Description: file.Meta.Description,
		Updated:     file.Meta.Updated,
		Entries:     entries,
	}, nil
}

func SummaryFor(cat *Catalog, limit int) Summary {
	if cat == nil {
		return Summary{}
	}
	runnable := Runnable(cat)
	installed := Installed(cat)
	return Summary{
		Root:           cat.Root,
		Updated:        cat.Updated,
		EntriesCount:   len(cat.Entries),
		Categories:     categoryCounts(cat.Entries),
		RunnableCount:  len(runnable),
		Runnable:       limitStatuses(runnable, limit),
		InstalledCount: len(installed),
		Installed:      limitStatuses(installed, limit),
	}
}

func Search(cat *Catalog, query string, category string, installedOnly bool, limit int) []EntryStatus {
	if cat == nil {
		return nil
	}
	query = strings.TrimSpace(strings.ToLower(query))
	category = strings.TrimSpace(strings.ToLower(category))
	statuses := statusList(cat)
	matches := make([]EntryStatus, 0, len(statuses))
	for _, item := range statuses {
		if category != "" && strings.ToLower(strings.TrimSpace(item.Category)) != category {
			continue
		}
		if installedOnly && !item.Installed {
			continue
		}
		if query == "" || entryMatches(item, query) {
			matches = append(matches, item)
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Installed != matches[j].Installed {
			return matches[i].Installed
		}
		if matches[i].Runnable != matches[j].Runnable {
			return matches[i].Runnable
		}
		return strings.ToLower(matches[i].Name) < strings.ToLower(matches[j].Name)
	})
	return limitStatuses(matches, limit)
}

func Find(cat *Catalog, name string) (EntryStatus, bool) {
	if cat == nil {
		return EntryStatus{}, false
	}
	name = strings.TrimSpace(strings.ToLower(name))
	for _, item := range statusList(cat) {
		if strings.EqualFold(item.Name, name) || strings.EqualFold(item.DisplayName, name) || strings.EqualFold(item.EntryPoint, name) {
			return item, true
		}
	}
	return EntryStatus{}, false
}

func Installed(cat *Catalog) []EntryStatus {
	if cat == nil {
		return nil
	}
	items := statusList(cat)
	out := make([]EntryStatus, 0, len(items))
	for _, item := range items {
		if item.Installed {
			out = append(out, item)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

func Runnable(cat *Catalog) []EntryStatus {
	if cat == nil {
		return nil
	}
	items := statusList(cat)
	out := make([]EntryStatus, 0, len(items))
	for _, item := range items {
		if item.Runnable {
			out = append(out, item)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Installed != out[j].Installed {
			return out[i].Installed
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

func StatusLabel(item EntryStatus) string {
	switch {
	case item.Installed:
		return "installed"
	case item.Runnable:
		return "source"
	default:
		return "catalog"
	}
}

func HumanSummary(cat *Catalog) string {
	summary := SummaryFor(cat, 6)
	if summary.EntriesCount == 0 {
		return "CLI Hub unavailable"
	}
	lines := []string{
		fmt.Sprintf("CLI Hub root: %s", summary.Root),
		fmt.Sprintf("catalog updated: %s", firstNonEmpty(summary.Updated, "unknown")),
		fmt.Sprintf("catalog entries: %d", summary.EntriesCount),
		fmt.Sprintf("runnable harnesses: %d", summary.RunnableCount),
		fmt.Sprintf("installed harnesses: %d", summary.InstalledCount),
	}
	if len(summary.Categories) > 0 {
		lines = append(lines, "top categories: "+joinCategories(summary.Categories, 6))
	}
	if len(summary.Installed) > 0 {
		names := make([]string, 0, len(summary.Installed))
		for _, item := range summary.Installed {
			names = append(names, item.Name)
		}
		lines = append(lines, "installed tools: "+strings.Join(names, ", "))
	}
	if len(summary.Runnable) > 0 {
		names := make([]string, 0, len(summary.Runnable))
		for _, item := range summary.Runnable {
			label := item.Name
			if !item.Installed {
				label += " (source)"
			}
			names = append(names, label)
		}
		lines = append(lines, "runnable tools: "+strings.Join(names, ", "))
	}
	lines = append(lines, "native use: search the catalog first, then prefer clihub_exec over raw shell when a matching harness exists.")
	return strings.Join(lines, "\n")
}

func statusList(cat *Catalog) []EntryStatus {
	items := make([]EntryStatus, 0, len(cat.Entries))
	for _, entry := range cat.Entries {
		items = append(items, statusForEntry(cat.Root, entry))
	}
	return items
}

func statusForEntry(root string, entry Entry) EntryStatus {
	status := EntryStatus{Entry: entry}
	if path, err := exec.LookPath(strings.TrimSpace(entry.EntryPoint)); err == nil {
		status.Installed = true
		status.Runnable = true
		status.RunMode = "installed"
		status.ExecutablePath = path
	}
	if source := sourcePath(root, entry); source != "" {
		status.SourcePath = source
	}
	if skill := skillPath(root, entry); skill != "" {
		status.SkillPath = skill
	}
	if module := devModule(status.SourcePath); module != "" {
		status.DevModule = module
	}
	if !status.Runnable && status.SourcePath != "" && status.DevModule != "" {
		status.Runnable = true
		status.RunMode = "source"
	}
	return status
}

func sourcePath(root string, entry Entry) string {
	candidates := []string{
		filepath.Join(root, entry.Name, "agent-harness"),
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	return ""
}

func skillPath(root string, entry Entry) string {
	rel := strings.TrimSpace(entry.SkillMD)
	if rel == "" {
		return ""
	}
	path := filepath.Join(root, filepath.FromSlash(rel))
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return ""
}

func devModule(source string) string {
	if strings.TrimSpace(source) == "" {
		return ""
	}
	base := filepath.Join(source, "cli_anything")
	entries, err := os.ReadDir(base)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		moduleBase := "cli_anything." + entry.Name()
		if _, err := os.Stat(filepath.Join(base, entry.Name(), "__main__.py")); err == nil {
			return moduleBase
		}
		if _, err := os.Stat(filepath.Join(base, entry.Name(), entry.Name()+"_cli.py")); err == nil {
			return moduleBase + "." + entry.Name() + "_cli"
		}
		if matches, err := filepath.Glob(filepath.Join(base, entry.Name(), "*_cli.py")); err == nil && len(matches) > 0 {
			name := strings.TrimSuffix(filepath.Base(matches[0]), ".py")
			return moduleBase + "." + name
		}
	}
	return ""
}

func entryMatches(item EntryStatus, query string) bool {
	fields := []string{
		item.Name,
		item.DisplayName,
		item.Description,
		item.Category,
		item.EntryPoint,
		item.Requires,
		item.Contributor,
	}
	for _, field := range fields {
		if strings.Contains(strings.ToLower(field), query) {
			return true
		}
	}
	return false
}

func categoryCounts(entries []Entry) []CategoryHit {
	counts := map[string]int{}
	for _, entry := range entries {
		key := strings.TrimSpace(entry.Category)
		if key == "" {
			key = "other"
		}
		counts[key]++
	}
	items := make([]CategoryHit, 0, len(counts))
	for key, count := range counts {
		items = append(items, CategoryHit{Name: key, Count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count == items[j].Count {
			return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
		}
		return items[i].Count > items[j].Count
	})
	return items
}

func limitStatuses(items []EntryStatus, limit int) []EntryStatus {
	if limit <= 0 || limit >= len(items) {
		return append([]EntryStatus(nil), items...)
	}
	return append([]EntryStatus(nil), items[:limit]...)
}

func joinCategories(items []CategoryHit, limit int) string {
	if limit > 0 && limit < len(items) {
		items = items[:limit]
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, fmt.Sprintf("%s (%d)", item.Name, item.Count))
	}
	return strings.Join(parts, ", ")
}

func normalizeRoot(candidate string) (string, bool) {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return "", false
	}
	abs, err := filepath.Abs(candidate)
	if err != nil {
		return "", false
	}
	if _, err := os.Stat(filepath.Join(abs, "registry.json")); err == nil {
		return abs, true
	}
	return "", false
}

func ancestorDirs(start string) []string {
	var dirs []string
	current := filepath.Clean(start)
	for {
		dirs = append(dirs, current)
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return dirs
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
