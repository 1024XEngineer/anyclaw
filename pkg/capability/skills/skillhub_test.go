package skills

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillhubSearchCatalogAndInstall(t *testing.T) {
	searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			_, _ = w.Write([]byte(`{"results":[{"displayName":"Travel Helper","slug":"travel-helper","summary":"Plans routes","version":"1.2.3"}]}`))
		case "/download":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(skillhubZipBytes(t, map[string]string{
				"SKILL.md": `---
name: travel-helper
description: Plans routes
---
Suggest routes.
`,
			}))
		default:
			http.NotFound(w, r)
		}
	}))
	defer searchServer.Close()

	originalSearchURL := SKILLHUB_SEARCH_URL
	originalDownloadURL := SKILLHUB_DOWNLOAD_URL
	SKILLHUB_SEARCH_URL = searchServer.URL + "/search"
	SKILLHUB_DOWNLOAD_URL = searchServer.URL + "/download"
	t.Cleanup(func() {
		SKILLHUB_SEARCH_URL = originalSearchURL
		SKILLHUB_DOWNLOAD_URL = originalDownloadURL
	})

	results, err := SearchSkillhub(context.Background(), "travel", 0)
	if err != nil {
		t.Fatalf("SearchSkillhub: %v", err)
	}
	if len(results) != 1 || results[0].Name != "travel-helper" {
		t.Fatalf("unexpected search results: %#v", results)
	}

	catalog, err := SearchSkillhubCatalog(context.Background(), "travel", 0)
	if err != nil {
		t.Fatalf("SearchSkillhubCatalog: %v", err)
	}
	if len(catalog) != 1 || catalog[0].InstallHint != "anyclaw skill install travel-helper" {
		t.Fatalf("unexpected catalog: %#v", catalog)
	}

	destDir := t.TempDir()
	if err := InstallSkillhubSkill(context.Background(), "travel-helper", destDir); err != nil {
		t.Fatalf("InstallSkillhubSkill: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(destDir, "travel-helper", "skill.json"))
	if err != nil {
		t.Fatalf("read installed skill.json: %v", err)
	}
	if !strings.Contains(string(data), `"travel-helper"`) {
		t.Fatalf("unexpected installed skill.json: %s", data)
	}
}

func TestSkillhubErrorPathsAndHelpers(t *testing.T) {
	errServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer errServer.Close()

	originalDownloadURL := SKILLHUB_DOWNLOAD_URL
	SKILLHUB_DOWNLOAD_URL = errServer.URL
	t.Cleanup(func() {
		SKILLHUB_DOWNLOAD_URL = originalDownloadURL
	})

	if err := InstallSkillhubSkill(context.Background(), "broken", t.TempDir()); err == nil {
		t.Fatal("expected bad gateway install error")
	}

	zipPath := filepath.Join(t.TempDir(), "escape.zip")
	if err := os.WriteFile(zipPath, skillhubZipBytes(t, map[string]string{"../escape.txt": "oops"}), 0o644); err != nil {
		t.Fatalf("write zip: %v", err)
	}
	if err := extractZip(zipPath, t.TempDir()); err == nil {
		t.Fatal("expected invalid zip path error")
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "skill.json"), []byte(`{"name":"existing"}`), 0o644); err != nil {
		t.Fatalf("write skill.json: %v", err)
	}
	if err := ConvertSkillhubToSkillJSON(dir); err != nil {
		t.Fatalf("expected existing skill.json to short-circuit, got %v", err)
	}

	missingDir := t.TempDir()
	if err := ConvertSkillhubToSkillJSON(missingDir); err == nil {
		t.Fatal("expected missing SKILL.md error")
	}

	def := buildSkillhubFileDefinition(filepath.Join(t.TempDir(), "fallback"), "plain text only")
	if def.Name == "" || def.Description == "" || def.Prompts["system"] == "" {
		t.Fatalf("unexpected fallback definition: %#v", def)
	}

	if !IsSkillhubInstalled() {
		t.Fatal("expected skillhub to be reported installed")
	}
	if got, err := FindSkillhubCLIPath(); err != nil || got != "integrated" {
		t.Fatalf("unexpected skillhub cli path %q, %v", got, err)
	}
}

func skillhubZipBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for name, content := range files {
		fileWriter, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create zip entry: %v", err)
		}
		if _, err := fileWriter.Write([]byte(content)); err != nil {
			t.Fatalf("write zip entry: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	return buf.Bytes()
}
