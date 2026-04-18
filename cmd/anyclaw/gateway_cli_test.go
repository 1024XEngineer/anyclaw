package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/1024XEngineer/anyclaw/pkg/config"
)

func TestNormalizeGatewayCommandSupportsStartAlias(t *testing.T) {
	if got := normalizeGatewayCommand("start"); got != "run" {
		t.Fatalf("expected start alias to normalize to run, got %q", got)
	}
	if got := normalizeGatewayCommand(" RUN "); got != "run" {
		t.Fatalf("expected run command to normalize to run, got %q", got)
	}
}

func TestEnsureGatewayControlUIBuiltUsesExistingBuild(t *testing.T) {
	repoRoot := createGatewayRepoFixture(t)
	buildRoot := filepath.Join(repoRoot, "dist", "control-ui")
	mustWriteGatewayFile(t, filepath.Join(buildRoot, "index.html"), "<html>ok</html>")
	configPath := writeGatewayConfig(t, repoRoot)
	restoreWorkingDir(t)

	originalRunner := runGatewayControlUIBuild
	defer func() { runGatewayControlUIBuild = originalRunner }()
	runGatewayControlUIBuild = func(context.Context, string) error {
		t.Fatal("did not expect control UI build runner to execute")
		return nil
	}

	t.Setenv("ANYCLAW_CONTROL_UI_ROOT", "")
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir repo root: %v", err)
	}

	if err := ensureGatewayControlUIBuilt(context.Background(), configPath); err != nil {
		t.Fatalf("expected existing build to pass, got %v", err)
	}
	if got := os.Getenv("ANYCLAW_CONTROL_UI_ROOT"); got != buildRoot {
		t.Fatalf("expected ANYCLAW_CONTROL_UI_ROOT=%q, got %q", buildRoot, got)
	}
}

func TestEnsureGatewayControlUIBuiltAutoBuildsMissingFrontend(t *testing.T) {
	repoRoot := createGatewayRepoFixture(t)
	configPath := writeGatewayConfig(t, repoRoot)
	buildRoot := filepath.Join(repoRoot, "dist", "control-ui")
	restoreWorkingDir(t)

	originalRunner := runGatewayControlUIBuild
	defer func() { runGatewayControlUIBuild = originalRunner }()

	called := false
	runGatewayControlUIBuild = func(_ context.Context, gotRepoRoot string) error {
		called = true
		if gotRepoRoot != repoRoot {
			t.Fatalf("expected repo root %q, got %q", repoRoot, gotRepoRoot)
		}
		mustWriteGatewayFile(t, filepath.Join(buildRoot, "index.html"), "<html>built</html>")
		return nil
	}

	t.Setenv("ANYCLAW_CONTROL_UI_ROOT", "")
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir repo root: %v", err)
	}

	if err := ensureGatewayControlUIBuilt(context.Background(), configPath); err != nil {
		t.Fatalf("expected auto-build to succeed, got %v", err)
	}
	if !called {
		t.Fatal("expected control UI build runner to execute")
	}
	if got := os.Getenv("ANYCLAW_CONTROL_UI_ROOT"); got != buildRoot {
		t.Fatalf("expected ANYCLAW_CONTROL_UI_ROOT=%q, got %q", buildRoot, got)
	}
}

func TestEnsureGatewayControlUIBuiltErrorsWhenRepoSourceIsMissing(t *testing.T) {
	tempDir := t.TempDir()
	configPath := writeGatewayConfig(t, tempDir)
	restoreWorkingDir(t)

	originalRunner := runGatewayControlUIBuild
	defer func() { runGatewayControlUIBuild = originalRunner }()
	runGatewayControlUIBuild = func(context.Context, string) error {
		t.Fatal("did not expect control UI build runner to execute")
		return nil
	}

	t.Setenv("ANYCLAW_CONTROL_UI_ROOT", "")
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}

	err := ensureGatewayControlUIBuilt(context.Background(), configPath)
	if err == nil {
		t.Fatal("expected missing repo source to fail")
	}
	if !strings.Contains(err.Error(), "corepack pnpm -C ui build") {
		t.Fatalf("expected build guidance, got %v", err)
	}
}

func createGatewayRepoFixture(t *testing.T) string {
	t.Helper()

	repoRoot := t.TempDir()
	mustWriteGatewayFile(t, filepath.Join(repoRoot, "package.json"), `{"name":"anyclaw-web-workspace"}`)
	mustWriteGatewayFile(t, filepath.Join(repoRoot, "scripts", "ui.mjs"), "console.log('build')")
	mustWriteGatewayFile(t, filepath.Join(repoRoot, "ui", "package.json"), `{"name":"@anyclaw/control-ui"}`)
	mustWriteGatewayFile(t, filepath.Join(repoRoot, "cmd", "anyclaw", "main.go"), "package main")
	return repoRoot
}

func writeGatewayConfig(t *testing.T, root string) string {
	t.Helper()

	cfg := config.DefaultConfig()
	configPath := filepath.Join(root, "anyclaw.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("save config: %v", err)
	}
	return configPath
}

func mustWriteGatewayFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func restoreWorkingDir(t *testing.T) {
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
