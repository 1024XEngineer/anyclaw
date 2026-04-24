package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/1024XEngineer/anyclaw/pkg/clihub"
)

func TestCLIHubExecHelperProcess(t *testing.T) {
	if os.Getenv("ANYCLAW_CLIHUB_HELPER") == "" {
		return
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}

	fmt.Printf("helper cwd: %s\nhelper args: %s", wd, strings.Join(os.Args[1:], "|"))
	os.Exit(0)
}

func TestRunAnyClawCLIRoutesCLIHubCommand(t *testing.T) {
	stdout, _, err := captureCLIOutput(t, func() error {
		return runAnyClawCLI([]string{"clihub"})
	})
	if err != nil {
		t.Fatalf("runAnyClawCLI clihub: %v", err)
	}
	if !strings.Contains(stdout, "AnyClaw clihub commands:") {
		t.Fatalf("expected clihub usage output, got %q", stdout)
	}
}

func TestCLIUsageIncludesCLIHubCommand(t *testing.T) {
	stdout, _, err := captureCLIOutput(t, func() error {
		return runAnyClawCLI([]string{"help"})
	})
	if err != nil {
		t.Fatalf("runAnyClawCLI help: %v", err)
	}
	if !strings.Contains(stdout, "anyclaw clihub <subcommand>") {
		t.Fatalf("expected clihub help entry, got %q", stdout)
	}
}

func TestCLIHubUsageDocumentsCwdBehavior(t *testing.T) {
	stdout, _, err := captureCLIOutput(t, func() error {
		return runCLIHubCommand([]string{"help"})
	})
	if err != nil {
		t.Fatalf("runCLIHubCommand help: %v", err)
	}
	for _, want := range []string{
		"--cwd <path>        Working directory override for installed executables",
		"clihub install requires an explicit trusted root via --root or ANYCLAW_CLI_ANYTHING_ROOT.",
		"Install does not execute catalog shell from roots discovered implicitly from the current workspace.",
		"Source harnesses always run from their checkout directory",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected %q in clihub help, got %q", want, stdout)
		}
	}
}

func TestCLIHubUsageDocumentsSearchLimitAndWorkspace(t *testing.T) {
	stdout, _, err := captureCLIOutput(t, func() error {
		return runCLIHubCommand([]string{"help"})
	})
	if err != nil {
		t.Fatalf("runCLIHubCommand help: %v", err)
	}

	for _, want := range []string{
		"anyclaw clihub search [query] [--category <name>] [--installed] [--limit <n>] [--json] [--workspace <path>]",
		"anyclaw clihub list [--installed] [--runnable] [--limit <n>] [--json] [--workspace <path>]",
		"anyclaw clihub installed [--json] [--workspace <path>]",
		"anyclaw clihub info <name> [--json] [--workspace <path>]",
		"anyclaw clihub capabilities [query] [--harness <name>] [--limit <n>] [--json] [--workspace <path>]",
		"anyclaw clihub exec <name> [--json=true|false] [--auto-install] [--cwd <path>] [--workspace <path>] [-- <args...>]",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected %q in clihub help, got %q", want, stdout)
		}
	}
}

func TestRunCLIHubCommandUnknownSubcommandPrintsUsage(t *testing.T) {
	stdout, _, err := captureCLIOutput(t, func() error {
		return runCLIHubCommand([]string{"unknown"})
	})
	if err == nil || !strings.Contains(err.Error(), "unknown clihub command") {
		t.Fatalf("expected unknown clihub command error, got %v", err)
	}
	if !strings.Contains(stdout, "AnyClaw clihub commands:") {
		t.Fatalf("expected clihub usage output, got %q", stdout)
	}
}

