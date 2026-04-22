package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type Transaction struct {
	*sql.Tx
	db        *DB
	completed bool
}

func (db *DB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*Transaction, error) {
	tx, err := db.DB.BeginTx(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("sqlite: begin transaction: %w", err)
	}
	return &Transaction{Tx: tx, db: db}, nil
}

func (db *DB) WithTransaction(ctx context.Context, opts *sql.TxOptions, fn func(tx *Transaction) error) error {
	tx, err := db.BeginTx(ctx, opts)
	if err != nil {
		return err
	}

	defer func() {
		if !tx.completed {
			if rbErr := tx.Rollback(); rbErr != nil {
			}
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("sqlite: transaction failed: %w, rollback error: %v", err, rbErr)
		}
		return err
	}

	return tx.Commit()
}

func (db *DB) WithTransactionRetry(ctx context.Context, opts *sql.TxOptions, maxRetries int, fn func(tx *Transaction) error) error {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		err := db.WithTransaction(ctx, opts, fn)
		if err == nil {
			return nil
		}

		lastErr = err

		if !isRetryableError(err) {
			return err
		}
	}
	return fmt.Errorf("sqlite: transaction failed after %d retries: %w", maxRetries, lastErr)
}

func (tx *Transaction) Commit() error {
	if tx.completed {
		return fmt.Errorf("sqlite: transaction already completed")
	}
	tx.completed = true
	if err := tx.Tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: commit transaction: %w", err)
	}
	return nil
}

func (tx *Transaction) Rollback() error {
	if tx.completed {
		return nil
	}
	tx.completed = true
	if err := tx.Tx.Rollback(); err != nil {
		return fmt.Errorf("sqlite: rollback transaction: %w", err)
	}
	return nil
}

func (tx *Transaction) Insert(ctx context.Context, table string, data map[string]any) (ExecResult, error) {
	return tx.db.InsertWithTx(ctx, tx.Tx, table, data)
}

func (tx *Transaction) Update(ctx context.Context, table string, data map[string]any, where string, whereArgs ...any) (int64, error) {
	return tx.db.UpdateWithTx(ctx, tx.Tx, table, data, where, whereArgs...)
}

func (tx *Transaction) Upsert(ctx context.Context, table string, data map[string]any, conflictColumns []string) (ExecResult, error) {
	return tx.db.UpsertWithTx(ctx, tx.Tx, table, data, conflictColumns)
}

func (tx *Transaction) Delete(ctx context.Context, table string, where string, whereArgs ...any) (int64, error) {
	return tx.db.DeleteWithTx(ctx, tx.Tx, table, where, whereArgs...)
}

func (tx *Transaction) Get(ctx context.Context, table string, columns []string, where string, whereArgs ...any) (map[string]any, error) {
	return tx.db.GetWithTx(ctx, tx.Tx, table, columns, where, whereArgs...)
}

func (tx *Transaction) List(ctx context.Context, table string, columns []string, where string, whereArgs ...any) ([]map[string]any, error) {
	return tx.db.ListWithTx(ctx, tx.Tx, table, columns, where, whereArgs...)
}

func (tx *Transaction) Query(ctx context.Context, query string, args ...any) ([]map[string]any, error) {
	return tx.db.QueryWithTx(ctx, tx.Tx, query, args...)
}

func (tx *Transaction) Count(ctx context.Context, table string, where string, whereArgs ...any) (int64, error) {
	return tx.db.CountWithTx(ctx, tx.Tx, table, where, whereArgs...)
}

func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	retryable := []string{"database is locked", "database is busy", "busy", "locked"}
	for _, substr := range retryable {
		if strings.Contains(errStr, substr) {
			return true
		}
	}
	return false
}
