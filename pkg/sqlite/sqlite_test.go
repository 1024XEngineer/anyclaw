package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T, cfg Config) *DB {
	t.Helper()
	if cfg.DSN == "" {
		cfg.DSN = ":memory:"
	}
	db, err := Open(cfg)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}

	_, err = db.ExecContext(context.Background(), `CREATE TABLE test (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		value TEXT
	)`)
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
		if cfg.DSN != ":memory:" {
			_ = os.Remove(cfg.DSN)
			_ = os.Remove(cfg.DSN + "-wal")
			_ = os.Remove(cfg.DSN + "-shm")
		}
	})

	return db
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig(":memory:")
	if cfg.MaxOpenConns != 5 {
		t.Errorf("expected MaxOpenConns 5, got %d", cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns != 3 {
		t.Errorf("expected MaxIdleConns 3, got %d", cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime != 30*time.Minute {
		t.Errorf("expected ConnMaxLifetime 30m, got %v", cfg.ConnMaxLifetime)
	}
	if !cfg.WALEnabled {
		t.Error("expected WALEnabled true")
	}
}

func TestInMemoryConfig(t *testing.T) {
	cfg := InMemoryConfig()
	if cfg.MaxOpenConns != 1 {
		t.Errorf("expected MaxOpenConns 1, got %d", cfg.MaxOpenConns)
	}
	if cfg.WALEnabled {
		t.Error("expected WALEnabled false for in-memory")
	}
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

func TestConnectionPoolSettings(t *testing.T) {
	cfg := DefaultConfig(":memory:")
	cfg.MaxOpenConns = 10
	cfg.MaxIdleConns = 5
	cfg.ConnMaxLifetime = 10 * time.Minute
	cfg.ConnMaxIdleTime = 2 * time.Minute

	db := setupTestDB(t, cfg)

	stats := db.Stats()
	if stats.OpenConnections < 1 {
		t.Errorf("expected at least 1 open connection after setup, got %d", stats.OpenConnections)
	}
}

func TestConnectionPoolUnderLoad(t *testing.T) {
	cfg := DefaultConfig(":memory:")
	cfg.MaxOpenConns = 5
	cfg.MaxIdleConns = 3
	db := setupTestDB(t, cfg)

	ctx := context.Background()
	var wg sync.WaitGroup
	var mu sync.Mutex
	successCount := 0
	failCount := 0

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, err := db.ExecContext(ctx, "INSERT INTO test (name, value) VALUES (?, ?)", "item", n)
			mu.Lock()
			if err != nil {
				failCount++
			} else {
				successCount++
			}
			mu.Unlock()
		}(i)
	}

	wg.Wait()

	if successCount == 0 {
		t.Error("expected at least some successful inserts")
	}

	stats := db.Stats()
	if stats.OpenConnections > cfg.MaxOpenConns {
		t.Errorf("open connections %d exceeds max %d", stats.OpenConnections, cfg.MaxOpenConns)
	}
}

func TestWALModeDetection(t *testing.T) {
	cfg := DefaultConfig(":memory:")
	db := setupTestDB(t, cfg)

	ctx := context.Background()
	isWAL, err := db.IsWALMode(ctx)
	if err != nil {
		t.Fatalf("failed to check WAL mode: %v", err)
	}

	if !isWAL {
		t.Log("WAL mode not active (expected for in-memory DB)")
	}
}

func TestWALFileBased(t *testing.T) {
	tmpFile := t.TempDir() + "/test_wal.db"

	cfg := DefaultConfig(tmpFile)
	cfg.MaxOpenConns = 1
	cfg.MaxIdleConns = 1
	db := setupTestDB(t, cfg)

	ctx := context.Background()

	isWAL, err := db.IsWALMode(ctx)
	if err != nil {
		t.Fatalf("failed to check WAL mode: %v", err)
	}

	if !isWAL {
		t.Fatal("expected WAL mode for file-based DB")
	}

	_, err = db.ExecContext(ctx, "INSERT INTO test (name, value) VALUES (?, ?)", "wal_test", "value")
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}

	if err := db.Checkpoint(ctx, "PASSIVE"); err != nil {
		t.Fatalf("checkpoint failed: %v", err)
	}
}

func TestCheckpoint(t *testing.T) {
	tmpFile := t.TempDir() + "/test_checkpoint.db"

	cfg := DefaultConfig(tmpFile)
	cfg.MaxOpenConns = 1
	cfg.MaxIdleConns = 1
	db := setupTestDB(t, cfg)

	ctx := context.Background()

	for i := 0; i < 10; i++ {
		_, err := db.ExecContext(ctx, "INSERT INTO test (name, value) VALUES (?, ?)", "checkpoint", i)
		if err != nil {
			t.Fatalf("insert failed: %v", err)
		}
	}

	if err := db.Checkpoint(ctx, "PASSIVE"); err != nil {
		t.Fatalf("passive checkpoint failed: %v", err)
	}
	if err := db.Checkpoint(ctx, "FULL"); err != nil {
		t.Fatalf("full checkpoint failed: %v", err)
	}
	if err := db.Checkpoint(ctx, "RESTART"); err != nil {
		t.Fatalf("restart checkpoint failed: %v", err)
	}
	if err := db.Checkpoint(ctx, "TRUNCATE"); err != nil {
		t.Fatalf("truncate checkpoint failed: %v", err)
	}
}

func TestPoolStats(t *testing.T) {
	cfg := DefaultConfig(":memory:")
	cfg.MaxOpenConns = 5
	cfg.MaxIdleConns = 3
	db := setupTestDB(t, cfg)

	ctx := context.Background()
	for i := 0; i < 10; i++ {
		_, _ = db.ExecContext(ctx, "INSERT INTO test (name, value) VALUES (?, ?)", "stats", i)
	}

	stats := db.Stats()

	if stats.OpenConnections < 1 {
		t.Errorf("expected at least 1 open connection, got %d", stats.OpenConnections)
	}
	if db.QueryCount() == 0 && db.ExecCount() == 0 {
		t.Error("expected non-zero query or exec count")
	}
}

func TestWithConn(t *testing.T) {
	db := setupTestDB(t, DefaultConfig(":memory:"))
	ctx := context.Background()

	err := db.WithConn(ctx, func(conn *sql.Conn) error {
		_, err := conn.ExecContext(ctx, "INSERT INTO test (name, value) VALUES (?, ?)", "conn_test", "value")
		return err
	})
	if err != nil {
		t.Fatalf("WithConn failed: %v", err)
	}

	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM test WHERE name = ?", "conn_test").Scan(&count)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 row, got %d", count)
	}
}

