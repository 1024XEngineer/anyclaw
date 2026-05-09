package marketregistry

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestServerSeededCatalogRoutes(t *testing.T) {
	server := newTestServer(t)

	var list struct {
		Data ListResult `json:"data"`
	}
	doJSON(t, server, http.MethodGet, "/v1/artifacts", nil, http.StatusOK, &list)
	if list.Data.Total != 3 {
		t.Fatalf("expected 3 seeded artifacts, got %d", list.Data.Total)
	}

	byKind := map[ArtifactKind]bool{}
	for _, item := range list.Data.Items {
		byKind[item.Kind] = true
	}
	for _, kind := range []ArtifactKind{ArtifactKindAgent, ArtifactKindSkill, ArtifactKindCLI} {
		if !byKind[kind] {
			t.Fatalf("expected seeded kind %s in catalog", kind)
		}
	}

	var detail struct {
		Data Artifact `json:"data"`
	}
	doJSON(t, server, http.MethodGet, "/v1/artifacts/cloud.skill.release-notes", nil, http.StatusOK, &detail)
	if detail.Data.ID != "cloud.skill.release-notes" || detail.Data.Kind != ArtifactKindSkill {
		t.Fatalf("unexpected detail artifact: %#v", detail.Data)
	}

	var versions struct {
		Data VersionListResult `json:"data"`
	}
	doJSON(t, server, http.MethodGet, "/v1/artifacts/cloud.skill.release-notes/versions", nil, http.StatusOK, &versions)
	if versions.Data.Total != 1 {
		t.Fatalf("expected one version, got %d", versions.Data.Total)
	}
	if versions.Data.Items[0].ChecksumSHA256 == "" || versions.Data.Items[0].SizeBytes == 0 {
		t.Fatalf("expected seeded version checksum and size: %#v", versions.Data.Items[0])
	}
}

func TestServerResolveAndDownload(t *testing.T) {
	server := newTestServer(t)

	var resolved struct {
		Data ResolvedArtifact `json:"data"`
	}
	doJSON(t, server, http.MethodPost, "/v1/artifacts/cloud.cli.repo-health/resolve", strings.NewReader(`{}`), http.StatusOK, &resolved)
	if resolved.Data.ArtifactID != "cloud.cli.repo-health" {
		t.Fatalf("unexpected resolved artifact: %#v", resolved.Data)
	}
	if resolved.Data.DownloadURL == "" || resolved.Data.ChecksumSHA256 == "" || resolved.Data.SizeBytes == 0 {
		t.Fatalf("resolve response missing download metadata: %#v", resolved.Data)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/download/cloud.cli.repo-health/1.0.0", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("download status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-Checksum-SHA256"); got != resolved.Data.ChecksumSHA256 {
		t.Fatalf("download checksum header = %q, want %q", got, resolved.Data.ChecksumSHA256)
	}
	sum := sha256.Sum256(rec.Body.Bytes())
	if got := hex.EncodeToString(sum[:]); got != resolved.Data.ChecksumSHA256 {
		t.Fatalf("download body checksum = %q, want %q", got, resolved.Data.ChecksumSHA256)
	}
	assertZipContains(t, rec.Body.Bytes(), "anyclaw.artifact.json")
}

func TestServerSearchAndErrors(t *testing.T) {
	server := newTestServer(t)

	var search struct {
		Data ListResult `json:"data"`
	}
	doJSON(t, server, http.MethodPost, "/v1/search", strings.NewReader(`{"kind":"skill","q":"release"}`), http.StatusOK, &search)
	if search.Data.Total != 1 || search.Data.Items[0].ID != "cloud.skill.release-notes" {
		t.Fatalf("unexpected search result: %#v", search.Data)
	}

	var missing ErrorResponse
	doJSON(t, server, http.MethodGet, "/v1/artifacts/does-not-exist", nil, http.StatusNotFound, &missing)
	if missing.Error.Code != "not_found" {
		t.Fatalf("unexpected error response: %#v", missing)
	}
}

func TestServerSearchUsesLexicalIndexAndScores(t *testing.T) {
	server := newTestServer(t)

	var search struct {
		Data ListResult `json:"data"`
	}
	doJSON(t, server, http.MethodPost, "/v1/search", strings.NewReader(`{"kind":"skill","q":"relea"}`), http.StatusOK, &search)
	if search.Data.Total != 1 || search.Data.Items[0].ID != "cloud.skill.release-notes" {
		t.Fatalf("unexpected lexical search result: %#v", search.Data)
	}
	item := search.Data.Items[0]
	if item.FinalScore <= 0 || item.LexicalScore <= 0 {
		t.Fatalf("expected score breakdown in search result: %#v", item)
	}
	if item.Score != item.FinalScore {
		t.Fatalf("expected score to mirror final_score, got score=%v final=%v", item.Score, item.FinalScore)
	}
	if len(item.MatchSignals) == 0 {
		t.Fatalf("expected match signals in search result: %#v", item)
	}
}

