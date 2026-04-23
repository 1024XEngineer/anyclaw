package vec

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"
	"unicode"

	"github.com/qdrant/go-client/qdrant"
)

type DistanceMetric string

const (
	DistanceCosine DistanceMetric = "cosine"
	DistanceL2     DistanceMetric = "l2"
)

const (
	noDistanceThreshold = -1.0
	defaultSearchLimit  = 10
	defaultScrollLimit  = 128
)

type VecStore struct {
	client        qdrantClient
	managedClient *qdrant.Client
	ownsClient    bool

	tableName  string
	host       string
	port       int
	apiKey     string
	useTLS     bool
	dimensions int
	distance   DistanceMetric
	metadata   []string
	auxColumns []string
}

type VecStoreConfig struct {
	Client *qdrant.Client

	Host   string
	Port   int
	APIKey string
	UseTLS bool

	TableName  string
	Dimensions int
	Distance   DistanceMetric
	Metadata   []string
	AuxColumns []string
}

func NewVecStore(cfg VecStoreConfig) *VecStore {
	if cfg.Distance == "" {
		cfg.Distance = DistanceCosine
	}

	vs := &VecStore{
		tableName:  cfg.TableName,
		host:       cfg.Host,
		port:       cfg.Port,
		apiKey:     cfg.APIKey,
		useTLS:     cfg.UseTLS,
		dimensions: cfg.Dimensions,
		distance:   cfg.Distance,
		metadata:   slices.Clone(cfg.Metadata),
		auxColumns: slices.Clone(cfg.AuxColumns),
	}

	if cfg.Client != nil {
		vs.client = cfg.Client
		vs.managedClient = cfg.Client
	}

	return vs
}

func (vs *VecStore) Init(ctx context.Context) error {
	if err := vs.validateConfig(); err != nil {
		return err
	}
	if err := vs.ensureClient(); err != nil {
		return err
	}

	exists, err := vs.client.CollectionExists(ctx, vs.tableName)
	if err != nil {
		return fmt.Errorf("check collection existence: %w", err)
	}
	if !exists {
		return vs.createCollection(ctx)
	}

	return vs.validateExistingCollection(ctx)
}

func (vs *VecStore) Close() error {
	if !vs.ownsClient || vs.managedClient == nil {
		return nil
	}

	err := vs.managedClient.Close()
	vs.managedClient = nil
	vs.client = nil
	vs.ownsClient = false
	return err
}

func (vs *VecStore) Insert(ctx context.Context, id int64, vector []float32, metadata map[string]string) error {
	return vs.InsertWithAux(ctx, id, vector, metadata, nil)
}

func (vs *VecStore) InsertWithAux(ctx context.Context, id int64, vector []float32, metadata map[string]string, aux map[string]string) error {
	if err := vs.validateConfig(); err != nil {
		return err
	}
	if err := vs.ensureClient(); err != nil {
		return err
	}
	if err := validateID(id); err != nil {
		return err
	}
	if err := validateColumnValues("metadata", metadata, vs.metadata); err != nil {
		return err
	}
	if err := validateColumnValues("aux", aux, vs.auxColumns); err != nil {
		return err
	}
	if len(vector) != vs.dimensions {
		return fmt.Errorf("vector dimension mismatch: expected %d, got %d", vs.dimensions, len(vector))
	}

	point, err := vs.buildPoint(id, vector, metadata, aux)
	if err != nil {
		return err
	}

	_, err = vs.client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: vs.tableName,
		Points:         []*qdrant.PointStruct{point},
		Wait:           qdrant.PtrOf(true),
	})
	if err != nil {
		return fmt.Errorf("upsert vector item: %w", err)
	}

	return nil
}

