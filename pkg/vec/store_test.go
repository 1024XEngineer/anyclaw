package vec

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/qdrant/go-client/qdrant"
)

func setupVecStore(t *testing.T, distance DistanceMetric) (*VecStore, *fakeQdrantClient) {
	t.Helper()

	client := newFakeQdrantClient("1.15.3")
	vs := NewVecStore(VecStoreConfig{
		TableName:  "test_vectors",
		Dimensions: 4,
		Distance:   distance,
		Metadata:   []string{"category", "source"},
		AuxColumns: []string{"tag"},
	})
	vs.client = client

	if err := vs.Init(context.Background()); err != nil {
		t.Fatalf("failed to init vec store: %v", err)
	}

	return vs, client
}

func TestVecStoreInitAndTableInfo(t *testing.T) {
	vs, client := setupVecStore(t, DistanceCosine)

	if client.createCollectionCalls != 1 {
		t.Fatalf("expected one collection creation, got %d", client.createCollectionCalls)
	}

	version, err := vs.VecVersion(context.Background())
	if err != nil {
		t.Fatalf("failed to get vec version: %v", err)
	}
	if version != "1.15.3" {
		t.Fatalf("expected qdrant version 1.15.3, got %q", version)
	}

	info, err := vs.TableInfo(context.Background())
	if err != nil {
		t.Fatalf("failed to get table info: %v", err)
	}

	if info.TableName != "test_vectors" {
		t.Fatalf("expected table name test_vectors, got %q", info.TableName)
	}
	if info.Dimensions != 4 {
		t.Fatalf("expected dimensions 4, got %d", info.Dimensions)
	}
	if info.Distance != string(DistanceCosine) {
		t.Fatalf("expected cosine distance, got %q", info.Distance)
	}
	if info.VectorCount != 0 {
		t.Fatalf("expected zero vectors, got %d", info.VectorCount)
	}
	if info.VecVersion != "1.15.3" {
		t.Fatalf("expected version 1.15.3, got %q", info.VecVersion)
	}
}

