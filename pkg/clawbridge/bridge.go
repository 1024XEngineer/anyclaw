package clawbridge

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const EnvRoot = "ANYCLAW_CLAW_CODE_ROOT"

var ErrNotFound = errors.New("claw-code-main bridge not found")

type snapshotEntry struct {
	Name           string `json:"name"`
	SourceHint     string `json:"source_hint"`
	Responsibility string `json:"responsibility"`
}

type subsystemRecord struct {
	ArchiveName string   `json:"archive_name"`
	PackageName string   `json:"package_name"`
	ModuleCount int      `json:"module_count"`
	SampleFiles []string `json:"sample_files"`
}

type FamilySummary struct {
	Name    string   `json:"name"`
	Count   int      `json:"count"`
	Samples []string `json:"samples,omitempty"`
}

type SubsystemSummary struct {
	Name        string   `json:"name"`
	ModuleCount int      `json:"module_count"`
	SampleFiles []string `json:"sample_files,omitempty"`
}

type Summary struct {
	Root          string             `json:"root"`
	CommandsCount int                `json:"commands_count"`
	ToolsCount    int                `json:"tools_count"`
	CommandGroups int                `json:"command_groups"`
	ToolGroups    int                `json:"tool_groups"`
	Subsystems    []SubsystemSummary `json:"subsystems,omitempty"`
	CommandFamily []FamilySummary    `json:"command_families,omitempty"`
	ToolFamily    []FamilySummary    `json:"tool_families,omitempty"`
}

