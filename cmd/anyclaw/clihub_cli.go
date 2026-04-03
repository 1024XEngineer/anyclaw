package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/anyclaw/anyclaw/pkg/clihub"
)

func runCLIHubCommand(args []string) error {
	if len(args) == 0 {
		printCLIHubUsage()
		return nil
	}

	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "search":
		return runCLIHubSearch(args[1:])
	case "installed":
		return runCLIHubInstalled(args[1:])
	case "info":
		return runCLIHubInfo(args[1:])
	case "help", "-h", "--help":
		printCLIHubUsage()
		return nil
	default:
		return fmt.Errorf("unknown clihub command: %s", args[0])
	}
}

func printCLIHubUsage() {
	fmt.Print(`AnyClaw clihub commands:

Usage:
  anyclaw clihub search [query] [--category <name>] [--installed] [--json]
  anyclaw clihub installed [--json]
  anyclaw clihub info <name> [--json]

Flags:
  --root <path>       Explicit CLI-Anything root
  --workspace <path>  Start discovery from this workspace
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
		installState := "not installed"
		if item.Installed {
			installState = "installed"
		}
		fmt.Printf("- %s (%s)\n", firstNonEmptyCLIHub(item.DisplayName, item.Name), installState)
		fmt.Printf("  %s\n", strings.TrimSpace(item.Description))
		if strings.TrimSpace(item.InstallCmd) != "" {
			fmt.Printf("  install: %s\n", strings.TrimSpace(item.InstallCmd))
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
	fmt.Printf("Description: %s\n", item.Description)
	if strings.TrimSpace(item.Requires) != "" {
		fmt.Printf("Requires: %s\n", item.Requires)
	}
	if strings.TrimSpace(item.InstallCmd) != "" {
		fmt.Printf("Install: %s\n", item.InstallCmd)
	}
	return nil
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
