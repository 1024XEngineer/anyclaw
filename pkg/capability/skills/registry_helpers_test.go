package skills

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestConvertMarkdownToSkillJSONUsesDetailFallbacks(t *testing.T) {
	t.Parallel()

	detail := &SkillDetail{
		Description: "Remote skill description",
		Version:     "2.3.4",
		Permissions: []string{"network", "filesystem"},
		Entrypoint:  "run.py",
		Homepage:    "https://example.com/weather",
		Registry:    "partner-registry",
	}

	skillJSON, err := ConvertMarkdownToSkillJSON("# Weather\n- fetch forecast\nAlways answer clearly.", "weather", detail)
	if err != nil {
		t.Fatalf("ConvertMarkdownToSkillJSON returned error: %v", err)
	}

	var got skillFileDefinition
	if err := json.Unmarshal([]byte(skillJSON), &got); err != nil {
		t.Fatalf("unmarshal skill JSON: %v", err)
	}

	if got.Name != "weather" {
		t.Fatalf("expected name weather, got %q", got.Name)
	}
	if got.Description != detail.Description {
		t.Fatalf("expected description %q, got %q", detail.Description, got.Description)
	}
	if got.Version != detail.Version {
		t.Fatalf("expected version %q, got %q", detail.Version, got.Version)
	}
	if got.Registry != detail.Registry {
		t.Fatalf("expected registry %q, got %q", detail.Registry, got.Registry)
	}
	if got.Entrypoint != detail.Entrypoint {
		t.Fatalf("expected entrypoint %q, got %q", detail.Entrypoint, got.Entrypoint)
	}
	if got.Prompts["system"] != "fetch forecast Always answer clearly." {
		t.Fatalf("unexpected system prompt: %q", got.Prompts["system"])
	}
}

func TestConvertSkillhubToSkillJSONWritesExpectedFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	content := `---
name: travel_helper
description: Plan lighter itineraries
---
Suggest efficient routes.
Highlight trade-offs.
`
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}

	if err := ConvertSkillhubToSkillJSON(dir); err != nil {
		t.Fatalf("ConvertSkillhubToSkillJSON returned error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "skill.json"))
	if err != nil {
		t.Fatalf("read skill.json: %v", err)
	}

	var got skillFileDefinition
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal skill.json: %v", err)
	}

	if got.Name != "travel_helper" {
		t.Fatalf("expected name travel_helper, got %q", got.Name)
	}
	if got.Description != "Plan lighter itineraries" {
		t.Fatalf("unexpected description: %q", got.Description)
	}
	if got.Source != "skillhub" {
		t.Fatalf("expected source skillhub, got %q", got.Source)
	}
	if got.Prompts["system"] != "Suggest efficient routes.\nHighlight trade-offs." {
		t.Fatalf("unexpected system prompt: %q", got.Prompts["system"])
	}
}

func TestPathWithinBaseRejectsPrefixLookalikes(t *testing.T) {
	t.Parallel()

	baseDir := filepath.Join(t.TempDir(), "skills")
	inside := filepath.Join(baseDir, "weather", "skill.json")
	outside := filepath.Join(baseDir+"-backup", "weather", "skill.json")

	if !pathWithinBase(baseDir, inside) {
		t.Fatalf("expected %q to be inside %q", inside, baseDir)
	}
	if pathWithinBase(baseDir, outside) {
		t.Fatalf("expected %q to be outside %q", outside, baseDir)
	}
}
