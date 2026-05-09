package marketregistry

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var (
	ErrArtifactNotFound     = errors.New("artifact not found")
	ErrVersionNotFound      = errors.New("artifact version not found")
	ErrNoCompatibleVersion  = errors.New("no compatible artifact version found")
	ErrInvalidArtifactKind  = errors.New("invalid artifact kind")
	ErrArtifactUnavailable  = errors.New("artifact unavailable")
	defaultProtocolVersion  = "1.0"
	defaultRegistrySourceID = "anyclaw-cloud"
)

type Store struct {
	db        *sql.DB
	vector    *vectorRuntime
	enableFTS bool
}

type searchExecution struct {
	items []searchCandidate
	total int
	meta  RetrievalMeta
}

func OpenStore(ctx context.Context, dataDir string) (*Store, error) {
	return OpenStoreWithConfig(ctx, StoreConfig{DataDir: dataDir})
}

func OpenStoreWithConfig(ctx context.Context, cfg StoreConfig) (*Store, error) {
	dataDir := cfg.DataDir
	if strings.TrimSpace(dataDir) == "" {
		dataDir = ".anyclaw-registry"
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "audit"), 0o755); err != nil {
		return nil, err
	}
	driver := strings.TrimSpace(cfg.Driver)
	if driver == "" {
		driver = "sqlite"
	}
	dsn := strings.TrimSpace(cfg.DSN)
	if dsn == "" {
		if driver != "sqlite" {
			return nil, fmt.Errorf("registry db dsn is required for driver %s", driver)
		}
		dsn = filepath.Join(dataDir, "registry.db")
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, err
	}
	if driver == "sqlite" {
		// Serialize SQLite access so publish/upsert requests and the embedding
		// worker do not contend on concurrent writes.
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
	}
	store := &Store{db: db, enableFTS: driver == "sqlite"}
	if err := store.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) SetVectorRuntime(runtime *vectorRuntime) {
	if s == nil {
		return
	}
	s.vector = runtime
}

