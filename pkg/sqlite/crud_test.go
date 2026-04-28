package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
)

func TestCRUDHelpersInsertSelectUpdateDelete(t *testing.T) {
	db := setupTestDB(t, InMemoryConfig())
	ctx := context.Background()

	inserted, err := db.InsertRow(ctx, "test", map[string]any{
		"name":  "alpha",
		"value": "one",
	})
	if err != nil {
		t.Fatalf("InsertRow: %v", err)
	}
	if inserted.LastInsertID == 0 || inserted.RowsAffected != 1 {
		t.Fatalf("inserted = %+v, want last ID and one affected row", inserted)
	}

	row, err := db.SelectOne(ctx, "test", SelectOptions{
		Columns: []string{"id", "name", "value"},
		Filters: []Filter{{Column: "name", Value: "alpha"}},
	})
	if err != nil {
		t.Fatalf("SelectOne: %v", err)
	}
	if row["name"] != "alpha" || row["value"] != "one" {
		t.Fatalf("row = %+v, want inserted values", row)
	}

	updated, err := db.UpdateRows(ctx, "test", map[string]any{"value": "two"}, Filter{Column: "name", Value: "alpha"})
	if err != nil {
		t.Fatalf("UpdateRows: %v", err)
	}
	if updated != 1 {
		t.Fatalf("updated = %d, want 1", updated)
	}

	exists, err := db.RowExists(ctx, "test", Filter{Column: "value", Op: "=", Value: "two"})
	if err != nil {
		t.Fatalf("RowExists: %v", err)
	}
	if !exists {
		t.Fatal("expected updated row to exist")
	}

	count, err := db.CountRows(ctx, "test", Filter{Column: "value", Op: "LIKE", Value: "t%"})
	if err != nil {
		t.Fatalf("CountRows: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}

	deleted, err := db.DeleteRows(ctx, "test", Filter{Column: "name", Value: "alpha"})
	if err != nil {
		t.Fatalf("DeleteRows: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
}

func TestCRUDHelpersSelectOptions(t *testing.T) {
	db := setupTestDB(t, InMemoryConfig())
	ctx := context.Background()
	for _, name := range []string{"charlie", "bravo", "alpha"} {
		if _, err := db.InsertRow(ctx, "test", map[string]any{"name": name, "value": name}); err != nil {
			t.Fatalf("InsertRow %s: %v", name, err)
		}
	}

	rows, err := db.SelectRows(ctx, "test", SelectOptions{})
	if err != nil {
		t.Fatalf("SelectRows default options: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("default select rows = %d, want all rows", len(rows))
	}

	rows, err = db.SelectRows(ctx, "test", SelectOptions{
		Columns: []string{"name"},
		Orders:  []Order{{Column: "name", Desc: false}},
		Limit:   2,
		Offset:  1,
	})
	if err != nil {
		t.Fatalf("SelectRows ordered window: %v", err)
	}
	if len(rows) != 2 || rows[0]["name"] != "bravo" || rows[1]["name"] != "charlie" {
		t.Fatalf("rows = %+v, want bravo/charlie window", rows)
	}
}

func TestCRUDHelpersUpsertRow(t *testing.T) {
	db := setupTestDB(t, InMemoryConfig())
	ctx := context.Background()

	if _, err := db.ExecContext(ctx, `CREATE UNIQUE INDEX test_name_unique ON test(name)`); err != nil {
		t.Fatalf("create unique index: %v", err)
	}
	if _, err := db.UpsertRow(ctx, "test", map[string]any{"name": "alpha", "value": "one"}, []string{"name"}); err != nil {
		t.Fatalf("UpsertRow insert: %v", err)
	}
	if _, err := db.UpsertRow(ctx, "test", map[string]any{"name": "alpha", "value": "two"}, []string{"name"}); err != nil {
		t.Fatalf("UpsertRow update: %v", err)
	}

	row, err := db.SelectOne(ctx, "test", SelectOptions{
		Columns: []string{"value"},
		Filters: []Filter{{Column: "name", Value: "alpha"}},
	})
	if err != nil {
		t.Fatalf("SelectOne after upsert: %v", err)
	}
	if row["value"] != "two" {
		t.Fatalf("value = %v, want two", row["value"])
	}
}

func TestCRUDHelpersNilFilters(t *testing.T) {
	db := setupTestDB(t, InMemoryConfig())
	ctx := context.Background()

	if _, err := db.InsertRow(ctx, "test", map[string]any{"name": "alpha", "value": nil}); err != nil {
		t.Fatalf("InsertRow nil value: %v", err)
	}

	exists, err := db.RowExists(ctx, "test", Filter{Column: "value", Value: nil})
	if err != nil {
		t.Fatalf("RowExists nil: %v", err)
	}
	if !exists {
		t.Fatal("expected IS NULL filter to match")
	}

	exists, err = db.RowExists(ctx, "test", Filter{Column: "value", Op: "!=", Value: nil})
	if err != nil {
		t.Fatalf("RowExists not nil: %v", err)
	}
	if exists {
		t.Fatal("expected IS NOT NULL filter not to match")
	}
}

func TestCRUDHelpersRejectUnsafeSQLFragments(t *testing.T) {
	db := setupTestDB(t, InMemoryConfig())
	ctx := context.Background()

	tests := []struct {
		name string
		fn   func() error
		want string
	}{
		{
			name: "unsafe table",
			fn: func() error {
				_, err := db.SelectRows(ctx, "test; DROP TABLE test", SelectOptions{})
				return err
			},
			want: "invalid identifier",
		},
		{
			name: "unsafe column",
			fn: func() error {
				_, err := db.InsertRow(ctx, "test", map[string]any{"name) VALUES ('x');--": "alpha"})
				return err
			},
			want: "invalid identifier",
		},
		{
			name: "unsafe order",
			fn: func() error {
				_, err := db.SelectRows(ctx, "test", SelectOptions{Orders: []Order{{Column: "name DESC; DROP TABLE test"}}})
				return err
			},
			want: "invalid identifier",
		},
		{
			name: "unsupported operator",
			fn: func() error {
				_, err := db.SelectRows(ctx, "test", SelectOptions{Filters: []Filter{{Column: "name", Op: "IN", Value: "alpha"}}})
				return err
			},
			want: "unsupported filter operator",
		},
		{
			name: "missing upsert conflict data",
			fn: func() error {
				_, err := db.UpsertRow(ctx, "test", map[string]any{"value": "alpha"}, []string{"name"})
				return err
			},
			want: "missing from upsert data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestCRUDHelpersRequireFiltersForDestructiveMutations(t *testing.T) {
	db := setupTestDB(t, InMemoryConfig())
	ctx := context.Background()

	if _, err := db.UpdateRows(ctx, "test", map[string]any{"value": "all"}); err == nil {
		t.Fatal("expected UpdateRows without filters to fail")
	}
	if _, err := db.DeleteRows(ctx, "test"); err == nil {
		t.Fatal("expected DeleteRows without filters to fail")
	}
}

func TestCRUDTransactionCommitAndRollback(t *testing.T) {
	db := setupTestDB(t, InMemoryConfig())
	ctx := context.Background()

	if err := db.WithTransaction(ctx, nil, func(tx *Transaction) error {
		if _, err := tx.InsertRow(ctx, "test", map[string]any{"name": "committed", "value": "ok"}); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatalf("WithTransaction commit: %v", err)
	}

	exists, err := db.RowExists(ctx, "test", Filter{Column: "name", Value: "committed"})
	if err != nil {
		t.Fatalf("RowExists committed: %v", err)
	}
	if !exists {
		t.Fatal("expected committed row")
	}

	err = db.WithTransaction(ctx, nil, func(tx *Transaction) error {
		if _, err := tx.InsertRow(ctx, "test", map[string]any{"name": "rolled-back", "value": "no"}); err != nil {
			return err
		}
		return errors.New("stop")
	})
	if err == nil {
		t.Fatal("expected transaction callback error")
	}

	exists, err = db.RowExists(ctx, "test", Filter{Column: "name", Value: "rolled-back"})
	if err != nil {
		t.Fatalf("RowExists rolled-back: %v", err)
	}
	if exists {
		t.Fatal("expected rolled-back row not to exist")
	}
}

func TestCRUDTransactionRejectsUseAfterCompletion(t *testing.T) {
	db := setupTestDB(t, InMemoryConfig())
	ctx := context.Background()

	tx, err := db.BeginTransaction(ctx, &sql.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTransaction: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	_, err = tx.InsertRow(ctx, "test", map[string]any{"name": "after", "value": "done"})
	if err == nil || !strings.Contains(err.Error(), "already completed") {
		t.Fatalf("InsertRow after rollback err = %v, want already completed", err)
	}
}
