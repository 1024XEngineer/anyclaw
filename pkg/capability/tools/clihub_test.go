package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anyclaw/anyclaw/pkg/clihub"
)

func TestRegisterCLIHubToolsRegistersCatalogTool(t *testing.T) {
	hubRoot := filepath.Join(t.TempDir(), "CLI-Anything-0.2.0")
	if err := writeCLIHubToolFixture(hubRoot); err != nil {
		t.Fatalf("writeCLIHubToolFixture: %v", err)
	}
	t.Setenv("ANYCLAW_CLI_ANYTHING_ROOT", hubRoot)

	registry := NewRegistry()
	RegisterCLIHubTools(registry, BuiltinOptions{WorkingDir: t.TempDir(), ExecutionMode: "host-reviewed"})

	tool, ok := registry.Get("clihub_catalog")
	if !ok {
		t.Fatalf("expected clihub_catalog to be registered")
	}
	result, err := tool.Handler(context.Background(), map[string]any{"query": "office", "limit": 5})
	if err != nil {
		t.Fatalf("tool.Handler: %v", err)
	}
	if !strings.Contains(result, "\"count\": 2") || !strings.Contains(result, "\"libreoffice\"") {
		t.Fatalf("unexpected catalog output: %s", result)
	}
}

func TestBuildCLIHubCommandInjectsJSONAndUsesDevModuleFallback(t *testing.T) {
	status := clihub.EntryStatus{
		Entry: clihub.Entry{
			Name:       "shotcut",
			InstallCmd: "pip install ...",
		},
		SourcePath: `D:\tmp\shotcut\agent-harness`,
		DevModule:  "cli_anything.shotcut",
	}

	command, cwd, shellName, err := buildCLIHubCommand(status, []string{"project", "info"}, true, "", BuiltinOptions{
		ExecutionMode: "host-reviewed",
		WorkingDir:    `D:\tmp`,
	}, CLIHubExecOptions{
		PreferLocalSrc: true,
	})
	if err != nil {
		t.Fatalf("buildCLIHubCommand: %v", err)
	}
	if cwd != status.SourcePath {
		t.Fatalf("expected cwd %q, got %q", status.SourcePath, cwd)
	}
	if shellName == "" {
		t.Fatalf("expected shell name to be set")
	}
	if !strings.Contains(command, "--json") || !strings.Contains(command, "cli_anything.shotcut") || !(strings.Contains(command, "python") || strings.Contains(command, "py")) {
		t.Fatalf("unexpected command: %s", command)
	}
}

func TestBuildCLIHubCommandRequiresInstallWhenNoExecutableOrDevModule(t *testing.T) {
	_, _, _, err := buildCLIHubCommand(clihub.EntryStatus{
		Entry: clihub.Entry{
			Name:       "zotero",
			InstallCmd: "pip install example",
		},
	}, []string{"library", "list"}, true, "", BuiltinOptions{
		ExecutionMode: "host-reviewed",
		WorkingDir:    `D:\tmp`,
	}, CLIHubExecOptions{
		AutoInstall: false,
	})
	if err == nil || !strings.Contains(err.Error(), "install it first") {
		t.Fatalf("expected install hint error, got %v", err)
	}
}

func writeCLIHubToolFixture(root string) error {
	if err := os.MkdirAll(filepath.Join(root, "shotcut", "agent-harness", "cli_anything", "shotcut"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "shotcut", "agent-harness", "cli_anything", "shotcut", "__main__.py"), []byte("print('ok')"), 0o644); err != nil {
		return err
	}
	payload := map[string]any{
		"meta": map[string]any{
			"repo":        "https://example.com/CLI-Anything",
			"description": "CLI-Hub",
			"updated":     "2026-03-29",
		},
		"clis": []map[string]any{
			{"name": "libreoffice", "display_name": "LibreOffice", "description": "Office suite", "category": "office", "entry_point": "cli-anything-libreoffice"},
			{"name": "zotero", "display_name": "Zotero", "description": "References", "category": "office", "entry_point": "cli-anything-zotero"},
			{"name": "shotcut", "display_name": "Shotcut", "description": "Video editing", "category": "video", "entry_point": "cli-anything-shotcut"},
		},
	}
	return writeCLIHubToolJSON(filepath.Join(root, "registry.json"), payload)
}

func writeCLIHubToolJSON(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