func TestVecStoreInitRejectsCollectionMismatch(t *testing.T) {
	client := newFakeQdrantClient("1.15.3")
	client.collections["test_vectors"] = newFakeCollection("test_vectors", 8, qdrant.Distance_Cosine)

	vs := NewVecStore(VecStoreConfig{
		TableName:  "test_vectors",
		Dimensions: 4,
		Distance:   DistanceCosine,
	})
	vs.client = client

	err := vs.Init(context.Background())
	if err == nil {
		t.Fatal("expected mismatched collection init to fail")
	}
	if !strings.Contains(err.Error(), "dimensions mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVecStoreEnsurePayloadIndexes(t *testing.T) {
	vs, client := setupVecStore(t, DistanceCosine)

	if err := vs.EnsurePayloadIndexes(context.Background()); err != nil {
		t.Fatalf("ensure payload indexes failed: %v", err)
	}

	collection := client.collections["test_vectors"]
	for _, field := range []string{"category", "source", "tag"} {
		fieldType, ok := collection.fieldIndexes[field]
		if !ok {
			t.Fatalf("expected payload index for %q", field)
		}
		if fieldType != qdrant.FieldType_FieldTypeKeyword {
			t.Fatalf("expected keyword index for %q, got %v", field, fieldType)
		}
	}
}

func TestVecStoreDropDeletesCollection(t *testing.T) {
	vs, client := setupVecStore(t, DistanceCosine)

	if err := vs.Drop(context.Background()); err != nil {
		t.Fatalf("drop collection failed: %v", err)
	}

	if _, ok := client.collections["test_vectors"]; ok {
		t.Fatal("expected collection to be deleted")
	}
}

func TestVecStoreInsertGetUpdateDeleteAndCount(t *testing.T) {
	vs, _ := setupVecStore(t, DistanceCosine)
	ctx := context.Background()

	err := vs.InsertWithAux(ctx, 42, []float32{0.1, 0.2, 0.3, 0.4}, map[string]string{
		"category": "test",
		"source":   "unit",
	}, map[string]string{
		"tag": "primary",
	})
	if err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	count, err := vs.Count(ctx)
	if err != nil {
		t.Fatalf("count failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 vector, got %d", count)
	}

	item, err := vs.Get(ctx, 42)
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if item.RowID != 42 || item.ID != 42 {
		t.Fatalf("expected row id 42, got rowid=%d id=%d", item.RowID, item.ID)
	}
	if item.Metadata["category"] != "test" || item.Metadata["source"] != "unit" {
		t.Fatalf("unexpected metadata: %#v", item.Metadata)
	}
	if item.Aux["tag"] != "primary" {
		t.Fatalf("unexpected aux data: %#v", item.Aux)
	}

	if err := vs.UpdateVector(ctx, 42, []float32{0.9, 0.8, 0.7, 0.6}); err != nil {
		t.Fatalf("update vector failed: %v", err)
	}
	if err := vs.UpdateMetadata(ctx, 42, map[string]string{"category": "updated"}); err != nil {
		t.Fatalf("update metadata failed: %v", err)
	}

	item, err = vs.Get(ctx, 42)
	if err != nil {
		t.Fatalf("get after update failed: %v", err)
	}
	if item.Vector[0] != 0.9 {
		t.Fatalf("expected updated vector, got %v", item.Vector)
	}
	if item.Metadata["category"] != "updated" {
		t.Fatalf("expected updated metadata, got %#v", item.Metadata)
	}
	if item.Metadata["source"] != "unit" {
		t.Fatalf("expected untouched metadata field, got %#v", item.Metadata)
	}

	if err := vs.Delete(ctx, 42); err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	count, err = vs.Count(ctx)
	if err != nil {
		t.Fatalf("count after delete failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 vectors after delete, got %d", count)
	}
}

func TestVecStoreInsertBatchAndList(t *testing.T) {
	vs, _ := setupVecStore(t, DistanceCosine)
	ctx := context.Background()

	err := vs.InsertBatch(ctx, []VecItem{
		{
			ID:       1,
			Vector:   []float32{0.1, 0.2, 0.3, 0.4},
			Metadata: map[string]string{"category": "a", "source": "batch"},
			Aux:      map[string]string{"tag": "first"},
		},
		{
			ID:       2,
			Vector:   []float32{0.4, 0.3, 0.2, 0.1},
			Metadata: map[string]string{"category": "b", "source": "batch"},
			Aux:      map[string]string{"tag": "second"},
		},
	})
	if err != nil {
		t.Fatalf("insert batch failed: %v", err)
	}

	items, err := vs.List(ctx, 10)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].ID != 1 || items[1].ID != 2 {
		t.Fatalf("expected ordered ids [1 2], got [%d %d]", items[0].ID, items[1].ID)
	}
	if items[1].Aux["tag"] != "second" {
		t.Fatalf("expected aux payload from batch insert, got %#v", items[1].Aux)
	}
}

func TestVecStoreListAllWithPagination(t *testing.T) {
	vs, _ := setupVecStore(t, DistanceCosine)
	ctx := context.Background()

	items := make([]VecItem, 0, defaultScrollLimit+5)
	for i := 1; i <= defaultScrollLimit+5; i++ {
		items = append(items, VecItem{
			ID:       int64(i),
			Vector:   []float32{float32(i), 0, 0, 0},
			Metadata: map[string]string{"category": "bulk", "source": "test"},
			Aux:      map[string]string{"tag": fmt.Sprintf("item-%d", i)},
		})
	}

	if err := vs.InsertBatch(ctx, items); err != nil {
		t.Fatalf("insert batch failed: %v", err)
	}

	listed, err := vs.List(ctx, 0)
	if err != nil {
		t.Fatalf("list all failed: %v", err)
	}
	if len(listed) != len(items) {
		t.Fatalf("expected %d listed items, got %d", len(items), len(listed))
	}
	if listed[len(listed)-1].ID != int64(defaultScrollLimit+5) {
		t.Fatalf("expected last id %d, got %d", defaultScrollLimit+5, listed[len(listed)-1].ID)
	}
}