func (vs *VecStore) InsertBatch(ctx context.Context, items []VecItem) error {
	if err := vs.validateConfig(); err != nil {
		return err
	}
	if err := vs.ensureClient(); err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}

	points := make([]*qdrant.PointStruct, 0, len(items))
	for _, item := range items {
		if err := validateID(item.ID); err != nil {
			return fmt.Errorf("item %d: %w", item.ID, err)
		}
		if err := validateColumnValues("metadata", item.Metadata, vs.metadata); err != nil {
			return fmt.Errorf("item %d: %w", item.ID, err)
		}
		if err := validateColumnValues("aux", item.Aux, vs.auxColumns); err != nil {
			return fmt.Errorf("item %d: %w", item.ID, err)
		}
		if len(item.Vector) != vs.dimensions {
			return fmt.Errorf("vector dimension mismatch for id %d: expected %d, got %d", item.ID, vs.dimensions, len(item.Vector))
		}

		point, err := vs.buildPoint(item.ID, item.Vector, item.Metadata, item.Aux)
		if err != nil {
			return fmt.Errorf("item %d: %w", item.ID, err)
		}
		points = append(points, point)
	}

	_, err := vs.client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: vs.tableName,
		Points:         points,
		Wait:           qdrant.PtrOf(true),
	})
	if err != nil {
		return fmt.Errorf("upsert batch: %w", err)
	}

	return nil
}

func (vs *VecStore) Search(ctx context.Context, queryVector []float32, limit int) ([]VecSearchResult, error) {
	return vs.SearchWithFilter(ctx, queryVector, limit, noDistanceThreshold, nil)
}

func (vs *VecStore) SearchWithFilter(ctx context.Context, queryVector []float32, limit int, threshold float64, metadataFilter map[string]string) ([]VecSearchResult, error) {
	if err := vs.validateConfig(); err != nil {
		return nil, err
	}
	if err := vs.ensureClient(); err != nil {
		return nil, err
	}
	if len(queryVector) != vs.dimensions {
		return nil, fmt.Errorf("query vector dimension mismatch: expected %d, got %d", vs.dimensions, len(queryVector))
	}
	if limit <= 0 {
		limit = defaultSearchLimit
	}

	filter, err := vs.buildMetadataFilter(metadataFilter)
	if err != nil {
		return nil, err
	}

	req := &qdrant.QueryPoints{
		CollectionName: vs.tableName,
		Query:          qdrant.NewQueryDense(queryVector),
		Limit:          qdrant.PtrOf(uint64(limit)),
		WithPayload:    qdrant.NewWithPayload(true),
	}
	if filter != nil {
		req.Filter = filter
	}
	if threshold >= 0 {
		scoreThreshold := scoreThresholdFromDistance(vs.distance, threshold)
		req.ScoreThreshold = qdrant.PtrOf(scoreThreshold)
	}

	points, err := vs.client.Query(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}

	results := make([]VecSearchResult, 0, len(points))
	for _, point := range points {
		result, err := vs.scoredPointToResult(point)
		if err != nil {
			return nil, err
		}
		if threshold >= 0 && result.Distance > threshold {
			continue
		}
		results = append(results, result)
	}

	return results, nil
}

func (vs *VecStore) Get(ctx context.Context, id int64) (*VecItem, error) {
	if err := vs.validateConfig(); err != nil {
		return nil, err
	}
	if err := vs.ensureClient(); err != nil {
		return nil, err
	}
	if err := validateID(id); err != nil {
		return nil, err
	}

	points, err := vs.client.Get(ctx, &qdrant.GetPoints{
		CollectionName: vs.tableName,
		Ids:            []*qdrant.PointId{qdrant.NewIDNum(uint64(id))},
		WithPayload:    qdrant.NewWithPayload(true),
		WithVectors:    qdrant.NewWithVectors(true),
	})
	if err != nil {
		return nil, fmt.Errorf("get vector item: %w", err)
	}
	if len(points) == 0 {
		return nil, fmt.Errorf("get vector item: id %d not found", id)
	}

	return vs.retrievedPointToItem(points[0])
}

