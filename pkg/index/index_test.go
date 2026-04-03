package index

import (
	"context"
	"database/sql"
	"sync/atomic"
	"testing"

	_ "modernc.org/sqlite"
	_ "modernc.org/sqlite/vec"
)

func setupIndexManager(t *testing.T) (*IndexManager, *mockEmbedder) {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	embedder := &mockEmbedder{dim: 4}

	im := NewIndexManager(db, embedder)
	if err := im.Init(context.Background()); err != nil {
		t.Fatalf("init index manager: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
	})

	return im, embedder
}

func TestCreateIndex(t *testing.T) {
	im, _ := setupIndexManager(t)
	ctx := context.Background()

	info, err := im.Create(ctx, Config{
		Name:       "test_index",
		Dimensions: 4,
		Distance:   "cosine",
		Metadata:   []string{"category"},
	})
	if err != nil {
		t.Fatalf("create index: %v", err)
	}

	if info.Name != "test_index" {
		t.Errorf("expected name test_index, got %s", info.Name)
	}
	if info.Status != StatusReady {
		t.Errorf("expected status ready, got %s", info.Status)
	}
	if info.Dimensions != 4 {
		t.Errorf("expected dimensions 4, got %d", info.Dimensions)
	}
}

func TestCreateDuplicateIndex(t *testing.T) {
	im, _ := setupIndexManager(t)
	ctx := context.Background()

	_, err := im.Create(ctx, Config{
		Name:       "dup",
		Dimensions: 4,
	})
	if err != nil {
		t.Fatalf("first create: %v", err)
	}

	_, err = im.Create(ctx, Config{
		Name:       "dup",
		Dimensions: 4,
	})
	if err == nil {
		t.Error("expected error for duplicate index")
	}
}

func TestGetIndex(t *testing.T) {
	im, _ := setupIndexManager(t)
	ctx := context.Background()

	im.Create(ctx, Config{
		Name:       "get_test",
		Dimensions: 4,
		Metadata:   []string{"tag"},
	})

	info, err := im.Get("get_test")
	if err != nil {
		t.Fatalf("get index: %v", err)
	}

	if len(info.Metadata) != 1 || info.Metadata[0] != "tag" {
		t.Errorf("expected metadata [tag], got %v", info.Metadata)
	}
}

func TestListIndexes(t *testing.T) {
	im, _ := setupIndexManager(t)
	ctx := context.Background()

	im.Create(ctx, Config{Name: "idx1", Dimensions: 4})
	im.Create(ctx, Config{Name: "idx2", Dimensions: 4})
	im.Create(ctx, Config{Name: "idx3", Dimensions: 8})

	indexes := im.List()
	if len(indexes) != 3 {
		t.Errorf("expected 3 indexes, got %d", len(indexes))
	}
}

func TestUpdateIndex(t *testing.T) {
	im, _ := setupIndexManager(t)
	ctx := context.Background()

	im.Create(ctx, Config{
		Name:       "update_test",
		Dimensions: 4,
		Metadata:   []string{"old"},
	})

	info, err := im.Update(ctx, "update_test", Config{
		Dimensions: 4,
		Metadata:   []string{"new"},
	})
	if err != nil {
		t.Fatalf("update index: %v", err)
	}

	if len(info.Metadata) != 1 || info.Metadata[0] != "new" {
		t.Errorf("expected metadata [new], got %v", info.Metadata)
	}
}

func TestUpdateIndexDimensionChange(t *testing.T) {
	im, _ := setupIndexManager(t)
	ctx := context.Background()

	im.Create(ctx, Config{Name: "dim_test", Dimensions: 4})

	_, err := im.Update(ctx, "dim_test", Config{
		Dimensions: 8,
	})
	if err == nil {
		t.Error("expected error when changing dimensions")
	}
}

func TestDeleteIndex(t *testing.T) {
	im, _ := setupIndexManager(t)
	ctx := context.Background()

	im.Create(ctx, Config{Name: "del_test", Dimensions: 4})

	indexes := im.List()
	if len(indexes) != 1 {
		t.Fatalf("expected 1 index before delete")
	}

	if err := im.Delete(ctx, "del_test"); err != nil {
		t.Fatalf("delete index: %v", err)
	}

	indexes = im.List()
	if len(indexes) != 0 {
		t.Errorf("expected 0 indexes after delete, got %d", len(indexes))
	}
}

func TestDeleteNonExistentIndex(t *testing.T) {
	im, _ := setupIndexManager(t)
	ctx := context.Background()

	err := im.Delete(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for non-existent index")
	}
}

