package index

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/embedding"
	"github.com/1024XEngineer/anyclaw/pkg/sqlite"
	"github.com/1024XEngineer/anyclaw/pkg/vec"
)

type Status string

const (
	StatusPending    Status = "pending"
	StatusCreating   Status = "creating"
	StatusReady      Status = "ready"
	StatusUpdating   Status = "updating"
	StatusRebuilding Status = "rebuilding"
	StatusError      Status = "error"
	StatusDeleting   Status = "deleting"
	StatusDeleted    Status = "deleted"
)

type Config struct {
	Name       string
	Dimensions int
	Distance   vec.DistanceMetric
	Metadata   []string
	AuxColumns []string
	TableName  string
}

func (c Config) TableNameOrDefault() string {
	if c.TableName != "" {
		return c.TableName
	}
	return "vec_" + c.Name
}

type IndexInfo struct {
	Name        string    `json:"name"`
	TableName   string    `json:"table_name"`
	Dimensions  int       `json:"dimensions"`
	Distance    string    `json:"distance"`
	Metadata    []string  `json:"metadata"`
	AuxColumns  []string  `json:"aux_columns"`
	Status      Status    `json:"status"`
	VectorCount int64     `json:"vector_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Error       string    `json:"error,omitempty"`
}

type Progress struct {
	Total     int
	Processed int
	Failed    int
	Elapsed   time.Duration
	ETA       time.Duration
	CurrentID any
	Message   string
	Done      bool
}

type ProgressFunc func(p Progress)

type Option func(*IndexManager)

func WithVectorDir(path string) Option {
	return func(im *IndexManager) {
		im.vecDir = path
	}
}

type IndexManager struct {
	db        *sql.DB
	embedder  embedding.Provider
	indexes   map[string]*IndexInfo
	metaTable string
	vecDir    string
	mu        sync.RWMutex
}

func NewIndexManager(db *sql.DB, embedder embedding.Provider, opts ...Option) *IndexManager {
	im := &IndexManager{
		db:        db,
		embedder:  embedder,
		indexes:   make(map[string]*IndexInfo),
		metaTable: "vector_index_meta",
	}

	for _, opt := range opts {
		opt(im)
	}

	return im
}

func (im *IndexManager) Init(ctx context.Context) error {
	if err := im.createMetaTable(ctx); err != nil {
		return fmt.Errorf("create meta table: %w", err)
	}
	if err := im.loadIndexes(ctx); err != nil {
		return fmt.Errorf("load indexes: %w", err)
	}
	return nil
}

func (im *IndexManager) createMetaTable(ctx context.Context) error {
	query := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		name TEXT PRIMARY KEY,
		table_name TEXT NOT NULL,
		dimensions INTEGER NOT NULL,
		distance TEXT NOT NULL,
		metadata TEXT,
		aux_columns TEXT,
		status TEXT NOT NULL,
		vector_count INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		error TEXT
	)`, im.metaTable)

	_, err := im.db.ExecContext(ctx, query)
	return err
}

func (im *IndexManager) loadIndexes(ctx context.Context) error {
	rows, err := im.db.QueryContext(ctx, fmt.Sprintf(
		"SELECT name, table_name, dimensions, distance, metadata, aux_columns, status, vector_count, created_at, updated_at, error FROM %s",
		im.metaTable,
	))
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var info IndexInfo
		var metaJSON, auxJSON, statusStr, createdAt, updatedAt string
		var errStr sql.NullString

		if err := rows.Scan(&info.Name, &info.TableName, &info.Dimensions, &info.Distance,
			&metaJSON, &auxJSON, &statusStr, &info.VectorCount, &createdAt, &updatedAt, &errStr); err != nil {
			continue
		}

		info.Status = Status(statusStr)
		if metaJSON != "" {
			_ = json.Unmarshal([]byte(metaJSON), &info.Metadata)
		}
		if auxJSON != "" {
			_ = json.Unmarshal([]byte(auxJSON), &info.AuxColumns)
		}
		if errStr.Valid {
			info.Error = errStr.String
		}
		info.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		info.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

		im.indexes[info.Name] = &info
	}

	return nil
}

