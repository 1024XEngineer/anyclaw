package blender

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("ANYCLAW_BLENDER_ADAPTER_HELPER") == "1" {
		args := os.Args[1:]
		for i, arg := range args {
			if arg == "--python" && i+1 < len(args) {
				script, err := os.ReadFile(args[i+1])
				if err != nil {
					fmt.Fprintf(os.Stderr, "read script: %v", err)
					os.Exit(1)
				}
				if strings.Contains(string(script), "bpy.context.window.scene") {
					fmt.Fprint(os.Stderr, "background mode has no bpy.context.window")
					os.Exit(1)
				}
				fmt.Println("Created scene: TestScene")
				os.Exit(0)
			}
		}
		fmt.Fprint(os.Stderr, "missing --python script")
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func TestCreateSceneWorksInBackgroundMode(t *testing.T) {
	t.Setenv("ANYCLAW_BLENDER_ADAPTER_HELPER", "1")
	client := NewClient(Config{
		BlenderPath: fakeBlenderPath(t),
		Workspace:   t.TempDir(),
	})

	out, err := client.Run(context.Background(), []string{"scene", "TestScene"})
	if err != nil {
		t.Fatalf("create scene returned error: %v; output: %s", err, out)
	}
	if !strings.Contains(out, "Scene created: TestScene") {
		t.Fatalf("expected create scene confirmation, got %q", out)
	}
}

func fakeBlenderPath(t *testing.T) string {
	t.Helper()

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}

	path := filepath.Join(t.TempDir(), "blender-helper.exe")
	copyExecutable(t, exe, path)
	return path
}

func copyExecutable(t *testing.T, src, dst string) {
	t.Helper()

	in, err := os.Open(src)
	if err != nil {
		t.Fatalf("open source executable: %v", err)
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		t.Fatalf("create helper executable: %v", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		t.Fatalf("copy helper executable: %v", err)
	}
}
