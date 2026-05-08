package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/1024XEngineer/anyclaw/pkg/config"
	"github.com/1024XEngineer/anyclaw/pkg/marketplace"
	appRuntime "github.com/1024XEngineer/anyclaw/pkg/runtime"
)

func TestMarketArtifactsCloudUsesRegistryEndpoint(t *testing.T) {
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/artifacts" {
			t.Fatalf("unexpected registry path %s", r.URL.Path)
		}
		if r.URL.Query().Get("kind") != "skill" {
			t.Fatalf("expected kind=skill, got %q", r.URL.Query().Get("kind"))
		}
		want := map[string]string{
			"arch":       "amd64",
			"os":         "windows",
			"permission": "fs.read",
			"publisher":  "AnyClaw Labs",
			"q":          "release",
			"risk":       "low",
			"sort":       "updated",
			"tag":        "docs",
			"trust":      "verified",
		}
		for key, value := range want {
			if got := r.URL.Query().Get(key); got != value {
				t.Fatalf("expected %s=%q, got %q", key, value, got)
			}
		}
		writeRegistryJSON(t, w, map[string]any{
			"data": map[string]any{
				"items": []map[string]any{{
					"id":             "cloud.skill.release-notes",
					"kind":           "skill",
					"name":           "Release Notes Writer",
					"summary":        "Writes release notes.",
					"latest_version": "1.0.0",
					"source":         "anyclaw-cloud",
					"publisher":      "AnyClaw Labs",
					"risk_level":     "low",
					"trust_level":    "verified",
					"permissions":    []string{"fs.read"},
					"compatibility":  map[string]any{"anyclaw_min": "0.1.0"},
				}},
				"total":  1,
				"limit":  50,
				"offset": 0,
			},
		})
	}))
	defer registry.Close()

	server := newCloudMarketTestServer(t, registry.URL)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/market/artifacts?source=cloud&kind=skill&q=release&risk=low&trust=verified&tag=docs&permission=fs.read&publisher=AnyClaw%20Labs&os=windows&arch=amd64&sort=updated", nil)
	server.handleMarketArtifacts(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Data struct {
			Items []struct {
				ID      string `json:"id"`
				Source  string `json:"source"`
				Status  string `json:"status"`
				Owner   string `json:"owner"`
				Enabled bool   `json:"enabled"`
			} `json:"items"`
			Total int `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.Total != 1 || len(payload.Data.Items) != 1 {
		t.Fatalf("unexpected payload: %#v", payload.Data)
	}
	item := payload.Data.Items[0]
	if item.ID != "cloud.skill.release-notes" || item.Source != "cloud" || item.Status != "available" || !item.Enabled {
		t.Fatalf("unexpected cloud artifact: %#v", item)
	}
	if item.Owner != "AnyClaw Labs" {
		t.Fatalf("unexpected owner %q", item.Owner)
	}
}

func TestMarketArtifactsCloudUnavailableDegradesToEmptyList(t *testing.T) {
	server := newCloudMarketTestServer(t, "http://127.0.0.1:1")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/market/artifacts?source=cloud&kind=agent", nil)
	server.handleMarketArtifacts(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Data struct {
			Items []any `json:"items"`
			Total int   `json:"total"`
		} `json:"data"`
		Meta struct {
			CloudError string `json:"cloud_error"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.Total != 0 || len(payload.Data.Items) != 0 {
		t.Fatalf("expected empty degraded list, got %#v", payload.Data)
	}
	if payload.Meta.CloudError == "" {
		t.Fatal("expected cloud_error metadata")
	}
}

func TestMarketArtifactCloudDetailAndVersions(t *testing.T) {
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/artifacts/anyclaw.agent.marketplace-operator":
			writeRegistryJSON(t, w, map[string]any{"data": map[string]any{
				"id":             "anyclaw.agent.marketplace-operator",
				"kind":           "agent",
				"name":           "Marketplace Operator",
				"summary":        "Runs marketplace releases.",
				"latest_version": "1.0.0",
				"source":         "anyclaw-cloud",
				"publisher":      "AnyClaw Labs",
				"risk_level":     "medium",
				"trust_level":    "verified",
				"permissions":    []string{"fs.read"},
				"compatibility":  map[string]any{"anyclaw_min": "0.1.0"},
			}})
		case "/v1/artifacts/anyclaw.agent.marketplace-operator/versions":
			writeRegistryJSON(t, w, map[string]any{"data": map[string]any{
				"items": []map[string]any{{
					"version":     "1.0.0",
					"size_bytes":  128,
					"released_at": "2026-05-07T00:00:00Z",
				}},
				"total": 1,
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer registry.Close()

	server := newCloudMarketTestServer(t, registry.URL)
	rec := httptest.NewRecorder()
	server.handleMarketArtifactDetail(rec, httptest.NewRequest(http.MethodGet, "/market/artifacts/anyclaw.agent.marketplace-operator?source=cloud", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("detail status = %d, body = %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	server.handleMarketArtifactDetail(rec, httptest.NewRequest(http.MethodGet, "/market/artifacts/anyclaw.agent.marketplace-operator/versions?source=cloud", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("versions status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestMarketArtifactCloudDetailFallsBackToCloudForUnknownNonCloudPrefix(t *testing.T) {
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/artifacts/anyclaw.skill.skill-author" {
			t.Fatalf("unexpected registry path %s", r.URL.Path)
		}
		writeRegistryJSON(t, w, map[string]any{"data": map[string]any{
			"id":             "anyclaw.skill.skill-author",
			"kind":           "skill",
			"name":           "Skill Author",
			"summary":        "Creates marketplace-ready skills.",
			"latest_version": "1.0.0",
			"source":         "anyclaw-cloud",
			"publisher":      "AnyClaw Labs",
			"risk_level":     "low",
			"trust_level":    "verified",
			"permissions":    []string{"fs.read"},
			"compatibility":  map[string]any{"anyclaw_min": "0.1.0"},
		}})
	}))
	defer registry.Close()

	server := newCloudMarketTestServer(t, registry.URL)
	rec := httptest.NewRecorder()
	server.handleMarketArtifactDetail(rec, httptest.NewRequest(http.MethodGet, "/market/artifacts/anyclaw.skill.skill-author", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("detail status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestMarketArtifactHandlersValidateMethodRuntimeAndPaths(t *testing.T) {
	rec := httptest.NewRecorder()
	(*Server)(nil).handleMarketArtifacts(rec, httptest.NewRequest(http.MethodGet, "/market/artifacts", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("nil artifacts status = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	newCloudMarketTestServer(t, "").handleMarketArtifacts(rec, httptest.NewRequest(http.MethodPost, "/market/artifacts", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method status = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	newCloudMarketTestServer(t, "").handleMarketArtifactDetail(rec, httptest.NewRequest(http.MethodGet, "/market/artifacts", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("collection path status = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	(*Server)(nil).handleMarketArtifactDetail(rec, httptest.NewRequest(http.MethodGet, "/market/artifacts/skill:missing", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("nil detail status = %d", rec.Code)
	}
}

func TestMarketArtifactHelpers(t *testing.T) {
	if id, versions := parseMarketArtifactPath("/market/artifacts/skill:release/versions"); id != "skill:release" || !versions {
		t.Fatalf("unexpected versions path parse id=%q versions=%v", id, versions)
	}
	if id, versions := parseMarketArtifactPath("/market/artifacts/"); id != "" || versions {
		t.Fatalf("unexpected empty path parse id=%q versions=%v", id, versions)
	}
	empty := emptyMarketList(marketplace.Filter{Limit: -1, Offset: -4})
	if empty.Limit != 50 || empty.Offset != 0 || empty.Total != 0 || len(empty.Items) != 0 {
		t.Fatalf("unexpected empty list: %#v", empty)
	}
	server := newCloudMarketTestServer(t, "")
	if server.cloudRegistryClient() != nil {
		t.Fatal("expected nil cloud client without endpoint")
	}
	if !server.shouldUseCloudMarketArtifact(httptest.NewRequest(http.MethodGet, "/market/artifacts/cloud.skill.x", nil), "cloud.skill.x") {
		t.Fatal("expected cloud prefix to use cloud")
	}
	if !server.shouldUseCloudMarketArtifact(httptest.NewRequest(http.MethodGet, "/market/artifacts/local?source=cloud", nil), "local") {
		t.Fatal("expected source=cloud to use cloud")
	}
	if _, err := server.cloudMarketArtifact(httptest.NewRequest(http.MethodGet, "/market/artifacts/x", nil), "x"); err == nil {
		t.Fatal("expected missing cloud client error")
	}
}

func newCloudMarketTestServer(t *testing.T, endpoint string) *Server {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.Marketplace.RegistryEndpoint = endpoint
	cfg.Marketplace.RequestTimeoutSeconds = 1
	cfg.Marketplace.CacheTTLSeconds = 0
	return &Server{
		mainRuntime: &appRuntime.MainRuntime{
			Config:     cfg,
			WorkingDir: t.TempDir(),
		},
	}
}

func writeRegistryJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatal(err)
	}
}
