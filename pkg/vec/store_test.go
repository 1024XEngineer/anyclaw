package vec

import (
	"context"
	"math"
	"testing"
)

func setupVecStore(t *testing.T) *VecStore {
	t.Helper()

	vs := NewVecStore(VecStoreConfig{
		TableName:   "test_vectors",
		Dimensions:  4,
		Distance:    DistanceCosine,
		Metadata:    []string{"category", "source"},
		PersistPath: t.TempDir(),
	})

	if err := vs.Init(context.Background()); err != nil {
		t.Fatalf("failed to init vec store: %v", err)
	}

	return vs
}

func TestVecStoreInit(t *testing.T) {
	vs := setupVecStore(t)

	version, err := vs.VecVersion(context.Background())
	if err != nil {
		t.Fatalf("failed to get vec version: %v", err)
	}
	if version == "" {
		t.Error("expected non-empty vec version")
	}

	info, err := vs.TableInfo(context.Background())
	if err != nil {
		t.Fatalf("failed to get table info: %v", err)
	}

	if info.TableName != "test_vectors" {
		t.Errorf("expected table name test_vectors, got %s", info.TableName)
	}
	if info.Dimensions != 4 {
		t.Errorf("expected dimensions 4, got %d", info.Dimensions)
	}
	if info.Distance != "cosine" {
		t.Errorf("expected distance cosine, got %s", info.Distance)
	}
}

func TestVecStoreInsert(t *testing.T) {
	vs := setupVecStore(t)
	ctx := context.Background()

	err := vs.Insert(ctx, 1, []float32{0.1, 0.2, 0.3, 0.4}, map[string]string{
		"category": "test",
		"source":   "unit",
	})
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}

	count, err := vs.Count(ctx)
	if err != nil {
		t.Fatalf("failed to count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 vector, got %d", count)
	}
}

func TestVecStoreInsertBatch(t *testing.T) {
	vs := setupVecStore(t)
	ctx := context.Background()

	items := []VecItem{
		{ID: int64(1), Vector: []float32{0.1, 0.2, 0.3, 0.4}, Metadata: map[string]string{"category": "a"}},
		{ID: int64(2), Vector: []float32{0.5, 0.6, 0.7, 0.8}, Metadata: map[string]string{"category": "b"}},
		{ID: int64(3), Vector: []float32{0.9, 1.0, 0.1, 0.2}, Metadata: map[string]string{"category": "a"}},
	}

	if err := vs.InsertBatch(ctx, items); err != nil {
		t.Fatalf("failed to insert batch: %v", err)
	}

	count, err := vs.Count(ctx)
	if err != nil {
		t.Fatalf("failed to count: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 vectors, got %d", count)
	}
}

func TestVecStoreSearch(t *testing.T) {
	vs := setupVecStore(t)
	ctx := context.Background()

	_ = vs.Insert(ctx, 1, []float32{0.1, 0.2, 0.3, 0.4}, nil)
	_ = vs.Insert(ctx, 2, []float32{0.11, 0.21, 0.31, 0.41}, nil)
	_ = vs.Insert(ctx, 3, []float32{0.9, 0.8, 0.7, 0.6}, nil)

	results, err := vs.Search(ctx, []float32{0.1, 0.2, 0.3, 0.4}, 10)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
	if results[0].RowID != 1 {
		t.Errorf("expected first result rowid 1, got %d", results[0].RowID)
	}
	if results[0].Distance > 0.001 {
		t.Errorf("expected first result distance near 0, got %f", results[0].Distance)
	}
}

func TestVecStoreSearchWithThreshold(t *testing.T) {
	vs := setupVecStore(t)
	ctx := context.Background()

	_ = vs.Insert(ctx, 1, []float32{0.5, 0.5, 0.5, 0.5}, nil)
	_ = vs.Insert(ctx, 2, []float32{0.51, 0.51, 0.51, 0.51}, nil)
	_ = vs.Insert(ctx, 3, []float32{0.1, 0.2, 0.3, 0.4}, nil)

	results, err := vs.SearchWithFilter(ctx, []float32{0.5, 0.5, 0.5, 0.5}, 10, 0.01, nil)
	if err != nil {
		t.Fatalf("search with threshold failed: %v", err)
	}

	if len(results) < 1 {
		t.Errorf("expected at least 1 result, got %d", len(results))
	}
	if results[0].Distance > 0.01 {
		t.Errorf("expected distance <= 0.01, got %f", results[0].Distance)
	}
}