func TestRunCLIHubSearchSupportsJSONAndTextOutput(t *testing.T) {
	root := writeCLIHubTestRoot(t)

	stdout, _, err := captureCLIOutput(t, func() error {
		return runCLIHubSearch([]string{"--root", root, "--category", "video", "--json", "video"})
	})
	if err != nil {
		t.Fatalf("runCLIHubSearch json: %v", err)
	}

	var payload struct {
		Count   int                  `json:"count"`
		Query   string               `json:"query"`
		Results []clihub.EntryStatus `json:"results"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("Unmarshal output: %v\noutput=%s", err, stdout)
	}
	if payload.Count != 1 || payload.Query != "video" || len(payload.Results) != 1 || payload.Results[0].Name != "video-source" {
		t.Fatalf("unexpected search payload: %#v", payload)
	}

	stdout, _, err = captureCLIOutput(t, func() error {
		return runCLIHubSearch([]string{"--root", root, "helper"})
	})
	if err != nil {
		t.Fatalf("runCLIHubSearch text: %v", err)
	}
	if !strings.Contains(stdout, "CLI Hub root:") || !strings.Contains(stdout, "Helper Tool (installed)") {
		t.Fatalf("unexpected text search output: %q", stdout)
	}
}

func TestRunCLIHubListInstalledAndInfoCommands(t *testing.T) {
	root := writeCLIHubTestRoot(t)

	stdout, _, err := captureCLIOutput(t, func() error {
		return runCLIHubList([]string{"--root", root, "--runnable", "--json"})
	})
	if err != nil {
		t.Fatalf("runCLIHubList json: %v", err)
	}

	var listPayload struct {
		Count    int                  `json:"count"`
		Runnable bool                 `json:"runnable"`
		Results  []clihub.EntryStatus `json:"results"`
	}
	if err := json.Unmarshal([]byte(stdout), &listPayload); err != nil {
		t.Fatalf("Unmarshal output: %v\noutput=%s", err, stdout)
	}
	if !listPayload.Runnable || listPayload.Count != 2 || !containsCLIHubEntry(listPayload.Results, "helper") || !containsCLIHubEntry(listPayload.Results, "video-source") {
		t.Fatalf("unexpected list payload: %#v", listPayload)
	}

	stdout, _, err = captureCLIOutput(t, func() error {
		return runCLIHubInstalled([]string{"--root", root})
	})
	if err != nil {
		t.Fatalf("runCLIHubInstalled: %v", err)
	}
	if !strings.Contains(stdout, "Installed CLI-Anything harnesses:") || !strings.Contains(stdout, "helper ->") {
		t.Fatalf("unexpected installed output: %q", stdout)
	}

	stdout, _, err = captureCLIOutput(t, func() error {
		return runCLIHubInfo([]string{"--root", root, "video-source"})
	})
	if err != nil {
		t.Fatalf("runCLIHubInfo: %v", err)
	}
	for _, want := range []string{
		"Name: video-source",
		"Status: source",
		"Runnable: true",
		"Skill: ",
		"Dev module: cli_anything.editor",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected %q in info output, got %q", want, stdout)
		}
	}
}

func TestRunCLIHubInstallSupportsAlreadyInstalledAndCatalogEntries(t *testing.T) {
	root := writeCLIHubTestRoot(t)

	stdout, _, err := captureCLIOutput(t, func() error {
		return runCLIHubInstall([]string{"--root", root, "helper"})
	})
	if err != nil {
		t.Fatalf("runCLIHubInstall helper: %v", err)
	}
	if !strings.Contains(stdout, "helper is already installed") {
		t.Fatalf("expected already-installed output, got %q", stdout)
	}

	stdout, _, err = captureCLIOutput(t, func() error {
		return runCLIHubInstall([]string{"--root", root, "catalog-only"})
	})
	if err != nil {
		t.Fatalf("runCLIHubInstall catalog-only: %v", err)
	}
	if !strings.Contains(stdout, "Installed catalog-only") {
		t.Fatalf("expected install success output, got %q", stdout)
	}
}

func TestRunCLIHubInstallRejectsImplicitRootDiscovery(t *testing.T) {
	root := writeCLIHubTestRoot(t)

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("Chdir root: %v", err)
	}
	defer func() {
		_ = os.Chdir(originalWD)
	}()

	_, _, err = captureCLIOutput(t, func() error {
		return runCLIHubInstall([]string{"catalog-only"})
	})
	if err == nil || !strings.Contains(err.Error(), "explicit trusted root") {
		t.Fatalf("expected trusted root error, got %v", err)
	}
}

func TestRunCLIHubInstallAllowsExplicitEnvRoot(t *testing.T) {
	root := writeCLIHubTestRoot(t)
	t.Setenv(clihub.EnvRoot, root)

	stdout, _, err := captureCLIOutput(t, func() error {
		return runCLIHubInstall([]string{"helper"})
	})
	if err != nil {
		t.Fatalf("runCLIHubInstall env root: %v", err)
	}
	if !strings.Contains(stdout, "helper is already installed") {
		t.Fatalf("expected env-root install output, got %q", stdout)
	}
}

func TestRunCLIHubCapabilitiesSupportsHarnessAndIntentQueries(t *testing.T) {
	root := writeCLIHubTestRoot(t)

	stdout, _, err := captureCLIOutput(t, func() error {
		return runCLIHubCapabilities([]string{"--root", root, "--harness", "helper", "--json"})
	})
	if err != nil {
		t.Fatalf("runCLIHubCapabilities harness json: %v", err)
	}

	var harnessPayload struct {
		Count        int                 `json:"count"`
		Harness      string              `json:"harness"`
		Capabilities []clihub.Capability `json:"capabilities"`
	}
	if err := json.Unmarshal([]byte(stdout), &harnessPayload); err != nil {
		t.Fatalf("Unmarshal harness output: %v\noutput=%s", err, stdout)
	}
	if harnessPayload.Harness != "helper" || harnessPayload.Count != 2 {
		t.Fatalf("unexpected harness payload: %#v", harnessPayload)
	}

	stdout, _, err = captureCLIOutput(t, func() error {
		return runCLIHubCapabilities([]string{"--root", root, "create", "video"})
	})
	if err != nil {
		t.Fatalf("runCLIHubCapabilities query text: %v", err)
	}
	if !strings.Contains(stdout, "CLI Hub capabilities (") || !strings.Contains(stdout, "video-source -> Project / new") {
		t.Fatalf("unexpected capabilities output: %q", stdout)
	}
}

func TestRunCLIHubExecRunsHelperBinary(t *testing.T) {
	root := writeCLIHubTestRoot(t)
	t.Setenv("ANYCLAW_CLIHUB_HELPER", "1")
	cwd := t.TempDir()

	stdout, _, err := captureCLIOutput(t, func() error {
		return runCLIHubExec([]string{"--root", root, "--cwd", cwd, "--json=false", "helper", "--", "-test.run=TestCLIHubExecHelperProcess", "--", "ping"})
	})
	if err != nil {
		t.Fatalf("runCLIHubExec: %v", err)
	}
	if !strings.Contains(stdout, "helper cwd: "+cwd) || !strings.Contains(stdout, "helper args:") || !strings.Contains(stdout, "ping") {
		t.Fatalf("unexpected exec output: %q", stdout)
	}
}

func TestResolveInvocationKeepsSourceHarnessCheckoutAsCwd(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "video-source", "agent-harness")
	requestedCwd := filepath.Join(t.TempDir(), "workspace")

	args, cwd, err := clihub.ResolveInvocation(clihub.EntryStatus{
		Entry: clihub.Entry{
			Name:       "video-source",
			EntryPoint: "video-source",
		},
		Runnable:   true,
		RunMode:    "source",
		SourcePath: sourcePath,
		DevModule:  "cli_anything.editor",
	}, requestedCwd, clihub.ExecOptions{
		PreferLocalSrc: true,
		RequestedCwd:   requestedCwd,
	})
	if err != nil {
		t.Fatalf("ResolveInvocation: %v", err)
	}
	if len(args) == 0 {
		t.Fatalf("expected command args, got %#v", args)
	}
	if cwd != sourcePath {
		t.Fatalf("expected source harness cwd %q, got %q", sourcePath, cwd)
	}
}

func TestResolveCLIHubStartSupportsRootWorkspaceAndCWD(t *testing.T) {
	root := filepath.Join(t.TempDir(), "root")
	if got, err := resolveCLIHubStart(root, ""); err != nil || got != root {
		t.Fatalf("resolveCLIHubStart root = %q, %v", got, err)
	}

	workspace := filepath.Join(t.TempDir(), "workspace")
	if got, err := resolveCLIHubStart("", workspace); err != nil || got != workspace {
		t.Fatalf("resolveCLIHubStart workspace = %q, %v", got, err)
	}

	got, err := resolveCLIHubStart("", "")
	if err != nil {
		t.Fatalf("resolveCLIHubStart cwd: %v", err)
	}
	if strings.TrimSpace(got) == "" {
		t.Fatalf("expected cwd fallback, got %q", got)
	}
}

func TestSplitCLIHubExecArgs(t *testing.T) {
	flagArgs, passthrough := splitCLIHubExecArgs([]string{"shotcut", "--json=false", "--", "project", "info", "--help"})
	if !reflect.DeepEqual(flagArgs, []string{"shotcut", "--json=false"}) {
		t.Fatalf("unexpected flag args: %#v", flagArgs)
	}
	if !reflect.DeepEqual(passthrough, []string{"project", "info", "--help"}) {
		t.Fatalf("unexpected passthrough args: %#v", passthrough)
	}
}

func TestReorderFlagArgsKeepsPositionalsForCLIHubExec(t *testing.T) {
	got := reorderFlagArgs([]string{"shotcut", "--cwd", "D:\\tmp", "project", "info"}, map[string]bool{
		"--cwd": true,
	})
	want := []string{"--cwd", "D:\\tmp", "shotcut", "project", "info"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("reorderFlagArgs = %#v, want %#v", got, want)
	}
}

func containsCLIHubEntry(entries []clihub.EntryStatus, name string) bool {
	for _, entry := range entries {
		if entry.Name == name {
			return true
		}
	}
	return false
}

func writeCLIHubTestRoot(t *testing.T) string {
	t.Helper()

	root := filepath.Join(t.TempDir(), "CLI-Anything-0.2.0")
	mustWriteCLIHubFile(t, filepath.Join(root, "helper", "skills", "SKILL.md"), helperSkillMarkdown())
	mustWriteCLIHubFile(t, filepath.Join(root, "video-source", "skills", "SKILL.md"), videoSourceSkillMarkdown())
	mustWriteCLIHubFile(t, filepath.Join(root, "video-source", "agent-harness", "cli_anything", "editor", "__main__.py"), "print('video source')\n")

	registry := struct {
		Meta struct {
			Repo        string `json:"repo"`
			Description string `json:"description"`
			Updated     string `json:"updated"`
		} `json:"meta"`
		CLIs []clihub.Entry `json:"clis"`
	}{}
	registry.Meta.Repo = "test/clihub"
	registry.Meta.Description = "CLI Hub test catalog"
	registry.Meta.Updated = "2026-04-24"
	registry.CLIs = []clihub.Entry{
		{
			Name:        "helper",
			DisplayName: "Helper Tool",
			Description: "Installed helper tool",
			EntryPoint:  os.Args[0],
			SkillMD:     "helper/skills/SKILL.md",
			Category:    "automation",
		},
		{
			Name:        "video-source",
			DisplayName: "Video Source",
			Description: "Runnable source harness",
			EntryPoint:  "video-source",
			InstallCmd:  "echo installing video-source",
			SkillMD:     "video-source/skills/SKILL.md",
			Category:    "video",
		},
		{
			Name:        "catalog-only",
			DisplayName: "Catalog Only",
			Description: "Catalog entry that needs installation",
			EntryPoint:  "catalog-only",
			InstallCmd:  "echo installing catalog-only",
			Category:    "automation",
		},
	}

	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		t.Fatalf("Marshal registry: %v", err)
	}
	mustWriteCLIHubFile(t, filepath.Join(root, "registry.json"), string(data))
	return root
}

func mustWriteCLIHubFile(t *testing.T, path string, contents string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
}

func helperSkillMarkdown() string {
	return `---
name: Helper Tool
description: Helper automation commands
---

## Project Group
| Command | Description |
| --- | --- |
| list | List helper projects |
| new | Create helper project |
`
}

func videoSourceSkillMarkdown() string {
	return `---
name: Video Source
description: Video editing commands
---

## Project Group
| Command | Description |
| --- | --- |
| new | Create a new video project |
| render | Render the current timeline |
`
}