func (im *IndexManager) Create(ctx context.Context, cfg Config) (*IndexInfo, error) {
	im.mu.Lock()
	defer im.mu.Unlock()

	if _, exists := im.indexes[cfg.Name]; exists {
		return nil, fmt.Errorf("index %q already exists", cfg.Name)
	}

	tableName := cfg.TableNameOrDefault()
	distance := cfg.Distance
	if distance == "" {
		distance = vec.DistanceCosine
	}

	info := &IndexInfo{
		Name:       cfg.Name,
		TableName:  tableName,
		Dimensions: cfg.Dimensions,
		Distance:   string(distance),
		Metadata:   append([]string(nil), cfg.Metadata...),
		AuxColumns: append([]string(nil), cfg.AuxColumns...),
		Status:     StatusCreating,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	if err := im.saveIndexMeta(ctx, info); err != nil {
		return nil, err
	}

	vs := im.newVecStore(info)
	if err := vs.Init(ctx); err != nil {
		if cleanupErr := im.deleteIndexMeta(ctx, info.Name); cleanupErr != nil {
			return nil, fmt.Errorf("create vector collection: %w (cleanup index metadata: %v)", err, cleanupErr)
		}
		return nil, fmt.Errorf("create vector collection: %w", err)
	}

	info.Status = StatusReady
	info.Error = ""
	if err := im.saveIndexMeta(ctx, info); err != nil {
		return nil, err
	}
	im.indexes[cfg.Name] = info

	return info, nil
}

func (im *IndexManager) Update(ctx context.Context, name string, cfg Config) (*IndexInfo, error) {
	im.mu.Lock()
	defer im.mu.Unlock()

	info, exists := im.indexes[name]
	if !exists {
		return nil, fmt.Errorf("index %q not found", name)
	}

	info.Status = StatusUpdating
	info.UpdatedAt = time.Now()
	_ = im.saveIndexMeta(ctx, info)

	if cfg.Dimensions != 0 && cfg.Dimensions != info.Dimensions {
		info.Status = StatusError
		info.Error = "cannot change dimensions of existing index"
		_ = im.saveIndexMeta(ctx, info)
		return nil, fmt.Errorf("cannot change dimensions: existing=%d, requested=%d", info.Dimensions, cfg.Dimensions)
	}

	if cfg.Distance != "" && cfg.Distance != vec.DistanceMetric(info.Distance) {
		info.Status = StatusError
		info.Error = "cannot change distance metric of existing index"
		_ = im.saveIndexMeta(ctx, info)
		return nil, fmt.Errorf("cannot change distance metric")
	}

	if len(cfg.Metadata) > 0 {
		info.Metadata = append([]string(nil), cfg.Metadata...)
	}
	if len(cfg.AuxColumns) > 0 {
		info.AuxColumns = append([]string(nil), cfg.AuxColumns...)
	}

	info.Status = StatusReady
	info.Error = ""
	info.UpdatedAt = time.Now()
	if err := im.saveIndexMeta(ctx, info); err != nil {
		return nil, err
	}
	im.indexes[name] = info

	return info, nil
}

func (im *IndexManager) Delete(ctx context.Context, name string) error {
	im.mu.Lock()
	defer im.mu.Unlock()

	info, exists := im.indexes[name]
	if !exists {
		return fmt.Errorf("index %q not found", name)
	}

	info.Status = StatusDeleting
	info.UpdatedAt = time.Now()
	_ = im.saveIndexMeta(ctx, info)

	if err := im.newVecStore(info).Drop(ctx); err != nil {
		info.Status = StatusError
		info.Error = err.Error()
		_ = im.saveIndexMeta(ctx, info)
		return err
	}

	if err := im.deleteIndexMeta(ctx, name); err != nil {
		return fmt.Errorf("delete meta: %w", err)
	}

	delete(im.indexes, name)
	return nil
}

func (im *IndexManager) Get(name string) (*IndexInfo, error) {
	im.mu.RLock()
	info, exists := im.indexes[name]
	if !exists {
		im.mu.RUnlock()
		return nil, fmt.Errorf("index %q not found", name)
	}
	cloned := cloneIndexInfo(info)
	im.mu.RUnlock()

	if count, err := im.countVectors(context.Background(), cloned); err == nil {
		cloned.VectorCount = count
	}

	return cloned, nil
}

func (im *IndexManager) List() []*IndexInfo {
	im.mu.RLock()
	defer im.mu.RUnlock()

	result := make([]*IndexInfo, 0, len(im.indexes))
	for _, info := range im.indexes {
		result = append(result, cloneIndexInfo(info))
	}
	return result
}

func (im *IndexManager) Index(ctx context.Context, indexName string, items []IndexItem, progress ProgressFunc) (*IndexResult, error) {
	im.mu.RLock()
	info, exists := im.indexes[indexName]
	im.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("index %q not found", indexName)
	}
	if info.Status != StatusReady {
		return nil, fmt.Errorf("index %q is not ready (status: %s)", indexName, info.Status)
	}

	im.mu.Lock()
	info.Status = StatusUpdating
	info.Error = ""
	_ = im.saveIndexMeta(ctx, info)
	im.mu.Unlock()

	start := time.Now()
	vs := im.newVecStore(info)

	result := &IndexResult{
		IndexName: indexName,
		Total:     len(items),
		StartedAt: start,
	}

	batchSize := 100
	for i := 0; i < len(items); i += batchSize {
		end := i + batchSize
		if end > len(items) {
			end = len(items)
		}
		batch := items[i:end]

		vecItems := make([]vec.VecItem, 0, len(batch))
		pendingOffsets := make([]int, 0, len(batch))
		pendingItems := make([]IndexItem, 0, len(batch))
		pendingTexts := make([]string, 0, len(batch))
		for offset, item := range batch {
			if item.Vector != nil {
				vecItems = append(vecItems, vec.VecItem{
					ID:       item.ID,
					Vector:   item.Vector,
					Metadata: item.Metadata,
				})
				continue
			}

			if im.embedder == nil || item.Text == "" {
				result.Failed++
				continue
			}

			pendingOffsets = append(pendingOffsets, offset)
			pendingItems = append(pendingItems, item)
			pendingTexts = append(pendingTexts, item.Text)
		}

		if len(pendingTexts) > 0 {
			embeddings, err := im.embedder.EmbedBatch(ctx, pendingTexts)
			if err != nil {
				for idx, item := range pendingItems {
					result.Failed++
					if progress != nil {
						progress(Progress{
							Total:     len(items),
							Processed: i + pendingOffsets[idx] + 1,
							Failed:    result.Failed,
							Elapsed:   time.Since(start),
							CurrentID: item.ID,
							Message:   fmt.Sprintf("embed failed: %v", err),
						})
					}
				}
			} else if len(embeddings) != len(pendingItems) {
				err := fmt.Errorf("embed batch returned %d embeddings for %d texts", len(embeddings), len(pendingItems))
				for idx, item := range pendingItems {
					result.Failed++
					if progress != nil {
						progress(Progress{
							Total:     len(items),
							Processed: i + pendingOffsets[idx] + 1,
							Failed:    result.Failed,
							Elapsed:   time.Since(start),
							CurrentID: item.ID,
							Message:   fmt.Sprintf("embed failed: %v", err),
						})
					}
				}
			} else {
				for idx, item := range pendingItems {
					vecItems = append(vecItems, vec.VecItem{
						ID:       item.ID,
						Vector:   embeddings[idx],
						Metadata: item.Metadata,
					})
				}
			}
		}

		if len(vecItems) > 0 {
			if err := vs.InsertBatch(ctx, vecItems); err != nil {
				result.Failed += len(vecItems)
				result.CompletedAt = time.Now()
				result.Duration = result.CompletedAt.Sub(result.StartedAt)

				im.mu.Lock()
				info.Status = StatusError
				info.Error = err.Error()
				info.UpdatedAt = result.CompletedAt
				count, _ := im.countVectors(ctx, info)
				info.VectorCount = count
				_ = im.saveIndexMeta(ctx, info)
				im.mu.Unlock()

				return result, fmt.Errorf("insert vectors: %w", err)
			}
			result.Indexed += len(vecItems)
		}

		processed := end
		elapsed := time.Since(start)
		eta := time.Duration(0)
		if processed > 0 && elapsed > 0 {
			rate := float64(processed) / elapsed.Seconds()
			if rate > 0 {
				remaining := len(items) - processed
				eta = time.Duration(float64(remaining)/rate) * time.Second
			}
		}

		if progress != nil && len(batch) > 0 {
			progress(Progress{
				Total:     len(items),
				Processed: processed,
				Failed:    result.Failed,
				Elapsed:   elapsed,
				ETA:       eta,
				CurrentID: batch[len(batch)-1].ID,
				Message:   fmt.Sprintf("indexed %d/%d", processed, len(items)),
			})
		}
	}

	result.CompletedAt = time.Now()
	result.Duration = result.CompletedAt.Sub(result.StartedAt)

	im.mu.Lock()
	info.Status = StatusReady
	info.Error = ""
	info.UpdatedAt = time.Now()
	count, _ := im.countVectors(ctx, info)
	info.VectorCount = count
	_ = im.saveIndexMeta(ctx, info)
	im.mu.Unlock()

	if progress != nil {
		progress(Progress{
			Total:     len(items),
			Processed: len(items),
			Failed:    result.Failed,
			Elapsed:   result.Duration,
			Done:      true,
			Message:   fmt.Sprintf("completed: %d indexed, %d failed", result.Indexed, result.Failed),
		})
	}

	return result, nil
}

