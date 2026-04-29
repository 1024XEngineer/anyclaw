package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/capability/tools"
	"github.com/1024XEngineer/anyclaw/pkg/config"
)

func runShellCommand(args []string) error {
	fs := flag.NewFlagSet("shell", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	command := fs.String("execute", "", "shell command to execute")
	cwd := fs.String("cwd", "", "working directory override")
	shellName := fs.String("shell", "auto", "shell to use: auto, cmd, powershell, pwsh, sh, or bash")
	mode := fs.String("mode", "", "execution mode override: sandbox or host-reviewed")
	timeoutSeconds := fs.Int("timeout", 0, "command timeout in seconds")
	dryRun := fs.Bool("dry-run", false, "print the planned execution without running it")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cmdStr := strings.TrimSpace(*command)
	if cmdStr == "" {
		return fmt.Errorf("--execute is required")
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	if override := strings.TrimSpace(*mode); override != "" {
		cfg.Sandbox.ExecutionMode = override
	}
	if *timeoutSeconds > 0 {
		cfg.Security.CommandTimeoutSeconds = *timeoutSeconds
	}

	workingDir := config.ResolvePath(*configPath, cfg.Agent.WorkingDir)
	if workingDir == "" {
		workingDir = config.ResolvePath(*configPath, ".anyclaw")
	}
	executionCwd := strings.TrimSpace(*cwd)
	if executionCwd != "" {
		executionCwd = config.ResolvePath(*configPath, executionCwd)
	}

	if *dryRun {
		fmt.Printf("Dry-run: would execute in %q with shell %q under mode %q: %s\n", firstNonEmptyShellDir(executionCwd, workingDir), *shellName, cfg.Sandbox.ExecutionMode, cmdStr)
		return nil
	}

	sandboxManager := tools.NewSandboxManager(cfg.Sandbox, workingDir)
	output, err := tools.RunCommandToolWithPolicy(context.Background(), map[string]any{
		"command": cmdStr,
		"cwd":     executionCwd,
		"shell":   *shellName,
	}, tools.BuiltinOptions{
		WorkingDir:            workingDir,
		PermissionLevel:       cfg.Agent.PermissionLevel,
		ExecutionMode:         cfg.Sandbox.ExecutionMode,
		DangerousPatterns:     cfg.Security.DangerousCommandPatterns,
		ProtectedPaths:        cfg.Security.ProtectedPaths,
		AllowedReadPaths:      cfg.Security.AllowedReadPaths,
		AllowedWritePaths:     cfg.Security.AllowedWritePaths,
		CommandTimeoutSeconds: cfg.Security.CommandTimeoutSeconds,
		Sandbox:               sandboxManager,
		ConfirmDangerousCommand: func(command string) bool {
			if !cfg.Agent.RequireConfirmationForDangerous {
				return true
			}
			fmt.Printf("Dangerous command detected: %s\n", command)
			fmt.Print("Execute anyway? (y/N): ")
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

func firstNonEmptyShellDir(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
