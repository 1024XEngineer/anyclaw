package vec

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"

	chromem "github.com/philippgille/chromem-go"
)

type DistanceMetric string

const (
	DistanceCosine DistanceMetric = "cosine"
	DistanceL2     DistanceMetric = "l2"
)

const vecBackendVersion = "chromem-go"

var (
	inMemoryDBsMu sync.Mutex
	inMemoryDBs   = make(map[string]*chromem.DB)
)

type VecStore struct {
	legacyDB    *sql.DB
	tableName   string
	dimensions  int
	distance    DistanceMetric
	metadata    []string
	auxColumns  []string
	persistPath string
	compress    bool

	mu         sync.Mutex
	chromemDB  *chromem.DB
	collection *chromem.Collection
}

type VecStoreConfig struct {
	DB          *sql.DB
	TableName   string
	Dimensions  int
	Distance    DistanceMetric
	Metadata    []string
	AuxColumns  []string
	PersistPath string
	Compress    bool
}

func NewVecStore(cfg VecStoreConfig) *VecStore {
	if cfg.Distance == "" {
		cfg.Distance = DistanceCosine
	}
	return &VecStore{
		legacyDB:    cfg.DB,
		tableName:   cfg.TableName,
		dimensions:  cfg.Dimensions,
		distance:    cfg.Distance,
		metadata:    append([]string(nil), cfg.Metadata...),
		auxColumns:  append([]string(nil), cfg.AuxColumns...),
		persistPath: strings.TrimSpace(cfg.PersistPath),
		compress:    cfg.Compress,
	}
}

func (vs *VecStore) Init(ctx context.Context) error {
	_, err := vs.ensureCollection(ctx, true)
	return err
}

func (vs *VecStore) Insert(ctx context.Context, id any, vector []float32, metadata map[string]string) error {
	col, err := vs.ensureCollection(ctx, true)
	if err != nil {
		return err
	}
	if err := vs.validateVector(vector); err != nil {
		return err
	}

	docID, err := normalizeID(id)
	if err != nil {
		return err
	}

	err = col.AddDocument(ctx, chromem.Document{
		ID:        docID,
		Metadata:  sanitizeMetadata(vs.metadata, metadata),
		Embedding: cloneVector(vector),
	})
	if err != nil {
		return fmt.Errorf("insert vector: %w", err)
	}

	return nil
}

func (vs *VecStore) InsertBatch(ctx context.Context, items []VecItem) error {
	if len(items) == 0 {
		return nil
	}

	col, err := vs.ensureCollection(ctx, true)
	if err != nil {
		return err
	}

	docs := make([]chromem.Document, 0, len(items))
	for _, item := range items {
		if err := vs.validateVector(item.Vector); err != nil {
			return fmt.Errorf("vector dimension mismatch for id %v: %w", item.ID, err)
		}

		docID, err := normalizeID(item.ID)
		if err != nil {
			return err
		}

		docs = append(docs, chromem.Document{
			ID:        docID,
			Metadata:  sanitizeMetadata(vs.metadata, item.Metadata),
			Embedding: cloneVector(item.Vector),
		})
	}

	if err := col.AddDocuments(ctx, docs, 1); err != nil {
		return fmt.Errorf("insert batch: %w", err)
	}

	return nil
}

func (vs *VecStore) Search(ctx context.Context, queryVector []float32, limit int) ([]VecSearchResult, error) {
	return vs.SearchWithFilter(ctx, queryVector, limit, 0, nil)
}

