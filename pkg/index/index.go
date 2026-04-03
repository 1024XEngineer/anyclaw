package index

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/anyclaw/anyclaw/pkg/embedding"
	"github.com/anyclaw/anyclaw/pkg/vec"
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

type IndexManager struct {
	db        *sql.DB
	embedder  embedding.Provider
	indexes   map[string]*IndexInfo
	metaTable string
	mu        sync.RWMutex
}

func NewIndexManager(db *sql.DB, embedder embedding.Provider) *IndexManager {
	return &IndexManager{
		db:        db,
		embedder:  embedder,
		indexes:   make(map[string]*IndexInfo),
		metaTable: "vector_index_meta",
	}
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
			json.Unmarshal([]byte(metaJSON), &info.Metadata)
		}
		if auxJSON != "" {
			json.Unmarshal([]byte(auxJSON), &info.AuxColumns)
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

	info := &IndexInfo{
		Name:       cfg.Name,
		TableName:  tableName,
		Dimensions: cfg.Dimensions,
		Distance:   string(cfg.Distance),
		Metadata:   cfg.Metadata,
		AuxColumns: cfg.AuxColumns,
		Status:     StatusCreating,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	if err := im.saveIndexMeta(ctx, info); err != nil {
		return nil, err
	}

	vs := vec.NewVecStore(vec.VecStoreConfig{
		DB:         im.db,
		TableName:  tableName,
		Dimensions: cfg.Dimensions,
		Distance:   cfg.Distance,
		Metadata:   cfg.Metadata,
		AuxColumns: cfg.AuxColumns,
	})

	if err := vs.Init(ctx); err != nil {
		info.Status = StatusError
		info.Error = err.Error()
		im.saveIndexMeta(ctx, info)
		return nil, fmt.Errorf("create vector table: %w", err)
	}

	info.Status = StatusReady
	im.saveIndexMeta(ctx, info)
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
	im.saveIndexMeta(ctx, info)

	if cfg.Dimensions != info.Dimensions {
		info.Status = StatusError
		info.Error = "cannot change dimensions of existing index"
		im.saveIndexMeta(ctx, info)
		return nil, fmt.Errorf("cannot change dimensions: existing=%d, requested=%d", info.Dimensions, cfg.Dimensions)
	}

	if cfg.Distance != "" && cfg.Distance != vec.DistanceMetric(info.Distance) {
		info.Status = StatusError
		info.Error = "cannot change distance metric of existing index"
		im.saveIndexMeta(ctx, info)
		return nil, fmt.Errorf("cannot change distance metric")
	}

	if len(cfg.Metadata) > 0 {
		info.Metadata = cfg.Metadata
	}
	if len(cfg.AuxColumns) > 0 {
		info.AuxColumns = cfg.AuxColumns
	}

	info.Status = StatusReady
	info.UpdatedAt = time.Now()
	im.saveIndexMeta(ctx, info)
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
	im.saveIndexMeta(ctx, info)

	_, err := im.db.ExecContext(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", info.TableName))
	if err != nil {
		info.Status = StatusError
		info.Error = err.Error()
		im.saveIndexMeta(ctx, info)
		return fmt.Errorf("drop table: %w", err)
	}

	_, err = im.db.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE name = ?", im.metaTable), name)
	if err != nil {
		return fmt.Errorf("delete meta: %w", err)
	}

	delete(im.indexes, name)
	return nil
}

func (im *IndexManager) Get(name string) (*IndexInfo, error) {
	im.mu.RLock()
	defer im.mu.RUnlock()

	info, exists := im.indexes[name]
	if !exists {
		return nil, fmt.Errorf("index %q not found", name)
	}

	count, err := im.countVectors(context.Background(), info.TableName)
	if err == nil {
		info.VectorCount = count
	}

	return info, nil
}

func (im *IndexManager) List() []*IndexInfo {
	im.mu.RLock()
	defer im.mu.RUnlock()

	var result []*IndexInfo
	for _, info := range im.indexes {
		result = append(result, info)
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
	im.saveIndexMeta(ctx, info)
	im.mu.Unlock()

	start := time.Now()
	vs := vec.NewVecStore(vec.VecStoreConfig{
		DB:         im.db,
		TableName:  info.TableName,
		Dimensions: info.Dimensions,
		Distance:   vec.DistanceMetric(info.Distance),
		Metadata:   info.Metadata,
	})

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
		for _, item := range batch {
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

			emb, err := im.embedder.Embed(ctx, item.Text)
			if err != nil {
				result.Failed++
				if progress != nil {
					progress(Progress{
						Total:     len(items),
						Processed: i + 1,
						Failed:    result.Failed,
						Elapsed:   time.Since(start),
						CurrentID: item.ID,
						Message:   fmt.Sprintf("embed failed: %v", err),
					})
				}
				continue
			}

			vecItems = append(vecItems, vec.VecItem{
				ID:       item.ID,
				Vector:   emb,
				Metadata: item.Metadata,
			})
		}

		if len(vecItems) > 0 {
			if err := vs.InsertBatch(ctx, vecItems); err != nil {
				result.Failed += len(vecItems)
			} else {
				result.Indexed += len(vecItems)
			}
		}

		processed := end
		elapsed := time.Since(start)
		rate := float64(processed) / elapsed.Seconds()
		remaining := len(items) - processed
		eta := time.Duration(float64(remaining)/rate) * time.Second

		if progress != nil {
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
	info.UpdatedAt = time.Now()
	count, _ := im.countVectors(ctx, info.TableName)
	info.VectorCount = count
	im.saveIndexMeta(ctx, info)
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

	vs := vec.NewVecStore(vec.VecStoreConfig{
		DB:        im.db,
		TableName: info.TableName,
	})

	removed := 0
	for _, id := range ids {
		if err := vs.Delete(ctx, id); err == nil {
			removed++
		}
	}

	im.mu.Lock()
	info.UpdatedAt = time.Now()
	count, _ := im.countVectors(ctx, info.TableName)
	info.VectorCount = count
	im.saveIndexMeta(ctx, info)
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
	info.UpdatedAt = time.Now()
	im.saveIndexMeta(ctx, info)
	im.mu.Unlock()

	_, err := im.db.ExecContext(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", info.TableName))
	if err != nil {
		im.mu.Lock()
		info.Status = StatusError
		info.Error = err.Error()
		im.saveIndexMeta(ctx, info)
		im.mu.Unlock()
		return nil, fmt.Errorf("drop table for rebuild: %w", err)
	}

	vs := vec.NewVecStore(vec.VecStoreConfig{
		DB:         im.db,
		TableName:  info.TableName,
		Dimensions: info.Dimensions,
		Distance:   vec.DistanceMetric(info.Distance),
		Metadata:   info.Metadata,
		AuxColumns: info.AuxColumns,
	})

	if err := vs.Init(ctx); err != nil {
		im.mu.Lock()
		info.Status = StatusError
		info.Error = err.Error()
		im.saveIndexMeta(ctx, info)
		im.mu.Unlock()
		return nil, fmt.Errorf("recreate table: %w", err)
	}

	im.mu.Lock()
	info.Status = StatusReady
	info.UpdatedAt = time.Now()
	info.VectorCount = 0
	im.saveIndexMeta(ctx, info)
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
		CompletedAt: time.Now(),
		Duration:    time.Since(time.Now()),
	}, nil
}

func (im *IndexManager) Search(ctx context.Context, indexName string, queryVector []float32, limit int) ([]vec.VecSearchResult, error) {
	im.mu.RLock()
	info, exists := im.indexes[indexName]
	im.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("index %q not found", indexName)
	}

	vs := vec.NewVecStore(vec.VecStoreConfig{
		DB:         im.db,
		TableName:  info.TableName,
		Dimensions: info.Dimensions,
		Distance:   vec.DistanceMetric(info.Distance),
		Metadata:   info.Metadata,
	})

	return vs.Search(ctx, queryVector, limit)
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

func (im *IndexManager) countVectors(ctx context.Context, tableName string) (int64, error) {
	var count int64
	err := im.db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName)).Scan(&count)
	return count, err
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
