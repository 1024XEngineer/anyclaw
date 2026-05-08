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
	db *sql.DB
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
	store := &Store{db: db}
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
	}
	for _, stmt := range statements {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE artifact_versions ADD COLUMN signature TEXT NOT NULL DEFAULT ''`)
	return nil
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
	err = tx.Commit()
	return err
}

func (s *Store) List(ctx context.Context, filter SearchFilter) (ListResult, error) {
	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}

	rows, err := s.db.QueryContext(ctx, `SELECT
		id, kind, name, summary, description_md, latest_version, source, publisher,
		risk_level, trust_level, permissions_json, compatibility_json, dependencies_json,
		icon_url, tags_json, hit_signals_json, score, updated_at, manifest_summary_json
		FROM artifacts`)
	if err != nil {
		return ListResult{}, err
	}
	defer rows.Close()

	var all []Artifact
	for rows.Next() {
		artifact, err := scanArtifact(rows)
		if err != nil {
			return ListResult{}, err
		}
		if matchesArtifact(artifact, filter) {
			all = append(all, artifact)
		}
	}
	if err := rows.Err(); err != nil {
		return ListResult{}, err
	}

	sortArtifacts(all, filter.Sort)

	total := len(all)
	if filter.Offset >= len(all) {
		all = nil
	} else {
		all = all[filter.Offset:]
		if len(all) > filter.Limit {
			all = all[:filter.Limit]
		}
	}
	return ListResult{
		Items:  all,
		Total:  total,
		Limit:  filter.Limit,
		Offset: filter.Offset,
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

func matchesArtifact(artifact Artifact, filter SearchFilter) bool {
	if filter.Kind != "" && artifact.Kind != filter.Kind {
		return false
	}
	if filter.Source != "" && !strings.EqualFold(artifact.Source, filter.Source) {
		return false
	}
	if filter.Risk != "" && !strings.EqualFold(artifact.RiskLevel, filter.Risk) {
		return false
	}
	if filter.Trust != "" && !strings.EqualFold(artifact.TrustLevel, filter.Trust) {
		return false
	}
	if filter.Tag != "" && !containsFold(artifact.Tags, filter.Tag) {
		return false
	}
	if filter.Permission != "" && !containsFold(artifact.Permissions, filter.Permission) {
		return false
	}
	if filter.Publisher != "" && !strings.Contains(strings.ToLower(artifact.Publisher), strings.ToLower(strings.TrimSpace(filter.Publisher))) {
		return false
	}
	if filter.OS != "" && len(artifact.Compatibility.OS) > 0 && !containsFold(artifact.Compatibility.OS, filter.OS) {
		return false
	}
	if filter.Arch != "" && len(artifact.Compatibility.Arch) > 0 && !containsFold(artifact.Compatibility.Arch, filter.Arch) {
		return false
	}
	query := strings.ToLower(strings.TrimSpace(filter.Query))
	if query == "" {
		return true
	}
	fields := []string{
		artifact.ID,
		artifact.Name,
		artifact.Summary,
		artifact.DescriptionMD,
		artifact.Publisher,
		strings.Join(artifact.Tags, " "),
		strings.Join(artifact.HitSignals, " "),
	}
	return strings.Contains(strings.ToLower(strings.Join(fields, " ")), query)
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
			if items[i].Score == items[j].Score {
				if items[i].UpdatedAt == items[j].UpdatedAt {
					return fallbackArtifactLess(items[i], items[j])
				}
				return items[i].UpdatedAt > items[j].UpdatedAt
			}
			return items[i].Score > items[j].Score
		}
	})
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