func (vs *VecStore) SearchWithFilter(ctx context.Context, queryVector []float32, limit int, threshold float64, metadataFilter map[string]string) ([]VecSearchResult, error) {
	col, err := vs.ensureCollection(ctx, false)
	if err != nil {
		return nil, err
	}
	if err := vs.validateVector(queryVector); err != nil {
		return nil, fmt.Errorf("query vector dimension mismatch: %w", err)
	}
	if limit <= 0 {
		limit = 10
	}

	count := col.Count()
	if count == 0 {
		return nil, nil
	}
	if limit > count {
		limit = count
	}

	results, err := col.QueryEmbedding(ctx, cloneVector(queryVector), limit, metadataFilter, nil)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}

	out := make([]VecSearchResult, 0, len(results))
	for _, result := range results {
		distance := 1.0 - float64(result.Similarity)
		if threshold > 0 && distance > threshold {
			continue
		}

		rowID, typedID := decodeID(result.ID)
		out = append(out, VecSearchResult{
			RowID:    rowID,
			ID:       typedID,
			Distance: distance,
			Metadata: metadataToAny(result.Metadata),
		})
	}

	return out, nil
}

func (vs *VecStore) Get(ctx context.Context, id any) (*VecItem, error) {
	col, err := vs.ensureCollection(ctx, false)
	if err != nil {
		return nil, err
	}

	docID, err := normalizeID(id)
	if err != nil {
		return nil, err
	}

	doc, err := col.GetByID(ctx, docID)
	if err != nil {
		return nil, fmt.Errorf("get vector item: %w", err)
	}

	item := vecItemFromDocument(doc)
	return &item, nil
}

func (vs *VecStore) Delete(ctx context.Context, id any) error {
	col, err := vs.ensureCollection(ctx, false)
	if err != nil {
		return err
	}

	docID, err := normalizeID(id)
	if err != nil {
		return err
	}

	if err := col.Delete(ctx, nil, nil, docID); err != nil {
		return fmt.Errorf("delete vector item: %w", err)
	}

	return nil
}

func (vs *VecStore) UpdateVector(ctx context.Context, id any, vector []float32) error {
	if err := vs.validateVector(vector); err != nil {
		return err
	}

	item, err := vs.Get(ctx, id)
	if err != nil {
		return err
	}

	return vs.Insert(ctx, item.ID, vector, item.Metadata)
}

func (vs *VecStore) UpdateMetadata(ctx context.Context, id any, metadata map[string]string) error {
	if len(metadata) == 0 {
		return nil
	}

	item, err := vs.Get(ctx, id)
	if err != nil {
		return err
	}

	if item.Metadata == nil {
		item.Metadata = make(map[string]string)
	}
	for _, key := range vs.metadata {
		if value, ok := metadata[key]; ok {
			item.Metadata[key] = value
		}
	}

	return vs.Insert(ctx, item.ID, item.Vector, item.Metadata)
}

func (vs *VecStore) Count(ctx context.Context) (int64, error) {
	col, err := vs.ensureCollection(ctx, false)
	if err != nil {
		return 0, err
	}
	return int64(col.Count()), nil
}

