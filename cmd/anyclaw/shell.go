package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/anyclaw/anyclaw/pkg/config"
	"github.com/anyclaw/anyclaw/pkg/tools"
)

// runShellCommand provides a very lightweight CLI that executes a shell command
// from the AnyClaw binary. This is an MVP to allow command-line control of
// shell operations, with an optional dry-run mode.
func runShellCommand(args []string) error {
	fs := flag.NewFlagSet("shell", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	var (
		configPath  = fs.String("config", "anyclaw.json", "path to config file")
		cmdStr      = fs.String("execute", "", "shell command to execute")
		dryRun      = fs.Bool("dry-run", false, "dry run: show command without executing")
		cwd         = fs.String("cwd", "", "working directory to execute in (optional)")
		shellName   = fs.String("shell", "auto", "shell to use: auto, cmd, powershell, pwsh, sh, bash")
		timeoutSecs = fs.Int("timeout", 0, "optional timeout in seconds")
		mode        = fs.String("mode", "", "execution mode override: sandbox or host-reviewed")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *cmdStr == "" {
		return fmt.Errorf("--execute is required")
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	if strings.TrimSpace(*mode) != "" {
		cfg.Sandbox.ExecutionMode = strings.TrimSpace(*mode)
	}
	if *timeoutSecs > 0 {
		cfg.Security.CommandTimeoutSeconds = *timeoutSecs
	}

	if *dryRun {
		fmt.Printf("Dry-run: would execute in %q with shell %q under mode %q: %s\n", *cwd, *shellName, cfg.Sandbox.ExecutionMode, *cmdStr)
		return nil
	}

	workingDir := cfg.Agent.WorkingDir
	sandboxManager := tools.NewSandboxManager(cfg.Sandbox, workingDir)
	output, err := tools.RunCommandToolWithPolicy(context.Background(), map[string]any{
		"command": *cmdStr,
		"cwd":     *cwd,
		"shell":   *shellName,
	}, tools.BuiltinOptions{
		WorkingDir:            workingDir,
		PermissionLevel:       cfg.Agent.PermissionLevel,
		ExecutionMode:         cfg.Sandbox.ExecutionMode,
		DangerousPatterns:     cfg.Security.DangerousCommandPatterns,
		ProtectedPaths:        cfg.Security.ProtectedPaths,
		CommandTimeoutSeconds: cfg.Security.CommandTimeoutSeconds,
		Sandbox:               sandboxManager,
		ConfirmDangerousCommand: func(command string) bool {
			if !cfg.Agent.RequireConfirmationForDangerous {
				return true
			}
			fmt.Printf("检测到高风险命令: %s\n", command)
			fmt.Printf("确认执行? (y/N): ")
			var confirm string
			_, _ = fmt.Scanln(&confirm)
			return strings.EqualFold(strings.TrimSpace(confirm), "y")
		},
	})
	if output != "" {
		fmt.Print(output)
	}
	return err
}
