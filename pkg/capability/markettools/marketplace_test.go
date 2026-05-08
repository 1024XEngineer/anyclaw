package markettools

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/1024XEngineer/anyclaw/pkg/capability/tools"
	"github.com/1024XEngineer/anyclaw/pkg/marketplace"
	marketregistry "github.com/1024XEngineer/anyclaw/pkg/marketplace/registry"
)

func TestRegisterMarketplaceToolsMainAgentOnly(t *testing.T) {
	registry := tools.NewRegistry()
	Register(registry, Options{Store: marketplace.NewStore(t.TempDir())})
	if _, ok := registry.Get("market_search_artifacts"); !ok {
		t.Fatal("expected market_search_artifacts tool")
	}
	if subTools := registry.ListForRole(true); toolListed(subTools, "market_search_artifacts") {
		t.Fatalf("market tools should be main-agent only: %#v", subTools)
	}
	_, err := registry.Call(tools.WithToolCaller(context.Background(), tools.ToolCaller{Role: tools.ToolCallerRoleSubAgent}), "market_search_artifacts", map[string]any{
		"query": "release notes",
	})
	if err == nil || !strings.Contains(err.Error(), "not available for caller role sub_agent") {
		t.Fatalf("sub-agent call err = %v, want visibility denial", err)
	}
}

