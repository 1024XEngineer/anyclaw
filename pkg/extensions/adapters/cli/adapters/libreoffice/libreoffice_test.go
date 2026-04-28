package libreoffice

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
	if os.Getenv("ANYCLAW_LIBREOFFICE_ADAPTER_HELPER") == "1" {
		args := os.Args[1:]
		format := ""
		outDir := ""
		input := ""
		for i := 0; i < len(args); i++ {
			switch args[i] {
			case "--convert-to":
				if i+1 < len(args) {
					format = args[i+1]
					i++
				}
			case "--outdir":
				if i+1 < len(args) {
					outDir = args[i+1]
					i++
				}
			default:
				input = args[i]
			}
		}
		if format == "" || outDir == "" || input == "" {
			fmt.Fprintf(os.Stderr, "missing conversion args: %v", args)
			os.Exit(1)
		}
		name := strings.TrimSuffix(filepath.Base(input), filepath.Ext(input)) + "." + format
		if err := os.WriteFile(filepath.Join(outDir, name), []byte("converted"), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "write output: %v", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	os.Exit(m.Run())
}

func TestConvertRenamesLibreOfficeDerivedOutput(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "foo.docx")
	output := filepath.Join(dir, "custom", "bar.pdf")
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		t.Fatalf("mkdir output dir: %v", err)
	}
	writeFile(t, input)
	t.Setenv("ANYCLAW_LIBREOFFICE_ADAPTER_HELPER", "1")
	client := NewClient(Config{SofficePath: fakeSofficePath(t)})

	if _, err := client.Run(context.Background(), []string{"convert", input, output}); err != nil {
		t.Fatalf("convert returned error: %v", err)
	}

	assertFileExists(t, output)
	assertFileMissing(t, filepath.Join(filepath.Dir(output), "foo.pdf"))
}

func TestToPDFRenamesLibreOfficeDerivedOutput(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "foo.odt")
	output := filepath.Join(dir, "custom", "bar.pdf")
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		t.Fatalf("mkdir output dir: %v", err)
	}
	writeFile(t, input)
	t.Setenv("ANYCLAW_LIBREOFFICE_ADAPTER_HELPER", "1")
	client := NewClient(Config{SofficePath: fakeSofficePath(t)})

	if _, err := client.Run(context.Background(), []string{"pdf", input, output}); err != nil {
		t.Fatalf("pdf returned error: %v", err)
	}

	assertFileExists(t, output)
	assertFileMissing(t, filepath.Join(filepath.Dir(output), "foo.pdf"))
}

func fakeSofficePath(t *testing.T) string {
	t.Helper()

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}

	path := filepath.Join(t.TempDir(), "soffice-helper.exe")
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

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("document"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func assertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file %s to exist: %v", path, err)
	}
}

func assertFileMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected file %s to be absent, stat err=%v", path, err)
	}
}
