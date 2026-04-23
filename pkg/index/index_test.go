package index

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/1024XEngineer/anyclaw/pkg/vec"
	_ "modernc.org/sqlite"
)

func setupIndexManager(t *testing.T) (*IndexManager, *mockEmbedder, *fakeBackendFactory) {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	embedder := &mockEmbedder{dim: 4}
	im := NewIndexManager(ManagerConfig{
		DB:       db,
		Embedder: embedder,
	})

	factory := newFakeBackendFactory()
	im.backendFactory = factory.NewBackend

	if err := im.Init(context.Background()); err != nil {
		t.Fatalf("init index manager: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
	})

	return im, embedder, factory
}

func TestInitRejectsNilDB(t *testing.T) {
	im := NewIndexManager(ManagerConfig{})
	if err := im.Init(context.Background()); err == nil {
		t.Fatal("expected init with nil db to fail")
	}
}

func TestCreateIndex(t *testing.T) {
	im, _, factory := setupIndexManager(t)
	ctx := context.Background()

	info, err := im.Create(ctx, Config{
		Name:       "test_index",
		Dimensions: 4,
		Distance:   vec.DistanceCosine,
		Metadata:   []string{"category"},
		AuxColumns: []string{"tag"},
	})
	if err != nil {
		t.Fatalf("create index: %v", err)
	}

	if info.Name != "test_index" {
		t.Fatalf("expected name test_index, got %s", info.Name)
	}
	if info.Status != StatusReady {
		t.Fatalf("expected status ready, got %s", info.Status)
	}
	if info.VectorCount != 0 {
		t.Fatalf("expected vector count 0, got %d", info.VectorCount)
	}

	store := factory.store(info.TableName)
	for _, field := range []string{"category", "tag"} {
		if !store.fieldIndexes[field] {
			t.Fatalf("expected field index for %q", field)
		}
	}
}

func TestCreateDuplicateIndex(t *testing.T) {
	im, _, _ := setupIndexManager(t)
	ctx := context.Background()

	_, err := im.Create(ctx, Config{
		Name:       "dup",
		Dimensions: 4,
		Distance:   vec.DistanceCosine,
	})
	if err != nil {
		t.Fatalf("first create: %v", err)
	}

	_, err = im.Create(ctx, Config{
		Name:       "dup",
		Dimensions: 4,
		Distance:   vec.DistanceCosine,
	})
	if err == nil {
		t.Fatal("expected duplicate create to fail")
	}
}

func TestUpdateIndexAddsPayloadIndexes(t *testing.T) {
	im, _, factory := setupIndexManager(t)
	ctx := context.Background()

	info, err := im.Create(ctx, Config{
		Name:       "update_test",
		Dimensions: 4,
		Distance:   vec.DistanceCosine,
		Metadata:   []string{"category"},
	})
	if err != nil {
		t.Fatalf("create index: %v", err)
	}

	updated, err := im.Update(ctx, "update_test", Config{
		Metadata:   []string{"category", "source"},
		AuxColumns: []string{"tag"},
	})
	if err != nil {
		t.Fatalf("update index: %v", err)
	}

	if updated.Status != StatusReady {
		t.Fatalf("expected updated index to be ready, got %s", updated.Status)
	}

	store := factory.store(info.TableName)
	for _, field := range []string{"category", "source", "tag"} {
		if !store.fieldIndexes[field] {
			t.Fatalf("expected field index for %q", field)
		}
	}
}

func TestDeleteIndex(t *testing.T) {
	im, _, factory := setupIndexManager(t)
	ctx := context.Background()

	info, err := im.Create(ctx, Config{
		Name:       "del_test",
		Dimensions: 4,
		Distance:   vec.DistanceCosine,
	})
	if err != nil {
		t.Fatalf("create index: %v", err)
	}

	if err := im.Delete(ctx, "del_test"); err != nil {
		t.Fatalf("delete index: %v", err)
	}

	if _, err := im.Get("del_test"); err == nil {
		t.Fatal("expected deleted index to be missing")
	}
	if !factory.store(info.TableName).dropped {
		t.Fatal("expected backend collection to be dropped")
	}
}

