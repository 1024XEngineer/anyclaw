package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/anyclaw/anyclaw/pkg/agentstore"
	appRuntime "github.com/anyclaw/anyclaw/pkg/runtime"
	taskModule "github.com/anyclaw/anyclaw/pkg/task"
	"github.com/anyclaw/anyclaw/pkg/ui"
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
  anyclaw task run --multi <description>
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

	app, err := appRuntime.Bootstrap(appRuntime.BootstrapOptions{
		ConfigPath: "anyclaw.json",
		Progress:   func(ev appRuntime.BootEvent) {},
	})
	if err != nil {
		return fmt.Errorf("bootstrap failed: %w", err)
	}

	if app.Orchestrator == nil {
		return fmt.Errorf("orchestrator is disabled; set orchestrator.enabled=true in anyclaw.json")
	}

	taskMgr := taskModule.NewTaskManager(app.Orchestrator)

	mode := taskModule.ModeSingle
	if *multi {
		mode = taskModule.ModeMulti
	}

	req := taskModule.TaskRequest{
		Input:         input,
		Mode:          mode,
		SelectedAgent: *agentName,
	}

	if *multi {
		fmt.Printf("%s\n", ui.Bold.Sprint("Multi-agent mode"))
	} else if *agentName != "" {
		fmt.Printf("%s %s\n", ui.Bold.Sprint("Single-agent mode:"), *agentName)
	} else {
		fmt.Printf("%s\n", ui.Bold.Sprint("Single-agent mode"))
	}
	fmt.Printf("Task: %s\n\n", input)

	task, err := taskMgr.CreateTask(req)
	if err != nil {
		return err
	}

	fmt.Printf("%s Running...\n\n", ui.Cyan.Sprint(">"))
	result, err := taskMgr.ExecuteTask(ctx, task.ID)
	if err != nil {
		if result != nil && result.Output != "" {
			fmt.Printf("%s\n\n", result.Output)
		}
		printError("%v", err)
		return nil
	}

	fmt.Printf("%s\n", result.Output)
	fmt.Printf("\n%s Duration: %s\n", ui.Dim.Sprint(""), result.Duration)
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
