package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/1024XEngineer/anyclaw/pkg/config"
)

func TestSelectDesktopBundleRootPrefersRepositoryRoot(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "cmd", "anyclaw-desktop", "build", "bin")

	mustMkdirAll(t, filepath.Join(root, "dist", "control-ui"))
	mustMkdirAll(t, filepath.Join(root, "skills"))
	mustMkdirAll(t, binDir)
	mustWriteFile(t, filepath.Join(root, "go.mod"), "module github.com/1024XEngineer/anyclaw\n")
	mustWriteFile(t, filepath.Join(root, defaultDesktopConfigName), "{}\n")
	mustWriteFile(t, filepath.Join(binDir, defaultDesktopConfigName), "{}\n")

	got := selectDesktopBundleRoot([]string{binDir})
	if got != root {
		t.Fatalf("expected bundle root %q, got %q", root, got)
	}
}

func TestResolveDesktopConfigPathWithPrefersBundleRootConfig(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "cmd", "anyclaw-desktop", "build", "bin")
	userConfigDir := filepath.Join(root, "AppData", "Roaming")

	mustMkdirAll(t, binDir)
	mustMkdirAll(t, filepath.Join(userConfigDir, "AnyClaw"))
	mustWriteFile(t, filepath.Join(root, defaultDesktopConfigName), "{\n  \"agent\": {\"name\": \"root\"}\n}\n")
	mustWriteFile(t, filepath.Join(binDir, defaultDesktopConfigName), "{\n  \"agent\": {\"name\": \"bin\"}\n}\n")

	got := resolveDesktopConfigPathWith(root, binDir, userConfigDir, "")
	want := filepath.Join(root, defaultDesktopConfigName)
	if got != want {
		t.Fatalf("expected config path %q, got %q", want, got)
	}
}

func TestResolveDesktopConfigPathWithEnvOverride(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "cmd", "anyclaw-desktop", "build", "bin")
	override := filepath.Join(root, "custom", defaultDesktopConfigName)

	mustMkdirAll(t, binDir)
	mustMkdirAll(t, filepath.Dir(override))
	mustWriteFile(t, filepath.Join(root, defaultDesktopConfigName), "{}\n")
	mustWriteFile(t, override, "{}\n")

	got := resolveDesktopConfigPathWith(root, binDir, "", override)
	if got != override {
		t.Fatalf("expected env override %q, got %q", override, got)
	}
}

func TestEnsureDesktopControlUIBuiltUsesExistingBundleBuild(t *testing.T) {
	root := createDesktopRepoFixture(t)
	buildRoot := filepath.Join(root, "dist", "control-ui")
	mustWriteFile(t, filepath.Join(buildRoot, "index.html"), "<html>ok</html>")
	configPath := writeDesktopConfig(t, root)
	restoreDesktopWorkingDir(t)
	t.Setenv("ANYCLAW_CONTROL_UI_ROOT", "")

	originalRunner := runDesktopControlUIBuild
	t.Cleanup(func() { runDesktopControlUIBuild = originalRunner })
	runDesktopControlUIBuild = func(context.Context, string) error {
		t.Fatal("did not expect control UI build runner to execute")
		return nil
	}

	if err := ensureDesktopControlUIBuilt(context.Background(), configPath, root); err != nil {
		t.Fatalf("expected existing build to pass, got %v", err)
	}
	if got := os.Getenv("ANYCLAW_CONTROL_UI_ROOT"); got != buildRoot {
		t.Fatalf("expected ANYCLAW_CONTROL_UI_ROOT=%q, got %q", buildRoot, got)
	}
}

