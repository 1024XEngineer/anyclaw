package clihub

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type Capability struct {
	Harness    string    `json:"harness"`
	Group      string    `json:"group"`
	Command    string    `json:"command"`
	Action     string    `json:"action"`
	Keywords   []string  `json:"keywords"`
	Args       []ArgSpec `json:"args"`
	SourcePath string    `json:"source_path"`
	DevModule  string    `json:"dev_module"`
	Category   string    `json:"category"`
}

type ArgSpec struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Type        string `json:"type"`
}

type CapabilityRegistry struct {
	Root         string
	Capabilities []Capability
	harnessIndex map[string][]int
	keywordIndex map[string][]int
	actionIndex  map[string][]int
}

func LoadCapabilityRegistry(root string) (*CapabilityRegistry, error) {
	cat, err := Load(root)
	if err != nil {
		return nil, err
	}

	registry := &CapabilityRegistry{
		Root:         root,
		Capabilities: []Capability{},
		harnessIndex: make(map[string][]int),
		keywordIndex: make(map[string][]int),
		actionIndex:  make(map[string][]int),
	}

	skills := LoadSkillsForCatalog(cat)

	for _, entry := range cat.Entries {
		status := statusForEntry(root, entry)
		skill := skills[entry.Name]

		if skill == nil {
			continue
		}

		for _, cmd := range skill.Commands {
			keywords := generateKeywords(entry.Name, cmd)

			cap := Capability{
				Harness:    entry.Name,
				Group:      cmd.Group,
				Command:    cmd.Name,
				Action:     fmt.Sprintf("%s %s", cmd.Group, cmd.Name),
				Keywords:   keywords,
				SourcePath: status.SourcePath,
				DevModule:  status.DevModule,
				Category:   entry.Category,
			}

			idx := len(registry.Capabilities)
			registry.Capabilities = append(registry.Capabilities, cap)

			registry.harnessIndex[strings.ToLower(entry.Name)] = append(registry.harnessIndex[strings.ToLower(entry.Name)], idx)

			for _, kw := range keywords {
				lower := strings.ToLower(kw)
				registry.keywordIndex[lower] = append(registry.keywordIndex[lower], idx)
			}

			if cmd.Group != "" {
				groupLower := strings.ToLower(cmd.Group)
				registry.actionIndex[groupLower] = append(registry.actionIndex[groupLower], idx)
			}
		}
	}

	return registry, nil
}

func generateKeywords(harness string, cmd Command) []string {
	var keywords []string

	harnessLower := strings.ToLower(harness)
	cmdLower := strings.ToLower(cmd.Name)
	groupLower := strings.ToLower(cmd.Group)

	keywords = append(keywords, harnessLower)
	keywords = append(keywords, cmdLower)

	if groupLower != "" {
		keywords = append(keywords, groupLower)
	}

	actionVerbs := map[string][]string{
		"new":     {"create", "make", "add", "start", "init"},
		"list":    {"show", "get", "display", "view", "ls"},
		"add":     {"insert", "put", "include", "attach"},
		"remove":  {"delete", "drop", "clear", "rm"},
		"set":     {"configure", "update", "change", "modify", "assign"},
		"get":     {"retrieve", "fetch", "obtain", "read"},
		"export":  {"render", "save", "output", "convert"},
		"import":  {"load", "open", "read", "input"},
		"start":   {"launch", "run", "begin", "open"},
		"stop":    {"terminate", "end", "close", "quit"},
		"info":    {"show", "details", "status", "info"},
		"search":  {"find", "query", "lookup", "filter"},
		"install": {"setup", "install", "add", "enable"},
	}

	if verbs, ok := actionVerbs[cmdLower]; ok {
		keywords = append(keywords, verbs...)
	}

	descWords := strings.Fields(strings.ToLower(cmd.Description))
	for _, word := range descWords {
		if len(word) > 3 {
			keywords = append(keywords, word)
		}
	}

	return keywords
}

func (r *CapabilityRegistry) FindByIntent(query string) []Capability {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil
	}

	queryWords := strings.Fields(query)

	var scored []struct {
		cap     Capability
		score   int
		matched []string
	}

	for _, cap := range r.Capabilities {
		score := 0
		var matched []string

		for _, qw := range queryWords {
			qw = strings.Trim(qw, "`\"")

			if strings.Contains(strings.ToLower(cap.Harness), qw) && qw == strings.ToLower(cap.Harness) {
				score += 10
				matched = append(matched, cap.Harness)
			}

			if strings.Contains(strings.ToLower(cap.Command), qw) {
				score += 8
				matched = append(matched, cap.Command)
			}

			if strings.Contains(strings.ToLower(cap.Group), qw) {
				score += 5
				matched = append(matched, cap.Group)
			}

			for _, kw := range cap.Keywords {
				if strings.Contains(kw, qw) || qw == kw {
					score += 1
					matched = append(matched, kw)
				}
			}
		}

		if strings.Contains(query, "video") && cap.Category == "video" {
			score += 20
		}
		if strings.Contains(query, "audio") && cap.Category == "audio" {
			score += 20
		}
		if strings.Contains(query, "office") && cap.Category == "office" {
			score += 20
		}

		if strings.HasPrefix(query, "create ") || strings.HasPrefix(query, "new ") {
			if cap.Command == "new" {
				score += 15
			}
		}
		if strings.HasPrefix(query, "list ") || strings.HasPrefix(query, "show ") {
			if cap.Command == "list" {
				score += 15
			}
		}
		if strings.HasPrefix(query, "export ") || strings.HasPrefix(query, "render ") {
			if cap.Command == "render" {
				score += 25
				if cap.Group == "Export" {
					score += 15
				}
			}
			if cap.Command == "export" && cap.Category == "office" {
				score += 10
			}
			if cap.Command == "export" && cap.Category == "" {
				score += 5
			}
		}

		if strings.Contains(query, "timeline") {
			if cap.Group == "Timeline" {
				score += 20
			}
		}
		if strings.Contains(query, "project") {
			if cap.Group == "Project" {
				score += 15
			}
		}

		if strings.Contains(query, "clip") {
			if strings.Contains(strings.ToLower(cap.Command), "clip") || strings.Contains(strings.ToLower(cap.Group), "clip") {
				score += 15
			}
		}

		if score > 0 {
			scored = append(scored, struct {
				cap     Capability
				score   int
				matched []string
			}{cap, score, matched})
		}
	}

	sortSliceByScore(scored)

	var result []Capability
	for _, s := range scored {
		result = append(result, s.cap)
	}

	return result
}

