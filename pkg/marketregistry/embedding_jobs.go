package marketregistry

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/vec"
)

func (s *Store) queueArtifactEmbeddingJobTx(ctx context.Context, tx *sql.Tx, artifact Artifact) error {
	if s == nil || tx == nil {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	embeddingText := buildArtifactEmbeddingText(artifact)
	model := ""
	if s.vector != nil {
		model = s.vector.artifactModel()
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO artifact_embeddings (
		artifact_id, model, vector_json, vector_dim, embedding_text, status, error, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(artifact_id) DO UPDATE SET
		model = excluded.model,
		embedding_text = excluded.embedding_text,
		status = excluded.status,
		error = excluded.error,
		updated_at = excluded.updated_at`,
		artifact.ID, model, "[]", 0, embeddingText, artifactEmbeddingStatusPending, "", now); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO artifact_embedding_jobs (
		artifact_id, job_type, status, attempt_count, error, created_at, updated_at
	) VALUES (?, ?, ?, 0, '', ?, ?)
	ON CONFLICT(artifact_id, job_type) DO UPDATE SET
		status = excluded.status,
		attempt_count = 0,
		error = excluded.error,
		updated_at = excluded.updated_at`,
		artifact.ID, artifactEmbeddingJobType, artifactEmbeddingJobQueued, now, now)
	return err
}

func (s *Store) claimNextArtifactEmbeddingJob(ctx context.Context) (*ArtifactEmbeddingJob, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	row := tx.QueryRowContext(ctx, `SELECT id, artifact_id, job_type, status, attempt_count, error, created_at, updated_at
		FROM artifact_embedding_jobs
		WHERE job_type = ? AND status IN (?, ?)
		AND attempt_count < ?
		ORDER BY updated_at ASC, id ASC
		LIMIT 1`, artifactEmbeddingJobType, artifactEmbeddingJobQueued, artifactEmbeddingJobFailed, s.vector.cfg.MaxJobAttempts)
	var job ArtifactEmbeddingJob
	scanErr := row.Scan(&job.ID, &job.ArtifactID, &job.JobType, &job.Status, &job.AttemptCount, &job.Error, &job.CreatedAt, &job.UpdatedAt)
	if errors.Is(scanErr, sql.ErrNoRows) {
		_ = tx.Rollback()
		return nil, nil
	}
	if scanErr != nil {
		return nil, scanErr
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err = tx.ExecContext(ctx, `UPDATE artifact_embedding_jobs
		SET status = ?, attempt_count = attempt_count + 1, updated_at = ?
		WHERE id = ?`, artifactEmbeddingJobRunning, now, job.ID); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE artifact_embeddings
		SET status = ?, updated_at = ?
		WHERE artifact_id = ?`, artifactEmbeddingStatusPending, now, job.ArtifactID); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	job.Status = artifactEmbeddingJobRunning
	job.AttemptCount++
	job.UpdatedAt = now
	return &job, nil
}

func (s *Store) markArtifactEmbeddingJobSucceeded(ctx context.Context, job ArtifactEmbeddingJob, artifact Artifact, vectorData []float32) error {
	vectorJSON, err := encodeVector(vectorData)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if _, err = tx.ExecContext(ctx, `UPDATE artifact_embedding_jobs
		SET status = ?, error = '', updated_at = ?
		WHERE id = ?`, artifactEmbeddingJobSucceeded, now, job.ID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE artifact_embeddings
		SET model = ?, vector_json = ?, vector_dim = ?, embedding_text = ?, status = ?, error = '', updated_at = ?
		WHERE artifact_id = ?`,
		s.vector.artifactModel(), vectorJSON, len(vectorData), buildArtifactEmbeddingText(artifact), artifactEmbeddingStatusReady, now, artifact.ID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) markArtifactEmbeddingJobFailed(ctx context.Context, job ArtifactEmbeddingJob, reason string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	jobStatus := artifactEmbeddingJobFailed
	embeddingStatus := artifactEmbeddingStatusPending
	if s != nil && s.vector != nil && job.AttemptCount >= s.vector.cfg.MaxJobAttempts {
		embeddingStatus = artifactEmbeddingStatusFailed
	}
	_, err := s.db.ExecContext(ctx, `UPDATE artifact_embedding_jobs
		SET status = ?, error = ?, updated_at = ?
		WHERE id = ?`, jobStatus, reason, now, job.ID)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE artifact_embeddings
		SET status = ?, error = ?, updated_at = ?
		WHERE artifact_id = ?`, embeddingStatus, reason, now, job.ArtifactID)
	return err
}

func (s *Store) loadArtifactEmbedding(ctx context.Context, artifactID string) (*ArtifactEmbedding, error) {
	row := s.db.QueryRowContext(ctx, `SELECT artifact_id, model, vector_json, vector_dim, embedding_text, status, error, updated_at
		FROM artifact_embeddings WHERE artifact_id = ?`, strings.TrimSpace(artifactID))
	var item ArtifactEmbedding
	var vectorJSON string
	if err := row.Scan(&item.ArtifactID, &item.Model, &vectorJSON, &item.VectorDim, &item.EmbeddingText, &item.Status, &item.Error, &item.LastUpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	vectorData, err := decodeVector(vectorJSON)
	if err != nil {
		return nil, err
	}
	item.Vector = vectorData
	return &item, nil
}

func (s *Store) ListArtifactEmbeddingJobs(ctx context.Context, status string, limit int) (ArtifactEmbeddingJobList, error) {
	if limit <= 0 {
		limit = 100
	}
	status = strings.TrimSpace(status)
	query := `SELECT id, artifact_id, job_type, status, attempt_count, error, created_at, updated_at
		FROM artifact_embedding_jobs`
	args := make([]any, 0, 2)
	if status != "" {
		query += ` WHERE status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY updated_at DESC, id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return ArtifactEmbeddingJobList{}, err
	}
	defer rows.Close()
	items := make([]ArtifactEmbeddingJob, 0, limit)
	for rows.Next() {
		var item ArtifactEmbeddingJob
		if err := rows.Scan(&item.ID, &item.ArtifactID, &item.JobType, &item.Status, &item.AttemptCount, &item.Error, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return ArtifactEmbeddingJobList{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return ArtifactEmbeddingJobList{}, err
	}
	return ArtifactEmbeddingJobList{Items: items, Total: len(items)}, nil
}

func (s *Store) ListArtifactEmbeddings(ctx context.Context, status string, limit int) (ArtifactEmbeddingList, error) {
	if limit <= 0 {
		limit = 100
	}
	status = strings.TrimSpace(status)
	query := `SELECT artifact_id, model, vector_json, vector_dim, embedding_text, status, error, updated_at
		FROM artifact_embeddings`
	args := make([]any, 0, 2)
	if status != "" {
		query += ` WHERE status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY updated_at DESC, artifact_id ASC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return ArtifactEmbeddingList{}, err
	}
	defer rows.Close()
	items := make([]ArtifactEmbedding, 0, limit)
	for rows.Next() {
		var item ArtifactEmbedding
		var vectorJSON string
		if err := rows.Scan(&item.ArtifactID, &item.Model, &vectorJSON, &item.VectorDim, &item.EmbeddingText, &item.Status, &item.Error, &item.LastUpdatedAt); err != nil {
			return ArtifactEmbeddingList{}, err
		}
		vectorData, err := decodeVector(vectorJSON)
		if err != nil {
			return ArtifactEmbeddingList{}, err
		}
		item.Vector = vectorData
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return ArtifactEmbeddingList{}, err
	}
	return ArtifactEmbeddingList{Items: items, Total: len(items)}, nil
}

func (s *Store) EmbeddingAdminStats(ctx context.Context) (embeddingAdminStats, error) {
	var stats embeddingAdminStats
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM artifact_embedding_jobs WHERE status = ?`, artifactEmbeddingJobQueued).Scan(&stats.QueuedJobs); err != nil {
		return embeddingAdminStats{}, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM artifact_embedding_jobs WHERE status = ?`, artifactEmbeddingJobFailed).Scan(&stats.FailedJobs); err != nil {
		return embeddingAdminStats{}, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM artifact_embeddings WHERE status = ?`, artifactEmbeddingStatusReady).Scan(&stats.ReadyEmbeddings); err != nil {
		return embeddingAdminStats{}, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM artifact_embeddings WHERE status = ?`, artifactEmbeddingStatusPending).Scan(&stats.PendingEmbedding); err != nil {
		return embeddingAdminStats{}, err
	}
	return stats, nil
}

func (s *Store) loadEmbeddingsForArtifacts(ctx context.Context, artifactIDs []string) (map[string]ArtifactEmbedding, error) {
	if len(artifactIDs) == 0 {
		return map[string]ArtifactEmbedding{}, nil
	}
	placeholders := make([]string, 0, len(artifactIDs))
	args := make([]any, 0, len(artifactIDs))
	for _, artifactID := range artifactIDs {
		artifactID = strings.TrimSpace(artifactID)
		if artifactID == "" {
			continue
		}
		placeholders = append(placeholders, "?")
		args = append(args, artifactID)
	}
	if len(placeholders) == 0 {
		return map[string]ArtifactEmbedding{}, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT artifact_id, model, vector_json, vector_dim, embedding_text, status, error, updated_at
		FROM artifact_embeddings WHERE artifact_id IN (`+strings.Join(placeholders, ",")+`) AND status = ? AND model = ?`,
		append(args, artifactEmbeddingStatusReady, s.vector.artifactModel())...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]ArtifactEmbedding, len(artifactIDs))
	for rows.Next() {
		var item ArtifactEmbedding
		var vectorJSON string
		if err := rows.Scan(&item.ArtifactID, &item.Model, &vectorJSON, &item.VectorDim, &item.EmbeddingText, &item.Status, &item.Error, &item.LastUpdatedAt); err != nil {
			return nil, err
		}
		vectorData, err := decodeVector(vectorJSON)
		if err != nil {
			return nil, err
		}
		item.Vector = vectorData
		out[item.ArtifactID] = item
	}
	return out, rows.Err()
}

func (s *Server) runEmbeddingWorker(ctx context.Context) {
	ticker := time.NewTicker(s.vector.cfg.WorkerPoll)
	defer ticker.Stop()
	for {
		s.processOneEmbeddingJob(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		default:
			time.Sleep(200 * time.Millisecond)
		}
	}
}

func (s *Server) processOneEmbeddingJob(ctx context.Context) {
	if s == nil || s.vector == nil || !s.vector.enabled() {
		return
	}
	timer := s.metrics.newTimer("anyclaw_registry_embedding_job_duration_seconds", "Embedding job duration")
	job, err := s.store.claimNextArtifactEmbeddingJob(ctx)
	if err != nil || job == nil {
		return
	}
	artifact, err := s.store.Get(ctx, job.ArtifactID)
	if err != nil {
		if timer != nil {
			timer.Stop()
		}
		s.metrics.recordEmbeddingJob(false)
		_ = s.store.markArtifactEmbeddingJobFailed(ctx, *job, err.Error())
		return
	}
	embedCtx, cancel := context.WithTimeout(ctx, s.vector.cfg.QueryTimeout)
	defer cancel()
	vectorData, err := s.vector.artifactEmbed.Embed(embedCtx, buildArtifactEmbeddingText(artifact))
	if err != nil {
		if timer != nil {
			timer.Stop()
		}
		s.metrics.recordEmbeddingJob(false)
		_ = s.store.markArtifactEmbeddingJobFailed(ctx, *job, err.Error())
		return
	}
	if len(vectorData) == 0 {
		if timer != nil {
			timer.Stop()
		}
		s.metrics.recordEmbeddingJob(false)
		_ = s.store.markArtifactEmbeddingJobFailed(ctx, *job, "empty embedding")
		return
	}
	if timer != nil {
		timer.Stop()
	}
	s.metrics.recordEmbeddingJob(true)
	_ = s.store.markArtifactEmbeddingJobSucceeded(ctx, *job, artifact, vectorData)
}

func (s *Store) searchHybrid(ctx context.Context, filter SearchFilter, lexicalCandidates []searchCandidate, total int) ([]searchCandidate, int, RetrievalMeta) {
	meta := RetrievalMeta{
		SearchMode:    SearchModeLexicalFallback,
		VectorApplied: false,
		CandidateCounts: &CandidateCounts{
			Lexical: total,
			Vector:  0,
			Merged:  total,
		},
	}
	if s == nil || s.vector == nil || !s.vector.enabled() {
		meta.VectorFallbackReason = "disabled"
		if s != nil && s.vector != nil && s.vector.metrics != nil {
			s.vector.metrics.recordVectorFallback("disabled")
		}
		return lexicalCandidates, total, meta
	}
	searchTimer := s.vector.metrics.newTimer("anyclaw_registry_vector_search_duration_seconds", "Vector search duration")
	defer func() {
		if searchTimer != nil {
			searchTimer.Stop()
		}
	}()
	embedCtx, cancel := context.WithTimeout(ctx, s.vector.cfg.QueryTimeout)
	defer cancel()
	queryVector, err := s.vector.embedQuery(embedCtx, strings.TrimSpace(filter.Query))
	if err != nil || len(queryVector) == 0 {
		meta.VectorFallbackReason = "provider_error"
		if s.vector.metrics != nil {
			s.vector.metrics.recordVectorFallback("provider_error")
		}
		return lexicalCandidates, total, meta
	}

	vectorPool, err := s.listVectorPoolCandidates(ctx, filter)
	if err != nil {
		meta.VectorFallbackReason = "embedding_unavailable"
		if s.vector.metrics != nil {
			s.vector.metrics.recordVectorFallback("embedding_unavailable")
		}
		return lexicalCandidates, total, meta
	}
	vectorCandidates, err := s.rankVectorCandidates(ctx, vectorPool, queryVector)
	if err != nil {
		meta.VectorFallbackReason = "embedding_unavailable"
		if s.vector.metrics != nil {
			s.vector.metrics.recordVectorFallback("embedding_unavailable")
		}
		return lexicalCandidates, total, meta
	}
	if len(vectorCandidates) == 0 {
		meta.VectorFallbackReason = "empty_index"
		if s.vector.metrics != nil {
			s.vector.metrics.recordVectorFallback("empty_index")
		}
		return lexicalCandidates, total, meta
	}

	merged := mergeHybridCandidates(lexicalCandidates, vectorCandidates)
	sortSearchCandidates(merged, filter.Sort)
	meta.SearchMode = SearchModeHybrid
	meta.VectorApplied = true
	meta.VectorFallbackReason = ""
	meta.CandidateCounts = &CandidateCounts{
		Lexical: total,
		Vector:  len(vectorCandidates),
		Merged:  len(merged),
	}
	mergedTotal := total
	if len(merged) > mergedTotal {
		mergedTotal = len(merged)
	}
	if s.vector.metrics != nil {
		s.vector.metrics.recordVectorApplied()
	}
	return merged, mergedTotal, meta
}

func (r *vectorRuntime) embedQuery(ctx context.Context, query string) ([]float32, error) {
	if r == nil || !r.enabled() {
		return nil, fmt.Errorf("vector runtime disabled")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("query is empty")
	}
	if r.queryCache != nil {
		if cached, ok := r.queryCache.Get(query); ok {
			if r.metrics != nil {
				r.metrics.recordQueryCacheHit()
			}
			return cached, nil
		}
	}
	if r.metrics != nil {
		r.metrics.recordQueryCacheMiss()
	}
	timer := r.metrics.newTimer("anyclaw_registry_query_embedding_duration_seconds", "Query embedding duration")
	embeddingData, err := r.queryEmbed.Embed(ctx, query)
	if timer != nil {
		timer.Stop()
	}
	if err != nil {
		return nil, err
	}
	if r.queryCache != nil && len(embeddingData) > 0 {
		r.queryCache.Set(query, embeddingData)
	}
	return embeddingData, nil
}

func (s *Store) listVectorPoolCandidates(ctx context.Context, filter SearchFilter) ([]searchCandidate, error) {
	vectorFilter := filter
	vectorFilter.Offset = 0
	if s != nil && s.vector != nil && s.vector.cfg.HybridCandidate > 0 {
		vectorFilter.Limit = s.vector.cfg.HybridCandidate
	}
	candidates, _, err := s.listStructuredCandidates(ctx, vectorFilter)
	if err != nil {
		return nil, err
	}
	if limit := vectorFilter.Limit; limit > 0 && len(candidates) > limit {
		candidates = candidates[:limit]
	}
	return candidates, nil
}

func (s *Store) rankVectorCandidates(ctx context.Context, lexicalCandidates []searchCandidate, queryVector []float32) ([]searchCandidate, error) {
	artifactIDs := make([]string, 0, len(lexicalCandidates))
	for _, candidate := range lexicalCandidates {
		artifactIDs = append(artifactIDs, candidate.ID)
	}
	embeddings, err := s.loadEmbeddingsForArtifacts(ctx, artifactIDs)
	if err != nil {
		return nil, err
	}
	vectorCandidates := make([]searchCandidate, 0, len(embeddings))
	for _, candidate := range lexicalCandidates {
		item, ok := embeddings[candidate.ID]
		if !ok || len(item.Vector) == 0 {
			continue
		}
		score := normalizeCosineScore(vec.CosineSimilarity(queryVector, item.Vector))
		candidate.VectorScore = &score
		candidate.FinalScore = hybridFinalScore(candidate.Artifact, score)
		candidate.Score = candidate.FinalScore
		candidate.MatchSignals = appendUniqueSignals(candidate.MatchSignals, "vector")
		vectorCandidates = append(vectorCandidates, candidate)
	}
	sort.SliceStable(vectorCandidates, func(i, j int) bool {
		left := valueOrZero(vectorCandidates[i].VectorScore)
		right := valueOrZero(vectorCandidates[j].VectorScore)
		if left == right {
			return vectorCandidates[i].FinalScore > vectorCandidates[j].FinalScore
		}
		return left > right
	})
	if limit := s.vector.cfg.HybridTopK; limit > 0 && len(vectorCandidates) > limit {
		vectorCandidates = vectorCandidates[:limit]
	}
	return vectorCandidates, nil
}

func mergeHybridCandidates(lexicalCandidates, vectorCandidates []searchCandidate) []searchCandidate {
	merged := make(map[string]searchCandidate, len(lexicalCandidates)+len(vectorCandidates))
	for _, candidate := range lexicalCandidates {
		merged[candidate.ID] = candidate
	}
	for _, candidate := range vectorCandidates {
		if existing, ok := merged[candidate.ID]; ok {
			if candidate.VectorScore != nil {
				existing.VectorScore = candidate.VectorScore
			}
			existing.FinalScore = hybridFinalScore(existing.Artifact, valueOrZero(candidate.VectorScore))
			existing.Score = existing.FinalScore
			existing.MatchSignals = appendUniqueSignals(existing.MatchSignals, "vector")
			merged[candidate.ID] = existing
			continue
		}
		merged[candidate.ID] = candidate
	}
	out := make([]searchCandidate, 0, len(merged))
	for _, candidate := range merged {
		out = append(out, candidate)
	}
	return out
}

func hybridFinalScore(artifact Artifact, vectorScore float64) float64 {
	base := artifact.FinalScore
	if base <= 0 {
		base = finalScore(artifact)
	}
	if vectorScore <= 0 {
		return clampScore(base)
	}
	return clampScore(base*0.7 + vectorScore*0.3)
}

func normalizeCosineScore(score float64) float64 {
	return clampScore((score + 1.0) / 2.0)
}

func valueOrZero(score *float64) float64 {
	if score == nil {
		return 0
	}
	return *score
}

func appendUniqueSignals(base []string, values ...string) []string {
	seen := make(map[string]struct{}, len(base)+len(values))
	out := make([]string, 0, len(base)+len(values))
	for _, item := range append(base, values...) {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		key := strings.ToLower(item)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

func encodeVector(vectorData []float32) (string, error) {
	data, err := json.Marshal(vectorData)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func decodeVector(raw string) ([]float32, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var vectorData []float32
	if err := json.Unmarshal([]byte(raw), &vectorData); err != nil {
		return nil, fmt.Errorf("decode vector: %w", err)
	}
	return vectorData, nil
}
