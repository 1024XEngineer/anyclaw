package marketplace

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type RegistryResolver interface {
	Resolve(ctx context.Context, artifactID, versionConstraint string) (ResolvedPackage, error)
	Download(ctx context.Context, rawURL string) ([]byte, error)
}

type ResolvedPackage struct {
	ArtifactID     string               `json:"artifact_id"`
	Version        string               `json:"version"`
	DownloadURL    string               `json:"download_url"`
	ChecksumSHA256 string               `json:"checksum_sha256"`
	SizeBytes      int64                `json:"size_bytes"`
	Compatibility  Compatibility        `json:"compatibility"`
	Dependencies   []ArtifactDependency `json:"dependencies,omitempty"`
	RiskLevel      string               `json:"risk_level"`
	TrustLevel     string               `json:"trust_level"`
	Permissions    []string             `json:"permissions"`
	Signature      string               `json:"signature,omitempty"`
	Kind           ArtifactKind         `json:"kind"`
	Name           string               `json:"name"`
}

type InstallUseCase struct {
	store    *Store
	registry RegistryResolver
	policy   DecisionPolicy
}

func NewInstallUseCase(store *Store, registry RegistryResolver) *InstallUseCase {
	return NewInstallUseCaseWithPolicy(store, registry, PolicyConfig{})
}

func NewInstallUseCaseWithPolicy(store *Store, registry RegistryResolver, cfg PolicyConfig) *InstallUseCase {
	return &InstallUseCase{store: store, registry: registry, policy: NewDecisionPolicy(cfg)}
}

func (uc *InstallUseCase) Start(ctx context.Context, req InstallRequest) (*InstallJob, bool, error) {
	if uc == nil || uc.store == nil {
		return nil, false, fmt.Errorf("marketplace install store is not configured")
	}
	if strings.TrimSpace(req.ArtifactID) == "" {
		return nil, false, fmt.Errorf("artifact_id is required")
	}
	job, reused, err := uc.store.CreateInstallJob(req, req.IdempotencyKey)
	if err == nil && !reused {
		uc.audit("market.install.started", job, nil, nil)
		uc.event("market.install.started", "info", "Marketplace install started", job, nil)
	}
	return job, reused, err
}

