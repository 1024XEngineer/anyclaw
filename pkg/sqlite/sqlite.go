package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type Config struct {
	DSN               string
	MaxOpenConns      int
	MaxIdleConns      int
	ConnMaxLifetime   time.Duration
	ConnMaxIdleTime   time.Duration
	BusyTimeout       time.Duration
	JournalMode       string
	Synchronous       string
	CacheSize         int
	ForeignKeyEnabled bool
	WALEnabled        bool
	WALAutoCheckpoint int
	MmapSize          int64
	TempStore         string
}

func DefaultConfig(dsn string) Config {
	return Config{
		DSN:               dsn,
		MaxOpenConns:      5,
		MaxIdleConns:      3,
		ConnMaxLifetime:   30 * time.Minute,
		ConnMaxIdleTime:   5 * time.Minute,
		BusyTimeout:       30 * time.Second,
		JournalMode:       "WAL",
		Synchronous:       "NORMAL",
		CacheSize:         -64000,
		ForeignKeyEnabled: true,
		WALEnabled:        true,
		WALAutoCheckpoint: 1000,
		MmapSize:          256 * 1024 * 1024,
		TempStore:         "MEMORY",
	}
}

func InMemoryConfig() Config {
	cfg := DefaultConfig(":memory:")
	cfg.MaxOpenConns = 1
	cfg.MaxIdleConns = 1
	cfg.WALEnabled = false
	cfg.JournalMode = "MEMORY"
	return cfg
}

type PoolStats struct {
	OpenConnections   int
	InUse             int64
	Idle              int64
	WaitCount         int64
	WaitDuration      time.Duration
	MaxIdleClosed     int64
	MaxLifetimeClosed int64
}

type DB struct {
	*sql.DB
	mu         sync.RWMutex
	closed     bool
	cfg        Config
	queryCount atomic.Int64
	execCount  atomic.Int64
}

func (db *DB) DSN() string {
	return db.cfg.DSN
}

func Open(cfg Config) (*DB, error) {
	if cfg.DSN == "" {
		cfg.DSN = ":memory:"
	}
	if cfg.MaxOpenConns <= 0 {
		cfg.MaxOpenConns = 1
	}
	if cfg.MaxIdleConns <= 0 {
		cfg.MaxIdleConns = 1
	}
	if cfg.MaxIdleConns > cfg.MaxOpenConns {
		cfg.MaxIdleConns = cfg.MaxOpenConns
	}

	db, err := sql.Open("sqlite", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open database: %w", err)
	}

	wrapper := &DB{DB: db, cfg: cfg}

	if err := wrapper.configure(); err != nil {
		db.Close()
		return nil, err
	}

	return wrapper, nil
}

func (db *DB) configure() error {
	db.SetMaxOpenConns(db.cfg.MaxOpenConns)
	db.SetMaxIdleConns(db.cfg.MaxIdleConns)
	db.SetConnMaxLifetime(db.cfg.ConnMaxLifetime)
	db.SetConnMaxIdleTime(db.cfg.ConnMaxIdleTime)

	pragmas := db.buildPragmas()
	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			return fmt.Errorf("sqlite: exec pragma %q: %w", pragma, err)
		}
	}

	return nil
}

