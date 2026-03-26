package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func runPluginCommand(args []string) error {
	if len(args) == 0 {
		printPluginUsage()
		return nil
	}
	if args[0] != "new" {
		printPluginUsage()
		return fmt.Errorf("unknown plugin command: %s", args[0])
	}
	fs := flag.NewFlagSet("plugin new", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	kind := fs.String("kind", "tool", "plugin kind: tool|ingress|channel")
	name := fs.String("name", "", "plugin name")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if strings.TrimSpace(*name) == "" {
		return fmt.Errorf("--name is required")
	}
	return scaffoldPlugin(*name, *kind)
}

func printPluginUsage() {
	fmt.Print(`AnyClaw plugin commands:

Usage:
  anyclaw plugin new --name my-plugin --kind tool
  anyclaw plugin new --name my-ingress --kind ingress
  anyclaw plugin new --name my-channel --kind channel
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
	default:
		return fmt.Errorf("unsupported plugin kind: %s", kind)
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), data, 0o644); err != nil {
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
	default:
		return "print('plugin scaffold')\n"
	}
}