func TestIndexWithVectors(t *testing.T) {
	im, _ := setupIndexManager(t)
	ctx := context.Background()

	im.Create(ctx, Config{
		Name:       "vec_index",
		Dimensions: 4,
		Distance:   "cosine",
	})

	items := []IndexItem{
		{ID: 1, Vector: []float32{0.1, 0.2, 0.3, 0.4}},
		{ID: 2, Vector: []float32{0.5, 0.6, 0.7, 0.8}},
		{ID: 3, Vector: []float32{0.9, 1.0, 0.1, 0.2}},
	}

	var progressCount atomic.Int32
	result, err := im.Index(ctx, "vec_index", items, func(p Progress) {
		progressCount.Add(1)
	})
	if err != nil {
		t.Fatalf("index: %v", err)
	}

	if result.Indexed != 3 {
		t.Errorf("expected 3 indexed, got %d", result.Indexed)
	}
	if result.Failed != 0 {
		t.Errorf("expected 0 failed, got %d", result.Failed)
	}
	if progressCount.Load() == 0 {
		t.Error("expected progress callback to be called")
	}

	info, _ := im.Get("vec_index")
	if info.VectorCount != 3 {
		t.Errorf("expected vector count 3, got %d", info.VectorCount)
	}
}

func TestIndexWithEmbedding(t *testing.T) {
	im, embedder := setupIndexManager(t)
	ctx := context.Background()

	im.Create(ctx, Config{
		Name:       "embed_index",
		Dimensions: 4,
	})

	items := []IndexItem{
		{ID: 1, Text: "hello world"},
		{ID: 2, Text: "foo bar"},
	}

	result, err := im.Index(ctx, "embed_index", items, nil)
	if err != nil {
		t.Fatalf("index: %v", err)
	}

	if result.Indexed != 2 {
		t.Errorf("expected 2 indexed, got %d", result.Indexed)
	}
	if embedder.callCount.Load() != 2 {
		t.Errorf("expected 2 embed calls, got %d", embedder.callCount.Load())
	}
}

func TestIndexMixedVectorsAndText(t *testing.T) {
	im, embedder := setupIndexManager(t)
	ctx := context.Background()

	im.Create(ctx, Config{
		Name:       "mixed_index",
		Dimensions: 4,
	})

	items := []IndexItem{
		{ID: 1, Vector: []float32{0.1, 0.2, 0.3, 0.4}},
		{ID: 2, Text: "embed me"},
		{ID: 3, Vector: []float32{0.5, 0.6, 0.7, 0.8}},
	}

	result, err := im.Index(ctx, "mixed_index", items, nil)
	if err != nil {
		t.Fatalf("index: %v", err)
	}

	if result.Indexed != 3 {
		t.Errorf("expected 3 indexed, got %d", result.Indexed)
	}
	if embedder.callCount.Load() != 1 {
		t.Errorf("expected 1 embed call, got %d", embedder.callCount.Load())
	}
}

