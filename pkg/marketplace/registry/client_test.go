package registry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/marketplace"
)

func TestClientListConvertsCloudArtifactsAndCaches(t *testing.T) {
	var listCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/artifacts" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		listCalls++
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("Authorization = %q", got)
		}
		writeTestJSON(t, w, map[string]any{
			"data": map[string]any{
				"items": []map[string]any{
					{
						"id":             "cloud.skill.release-notes",
						"kind":           "skill",
						"name":           "Release Notes Writer",
						"summary":        "Writes release notes.",
						"description_md": "Detailed release notes skill.",
						"latest_version": "1.0.0",
						"source":         "anyclaw-cloud",
						"publisher":      "AnyClaw Labs",
						"risk_level":     "low",
						"trust_level":    "verified",
						"permissions":    []string{"fs.read"},
						"compatibility":  map[string]any{"anyclaw_min": "0.1.0", "os": []string{"windows"}},
						"tags":           []string{"release"},
						"hit_signals":    []string{"changelog"},
						"score":          0.9,
					},
				},
				"total":  1,
				"limit":  10,
				"offset": 0,
			},
		})
	}))
	defer server.Close()

	client := NewClient(ClientConfig{
		Endpoint:        server.URL,
		Token:           "test-token",
		ProtocolVersion: "1.0",
		CacheTTL:        time.Minute,
	})
	result, err := client.List(context.Background(), marketplace.Filter{
		Kind:  marketplace.ArtifactKindSkill,
		Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || len(result.Items) != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
	item := result.Items[0]
	if item.Source != marketplace.SourceCloud || item.Status != marketplace.StatusAvailable {
		t.Fatalf("unexpected source/status: %#v", item)
	}
	if item.Owner != "AnyClaw Labs" || !item.Verified {
		t.Fatalf("unexpected owner/trust: %#v", item)
	}
	if item.Description != "Detailed release notes skill." {
		t.Fatalf("unexpected description %q", item.Description)
	}

	if _, err := client.List(context.Background(), marketplace.Filter{Kind: marketplace.ArtifactKindSkill, Limit: 10}); err != nil {
		t.Fatal(err)
	}
	if listCalls != 1 {
		t.Fatalf("expected cached second list call, got %d server calls", listCalls)
	}
}

func TestClientDetailVersionsAndResolve(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/artifacts/cloud.agent.code-reviewer":
			writeTestJSON(t, w, map[string]any{"data": testRemoteArtifact("agent")})
		case "/v1/artifacts/cloud.agent.code-reviewer/versions":
			writeTestJSON(t, w, map[string]any{
				"data": map[string]any{
					"items": []map[string]any{{
						"version":          "1.0.0",
						"released_at":      "2026-05-07T00:00:00Z",
						"changelog_md":     "Initial release.",
						"permissions_diff": []string{"fs.read"},
						"size_bytes":       128,
					}},
					"total": 1,
				},
			})
		case "/v1/artifacts/cloud.agent.code-reviewer/resolve":
			if r.Method != http.MethodPost {
				t.Fatalf("resolve method = %s", r.Method)
			}
			writeTestJSON(t, w, map[string]any{
				"data": map[string]any{
					"artifact_id":     "cloud.agent.code-reviewer",
					"version":         "1.0.0",
					"download_url":    serverURL(r) + "/v1/download/cloud.agent.code-reviewer/1.0.0",
					"checksum_sha256": "abc",
					"size_bytes":      128,
					"compatibility":   map[string]any{"anyclaw_min": "0.1.0"},
					"risk_level":      "medium",
					"trust_level":     "verified",
					"permissions":     []string{"fs.read"},
					"kind":            "agent",
					"name":            "Code Reviewer",
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(ClientConfig{Endpoint: server.URL})
	artifact, err := client.Get(context.Background(), "cloud.agent.code-reviewer")
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Kind != marketplace.ArtifactKindAgent || artifact.Source != marketplace.SourceCloud {
		t.Fatalf("unexpected artifact: %#v", artifact)
	}

	versions, err := client.Versions(context.Background(), "cloud.agent.code-reviewer")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 || versions[0].Version != "1.0.0" || versions[0].SizeBytes != 128 {
		t.Fatalf("unexpected versions: %#v", versions)
	}

	resolved, err := client.Resolve(context.Background(), "cloud.agent.code-reviewer", ResolveRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ArtifactID != "cloud.agent.code-reviewer" || resolved.DownloadURL == "" {
		t.Fatalf("unexpected resolve result: %#v", resolved)
	}
}

func TestClientAcceptsEndpointWithV1Path(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/artifacts" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		writeTestJSON(t, w, map[string]any{
			"data": map[string]any{
				"items": []map[string]any{testRemoteArtifact("skill")},
				"total": 1,
				"limit": 1,
			},
		})
	}))
	defer server.Close()

	client := NewClient(ClientConfig{Endpoint: server.URL + "/v1"})
	result, err := client.List(context.Background(), marketplace.Filter{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || len(result.Items) != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func testRemoteArtifact(kind string) map[string]any {
	return map[string]any{
		"id":             "cloud." + kind + ".code-reviewer",
		"kind":           kind,
		"name":           "Code Reviewer",
		"summary":        "Reviews code.",
		"latest_version": "1.0.0",
		"source":         "anyclaw-cloud",
		"publisher":      "AnyClaw Labs",
		"risk_level":     "medium",
		"trust_level":    "verified",
		"permissions":    []string{"fs.read"},
		"compatibility":  map[string]any{"anyclaw_min": "0.1.0"},
	}
}

func writeTestJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatal(err)
	}
}

func serverURL(r *http.Request) string {
	return "http://" + r.Host
}