func (vs *VecStore) Delete(ctx context.Context, id int64) error {
	if err := vs.validateConfig(); err != nil {
		return err
	}
	if err := vs.ensureClient(); err != nil {
		return err
	}
	if err := validateID(id); err != nil {
		return err
	}

	_, err := vs.client.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: vs.tableName,
		Points:         qdrant.NewPointsSelector(qdrant.NewIDNum(uint64(id))),
		Wait:           qdrant.PtrOf(true),
	})
	if err != nil {
		return fmt.Errorf("delete vector item: %w", err)
	}

	return nil
}

func (vs *VecStore) UpdateVector(ctx context.Context, id int64, vector []float32) error {
	if err := vs.validateConfig(); err != nil {
		return err
	}
	if err := vs.ensureClient(); err != nil {
		return err
	}
	if err := validateID(id); err != nil {
		return err
	}
	if len(vector) != vs.dimensions {
		return fmt.Errorf("vector dimension mismatch: expected %d, got %d", vs.dimensions, len(vector))
	}

	_, err := vs.client.UpdateVectors(ctx, &qdrant.UpdatePointVectors{
		CollectionName: vs.tableName,
		Points: []*qdrant.PointVectors{
			{
				Id:      qdrant.NewIDNum(uint64(id)),
				Vectors: qdrant.NewVectorsDense(vector),
			},
		},
		Wait: qdrant.PtrOf(true),
	})
	if err != nil {
		return fmt.Errorf("update vector: %w", err)
	}

	return nil
}

func (vs *VecStore) UpdateMetadata(ctx context.Context, id int64, metadata map[string]string) error {
	if err := vs.validateConfig(); err != nil {
		return err
	}
	if err := vs.ensureClient(); err != nil {
		return err
	}
	if err := validateID(id); err != nil {
		return err
	}
	if len(metadata) == 0 {
		return nil
	}
	if err := validateColumnValues("metadata", metadata, vs.metadata); err != nil {
		return err
	}

	payload := make(map[string]any, len(metadata))
	for _, key := range vs.metadata {
		if value, ok := metadata[key]; ok {
			payload[key] = value
		}
	}
	if len(payload) == 0 {
		return nil
	}

	valueMap, err := qdrant.TryValueMap(payload)
	if err != nil {
		return fmt.Errorf("encode metadata payload: %w", err)
	}

	_, err = vs.client.SetPayload(ctx, &qdrant.SetPayloadPoints{
		CollectionName: vs.tableName,
		PointsSelector: qdrant.NewPointsSelector(qdrant.NewIDNum(uint64(id))),
		Payload:        valueMap,
		Wait:           qdrant.PtrOf(true),
	})
	if err != nil {
		return fmt.Errorf("update metadata: %w", err)
	}

	return nil
}

func (vs *VecStore) Count(ctx context.Context) (int64, error) {
	if err := vs.validateConfig(); err != nil {
		return 0, err
	}
	if err := vs.ensureClient(); err != nil {
		return 0, err
	}

	count, err := vs.client.Count(ctx, &qdrant.CountPoints{
		CollectionName: vs.tableName,
		Exact:          qdrant.PtrOf(true),
	})
	if err != nil {
		return 0, fmt.Errorf("count vector items: %w", err)
	}

	return int64(count), nil
}