func (vs *VecStore) List(ctx context.Context, limit int) ([]VecItem, error) {
	_ = ctx

	db, err := vs.ensureDB()
	if err != nil {
		return nil, err
	}
	if _, err := vs.ensureCollection(context.Background(), false); err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := db.ExportToWriter(&buf, false, "", vs.tableName); err != nil {
		return nil, fmt.Errorf("export collection: %w", err)
	}

	type persistenceCollection struct {
		Name      string
		Metadata  map[string]string
		Documents map[string]*chromem.Document
	}
	persistenceDB := struct {
		Collections map[string]*persistenceCollection
	}{}

	if err := gob.NewDecoder(&buf).Decode(&persistenceDB); err != nil {
		return nil, fmt.Errorf("decode collection export: %w", err)
	}

	pc, ok := persistenceDB.Collections[vs.tableName]
	if !ok || len(pc.Documents) == 0 {
		return nil, nil
	}

	ids := make([]string, 0, len(pc.Documents))
	for id := range pc.Documents {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	if limit > 0 && limit < len(ids) {
		ids = ids[:limit]
	}

	items := make([]VecItem, 0, len(ids))
	for _, id := range ids {
		doc := pc.Documents[id]
		if doc == nil {
			continue
		}
		items = append(items, vecItemFromDocument(*doc))
	}

	return items, nil
}

func (vs *VecStore) Drop(ctx context.Context) error {
	_ = ctx

	vs.mu.Lock()
	defer vs.mu.Unlock()

	if err := vs.validateTableName(); err != nil {
		return err
	}

	if vs.chromemDB == nil {
		db, err := vs.openDB()
		if err != nil {
			return err
		}
		vs.chromemDB = db
	}

	if err := vs.chromemDB.DeleteCollection(vs.tableName); err != nil {
		return fmt.Errorf("drop vector collection: %w", err)
	}

	vs.collection = nil
	return nil
}

func (vs *VecStore) VecVersion(ctx context.Context) (string, error) {
	_, err := vs.ensureCollection(ctx, false)
	if err != nil {
		return "", err
	}
	return vecBackendVersion, nil
}

func (vs *VecStore) TableInfo(ctx context.Context) (*VecTableInfo, error) {
	count, err := vs.Count(ctx)
	if err != nil {
		return nil, err
	}

	return &VecTableInfo{
		TableName:   vs.tableName,
		Dimensions:  vs.dimensions,
		Distance:    string(vs.distance),
		VectorCount: count,
		VecVersion:  vecBackendVersion,
	}, nil
}

type VecItem struct {
	RowID    int64
	ID       any
	Vector   []float32
	Metadata map[string]string
}

type VecSearchResult struct {
	RowID    int64
	ID       any
	Distance float64
	Metadata map[string]any
}

type VecTableInfo struct {
	TableName   string `json:"table_name"`
	Dimensions  int    `json:"dimensions"`
	Distance    string `json:"distance"`
	VectorCount int64  `json:"vector_count"`
	VecVersion  string `json:"vec_version"`
}

func (vs *VecStore) ensureDB() (*chromem.DB, error) {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	return vs.ensureDBLocked()
}

func (vs *VecStore) ensureDBLocked() (*chromem.DB, error) {
	if vs.chromemDB != nil {
		return vs.chromemDB, nil
	}

	db, err := vs.openDB()
	if err != nil {
		return nil, err
	}

	vs.chromemDB = db
	return db, nil
}

func (vs *VecStore) openDB() (*chromem.DB, error) {
	if key := vs.sharedDBKey(); key != "" {
		inMemoryDBsMu.Lock()
		defer inMemoryDBsMu.Unlock()

		if db, ok := inMemoryDBs[key]; ok {
			return db, nil
		}

		db := chromem.NewDB()
		inMemoryDBs[key] = db
		return db, nil
	}

	if vs.persistPath == "" {
		return chromem.NewDB(), nil
	}

	db, err := chromem.NewPersistentDB(vs.persistPath, vs.compress)
	if err != nil {
		return nil, fmt.Errorf("open chromem db: %w", err)
	}

	return db, nil
}

func (vs *VecStore) sharedDBKey() string {
	if vs.persistPath != "" || vs.legacyDB == nil {
		return ""
	}
	return fmt.Sprintf("sql:%p", vs.legacyDB)
}

func (vs *VecStore) ensureCollection(ctx context.Context, create bool) (*chromem.Collection, error) {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	if err := vs.validateTableName(); err != nil {
		return nil, err
	}
	if err := vs.validateDistance(); err != nil {
		return nil, err
	}
	if create {
		if err := vs.validateDimensions(); err != nil {
			return nil, err
		}
	}

	if vs.collection != nil {
		return vs.collection, nil
	}

	db, err := vs.ensureDBLocked()
	if err != nil {
		return nil, err
	}

	collection := db.GetCollection(vs.tableName, nil)
	if collection == nil {
		if !create {
			return nil, fmt.Errorf("vector collection %q not found", vs.tableName)
		}

		collection, err = db.CreateCollection(vs.tableName, vs.collectionMetadata(), nil)
		if err != nil {
			return nil, fmt.Errorf("create vector collection: %w", err)
		}
	}

	vs.collection = collection
	return collection, nil
}

func (vs *VecStore) collectionMetadata() map[string]string {
	return map[string]string{
		"distance":   string(vs.distance),
		"dimensions": strconv.Itoa(vs.dimensions),
		"backend":    vecBackendVersion,
	}
}

func (vs *VecStore) validateTableName() error {
	if strings.TrimSpace(vs.tableName) == "" {
		return fmt.Errorf("table name is required")
	}
	return nil
}

func (vs *VecStore) validateDimensions() error {
	if vs.dimensions <= 0 {
		return fmt.Errorf("dimensions must be positive")
	}
	return nil
}

func (vs *VecStore) validateDistance() error {
	switch vs.distance {
	case "", DistanceCosine:
		return nil
	case DistanceL2:
		return fmt.Errorf("distance metric %q is unsupported by %s", vs.distance, vecBackendVersion)
	default:
		return fmt.Errorf("distance metric %q is unsupported", vs.distance)
	}
}

func (vs *VecStore) validateVector(vector []float32) error {
	if err := vs.validateDistance(); err != nil {
		return err
	}
	if err := vs.validateDimensions(); err != nil {
		return err
	}
	if len(vector) != vs.dimensions {
		return fmt.Errorf("expected %d, got %d", vs.dimensions, len(vector))
	}
	return nil
}

func sanitizeMetadata(allowed []string, metadata map[string]string) map[string]string {
	if len(allowed) == 0 {
		if metadata == nil {
			return nil
		}
		return cloneMetadata(metadata)
	}

	clean := make(map[string]string, len(allowed))
	for _, key := range allowed {
		clean[key] = metadata[key]
	}
	return clean
}

func cloneVector(v []float32) []float32 {
	if v == nil {
		return nil
	}
	out := make([]float32, len(v))
	copy(out, v)
	return out
}

func cloneMetadata(metadata map[string]string) map[string]string {
	if metadata == nil {
		return nil
	}
	out := make(map[string]string, len(metadata))
	for key, value := range metadata {
		out[key] = value
	}
	return out
}

func metadataToAny(metadata map[string]string) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	out := make(map[string]any, len(metadata))
	for key, value := range metadata {
		out[key] = value
	}
	return out
}

