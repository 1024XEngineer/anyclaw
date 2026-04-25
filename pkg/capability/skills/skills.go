package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/capability/tools"
)

type Skill struct {
	Name           string            `json:"name"`
	Description    string            `json:"description"`
	Version        string            `json:"version"`
	Category       string            `json:"category,omitempty"`
	Commands       []Command         `json:"commands"`
	Prompts        map[string]string `json:"prompts"`
	Tools          []Tool            `json:"tools"`
	Metadata       map[string]string `json:"metadata"`
	Permissions    []string          `json:"permissions,omitempty"`
	Entrypoint     string            `json:"entrypoint,omitempty"`
	Source         string            `json:"source,omitempty"`
	Registry       string            `json:"registry,omitempty"`
	Homepage       string            `json:"homepage,omitempty"`
	InstallCommand string            `json:"install_command,omitempty"`
}

type Command struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Pattern     string `json:"pattern"`
	Action      string `json:"action"`
}

type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
	Handler     string `json:"handler"`
}

type SkillsManager struct {
	dir    string
	skills map[string]*Skill
}

type ExecutionOptions struct {
	AllowExec          bool
	ExecTimeoutSeconds int
}

type SkillCatalogEntry struct {
	Name         string   `json:"name"`
	FullName     string   `json:"full_name,omitempty"`
	Description  string   `json:"description"`
	Version      string   `json:"version,omitempty"`
	Category     string   `json:"category,omitempty"`
	Registry     string   `json:"registry,omitempty"`
	Homepage     string   `json:"homepage,omitempty"`
	Source       string   `json:"source,omitempty"`
	Permissions  []string `json:"permissions,omitempty"`
	Entrypoint   string   `json:"entrypoint,omitempty"`
	InstallHint  string   `json:"install_hint,omitempty"`
	Installed    bool     `json:"installed,omitempty"`
	InstalledDir string   `json:"installed_dir,omitempty"`
	Builtin      bool     `json:"builtin,omitempty"`
}

func NewSkillsManager(dir string) *SkillsManager {
	return &SkillsManager{
		dir:    dir,
		skills: make(map[string]*Skill),
	}
}

func (s *SkillsManager) Load() error {
	if s.dir == "" {
		s.dir = "skills"
	}

	for name, definition := range BuiltinSkillDefinitions() {
		s.skills[name] = skillFromDefinition(definition, "builtin://"+name)
	}

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			if len(s.skills) == 0 {
				fmt.Printf("Warning: skills directory not found: %s (skipping)\n", s.dir)
			}
			return nil
		}
		return fmt.Errorf("skills directory not found: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillPath := filepath.Join(s.dir, entry.Name())
		skill, err := s.loadSkill(skillPath)
		if err != nil {
			fmt.Printf("Warning: failed to load skill %s: %v\n", entry.Name(), err)
			continue
		}

		s.skills[skill.Name] = skill
	}

	return nil
}

func skillFromDefinition(definition skillFileDefinition, virtualPath string) *Skill {
	skill := &Skill{
		Name:           definition.Name,
		Description:    definition.Description,
		Version:        definition.Version,
		Category:       definition.Category,
		Prompts:        map[string]string{},
		Metadata:       map[string]string{},
		Permissions:    append([]string(nil), definition.Permissions...),
		Entrypoint:     definition.Entrypoint,
		Source:         definition.Source,
		Registry:       definition.Registry,
		Homepage:       definition.Homepage,
		InstallCommand: definition.InstallCommand,
	}
	for key, value := range definition.Prompts {
		skill.Prompts[key] = value
	}
	if skill.Name == "" {
		skill.Name = strings.TrimPrefix(strings.TrimSpace(skill.Entrypoint), "builtin://")
	}
	if skill.Entrypoint == "" {
		skill.Entrypoint = "builtin://" + skill.Name
	}
	if skill.Source == "" {
		skill.Source = "builtin"
	}
	if skill.Registry == "" {
		skill.Registry = "builtin"
	}
	if skill.InstallCommand == "" && skill.Name != "" {
		skill.InstallCommand = "anyclaw skill install " + skill.Name
	}
	if virtualPath == "" {
		virtualPath = "builtin://" + skill.Name
	}
	skill.Metadata["path"] = virtualPath
	if strings.TrimSpace(skill.Category) != "" {
		skill.Metadata["category"] = skill.Category
	}
	skill.Metadata["source"] = skill.Source
	skill.Metadata["registry"] = skill.Registry
	return skill
}

