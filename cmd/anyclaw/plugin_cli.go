package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/anyclaw/anyclaw/pkg/config"
	"github.com/anyclaw/anyclaw/pkg/plugin"
	"github.com/anyclaw/anyclaw/pkg/ui"
)

func runPluginCommand(args []string) error {
	if len(args) == 0 {
		printPluginUsage()
		return nil
	}
	switch args[0] {
	case "new":
		return runPluginNew(args[1:])
	case "list":
		return runPluginList(args[1:])
	case "info":
		return runPluginInfo(args[1:])
	case "enable":
		return runPluginToggle(args[1:], true)
	case "disable":
		return runPluginToggle(args[1:], false)
	case "doctor":
		return runPluginDoctor(args[1:])
	default:
		printPluginUsage()
		return fmt.Errorf("unknown plugin command: %s", args[0])
	}

}

func runPluginNew(args []string) error {
	fs := flag.NewFlagSet("plugin new", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	kind := fs.String("kind", "tool", "plugin kind: tool|ingress|channel|app|node|surface")
	name := fs.String("name", "", "plugin name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*name) == "" {
		return fmt.Errorf("--name is required")
	}
	return scaffoldPlugin(*name, *kind)
}

type pluginDoctorIssue struct {
	Plugin   string `json:"plugin"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

func printPluginUsage() {
	fmt.Print(`AnyClaw plugin commands:

Usage:
  anyclaw plugin new --name my-plugin --kind tool
  anyclaw plugin new --name my-ingress --kind ingress
  anyclaw plugin new --name my-channel --kind channel
  anyclaw plugin new --name my-app --kind app
  anyclaw plugin new --name my-node --kind node
  anyclaw plugin new --name my-surface --kind surface
  anyclaw plugin list
  anyclaw plugin info <name>
  anyclaw plugin enable <name>
  anyclaw plugin disable <name>
  anyclaw plugin doctor
`)
}

func scaffoldPlugin(name string, kind string) error {
	pluginDir := filepath.Join("plugins", name)
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		return err
	}
	manifest := map[string]any{
		"name":            name,
		"version":         "1.0.0",
		"description":     "Scaffolded plugin",
		"kinds":           []string{kind},
		"enabled":         true,
		"entrypoint":      scriptNameForKind(kind),
		"exec_policy":     "manual-allow",
		"permissions":     []string{"tool:exec"},
		"timeout_seconds": 5,
		"signer":          "dev-local",
		"signature":       "sha256:replace-after-build",
	}
	switch kind {
	case "tool":
		manifest["tool"] = map[string]any{
			"name":        strings.ReplaceAll(name, "-", "_"),
			"description": "Example tool plugin",
			"input_schema": map[string]any{
				"type":       "object",
				"properties": map[string]any{"query": map[string]any{"type": "string"}},
				"required":   []string{"query"},
			},
		}
	case "ingress":
		manifest["ingress"] = map[string]any{
			"name":        name,
			"path":        "/ingress/plugins/" + name,
			"description": "Example ingress plugin",
		}
	case "channel":
		manifest["channel"] = map[string]any{
			"name":        name,
			"description": "Example channel plugin",
		}
		manifest["permissions"] = []string{"tool:exec", "net:out"}
	case "app":
		manifest["app"] = map[string]any{
			"name":         name,
			"description":  "Example desktop app connector",
			"transport":    "desktop",
			"platforms":    []string{"windows"},
			"capabilities": []string{"desktop-control", "typing", "hotkeys", "screenshots", "vision", "ocr"},
			"desktop": map[string]any{
				"launch_command":         "notepad.exe",
				"window_title":           "Notepad",
				"focus_strategy":         "title-contains",
				"detection_hints":        []string{"Untitled - Notepad"},
				"requires_host_reviewed": true,
			},
			"actions": []map[string]any{
				{
					"name":        "run",
					"description": "Run the desktop app action",
					"kind":        "execute",
					"input_schema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"task": map[string]any{"type": "string"},
						},
						"required": []string{"task"},
					},
				},
			},
			"workflows": []map[string]any{
				{
					"name":        "draft-note",
					"description": "Launch the app and draft a quick note",
					"action":      "run",
					"tags":        []string{"drafting", "text"},
					"defaults": map[string]any{
						"task": "Draft a short note in the app window.",
					},
				},
			},
		}
	case "node":
		manifest["node"] = map[string]any{
			"name":         name,
			"description":  "Example node extension for host/device actions",
			"platforms":    []string{"ios", "android"},
			"capabilities": []string{"camera", "device-actions", "notifications"},
			"actions": []map[string]any{
				{
					"name":        "capture-status",
					"description": "Capture a lightweight node status snapshot",
					"input_schema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"label": map[string]any{"type": "string"},
						},
					},
				},
			},
		}
	case "surface":
		manifest["surface"] = map[string]any{
			"name":         name,
			"description":  "Example OpenClaw-style web surface extension",
			"path":         "/__openclaw__/surfaces/" + name,
			"capabilities": []string{"html", "css", "js", "dashboard"},
		}
		manifest["permissions"] = []string{"tool:exec", "fs:read"}
	default:
		return fmt.Errorf("unsupported plugin kind: %s", kind)
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "openclaw.plugin.json"), data, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), data, 0o644); err != nil {
		return err
	}
	codexPluginDir := filepath.Join(pluginDir, ".codex-plugin")
	if err := os.MkdirAll(codexPluginDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(codexPluginDir, "plugin.json"), data, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(pluginDir, scriptNameForKind(kind)), []byte(pluginScript(kind)), 0o644); err != nil {
		return err
	}
	printSuccess("Scaffolded %s plugin at %s", kind, pluginDir)
	printInfo("Next: implement the script, compute sha256, update plugin.json, and enable exec if needed")
	return nil
}

func scriptNameForKind(kind string) string {
	switch kind {
	case "tool":
		return "tool.py"
	case "ingress":
		return "ingress.py"
	case "channel":
		return "channel.py"
	case "app":
		return "app.py"
	case "node":
		return "node.py"
	case "surface":
		return "surface.py"
	default:
		return "plugin.py"
	}
}

func pluginScript(kind string) string {
	switch kind {
	case "tool":
		return "import json, os\ninput_data = json.loads(os.environ.get('ANYCLAW_PLUGIN_INPUT', '{}'))\nprint(f\"tool received: {input_data}\")\n"
	case "ingress":
		return "import json, os\ninput_data = json.loads(os.environ.get('ANYCLAW_PLUGIN_INPUT', '{}'))\nprint(json.dumps({'ok': True, 'received': input_data}))\n"
	case "channel":
		return "import json\nprint(json.dumps([{'source': 'example-user', 'message': 'hello from channel plugin'}]))\n"
	case "app":
		return "import json, os\npayload = json.loads(os.environ.get('ANYCLAW_PLUGIN_INPUT', '{}'))\nprint(json.dumps({'protocol': 'anyclaw.app.desktop.v1', 'summary': 'Example desktop connector plan executed.', 'steps': [{'tool': 'desktop_open', 'label': 'Launch app', 'input': {'target': 'notepad.exe', 'kind': 'app'}, 'retry': 1, 'wait_after_ms': 400}, {'label': 'Focus editor window', 'target': {'title': 'Notepad'}, 'action': 'focus', 'retry': 2, 'retry_delay_ms': 300, 'continue_on_error': True}, {'label': 'Type task text', 'target': {'title': 'Notepad', 'control_type': 'document'}, 'value': payload.get('input', {}).get('task', 'hello from AnyClaw')}] }))\n"
	case "node":
		return "import json, os\npayload = json.loads(os.environ.get('ANYCLAW_PLUGIN_INPUT', '{}'))\nprint(json.dumps({'ok': True, 'node': 'example', 'action': 'capture-status', 'received': payload}))\n"
	case "surface":
		return "import json\nprint(json.dumps({'ok': True, 'surface': 'example', 'path': '/__openclaw__/surfaces/example'}))\n"
	default:
		return "print('plugin scaffold')\n"
	}
}

func runPluginList(args []string) error {
	fs := flag.NewFlagSet("plugin list", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	jsonOut := fs.Bool("json", false, "output JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, registry, err := loadPluginRegistry(*configPath)
	if err != nil {
		if os.IsNotExist(err) {
			printInfo("No plugins directory found.")
			return nil
		}
		return err
	}
	items := make([]plugin.Manifest, 0)
	for _, manifest := range registry.List() {
		if manifest.Builtin {
			continue
		}
		items = append(items, manifest)
	}
	if *jsonOut {
		return writePrettyJSON(items)
	}
	if len(items) == 0 {
		printInfo("No local plugins found.")
		return nil
	}
	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	for _, manifest := range items {
		line := fmt.Sprintf("- %s", manifest.Name)
		if len(manifest.Kinds) > 0 {
			line += " [" + strings.Join(manifest.Kinds, ", ") + "]"
		}
		if manifest.Enabled {
			line += " enabled"
		} else {
			line += " disabled"
		}
		if manifest.Verified {
			line += " verified"
		} else if cfg.Plugins.RequireTrust && manifest.Entrypoint != "" {
			line += " unverified"
		}
		if manifest.Surface != nil && strings.TrimSpace(manifest.Surface.Path) != "" {
			line += " " + manifest.Surface.Path
		}
		fmt.Println(line)
	}
	return nil
}

func runPluginInfo(args []string) error {
	fs := flag.NewFlagSet("plugin info", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("usage: anyclaw plugin info <name>")
	}

	manifest, cfg, err := findPluginManifest(*configPath, fs.Arg(0))
	if err != nil {
		return err
	}

	fmt.Println(ui.Bold.Sprint(manifest.Name))
	fmt.Println()
	fmt.Printf("  version: %s\n", manifest.Version)
	fmt.Printf("  enabled: %v\n", manifest.Enabled)
	fmt.Printf("  kinds:   %s\n", strings.Join(manifest.Kinds, ", "))
	if strings.TrimSpace(manifest.Description) != "" {
		fmt.Printf("  desc:    %s\n", manifest.Description)
	}
	if strings.TrimSpace(manifest.Entrypoint) != "" {
		fmt.Printf("  entry:   %s\n", manifest.Entrypoint)
	}
	if cfg.Plugins.RequireTrust && strings.TrimSpace(manifest.Entrypoint) != "" {
		fmt.Printf("  trust:   %s\n", firstNonEmptyPluginTrust(manifest))
	}
	return nil
}

func runPluginToggle(args []string, enabled bool) error {
	name := "plugin enable"
	if !enabled {
		name = "plugin disable"
	}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("usage: anyclaw %s <name>", name)
	}

	cfg, registry, err := loadPluginRegistry(*configPath)
	if err != nil {
		return err
	}

	manifest, ok := findPluginByName(registry.List(), fs.Arg(0))
	if !ok || manifest.Builtin {
		return fmt.Errorf("plugin not found: %s", fs.Arg(0))
	}

	enabledSet := currentPluginEnabledSet(cfg, registry.List())
	if enabled {
		enabledSet[manifest.Name] = true
	} else {
		delete(enabledSet, manifest.Name)
	}

	cfg.Plugins.Enabled = sortedEnabledPluginNames(enabledSet)
	if err := cfg.Save(*configPath); err != nil {
		return err
	}
	if enabled {
		printSuccess("Enabled plugin: %s", manifest.Name)
	} else {
		printSuccess("Disabled plugin: %s", manifest.Name)
	}
	return nil
}

func runPluginDoctor(args []string) error {
	fs := flag.NewFlagSet("plugin doctor", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	jsonOut := fs.Bool("json", false, "output JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, registry, err := loadPluginRegistry(*configPath)
	if err != nil {
		return err
	}

	issues := make([]pluginDoctorIssue, 0)
	for _, manifest := range registry.List() {
		if manifest.Builtin {
			continue
		}
		if len(manifest.Kinds) == 0 {
			issues = append(issues, pluginDoctorIssue{
				Plugin:   manifest.Name,
				Severity: "error",
				Message:  "plugin manifest does not declare any kinds",
			})
		}
		if strings.TrimSpace(manifest.Entrypoint) != "" && !cfg.Plugins.AllowExec {
			issues = append(issues, pluginDoctorIssue{
				Plugin:   manifest.Name,
				Severity: "warning",
				Message:  "plugin has an entrypoint but plugins.allow_exec is false",
			})
		}
		if strings.TrimSpace(manifest.Entrypoint) != "" && cfg.Plugins.RequireTrust && !manifest.Verified {
			issues = append(issues, pluginDoctorIssue{
				Plugin:   manifest.Name,
				Severity: "warning",
				Message:  "plugin is not verified but plugins.require_trust is enabled",
			})
		}
	}

	if *jsonOut {
		return writePrettyJSON(map[string]any{
			"count":  len(issues),
			"issues": issues,
		})
	}

	if len(issues) == 0 {
		printSuccess("No plugin issues detected")
		return nil
	}
	printSuccess("Plugin issues: %d", len(issues))
	for _, issue := range issues {
		fmt.Printf("  - %s [%s] %s\n", issue.Plugin, issue.Severity, issue.Message)
	}
	return nil
}

func loadPluginRegistry(configPath string) (*config.Config, *plugin.Registry, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, nil, err
	}
	registry, err := plugin.NewRegistry(cfg.Plugins)
	if err != nil {
		return nil, nil, err
	}
	return cfg, registry, nil
}

func findPluginManifest(configPath string, name string) (plugin.Manifest, *config.Config, error) {
	cfg, registry, err := loadPluginRegistry(configPath)
	if err != nil {
		return plugin.Manifest{}, nil, err
	}
	manifest, ok := findPluginByName(registry.List(), name)
	if !ok {
		return plugin.Manifest{}, nil, fmt.Errorf("plugin not found: %s", name)
	}
	return manifest, cfg, nil
}

func findPluginByName(items []plugin.Manifest, name string) (plugin.Manifest, bool) {
	name = strings.TrimSpace(strings.ToLower(name))
	for _, manifest := range items {
		if strings.EqualFold(strings.TrimSpace(manifest.Name), name) {
			return manifest, true
		}
	}
	return plugin.Manifest{}, false
}

func currentPluginEnabledSet(cfg *config.Config, items []plugin.Manifest) map[string]bool {
	set := map[string]bool{}
	if len(cfg.Plugins.Enabled) > 0 {
		for _, name := range cfg.Plugins.Enabled {
			name = strings.TrimSpace(name)
			if name != "" {
				set[name] = true
			}
		}
		return set
	}
	for _, manifest := range items {
		if manifest.Builtin || !manifest.Enabled {
			continue
		}
		set[manifest.Name] = true
	}
	return set
}

func sortedEnabledPluginNames(enabled map[string]bool) []string {
	items := make([]string, 0, len(enabled))
	for name, ok := range enabled {
		if ok && strings.TrimSpace(name) != "" {
			items = append(items, name)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(items[i]) < strings.ToLower(items[j])
	})
	return items
}

func firstNonEmptyPluginTrust(manifest plugin.Manifest) string {
	if strings.TrimSpace(manifest.Trust) != "" {
		return manifest.Trust
	}
	if manifest.Verified {
		return "verified"
	}
	return "unverified"
}
