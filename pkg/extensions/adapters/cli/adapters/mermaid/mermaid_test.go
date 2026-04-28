package mermaid

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("ANYCLAW_MERMAID_ADAPTER_HELPER") == "1" {
		output := ""
		args := os.Args[1:]
		for i := 0; i < len(args); i++ {
			if args[i] == "-o" && i+1 < len(args) {
				output = args[i+1]
				i++
			}
		}
		expected := os.Getenv("ANYCLAW_MERMAID_EXPECT_OUTPUT")
		if output != expected {
			fmt.Fprintf(os.Stderr, "expected -o %q, got %q; args=%v", expected, output, args)
			os.Exit(1)
		}
		if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "mkdir output dir: %v", err)
			os.Exit(1)
		}
		if err := os.WriteFile(output, []byte("rendered"), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "write output: %v", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	os.Exit(m.Run())
}

func TestRenderPassesOutputFilePathToMermaidCLI(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "in.mmd")
	output := filepath.Join(dir, "custom", "out.png")
	if err := os.WriteFile(input, []byte("graph TD; A-->B;"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	t.Setenv("ANYCLAW_MERMAID_ADAPTER_HELPER", "1")
	t.Setenv("ANYCLAW_MERMAID_EXPECT_OUTPUT", output)
	t.Setenv("PATH", fakeNpxDir(t)+string(os.PathListSeparator)+os.Getenv("PATH"))

	if _, err := New().Execute(context.Background(), []string{"render", input, output}); err != nil {
		t.Fatalf("render returned error: %v", err)
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatalf("expected output %s to exist: %v", output, err)
	}
}

func fakeNpxDir(t *testing.T) string {
	t.Helper()

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}

	dir := t.TempDir()
	name := "npx"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	copyExecutable(t, exe, filepath.Join(dir, name))
	return dir
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
