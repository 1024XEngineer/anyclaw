package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/config"
	"github.com/1024XEngineer/anyclaw/pkg/input/cli/ui"
	appRuntime "github.com/1024XEngineer/anyclaw/pkg/runtime"
	cron "github.com/1024XEngineer/anyclaw/pkg/runtime/execution/schedule"
)

func runCronCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		printCronUsage()
		return nil
	}

	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "run", "start":
		return runCronServer(ctx, args[1:])
	case "list", "ls":
		return listCronTasks(args[1:])
	case "add":
		return addCronTask(args[1:])
	case "delete", "remove", "rm":
		return deleteCronTask(args[1:])
	case "enable":
		return setCronTaskEnabled(args[1:], true)
	case "disable":
		return setCronTaskEnabled(args[1:], false)
	case "trigger", "now":
		return runCronTaskNow(args[1:])
	case "history":
		return showCronHistory(args[1:])
	case "status", "stats":
		return showCronStatus(args[1:])
	default:
		printCronUsage()
		return fmt.Errorf("unknown cron command: %s", args[0])
	}
}

func runCronServer(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("cron run", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	app, err := appRuntime.Bootstrap(appRuntime.BootstrapOptions{ConfigPath: *configPath})
	if err != nil {
		return fmt.Errorf("cron bootstrap failed: %w", err)
	}
	defer app.Close()

	scheduler, _, err := loadCronScheduler(*configPath, app.Config, app.NewCronExecutor())
	if err != nil {
		return err
	}
	if err := scheduler.Start(); err != nil {
		return fmt.Errorf("start scheduler: %w", err)
	}
	defer scheduler.Stop()

	fmt.Println(ui.Dim.Sprint(strings.Repeat("-", 50)))
	printSuccess("Cron scheduler started")
	printInfo("Total tasks: %d", len(scheduler.ListTasks()))

	<-ctx.Done()
	return nil
}

func listCronTasks(args []string) error {
	fs := flag.NewFlagSet("cron list", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	format := fs.String("format", "text", "output format: text, json")
	if err := fs.Parse(args); err != nil {
		return err
	}

	scheduler, _, err := loadCronSchedulerFromConfig(*configPath)
	if err != nil {
		return err
	}
	tasks := scheduler.ListTasks()
	if strings.EqualFold(*format, "json") {
		return writePrettyJSON(tasks)
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
		fmt.Printf("    ID: %s\n", task.ID)
		fmt.Printf("    Schedule: %s\n", task.Schedule)
		fmt.Printf("    Command: %s\n", task.Command)
		if task.Agent != "" {
			fmt.Printf("    Agent: %s\n", task.Agent)
		}
		if task.NextRun != nil {
			fmt.Printf("    Next run: %s\n", task.NextRun.Format(time.RFC3339))
		}
	}
	return nil
}

func addCronTask(args []string) error {
	fs := flag.NewFlagSet("cron add", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	name := fs.String("name", "", "task name")
	schedule := fs.String("schedule", "", "cron schedule")
	command := fs.String("command", "", "command to run")
	agentName := fs.String("agent", "", "agent profile name")
	workspace := fs.String("workspace", "", "working directory override")
	timeout := fs.Int("timeout", 0, "timeout in seconds")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if strings.TrimSpace(*name) == "" || strings.TrimSpace(*schedule) == "" || strings.TrimSpace(*command) == "" {
		return fmt.Errorf("name, schedule and command are required")
	}

	scheduler, persister, err := loadCronSchedulerFromConfig(*configPath)
	if err != nil {
		return err
	}
	task := &cron.Task{
		Name:      strings.TrimSpace(*name),
		Schedule:  strings.TrimSpace(*schedule),
		Command:   strings.TrimSpace(*command),
		Agent:     strings.TrimSpace(*agentName),
		Workspace: strings.TrimSpace(*workspace),
		Timeout:   *timeout,
	}
	task.Input = cronTaskInput(task)
	if err := task.Validate(); err != nil {
		return err
	}
	taskID, err := scheduler.AddTask(task)
	if err != nil {
		return err
	}
	if err := persister.SaveTasks(scheduler.ListTasks()); err != nil {
		return err
	}

	printSuccess("Added cron task: %s (%s)", task.Name, taskID)
	return nil
}

func deleteCronTask(args []string) error {
	fs := flag.NewFlagSet("cron delete", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	taskID := fs.String("id", "", "task ID")
	if err := fs.Parse(args); err != nil {
		return err
	}

	id := strings.TrimSpace(firstNonEmptyCronArg(*taskID, strings.Join(fs.Args(), " ")))
	if id == "" {
		return fmt.Errorf("task id is required")
	}

	scheduler, persister, err := loadCronSchedulerFromConfig(*configPath)
	if err != nil {
		return err
	}
	if err := scheduler.DeleteTask(id); err != nil {
		return err
	}
	if err := persister.SaveTasks(scheduler.ListTasks()); err != nil {
		return err
	}

	printSuccess("Deleted cron task: %s", id)
	return nil
}

func setCronTaskEnabled(args []string, enabled bool) error {
	name := "cron disable"
	if enabled {
		name = "cron enable"
	}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	taskID := fs.String("id", "", "task ID")
	if err := fs.Parse(args); err != nil {
		return err
	}

	id := strings.TrimSpace(firstNonEmptyCronArg(*taskID, strings.Join(fs.Args(), " ")))
	if id == "" {
		return fmt.Errorf("task id is required")
	}

	scheduler, persister, err := loadCronSchedulerFromConfig(*configPath)
	if err != nil {
		return err
	}
	if enabled {
		err = scheduler.EnableTask(id)
	} else {
		err = scheduler.DisableTask(id)
	}
	if err != nil {
		return err
	}
	if err := persister.SaveTasks(scheduler.ListTasks()); err != nil {
		return err
	}

	if enabled {
		printSuccess("Enabled cron task: %s", id)
	} else {
		printSuccess("Disabled cron task: %s", id)
	}
	return nil
}

func runCronTaskNow(args []string) error {
	fs := flag.NewFlagSet("cron trigger", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	taskID := fs.String("id", "", "task ID")
	wait := fs.Bool("wait", false, "wait briefly for task completion")
	if err := fs.Parse(args); err != nil {
		return err
	}

	id := strings.TrimSpace(firstNonEmptyCronArg(*taskID, strings.Join(fs.Args(), " ")))
	if id == "" {
		return fmt.Errorf("task id is required")
	}

	app, err := appRuntime.Bootstrap(appRuntime.BootstrapOptions{ConfigPath: *configPath})
	if err != nil {
		return fmt.Errorf("cron bootstrap failed: %w", err)
	}
	defer app.Close()

	scheduler, _, err := loadCronScheduler(*configPath, app.Config, app.NewCronExecutor())
	if err != nil {
		return err
	}
	if err := scheduler.RunTaskNow(id); err != nil {
		return err
	}
	if *wait {
		time.Sleep(250 * time.Millisecond)
	}

	printSuccess("Triggered cron task: %s", id)
	return nil
}

func showCronHistory(args []string) error {
	fs := flag.NewFlagSet("cron history", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	taskID := fs.String("id", "", "task ID")
	limit := fs.Int("limit", 10, "number of runs to show")
	jsonOut := fs.Bool("json", false, "output JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	scheduler, _, err := loadCronSchedulerFromConfig(*configPath)
	if err != nil {
		return err
	}

	var runs []*cron.TaskRun
	if strings.TrimSpace(*taskID) != "" {
		runs = scheduler.GetRunHistory(strings.TrimSpace(*taskID), *limit)
	} else {
		runs = scheduler.GetAllRuns(*limit)
	}
	if *jsonOut {
		return writePrettyJSON(runs)
	}
	if len(runs) == 0 {
		printInfo("No task runs found")
		return nil
	}
	printSuccess("Task runs (%d):", len(runs))
	for _, run := range runs {
		fmt.Printf("  - %s task=%s status=%s started=%s\n", run.ID, run.TaskID, run.Status, run.StartTime.Format(time.RFC3339))
		if run.Error != "" {
			fmt.Printf("    error=%s\n", run.Error)
		}
	}
	return nil
}

func showCronStatus(args []string) error {
	fs := flag.NewFlagSet("cron status", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	jsonOut := fs.Bool("json", false, "output JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	scheduler, _, err := loadCronSchedulerFromConfig(*configPath)
	if err != nil {
		return err
	}
	stats := scheduler.Stats()
	if *jsonOut {
		payload := map[string]any{
			"stats":     stats,
			"next_runs": scheduler.NextRunTimes(5),
		}
		return writePrettyJSON(payload)
	}

	printSuccess("Cron Status:")
	fmt.Printf("  Total tasks: %v\n", stats["total_tasks"])
	fmt.Printf("  Enabled tasks: %v\n", stats["enabled_tasks"])
	fmt.Printf("  Total runs: %v\n", stats["total_runs"])
	fmt.Printf("  Success: %v\n", stats["success_runs"])
	fmt.Printf("  Failed: %v\n", stats["failed_runs"])
	return nil
}

func loadCronSchedulerFromConfig(configPath string) (*cron.Scheduler, *cron.FilePersister, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, nil, err
	}
	return loadCronScheduler(configPath, cfg, nil)
}

func loadCronScheduler(configPath string, cfg *config.Config, executor cron.Executor) (*cron.Scheduler, *cron.FilePersister, error) {
	baseDir := config.ResolvePath(configPath, filepath.Join(cfg.Agent.WorkDir, "cron"))
	if strings.TrimSpace(cfg.Agent.WorkDir) == "" {
		baseDir = config.ResolvePath(configPath, filepath.Join(".anyclaw", "cron"))
	}
	persister, err := cron.NewFilePersister(baseDir)
	if err != nil {
		return nil, nil, err
	}
	scheduler := cron.NewScheduler(executor)
	scheduler.SetPersister(persister)
	if err := scheduler.LoadPersisted(); err != nil {
		return nil, nil, err
	}
	return scheduler, persister, nil
}

func firstNonEmptyCronArg(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func cronTaskInput(task *cron.Task) map[string]interface{} {
	if task == nil {
		return nil
	}
	input := map[string]interface{}{}
	if strings.TrimSpace(task.Agent) != "" {
		input["agent"] = strings.TrimSpace(task.Agent)
	}
	if strings.TrimSpace(task.Workspace) != "" {
		input["workspace"] = strings.TrimSpace(task.Workspace)
	}
	if len(input) == 0 {
		return nil
	}
	return input
}

func printCronUsage() {
	fmt.Print(`AnyClaw cron commands:

Usage:
  anyclaw cron run [--config anyclaw.json]
  anyclaw cron list [--config anyclaw.json] [--format text|json]
  anyclaw cron add --name <name> --schedule <cron> --command <cmd> [--agent <name>]
  anyclaw cron delete --id <task_id>
  anyclaw cron enable --id <task_id>
  anyclaw cron disable --id <task_id>
  anyclaw cron trigger --id <task_id> [--wait]
  anyclaw cron history [--id <task_id>] [--limit N] [--json]
  anyclaw cron status [--json]

Cron expressions:
  @yearly, @monthly, @weekly, @daily, @hourly, or five-field numeric cron
`)
}