func TestSearch(t *testing.T) {
	im, _ := setupIndexManager(t)
	ctx := context.Background()

	im.Create(ctx, Config{
		Name:       "search_index",
		Dimensions: 4,
		Distance:   "cosine",
	})

	items := []IndexItem{
		{ID: 1, Vector: []float32{0.1, 0.2, 0.3, 0.4}},
		{ID: 2, Vector: []float32{0.11, 0.21, 0.31, 0.41}},
		{ID: 3, Vector: []float32{0.9, 0.8, 0.7, 0.6}},
	}

	im.Index(ctx, "search_index", items, nil)

	results, err := im.Search(ctx, "search_index", []float32{0.1, 0.2, 0.3, 0.4}, 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
}

func TestSearchByText(t *testing.T) {
	im, _ := setupIndexManager(t)
	ctx := context.Background()

	im.Create(ctx, Config{
		Name:       "text_search_index",
		Dimensions: 4,
	})

	items := []IndexItem{
		{ID: 1, Vector: []float32{0.1, 0.2, 0.3, 0.4}},
		{ID: 2, Vector: []float32{0.5, 0.6, 0.7, 0.8}},
	}

	im.Index(ctx, "text_search_index", items, nil)

	results, err := im.SearchByText(ctx, "text_search_index", "query", 10)
	if err != nil {
		t.Fatalf("search by text: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestSearchNonExistentIndex(t *testing.T) {
	im, _ := setupIndexManager(t)
	ctx := context.Background()

	_, err := im.Search(ctx, "nonexistent", []float32{0.1, 0.2}, 10)
	if err == nil {
		t.Error("expected error for non-existent index")
	}
}

func TestRemoveVectors(t *testing.T) {
	im, _ := setupIndexManager(t)
	ctx := context.Background()

	im.Create(ctx, Config{Name: "remove_index", Dimensions: 4})

	items := []IndexItem{
		{ID: 1, Vector: []float32{0.1, 0.2, 0.3, 0.4}},
		{ID: 2, Vector: []float32{0.5, 0.6, 0.7, 0.8}},
		{ID: 3, Vector: []float32{0.9, 1.0, 0.1, 0.2}},
	}

	im.Index(ctx, "remove_index", items, nil)

	removed, err := im.RemoveVectors(ctx, "remove_index", []any{1, 2})
	if err != nil {
		t.Fatalf("remove vectors: %v", err)
	}

	if removed != 2 {
		t.Errorf("expected 2 removed, got %d", removed)
	}

	info, _ := im.Get("remove_index")
	if info.VectorCount != 1 {
		t.Errorf("expected vector count 1, got %d", info.VectorCount)
	}
}

func TestRebuildIndex(t *testing.T) {
	im, _ := setupIndexManager(t)
	ctx := context.Background()

	im.Create(ctx, Config{Name: "rebuild_index", Dimensions: 4})

	items := []IndexItem{
		{ID: 1, Vector: []float32{0.1, 0.2, 0.3, 0.4}},
		{ID: 2, Vector: []float32{0.5, 0.6, 0.7, 0.8}},
	}

	im.Index(ctx, "rebuild_index", items, nil)

	var progressCalled bool
	result, err := im.Rebuild(ctx, "rebuild_index", func(p Progress) {
		progressCalled = true
	})
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	if !progressCalled {
		t.Error("expected progress callback")
	}

	info, _ := im.Get("rebuild_index")
	if info.Status != StatusReady {
		t.Errorf("expected status ready after rebuild, got %s", info.Status)
	}
	if info.VectorCount != 0 {
		t.Errorf("expected vector count 0 after rebuild, got %d", info.VectorCount)
	}

	_ = result
}

func TestRebuildNonExistentIndex(t *testing.T) {
	im, _ := setupIndexManager(t)
	ctx := context.Background()

	_, err := im.Rebuild(ctx, "nonexistent", nil)
	if err == nil {
		t.Error("expected error for non-existent index rebuild")
	}
}

func TestIndexMetaPersistence(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	embedder := &mockEmbedder{dim: 4}
	im := NewIndexManager(db, embedder)

	ctx := context.Background()
	if err := im.Init(ctx); err != nil {
		t.Fatalf("init: %v", err)
	}

	im.Create(ctx, Config{
		Name:       "persist_test",
		Dimensions: 4,
		Distance:   "cosine",
		Metadata:   []string{"tag1", "tag2"},
	})

	im2 := NewIndexManager(db, embedder)
	if err := im2.Init(ctx); err != nil {
		t.Fatalf("re-init: %v", err)
	}

	info, err := im2.Get("persist_test")
	if err != nil {
		t.Fatalf("get persisted index: %v", err)
	}

	if info.Dimensions != 4 {
		t.Errorf("expected dimensions 4, got %d", info.Dimensions)
	}
	if info.Distance != "cosine" {
		t.Errorf("expected distance cosine, got %s", info.Distance)
	}
	if len(info.Metadata) != 2 {
		t.Errorf("expected 2 metadata fields, got %d", len(info.Metadata))
	}
}

func TestIndexNotReady(t *testing.T) {
	im, _ := setupIndexManager(t)
	ctx := context.Background()

	im.Create(ctx, Config{Name: "not_ready", Dimensions: 4})

	im.mu.Lock()
	im.indexes["not_ready"].Status = StatusError
	im.mu.Unlock()

	_, err := im.Index(ctx, "not_ready", []IndexItem{{ID: 1, Vector: []float32{0.1, 0.2, 0.3, 0.4}}}, nil)
	if err == nil {
		t.Error("expected error for non-ready index")
	}
}

type mockEmbedder struct {
	dim       int
	callCount atomic.Int32
}

func (m *mockEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	m.callCount.Add(1)
	result := make([]float32, m.dim)
	for i := range result {
		result[i] = float32(len(text)) / float32(m.dim)
	}
	return result, nil
}

func (m *mockEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	var results [][]float32
	for _, text := range texts {
		emb, err := m.Embed(ctx, text)
		if err != nil {
			return nil, err
		}
		results = append(results, emb)
	}
	return results, nil
}

func (m *mockEmbedder) Name() string   { return "mock" }
func (m *mockEmbedder) Dimension() int { return m.dim }
