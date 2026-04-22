package skills

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestConvertMarkdownToSkillJSONUsesDetailFallbacks(t *testing.T) {
	detail := &SkillDetail{
		Description: "Remote skill description",
		Version:     "2.3.4",
		Permissions: []string{"network", "filesystem"},
		Entrypoint:  "run.py",
		Homepage:    "https://example.com/weather",
		Registry:    "partner-registry",
	}

	skillJSON, err := ConvertMarkdownToSkillJSON("# Weather\n- fetch forecast\nAlways answer clearly.", "weather", detail)
	if err != nil {
		t.Fatalf("ConvertMarkdownToSkillJSON returned error: %v", err)
	}

	var got skillFileDefinition
	if err := json.Unmarshal([]byte(skillJSON), &got); err != nil {
		t.Fatalf("unmarshal skill JSON: %v", err)
	}

	if got.Name != "weather" {
		t.Fatalf("expected name weather, got %q", got.Name)
	}
	if got.Description != detail.Description {
		t.Fatalf("expected description %q, got %q", detail.Description, got.Description)
	}
	if got.Version != detail.Version {
		t.Fatalf("expected version %q, got %q", detail.Version, got.Version)
	}
	if got.Registry != detail.Registry {
		t.Fatalf("expected registry %q, got %q", detail.Registry, got.Registry)
	}
	if got.Entrypoint != detail.Entrypoint {
		t.Fatalf("expected entrypoint %q, got %q", detail.Entrypoint, got.Entrypoint)
	}
	if got.Prompts["system"] != "fetch forecast Always answer clearly." {
		t.Fatalf("unexpected system prompt: %q", got.Prompts["system"])
	}
}

func TestSearchSkillhubCatalogUsesUnifiedInstallHint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"displayName":"Weather Helper","slug":"weather","summary":"Forecast support","version":"1.2.3"}]}`))
	}))
	defer server.Close()

	originalSearchURL := SKILLHUB_SEARCH_URL
	SKILLHUB_SEARCH_URL = server.URL
	t.Cleanup(func() {
		SKILLHUB_SEARCH_URL = originalSearchURL
	})

	entries, err := SearchSkillhubCatalog(context.Background(), "weather", 5)
	if err != nil {
		t.Fatalf("SearchSkillhubCatalog returned error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].InstallHint != "anyclaw skill install weather" {
		t.Fatalf("unexpected install hint: %q", entries[0].InstallHint)
	}
}

func TestConvertSkillhubToSkillJSONWritesExpectedFile(t *testing.T) {
	dir := t.TempDir()
	content := `---
