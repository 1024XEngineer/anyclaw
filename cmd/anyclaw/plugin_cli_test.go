package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/1024XEngineer/anyclaw/pkg/config"
	"github.com/1024XEngineer/anyclaw/pkg/extensions/plugin"
)

func TestScaffoldPluginSupportsNodeKind(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir tempDir: %v", err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()

	if err := scaffoldPlugin("plugins", "demo-node", "node"); err != nil {
		t.Fatalf("scaffoldPlugin node: %v", err)
	}

	data, err := os.ReadFile(filepath.Join("plugins", "demo-node", "plugin.json"))
	if err != nil {
		t.Fatalf("ReadFile manifest: %v", err)
	}
	var manifest plugin.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("Unmarshal manifest: %v", err)
	}
	if manifest.Node == nil {
		t.Fatal("expected node spec in manifest")
	}
	if manifest.Node.Name != "demo-node" {
		t.Fatalf("unexpected node name: %q", manifest.Node.Name)
	}
	if _, err := os.Stat(filepath.Join("plugins", "demo-node", "node.py")); err != nil {
		t.Fatalf("expected node scaffold script: %v", err)
	}
}

func TestScaffoldPluginSupportsSurfaceKind(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir tempDir: %v", err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()

	if err := scaffoldPlugin("plugins", "demo-surface", "surface"); err != nil {
		t.Fatalf("scaffoldPlugin surface: %v", err)
	}

	data, err := os.ReadFile(filepath.Join("plugins", "demo-surface", "plugin.json"))
	if err != nil {
		t.Fatalf("ReadFile manifest: %v", err)
	}
	var manifest plugin.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("Unmarshal manifest: %v", err)
	}
	if manifest.Surface == nil {
		t.Fatal("expected surface spec in manifest")
	}
	if manifest.Surface.Path != "/__openclaw__/surfaces/demo-surface" {
		t.Fatalf("unexpected surface path: %q", manifest.Surface.Path)
	}
	if _, err := os.Stat(filepath.Join("plugins", "demo-surface", "openclaw.plugin.json")); err != nil {
		t.Fatalf("expected openclaw manifest: %v", err)
	}
	if _, err := os.Stat(filepath.Join("plugins", "demo-surface", "surface.py")); err != nil {
		t.Fatalf("expected surface scaffold script: %v", err)
	}
}

func TestRunPluginToggleUpdatesEnabledList(t *testing.T) {
	pluginDir := filepath.Join(t.TempDir(), "plugins")
	itemDir := filepath.Join(pluginDir, "demo-plugin")
	if err := os.MkdirAll(itemDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	manifest := plugin.Manifest{
		Name:        "demo-plugin",
		Version:     "1.0.0",
		Description: "Demo plugin",
		Kinds:       []string{"tool"},
		Enabled:     true,
		Entrypoint:  "tool.py",
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	if err := os.WriteFile(filepath.Join(itemDir, "plugin.json"), data, 0o644); err != nil {
		t.Fatalf("WriteFile manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(itemDir, "tool.py"), []byte("print('ok')\n"), 0o644); err != nil {
		t.Fatalf("WriteFile entrypoint: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Plugins.Dir = pluginDir
	cfg.Plugins.AllowExec = true
	cfg.Plugins.RequireTrust = false
	configPath := filepath.Join(t.TempDir(), "anyclaw.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save config: %v", err)
	}

	if err := runPluginCommand([]string{"enable", "--config", configPath, "demo-plugin"}); err != nil {
		t.Fatalf("runPluginCommand enable: %v", err)
	}
	updated, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Load updated config: %v", err)
	}
	if len(updated.Plugins.Enabled) != 1 || updated.Plugins.Enabled[0] != "demo-plugin" {
		t.Fatalf("unexpected enabled list after enable: %#v", updated.Plugins.Enabled)
	}

	if err := runPluginCommand([]string{"disable", "--config", configPath, "demo-plugin"}); err != nil {
		t.Fatalf("runPluginCommand disable: %v", err)
	}
	updated, err = config.Load(configPath)
	if err != nil {
		t.Fatalf("Load updated config after disable: %v", err)
	}
	if strings.Join(updated.Plugins.Enabled, ",") == "demo-plugin" {
		t.Fatalf("expected plugin to be disabled, got %#v", updated.Plugins.Enabled)
	}
}

func TestRunPluginCommandNewScaffoldsCodexManifest(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir tempDir: %v", err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()

	if err := runPluginCommand([]string{"new", "--name", "demo-tool", "--kind", "tool"}); err != nil {
		t.Fatalf("runPluginCommand new: %v", err)
	}
	if _, err := os.Stat(filepath.Join("plugins", "demo-tool", ".codex-plugin", "plugin.json")); err != nil {
		t.Fatalf("expected codex plugin manifest: %v", err)
	}
}

func TestRunPluginCommandNewUsesConfiguredPluginDir(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir tempDir: %v", err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()

	cfg := config.DefaultConfig()
	cfg.Plugins.Dir = filepath.Join("custom", "plugins")
	configPath := filepath.Join(tempDir, "configs", "anyclaw.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save config: %v", err)
	}

	if err := runPluginCommand([]string{"new", "--config", configPath, "--name", "demo-tool", "--kind", "tool"}); err != nil {
		t.Fatalf("runPluginCommand new with config: %v", err)
	}

	customDir := filepath.Join(tempDir, "configs", "custom", "plugins", "demo-tool")
	if _, err := os.Stat(filepath.Join(customDir, ".codex-plugin", "plugin.json")); err != nil {
		t.Fatalf("expected codex plugin manifest in configured dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tempDir, "plugins", "demo-tool")); !os.IsNotExist(err) {
		t.Fatalf("expected default plugins dir to remain unused, got %v", err)
	}
}

func TestRunPluginCommandNewRejectsPathTraversalName(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir tempDir: %v", err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()

	err = runPluginCommand([]string{"new", "--name", "../escape", "--kind", "tool"})
	if err == nil || !strings.Contains(err.Error(), "path separators") {
		t.Fatalf("expected path separator validation error, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(tempDir, "escape")); !os.IsNotExist(statErr) {
		t.Fatalf("expected traversal target to stay absent, got %v", statErr)
	}
}

func TestRunPluginCommandNewNormalizesSafeSlug(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir tempDir: %v", err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()

	if err := runPluginCommand([]string{"new", "--name", "Demo Tool", "--kind", "tool"}); err != nil {
		t.Fatalf("runPluginCommand new normalized: %v", err)
	}
	data, err := os.ReadFile(filepath.Join("plugins", "demo-tool", "plugin.json"))
	if err != nil {
		t.Fatalf("ReadFile manifest: %v", err)
	}
	var manifest plugin.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("Unmarshal manifest: %v", err)
	}
	if manifest.Name != "demo-tool" {
		t.Fatalf("expected normalized manifest name, got %q", manifest.Name)
	}
}
