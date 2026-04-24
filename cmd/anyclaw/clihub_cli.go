package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/clihub"
)

func runCLIHubCommand(args []string) error {
	if len(args) == 0 {
		printCLIHubUsage()
		return nil
	}

	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "search":
		return runCLIHubSearch(args[1:])
	case "list":
		return runCLIHubList(args[1:])
	case "install":
		return runCLIHubInstall(args[1:])
	case "installed":
		return runCLIHubInstalled(args[1:])
	case "info":
		return runCLIHubInfo(args[1:])
	case "capabilities", "caps":
		return runCLIHubCapabilities(args[1:])
	case "exec", "run":
		return runCLIHubExec(args[1:])
	case "help", "-h", "--help":
		printCLIHubUsage()
		return nil
	default:
		printCLIHubUsage()
		return fmt.Errorf("unknown clihub command: %s", args[0])
	}
}

func printCLIHubUsage() {
	fmt.Print(`AnyClaw clihub commands:

Usage:
  anyclaw clihub search [query] [--category <name>] [--installed] [--limit <n>] [--json] [--workspace <path>]
  anyclaw clihub list [--installed] [--runnable] [--limit <n>] [--json] [--workspace <path>]
  anyclaw clihub install <name> [--root <path>]
  anyclaw clihub installed [--json] [--workspace <path>]
  anyclaw clihub info <name> [--json] [--workspace <path>]
  anyclaw clihub capabilities [query] [--harness <name>] [--limit <n>] [--json] [--workspace <path>]
  anyclaw clihub exec <name> [--json=true|false] [--auto-install] [--cwd <path>] [--workspace <path>] [-- <args...>]

Flags:
  --root <path>       Explicit CLI-Anything root
  --workspace <path>  Start discovery from this workspace
  --cwd <path>        Working directory override for installed executables

Notes:
  clihub install requires an explicit trusted root via --root or ANYCLAW_CLI_ANYTHING_ROOT.
  Install does not execute catalog shell from roots discovered implicitly from the current workspace.
  Source harnesses always run from their checkout directory so local module imports resolve correctly.
`)
}

