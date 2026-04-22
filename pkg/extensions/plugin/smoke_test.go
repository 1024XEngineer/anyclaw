package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHubClientDownloadWritesFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/plugins/demo/download" {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("version"); got != "1.0.0" {
			t.Fatalf("unexpected version %q", got)
		}
		_, _ = w.Write([]byte("archive-bytes"))
	}))
	defer server.Close()

	destDir := t.TempDir()
	destPath := filepath.Join(destDir, "downloads", "demo.tar.gz")

	client := NewHubClient(server.URL)
	if err := client.Download(context.Background(), "demo", "1.0.0", destPath); err != nil {
		t.Fatalf("Download: %v", err)
	}

	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if got := string(data); got != "archive-bytes" {
		t.Fatalf("unexpected archive contents %q", got)
	}
}

func TestHubClientSearchBuildsResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/plugins/search" {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("q"); got != "demo" {
			t.Fatalf("unexpected query %q", got)
		}
		if got := r.URL.Query().Get("category"); got != "tool" {
			t.Fatalf("unexpected category %q", got)
		}
		if got := r.URL.Query().Get("limit"); got != "5" {
			t.Fatalf("unexpected limit %q", got)
		}
		if got := r.URL.Query().Get("offset"); got != "10" {
			t.Fatalf("unexpected offset %q", got)
		}
		_ = json.NewEncoder(w).Encode(HubSearchResult{
			Plugins: []HubPlugin{{ID: "demo", Name: "Demo"}},
			Total:   1,
			Page:    3,
			Limit:   5,
		})
	}))
	defer server.Close()

	client := NewHubClient(server.URL)
	result, err := client.Search(context.Background(), "demo", "tool", 5, 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if result.Total != 1 || len(result.Plugins) != 1 || result.Plugins[0].ID != "demo" {
		t.Fatalf("unexpected search result: %#v", result)
	}
}

func TestHubClientEscapesPathAndQueryValues(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case "/api/v1/plugins/plug%2Fin/download":
			if got := r.URL.Query().Get("version"); got != "1.0.0 & stable" {
				t.Fatalf("unexpected version %q", got)
			}
			_, _ = w.Write([]byte("archive-bytes"))
		case "/api/v1/plugins/search":
			if got := r.URL.Query().Get("q"); got != "demo & tool" {
				t.Fatalf("unexpected query %q", got)
			}
			if got := r.URL.Query().Get("category"); got != "cat/one" {
				t.Fatalf("unexpected category %q", got)
			}
			_ = json.NewEncoder(w).Encode(HubSearchResult{})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewHubClient(server.URL)
	if _, err := client.Search(context.Background(), "demo & tool", "cat/one", 5, 0); err != nil {
		t.Fatalf("Search: %v", err)
	}

	destPath := filepath.Join(t.TempDir(), "demo.tar.gz")
	if err := client.Download(context.Background(), "plug/in", "1.0.0 & stable", destPath); err != nil {
		t.Fatalf("Download: %v", err)
	}
}

func TestHubClientGettersReturnDecodedResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/plugins/demo":
			_ = json.NewEncoder(w).Encode(HubPlugin{ID: "demo", Name: "Demo", Version: "1.0.0"})
		case "/api/v1/categories":
			_ = json.NewEncoder(w).Encode(HubCategories{Categories: []HubCategory{{ID: "tool", Name: "Tools"}}})
		case "/api/v1/plugins/demo/versions":
			_ = json.NewEncoder(w).Encode([]string{"2.0.0", "1.0.0"})
		case "/api/v1/stats":
			_ = json.NewEncoder(w).Encode(HubStats{TotalPlugins: 3, TotalDownloads: 8, TotalStars: 5, TotalSigners: 2})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewHubClient(server.URL)

	plugin, err := client.GetPlugin(context.Background(), "demo")
	if err != nil {
		t.Fatalf("GetPlugin: %v", err)
	}
	if plugin.ID != "demo" || plugin.Version != "1.0.0" {
		t.Fatalf("unexpected plugin: %#v", plugin)
	}

	categories, err := client.GetCategories(context.Background())
	if err != nil {
		t.Fatalf("GetCategories: %v", err)
	}
	if len(categories) != 1 || categories[0].ID != "tool" {
		t.Fatalf("unexpected categories: %#v", categories)
	}

	versions, err := client.GetVersions(context.Background(), "demo")
	if err != nil {
		t.Fatalf("GetVersions: %v", err)
	}
	if len(versions) != 2 || versions[0] != "2.0.0" {
		t.Fatalf("unexpected versions: %#v", versions)
	}

	stats, err := client.GetStats(context.Background())
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats.TotalPlugins != 3 || stats.TotalDownloads != 8 {
		t.Fatalf("unexpected stats: %#v", stats)
	}
}

