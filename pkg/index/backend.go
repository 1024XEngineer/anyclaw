package index

import (
	"context"
	"fmt"

	"github.com/1024XEngineer/anyclaw/pkg/vec"
)

type VecBackendConfig struct {
	Host   string
	Port   int
	APIKey string
	UseTLS bool
}

type vectorBackend interface {
	Init(context.Context) error
	EnsurePayloadIndexes(context.Context) error
	Drop(context.Context) error
	Count(context.Context) (int64, error)
	InsertBatch(context.Context, []vec.VecItem) error
	Delete(context.Context, int64) error
	Search(context.Context, []float32, int) ([]vec.VecSearchResult, error)
	SearchWithFilter(context.Context, []float32, int, map[string]string) ([]vec.VecSearchResult, error)
	Close() error
}

type vectorBackendFactory func(info *IndexInfo) (vectorBackend, error)

type vecStoreBackend struct {
	store *vec.VecStore
}

func newVecBackendFactory(cfg VecBackendConfig) vectorBackendFactory {
	return func(info *IndexInfo) (vectorBackend, error) {
		if info == nil {
			return nil, fmt.Errorf("index info cannot be nil")
		}

		return &vecStoreBackend{
			store: vec.NewVecStore(vec.VecStoreConfig{
				Host:       cfg.Host,
				Port:       cfg.Port,
				APIKey:     cfg.APIKey,
				UseTLS:     cfg.UseTLS,
				TableName:  info.TableName,
				Dimensions: info.Dimensions,
				Distance:   vec.DistanceMetric(info.Distance),
				Metadata:   info.Metadata,
				AuxColumns: info.AuxColumns,
			}),
		}, nil
	}
}

func (b *vecStoreBackend) Init(ctx context.Context) error {
	return b.store.Init(ctx)
}

func (b *vecStoreBackend) EnsurePayloadIndexes(ctx context.Context) error {
	return b.store.EnsurePayloadIndexes(ctx)
}

func (b *vecStoreBackend) Drop(ctx context.Context) error {
	return b.store.Drop(ctx)
}

func (b *vecStoreBackend) Count(ctx context.Context) (int64, error) {
	return b.store.Count(ctx)
}

func (b *vecStoreBackend) InsertBatch(ctx context.Context, items []vec.VecItem) error {
	return b.store.InsertBatch(ctx, items)
}

func (b *vecStoreBackend) Delete(ctx context.Context, id int64) error {
	return b.store.Delete(ctx, id)
}

func (b *vecStoreBackend) Search(ctx context.Context, queryVector []float32, limit int) ([]vec.VecSearchResult, error) {
	return b.store.Search(ctx, queryVector, limit)
}

func (b *vecStoreBackend) SearchWithFilter(ctx context.Context, queryVector []float32, limit int, metadataFilter map[string]string) ([]vec.VecSearchResult, error) {
	return b.store.SearchWithFilter(ctx, queryVector, limit, -1, metadataFilter)
}

func (b *vecStoreBackend) Close() error {
	return b.store.Close()
}
