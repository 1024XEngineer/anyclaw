package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// runShellCommand provides a very lightweight CLI that executes a shell command
// from the AnyClaw binary. This is an MVP to allow command-line control of
// shell operations, with an optional dry-run mode.
func runShellCommand(args []string) error {
	fs := flag.NewFlagSet("shell", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	var (
		cmdStr = fs.String("execute", "", "shell command to execute")
		dryRun = fs.Bool("dry-run", false, "dry run: show command without executing")
		cwd    = fs.String("cwd", "", "working directory to execute in (optional)")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *cmdStr == "" {
		return fmt.Errorf("--execute is required")
	}

	if *dryRun {
		fmt.Printf("Dry-run: would execute in %q: %s\n", *cwd, *cmdStr)
		return nil
	}

	// Run in a context. We keep a very simple context here; the actual
	// sandboxing strategy can be layered later.
	ctx := context.Background()
	cmd := shellCommand(ctx, *cmdStr)
	if *cwd != "" {
		cmd.Dir = *cwd
	}
	output, err := cmd.CombinedOutput()
	if len(output) > 0 {
		// Print command output regardless of success for visibility
		fmt.Print(string(output))
	}
	if err != nil {
		return fmt.Errorf("command failed: %w", err)
	}
	return nil
}

// shellCommand mirrors the behavior used elsewhere in the project to invoke a
// shell command in a cross-platform way.
func shellCommand(ctx context.Context, command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "cmd", "/C", command)
	}
	return exec.CommandContext(ctx, "sh", "-c", command)
}
