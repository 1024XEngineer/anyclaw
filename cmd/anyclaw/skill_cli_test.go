package main

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/1024XEngineer/anyclaw/pkg/capability/skills"
)

func clearSkillCLIEnv(t *testing.T) {
	t.Helper()
	clearModelsCLIEnv(t)
	t.Setenv("ANYCLAW_SKILLS_DIR", "")
}

func TestRunAnyClawCLIRoutesSkillList(t *testing.T) {
	clearSkillCLIEnv(t)

	skillsDir := t.TempDir()
	t.Setenv("ANYCLAW_SKILLS_DIR", skillsDir)
	writeSkillCLIFile(t, skillsDir, "demo-skill", "Demo skill", "1.2.3")

	stdout, _, err := captureCLIOutput(t, func() error {
		return runAnyClawCLI([]string{"skill", "list"})
	})
	if err != nil {
		t.Fatalf("runAnyClawCLI skill list: %v", err)
	}
	if !strings.Contains(stdout, "- demo-skill v1.2.3") {
		t.Fatalf("expected local skill in output, got %q", stdout)
	}
}

func TestRunSkillCommandWithoutArgsPrintsUsage(t *testing.T) {
	clearSkillCLIEnv(t)

	stdout, _, err := captureCLIOutput(t, func() error {
		return runSkillCommand(nil)
	})
	if err != nil {
		t.Fatalf("runSkillCommand: %v", err)
	}
	if !strings.Contains(stdout, "AnyClaw skill commands:") {
		t.Fatalf("expected skill usage output, got %q", stdout)
	}
}

func TestRunSkillCommandUnknownSubcommandPrintsUsage(t *testing.T) {
	clearSkillCLIEnv(t)

	stdout, _, err := captureCLIOutput(t, func() error {
		return runSkillCommand([]string{"unknown"})
	})
	if err == nil || !strings.Contains(err.Error(), "unknown skill command") {
		t.Fatalf("expected unknown skill command error, got %v", err)
	}
	if !strings.Contains(stdout, "AnyClaw skill commands:") {
		t.Fatalf("expected skill usage output, got %q", stdout)
	}
}

func TestRunSkillSearchUsesRemoteResults(t *testing.T) {
	clearSkillCLIEnv(t)
	withSkillSearchStub(t, func(context.Context, string, int) ([]skills.SkillSearchResult, error) {
		return []skills.SkillSearchResult{
			{
				Name:        "planner",
				FullName:    "acme/planner",
				Description: "Planning skill",
				Installs:    1200,
				Stars:       150,
			},
		}, nil
	})

	stdout, _, err := captureCLIOutput(t, func() error {
		return runSkillCommand([]string{"search", "planner"})
	})
	if err != nil {
		t.Fatalf("runSkillCommand search: %v", err)
	}
	for _, want := range []string{
		"Searching skills.sh: planner",
		"Found 1 skills",
		"acme/planner",
		"Planning skill",
		"recommended",
		"anyclaw skill install planner",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected %q in output, got %q", want, stdout)
		}
	}
}

func TestRunSkillSearchFallsBackToBuiltins(t *testing.T) {
	clearSkillCLIEnv(t)
	withSkillSearchStub(t, func(context.Context, string, int) ([]skills.SkillSearchResult, error) {
		return nil, nil
	})

	stdout, _, err := captureCLIOutput(t, func() error {
		return runSkillCommand([]string{"search", "missing"})
	})
	if err != nil {
		t.Fatalf("runSkillCommand search fallback: %v", err)
	}
	if !strings.Contains(stdout, "Built-in skills:") {
		t.Fatalf("expected builtin fallback output, got %q", stdout)
	}
	if builtin := firstBuiltinSkillName(t); !strings.Contains(stdout, builtin) {
		t.Fatalf("expected builtin skill %q in output, got %q", builtin, stdout)
	}
}

func TestRunSkillCatalogPrintsResults(t *testing.T) {
	clearSkillCLIEnv(t)
	withSkillCatalogStub(t, func(context.Context, string, int) ([]skills.SkillCatalogEntry, error) {
		return []skills.SkillCatalogEntry{
			{
				Name:        "planner",
				FullName:    "acme/planner",
				Description: "Planning skill",
				Version:     "1.2.3",
			},
		}, nil
	})

	stdout, _, err := captureCLIOutput(t, func() error {
		return runSkillCommand([]string{"catalog", "planner"})
	})
	if err != nil {
		t.Fatalf("runSkillCommand catalog: %v", err)
	}
	for _, want := range []string{
		"Skill catalog:",
		"- acme/planner v1.2.3",
		"Planning skill",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected %q in output, got %q", want, stdout)
		}
	}
}