func TestServerSearchFiltersCombine(t *testing.T) {
	server := newTestServer(t)

	var list struct {
		Data ListResult `json:"data"`
	}
	doJSON(t, server, http.MethodGet, "/v1/artifacts?kind=skill&q=release&risk=low&trust=verified&tag=writing&publisher=AnyClaw%20Labs&permission=fs.read&os=windows&arch=amd64&sort=name", nil, http.StatusOK, &list)
	if list.Data.Total != 1 || list.Data.Items[0].ID != "cloud.skill.release-notes" {
		t.Fatalf("unexpected combined filter result: %#v", list.Data)
	}

	var empty struct {
		Data ListResult `json:"data"`
	}
	doJSON(t, server, http.MethodGet, "/v1/artifacts?kind=skill&q=release&risk=high&trust=verified&tag=writing&publisher=AnyClaw%20Labs", nil, http.StatusOK, &empty)
	if empty.Data.Total != 0 {
		t.Fatalf("expected no results when one filter mismatches, got %#v", empty.Data)
	}
}

func TestServerListSupportsStructuredFilteringWithoutQuery(t *testing.T) {
	server, err := NewServer(context.Background(), ServerConfig{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })

	now := "2026-05-08T10:00:00Z"
	for _, artifact := range []Artifact{
		{
			ID:            "cloud.skill.alpha-writer",
			Kind:          ArtifactKindSkill,
			Name:          "Alpha Writer",
			Summary:       "Writes release notes.",
			LatestVersion: "1.0.0",
			Source:        defaultRegistrySourceID,
			Publisher:     "AnyClaw Labs",
			RiskLevel:     "low",
			TrustLevel:    "verified",
			Permissions:   []string{"fs.read"},
			Compatibility: Compatibility{OS: []string{"windows"}, Arch: []string{"amd64"}},
			Tags:          []string{"writer", "notes"},
			UpdatedAt:     now,
		},
		{
			ID:            "cloud.skill.beta-audit",
			Kind:          ArtifactKindSkill,
			Name:          "Beta Audit",
			Summary:       "Audits permissions.",
			LatestVersion: "1.0.0",
			Source:        defaultRegistrySourceID,
			Publisher:     "Community Labs",
			RiskLevel:     "medium",
			TrustLevel:    "community",
			Permissions:   []string{"process.exec"},
			Compatibility: Compatibility{OS: []string{"linux"}, Arch: []string{"arm64"}},
			Tags:          []string{"audit"},
			UpdatedAt:     now,
		},
	} {
		if err := server.store.UpsertArtifact(context.Background(), artifact, nil); err != nil {
			t.Fatalf("upsert artifact %s: %v", artifact.ID, err)
		}
	}

	var list struct {
		Data ListResult `json:"data"`
	}
	doJSON(t, server, http.MethodGet, "/v1/artifacts?kind=skill&tag=writer&permission=fs.read&publisher=AnyClaw%20Labs&os=windows&arch=amd64", nil, http.StatusOK, &list)
	if list.Data.Total != 1 || list.Data.Items[0].ID != "cloud.skill.alpha-writer" {
		t.Fatalf("unexpected structured filter result without query: %#v", list.Data)
	}
}

