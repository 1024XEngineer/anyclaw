package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/anyclaw/anyclaw/pkg/cron"
	"github.com/anyclaw/anyclaw/pkg/ui"
)

func runCronCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		printCronUsage()
		return nil
	}

	switch args[0] {
	case "run", "start":
		return runCronServer(ctx, args[1:])
	case "list":
		return listCronTasks(args[1:])
	case "add":
		return addCronTask(args[1:])
	case "delete", "remove":
		return deleteCronTask(args[1:])
	case "enable":
		return enableCronTask(args[1:])
	case "disable":
		return disableCronTask(args[1:])
	case "trigger":
		return runCronTaskNow(args[1:])
	case "history":
		return showCronHistory(args[1:])
	case "status":
		return showCronStatus(args[1:])
	default:
		printCronUsage()
		return fmt.Errorf("unknown cron command: %s", args[0])
	}
}

func runCronServer(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("cron run", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	fs.Parse(args)

	scheduler := cron.NewScheduler(nil)
	if err := scheduler.Start(); err != nil {
		return fmt.Errorf("start scheduler: %w", err)
	}

	fmt.Println(ui.Dim.Sprint(strings.Repeat("-", 50)))
	printSuccess("Cron server started")
	printInfo("Total tasks: %d", len(scheduler.ListTasks()))

	<-ctx.Done()
	scheduler.Stop()
	return nil
}

func listCronTasks(args []string) error {
	fs := flag.NewFlagSet("cron list", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	format := fs.String("format", "text", "output format: text, json")
	if err := fs.Parse(args); err != nil {
		return err
	}

	tasks := listTasksFromConfig()

	if *format == "json" {
		data, err := json.MarshalIndent(tasks, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	if len(tasks) == 0 {
		printInfo("No cron tasks configured")
		return nil
	}

	printSuccess("Cron tasks (%d):", len(tasks))
	for _, task := range tasks {
		status := ui.Red.Sprint("disabled")
		if task.Enabled {
			status = ui.Green.Sprint("enabled")
		}
		fmt.Printf("  %s%s%s %s\n", ui.Bold.Sprint(""), task.Name, ui.Reset.Sprint(""), status)
		fmt.Printf("    Schedule: %s\n", task.Schedule)
		fmt.Printf("    Command: %s\n", task.Command)
		if task.NextRun != nil {
			fmt.Printf("    Next run: %s\n", task.NextRun.Format(time.RFC3339))
		}
	}

	return nil
}

func addCronTask(args []string) error {
	fs := flag.NewFlagSet("cron add", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	name := fs.String("name", "", "task name")
	schedule := fs.String("schedule", "", "cron schedule")
	command := fs.String("command", "", "command to run")
	agent := fs.String("agent", "", "agent name")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *name == "" || *schedule == "" || *command == "" {
		return fmt.Errorf("name, schedule and command are required")
	}

	task := &cron.Task{
		Name:     *name,
		Schedule: *schedule,
		Command:  *command,
		Agent:    *agent,
	}

	if err := task.Validate(); err != nil {
		return err
	}

	scheduler := getCronScheduler()
	taskID, err := scheduler.AddTask(task)
	if err != nil {
		return err
	}

	printSuccess("Added cron task: %s (%s)", *name, taskID)
	return nil
}

func deleteCronTask(args []string) error {
	fs := flag.NewFlagSet("cron delete", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	taskID := fs.String("id", "", "task ID")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *taskID == "" {
		return fmt.Errorf("task id is required")
	}

	scheduler := getCronScheduler()
	if err := scheduler.DeleteTask(*taskID); err != nil {
		return err
	}

	printSuccess("Deleted cron task: %s", *taskID)
	return nil
}

func enableCronTask(args []string) error {
	fs := flag.NewFlagSet("cron enable", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	taskID := fs.String("id", "", "task ID")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *taskID == "" {
		return fmt.Errorf("task id is required")
	}

	scheduler := getCronScheduler()
	if err := scheduler.EnableTask(*taskID); err != nil {
		return err
	}

	printSuccess("Enabled cron task: %s", *taskID)
	return nil
}

func disableCronTask(args []string) error {
	fs := flag.NewFlagSet("cron disable", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	taskID := fs.String("id", "", "task ID")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *taskID == "" {
		return fmt.Errorf("task id is required")
	}

	scheduler := getCronScheduler()
	if err := scheduler.DisableTask(*taskID); err != nil {
		return err
	}

	printSuccess("Disabled cron task: %s", *taskID)
	return nil
}

func runCronTaskNow(args []string) error {
	fs := flag.NewFlagSet("cron run", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	taskID := fs.String("id", "", "task ID")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *taskID == "" {
		return fmt.Errorf("task id is required")
	}

	scheduler := getCronScheduler()
	if err := scheduler.RunTaskNow(*taskID); err != nil {
		return err
	}

	printSuccess("Triggered cron task: %s", *taskID)
	return nil
}

func showCronHistory(args []string) error {
	fs := flag.NewFlagSet("cron history", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	taskID := fs.String("id", "", "task ID")
	limit := fs.Int("limit", 10, "number of runs to show")
	if err := fs.Parse(args); err != nil {
		return err
	}

	scheduler := getCronScheduler()

	var runs []*cron.TaskRun
	if *taskID != "" {
		runs = scheduler.GetRunHistory(*taskID, *limit)
	} else {
		runs = scheduler.GetAllRuns(*limit)
	}

	if len(runs) == 0 {
		printInfo("No task runs found")
		return nil
	}

	printSuccess("Task runs (%d):", len(runs))
	for _, run := range runs {
		status := ui.Yellow.Sprint(run.Status)
		if run.Status == "success" {
			status = ui.Green.Sprint(run.Status)
		} else if run.Status == "failed" {
			status = ui.Red.Sprint(run.Status)
		}
		fmt.Printf("  %s - %s\n", run.ID, status)
		fmt.Printf("    Task: %s\n", run.TaskID)
		fmt.Printf("    Started: %s\n", run.StartTime.Format(time.RFC3339))
		if run.EndTime != nil {
			duration := run.EndTime.Sub(run.StartTime)
			fmt.Printf("    Duration: %v\n", duration)
		}
		if run.Error != "" {
			fmt.Printf("    Error: %s\n", run.Error)
		}
	}

	return nil
}

func showCronStatus(args []string) error {
	fs := flag.NewFlagSet("cron status", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	if err := fs.Parse(args); err != nil {
		return err
	}

	scheduler := getCronScheduler()

	stats := scheduler.Stats()

	printSuccess("Cron Status:")
	fmt.Printf("  Total tasks: %d\n", stats["total_tasks"])
	fmt.Printf("  Enabled tasks: %d\n", stats["enabled_tasks"])
	fmt.Printf("  Total runs: %d\n", stats["total_runs"])
	fmt.Printf("  Success: %d\n", stats["success_runs"])
	fmt.Printf("  Failed: %d\n", stats["failed_runs"])

	nextRuns := scheduler.NextRunTimes(5)
	if len(nextRuns) > 0 {
		fmt.Printf("\n  Next runs:\n")
		for id, next := range nextRuns {
			fmt.Printf("    %s: %s\n", id, next.Format(time.RFC3339))
		}
	}

	return nil
}

func getCronScheduler() *cron.Scheduler {
	return cron.New()
}

func listTasksFromConfig() []*cron.Task {
	data, err := os.ReadFile("anyclaw.json")
	if err != nil {
		return nil
	}

	var cfg struct {
		Cron struct {
			Tasks []*cron.Task `json:"tasks"`
		} `json:"cron"`
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil
	}

	if cfg.Cron.Tasks == nil || len(cfg.Cron.Tasks) == 0 {
		return nil
	}

	return cfg.Cron.Tasks
}

func printCronUsage() {
	fmt.Print(`AnyClaw cron commands:

Usage:
  anyclaw cron run [--config config.json]
  anyclaw cron list [--format text|json]
  anyclaw cron add --name <name> --schedule <cron> --command <cmd>
  anyclaw cron delete --id <task_id>
  anyclaw cron enable --id <task_id>
  anyclaw cron disable --id <task_id>
  anyclaw cron run --id <task_id>
  anyclaw cron history [--id <task_id>] [--limit N]
  anyclaw cron status

Examples:
  anyclaw cron add --name "daily-backup" --schedule "0 2 * * *" --command "backup"
  anyclaw cron add --name "hourly-check" --schedule "@hourly" --command "health-check"
  anyclaw cron list
  anyclaw cron history --limit 20
  anyclaw cron disable --id task-1234567890

Cron expressions:
  @yearly   - once a year (0 0 1 1 *)
  @monthly  - once a month (0 0 1 * *)
  @weekly   - once a week (0 0 * * 0)
  @daily    - once a day (0 0 * * *)
  @hourly   - once an hour (0 * * * *)
`)
}