func TestIndexWithVectorsAndText(t *testing.T) {
	im, embedder, _ := setupIndexManager(t)
	ctx := context.Background()

	_, err := im.Create(ctx, Config{
		Name:       "vec_index",
		Dimensions: 4,
		Distance:   vec.DistanceCosine,
		Metadata:   []string{"category"},
		AuxColumns: []string{"tag"},
	})
	if err != nil {
		t.Fatalf("create index: %v", err)
	}

	items := []IndexItem{
		{ID: 1, Vector: []float32{1, 0, 0, 0}, Metadata: map[string]string{"category": "keep"}, Aux: map[string]string{"tag": "v1"}},
		{ID: 2, Text: "embed me", Metadata: map[string]string{"category": "text"}, Aux: map[string]string{"tag": "v2"}},
		{ID: 3, Vector: []float32{0, 1, 0, 0}, Metadata: map[string]string{"category": "skip"}, Aux: map[string]string{"tag": "v3"}},
	}

	var progressCount atomic.Int32
	result, err := im.Index(ctx, "vec_index", items, func(p Progress) {
		progressCount.Add(1)
	})
	if err != nil {
		t.Fatalf("index: %v", err)
	}

	if result.Indexed != 3 {
		t.Fatalf("expected 3 indexed, got %d", result.Indexed)
	}
	if result.Failed != 0 {
		t.Fatalf("expected 0 failed, got %d", result.Failed)
	}
	if embedder.callCount.Load() != 1 {
		t.Fatalf("expected one embed call, got %d", embedder.callCount.Load())
	}
	if progressCount.Load() == 0 {
		t.Fatal("expected progress callback")
	}

	info, err := im.Get("vec_index")
	if err != nil {
		t.Fatalf("get index: %v", err)
	}
	if info.VectorCount != 3 {
		t.Fatalf("expected vector count 3, got %d", info.VectorCount)
	}
}

func TestSearchWithFilterDoesNotRequireExactMatch(t *testing.T) {
	im, _, _ := setupIndexManager(t)
	ctx := context.Background()

	_, err := im.Create(ctx, Config{
		Name:       "search_index",
		Dimensions: 4,
		Distance:   vec.DistanceCosine,
		Metadata:   []string{"category"},
	})
	if err != nil {
		t.Fatalf("create index: %v", err)
	}

	_, err = im.Index(ctx, "search_index", []IndexItem{
		{ID: 1, Vector: []float32{0.9, 0.1, 0, 0}, Metadata: map[string]string{"category": "keep"}},
		{ID: 2, Vector: []float32{0, 1, 0, 0}, Metadata: map[string]string{"category": "skip"}},
	}, nil)
	if err != nil {
		t.Fatalf("index vectors: %v", err)
	}

	results, err := im.SearchWithFilter(ctx, "search_index", []float32{1, 0, 0, 0}, 10, map[string]string{
		"category": "keep",
	})
	if err != nil {
		t.Fatalf("search with filter: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected one filtered result, got %d", len(results))
	}
	if results[0].ID != 1 {
		t.Fatalf("expected id 1, got %d", results[0].ID)
	}
	if results[0].Distance == 0 {
		t.Fatal("expected a non-exact match distance")
	}
}

func TestSearchByTextWithFilter(t *testing.T) {
	im, _, _ := setupIndexManager(t)
	ctx := context.Background()

	_, err := im.Create(ctx, Config{
		Name:       "text_search",
		Dimensions: 4,
		Distance:   vec.DistanceCosine,
		Metadata:   []string{"category"},
	})
	if err != nil {
		t.Fatalf("create index: %v", err)
	}

	_, err = im.Index(ctx, "text_search", []IndexItem{
		{ID: 1, Vector: []float32{1, 0, 0, 0}, Metadata: map[string]string{"category": "keep"}},
		{ID: 2, Vector: []float32{0, 1, 0, 0}, Metadata: map[string]string{"category": "skip"}},
	}, nil)
	if err != nil {
		t.Fatalf("index vectors: %v", err)
	}

	results, err := im.SearchByTextWithFilter(ctx, "text_search", "query", 10, map[string]string{
		"category": "keep",
	})
	if err != nil {
		t.Fatalf("search by text with filter: %v", err)
	}
	if len(results) != 1 || results[0].ID != 1 {
		t.Fatalf("expected filtered text search to return id 1, got %#v", results)
	}
}

func TestRemoveVectors(t *testing.T) {
	im, _, _ := setupIndexManager(t)
	ctx := context.Background()

	_, err := im.Create(ctx, Config{
		Name:       "remove_index",
		Dimensions: 4,
		Distance:   vec.DistanceCosine,
	})
	if err != nil {
		t.Fatalf("create index: %v", err)
	}

	_, err = im.Index(ctx, "remove_index", []IndexItem{
		{ID: 1, Vector: []float32{1, 0, 0, 0}},
		{ID: 2, Vector: []float32{0, 1, 0, 0}},
		{ID: 3, Vector: []float32{0, 0, 1, 0}},
	}, nil)
	if err != nil {
		t.Fatalf("index vectors: %v", err)
	}

	removed, err := im.RemoveVectors(ctx, "remove_index", []int64{1, 2})
	if err != nil {
		t.Fatalf("remove vectors: %v", err)
	}
	if removed != 2 {
		t.Fatalf("expected 2 removed, got %d", removed)
	}

	info, err := im.Get("remove_index")
	if err != nil {
		t.Fatalf("get index: %v", err)
	}
	if info.VectorCount != 1 {
		t.Fatalf("expected vector count 1, got %d", info.VectorCount)
	}
}