func TestServerListSortsByUpdatedAndRiskAwareScore(t *testing.T) {
	server, err := NewServer(context.Background(), ServerConfig{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })

	artifacts := []Artifact{
		{
			ID:            "cloud.skill.safe-writer",
			Kind:          ArtifactKindSkill,
			Name:          "Safe Writer",
			Summary:       "Writes summaries safely.",
			LatestVersion: "1.0.0",
			Source:        defaultRegistrySourceID,
			Publisher:     "AnyClaw Labs",
			RiskLevel:     "low",
			TrustLevel:    "verified",
			Permissions:   []string{"fs.read"},
			Tags:          []string{"writer"},
			UpdatedAt:     "2026-05-01T10:00:00Z",
		},
		{
			ID:            "cloud.skill.risky-writer",
			Kind:          ArtifactKindSkill,
			Name:          "Risky Writer",
			Summary:       "Writes summaries with elevated permissions.",
			LatestVersion: "1.0.0",
			Source:        defaultRegistrySourceID,
			Publisher:     "AnyClaw Labs",
			RiskLevel:     "high",
			TrustLevel:    "verified",
			Permissions:   []string{"process.exec"},
			Tags:          []string{"writer"},
			UpdatedAt:     "2026-05-01T10:00:00Z",
		},
		{
			ID:            "cloud.skill.new-writer",
			Kind:          ArtifactKindSkill,
			Name:          "New Writer",
			Summary:       "Newest writing helper.",
			LatestVersion: "1.0.0",
			Source:        defaultRegistrySourceID,
			Publisher:     "AnyClaw Labs",
			RiskLevel:     "low",
			TrustLevel:    "verified",
			Permissions:   []string{"fs.read"},
			Tags:          []string{"writer"},
			UpdatedAt:     "2026-05-08T10:00:00Z",
		},
	}
	for _, artifact := range artifacts {
		if err := server.store.UpsertArtifact(context.Background(), artifact, nil); err != nil {
			t.Fatalf("upsert artifact %s: %v", artifact.ID, err)
		}
	}

	var scoreList struct {
		Data ListResult `json:"data"`
	}
	doJSON(t, server, http.MethodGet, "/v1/artifacts?kind=skill&tag=writer", nil, http.StatusOK, &scoreList)
	if scoreList.Data.Total != 3 {
		t.Fatalf("expected 3 score-sorted items, got %#v", scoreList.Data)
	}
	if scoreList.Data.Items[0].ID != "cloud.skill.new-writer" || scoreList.Data.Items[2].ID != "cloud.skill.risky-writer" {
		t.Fatalf("unexpected default score order: %#v", scoreList.Data.Items)
	}

	var updatedList struct {
		Data ListResult `json:"data"`
	}
	doJSON(t, server, http.MethodGet, "/v1/artifacts?kind=skill&tag=writer&sort=updated", nil, http.StatusOK, &updatedList)
	if updatedList.Data.Total != 3 {
		t.Fatalf("expected 3 updated-sorted items, got %#v", updatedList.Data)
	}
	if updatedList.Data.Items[0].ID != "cloud.skill.new-writer" {
		t.Fatalf("expected newest item first for updated sort, got %#v", updatedList.Data.Items)
	}
}

func TestServerSearchModeLexicalMeta(t *testing.T) {
	server := newTestServer(t)

	var search struct {
		Data ListResult   `json:"data"`
		Meta ResponseMeta `json:"meta"`
	}
	doJSON(t, server, http.MethodGet, "/v1/artifacts?kind=skill&q=release&search_mode=lexical", nil, http.StatusOK, &search)
	if search.Meta.SearchMode != SearchModeLexical {
		t.Fatalf("expected lexical meta search mode, got %#v", search.Meta)
	}
	if search.Meta.VectorApplied == nil || *search.Meta.VectorApplied {
		t.Fatalf("expected vector_applied=false, got %#v", search.Meta)
	}
	if search.Data.RetrievalMeta == nil || search.Data.RetrievalMeta.SearchMode != SearchModeLexical {
		t.Fatalf("expected retrieval meta in data, got %#v", search.Data.RetrievalMeta)
	}
}

func TestServerSearchModeHybridFallsBackOpen(t *testing.T) {
	server := newTestServer(t)

	var search struct {
		Data ListResult   `json:"data"`
		Meta ResponseMeta `json:"meta"`
	}
	doJSON(t, server, http.MethodPost, "/v1/search", strings.NewReader(`{"kind":"skill","q":"release","search_mode":"hybrid"}`), http.StatusOK, &search)
	if search.Data.Total == 0 {
		t.Fatalf("expected lexical fallback results, got %#v", search.Data)
	}
	if search.Meta.SearchMode != SearchModeLexicalFallback {
		t.Fatalf("expected lexical_fallback meta, got %#v", search.Meta)
	}
	if search.Meta.VectorApplied == nil || *search.Meta.VectorApplied {
		t.Fatalf("expected vector_applied=false during fallback, got %#v", search.Meta)
	}
	if search.Meta.VectorFallbackReason != "disabled" {
		t.Fatalf("expected disabled fallback reason, got %#v", search.Meta)
	}
	if search.Data.RetrievalMeta == nil || search.Data.RetrievalMeta.SearchMode != SearchModeLexicalFallback {
		t.Fatalf("expected fallback retrieval meta in data, got %#v", search.Data.RetrievalMeta)
	}
}

