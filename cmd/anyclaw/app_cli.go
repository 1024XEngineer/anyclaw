package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	appstore "github.com/anyclaw/anyclaw/pkg/apps"
	"github.com/anyclaw/anyclaw/pkg/config"
	"github.com/anyclaw/anyclaw/pkg/plugin"
	"github.com/anyclaw/anyclaw/pkg/ui"
)

func runAppCommand(args []string) error {
	if len(args) == 0 {
		printAppUsage()
		return nil
	}
	switch args[0] {
	case "list":
		return runAppList(args[1:])
	case "discover":
		return runAppDiscover(args[1:])
	case "generate":
		return runAppGenerate(args[1:])
	case "learn":
		return runAppLearn(args[1:])
	case "status":
		return runAppStatus(args[1:])
	case "workflows":
		return runAppWorkflowsCommand(args[1:])
	case "bindings":
		return runAppBindingsCommand(args[1:])
	case "pairings":
		return runAppPairingsCommand(args[1:])
	default:
		printAppUsage()
		return fmt.Errorf("unknown app command: %s", args[0])
	}
}

func printAppUsage() {
	fmt.Print(`AnyClaw app commands:

Usage:
  anyclaw app list [--config anyclaw.json]
  anyclaw app discover [--config anyclaw.json] [--scan] [--inspect]
  anyclaw app generate --name "App Name" --process app.exe --window "App Window" --launch "C:\\Path\\to\\app.exe" [--category IM]
  anyclaw app learn start --app <app> [--workflow <name>]
  anyclaw app learn capture [--app <app>]
  anyclaw app learn verify <element-id>
  anyclaw app learn save --workflow <name> --step <step-name>
  anyclaw app learn list [--app <app>]
  anyclaw app learn export <pairing-id>
  anyclaw app status [--process <name>] [--watch]
  anyclaw app workflows resolve [--config anyclaw.json] [--limit 3] [--query "remove background"] [task text]
  anyclaw app bindings list [app] [--config anyclaw.json]
  anyclaw app bindings set --app demo-app --name primary [--config-values key=value,key2=value2] [--secret-values token=xxx]
  anyclaw app bindings delete --id <binding-id>
  anyclaw app pairings list [app] [--config anyclaw.json]
  anyclaw app pairings set --app demo-app --workflow remove-background --name personal-default [--binding primary] [--triggers photo,png] [--default-values export=png]
  anyclaw app pairings delete --id <pairing-id>
`)
}

