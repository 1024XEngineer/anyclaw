package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/config"
	cron "github.com/1024XEngineer/anyclaw/pkg/runtime/execution/schedule"
)

func TestNormalizeRootCommandSupportsOperationalAliases(t *testing.T) {
	tests := map[string]string{
		"skill":    "skill",
		"skills":   "skill",
		"plugin":   "plugin",
		"plugins":  "plugin",
		"agent":    "agent",
		"agents":   "agent",
		"channel":  "channels",
		"session":  "sessions",
		"approval": "approvals",
		"model":    "models",
		"setup":    "onboard",
		"task":     "task",
		"tasks":    "task",
		"shell":    "shell",
		"cron":     "cron",
		"pi":       "pi",
	}

	for input, want := range tests {
		if got := normalizeRootCommand(input); got != want {
			t.Fatalf("normalizeRootCommand(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestCLIUsageIncludesOperationalCommands(t *testing.T) {
	stdout, _, err := captureCLIOutput(t, func() error {
		return runAnyClawCLI([]string{"help"})
	})
	if err != nil {
		t.Fatalf("runAnyClawCLI help: %v", err)
	}

	for _, want := range []string{
		"anyclaw agent <subcommand>",
		"anyclaw cron <subcommand>",
		"anyclaw pi <subcommand>",
		"anyclaw shell --execute",
		"anyclaw task <subcommand>",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected %q in CLI usage, got %q", want, stdout)
		}
	}
}

func TestNewSignalContextStopCancelsContext(t *testing.T) {
	ctx, stop := newSignalContext()
	stop()

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("expected signal context stop function to cancel context")
	}
}

func TestOperationalCommandsPrintUsage(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{args: []string{"agent"}, want: "AnyClaw agent commands:"},
		{args: []string{"cron"}, want: "AnyClaw cron commands:"},
		{args: []string{"pi"}, want: "AnyClaw Pi Agent commands:"},
		{args: []string{"task"}, want: "AnyClaw task commands:"},
	}

	for _, tc := range tests {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			stdout, _, err := captureCLIOutput(t, func() error {
				return runAnyClawCLI(tc.args)
			})
			if err != nil {
				t.Fatalf("runAnyClawCLI %v: %v", tc.args, err)
			}
			if !strings.Contains(stdout, tc.want) {
				t.Fatalf("expected %q in output, got %q", tc.want, stdout)
			}
		})
	}
}

func TestShellDryRunUsesConfigWithoutExecuting(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Agent.WorkDir = t.TempDir()
	cfg.Sandbox.ExecutionMode = "host-reviewed"
	configPath := filepath.Join(t.TempDir(), "anyclaw.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save config: %v", err)
	}

	stdout, _, err := captureCLIOutput(t, func() error {
		return runAnyClawCLI([]string{"shell", "--config", configPath, "--execute", "echo should-not-run", "--dry-run", "--cwd", cfg.Agent.WorkDir})
	})
	if err != nil {
		t.Fatalf("run shell dry-run: %v", err)
	}
	if !strings.Contains(stdout, "Dry-run: would execute") || !strings.Contains(stdout, "echo should-not-run") {
		t.Fatalf("unexpected dry-run output: %q", stdout)
	}
}

func TestCronAddPersistsAgentInput(t *testing.T) {
	workDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Agent.WorkDir = workDir
	configPath := filepath.Join(t.TempDir(), "anyclaw.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save config: %v", err)
	}

	_, _, err := captureCLIOutput(t, func() error {
		return runAnyClawCLI([]string{
			"cron", "add",
			"--config", configPath,
			"--name", "hourly-check",
			"--schedule", "@hourly",
			"--command", "check status",
			"--agent", "reviewer",
		})
	})
	if err != nil {
		t.Fatalf("cron add: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(workDir, "cron", "tasks", "tasks.json"))
	if err != nil {
		t.Fatalf("read tasks file: %v", err)
	}
	var tasks []*cron.Task
	if err := json.Unmarshal(data, &tasks); err != nil {
		t.Fatalf("unmarshal tasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("tasks len = %d, want 1", len(tasks))
	}
	if got := tasks[0].Input["agent"]; got != "reviewer" {
		t.Fatalf("persisted input agent = %v, want reviewer", got)
	}
}