func (vs *VecStore) List(ctx context.Context, limit int) ([]VecItem, error) {
	if err := vs.validateConfig(); err != nil {
		return nil, err
	}
	if err := vs.ensureClient(); err != nil {
		return nil, err
	}

	pageLimit := defaultScrollLimit
	if limit > 0 && limit < pageLimit {
		pageLimit = limit
	}

	var offset *qdrant.PointId
	var items []VecItem

	for {
		points, nextOffset, err := vs.client.ScrollAndOffset(ctx, &qdrant.ScrollPoints{
			CollectionName: vs.tableName,
			Offset:         offset,
			Limit:          qdrant.PtrOf(uint32(pageLimit)),
			WithPayload:    qdrant.NewWithPayload(true),
			WithVectors:    qdrant.NewWithVectors(true),
		})
		if err != nil {
			return nil, fmt.Errorf("list vector items: %w", err)
		}
		if len(points) == 0 {
			break
		}

		for _, point := range points {
			item, err := vs.retrievedPointToItem(point)
			if err != nil {
				return nil, err
			}
			items = append(items, *item)
			if limit > 0 && len(items) >= limit {
				return items, nil
			}
		}

		if nextOffset == nil {
			break
		}
		offset = nextOffset
	}

	return items, nil
}

func (vs *VecStore) VecVersion(ctx context.Context) (string, error) {
	if err := vs.validateConfig(); err != nil {
		return "", err
	}
	if err := vs.ensureClient(); err != nil {
		return "", err
	}

	reply, err := vs.client.HealthCheck(ctx)
	if err != nil {
		return "", fmt.Errorf("qdrant health check: %w", err)
	}

	return reply.GetVersion(), nil
}

func (vs *VecStore) TableInfo(ctx context.Context) (*VecTableInfo, error) {
	if err := vs.validateConfig(); err != nil {
		return nil, err
	}
	if err := vs.ensureClient(); err != nil {
		return nil, err
	}

	collectionInfo, err := vs.client.GetCollectionInfo(ctx, vs.tableName)
	if err != nil {
		return nil, fmt.Errorf("get collection info: %w", err)
	}

	params, err := collectionVectorParams(collectionInfo)
	if err != nil {
		return nil, err
	}

	distance, err := distanceMetricFromQdrant(params.GetDistance())
	if err != nil {
		return nil, err
	}

	count, err := vs.Count(ctx)
	if err != nil {
		return nil, err
	}

	info := &VecTableInfo{
		TableName:   vs.tableName,
		Dimensions:  int(params.GetSize()),
		Distance:    string(distance),
		VectorCount: count,
	}

	if version, err := vs.VecVersion(ctx); err == nil {
		info.VecVersion = version
	}

	return info, nil
}

type VecItem struct {
	RowID    int64
	ID       int64
	Vector   []float32
	Metadata map[string]string
	Aux      map[string]string
}

type VecSearchResult struct {
	RowID    int64
	ID       int64
	Distance float64
	Metadata map[string]any
	Aux      map[string]any
}

type VecTableInfo struct {
	TableName   string `json:"table_name"`
	Dimensions  int    `json:"dimensions"`
	Distance    string `json:"distance"`
	VectorCount int64  `json:"vector_count"`
	VecVersion  string `json:"vec_version"`
}

type qdrantClient interface {
	CollectionExists(context.Context, string) (bool, error)
	GetCollectionInfo(context.Context, string) (*qdrant.CollectionInfo, error)
	CreateCollection(context.Context, *qdrant.CreateCollection) error
	DeleteCollection(context.Context, string) error
	CreateFieldIndex(context.Context, *qdrant.CreateFieldIndexCollection) (*qdrant.UpdateResult, error)
	Upsert(context.Context, *qdrant.UpsertPoints) (*qdrant.UpdateResult, error)
	Get(context.Context, *qdrant.GetPoints) ([]*qdrant.RetrievedPoint, error)
	Scroll(context.Context, *qdrant.ScrollPoints) ([]*qdrant.RetrievedPoint, error)
	ScrollAndOffset(context.Context, *qdrant.ScrollPoints) ([]*qdrant.RetrievedPoint, *qdrant.PointId, error)
	Count(context.Context, *qdrant.CountPoints) (uint64, error)
	Query(context.Context, *qdrant.QueryPoints) ([]*qdrant.ScoredPoint, error)
	UpdateVectors(context.Context, *qdrant.UpdatePointVectors) (*qdrant.UpdateResult, error)
	SetPayload(context.Context, *qdrant.SetPayloadPoints) (*qdrant.UpdateResult, error)
	Delete(context.Context, *qdrant.DeletePoints) (*qdrant.UpdateResult, error)
	HealthCheck(context.Context) (*qdrant.HealthCheckReply, error)
}