func TestVecStoreSearchWithZeroThresholdReturnsExactMatchesOnly(t *testing.T) {
	tests := []struct {
		name     string
		distance DistanceMetric
		match    []float32
		other    []float32
	}{
		{
			name:     "cosine",
			distance: DistanceCosine,
			match:    []float32{1, 0, 0, 0},
			other:    []float32{0, 1, 0, 0},
		},
		{
			name:     "l2",
			distance: DistanceL2,
			match:    []float32{1, 1, 1, 1},
			other:    []float32{2, 2, 2, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vs, _ := setupVecStore(t, tt.distance)
			ctx := context.Background()

			if err := vs.Insert(ctx, 1, tt.match, nil); err != nil {
				t.Fatalf("insert exact match failed: %v", err)
			}
			if err := vs.Insert(ctx, 2, tt.other, nil); err != nil {
				t.Fatalf("insert non-match failed: %v", err)
			}

			results, err := vs.SearchWithFilter(ctx, tt.match, 10, 0, nil)
			if err != nil {
				t.Fatalf("search with zero threshold failed: %v", err)
			}
			if len(results) != 1 {
				t.Fatalf("expected 1 exact match, got %d", len(results))
			}
			if results[0].ID != 1 {
				t.Fatalf("expected exact match id 1, got %d", results[0].ID)
			}
			if results[0].Distance != 0 {
				t.Fatalf("expected exact match distance 0, got %f", results[0].Distance)
			}

			all, err := vs.Search(ctx, tt.match, 10)
			if err != nil {
				t.Fatalf("unfiltered search failed: %v", err)
			}
			if len(all) != 2 {
				t.Fatalf("expected 2 results without threshold, got %d", len(all))
			}
		})
	}
}