func (s *Store) migrate(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS artifacts (
			id TEXT PRIMARY KEY,
			kind TEXT NOT NULL,
			name TEXT NOT NULL,
			summary TEXT NOT NULL,
			description_md TEXT NOT NULL DEFAULT '',
			latest_version TEXT NOT NULL,
			source TEXT NOT NULL,
			publisher TEXT NOT NULL,
			risk_level TEXT NOT NULL,
			trust_level TEXT NOT NULL,
			permissions_json TEXT NOT NULL,
			compatibility_json TEXT NOT NULL,
			dependencies_json TEXT NOT NULL,
			icon_url TEXT NOT NULL DEFAULT '',
			tags_json TEXT NOT NULL,
			hit_signals_json TEXT NOT NULL,
			score REAL NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL,
			manifest_summary_json TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS artifact_versions (
			artifact_id TEXT NOT NULL,
			version TEXT NOT NULL,
			released_at TEXT NOT NULL,
			changelog_md TEXT NOT NULL DEFAULT '',
			compatibility_json TEXT NOT NULL,
			permissions_json TEXT NOT NULL,
			permissions_diff_json TEXT NOT NULL,
			size_bytes INTEGER NOT NULL DEFAULT 0,
			checksum_sha256 TEXT NOT NULL DEFAULT '',
			signature TEXT NOT NULL DEFAULT '',
			storage_key TEXT NOT NULL DEFAULT '',
			deprecated INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (artifact_id, version),
			FOREIGN KEY (artifact_id) REFERENCES artifacts(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS publishers (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			trust_level TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS tokens (
			id TEXT PRIMARY KEY,
			publisher_id TEXT NOT NULL,
			token_hash TEXT NOT NULL,
			created_at TEXT NOT NULL,
			revoked_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS downloads (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			artifact_id TEXT NOT NULL,
			version TEXT NOT NULL,
			remote_addr TEXT NOT NULL,
			user_agent TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS quarantine (
			artifact_id TEXT PRIMARY KEY,
			reason TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS audit_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			event_type TEXT NOT NULL,
			artifact_id TEXT NOT NULL DEFAULT '',
			version TEXT NOT NULL DEFAULT '',
			detail_json TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS artifact_embeddings (
			artifact_id TEXT PRIMARY KEY,
			model TEXT NOT NULL DEFAULT '',
			vector_json TEXT NOT NULL DEFAULT '[]',
			vector_dim INTEGER NOT NULL DEFAULT 0,
			embedding_text TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'pending',
			error TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS artifact_embedding_jobs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			artifact_id TEXT NOT NULL,
			job_type TEXT NOT NULL,
			status TEXT NOT NULL,
			attempt_count INTEGER NOT NULL DEFAULT 0,
			error TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_artifacts_kind ON artifacts(kind)`,
		`CREATE INDEX IF NOT EXISTS idx_artifacts_source ON artifacts(source)`,
		`CREATE INDEX IF NOT EXISTS idx_artifacts_risk ON artifacts(risk_level)`,
		`CREATE INDEX IF NOT EXISTS idx_artifacts_trust ON artifacts(trust_level)`,
		`CREATE INDEX IF NOT EXISTS idx_artifacts_updated ON artifacts(updated_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_quarantine_artifact_id ON quarantine(artifact_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_artifact_embedding_jobs_unique ON artifact_embedding_jobs(artifact_id, job_type)`,
		`CREATE INDEX IF NOT EXISTS idx_artifact_embedding_jobs_status ON artifact_embedding_jobs(status, updated_at, id)`,
		`CREATE INDEX IF NOT EXISTS idx_artifact_embeddings_status ON artifact_embeddings(status, updated_at)`,
	}
	for _, stmt := range statements {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	if s.enableFTS {
		if _, err := s.db.ExecContext(ctx, `CREATE VIRTUAL TABLE IF NOT EXISTS artifacts_fts USING fts5(
			artifact_id UNINDEXED,
			name,
			summary,
			description_md,
			publisher,
			kind,
			tags_text,
			hit_signals_text,
			use_case_text,
			search_text,
			tokenize = 'unicode61'
		)`); err != nil {
			return err
		}
	}
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE artifact_versions ADD COLUMN signature TEXT NOT NULL DEFAULT ''`)
	if !s.enableFTS {
		return nil
	}
	return s.rebuildArtifactSearchIndex(ctx)
}

func (s *Store) CountArtifacts(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM artifacts`).Scan(&count)
	return count, err
}

func (s *Store) DeleteArtifact(ctx context.Context, artifactID string) (ArtifactDeletion, error) {
	artifactID = strings.TrimSpace(artifactID)
	if artifactID == "" {
		return ArtifactDeletion{}, fmt.Errorf("artifact_id is required")
	}
	if _, err := s.Get(ctx, artifactID); err != nil {
		return ArtifactDeletion{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ArtifactDeletion{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if _, err = tx.ExecContext(ctx, `DELETE FROM artifact_versions WHERE artifact_id = ?`, artifactID); err != nil {
		return ArtifactDeletion{}, err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM artifact_embedding_jobs WHERE artifact_id = ?`, artifactID); err != nil {
		return ArtifactDeletion{}, err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM artifact_embeddings WHERE artifact_id = ?`, artifactID); err != nil {
		return ArtifactDeletion{}, err
	}
	if s.enableFTS {
		if _, err = tx.ExecContext(ctx, `DELETE FROM artifacts_fts WHERE artifact_id = ?`, artifactID); err != nil {
			return ArtifactDeletion{}, err
		}
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM quarantine WHERE artifact_id = ?`, artifactID); err != nil {
		return ArtifactDeletion{}, err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM artifacts WHERE id = ?`, artifactID)
	if err != nil {
		return ArtifactDeletion{}, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return ArtifactDeletion{}, err
	}
	if rows == 0 {
		return ArtifactDeletion{}, ErrArtifactNotFound
	}
	record := ArtifactDeletion{
		ArtifactID: artifactID,
		DeletedAt:  time.Now().UTC().Format(time.RFC3339),
	}
	detail, err := encodeJSON(map[string]any{})
	if err != nil {
		return ArtifactDeletion{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO audit_events (event_type, artifact_id, version, detail_json, created_at) VALUES (?, ?, ?, ?, ?)`,
		"artifact.deleted", artifactID, "", detail, record.DeletedAt); err != nil {
		return ArtifactDeletion{}, err
	}
	if err = tx.Commit(); err != nil {
		return ArtifactDeletion{}, err
	}
	return record, nil
}

func (s *Store) UpsertArtifact(ctx context.Context, artifact Artifact, versions []ArtifactVersion) error {
	if err := validateArtifact(artifact); err != nil {
		return err
	}
	if artifact.Source == "" {
		artifact.Source = defaultRegistrySourceID
	}
	if artifact.UpdatedAt == "" {
		artifact.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	permissions, err := encodeJSON(artifact.Permissions)
	if err != nil {
		return err
	}
	compatibility, err := encodeJSON(artifact.Compatibility)
	if err != nil {
		return err
	}
	dependencies, err := encodeJSON(artifact.Dependencies)
	if err != nil {
		return err
	}
	tags, err := encodeJSON(artifact.Tags)
	if err != nil {
		return err
	}
	hitSignals, err := encodeJSON(artifact.HitSignals)
	if err != nil {
		return err
	}
	manifestSummary, err := encodeJSON(artifact.ManifestSummary)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `INSERT INTO artifacts (
		id, kind, name, summary, description_md, latest_version, source, publisher,
		risk_level, trust_level, permissions_json, compatibility_json,
		dependencies_json, icon_url, tags_json, hit_signals_json, score,
		updated_at, manifest_summary_json
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		kind = excluded.kind,
		name = excluded.name,
		summary = excluded.summary,
		description_md = excluded.description_md,
		latest_version = excluded.latest_version,
		source = excluded.source,
		publisher = excluded.publisher,
		risk_level = excluded.risk_level,
		trust_level = excluded.trust_level,
		permissions_json = excluded.permissions_json,
		compatibility_json = excluded.compatibility_json,
		dependencies_json = excluded.dependencies_json,
		icon_url = excluded.icon_url,
		tags_json = excluded.tags_json,
		hit_signals_json = excluded.hit_signals_json,
		score = excluded.score,
		updated_at = excluded.updated_at,
		manifest_summary_json = excluded.manifest_summary_json`,
		artifact.ID, artifact.Kind, artifact.Name, artifact.Summary, artifact.DescriptionMD,
		artifact.LatestVersion, artifact.Source, artifact.Publisher, artifact.RiskLevel,
		artifact.TrustLevel, permissions, compatibility, dependencies, artifact.IconURL,
		tags, hitSignals, artifact.Score, artifact.UpdatedAt, manifestSummary)
	if err != nil {
		return err
	}

	for _, version := range versions {
		if version.ArtifactID == "" {
			version.ArtifactID = artifact.ID
		}
		if version.ReleasedAt == "" {
			version.ReleasedAt = artifact.UpdatedAt
		}
		if version.Compatibility.AnyClawMin == "" && len(version.Compatibility.OS) == 0 && len(version.Compatibility.Arch) == 0 {
			version.Compatibility = artifact.Compatibility
		}
		if len(version.Permissions) == 0 {
			version.Permissions = artifact.Permissions
		}
		compatibility, err := encodeJSON(version.Compatibility)
		if err != nil {
			return err
		}
		permissions, err := encodeJSON(version.Permissions)
		if err != nil {
			return err
		}
		permissionsDiff, err := encodeJSON(version.PermissionsDiff)
		if err != nil {
			return err
		}
		deprecated := 0
		if version.Deprecated {
			deprecated = 1
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO artifact_versions (
			artifact_id, version, released_at, changelog_md, compatibility_json,
			permissions_json, permissions_diff_json, size_bytes, checksum_sha256,
			signature, storage_key, deprecated
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(artifact_id, version) DO UPDATE SET
			released_at = excluded.released_at,
			changelog_md = excluded.changelog_md,
			compatibility_json = excluded.compatibility_json,
			permissions_json = excluded.permissions_json,
			permissions_diff_json = excluded.permissions_diff_json,
			size_bytes = excluded.size_bytes,
			checksum_sha256 = excluded.checksum_sha256,
			signature = excluded.signature,
			storage_key = excluded.storage_key,
			deprecated = excluded.deprecated`,
			version.ArtifactID, version.Version, version.ReleasedAt, version.ChangelogMD,
			compatibility, permissions, permissionsDiff, version.SizeBytes,
			version.ChecksumSHA256, version.Signature, version.StorageKey, deprecated)
		if err != nil {
			return err
		}
	}
	if err := s.upsertArtifactSearchDocTx(ctx, tx, artifact); err != nil {
		return err
	}
	if err := s.queueArtifactEmbeddingJobTx(ctx, tx, artifact); err != nil {
		return err
	}
	err = tx.Commit()
	return err
}

func (s *Store) List(ctx context.Context, filter SearchFilter) (ListResult, error) {
	normalized := normalizeSearchFilter(filter)
	exec, err := s.search(ctx, normalized)
	if err != nil {
		return ListResult{}, err
	}
	items := make([]Artifact, 0, len(exec.items))
	for _, candidate := range exec.items {
		items = append(items, candidate.Artifact)
	}
	return ListResult{
		Items:         items,
		Total:         exec.total,
		Limit:         normalized.Limit,
		Offset:        normalized.Offset,
		RetrievalMeta: cloneRetrievalMeta(exec.meta),
	}, nil
}

func (s *Store) Get(ctx context.Context, id string) (Artifact, error) {
	row := s.db.QueryRowContext(ctx, `SELECT
		id, kind, name, summary, description_md, latest_version, source, publisher,
		risk_level, trust_level, permissions_json, compatibility_json, dependencies_json,
		icon_url, tags_json, hit_signals_json, score, updated_at, manifest_summary_json
		FROM artifacts WHERE id = ?`, id)
	artifact, err := scanArtifact(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Artifact{}, ErrArtifactNotFound
	}
	return artifact, err
}

func (s *Store) Versions(ctx context.Context, artifactID string) ([]ArtifactVersion, error) {
	if _, err := s.Get(ctx, artifactID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT
		artifact_id, version, released_at, changelog_md, compatibility_json,
		permissions_json, permissions_diff_json, size_bytes, checksum_sha256,
		signature, storage_key, deprecated
		FROM artifact_versions WHERE artifact_id = ? ORDER BY released_at DESC, version DESC`, artifactID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []ArtifactVersion
	for rows.Next() {
		version, err := scanVersion(rows)
		if err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	return versions, rows.Err()
}

func (s *Store) Version(ctx context.Context, artifactID, version string) (ArtifactVersion, error) {
	row := s.db.QueryRowContext(ctx, `SELECT
		artifact_id, version, released_at, changelog_md, compatibility_json,
		permissions_json, permissions_diff_json, size_bytes, checksum_sha256,
		signature, storage_key, deprecated
		FROM artifact_versions WHERE artifact_id = ? AND version = ?`, artifactID, version)
	item, err := scanVersion(row)
	if errors.Is(err, sql.ErrNoRows) {
		return ArtifactVersion{}, ErrVersionNotFound
	}
	return item, err
}

func (s *Store) Resolve(ctx context.Context, artifactID string, req ResolveRequest) (Artifact, ArtifactVersion, error) {
	if _, err := s.Quarantine(ctx, artifactID); err == nil {
		return Artifact{}, ArtifactVersion{}, ErrArtifactUnavailable
	}
	artifact, err := s.Get(ctx, artifactID)
	if err != nil {
		return Artifact{}, ArtifactVersion{}, err
	}
	versions, err := s.Versions(ctx, artifactID)
	if err != nil {
		return Artifact{}, ArtifactVersion{}, err
	}

	want := strings.TrimSpace(req.VersionConstraint)
	foundRequestedVersion := false
	for _, version := range versions {
		if want != "" && version.Version != want {
			continue
		}
		if want != "" {
			foundRequestedVersion = true
		}
		if !compatibleWithClient(version.Compatibility, req) {
			continue
		}
		return artifact, version, nil
	}
	if want != "" && !foundRequestedVersion {
		return Artifact{}, ArtifactVersion{}, ErrVersionNotFound
	}
	return Artifact{}, ArtifactVersion{}, ErrNoCompatibleVersion
}

func (s *Store) RecordDownload(ctx context.Context, artifactID, version, remoteAddr, userAgent string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO downloads (
		artifact_id, version, remote_addr, user_agent, created_at
	) VALUES (?, ?, ?, ?, ?)`, artifactID, version, remoteAddr, userAgent, time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *Store) DownloadStats(ctx context.Context, limit int) (DownloadStatsResult, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT artifact_id, version, COUNT(*), MAX(created_at)
		FROM downloads GROUP BY artifact_id, version ORDER BY COUNT(*) DESC, MAX(created_at) DESC LIMIT ?`, limit)
	if err != nil {
		return DownloadStatsResult{}, err
	}
	defer rows.Close()
	var items []DownloadStat
	for rows.Next() {
		var item DownloadStat
		if err := rows.Scan(&item.ArtifactID, &item.Version, &item.Count, &item.LastAt); err != nil {
			return DownloadStatsResult{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return DownloadStatsResult{}, err
	}
	return DownloadStatsResult{Items: items, Total: len(items)}, nil
}

func (s *Store) Quarantine(ctx context.Context, artifactID string) (QuarantineRecord, error) {
	var record QuarantineRecord
	err := s.db.QueryRowContext(ctx, `SELECT artifact_id, reason, created_at FROM quarantine WHERE artifact_id = ?`, strings.TrimSpace(artifactID)).
		Scan(&record.ArtifactID, &record.Reason, &record.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return QuarantineRecord{}, ErrArtifactNotFound
	}
	return record, err
}

func (s *Store) SetQuarantine(ctx context.Context, artifactID, reason string) (QuarantineRecord, error) {
	record := QuarantineRecord{
		ArtifactID: strings.TrimSpace(artifactID),
		Reason:     strings.TrimSpace(reason),
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
	}
	if record.ArtifactID == "" {
		return QuarantineRecord{}, fmt.Errorf("artifact_id is required")
	}
	if record.Reason == "" {
		record.Reason = "quarantined by administrator"
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO quarantine (artifact_id, reason, created_at)
		VALUES (?, ?, ?) ON CONFLICT(artifact_id) DO UPDATE SET reason = excluded.reason, created_at = excluded.created_at`,
		record.ArtifactID, record.Reason, record.CreatedAt); err != nil {
		return QuarantineRecord{}, err
	}
	_ = s.AppendAudit(ctx, RegistryAuditEvent{Event: "artifact.quarantined", Artifact: record.ArtifactID, Detail: map[string]any{"reason": record.Reason}})
	return record, nil
}

func (s *Store) ClearQuarantine(ctx context.Context, artifactID string) error {
	artifactID = strings.TrimSpace(artifactID)
	if artifactID == "" {
		return fmt.Errorf("artifact_id is required")
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM quarantine WHERE artifact_id = ?`, artifactID)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ErrArtifactNotFound
	}
	_ = s.AppendAudit(ctx, RegistryAuditEvent{Event: "artifact.unquarantined", Artifact: artifactID})
	return nil
}

func (s *Store) CreatePublisherToken(ctx context.Context, publisherID string) (PublisherToken, error) {
	publisherID = strings.TrimSpace(publisherID)
	if publisherID == "" {
		return PublisherToken{}, fmt.Errorf("publisher_id is required")
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return PublisherToken{}, err
	}
	token := "acr_" + hex.EncodeToString(raw)
	now := time.Now().UTC().Format(time.RFC3339)
	hash := sha256.Sum256([]byte(token))
	item := PublisherToken{
		ID:          "token-" + time.Now().UTC().Format("20060102150405.000000000"),
		PublisherID: publisherID,
		Token:       token,
		CreatedAt:   now,
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO tokens (id, publisher_id, token_hash, created_at) VALUES (?, ?, ?, ?)`,
		item.ID, item.PublisherID, hex.EncodeToString(hash[:]), item.CreatedAt)
	if err != nil {
		return PublisherToken{}, err
	}
	_ = s.AppendAudit(ctx, RegistryAuditEvent{Event: "publisher_token.created", Detail: map[string]any{"publisher_id": publisherID, "token_id": item.ID}})
	return item, nil
}

func (s *Store) ValidatePublisherToken(ctx context.Context, token string) (string, bool, error) {
	hash := sha256.Sum256([]byte(strings.TrimSpace(token)))
	var publisherID string
	err := s.db.QueryRowContext(ctx, `SELECT publisher_id FROM tokens WHERE token_hash = ? AND revoked_at = ''`, hex.EncodeToString(hash[:])).Scan(&publisherID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return publisherID, true, nil
}

func (s *Store) PublisherTokens(ctx context.Context, limit int) (PublisherTokenList, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, publisher_id, created_at, revoked_at FROM tokens ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return PublisherTokenList{}, err
	}
	defer rows.Close()
	var items []PublisherToken
	for rows.Next() {
		var item PublisherToken
		if err := rows.Scan(&item.ID, &item.PublisherID, &item.CreatedAt, &item.RevokedAt); err != nil {
			return PublisherTokenList{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return PublisherTokenList{}, err
	}
	return PublisherTokenList{Items: items, Total: len(items)}, nil
}

func (s *Store) RevokePublisherToken(ctx context.Context, tokenID string) (PublisherTokenRevocation, error) {
	tokenID = strings.TrimSpace(tokenID)
	if tokenID == "" {
		return PublisherTokenRevocation{}, fmt.Errorf("token id is required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.ExecContext(ctx, `UPDATE tokens SET revoked_at = ? WHERE id = ? AND revoked_at = ''`, now, tokenID)
	if err != nil {
		return PublisherTokenRevocation{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return PublisherTokenRevocation{}, err
	}
	if affected == 0 {
		return PublisherTokenRevocation{}, ErrArtifactNotFound
	}
	var publisherID string
	if err := s.db.QueryRowContext(ctx, `SELECT publisher_id FROM tokens WHERE id = ?`, tokenID).Scan(&publisherID); err != nil {
		return PublisherTokenRevocation{}, err
	}
	record := PublisherTokenRevocation{ID: tokenID, PublisherID: publisherID, RevokedAt: now}
	_ = s.AppendAudit(ctx, RegistryAuditEvent{Event: "publisher_token.revoked", Detail: map[string]any{"publisher_id": publisherID, "token_id": tokenID}})
	return record, nil
}

func (s *Store) AppendAudit(ctx context.Context, event RegistryAuditEvent) error {
	if event.Created == "" {
		event.Created = time.Now().UTC().Format(time.RFC3339)
	}
	detail, err := encodeJSON(event.Detail)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO audit_events (event_type, artifact_id, version, detail_json, created_at) VALUES (?, ?, ?, ?, ?)`,
		event.Event, event.Artifact, event.Version, detail, event.Created)
	return err
}

func (s *Store) AuditEvents(ctx context.Context, limit int) (RegistryAuditList, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, event_type, artifact_id, version, detail_json, created_at FROM audit_events ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return RegistryAuditList{}, err
	}
	defer rows.Close()
	var items []RegistryAuditEvent
	for rows.Next() {
		var item RegistryAuditEvent
		var detail string
		if err := rows.Scan(&item.ID, &item.Event, &item.Artifact, &item.Version, &detail, &item.Created); err != nil {
			return RegistryAuditList{}, err
		}
		if err := decodeJSON(detail, &item.Detail); err != nil {
			return RegistryAuditList{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return RegistryAuditList{}, err
	}
	return RegistryAuditList{Items: items, Total: len(items)}, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanArtifact(row scanner) (Artifact, error) {
	var artifact Artifact
	var permissions, compatibility, dependencies, tags, hitSignals, manifestSummary string
	err := row.Scan(
		&artifact.ID, &artifact.Kind, &artifact.Name, &artifact.Summary,
		&artifact.DescriptionMD, &artifact.LatestVersion, &artifact.Source,
		&artifact.Publisher, &artifact.RiskLevel, &artifact.TrustLevel,
		&permissions, &compatibility, &dependencies, &artifact.IconURL,
		&tags, &hitSignals, &artifact.Score, &artifact.UpdatedAt,
		&manifestSummary,
	)
	if err != nil {
		return Artifact{}, err
	}
	artifact.Version = artifact.LatestVersion
	if err := decodeJSON(permissions, &artifact.Permissions); err != nil {
		return Artifact{}, err
	}
	if err := decodeJSON(compatibility, &artifact.Compatibility); err != nil {
		return Artifact{}, err
	}
	if err := decodeJSON(dependencies, &artifact.Dependencies); err != nil {
		return Artifact{}, err
	}
	if err := decodeJSON(tags, &artifact.Tags); err != nil {
		return Artifact{}, err
	}
	if err := decodeJSON(hitSignals, &artifact.HitSignals); err != nil {
		return Artifact{}, err
	}
	if err := decodeJSON(manifestSummary, &artifact.ManifestSummary); err != nil {
		return Artifact{}, err
	}
	return artifact, nil
}

func scanVersion(row scanner) (ArtifactVersion, error) {
	var version ArtifactVersion
	var compatibility, permissions, permissionsDiff string
	var deprecated int
	err := row.Scan(
		&version.ArtifactID, &version.Version, &version.ReleasedAt,
		&version.ChangelogMD, &compatibility, &permissions, &permissionsDiff,
		&version.SizeBytes, &version.ChecksumSHA256, &version.Signature,
		&version.StorageKey, &deprecated,
	)
	if err != nil {
		return ArtifactVersion{}, err
	}
	version.Deprecated = deprecated != 0
	if err := decodeJSON(compatibility, &version.Compatibility); err != nil {
		return ArtifactVersion{}, err
	}
	if err := decodeJSON(permissions, &version.Permissions); err != nil {
		return ArtifactVersion{}, err
	}
	if err := decodeJSON(permissionsDiff, &version.PermissionsDiff); err != nil {
		return ArtifactVersion{}, err
	}
	return version, nil
}

func sortArtifacts(items []Artifact, mode string) {
	sortMode := strings.ToLower(strings.TrimSpace(mode))
	sort.SliceStable(items, func(i, j int) bool {
		switch sortMode {
		case "updated", "updated_desc":
			if items[i].UpdatedAt == items[j].UpdatedAt {
				return fallbackArtifactLess(items[i], items[j])
			}
			return items[i].UpdatedAt > items[j].UpdatedAt
		case "name", "name_asc":
			left := strings.ToLower(items[i].Name)
			right := strings.ToLower(items[j].Name)
			if left == right {
				return fallbackArtifactLess(items[i], items[j])
			}
			return left < right
		default:
			leftScore := scoreValue(items[i])
			rightScore := scoreValue(items[j])
			if leftScore == rightScore {
				if items[i].UpdatedAt == items[j].UpdatedAt {
					return fallbackArtifactLess(items[i], items[j])
				}
				return items[i].UpdatedAt > items[j].UpdatedAt
			}
			return leftScore > rightScore
		}
	})
}

func (s *Store) search(ctx context.Context, filter SearchFilter) (searchExecution, error) {
	candidates, total, err := s.searchCandidates(ctx, filter)
	if err != nil {
		return searchExecution{}, err
	}
	meta := RetrievalMeta{
		SearchMode:    resolvedSearchMode(filter, false),
		VectorApplied: false,
		CandidateCounts: &CandidateCounts{
			Lexical: total,
			Vector:  0,
			Merged:  total,
		},
	}
	if shouldAttemptHybrid(filter) {
		merged, mergedTotal, hybridMeta := s.searchHybrid(ctx, filter, candidates, total)
		if hybridMeta.VectorApplied {
			candidates = merged
			total = mergedTotal
			meta = hybridMeta
		} else {
			meta = hybridMeta
		}
	}
	return searchExecution{
		items: paginateCandidates(candidates, filter.Offset, filter.Limit),
		total: total,
		meta:  meta,
	}, nil
}

func (s *Store) searchCandidates(ctx context.Context, filter SearchFilter) ([]searchCandidate, int, error) {
	if strings.TrimSpace(filter.Query) == "" {
		return s.listStructuredCandidates(ctx, filter)
	}
	return s.listLexicalCandidates(ctx, filter)
}

func (s *Store) listStructuredCandidates(ctx context.Context, filter SearchFilter) ([]searchCandidate, int, error) {
	where, args := appendStructuredFilterClauses(nil, nil, filter)
	whereSQL := ""
	if len(where) > 0 {
		whereSQL = " WHERE " + strings.Join(where, " AND ")
	}
	countQuery := `SELECT COUNT(*) FROM artifacts a` + whereSQL
	var total int
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if total == 0 || filter.Offset >= total {
		return nil, total, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT
		a.id, a.kind, a.name, a.summary, a.description_md, a.latest_version, a.source, a.publisher,
		a.risk_level, a.trust_level, a.permissions_json, a.compatibility_json, a.dependencies_json,
		a.icon_url, a.tags_json, a.hit_signals_json, a.score, a.updated_at, a.manifest_summary_json
		FROM artifacts a`+whereSQL, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	candidates := make([]searchCandidate, 0, total)
	for rows.Next() {
		artifact, err := scanArtifact(rows)
		if err != nil {
			return nil, 0, err
		}
		candidate := searchCandidate{Artifact: artifact}
		applySearchScores(&candidate, filter)
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	sortSearchCandidates(candidates, filter.Sort)
	return candidates, total, nil
}

func (s *Store) listLexicalCandidates(ctx context.Context, filter SearchFilter) ([]searchCandidate, int, error) {
	if s == nil || !s.enableFTS {
		return s.listStructuredCandidates(ctx, filter)
	}
	matchQuery := buildFTSQuery(filter.Query)
	if matchQuery == "" {
		return s.listStructuredCandidates(ctx, filter)
	}
	where, args := appendStructuredFilterClauses([]string{`artifacts_fts MATCH ?`}, []any{matchQuery}, filter)
	whereSQL := " WHERE " + strings.Join(where, " AND ")
	countQuery := `SELECT COUNT(*) FROM artifacts_fts JOIN artifacts a ON a.id = artifacts_fts.artifact_id` + whereSQL
	var total int
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if total == 0 || filter.Offset >= total {
		return nil, total, nil
	}

	candidateLimit := filter.Offset + filter.Limit*4
	if candidateLimit < filter.Offset+filter.Limit {
		candidateLimit = filter.Offset + filter.Limit
	}
	if candidateLimit < filter.Limit*2 {
		candidateLimit = filter.Limit * 2
	}
	if candidateLimit < defaultSearchCandidateCap/2 {
		candidateLimit = defaultSearchCandidateCap / 2
	}
	if candidateLimit > defaultSearchCandidateCap {
		candidateLimit = defaultSearchCandidateCap
	}

	query := `SELECT
		a.id, a.kind, a.name, a.summary, a.description_md, a.latest_version, a.source, a.publisher,
		a.risk_level, a.trust_level, a.permissions_json, a.compatibility_json, a.dependencies_json,
		a.icon_url, a.tags_json, a.hit_signals_json, a.score, a.updated_at, a.manifest_summary_json,
		bm25(artifacts_fts) AS lexical_rank
		FROM artifacts_fts
		JOIN artifacts a ON a.id = artifacts_fts.artifact_id` + whereSQL + `
		ORDER BY bm25(artifacts_fts), a.updated_at DESC
		LIMIT ?`
	argsWithLimit := append(append([]any(nil), args...), candidateLimit)
	rows, err := s.db.QueryContext(ctx, query, argsWithLimit...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	candidates := make([]searchCandidate, 0, candidateLimit)
	for rows.Next() {
		artifact, rank, err := scanArtifactWithLexicalRank(rows)
		if err != nil {
			return nil, 0, err
		}
		candidate := searchCandidate{Artifact: artifact, lexicalRank: rank}
		applySearchScores(&candidate, filter)
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	sortSearchCandidates(candidates, filter.Sort)
	return candidates, total, nil
}

func paginateCandidates(items []searchCandidate, offset int, limit int) []searchCandidate {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	if offset >= len(items) {
		return nil
	}
	items = items[offset:]
	if len(items) > limit {
		items = items[:limit]
	}
	return items
}

func sortSearchCandidates(items []searchCandidate, mode string) {
	sort.SliceStable(items, func(i, j int) bool {
		switch strings.ToLower(strings.TrimSpace(mode)) {
		case "updated", "updated_desc":
			if items[i].UpdatedAt == items[j].UpdatedAt {
				return fallbackArtifactLess(items[i].Artifact, items[j].Artifact)
			}
			return items[i].UpdatedAt > items[j].UpdatedAt
		case "name", "name_asc":
			left := strings.ToLower(items[i].Name)
			right := strings.ToLower(items[j].Name)
			if left == right {
				return fallbackArtifactLess(items[i].Artifact, items[j].Artifact)
			}
			return left < right
		default:
			if items[i].FinalScore == items[j].FinalScore {
				if items[i].lexicalRank == items[j].lexicalRank {
					return fallbackArtifactLess(items[i].Artifact, items[j].Artifact)
				}
				return items[i].lexicalRank < items[j].lexicalRank
			}
			return items[i].FinalScore > items[j].FinalScore
		}
	})
}

func normalizeSearchFilter(filter SearchFilter) SearchFilter {
	if filter.Limit <= 0 {
		filter.Limit = defaultSearchLimit
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	filter.Query = strings.TrimSpace(filter.Query)
	switch filter.SearchMode {
	case SearchModeAuto, SearchModeLexical, SearchModeHybrid:
	default:
		filter.SearchMode = SearchModeAuto
	}
	return filter
}

func resolvedSearchMode(filter SearchFilter, vectorApplied bool) SearchMode {
	if strings.TrimSpace(filter.Query) == "" {
		return SearchModeLexical
	}
	switch filter.SearchMode {
	case SearchModeLexical:
		return SearchModeLexical
	case SearchModeAuto, SearchModeHybrid:
		if vectorApplied {
			return SearchModeHybrid
		}
		return SearchModeHybrid
	default:
		return SearchModeLexical
	}
}

func shouldAttemptHybrid(filter SearchFilter) bool {
	if strings.TrimSpace(filter.Query) == "" {
		return false
	}
	return filter.SearchMode == SearchModeAuto || filter.SearchMode == SearchModeHybrid
}

func cloneRetrievalMeta(meta RetrievalMeta) *RetrievalMeta {
	counts := meta.CandidateCounts
	if counts != nil {
		copyCounts := *counts
		counts = &copyCounts
	}
	return &RetrievalMeta{
		SearchMode:           meta.SearchMode,
		VectorApplied:        meta.VectorApplied,
		VectorFallbackReason: meta.VectorFallbackReason,
		CandidateCounts:      counts,
	}
}

func scoreValue(item Artifact) float64 {
	if item.FinalScore > 0 {
		return item.FinalScore
	}
	if item.Score > 0 {
		return item.Score
	}
	return item.LexicalScore
}

func scanArtifactWithLexicalRank(row scanner) (Artifact, float64, error) {
	var artifact Artifact
	var permissions, compatibility, dependencies, tags, hitSignals, manifestSummary string
	var lexicalRank float64
	err := row.Scan(
		&artifact.ID, &artifact.Kind, &artifact.Name, &artifact.Summary,
		&artifact.DescriptionMD, &artifact.LatestVersion, &artifact.Source,
		&artifact.Publisher, &artifact.RiskLevel, &artifact.TrustLevel,
		&permissions, &compatibility, &dependencies, &artifact.IconURL,
		&tags, &hitSignals, &artifact.Score, &artifact.UpdatedAt,
		&manifestSummary, &lexicalRank,
	)
	if err != nil {
		return Artifact{}, 0, err
	}
	artifact.Version = artifact.LatestVersion
	if err := decodeJSON(permissions, &artifact.Permissions); err != nil {
		return Artifact{}, 0, err
	}
	if err := decodeJSON(compatibility, &artifact.Compatibility); err != nil {
		return Artifact{}, 0, err
	}
	if err := decodeJSON(dependencies, &artifact.Dependencies); err != nil {
		return Artifact{}, 0, err
	}
	if err := decodeJSON(tags, &artifact.Tags); err != nil {
		return Artifact{}, 0, err
	}
	if err := decodeJSON(hitSignals, &artifact.HitSignals); err != nil {
		return Artifact{}, 0, err
	}
	if err := decodeJSON(manifestSummary, &artifact.ManifestSummary); err != nil {
		return Artifact{}, 0, err
	}
	return artifact, lexicalRank, nil
}

func (s *Store) upsertArtifactSearchDocTx(ctx context.Context, tx *sql.Tx, artifact Artifact) error {
	if s == nil || !s.enableFTS {
		return nil
	}
	doc := buildArtifactSearchDocument(artifact)
	if _, err := tx.ExecContext(ctx, `DELETE FROM artifacts_fts WHERE artifact_id = ?`, artifact.ID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO artifacts_fts (
		artifact_id, name, summary, description_md, publisher, kind,
		tags_text, hit_signals_text, use_case_text, search_text
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		doc["artifact_id"], doc["name"], doc["summary"], doc["description_md"],
		doc["publisher"], doc["kind"], doc["tags_text"], doc["hit_signals_text"],
		doc["use_case_text"], doc["search_text"],
	)
	return err
}

func (s *Store) rebuildArtifactSearchIndex(ctx context.Context) error {
	if s == nil || !s.enableFTS {
		return nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT
		id, kind, name, summary, description_md, latest_version, source, publisher,
		risk_level, trust_level, permissions_json, compatibility_json, dependencies_json,
		icon_url, tags_json, hit_signals_json, score, updated_at, manifest_summary_json
		FROM artifacts`)
	if err != nil {
		return err
	}
	artifacts := make([]Artifact, 0, 64)
	for rows.Next() {
		artifact, scanErr := scanArtifact(rows)
		if scanErr != nil {
			_ = rows.Close()
			return scanErr
		}
		artifacts = append(artifacts, artifact)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.ExecContext(ctx, `DELETE FROM artifacts_fts`); err != nil {
		return err
	}
	for _, artifact := range artifacts {
		if err = s.upsertArtifactSearchDocTx(ctx, tx, artifact); err != nil {
			return err
		}
	}
	err = tx.Commit()
	return err
}

func fallbackArtifactLess(left, right Artifact) bool {
	if left.Kind != right.Kind {
		return left.Kind < right.Kind
	}
	return strings.ToLower(left.ID) < strings.ToLower(right.ID)
}

func containsFold(values []string, want string) bool {
	want = strings.ToLower(strings.TrimSpace(want))
	if want == "" {
		return true
	}
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), want) {
			return true
		}
	}
	return false
}

func compatibleWithClient(compatibility Compatibility, req ResolveRequest) bool {
	if req.ClientEnv.OS != "" && len(compatibility.OS) > 0 && !containsString(compatibility.OS, req.ClientEnv.OS) {
		return false
	}
	if req.ClientEnv.Arch != "" && len(compatibility.Arch) > 0 && !containsString(compatibility.Arch, req.ClientEnv.Arch) {
		return false
	}
	return true
}

func validateArtifact(artifact Artifact) error {
	if artifact.ID == "" || artifact.Name == "" || artifact.LatestVersion == "" {
		return fmt.Errorf("artifact id, name, and latest_version are required")
	}
	switch artifact.Kind {
	case ArtifactKindAgent, ArtifactKindSkill, ArtifactKindCLI:
		return nil
	default:
		return ErrInvalidArtifactKind
	}
}

func encodeJSON(v any) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func decodeJSON(data string, dst any) error {
	if strings.TrimSpace(data) == "" {
		data = "null"
	}
	return json.Unmarshal([]byte(data), dst)
}

func containsString(items []string, item string) bool {
	for _, current := range items {
		if current == item {
			return true
		}
	}
	return false
}