func DiscoverRoot(start string) (string, bool) {
	if envRoot := strings.TrimSpace(os.Getenv(EnvRoot)); envRoot != "" {
		if root, ok := normalizeBridgeRoot(envRoot); ok {
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
		for _, candidate := range []string{dir, filepath.Join(dir, "claw-code-main")} {
			if root, ok := normalizeBridgeRoot(candidate); ok {
				return root, true
			}
		}
	}

	return "", false
}

func LoadAuto(start string) (*Summary, error) {
	root, ok := DiscoverRoot(start)
	if !ok {
		return nil, ErrNotFound
	}
	return Load(root)
}

func Load(root string) (*Summary, error) {
	normalizedRoot, ok := normalizeBridgeRoot(root)
	if !ok {
		return nil, ErrNotFound
	}

	commands, err := loadSnapshot(filepath.Join(normalizedRoot, "src", "reference_data", "commands_snapshot.json"))
	if err != nil {
		return nil, err
	}
	tools, err := loadSnapshot(filepath.Join(normalizedRoot, "src", "reference_data", "tools_snapshot.json"))
	if err != nil {
		return nil, err
	}
	subsystems, err := loadSubsystems(filepath.Join(normalizedRoot, "src", "reference_data", "subsystems"))
	if err != nil {
		return nil, err
	}

	commandFamily := summarizeFamilies(commands, "commands")
	toolFamily := summarizeFamilies(tools, "tools")

	return &Summary{
		Root:          normalizedRoot,
		CommandsCount: len(commands),
		ToolsCount:    len(tools),
		CommandGroups: len(commandFamily),
		ToolGroups:    len(toolFamily),
		Subsystems:    subsystems,
		CommandFamily: commandFamily,
		ToolFamily:    toolFamily,
	}, nil
}

func RenderJSON(summary *Summary, section string, family string, limit int) (string, error) {
	payload, err := Lookup(summary, section, family, limit)
	if err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func Lookup(summary *Summary, section string, family string, limit int) (map[string]any, error) {
	if summary == nil {
		return nil, fmt.Errorf("bridge summary is required")
	}
	limit = normalizeLimit(limit)
	section = strings.ToLower(strings.TrimSpace(section))
	switch section {
	case "", "summary":
		return map[string]any{
			"root":              summary.Root,
			"commands_count":    summary.CommandsCount,
			"tools_count":       summary.ToolsCount,
			"command_groups":    summary.CommandGroups,
			"tool_groups":       summary.ToolGroups,
			"top_commands":      limitFamilies(summary.CommandFamily, limit),
			"top_tools":         limitFamilies(summary.ToolFamily, limit),
			"top_subsystems":    limitSubsystems(summary.Subsystems, limit),
			"orchestration_tip": "Use this surface as a local orchestration reference: inspect, plan, execute, verify, review, and adapt until the requested outcome is real.",
		}, nil
	case "commands":
		if family != "" {
			item, ok := findFamily(summary.CommandFamily, family)
			if !ok {
				return nil, fmt.Errorf("command family not found: %s", family)
			}
			return map[string]any{"root": summary.Root, "section": "commands", "family": item}, nil
		}
		return map[string]any{
			"root":          summary.Root,
			"section":       "commands",
			"count":         summary.CommandsCount,
			"family_count":  summary.CommandGroups,
			"top_families":  limitFamilies(summary.CommandFamily, limit),
			"suggested_use": "Map user requests into command domains such as agents, tasks, skills, review, plan, plugin, or MCP-style orchestration when that helps you reason about the next step.",
		}, nil
	case "tools":
		if family != "" {
			item, ok := findFamily(summary.ToolFamily, family)
			if !ok {
				return nil, fmt.Errorf("tool family not found: %s", family)
			}
			return map[string]any{"root": summary.Root, "section": "tools", "family": item}, nil
		}
		return map[string]any{
			"root":          summary.Root,
			"section":       "tools",
			"count":         summary.ToolsCount,
			"family_count":  summary.ToolGroups,
			"top_families":  limitFamilies(summary.ToolFamily, limit),
			"suggested_use": "Prefer higher-level tool families first, then fall back to lower-level execution only when needed.",
		}, nil
	case "subsystems":
		if family != "" {
			item, ok := findSubsystem(summary.Subsystems, family)
			if !ok {
				return nil, fmt.Errorf("subsystem not found: %s", family)
			}
			return map[string]any{"root": summary.Root, "section": "subsystems", "subsystem": item}, nil
		}
		return map[string]any{
			"root":       summary.Root,
			"section":    "subsystems",
			"count":      len(summary.Subsystems),
			"subsystems": limitSubsystems(summary.Subsystems, limit),
			"focus_hint": "Subsystems show where the archived harness concentrated its runtime, transport, command, and assistant concerns.",
		}, nil
	default:
		return nil, fmt.Errorf("unsupported section: %s", section)
	}
}

func HumanSummary(summary *Summary) string {
	if summary == nil {
		return "claw-code-main bridge unavailable"
	}

	lines := []string{
		fmt.Sprintf("claw-code-main root: %s", summary.Root),
		fmt.Sprintf("commands: %d snapshot entries across %d groups", summary.CommandsCount, summary.CommandGroups),
		fmt.Sprintf("tools: %d snapshot entries across %d groups", summary.ToolsCount, summary.ToolGroups),
		fmt.Sprintf("subsystems: %d mirrored areas", len(summary.Subsystems)),
	}

	if top := joinFamilyNames(summary.CommandFamily, 6); top != "" {
		lines = append(lines, "top command groups: "+top)
	}
	if top := joinFamilyNames(summary.ToolFamily, 6); top != "" {
		lines = append(lines, "top tool groups: "+top)
	}
	if top := joinSubsystemNames(summary.Subsystems, 5); top != "" {
		lines = append(lines, "top subsystems: "+top)
	}

	lines = append(lines, "integration mode: AnyClaw can use this surface as an execution and orchestration reference.")
	return strings.Join(lines, "\n")
}

func normalizeBridgeRoot(candidate string) (string, bool) {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return "", false
	}
	absCandidate, err := filepath.Abs(candidate)
	if err != nil {
		return "", false
	}
	if hasBridgeFiles(absCandidate) {
		return absCandidate, true
	}
	return "", false
}

func hasBridgeFiles(root string) bool {
	required := []string{
		filepath.Join(root, "src", "reference_data", "commands_snapshot.json"),
		filepath.Join(root, "src", "reference_data", "tools_snapshot.json"),
		filepath.Join(root, "src", "reference_data", "subsystems"),
	}
	for _, item := range required {
		if _, err := os.Stat(item); err != nil {
			return false
		}
	}
	return true
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

func loadSnapshot(path string) ([]snapshotEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var items []snapshotEntry
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func loadSubsystems(dir string) ([]SubsystemSummary, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	items := make([]SubsystemSummary, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		var record subsystemRecord
		if err := json.Unmarshal(data, &record); err != nil {
			return nil, err
		}
		name := firstNonEmpty(record.ArchiveName, record.PackageName, strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())))
		items = append(items, SubsystemSummary{
			Name:        name,
			ModuleCount: record.ModuleCount,
			SampleFiles: limitStrings(record.SampleFiles, 4),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].ModuleCount == items[j].ModuleCount {
			return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
		}
		return items[i].ModuleCount > items[j].ModuleCount
	})
	return items, nil
}

func summarizeFamilies(items []snapshotEntry, prefix string) []FamilySummary {
	type accumulator struct {
		count   int
		samples []string
		seen    map[string]struct{}
	}

	families := map[string]*accumulator{}
	for _, item := range items {
		name := familyName(item.SourceHint, prefix, item.Name)
		if name == "" {
			name = "misc"
		}
		acc, ok := families[name]
		if !ok {
			acc = &accumulator{seen: map[string]struct{}{}}
			families[name] = acc
		}
		acc.count++
		sample := strings.TrimSpace(item.Name)
		if sample == "" {
			sample = name
		}
		if _, exists := acc.seen[sample]; !exists {
			acc.seen[sample] = struct{}{}
			acc.samples = append(acc.samples, sample)
		}
	}

	summaries := make([]FamilySummary, 0, len(families))
	for name, acc := range families {
		summaries = append(summaries, FamilySummary{
			Name:    name,
			Count:   acc.count,
			Samples: limitStrings(acc.samples, 5),
		})
	}
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].Count == summaries[j].Count {
			return strings.ToLower(summaries[i].Name) < strings.ToLower(summaries[j].Name)
		}
		return summaries[i].Count > summaries[j].Count
	})
	return summaries
}

