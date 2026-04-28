package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"sync"
)

type ExecResult struct {
	LastInsertID int64
	RowsAffected int64
}

type Filter struct {
	Column string
	Op     string
	Value  any
}

type Order struct {
	Column string
	Desc   bool
}

type SelectOptions struct {
	Columns  []string
	Filters  []Filter
	Orders   []Order
	Limit    int
	Offset   int
	Distinct bool
}

type Transaction struct {
	tx        *sql.Tx
	mu        sync.Mutex
	completed bool
}

type sqlExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type sqlQuerier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func (db *DB) InsertRow(ctx context.Context, table string, data map[string]any) (ExecResult, error) {
	return insertRow(ctx, db, table, data)
}

func (tx *Transaction) InsertRow(ctx context.Context, table string, data map[string]any) (ExecResult, error) {
	if err := tx.ensureActive(); err != nil {
		return ExecResult{}, err
	}
	return insertRow(ctx, tx.tx, table, data)
}

func (db *DB) UpdateRows(ctx context.Context, table string, data map[string]any, filters ...Filter) (int64, error) {
	return updateRows(ctx, db, table, data, filters)
}

func (tx *Transaction) UpdateRows(ctx context.Context, table string, data map[string]any, filters ...Filter) (int64, error) {
	if err := tx.ensureActive(); err != nil {
		return 0, err
	}
	return updateRows(ctx, tx.tx, table, data, filters)
}

func (db *DB) UpsertRow(ctx context.Context, table string, data map[string]any, conflictColumns []string) (ExecResult, error) {
	return upsertRow(ctx, db, table, data, conflictColumns)
}

func (tx *Transaction) UpsertRow(ctx context.Context, table string, data map[string]any, conflictColumns []string) (ExecResult, error) {
	if err := tx.ensureActive(); err != nil {
		return ExecResult{}, err
	}
	return upsertRow(ctx, tx.tx, table, data, conflictColumns)
}

func (db *DB) DeleteRows(ctx context.Context, table string, filters ...Filter) (int64, error) {
	return deleteRows(ctx, db, table, filters)
}

func (tx *Transaction) DeleteRows(ctx context.Context, table string, filters ...Filter) (int64, error) {
	if err := tx.ensureActive(); err != nil {
		return 0, err
	}
	return deleteRows(ctx, tx.tx, table, filters)
}

func (db *DB) SelectRows(ctx context.Context, table string, opts SelectOptions) ([]map[string]any, error) {
	return selectRows(ctx, db, table, opts)
}

func (tx *Transaction) SelectRows(ctx context.Context, table string, opts SelectOptions) ([]map[string]any, error) {
	if err := tx.ensureActive(); err != nil {
		return nil, err
	}
	return selectRows(ctx, tx.tx, table, opts)
}

func (db *DB) SelectOne(ctx context.Context, table string, opts SelectOptions) (map[string]any, error) {
	return selectOne(ctx, db, table, opts)
}

func (tx *Transaction) SelectOne(ctx context.Context, table string, opts SelectOptions) (map[string]any, error) {
	if err := tx.ensureActive(); err != nil {
		return nil, err
	}
	return selectOne(ctx, tx.tx, table, opts)
}

func (db *DB) CountRows(ctx context.Context, table string, filters ...Filter) (int64, error) {
	return countRows(ctx, db, table, filters)
}

func (tx *Transaction) CountRows(ctx context.Context, table string, filters ...Filter) (int64, error) {
	if err := tx.ensureActive(); err != nil {
		return 0, err
	}
	return countRows(ctx, tx.tx, table, filters)
}

