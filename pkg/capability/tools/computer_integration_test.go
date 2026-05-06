package tools

import (
	"context"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestComputerControlWindowsIntegration(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows desktop integration test")
	}
	if strings.TrimSpace(strings.ToLower(os.Getenv("ANYCLAW_DESKTOP_INTEGRATION"))) != "1" {
		t.Skip("set ANYCLAW_DESKTOP_INTEGRATION=1 to run the real desktop-control smoke test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	opts := BuiltinOptions{
		WorkingDir:      t.TempDir(),
		ExecutionMode:   "host-reviewed",
		PermissionLevel: "full",
		Computer: ComputerOptions{
			Enabled:               true,
			Backend:               "codex_local",
			CoordinateSpace:       defaultComputerCoordinateSpace,
			MaxActionsPerTurn:     3,
			ObserveAfterAction:    true,
			IncludeWindowsDefault: true,
			AllowedApps:           []string{"notepad"},
		},
	}

	if _, err := DesktopOpenTool(ctx, map[string]any{"target": "notepad", "kind": "app"}, opts); err != nil {
		t.Fatalf("open notepad: %v", err)
	}
	t.Cleanup(func() {
		_, _ = runDesktopPowerShell(context.Background(), `Get-Process notepad -ErrorAction SilentlyContinue | Stop-Process -Force; "stopped"`)
	})
	if _, err := DesktopWaitWindowTool(ctx, map[string]any{"process_name": "notepad", "timeout_ms": 5000}, opts); err != nil {
		t.Fatalf("wait notepad: %v", err)
	}

	observe, err := ComputerObserveTool(ctx, map[string]any{"include_windows": true}, opts)
	if err != nil {
		t.Fatalf("computer observe: %v", err)
	}
	if !strings.Contains(observe, "notepad") {
		t.Fatalf("expected observe output to include notepad, got %s", observe)
	}

	result, err := ComputerActionTool(ctx, map[string]any{
		"actions": []any{
			map[string]any{"type": "type", "text": "anyclaw computer integration"},
			map[string]any{"type": "keypress", "keys": []any{"ctrl", "a"}},
			map[string]any{"type": "keypress", "keys": []any{"delete"}},
		},
		"include_windows": true,
	}, opts)
	if err != nil {
		t.Fatalf("computer action: %v", err)
	}
	if !strings.Contains(result, `"actions"`) {
		t.Fatalf("expected action snapshots, got %s", result)
	}
}