name: travel_helper
description: Plan lighter itineraries
---
Suggest efficient routes.
Highlight trade-offs.
`
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}

	if err := ConvertSkillhubToSkillJSON(dir); err != nil {
		t.Fatalf("ConvertSkillhubToSkillJSON returned error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "skill.json"))
	if err != nil {
		t.Fatalf("read skill.json: %v", err)
	}

	var got skillFileDefinition
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal skill.json: %v", err)
	}

	if got.Name != "travel_helper" {
		t.Fatalf("expected name travel_helper, got %q", got.Name)
	}
	if got.Description != "Plan lighter itineraries" {
		t.Fatalf("unexpected description: %q", got.Description)
	}
	if got.Source != "skillhub" {
		t.Fatalf("expected source skillhub, got %q", got.Source)
	}
	if got.Prompts["system"] != "Suggest efficient routes.\nHighlight trade-offs." {
		t.Fatalf("unexpected system prompt: %q", got.Prompts["system"])
	}
}

func TestPathWithinBaseRejectsPrefixLookalikes(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "skills")
	inside := filepath.Join(baseDir, "weather", "skill.json")
	outside := filepath.Join(baseDir+"-backup", "weather", "skill.json")

	if !pathWithinBase(baseDir, inside) {
		t.Fatalf("expected %q to be inside %q", inside, baseDir)
	}
	if pathWithinBase(baseDir, outside) {
		t.Fatalf("expected %q to be outside %q", outside, baseDir)
	}
}

func TestRegistryHelperRemoteAndFileUtilities(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != remoteRegistryUserAgent {
			http.Error(w, "missing user agent", http.StatusBadRequest)
			return
		}
		switch r.URL.Path {
		case "/json":
			_, _ = w.Write([]byte(`{"name":"ok"}`))
		case "/text":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte("hello"))
		case "/bad-status":
			http.Error(w, "bad", http.StatusBadGateway)
		case "/bad-json":
			_, _ = w.Write([]byte(`{`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := newRemoteClient(2 * time.Second)
	if client.Timeout != 2*time.Second {
		t.Fatalf("unexpected remote client timeout: %s", client.Timeout)
	}
	downloadClient := newRemoteDownloadClient(3 * time.Second)
	if downloadClient.Timeout != 3*time.Second {
		t.Fatalf("unexpected download client timeout: %s", downloadClient.Timeout)
	}
	if err := downloadClient.CheckRedirect(nil, make([]*http.Request, 10)); err == nil {
		t.Fatal("expected too many redirects error")
	}

	resp, err := doRemoteRequest(context.Background(), client, server.URL+"/json")
	if err != nil {
		t.Fatalf("doRemoteRequest: %v", err)
	}
	resp.Body.Close()

	body, err := fetchRemoteBody(context.Background(), client, server.URL+"/json")
	if err != nil || string(body) != `{"name":"ok"}` {
		t.Fatalf("unexpected fetchRemoteBody result %q, %v", body, err)
	}
	if _, err := fetchRemoteBody(context.Background(), client, server.URL+"/bad-status"); err == nil {
		t.Fatal("expected fetchRemoteBody status error")
	}

	var target map[string]string
	if err := fetchRemoteJSON(context.Background(), client, server.URL+"/json", &target); err != nil || target["name"] != "ok" {
		t.Fatalf("unexpected fetchRemoteJSON result %#v, %v", target, err)
	}
	if err := fetchRemoteJSON(context.Background(), client, server.URL+"/bad-json", &target); err == nil {
		t.Fatal("expected fetchRemoteJSON invalid json error")
	}

	text, status, err := fetchRemoteText(context.Background(), client, server.URL+"/text")
	if err != nil || status != http.StatusCreated || text != "hello" {
		t.Fatalf("unexpected fetchRemoteText result %q %d %v", text, status, err)
	}

	if normalizeSearchLimit(0) != defaultSkillSearchLimit || normalizeSearchLimit(7) != 7 {
		t.Fatal("unexpected normalized search limit")
	}
	if firstNonEmpty("", "  ", "alpha", "beta") != "alpha" {
		t.Fatal("expected firstNonEmpty to skip blanks")
	}
}

func TestRegistryHelperMarshalAndWriteHelpers(t *testing.T) {
	dir := t.TempDir()
	definition := skillFileDefinition{
		Name:           "writer",
		Description:    "Writes docs",
		Version:        "1.0.0",
		Category:       "content",
		InstallCommand: "anyclaw skill install writer",
		Prompts:        map[string]string{"system": "Write clearly."},
	}

	data, err := marshalSkillJSON(definition)
	if err != nil {
		t.Fatalf("marshalSkillJSON: %v", err)
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		t.Fatalf("expected marshaled JSON to end with newline: %q", data)
	}

	if err := writeSkillJSONFile(filepath.Join(dir, "one"), data); err != nil {
		t.Fatalf("writeSkillJSONFile: %v", err)
	}
	if err := writeSkillFile(filepath.Join(dir, "two"), definition); err != nil {
		t.Fatalf("writeSkillFile: %v", err)
	}
	if err := installSkillDefinition(dir, "three", definition); err != nil {
		t.Fatalf("installSkillDefinition: %v", err)
	}

	entries := buildCatalogEntries([]catalogEntrySpec{{
		Name:         "writer",
		FullName:     "Writer",
		Description:  "Writes docs",
		Version:      "1.0.0",
		Category:     "content",
		Registry:     "builtin",
		Homepage:     "https://example.com",
		Source:       "builtin",
		Permissions:  []string{"files:write"},
		Entrypoint:   "builtin://writer",
		InstallHint:  "anyclaw skill install writer",
		Installed:    true,
		InstalledDir: filepath.Join(dir, "three"),
		Builtin:      true,
	}})
	if len(entries) != 1 || entries[0].Category != "content" || !entries[0].Builtin {
		t.Fatalf("unexpected catalog entries: %#v", entries)
	}
}

func TestDoRemoteRequestRejectsBadURL(t *testing.T) {
	client := newRemoteClient(time.Second)
	if _, err := doRemoteRequest(context.Background(), client, "://bad"); err == nil {
		t.Fatal("expected bad url error")
	}
}
