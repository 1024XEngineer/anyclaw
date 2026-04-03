package sqlite

import (
	"context"
	"database/sql"
	"testing"
)

func setupTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(DefaultConfig(":memory:"))
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}

	ctx := context.Background()
	_, err = db.ExecContext(ctx, `CREATE TABLE users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		email TEXT UNIQUE,
		age INTEGER,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
	})

	return db
}

func TestOpenAndClose(t *testing.T) {
	db, err := Open(DefaultConfig(":memory:"))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if err := db.Close(); err != nil {
		t.Fatalf("expected no error on close, got %v", err)
	}

	if err := db.Close(); err != nil {
		t.Fatalf("expected no error on double close, got %v", err)
	}
}

func TestPing(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	if err := db.Ping(ctx); err != nil {
		t.Fatalf("expected no error on ping, got %v", err)
	}
}

func TestInsert(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	result, err := db.Insert(ctx, "users", map[string]any{
		"name":  "Alice",
		"email": "alice@example.com",
		"age":   30,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if result.RowsAffected != 1 {
		t.Errorf("expected 1 row affected, got %d", result.RowsAffected)
	}
}

func TestInsertEmptyData(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	_, err := db.Insert(ctx, "users", map[string]any{})
	if err == nil {
		t.Fatal("expected error for empty data")
	}
}

func TestGet(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	_, err := db.Insert(ctx, "users", map[string]any{
		"name":  "Bob",
		"email": "bob@example.com",
		"age":   25,
	})
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}

	row, err := db.Get(ctx, "users", []string{"id", "name", "email", "age"}, "name = ?", "Bob")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if row["name"] != "Bob" {
		t.Errorf("expected name Bob, got %v", row["name"])
	}
	if row["age"].(int64) != 25 {
		t.Errorf("expected age 25, got %v", row["age"])
	}
}

func TestGetNotFound(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	_, err := db.Get(ctx, "users", []string{"id", "name"}, "name = ?", "NonExistent")
	if err != sql.ErrNoRows {
		t.Errorf("expected ErrNoRows, got %v", err)
	}
}

func TestList(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	_, err := db.Insert(ctx, "users", map[string]any{"name": "Alice", "email": "alice@example.com", "age": 30})
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}
	_, err = db.Insert(ctx, "users", map[string]any{"name": "Bob", "email": "bob@example.com", "age": 25})
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}

	rows, err := db.List(ctx, "users", []string{"id", "name", "age"}, "", nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(rows))
	}
}

func TestListWithWhere(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	_, err := db.Insert(ctx, "users", map[string]any{"name": "Alice", "email": "alice@example.com", "age": 30})
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}
	_, err = db.Insert(ctx, "users", map[string]any{"name": "Bob", "email": "bob@example.com", "age": 25})
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}

	rows, err := db.List(ctx, "users", []string{"id", "name"}, "age > ?", 28)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(rows) != 1 {
		t.Errorf("expected 1 row, got %d", len(rows))
	}
	if rows[0]["name"] != "Alice" {
		t.Errorf("expected Alice, got %v", rows[0]["name"])
	}
}

func TestUpdate(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	_, err := db.Insert(ctx, "users", map[string]any{"name": "Charlie", "email": "charlie@example.com", "age": 35})
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}

	affected, err := db.Update(ctx, "users", map[string]any{"age": 36}, "name = ?", "Charlie")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if affected != 1 {
		t.Errorf("expected 1 row affected, got %d", affected)
	}

	row, err := db.Get(ctx, "users", []string{"age"}, "name = ?", "Charlie")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if row["age"].(int64) != 36 {
		t.Errorf("expected age 36, got %v", row["age"])
	}
}

func TestUpsert(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	result, err := db.Upsert(ctx, "users", map[string]any{
		"name":  "Dave",
		"email": "dave@example.com",
		"age":   40,
	}, []string{"email"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if result.RowsAffected != 1 {
		t.Errorf("expected 1 row affected on insert, got %d", result.RowsAffected)
	}

	result, err = db.Upsert(ctx, "users", map[string]any{
		"name":  "Dave Updated",
		"email": "dave@example.com",
		"age":   41,
	}, []string{"email"})
	if err != nil {
		t.Fatalf("expected no error on upsert, got %v", err)
	}

	row, err := db.Get(ctx, "users", []string{"name", "age"}, "email = ?", "dave@example.com")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if row["name"] != "Dave Updated" {
		t.Errorf("expected name 'Dave Updated', got %v", row["name"])
	}
	if row["age"].(int64) != 41 {
		t.Errorf("expected age 41, got %v", row["age"])
	}
}

func TestDelete(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	_, err := db.Insert(ctx, "users", map[string]any{"name": "Eve", "email": "eve@example.com", "age": 28})
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}

	affected, err := db.Delete(ctx, "users", "name = ?", "Eve")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if affected != 1 {
		t.Errorf("expected 1 row affected, got %d", affected)
	}

	_, err = db.Get(ctx, "users", []string{"id"}, "name = ?", "Eve")
	if err != sql.ErrNoRows {
		t.Errorf("expected ErrNoRows after delete, got %v", err)
	}
}

func TestCount(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	_, err := db.Insert(ctx, "users", map[string]any{"name": "User1", "email": "user1@example.com", "age": 20})
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}
	_, err = db.Insert(ctx, "users", map[string]any{"name": "User2", "email": "user2@example.com", "age": 30})
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}

	count, err := db.Count(ctx, "users", "", nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if count != 2 {
		t.Errorf("expected count 2, got %d", count)
	}

	count, err = db.Count(ctx, "users", "age > ?", 25)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if count != 1 {
		t.Errorf("expected count 1, got %d", count)
	}
}

func TestExists(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	_, err := db.Insert(ctx, "users", map[string]any{"name": "Frank", "email": "frank@example.com", "age": 45})
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}

	exists, err := db.Exists(ctx, "users", "name = ?", "Frank")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !exists {
		t.Error("expected exists to be true")
	}

	exists, err = db.Exists(ctx, "users", "name = ?", "Ghost")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if exists {
		t.Error("expected exists to be false")
	}
}

func TestTransactionCommit(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	err := db.WithTransaction(ctx, nil, func(tx *Transaction) error {
		_, err := tx.Insert(ctx, "users", map[string]any{"name": "TxUser", "email": "tx@example.com", "age": 50})
		if err != nil {
			return err
		}

		_, err = tx.Insert(ctx, "users", map[string]any{"name": "TxUser2", "email": "tx2@example.com", "age": 51})
		return err
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	count, err := db.Count(ctx, "users", "name LIKE ?", "TxUser%")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if count != 2 {
		t.Errorf("expected 2 users after commit, got %d", count)
	}
}

func TestTransactionRollback(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	err := db.WithTransaction(ctx, nil, func(tx *Transaction) error {
		_, err := tx.Insert(ctx, "users", map[string]any{"name": "RollbackUser", "email": "rollback@example.com", "age": 60})
		if err != nil {
			return err
		}

		return sql.ErrTxDone
	})
	if err == nil {
		t.Fatal("expected error from transaction")
	}

	count, err := db.Count(ctx, "users", "name = ?", "RollbackUser")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if count != 0 {
		t.Errorf("expected 0 users after rollback, got %d", count)
	}
}

func TestTransactionManualCommit(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("failed to begin tx: %v", err)
	}

	_, err = tx.Insert(ctx, "users", map[string]any{"name": "ManualUser", "email": "manual@example.com", "age": 70})
	if err != nil {
		t.Fatalf("failed to insert in tx: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	row, err := db.Get(ctx, "users", []string{"name"}, "name = ?", "ManualUser")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if row["name"] != "ManualUser" {
		t.Errorf("expected ManualUser, got %v", row["name"])
	}
}

func TestTransactionManualRollback(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("failed to begin tx: %v", err)
	}

	_, err = tx.Insert(ctx, "users", map[string]any{"name": "RollbackManual", "email": "rb@example.com", "age": 80})
	if err != nil {
		t.Fatalf("failed to insert in tx: %v", err)
	}

	if err := tx.Rollback(); err != nil {
		t.Fatalf("failed to rollback: %v", err)
	}

	count, err := db.Count(ctx, "users", "name = ?", "RollbackManual")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if count != 0 {
		t.Errorf("expected 0 users after rollback, got %d", count)
	}
}

func TestQueryBuilder(t *testing.T) {
	q := Query("users").
		Select("id", "name", "email").
		Where("age > ?", 25).
		Where("name LIKE ?", "%a%").
		OrderBy("name ASC").
		Limit(10).
		Offset(5)

	query, args := q.Build()

	expected := "SELECT id, name, email FROM users WHERE age > ? AND name LIKE ? ORDER BY name ASC LIMIT 10 OFFSET 5"
	if query != expected {
		t.Errorf("expected query %q, got %q", expected, query)
	}

	if len(args) != 2 {
		t.Errorf("expected 2 args, got %d", len(args))
	}
	if args[0] != 25 {
		t.Errorf("expected arg[0] to be 25, got %v", args[0])
	}
}

func TestQueryWithQueryBuilder(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	_, err := db.Insert(ctx, "users", map[string]any{"name": "Alice", "email": "alice@example.com", "age": 30})
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}
	_, err = db.Insert(ctx, "users", map[string]any{"name": "Bob", "email": "bob@example.com", "age": 25})
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}

	q := Query("users").Select("name", "age").Where("age >= ?", 28).OrderBy("name ASC")
	query, args := q.Build()

	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(rows) != 1 {
		t.Errorf("expected 1 row, got %d", len(rows))
	}
	if rows[0]["name"] != "Alice" {
		t.Errorf("expected Alice, got %v", rows[0]["name"])
	}
}

func TestTransactionMethods(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	err := db.WithTransaction(ctx, nil, func(tx *Transaction) error {
		result, err := tx.Insert(ctx, "users", map[string]any{"name": "TxMethod", "email": "txm@example.com", "age": 90})
		if err != nil {
			return err
		}

		if result.RowsAffected != 1 {
			t.Errorf("expected 1 row affected, got %d", result.RowsAffected)
		}

		row, err := tx.Get(ctx, "users", []string{"name", "age"}, "name = ?", "TxMethod")
		if err != nil {
			return err
		}

		if row["name"] != "TxMethod" {
			t.Errorf("expected TxMethod, got %v", row["name"])
		}

		count, err := tx.Count(ctx, "users", "", nil)
		if err != nil {
			return err
		}

		if count != 1 {
			t.Errorf("expected count 1, got %d", count)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestDoubleCommit(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("failed to begin tx: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("first commit failed: %v", err)
	}

	if err := tx.Commit(); err == nil {
		t.Fatal("expected error on double commit")
	}
}

func TestDoubleRollback(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("failed to begin tx: %v", err)
	}

	if err := tx.Rollback(); err != nil {
		t.Fatalf("first rollback failed: %v", err)
	}

	if err := tx.Rollback(); err != nil {
		t.Fatalf("second rollback should not error: %v", err)
	}
}
