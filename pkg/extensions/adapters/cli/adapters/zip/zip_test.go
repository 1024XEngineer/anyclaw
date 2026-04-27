package zip

import (
	stdzip "archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCreateListAndExtract(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "alpha.txt")
	archive := filepath.Join(dir, "bundle.zip")
	dest := filepath.Join(dir, "out")
	if err := os.WriteFile(source, []byte("hello zip"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	client := NewClient(Config{})
	if output, err := client.Run(context.Background(), []string{"create", archive, source}); err != nil {
		t.Fatalf("create: %v", err)
	} else if !strings.Contains(output, "Created:") {
		t.Fatalf("create output = %q, want Created", output)
	}

	files, err := ListZIP(archive)
	if err != nil {
		t.Fatalf("ListZIP: %v", err)
	}
	if len(files) != 1 || files[0] != "alpha.txt" {
		t.Fatalf("files = %v, want alpha.txt", files)
	}

	output, err := client.Run(context.Background(), []string{"list", archive})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(output, "alpha.txt") {
		t.Fatalf("list output = %q, want alpha.txt", output)
	}

	if _, err := client.Run(context.Background(), []string{"extract", archive, dest}); err != nil {
		t.Fatalf("extract: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dest, "alpha.txt"))
	if err != nil {
		t.Fatalf("ReadFile extracted: %v", err)
	}
	if string(data) != "hello zip" {
		t.Fatalf("extracted content = %q, want hello zip", data)
	}
}

func TestExtractRejectsZipSlipPath(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "malicious.zip")
	dest := filepath.Join(dir, "out")
	if err := writeTestArchive(archive, map[string]string{
		"../evil.txt": "owned",
	}); err != nil {
		t.Fatalf("writeTestArchive: %v", err)
	}

	_, err := NewClient(Config{}).Run(context.Background(), []string{"extract", archive, dest})
	if err == nil {
		t.Fatal("expected unsafe path error")
	}
	if !strings.Contains(err.Error(), "unsafe zip entry path") {
		t.Fatalf("error = %v, want unsafe path", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "evil.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("evil file stat = %v, want not exist", statErr)
	}
}

func TestExtractRejectsSymlinkParent(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "symlink.zip")
	dest := filepath.Join(dir, "out")
	outside := filepath.Join(dir, "outside")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("MkdirAll dest: %v", err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("MkdirAll outside: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(dest, "link")); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}
	if err := writeTestArchive(archive, map[string]string{
		"link/pwn.txt": "owned",
	}); err != nil {
		t.Fatalf("writeTestArchive: %v", err)
	}

	_, err := NewClient(Config{}).Run(context.Background(), []string{"extract", archive, dest})
	if err == nil {
		t.Fatal("expected symlink parent error")
	}
	if !strings.Contains(err.Error(), "unsafe zip entry path") {
		t.Fatalf("error = %v, want unsafe path", err)
	}
	if _, statErr := os.Stat(filepath.Join(outside, "pwn.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("outside file stat = %v, want not exist", statErr)
	}
}

func TestRunAddCreatesAndUpdatesArchive(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.txt")
	second := filepath.Join(dir, "second.txt")
	archive := filepath.Join(dir, "bundle.zip")
	if err := os.WriteFile(first, []byte("one"), 0o644); err != nil {
		t.Fatalf("WriteFile first: %v", err)
	}
	if err := os.WriteFile(second, []byte("two"), 0o644); err != nil {
		t.Fatalf("WriteFile second: %v", err)
	}

	client := NewClient(Config{})
	if _, err := client.Run(context.Background(), []string{"add", archive, first}); err != nil {
		t.Fatalf("first add: %v", err)
	}
	if _, err := client.Run(context.Background(), []string{"add", archive, second}); err != nil {
		t.Fatalf("second add: %v", err)
	}

	files, err := ListZIP(archive)
	if err != nil {
		t.Fatalf("ListZIP: %v", err)
	}
	if strings.Join(files, ",") != "first.txt,second.txt" {
		t.Fatalf("files = %v, want first.txt and second.txt", files)
	}
}

func writeTestArchive(path string, entries map[string]string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	writer := stdzip.NewWriter(file)
	for name, content := range entries {
		entry, err := writer.Create(name)
		if err != nil {
			_ = writer.Close()
			_ = file.Close()
			return err
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			_ = writer.Close()
			_ = file.Close()
			return err
		}
	}
	if err := writer.Close(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}
