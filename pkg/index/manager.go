package index

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/embedding"
	"github.com/1024XEngineer/anyclaw/pkg/vec"
)

type ManagerConfig struct {
	DB            *sql.DB
	Embedder      embedding.Provider
	MetaTable     string
	VectorBackend VecBackendConfig
}

type IndexManager struct {
	embedder       embedding.Provider
	metaStore      *indexMetaStore
	backendFactory vectorBackendFactory
	indexes        map[string]*IndexInfo
	mu             sync.RWMutex
}

func NewIndexManager(cfg ManagerConfig) *IndexManager {
	return &IndexManager{
		embedder:       cfg.Embedder,
		metaStore:      newIndexMetaStore(cfg.DB, cfg.MetaTable),
		backendFactory: newVecBackendFactory(cfg.VectorBackend),
		indexes:        make(map[string]*IndexInfo),
	}
}

func (im *IndexManager) Init(ctx context.Context) error {
	if im.metaStore == nil {
		return fmt.Errorf("metadata store is required")
	}

	if err := im.metaStore.Init(ctx); err != nil {
		return fmt.Errorf("init metadata store: %w", err)
	}

	indexes, err := im.metaStore.Load(ctx)
	if err != nil {
		return fmt.Errorf("load indexes: %w", err)
	}

	im.mu.Lock()
	im.indexes = indexes
	im.mu.Unlock()
	return nil
}