func runAppList(args []string) error {
	fs := flag.NewFlagSet("app list", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	registry, err := plugin.NewRegistry(cfg.Plugins)
	if err != nil {
		return err
	}

	items := registry.List()
	count := 0
	for _, manifest := range items {
		if manifest.App == nil {
			continue
		}
		count++
		name := manifest.Name
		if strings.TrimSpace(manifest.App.Name) != "" {
			name = strings.TrimSpace(manifest.App.Name)
		}
		fmt.Println(ui.Bold.Sprint(name))
		fmt.Println("  plugin: " + manifest.Name)
		if desc := strings.TrimSpace(firstNonEmptyCLI(manifest.App.Description, manifest.Description)); desc != "" {
			fmt.Println("  desc:   " + desc)
		}
		fmt.Println("  enabled: " + fmt.Sprintf("%v", manifest.Enabled))
		if transport := strings.TrimSpace(manifest.App.Transport); transport != "" {
			fmt.Println("  transport: " + transport)
		} else if manifest.App.Desktop != nil {
			fmt.Println("  transport: desktop")
		}
		if len(manifest.App.Platforms) > 0 {
			fmt.Println("  platforms: " + strings.Join(manifest.App.Platforms, ", "))
		}
		if len(manifest.App.Capabilities) > 0 {
			fmt.Println("  capabilities: " + strings.Join(manifest.App.Capabilities, ", "))
		}
		if manifest.App.Desktop != nil {
			if strings.TrimSpace(manifest.App.Desktop.LaunchCommand) != "" {
				fmt.Println("  launch: " + manifest.App.Desktop.LaunchCommand)
			}
			if strings.TrimSpace(manifest.App.Desktop.WindowTitle) != "" {
				fmt.Println("  window: " + manifest.App.Desktop.WindowTitle)
			}
		}
		if len(manifest.App.Actions) == 0 {
			fmt.Println("  actions: none")
		} else {
			fmt.Println("  actions:")
			for _, action := range manifest.App.Actions {
				if strings.TrimSpace(action.Name) == "" {
					continue
				}
				if strings.TrimSpace(action.Kind) != "" {
					fmt.Printf("    - %s [%s] -> %s\n", action.Name, action.Kind, plugin.AppActionToolName(manifest.Name, action.Name))
				} else {
					fmt.Printf("    - %s -> %s\n", action.Name, plugin.AppActionToolName(manifest.Name, action.Name))
				}
			}
		}
		if len(manifest.App.Workflows) == 0 {
			fmt.Println("  workflows: none")
		} else {
			fmt.Println("  workflows:")
			for _, workflow := range manifest.App.Workflows {
				if strings.TrimSpace(workflow.Name) == "" || strings.TrimSpace(workflow.Action) == "" {
					continue
				}
				if len(workflow.Tags) > 0 {
					fmt.Printf("    - %s -> %s (action: %s, tags: %s)\n", workflow.Name, plugin.AppWorkflowToolName(manifest.Name, workflow.Name), workflow.Action, strings.Join(workflow.Tags, ", "))
				} else {
					fmt.Printf("    - %s -> %s (action: %s)\n", workflow.Name, plugin.AppWorkflowToolName(manifest.Name, workflow.Name), workflow.Action)
				}
			}
		}
		fmt.Println()
	}
	if count == 0 {
		fmt.Println("No app connectors found.")
	}
	return nil
}

func runAppBindingsCommand(args []string) error {
	if len(args) == 0 {
		printAppUsage()
		return nil
	}
	switch args[0] {
	case "list":
		return runAppBindingsList(args[1:])
	case "set":
		return runAppBindingsSet(args[1:])
	case "delete":
		return runAppBindingsDelete(args[1:])
	default:
		printAppUsage()
		return fmt.Errorf("unknown app bindings command: %s", args[0])
	}
}

func runAppPairingsCommand(args []string) error {
	if len(args) == 0 {
		printAppUsage()
		return nil
	}
	switch args[0] {
	case "list":
		return runAppPairingsList(args[1:])
	case "set":
		return runAppPairingsSet(args[1:])
	case "delete":
		return runAppPairingsDelete(args[1:])
	default:
		printAppUsage()
		return fmt.Errorf("unknown app pairings command: %s", args[0])
	}
}

func runAppWorkflowsCommand(args []string) error {
	if len(args) == 0 {
		printAppUsage()
		return nil
	}
	switch args[0] {
	case "resolve":
		return runAppWorkflowsResolve(args[1:])
	default:
		printAppUsage()
		return fmt.Errorf("unknown app workflows command: %s", args[0])
	}
}

func runAppWorkflowsResolve(args []string) error {
	fs := flag.NewFlagSet("app workflows resolve", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	limit := fs.Int("limit", 3, "maximum number of workflow suggestions")
	query := fs.String("query", "", "task text to resolve into app workflows")
	if err := fs.Parse(args); err != nil {
		return err
	}

	taskText := strings.TrimSpace(*query)
	if taskText == "" && fs.NArg() > 0 {
		taskText = strings.TrimSpace(strings.Join(fs.Args(), " "))
	}
	if taskText == "" {
		return fmt.Errorf("workflow query is required")
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	registry, err := plugin.NewRegistry(cfg.Plugins)
	if err != nil {
		return err
	}

	var pairings []*appstore.Pairing
	if store, err := appstore.NewStore(*configPath); err == nil {
		pairings = store.ListPairings()
	}
	matches := registry.ResolveWorkflowMatchesWithPairings(taskText, *limit, pairings)
	if len(matches) == 0 {
		fmt.Println("No app workflows matched.")
		return nil
	}

	fmt.Println(ui.Bold.Sprint("Workflow matches"))
	fmt.Println("  query: " + taskText)
	fmt.Println()
	for _, match := range matches {
		fmt.Println(ui.Bold.Sprint(match.Workflow.ToolName))
		fmt.Println("  app:      " + match.Workflow.App)
		fmt.Println("  plugin:   " + match.Workflow.Plugin)
		fmt.Println("  workflow: " + match.Workflow.Name)
		fmt.Println("  action:   " + match.Workflow.Action)
		fmt.Println("  score:    " + fmt.Sprintf("%d", match.Score))
		if desc := strings.TrimSpace(match.Workflow.Description); desc != "" {
			fmt.Println("  desc:     " + desc)
		}
		if len(match.Workflow.Tags) > 0 {
			fmt.Println("  tags:     " + strings.Join(match.Workflow.Tags, ", "))
		}
		if match.Pairing != nil {
			fmt.Println("  pairing:  " + match.Pairing.Name)
			if strings.TrimSpace(match.Pairing.Binding) != "" {
				fmt.Println("  binding:  " + strings.TrimSpace(match.Pairing.Binding))
			}
			if len(match.Pairing.Triggers) > 0 {
				fmt.Println("  triggers: " + strings.Join(match.Pairing.Triggers, ", "))
			}
			if len(match.Pairing.Defaults) > 0 {
				fmt.Println("  defaults: " + formatCLIAnyMap(match.Pairing.Defaults))
			}
		}
		if reason := strings.TrimSpace(match.Reason); reason != "" {
			fmt.Println("  reason:   " + reason)
		}
		fmt.Println()
	}
	return nil
}

func runAppBindingsList(args []string) error {
	fs := flag.NewFlagSet("app bindings list", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	store, err := appstore.NewStore(*configPath)
	if err != nil {
		return err
	}
	filterApp := ""
	if fs.NArg() > 0 {
		filterApp = strings.TrimSpace(fs.Arg(0))
	}
	items := store.List()
	filtered := make([]*appstore.Binding, 0, len(items))
	for _, item := range items {
		if filterApp != "" && !strings.EqualFold(strings.TrimSpace(item.App), filterApp) {
			continue
		}
		filtered = append(filtered, item)
	}
	if len(filtered) == 0 {
		fmt.Println("No app bindings found.")
		return nil
	}
	for _, item := range filtered {
		fmt.Println(ui.Bold.Sprint(item.Name))
		fmt.Println("  id:      " + item.ID)
		fmt.Println("  app:     " + item.App)
		fmt.Println("  enabled: " + fmt.Sprintf("%v", item.Enabled))
		if item.Target != "" {
			fmt.Println("  target:  " + item.Target)
		}
		if item.Workspace != "" {
			fmt.Println("  scope:   " + item.Workspace)
		}
		if len(item.Config) > 0 {
			keys := mapKeys(item.Config)
			fmt.Println("  config:  " + strings.Join(keys, ", "))
		}
		if len(item.Secrets) > 0 {
			keys := mapKeys(item.Secrets)
			fmt.Println("  secrets: " + strings.Join(keys, ", "))
		}
		fmt.Println()
	}
	return nil
}

func runAppBindingsSet(args []string) error {
	fs := flag.NewFlagSet("app bindings set", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	id := fs.String("id", "", "existing binding id")
	app := fs.String("app", "", "app plugin name")
	name := fs.String("name", "", "binding name")
	description := fs.String("description", "", "binding description")
	target := fs.String("target", "", "optional target/account")
	org := fs.String("org", "", "optional org scope")
	project := fs.String("project", "", "optional project scope")
	workspace := fs.String("workspace", "", "optional workspace scope")
	configValues := fs.String("config-values", "", "comma-separated config key=value pairs")
	secretValues := fs.String("secret-values", "", "comma-separated secret key=value pairs")
	enabled := fs.Bool("enabled", true, "whether the binding is enabled")
	if err := fs.Parse(args); err != nil {
		return err
	}

	store, err := appstore.NewStore(*configPath)
	if err != nil {
		return err
	}

	binding := &appstore.Binding{
		ID:          strings.TrimSpace(*id),
		App:         strings.TrimSpace(*app),
		Name:        strings.TrimSpace(*name),
		Description: strings.TrimSpace(*description),
		Enabled:     *enabled,
		Target:      strings.TrimSpace(*target),
		Org:         strings.TrimSpace(*org),
		Project:     strings.TrimSpace(*project),
		Workspace:   strings.TrimSpace(*workspace),
		Config:      parseKV(*configValues),
		Secrets:     parseKV(*secretValues),
	}
	if err := store.Upsert(binding); err != nil {
		return err
	}
	printSuccess("Saved app binding: %s (%s)", binding.Name, binding.App)
	return nil
}

func runAppBindingsDelete(args []string) error {
	fs := flag.NewFlagSet("app bindings delete", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	id := fs.String("id", "", "binding id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*id) == "" {
		return fmt.Errorf("--id is required")
	}
	store, err := appstore.NewStore(*configPath)
	if err != nil {
		return err
	}
	if err := store.Delete(*id); err != nil {
		return err
	}
	printSuccess("Deleted app binding: %s", *id)
	return nil
}

func runAppPairingsList(args []string) error {
	fs := flag.NewFlagSet("app pairings list", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	store, err := appstore.NewStore(*configPath)
	if err != nil {
		return err
	}
	filterApp := ""
	if fs.NArg() > 0 {
		filterApp = strings.TrimSpace(fs.Arg(0))
	}
	items := store.ListPairings()
	filtered := make([]*appstore.Pairing, 0, len(items))
	for _, item := range items {
		if filterApp != "" && !strings.EqualFold(strings.TrimSpace(item.App), filterApp) {
			continue
		}
		filtered = append(filtered, item)
	}
	if len(filtered) == 0 {
		fmt.Println("No app pairings found.")
		return nil
	}
	for _, item := range filtered {
		fmt.Println(ui.Bold.Sprint(item.Name))
		fmt.Println("  id:       " + item.ID)
		fmt.Println("  app:      " + item.App)
		fmt.Println("  workflow: " + item.Workflow)
		fmt.Println("  enabled:  " + fmt.Sprintf("%v", item.Enabled))
		if item.Binding != "" {
			fmt.Println("  binding:  " + item.Binding)
		}
		if len(item.Triggers) > 0 {
			fmt.Println("  triggers: " + strings.Join(item.Triggers, ", "))
		}
		if len(item.Defaults) > 0 {
			fmt.Println("  defaults: " + formatCLIAnyMap(item.Defaults))
		}
		fmt.Println()
	}
	return nil
}

func runAppPairingsSet(args []string) error {
	fs := flag.NewFlagSet("app pairings set", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	id := fs.String("id", "", "existing pairing id")
	app := fs.String("app", "", "app plugin name")
	workflow := fs.String("workflow", "", "workflow name or tool name")
	name := fs.String("name", "", "pairing name")
	description := fs.String("description", "", "pairing description")
	binding := fs.String("binding", "", "optional binding reference")
	org := fs.String("org", "", "optional org scope")
	project := fs.String("project", "", "optional project scope")
	workspace := fs.String("workspace", "", "optional workspace scope")
	triggers := fs.String("triggers", "", "comma-separated trigger phrases")
	defaultValues := fs.String("default-values", "", "comma-separated default key=value pairs")
	enabled := fs.Bool("enabled", true, "whether the pairing is enabled")
	if err := fs.Parse(args); err != nil {
		return err
	}

	store, err := appstore.NewStore(*configPath)
	if err != nil {
		return err
	}

	pairing := &appstore.Pairing{
		ID:          strings.TrimSpace(*id),
		App:         strings.TrimSpace(*app),
		Workflow:    strings.TrimSpace(*workflow),
		Binding:     strings.TrimSpace(*binding),
		Name:        strings.TrimSpace(*name),
		Description: strings.TrimSpace(*description),
		Enabled:     *enabled,
		Org:         strings.TrimSpace(*org),
		Project:     strings.TrimSpace(*project),
		Workspace:   strings.TrimSpace(*workspace),
		Triggers:    parseCSV(*triggers),
		Defaults:    parseKVAny(*defaultValues),
	}
	if err := store.UpsertPairing(pairing); err != nil {
		return err
	}
	printSuccess("Saved app pairing: %s -> %s (%s)", pairing.Name, pairing.App, pairing.Workflow)
	return nil
}

func runAppPairingsDelete(args []string) error {
	fs := flag.NewFlagSet("app pairings delete", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	id := fs.String("id", "", "pairing id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*id) == "" {
		return fmt.Errorf("--id is required")
	}
	store, err := appstore.NewStore(*configPath)
	if err != nil {
		return err
	}
	if err := store.DeletePairing(*id); err != nil {
		return err
	}
	printSuccess("Deleted app pairing: %s", *id)
	return nil
}

func parseKV(input string) map[string]string {
	values := map[string]string{}
	for _, item := range strings.Split(input, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		parts := strings.SplitN(item, "=", 2)
		key := strings.TrimSpace(parts[0])
		if key == "" {
			continue
		}
		value := ""
		if len(parts) == 2 {
			value = strings.TrimSpace(parts[1])
		}
		values[key] = value
	}
	return values
}

func parseKVAny(input string) map[string]any {
	values := map[string]any{}
	for key, value := range parseKV(input) {
		values[key] = inferCLIValue(value)
	}
	return values
}

func parseCSV(input string) []string {
	items := make([]string, 0)
	for _, item := range strings.Split(input, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			items = append(items, item)
		}
	}
	return items
}

func inferCLIValue(value string) any {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true":
		return true
	case "false":
		return false
	}
	return strings.TrimSpace(value)
}

func formatCLIAnyMap(items map[string]any) string {
	if len(items) == 0 {
		return ""
	}
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", key, items[key]))
	}
	return strings.Join(parts, ", ")
}

func mapKeys(items map[string]string) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func firstNonEmptyCLI(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func runAppDiscover(args []string) error {
	fs := flag.NewFlagSet("app discover", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	doScan := fs.Bool("scan", false, "scan for installed apps")
	doInspect := fs.Bool("inspect", false, "inspect UI elements for running apps")
	showUI := fs.Bool("ui", false, "show UI map for discovered apps")
	if err := fs.Parse(args); err != nil {
		return err
	}

	_, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	store, err := appstore.NewStore(*configPath)
	if err != nil {
		return err
	}

	ctx := context.Background()

	if *doScan {
		fmt.Println("Scanning for installed applications...")
		apps, err := appstore.DiscoverApps(ctx, nil)
		if err != nil {
			return fmt.Errorf("failed to discover apps: %w", err)
		}
		fmt.Printf("Found %d applications\n\n", len(apps))

		for _, app := range apps {
			fmt.Println(ui.Bold.Sprint(app.Name))
			fmt.Printf("  ID:          %s\n", app.ID)
			fmt.Printf("  Path:        %s\n", app.Path)
			fmt.Printf("  Process:     %s\n", app.ProcessName)
			fmt.Printf("  Category:    %s\n", app.Category)
			fmt.Printf("  Vendor:      %s\n", app.Vendor)
			if app.Version != "" {
				fmt.Printf("  Version:     %s\n", app.Version)
			}
			if app.WindowTitle != "" {
				fmt.Printf("  Window:      %s\n", app.WindowTitle)
			}
			fmt.Println()

			if err := store.UpsertApp(&app); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save app %s: %v\n", app.Name, err)
			}
		}
		fmt.Println("Apps saved to store.")
	}

	if *doInspect {
		apps := store.ListApps()
		if len(apps) == 0 {
			fmt.Println("No apps in store. Run 'anyclaw app discover --scan' first.")
			return nil
		}

		fmt.Println("Inspecting UI for running applications...")
		for _, app := range apps {
			_, err := appstore.ProbeAppWindow(ctx, app)
			if err != nil {
				fmt.Printf("Skipping %s (not running)\n", app.Name)
				continue
			}
			fmt.Printf("\n%s is running - inspecting UI...\n", app.Name)

			uiMap, err := appstore.InspectAppUI(ctx, app, 100)
			if err != nil {
				fmt.Printf("Warning: failed to inspect UI for %s: %v\n", app.Name, err)
				continue
			}
			fmt.Printf("Found %d UI elements\n", len(uiMap.Elements))

			if *showUI {
				for _, el := range uiMap.Elements {
					if el.Name != "" {
						fmt.Printf("  - %s [%s] %s\n", el.Name, el.ControlType, el.AutomationID)
					}
				}
			}

			if err := store.UpsertUIMap(uiMap); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save UI map for %s: %v\n", app.Name, err)
			}
		}
		fmt.Println("\nUI maps saved to store.")
	}

	if !*doScan && !*doInspect {
		apps := store.ListApps()
		if len(apps) == 0 {
			fmt.Println("No discovered apps. Run 'anyclaw app discover --scan' first.")
			return nil
		}

		fmt.Println("Discovered applications:")
		for _, app := range apps {
			fmt.Println(ui.Bold.Sprint(app.Name))
			fmt.Printf("  Process:  %s\n", app.ProcessName)
			fmt.Printf("  Category: %s\n", app.Category)
			fmt.Printf("  Path:     %s\n", app.Path)
			fmt.Println()
		}

		uiMaps := store.ListUIMaps()
		if len(uiMaps) > 0 {
			fmt.Println("Available UI Maps:")
			for _, uiMap := range uiMaps {
				fmt.Printf("  %s: %d elements\n", uiMap.AppName, len(uiMap.Elements))
			}
		}
	}

	return nil
}

func runAppGenerate(args []string) error {
	fs := flag.NewFlagSet("app generate", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	appName := fs.String("name", "", "Application name (required)")
	processName := fs.String("process", "", "Process name (required)")
	windowTitle := fs.String("window", "", "Window title pattern (required)")
	launchCmd := fs.String("launch", "", "Launch command path (required)")
	category := fs.String("category", "general", "App category (IM, Browser, Editor, Office, System)")
	outputFile := fs.String("output", "", "Output file (default: stdout)")
	pluginOnly := fs.Bool("plugin-only", false, "Generate only plugin.json template")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *appName == "" || *processName == "" || *windowTitle == "" || *launchCmd == "" {
		fs.Usage()
		return fmt.Errorf("missing required flags")
	}

	if *pluginOnly {
		template := appstore.GeneratePluginTemplate(*appName, *processName, *windowTitle, *launchCmd, *category)
		if *outputFile != "" {
			return os.WriteFile(*outputFile, []byte(template), 0644)
		}
		fmt.Println(template)
		return nil
	}

	code := appstore.GenerateConnectorCode(*appName, *processName, *windowTitle, *launchCmd)
	if *outputFile != "" {
		return os.WriteFile(*outputFile, []byte(code), 0644)
	}
	fmt.Println(code)
	return nil
}

var learnSession *appstore.LearnSession

func runAppLearn(args []string) error {
	fs := flag.NewFlagSet("app learn", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)

	subCmd := fs.String("subcommand", "", "subcommand (start/capture/verify/save/list/export)")
	appName := fs.String("app", "", "app name")
	processName := fs.String("process", "", "process name")
	windowTitle := fs.String("window", "", "window title")
	workflow := fs.String("workflow", "", "workflow name")
	stepName := fs.String("step", "", "step name")
	elementID := fs.String("element", "", "element ID to verify")
	configPath := fs.String("config", "anyclaw.json", "config path")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *subCmd == "" {
		if len(args) > 0 {
			*subCmd = args[0]
		}
	}

	ctx := context.Background()

	switch *subCmd {
	case "start":
		if *appName == "" {
			return fmt.Errorf("--app is required")
		}
		if *processName == "" {
			*processName = *appName
		}
		if *windowTitle == "" {
			*windowTitle = *appName
		}

		config := appstore.NewUILearnConfig("", *appName, *windowTitle, *processName)
		session, err := appstore.StartLearnSession(ctx, config)
		if err != nil {
			return err
		}
		learnSession = session
		fmt.Printf("Started learn session for %s\n", *appName)
		fmt.Printf("Session ID: %s\n", session.ID)
		fmt.Printf("Window: %s\n", session.WindowTitle)
		fmt.Println("\nNext: run 'anyclaw app learn capture' to capture UI elements")

	case "capture":
		if learnSession == nil {
			return fmt.Errorf("no active session. run 'anyclaw app learn start' first")
		}
		elements, err := learnSession.CaptureUI(ctx, 100)
		if err != nil {
			return fmt.Errorf("capture failed: %v", err)
		}
		fmt.Printf("Captured %d UI elements:\n\n", len(elements))
		for i, elem := range elements {
			status := "unverified"
			if elem.Verified {
				status = "verified"
			}
			fmt.Printf("  [%d] %s (%s)\n", i+1, ui.Bold.Sprint(elem.Label), status)
			if elem.Selector.AutomationID != "" {
				fmt.Printf("       ID: %s\n", elem.Selector.AutomationID)
			}
			if elem.Selector.ControlType != "" {
				fmt.Printf("       Type: %s\n", elem.Selector.ControlType)
			}
			if len(elem.Selector.Fallbacks) > 0 {
				fmt.Printf("       Fallbacks: %d\n", len(elem.Selector.Fallbacks))
			}
			fmt.Println()
		}
		fmt.Println("Next: run 'anyclaw app learn verify <index>' to verify an element")

	case "verify":
		if learnSession == nil {
			return fmt.Errorf("no active session")
		}
		if *elementID == "" && len(args) > 1 {
			*elementID = args[1]
		}
		if *elementID == "" {
			return fmt.Errorf("--element or element index is required")
		}

		verified, err := learnSession.VerifyElement(ctx, *elementID)
		if err != nil {
			return fmt.Errorf("verify failed: %v", err)
		}
		if verified {
			fmt.Println("Element verified successfully!")
		} else {
			fmt.Println("Element not found. Try adding a fallback selector.")
		}

	case "save":
		if learnSession == nil {
			return fmt.Errorf("no active session")
		}
		if *workflow == "" {
			return fmt.Errorf("--workflow is required")
		}
		if *stepName == "" {
			*stepName = "default"
		}

		pairing := learnSession.SavePairing(*workflow, *stepName)

		store, err := appstore.NewPairingStore(*configPath)
		if err != nil {
			return err
		}
		if err := store.Upsert(pairing); err != nil {
			return err
		}

		fmt.Printf("Saved pairing: %s / %s\n", *workflow, *stepName)
		fmt.Printf("Pairing ID: %s\n", pairing.ID)
		fmt.Printf("Elements: %d\n", len(pairing.Elements))

	case "list":
		store, err := appstore.NewPairingStore(*configPath)
		if err != nil {
			return err
		}

		pairings := store.List()
		if len(pairings) == 0 {
			fmt.Println("No UI pairings found.")
			return nil
		}

		fmt.Println("UI Pairings:")
		for _, p := range pairings {
			fmt.Printf("\n%s / %s\n", ui.Bold.Sprint(p.AppName), p.Workflow)
			fmt.Printf("  ID: %s\n", p.ID)
			fmt.Printf("  Elements: %d\n", len(p.Elements))
			verified := 0
			for _, e := range p.Elements {
				if e.Verified {
					verified++
				}
			}
			fmt.Printf("  Verified: %d/%d\n", verified, len(p.Elements))
		}

	case "export":
		if *elementID == "" && len(args) > 1 {
			*elementID = args[1]
		}
		if *elementID == "" {
			return fmt.Errorf("--element or pairing ID is required")
		}

		store, err := appstore.NewPairingStore(*configPath)
		if err != nil {
			return err
		}

		for _, p := range store.List() {
			if p.ID == *elementID {
				fmt.Println(appstore.GeneratePairingJSON(p))
				return nil
			}
		}
		return fmt.Errorf("pairing not found: %s", *elementID)

	default:
		fmt.Print(`UI Learning Commands:
  anyclaw app learn start --app <name> [--process <proc>] [--window <title>]
  anyclaw app learn capture
  anyclaw app learn verify <element-id>
  anyclaw app learn save --workflow <name> --step <step>
  anyclaw app learn list
  anyclaw app learn export <pairing-id>
`)
	}

	return nil
}

var windowMonitor = appstore.NewWindowMonitor()

func runAppStatus(args []string) error {
	fs := flag.NewFlagSet("app status", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)

	processName := fs.String("process", "", "process name to check")
	windowTitle := fs.String("window", "", "window title pattern")
	watch := fs.Bool("watch", false, "watch for changes")
	interval := fs.Int("interval", 1000, "watch interval in ms")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *processName == "" && len(args) > 0 {
		*processName = args[0]
	}

	if *processName == "" {
		probes := windowMonitor.GetAllProbes()
		if len(probes) == 0 {
			fmt.Println("No window status data. Run with --process first.")
			return nil
		}

		fmt.Println("Window Status:")
		for name, probe := range probes {
			status := string(probe.Status)
			if probe.IsFocused {
				status = "focused"
			}
			fmt.Printf("  %s: %s\n", name, status)
			if probe.WindowTitle != "" {
				fmt.Printf("    Window: %s\n", probe.WindowTitle)
			}
		}
		return nil
	}

	if *windowTitle == "" {
		*windowTitle = *processName
	}

	if *watch {
		fmt.Printf("Watching %s for changes (Ctrl+C to exit)...\n", *processName)

		windowMonitor.WatchForChanges(*processName, time.Duration(*interval)*time.Millisecond, func(change appstore.WindowStateChange) {
			fmt.Printf("[%s] %s: %s -> %s\n",
				change.Timestamp.Format("15:04:05"),
				change.ProcessName,
				change.FromStatus,
				change.ToStatus)
		})

		select {}
	}

	result, err := windowMonitor.Probe(*processName, *windowTitle)
	if err != nil {
		return fmt.Errorf("probe failed: %v", err)
	}

	fmt.Printf("Process: %s\n", result.ProcessName)
	fmt.Printf("Status: %s\n", result.Status)

	if result.ProcessID > 0 {
		fmt.Printf("PID: %d\n", result.ProcessID)
	}

	if result.WindowTitle != "" {
		fmt.Printf("Window: %s\n", result.WindowTitle)
		fmt.Printf("Position: (%d, %d)\n", result.X, result.Y)
		fmt.Printf("Size: %dx%d\n", result.Width, result.Height)
		fmt.Printf("Center: (%d, %d)\n", result.CenterX, result.CenterY)
	}

	fmt.Printf("Focused: %v\n", result.IsFocused)
	fmt.Printf("Minimized: %v\n", result.IsMinimized)
	fmt.Printf("Visible: %v\n", result.IsVisible)
	fmt.Printf("Enabled: %v\n", result.IsEnabled)
	fmt.Printf("Probe Time: %s\n", result.ProbeTime.Format("15:04:05"))

	return nil
}