func (db *DB) RowExists(ctx context.Context, table string, filters ...Filter) (bool, error) {
	count, err := db.CountRows(ctx, table, filters...)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (tx *Transaction) RowExists(ctx context.Context, table string, filters ...Filter) (bool, error) {
	count, err := tx.CountRows(ctx, table, filters...)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (db *DB) BeginTransaction(ctx context.Context, opts *sql.TxOptions) (*Transaction, error) {
	tx, err := db.DB.BeginTx(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("sqlite: begin transaction: %w", err)
	}
	return &Transaction{tx: tx}, nil
}

func (db *DB) WithTransaction(ctx context.Context, opts *sql.TxOptions, fn func(tx *Transaction) error) error {
	tx, err := db.BeginTransaction(ctx, opts)
	if err != nil {
		return err
	}

	defer func() {
		_ = tx.Rollback()
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("sqlite: transaction failed: %w; rollback failed: %v", err, rbErr)
		}
		return err
	}
	return tx.Commit()
}

func (tx *Transaction) Commit() error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.completed {
		return fmt.Errorf("sqlite: transaction already completed")
	}
	tx.completed = true
	if err := tx.tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: commit transaction: %w", err)
	}
	return nil
}

func (tx *Transaction) Rollback() error {
	tx.mu.Lock()
	defer tx.mu.Unlock()

	if tx.completed {
		return nil
	}
	tx.completed = true
	if err := tx.tx.Rollback(); err != nil {
		return fmt.Errorf("sqlite: rollback transaction: %w", err)
	}
	return nil
}

func (tx *Transaction) ensureActive() error {
	if tx == nil || tx.tx == nil {
		return fmt.Errorf("sqlite: transaction is nil")
	}
	tx.mu.Lock()
	defer tx.mu.Unlock()
	if tx.completed {
		return fmt.Errorf("sqlite: transaction already completed")
	}
	return nil
}

func insertRow(ctx context.Context, exec sqlExecutor, table string, data map[string]any) (ExecResult, error) {
	if len(data) == 0 {
		return ExecResult{}, fmt.Errorf("sqlite: insert row: no data provided")
	}
	tableSQL, err := quoteQualifiedIdentifier(table)
	if err != nil {
		return ExecResult{}, err
	}

	columns := sortedKeys(data)
	columnSQL := make([]string, 0, len(columns))
	placeholders := make([]string, 0, len(columns))
	args := make([]any, 0, len(columns))
	for _, column := range columns {
		quoted, err := quoteIdentifier(column)
		if err != nil {
			return ExecResult{}, err
		}
		columnSQL = append(columnSQL, quoted)
		placeholders = append(placeholders, "?")
		args = append(args, data[column])
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", tableSQL, strings.Join(columnSQL, ", "), strings.Join(placeholders, ", "))
	result, err := exec.ExecContext(ctx, query, args...)
	if err != nil {
		return ExecResult{}, fmt.Errorf("sqlite: insert row into %q: %w", table, err)
	}
	return execResult(result), nil
}

func updateRows(ctx context.Context, exec sqlExecutor, table string, data map[string]any, filters []Filter) (int64, error) {
	if len(data) == 0 {
		return 0, fmt.Errorf("sqlite: update rows: no data provided")
	}
	if len(filters) == 0 {
		return 0, fmt.Errorf("sqlite: update rows: at least one filter is required")
	}
	tableSQL, err := quoteQualifiedIdentifier(table)
	if err != nil {
		return 0, err
	}

	columns := sortedKeys(data)
	setClauses := make([]string, 0, len(columns))
	args := make([]any, 0, len(columns)+len(filters))
	for _, column := range columns {
		quoted, err := quoteIdentifier(column)
		if err != nil {
			return 0, err
		}
		setClauses = append(setClauses, fmt.Sprintf("%s = ?", quoted))
		args = append(args, data[column])
	}

	whereSQL, whereArgs, err := buildWhereClause(filters)
	if err != nil {
		return 0, err
	}
	args = append(args, whereArgs...)

	query := fmt.Sprintf("UPDATE %s SET %s WHERE %s", tableSQL, strings.Join(setClauses, ", "), whereSQL)
	result, err := exec.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("sqlite: update rows in %q: %w", table, err)
	}
	affected, _ := result.RowsAffected()
	return affected, nil
}