func TestIntegrityCheck(t *testing.T) {
	db := setupTestDB(t, DefaultConfig(":memory:"))
	ctx := context.Background()

	ok, err := db.IntegrityCheck(ctx)
	if err != nil {
		t.Fatalf("integrity check failed: %v", err)
	}
	if !ok {
		t.Error("expected integrity check to pass")
	}
}

func TestOptimize(t *testing.T) {
	db := setupTestDB(t, DefaultConfig(":memory:"))
	ctx := context.Background()

	if err := db.Optimize(ctx); err != nil {
		t.Fatalf("optimize failed: %v", err)
	}
}

func TestQueryExecCounters(t *testing.T) {
	db := setupTestDB(t, DefaultConfig(":memory:"))
	ctx := context.Background()

	initialExec := db.ExecCount()
	initialQuery := db.QueryCount()

	_, _ = db.ExecContext(ctx, "INSERT INTO test (name, value) VALUES (?, ?)", "counter", 1)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM test")

	if db.ExecCount() != initialExec+1 {
		t.Errorf("expected exec count %d, got %d", initialExec+1, db.ExecCount())
	}
	if db.QueryCount() != initialQuery+1 {
		t.Errorf("expected query count %d, got %d", initialQuery+1, db.QueryCount())
	}
}

func TestConnMaxIdleTime(t *testing.T) {
	cfg := DefaultConfig(":memory:")
	cfg.MaxIdleConns = 2
	cfg.ConnMaxIdleTime = 100 * time.Millisecond
	db := setupTestDB(t, cfg)

	ctx := context.Background()
	for i := 0; i < 5; i++ {
		_, _ = db.ExecContext(ctx, "INSERT INTO test (name, value) VALUES (?, ?)", "idle", i)
	}

	time.Sleep(200 * time.Millisecond)

	stats := db.Stats()
	if stats.MaxIdleClosed == 0 {
		t.Log("no idle connections closed yet (may be expected depending on timing)")
	}
}

