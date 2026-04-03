package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type Config struct {
	DSN               string
	MaxOpenConns      int
	MaxIdleConns      int
	ConnMaxLifetime   time.Duration
	BusyTimeout       time.Duration
	JournalMode       string
	Synchronous       string
	CacheSize         int
	ForeignKeyEnabled bool
	WALEnabled        bool
}

func DefaultConfig(dsn string) Config {
	return Config{
		DSN:               dsn,
		MaxOpenConns:      1,
		MaxIdleConns:      1,
		ConnMaxLifetime:   time.Hour,
		BusyTimeout:       30 * time.Second,
		JournalMode:       "WAL",
		Synchronous:       "NORMAL",
		CacheSize:         -64000,
		ForeignKeyEnabled: true,
		WALEnabled:        true,
	}
}

type DB struct {
	*sql.DB
	mu     sync.RWMutex
	closed bool
}

func Open(cfg Config) (*DB, error) {
	if cfg.DSN == "" {
		cfg.DSN = ":memory:"
	}

	db, err := sql.Open("sqlite", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open database: %w", err)
	}

	wrapper := &DB{DB: db}

	if err := wrapper.configure(cfg); err != nil {
		db.Close()
		return nil, err
	}

	return wrapper, nil
}

func (db *DB) configure(cfg Config) error {
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	pragmas := []string{
		fmt.Sprintf("PRAGMA busy_timeout = %d", cfg.BusyTimeout.Milliseconds()),
		fmt.Sprintf("PRAGMA journal_mode = %s", cfg.JournalMode),
		fmt.Sprintf("PRAGMA synchronous = %s", cfg.Synchronous),
		fmt.Sprintf("PRAGMA cache_size = %d", cfg.CacheSize),
	}

	if cfg.ForeignKeyEnabled {
		pragmas = append(pragmas, "PRAGMA foreign_keys = ON")
	}

	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			return fmt.Errorf("sqlite: exec pragma %q: %w", pragma, err)
		}
	}

	return nil
}

func (db *DB) Close() error {
	db.mu.Lock()
	if db.closed {
		db.mu.Unlock()
		return nil
	}
	db.closed = true
	db.mu.Unlock()
	return db.DB.Close()
}

func (db *DB) Ping(ctx context.Context) error {
	return db.DB.PingContext(ctx)
}