func normalizeID(id any) (string, error) {
	if id == nil {
		return "", fmt.Errorf("id is required")
	}

	switch value := id.(type) {
	case string:
		if strings.TrimSpace(value) == "" {
			return "", fmt.Errorf("id is required")
		}
		return value, nil
	default:
		return fmt.Sprint(id), nil
	}
}

func decodeID(id string) (int64, any) {
	if rowID, err := strconv.ParseInt(id, 10, 64); err == nil {
		return rowID, rowID
	}
	return 0, id
}

func vecItemFromDocument(doc chromem.Document) VecItem {
	rowID, id := decodeID(doc.ID)
	return VecItem{
		RowID:    rowID,
		ID:       id,
		Vector:   cloneVector(doc.Embedding),
		Metadata: cloneMetadata(doc.Metadata),
	}
}

func vectorToBlob(v []float32) []byte {
	blob := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(blob[i*4:], math.Float32bits(f))
	}
	return blob
}

func blobToVector(blob []byte) []float32 {
	n := len(blob) / 4
	v := make([]float32, n)
	for i := 0; i < n; i++ {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(blob[i*4:]))
	}
	return v
}

func CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func CosineDistance(a, b []float32) float64 {
	return 1.0 - CosineSimilarity(a, b)
}

func L2Distance(a, b []float32) float64 {
	if len(a) != len(b) {
		return math.MaxFloat64
	}
	var sum float64
	for i := range a {
		diff := float64(a[i]) - float64(b[i])
		sum += diff * diff
	}
	return math.Sqrt(sum)
}