func TestEnsureDesktopControlUIBuiltAutoBuildsMissingFrontend(t *testing.T) {
	root := createDesktopRepoFixture(t)
	buildRoot := filepath.Join(root, "dist", "control-ui")
	configPath := writeDesktopConfig(t, root)
	restoreDesktopWorkingDir(t)
	t.Setenv("ANYCLAW_CONTROL_UI_ROOT", "")

	originalRunner := runDesktopControlUIBuild
	t.Cleanup(func() { runDesktopControlUIBuild = originalRunner })

	called := false
	runDesktopControlUIBuild = func(_ context.Context, gotRepoRoot string) error {
		called = true
		if gotRepoRoot != root {
			t.Fatalf("expected repo root %q, got %q", root, gotRepoRoot)
		}
		mustWriteFile(t, filepath.Join(buildRoot, "index.html"), "<html>built</html>")
		return nil
	}

	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir repo root: %v", err)
	}
	if err := ensureDesktopControlUIBuilt(context.Background(), configPath, root); err != nil {
		t.Fatalf("expected auto-build to succeed, got %v", err)
	}
	if !called {
		t.Fatal("expected control UI build runner to execute")
	}
	if got := os.Getenv("ANYCLAW_CONTROL_UI_ROOT"); got != buildRoot {
		t.Fatalf("expected ANYCLAW_CONTROL_UI_ROOT=%q, got %q", buildRoot, got)
	}
}

func TestEnsureDesktopControlUIBuiltErrorsWhenBuildIsMissing(t *testing.T) {
	root := t.TempDir()
	configPath := writeDesktopConfig(t, root)
	restoreDesktopWorkingDir(t)
	t.Setenv("ANYCLAW_CONTROL_UI_ROOT", "")

	originalRunner := runDesktopControlUIBuild
	t.Cleanup(func() { runDesktopControlUIBuild = originalRunner })
	runDesktopControlUIBuild = func(context.Context, string) error {
		t.Fatal("did not expect control UI build runner to execute")
		return nil
	}

	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	err := ensureDesktopControlUIBuilt(context.Background(), configPath, root)
	if err == nil {
		t.Fatal("expected missing control UI build to fail")
	}
	if !strings.Contains(err.Error(), "corepack pnpm -C ui build") {
		t.Fatalf("expected explicit build guidance, got %v", err)
	}
}

func TestDerivePetStateUsesPendingApprovalSummary(t *testing.T) {
	status := gatewayStatusResponse{}
	status.Approvals.Pending = 1

	state, label, detail, lastEvent := derivePetState(status, nil, []gatewayApproval{
		{
			Status: "pending",
			Payload: map[string]any{
				"message": "帮我在桌面建立一个叫哈喽的 md 文件",
				"args": map[string]any{
					"command": "dir \"%USERPROFILE%\\Desktop\"",
				},
			},
			ToolName: "run_command",
		},
	})

	if state != "waiting" || label != "等待确认" {
		t.Fatalf("expected waiting state, got state=%q label=%q detail=%q lastEvent=%q", state, label, detail, lastEvent)
	}
	if detail != "等待批准：帮我在桌面建立一个叫哈喽的 md 文件" {
		t.Fatalf("expected pending approval summary detail, got %q", detail)
	}
	if lastEvent != "" {
		t.Fatalf("expected empty last event, got %q", lastEvent)
	}
}

func createDesktopRepoFixture(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "package.json"), `{"name":"anyclaw-web-workspace"}`)
	mustWriteFile(t, filepath.Join(root, "scripts", "ui.mjs"), "console.log('build')")
	mustWriteFile(t, filepath.Join(root, "ui", "package.json"), `{"name":"@anyclaw/control-ui"}`)
	mustWriteFile(t, filepath.Join(root, "cmd", "anyclaw", "main.go"), "package main")
	return root
}

func writeDesktopConfig(t *testing.T, root string) string {
	t.Helper()

	cfg := config.DefaultConfig()
	configPath := filepath.Join(root, defaultDesktopConfigName)
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("save desktop config: %v", err)
	}
	return configPath
}

func restoreDesktopWorkingDir(t *testing.T) {
	t.Helper()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})
}

func mustMkdirAll(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %q: %v", dir, err)
	}
}

func mustWriteFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %q: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}