func (uc *InstallUseCase) Execute(ctx context.Context, jobID string) error {
	if uc == nil || uc.store == nil || uc.registry == nil {
		return fmt.Errorf("marketplace installer is not configured")
	}
	job, err := uc.store.GetJob(jobID)
	if err != nil {
		return err
	}
	if job.State == JobSucceeded || job.State == JobFailed || job.State == JobRolledBack {
		return nil
	}
	if err := uc.mark(job, JobRunning, "resolve", 1, ""); err != nil {
		return err
	}
	resolved, err := uc.registry.Resolve(ctx, job.ArtifactID, job.VersionConstraint)
	if err != nil {
		return uc.fail(job, err, false)
	}
	userConfirmed := strings.EqualFold(job.Metadata["user_confirmed"], "true")
	riskAcknowledged := strings.EqualFold(job.Metadata["risk_acknowledged"], "true")
	job.Version = resolved.Version
	job.ChecksumSHA256 = resolved.ChecksumSHA256
	metadata := cloneStringMap(job.Metadata)
	if metadata == nil {
		metadata = map[string]string{}
	}
	metadata["download_url"] = resolved.DownloadURL
	job.Metadata = metadata
	decision := uc.policy.DecideInstall(InstallRequest{
		ArtifactID:        job.ArtifactID,
		VersionConstraint: job.VersionConstraint,
		InstalledBy:       job.InstalledBy,
		UserConfirmed:     userConfirmed,
		RiskAcknowledged:  riskAcknowledged,
	}, resolved)
	job.Decision = &decision
	_ = uc.store.UpdateJob(job)
	uc.audit("market.policy.decision", job, &decision, map[string]any{
		"kind":        resolved.Kind,
		"version":     resolved.Version,
		"risk_level":  resolved.RiskLevel,
		"trust_level": resolved.TrustLevel,
		"permissions": resolved.Permissions,
	})
	uc.event("market.policy.decision", eventLevelForDecision(decision), decision.Reason, job, &decision)
	if decision.Decision == DecisionBlock {
		return uc.fail(job, fmt.Errorf("marketplace policy blocked install: %s", decision.Reason), false)
	}
	if decision.Decision == DecisionAsk && decision.RequiresUserConfirmation {
		return uc.fail(job, fmt.Errorf("marketplace policy requires user confirmation: %s", decision.Reason), false)
	}
	if decision.Decision == DecisionAsk && decision.RequiresRiskAcknowledgement {
		return uc.fail(job, fmt.Errorf("marketplace policy requires high-risk permission acknowledgement: %s", decision.Reason), false)
	}
	if strings.TrimSpace(resolved.ChecksumSHA256) == "" {
		decision := PolicyDecision{Decision: DecisionBlock, Reason: "missing checksum", Reasons: []string{"missing checksum"}}
		job.Decision = &decision
		_ = uc.store.UpdateJob(job)
		uc.audit("market.policy.decision", job, &decision, map[string]any{"artifact_id": resolved.ArtifactID, "version": resolved.Version})
		uc.event("market.policy.decision", "error", decision.Reason, job, &decision)
		return uc.fail(job, fmt.Errorf("marketplace policy blocked install: %s", decision.Reason), false)
	}

	if err := uc.mark(job, JobRunning, "download", 2, ""); err != nil {
		return err
	}
	archiveBytes, err := uc.registry.Download(ctx, resolved.DownloadURL)
	if err != nil {
		return uc.fail(job, err, false)
	}

	if err := uc.mark(job, JobRunning, "verify", 3, ""); err != nil {
		return err
	}
	actualChecksum := sha256Hex(archiveBytes)
	if !strings.EqualFold(actualChecksum, resolved.ChecksumSHA256) {
		decision := PolicyDecision{Decision: DecisionBlock, Reason: "checksum mismatch", Reasons: []string{"checksum mismatch"}}
		job.Decision = &decision
		_ = uc.store.UpdateJob(job)
		uc.audit("market.policy.decision", job, &decision, map[string]any{"checksum_expected": resolved.ChecksumSHA256, "checksum_actual": actualChecksum})
		uc.event("market.policy.decision", "error", decision.Reason, job, &decision)
		return uc.fail(job, fmt.Errorf("checksum mismatch: expected %s, got %s", resolved.ChecksumSHA256, actualChecksum), true)
	}

	if err := uc.mark(job, JobRunning, "install", 4, ""); err != nil {
		return err
	}
	manifest, installedPath, err := uc.installArchive(job, resolved, archiveBytes)
	if err != nil {
		return uc.fail(job, err, true)
	}
	job.InstalledPath = installedPath
	_ = uc.store.UpdateJob(job)

	if err := uc.mark(job, JobRunning, "receipt", 5, ""); err != nil {
		return err
	}
	receipt := &InstallReceipt{
		ID:             receiptID(resolved.ArtifactID, resolved.Version),
		JobID:          job.ID,
		ArtifactID:     resolved.ArtifactID,
		Kind:           resolved.Kind,
		Name:           resolved.Name,
		Description:    firstNonEmpty(manifest.Description, manifest.Summary),
		Version:        resolved.Version,
		Source:         SourceCloud,
		SourceID:       "registry",
		InstalledPath:  installedPath,
		InstalledBy:    firstNonEmpty(job.InstalledBy, "user"),
		InstalledAt:    time.Now().UTC().Format(time.RFC3339),
		ChecksumSHA256: actualChecksum,
		Permissions:    append([]string(nil), resolved.Permissions...),
		RiskLevel:      resolved.RiskLevel,
		TrustLevel:     resolved.TrustLevel,
		Compatibility:  manifest.Compatibility,
		Dependencies:   resolved.Dependencies,
		Decision:       cloneDecision(job.Decision),
	}
	if err := uc.store.SaveReceipt(receipt); err != nil {
		_ = os.RemoveAll(installedPath)
		return uc.fail(job, fmt.Errorf("receipt: %w", err), true)
	}
	if job.Type == "upgrade" {
		if _, err := uc.store.UpdateBindingsForArtifactReceipt(receipt.ArtifactID, receipt.ID, receipt.Version); err != nil {
			_ = os.RemoveAll(installedPath)
			return uc.fail(job, fmt.Errorf("binding upgrade: %w", err), true)
		}
	}
	job.ReceiptID = receipt.ID
	job.State = JobSucceeded
	job.ProgressStep = "succeeded"
	job.ProgressIndex = job.ProgressTotal
	job.CompletedAt = time.Now().UTC().Format(time.RFC3339)
	if err := uc.store.UpdateJob(job); err != nil {
		return err
	}
	uc.audit("market.install.succeeded", job, job.Decision, map[string]any{"receipt_id": receipt.ID, "installed_path": installedPath})
	uc.event("market.install.succeeded", "success", "Marketplace install succeeded", job, job.Decision)
	return nil
}

