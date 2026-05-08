package marketplace

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInstallUseCaseSuccessWritesReceipt(t *testing.T) {
	archive := testArtifactArchive(t, "cloud.skill.release-notes", ArtifactKindSkill, "1.0.0")
	registry := &fakeInstallRegistry{
		resolved: ResolvedPackage{
			ArtifactID:     "cloud.skill.release-notes",
			Version:        "1.0.0",
			DownloadURL:    "memory://release-notes",
			ChecksumSHA256: sha256Hex(archive),
			Kind:           ArtifactKindSkill,
			Name:           "Release Notes",
			Permissions:    []string{"fs.read"},
			RiskLevel:      "low",
			TrustLevel:     "verified",
		},
		archive: archive,
	}
	store := NewStore(t.TempDir())
	uc := NewInstallUseCase(store, registry)
	job, reused, err := uc.Start(context.Background(), InstallRequest{ArtifactID: "cloud.skill.release-notes", InstalledBy: "user", UserConfirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	if reused {
		t.Fatal("first install should not reuse a job")
	}
	if err := uc.Execute(context.Background(), job.ID); err != nil {
		t.Fatal(err)
	}

	done, err := store.GetJob(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if done.State != JobSucceeded {
		t.Fatalf("job state = %s, want succeeded; error=%s", done.State, done.Error)
	}
	if done.ReceiptID == "" || done.InstalledPath == "" {
		t.Fatalf("job missing receipt/path: %#v", done)
	}
	if _, err := os.Stat(filepath.Join(done.InstalledPath, "anyclaw.artifact.json")); err != nil {
		t.Fatalf("expected installed manifest: %v", err)
	}

	data, err := os.ReadFile(store.ReceiptPath(done.ReceiptID))
	if err != nil {
		t.Fatal(err)
	}
	var receipt InstallReceipt
	if err := json.Unmarshal(data, &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.ArtifactID != "cloud.skill.release-notes" || receipt.Version != "1.0.0" {
		t.Fatalf("unexpected receipt: %#v", receipt)
	}
	if receipt.Decision == nil || receipt.Decision.Decision != DecisionAsk {
		t.Fatalf("expected ask decision in receipt, got %#v", receipt.Decision)
	}
	auditData, err := os.ReadFile(store.AuditPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(auditData), "market.policy.decision") || !strings.Contains(string(auditData), "market.install.succeeded") {
		t.Fatalf("audit missing policy/success events: %s", string(auditData))
	}
}

func TestInstallUseCaseChecksumMismatchRollsBack(t *testing.T) {
	archive := testArtifactArchive(t, "cloud.cli.repo-health", ArtifactKindCLI, "1.0.0")
	registry := &fakeInstallRegistry{
		resolved: ResolvedPackage{
			ArtifactID:     "cloud.cli.repo-health",
			Version:        "1.0.0",
			DownloadURL:    "memory://repo-health",
			ChecksumSHA256: "not-the-real-checksum",
			Kind:           ArtifactKindCLI,
			Name:           "Repo Health",
		},
		archive: archive,
	}
	store := NewStore(t.TempDir())
	uc := NewInstallUseCase(store, registry)
	job, _, err := uc.Start(context.Background(), InstallRequest{ArtifactID: "cloud.cli.repo-health", UserConfirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := uc.Execute(context.Background(), job.ID); err == nil {
		t.Fatal("expected checksum mismatch error")
	}

	done, err := store.GetJob(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if done.State != JobRolledBack || !done.RolledBack {
		t.Fatalf("job state = %s rolled_back=%v, want rolled_back", done.State, done.RolledBack)
	}
	if _, err := os.Stat(filepath.Join(store.InstalledDir(), "cli", "cloud-cli-repo-health", "1-0-0")); !os.IsNotExist(err) {
		t.Fatalf("expected no installed dir after rollback, stat err=%v", err)
	}
}

func TestInstallUseCaseMissingChecksumBlocksBeforeDownload(t *testing.T) {
	registry := &fakeInstallRegistry{
		resolved: ResolvedPackage{
			ArtifactID:  "cloud.cli.repo-health",
			Version:     "1.0.0",
			DownloadURL: "memory://repo-health",
			Kind:        ArtifactKindCLI,
			Name:        "Repo Health",
			RiskLevel:   "medium",
			TrustLevel:  "verified",
		},
		archive: testArtifactArchive(t, "cloud.cli.repo-health", ArtifactKindCLI, "1.0.0"),
	}
	store := NewStore(t.TempDir())
	uc := NewInstallUseCase(store, registry)
	job, _, err := uc.Start(context.Background(), InstallRequest{ArtifactID: "cloud.cli.repo-health", UserConfirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := uc.Execute(context.Background(), job.ID); err == nil {
		t.Fatal("expected missing checksum block")
	}
	if registry.downloads != 0 {
		t.Fatalf("download count = %d, want 0", registry.downloads)
	}
	done, err := store.GetJob(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if done.State != JobFailed || done.Decision == nil || done.Decision.Reason != "missing checksum" {
		t.Fatalf("job = %#v, want missing checksum block", done)
	}
}

func TestInstallUseCasePolicyBlocksBeforeDownload(t *testing.T) {
	registry := &fakeInstallRegistry{
		resolved: ResolvedPackage{
			ArtifactID:  "cloud.cli.danger",
			Version:     "1.0.0",
			DownloadURL: "memory://danger",
			Kind:        ArtifactKindCLI,
			Name:        "Danger",
			RiskLevel:   "high",
			TrustLevel:  "verified",
			Permissions: []string{"process.exec"},
		},
		archive: testArtifactArchive(t, "cloud.cli.danger", ArtifactKindCLI, "1.0.0"),
	}
	store := NewStore(t.TempDir())
	uc := NewInstallUseCase(store, registry)
	job, _, err := uc.Start(context.Background(), InstallRequest{ArtifactID: "cloud.cli.danger", UserConfirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := uc.Execute(context.Background(), job.ID); err == nil {
		t.Fatal("expected policy block")
	}
	if registry.downloads != 0 {
		t.Fatalf("download count = %d, want 0", registry.downloads)
	}
	done, err := store.GetJob(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if done.State != JobFailed || done.Decision == nil || done.Decision.Decision != DecisionBlock {
		t.Fatalf("job = %#v, want failed blocked job", done)
	}
}

func TestInstallUseCasePolicyHighRiskPermissionRequiresAcknowledgementBeforeDownload(t *testing.T) {
	registry := &fakeInstallRegistry{
		resolved: ResolvedPackage{
			ArtifactID:     "cloud.skill.shell-helper",
			Version:        "1.0.0",
			DownloadURL:    "memory://shell-helper",
			ChecksumSHA256: "unused-before-download",
			Kind:           ArtifactKindSkill,
			Name:           "Shell Helper",
			RiskLevel:      "low",
			TrustLevel:     "verified",
			Permissions:    []string{"fs.read", "process.exec"},
		},
		archive: testArtifactArchive(t, "cloud.skill.shell-helper", ArtifactKindSkill, "1.0.0"),
	}
	store := NewStore(t.TempDir())
	uc := NewInstallUseCaseWithPolicy(store, registry, PolicyConfig{AutoInstallSkill: true})
	job, _, err := uc.Start(context.Background(), InstallRequest{ArtifactID: "cloud.skill.shell-helper", UserConfirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := uc.Execute(context.Background(), job.ID); err == nil {
		t.Fatal("expected high-risk acknowledgement requirement")
	}
	if registry.downloads != 0 {
		t.Fatalf("download count = %d, want 0", registry.downloads)
	}
	done, err := store.GetJob(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if done.State != JobFailed || done.Decision == nil || !done.Decision.RequiresRiskAcknowledgement {
		t.Fatalf("job = %#v, want failed job requiring risk acknowledgement", done)
	}
	if len(done.Decision.HighRiskPermissions) != 1 || done.Decision.HighRiskPermissions[0] != "process.exec" {
		t.Fatalf("high risk permissions = %#v", done.Decision.HighRiskPermissions)
	}
}

func TestInstallUseCaseIdempotencyReusesJob(t *testing.T) {
	store := NewStore(t.TempDir())
	uc := NewInstallUseCase(store, &fakeInstallRegistry{})
	first, reused, err := uc.Start(context.Background(), InstallRequest{
		ArtifactID:     "cloud.agent.code-reviewer",
		IdempotencyKey: "idem-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if reused {
		t.Fatal("first request should not be reused")
	}
	second, reused, err := uc.Start(context.Background(), InstallRequest{
		ArtifactID:     "cloud.agent.code-reviewer",
		IdempotencyKey: "idem-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reused {
		t.Fatal("second request should reuse existing job")
	}
	if second.ID != first.ID {
		t.Fatalf("expected same job id, got %s and %s", first.ID, second.ID)
	}
}

func TestUpgradeUseCaseKeepsBindingsAndMovesThemToNewReceipt(t *testing.T) {
	oldArchive := testArtifactArchive(t, "cloud.skill.release-notes", ArtifactKindSkill, "1.0.0")
	newArchive := testArtifactArchive(t, "cloud.skill.release-notes", ArtifactKindSkill, "2.0.0")
	store := NewStore(t.TempDir())
	oldPath := filepath.Join(store.InstalledDir(), "skill", "cloud-skill-release-notes", "1-0-0")
	if err := os.MkdirAll(oldPath, 0o755); err != nil {
		t.Fatal(err)
	}
	oldReceipt := &InstallReceipt{
		ID:             "cloud.skill.release-notes@1.0.0",
		ArtifactID:     "cloud.skill.release-notes",
		Kind:           ArtifactKindSkill,
		Name:           "Release Notes",
		Version:        "1.0.0",
		Source:         SourceCloud,
		InstalledPath:  oldPath,
		InstalledBy:    "user",
		InstalledAt:    time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
		ChecksumSHA256: sha256Hex(oldArchive),
	}
	if err := store.SaveReceipt(oldReceipt); err != nil {
		t.Fatal(err)
	}
	binding, err := store.CreateBinding(BindingRequest{ArtifactID: oldReceipt.ArtifactID, ReceiptID: oldReceipt.ID, TargetType: TargetRuntimeGlobal})
	if err != nil {
		t.Fatal(err)
	}
	registry := &fakeInstallRegistry{
		resolved: ResolvedPackage{
			ArtifactID:     oldReceipt.ArtifactID,
			Version:        "2.0.0",
			DownloadURL:    "memory://release-notes-2",
			ChecksumSHA256: sha256Hex(newArchive),
			Kind:           ArtifactKindSkill,
			Name:           "Release Notes",
			RiskLevel:      "low",
			TrustLevel:     "verified",
			Permissions:    []string{"fs.read"},
		},
		archive: newArchive,
	}
	job, reused, err := store.CreateUpgradeJob(UpgradeRequest{ArtifactID: oldReceipt.ArtifactID, VersionConstraint: "2.0.0", UserConfirmed: true}, "")
	if err != nil {
		t.Fatal(err)
	}
	if reused {
		t.Fatal("upgrade should not reuse job")
	}
	uc := NewInstallUseCase(store, registry)
	if err := uc.Execute(context.Background(), job.ID); err != nil {
		t.Fatal(err)
	}
	done, err := store.GetJob(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if done.State != JobSucceeded || done.Type != "upgrade" {
		t.Fatalf("job = %#v, want succeeded upgrade", done)
	}
	bindings, err := store.ListBindings()
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings.Items) != 1 || bindings.Items[0].ID != binding.ID || bindings.Items[0].Version != "2.0.0" || bindings.Items[0].ReceiptID != "cloud.skill.release-notes@2.0.0" {
		t.Fatalf("bindings = %#v, want upgraded binding", bindings.Items)
	}
}

func TestUpgradeChecksumFailureKeepsPreviousReceiptAndBinding(t *testing.T) {
	oldArchive := testArtifactArchive(t, "cloud.cli.repo-health", ArtifactKindCLI, "1.0.0")
	newArchive := testArtifactArchive(t, "cloud.cli.repo-health", ArtifactKindCLI, "2.0.0")
	store := NewStore(t.TempDir())
	oldReceipt := &InstallReceipt{
		ID:             "cloud.cli.repo-health@1.0.0",
		ArtifactID:     "cloud.cli.repo-health",
		Kind:           ArtifactKindCLI,
		Name:           "Repo Health",
		Version:        "1.0.0",
		Source:         SourceCloud,
		InstalledPath:  filepath.Join(store.InstalledDir(), "cli", "cloud-cli-repo-health", "1-0-0"),
		InstalledBy:    "user",
		InstalledAt:    time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
		ChecksumSHA256: sha256Hex(oldArchive),
	}
	if err := os.MkdirAll(oldReceipt.InstalledPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveReceipt(oldReceipt); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateBinding(BindingRequest{ArtifactID: oldReceipt.ArtifactID, ReceiptID: oldReceipt.ID, TargetType: TargetRuntimeGlobal}); err != nil {
		t.Fatal(err)
	}
	registry := &fakeInstallRegistry{
		resolved: ResolvedPackage{
			ArtifactID:     oldReceipt.ArtifactID,
			Version:        "2.0.0",
			DownloadURL:    "memory://repo-health-2",
			ChecksumSHA256: "bad-checksum",
			Kind:           ArtifactKindCLI,
			Name:           "Repo Health",
			RiskLevel:      "medium",
			TrustLevel:     "verified",
			Permissions:    []string{"process.exec"},
		},
		archive: newArchive,
	}
	job, _, err := store.CreateUpgradeJob(UpgradeRequest{ArtifactID: oldReceipt.ArtifactID, VersionConstraint: "2.0.0", UserConfirmed: true}, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := NewInstallUseCase(store, registry).Execute(context.Background(), job.ID); err == nil {
		t.Fatal("expected checksum failure")
	}
	if _, err := store.GetReceipt(oldReceipt.ID); err != nil {
		t.Fatalf("old receipt should remain: %v", err)
	}
	bindings, err := store.ListBindings()
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings.Items) != 1 || bindings.Items[0].Version != "1.0.0" || bindings.Items[0].ReceiptID != oldReceipt.ID {
		t.Fatalf("bindings = %#v, want previous binding", bindings.Items)
	}
}

type fakeInstallRegistry struct {
	resolved  ResolvedPackage
	archive   []byte
	downloads int
}

func (f *fakeInstallRegistry) Resolve(context.Context, string, string) (ResolvedPackage, error) {
	return f.resolved, nil
}

func (f *fakeInstallRegistry) Download(context.Context, string) ([]byte, error) {
	f.downloads++
	return append([]byte(nil), f.archive...), nil
}

func testArtifactArchive(t *testing.T, id string, kind ArtifactKind, version string) []byte {
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
	if _, err := w.Write([]byte("fixture")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
