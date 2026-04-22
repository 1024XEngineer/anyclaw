package sqlite

import (
	"context"
	"database/sql"
	"fmt"
)

type ExecResult struct {
	LastInsertId int64
	RowsAffected int64
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func (db *DB) Insert(ctx context.Context, table string, data map[string]any) (ExecResult, error) {
	return execInsert(ctx, db.DB, table, data)
}

func (db *DB) InsertWithTx(ctx context.Context, tx *sql.Tx, table string, data map[string]any) (ExecResult, error) {
	return execInsert(ctx, tx, table, data)
}

func execInsert(ctx context.Context, e execer, table string, data map[string]any) (ExecResult, error) {
	if len(data) == 0 {
		return ExecResult{}, fmt.Errorf("sqlite: insert: no data provided")
	}

	columns := make([]string, 0, len(data))
	values := make([]any, 0, len(data))
	placeholders := make([]string, 0, len(data))

	i := 1
	for col, val := range data {
		columns = append(columns, col)
		values = append(values, val)
		placeholders = append(placeholders, fmt.Sprintf("$%d", i))
		i++
	}

	query := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s)",
		table,
		joinColumns(columns),
		joinStrings(placeholders),
	)

	result, err := e.ExecContext(ctx, query, values...)
	if err != nil {
		return ExecResult{}, fmt.Errorf("sqlite: insert into %q: %w", table, err)
	}

	lastID, _ := result.LastInsertId()
	affected, _ := result.RowsAffected()

	return ExecResult{
		LastInsertId: lastID,
		RowsAffected: affected,
	}, nil
}

func (db *DB) Update(ctx context.Context, table string, data map[string]any, where string, whereArgs ...any) (int64, error) {
	return execUpdate(ctx, db.DB, table, data, where, whereArgs...)
}

func (db *DB) UpdateWithTx(ctx context.Context, tx *sql.Tx, table string, data map[string]any, where string, whereArgs ...any) (int64, error) {
	return execUpdate(ctx, tx, table, data, where, whereArgs...)
}

func execUpdate(ctx context.Context, e execer, table string, data map[string]any, where string, whereArgs ...any) (int64, error) {
	if len(data) == 0 {
		return 0, fmt.Errorf("sqlite: update: no data provided")
	}

	setClauses := make([]string, 0, len(data))
	values := make([]any, 0, len(data))

	i := 1
	for col, val := range data {
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", col, i))
		values = append(values, val)
		i++
	}

	query := fmt.Sprintf(
		"UPDATE %s SET %s",
		table,
		joinStrings(setClauses),
	)

	if where != "" {
		query += " WHERE " + where
		values = append(values, whereArgs...)
	}

	result, err := e.ExecContext(ctx, query, values...)
	if err != nil {
		return 0, fmt.Errorf("sqlite: update %q: %w", table, err)
	}

	affected, _ := result.RowsAffected()
	return affected, nil
}

func (db *DB) Upsert(ctx context.Context, table string, data map[string]any, conflictColumns []string) (ExecResult, error) {
	return execUpsert(ctx, db.DB, table, data, conflictColumns)
}

func (db *DB) UpsertWithTx(ctx context.Context, tx *sql.Tx, table string, data map[string]any, conflictColumns []string) (ExecResult, error) {
	return execUpsert(ctx, tx, table, data, conflictColumns)
}

func execUpsert(ctx context.Context, e execer, table string, data map[string]any, conflictColumns []string) (ExecResult, error) {
	if len(data) == 0 {
		return ExecResult{}, fmt.Errorf("sqlite: upsert: no data provided")
	}
	if len(conflictColumns) == 0 {
		return ExecResult{}, fmt.Errorf("sqlite: upsert: no conflict columns provided")
	}

	columns := make([]string, 0, len(data))
	values := make([]any, 0, len(data))
	placeholders := make([]string, 0, len(data))

	i := 1
	for col, val := range data {
		columns = append(columns, col)
		values = append(values, val)
		placeholders = append(placeholders, fmt.Sprintf("$%d", i))
		i++
	}

	updateClauses := make([]string, 0, len(data)-len(conflictColumns))
	conflictSet := make(map[string]bool)
	for _, c := range conflictColumns {
		conflictSet[c] = true
	}

	for _, col := range columns {
		if !conflictSet[col] {
			updateClauses = append(updateClauses, fmt.Sprintf("%s = excluded.%s", col, col))
		}
	}

	query := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s) ON CONFLICT (%s) DO UPDATE SET %s",
		table,
		joinColumns(columns),
		joinStrings(placeholders),
		joinStrings(conflictColumns),
		joinStrings(updateClauses),
	)

	result, err := e.ExecContext(ctx, query, values...)
	if err != nil {
		return ExecResult{}, fmt.Errorf("sqlite: upsert into %q: %w", table, err)
	}

	lastID, _ := result.LastInsertId()
	affected, _ := result.RowsAffected()

	return ExecResult{
		LastInsertId: lastID,
		RowsAffected: affected,
	}, nil
}

func (db *DB) Delete(ctx context.Context, table string, where string, whereArgs ...any) (int64, error) {
	return execDelete(ctx, db.DB, table, where, whereArgs...)
}

func (db *DB) DeleteWithTx(ctx context.Context, tx *sql.Tx, table string, where string, whereArgs ...any) (int64, error) {
	return execDelete(ctx, tx, table, where, whereArgs...)
}

func execDelete(ctx context.Context, e execer, table string, where string, whereArgs ...any) (int64, error) {
	query := fmt.Sprintf("DELETE FROM %s", table)
	args := make([]any, 0, len(whereArgs))

	if where != "" {
		query += " WHERE " + where
		args = whereArgs
	}

	result, err := e.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("sqlite: delete from %q: %w", table, err)
	}

	affected, _ := result.RowsAffected()
	return affected, nil
}

func joinColumns(cols []string) string {
	result := ""
	for i, c := range cols {
		if i > 0 {
			result += ", "
		}
		result += c
	}
	return result
}

func joinStrings(parts []string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += ", "
		}
		result += p
	}
	return result
}