var newQdrantClient = func(cfg *qdrant.Config) (*qdrant.Client, error) {
	return qdrant.NewClient(cfg)
}

func (vs *VecStore) ensureClient() error {
	if vs.client != nil {
		return nil
	}

	client, err := newQdrantClient(&qdrant.Config{
		Host:   vs.host,
		Port:   vs.port,
		APIKey: vs.apiKey,
		UseTLS: vs.useTLS,
	})
	if err != nil {
		return fmt.Errorf("create qdrant client: %w", err)
	}

	vs.client = client
	vs.managedClient = client
	vs.ownsClient = true
	return nil
}

func (vs *VecStore) createCollection(ctx context.Context) error {
	err := vs.client.CreateCollection(ctx, &qdrant.CreateCollection{
		CollectionName: vs.tableName,
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     uint64(vs.dimensions),
			Distance: distanceMetricToQdrant(vs.distance),
		}),
	})
	if err != nil {
		return fmt.Errorf("create collection: %w", err)
	}
	return nil
}

func (vs *VecStore) Drop(ctx context.Context) error {
	if err := vs.validateConfig(); err != nil {
		return err
	}
	if err := vs.ensureClient(); err != nil {
		return err
	}

	exists, err := vs.client.CollectionExists(ctx, vs.tableName)
	if err != nil {
		return fmt.Errorf("check collection existence: %w", err)
	}
	if !exists {
		return nil
	}

	if err := vs.client.DeleteCollection(ctx, vs.tableName); err != nil {
		return fmt.Errorf("delete collection: %w", err)
	}
	return nil
}

func (vs *VecStore) EnsurePayloadIndexes(ctx context.Context) error {
	if err := vs.validateConfig(); err != nil {
		return err
	}
	if err := vs.ensureClient(); err != nil {
		return err
	}

	fields := make([]string, 0, len(vs.metadata)+len(vs.auxColumns))
	fields = append(fields, vs.metadata...)
	fields = append(fields, vs.auxColumns...)
	if len(fields) == 0 {
		return nil
	}

	wait := true
	for _, field := range fields {
		_, err := vs.client.CreateFieldIndex(ctx, &qdrant.CreateFieldIndexCollection{
			CollectionName: vs.tableName,
			FieldName:      field,
			FieldType:      qdrant.FieldType_FieldTypeKeyword.Enum(),
			Wait:           &wait,
		})
		if err != nil {
			return fmt.Errorf("create payload index for %q: %w", field, err)
		}
	}

	return nil
}

func (vs *VecStore) validateExistingCollection(ctx context.Context) error {
	info, err := vs.client.GetCollectionInfo(ctx, vs.tableName)
	if err != nil {
		return fmt.Errorf("get collection info: %w", err)
	}

	params, err := collectionVectorParams(info)
	if err != nil {
		return err
	}

	if int(params.GetSize()) != vs.dimensions {
		return fmt.Errorf("collection %q dimensions mismatch: expected %d, got %d", vs.tableName, vs.dimensions, params.GetSize())
	}

	distance, err := distanceMetricFromQdrant(params.GetDistance())
	if err != nil {
		return err
	}
	if distance != vs.distance {
		return fmt.Errorf("collection %q distance mismatch: expected %q, got %q", vs.tableName, vs.distance, distance)
	}

	return nil
}

func (vs *VecStore) buildPoint(id int64, vector []float32, metadata map[string]string, aux map[string]string) (*qdrant.PointStruct, error) {
	payload, err := vs.buildPayload(metadata, aux)
	if err != nil {
		return nil, err
	}

	return &qdrant.PointStruct{
		Id:      qdrant.NewIDNum(uint64(id)),
		Vectors: qdrant.NewVectorsDense(vector),
		Payload: payload,
	}, nil
}