func (s *SkillsManager) loadSkill(path string) (*Skill, error) {
	skillFile := filepath.Join(path, "skill.json")
	skillMdFile := filepath.Join(path, "SKILL.md")

	// Try to load skill.json first
	data, err := os.ReadFile(skillFile)
	if err != nil {
		// If skill.json doesn't exist, try to convert SKILL.md
		if _, mdErr := os.Stat(skillMdFile); mdErr == nil {
			if convertErr := ConvertSkillhubToSkillJSON(path); convertErr != nil {
				return nil, fmt.Errorf("failed to convert SKILL.md: %w", convertErr)
			}
			// Try to read the converted skill.json
			data, err = os.ReadFile(skillFile)
			if err != nil {
				return nil, fmt.Errorf("failed to read converted skill.json: %w", err)
			}
		} else {
			return nil, fmt.Errorf("skill.json not found: %w", err)
		}
	}

	var skill Skill
	if err := json.Unmarshal(data, &skill); err != nil {
		return nil, fmt.Errorf("invalid skill.json: %w", err)
	}

	if skill.Name == "" {
		skill.Name = filepath.Base(path)
	}
	if skill.Metadata == nil {
		skill.Metadata = map[string]string{}
	}
	if strings.TrimSpace(skill.Category) == "" {
		skill.Category = skill.Metadata["category"]
	}
	if strings.TrimSpace(skill.Metadata["category"]) == "" && strings.TrimSpace(skill.Category) != "" {
		skill.Metadata["category"] = skill.Category
	}
	if skill.Source == "" {
		skill.Source = skill.Metadata["source"]
	}
	if skill.Registry == "" {
		skill.Registry = skill.Metadata["registry"]
	}
	if skill.Homepage == "" {
		skill.Homepage = skill.Metadata["homepage"]
	}
	if skill.Entrypoint == "" {
		skill.Entrypoint = skill.Metadata["entrypoint"]
	}
	if skill.InstallCommand == "" {
		skill.InstallCommand = skill.Metadata["install_command"]
	}
	skill.Metadata["path"] = path

	return &skill, nil
}

func (s *SkillsManager) Get(name string) (*Skill, bool) {
	skill, ok := s.skills[name]
	return skill, ok
}

func (s *SkillsManager) List() []*Skill {
	var list []*Skill
	for _, skill := range s.skills {
		list = append(list, skill)
	}
	return list
}

func (s *SkillsManager) ListByCategory() map[string][]*Skill {
	categories := make(map[string][]*Skill)

	for _, skill := range s.skills {
		cat := skill.Metadata["category"]
		if cat == "" {
			cat = "general"
		}
		categories[cat] = append(categories[cat], skill)
	}

	return categories
}

func (s *SkillsManager) FindByCommand(input string) []*Skill {
	var matched []*Skill

	for _, skill := range s.skills {
		for _, cmd := range skill.Commands {
			if cmd.Pattern != "" && strings.Contains(input, cmd.Pattern) {
				matched = append(matched, skill)
				break
			}
		}
	}

	return matched
}

type SkillTool struct {
	Name        string
	Description string
	InputSchema map[string]any
	SkillName   string
}

func (s *Skill) ToTool() *SkillTool {
	return &SkillTool{
		Name:        "skill_" + s.Name,
		Description: s.Description,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]string{"type": "string"},
				"params": map[string]any{"type": "object"},
			},
			"required": []string{"action"},
		},
		SkillName: s.Name,
	}
}

func (s *SkillsManager) RegisterTools(registry *tools.Registry, opts ExecutionOptions) {
	for _, skill := range s.skills {
		skill := skill
		toolDef := skill.ToTool()
		registry.RegisterTool(toolDef.Name, toolDef.Description, toolDef.InputSchema, func(ctx context.Context, input map[string]any) (string, error) {
			return s.Execute(ctx, skill.Name, input, opts)
		})
	}
}

func (s *SkillsManager) FilterEnabled(names []string) *SkillsManager {
	if len(names) == 0 {
		return s
	}
	allowed := make(map[string]struct{}, len(names))
	for _, name := range names {
		name = strings.TrimSpace(strings.ToLower(name))
		if name != "" {
			allowed[name] = struct{}{}
		}
	}
	filtered := NewSkillsManager(s.dir)
	for name, skill := range s.skills {
		if _, ok := allowed[strings.TrimSpace(strings.ToLower(name))]; !ok {
			continue
		}
		filtered.skills[name] = skill
	}
	return filtered
}

func (s *SkillsManager) Execute(ctx context.Context, name string, input map[string]any, opts ExecutionOptions) (string, error) {
	skill, ok := s.skills[name]
	if !ok {
		return "", fmt.Errorf("skill not found: %s", name)
	}
	if !skill.IsExecutable() {
		prompt, _ := skill.Prompts["system"]
		if strings.TrimSpace(prompt) == "" {
			return fmt.Sprintf("skill %s is declarative only; no executable entrypoint configured", name), nil
		}
		return fmt.Sprintf("skill %s has no executable entrypoint; system prompt:\n%s", name, prompt), nil
	}
	if !opts.AllowExec {
		return "", fmt.Errorf("skill execution disabled for entrypoint-backed skills: %s", name)
	}
	return executeSkillEntrypoint(ctx, skill, input, opts)
}

func (s *Skill) IsExecutable() bool {
	entry := strings.TrimSpace(s.Entrypoint)
	if entry == "" {
		return false
	}
	return !strings.HasPrefix(entry, "builtin://")
}