func (uc *InstallUseCase) installArchive(job *InstallJob, resolved ResolvedPackage, archiveBytes []byte) (artifactManifest, string, error) {
	stageDir, err := os.MkdirTemp("", "anyclaw-market-install-*")
	if err != nil {
		return artifactManifest{}, "", err
	}
	defer os.RemoveAll(stageDir)
	if err := extractArchiveBytes(archiveBytes, resolved.DownloadURL, stageDir); err != nil {
		return artifactManifest{}, "", err
	}
	manifest, err := readArtifactManifest(stageDir)
	if err != nil {
		return artifactManifest{}, "", err
	}
	if !strings.EqualFold(manifest.ID, resolved.ArtifactID) {
		return artifactManifest{}, "", fmt.Errorf("manifest id mismatch: expected %s, got %s", resolved.ArtifactID, manifest.ID)
	}
	if manifest.Kind != resolved.Kind {
		return artifactManifest{}, "", fmt.Errorf("manifest kind mismatch: expected %s, got %s", resolved.Kind, manifest.Kind)
	}
	finalDir := filepath.Join(uc.store.InstalledDir(), string(resolved.Kind), safeName(resolved.ArtifactID), safeName(resolved.Version))
	backupDir := finalDir + ".rollback"
	_ = os.RemoveAll(backupDir)
	if _, err := os.Stat(finalDir); err == nil {
		if err := os.Rename(finalDir, backupDir); err != nil {
			return artifactManifest{}, "", err
		}
	}
	if err := os.MkdirAll(filepath.Dir(finalDir), 0o755); err != nil {
		restoreInstallBackup(finalDir, backupDir)
		return artifactManifest{}, "", err
	}
	if err := copyDir(stageDir, finalDir); err != nil {
		_ = os.RemoveAll(finalDir)
		restoreInstallBackup(finalDir, backupDir)
		return artifactManifest{}, "", err
	}
	_ = os.RemoveAll(backupDir)
	return manifest, finalDir, nil
}

func (uc *InstallUseCase) mark(job *InstallJob, state JobState, step string, index int, msg string) error {
	job.State = state
	job.ProgressStep = step
	job.ProgressIndex = index
	if msg != "" {
		job.Error = msg
	}
	return uc.store.UpdateJob(job)
}

func (uc *InstallUseCase) fail(job *InstallJob, err error, rolledBack bool) error {
	if rolledBack {
		job.State = JobRolledBack
		job.RolledBack = true
	} else {
		job.State = JobFailed
	}
	job.Error = err.Error()
	job.CompletedAt = time.Now().UTC().Format(time.RFC3339)
	_ = uc.store.UpdateJob(job)
	uc.audit("market.install.failed", job, job.Decision, map[string]any{"error": err.Error(), "rolled_back": rolledBack})
	uc.event("market.install.failed", "error", err.Error(), job, job.Decision)
	return err
}

func (uc *InstallUseCase) audit(eventType string, job *InstallJob, decision *PolicyDecision, detail map[string]any) {
	if uc == nil || uc.store == nil || job == nil {
		return
	}
	event := MarketAuditEvent{
		Type:       eventType,
		ArtifactID: job.ArtifactID,
		JobID:      job.ID,
		Actor:      firstNonEmpty(job.InstalledBy, "user"),
		Detail:     detail,
	}
	if decision != nil {
		event.Decision = string(decision.Decision)
		event.Reason = decision.Reason
	}
	_ = uc.store.AppendAudit(event)
}

func (uc *InstallUseCase) event(eventType, level, message string, job *InstallJob, decision *PolicyDecision) {
	if uc == nil || uc.store == nil || job == nil {
		return
	}
	payload := map[string]any{
		"state": job.State,
	}
	if decision != nil {
		payload["decision"] = decision.Decision
		payload["reason"] = decision.Reason
	}
	_ = uc.store.AppendEvent(MarketEvent{
		Type:       eventType,
		Level:      level,
		Message:    message,
		ArtifactID: job.ArtifactID,
		JobID:      job.ID,
		Payload:    payload,
	})
}