func (im *IndexManager) Create(ctx context.Context, cfg Config) (*IndexInfo, error) {
	cfg = cfg.normalized()
	if err := validateIndexConfig(cfg); err != nil {
		return nil, err
	}

	now := time.Now()
	info := &IndexInfo{
		Name:       cfg.Name,
		TableName:  cfg.TableName,
		Dimensions: cfg.Dimensions,
		Distance:   string(cfg.Distance),
		Metadata:   cloneStringSlice(cfg.Metadata),
		AuxColumns: cloneStringSlice(cfg.AuxColumns),
		Status:     StatusCreating,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if err := im.insertIndex(ctx, info); err != nil {
		return nil, err
	}

	backend, err := im.newBackend(info)
	if err != nil {
		im.markIndexError(ctx, info.Name, err)
		return nil, err
	}
	defer backend.Close()

	if err := backend.Init(ctx); err != nil {
		im.markIndexError(ctx, info.Name, err)
		return nil, fmt.Errorf("create vector collection: %w", err)
	}
	if err := backend.EnsurePayloadIndexes(ctx); err != nil {
		im.markIndexError(ctx, info.Name, err)
		return nil, fmt.Errorf("create payload indexes: %w", err)
	}

	info.Status = StatusReady
	info.Error = ""
	info.UpdatedAt = time.Now()
	info.VectorCount = 0
	return im.persistIndex(ctx, info)
}

func (im *IndexManager) Update(ctx context.Context, name string, cfg Config) (*IndexInfo, error) {
	current, err := im.getIndex(name)
	if err != nil {
		return nil, err
	}

	nextCfg := buildConfigFromInfo(current)
	if cfg.Dimensions != 0 && cfg.Dimensions != current.Dimensions {
		return nil, fmt.Errorf("cannot change dimensions: existing=%d, requested=%d", current.Dimensions, cfg.Dimensions)
	}
	if cfg.Distance != "" && cfg.Distance != vec.DistanceMetric(current.Distance) {
		return nil, fmt.Errorf("cannot change distance metric")
	}
	if len(cfg.Metadata) > 0 {
		nextCfg.Metadata = cloneStringSlice(cfg.Metadata)
	}
	if len(cfg.AuxColumns) > 0 {
		nextCfg.AuxColumns = cloneStringSlice(cfg.AuxColumns)
	}
	if err := validateIndexConfig(nextCfg); err != nil {
		return nil, err
	}

	updated := cloneIndexInfo(current)
	updated.Metadata = cloneStringSlice(nextCfg.Metadata)
	updated.AuxColumns = cloneStringSlice(nextCfg.AuxColumns)
	updated.Status = StatusUpdating
	updated.Error = ""
	updated.UpdatedAt = time.Now()

	if _, err := im.persistIndex(ctx, updated); err != nil {
		return nil, err
	}

	backend, err := im.openBackend(ctx, updated)
	if err != nil {
		im.markIndexError(ctx, name, err)
		return nil, fmt.Errorf("validate updated index: %w", err)
	}
	defer backend.Close()

	if err := backend.EnsurePayloadIndexes(ctx); err != nil {
		im.markIndexError(ctx, name, err)
		return nil, fmt.Errorf("create payload indexes: %w", err)
	}

	updated.Status = StatusReady
	updated.Error = ""
	updated.UpdatedAt = time.Now()
	return im.persistIndex(ctx, updated)
}

func (im *IndexManager) Delete(ctx context.Context, name string) error {
	current, err := im.getIndex(name)
	if err != nil {
		return err
	}

	deleting := cloneIndexInfo(current)
	deleting.Status = StatusDeleting
	deleting.Error = ""
	deleting.UpdatedAt = time.Now()
	if _, err := im.persistIndex(ctx, deleting); err != nil {
		return err
	}

	backend, err := im.newBackend(deleting)
	if err != nil {
		im.markIndexError(ctx, name, err)
		return err
	}
	defer backend.Close()

	if err := backend.Drop(ctx); err != nil {
		im.markIndexError(ctx, name, err)
		return fmt.Errorf("delete vector collection: %w", err)
	}

	if err := im.deleteIndex(ctx, name); err != nil {
		return fmt.Errorf("delete index metadata: %w", err)
	}
	return nil
}

func (im *IndexManager) Get(name string) (*IndexInfo, error) {
	info, err := im.getIndex(name)
	if err != nil {
		return nil, err
	}

	backend, err := im.openBackend(context.Background(), info)
	if err == nil {
		defer backend.Close()
		if count, err := backend.Count(context.Background()); err == nil {
			info.VectorCount = count
			im.cacheVectorCount(name, count)
		}
	}

	return info, nil
}

func (im *IndexManager) List() []*IndexInfo {
	im.mu.RLock()
	defer im.mu.RUnlock()

	result := make([]*IndexInfo, 0, len(im.indexes))
	for _, info := range im.indexes {
		result = append(result, cloneIndexInfo(info))
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

func (im *IndexManager) insertIndex(ctx context.Context, info *IndexInfo) error {
	if info == nil {
		return fmt.Errorf("index info cannot be nil")
	}

	cloned := cloneIndexInfo(info)

	im.mu.Lock()
	defer im.mu.Unlock()

	if _, exists := im.indexes[cloned.Name]; exists {
		return fmt.Errorf("index %q already exists", cloned.Name)
	}

	if err := im.metaStore.Save(ctx, cloned); err != nil {
		return err
	}
	im.indexes[cloned.Name] = cloned
	return nil
}

func (im *IndexManager) persistIndex(ctx context.Context, info *IndexInfo) (*IndexInfo, error) {
	if info == nil {
		return nil, fmt.Errorf("index info cannot be nil")
	}

	cloned := cloneIndexInfo(info)

	im.mu.Lock()
	defer im.mu.Unlock()

	if err := im.metaStore.Save(ctx, cloned); err != nil {
		return nil, err
	}
	im.indexes[cloned.Name] = cloned
	return cloneIndexInfo(cloned), nil
}

func (im *IndexManager) deleteIndex(ctx context.Context, name string) error {
	im.mu.Lock()
	defer im.mu.Unlock()

	if err := im.metaStore.Delete(ctx, name); err != nil {
		return err
	}
	delete(im.indexes, name)
	return nil
}

func (im *IndexManager) getIndex(name string) (*IndexInfo, error) {
	im.mu.RLock()
	defer im.mu.RUnlock()

	info, exists := im.indexes[name]
	if !exists {
		return nil, fmt.Errorf("index %q not found", name)
	}

	return cloneIndexInfo(info), nil
}

func (im *IndexManager) cacheVectorCount(name string, count int64) {
	im.mu.Lock()
	defer im.mu.Unlock()

	if info, ok := im.indexes[name]; ok {
		info.VectorCount = count
	}
}

func (im *IndexManager) newBackend(info *IndexInfo) (vectorBackend, error) {
	if im.backendFactory == nil {
		return nil, fmt.Errorf("vector backend factory is not configured")
	}
	return im.backendFactory(info)
}

func (im *IndexManager) openBackend(ctx context.Context, info *IndexInfo) (vectorBackend, error) {
	backend, err := im.newBackend(info)
	if err != nil {
		return nil, err
	}

	if err := backend.Init(ctx); err != nil {
		backend.Close()
		return nil, err
	}

	return backend, nil
}

func (im *IndexManager) markIndexError(ctx context.Context, name string, cause error) {
	info, err := im.getIndex(name)
	if err != nil {
		return
	}

	info.Status = StatusError
	info.Error = cause.Error()
	info.UpdatedAt = time.Now()
	_, _ = im.persistIndex(ctx, info)
}

func buildConfigFromInfo(info *IndexInfo) Config {
	return Config{
		Name:       info.Name,
		TableName:  info.TableName,
		Dimensions: info.Dimensions,
		Distance:   vec.DistanceMetric(info.Distance),
		Metadata:   cloneStringSlice(info.Metadata),
		AuxColumns: cloneStringSlice(info.AuxColumns),
	}
}

func validateIndexConfig(cfg Config) error {
	if strings.TrimSpace(cfg.Name) == "" {
		return fmt.Errorf("index name cannot be empty")
	}

	cfg = cfg.normalized()
	if cfg.Dimensions <= 0 {
		return fmt.Errorf("dimensions must be greater than 0")
	}
	if cfg.Distance != vec.DistanceCosine && cfg.Distance != vec.DistanceL2 {
		return fmt.Errorf("unsupported distance metric %q", cfg.Distance)
	}
	if err := validateIdentifier("table name", cfg.TableName); err != nil {
		return err
	}

	seen := map[string]string{
		"rowid":  "reserved column",
		"vector": "reserved column",
	}
	for _, column := range cfg.Metadata {
		if err := validateIdentifier("metadata column", column); err != nil {
			return err
		}
		lower := strings.ToLower(column)
		if prev, ok := seen[lower]; ok {
			return fmt.Errorf("duplicate column %q conflicts with %s", column, prev)
		}
		seen[lower] = "metadata column"
	}
	for _, column := range cfg.AuxColumns {
		if err := validateIdentifier("aux column", column); err != nil {
			return err
		}
		lower := strings.ToLower(column)
		if prev, ok := seen[lower]; ok {
			return fmt.Errorf("duplicate column %q conflicts with %s", column, prev)
		}
		seen[lower] = "aux column"
	}

	return nil
}
