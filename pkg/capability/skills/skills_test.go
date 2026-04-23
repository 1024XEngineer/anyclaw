package skills

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/capability/tools"
)

const testExecTimeoutSeconds = 30

func TestSkillsManagerLoadAndHelpers(t *testing.T) {
	root := t.TempDir()

	jsonDir := filepath.Join(root, "planner")
	if err := os.MkdirAll(jsonDir, 0o755); err != nil {
		t.Fatalf("mkdir planner: %v", err)
	}
	if err := os.WriteFile(filepath.Join(jsonDir, "skill.json"), []byte(`{
  "name": "planner",
  "description": "Plans work",
  "version": "1.2.3",
  "category": "ops",
  "commands": [{"name":"plan","pattern":"/plan"}],
  "prompts": {"system":"Plan carefully."},
  "permissions": ["files:read"],
  "entrypoint": "builtin://planner",
  "metadata": {"source":"local","registry":"custom","homepage":"https://example.com/planner","install_command":"planner install"}
}`), 0o644); err != nil {
		t.Fatalf("write planner skill: %v", err)
	}

	mdDir := filepath.Join(root, "markdown-skill")
	if err := os.MkdirAll(mdDir, 0o755); err != nil {
		t.Fatalf("mkdir markdown-skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mdDir, "SKILL.md"), []byte(`---
name: markdown-skill
description: Imported from markdown
---
Always answer with examples.
`), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}

	badDir := filepath.Join(root, "broken")
	if err := os.MkdirAll(badDir, 0o755); err != nil {
		t.Fatalf("mkdir broken: %v", err)
	}
	if err := os.WriteFile(filepath.Join(badDir, "skill.json"), []byte(`{`), 0o644); err != nil {
		t.Fatalf("write broken skill: %v", err)
	}

	manager := NewSkillsManager(root)
	if err := manager.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	planner, ok := manager.Get("planner")
	if !ok {
		t.Fatal("expected planner skill to load")
	}
	if planner.Source != "local" || planner.Registry != "custom" {
		t.Fatalf("unexpected planner metadata: source=%q registry=%q", planner.Source, planner.Registry)
	}
	if planner.Category != "ops" || planner.Metadata["category"] != "ops" {
		t.Fatalf("expected planner category to persist, got %+v", planner)
	}

	imported, ok := manager.Get("markdown-skill")
	if !ok {
		t.Fatal("expected markdown skill to load")
	}
	if imported.Source != "skillhub" {
		t.Fatalf("expected markdown skill to use skillhub source, got %q", imported.Source)
	}

	if _, ok := manager.Get("broken"); ok {
		t.Fatal("broken skill should be skipped")
	}

	list := manager.List()
	if len(list) < 47 {
		t.Fatalf("expected builtins plus local skills, got %d", len(list))
	}

	byCategory := manager.ListByCategory()
	if len(byCategory["ops"]) == 0 {
		t.Fatal("expected ops category to contain planner")
	}
	if len(byCategory["general"]) == 0 {
		t.Fatal("expected general category for markdown skill")
	}

	found := manager.FindByCommand("please /plan this")
	if len(found) != 1 || found[0].Name != "planner" {
		t.Fatalf("unexpected FindByCommand result: %#v", found)
	}

	toolDef := planner.ToTool()
	if toolDef.Name != "skill_planner" || toolDef.SkillName != "planner" {
		t.Fatalf("unexpected tool definition: %#v", toolDef)
	}
}

func TestSkillsManagerExecutionAndPrompts(t *testing.T) {
	scriptPath := writeExecutableSkillScript(t, "echo-skill", scriptBodyForTest("echo hello"))
	timeoutPath := writeExecutableSkillScript(t, "slow-skill", scriptBodyForTest("sleep"))
	failPath := writeExecutableSkillScript(t, "fail-skill", scriptBodyForTest("fail"))

	manager := NewSkillsManager(t.TempDir())
	manager.skills["plain"] = &Skill{
		Name:        "plain",
		Description: "plain skill",
		Prompts:     map[string]string{"system": "Answer plainly."},
		Metadata:    map[string]string{"category": "general"},
	}
	manager.skills["empty-prompt"] = &Skill{
		Name:     "empty-prompt",
		Prompts:  map[string]string{},
		Metadata: map[string]string{},
	}
	manager.skills["runner"] = &Skill{
		Name:        "runner",
		Version:     "1.0.0",
		Entrypoint:  filepath.Base(scriptPath),
		Prompts:     map[string]string{"system": "run"},
		Permissions: []string{"tools:exec"},
		Metadata:    map[string]string{"path": filepath.Dir(scriptPath)},
	}
	manager.skills["slow"] = &Skill{
		Name:       "slow",
		Entrypoint: filepath.Base(timeoutPath),
		Metadata:   map[string]string{"path": filepath.Dir(timeoutPath)},
	}
	manager.skills["failure"] = &Skill{
		Name:       "failure",
		Entrypoint: filepath.Base(failPath),
		Metadata:   map[string]string{"path": filepath.Dir(failPath)},
	}

	if _, err := manager.Execute(context.Background(), "missing", nil, ExecutionOptions{}); err == nil {
		t.Fatal("expected missing skill to error")
	}

	got, err := manager.Execute(context.Background(), "plain", nil, ExecutionOptions{})
	if err != nil || !strings.Contains(got, "Answer plainly.") {
		t.Fatalf("unexpected plain execute result %q, %v", got, err)
	}

	got, err = manager.Execute(context.Background(), "empty-prompt", nil, ExecutionOptions{})
	if err != nil || !strings.Contains(got, "declarative only") {
		t.Fatalf("unexpected empty-prompt result %q, %v", got, err)
	}

	if _, err := manager.Execute(context.Background(), "runner", nil, ExecutionOptions{}); err == nil {
		t.Fatal("expected disabled exec to error")
	}

	got, err = manager.Execute(context.Background(), "runner", map[string]any{"task": "say"}, ExecutionOptions{AllowExec: true, ExecTimeoutSeconds: testExecTimeoutSeconds})
	if err != nil {
		t.Fatalf("runner execute: %v", err)
	}
	if !strings.Contains(got, "hello") {
		t.Fatalf("expected script output, got %q", got)
	}

	if _, err := manager.Execute(context.Background(), "slow", nil, ExecutionOptions{AllowExec: true, ExecTimeoutSeconds: 1}); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout error, got %v", err)
	}

	if _, err := manager.Execute(context.Background(), "failure", nil, ExecutionOptions{AllowExec: true, ExecTimeoutSeconds: 30}); err == nil || !strings.Contains(err.Error(), "failed") {
		t.Fatalf("expected failure error, got %v", err)
	}

	prompt, err := manager.GetPrompt("plain", "system")
	if err != nil || prompt != "Answer plainly." {
		t.Fatalf("unexpected prompt %q, %v", prompt, err)
	}
	if _, err := manager.GetPrompt("plain", "missing"); err == nil {
		t.Fatal("expected missing prompt error")
	}
	if _, err := manager.GetPrompt("missing", "system"); err == nil {
		t.Fatal("expected missing skill prompt error")
	}

	systemPrompts := manager.GetSystemPrompts()
	if len(systemPrompts) < 2 {
		t.Fatalf("expected system prompts, got %#v", systemPrompts)
	}
}

func TestSkillsManagerRegisterFilterCatalogAndUtilities(t *testing.T) {
	manager := NewSkillsManager("/virtual/skills")
	manager.skills["Planner"] = &Skill{
		Name:           "Planner",
		Description:    "Plans",
		Version:        "1.0.0",
		Category:       "ops",
		Permissions:    []string{"files:read"},
		Entrypoint:     "builtin://Planner",
		Registry:       "builtin",
		Source:         "builtin",
		InstallCommand: "anyclaw skill install Planner",
		Metadata:       map[string]string{"category": "ops"},
	}
	manager.skills["Runner"] = &Skill{
		Name:           "Runner",
		Description:    "Runs",
		Version:        "2.0.0",
		Permissions:    []string{"tools:exec"},
		Entrypoint:     filepath.Base(writeExecutableSkillScript(t, "runner-tool", scriptBodyForTest("echo tool"))),
		Registry:       "local",
		Source:         "local",
		InstallCommand: "manual",
		Metadata:       map[string]string{"path": filepath.Dir(writeExecutableSkillScript(t, "runner-tool-2", scriptBodyForTest("echo tool"))), "category": "ops"},
	}

	filtered := manager.FilterEnabled([]string{" runner "})
	if len(filtered.skills) != 1 {
		t.Fatalf("expected 1 filtered skill, got %d", len(filtered.skills))
	}
	if filtered.FilterEnabled(nil) != filtered {
		t.Fatal("expected FilterEnabled(nil) to return same manager")
	}

	registry := tools.NewRegistry()
	manager.RegisterTools(registry, ExecutionOptions{})
	got, err := registry.Call(context.Background(), "skill_Planner", map[string]any{"action": "run"})
	if err != nil || !strings.Contains(got, "declarative only") {
		t.Fatalf("unexpected registry result %q, %v", got, err)
	}

	entries := manager.Catalog()
	if len(entries) != 2 {
		t.Fatalf("expected 2 catalog entries, got %d", len(entries))
	}
	var plannerEntry SkillCatalogEntry
	for _, entry := range entries {
		if entry.Name == "Planner" {
			plannerEntry = entry
			break
		}
	}
	if !plannerEntry.Builtin || plannerEntry.Category != "ops" {
		t.Fatalf("unexpected planner catalog entry: %#v", plannerEntry)
	}

	if !manager.skills["Runner"].IsExecutable() {
		t.Fatal("expected runner to be executable")
	}
	if manager.skills["Planner"].IsExecutable() {
		t.Fatal("builtin planner should not be executable")
	}

	if launcher, args, err := resolveSkillLauncher(filepath.Join(t.TempDir(), "run.bin")); err != nil || launcher == "" || len(args) != 0 {
		t.Fatalf("unexpected default launcher result: %q %#v %v", launcher, args, err)
	}
	if _, err := findFirstExecutable("definitely-not-a-real-binary"); err == nil {
		t.Fatal("expected missing executable error")
	}
	if got := NormalizeKey("  Plan "); got != "plan" {
		t.Fatalf("unexpected normalized key %q", got)
	}
}

func writeExecutableSkillScript(t *testing.T, name, body string) string {
	t.Helper()

	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir script dir: %v", err)
	}

	var scriptPath string
	if runtime.GOOS == "windows" {
		scriptPath = filepath.Join(dir, "run.ps1")
	} else {
		scriptPath = filepath.Join(dir, "run.sh")
	}

	if err := os.WriteFile(scriptPath, []byte(body), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	return scriptPath
}

func scriptBodyForTest(mode string) string {
	if runtime.GOOS == "windows" {
		switch mode {
		case "sleep":
			return "Start-Sleep -Seconds 2\nWrite-Output 'slow'\n"
		case "fail":
			return "Write-Error 'boom'\nexit 1\n"
		default:
			return "Write-Output 'hello'\n"
		}
	}

	switch mode {
	case "sleep":
		return "#!/bin/sh\nsleep 2\necho slow\n"
	case "fail":
		return "#!/bin/sh\necho boom 1>&2\nexit 1\n"
	default:
		return "#!/bin/sh\necho hello\n"
	}
}

func TestSkillFromDefinitionDefaults(t *testing.T) {
	def := skillFileDefinition{
		Entrypoint: "builtin://writer",
		Prompts: map[string]string{
			"system": "write",
		},
	}
	skill := skillFromDefinition(def, "")
	if skill.Name != "writer" {
		t.Fatalf("expected inferred name writer, got %q", skill.Name)
	}
	if skill.InstallCommand != "anyclaw skill install writer" {
		t.Fatalf("unexpected install command: %q", skill.InstallCommand)
	}
	if skill.Metadata["path"] != "builtin://writer" {
		t.Fatalf("unexpected metadata path: %q", skill.Metadata["path"])
	}
}

func TestLoadSkillErrors(t *testing.T) {
	manager := NewSkillsManager(t.TempDir())

	missingDir := filepath.Join(t.TempDir(), "missing")
	if err := os.MkdirAll(missingDir, 0o755); err != nil {
		t.Fatalf("mkdir missingDir: %v", err)
	}
	if _, err := manager.loadSkill(missingDir); err == nil {
		t.Fatal("expected missing skill file error")
	}

	invalidDir := filepath.Join(t.TempDir(), "invalid")
	if err := os.MkdirAll(invalidDir, 0o755); err != nil {
		t.Fatalf("mkdir invalidDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(invalidDir, "skill.json"), []byte(`not-json`), 0o644); err != nil {
		t.Fatalf("write invalid skill: %v", err)
	}
	if _, err := manager.loadSkill(invalidDir); err == nil {
		t.Fatal("expected invalid json error")
	}
}

func TestExecuteSkillEntrypointRejectsMissingDirectory(t *testing.T) {
	skill := &Skill{Name: "no-dir", Entrypoint: "run.ps1", Metadata: map[string]string{}}
	if _, err := executeSkillEntrypoint(context.Background(), skill, map[string]any{}, ExecutionOptions{AllowExec: true}); err == nil {
		t.Fatal("expected missing execution directory error")
	}
}

func TestExecuteSkillEntrypointRejectsInvalidInput(t *testing.T) {
	skill := &Skill{Name: "bad-input", Entrypoint: "builtin://bad", Metadata: map[string]string{"path": t.TempDir()}}
	input := map[string]any{"bad": make(chan int)}
	if _, err := executeSkillEntrypoint(context.Background(), skill, input, ExecutionOptions{AllowExec: true}); err == nil {
		t.Fatal("expected json marshal error")
	}
}

func TestResolveSkillLauncherPrefersKnownRuntimes(t *testing.T) {
	var entrypoint string
	if runtime.GOOS == "windows" {
		entrypoint = filepath.Join(t.TempDir(), "run.ps1")
	} else {
		entrypoint = filepath.Join(t.TempDir(), "run.sh")
	}

	launcher, args, err := resolveSkillLauncher(entrypoint)
	if err != nil {
		t.Fatalf("resolveSkillLauncher: %v", err)
	}
	if launcher == "" || len(args) == 0 {
		t.Fatalf("expected launcher and args, got %q %#v", launcher, args)
	}
}

func TestExecuteSkillEntrypointUsesEnvPayload(t *testing.T) {
	scriptPath := writeExecutableSkillScript(t, "env-skill", envEchoScript())
	skill := &Skill{
		Name:        "env-skill",
		Version:     "9.9.9",
		Entrypoint:  filepath.Base(scriptPath),
		Permissions: []string{"files:read", "tools:exec"},
		Metadata:    map[string]string{"path": filepath.Dir(scriptPath)},
	}

	out, err := executeSkillEntrypoint(context.Background(), skill, map[string]any{"message": "ok"}, ExecutionOptions{AllowExec: true, ExecTimeoutSeconds: testExecTimeoutSeconds})
	if err != nil {
		t.Fatalf("executeSkillEntrypoint: %v", err)
	}

	var payload map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &payload); err != nil {
		t.Fatalf("unmarshal env payload: %v", err)
	}
	if payload["name"] != "env-skill" || payload["version"] != "9.9.9" || !strings.Contains(payload["perms"], "tools:exec") {
		t.Fatalf("unexpected env payload: %#v", payload)
	}
	if payload["input"] == "" {
		t.Fatalf("expected serialized input in payload: %#v", payload)
	}
}

func envEchoScript() string {
	if runtime.GOOS == "windows" {
		return "$out = @{input=$env:ANYCLAW_SKILL_INPUT;name=$env:ANYCLAW_SKILL_NAME;version=$env:ANYCLAW_SKILL_VERSION;perms=$env:ANYCLAW_SKILL_PERMISSIONS} | ConvertTo-Json -Compress\nWrite-Output $out\n"
	}
	return "#!/bin/sh\ninput=missing\nif [ -n \"$ANYCLAW_SKILL_INPUT\" ]; then\n  input=present\nfi\nprintf '{\"input\":\"%s\",\"name\":\"%s\",\"version\":\"%s\",\"perms\":\"%s\"}' \"$input\" \"$ANYCLAW_SKILL_NAME\" \"$ANYCLAW_SKILL_VERSION\" \"$ANYCLAW_SKILL_PERMISSIONS\"\n"
}

func TestExecuteSkillEntrypointTimeoutIsFast(t *testing.T) {
	start := time.Now()
	scriptPath := writeExecutableSkillScript(t, "quick-timeout", scriptBodyForTest("sleep"))
	skill := &Skill{
		Name:       "quick-timeout",
		Entrypoint: filepath.Base(scriptPath),
		Metadata:   map[string]string{"path": filepath.Dir(scriptPath)},
	}
	_, _ = executeSkillEntrypoint(context.Background(), skill, nil, ExecutionOptions{AllowExec: true, ExecTimeoutSeconds: 1})
	if time.Since(start) > 5*time.Second {
		t.Fatal("timeout test took unexpectedly long")
	}
}