func eventLevelForDecision(decision PolicyDecision) string {
	switch decision.Decision {
	case DecisionBlock:
		return "error"
	case DecisionAuto:
		return "success"
	default:
		return "info"
	}
}

func cloneDecision(decision *PolicyDecision) *PolicyDecision {
	if decision == nil {
		return nil
	}
	copy := *decision
	copy.Reasons = append([]string(nil), decision.Reasons...)
	copy.Permissions = append([]string(nil), decision.Permissions...)
	copy.HighRiskPermissions = append([]string(nil), decision.HighRiskPermissions...)
	return &copy
}

type artifactManifest struct {
	ID            string        `json:"id"`
	Kind          ArtifactKind  `json:"kind"`
	Name          string        `json:"name"`
	Description   string        `json:"description,omitempty"`
	Summary       string        `json:"summary,omitempty"`
	Version       string        `json:"version"`
	Compatibility Compatibility `json:"compatibility,omitempty"`
}

func readArtifactManifest(root string) (artifactManifest, error) {
	data, err := os.ReadFile(filepath.Join(root, "anyclaw.artifact.json"))
	if err != nil {
		return artifactManifest{}, fmt.Errorf("manifest missing: %w", err)
	}
	var manifest artifactManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return artifactManifest{}, fmt.Errorf("manifest invalid: %w", err)
	}
	if manifest.ID == "" || manifest.Kind == "" || manifest.Version == "" {
		return artifactManifest{}, fmt.Errorf("manifest requires id, kind, and version")
	}
	return manifest, nil
}

func extractArchiveBytes(data []byte, rawURL string, destDir string) error {
	if strings.HasSuffix(strings.ToLower(strings.TrimSpace(rawURL)), ".tar.gz") || strings.HasSuffix(strings.ToLower(strings.TrimSpace(rawURL)), ".tgz") {
		return extractTarGzBytes(data, destDir)
	}
	return extractZipBytes(data, destDir)
}

func extractZipBytes(data []byte, destDir string) error {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	for _, file := range reader.File {
		if file.FileInfo().Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("zip symlinks are not supported: %s", file.Name)
		}
		targetPath := filepath.Join(destDir, file.Name)
		if !pathWithinBase(destDir, targetPath) {
			return fmt.Errorf("zip entry escapes destination: %s", file.Name)
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}
		src, err := file.Open()
		if err != nil {
			return err
		}
		dst, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, file.Mode())
		if err != nil {
			src.Close()
			return err
		}
		_, copyErr := io.Copy(dst, src)
		src.Close()
		dst.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}

func extractTarGzBytes(data []byte, destDir string) error {
	gzr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer gzr.Close()
	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		targetPath := filepath.Join(destDir, header.Name)
		if !pathWithinBase(destDir, targetPath) {
			return fmt.Errorf("tar entry escapes destination: %s", header.Name)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return err
			}
			dst, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(header.Mode)&0o777)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(dst, tr)
			closeErr := dst.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			return fmt.Errorf("unsupported tar entry type for %s", header.Name)
		}
	}
	return nil
}

func copyDir(srcDir, destDir string) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlinks are not supported: %s", path)
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil || rel == "." {
			return err
		}
		target := filepath.Join(destDir, rel)
		if !pathWithinBase(destDir, target) {
			return fmt.Errorf("copy target escapes destination: %s", rel)
		}
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(srcPath, destPath string, mode os.FileMode) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.OpenFile(destPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(dst, src)
	closeErr := dst.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func restoreInstallBackup(finalDir, backupDir string) {
	if _, err := os.Stat(backupDir); err == nil {
		_ = os.RemoveAll(finalDir)
		_ = os.Rename(backupDir, finalDir)
	}
}

func pathWithinBase(baseDir, targetPath string) bool {
	baseDir = filepath.Clean(baseDir)
	targetPath = filepath.Clean(targetPath)
	return targetPath == baseDir || strings.HasPrefix(targetPath, baseDir+string(os.PathSeparator))
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func receiptID(artifactID, version string) string {
	return artifactID + "@" + version
}