func TestServerHybridSearchUsesVectorAndCanExpandBeyondLexicalHits(t *testing.T) {
	artifactProvider := &mockEmbeddingProvider{
		name: "mock-artifact",
		dim:  3,
		embeddings: map[string][]float32{
			buildArtifactEmbeddingText(Artifact{
				Kind:            ArtifactKindSkill,
				Name:            "Alpha Writer",
				Summary:         "Semantic authoring helper.",
				Publisher:       "AnyClaw Labs",
				Permissions:     []string{"fs.read"},
				ManifestSummary: map[string]string{"use_case": "semantic writing"},
			}): {0.99, 0.01, 0},
			buildArtifactEmbeddingText(Artifact{
				Kind:            ArtifactKindSkill,
				Name:            "Beta Release Tool",
				Summary:         "Release note generator.",
				Publisher:       "AnyClaw Labs",
				Permissions:     []string{"fs.read"},
				ManifestSummary: map[string]string{"use_case": "release notes"},
			}): {0.20, 0.80, 0},
		},
	}
	queryProvider := &mockEmbeddingProvider{
		name: "mock-query",
		dim:  3,
		embeddings: map[string][]float32{
			"semantic author": {1, 0, 0},
		},
	}
	server, err := NewServer(context.Background(), ServerConfig{
		DataDir: t.TempDir(),
		Vector: VectorConfig{
			Enabled:         true,
			FailOpen:        true,
			Model:           "mock-doc",
			QueryModel:      "mock-query",
			HybridTopK:      10,
			HybridCandidate: 20,
		},
		TestArtifactEmbed: artifactProvider,
		TestQueryEmbed:    queryProvider,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })

	alpha := Artifact{
		ID:              "cloud.skill.alpha-writer",
		Kind:            ArtifactKindSkill,
		Name:            "Alpha Writer",
		Summary:         "Semantic authoring helper.",
		LatestVersion:   "1.0.0",
		Source:          defaultRegistrySourceID,
		Publisher:       "AnyClaw Labs",
		RiskLevel:       "low",
		TrustLevel:      "verified",
		Permissions:     []string{"fs.read"},
		Tags:            []string{"semantic"},
		UpdatedAt:       "2026-05-08T10:00:00Z",
		ManifestSummary: map[string]string{"use_case": "semantic writing"},
	}
	beta := Artifact{
		ID:              "cloud.skill.beta-release",
		Kind:            ArtifactKindSkill,
		Name:            "Beta Release Tool",
		Summary:         "Release note generator.",
		LatestVersion:   "1.0.0",
		Source:          defaultRegistrySourceID,
		Publisher:       "AnyClaw Labs",
		RiskLevel:       "low",
		TrustLevel:      "verified",
		Permissions:     []string{"fs.read"},
		Tags:            []string{"release"},
		UpdatedAt:       "2026-05-08T10:00:00Z",
		ManifestSummary: map[string]string{"use_case": "release notes"},
	}
	for _, artifact := range []Artifact{alpha, beta} {
		if err := server.store.UpsertArtifact(context.Background(), artifact, nil); err != nil {
			t.Fatalf("upsert artifact %s: %v", artifact.ID, err)
		}
	}
	server.processOneEmbeddingJob(context.Background())
	server.processOneEmbeddingJob(context.Background())

	var search struct {
		Data ListResult   `json:"data"`
		Meta ResponseMeta `json:"meta"`
	}
	doJSON(t, server, http.MethodPost, "/v1/search", strings.NewReader(`{"kind":"skill","q":"semantic author","search_mode":"hybrid"}`), http.StatusOK, &search)
	if search.Meta.SearchMode != SearchModeHybrid {
		t.Fatalf("expected hybrid mode, got %#v", search.Meta)
	}
	if search.Meta.VectorApplied == nil || !*search.Meta.VectorApplied {
		t.Fatalf("expected vector_applied=true, got %#v", search.Meta)
	}
	if search.Data.Total < 1 || len(search.Data.Items) < 1 {
		t.Fatalf("expected hybrid results, got %#v", search.Data)
	}
	if search.Data.Items[0].ID != "cloud.skill.alpha-writer" {
		t.Fatalf("expected semantic vector hit first, got %#v", search.Data.Items)
	}
	if search.Data.Items[0].VectorScore == nil || *search.Data.Items[0].VectorScore <= 0 {
		t.Fatalf("expected vector score on first item, got %#v", search.Data.Items[0])
	}
}

