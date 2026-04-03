package vec

import (
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
)

type DistanceMetric string

const (
	DistanceCosine DistanceMetric = "cosine"
	DistanceL2     DistanceMetric = "l2"
)

type VecStore struct {
	db         *sql.DB
	tableName  string
	dimensions int
	distance   DistanceMetric
	metadata   []string
	auxColumns []string
}

type VecStoreConfig struct {
	DB         *sql.DB
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
	return &VecStore{
		db:         cfg.DB,
		tableName:  cfg.TableName,
		dimensions: cfg.Dimensions,
		distance:   cfg.Distance,
		metadata:   cfg.Metadata,
		auxColumns: cfg.AuxColumns,
	}
}

func (vs *VecStore) Init(ctx context.Context) error {
	if err := vs.enableVec(ctx); err != nil {
		return err
	}
	return vs.createTable(ctx)
}

func (vs *VecStore) enableVec(ctx context.Context) error {
	var version string
	err := vs.db.QueryRowContext(ctx, "SELECT vec_version()").Scan(&version)
	if err != nil {
		return fmt.Errorf("sqlite-vec not available (requires modernc.org/sqlite/vec): %w", err)
	}
	return nil
}

func (vs *VecStore) createTable(ctx context.Context) error {
	sql := vs.buildCreateTableSQL()
	_, err := vs.db.ExecContext(ctx, sql)
	if err != nil {
		return fmt.Errorf("create vec0 table: %w", err)
	}
	return nil
}

func (vs *VecStore) buildCreateTableSQL() string {
	cols := []string{
		fmt.Sprintf("vector float[%d] distance=%s", vs.dimensions, vs.distance),
	}

	for _, meta := range vs.metadata {
		cols = append(cols, fmt.Sprintf("+%s text", meta))
	}

	for _, aux := range vs.auxColumns {
		cols = append(cols, fmt.Sprintf("#%s text", aux))
	}

	return fmt.Sprintf(
		"CREATE VIRTUAL TABLE IF NOT EXISTS %s USING vec0(%s)",
		vs.tableName,
		strings.Join(cols, ", "),
	)
}

func (vs *VecStore) Insert(ctx context.Context, id any, vector []float32, metadata map[string]string) error {
	if len(vector) != vs.dimensions {
		return fmt.Errorf("vector dimension mismatch: expected %d, got %d", vs.dimensions, len(vector))
	}

	vs.db.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE rowid = ?", vs.tableName), id)

	cols := []string{"rowid", "vector"}
	args := []any{id, vectorToBlob(vector)}

	for _, key := range vs.metadata {
		val := ""
		if metadata != nil {
			val = metadata[key]
		}
		cols = append(cols, key)
		args = append(args, val)
	}

	placeholders := make([]string, len(cols))
	for i := range cols {
		placeholders[i] = "?"
	}

	query := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s)",
		vs.tableName,
		strings.Join(cols, ", "),
		strings.Join(placeholders, ", "),
	)

	_, err := vs.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("insert vector: %w", err)
	}
	return nil
}

func (vs *VecStore) InsertBatch(ctx context.Context, items []VecItem) error {
	tx, err := vs.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	cols := []string{"rowid", "vector"}
	for _, key := range vs.metadata {
		cols = append(cols, key)
	}

	placeholders := make([]string, len(cols))
	for i := range cols {
		placeholders[i] = "?"
	}

	query := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s)",
		vs.tableName,
		strings.Join(cols, ", "),
		strings.Join(placeholders, ", "),
	)

	delQuery := fmt.Sprintf("DELETE FROM %s WHERE rowid = ?", vs.tableName)

	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	delStmt, err := tx.PrepareContext(ctx, delQuery)
	if err != nil {
		return fmt.Errorf("prepare delete: %w", err)
	}
	defer delStmt.Close()

	for _, item := range items {
		if len(item.Vector) != vs.dimensions {
			return fmt.Errorf("vector dimension mismatch for id %v: expected %d, got %d",
				item.ID, vs.dimensions, len(item.Vector))
		}

		delStmt.ExecContext(ctx, item.ID)

		args := []any{item.ID, vectorToBlob(item.Vector)}
		for _, key := range vs.metadata {
			val := ""
			if item.Metadata != nil {
				val = item.Metadata[key]
			}
			args = append(args, val)
		}

		if _, err := stmt.ExecContext(ctx, args...); err != nil {
			return fmt.Errorf("insert item %v: %w", item.ID, err)
		}
	}

	return tx.Commit()
}

func (vs *VecStore) Search(ctx context.Context, queryVector []float32, limit int) ([]VecSearchResult, error) {
	return vs.SearchWithFilter(ctx, queryVector, limit, 0, nil)
}

