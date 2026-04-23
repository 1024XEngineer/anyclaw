package vec

import (
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"slices"
	"strings"
	"unicode"
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
		metadata:   slices.Clone(cfg.Metadata),
		auxColumns: slices.Clone(cfg.AuxColumns),
	}
}

func (vs *VecStore) Init(ctx context.Context) error {
	if err := vs.validateConfig(); err != nil {
		return err
	}
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
	sql, err := vs.buildCreateTableSQL()
	if err != nil {
		return err
	}
	_, err = vs.db.ExecContext(ctx, sql)
	if err != nil {
		return fmt.Errorf("create vec0 table: %w", err)
	}
	return nil
}

func (vs *VecStore) buildCreateTableSQL() (string, error) {
	if err := vs.validateConfig(); err != nil {
		return "", err
	}

	cols := []string{
		fmt.Sprintf("vector float[%d] distance=%s", vs.dimensions, vs.distance),
	}

	for _, meta := range vs.metadata {
		cols = append(cols, fmt.Sprintf("%s text", meta))
	}

	for _, aux := range vs.auxColumns {
		cols = append(cols, fmt.Sprintf("+%s text", aux))
	}

	return fmt.Sprintf(
		"CREATE VIRTUAL TABLE IF NOT EXISTS %s USING vec0(%s)",
		quoteIdentifier(vs.tableName),
		strings.Join(cols, ", "),
	), nil
}

func (vs *VecStore) Insert(ctx context.Context, id any, vector []float32, metadata map[string]string) error {
	return vs.InsertWithAux(ctx, id, vector, metadata, nil)
}

func (vs *VecStore) InsertWithAux(ctx context.Context, id any, vector []float32, metadata map[string]string, aux map[string]string) error {
	if err := vs.validateConfig(); err != nil {
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

	tx, err := vs.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin insert transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE rowid = ?", quoteIdentifier(vs.tableName)), id); err != nil {
		return fmt.Errorf("delete existing vector: %w", err)
	}

	if err := vs.execInsert(ctx, tx, id, vector, metadata, aux); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit insert transaction: %w", err)
	}
	return nil
}

func (vs *VecStore) InsertBatch(ctx context.Context, items []VecItem) error {
	if err := vs.validateConfig(); err != nil {
		return err
	}
	tx, err := vs.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	delQuery := fmt.Sprintf("DELETE FROM %s WHERE rowid = ?", quoteIdentifier(vs.tableName))

	delStmt, err := tx.PrepareContext(ctx, delQuery)
	if err != nil {
		return fmt.Errorf("prepare delete: %w", err)
	}
	defer delStmt.Close()

	for _, item := range items {
		if err := validateColumnValues("metadata", item.Metadata, vs.metadata); err != nil {
			return fmt.Errorf("item %v: %w", item.ID, err)
		}
		if err := validateColumnValues("aux", item.Aux, vs.auxColumns); err != nil {
			return fmt.Errorf("item %v: %w", item.ID, err)
		}
		if len(item.Vector) != vs.dimensions {
			return fmt.Errorf("vector dimension mismatch for id %v: expected %d, got %d",
				item.ID, vs.dimensions, len(item.Vector))
		}

		if _, err := delStmt.ExecContext(ctx, item.ID); err != nil {
			return fmt.Errorf("delete existing item %v: %w", item.ID, err)
		}

		if err := vs.execInsert(ctx, tx, item.ID, item.Vector, item.Metadata, item.Aux); err != nil {
			return fmt.Errorf("insert item %v: %w", item.ID, err)
		}
	}

	return tx.Commit()
}

func (vs *VecStore) Search(ctx context.Context, queryVector []float32, limit int) ([]VecSearchResult, error) {
	return vs.SearchWithFilter(ctx, queryVector, limit, 0, nil)
}

