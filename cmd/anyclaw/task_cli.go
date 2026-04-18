package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/capability/catalogs"
	"github.com/1024XEngineer/anyclaw/pkg/input/cli/ui"
	appRuntime "github.com/1024XEngineer/anyclaw/pkg/runtime"
)

func runTaskCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		printTaskUsage()
		return nil
	}

	switch args[0] {
	case "run":
		return runTaskRun(ctx, args[1:])
	case "list":
		return runTaskList()
	default:
		printTaskUsage()
		return fmt.Errorf("unknown task command: %s", args[0])
	}
}

func printTaskUsage() {
	fmt.Print(`AnyClaw task commands:

Usage:
  anyclaw task run <description>
  anyclaw task run --agent <name> <description>
  anyclaw task list
`)
}

func runTaskRun(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("task run", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	multi := fs.Bool("multi", false, "multi-agent mode")
	agentName := fs.String("agent", "", "selected agent")
	if err := fs.Parse(args); err != nil {
		return err
	}

	input := strings.Join(fs.Args(), " ")
	if input == "" {
		return fmt.Errorf("please provide a task description")
	}
	if *multi {
		return fmt.Errorf("multi-agent mode has been removed; run the task with a single agent")
	}

	app, err := appRuntime.NewTargetApp("anyclaw.json", *agentName, "")
	if err != nil {
		return fmt.Errorf("bootstrap failed: %w", err)
	}
	selectedAgent := app.Config.Agent.Name
	if *agentName != "" {
		selectedAgent = *agentName
	}
	fmt.Printf("%s %s\n", ui.Bold.Sprint("Single-agent mode:"), selectedAgent)
	fmt.Printf("Task: %s\n\n", input)

	fmt.Printf("%s Running...\n\n", ui.Cyan.Sprint(">"))
	result, err := app.Agent.Run(ctx, input)
	if err != nil {
		printError("%v", err)
		return nil
	}

	fmt.Printf("%s\n", result)
	return nil
}

func runTaskList() error {
	sm, err := agentstore.NewStoreManager(".anyclaw", "anyclaw.json")
	if err != nil {
		_ = sm
	}
	fmt.Println("Task listing currently requires a running gateway")
	fmt.Println("Run: anyclaw gateway run")
	return nil
}