func upsertRow(ctx context.Context, exec sqlExecutor, table string, data map[string]any, conflictColumns []string) (ExecResult, error) {
	if len(data) == 0 {
		return ExecResult{}, fmt.Errorf("sqlite: upsert row: no data provided")
	}
	if len(conflictColumns) == 0 {
		return ExecResult{}, fmt.Errorf("sqlite: upsert row: conflict columns are required")
	}

	tableSQL, err := quoteQualifiedIdentifier(table)
	if err != nil {
		return ExecResult{}, err
	}

	conflicts := make(map[string]struct{}, len(conflictColumns))
	conflictSQL := make([]string, 0, len(conflictColumns))
	for _, column := range conflictColumns {
		quoted, err := quoteIdentifier(column)
		if err != nil {
			return ExecResult{}, err
		}
		if _, ok := data[column]; !ok {
			return ExecResult{}, fmt.Errorf("sqlite: conflict column %q is missing from upsert data", column)
		}
		conflicts[column] = struct{}{}
		conflictSQL = append(conflictSQL, quoted)
	}

	columns := sortedKeys(data)
	columnSQL := make([]string, 0, len(columns))
	placeholders := make([]string, 0, len(columns))
	updateClauses := make([]string, 0, len(columns))
	args := make([]any, 0, len(columns))
	for _, column := range columns {
		quoted, err := quoteIdentifier(column)
		if err != nil {
			return ExecResult{}, err
		}
		columnSQL = append(columnSQL, quoted)
		placeholders = append(placeholders, "?")
		args = append(args, data[column])
		if _, ok := conflicts[column]; !ok {
			updateClauses = append(updateClauses, fmt.Sprintf("%s = excluded.%s", quoted, quoted))
		}
	}
	if len(updateClauses) == 0 {
		updateClauses = append(updateClauses, fmt.Sprintf("%s = %s", conflictSQL[0], conflictSQL[0]))
	}

	query := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s) ON CONFLICT (%s) DO UPDATE SET %s",
		tableSQL,
		strings.Join(columnSQL, ", "),
		strings.Join(placeholders, ", "),
		strings.Join(conflictSQL, ", "),
		strings.Join(updateClauses, ", "),
	)
	result, err := exec.ExecContext(ctx, query, args...)
	if err != nil {
		return ExecResult{}, fmt.Errorf("sqlite: upsert row into %q: %w", table, err)
	}
	return execResult(result), nil
}

func deleteRows(ctx context.Context, exec sqlExecutor, table string, filters []Filter) (int64, error) {
	if len(filters) == 0 {
		return 0, fmt.Errorf("sqlite: delete rows: at least one filter is required")
	}
	tableSQL, err := quoteQualifiedIdentifier(table)
	if err != nil {
		return 0, err
	}
	whereSQL, args, err := buildWhereClause(filters)
	if err != nil {
		return 0, err
	}

	query := fmt.Sprintf("DELETE FROM %s WHERE %s", tableSQL, whereSQL)
	result, err := exec.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("sqlite: delete rows from %q: %w", table, err)
	}
	affected, _ := result.RowsAffected()
	return affected, nil
}

func selectRows(ctx context.Context, querier sqlQuerier, table string, opts SelectOptions) ([]map[string]any, error) {
	query, args, err := buildSelect(table, opts)
	if err != nil {
		return nil, err
	}
	rows, err := querier.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: select rows from %q: %w", table, err)
	}
	defer rows.Close()
	return scanRowsToMaps(rows)
}

func selectOne(ctx context.Context, querier sqlQuerier, table string, opts SelectOptions) (map[string]any, error) {
	opts.Limit = 1
	rows, err := selectRows(ctx, querier, table, opts)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, sql.ErrNoRows
	}
	return rows[0], nil
}

func countRows(ctx context.Context, querier sqlQuerier, table string, filters []Filter) (int64, error) {
	tableSQL, err := quoteQualifiedIdentifier(table)
	if err != nil {
		return 0, err
	}
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", tableSQL)
	args := []any(nil)
	if len(filters) > 0 {
		whereSQL, whereArgs, err := buildWhereClause(filters)
		if err != nil {
			return 0, err
		}
		query += " WHERE " + whereSQL
		args = whereArgs
	}

	var count int64
	if err := querier.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("sqlite: count rows in %q: %w", table, err)
	}
	return count, nil
}

