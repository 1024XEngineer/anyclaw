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
	"strings"
	"testing"
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

func TestServerRequireAdminToken(t *testing.T) {
	_, err := NewServer(context.Background(), ServerConfig{
		DataDir:           t.TempDir(),
		RequireAdminToken: true,
	})
	if err == nil || !strings.Contains(err.Error(), "admin token is required") {
		t.Fatalf("expected missing admin token error, got %v", err)
	}

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
		DataDir: t.TempDir(),
		Seed:    true,
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
