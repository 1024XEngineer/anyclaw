package clihub

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCapabilityRegistryLoad(t *testing.T) {
	root := findTestRoot()
	if root == "" {
		t.Skip("CLI-Anything root not found")
	}

	reg, err := LoadCapabilityRegistry(root)
	if err != nil {
		t.Fatalf("LoadCapabilityRegistry: %v", err)
	}

	if reg.Count() == 0 {
		t.Fatalf("expected capabilities, got 0")
	}

	t.Logf("Loaded %d capabilities from %d harnesses", reg.Count(), len(reg.Harnesses()))

	shotcutCaps := reg.FindByHarness("shotcut")
	if len(shotcutCaps) == 0 {
		t.Fatalf("expected shotcut capabilities")
	}
	t.Logf("Shotcut has %d commands", len(shotcutCaps))
}

func TestIntentMatching(t *testing.T) {
	root := findTestRoot()
	if root == "" {
		t.Skip("CLI-Anything root not found")
	}

	engine, err := NewIntentEngine(root)
	if err != nil {
		t.Fatalf("NewIntentEngine: %v", err)
	}

	tests := []struct {
		query       string
		wantHarness string
		wantCommand string
	}{
		{"create a new video project", "shotcut", "new"},
		{"list all models", "ollama", "list"},
		{"add a clip to timeline", "shotcut", "add-clip"},
	}

	for _, tt := range tests {
		intent := engine.Parse(tt.query)
		if intent == nil {
			t.Errorf("no match for: %s", tt.query)
			continue
		}
		if intent.Target != tt.wantHarness {
			t.Errorf("%s -> got harness %s, want %s", tt.query, intent.Target, tt.wantHarness)
		}
		if intent.Subject != tt.wantCommand {
			t.Errorf("%s -> got command %s, want %s", tt.query, intent.Subject, tt.wantCommand)
		}
	}
}

func TestSkillParsing(t *testing.T) {
	root := findTestRoot()
	if root == "" {
		t.Skip("CLI-Anything root not found")
	}

	cat, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	skills := LoadSkillsForCatalog(cat)
	if len(skills) == 0 {
		t.Fatalf("expected skills")
	}

	shotcut, ok := skills["shotcut"]
	if !ok {
		t.Fatalf("expected shotcut skill")
	}

	if len(shotcut.Commands) == 0 {
		t.Fatalf("expected commands in shotcut skill")
	}

	t.Logf("Shotcut skill has %d commands", len(shotcut.Commands))

	for _, cmd := range shotcut.Commands {
		if cmd.Group == "" {
			t.Logf("  - %s (no group)", cmd.Name)
		} else {
			t.Logf("  - [%s] %s", cmd.Group, cmd.Name)
		}
	}
}

func findTestRoot() string {
	dirs := []string{
		"D:\\anyclaw\\anyclaw\\anyclaw\\anyclaw3\\CLI-Anything-0.2.0",
		"D:\\anyclaw\\anyclaw\\anyclaw\\CLI-Anything-0.2.0",
		"..\\CLI-Anything-0.2.0",
		".",
	}

	for _, dir := range dirs {
		if info, err := os.Stat(filepath.Join(dir, "registry.json")); err == nil && !info.IsDir() {
			return dir
		}
	}

	for p := "."; ; {
		if info, err := os.Stat(filepath.Join(p, "registry.json")); err == nil && !info.IsDir() {
			return p
		}
		parent := filepath.Dir(p)
		if parent == p {
			break
		}
		p = parent
	}

	return ""
}
