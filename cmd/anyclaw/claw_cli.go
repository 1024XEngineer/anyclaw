package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/anyclaw/anyclaw/pkg/clawbridge"
)

func runClawCommand(args []string) error {
	if len(args) == 0 {
		printClawCommandHelp()
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
		printClawCommandHelp()
		return nil
	default:
		return fmt.Errorf("unknown claw command: %s", args[0])
	}
}

func printClawCommandHelp() {
	fmt.Print(`AnyClaw claw commands:

  anyclaw claw status
  anyclaw claw summary [--json]
  anyclaw claw lookup --section <summary|commands|tools|subsystems> [--family <name>] [--limit <n>] [--json]

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
	jsonFlag := fs.Bool("json", false, "print JSON")
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
			return printJSON(map[string]any{"available": false})
		}
		fmt.Println("claw-code-main bridge: unavailable")
		return nil
	}

	summary, err := clawbridge.Load(root)
	if err != nil {
		return err
	}
	if *jsonFlag {
		return printJSON(map[string]any{
			"available":       true,
			"root":            summary.Root,
			"commands_count":  summary.CommandsCount,
			"tools_count":     summary.ToolsCount,
			"subsystem_count": len(summary.Subsystems),
		})
	}
	fmt.Println("claw-code-main bridge: available")
	fmt.Println(clawbridge.HumanSummary(summary))
	return nil
}

func runClawSummary(args []string) error {
	fs := flag.NewFlagSet("claw summary", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	rootFlag := fs.String("root", "", "explicit claw-code-main root")
	workspaceFlag := fs.String("workspace", "", "workspace path used for discovery")
	jsonFlag := fs.Bool("json", false, "print JSON")
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
		return printLookupJSON(summary, "summary", "", *limitFlag)
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
	jsonFlag := fs.Bool("json", true, "print JSON")
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
		return printLookupJSON(summary, *sectionFlag, *familyFlag, *limitFlag)
	}
	rendered, err := clawbridge.RenderJSON(summary, *sectionFlag, *familyFlag, *limitFlag)
	if err != nil {
		return err
	}
	fmt.Println(rendered)
	return nil
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

func printLookupJSON(summary *clawbridge.Summary, section string, family string, limit int) error {
	rendered, err := clawbridge.RenderJSON(summary, section, family, limit)
	if err != nil {
		return err
	}
	fmt.Println(rendered)
	return nil
}

func printJSON(value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
