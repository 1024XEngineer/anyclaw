package main

import (
	"os"
	"path/filepath"
	"testing"
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

func mustMkdirAll(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %q: %v", dir, err)
	}
}

func mustWriteFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}