func TestHubClientReturnsErrorsForBadStatusAndInvalidJSON(t *testing.T) {
	t.Run("search status", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusBadGateway)
		}))
		defer server.Close()

		_, err := NewHubClient(server.URL).Search(context.Background(), "demo", "", 1, 0)
		if err == nil || !strings.Contains(err.Error(), "unexpected status code") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("plugin invalid json", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("{"))
		}))
		defer server.Close()

		_, err := NewHubClient(server.URL).GetPlugin(context.Background(), "demo")
		if err == nil || !strings.Contains(err.Error(), "failed to decode response") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("search invalid json", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("{"))
		}))
		defer server.Close()

		_, err := NewHubClient(server.URL).Search(context.Background(), "demo", "", 1, 0)
		if err == nil || !strings.Contains(err.Error(), "failed to decode response") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("download status", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "denied", http.StatusForbidden)
		}))
		defer server.Close()

		err := NewHubClient(server.URL).Download(context.Background(), "demo", "1.0.0", filepath.Join(t.TempDir(), "demo.tar.gz"))
		if err == nil || !strings.Contains(err.Error(), "download failed with status") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestHubClientReturnsCreateRequestErrors(t *testing.T) {
	badClient := NewHubClient("http://bad\nhost")
	calls := map[string]func() error{
		"search": func() error {
			_, err := badClient.Search(context.Background(), "demo", "", 1, 0)
			return err
		},
		"get_plugin": func() error {
			_, err := badClient.GetPlugin(context.Background(), "demo")
			return err
		},
		"download": func() error {
			return badClient.Download(context.Background(), "demo", "1.0.0", filepath.Join(t.TempDir(), "demo.tar.gz"))
		},
		"categories": func() error {
			_, err := badClient.GetCategories(context.Background())
			return err
		},
		"versions": func() error {
			_, err := badClient.GetVersions(context.Background(), "demo")
			return err
		},
		"stats": func() error {
			_, err := badClient.GetStats(context.Background())
			return err
		},
	}

	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			err := call()
			if err == nil || !strings.Contains(err.Error(), "failed to create request") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestHubClientReturnsCreateRequestErrorsForNilContext(t *testing.T) {
	client := NewHubClient("https://hub.example.com")
	calls := map[string]func() error{
		"search": func() error {
			_, err := client.Search(nil, "demo", "", 1, 0)
			return err
		},
		"get_plugin": func() error {
			_, err := client.GetPlugin(nil, "demo")
			return err
		},
		"download": func() error {
			return client.Download(nil, "demo", "1.0.0", filepath.Join(t.TempDir(), "demo.tar.gz"))
		},
		"categories": func() error {
			_, err := client.GetCategories(nil)
			return err
		},
		"versions": func() error {
			_, err := client.GetVersions(nil, "demo")
			return err
		},
		"stats": func() error {
			_, err := client.GetStats(nil)
			return err
		},
	}

	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			err := call()
			if err == nil || !strings.Contains(err.Error(), "failed to create request") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestHubClientReturnsCategoryVersionAndStatsErrors(t *testing.T) {
	t.Run("category status", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusBadGateway)
		}))
		defer server.Close()

		_, err := NewHubClient(server.URL).GetCategories(context.Background())
		if err == nil || !strings.Contains(err.Error(), "unexpected status code") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("category decode", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("{"))
		}))
		defer server.Close()

		_, err := NewHubClient(server.URL).GetCategories(context.Background())
		if err == nil || !strings.Contains(err.Error(), "failed to decode response") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("version decode", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("{"))
		}))
		defer server.Close()

		_, err := NewHubClient(server.URL).GetVersions(context.Background(), "demo")
		if err == nil || !strings.Contains(err.Error(), "failed to decode response") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("stats status", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusBadGateway)
		}))
		defer server.Close()

		_, err := NewHubClient(server.URL).GetStats(context.Background())
		if err == nil || !strings.Contains(err.Error(), "unexpected status code") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("stats decode", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("{"))
		}))
		defer server.Close()

		_, err := NewHubClient(server.URL).GetStats(context.Background())
		if err == nil || !strings.Contains(err.Error(), "failed to decode response") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestHubManagerInstallPluginUsesLatestVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/plugins/demo/versions":
			_ = json.NewEncoder(w).Encode([]string{"2.0.0", "1.0.0"})
		case "/api/v1/plugins/demo/download":
			if got := r.URL.Query().Get("version"); got != "2.0.0" {
				t.Fatalf("unexpected downloaded version %q", got)
			}
			_, _ = w.Write([]byte("latest-archive"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	installDir := t.TempDir()
	manager := NewHubManager(server.URL, t.TempDir())

	if err := manager.InstallPlugin(context.Background(), "demo", "", installDir); err != nil {
		t.Fatalf("InstallPlugin: %v", err)
	}

	destPath := filepath.Join(installDir, "demo.tar.gz")
	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if got := string(data); got != "latest-archive" {
		t.Fatalf("unexpected installed archive contents %q", got)
	}
}

func TestHubManagerSearchAndUpdateDelegateToHubClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/plugins/search":
			_ = json.NewEncoder(w).Encode(HubSearchResult{
				Plugins: []HubPlugin{{ID: "demo", Name: "Demo", Version: "1.0.0"}},
			})
		case "/api/v1/plugins/demo/versions":
			_ = json.NewEncoder(w).Encode([]string{"3.0.0", "2.0.0"})
		case "/api/v1/plugins/demo/download":
			if got := r.URL.Query().Get("version"); got != "3.0.0" {
				t.Fatalf("unexpected updated version %q", got)
			}
			_, _ = w.Write([]byte("updated-archive"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	manager := NewHubManager(server.URL, t.TempDir())

	results, err := manager.SearchPlugins(context.Background(), "demo", "", 10)
	if err != nil {
		t.Fatalf("SearchPlugins: %v", err)
	}
	if len(results) != 1 || results[0].ID != "demo" {
		t.Fatalf("unexpected search results: %#v", results)
	}

	installDir := t.TempDir()
	if err := manager.UpdatePlugin(context.Background(), "demo", installDir); err != nil {
		t.Fatalf("UpdatePlugin: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(installDir, "demo.tar.gz"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if got := string(data); got != "updated-archive" {
		t.Fatalf("unexpected update contents %q", got)
	}
}

func TestHubManagerInstallAndUpdateFailWithoutVersions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/versions") {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode([]string{})
	}))
	defer server.Close()

	manager := NewHubManager(server.URL, t.TempDir())
	installDir := t.TempDir()

	if err := manager.InstallPlugin(context.Background(), "demo", "", installDir); err == nil || !strings.Contains(err.Error(), "no versions available") {
		t.Fatalf("unexpected install error: %v", err)
	}
	if err := manager.UpdatePlugin(context.Background(), "demo", installDir); err == nil || !strings.Contains(err.Error(), "no versions available") {
		t.Fatalf("unexpected update error: %v", err)
	}
}