func (im *IndexManager) RemoveVectors(ctx context.Context, indexName string, ids []any) (int, error) {
	im.mu.RLock()
	info, exists := im.indexes[indexName]
	im.mu.RUnlock()

	if !exists {
		return 0, fmt.Errorf("index %q not found", indexName)
	}

	vs := im.newVecStore(info)
	removed := 0
	for _, id := range ids {
		before, err := vs.Count(ctx)
		if err != nil {
			return removed, fmt.Errorf("count vectors before delete: %w", err)
		}
		if before == 0 {
			break
		}

		if err := vs.Delete(ctx, id); err != nil {
			return removed, fmt.Errorf("delete vector %v: %w", id, err)
		}

		after, err := vs.Count(ctx)
		if err != nil {
			return removed, fmt.Errorf("count vectors after delete: %w", err)
		}
		if after < before {
			removed++
		}
	}

	im.mu.Lock()
	info.UpdatedAt = time.Now()
	count, _ := im.countVectors(ctx, info)
	info.VectorCount = count
	_ = im.saveIndexMeta(ctx, info)
	im.mu.Unlock()

	return removed, nil
}

func (im *IndexManager) Rebuild(ctx context.Context, indexName string, progress ProgressFunc) (*IndexResult, error) {
	im.mu.RLock()
	info, exists := im.indexes[indexName]
	im.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("index %q not found", indexName)
	}

	im.mu.Lock()
	info.Status = StatusRebuilding
	info.Error = ""
	info.UpdatedAt = time.Now()
	_ = im.saveIndexMeta(ctx, info)
	im.mu.Unlock()

	vs := im.newVecStore(info)
	if err := vs.Drop(ctx); err != nil {
		im.mu.Lock()
		info.Status = StatusError
		info.Error = err.Error()
		_ = im.saveIndexMeta(ctx, info)
		im.mu.Unlock()
		return nil, err
	}
	if err := vs.Init(ctx); err != nil {
		im.mu.Lock()
		info.Status = StatusError
		info.Error = err.Error()
		_ = im.saveIndexMeta(ctx, info)
		im.mu.Unlock()
		return nil, fmt.Errorf("recreate collection: %w", err)
	}

	completedAt := time.Now()

	im.mu.Lock()
	info.Status = StatusReady
	info.Error = ""
	info.UpdatedAt = completedAt
	info.VectorCount = 0
	_ = im.saveIndexMeta(ctx, info)
	im.mu.Unlock()

	if progress != nil {
		progress(Progress{
			Total:   0,
			Done:    true,
			Message: "index rebuilt (empty, re-index needed)",
		})
	}

	return &IndexResult{
		IndexName:   indexName,
		CompletedAt: completedAt,
		Duration:    0,
	}, nil
}