func TestVecStoreSearchWithMetadataFilter(t *testing.T) {
	vs := setupVecStore(t)
	ctx := context.Background()

	_ = vs.Insert(ctx, 1, []float32{0.1, 0.2, 0.3, 0.4}, map[string]string{"category": "a"})
	_ = vs.Insert(ctx, 2, []float32{0.1, 0.2, 0.3, 0.41}, map[string]string{"category": "b"})

	results, err := vs.SearchWithFilter(ctx, []float32{0.1, 0.2, 0.3, 0.4}, 10, 0, map[string]string{"category": "b"})
	if err != nil {
		t.Fatalf("search with metadata filter failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 filtered result, got %d", len(results))
	}
	if results[0].RowID != 2 {
		t.Errorf("expected rowid 2, got %d", results[0].RowID)
	}
}

func TestVecStoreGet(t *testing.T) {
	vs := setupVecStore(t)
	ctx := context.Background()

	meta := map[string]string{"category": "test", "source": "unit"}
	raw := []float32{0.1, 0.2, 0.3, 0.4}
	_ = vs.Insert(ctx, 42, raw, meta)

	item, err := vs.Get(ctx, 42)
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}

	if item.RowID != 42 {
		t.Errorf("expected rowid 42, got %d", item.RowID)
	}
	if len(item.Vector) != 4 {
		t.Errorf("expected 4 dimensions, got %d", len(item.Vector))
	}
	assertVectorApproxEqual(t, item.Vector, normalized(raw))
	if item.Metadata["category"] != "test" {
		t.Errorf("expected category test, got %s", item.Metadata["category"])
	}
}

func TestVecStoreDelete(t *testing.T) {
	vs := setupVecStore(t)
	ctx := context.Background()

	_ = vs.Insert(ctx, 1, []float32{0.1, 0.2, 0.3, 0.4}, nil)

	count, _ := vs.Count(ctx)
	if count != 1 {
		t.Fatalf("expected 1 vector before delete")
	}

	if err := vs.Delete(ctx, 1); err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	count, _ = vs.Count(ctx)
	if count != 0 {
		t.Errorf("expected 0 vectors after delete, got %d", count)
	}
}

func TestVecStoreUpdateVector(t *testing.T) {
	vs := setupVecStore(t)
	ctx := context.Background()

	_ = vs.Insert(ctx, 1, []float32{0.1, 0.2, 0.3, 0.4}, nil)

	newVec := []float32{0.9, 0.8, 0.7, 0.6}
	if err := vs.UpdateVector(ctx, 1, newVec); err != nil {
		t.Fatalf("update vector failed: %v", err)
	}

	item, err := vs.Get(ctx, 1)
	if err != nil {
		t.Fatalf("get after update failed: %v", err)
	}

	assertVectorApproxEqual(t, item.Vector, normalized(newVec))
}

func TestVecStoreUpdateMetadata(t *testing.T) {
	vs := setupVecStore(t)
	ctx := context.Background()

	_ = vs.Insert(ctx, 1, []float32{0.1, 0.2, 0.3, 0.4}, map[string]string{
		"category": "old",
		"source":   "old",
	})

	if err := vs.UpdateMetadata(ctx, 1, map[string]string{
		"category": "new",
	}); err != nil {
		t.Fatalf("update metadata failed: %v", err)
	}

	item, err := vs.Get(ctx, 1)
	if err != nil {
		t.Fatalf("get after metadata update failed: %v", err)
	}

	if item.Metadata["category"] != "new" {
		t.Errorf("expected category new, got %s", item.Metadata["category"])
	}
	if item.Metadata["source"] != "old" {
		t.Errorf("expected source old, got %s", item.Metadata["source"])
	}
}

func TestVecStoreList(t *testing.T) {
	vs := setupVecStore(t)
	ctx := context.Background()

	_ = vs.Insert(ctx, 1, []float32{0.1, 0.2, 0.3, 0.4}, nil)
	_ = vs.Insert(ctx, 2, []float32{0.5, 0.6, 0.7, 0.8}, nil)

	items, err := vs.List(ctx, 10)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}

	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
	if items[0].RowID != 1 || items[1].RowID != 2 {
		t.Errorf("expected sorted items by id, got %+v", items)
	}
}

