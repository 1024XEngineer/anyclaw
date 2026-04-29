package main

import (
	"context"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/input/cli/ui"
	appRuntime "github.com/1024XEngineer/anyclaw/pkg/runtime"
)

func runTaskCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		printTaskUsage()
		return nil
	}

	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "run":
		return runTaskRun(ctx, args[1:])
	case "list", "ls":
		return runTaskList(args[1:])
	default:
		printTaskUsage()
		return fmt.Errorf("unknown task command: %s", args[0])
	}
}

func runTaskRun(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("task run", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	agentName := fs.String("agent", "", "agent profile name")
	workingDir := fs.String("cwd", "", "working directory override")
	if err := fs.Parse(args); err != nil {
		return err
	}

	input := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if input == "" {
		return fmt.Errorf("please provide a task description")
	}

	app, err := appRuntime.NewTargetApp(*configPath, *agentName, *workingDir)
	if err != nil {
		return fmt.Errorf("bootstrap failed: %w", err)
	}
	defer app.Close()

	fmt.Printf("%s %s\n", ui.Bold.Sprint("Agent:"), app.Config.Agent.Name)
	fmt.Printf("Task: %s\n\n", input)

	result, err := app.Agent.Run(ctx, input)
	if err != nil {
		return err
	}
	fmt.Println(result)
	return nil
}

func runTaskList(args []string) error {
	fs := flag.NewFlagSet("task list", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	jsonOut := fs.Bool("json", false, "output JSON")
	status := fs.String("status", "", "filter by task status")
	assistant := fs.String("assistant", "", "filter by assistant")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := loadGatewayConfig(*configPath)
	if err != nil {
		return err
	}

	path := "/v2/tasks"
	query := url.Values{}
	query.Set("status", *status)
	query.Set("assistant", *assistant)
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}

	ctx, cancel := newGatewayRequestContext()
	defer cancel()
	var tasks []map[string]any
	if err := doGatewayJSONRequest(ctx, cfg, httpMethodGet, path, nil, &tasks); err != nil {
		return err
	}

	if *jsonOut {
		return writePrettyJSON(map[string]any{
			"count": len(tasks),
			"tasks": tasks,
		})
	}
	if len(tasks) == 0 {
		printInfo("No tasks found")
		return nil
	}
	printSuccess("Found %d task(s)", len(tasks))
	for _, task := range tasks {
		fmt.Printf("  - %v", task["id"])
		if title := strings.TrimSpace(fmt.Sprint(task["title"])); title != "" && title != "<nil>" {
			fmt.Printf(" %s", title)
		}
		if status := strings.TrimSpace(fmt.Sprint(task["status"])); status != "" && status != "<nil>" {
			fmt.Printf(" [%s]", status)
		}
		fmt.Println()
	}
	return nil
}

func printTaskUsage() {
	fmt.Print(`AnyClaw task commands:

Usage:
  anyclaw task run [--config anyclaw.json] [--agent <name>] <description>
  anyclaw task list [--config anyclaw.json] [--status queued] [--assistant <name>] [--json]
`)
}