func TestRunSkillInstallBuiltinAndInfo(t *testing.T) {
	clearSkillCLIEnv(t)

	skillsDir := t.TempDir()
	t.Setenv("ANYCLAW_SKILLS_DIR", skillsDir)
	name := firstBuiltinSkillName(t)

	stdout, _, err := captureCLIOutput(t, func() error {
		return runSkillCommand([]string{"install", name})
	})
	if err != nil {
		t.Fatalf("runSkillCommand install builtin: %v", err)
	}
	if !strings.Contains(stdout, "Installed skill: "+name) {
		t.Fatalf("unexpected install output: %q", stdout)
	}
	if _, err := os.Stat(filepath.Join(skillsDir, name, "skill.json")); err != nil {
		t.Fatalf("expected installed skill.json: %v", err)
	}

	stdout, _, err = captureCLIOutput(t, func() error {
		return runSkillCommand([]string{"info", name})
	})
	if err != nil {
		t.Fatalf("runSkillCommand info: %v", err)
	}
	if !strings.Contains(stdout, "Name: "+name) {
		t.Fatalf("expected info output for %q, got %q", name, stdout)
	}
}

func TestRunSkillInstallUsesGitHubReference(t *testing.T) {
	clearSkillCLIEnv(t)

	skillsDir := t.TempDir()
	t.Setenv("ANYCLAW_SKILLS_DIR", skillsDir)

	var gotOwner, gotRepo, gotSkill, gotDir string
	withSkillInstallStub(t, func(_ context.Context, owner string, repo string, skillName string, destDir string) error {
		gotOwner, gotRepo, gotSkill, gotDir = owner, repo, skillName, destDir
		return nil
	})

	stdout, _, err := captureCLIOutput(t, func() error {
		return runSkillCommand([]string{"install", "acme/demo-repo/demo-skill"})
	})
	if err != nil {
		t.Fatalf("runSkillCommand install ref: %v", err)
	}
	if gotOwner != "acme" || gotRepo != "demo-repo" || gotSkill != "demo-skill" || gotDir != skillsDir {
		t.Fatalf("unexpected install args: owner=%q repo=%q skill=%q dir=%q", gotOwner, gotRepo, gotSkill, gotDir)
	}
	if !strings.Contains(stdout, "Installed skill: demo-skill") {
		t.Fatalf("unexpected install output: %q", stdout)
	}
}

func TestRunSkillInstallUsesSearchResultSource(t *testing.T) {
	clearSkillCLIEnv(t)

	skillsDir := t.TempDir()
	t.Setenv("ANYCLAW_SKILLS_DIR", skillsDir)

	withSkillSearchStub(t, func(context.Context, string, int) ([]skills.SkillSearchResult, error) {
		return []skills.SkillSearchResult{
			{Name: "demo-skill", Source: "https://github.com/acme/demo-repo"},
		}, nil
	})

	var gotOwner, gotRepo, gotSkill string
	withSkillInstallStub(t, func(_ context.Context, owner string, repo string, skillName string, destDir string) error {
		gotOwner, gotRepo, gotSkill = owner, repo, skillName
		return nil
	})

	stdout, _, err := captureCLIOutput(t, func() error {
		return runSkillCommand([]string{"install", "demo-skill"})
	})
	if err != nil {
		t.Fatalf("runSkillCommand install search result: %v", err)
	}
	if gotOwner != "acme" || gotRepo != "demo-repo" || gotSkill != "demo-skill" {
		t.Fatalf("unexpected install args from search result: owner=%q repo=%q skill=%q", gotOwner, gotRepo, gotSkill)
	}
	if !strings.Contains(stdout, "Installed skill: demo-skill") {
		t.Fatalf("unexpected install output: %q", stdout)
	}
}

func TestRunSkillListHandlesEmptyDirAndInfoErrors(t *testing.T) {
	clearSkillCLIEnv(t)

	skillsDir := t.TempDir()
	t.Setenv("ANYCLAW_SKILLS_DIR", skillsDir)

	stdout, _, err := captureCLIOutput(t, func() error {
		return runSkillCommand([]string{"list"})
	})
	if err != nil {
		t.Fatalf("runSkillCommand list: %v", err)
	}
	if !strings.Contains(stdout, "No local skills found.") {
		t.Fatalf("expected empty list message, got %q", stdout)
	}

	if _, _, err := captureCLIOutput(t, func() error {
		return runSkillCommand([]string{"info"})
	}); err == nil || !strings.Contains(err.Error(), "usage: anyclaw skill info <name>") {
		t.Fatalf("expected skill info usage error, got %v", err)
	}
	if _, _, err := captureCLIOutput(t, func() error {
		return runSkillCommand([]string{"info", "missing"})
	}); err == nil || !strings.Contains(err.Error(), "skill not found") {
		t.Fatalf("expected missing skill error, got %v", err)
	}
}

