package memory

import (
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfigUsesSQLiteAsPrimaryBackend(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig("workspace")
	if cfg.Backend != BackendSQLite {
		t.Fatalf("expected default backend %q, got %q", BackendSQLite, cfg.Backend)
	}
	if cfg.SQLite.DSN != filepath.Join("workspace", "memory.db") {
		t.Fatalf("expected sqlite dsn %q, got %q", filepath.Join("workspace", "memory.db"), cfg.SQLite.DSN)
	}
	if cfg.SQLite.BusyTimeout != 30*time.Second {
		t.Fatalf("expected busy timeout 30s, got %s", cfg.SQLite.BusyTimeout)
	}
	if !cfg.SQLite.Cache.Enabled {
		t.Fatal("expected sqlite cache to be enabled by default")
	}
}

func TestDefaultSQLiteConfigFallsBackToRelativeDatabasePath(t *testing.T) {
	t.Parallel()

	cfg := DefaultSQLiteConfig("")
	if cfg.DSN != "memory.db" {
		t.Fatalf("expected relative sqlite dsn %q, got %q", "memory.db", cfg.DSN)
	}
}
