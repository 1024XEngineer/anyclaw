package plugin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHubClientDownloadWritesArchiveToDisk(t *testing.T) {
	const archiveBody = "plugin-archive"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/plugins/demo/download" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("version"); got != "1.2.3" {
			t.Fatalf("unexpected version: %s", got)
		}
		_, _ = w.Write([]byte(archiveBody))
	}))
	defer server.Close()

	client := NewHubClient(server.URL)
	client.httpClient = server.Client()

	dest := filepath.Join(t.TempDir(), "downloads", "demo.tar.gz")
	if err := client.Download(context.Background(), "demo", "1.2.3", dest); err != nil {
		t.Fatalf("Download: %v", err)
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != archiveBody {
		t.Fatalf("unexpected archive contents: %q", string(data))
	}
}

func TestHubClientDownloadFailsWhenDestinationCannotBeCreated(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("plugin-archive"))
	}))
	defer server.Close()

	client := NewHubClient(server.URL)
	client.httpClient = server.Client()

	tempDir := t.TempDir()
	blocker := filepath.Join(tempDir, "blocked")
	if err := os.WriteFile(blocker, []byte("occupied"), 0644); err != nil {
		t.Fatalf("WriteFile blocker: %v", err)
	}

	dest := filepath.Join(blocker, "demo.tar.gz")
	err := client.Download(context.Background(), "demo", "1.2.3", dest)
	if err == nil {
		t.Fatal("expected download to fail when destination parent is a file")
	}
	if !strings.Contains(err.Error(), "create parent dir") {
		t.Fatalf("expected parent dir failure, got: %v", err)
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Fatalf("expected no archive to be written, stat err=%v", statErr)
	}
}