func (vs *VecStore) buildPayload(metadata map[string]string, aux map[string]string) (map[string]*qdrant.Value, error) {
	payload := make(map[string]any, len(vs.metadata)+len(vs.auxColumns))

	for _, key := range vs.metadata {
		payload[key] = ""
		if metadata != nil {
			if value, ok := metadata[key]; ok {
				payload[key] = value
			}
		}
	}

	for _, key := range vs.auxColumns {
		payload[key] = ""
		if aux != nil {
			if value, ok := aux[key]; ok {
				payload[key] = value
			}
		}
	}

	valueMap, err := qdrant.TryValueMap(payload)
	if err != nil {
		return nil, fmt.Errorf("encode payload: %w", err)
	}
	return valueMap, nil
}

func (vs *VecStore) buildMetadataFilter(metadataFilter map[string]string) (*qdrant.Filter, error) {
	if len(metadataFilter) == 0 {
		return nil, nil
	}

	conditions := make([]*qdrant.Condition, 0, len(metadataFilter))
	for key, value := range metadataFilter {
		if !slices.Contains(vs.metadata, key) {
			return nil, fmt.Errorf("unknown metadata filter column %q", key)
		}
		conditions = append(conditions, qdrant.NewMatch(key, value))
	}

	return &qdrant.Filter{Must: conditions}, nil
}

func (vs *VecStore) retrievedPointToItem(point *qdrant.RetrievedPoint) (*VecItem, error) {
	id, err := extractPointID(point.GetId())
	if err != nil {
		return nil, err
	}

	vector, err := extractDenseVector(point.GetVectors())
	if err != nil {
		return nil, err
	}

	metadata, err := payloadToStringMap(point.GetPayload(), vs.metadata)
	if err != nil {
		return nil, err
	}
	aux, err := payloadToStringMap(point.GetPayload(), vs.auxColumns)
	if err != nil {
		return nil, err
	}

	return &VecItem{
		RowID:    id,
		ID:       id,
		Vector:   vector,
		Metadata: metadata,
		Aux:      aux,
	}, nil
}

func (vs *VecStore) scoredPointToResult(point *qdrant.ScoredPoint) (VecSearchResult, error) {
	id, err := extractPointID(point.GetId())
	if err != nil {
		return VecSearchResult{}, err
	}

	metadata, err := payloadToAnyMap(point.GetPayload(), vs.metadata)
	if err != nil {
		return VecSearchResult{}, err
	}
	aux, err := payloadToAnyMap(point.GetPayload(), vs.auxColumns)
	if err != nil {
		return VecSearchResult{}, err
	}

	return VecSearchResult{
		RowID:    id,
		ID:       id,
		Distance: distanceFromScore(vs.distance, point.GetScore()),
		Metadata: metadata,
		Aux:      aux,
	}, nil
}

func collectionVectorParams(info *qdrant.CollectionInfo) (*qdrant.VectorParams, error) {
	if info == nil {
		return nil, fmt.Errorf("collection info cannot be nil")
	}

	config := info.GetConfig()
	if config == nil || config.GetParams() == nil {
		return nil, fmt.Errorf("collection config is incomplete")
	}

	vectorsConfig := config.GetParams().GetVectorsConfig()
	if vectorsConfig == nil {
		return nil, fmt.Errorf("collection vector configuration is missing")
	}

	if params := vectorsConfig.GetParams(); params != nil {
		return params, nil
	}

	return nil, fmt.Errorf("named vector collections are not supported")
}