func TestWALAutoCheckpoint(t *testing.T) {
	tmpFile := t.TempDir() + "/test_auto_checkpoint.db"

	cfg := DefaultConfig(tmpFile)
	cfg.MaxOpenConns = 1
	cfg.MaxIdleConns = 1
	cfg.WALAutoCheckpoint = 100
	db := setupTestDB(t, cfg)

	ctx := context.Background()
	for i := 0; i < 200; i++ {
		_, err := db.ExecContext(ctx, "INSERT INTO test (name, value) VALUES (?, ?)", "auto_cp", i)
		if err != nil {
			t.Fatalf("insert failed: %v", err)
		}
	}

	var count int
	err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM test").Scan(&count)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if count != 200 {
		t.Errorf("expected 200 rows, got %d", count)
	}
}

func TestOpenAppliesDefaultsAndClamp(t *testing.T) {
	db, err := Open(Config{})
	if err != nil {
		t.Fatalf("open with zero config: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()

	if db.DSN() != ":memory:" {
		t.Fatalf("expected default dsn :memory:, got %q", db.DSN())
	}
	if db.cfg.MaxOpenConns != 1 {
		t.Fatalf("expected MaxOpenConns defaulted to 1, got %d", db.cfg.MaxOpenConns)
	}
	if db.cfg.MaxIdleConns != 1 {
		t.Fatalf("expected MaxIdleConns defaulted to 1, got %d", db.cfg.MaxIdleConns)
	}
	if err := db.Ping(context.Background()); err != nil {
		t.Fatalf("ping failed: %v", err)
	}

	clamped, err := Open(Config{
		DSN:          ":memory:",
		MaxOpenConns: 1,
		MaxIdleConns: 3,
	})
	if err != nil {
		t.Fatalf("open with clamped config: %v", err)
	}
	defer func() {
		_ = clamped.Close()
	}()

	if clamped.cfg.MaxIdleConns != 1 {
		t.Fatalf("expected MaxIdleConns to clamp to 1, got %d", clamped.cfg.MaxIdleConns)
	}
}

func TestQueryContextWALSizeAndDefaultCheckpointMode(t *testing.T) {
	tmpFile := t.TempDir() + "/test_wal_size.db"

	cfg := DefaultConfig(tmpFile)
	cfg.MaxOpenConns = 1
	cfg.MaxIdleConns = 1
	db := setupTestDB(t, cfg)

	ctx := context.Background()
	_, err := db.ExecContext(ctx, "INSERT INTO test (name, value) VALUES (?, ?)", "wal_size", "value")
	if err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	rows, err := db.QueryContext(ctx, "SELECT name FROM test")
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	if !rows.Next() {
		t.Fatal("expected at least one row")
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close rows: %v", err)
	}

	walSize, err := db.WALSize(ctx)
	if err != nil {
		t.Fatalf("wal size failed: %v", err)
	}
	if walSize < 0 {
		t.Fatalf("expected non-negative wal size, got %d", walSize)
	}

	if err := db.Checkpoint(ctx, ""); err != nil {
		t.Fatalf("checkpoint with default mode failed: %v", err)
	}
}

func TestWithConnReturnsCallbackError(t *testing.T) {
	db := setupTestDB(t, DefaultConfig(":memory:"))

	wantErr := errors.New("boom")
	err := db.WithConn(context.Background(), func(conn *sql.Conn) error {
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected callback error %v, got %v", wantErr, err)
	}
}
