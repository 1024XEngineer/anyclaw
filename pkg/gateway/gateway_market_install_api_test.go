package gateway

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/config"
	"github.com/1024XEngineer/anyclaw/pkg/marketplace"
	appRuntime "github.com/1024XEngineer/anyclaw/pkg/runtime"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

func TestMarketInstallCreatesJobAndReceipt(t *testing.T) {
	archive := testGatewayArtifactArchive(t, "cloud.skill.release-notes", marketplace.ArtifactKindSkill, "1.0.0")
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/artifacts/cloud.skill.release-notes/resolve":
			writeRegistryJSON(t, w, map[string]any{"data": map[string]any{
				"artifact_id":     "cloud.skill.release-notes",
				"version":         "1.0.0",
				"download_url":    "http://" + r.Host + "/v1/download/cloud.skill.release-notes/1.0.0",
				"checksum_sha256": marketplaceTestSHA256(archive),
				"size_bytes":      len(archive),
				"compatibility":   map[string]any{"anyclaw_min": "0.1.0"},
				"risk_level":      "low",
				"trust_level":     "verified",
				"permissions":     []string{"fs.read"},
				"kind":            "skill",
				"name":            "Release Notes",
			}})
		case "/v1/download/cloud.skill.release-notes/1.0.0":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer registry.Close()

	server := newMarketInstallTestServer(t, registry.URL)
	server.startWorkers(t.Context())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/market/install", strings.NewReader(`{"artifact_id":"cloud.skill.release-notes","user_confirmed":true}`))
	req.Header.Set("Idempotency-Key", "install-1")
	server.handleMarketInstall(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("install status = %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		JobID string                 `json:"job_id"`
		Job   marketplace.InstallJob `json:"job"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.JobID == "" {
		t.Fatal("expected job_id")
	}

	job := waitMarketJob(t, server, payload.JobID)
	if job.State != marketplace.JobSucceeded {
		t.Fatalf("job state = %s error=%s", job.State, job.Error)
	}
	if job.ReceiptID == "" {
		t.Fatal("expected receipt id")
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/market/install", strings.NewReader(`{"artifact_id":"cloud.skill.release-notes","user_confirmed":true}`))
	req.Header.Set("Idempotency-Key", "install-1")
	server.handleMarketInstall(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("idempotent install status = %d", rec.Code)
	}
	var reused struct {
		JobID  string `json:"job_id"`
		Reused bool   `json:"reused"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &reused); err != nil {
		t.Fatal(err)
	}
	if !reused.Reused || reused.JobID != payload.JobID {
		t.Fatalf("expected reused job %s, got %#v", payload.JobID, reused)
	}
}

func TestMarketInstallPolicyBlockIsAuditedAndEvented(t *testing.T) {
	archive := testGatewayArtifactArchive(t, "cloud.cli.danger", marketplace.ArtifactKindCLI, "1.0.0")
	downloaded := false
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/artifacts/cloud.cli.danger/resolve":
			writeRegistryJSON(t, w, map[string]any{"data": map[string]any{
				"artifact_id":     "cloud.cli.danger",
				"version":         "1.0.0",
				"download_url":    "http://" + r.Host + "/v1/download/cloud.cli.danger/1.0.0",
				"checksum_sha256": marketplaceTestSHA256(archive),
				"size_bytes":      len(archive),
				"risk_level":      "high",
				"trust_level":     "verified",
				"permissions":     []string{"process.exec"},
				"kind":            "cli",
				"name":            "Danger",
			}})
		case "/v1/download/cloud.cli.danger/1.0.0":
			downloaded = true
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer registry.Close()

	server := newMarketInstallTestServer(t, registry.URL)
	server.startWorkers(t.Context())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/market/install", strings.NewReader(`{"artifact_id":"cloud.cli.danger","user_confirmed":true}`))
	server.handleMarketInstall(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("install status = %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	job := waitMarketJob(t, server, payload.JobID)
	if job.State != marketplace.JobFailed || job.Decision == nil || job.Decision.Decision != marketplace.DecisionBlock {
		t.Fatalf("job = %#v, want failed blocked job", job)
	}
	if downloaded {
		t.Fatal("policy block should happen before package download")
	}
	auditData, err := os.ReadFile(server.marketplaceStore().AuditPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(auditData), "market.policy.decision") || !strings.Contains(string(auditData), "block") {
		t.Fatalf("audit missing block decision: %s", string(auditData))
	}

	rec = httptest.NewRecorder()
	server.handleMarketEvents(rec, httptest.NewRequest(http.MethodGet, "/market/events", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("events status = %d body=%s", rec.Code, rec.Body.String())
	}
	var events struct {
		Data marketplace.MarketEventListResult `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &events); err != nil {
		t.Fatal(err)
	}
	if events.Data.Total == 0 {
		t.Fatal("expected marketplace events")
	}
}

func TestMarketInstallPolicyHighRiskPermissionRequiresAcknowledgement(t *testing.T) {
	archive := testGatewayArtifactArchive(t, "cloud.skill.shell-helper", marketplace.ArtifactKindSkill, "1.0.0")
	downloaded := false
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/artifacts/cloud.skill.shell-helper/resolve":
			writeRegistryJSON(t, w, map[string]any{"data": map[string]any{
				"artifact_id":     "cloud.skill.shell-helper",
				"version":         "1.0.0",
				"download_url":    "http://" + r.Host + "/v1/download/cloud.skill.shell-helper/1.0.0",
				"checksum_sha256": marketplaceTestSHA256(archive),
				"size_bytes":      len(archive),
				"risk_level":      "low",
				"trust_level":     "verified",
				"permissions":     []string{"fs.read", "process.exec"},
				"kind":            "skill",
				"name":            "Shell Helper",
			}})
		case "/v1/download/cloud.skill.shell-helper/1.0.0":
			downloaded = true
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer registry.Close()

	server := newMarketInstallTestServer(t, registry.URL)
	server.startWorkers(t.Context())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/market/install", strings.NewReader(`{"artifact_id":"cloud.skill.shell-helper","user_confirmed":true}`))
	server.handleMarketInstall(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("install status = %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	job := waitMarketJob(t, server, payload.JobID)
	if job.State != marketplace.JobFailed || job.Decision == nil || !job.Decision.RequiresRiskAcknowledgement {
		t.Fatalf("job = %#v, want failed job requiring risk acknowledgement", job)
	}
	if downloaded {
		t.Fatal("high-risk acknowledgement should be required before package download")
	}
}

func TestMarketInstallChecksumFailureIsQueryable(t *testing.T) {
	archive := testGatewayArtifactArchive(t, "cloud.cli.repo-health", marketplace.ArtifactKindCLI, "1.0.0")
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/artifacts/cloud.cli.repo-health/resolve":
			writeRegistryJSON(t, w, map[string]any{"data": map[string]any{
				"artifact_id":     "cloud.cli.repo-health",
				"version":         "1.0.0",
				"download_url":    "http://" + r.Host + "/v1/download/cloud.cli.repo-health/1.0.0",
				"checksum_sha256": "bad-checksum",
				"size_bytes":      len(archive),
				"kind":            "cli",
				"name":            "Repo Health",
			}})
		case "/v1/download/cloud.cli.repo-health/1.0.0":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer registry.Close()

	server := newMarketInstallTestServer(t, registry.URL)
	server.startWorkers(t.Context())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/market/install", strings.NewReader(`{"artifact_id":"cloud.cli.repo-health","user_confirmed":true}`))
	server.handleMarketInstall(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("install status = %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	job := waitMarketJob(t, server, payload.JobID)
	if job.State != marketplace.JobRolledBack {
		t.Fatalf("job state = %s error=%s", job.State, job.Error)
	}

	rec = httptest.NewRecorder()
	server.handleMarketJobDetail(rec, httptest.NewRequest(http.MethodGet, "/market/jobs/"+payload.JobID, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("job detail status = %d body=%s", rec.Code, rec.Body.String())
	}
	var detail struct {
		Data marketplace.InstallJob `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Data.State != marketplace.JobRolledBack {
		t.Fatalf("detail state = %s", detail.Data.State)
	}
}

func TestMarketUninstallRemovesBindingsReceiptAndRefreshes(t *testing.T) {
	server := newMarketInstallTestServer(t, "")
	installedPath := t.TempDir()
	receipt := &marketplace.InstallReceipt{
		ID:            "cloud.skill.release-notes@1.0.0",
		ArtifactID:    "cloud.skill.release-notes",
		Kind:          marketplace.ArtifactKindSkill,
		Name:          "Release Notes",
		Version:       "1.0.0",
		Source:        marketplace.SourceCloud,
		InstalledPath: installedPath,
		InstalledBy:   "user",
		InstalledAt:   time.Now().UTC().Format(time.RFC3339),
	}
	if err := server.marketplaceStore().SaveReceipt(receipt); err != nil {
		t.Fatal(err)
	}
	if _, err := server.marketplaceStore().CreateBinding(marketplace.BindingRequest{
		ArtifactID: receipt.ArtifactID,
		ReceiptID:  receipt.ID,
		TargetType: marketplace.TargetRuntimeGlobal,
	}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/market/uninstall", strings.NewReader(`{"artifact_id":"cloud.skill.release-notes"}`))
	server.handleMarketUninstall(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("uninstall status = %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Data marketplace.UninstallResult `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.ReceiptID != receipt.ID || len(payload.Data.RemovedBindings) != 1 {
		t.Fatalf("unexpected uninstall payload: %#v", payload.Data)
	}
	if _, err := server.marketplaceStore().GetReceipt(receipt.ID); err != marketplace.ErrArtifactNotFound {
		t.Fatalf("receipt err = %v, want not found", err)
	}
	bindings, err := server.marketplaceStore().ListBindings()
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings.Items) != 0 {
		t.Fatalf("bindings = %#v, want empty", bindings.Items)
	}
}

func TestMarketUpgradeRefreshesExistingBindings(t *testing.T) {
	newArchive := testGatewayArtifactArchive(t, "cloud.skill.release-notes", marketplace.ArtifactKindSkill, "2.0.0")
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/artifacts/cloud.skill.release-notes/resolve":
			writeRegistryJSON(t, w, map[string]any{"data": map[string]any{
				"artifact_id":     "cloud.skill.release-notes",
				"version":         "2.0.0",
				"download_url":    "http://" + r.Host + "/v1/download/cloud.skill.release-notes/2.0.0",
				"checksum_sha256": marketplaceTestSHA256(newArchive),
				"size_bytes":      len(newArchive),
				"risk_level":      "low",
				"trust_level":     "verified",
				"permissions":     []string{"fs.read"},
				"kind":            "skill",
				"name":            "Release Notes",
			}})
		case "/v1/download/cloud.skill.release-notes/2.0.0":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(newArchive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer registry.Close()

	server := newMarketInstallTestServer(t, registry.URL)
	server.startWorkers(t.Context())
	defaultOrg, defaultProject, defaultWorkspace := defaultResourceIDs(server.mainRuntime.WorkingDir)
	if err := server.ensureDefaultWorkspace(); err != nil {
		t.Fatal(err)
	}
	oldPath := server.marketplaceStore().InstalledDir() + "/skill/cloud-skill-release-notes/1-0-0"
	if err := os.MkdirAll(oldPath, 0o755); err != nil {
		t.Fatal(err)
	}
	receipt := &marketplace.InstallReceipt{
		ID:            "cloud.skill.release-notes@1.0.0",
		ArtifactID:    "cloud.skill.release-notes",
		Kind:          marketplace.ArtifactKindSkill,
		Name:          "Release Notes",
		Version:       "1.0.0",
		Source:        marketplace.SourceCloud,
		InstalledPath: oldPath,
		InstalledBy:   "user",
		InstalledAt:   time.Now().UTC().Format(time.RFC3339),
	}
	if err := server.marketplaceStore().SaveReceipt(receipt); err != nil {
		t.Fatal(err)
	}
	if _, err := server.marketplaceStore().CreateBinding(marketplace.BindingRequest{
		ArtifactID: receipt.ArtifactID,
		ReceiptID:  receipt.ID,
		TargetType: marketplace.TargetWorkspace,
		TargetID:   defaultWorkspace,
	}); err != nil {
		t.Fatal(err)
	}
	server.runtimePool.Remember("Main Agent", defaultOrg, defaultProject, defaultWorkspace, &appRuntime.MainRuntime{Config: &config.Config{Agent: config.AgentConfig{Name: "Main Agent"}}})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/market/upgrade", strings.NewReader(`{"artifact_id":"cloud.skill.release-notes","user_confirmed":true}`))
	server.handleMarketUpgrade(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("upgrade status = %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	job := waitMarketJob(t, server, payload.JobID)
	if job.State != marketplace.JobSucceeded {
		t.Fatalf("job state = %s error=%s", job.State, job.Error)
	}
	for _, item := range server.runtimePool.List() {
		if item.Workspace == defaultWorkspace {
			t.Fatalf("bound workspace runtime should have been refreshed: %+v", item)
		}
	}
	if metrics := server.runtimePool.Metrics(); metrics.Refreshes != 1 {
		t.Fatalf("expected upgrade to refresh one binding scope, got %+v", metrics)
	}
}

func TestRound15MarketInstallIntegratesSkillAgentAndCLI(t *testing.T) {
	server := newMarketInstallTestServer(t, "")
	skillReceipt := &marketplace.InstallReceipt{
		ID:            "anyclaw.skill.skill-author@1.0.0",
		ArtifactID:    "anyclaw.skill.skill-author",
		Kind:          marketplace.ArtifactKindSkill,
		Name:          "Skill Author",
		Version:       "1.0.0",
		Source:        marketplace.SourceCloud,
		SourceID:      "registry",
		InstalledPath: t.TempDir(),
		InstalledAt:   time.Now().UTC().Format(time.RFC3339),
		Permissions:   []string{"fs.read"},
	}
	writeJSONFile(t, filepath.Join(skillReceipt.InstalledPath, "anyclaw.artifact.json"), map[string]any{
		"id": "anyclaw.skill.skill-author", "kind": "skill", "name": "Skill Author", "summary": "Helps authors build AnyClaw skills.", "version": "1.0.0",
	})
	if err := server.marketplaceStore().SaveReceipt(skillReceipt); err != nil {
		t.Fatal(err)
	}
	if _, err := server.integrateMarketReceipt(skillReceipt); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	server.handleSkills(rec, newAdminRequest(http.MethodGet, "/skills", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("skills status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Skill Author") {
		t.Fatalf("skill catalog missing installed skill: %s", rec.Body.String())
	}
	profile, ok := server.mainRuntime.Config.ResolveMainAgentProfile()
	if !ok || !profileHasSkill(profile, "Skill Author") {
		t.Fatalf("main profile missing installed skill: %#v", profile)
	}

	agentReceipt := &marketplace.InstallReceipt{
		ID:            "anyclaw.agent.marketplace-operator@1.0.0",
		ArtifactID:    "anyclaw.agent.marketplace-operator",
		Kind:          marketplace.ArtifactKindAgent,
		Name:          "Marketplace Operator",
		Version:       "1.0.0",
		Source:        marketplace.SourceCloud,
		SourceID:      "registry",
		InstalledPath: t.TempDir(),
		InstalledAt:   time.Now().UTC().Format(time.RFC3339),
		Permissions:   []string{"fs.read"},
	}
	writeJSONFile(t, filepath.Join(agentReceipt.InstalledPath, "anyclaw.artifact.json"), map[string]any{
		"id": "anyclaw.agent.marketplace-operator", "kind": "agent", "name": "Marketplace Operator", "summary": "Runs marketplace releases.", "version": "1.0.0", "tags": []string{"marketplace"},
	})
	if err := server.marketplaceStore().SaveReceipt(agentReceipt); err != nil {
		t.Fatal(err)
	}
	if _, err := server.integrateMarketReceipt(agentReceipt); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	server.handleAgents(rec, newAdminRequest(http.MethodGet, "/agents", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("agents status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Marketplace Operator") {
		t.Fatalf("agent catalog missing installed agent: %s", rec.Body.String())
	}

	cliReceipt := &marketplace.InstallReceipt{
		ID:            "anyclaw.cli.agent-native-runner@1.0.0",
		ArtifactID:    "anyclaw.cli.agent-native-runner",
		Kind:          marketplace.ArtifactKindCLI,
		Name:          "Agent Native Runner",
		Version:       "1.0.0",
		Source:        marketplace.SourceCloud,
		SourceID:      "registry",
		InstalledPath: t.TempDir(),
		InstalledAt:   time.Now().UTC().Format(time.RFC3339),
		Permissions:   []string{"process.exec"},
	}
	writeJSONFile(t, filepath.Join(cliReceipt.InstalledPath, "anyclaw.artifact.json"), map[string]any{
		"id": "anyclaw.cli.agent-native-runner", "kind": "cli", "name": "Agent Native Runner", "summary": "Runs native agent commands.", "version": "1.0.0", "manifest_summary": map[string]string{"command": "agent-native-runner"},
	})
	if err := server.marketplaceStore().SaveReceipt(cliReceipt); err != nil {
		t.Fatal(err)
	}
	if _, err := server.integrateMarketReceipt(cliReceipt); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	server.handleMarketArtifacts(rec, httptest.NewRequest(http.MethodGet, "/market/artifacts?source=local&kind=cli&limit=100", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("local cli market status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "agent-native-runner") {
		t.Fatalf("CLI catalog missing installed command: %s", rec.Body.String())
	}
}

func TestRound15MarketUninstallCleansIntegratedResources(t *testing.T) {
	server := newMarketInstallTestServer(t, "")
	receipt := &marketplace.InstallReceipt{
		ID:            "anyclaw.skill.skill-author@1.0.0",
		ArtifactID:    "anyclaw.skill.skill-author",
		Kind:          marketplace.ArtifactKindSkill,
		Name:          "Skill Author",
		Version:       "1.0.0",
		Source:        marketplace.SourceCloud,
		SourceID:      "registry",
		InstalledPath: t.TempDir(),
		InstalledAt:   time.Now().UTC().Format(time.RFC3339),
	}
	writeJSONFile(t, filepath.Join(receipt.InstalledPath, "anyclaw.artifact.json"), map[string]any{
		"id": "anyclaw.skill.skill-author", "kind": "skill", "name": "Skill Author", "summary": "Helps authors build AnyClaw skills.", "version": "1.0.0",
	})
	if err := server.marketplaceStore().SaveReceipt(receipt); err != nil {
		t.Fatal(err)
	}
	record, err := server.integrateMarketReceipt(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(record.SkillDir); err != nil {
		t.Fatalf("expected integrated skill dir: %v", err)
	}
	if _, err := server.marketplaceStore().CreateBinding(marketplace.BindingRequest{ArtifactID: receipt.ArtifactID, ReceiptID: receipt.ID, TargetType: marketplace.TargetRuntimeGlobal}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	server.handleMarketUninstall(rec, httptest.NewRequest(http.MethodPost, "/market/uninstall", strings.NewReader(`{"artifact_id":"anyclaw.skill.skill-author"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("uninstall status = %d body=%s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(record.SkillDir); !os.IsNotExist(err) {
		t.Fatalf("expected integrated skill dir removed, stat err=%v", err)
	}
	if profile, ok := server.mainRuntime.Config.ResolveMainAgentProfile(); ok && profileHasSkill(profile, "Skill Author") {
		t.Fatalf("main profile still has removed skill: %#v", profile.Skills)
	}
	if _, err := os.Stat(server.marketIntegrationPath(receipt.ID)); !os.IsNotExist(err) {
		t.Fatalf("integration receipt should be removed, stat err=%v", err)
	}
}

func testGatewayArtifactArchive(t *testing.T, id string, kind marketplace.ArtifactKind, version string) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	manifest := map[string]any{
		"id":      id,
		"kind":    kind,
		"name":    id,
		"version": version,
	}
	w, err := writer.Create("anyclaw.artifact.json")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(manifest)
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	w, err = writer.Create("README.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("gateway fixture")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func marketplaceTestSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func newMarketInstallTestServer(t *testing.T, endpoint string) *Server {
	t.Helper()
	workDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Agent.Name = "Main Agent"
	cfg.Agent.ActiveProfile = "Main Agent"
	cfg.Agent.Profiles = []config.AgentProfile{{Name: "Main Agent", PermissionLevel: "limited"}}
	cfg.Agent.WorkDir = workDir
	cfg.Agent.WorkingDir = workDir
	cfg.Skills.Dir = filepath.Join(workDir, "skills")
	cfg.Marketplace.RegistryEndpoint = endpoint
	cfg.Marketplace.RequestTimeoutSeconds = 2
	cfg.Marketplace.CacheTTLSeconds = 0
	store, err := state.NewStore(workDir)
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		mainRuntime: &appRuntime.MainRuntime{
			Config:     cfg,
			ConfigPath: filepath.Join(workDir, "anyclaw.json"),
			WorkDir:    workDir,
			WorkingDir: workDir,
		},
		store:       store,
		runtimePool: appRuntime.NewRuntimePool("anyclaw.json", store, 4, time.Minute),
		jobQueue:    make(chan func(), 16),
		marketJobs:  marketplace.NewStore(workDir),
	}
	server.sessions = state.NewSessionManager(store, nil)
	server.hotReload = appRuntime.NewHotReloadCoordinator(server.runtimePool, store)
	return server
}

func waitMarketJob(t *testing.T, server *Server, jobID string) *marketplace.InstallJob {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		job, err := server.marketplaceStore().GetJob(jobID)
		if err == nil && isTerminalMarketJob(job.State) {
			return job
		}
		time.Sleep(20 * time.Millisecond)
	}
	job, err := server.marketplaceStore().GetJob(jobID)
	if err != nil {
		t.Fatal(err)
	}
	t.Fatalf("job did not finish: %#v", job)
	return nil
}

func writeJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func profileHasSkill(profile config.AgentProfile, name string) bool {
	for _, skill := range profile.Skills {
		if strings.EqualFold(strings.TrimSpace(skill.Name), strings.TrimSpace(name)) && skill.Enabled {
			return true
		}
	}
	return false
}
