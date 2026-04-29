package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/clawbridge"
)

func runClawCommand(args []string) error {
	if len(args) == 0 {
		printClawUsage()
		return nil
	}

	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "status":
		return runClawStatus(args[1:])
	case "summary":
		return runClawSummary(args[1:])
	case "lookup":
		return runClawLookup(args[1:])
	case "help", "-h", "--help":
		printClawUsage()
		return nil
	default:
		printClawUsage()
		return fmt.Errorf("unknown claw command: %s", args[0])
	}
}

func printClawUsage() {
	fmt.Print(`AnyClaw claw commands:

Usage:
  anyclaw claw status [--json]
  anyclaw claw summary [--json] [--limit 6]
  anyclaw claw lookup [--section summary|commands|tools|subsystems] [--family <name>] [--limit 6]

Flags:
  --root <path>       Explicit claw-code-main root
  --workspace <path>  Start discovery from this workspace
`)
}

func runClawStatus(args []string) error {
	fs := flag.NewFlagSet("claw status", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	rootFlag := fs.String("root", "", "explicit claw-code-main root")
	workspaceFlag := fs.String("workspace", "", "workspace path used for discovery")
	jsonFlag := fs.Bool("json", false, "output JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	start, err := resolveClawStart(*rootFlag, *workspaceFlag)
	if err != nil {
		return err
	}

	root, ok := clawbridge.DiscoverRoot(start)
	if !ok {
		if *jsonFlag {
			return writePrettyJSON(map[string]any{"available": false})
		}
		printInfo("claw-code-main bridge: unavailable")
		return nil
	}

	summary, err := clawbridge.Load(root)
	if err != nil {
		return err
	}
	if *jsonFlag {
		return writePrettyJSON(map[string]any{
			"available":       true,
			"root":            summary.Root,
			"commands_count":  summary.CommandsCount,
			"tools_count":     summary.ToolsCount,
			"subsystem_count": len(summary.Subsystems),
		})
	}

	printSuccess("claw-code-main bridge: available")
	fmt.Println(clawbridge.HumanSummary(summary))
	return nil
}

func runClawSummary(args []string) error {
	fs := flag.NewFlagSet("claw summary", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	rootFlag := fs.String("root", "", "explicit claw-code-main root")
	workspaceFlag := fs.String("workspace", "", "workspace path used for discovery")
	jsonFlag := fs.Bool("json", false, "output JSON")
	limitFlag := fs.Int("limit", 6, "maximum items to show")
	if err := fs.Parse(args); err != nil {
		return err
	}

	root, err := resolveClawRoot(*rootFlag, *workspaceFlag)
	if err != nil {
		return err
	}
	summary, err := clawbridge.Load(root)
	if err != nil {
		return err
	}
	if *jsonFlag {
		return printClawLookupJSON(summary, "summary", "", *limitFlag)
	}

	fmt.Println(clawbridge.HumanSummary(summary))
	return nil
}

func runClawLookup(args []string) error {
	fs := flag.NewFlagSet("claw lookup", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	rootFlag := fs.String("root", "", "explicit claw-code-main root")
	workspaceFlag := fs.String("workspace", "", "workspace path used for discovery")
	sectionFlag := fs.String("section", "summary", "summary, commands, tools, or subsystems")
	familyFlag := fs.String("family", "", "family or subsystem name")
	limitFlag := fs.Int("limit", 6, "maximum items to show")
	if err := fs.Parse(args); err != nil {
		return err
	}

	root, err := resolveClawRoot(*rootFlag, *workspaceFlag)
	if err != nil {
		return err
	}
	summary, err := clawbridge.Load(root)
	if err != nil {
		return err
	}
	return printClawLookupJSON(summary, *sectionFlag, *familyFlag, *limitFlag)
}

func resolveClawStart(root string, workspace string) (string, error) {
	if strings.TrimSpace(root) != "" {
		return strings.TrimSpace(root), nil
	}
	if strings.TrimSpace(workspace) != "" {
		return strings.TrimSpace(workspace), nil
	}
	return os.Getwd()
}

func resolveClawRoot(root string, workspace string) (string, error) {
	start, err := resolveClawStart(root, workspace)
	if err != nil {
		return "", err
	}
	discovered, ok := clawbridge.DiscoverRoot(start)
	if !ok {
		return "", fmt.Errorf("claw-code-main reference not found; set %s or pass --root", clawbridge.EnvRoot)
	}
	return discovered, nil
}

func printClawLookupJSON(summary *clawbridge.Summary, section string, family string, limit int) error {
	rendered, err := clawbridge.RenderJSON(summary, section, family, limit)
	if err != nil {
		return err
	}
	fmt.Println(rendered)
	return nil
}
