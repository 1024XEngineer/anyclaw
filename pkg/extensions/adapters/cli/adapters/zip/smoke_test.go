package zip

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCreatesArchive(t *testing.T) {
	tempDir := t.TempDir()
	source := filepath.Join(tempDir, "note.txt")
	if err := os.WriteFile(source, []byte("hello"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	archive := filepath.Join(tempDir, "bundle.zip")
	client := NewClient(Config{})
	if _, err := client.Run(context.Background(), []string{"create", archive, source}); err != nil {
		t.Fatalf("Run create: %v", err)
	}

	files, err := ListZIP(archive)
	if err != nil {
		t.Fatalf("ListZIP: %v", err)
	}
	if len(files) != 1 || files[0] != "note.txt" {
		t.Fatalf("unexpected archive contents: %#v", files)
	}
}

func TestRunReturnsUsageErrorWhenArgsMissing(t *testing.T) {
	client := NewClient(Config{})
	_, err := client.Run(context.Background(), nil)
	if err == nil {
		t.Fatal("expected usage error")
	}
	if !strings.Contains(err.Error(), "usage: zip") {
		t.Fatalf("unexpected error: %v", err)
	}
}
