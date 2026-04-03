package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type QueryBuilder struct {
	table     string
	columns   []string
	where     []string
	whereArgs []any
	orderBy   []string
	limit     int
	offset    int
	distinct  bool
}

func Query(table string) *QueryBuilder {
	return &QueryBuilder{
		table:  table,
		limit:  -1,
		offset: -1,
	}
}

func (q *QueryBuilder) Select(columns ...string) *QueryBuilder {
	q.columns = columns
	return q
}

func (q *QueryBuilder) Where(condition string, args ...any) *QueryBuilder {
	q.where = append(q.where, condition)
	q.whereArgs = append(q.whereArgs, args...)
	return q
}

func (q *QueryBuilder) OrderBy(clause string) *QueryBuilder {
	q.orderBy = append(q.orderBy, clause)
	return q
}

func (q *QueryBuilder) Limit(n int) *QueryBuilder {
	q.limit = n
	return q
}

func (q *QueryBuilder) Offset(n int) *QueryBuilder {
	q.offset = n
	return q
}

func (q *QueryBuilder) Distinct() *QueryBuilder {
	q.distinct = true
	return q
}

func (q *QueryBuilder) Build() (string, []any) {
	cols := "*"
	if len(q.columns) > 0 {
		cols = joinStrings(q.columns)
	}

	distinct := ""
	if q.distinct {
		distinct = "DISTINCT "
	}

	query := fmt.Sprintf("SELECT %s%s FROM %s", distinct, cols, q.table)

	if len(q.where) > 0 {
		query += " WHERE " + strings.Join(q.where, " AND ")
	}

	if len(q.orderBy) > 0 {
		query += " ORDER BY " + joinStrings(q.orderBy)
	}

	if q.limit >= 0 {
		query += fmt.Sprintf(" LIMIT %d", q.limit)
	}

	if q.offset >= 0 {
		query += fmt.Sprintf(" OFFSET %d", q.offset)
	}

	return query, q.whereArgs
}

func (db *DB) Get(ctx context.Context, table string, columns []string, where string, whereArgs ...any) (map[string]any, error) {
	q := Query(table).Select(columns...).Where(where, whereArgs...).Limit(1)
	query, args := q.Build()

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, sql.ErrNoRows
	}
	return scanRowToMap(rows, columns)
}

func (db *DB) GetWithTx(ctx context.Context, tx *sql.Tx, table string, columns []string, where string, whereArgs ...any) (map[string]any, error) {
	q := Query(table).Select(columns...).Where(where, whereArgs...).Limit(1)
	query, args := q.Build()

	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, sql.ErrNoRows
	}
	return scanRowToMap(rows, columns)
}

func (db *DB) List(ctx context.Context, table string, columns []string, where string, whereArgs ...any) ([]map[string]any, error) {
	q := Query(table).Select(columns...)
	if where != "" {
		q.Where(where, whereArgs...)
	}
	query, args := q.Build()

	return db.queryRows(ctx, query, args, columns)
}

func (db *DB) ListWithTx(ctx context.Context, tx *sql.Tx, table string, columns []string, where string, whereArgs ...any) ([]map[string]any, error) {
	q := Query(table).Select(columns...)
	if where != "" {
		q.Where(where, whereArgs...)
	}
	query, args := q.Build()

	return queryRowsTx(ctx, tx, query, args, columns)
}

func (db *DB) Query(ctx context.Context, query string, args ...any) ([]map[string]any, error) {
	return db.queryRows(ctx, query, args, nil)
}

func (db *DB) QueryWithTx(ctx context.Context, tx *sql.Tx, query string, args ...any) ([]map[string]any, error) {
	return queryRowsTx(ctx, tx, query, args, nil)
}

func (db *DB) QueryRow(ctx context.Context, query string, args ...any) (map[string]any, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, sql.ErrNoRows
	}

	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	return scanRowToMap(rows, columns)
}

func (db *DB) QueryRowWithTx(ctx context.Context, tx *sql.Tx, query string, args ...any) (map[string]any, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, sql.ErrNoRows
	}

	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	return scanRowToMap(rows, columns)
}

func (db *DB) Count(ctx context.Context, table string, where string, whereArgs ...any) (int64, error) {
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", table)
	args := make([]any, 0, len(whereArgs))

	if where != "" {
		query += " WHERE " + where
		args = whereArgs
	}

	var count int64
	err := db.QueryRowContext(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("sqlite: count %q: %w", table, err)
	}
	return count, nil
}

func (db *DB) CountWithTx(ctx context.Context, tx *sql.Tx, table string, where string, whereArgs ...any) (int64, error) {
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", table)
	args := make([]any, 0, len(whereArgs))

	if where != "" {
		query += " WHERE " + where
		args = whereArgs
	}

	var count int64
	err := tx.QueryRowContext(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("sqlite: count %q: %w", table, err)
	}
	return count, nil
}

func (db *DB) Exists(ctx context.Context, table string, where string, whereArgs ...any) (bool, error) {
	count, err := db.Count(ctx, table, where, whereArgs...)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (db *DB) queryRows(ctx context.Context, query string, args []any, columns []string) ([]map[string]any, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: query: %w", err)
	}
	defer rows.Close()

	return scanRows(rows, columns)
}

func queryRowsTx(ctx context.Context, tx *sql.Tx, query string, args []any, columns []string) ([]map[string]any, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: query tx: %w", err)
	}
	defer rows.Close()

	return scanRows(rows, columns)
}

func scanRows(rows *sql.Rows, columns []string) ([]map[string]any, error) {
	if len(columns) == 0 {
		var err error
		columns, err = rows.Columns()
		if err != nil {
			return nil, err
		}
	}

	var results []map[string]any

	for rows.Next() {
		row, err := scanRowToMap(rows, columns)
		if err != nil {
			continue
		}
		results = append(results, row)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return results, nil
}

func scanRowToMap(row *sql.Rows, columns []string) (map[string]any, error) {
	if len(columns) == 0 {
		return nil, fmt.Errorf("sqlite: no columns specified")
	}

	values := make([]any, len(columns))
	valuePtrs := make([]any, len(columns))
	for i := range columns {
		valuePtrs[i] = &values[i]
	}

	if err := row.Scan(valuePtrs...); err != nil {
		return nil, err
	}

	result := make(map[string]any)
	for i, col := range columns {
		result[col] = values[i]
	}

	return result, nil
}
