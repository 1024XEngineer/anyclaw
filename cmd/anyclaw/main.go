package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

func main() {
	configureConsoleUTF8()

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
	case "config":
		return runConfigCommand(args[1:])
	case "plugin":
		return runPluginCommand(args[1:])
	case "channels":
		return runChannelsCommand(args[1:])
	case "status":
		return runStatusCommand(args[1:])
	case "health":
		return runHealthCommand(args[1:])
	case "sessions":
		return runSessionsCommand(args[1:])
	case "approvals":
		return runApprovalsCommand(args[1:])
	case "skill", "skills":
		return runSkillCommand(args[1:])
	case "doctor":
		return runDoctorCommand(args[1:])
	case "onboard", "setup":
		return runOnboardCommand(args[1:])
	case "gateway":
		return runGatewayCommand(context.Background(), args[1:])
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
  anyclaw config <subcommand>         Run config management commands
  anyclaw plugin <subcommand>         Run plugin management commands
  anyclaw channels <subcommand>       Run channels management commands
  anyclaw status [options]            Show gateway runtime status
  anyclaw health [options]            Show gateway health summary
  anyclaw sessions [options]          List recent sessions
  anyclaw approvals <subcommand>      Manage pending approvals
  anyclaw skill <subcommand>          Run skill management commands
  anyclaw doctor [options]            Run configuration diagnostics
  anyclaw onboard/setup [options]     Run first-run model onboarding
  anyclaw gateway <subcommand>        Run gateway management commands
`)
}

func printError(format string, args ...any) {
	fmt.Printf("Error: "+format+"\n", args...)
}

func printSuccess(format string, args ...any) {
	fmt.Printf(format+"\n", args...)
}

func printInfo(format string, args ...any) {
	fmt.Printf(format+"\n", args...)
}

func printWarn(format string, args ...any) {
	fmt.Printf("Warning: "+format+"\n", args...)
}

func writePrettyJSON(value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