func payloadToStringMap(payload map[string]*qdrant.Value, keys []string) (map[string]string, error) {
	if len(keys) == 0 {
		return nil, nil
	}

	result := make(map[string]string, len(keys))
	for _, key := range keys {
		value, ok := payload[key]
		if !ok || value == nil {
			result[key] = ""
			continue
		}

		decoded, err := payloadValueToString(value)
		if err != nil {
			return nil, fmt.Errorf("decode payload %q: %w", key, err)
		}
		result[key] = decoded
	}

	return result, nil
}

func payloadToAnyMap(payload map[string]*qdrant.Value, keys []string) (map[string]any, error) {
	if len(keys) == 0 {
		return nil, nil
	}

	result := make(map[string]any, len(keys))
	for _, key := range keys {
		value, ok := payload[key]
		if !ok || value == nil {
			result[key] = ""
			continue
		}

		decoded, err := payloadValueToAny(value)
		if err != nil {
			return nil, fmt.Errorf("decode payload %q: %w", key, err)
		}
		if decoded == nil {
			result[key] = ""
			continue
		}
		result[key] = decoded
	}

	return result, nil
}

func payloadValueToString(value *qdrant.Value) (string, error) {
	decoded, err := payloadValueToAny(value)
	if err != nil {
		return "", err
	}
	if decoded == nil {
		return "", nil
	}
	if text, ok := decoded.(string); ok {
		return text, nil
	}
	return fmt.Sprint(decoded), nil
}

func payloadValueToAny(value *qdrant.Value) (any, error) {
	if value == nil {
		return nil, nil
	}

	switch kind := value.GetKind().(type) {
	case *qdrant.Value_NullValue:
		return nil, nil
	case *qdrant.Value_StringValue:
		return kind.StringValue, nil
	case *qdrant.Value_IntegerValue:
		return kind.IntegerValue, nil
	case *qdrant.Value_DoubleValue:
		return kind.DoubleValue, nil
	case *qdrant.Value_BoolValue:
		return kind.BoolValue, nil
	case *qdrant.Value_StructValue:
		return structValueToMap(kind.StructValue)
	case *qdrant.Value_ListValue:
		return listValueToSlice(kind.ListValue)
	default:
		return nil, fmt.Errorf("unsupported payload kind %T", kind)
	}
}

func structValueToMap(value *qdrant.Struct) (map[string]any, error) {
	if value == nil {
		return nil, nil
	}

	result := make(map[string]any, len(value.GetFields()))
	for key, fieldValue := range value.GetFields() {
		decoded, err := payloadValueToAny(fieldValue)
		if err != nil {
			return nil, err
		}
		result[key] = decoded
	}

	return result, nil
}

func listValueToSlice(value *qdrant.ListValue) ([]any, error) {
	if value == nil {
		return nil, nil
	}

	result := make([]any, 0, len(value.GetValues()))
	for _, item := range value.GetValues() {
		decoded, err := payloadValueToAny(item)
		if err != nil {
			return nil, err
		}
		result = append(result, decoded)
	}

	return result, nil
}

func extractPointID(id *qdrant.PointId) (int64, error) {
	if id == nil {
		return 0, fmt.Errorf("point id is missing")
	}
	if num := id.GetNum(); num > 0 {
		return int64(num), nil
	}
	if uuid := id.GetUuid(); uuid != "" {
		return 0, fmt.Errorf("uuid point ids are not supported")
	}
	return 0, fmt.Errorf("point id is missing")
}

func extractDenseVector(vectors *qdrant.VectorsOutput) ([]float32, error) {
	if vectors == nil {
		return nil, fmt.Errorf("vector is missing")
	}

	if vector := vectors.GetVector(); vector != nil {
		if dense := vector.GetDenseVector(); dense != nil {
			return slices.Clone(dense.GetData()), nil
		}
		if dense := vector.GetDense(); dense != nil {
			return slices.Clone(dense.GetData()), nil
		}
		if data := vector.GetData(); len(data) > 0 {
			return slices.Clone(data), nil
		}
	}

	if namedVectors := vectors.GetVectors(); namedVectors != nil {
		if vector, ok := namedVectors.GetVectors()[""]; ok {
			if dense := vector.GetDenseVector(); dense != nil {
				return slices.Clone(dense.GetData()), nil
			}
			if dense := vector.GetDense(); dense != nil {
				return slices.Clone(dense.GetData()), nil
			}
		}
	}

	return nil, fmt.Errorf("only dense vectors are supported")
}