func (vs *VecStore) SearchWithFilter(ctx context.Context, queryVector []float32, limit int, threshold float64, metadataFilter map[string]string) ([]VecSearchResult, error) {
	if err := vs.validateConfig(); err != nil {
		return nil, err
	}
	if len(queryVector) != vs.dimensions {
		return nil, fmt.Errorf("query vector dimension mismatch: expected %d, got %d", vs.dimensions, len(queryVector))
	}

	if limit <= 0 {
		limit = 10
	}

	selectCols := "SELECT rowid, distance"
	for _, meta := range vs.metadata {
		selectCols += fmt.Sprintf(", %s", quoteIdentifier(meta))
	}
	for _, aux := range vs.auxColumns {
		selectCols += fmt.Sprintf(", %s", quoteIdentifier(aux))
	}

	query := fmt.Sprintf(
		"%s FROM %s WHERE vector MATCH ? AND k = ?",
		selectCols,
		quoteIdentifier(vs.tableName),
	)

	args := []any{vectorToBlob(queryVector), limit}

	if threshold > 0 {
		query += " AND distance <= ?"
		args = append(args, threshold)
	}

	for key, val := range metadataFilter {
		if !slices.Contains(vs.metadata, key) {
			return nil, fmt.Errorf("unknown metadata filter column %q", key)
		}
		query += fmt.Sprintf(" AND %s = ?", quoteIdentifier(key))
		args = append(args, val)
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

		auxVals := make([]any, len(vs.auxColumns))
		for i := range vs.auxColumns {
			auxVals[i] = new(string)
			scanArgs = append(scanArgs, auxVals[i])
		}

		if err := rows.Scan(scanArgs...); err != nil {
			return nil, fmt.Errorf("scan search result: %w", err)
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

		if len(vs.auxColumns) > 0 {
			r.Aux = make(map[string]any)
			for i, key := range vs.auxColumns {
				if sv, ok := auxVals[i].(*string); ok {
					r.Aux[key] = *sv
				}
			}
		}

		results = append(results, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate search results: %w", err)
	}

	return results, nil
}

func (vs *VecStore) Get(ctx context.Context, id any) (*VecItem, error) {
	if err := vs.validateConfig(); err != nil {
		return nil, err
	}
	selectCols := "SELECT rowid, vector"
	for _, meta := range vs.metadata {
		selectCols += fmt.Sprintf(", %s", quoteIdentifier(meta))
	}
	for _, aux := range vs.auxColumns {
		selectCols += fmt.Sprintf(", %s", quoteIdentifier(aux))
	}

	query := fmt.Sprintf("%s FROM %s WHERE rowid = ?", selectCols, quoteIdentifier(vs.tableName))

	row := vs.db.QueryRowContext(ctx, query, id)

	var item VecItem
	var vecBlob []byte
	scanArgs := []any{&item.RowID, &vecBlob}

	metaVals := make([]any, len(vs.metadata))
	for i := range vs.metadata {
		metaVals[i] = new(string)
		scanArgs = append(scanArgs, metaVals[i])
	}

	auxVals := make([]any, len(vs.auxColumns))
	for i := range vs.auxColumns {
		auxVals[i] = new(string)
		scanArgs = append(scanArgs, auxVals[i])
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

	if len(vs.auxColumns) > 0 {
		item.Aux = make(map[string]string)
		for i, key := range vs.auxColumns {
			if sv, ok := auxVals[i].(*string); ok {
				item.Aux[key] = *sv
			}
		}
	}

	return &item, nil
}

func (vs *VecStore) Delete(ctx context.Context, id any) error {
	if err := vs.validateConfig(); err != nil {
		return err
	}
	query := fmt.Sprintf("DELETE FROM %s WHERE rowid = ?", quoteIdentifier(vs.tableName))
	_, err := vs.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete vector item: %w", err)
	}
	return nil
}

func (vs *VecStore) UpdateVector(ctx context.Context, id any, vector []float32) error {
	if err := vs.validateConfig(); err != nil {
		return err
	}
	if len(vector) != vs.dimensions {
		return fmt.Errorf("vector dimension mismatch: expected %d, got %d", vs.dimensions, len(vector))
	}

	query := fmt.Sprintf("UPDATE %s SET vector = ? WHERE rowid = ?", quoteIdentifier(vs.tableName))
	_, err := vs.db.ExecContext(ctx, query, vectorToBlob(vector), id)
	if err != nil {
		return fmt.Errorf("update vector: %w", err)
	}
	return nil
}

func (vs *VecStore) UpdateMetadata(ctx context.Context, id any, metadata map[string]string) error {
	if err := vs.validateConfig(); err != nil {
		return err
	}
	if len(metadata) == 0 {
		return nil
	}
	if err := validateColumnValues("metadata", metadata, vs.metadata); err != nil {
		return err
	}

	setClauses := make([]string, 0, len(metadata))
	args := make([]any, 0, len(metadata)+1)

	for _, key := range vs.metadata {
		if val, ok := metadata[key]; ok {
			setClauses = append(setClauses, fmt.Sprintf("%s = ?", quoteIdentifier(key)))
			args = append(args, val)
		}
	}

	if len(setClauses) == 0 {
		return nil
	}

	args = append(args, id)
	query := fmt.Sprintf("UPDATE %s SET %s WHERE rowid = ?", quoteIdentifier(vs.tableName), strings.Join(setClauses, ", "))

	_, err := vs.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update metadata: %w", err)
	}
	return nil
}

func (vs *VecStore) Count(ctx context.Context) (int64, error) {
	if err := vs.validateConfig(); err != nil {
		return 0, err
	}
	var count int64
	err := vs.db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", quoteIdentifier(vs.tableName))).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (vs *VecStore) List(ctx context.Context, limit int) ([]VecItem, error) {
	if err := vs.validateConfig(); err != nil {
		return nil, err
	}
	selectCols := "SELECT rowid, vector"
	for _, meta := range vs.metadata {
		selectCols += fmt.Sprintf(", %s", quoteIdentifier(meta))
	}
	for _, aux := range vs.auxColumns {
		selectCols += fmt.Sprintf(", %s", quoteIdentifier(aux))
	}

	query := fmt.Sprintf("%s FROM %s", selectCols, quoteIdentifier(vs.tableName))
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
		scanArgs := []any{&item.RowID, &vecBlob}

		metaVals := make([]any, len(vs.metadata))
		for i := range vs.metadata {
			metaVals[i] = new(string)
			scanArgs = append(scanArgs, metaVals[i])
		}

		auxVals := make([]any, len(vs.auxColumns))
		for i := range vs.auxColumns {
			auxVals[i] = new(string)
			scanArgs = append(scanArgs, auxVals[i])
		}

		if err := rows.Scan(scanArgs...); err != nil {
			return nil, fmt.Errorf("scan listed item: %w", err)
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

		if len(vs.auxColumns) > 0 {
			item.Aux = make(map[string]string)
			for i, key := range vs.auxColumns {
				if sv, ok := auxVals[i].(*string); ok {
					item.Aux[key] = *sv
				}
			}
		}

		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate listed items: %w", err)
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
	Aux      map[string]string
}

type VecSearchResult struct {
	RowID    int64
	ID       any
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

type sqlExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func (vs *VecStore) execInsert(ctx context.Context, execer sqlExecutor, id any, vector []float32, metadata map[string]string, aux map[string]string) error {
	cols := []string{"rowid", "vector"}
	args := []any{id, vectorToBlob(vector)}

	for _, key := range vs.metadata {
		val := ""
		if metadata != nil {
			val = metadata[key]
		}
		cols = append(cols, quoteIdentifier(key))
		args = append(args, val)
	}

	for _, key := range vs.auxColumns {
		val := ""
		if aux != nil {
			val = aux[key]
		}
		cols = append(cols, quoteIdentifier(key))
		args = append(args, val)
	}

	placeholders := make([]string, len(cols))
	for i := range cols {
		placeholders[i] = "?"
	}

	query := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s)",
		quoteIdentifier(vs.tableName),
		strings.Join(cols, ", "),
		strings.Join(placeholders, ", "),
	)

	if _, err := execer.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("insert vector: %w", err)
	}

	return nil
}

func (vs *VecStore) validateConfig() error {
	if vs.db == nil {
		return fmt.Errorf("db cannot be nil")
	}
	if err := validateIdentifier("table name", vs.tableName); err != nil {
		return err
	}
	if vs.dimensions <= 0 {
		return fmt.Errorf("dimensions must be greater than 0")
	}
	if err := validateDistanceMetric(vs.distance); err != nil {
		return err
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

func quoteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
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