func TestVecStoreDimensionMismatch(t *testing.T) {
	vs := setupVecStore(t)
	ctx := context.Background()

	err := vs.Insert(ctx, 1, []float32{0.1, 0.2}, nil)
	if err == nil {
		t.Error("expected dimension mismatch error")
	}

	if _, err := vs.Search(ctx, []float32{0.1, 0.2}, 10); err == nil {
		t.Error("expected dimension mismatch error on search")
	}
}

func TestVecStoreUnsupportedL2(t *testing.T) {
	vs := NewVecStore(VecStoreConfig{
		TableName:   "l2_vectors",
		Dimensions:  4,
		Distance:    DistanceL2,
		PersistPath: t.TempDir(),
	})

	if err := vs.Init(context.Background()); err == nil {
		t.Fatal("expected unsupported l2 error")
	}
}

func TestCosineSimilarity(t *testing.T) {
	a := []float32{1.0, 0.0, 0.0}
	b := []float32{1.0, 0.0, 0.0}

	sim := CosineSimilarity(a, b)
	if sim < 0.999 {
		t.Errorf("expected similarity ~1.0, got %f", sim)
	}

	c := []float32{0.0, 1.0, 0.0}
	sim = CosineSimilarity(a, c)
	if sim > 0.001 {
		t.Errorf("expected similarity ~0.0, got %f", sim)
	}
}

func TestCosineDistance(t *testing.T) {
	a := []float32{1.0, 0.0, 0.0}
	b := []float32{1.0, 0.0, 0.0}

	dist := CosineDistance(a, b)
	if dist > 0.001 {
		t.Errorf("expected distance ~0.0, got %f", dist)
	}
}

func TestL2Distance(t *testing.T) {
	a := []float32{0.0, 0.0}
	b := []float32{3.0, 4.0}

	dist := L2Distance(a, b)
	if dist < 4.9 || dist > 5.1 {
		t.Errorf("expected distance ~5.0, got %f", dist)
	}
}

func TestVectorBlobRoundTrip(t *testing.T) {
	original := []float32{0.1, 0.2, 0.3, 0.4, 0.5}
	blob := vectorToBlob(original)
	restored := blobToVector(blob)

	if len(restored) != len(original) {
		t.Fatalf("length mismatch: %d vs %d", len(restored), len(original))
	}

	for i := range original {
		if restored[i] != original[i] {
			t.Errorf("index %d: expected %f, got %f", i, original[i], restored[i])
		}
	}
}

func TestVecStoreUpsert(t *testing.T) {
	vs := setupVecStore(t)
	ctx := context.Background()

	_ = vs.Insert(ctx, 1, []float32{0.1, 0.2, 0.3, 0.4}, nil)

	count, _ := vs.Count(ctx)
	if count != 1 {
		t.Fatalf("expected 1 vector after first insert")
	}

	updated := []float32{0.5, 0.6, 0.7, 0.8}
	_ = vs.Insert(ctx, 1, updated, nil)

	count, _ = vs.Count(ctx)
	if count != 1 {
		t.Errorf("expected 1 vector after upsert, got %d", count)
	}

	item, err := vs.Get(ctx, 1)
	if err != nil {
		t.Fatalf("get after upsert failed: %v", err)
	}

	assertVectorApproxEqual(t, item.Vector, normalized(updated))
}

func normalized(v []float32) []float32 {
	out := make([]float32, len(v))
	copy(out, v)

	var sum float64
	for _, value := range out {
		sum += float64(value * value)
	}
	if sum == 0 {
		return out
	}

	norm := float32(math.Sqrt(sum))
	for i := range out {
		out[i] /= norm
	}
	return out
}

func assertVectorApproxEqual(t *testing.T, got, want []float32) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("vector length mismatch: %d vs %d", len(got), len(want))
	}

	for i := range want {
		diff := math.Abs(float64(got[i] - want[i]))
		if diff > 1e-5 {
			t.Fatalf("vector[%d] expected %f, got %f", i, want[i], got[i])
		}
	}
}