func (im *IndexManager) Search(ctx context.Context, indexName string, queryVector []float32, limit int) ([]vec.VecSearchResult, error) {
	im.mu.RLock()
	info, exists := im.indexes[indexName]
	im.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("index %q not found", indexName)
	}

	return im.newVecStore(info).Search(ctx, queryVector, limit)
}

func (im *IndexManager) SearchByText(ctx context.Context, indexName string, queryText string, limit int) ([]vec.VecSearchResult, error) {
	if im.embedder == nil {
		return nil, fmt.Errorf("no embedder configured")
	}

	queryVector, err := im.embedder.Embed(ctx, queryText)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	return im.Search(ctx, indexName, queryVector, limit)
}

func (im *IndexManager) saveIndexMeta(ctx context.Context, info *IndexInfo) error {
	metaJSON, _ := json.Marshal(info.Metadata)
	auxJSON, _ := json.Marshal(info.AuxColumns)

	_, err := im.db.ExecContext(ctx, fmt.Sprintf(
		`INSERT OR REPLACE INTO %s (name, table_name, dimensions, distance, metadata, aux_columns, status, vector_count, created_at, updated_at, error)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		im.metaTable,
	), info.Name, info.TableName, info.Dimensions, info.Distance,
		string(metaJSON), string(auxJSON), string(info.Status), info.VectorCount,
		info.CreatedAt.Format(time.RFC3339), info.UpdatedAt.Format(time.RFC3339), info.Error)

	return err
}

func (im *IndexManager) countVectors(ctx context.Context, info *IndexInfo) (int64, error) {
	return im.newVecStore(info).Count(ctx)
}

func (im *IndexManager) deleteIndexMeta(ctx context.Context, name string) error {
	_, err := im.db.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE name = ?", im.metaTable), name)
	return err
}

func (im *IndexManager) resolvedVectorDir() string {
	if im.vecDir != "" {
		return im.vecDir
	}
	return sqlite.SidecarDirForSQLDB(context.Background(), im.db, "vec")
}

func cloneIndexInfo(info *IndexInfo) *IndexInfo {
	if info == nil {
		return nil
	}

	cloned := *info
	cloned.Metadata = append([]string(nil), info.Metadata...)
	cloned.AuxColumns = append([]string(nil), info.AuxColumns...)
	return &cloned
}

func (im *IndexManager) newVecStore(info *IndexInfo) *vec.VecStore {
	return vec.NewVecStore(vec.VecStoreConfig{
		DB:          im.db,
		TableName:   info.TableName,
		Dimensions:  info.Dimensions,
		Distance:    vec.DistanceMetric(info.Distance),
		Metadata:    info.Metadata,
		AuxColumns:  info.AuxColumns,
		PersistPath: im.resolvedVectorDir(),
	})
}

type IndexItem struct {
	ID       any
	Text     string
	Vector   []float32
	Metadata map[string]string
}

type IndexResult struct {
	IndexName   string        `json:"index_name"`
	Total       int           `json:"total"`
	Indexed     int           `json:"indexed"`
	Failed      int           `json:"failed"`
	StartedAt   time.Time     `json:"started_at"`
	CompletedAt time.Time     `json:"completed_at"`
	Duration    time.Duration `json:"duration"`
}