func executeSkillEntrypoint(ctx context.Context, skill *Skill, input map[string]any, opts ExecutionOptions) (string, error) {
	timeout := 10 * time.Second
	if opts.ExecTimeoutSeconds > 0 {
		timeout = time.Duration(opts.ExecTimeoutSeconds) * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	payload, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	skillPath := strings.TrimSpace(skill.Metadata["path"])
	if skillPath == "" {
		return "", fmt.Errorf("skill %s has no execution directory", skill.Name)
	}
	skillPath, err = filepath.Abs(skillPath)
	if err != nil {
		return "", fmt.Errorf("resolve skill directory: %w", err)
	}
	entrypoint := strings.TrimSpace(skill.Entrypoint)
	if !filepath.IsAbs(entrypoint) {
		entrypoint = filepath.Join(skillPath, entrypoint)
	}
	entrypoint = filepath.Clean(entrypoint)
	if !pathWithinBase(skillPath, entrypoint) {
		return "", fmt.Errorf("skill %s entrypoint must stay within skill directory", skill.Name)
	}
	launcher, args, err := resolveSkillLauncher(entrypoint)
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, launcher, args...)
	cmd.Dir = skillPath
	cmd.Env = append(os.Environ(),
		"ANYCLAW_SKILL_INPUT="+string(payload),
		"ANYCLAW_SKILL_NAME="+skill.Name,
		"ANYCLAW_SKILL_VERSION="+skill.Version,
		"ANYCLAW_SKILL_DIR="+skillPath,
		"ANYCLAW_SKILL_TIMEOUT_SECONDS="+fmt.Sprintf("%d", int(timeout/time.Second)),
		"ANYCLAW_SKILL_PERMISSIONS="+strings.Join(skill.Permissions, ","),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("skill %s timed out after %s", skill.Name, timeout)
		}
		return "", fmt.Errorf("skill %s failed: %w: %s", skill.Name, err, string(out))
	}
	return string(out), nil
}

func resolveSkillLauncher(entrypoint string) (string, []string, error) {
	ext := strings.ToLower(filepath.Ext(entrypoint))
	switch ext {
	case ".py":
		python, err := findFirstExecutable("python3", "python")
		if err != nil {
			return "", nil, fmt.Errorf("python launcher not found for skill entrypoint %s", entrypoint)
		}
		return python, []string{entrypoint}, nil
	case ".js", ".mjs", ".cjs":
		node, err := findFirstExecutable("node")
		if err != nil {
			return "", nil, fmt.Errorf("node launcher not found for skill entrypoint %s", entrypoint)
		}
		return node, []string{entrypoint}, nil
	case ".sh":
		if runtime.GOOS == "windows" {
			bash, err := findFirstExecutable("bash")
			if err != nil {
				return "", nil, fmt.Errorf("bash launcher not found for skill entrypoint %s", entrypoint)
			}
			return bash, []string{entrypoint}, nil
		}
		return "sh", []string{entrypoint}, nil
	case ".ps1":
		ps, err := findFirstExecutable("pwsh", "powershell")
		if err != nil {
			return "", nil, fmt.Errorf("powershell launcher not found for skill entrypoint %s", entrypoint)
		}
		if runtime.GOOS == "windows" {
			return ps, []string{"-ExecutionPolicy", "Bypass", "-File", entrypoint}, nil
		}
		return ps, []string{"-File", entrypoint}, nil
	default:
		return entrypoint, nil, nil
	}
}

func findFirstExecutable(names ...string) (string, error) {
	for _, name := range names {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("executable not found")
}

func (s *SkillsManager) GetPrompt(name string, promptType string) (string, error) {
	skill, ok := s.skills[name]
	if !ok {
		return "", fmt.Errorf("skill not found: %s", name)
	}

	prompt, ok := skill.Prompts[promptType]
	if !ok {
		return "", fmt.Errorf("prompt not found: %s/%s", name, promptType)
	}

	return prompt, nil
}

func (s *SkillsManager) GetSystemPrompts() []string {
	var prompts []string

	for _, skill := range s.skills {
		if prompt, ok := skill.Prompts["system"]; ok {
			prompts = append(prompts, prompt)
		}
	}

	return prompts
}

type SkillInfo struct {
	Name        string
	Description string
	Version     string
	Permissions []string
	Entrypoint  string
	Registry    string
	Source      string
	InstallHint string
}

func (s *SkillsManager) Catalog() []SkillCatalogEntry {
	entries := make([]SkillCatalogEntry, 0, len(s.skills))
	for _, skill := range s.skills {
		entries = append(entries, SkillCatalogEntry{
			Name:         skill.Name,
			Description:  skill.Description,
			Version:      skill.Version,
			Category:     firstNonEmpty(skill.Category, skill.Metadata["category"]),
			Registry:     skill.Registry,
			Homepage:     skill.Homepage,
			Source:       skill.Source,
			Permissions:  append([]string(nil), skill.Permissions...),
			Entrypoint:   skill.Entrypoint,
			Installed:    true,
			InstalledDir: filepath.Join(s.dir, skill.Name),
			InstallHint:  skill.InstallCommand,
			Builtin:      strings.HasPrefix(strings.TrimSpace(skill.Entrypoint), "builtin://"),
		})
	}
	return entries
}
