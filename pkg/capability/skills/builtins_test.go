package skills

import (
	"os"
	"path/filepath"
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