func TestVecStoreSearchWithMetadataFilterAndAux(t *testing.T) {
	vs, _ := setupVecStore(t, DistanceCosine)
	ctx := context.Background()

	if err := vs.InsertWithAux(ctx, 1, []float32{0.1, 0.2, 0.3, 0.4}, map[string]string{
		"category": "keep",
		"source":   "unit",
	}, map[string]string{
		"tag": "visible",
	}); err != nil {
		t.Fatalf("insert item 1 failed: %v", err)
	}
	if err := vs.InsertWithAux(ctx, 2, []float32{0.1, 0.2, 0.3, 0.4}, map[string]string{
		"category": "skip",
		"source":   "unit",
	}, map[string]string{
		"tag": "hidden",
	}); err != nil {
		t.Fatalf("insert item 2 failed: %v", err)
	}

	results, err := vs.SearchWithFilter(ctx, []float32{0.1, 0.2, 0.3, 0.4}, 10, 0, map[string]string{
		"category": "keep",
	})
	if err != nil {
		t.Fatalf("search with metadata filter failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != 1 {
		t.Fatalf("expected id 1, got %d", results[0].ID)
	}
	if results[0].Metadata["category"] != "keep" {
		t.Fatalf("expected category keep, got %#v", results[0].Metadata)
	}
	if results[0].Aux["tag"] != "visible" {
		t.Fatalf("expected aux tag visible, got %#v", results[0].Aux)
	}
}

func TestVecStoreSearchRejectsUnknownMetadataFilter(t *testing.T) {
	vs, _ := setupVecStore(t, DistanceCosine)

	_, err := vs.SearchWithFilter(context.Background(), []float32{0.1, 0.2, 0.3, 0.4}, 10, 0, map[string]string{
		"unknown": "value",
	})
	if err == nil {
		t.Fatal("expected unknown metadata filter to fail")
	}
	if !strings.Contains(err.Error(), `unknown metadata filter column "unknown"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVecStoreRejectsNonPositiveIDs(t *testing.T) {
	vs, _ := setupVecStore(t, DistanceCosine)
	ctx := context.Background()

	for _, id := range []int64{0, -1} {
		err := vs.Insert(ctx, id, []float32{0.1, 0.2, 0.3, 0.4}, nil)
		if err == nil {
			t.Fatalf("expected id %d to fail", id)
		}
		if !strings.Contains(err.Error(), "id must be greater than 0") {
			t.Fatalf("unexpected error for id %d: %v", id, err)
		}
	}
}

func TestVecStoreValidationErrorsDoNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("expected validation error, got panic: %v", r)
		}
	}()

	vs := NewVecStore(VecStoreConfig{
		TableName:  "vectors",
		Dimensions: 4,
		Distance:   DistanceCosine,
	})

	_, err := vs.VecVersion(context.Background())
	if err == nil {
		t.Fatal("expected VecVersion validation error")
	}
	if !strings.Contains(err.Error(), "qdrant client or host must be configured") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVecStoreRejectsInvalidConfig(t *testing.T) {
	client := newFakeQdrantClient("1.15.3")

	tests := []struct {
		name string
		cfg  VecStoreConfig
		want string
	}{
		{
			name: "invalid table name",
			cfg: VecStoreConfig{
				TableName:  "bad-name",
				Dimensions: 4,
				Distance:   DistanceCosine,
			},
			want: `invalid table name "bad-name"`,
		},
		{
			name: "unsupported distance metric",
			cfg: VecStoreConfig{
				TableName:  "vectors",
				Dimensions: 4,
				Distance:   DistanceMetric("dot"),
			},
			want: `unsupported distance metric "dot"`,
		},
		{
			name: "invalid metadata column",
			cfg: VecStoreConfig{
				TableName:  "vectors",
				Dimensions: 4,
				Distance:   DistanceCosine,
				Metadata:   []string{"bad-name"},
			},
			want: `invalid metadata column "bad-name"`,
		},
		{
			name: "duplicate columns",
			cfg: VecStoreConfig{
				TableName:  "vectors",
				Dimensions: 4,
				Distance:   DistanceCosine,
				Metadata:   []string{"shared"},
				AuxColumns: []string{"shared"},
			},
			want: `duplicate column "shared" conflicts with metadata column`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vs := NewVecStore(tt.cfg)
			vs.client = client

			_, err := vs.Count(context.Background())
			if err == nil {
				t.Fatal("expected config validation to fail")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestVecStoreRejectsUnknownColumns(t *testing.T) {
	vs, _ := setupVecStore(t, DistanceCosine)
	ctx := context.Background()

	err := vs.Insert(ctx, 1, []float32{0.1, 0.2, 0.3, 0.4}, map[string]string{
		"category": "ok",
		"unknown":  "bad",
	})
	if err == nil {
		t.Fatal("expected unknown metadata column to fail")
	}
	if !strings.Contains(err.Error(), `unknown metadata column "unknown"`) {
		t.Fatalf("unexpected metadata error: %v", err)
	}

	err = vs.InsertWithAux(ctx, 1, []float32{0.1, 0.2, 0.3, 0.4}, map[string]string{
		"category": "ok",
	}, map[string]string{
		"ghost": "bad",
	})
	if err == nil {
		t.Fatal("expected unknown aux column to fail")
	}
	if !strings.Contains(err.Error(), `unknown aux column "ghost"`) {
		t.Fatalf("unexpected aux error: %v", err)
	}
}

func TestVecStoreUpdateMetadataRejectsUnknownKeys(t *testing.T) {
	vs, _ := setupVecStore(t, DistanceCosine)
	ctx := context.Background()

	if err := vs.Insert(ctx, 1, []float32{0.1, 0.2, 0.3, 0.4}, map[string]string{
		"category": "old",
		"source":   "unit",
	}); err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	err := vs.UpdateMetadata(ctx, 1, map[string]string{
		"unknown": "bad",
	})
	if err == nil {
		t.Fatal("expected unknown update key to fail")
	}
	if !strings.Contains(err.Error(), `unknown metadata column "unknown"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCosineSimilarity(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{1, 0, 0}
	c := []float32{0, 1, 0}

	if got := CosineSimilarity(a, b); got < 0.999 {
		t.Fatalf("expected similarity close to 1, got %f", got)
	}
	if got := CosineSimilarity(a, c); got > 0.001 {
		t.Fatalf("expected similarity close to 0, got %f", got)
	}
}

func TestCosineDistance(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{1, 0, 0}

	if got := CosineDistance(a, b); got != 0 {
		t.Fatalf("expected zero distance, got %f", got)
	}
}

func TestL2Distance(t *testing.T) {
	a := []float32{0, 0}
	b := []float32{3, 4}

	if got := L2Distance(a, b); got < 4.9 || got > 5.1 {
		t.Fatalf("expected distance close to 5, got %f", got)
	}
}

type fakeQdrantClient struct {
	collections           map[string]*fakeCollection
	healthVersion         string
	createCollectionCalls int
}

type fakeCollection struct {
	name         string
	size         uint64
	distance     qdrant.Distance
	points       map[int64]*fakePoint
	fieldIndexes map[string]qdrant.FieldType
}

type fakePoint struct {
	id      int64
	vector  []float32
	payload map[string]any
}

func newFakeQdrantClient(version string) *fakeQdrantClient {
	return &fakeQdrantClient{
		collections:   make(map[string]*fakeCollection),
		healthVersion: version,
	}
}

func newFakeCollection(name string, size uint64, distance qdrant.Distance) *fakeCollection {
	return &fakeCollection{
		name:         name,
		size:         size,
		distance:     distance,
		points:       make(map[int64]*fakePoint),
		fieldIndexes: make(map[string]qdrant.FieldType),
	}
}

func (c *fakeQdrantClient) CollectionExists(_ context.Context, collectionName string) (bool, error) {
	_, ok := c.collections[collectionName]
	return ok, nil
}

func (c *fakeQdrantClient) GetCollectionInfo(_ context.Context, collectionName string) (*qdrant.CollectionInfo, error) {
	collection, err := c.collection(collectionName)
	if err != nil {
		return nil, err
	}

	return &qdrant.CollectionInfo{
		Config: &qdrant.CollectionConfig{
			Params: &qdrant.CollectionParams{
				VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
					Size:     collection.size,
					Distance: collection.distance,
				}),
			},
		},
		PointsCount: qdrant.PtrOf(uint64(len(collection.points))),
	}, nil
}

func (c *fakeQdrantClient) CreateCollection(_ context.Context, request *qdrant.CreateCollection) error {
	if _, ok := c.collections[request.GetCollectionName()]; ok {
		return fmt.Errorf("collection already exists")
	}

	params := request.GetVectorsConfig().GetParams()
	if params == nil {
		return fmt.Errorf("named vector collections are not supported")
	}

	c.collections[request.GetCollectionName()] = newFakeCollection(
		request.GetCollectionName(),
		params.GetSize(),
		params.GetDistance(),
	)
	c.createCollectionCalls++
	return nil
}

func (c *fakeQdrantClient) DeleteCollection(_ context.Context, collectionName string) error {
	if _, ok := c.collections[collectionName]; !ok {
		return nil
	}
	delete(c.collections, collectionName)
	return nil
}

func (c *fakeQdrantClient) CreateFieldIndex(_ context.Context, request *qdrant.CreateFieldIndexCollection) (*qdrant.UpdateResult, error) {
	collection, err := c.collection(request.GetCollectionName())
	if err != nil {
		return nil, err
	}

	collection.fieldIndexes[request.GetFieldName()] = request.GetFieldType()
	return &qdrant.UpdateResult{}, nil
}

func (c *fakeQdrantClient) Upsert(_ context.Context, request *qdrant.UpsertPoints) (*qdrant.UpdateResult, error) {
	collection, err := c.collection(request.GetCollectionName())
	if err != nil {
		return nil, err
	}

	for _, point := range request.GetPoints() {
		id, err := extractPointID(point.GetId())
		if err != nil {
			return nil, err
		}
		vector, err := extractDenseInputVector(point.GetVectors())
		if err != nil {
			return nil, err
		}
		if uint64(len(vector)) != collection.size {
			return nil, fmt.Errorf("vector dimension mismatch: expected %d, got %d", collection.size, len(vector))
		}
		payload, err := payloadMapToAny(point.GetPayload())
		if err != nil {
			return nil, err
		}
		collection.points[id] = &fakePoint{
			id:      id,
			vector:  vector,
			payload: payload,
		}
	}

	return &qdrant.UpdateResult{}, nil
}

func (c *fakeQdrantClient) Get(_ context.Context, request *qdrant.GetPoints) ([]*qdrant.RetrievedPoint, error) {
	collection, err := c.collection(request.GetCollectionName())
	if err != nil {
		return nil, err
	}

	points := make([]*qdrant.RetrievedPoint, 0, len(request.GetIds()))
	for _, pointID := range request.GetIds() {
		id, err := extractPointID(pointID)
		if err != nil {
			return nil, err
		}
		if point, ok := collection.points[id]; ok {
			points = append(points, newRetrievedPoint(point))
		}
	}

	return points, nil
}

func (c *fakeQdrantClient) Scroll(ctx context.Context, request *qdrant.ScrollPoints) ([]*qdrant.RetrievedPoint, error) {
	points, _, err := c.ScrollAndOffset(ctx, request)
	return points, err
}

func (c *fakeQdrantClient) ScrollAndOffset(_ context.Context, request *qdrant.ScrollPoints) ([]*qdrant.RetrievedPoint, *qdrant.PointId, error) {
	collection, err := c.collection(request.GetCollectionName())
	if err != nil {
		return nil, nil, err
	}

	ids := collection.sortedIDs()
	start := 0
	if request.GetOffset() != nil {
		offsetID, err := extractPointID(request.GetOffset())
		if err != nil {
			return nil, nil, err
		}
		for i, id := range ids {
			if id > offsetID {
				start = i
				break
			}
			start = len(ids)
		}
	}

	limit := len(ids)
	if request.GetLimit() > 0 {
		limit = int(request.GetLimit())
	}

	selected := make([]*qdrant.RetrievedPoint, 0, limit)
	for i := start; i < len(ids) && len(selected) < limit; i++ {
		point := collection.points[ids[i]]
		if !matchesFilter(point, request.GetFilter()) {
			continue
		}
		selected = append(selected, newRetrievedPoint(point))
	}

	nextIndex := start + len(selected)
	if nextIndex >= len(ids) {
		return selected, nil, nil
	}

	nextOffset := qdrant.NewIDNum(uint64(ids[nextIndex-1]))
	return selected, nextOffset, nil
}

func (c *fakeQdrantClient) Count(_ context.Context, request *qdrant.CountPoints) (uint64, error) {
	collection, err := c.collection(request.GetCollectionName())
	if err != nil {
		return 0, err
	}

	var count uint64
	for _, point := range collection.points {
		if matchesFilter(point, request.GetFilter()) {
			count++
		}
	}

	return count, nil
}

func (c *fakeQdrantClient) Query(_ context.Context, request *qdrant.QueryPoints) ([]*qdrant.ScoredPoint, error) {
	collection, err := c.collection(request.GetCollectionName())
	if err != nil {
		return nil, err
	}

	queryVector := extractQueryVector(request.GetQuery())
	if len(queryVector) != int(collection.size) {
		return nil, fmt.Errorf("query vector dimension mismatch: expected %d, got %d", collection.size, len(queryVector))
	}

	results := make([]*qdrant.ScoredPoint, 0, len(collection.points))
	for _, point := range collection.points {
		if !matchesFilter(point, request.GetFilter()) {
			continue
		}

		score := pointScore(collection.distance, queryVector, point.vector)
		if threshold := request.GetScoreThreshold(); request.ScoreThreshold != nil {
			if collection.distance == qdrant.Distance_Cosine && score < threshold {
				continue
			}
			if collection.distance == qdrant.Distance_Euclid && score > threshold {
				continue
			}
		}

		results = append(results, newScoredPoint(point, score))
	}

	sort.Slice(results, func(i, j int) bool {
		if collection.distance == qdrant.Distance_Cosine {
			return results[i].GetScore() > results[j].GetScore()
		}
		return results[i].GetScore() < results[j].GetScore()
	})

	limit := defaultSearchLimit
	if request.GetLimit() > 0 {
		limit = int(request.GetLimit())
	}
	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

func (c *fakeQdrantClient) UpdateVectors(_ context.Context, request *qdrant.UpdatePointVectors) (*qdrant.UpdateResult, error) {
	collection, err := c.collection(request.GetCollectionName())
	if err != nil {
		return nil, err
	}

	for _, point := range request.GetPoints() {
		id, err := extractPointID(point.GetId())
		if err != nil {
			return nil, err
		}
		existing, ok := collection.points[id]
		if !ok {
			return nil, fmt.Errorf("point %d not found", id)
		}

		vector, err := extractDenseInputVector(point.GetVectors())
		if err != nil {
			return nil, err
		}
		if uint64(len(vector)) != collection.size {
			return nil, fmt.Errorf("vector dimension mismatch: expected %d, got %d", collection.size, len(vector))
		}
		existing.vector = vector
	}

	return &qdrant.UpdateResult{}, nil
}

func (c *fakeQdrantClient) SetPayload(_ context.Context, request *qdrant.SetPayloadPoints) (*qdrant.UpdateResult, error) {
	collection, err := c.collection(request.GetCollectionName())
	if err != nil {
		return nil, err
	}

	payload, err := payloadMapToAny(request.GetPayload())
	if err != nil {
		return nil, err
	}

	ids, err := selectorIDs(request.GetPointsSelector())
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		point, ok := collection.points[id]
		if !ok {
			continue
		}
		for key, value := range payload {
			point.payload[key] = value
		}
	}

	return &qdrant.UpdateResult{}, nil
}

func (c *fakeQdrantClient) Delete(_ context.Context, request *qdrant.DeletePoints) (*qdrant.UpdateResult, error) {
	collection, err := c.collection(request.GetCollectionName())
	if err != nil {
		return nil, err
	}

	ids, err := selectorIDs(request.GetPoints())
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		delete(collection.points, id)
	}

	return &qdrant.UpdateResult{}, nil
}