func TestSearchToolRoutesMissingCapabilityToCloud(t *testing.T) {
	server := testMarketRegistryServer(t, "cloud.skill.release-notes", "skill", "low", "verified", []string{"fs.read"}, nil)
	defer server.Close()

	registry := tools.NewRegistry()
	Register(registry, Options{
		Store:    marketplace.NewStore(t.TempDir()),
		Registry: marketregistry.NewClient(marketregistry.ClientConfig{Endpoint: server.URL}),
	})
	out, err := registry.Call(tools.WithToolCaller(context.Background(), tools.ToolCaller{Role: tools.ToolCallerRoleMainAgent}), "market_search_artifacts", map[string]any{
		"query": "please write release notes",
		"kind":  "skill",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"action": "install_from_market"`) || !strings.Contains(out, "cloud.skill.release-notes") {
		t.Fatalf("output = %s, want cloud install route", out)
	}
}

func TestInstallToolAskReturnsConfirmationWithoutInstalling(t *testing.T) {
	archive := testMarketToolArchive(t, "cloud.agent.code-reviewer", marketplace.ArtifactKindAgent, "1.0.0")
	server := testMarketRegistryServer(t, "cloud.agent.code-reviewer", "agent", "medium", "verified", []string{"fs.read"}, archive)
	defer server.Close()

	store := marketplace.NewStore(t.TempDir())
	registry := tools.NewRegistry()
	Register(registry, Options{Store: store, Registry: marketregistry.NewClient(marketregistry.ClientConfig{Endpoint: server.URL})})
	out, err := registry.Call(tools.WithToolCaller(context.Background(), tools.ToolCaller{Role: tools.ToolCallerRoleMainAgent}), "market_install_artifact", map[string]any{
		"artifact_id": "cloud.agent.code-reviewer",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "requires_confirmation") {
		t.Fatalf("output = %s, want confirmation", out)
	}
	if _, err := store.LatestReceiptForArtifact("cloud.agent.code-reviewer"); err != marketplace.ErrArtifactNotFound {
		t.Fatalf("receipt err = %v, want not found", err)
	}
}

func TestInstallToolConfirmedInstallsAsAgent(t *testing.T) {
	archive := testMarketToolArchive(t, "cloud.skill.release-notes", marketplace.ArtifactKindSkill, "1.0.0")
	server := testMarketRegistryServer(t, "cloud.skill.release-notes", "skill", "low", "verified", []string{"fs.read"}, archive)
	defer server.Close()

	store := marketplace.NewStore(t.TempDir())
	registry := tools.NewRegistry()
	Register(registry, Options{Store: store, Registry: marketregistry.NewClient(marketregistry.ClientConfig{Endpoint: server.URL}), AutoInstallSkill: true})
	out, err := registry.Call(tools.WithToolCaller(context.Background(), tools.ToolCaller{Role: tools.ToolCallerRoleMainAgent}), "market_install_artifact", map[string]any{
		"artifact_id": "cloud.skill.release-notes",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "installed") {
		t.Fatalf("output = %s, want installed", out)
	}
	receipt, err := store.LatestReceiptForArtifact("cloud.skill.release-notes")
	if err != nil {
		t.Fatal(err)
	}
	if receipt.InstalledBy != "agent" {
		t.Fatalf("installed_by = %q, want agent", receipt.InstalledBy)
	}
}

func TestBindToolCreatesAgentBinding(t *testing.T) {
	store := marketplace.NewStore(t.TempDir())
	if err := store.SaveReceipt(&marketplace.InstallReceipt{
		ID:            "cloud.skill.release-notes@1.0.0",
		ArtifactID:    "cloud.skill.release-notes",
		Kind:          marketplace.ArtifactKindSkill,
		Name:          "Release Notes",
		Version:       "1.0.0",
		Source:        marketplace.SourceCloud,
		InstalledPath: t.TempDir(),
		InstalledBy:   "agent",
		InstalledAt:   "2026-05-07T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	registry := tools.NewRegistry()
	Register(registry, Options{Store: store})
	out, err := registry.Call(tools.WithToolCaller(context.Background(), tools.ToolCaller{Role: tools.ToolCallerRoleMainAgent}), "market_bind_artifact", map[string]any{
		"artifact_id": "cloud.skill.release-notes",
		"target_type": "runtime_global",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "bound") {
		t.Fatalf("output = %s, want bound", out)
	}
}

func TestMarketToolsValidationAndHelpers(t *testing.T) {
	if _, err := installArtifact(context.Background(), Options{}, map[string]any{"artifact_id": "x"}); err == nil {
		t.Fatal("expected install not configured error")
	}
	if _, err := installArtifact(context.Background(), Options{Store: marketplace.NewStore(t.TempDir()), Registry: marketregistry.NewClient(marketregistry.ClientConfig{Endpoint: "http://127.0.0.1:1"})}, map[string]any{}); err == nil || !strings.Contains(err.Error(), "artifact_id is required") {
		t.Fatalf("expected artifact id error, got %v", err)
	}
	if _, err := bindArtifact(context.Background(), Options{Store: marketplace.NewStore(t.TempDir())}, map[string]any{"artifact_id": "x"}); err == nil || !strings.Contains(err.Error(), "artifact_id and target_type") {
		t.Fatalf("expected bind validation error, got %v", err)
	}
	if stringValue(123) != "123" || !boolValue("true") || boolValue("false") || intValue(float64(7), 1) != 7 || intValue("bad", 9) != 9 {
		t.Fatal("market tool scalar helpers mismatch")
	}
	if firstNonEmpty("", " value ") != "value" {
		t.Fatal("firstNonEmpty mismatch")
	}
	out, err := marshalJSON(map[string]any{"ok": true})
	if err != nil || !strings.Contains(out, `"ok": true`) {
		t.Fatalf("marshalJSON = %q err=%v", out, err)
	}
}

func TestSearchArtifactsCloudOnlyAndLocalLimit(t *testing.T) {
	archive := testMarketToolArchive(t, "cloud.skill.release-notes", marketplace.ArtifactKindSkill, "1.0.0")
	server := testMarketRegistryServer(t, "cloud.skill.release-notes", "skill", "low", "verified", []string{"fs.read"}, archive)
	defer server.Close()

	store := marketplace.NewStore(t.TempDir())
	if err := store.SaveReceipt(&marketplace.InstallReceipt{
		ID:            "local.skill@1.0.0",
		ArtifactID:    "local.skill",
		Kind:          marketplace.ArtifactKindSkill,
		Name:          "Local Skill",
		Version:       "1.0.0",
		Source:        marketplace.SourceCloud,
		InstalledPath: t.TempDir(),
		InstalledAt:   "2026-05-07T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	out, err := searchArtifacts(context.Background(), Options{
		Store:    store,
		Registry: marketregistry.NewClient(marketregistry.ClientConfig{Endpoint: server.URL}),
	}, map[string]any{"query": "release", "kind": "skill", "source": "cloud", "limit": 1})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "Local Skill") || !strings.Contains(out, "cloud.skill.release-notes") {
		t.Fatalf("unexpected cloud-only search output: %s", out)
	}
	local, err := localArtifacts(store, marketplace.ArtifactKindSkill, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(local) != 1 || local[0].ID != "local.skill" {
		t.Fatalf("unexpected local artifacts: %#v", local)
	}
}

func toolListed(items []tools.ToolInfo, name string) bool {
	for _, item := range items {
		if item.Name == name {
			return true
		}
	}
	return false
}

func testMarketRegistryServer(t *testing.T, id, kind, risk, trust string, permissions []string, archive []byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/resolve"):
			writeMarketToolJSON(t, w, map[string]any{"data": map[string]any{
				"artifact_id":     id,
				"version":         "1.0.0",
				"download_url":    "http://" + r.Host + "/v1/download/" + id + "/1.0.0",
				"checksum_sha256": sha256MarketTool(archive),
				"size_bytes":      len(archive),
				"risk_level":      risk,
				"trust_level":     trust,
				"permissions":     permissions,
				"kind":            kind,
				"name":            id,
			}})
		case strings.Contains(r.URL.Path, "/v1/download/"):
			_, _ = w.Write(archive)
		case r.URL.Path == "/v1/artifacts":
			writeMarketToolJSON(t, w, map[string]any{"data": map[string]any{
				"items": []any{map[string]any{
					"id":             id,
					"kind":           kind,
					"name":           id,
					"summary":        id,
					"version":        "1.0.0",
					"latest_version": "1.0.0",
					"publisher":      "AnyClaw",
					"risk_level":     risk,
					"trust_level":    trust,
					"permissions":    permissions,
					"tags":           []string{"release notes", "changelog", "code review", "pull request"},
					"hit_signals":    []string{"release notes", "code review"},
					"score":          0.91,
				}},
				"total":  1,
				"limit":  10,
				"offset": 0,
			}})
		default:
			http.NotFound(w, r)
		}
	}))
}

func testMarketToolArchive(t *testing.T, id string, kind marketplace.ArtifactKind, version string) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	w, err := writer.Create("anyclaw.artifact.json")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(map[string]any{"id": id, "kind": kind, "name": id, "version": version})
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func writeMarketToolJSON(t *testing.T, w http.ResponseWriter, payload any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatal(err)
	}
}

func sha256MarketTool(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