func TestHubClientDownloadFailsWhenDestinationParentIsAFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("archive-bytes"))
	}))
	defer server.Close()

	destDir := t.TempDir()
	blocker := filepath.Join(destDir, "blocked")
	if err := os.WriteFile(blocker, []byte("occupied"), 0o644); err != nil {
		t.Fatalf("WriteFile blocker: %v", err)
	}

	client := NewHubClient(server.URL)
	destPath := filepath.Join(blocker, "demo.tar.gz")
	err := client.Download(context.Background(), "demo", "1.0.0", destPath)
	if err == nil {
		t.Fatal("expected download to fail when destination parent is a file")
	}
	if !strings.Contains(err.Error(), "create destination dir") {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, statErr := os.Stat(destPath); !os.IsNotExist(statErr) {
		t.Fatalf("expected no archive to be written, stat err=%v", statErr)
	}
}

func TestLocalCacheGetPluginReadsCachedPlugin(t *testing.T) {
	cacheDir := t.TempDir()
	cached := HubPlugin{ID: "demo", Name: "Demo Plugin", Version: "1.0.0"}
	data, err := json.Marshal(cached)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "demo.json"), data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, ok := (&LocalCache{dir: cacheDir}).GetPlugin("demo")
	if !ok {
		t.Fatal("expected cached plugin to be found")
	}
	if got.ID != cached.ID || got.Version != cached.Version {
		t.Fatalf("unexpected cached plugin %+v", got)
	}
}

