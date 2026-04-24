package skills

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuiltinSkillCatalogHasRecommendedCount(t *testing.T) {
	if got := len(BuiltinSkills); got != 45 {
		t.Fatalf("expected 45 builtin skills, got %d", got)
	}
}

func TestSkillsManagerLoadsBuiltinsWithoutDirectory(t *testing.T) {
	manager := NewSkillsManager(filepath.Join(t.TempDir(), "missing-skills"))
	if err := manager.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := len(manager.List()); got != 45 {
		t.Fatalf("expected 45 loaded builtins, got %d", got)
	}
	if _, ok := manager.Get("voice-designer"); !ok {
		t.Fatal("expected builtin voice-designer skill to be loaded")
	}
}

func TestSkillsManagerAllowsLocalOverrideOnBuiltin(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "coder")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "skill.json"), []byte(`{
  "name": "coder",
  "description": "Local override",
  "version": "9.9.9",
  "entrypoint": "builtin://coder"
}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	manager := NewSkillsManager(dir)
	if err := manager.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := len(manager.List()); got != 45 {
		t.Fatalf("expected builtin count with override to remain 45, got %d", got)
	}
	skill, ok := manager.Get("coder")
	if !ok {
		t.Fatal("expected coder skill")
	}
	if skill.Description != "Local override" {
		t.Fatalf("expected local override description, got %q", skill.Description)
	}
}

func TestBuiltinSkillCatalogPreservesCategoryAndInstallHint(t *testing.T) {
	manager := NewSkillsManager(filepath.Join(t.TempDir(), "missing-skills"))
	if err := manager.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	entries := manager.Catalog()
	for _, entry := range entries {
		if entry.Name != "coder" {
			continue
		}
		if entry.Category != "engineering" {
			t.Fatalf("expected coder category engineering, got %q", entry.Category)
		}
		if entry.InstallHint != "anyclaw skill install coder" {
			t.Fatalf("unexpected coder install hint: %q", entry.InstallHint)
		}
		return
	}

	t.Fatal("expected coder catalog entry")
}

func TestBuiltinSkillAccessors(t *testing.T) {
	names := ListBuiltinSkillNames()
	if len(names) != len(BuiltinSkills) {
		t.Fatalf("expected %d builtin names, got %d", len(BuiltinSkills), len(names))
	}
	if names[0] == "" {
		t.Fatal("expected first builtin name to be non-empty")
	}

	content, ok := GetBuiltinSkill("coder")
	if !ok || !strings.Contains(content, `"name": "coder"`) {
		t.Fatalf("unexpected builtin skill content: %q %v", content, ok)
	}
	if _, ok := GetBuiltinSkill("missing-skill"); ok {
		t.Fatal("expected missing builtin skill lookup to fail")
	}

	definition, ok := GetBuiltinSkillDefinition("coder")
	if !ok || definition.Name != "coder" || definition.Category != "engineering" {
		t.Fatalf("unexpected builtin skill definition: %#v %v", definition, ok)
	}
	if _, ok := GetBuiltinSkillDefinition("missing-skill"); ok {
		t.Fatal("expected missing builtin skill definition lookup to fail")
	}
}

func TestExecuteSkillEntrypointRejectsPathsOutsideSkillDir(t *testing.T) {
	skillDir := filepath.Join(t.TempDir(), "demo")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	skill := &Skill{
		Name:       "demo",
		Entrypoint: filepath.Join("..", "escape.sh"),
		Metadata: map[string]string{
			"path": skillDir,
		},
	}

	_, err := executeSkillEntrypoint(context.Background(), skill, map[string]any{"action": "run"}, ExecutionOptions{AllowExec: true})
	if err == nil {
		t.Fatal("expected entrypoint outside skill dir to be rejected")
	}
	if !strings.Contains(err.Error(), "must stay within skill directory") {
		t.Fatalf("unexpected error: %v", err)
	}
}