func TestRebuildIndex(t *testing.T) {
	im, _, factory := setupIndexManager(t)
	ctx := context.Background()

	info, err := im.Create(ctx, Config{
		Name:       "rebuild_index",
		Dimensions: 4,
		Distance:   vec.DistanceCosine,
		Metadata:   []string{"category"},
	})
	if err != nil {
		t.Fatalf("create index: %v", err)
	}

	_, err = im.Index(ctx, "rebuild_index", []IndexItem{
		{ID: 1, Vector: []float32{1, 0, 0, 0}, Metadata: map[string]string{"category": "a"}},
		{ID: 2, Vector: []float32{0, 1, 0, 0}, Metadata: map[string]string{"category": "b"}},
	}, nil)
	if err != nil {
		t.Fatalf("index vectors: %v", err)
	}

	var progressCalled bool
	result, err := im.Rebuild(ctx, "rebuild_index", func(p Progress) {
		progressCalled = true
	})
	if err != nil {
		t.Fatalf("rebuild index: %v", err)
	}
	if !progressCalled {
		t.Fatal("expected rebuild progress callback")
	}
	if result.Duration < 0 {
		t.Fatal("expected non-negative rebuild duration")
	}

	reloaded, err := im.Get("rebuild_index")
	if err != nil {
		t.Fatalf("get rebuilt index: %v", err)
	}
	if reloaded.Status != StatusReady {
		t.Fatalf("expected rebuilt index to be ready, got %s", reloaded.Status)
	}
	if reloaded.VectorCount != 0 {
		t.Fatalf("expected rebuilt index to be empty, got %d", reloaded.VectorCount)
	}
	if !factory.store(info.TableName).fieldIndexes["category"] {
		t.Fatal("expected payload indexes to be restored after rebuild")
	}
}

func TestIndexMetaPersistence(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	embedder := &mockEmbedder{dim: 4}
	factory := newFakeBackendFactory()

	im := NewIndexManager(ManagerConfig{
		DB:       db,
		Embedder: embedder,
	})
	im.backendFactory = factory.NewBackend

	ctx := context.Background()
	if err := im.Init(ctx); err != nil {
		t.Fatalf("init index manager: %v", err)
	}

	_, err = im.Create(ctx, Config{
		Name:       "persist_test",
		Dimensions: 4,
		Distance:   vec.DistanceCosine,
		Metadata:   []string{"tag1", "tag2"},
		AuxColumns: []string{"aux"},
	})
	if err != nil {
		t.Fatalf("create index: %v", err)
	}

	im2 := NewIndexManager(ManagerConfig{
		DB:       db,
		Embedder: embedder,
	})
	im2.backendFactory = factory.NewBackend
	if err := im2.Init(ctx); err != nil {
		t.Fatalf("re-init index manager: %v", err)
	}

	info, err := im2.Get("persist_test")
	if err != nil {
		t.Fatalf("get persisted index: %v", err)
	}
	if info.Distance != string(vec.DistanceCosine) {
		t.Fatalf("expected cosine distance, got %s", info.Distance)
	}
	if len(info.Metadata) != 2 || len(info.AuxColumns) != 1 {
		t.Fatalf("expected metadata and aux columns to persist, got %#v", info)
	}
}

func TestListIndexesSorted(t *testing.T) {
	im, _, _ := setupIndexManager(t)
	ctx := context.Background()

	for _, name := range []string{"idx3", "idx1", "idx2"} {
		_, err := im.Create(ctx, Config{
			Name:       name,
			Dimensions: 4,
			Distance:   vec.DistanceCosine,
		})
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}

	list := im.List()
	names := []string{list[0].Name, list[1].Name, list[2].Name}
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	for i := range names {
		if names[i] != sorted[i] {
			t.Fatalf("expected sorted list, got %v", names)
		}
	}
}

type mockEmbedder struct {
	dim       int
	callCount atomic.Int32
}

func (m *mockEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	m.callCount.Add(1)
	result := make([]float32, m.dim)
	if strings.EqualFold(text, "query") {
		result[0] = 1
		return result, nil
	}

	for i := range result {
		result[i] = float32(len(text)) / float32(m.dim)
	}
	return result, nil
}

func (m *mockEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, 0, len(texts))
	for _, text := range texts {
		embedding, err := m.Embed(ctx, text)
		if err != nil {
			return nil, err
		}
		result = append(result, embedding)
	}
	return result, nil
}

func (m *mockEmbedder) Name() string   { return "mock" }
func (m *mockEmbedder) Dimension() int { return m.dim }

