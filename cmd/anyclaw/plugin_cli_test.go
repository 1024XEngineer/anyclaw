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

func TestScaffoldPluginSupportsIngressAndChannelKinds(t *testing.T) {
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

	if err := scaffoldPlugin("plugins", "demo-ingress", "ingress"); err != nil {
		t.Fatalf("scaffoldPlugin ingress: %v", err)
	}
	if err := scaffoldPlugin("plugins", "demo-channel", "channel"); err != nil {
		t.Fatalf("scaffoldPlugin channel: %v", err)
	}

	ingressData, err := os.ReadFile(filepath.Join("plugins", "demo-ingress", "plugin.json"))
	if err != nil {
		t.Fatalf("ReadFile ingress manifest: %v", err)
	}
	channelData, err := os.ReadFile(filepath.Join("plugins", "demo-channel", "plugin.json"))
	if err != nil {
		t.Fatalf("ReadFile channel manifest: %v", err)
	}

	var ingressManifest plugin.Manifest
	if err := json.Unmarshal(ingressData, &ingressManifest); err != nil {
		t.Fatalf("Unmarshal ingress manifest: %v", err)
	}
	if ingressManifest.Ingress == nil || ingressManifest.Ingress.Path != "/ingress/plugins/demo-ingress" {
		t.Fatalf("unexpected ingress manifest: %#v", ingressManifest.Ingress)
	}

	var channelManifest plugin.Manifest
	if err := json.Unmarshal(channelData, &channelManifest); err != nil {
		t.Fatalf("Unmarshal channel manifest: %v", err)
	}
	if channelManifest.Channel == nil || len(channelManifest.Permissions) != 2 {
		t.Fatalf("unexpected channel manifest: %#v", channelManifest)
	}
	if _, err := os.Stat(filepath.Join("plugins", "demo-ingress", "ingress.py")); err != nil {
		t.Fatalf("expected ingress scaffold script: %v", err)
	}
	if _, err := os.Stat(filepath.Join("plugins", "demo-channel", "channel.py")); err != nil {
		t.Fatalf("expected channel scaffold script: %v", err)
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

func TestRunPluginNewRequiresName(t *testing.T) {
	if err := runPluginNew([]string{"--kind", "tool"}); err == nil || !strings.Contains(err.Error(), "--name is required") {
		t.Fatalf("expected missing name error, got %v", err)
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

func TestRunPluginCommandNewRejectsUnsupportedAppKind(t *testing.T) {
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

	err = runPluginCommand([]string{"new", "--name", "demo-app", "--kind", "app"})
	if err == nil || !strings.Contains(err.Error(), "unsupported plugin kind: app") {
		t.Fatalf("expected unsupported app kind error, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(tempDir, "plugins", "demo-app")); !os.IsNotExist(statErr) {
		t.Fatalf("expected unsupported kind to avoid scaffolding files, got %v", statErr)
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

func TestRunPluginCommandWithoutArgsPrintsUsage(t *testing.T) {
	stdout, _, err := captureCLIOutput(t, func() error {
		return runPluginCommand(nil)
	})
	if err != nil {
		t.Fatalf("runPluginCommand: %v", err)
	}
	if !strings.Contains(stdout, "AnyClaw plugin commands:") {
		t.Fatalf("expected plugin usage output, got %q", stdout)
	}
}

func TestRunPluginCommandUnknownSubcommandPrintsUsage(t *testing.T) {
	stdout, _, err := captureCLIOutput(t, func() error {
		return runPluginCommand([]string{"unknown"})
	})
	if err == nil || !strings.Contains(err.Error(), "unknown plugin command") {
		t.Fatalf("expected unknown plugin command error, got %v", err)
	}
	if !strings.Contains(stdout, "AnyClaw plugin commands:") {
		t.Fatalf("expected plugin usage output, got %q", stdout)
	}
}

func TestRunPluginListHandlesEmptyPluginDir(t *testing.T) {
	configPath, _ := writePluginCLIConfigWithRoot(t, func(cfg *config.Config, pluginRoot string) {
		cfg.Plugins.Dir = pluginRoot
	})

	stdout, _, err := captureCLIOutput(t, func() error {
		return runPluginList([]string{"--config", configPath})
	})
	if err != nil {
		t.Fatalf("runPluginList: %v", err)
	}
	if !strings.Contains(stdout, "No local plugins found.") {
		t.Fatalf("expected empty plugins message, got %q", stdout)
	}
}

func TestRunPluginListOutputsTextAndJSON(t *testing.T) {
	configPath, pluginRoot := writePluginCLIConfigWithRoot(t, func(cfg *config.Config, pluginRoot string) {
		cfg.Plugins.Dir = pluginRoot
		cfg.Plugins.RequireTrust = true
	})
	writePluginFixture(t, pluginRoot, plugin.Manifest{
		Name:        "demo-surface",
		Version:     "1.2.3",
		Description: "Demo surface plugin",
		Kinds:       []string{"surface"},
		Enabled:     false,
		Entrypoint:  "surface.py",
		Surface: &plugin.SurfaceSpec{
			Name:         "demo-surface",
			Description:  "Demo surface plugin",
			Path:         "/__openclaw__/surfaces/demo-surface",
			Capabilities: []string{"html"},
		},
	}, "print('surface')\n")

	stdout, _, err := captureCLIOutput(t, func() error {
		return runPluginList([]string{"--config", configPath})
	})
	if err != nil {
		t.Fatalf("runPluginList text: %v", err)
	}
	if !strings.Contains(stdout, "- demo-surface [surface] disabled unverified /__openclaw__/surfaces/demo-surface") {
		t.Fatalf("unexpected text output: %q", stdout)
	}

	stdout, _, err = captureCLIOutput(t, func() error {
		return runPluginList([]string{"--config", configPath, "--json"})
	})
	if err != nil {
		t.Fatalf("runPluginList json: %v", err)
	}
	var items []plugin.Manifest
	if err := json.Unmarshal([]byte(stdout), &items); err != nil {
		t.Fatalf("unmarshal json output: %v\nstdout=%s", err, stdout)
	}
	if len(items) != 1 || items[0].Name != "demo-surface" {
		t.Fatalf("unexpected json output: %#v", items)
	}
}

func TestRunPluginInfoPrintsManifestDetails(t *testing.T) {
	configPath, pluginRoot := writePluginCLIConfigWithRoot(t, func(cfg *config.Config, pluginRoot string) {
		cfg.Plugins.Dir = pluginRoot
		cfg.Plugins.RequireTrust = true
	})
	writePluginFixture(t, pluginRoot, plugin.Manifest{
		Name:        "demo-surface",
		Version:     "1.2.3",
		Description: "Demo surface plugin",
		Kinds:       []string{"surface"},
		Enabled:     false,
		Entrypoint:  "surface.py",
		Surface: &plugin.SurfaceSpec{
			Name:         "demo-surface",
			Description:  "Demo surface plugin",
			Path:         "/__openclaw__/surfaces/demo-surface",
			Capabilities: []string{"html"},
		},
	}, "print('surface')\n")

	stdout, _, err := captureCLIOutput(t, func() error {
		return runPluginInfo([]string{"--config", configPath, "demo-surface"})
	})
	if err != nil {
		t.Fatalf("runPluginInfo: %v", err)
	}
	for _, want := range []string{
		"demo-surface",
		"version: 1.2.3",
		"enabled: false",
		"kinds:   surface",
		"desc:    Demo surface plugin",
		"entry:   surface.py",
		"trust:   unverified",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected %q in output, got %q", want, stdout)
		}
	}
}

func TestRunPluginCommandRoutesListInfoAndDoctor(t *testing.T) {
	configPath, pluginRoot := writePluginCLIConfigWithRoot(t, func(cfg *config.Config, pluginRoot string) {
		cfg.Plugins.Dir = pluginRoot
		cfg.Plugins.AllowExec = true
		cfg.Plugins.RequireTrust = false
	})
	writePluginFixture(t, pluginRoot, plugin.Manifest{
		Name:        "demo-tool",
		Version:     "1.0.0",
		Description: "Demo tool plugin",
		Kinds:       []string{"tool"},
		Enabled:     true,
		Entrypoint:  "tool.py",
		Tool: &plugin.ToolSpec{
			Name:        "demo_tool",
			Description: "Demo tool",
			InputSchema: map[string]any{"type": "object"},
		},
	}, "print('ok')\n")

	stdout, _, err := captureCLIOutput(t, func() error {
		return runPluginCommand([]string{"list", "--config", configPath})
	})
	if err != nil || !strings.Contains(stdout, "demo-tool") {
		t.Fatalf("expected list route to succeed, err=%v stdout=%q", err, stdout)
	}

	stdout, _, err = captureCLIOutput(t, func() error {
		return runPluginCommand([]string{"info", "--config", configPath, "demo-tool"})
	})
	if err != nil || !strings.Contains(stdout, "Demo tool plugin") {
		t.Fatalf("expected info route to succeed, err=%v stdout=%q", err, stdout)
	}

	stdout, _, err = captureCLIOutput(t, func() error {
		return runPluginCommand([]string{"doctor", "--config", configPath})
	})
	if err != nil || !strings.Contains(stdout, "No plugin issues detected") {
		t.Fatalf("expected doctor route to succeed, err=%v stdout=%q", err, stdout)
	}
}

func TestRunPluginInfoAndManifestHelpersHandleMissingPlugin(t *testing.T) {
	configPath, pluginRoot := writePluginCLIConfigWithRoot(t, func(cfg *config.Config, pluginRoot string) {
		cfg.Plugins.Dir = pluginRoot
	})
	writePluginFixture(t, pluginRoot, plugin.Manifest{
		Name:    "demo-tool",
		Version: "1.0.0",
		Kinds:   []string{"tool"},
		Enabled: true,
		Tool: &plugin.ToolSpec{
			Name:        "demo_tool",
			Description: "Demo tool",
			InputSchema: map[string]any{"type": "object"},
		},
	}, "")

	if _, _, err := findPluginManifest(configPath, "missing"); err == nil || !strings.Contains(err.Error(), "plugin not found") {
		t.Fatalf("expected missing plugin error, got %v", err)
	}
	if _, _, err := captureCLIOutput(t, func() error {
		return runPluginInfo([]string{})
	}); err == nil || !strings.Contains(err.Error(), "usage: anyclaw plugin info <name>") {
		t.Fatalf("expected plugin info usage error, got %v", err)
	}
	if _, _, err := captureCLIOutput(t, func() error {
		return runPluginInfo([]string{"--config", configPath, "missing"})
	}); err == nil || !strings.Contains(err.Error(), "plugin not found") {
		t.Fatalf("expected plugin info missing plugin error, got %v", err)
	}
}

func TestRunPluginDoctorReportsIssuesAndJSON(t *testing.T) {
	configPath, pluginRoot := writePluginCLIConfigWithRoot(t, func(cfg *config.Config, pluginRoot string) {
		cfg.Plugins.Dir = pluginRoot
		cfg.Plugins.AllowExec = false
		cfg.Plugins.RequireTrust = true
	})
	writePluginFixture(t, pluginRoot, plugin.Manifest{
		Name:        "broken-plugin",
		Version:     "1.0.0",
		Description: "Broken plugin",
		Enabled:     true,
		Entrypoint:  "tool.py",
	}, "print('broken')\n")

	stdout, _, err := captureCLIOutput(t, func() error {
		return runPluginDoctor([]string{"--config", configPath, "--json"})
	})
	if err != nil {
		t.Fatalf("runPluginDoctor json: %v", err)
	}
	var payload struct {
		Count  int                 `json:"count"`
		Issues []pluginDoctorIssue `json:"issues"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("unmarshal doctor json: %v\nstdout=%s", err, stdout)
	}
	if payload.Count != 3 || len(payload.Issues) != 3 {
		t.Fatalf("expected 3 doctor issues, got %#v", payload)
	}
}

func TestRunPluginDoctorNoIssuesPrintsSuccess(t *testing.T) {
	configPath, pluginRoot := writePluginCLIConfigWithRoot(t, func(cfg *config.Config, pluginRoot string) {
		cfg.Plugins.Dir = pluginRoot
		cfg.Plugins.AllowExec = true
		cfg.Plugins.RequireTrust = false
	})
	writePluginFixture(t, pluginRoot, plugin.Manifest{
		Name:        "healthy-plugin",
		Version:     "1.0.0",
		Description: "Healthy plugin",
		Kinds:       []string{"tool"},
		Enabled:     true,
		Entrypoint:  "tool.py",
		Tool: &plugin.ToolSpec{
			Name:        "healthy_tool",
			Description: "Healthy tool",
			InputSchema: map[string]any{"type": "object"},
		},
	}, "print('ok')\n")

	stdout, _, err := captureCLIOutput(t, func() error {
		return runPluginDoctor([]string{"--config", configPath})
	})
	if err != nil {
		t.Fatalf("runPluginDoctor: %v", err)
	}
	if !strings.Contains(stdout, "No plugin issues detected") {
		t.Fatalf("expected success output, got %q", stdout)
	}
}

func TestRunPluginDoctorTextOutputIncludesIssues(t *testing.T) {
	configPath, pluginRoot := writePluginCLIConfigWithRoot(t, func(cfg *config.Config, pluginRoot string) {
		cfg.Plugins.Dir = pluginRoot
		cfg.Plugins.AllowExec = false
		cfg.Plugins.RequireTrust = true
	})
	writePluginFixture(t, pluginRoot, plugin.Manifest{
		Name:        "warning-plugin",
		Version:     "1.0.0",
		Description: "Warning plugin",
		Enabled:     true,
		Entrypoint:  "tool.py",
	}, "print('warn')\n")

	stdout, _, err := captureCLIOutput(t, func() error {
		return runPluginDoctor([]string{"--config", configPath})
	})
	if err != nil {
		t.Fatalf("runPluginDoctor text: %v", err)
	}
	if !strings.Contains(stdout, "Plugin issues: 3") || !strings.Contains(stdout, "plugin manifest does not declare any kinds") {
		t.Fatalf("expected issue summary output, got %q", stdout)
	}
}

func TestPluginCLIHelpers(t *testing.T) {
	if got, err := normalizePluginName("."); err == nil || !strings.Contains(err.Error(), "must not be") || got != "" {
		t.Fatalf("expected dot name rejection, got value=%q err=%v", got, err)
	}
	if got := scriptNameForKind("tool"); got != "tool.py" {
		t.Fatalf("unexpected tool script name: %q", got)
	}
	if got := scriptNameForKind("unknown"); got != "plugin.py" {
		t.Fatalf("unexpected default script name: %q", got)
	}
	if script := pluginScript("tool"); !strings.Contains(script, "tool received") {
		t.Fatalf("unexpected tool script: %q", script)
	}
	if script := pluginScript("unknown"); !strings.Contains(script, "plugin scaffold") {
		t.Fatalf("unexpected default script: %q", script)
	}
	if trust := firstNonEmptyPluginTrust(plugin.Manifest{Trust: "manual"}); trust != "manual" {
		t.Fatalf("unexpected explicit trust: %q", trust)
	}
	if trust := firstNonEmptyPluginTrust(plugin.Manifest{Verified: true}); trust != "verified" {
		t.Fatalf("unexpected verified trust: %q", trust)
	}
	if trust := firstNonEmptyPluginTrust(plugin.Manifest{}); trust != "unverified" {
		t.Fatalf("unexpected fallback trust: %q", trust)
	}
	if got := sortedEnabledPluginNames(map[string]bool{"zeta": true, "Alpha": true, " ": true}); strings.Join(got, ",") != "Alpha,zeta" {
		t.Fatalf("unexpected sorted enabled list: %#v", got)
	}
	manifest, ok := findPluginByName([]plugin.Manifest{{Name: "Demo-Tool"}}, "demo-tool")
	if !ok || manifest.Name != "Demo-Tool" {
		t.Fatalf("expected case-insensitive manifest lookup, got manifest=%#v ok=%v", manifest, ok)
	}
}

func writePluginCLIConfigWithRoot(t *testing.T, mutate func(*config.Config, string)) (string, string) {
	t.Helper()

	baseDir := t.TempDir()
	pluginRoot := filepath.Join(baseDir, "plugins")
	cfg := config.DefaultConfig()
	if mutate != nil {
		mutate(cfg, pluginRoot)
	}
	configPath := filepath.Join(baseDir, "config", "anyclaw.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("save config: %v", err)
	}
	return configPath, pluginRoot
}

func writePluginFixture(t *testing.T, pluginRoot string, manifest plugin.Manifest, entrypointContents string) {
	t.Helper()

	pluginDir := filepath.Join(pluginRoot, manifest.Name)
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("mkdir plugin dir: %v", err)
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), data, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if strings.TrimSpace(manifest.Entrypoint) != "" {
		if err := os.WriteFile(filepath.Join(pluginDir, manifest.Entrypoint), []byte(entrypointContents), 0o644); err != nil {
			t.Fatalf("write entrypoint: %v", err)
		}
	}
}
