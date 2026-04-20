package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAddWritesDailyMarkdownWhenDailyDirConfigured(t *testing.T) {
	workDir := t.TempDir()
	workspaceDir := t.TempDir()

	mem := NewFileMemory(workDir)
	mem.SetDailyDir(filepath.Join(workspaceDir, "memory"))
	if err := mem.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	entryTime := time.Date(2026, 3, 29, 14, 5, 0, 0, time.UTC)
	if err := mem.Add(MemoryEntry{
		ID:        "fact-1",
		Timestamp: entryTime,
		Type:      TypeFact,
		Content:   "Remember the release branch is frozen.",
		Metadata:  map[string]string{"source": "ops"},
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(workspaceDir, "memory", "2026-03-29.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "# Daily Memory 2026-03-29") {
		t.Fatalf("expected daily header, got %q", text)
	}
	if !strings.Contains(text, "Remember the release branch is frozen.") {
		t.Fatalf("expected entry content, got %q", text)
	}
	if !strings.Contains(text, "- source: ops") {
		t.Fatalf("expected metadata, got %q", text)
	}
}

func TestSearchDailyMarkdownFindsDateAndSnippet(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "2026-03-29.md"), []byte("# Daily Memory 2026-03-29\n\nWe shipped the alpha build today."), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "2026-03-28.md"), []byte("# Daily Memory 2026-03-28\n\nNothing important."), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	results, err := SearchDailyMarkdown(dir, "alpha", 5, "")
	if err != nil {
		t.Fatalf("SearchDailyMarkdown: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 match, got %d", len(results))
	}
	if results[0].Date != "2026-03-29" {
		t.Fatalf("expected date 2026-03-29, got %q", results[0].Date)
	}
	if !strings.Contains(results[0].Snippet, "alpha build") {
		t.Fatalf("expected snippet to mention alpha build, got %q", results[0].Snippet)
	}
}

func TestGetDailyMarkdownSupportsRelativeReferences(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC()
	today := now.Format("2006-01-02")
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, today+".md"), []byte("# Daily Memory "+today+"\n\nToday entry."), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, yesterday+".md"), []byte("# Daily Memory "+yesterday+"\n\nYesterday entry."), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	file, err := GetDailyMarkdown(dir, "yesterday")
	if err != nil {
		t.Fatalf("GetDailyMarkdown(yesterday): %v", err)
	}
	if file.Date != yesterday {
		t.Fatalf("expected yesterday file, got %q", file.Date)
	}

	latest, err := GetDailyMarkdown(dir, "latest")
	if err != nil {
		t.Fatalf("GetDailyMarkdown(latest): %v", err)
	}
	if latest.Date != today {
		t.Fatalf("expected latest file, got %q", latest.Date)
	}
}