func runCLIHubSearch(args []string) error {
	fs := flag.NewFlagSet("clihub search", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	rootFlag := fs.String("root", "", "explicit CLI-Anything root")
	workspaceFlag := fs.String("workspace", "", "workspace path used for discovery")
	categoryFlag := fs.String("category", "", "category filter")
	installedFlag := fs.Bool("installed", false, "show only installed entries")
	limitFlag := fs.Int("limit", 10, "maximum results")
	jsonFlag := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(reorderFlagArgs(args, map[string]bool{
		"--root":      true,
		"--workspace": true,
		"--category":  true,
		"--installed": false,
		"--limit":     true,
		"--json":      false,
	})); err != nil {
		return err
	}

	root, err := resolveCLIHubRoot(*rootFlag, *workspaceFlag)
	if err != nil {
		return err
	}
	cat, err := clihub.Load(root)
	if err != nil {
		return err
	}
	query := strings.TrimSpace(strings.Join(fs.Args(), " "))
	results := clihub.Search(cat, query, *categoryFlag, *installedFlag, *limitFlag)
	if *jsonFlag {
		return printCLIHubJSON(map[string]any{
			"root":           cat.Root,
			"updated":        cat.Updated,
			"query":          query,
			"category":       strings.TrimSpace(*categoryFlag),
			"installed_only": *installedFlag,
			"count":          len(results),
			"results":        results,
		})
	}

	fmt.Println(clihub.HumanSummary(cat))
	fmt.Println()
	for _, item := range results {
		fmt.Printf("- %s (%s)\n", firstNonEmptyCLIHub(item.DisplayName, item.Name), clihub.StatusLabel(item))
		fmt.Printf("  %s\n", strings.TrimSpace(item.Description))
		if strings.TrimSpace(item.InstallCmd) != "" {
			fmt.Printf("  install: %s\n", strings.TrimSpace(item.InstallCmd))
		}
	}
	return nil
}

func runCLIHubList(args []string) error {
	fs := flag.NewFlagSet("clihub list", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	rootFlag := fs.String("root", "", "explicit CLI-Anything root")
	workspaceFlag := fs.String("workspace", "", "workspace path used for discovery")
	installedFlag := fs.Bool("installed", false, "show only installed entries")
	runnableFlag := fs.Bool("runnable", false, "show installed entries plus local source harnesses")
	limitFlag := fs.Int("limit", 0, "maximum results (0 = all)")
	jsonFlag := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(reorderFlagArgs(args, map[string]bool{
		"--root":      true,
		"--workspace": true,
		"--installed": false,
		"--runnable":  false,
		"--limit":     true,
		"--json":      false,
	})); err != nil {
		return err
	}

	root, err := resolveCLIHubRoot(*rootFlag, *workspaceFlag)
	if err != nil {
		return err
	}
	cat, err := clihub.Load(root)
	if err != nil {
		return err
	}

	results := clihub.Search(cat, "", "", false, 0)
	switch {
	case *installedFlag:
		results = clihub.Installed(cat)
	case *runnableFlag:
		results = clihub.Runnable(cat)
	}
	if *limitFlag > 0 && len(results) > *limitFlag {
		results = results[:*limitFlag]
	}

	if *jsonFlag {
		return printCLIHubJSON(map[string]any{
			"root":      cat.Root,
			"updated":   cat.Updated,
			"count":     len(results),
			"installed": *installedFlag,
			"runnable":  *runnableFlag,
			"results":   results,
		})
	}

	if len(results) == 0 {
		fmt.Println("No CLI-Anything harnesses matched the requested status.")
		return nil
	}

	fmt.Printf("CLI-Anything tools (%d)\n", len(results))
	for _, item := range results {
		fmt.Printf("- %s (%s)\n", firstNonEmptyCLIHub(item.DisplayName, item.Name), clihub.StatusLabel(item))
		fmt.Printf("  entry: %s\n", firstNonEmptyCLIHub(item.EntryPoint, item.Name))
		if strings.TrimSpace(item.SourcePath) != "" {
			fmt.Printf("  source: %s\n", item.SourcePath)
		}
		if strings.TrimSpace(item.ExecutablePath) != "" {
			fmt.Printf("  executable: %s\n", item.ExecutablePath)
		}
	}
	return nil
}

func runCLIHubInstalled(args []string) error {
	fs := flag.NewFlagSet("clihub installed", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	rootFlag := fs.String("root", "", "explicit CLI-Anything root")
	workspaceFlag := fs.String("workspace", "", "workspace path used for discovery")
	jsonFlag := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(reorderFlagArgs(args, map[string]bool{
		"--root":      true,
		"--workspace": true,
		"--json":      false,
	})); err != nil {
		return err
	}
	root, err := resolveCLIHubRoot(*rootFlag, *workspaceFlag)
	if err != nil {
		return err
	}
	cat, err := clihub.Load(root)
	if err != nil {
		return err
	}
	results := clihub.Installed(cat)
	if *jsonFlag {
		return printCLIHubJSON(map[string]any{
			"root":    cat.Root,
			"updated": cat.Updated,
			"count":   len(results),
			"results": results,
		})
	}
	if len(results) == 0 {
		fmt.Println("No installed CLI-Anything harnesses found in PATH.")
		return nil
	}
	fmt.Println("Installed CLI-Anything harnesses:")
	for _, item := range results {
		fmt.Printf("- %s -> %s\n", item.Name, item.ExecutablePath)
	}
	return nil
}

func runCLIHubInstall(args []string) error {
	fs := flag.NewFlagSet("clihub install", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	rootFlag := fs.String("root", "", "explicit CLI-Anything root")
	workspaceFlag := fs.String("workspace", "", "workspace path used for discovery")
	if err := fs.Parse(reorderFlagArgs(args, map[string]bool{
		"--root":      true,
		"--workspace": true,
	})); err != nil {
		return err
	}
	if len(fs.Args()) == 0 {
		return fmt.Errorf("usage: anyclaw clihub install <name>")
	}

	cat, err := loadTrustedCLIHubInstallCatalog(*rootFlag, *workspaceFlag)
	if err != nil {
		return err
	}
	item, ok := clihub.Find(cat, fs.Args()[0])
	if !ok {
		return fmt.Errorf("CLI Hub entry not found: %s", fs.Args()[0])
	}
	if item.Installed {
		fmt.Printf("%s is already installed at %s\n", item.Name, item.ExecutablePath)
		return nil
	}
	if err := clihub.RunInstall(item); err != nil {
		return err
	}
	fmt.Printf("Installed %s\n", item.Name)
	return nil
}

func loadTrustedCLIHubInstallCatalog(root string, workspace string) (*clihub.Catalog, error) {
	if explicitRoot := strings.TrimSpace(root); explicitRoot != "" {
		cat, err := clihub.Load(explicitRoot)
		if err != nil {
			return nil, fmt.Errorf("failed to load CLI-Anything root %q: %w", explicitRoot, err)
		}
		return cat, nil
	}

	if envRoot := strings.TrimSpace(os.Getenv(clihub.EnvRoot)); envRoot != "" {
		cat, err := clihub.Load(envRoot)
		if err != nil {
			return nil, fmt.Errorf("failed to load CLI-Anything root from %s=%q: %w", clihub.EnvRoot, envRoot, err)
		}
		return cat, nil
	}

	if strings.TrimSpace(workspace) != "" {
		return nil, fmt.Errorf("clihub install requires an explicit trusted root; --workspace only supports read-only discovery, pass --root <path> or set %s", clihub.EnvRoot)
	}

	return nil, fmt.Errorf("clihub install requires an explicit trusted root; pass --root <path> or set %s", clihub.EnvRoot)
}

func runCLIHubInfo(args []string) error {
	fs := flag.NewFlagSet("clihub info", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	rootFlag := fs.String("root", "", "explicit CLI-Anything root")
	workspaceFlag := fs.String("workspace", "", "workspace path used for discovery")
	jsonFlag := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(reorderFlagArgs(args, map[string]bool{
		"--root":      true,
		"--workspace": true,
		"--json":      false,
	})); err != nil {
		return err
	}
	if len(fs.Args()) == 0 {
		return fmt.Errorf("usage: anyclaw clihub info <name>")
	}
	root, err := resolveCLIHubRoot(*rootFlag, *workspaceFlag)
	if err != nil {
		return err
	}
	cat, err := clihub.Load(root)
	if err != nil {
		return err
	}
	item, ok := clihub.Find(cat, fs.Args()[0])
	if !ok {
		return fmt.Errorf("CLI Hub entry not found: %s", fs.Args()[0])
	}
	if *jsonFlag {
		return printCLIHubJSON(item)
	}
	fmt.Printf("Name: %s\n", item.Name)
	fmt.Printf("Display: %s\n", firstNonEmptyCLIHub(item.DisplayName, item.Name))
	fmt.Printf("Category: %s\n", item.Category)
	fmt.Printf("Status: %s\n", clihub.StatusLabel(item))
	fmt.Printf("Runnable: %v\n", item.Runnable)
	fmt.Printf("Installed: %v\n", item.Installed)
	if strings.TrimSpace(item.ExecutablePath) != "" {
		fmt.Printf("Executable: %s\n", item.ExecutablePath)
	}
	if strings.TrimSpace(item.SourcePath) != "" {
		fmt.Printf("Source: %s\n", item.SourcePath)
	}
	if strings.TrimSpace(item.SkillPath) != "" {
		fmt.Printf("Skill: %s\n", item.SkillPath)
	}
	if strings.TrimSpace(item.DevModule) != "" {
		fmt.Printf("Dev module: %s\n", item.DevModule)
	}
	fmt.Printf("Description: %s\n", item.Description)
	if strings.TrimSpace(item.Requires) != "" {
		fmt.Printf("Requires: %s\n", item.Requires)
	}
	if strings.TrimSpace(item.InstallCmd) != "" {
		fmt.Printf("Install: %s\n", item.InstallCmd)
	}
	return nil
}

func runCLIHubCapabilities(args []string) error {
	fs := flag.NewFlagSet("clihub capabilities", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	rootFlag := fs.String("root", "", "explicit CLI-Anything root")
	workspaceFlag := fs.String("workspace", "", "workspace path used for discovery")
	harnessFlag := fs.String("harness", "", "filter capabilities by harness name")
	limitFlag := fs.Int("limit", 20, "maximum number of capabilities to show")
	jsonFlag := fs.Bool("json", false, "print JSON")
	if err := fs.Parse(reorderFlagArgs(args, map[string]bool{
		"--root":      true,
		"--workspace": true,
		"--harness":   true,
		"--limit":     true,
		"--json":      false,
	})); err != nil {
		return err
	}

	root, err := resolveCLIHubRoot(*rootFlag, *workspaceFlag)
	if err != nil {
		return err
	}
	reg, err := clihub.LoadCapabilityRegistry(root)
	if err != nil {
		return err
	}

	query := strings.TrimSpace(strings.Join(fs.Args(), " "))
	var caps []clihub.Capability
	switch {
	case strings.TrimSpace(*harnessFlag) != "":
		caps = reg.FindByHarness(*harnessFlag)
	case query != "":
		caps = reg.FindByIntent(query)
	default:
		caps = reg.All()
	}
	if *limitFlag > 0 && len(caps) > *limitFlag {
		caps = caps[:*limitFlag]
	}

	if *jsonFlag {
		return printCLIHubJSON(map[string]any{
			"root":         root,
			"query":        query,
			"harness":      strings.TrimSpace(*harnessFlag),
			"count":        len(caps),
			"capabilities": caps,
		})
	}

	if len(caps) == 0 {
		fmt.Println("No CLI Hub capabilities matched.")
		return nil
	}

	fmt.Printf("CLI Hub capabilities (%d)\n", len(caps))
	for _, cap := range caps {
		label := cap.Command
		if strings.TrimSpace(cap.Group) != "" {
			label = cap.Group + " / " + cap.Command
		}
		fmt.Printf("- %s -> %s\n", cap.Harness, label)
		if len(cap.Keywords) > 0 {
			keywords := cap.Keywords
			if len(keywords) > 8 {
				keywords = keywords[:8]
			}
			fmt.Printf("  keywords: %s\n", strings.Join(keywords, ", "))
		}
	}
	return nil
}

func runCLIHubExec(args []string) error {
	flagArgs, passthrough := splitCLIHubExecArgs(args)
	fs := flag.NewFlagSet("clihub exec", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	rootFlag := fs.String("root", "", "explicit CLI-Anything root")
	workspaceFlag := fs.String("workspace", "", "workspace path used for discovery")
	cwdFlag := fs.String("cwd", "", "working directory override for installed executables")
	autoInstallFlag := fs.Bool("auto-install", false, "install the harness automatically if needed")
	jsonFlag := fs.Bool("json", true, "inject --json for agent-style machine-readable output")
	if err := fs.Parse(reorderFlagArgs(flagArgs, map[string]bool{
		"--root":         true,
		"--workspace":    true,
		"--cwd":          true,
		"--auto-install": false,
		"--json":         true,
	})); err != nil {
		return err
	}

	if len(fs.Args()) == 0 {
		return fmt.Errorf("usage: anyclaw clihub exec <name> [--json=true|false] [--auto-install] [--cwd <path>] [-- <args...>]")
	}

	name := strings.TrimSpace(fs.Args()[0])
	if len(passthrough) == 0 && len(fs.Args()) > 1 {
		passthrough = append([]string(nil), fs.Args()[1:]...)
	}

	root, err := resolveCLIHubRoot(*rootFlag, *workspaceFlag)
	if err != nil {
		return err
	}
	cat, err := clihub.Load(root)
	if err != nil {
		return err
	}
	item, ok := clihub.Find(cat, name)
	if !ok {
		return fmt.Errorf("CLI Hub entry not found: %s", name)
	}

	resolved, err := clihub.ResolveCommand(item, passthrough, clihub.ExecOptions{
		JSON:              *jsonFlag,
		AutoInstall:       *autoInstallFlag,
		PreferLocalSrc:    true,
		RetryAfterInstall: true,
		RequestedCwd:      *cwdFlag,
	})
	if err != nil {
		return err
	}

	cmd := exec.Command(resolved.Args[0], resolved.Args[1:]...)
	cmd.Dir = resolved.Cwd
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func resolveCLIHubRoot(root string, workspace string) (string, error) {
	start, err := resolveCLIHubStart(root, workspace)
	if err != nil {
		return "", err
	}
	discovered, ok := clihub.DiscoverRoot(start)
	if !ok {
		return "", fmt.Errorf("CLI-Anything root not found; set %s or pass --root", clihub.EnvRoot)
	}
	return discovered, nil
}

func resolveCLIHubStart(root string, workspace string) (string, error) {
	if strings.TrimSpace(root) != "" {
		return strings.TrimSpace(root), nil
	}
	if strings.TrimSpace(workspace) != "" {
		return strings.TrimSpace(workspace), nil
	}
	return os.Getwd()
}

func printCLIHubJSON(value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func firstNonEmptyCLIHub(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func reorderFlagArgs(args []string, valueFlags map[string]bool) []string {
	if len(args) == 0 {
		return nil
	}
	flags := make([]string, 0, len(args))
	positionals := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, args[i])
			if valueFlags[arg] && i+1 < len(args) {
				flags = append(flags, args[i+1])
				i++
			}
			continue
		}
		positionals = append(positionals, args[i])
	}
	return append(flags, positionals...)
}

func splitCLIHubExecArgs(args []string) ([]string, []string) {
	for i, arg := range args {
		if strings.TrimSpace(arg) == "--" {
			return append([]string(nil), args[:i]...), append([]string(nil), args[i+1:]...)
		}
	}
	return append([]string(nil), args...), nil
}