func buildSelect(table string, opts SelectOptions) (string, []any, error) {
	tableSQL, err := quoteQualifiedIdentifier(table)
	if err != nil {
		return "", nil, err
	}

	columnsSQL := "*"
	if len(opts.Columns) > 0 {
		columns := make([]string, 0, len(opts.Columns))
		for _, column := range opts.Columns {
			quoted, err := quoteIdentifier(column)
			if err != nil {
				return "", nil, err
			}
			columns = append(columns, quoted)
		}
		columnsSQL = strings.Join(columns, ", ")
	}

	distinct := ""
	if opts.Distinct {
		distinct = "DISTINCT "
	}
	query := fmt.Sprintf("SELECT %s%s FROM %s", distinct, columnsSQL, tableSQL)

	args := []any(nil)
	if len(opts.Filters) > 0 {
		whereSQL, whereArgs, err := buildWhereClause(opts.Filters)
		if err != nil {
			return "", nil, err
		}
		query += " WHERE " + whereSQL
		args = whereArgs
	}

	if len(opts.Orders) > 0 {
		orders := make([]string, 0, len(opts.Orders))
		for _, order := range opts.Orders {
			quoted, err := quoteIdentifier(order.Column)
			if err != nil {
				return "", nil, err
			}
			direction := "ASC"
			if order.Desc {
				direction = "DESC"
			}
			orders = append(orders, quoted+" "+direction)
		}
		query += " ORDER BY " + strings.Join(orders, ", ")
	}

	if opts.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", opts.Limit)
	} else if opts.Offset > 0 {
		query += " LIMIT -1"
	}
	if opts.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", opts.Offset)
	}

	return query, args, nil
}

func buildWhereClause(filters []Filter) (string, []any, error) {
	clauses := make([]string, 0, len(filters))
	args := make([]any, 0, len(filters))
	for _, filter := range filters {
		column, err := quoteIdentifier(filter.Column)
		if err != nil {
			return "", nil, err
		}
		op, err := normalizeFilterOp(filter.Op)
		if err != nil {
			return "", nil, err
		}
		if filter.Value == nil {
			switch op {
			case "=":
				clauses = append(clauses, column+" IS NULL")
			case "!=", "<>":
				clauses = append(clauses, column+" IS NOT NULL")
			default:
				return "", nil, fmt.Errorf("sqlite: nil filter value only supports equality operators")
			}
			continue
		}
		clauses = append(clauses, column+" "+op+" ?")
		args = append(args, filter.Value)
	}
	return strings.Join(clauses, " AND "), args, nil
}

func normalizeFilterOp(op string) (string, error) {
	op = strings.ToUpper(strings.TrimSpace(op))
	if op == "" {
		op = "="
	}
	switch op {
	case "=", "!=", "<>", "<", "<=", ">", ">=", "LIKE":
		return op, nil
	default:
		return "", fmt.Errorf("sqlite: unsupported filter operator %q", op)
	}
}

func quoteQualifiedIdentifier(name string) (string, error) {
	parts := strings.Split(name, ".")
	if len(parts) == 0 || len(parts) > 2 {
		return "", fmt.Errorf("sqlite: invalid identifier %q", name)
	}
	quoted := make([]string, 0, len(parts))
	for _, part := range parts {
		q, err := quoteIdentifier(part)
		if err != nil {
			return "", err
		}
		quoted = append(quoted, q)
	}
	return strings.Join(quoted, "."), nil
}

func quoteIdentifier(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("sqlite: identifier is required")
	}
	for i, r := range name {
		isLetter := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
		isDigit := r >= '0' && r <= '9'
		if isLetter || r == '_' || i > 0 && isDigit {
			continue
		}
		return "", fmt.Errorf("sqlite: invalid identifier %q", name)
	}
	return `"` + name + `"`, nil
}

func sortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func scanRowsToMaps(rows *sql.Rows) ([]map[string]any, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	results := make([]map[string]any, 0)
	for rows.Next() {
		values := make([]any, len(columns))
		valuePtrs := make([]any, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, err
		}
		row := make(map[string]any, len(columns))
		for i, column := range columns {
			row[column] = values[i]
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func execResult(result sql.Result) ExecResult {
	lastID, _ := result.LastInsertId()
	affected, _ := result.RowsAffected()
	return ExecResult{
		LastInsertID: lastID,
		RowsAffected: affected,
	}
}
