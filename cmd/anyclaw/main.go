package main

import (
	"encoding/json"
	"fmt"
	"os"
)

func main() {
	if err := runAnyClawCLI(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runAnyClawCLI(args []string) error {
	if len(args) == 0 {
		printCLIUsage()
		return nil
	}

	switch args[0] {
	case "help", "-h", "--help":
		printCLIUsage()
		return nil
	case "mcp":
		return runMCPCommand(args[1:])
	case "models":
		return runModelsCommand(args[1:])
	case "plugin":
		return runPluginCommand(args[1:])
	case "skill", "skills":
		return runSkillCommand(args[1:])
	default:
		printCLIUsage()
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

func printCLIUsage() {
	fmt.Print(`AnyClaw commands:
Usage:
  anyclaw mcp <subcommand>            Run MCP-related commands
  anyclaw models <subcommand>         Run model management commands
  anyclaw plugin <subcommand>         Run plugin management commands
  anyclaw skill <subcommand>          Run skill management commands
`)
}

func printSuccess(format string, args ...any) {
	fmt.Printf(format+"\n", args...)
}

func printInfo(format string, args ...any) {
	fmt.Printf(format+"\n", args...)
}

func writePrettyJSON(value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