func familyName(sourceHint string, prefix string, fallback string) string {
	sourceHint = filepath.ToSlash(strings.TrimSpace(sourceHint))
	marker := prefix + "/"
	if strings.HasPrefix(sourceHint, marker) {
		rest := strings.TrimPrefix(sourceHint, marker)
		if idx := strings.Index(rest, "/"); idx >= 0 {
			return rest[:idx]
		}
		base := strings.TrimSuffix(rest, filepath.Ext(rest))
		if strings.TrimSpace(base) != "" {
			return base
		}
	}
	return strings.TrimSpace(fallback)
}

func limitFamilies(items []FamilySummary, limit int) []FamilySummary {
	if limit <= 0 || limit >= len(items) {
		return append([]FamilySummary(nil), items...)
	}
	return append([]FamilySummary(nil), items[:limit]...)
}

func limitSubsystems(items []SubsystemSummary, limit int) []SubsystemSummary {
	if limit <= 0 || limit >= len(items) {
		return append([]SubsystemSummary(nil), items...)
	}
	return append([]SubsystemSummary(nil), items[:limit]...)
}

func findFamily(items []FamilySummary, name string) (FamilySummary, bool) {
	for _, item := range items {
		if strings.EqualFold(item.Name, strings.TrimSpace(name)) {
			return item, true
		}
	}
	return FamilySummary{}, false
}

func findSubsystem(items []SubsystemSummary, name string) (SubsystemSummary, bool) {
	for _, item := range items {
		if strings.EqualFold(item.Name, strings.TrimSpace(name)) {
			return item, true
		}
	}
	return SubsystemSummary{}, false
}

func joinFamilyNames(items []FamilySummary, limit int) string {
	names := make([]string, 0, limit)
	for _, item := range limitFamilies(items, limit) {
		names = append(names, fmt.Sprintf("%s (%d)", item.Name, item.Count))
	}
	return strings.Join(names, ", ")
}

func joinSubsystemNames(items []SubsystemSummary, limit int) string {
	names := make([]string, 0, limit)
	for _, item := range limitSubsystems(items, limit) {
		names = append(names, fmt.Sprintf("%s (%d)", item.Name, item.ModuleCount))
	}
	return strings.Join(names, ", ")
}

func normalizeLimit(limit int) int {
	switch {
	case limit <= 0:
		return 6
	case limit > 25:
		return 25
	default:
		return limit
	}
}

func limitStrings(items []string, limit int) []string {
	if limit <= 0 || limit >= len(items) {
		return append([]string(nil), items...)
	}
	return append([]string(nil), items[:limit]...)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