func (vs *VecStore) SearchWithFilter(ctx context.Context, queryVector []float32, limit int, threshold float64, metadataFilter map[string]string) ([]VecSearchResult, error) {
	if len(queryVector) != vs.dimensions {
		return nil, fmt.Errorf("query vector dimension mismatch: expected %d, got %d", vs.dimensions, len(queryVector))
	}

	if limit <= 0 {
		limit = 10
	}

	selectCols := "SELECT rowid, distance"
	for _, meta := range vs.metadata {
		selectCols += fmt.Sprintf(", %s", meta)
	}

	query := fmt.Sprintf(
		"%s FROM %s WHERE vector MATCH ? AND k = ?",
		selectCols,
		vs.tableName,
	)

	args := []any{vectorToBlob(queryVector), limit}

	if threshold > 0 {
		query += " AND distance <= ?"
		args = append(args, threshold)
	}

	rows, err := vs.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}
	defer rows.Close()

	var results []VecSearchResult
	for rows.Next() {
		var r VecSearchResult
		var dist float64
		var rowID int64
		scanArgs := []any{&rowID, &dist}

		metaVals := make([]any, len(vs.metadata))
		for i := range vs.metadata {
			metaVals[i] = new(string)
			scanArgs = append(scanArgs, metaVals[i])
		}

		if err := rows.Scan(scanArgs...); err != nil {
			continue
		}

		r.RowID = rowID
		r.ID = rowID
		r.Distance = dist

		if len(vs.metadata) > 0 {
			r.Metadata = make(map[string]any)
			for i, key := range vs.metadata {
				if sv, ok := metaVals[i].(*string); ok {
					r.Metadata[key] = *sv
				}
			}
		}

		results = append(results, r)
	}

	return results, nil
}

func (vs *VecStore) Get(ctx context.Context, id any) (*VecItem, error) {
	selectCols := "SELECT rowid, vector"
	for _, meta := range vs.metadata {
		selectCols += fmt.Sprintf(", %s", meta)
	}

	query := fmt.Sprintf("%s FROM %s WHERE rowid = ?", selectCols, vs.tableName)

	row := vs.db.QueryRowContext(ctx, query, id)

	var item VecItem
	var vecBlob []byte
	scanArgs := []any{&item.RowID, &vecBlob}

	metaVals := make([]any, len(vs.metadata))
	for i := range vs.metadata {
		metaVals[i] = new(string)
		scanArgs = append(scanArgs, metaVals[i])
	}

	if err := row.Scan(scanArgs...); err != nil {
		return nil, fmt.Errorf("get vector item: %w", err)
	}

	item.ID = item.RowID
	item.Vector = blobToVector(vecBlob)

	if len(vs.metadata) > 0 {
		item.Metadata = make(map[string]string)
		for i, key := range vs.metadata {
			if sv, ok := metaVals[i].(*string); ok {
				item.Metadata[key] = *sv
			}
		}
	}

	return &item, nil
}

func (vs *VecStore) Delete(ctx context.Context, id any) error {
	query := fmt.Sprintf("DELETE FROM %s WHERE rowid = ?", vs.tableName)
	_, err := vs.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete vector item: %w", err)
	}
	return nil
}

func (vs *VecStore) UpdateVector(ctx context.Context, id any, vector []float32) error {
	if len(vector) != vs.dimensions {
		return fmt.Errorf("vector dimension mismatch: expected %d, got %d", vs.dimensions, len(vector))
	}

	query := fmt.Sprintf("UPDATE %s SET vector = ? WHERE rowid = ?", vs.tableName)
	_, err := vs.db.ExecContext(ctx, query, vectorToBlob(vector), id)
	if err != nil {
		return fmt.Errorf("update vector: %w", err)
	}
	return nil
}

func (vs *VecStore) UpdateMetadata(ctx context.Context, id any, metadata map[string]string) error {
	if len(metadata) == 0 {
		return nil
	}

	setClauses := make([]string, 0, len(metadata))
	args := make([]any, 0, len(metadata)+1)

	for _, key := range vs.metadata {
		if val, ok := metadata[key]; ok {
			setClauses = append(setClauses, fmt.Sprintf("%s = ?", key))
			args = append(args, val)
		}
	}

	if len(setClauses) == 0 {
		return nil
	}

	args = append(args, id)
	query := fmt.Sprintf("UPDATE %s SET %s WHERE rowid = ?", vs.tableName, strings.Join(setClauses, ", "))

	_, err := vs.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update metadata: %w", err)
	}
	return nil
}

func (vs *VecStore) Count(ctx context.Context) (int64, error) {
	var count int64
	err := vs.db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", vs.tableName)).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (vs *VecStore) List(ctx context.Context, limit int) ([]VecItem, error) {
	query := fmt.Sprintf("SELECT rowid, vector FROM %s", vs.tableName)
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := vs.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []VecItem
	for rows.Next() {
		var item VecItem
		var vecBlob []byte
		if err := rows.Scan(&item.RowID, &vecBlob); err != nil {
			continue
		}
		item.ID = item.RowID
		item.Vector = blobToVector(vecBlob)
		items = append(items, item)
	}

	return items, nil
}

func (vs *VecStore) VecVersion(ctx context.Context) (string, error) {
	var version string
	err := vs.db.QueryRowContext(ctx, "SELECT vec_version()").Scan(&version)
	return version, err
}

func (vs *VecStore) TableInfo(ctx context.Context) (*VecTableInfo, error) {
	var info VecTableInfo
	info.TableName = vs.tableName
	info.Dimensions = vs.dimensions
	info.Distance = string(vs.distance)

	count, err := vs.Count(ctx)
	if err != nil {
		return nil, err
	}
	info.VectorCount = count

	var vecVersion string
	if err := vs.db.QueryRowContext(ctx, "SELECT vec_version()").Scan(&vecVersion); err == nil {
		info.VecVersion = vecVersion
	}

	return &info, nil
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
