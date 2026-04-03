package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anyclaw/anyclaw/pkg/apps"
	"github.com/anyclaw/anyclaw/pkg/config"
	"github.com/anyclaw/anyclaw/pkg/plugin"
)

func TestHandleAppsListsAppPlugins(t *testing.T) {
	baseDir := t.TempDir()
	pluginDir := filepath.Join(baseDir, "demo-app")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	manifest := plugin.Manifest{
		Name:        "demo-app",
		Version:     "1.0.0",
		Description: "Demo app connector",
		Kinds:       []string{"app"},
		Enabled:     true,
		Entrypoint:  "app.py",
		Permissions: []string{"tool:exec"},
		App: &plugin.AppSpec{
			Name:         "Demo App",
			Description:  "Demo app connector",
			Transport:    "desktop",
			Platforms:    []string{"windows"},
			Capabilities: []string{"desktop-control"},
			Desktop: &apps.DesktopSpec{
				LaunchCommand: "demo.exe",
				WindowTitle:   "Demo Window",
			},
			Actions: []plugin.AppActionSpec{
				{Name: "run-task", Description: "Run a task", Kind: "execute"},
			},
			Workflows: []plugin.AppWorkflowSpec{
				{
					Name:        "quick-note",
					Description: "Draft a quick note",
					Action:      "run-task",
					Tags:        []string{"drafting"},
					Defaults:    map[string]any{"task": "Draft a note"},
				},
			},
		},
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), data, 0o644); err != nil {
		t.Fatalf("WriteFile manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "app.py"), []byte("print('ok')\n"), 0o644); err != nil {
		t.Fatalf("WriteFile entrypoint: %v", err)
	}

	plugins, err := plugin.NewRegistry(config.PluginsConfig{
		Dir:          baseDir,
		AllowExec:    true,
		RequireTrust: false,
	})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	store, err := NewStore(baseDir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	server := &Server{plugins: plugins, store: store}
	req := httptest.NewRequest(http.MethodGet, "/apps", nil)
	req = req.WithContext(context.WithValue(req.Context(), authUserKey, &AuthUser{
		Name:        "viewer",
		Permissions: []string{"plugins.read"},
	}))
	rec := httptest.NewRecorder()

	server.handleApps(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload []appView
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(payload) != 1 {
		t.Fatalf("expected 1 app, got %d", len(payload))
	}
	if payload[0].Plugin != "demo-app" {
		t.Fatalf("unexpected plugin name: %q", payload[0].Plugin)
	}
	if len(payload[0].Actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(payload[0].Actions))
	}
	if payload[0].Transport != "desktop" {
		t.Fatalf("expected desktop transport, got %q", payload[0].Transport)
	}
	if payload[0].Desktop == nil || payload[0].Desktop.LaunchCommand != "demo.exe" {
		t.Fatalf("expected desktop metadata to round-trip")
	}
	if payload[0].Actions[0].Kind != "execute" {
		t.Fatalf("expected action kind execute, got %q", payload[0].Actions[0].Kind)
	}
	if len(payload[0].Workflows) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(payload[0].Workflows))
	}
	if payload[0].Workflows[0].Action != "run-task" {
		t.Fatalf("expected workflow action run-task, got %q", payload[0].Workflows[0].Action)
	}
	if payload[0].Workflows[0].ToolName != plugin.AppWorkflowToolName("demo-app", "quick-note") {
		t.Fatalf("unexpected workflow tool name: %q", payload[0].Workflows[0].ToolName)
	}
	expectedTool := plugin.AppActionToolName("demo-app", "run-task")
	if strings.TrimSpace(payload[0].Actions[0].ToolName) != expectedTool {
		t.Fatalf("expected tool name %q, got %q", expectedTool, payload[0].Actions[0].ToolName)
	}
}
