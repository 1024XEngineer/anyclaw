package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/config"
)

var runGatewayControlUIBuild = func(ctx context.Context, repoRoot string) error {
	node, err := exec.LookPath("node")
	if err != nil {
		return fmt.Errorf("automatic control UI build requires Node.js in PATH: %w", err)
	}

	cmd := exec.CommandContext(ctx, node, filepath.Join(repoRoot, "scripts", "ui.mjs"), "build")
	cmd.Dir = repoRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run control UI build: %w", err)
	}
	return nil
}

func ensureGatewayControlUIBuilt(ctx context.Context, configPath string) error {
	if envRoot := strings.TrimSpace(os.Getenv("ANYCLAW_CONTROL_UI_ROOT")); envRoot != "" {
		if controlUIBuildExists(envRoot) {
			return nil
		}
		return fmt.Errorf("ANYCLAW_CONTROL_UI_ROOT points to a missing control UI build: %s", envRoot)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config for control UI: %w", err)
	}

	configuredRoot := config.ResolvePath(configPath, cfg.Gateway.ControlUI.Root)
	if controlUIBuildExists(configuredRoot) {
		return nil
	}

	repoRoot, ok := discoverGatewayRepoRoot(configPath)
	if !ok {
		if configuredRoot != "" {
			return fmt.Errorf("configured control UI root is missing: %s", configuredRoot)
		}
		return fmt.Errorf("control UI build is missing; run `corepack pnpm -C ui build` from the repo root or set ANYCLAW_CONTROL_UI_ROOT")
	}

	buildRoot := filepath.Join(repoRoot, "dist", "control-ui")
	if controlUIBuildExists(buildRoot) {
		if err := os.Setenv("ANYCLAW_CONTROL_UI_ROOT", buildRoot); err != nil {
			return fmt.Errorf("set ANYCLAW_CONTROL_UI_ROOT: %w", err)
		}
		return nil
	}

	printInfo("Control UI build missing, auto-building frontend from %s", repoRoot)
	if err := runGatewayControlUIBuild(ctx, repoRoot); err != nil {
		return fmt.Errorf("auto-build control UI: %w", err)
	}
	if !controlUIBuildExists(buildRoot) {
		return fmt.Errorf("control UI build completed but %s is still missing", buildRoot)
	}
	if err := os.Setenv("ANYCLAW_CONTROL_UI_ROOT", buildRoot); err != nil {
		return fmt.Errorf("set ANYCLAW_CONTROL_UI_ROOT: %w", err)
	}
	printSuccess("Control UI built: %s", buildRoot)
	return nil
}

func controlUIBuildExists(root string) bool {
	root = strings.TrimSpace(root)
	if root == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(root, "index.html"))
	return err == nil && !info.IsDir()
}

func discoverGatewayRepoRoot(configPath string) (string, bool) {
	starts := make([]string, 0, 3)
	if cwd, err := os.Getwd(); err == nil && strings.TrimSpace(cwd) != "" {
		starts = append(starts, cwd)
	}
	if resolvedConfig := config.ResolveConfigPath(configPath); strings.TrimSpace(resolvedConfig) != "" {
		starts = append(starts, filepath.Dir(resolvedConfig))
	}
	if executable, err := os.Executable(); err == nil && strings.TrimSpace(executable) != "" {
		starts = append(starts, filepath.Dir(executable))
	}

	seen := map[string]struct{}{}
	for _, start := range starts {
		for _, dir := range ancestorDirs(start) {
			if _, ok := seen[dir]; ok {
				continue
			}
			seen[dir] = struct{}{}
			if looksLikeGatewayRepoRoot(dir) {
				return dir, true
			}
		}
	}

	return "", false
}

func looksLikeGatewayRepoRoot(dir string) bool {
	required := []string{
		filepath.Join(dir, "package.json"),
		filepath.Join(dir, "scripts", "ui.mjs"),
		filepath.Join(dir, "ui", "package.json"),
		filepath.Join(dir, "cmd", "anyclaw"),
	}
	for _, item := range required {
		if _, err := os.Stat(item); err != nil {
			return false
		}
	}
	return true
}

func ancestorDirs(start string) []string {
	current := filepath.Clean(strings.TrimSpace(start))
	if current == "" {
		return nil
	}

	dirs := make([]string, 0, 8)
	for {
		dirs = append(dirs, current)
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return dirs
}
