package main

import (
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
	default:
		printCLIUsage()
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

func printCLIUsage() {
	fmt.Print(`AnyClaw commands:
Usage:
  anyclaw mcp <subcommand>            Run MCP-related commands
`)
}
