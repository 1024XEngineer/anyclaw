package clihub

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAutoDiscoversSiblingCatalog(t *testing.T) {
	workspace := t.TempDir()
	root := filepath.Join(workspace, "CLI-Anything-0.2.0")
	if err := writeCatalogFixture(root); err != nil {
		t.Fatalf("writeCatalogFixture: %v", err)
	}
	start := filepath.Join(workspace, "anyclaw", "workflows")
	if err := os.MkdirAll(start, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	cat, err := LoadAuto(start)
	if err != nil {
		t.Fatalf("LoadAuto: %v", err)
	}
	if cat.Root != root {
		t.Fatalf("expected root %q, got %q", root, cat.Root)
	}
	if len(cat.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(cat.Entries))
	}
}

func TestSearchAndFindIncludeSourceAndSkillPaths(t *testing.T) {
	root := filepath.Join(t.TempDir(), "CLI-Anything-0.2.0")
	if err := writeCatalogFixture(root); err != nil {
		t.Fatalf("writeCatalogFixture: %v", err)
	}
	cat, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	results := Search(cat, "video", "", false, 10)
	if len(results) != 1 || results[0].Name != "shotcut" {
		t.Fatalf("unexpected search results: %#v", results)
	}
	if results[0].SourcePath == "" || results[0].SkillPath == "" || results[0].DevModule == "" {
		t.Fatalf("expected local source metadata, got %#v", results[0])
	}
	if !results[0].Runnable || StatusLabel(results[0]) != "source" {
		t.Fatalf("expected shotcut to be runnable from source, got %#v", results[0])
	}

	entry, ok := Find(cat, "shotcut")
	if !ok || entry.Name != "shotcut" {
		t.Fatalf("expected to find shotcut, got %#v ok=%v", entry, ok)
	}
}

func TestSummaryForCountsCategories(t *testing.T) {
	root := filepath.Join(t.TempDir(), "CLI-Anything-0.2.0")
	if err := writeCatalogFixture(root); err != nil {
		t.Fatalf("writeCatalogFixture: %v", err)
	}
	cat, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	summary := SummaryFor(cat, 5)
	if summary.EntriesCount != 3 {
		t.Fatalf("unexpected entry count: %#v", summary)
	}
	if len(summary.Categories) == 0 || summary.Categories[0].Name != "office" {
		t.Fatalf("unexpected categories: %#v", summary.Categories)
	}
	if summary.RunnableCount != 1 || len(summary.Runnable) != 1 || summary.Runnable[0].Name != "shotcut" {
		t.Fatalf("unexpected runnable summary: %#v", summary)
	}
	if summary.InstalledCount != 0 {
		t.Fatalf("expected no installed entries in fixture, got %#v", summary)
	}
}

func TestDevModuleFallsBackToCLIFile(t *testing.T) {
	source := filepath.Join(t.TempDir(), "shotcut", "agent-harness")
	moduleDir := filepath.Join(source, "cli_anything", "shotcut")
	if err := os.MkdirAll(moduleDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(moduleDir, "shotcut_cli.py"), []byte("print('ok')"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got := devModule(source)
	if got != "cli_anything.shotcut.shotcut_cli" {
		t.Fatalf("devModule = %q, want %q", got, "cli_anything.shotcut.shotcut_cli")
	}
}

func writeCatalogFixture(root string) error {
	if err := os.MkdirAll(filepath.Join(root, "shotcut", "agent-harness", "cli_anything", "shotcut", "skills"), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, "shotcut", "agent-harness", "cli_anything", "shotcut"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "shotcut", "agent-harness", "cli_anything", "shotcut", "__main__.py"), []byte("print('ok')"), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "shotcut", "agent-harness", "cli_anything", "shotcut", "skills", "SKILL.md"), []byte("# skill"), 0o644); err != nil {
		return err
	}

	file := catalogFile{}
	file.Meta.Repo = "https://example.com/CLI-Anything"
	file.Meta.Description = "CLI-Hub"
	file.Meta.Updated = "2026-03-29"
	file.CLIs = []Entry{
		{Name: "libreoffice", DisplayName: "LibreOffice", Description: "Office suite", Category: "office", EntryPoint: "cli-anything-libreoffice"},
		{Name: "zotero", DisplayName: "Zotero", Description: "Reference manager", Category: "office", EntryPoint: "cli-anything-zotero"},
		{Name: "shotcut", DisplayName: "Shotcut", Description: "Video editing and rendering", Category: "video", EntryPoint: "cli-anything-shotcut", SkillMD: "shotcut/agent-harness/cli_anything/shotcut/skills/SKILL.md"},
	}
	data, err := json.Marshal(file)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, "registry.json"), data, 0o644)
}
