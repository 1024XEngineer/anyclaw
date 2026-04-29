package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

func main() {
	configureConsoleUTF8()

	if err := runAnyClawCLI(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runAnyClawCLI(args []string) error {
	ctx, stop := newSignalContext()
	defer stop()
	return runAnyClawCLIWithContext(ctx, args)
}

func runAnyClawCLIWithContext(ctx context.Context, args []string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(args) == 0 {
		printCLIUsage()
		return nil
	}

	command := normalizeRootCommand(args[0])
	switch command {
	case "help", "-h", "--help":
		printCLIUsage()
		return nil
	case "agent":
		return runAgentCommand(ctx, args[1:])
	case "mcp":
		return runMCPCommand(args[1:])
	case "models":
		return runModelsCommand(args[1:])
	case "config":
		return runConfigCommand(args[1:])
	case "clihub":
		return runCLIHubCommand(args[1:])
	case "plugin":
		return runPluginCommand(args[1:])
	case "channels":
		return runChannelsCommand(args[1:])
	case "task":
		return runTaskCommand(ctx, args[1:])
	case "shell":
		return runShellCommand(args[1:])
	case "cron":
		return runCronCommand(ctx, args[1:])
	case "pi":
		return runPiCommand(ctx, args[1:])
	case "store":
		return runStoreCommand(args[1:])
	case "claw":
		return runClawCommand(args[1:])
	case "pairing":
		return runPairingCommand(ctx, args[1:])
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
		return runGatewayCommand(ctx, args[1:])
	default:
		printCLIUsage()
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

func newSignalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

func normalizeRootCommand(command string) string {
	switch strings.ToLower(strings.TrimSpace(command)) {
	case "skills":
		return "skill"
	case "plugins":
		return "plugin"
	case "agents":
		return "agent"
	case "channel":
		return "channels"
	case "session":
		return "sessions"
	case "approval":
		return "approvals"
	case "model":
		return "models"
	case "setup":
		return "onboard"
	case "tasks":
		return "task"
	default:
		return strings.ToLower(strings.TrimSpace(command))
	}
}

func printCLIUsage() {
	fmt.Print(`AnyClaw commands:
Usage:
  anyclaw agent <subcommand>          Manage and run configured agents
  anyclaw mcp <subcommand>            Run MCP-related commands
  anyclaw models <subcommand>         Run model management commands
  anyclaw config <subcommand>         Run config management commands
  anyclaw clihub <subcommand>         Run CLI Hub commands
  anyclaw plugin <subcommand>         Run plugin management commands
  anyclaw channels <subcommand>       Run channels management commands
  anyclaw task <subcommand>           Run one-shot agent tasks
  anyclaw shell --execute <command>    Execute a reviewed shell command
  anyclaw cron <subcommand>           Manage local cron tasks
  anyclaw pi <subcommand>             Run Pi Agent RPC commands
  anyclaw store <subcommand>          Browse and install agent packages
  anyclaw claw <subcommand>           Inspect claw-code-main bridge reference data
  anyclaw pairing <subcommand>        Manage Gateway device pairing
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