func validateID(id int64) error {
	if id <= 0 {
		return fmt.Errorf("id must be greater than 0")
	}
	return nil
}

func (vs *VecStore) validateConfig() error {
	if err := validateIdentifier("table name", vs.tableName); err != nil {
		return err
	}
	if vs.dimensions <= 0 {
		return fmt.Errorf("dimensions must be greater than 0")
	}
	if err := validateDistanceMetric(vs.distance); err != nil {
		return err
	}
	if vs.client == nil && vs.host == "" {
		return fmt.Errorf("qdrant client or host must be configured")
	}
	if vs.port < 0 {
		return fmt.Errorf("port cannot be negative")
	}

	seen := map[string]string{
		"rowid":  "reserved column",
		"vector": "reserved column",
	}
	for _, column := range vs.metadata {
		if err := validateIdentifier("metadata column", column); err != nil {
			return err
		}
		lower := strings.ToLower(column)
		if prev, ok := seen[lower]; ok {
			return fmt.Errorf("duplicate column %q conflicts with %s", column, prev)
		}
		seen[lower] = "metadata column"
	}
	for _, column := range vs.auxColumns {
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

func validateDistanceMetric(distance DistanceMetric) error {
	switch distance {
	case DistanceCosine, DistanceL2:
		return nil
	default:
		return fmt.Errorf("unsupported distance metric %q", distance)
	}
}

func validateIdentifier(kind, value string) error {
	if value == "" {
		return fmt.Errorf("%s cannot be empty", kind)
	}

	for i, r := range value {
		if i == 0 {
			if r != '_' && !unicode.IsLetter(r) {
				return fmt.Errorf("invalid %s %q", kind, value)
			}
			continue
		}
		if r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return fmt.Errorf("invalid %s %q", kind, value)
		}
	}

	return nil
}

func validateColumnValues(kind string, values map[string]string, allowed []string) error {
	if len(values) == 0 {
		return nil
	}

	allowedSet := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		allowedSet[key] = struct{}{}
	}

	for key := range values {
		if _, ok := allowedSet[key]; !ok {
			return fmt.Errorf("unknown %s column %q", kind, key)
		}
	}

	return nil
}

func distanceMetricToQdrant(distance DistanceMetric) qdrant.Distance {
	switch distance {
	case DistanceL2:
		return qdrant.Distance_Euclid
	default:
		return qdrant.Distance_Cosine
	}
}

func distanceMetricFromQdrant(distance qdrant.Distance) (DistanceMetric, error) {
	switch distance {
	case qdrant.Distance_Cosine:
		return DistanceCosine, nil
	case qdrant.Distance_Euclid:
		return DistanceL2, nil
	default:
		return "", fmt.Errorf("unsupported qdrant distance %q", distance.String())
	}
}

func scoreThresholdFromDistance(distance DistanceMetric, threshold float64) float32 {
	switch distance {
	case DistanceL2:
		return float32(threshold)
	default:
		score := 1 - threshold
		if score > 1 {
			score = 1
		}
		if score < -1 {
			score = -1
		}
		return float32(score)
	}
}

func distanceFromScore(distance DistanceMetric, score float32) float64 {
	switch distance {
	case DistanceL2:
		return float64(score)
	default:
		value := 1 - float64(score)
		if math.Abs(value) < 1e-9 {
			return 0
		}
		if value < 0 {
			return 0
		}
		return value
	}
}

func CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dot float64
	var normA float64
	var normB float64
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
	return 1 - CosineSimilarity(a, b)
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