type fakeBackendFactory struct {
	mu     sync.Mutex
	stores map[string]*fakeVectorBackend
}

func newFakeBackendFactory() *fakeBackendFactory {
	return &fakeBackendFactory{
		stores: make(map[string]*fakeVectorBackend),
	}
}

func (f *fakeBackendFactory) NewBackend(info *IndexInfo) (vectorBackend, error) {
	if info == nil {
		return nil, fmt.Errorf("index info cannot be nil")
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	store, ok := f.stores[info.TableName]
	if !ok {
		store = &fakeVectorBackend{
			info:         cloneIndexInfo(info),
			points:       make(map[int64]vec.VecItem),
			fieldIndexes: make(map[string]bool),
		}
		f.stores[info.TableName] = store
		return store, nil
	}

	store.info = cloneIndexInfo(info)
	return store, nil
}

func (f *fakeBackendFactory) store(tableName string) *fakeVectorBackend {
	f.mu.Lock()
	defer f.mu.Unlock()

	store, ok := f.stores[tableName]
	if !ok {
		store = &fakeVectorBackend{
			points:       make(map[int64]vec.VecItem),
			fieldIndexes: make(map[string]bool),
		}
		f.stores[tableName] = store
	}
	return store
}

type fakeVectorBackend struct {
	mu           sync.Mutex
	info         *IndexInfo
	points       map[int64]vec.VecItem
	fieldIndexes map[string]bool
	initialized  bool
	dropped      bool
}

func (b *fakeVectorBackend) Init(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.initialized = true
	b.dropped = false
	if b.points == nil {
		b.points = make(map[int64]vec.VecItem)
	}
	if b.fieldIndexes == nil {
		b.fieldIndexes = make(map[string]bool)
	}
	return nil
}

func (b *fakeVectorBackend) EnsurePayloadIndexes(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, field := range b.info.Metadata {
		b.fieldIndexes[field] = true
	}
	for _, field := range b.info.AuxColumns {
		b.fieldIndexes[field] = true
	}
	return nil
}

func (b *fakeVectorBackend) Drop(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.points = make(map[int64]vec.VecItem)
	b.fieldIndexes = make(map[string]bool)
	b.initialized = false
	b.dropped = true
	return nil
}

func (b *fakeVectorBackend) Count(ctx context.Context) (int64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return int64(len(b.points)), nil
}

func (b *fakeVectorBackend) InsertBatch(ctx context.Context, items []vec.VecItem) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.initialized {
		return fmt.Errorf("backend not initialized")
	}

	for _, item := range items {
		b.points[item.ID] = vec.VecItem{
			ID:       item.ID,
			RowID:    item.ID,
			Vector:   append([]float32(nil), item.Vector...),
			Metadata: cloneStringMap(item.Metadata),
			Aux:      cloneStringMap(item.Aux),
		}
	}
	return nil
}

func (b *fakeVectorBackend) Delete(ctx context.Context, id int64) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	delete(b.points, id)
	return nil
}

func (b *fakeVectorBackend) Search(ctx context.Context, queryVector []float32, limit int) ([]vec.VecSearchResult, error) {
	return b.SearchWithFilter(ctx, queryVector, limit, nil)
}

func (b *fakeVectorBackend) SearchWithFilter(ctx context.Context, queryVector []float32, limit int, metadataFilter map[string]string) ([]vec.VecSearchResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(queryVector) != b.info.Dimensions {
		return nil, fmt.Errorf("query vector dimension mismatch: expected %d, got %d", b.info.Dimensions, len(queryVector))
	}

	results := make([]vec.VecSearchResult, 0, len(b.points))
	for _, item := range b.points {
		if !matchesMetadata(item.Metadata, metadataFilter) {
			continue
		}

		distance := vec.CosineDistance(queryVector, item.Vector)
		if b.info.Distance == string(vec.DistanceL2) {
			distance = vec.L2Distance(queryVector, item.Vector)
		}

		results = append(results, vec.VecSearchResult{
			ID:       item.ID,
			RowID:    item.ID,
			Distance: distance,
			Metadata: stringMapToAny(item.Metadata),
			Aux:      stringMapToAny(item.Aux),
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Distance < results[j].Distance
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

func (b *fakeVectorBackend) Close() error {
	return nil
}

func matchesMetadata(metadata map[string]string, filter map[string]string) bool {
	if len(filter) == 0 {
		return true
	}

	for key, value := range filter {
		if metadata[key] != value {
			return false
		}
	}
	return true
}

func stringMapToAny(values map[string]string) map[string]any {
	if len(values) == 0 {
		return nil
	}

	converted := make(map[string]any, len(values))
	for key, value := range values {
		converted[key] = value
	}
	return converted
}