func (c *fakeQdrantClient) HealthCheck(_ context.Context) (*qdrant.HealthCheckReply, error) {
	return &qdrant.HealthCheckReply{Version: c.healthVersion}, nil
}

func (c *fakeQdrantClient) collection(collectionName string) (*fakeCollection, error) {
	collection, ok := c.collections[collectionName]
	if !ok {
		return nil, fmt.Errorf("collection %q not found", collectionName)
	}
	return collection, nil
}

func (c *fakeCollection) sortedIDs() []int64 {
	ids := make([]int64, 0, len(c.points))
	for id := range c.points {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func extractDenseInputVector(vectors *qdrant.Vectors) ([]float32, error) {
	if vectors == nil {
		return nil, fmt.Errorf("vector is missing")
	}

	vector := vectors.GetVector()
	if vector == nil || vector.GetDense() == nil {
		return nil, fmt.Errorf("only dense vectors are supported")
	}

	return append([]float32(nil), vector.GetDense().GetData()...), nil
}

func extractQueryVector(query *qdrant.Query) []float32 {
	if query == nil {
		return nil
	}
	nearest := query.GetNearest()
	if nearest == nil || nearest.GetDense() == nil {
		return nil
	}
	return nearest.GetDense().GetData()
}

func newRetrievedPoint(point *fakePoint) *qdrant.RetrievedPoint {
	return &qdrant.RetrievedPoint{
		Id:      qdrant.NewIDNum(uint64(point.id)),
		Payload: mustValueMap(point.payload),
		Vectors: &qdrant.VectorsOutput{
			VectorsOptions: &qdrant.VectorsOutput_Vector{
				Vector: &qdrant.VectorOutput{
					Vector: &qdrant.VectorOutput_Dense{
						Dense: &qdrant.DenseVector{Data: append([]float32(nil), point.vector...)},
					},
				},
			},
		},
	}
}

func newScoredPoint(point *fakePoint, score float32) *qdrant.ScoredPoint {
	return &qdrant.ScoredPoint{
		Id:      qdrant.NewIDNum(uint64(point.id)),
		Payload: mustValueMap(point.payload),
		Score:   score,
	}
}

func mustValueMap(input map[string]any) map[string]*qdrant.Value {
	valueMap, err := qdrant.TryValueMap(input)
	if err != nil {
		panic(err)
	}
	return valueMap
}

func payloadMapToAny(payload map[string]*qdrant.Value) (map[string]any, error) {
	result := make(map[string]any, len(payload))
	for key, value := range payload {
		decoded, err := payloadValueToAny(value)
		if err != nil {
			return nil, err
		}
		result[key] = decoded
	}
	return result, nil
}

func selectorIDs(selector *qdrant.PointsSelector) ([]int64, error) {
	if selector == nil || selector.GetPoints() == nil {
		return nil, fmt.Errorf("points selector is missing")
	}

	ids := make([]int64, 0, len(selector.GetPoints().GetIds()))
	for _, pointID := range selector.GetPoints().GetIds() {
		id, err := extractPointID(pointID)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	return ids, nil
}

func matchesFilter(point *fakePoint, filter *qdrant.Filter) bool {
	if filter == nil {
		return true
	}

	for _, condition := range filter.GetMust() {
		field := condition.GetField()
		if field == nil {
			return false
		}

		actual, ok := point.payload[field.GetKey()]
		if !ok {
			return false
		}

		match := field.GetMatch()
		if match == nil {
			return false
		}
		if !matchesValue(actual, match) {
			return false
		}
	}

	return true
}

func matchesValue(actual any, match *qdrant.Match) bool {
	switch value := match.GetMatchValue().(type) {
	case *qdrant.Match_Keyword:
		return fmt.Sprint(actual) == value.Keyword
	case *qdrant.Match_Integer:
		return fmt.Sprint(actual) == fmt.Sprint(value.Integer)
	case *qdrant.Match_Boolean:
		return fmt.Sprint(actual) == fmt.Sprint(value.Boolean)
	default:
		return false
	}
}

func pointScore(distance qdrant.Distance, query, vector []float32) float32 {
	if distance == qdrant.Distance_Euclid {
		return float32(L2Distance(query, vector))
	}
	return float32(CosineSimilarity(query, vector))
}