func (db *DB) buildPragmas() []string {
	var pragmas []string

	pragmas = append(pragmas,
		fmt.Sprintf("PRAGMA busy_timeout = %d", db.cfg.BusyTimeout.Milliseconds()),
	)

	if db.cfg.JournalMode != "" {
		pragmas = append(pragmas, fmt.Sprintf("PRAGMA journal_mode = %s", db.cfg.JournalMode))
	}

	if db.cfg.Synchronous != "" {
		pragmas = append(pragmas, fmt.Sprintf("PRAGMA synchronous = %s", db.cfg.Synchronous))
	}

	if db.cfg.CacheSize != 0 {
		pragmas = append(pragmas, fmt.Sprintf("PRAGMA cache_size = %d", db.cfg.CacheSize))
	}

	if db.cfg.MmapSize > 0 {
		pragmas = append(pragmas, fmt.Sprintf("PRAGMA mmap_size = %d", db.cfg.MmapSize))
	}

	if db.cfg.TempStore != "" {
		pragmas = append(pragmas, fmt.Sprintf("PRAGMA temp_store = %s", db.cfg.TempStore))
	}

	if db.cfg.WALEnabled {
		pragmas = append(pragmas, "PRAGMA journal_mode = WAL")
		if db.cfg.WALAutoCheckpoint > 0 {
			pragmas = append(pragmas, fmt.Sprintf("PRAGMA wal_autocheckpoint = %d", db.cfg.WALAutoCheckpoint))
		}
		pragmas = append(pragmas, "PRAGMA journal_size_limit = 67108864")
	}

	if db.cfg.ForeignKeyEnabled {
		pragmas = append(pragmas, "PRAGMA foreign_keys = ON")
	}

	pragmas = append(pragmas,
		"PRAGMA optimize",
	)

	return pragmas
}

func (db *DB) Close() error {
	db.mu.Lock()
	if db.closed {
		db.mu.Unlock()
		return nil
	}
	db.closed = true
	db.mu.Unlock()

	if db.cfg.WALEnabled && db.cfg.DSN != ":memory:" {
		db.checkpointWAL(context.Background())
	}

	return db.DB.Close()
}

func (db *DB) Ping(ctx context.Context) error {
	return db.DB.PingContext(ctx)
}

func (db *DB) Checkpoint(ctx context.Context, mode string) error {
	if mode == "" {
		mode = "PASSIVE"
	}
	_, err := db.ExecContext(ctx, fmt.Sprintf("PRAGMA wal_checkpoint(%s)", mode))
	return err
}

func (db *DB) checkpointWAL(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)")
}

func (db *DB) WALSize(ctx context.Context) (int64, error) {
	var size int64
	err := db.QueryRowContext(ctx, "PRAGMA wal_checkpoint").Scan(&size)
	return size, err
}

func (db *DB) Stats() PoolStats {
	s := db.DB.Stats()
	return PoolStats{
		OpenConnections:   s.OpenConnections,
		InUse:             int64(s.InUse),
		Idle:              int64(s.Idle),
		WaitCount:         s.WaitCount,
		WaitDuration:      s.WaitDuration,
		MaxIdleClosed:     s.MaxIdleClosed,
		MaxLifetimeClosed: s.MaxLifetimeClosed,
	}
}

func (db *DB) QueryCount() int64 {
	return db.queryCount.Load()
}

func (db *DB) ExecCount() int64 {
	return db.execCount.Load()
}

func (db *DB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	db.execCount.Add(1)
	return db.DB.ExecContext(ctx, query, args...)
}

func (db *DB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	db.queryCount.Add(1)
	return db.DB.QueryContext(ctx, query, args...)
}

func (db *DB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	db.queryCount.Add(1)
	return db.DB.QueryRowContext(ctx, query, args...)
}

func (db *DB) WithConn(ctx context.Context, fn func(conn *sql.Conn) error) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("sqlite: get connection: %w", err)
	}
	defer conn.Close()

	if err := fn(conn); err != nil {
		return err
	}
	return nil
}

func (db *DB) IsWALMode(ctx context.Context) (bool, error) {
	var mode string
	err := db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode)
	if err != nil {
		return false, err
	}
	return mode == "wal", nil
}

func (db *DB) Optimize(ctx context.Context) error {
	_, err := db.ExecContext(ctx, "PRAGMA optimize")
	return err
}

func (db *DB) IntegrityCheck(ctx context.Context) (bool, error) {
	var result string
	err := db.QueryRowContext(ctx, "PRAGMA integrity_check(1)").Scan(&result)
	if err != nil {
		return false, err
	}
	return result == "ok", nil
}
