package cliadapter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

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

type EntryStatus struct {
	Entry
	Installed      bool   `json:"installed"`
	ExecutablePath string `json:"executable_path,omitempty"`
}

type Registry struct {
	mu         sync.RWMutex
	root       string
	entries    map[string]*EntryStatus
	categories map[string]int
}

func NewRegistry(root string) (*Registry, error) {
	r := &Registry{
		root:       root,
		entries:    make(map[string]*EntryStatus),
		categories: make(map[string]int),
	}

	if err := r.load(); err != nil {
		return nil, err
	}

	return r, nil
}

func (r *Registry) load() error {
	data, err := os.ReadFile(filepath.Join(r.root, "registry.json"))
	if err != nil {
		return err
	}

	var file struct {
		Meta struct {
			Repo        string `json:"repo"`
			Description string `json:"description"`
			Updated     string `json:"updated"`
		} `json:"meta"`
		CLIs []Entry `json:"clis"`
	}

	if err := json.Unmarshal(data, &file); err != nil {
		return err
	}

	for _, e := range file.CLIs {
		entry := e
		r.entries[e.Name] = &EntryStatus{
			Entry:     entry,
			Installed: false,
		}
		r.categories[e.Category]++
	}

	return nil
}

func (r *Registry) Get(name string) (*EntryStatus, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.entries[name]
	return entry, ok
}

func (r *Registry) List() []*EntryStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*EntryStatus, 0, len(r.entries))
	for _, e := range r.entries {
		result = append(result, e)
	}
	return result
}

func (r *Registry) Search(query string, category string, limit int) []*EntryStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*EntryStatus
	query = strings.ToLower(query)
	category = strings.ToLower(category)

	for _, e := range r.entries {
		if category != "" && strings.ToLower(e.Category) != category {
			continue
		}

		if query == "" {
			results = append(results, e)
			continue
		}

		if strings.Contains(strings.ToLower(e.Name), query) ||
			strings.Contains(strings.ToLower(e.DisplayName), query) ||
			strings.Contains(strings.ToLower(e.Description), query) {
			results = append(results, e)
		}

		if limit > 0 && len(results) >= limit {
			break
		}
	}

	return results
}

func (r *Registry) Categories() map[string]int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]int)
	for k, v := range r.categories {
		result[k] = v
	}
	return result
}

func (r *Registry) Root() string {
	return r.root
}

func (r *Registry) EntriesCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.entries)
}

type builtinAdapter struct {
	Name        string
	Description string
	Category    string
	Handler     func(args []string) (string, error)
}

var builtinAdapters = map[string]*builtinAdapter{}

func RegisterBuiltinAdapter(name, desc, category string, handler func(args []string) (string, error)) {
	builtinAdapters[name] = &builtinAdapter{
		Name:        name,
		Description: desc,
		Category:    category,
		Handler:     handler,
	}
}

func GetBuiltinAdapter(name string) (*builtinAdapter, bool) {
	a, ok := builtinAdapters[name]
	return a, ok
}

func ListBuiltinAdapters() []*builtinAdapter {
	result := make([]*builtinAdapter, 0, len(builtinAdapters))
	for _, a := range builtinAdapters {
		result = append(result, a)
	}
	return result
}

func (r *Registry) Find(name string) (*EntryStatus, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	name = strings.ToLower(name)

	if entry, ok := r.entries[name]; ok {
		return entry, true
	}

	for _, e := range r.entries {
		if strings.ToLower(e.EntryPoint) == name ||
			strings.ToLower(e.DisplayName) == name {
			return e, true
		}
	}

	if adapter, ok := builtinAdapters[name]; ok {
		return &EntryStatus{
			Entry: Entry{
				Name:        adapter.Name,
				DisplayName: adapter.Name,
				Description: adapter.Description,
				Category:    adapter.Category,
			},
			Installed: true,
		}, true
	}

	return nil, false
}

func (r *Registry) MarkInstalled(name string, path string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if e, ok := r.entries[name]; ok {
		e.Installed = true
		e.ExecutablePath = path
	}
}

func (r *Registry) JSON() (string, error) {
	data := map[string]any{
		"root":          r.root,
		"entries_count": r.EntriesCount(),
		"categories":    r.Categories(),
		"entries":       r.List(),
		"builtin_count": len(builtinAdapters),
	}

	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", err
	}

	return string(b), nil
}