func sortSliceByScore(scored []struct {
	cap     Capability
	score   int
	matched []string
}) {
	for i := 0; i < len(scored)-1; i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].score > scored[i].score {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}
}

func (r *CapabilityRegistry) FindByHarness(harness string) []Capability {
	harness = strings.ToLower(harness)
	if idxs, ok := r.harnessIndex[harness]; ok {
		result := make([]Capability, len(idxs))
		for i, idx := range idxs {
			result[i] = r.Capabilities[idx]
		}
		return result
	}
	return nil
}

func (r *CapabilityRegistry) FindByAction(action string) []Capability {
	action = strings.ToLower(action)
	if idxs, ok := r.actionIndex[action]; ok {
		result := make([]Capability, len(idxs))
		for i, idx := range idxs {
			result[i] = r.Capabilities[idx]
		}
		return result
	}
	return nil
}

func (r *CapabilityRegistry) BestMatch(query string) *Capability {
	matches := r.FindByIntent(query)
	if len(matches) > 0 {
		return &matches[0]
	}
	return nil
}

func (r *CapabilityRegistry) All() []Capability {
	return r.Capabilities
}

func (r *CapabilityRegistry) Count() int {
	return len(r.Capabilities)
}

func (r *CapabilityRegistry) Categories() []string {
	catMap := make(map[string]bool)
	for _, cap := range r.Capabilities {
		if cap.Category != "" {
			catMap[cap.Category] = true
		}
	}

	var result []string
	for cat := range catMap {
		result = append(result, cat)
	}
	return result
}

func (r *CapabilityRegistry) Harnesses() []string {
	harnessMap := make(map[string]bool)
	for _, cap := range r.Capabilities {
		harnessMap[cap.Harness] = true
	}

	var result []string
	for h := range harnessMap {
		result = append(result, h)
	}
	return result
}

func HumanCapabilitySummary(reg *CapabilityRegistry) string {
	if reg == nil || reg.Count() == 0 {
		return "No capabilities available"
	}

	lines := []string{
		fmt.Sprintf("CLI Hub Capabilities: %d commands across %d harnesses", reg.Count(), len(reg.Harnesses())),
		fmt.Sprintf("Categories: %s", strings.Join(reg.Categories(), ", ")),
	}

	byHarness := make(map[string][]Capability)
	for _, cap := range reg.Capabilities {
		byHarness[cap.Harness] = append(byHarness[cap.Harness], cap)
	}

	lines = append(lines, "Available capabilities:")
	for _, harness := range reg.Harnesses() {
		caps := byHarness[harness]
		var cmds []string
		for _, cap := range caps {
			cmds = append(cmds, cap.Command)
		}
		lines = append(lines, fmt.Sprintf("  - %s: %s", harness, strings.Join(cmds, ", ")))
	}

	return strings.Join(lines, "\n")
}

func SaveCapabilityRegistry(reg *CapabilityRegistry, path string) error {
	data, err := json.MarshalIndent(reg.Capabilities, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func LoadCapabilityRegistryFromFile(path string) ([]Capability, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var capabilities []Capability
	if err := json.Unmarshal(data, &capabilities); err != nil {
		return nil, err
	}
	return capabilities, nil
}

func ResolveCapabilityPath(root string, capability Capability) ([]string, string, error) {
	if capability.DevModule != "" && capability.SourcePath != "" {
		args, err := pythonModuleArgs(capability.DevModule)
		if err != nil {
			return nil, "", err
		}
		return args, capability.SourcePath, nil
	}
	return nil, "", fmt.Errorf("capability %s/%s not runnable", capability.Harness, capability.Command)
}

func FindCapabilityPath(root string, harness string, command string) string {
	reg, err := LoadCapabilityRegistry(root)
	if err != nil {
		return ""
	}

	for _, cap := range reg.Capabilities {
		if strings.EqualFold(cap.Harness, harness) && strings.EqualFold(cap.Command, command) {
			if cap.DevModule != "" {
				return cap.SourcePath
			}
		}
	}
	return ""
}