func TestServerPublishQueuesEmbeddingJob(t *testing.T) {
	server, err := NewServer(context.Background(), ServerConfig{
		DataDir: t.TempDir(),
		Vector: VectorConfig{
			Enabled:  true,
			FailOpen: true,
			Model:    "mock-doc",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })

	artifact := Artifact{
		ID:            "cloud.skill.embedding-test",
		Kind:          ArtifactKindSkill,
		Name:          "Embedding Test",
		Summary:       "Queues embedding jobs.",
		LatestVersion: "1.0.0",
		Source:        defaultRegistrySourceID,
		Publisher:     "AnyClaw Labs",
		RiskLevel:     "low",
		TrustLevel:    "verified",
		Permissions:   []string{"fs.read"},
	}
	if err := server.store.UpsertArtifact(context.Background(), artifact, nil); err != nil {
		t.Fatal(err)
	}
	job, err := server.store.claimNextArtifactEmbeddingJob(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if job == nil || job.ArtifactID != artifact.ID {
		t.Fatalf("expected queued embedding job, got %#v", job)
	}
}

func TestServerAdminTokenPublishQuarantineAndStats(t *testing.T) {
	server, err := NewServer(context.Background(), ServerConfig{
		DataDir:    t.TempDir(),
		Seed:       true,
		AdminToken: "admin-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })

	var unauthorized ErrorResponse
	doJSON(t, server, http.MethodGet, "/v1/admin/audit", nil, http.StatusUnauthorized, &unauthorized)

	var token struct {
		Data PublisherToken `json:"data"`
	}
	doJSONWithAuth(t, server, http.MethodPost, "/v1/admin/tokens", strings.NewReader(`{"publisher_id":"AnyClaw Labs"}`), "admin-secret", http.StatusOK, &token)
	if token.Data.Token == "" {
		t.Fatalf("expected one-time publisher token: %#v", token.Data)
	}
	var tokens struct {
		Data PublisherTokenList `json:"data"`
	}
	doJSONWithAuth(t, server, http.MethodGet, "/v1/admin/tokens", nil, "admin-secret", http.StatusOK, &tokens)
	if tokens.Data.Total != 1 || tokens.Data.Items[0].Token != "" || tokens.Data.Items[0].ID != token.Data.ID {
		t.Fatalf("unexpected publisher token list: %#v", tokens.Data)
	}

	publishBody := `{"artifact":{"id":"cloud.skill.test-publish","kind":"skill","name":"Published Skill","summary":"Published from test","latest_version":"1.0.0","risk_level":"low","trust_level":"verified","permissions":["fs.read"],"compatibility":{"os":["windows"]},"tags":["publish"]},"versions":[{"version":"1.0.0","signature":"sig-test"}]}`
	var published struct {
		Data Artifact `json:"data"`
	}
	doJSONWithAuth(t, server, http.MethodPost, "/v1/publish", strings.NewReader(publishBody), token.Data.Token, http.StatusOK, &published)
	if published.Data.ID != "cloud.skill.test-publish" {
		t.Fatalf("unexpected published artifact: %#v", published.Data)
	}
	if published.Data.Publisher != "AnyClaw Labs" {
		t.Fatalf("publisher = %q, want token publisher", published.Data.Publisher)
	}

	var resolved struct {
		Data ResolvedArtifact `json:"data"`
	}
	doJSON(t, server, http.MethodPost, "/v1/artifacts/cloud.skill.test-publish/resolve", strings.NewReader(`{}`), http.StatusOK, &resolved)
	if resolved.Data.Signature != "sig-test" {
		t.Fatalf("expected signature in resolve response: %#v", resolved.Data)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/download/cloud.skill.test-publish/1.0.0", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("download status = %d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("X-Artifact-Signature") != "sig-test" {
		t.Fatalf("expected signature header, got %q", rec.Header().Get("X-Artifact-Signature"))
	}

	var downloads struct {
		Data DownloadStatsResult `json:"data"`
	}
	doJSONWithAuth(t, server, http.MethodGet, "/v1/admin/downloads", nil, "admin-secret", http.StatusOK, &downloads)
	if downloads.Data.Total == 0 {
		t.Fatal("expected download stats")
	}

	var quarantine struct {
		Data QuarantineRecord `json:"data"`
	}
	doJSONWithAuth(t, server, http.MethodPost, "/v1/artifacts/cloud.skill.test-publish/quarantine", strings.NewReader(`{"reason":"bad package"}`), "admin-secret", http.StatusOK, &quarantine)
	if quarantine.Data.Reason != "bad package" {
		t.Fatalf("unexpected quarantine: %#v", quarantine.Data)
	}
	doJSON(t, server, http.MethodPost, "/v1/artifacts/cloud.skill.test-publish/resolve", strings.NewReader(`{}`), http.StatusGone, &ErrorResponse{})
	doJSONWithAuth(t, server, http.MethodPost, "/v1/artifacts/cloud.skill.test-publish/unquarantine", strings.NewReader(`{}`), "admin-secret", http.StatusOK, &struct{}{})
	doJSON(t, server, http.MethodPost, "/v1/artifacts/cloud.skill.test-publish/resolve", strings.NewReader(`{}`), http.StatusOK, &resolved)

	var audit struct {
		Data RegistryAuditList `json:"data"`
	}
	doJSONWithAuth(t, server, http.MethodGet, "/v1/admin/audit", nil, "admin-secret", http.StatusOK, &audit)
	if audit.Data.Total == 0 {
		t.Fatal("expected audit events")
	}
}

func TestServerRequiresAdminToken(t *testing.T) {
	_, err := NewServer(context.Background(), ServerConfig{
		DataDir:           t.TempDir(),
		RequireAdminToken: true,
	})
	if err == nil || !strings.Contains(err.Error(), "admin token is required") {
		t.Fatalf("expected missing admin token error, got %v", err)
	}

	serverWithoutAdmin, err := NewServer(context.Background(), ServerConfig{
		DataDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("expected registry to start without admin token when require flag is false, got %v", err)
	}
	t.Cleanup(func() { _ = serverWithoutAdmin.Close() })

	server, err := NewServer(context.Background(), ServerConfig{
		DataDir:           t.TempDir(),
		AdminToken:        "admin-secret",
		RequireAdminToken: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })

	var unauthorized ErrorResponse
	doJSON(t, server, http.MethodGet, "/v1/admin/audit", nil, http.StatusUnauthorized, &unauthorized)
}

func TestServerPublishRejectsPublisherMismatch(t *testing.T) {
	server, err := NewServer(context.Background(), ServerConfig{
		DataDir:    t.TempDir(),
		AdminToken: "admin-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })

	var token struct {
		Data PublisherToken `json:"data"`
	}
	doJSONWithAuth(t, server, http.MethodPost, "/v1/admin/tokens", strings.NewReader(`{"publisher_id":"publisher-a"}`), "admin-secret", http.StatusOK, &token)

	publishBody := `{"artifact":{"id":"cloud.skill.publisher-mismatch","kind":"skill","name":"Mismatch","summary":"Should fail","publisher":"publisher-b","latest_version":"1.0.0","risk_level":"low","trust_level":"verified"},"versions":[{"version":"1.0.0"}]}`
	var forbidden ErrorResponse
	doJSONWithAuth(t, server, http.MethodPost, "/v1/publish", strings.NewReader(publishBody), token.Data.Token, http.StatusForbidden, &forbidden)
	if forbidden.Error.Code != "publisher_mismatch" {
		t.Fatalf("unexpected error: %#v", forbidden)
	}
}

func TestServerRevokePublisherToken(t *testing.T) {
	server, err := NewServer(context.Background(), ServerConfig{
		DataDir:    t.TempDir(),
		AdminToken: "admin-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })

	var token struct {
		Data PublisherToken `json:"data"`
	}
	doJSONWithAuth(t, server, http.MethodPost, "/v1/admin/tokens", strings.NewReader(`{"publisher_id":"AnyClaw Labs"}`), "admin-secret", http.StatusOK, &token)

	var revoked struct {
		Data PublisherTokenRevocation `json:"data"`
	}
	doJSONWithAuth(t, server, http.MethodPost, "/v1/admin/tokens/"+token.Data.ID+"/revoke", nil, "admin-secret", http.StatusOK, &revoked)
	if revoked.Data.ID != token.Data.ID || revoked.Data.RevokedAt == "" {
		t.Fatalf("unexpected revocation: %#v", revoked.Data)
	}

	publishBody := `{"artifact":{"id":"cloud.skill.revoked-token","kind":"skill","name":"Revoked Token Skill","summary":"Should not publish","latest_version":"1.0.0","risk_level":"low","trust_level":"verified","permissions":["fs.read"],"compatibility":{"os":["windows"]}},"versions":[{"version":"1.0.0"}]}`
	var unauthorized ErrorResponse
	doJSONWithAuth(t, server, http.MethodPost, "/v1/publish", strings.NewReader(publishBody), token.Data.Token, http.StatusUnauthorized, &unauthorized)
}

func TestServerAdminDeleteArtifact(t *testing.T) {
	server, err := NewServer(context.Background(), ServerConfig{
		DataDir:    t.TempDir(),
		Seed:       true,
		AdminToken: "admin-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })

	var unauthorized ErrorResponse
	doJSON(t, server, http.MethodDelete, "/v1/admin/artifacts/cloud.skill.release-notes", nil, http.StatusUnauthorized, &unauthorized)

	var deleted struct {
		Data ArtifactDeletion `json:"data"`
	}
	doJSONWithAuth(t, server, http.MethodDelete, "/v1/admin/artifacts/cloud.skill.release-notes", nil, "admin-secret", http.StatusOK, &deleted)
	if deleted.Data.ArtifactID != "cloud.skill.release-notes" || deleted.Data.DeletedAt == "" {
		t.Fatalf("unexpected deletion response: %#v", deleted.Data)
	}

	var missing ErrorResponse
	doJSON(t, server, http.MethodGet, "/v1/artifacts/cloud.skill.release-notes", nil, http.StatusNotFound, &missing)

	var audit struct {
		Data RegistryAuditList `json:"data"`
	}
	doJSONWithAuth(t, server, http.MethodGet, "/v1/admin/audit", nil, "admin-secret", http.StatusOK, &audit)
	found := false
	for _, event := range audit.Data.Items {
		if event.Event == "artifact.deleted" && event.Artifact == "cloud.skill.release-notes" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected artifact.deleted audit event, got %#v", audit.Data.Items)
	}
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	server, err := NewServer(context.Background(), ServerConfig{
		DataDir:    t.TempDir(),
		Seed:       true,
		AdminToken: "test-admin-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := server.Close(); err != nil {
			t.Fatal(err)
		}
	})
	return server
}

func doJSON(t *testing.T, handler http.Handler, method, path string, body io.Reader, wantStatus int, dst any) {
	t.Helper()
	req := httptest.NewRequest(method, path, body)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("%s %s status = %d, want %d, body = %s", method, path, rec.Code, wantStatus, rec.Body.String())
	}
	if dst != nil {
		if err := json.NewDecoder(rec.Body).Decode(dst); err != nil {
			t.Fatalf("decode response: %v", err)
		}
	}
}

func doJSONWithAuth(t *testing.T, handler http.Handler, method, path string, body io.Reader, token string, wantStatus int, dst any) {
	t.Helper()
	req := httptest.NewRequest(method, path, body)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("%s %s status = %d, want %d, body = %s", method, path, rec.Code, wantStatus, rec.Body.String())
	}
	if dst != nil {
		if err := json.NewDecoder(rec.Body).Decode(dst); err != nil {
			t.Fatalf("decode response: %v", err)
		}
	}
}

func waitForEmbeddingStatus(t *testing.T, server *Server, artifactID string, wantStatus string) *ArtifactEmbedding {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		item, err := server.store.loadArtifactEmbedding(context.Background(), artifactID)
		if err != nil {
			t.Fatal(err)
		}
		if item != nil && item.Status == wantStatus {
			return item
		}
		time.Sleep(25 * time.Millisecond)
	}
	item, err := server.store.loadArtifactEmbedding(context.Background(), artifactID)
	if err != nil {
		t.Fatal(err)
	}
	t.Fatalf("timed out waiting for embedding %s to reach status %s, last=%#v", artifactID, wantStatus, item)
	return nil
}

func assertZipContains(t *testing.T, data []byte, name string) {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	for _, file := range reader.File {
		if file.Name == name {
			return
		}
	}
	t.Fatalf("zip did not contain %s", name)
}

type mockEmbeddingProvider struct {
	name       string
	dim        int
	shouldFail bool
	embeddings map[string][]float32
	callCount  int
}

func (m *mockEmbeddingProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	m.callCount++
	if m.shouldFail {
		return nil, context.Canceled
	}
	if vectorData, ok := m.embeddings[text]; ok {
		return append([]float32(nil), vectorData...), nil
	}
	result := make([]float32, m.dim)
	for i := range result {
		result[i] = float32(len(text) + i + 1)
	}
	return result, nil
}

func (m *mockEmbeddingProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, 0, len(texts))
	for _, text := range texts {
		item, err := m.Embed(ctx, text)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func (m *mockEmbeddingProvider) Name() string {
	return m.name
}

func (m *mockEmbeddingProvider) Dimension() int {
	return m.dim
}

func TestWorkerCreatesArtifactEmbeddingRecord(t *testing.T) {
	provider := &mockEmbeddingProvider{
		name: "mock",
		dim:  3,
	}
	server, err := NewServer(context.Background(), ServerConfig{
		DataDir: t.TempDir(),
		Vector: VectorConfig{
			Enabled: true,
			Model:   "mock-doc",
		},
		TestArtifactEmbed: provider,
		TestQueryEmbed:    provider,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })

	artifact := Artifact{
		ID:            "cloud.skill.worker-test",
		Kind:          ArtifactKindSkill,
		Name:          "Worker Test",
		Summary:       "Creates embeddings asynchronously.",
		LatestVersion: "1.0.0",
		Source:        defaultRegistrySourceID,
		Publisher:     "AnyClaw Labs",
		RiskLevel:     "low",
		TrustLevel:    "verified",
		Permissions:   []string{"fs.read"},
	}
	if err := server.store.UpsertArtifact(context.Background(), artifact, nil); err != nil {
		t.Fatal(err)
	}
	item := waitForEmbeddingStatus(t, server, artifact.ID, artifactEmbeddingStatusReady)
	if item == nil || item.Status != artifactEmbeddingStatusReady {
		t.Fatalf("expected ready embedding record, got %#v", item)
	}
	wantText := buildArtifactEmbeddingText(artifact)
	if item.EmbeddingText != wantText {
		t.Fatalf("unexpected embedding text: got %q want %q", item.EmbeddingText, wantText)
	}
	if len(item.Vector) != provider.dim {
		t.Fatalf("unexpected vector dimension: %#v", item)
	}
	if item.Model != "mock-doc" {
		t.Fatalf("unexpected model: %#v", item)
	}
	if reflect.DeepEqual(item.Vector, []float32{}) {
		t.Fatalf("expected non-empty vector")
	}
}

func TestServerAdminEmbeddingEndpointsAndMetrics(t *testing.T) {
	provider := &mockEmbeddingProvider{name: "mock", dim: 3}
	server, err := NewServer(context.Background(), ServerConfig{
		DataDir:    t.TempDir(),
		AdminToken: "admin-secret",
		Vector: VectorConfig{
			Enabled: true,
			Model:   "mock-doc",
		},
		TestArtifactEmbed: provider,
		TestQueryEmbed:    provider,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })

	artifact := Artifact{
		ID:            "cloud.skill.admin-embed",
		Kind:          ArtifactKindSkill,
		Name:          "Admin Embed",
		Summary:       "Visible from admin endpoints.",
		LatestVersion: "1.0.0",
		Source:        defaultRegistrySourceID,
		Publisher:     "AnyClaw Labs",
		RiskLevel:     "low",
		TrustLevel:    "verified",
		Permissions:   []string{"fs.read"},
	}
	if err := server.store.UpsertArtifact(context.Background(), artifact, nil); err != nil {
		t.Fatal(err)
	}
	waitForEmbeddingStatus(t, server, artifact.ID, artifactEmbeddingStatusReady)

	var jobs struct {
		Data ArtifactEmbeddingJobList `json:"data"`
	}
	doJSONWithAuth(t, server, http.MethodGet, "/v1/admin/embedding-jobs", nil, "admin-secret", http.StatusOK, &jobs)
	if jobs.Data.Total == 0 || jobs.Data.Items[0].ArtifactID != artifact.ID {
		t.Fatalf("unexpected embedding jobs: %#v", jobs.Data)
	}

	var embeddings struct {
		Data ArtifactEmbeddingList `json:"data"`
	}
	doJSONWithAuth(t, server, http.MethodGet, "/v1/admin/embeddings?status=ready", nil, "admin-secret", http.StatusOK, &embeddings)
	if embeddings.Data.Total == 0 || embeddings.Data.Items[0].ArtifactID != artifact.ID {
		t.Fatalf("unexpected embeddings list: %#v", embeddings.Data)
	}

	var detail struct {
		Data ArtifactEmbedding `json:"data"`
	}
	doJSONWithAuth(t, server, http.MethodGet, "/v1/admin/embeddings/"+artifact.ID, nil, "admin-secret", http.StatusOK, &detail)
	if detail.Data.ArtifactID != artifact.ID || detail.Data.Status != artifactEmbeddingStatusReady {
		t.Fatalf("unexpected embedding detail: %#v", detail.Data)
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics.json", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected metrics.json 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var metricsPayload map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&metricsPayload); err != nil {
		t.Fatalf("decode metrics json: %v", err)
	}
	if _, ok := metricsPayload["gauges"]; !ok {
		t.Fatalf("expected gauges in metrics payload, got %#v", metricsPayload)
	}
}

func TestServerHybridQueryCacheAvoidsRepeatedEmbeddingCalls(t *testing.T) {
	artifactProvider := &mockEmbeddingProvider{name: "doc", dim: 3}
	queryProvider := &mockEmbeddingProvider{
		name: "query",
		dim:  3,
		embeddings: map[string][]float32{
			"semantic author": {1, 0, 0},
		},
	}
	server, err := NewServer(context.Background(), ServerConfig{
		DataDir: t.TempDir(),
		Vector: VectorConfig{
			Enabled:        true,
			Model:          "mock-doc",
			QueryModel:     "mock-query",
			QueryCacheTTL:  time.Minute,
			QueryCacheSize: 16,
		},
		TestArtifactEmbed: artifactProvider,
		TestQueryEmbed:    queryProvider,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })

	artifact := Artifact{
		ID:              "cloud.skill.cache-target",
		Kind:            ArtifactKindSkill,
		Name:            "Semantic Author",
		Summary:         "Semantic authoring helper.",
		LatestVersion:   "1.0.0",
		Source:          defaultRegistrySourceID,
		Publisher:       "AnyClaw Labs",
		RiskLevel:       "low",
		TrustLevel:      "verified",
		Permissions:     []string{"fs.read"},
		ManifestSummary: map[string]string{"use_case": "semantic writing"},
	}
	if err := server.store.UpsertArtifact(context.Background(), artifact, nil); err != nil {
		t.Fatal(err)
	}
	waitForEmbeddingStatus(t, server, artifact.ID, artifactEmbeddingStatusReady)

	var result struct {
		Data ListResult `json:"data"`
	}
	doJSON(t, server, http.MethodPost, "/v1/search", strings.NewReader(`{"kind":"skill","q":"semantic author","search_mode":"hybrid"}`), http.StatusOK, &result)
	doJSON(t, server, http.MethodPost, "/v1/search", strings.NewReader(`{"kind":"skill","q":"semantic author","search_mode":"hybrid"}`), http.StatusOK, &result)
	if queryProvider.callCount != 1 {
		t.Fatalf("expected query provider to be called once due to cache, got %d", queryProvider.callCount)
	}
}