func TestLocalCacheGetPluginMissesInvalidEntries(t *testing.T) {
	if got, ok := (&LocalCache{}).GetPlugin("demo"); ok || got != nil {
		t.Fatalf("expected empty cache lookup to fail, got %#v, %v", got, ok)
	}

	cacheDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(cacheDir, "broken.json"), []byte("{"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if got, ok := (&LocalCache{dir: cacheDir}).GetPlugin("broken"); ok || got != nil {
		t.Fatalf("expected broken cache entry to fail, got %#v, %v", got, ok)
	}

	if got, ok := (&LocalCache{dir: cacheDir}).GetPlugin("missing"); ok || got != nil {
		t.Fatalf("expected missing cache entry to fail, got %#v, %v", got, ok)
	}
}

func TestNewHubClientConfiguresDefaultTimeout(t *testing.T) {
	client := NewHubClient("http://example.com")
	if client.baseURL != "http://example.com" {
		t.Fatalf("unexpected baseURL %q", client.baseURL)
	}
	if client.httpClient == nil || client.httpClient.Timeout != 30*time.Second {
		t.Fatalf("unexpected client timeout: %#v", client.httpClient)
	}
}

func TestWriteFileAtomicCreatesAndOverwritesFiles(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "nested", "demo.tar.gz")
	if err := writeFileAtomic(dest, []byte("first")); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := writeFileAtomic(dest, []byte("second")); err != nil {
		t.Fatalf("second write: %v", err)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if got := string(data); got != "second" {
		t.Fatalf("unexpected contents %q", got)
	}
}

func TestHubClientMethodsSurfaceTransportErrors(t *testing.T) {
	client := NewHubClient("http://127.0.0.1:1")
	ops := map[string]func() error{
		"search": func() error {
			_, err := client.Search(context.Background(), "demo", "", 1, 0)
			return err
		},
		"get_plugin": func() error {
			_, err := client.GetPlugin(context.Background(), "demo")
			return err
		},
		"download": func() error {
			return client.Download(context.Background(), "demo", "1.0.0", filepath.Join(t.TempDir(), "demo.tar.gz"))
		},
		"get_categories": func() error {
			_, err := client.GetCategories(context.Background())
			return err
		},
		"get_versions": func() error {
			_, err := client.GetVersions(context.Background(), "demo")
			return err
		},
		"get_stats": func() error {
			_, err := client.GetStats(context.Background())
			return err
		},
	}

	for name, op := range ops {
		t.Run(name, func(t *testing.T) {
			err := op()
			if err == nil || !strings.Contains(err.Error(), "failed to execute request") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestHubManagerPropagatesVersionLookupFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	manager := NewHubManager(server.URL, t.TempDir())
	installDir := t.TempDir()

	for _, tc := range []struct {
		name string
		run  func() error
	}{
		{
			name: "install",
			run: func() error {
				return manager.InstallPlugin(context.Background(), "demo", "", installDir)
			},
		},
		{
			name: "update",
			run: func() error {
				return manager.UpdatePlugin(context.Background(), "demo", installDir)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			if err == nil || !strings.Contains(err.Error(), "unexpected status code") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestHubClientGetPluginReturnsNotFoundMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	_, err := NewHubClient(server.URL).GetPlugin(context.Background(), "missing")
	if err == nil || err.Error() != fmt.Sprintf("plugin not found: %s", "missing") {
		t.Fatalf("unexpected error: %v", err)
	}
}

type stubTempFile struct {
	name     string
	writeErr error
	syncErr  error
	closeErr error
	buffer   bytes.Buffer
}

func (f *stubTempFile) Name() string {
	return f.name
}

func (f *stubTempFile) Write(data []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return f.buffer.Write(data)
}

func (f *stubTempFile) Sync() error {
	return f.syncErr
}

func (f *stubTempFile) Close() error {
	return f.closeErr
}

func TestWriteFileAtomicSurfacesFilesystemFailures(t *testing.T) {
	t.Run("mkdir all", func(t *testing.T) {
		writer := newAtomicFileWriter()
		writer.mkdirAll = func(string, os.FileMode) error { return errors.New("mkdir fail") }
		err := writer.write(filepath.Join(t.TempDir(), "demo.tar.gz"), strings.NewReader("data"))
		if err == nil || !strings.Contains(err.Error(), "create destination dir") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("create temp file", func(t *testing.T) {
		writer := newAtomicFileWriter()
		writer.createTemp = func(string, string) (atomicTempFile, error) { return nil, errors.New("temp fail") }
		err := writer.write(filepath.Join(t.TempDir(), "demo.tar.gz"), strings.NewReader("data"))
		if err == nil || !strings.Contains(err.Error(), "create temp file") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("write temp file", func(t *testing.T) {
		removed := false
		writer := newAtomicFileWriter()
		writer.createTemp = func(string, string) (atomicTempFile, error) {
			return &stubTempFile{name: "write-temp", writeErr: errors.New("write fail")}, nil
		}
		writer.remove = func(name string) error {
			if name == "write-temp" {
				removed = true
			}
			return nil
		}
		err := writer.write(filepath.Join(t.TempDir(), "demo.tar.gz"), strings.NewReader("data"))
		if err == nil || !strings.Contains(err.Error(), "write temp file") {
			t.Fatalf("unexpected error: %v", err)
		}
		if !removed {
			t.Fatal("expected temp file cleanup on write error")
		}
	})

	t.Run("sync temp file", func(t *testing.T) {
		writer := newAtomicFileWriter()
		writer.createTemp = func(string, string) (atomicTempFile, error) {
			return &stubTempFile{name: "sync-temp", syncErr: errors.New("sync fail")}, nil
		}
		err := writer.write(filepath.Join(t.TempDir(), "demo.tar.gz"), strings.NewReader("data"))
		if err == nil || !strings.Contains(err.Error(), "sync temp file") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("close temp file", func(t *testing.T) {
		writer := newAtomicFileWriter()
		writer.createTemp = func(string, string) (atomicTempFile, error) {
			return &stubTempFile{name: "close-temp", closeErr: errors.New("close fail")}, nil
		}
		err := writer.write(filepath.Join(t.TempDir(), "demo.tar.gz"), strings.NewReader("data"))
		if err == nil || !strings.Contains(err.Error(), "close temp file") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("rename temp file", func(t *testing.T) {
		writer := newAtomicFileWriter()
		writer.createTemp = func(string, string) (atomicTempFile, error) {
			return &stubTempFile{name: "rename-temp"}, nil
		}
		writer.rename = func(string, string) error { return errors.New("rename fail") }
		err := writer.write(filepath.Join(t.TempDir(), "demo.tar.gz"), strings.NewReader("data"))
		if err == nil || !strings.Contains(err.Error(), "rename temp file") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

type errReader struct {
	readErr error
}

func (r *errReader) Read([]byte) (int, error) {
	return 0, r.readErr
}

type staticRoundTripper struct {
	resp *http.Response
	err  error
}

func (rt staticRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	if rt.err != nil {
		return nil, rt.err
	}
	return rt.resp, nil
}

func TestHubClientDownloadReturnsReadError(t *testing.T) {
	client := NewHubClient("http://example.com")
	client.httpClient = &http.Client{
		Transport: staticRoundTripper{
			resp: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(&errReader{readErr: errors.New("read fail")}),
			},
		},
	}

	err := client.Download(context.Background(), "demo", "1.0.0", filepath.Join(t.TempDir(), "demo.tar.gz"))
	if err == nil || !strings.Contains(err.Error(), "write temp file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildURLRejectsBadBaseURL(t *testing.T) {
	_, err := NewHubClient("http://bad\nhost").buildURL([]string{"plugins"}, nil)
	if err == nil {
		t.Fatal("expected buildURL to fail")
	}
}

func TestBuildURLEscapesSegmentsAndQuery(t *testing.T) {
	result, err := NewHubClient("https://hub.example.com/root").buildURL(
		[]string{"plugins", "plug/in", "download"},
		url.Values{"version": []string{"1.0.0 & stable"}},
	)
	if err != nil {
		t.Fatalf("buildURL: %v", err)
	}
	expected := "https://hub.example.com/root/api/v1/plugins/plug%2Fin/download?version=1.0.0+%26+stable"
	if result != expected {
		t.Fatalf("unexpected URL %q", result)
	}
}

func TestHubManagerSearchPluginsPropagatesErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer server.Close()

	_, err := NewHubManager(server.URL, t.TempDir()).SearchPlugins(context.Background(), "demo", "", 10)
	if err == nil || !strings.Contains(err.Error(), "unexpected status code") {
		t.Fatalf("unexpected error: %v", err)
	}
}