func TestSkillCLIInstallValidationAndHelpers(t *testing.T) {
	clearSkillCLIEnv(t)

	if err := runSkillCommand([]string{"install"}); err == nil || !strings.Contains(err.Error(), "usage: anyclaw skill install <name>") {
		t.Fatalf("expected install usage error, got %v", err)
	}

	withSkillSearchStub(t, func(context.Context, string, int) ([]skills.SkillSearchResult, error) {
		return nil, nil
	})
	if err := runSkillCommand([]string{"install", "missing"}); err == nil || !strings.Contains(err.Error(), "skill not found: missing") {
		t.Fatalf("expected missing skill error, got %v", err)
	}

	withSkillSearchStub(t, func(context.Context, string, int) ([]skills.SkillSearchResult, error) {
		return []skills.SkillSearchResult{{Name: "bad", Source: "invalid-source"}}, nil
	})
	if err := runSkillCommand([]string{"install", "bad"}); err == nil || !strings.Contains(err.Error(), "skill source is invalid") {
		t.Fatalf("expected invalid source error, got %v", err)
	}

	if owner, repo, skillName, ok := parseSkillInstallRef("acme/demo-repo/demo"); !ok || owner != "acme" || repo != "demo-repo" || skillName != "demo" {
		t.Fatalf("unexpected parsed install ref: %q %q %q %v", owner, repo, skillName, ok)
	}
	if owner, repo, ok := resolveSkillSource("github.com/acme/demo-repo"); !ok || owner != "acme" || repo != "demo-repo" {
		t.Fatalf("unexpected github source: %q %q %v", owner, repo, ok)
	}
	if formatInstalls(1500) != "1.5K" {
		t.Fatalf("unexpected formatted installs: %s", formatInstalls(1500))
	}
	if getQualityBadge(1200, 150) != "recommended" {
		t.Fatalf("unexpected quality badge")
	}
	if skillDisplayName("demo", "acme/demo") != "acme/demo" {
		t.Fatalf("unexpected skill display name")
	}
	if skillDescription("") != "No description" {
		t.Fatalf("unexpected empty skill description")
	}
}

func TestParseSkillInstallRefRejectsTraversalSegments(t *testing.T) {
	clearSkillCLIEnv(t)

	for _, raw := range []string{
		"acme/repo/..",
		"acme/repo/.",
		"acme/repo/..\\..\\tmp\\pwn",
		"acme/repo/C:\\tmp\\pwn",
		"acme/repo/C:tmp",
	} {
		if _, _, _, ok := parseSkillInstallRef(raw); ok {
			t.Fatalf("expected install ref %q to be rejected", raw)
		}
	}

	if owner, repo, skillName, ok := parseSkillInstallRef("acme/demo-repo/demo-skill"); !ok || owner != "acme" || repo != "demo-repo" || skillName != "demo-skill" {
		t.Fatalf("expected valid install ref to pass, got owner=%q repo=%q skill=%q ok=%v", owner, repo, skillName, ok)
	}
}

func withSkillSearchStub(t *testing.T, stub func(context.Context, string, int) ([]skills.SkillSearchResult, error)) {
	t.Helper()
	original := searchRemoteSkills
	searchRemoteSkills = stub
	t.Cleanup(func() {
		searchRemoteSkills = original
	})
}

func withSkillCatalogStub(t *testing.T, stub func(context.Context, string, int) ([]skills.SkillCatalogEntry, error)) {
	t.Helper()
	original := searchSkillCatalog
	searchSkillCatalog = stub
	t.Cleanup(func() {
		searchSkillCatalog = original
	})
}

func withSkillInstallStub(t *testing.T, stub func(context.Context, string, string, string, string) error) {
	t.Helper()
	original := installGitHubSkill
	installGitHubSkill = stub
	t.Cleanup(func() {
		installGitHubSkill = original
	})
}

func writeSkillCLIFile(t *testing.T, skillsDir string, name string, description string, version string) {
	t.Helper()

	skillDir := filepath.Join(skillsDir, name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll skill dir: %v", err)
	}
	data := `{"name":"` + name + `","description":"` + description + `","version":"` + version + `"}`
	if err := os.WriteFile(filepath.Join(skillDir, "skill.json"), []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile skill.json: %v", err)
	}
}

func firstBuiltinSkillName(t *testing.T) string {
	t.Helper()

	names := make([]string, 0, len(skills.BuiltinSkills))
	for name := range skills.BuiltinSkills {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		t.Fatal("expected builtin skills to be available")
	}
	return names[0]
}
