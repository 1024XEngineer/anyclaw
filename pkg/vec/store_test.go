package vec

import (
	"context"
	"math"
	"strings"
	"testing"

	"github.com/1024XEngineer/anyclaw/pkg/sqlite"
	chromem "github.com/philippgille/chromem-go"
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

func TestNewVecStoreDefaultsAndSharedInMemoryDB(t *testing.T) {
	db, err := sqlite.Open(sqlite.InMemoryConfig())
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	cfg := VecStoreConfig{
		DB:         db.DB,
		TableName:  "shared_vectors",
		Dimensions: 2,
	}

	vs1 := NewVecStore(cfg)
	if vs1.distance != DistanceCosine {
		t.Fatalf("expected default distance cosine, got %q", vs1.distance)
	}
	if got := vs1.sharedDBKey(); !strings.HasPrefix(got, "sql:") {
		t.Fatalf("expected shared db key, got %q", got)
	}

	ctx := context.Background()
	if err := vs1.Init(ctx); err != nil {
		t.Fatalf("init shared store: %v", err)
	}
	if err := vs1.Insert(ctx, "doc-1", []float32{1, 0}, map[string]string{"category": "docs"}); err != nil {
		t.Fatalf("insert shared doc: %v", err)
	}

	vs2 := NewVecStore(cfg)
	if vs2.sharedDBKey() != vs1.sharedDBKey() {
		t.Fatalf("expected shared db keys to match, got %q vs %q", vs2.sharedDBKey(), vs1.sharedDBKey())
	}

	item, err := vs2.Get(ctx, "doc-1")
	if err != nil {
		t.Fatalf("get from second shared store: %v", err)
	}
	if item.RowID != 0 {
		t.Fatalf("expected string id rowid 0, got %d", item.RowID)
	}
	if item.ID != "doc-1" {
		t.Fatalf("expected string id doc-1, got %#v", item.ID)
	}
	if item.Metadata["category"] != "docs" {
		t.Fatalf("expected metadata to round-trip through shared db, got %+v", item.Metadata)
	}
}

func TestVecStorePersistentReopenAndDrop(t *testing.T) {
	path := t.TempDir()
	cfg := VecStoreConfig{
		TableName:   "persist_vectors",
		Dimensions:  2,
		PersistPath: path,
	}

	ctx := context.Background()
	vs1 := NewVecStore(cfg)
	if got := vs1.sharedDBKey(); got != "" {
		t.Fatalf("expected no shared db key for persistent store, got %q", got)
	}
	if err := vs1.Init(ctx); err != nil {
		t.Fatalf("init persistent store: %v", err)
	}
	if err := vs1.Insert(ctx, 1, []float32{1, 0}, nil); err != nil {
		t.Fatalf("insert persistent doc: %v", err)
	}

	vs2 := NewVecStore(cfg)
	count, err := vs2.Count(ctx)
	if err != nil {
		t.Fatalf("count after reopen: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 vector after reopen, got %d", count)
	}

	if err := vs2.Drop(ctx); err != nil {
		t.Fatalf("drop persistent collection: %v", err)
	}

	vs3 := NewVecStore(cfg)
	if _, err := vs3.Count(ctx); err == nil {
		t.Fatal("expected missing collection error after drop")
	}
}

func TestVecStoreValidationErrors(t *testing.T) {
	ctx := context.Background()

	if err := NewVecStore(VecStoreConfig{
		TableName:   " ",
		Dimensions:  2,
		PersistPath: t.TempDir(),
	}).Init(ctx); err == nil {
		t.Fatal("expected blank table name to fail")
	}

	if err := NewVecStore(VecStoreConfig{
		TableName:   "bad_dims",
		Dimensions:  0,
		PersistPath: t.TempDir(),
	}).Init(ctx); err == nil {
		t.Fatal("expected non-positive dimensions to fail")
	}

	if err := NewVecStore(VecStoreConfig{
		TableName:   "bad_distance",
		Dimensions:  2,
		Distance:    DistanceMetric("dot"),
		PersistPath: t.TempDir(),
	}).Init(ctx); err == nil {
		t.Fatal("expected unsupported custom distance to fail")
	}

	vs := NewVecStore(VecStoreConfig{
		TableName:   "validation_vectors",
		Dimensions:  2,
		PersistPath: t.TempDir(),
	})
	if err := vs.Init(ctx); err != nil {
		t.Fatalf("init validation store: %v", err)
	}

	if err := vs.Insert(ctx, nil, []float32{1, 0}, nil); err == nil {
		t.Fatal("expected nil id to fail")
	}
	if err := vs.Insert(ctx, "   ", []float32{1, 0}, nil); err == nil {
		t.Fatal("expected blank string id to fail")
	}
	if err := vs.InsertBatch(ctx, []VecItem{{ID: 1, Vector: []float32{1}}}); err == nil {
		t.Fatal("expected insert batch dimension mismatch to fail")
	}

	if err := NewVecStore(VecStoreConfig{
		TableName:   " ",
		Dimensions:  2,
		PersistPath: t.TempDir(),
	}).Drop(ctx); err == nil {
		t.Fatal("expected drop with blank table name to fail")
	}

	if _, err := NewVecStore(VecStoreConfig{
		TableName:   "missing_vectors",
		Dimensions:  2,
		PersistPath: t.TempDir(),
	}).Count(ctx); err == nil {
		t.Fatal("expected missing collection count to fail")
	}
}

func TestVecHelpers(t *testing.T) {
	if got := sanitizeMetadata(nil, nil); got != nil {
		t.Fatalf("expected nil metadata when nothing provided, got %+v", got)
	}

	meta := map[string]string{"keep": "yes", "drop": "no"}
	cloned := sanitizeMetadata(nil, meta)
	cloned["keep"] = "changed"
	if meta["keep"] != "yes" {
		t.Fatalf("expected sanitizeMetadata to clone unrestricted metadata, got %+v", meta)
	}

	filtered := sanitizeMetadata([]string{"keep", "missing"}, meta)
	if filtered["keep"] != "yes" || filtered["missing"] != "" {
		t.Fatalf("unexpected filtered metadata: %+v", filtered)
	}

	if cloneVector(nil) != nil {
		t.Fatal("expected cloneVector(nil) to be nil")
	}
	vector := []float32{3, 4}
	vectorClone := cloneVector(vector)
	vectorClone[0] = 99
	if vector[0] != 3 {
		t.Fatalf("expected cloneVector to copy data, got %+v", vector)
	}

	if cloneMetadata(nil) != nil {
		t.Fatal("expected cloneMetadata(nil) to be nil")
	}
	metadataClone := cloneMetadata(meta)
	metadataClone["keep"] = "changed"
	if meta["keep"] != "yes" {
		t.Fatalf("expected cloneMetadata to copy data, got %+v", meta)
	}

	if metadataToAny(nil) != nil {
		t.Fatal("expected metadataToAny(nil) to be nil")
	}
	if anyMeta := metadataToAny(map[string]string{"kind": "doc"}); anyMeta["kind"] != "doc" {
		t.Fatalf("expected metadataToAny to preserve values, got %+v", anyMeta)
	}

	if _, err := normalizeID(nil); err == nil {
		t.Fatal("expected nil id normalization to fail")
	}
	if _, err := normalizeID("   "); err == nil {
		t.Fatal("expected blank string normalization to fail")
	}
	if got, err := normalizeID(7); err != nil || got != "7" {
		t.Fatalf("expected numeric id to normalize to 7, got %q / %v", got, err)
	}

	rowID, id := decodeID("9")
	if rowID != 9 || id != int64(9) {
		t.Fatalf("expected numeric decode to return rowid 9/int64(9), got %d/%#v", rowID, id)
	}
	rowID, id = decodeID("doc-1")
	if rowID != 0 || id != "doc-1" {
		t.Fatalf("expected string decode to keep string id, got %d/%#v", rowID, id)
	}

	raw := []float32{3, 4}
	doc := chromem.Document{
		ID:        "doc-2",
		Embedding: raw,
		Metadata:  map[string]string{"kind": "doc"},
	}
	item := vecItemFromDocument(doc)
	raw[0] = 99
	if item.RowID != 0 || item.ID != "doc-2" {
		t.Fatalf("unexpected vec item identity: %+v", item)
	}
	if item.Vector[0] == 99 {
		t.Fatalf("expected vecItemFromDocument to clone embedding, got %+v", item.Vector)
	}

	if sim := CosineSimilarity([]float32{1}, []float32{1, 2}); sim != 0 {
		t.Fatalf("expected cosine similarity 0 for mismatched vectors, got %f", sim)
	}
	if sim := CosineSimilarity([]float32{0, 0}, []float32{0, 0}); sim != 0 {
		t.Fatalf("expected cosine similarity 0 for zero vectors, got %f", sim)
	}
	if dist := L2Distance([]float32{1}, []float32{1, 2}); dist != math.MaxFloat64 {
		t.Fatalf("expected max float for mismatched L2 distance, got %f", dist)
	}

	if err := (&VecStore{dimensions: 0, distance: DistanceCosine}).validateVector([]float32{1}); err == nil {
		t.Fatal("expected validateVector to fail when dimensions are non-positive")
	}
	if err := (&VecStore{dimensions: 2, distance: DistanceMetric("dot")}).validateVector([]float32{1, 0}); err == nil {
		t.Fatal("expected validateVector to fail for unsupported distance")
	}
}

func TestVecStoreOperationErrorPaths(t *testing.T) {
	ctx := context.Background()
	vs := NewVecStore(VecStoreConfig{
		TableName:   "ops_vectors",
		Dimensions:  2,
		Metadata:    []string{"category"},
		PersistPath: t.TempDir(),
	})
	if err := vs.Init(ctx); err != nil {
		t.Fatalf("init ops store: %v", err)
	}

	if err := vs.InsertBatch(ctx, nil); err != nil {
		t.Fatalf("expected empty batch insert to succeed, got %v", err)
	}
	if err := vs.InsertBatch(ctx, []VecItem{{ID: "   ", Vector: []float32{1, 0}}}); err == nil {
		t.Fatal("expected blank batch id to fail")
	}

	results, err := vs.SearchWithFilter(ctx, []float32{1, 0}, 0, 0, nil)
	if err != nil {
		t.Fatalf("search empty store: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected no results from empty store, got %d", len(results))
	}

	if _, err := vs.Get(ctx, "   "); err == nil {
		t.Fatal("expected blank get id to fail")
	}
	if _, err := vs.Get(ctx, "missing"); err == nil {
		t.Fatal("expected missing document get to fail")
	}

	if err := vs.Delete(ctx, "   "); err == nil {
		t.Fatal("expected blank delete id to fail")
	}

	if err := vs.UpdateVector(ctx, 1, []float32{1}); err == nil {
		t.Fatal("expected update vector dimension mismatch to fail")
	}
	if err := vs.UpdateVector(ctx, "missing", []float32{1, 0}); err == nil {
		t.Fatal("expected update vector for missing id to fail")
	}

	if err := vs.UpdateMetadata(ctx, 1, nil); err != nil {
		t.Fatalf("expected empty metadata update to be a no-op, got %v", err)
	}

	if err := vs.Insert(ctx, 1, []float32{1, 0}, nil); err != nil {
		t.Fatalf("insert metadata-less doc: %v", err)
	}
	if err := vs.UpdateMetadata(ctx, 1, map[string]string{"category": "docs"}); err != nil {
		t.Fatalf("update metadata on nil metadata doc: %v", err)
	}

	item, err := vs.Get(ctx, 1)
	if err != nil {
		t.Fatalf("get metadata-updated doc: %v", err)
	}
	if item.Metadata["category"] != "docs" {
		t.Fatalf("expected metadata update to populate nil map, got %+v", item.Metadata)
	}
}

func TestVecStoreListDropAndVersionErrorPaths(t *testing.T) {
	ctx := context.Background()

	missing := NewVecStore(VecStoreConfig{
		TableName:   "missing_info",
		Dimensions:  2,
		PersistPath: t.TempDir(),
	})
	if _, err := missing.VecVersion(ctx); err == nil {
		t.Fatal("expected VecVersion on missing collection to fail")
	}
	if _, err := missing.TableInfo(ctx); err == nil {
		t.Fatal("expected TableInfo on missing collection to fail")
	}
	if err := missing.Delete(ctx, 1); err == nil {
		t.Fatal("expected Delete on missing collection to fail")
	}

	empty := NewVecStore(VecStoreConfig{
		TableName:   "empty_list",
		Dimensions:  2,
		PersistPath: t.TempDir(),
	})
	if err := empty.Init(ctx); err != nil {
		t.Fatalf("init empty list store: %v", err)
	}
	items, err := empty.List(ctx, 1)
	if err != nil {
		t.Fatalf("list empty store: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected empty list, got %+v", items)
	}

	cfg := VecStoreConfig{
		TableName:   "drop_vectors",
		Dimensions:  2,
		PersistPath: t.TempDir(),
	}
	creator := NewVecStore(cfg)
	if err := creator.Init(ctx); err != nil {
		t.Fatalf("init drop store: %v", err)
	}
	if err := creator.Insert(ctx, 1, []float32{1, 0}, nil); err != nil {
		t.Fatalf("insert drop store doc: %v", err)
	}

	limited, err := creator.List(ctx, 1)
	if err != nil {
		t.Fatalf("list with limit: %v", err)
	}
	if len(limited) != 1 {
		t.Fatalf("expected limited list to return 1 item, got %d", len(limited))
	}

	dropper := NewVecStore(cfg)
	if err := dropper.Drop(ctx); err != nil {
		t.Fatalf("drop existing collection with fresh store: %v", err)
	}
	if err := dropper.Drop(ctx); err != nil {
		t.Fatalf("expected repeat drop to remain safe, got %v", err)
	}
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
