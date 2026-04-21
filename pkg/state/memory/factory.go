package memory

import (
	"fmt"
	"os"
	"path/filepath"
)

func NewMemoryBackend(cfg Config) (MemoryBackend, error) {
	sqliteCfg := effectiveSQLiteConfig(cfg)

	switch cfg.Backend {
	case BackendFile:
		return NewFileMemory(cfg.WorkDir), nil
	case BackendSQLite:
		mem, err := NewSQLiteMemory(cfg.WorkDir, sqliteCfg.DSN, sqliteOptionsFromConfig(sqliteCfg)...)
		if err != nil {
			return nil, err
		}
		return mem, nil
	case BackendDual:
		return nil, fmt.Errorf("backend %q is not supported by NewMemoryBackend; use NewDualMemory explicitly", cfg.Backend)
	default:
		return nil, fmt.Errorf("unknown backend type: %s", cfg.Backend)
	}
}

func NewMemoryBackendWithDefaults(workDir string) (MemoryBackend, error) {
	cfg := DefaultConfig(workDir)
	return NewMemoryBackend(cfg)
}

func MigrateFileToSQLite(workDir string, dsn string) error {
	fileMem := NewFileMemory(workDir)
	if err := fileMem.Init(); err != nil {
		return fmt.Errorf("failed to init file backend: %w", err)
	}

	sqliteMem, err := NewSQLiteMemory(workDir, dsn)
	if err != nil {
		return fmt.Errorf("failed to create SQLite memory: %w", err)
	}
	if err := sqliteMem.Init(); err != nil {
		return fmt.Errorf("failed to init SQLite backend: %w", err)
	}
	defer sqliteMem.Close()

	entries, err := fileMem.List()
	if err != nil {
		return fmt.Errorf("failed to list file entries: %w", err)
	}

	for _, entry := range entries {
		if err := sqliteMem.Add(entry); err != nil {
			return fmt.Errorf("failed to migrate entry %s: %w", entry.ID, err)
		}
	}

	return nil
}

func BackupSQLiteToFile(dsn string, workDir string) error {
	sqliteMem, err := NewSQLiteMemory(workDir, dsn)
	if err != nil {
		return fmt.Errorf("failed to create SQLite memory: %w", err)
	}
	if err := sqliteMem.Init(); err != nil {
		return fmt.Errorf("failed to init SQLite backend: %w", err)
	}
	defer sqliteMem.Close()

	fileMem := NewFileMemory(workDir)
	if err := fileMem.Init(); err != nil {
		return fmt.Errorf("failed to init file backend: %w", err)
	}

	entries, err := sqliteMem.List()
	if err != nil {
		return fmt.Errorf("failed to list SQLite entries: %w", err)
	}

	for _, entry := range entries {
		if err := fileMem.Add(entry); err != nil {
			return fmt.Errorf("failed to backup entry %s: %w", entry.ID, err)
		}
	}

	return nil
}

func EnsureMemoryDir(workDir string) error {
	memoryDir := filepath.Join(workDir, "memory")
	return os.MkdirAll(memoryDir, 0o755)
}

func effectiveSQLiteConfig(cfg Config) SQLiteConfig {
	resolved := DefaultSQLiteConfig(cfg.WorkDir)
	if cfg.SQLite.DSN != "" {
		resolved.DSN = cfg.SQLite.DSN
	}
	if cfg.SQLite.MaxOpen > 0 {
		resolved.MaxOpen = cfg.SQLite.MaxOpen
	}
	if cfg.SQLite.BusyTimeout > 0 {
		resolved.BusyTimeout = cfg.SQLite.BusyTimeout
	}
	if cfg.SQLite.Embedder != nil {
		resolved.Embedder = cfg.SQLite.Embedder
	}
	if cfg.SQLite.Cache.Enabled || cfg.SQLite.Cache.MaxSize > 0 || cfg.SQLite.Cache.TTL > 0 {
		resolved.Cache = cfg.SQLite.Cache
	}

	if cfg.DSN != "" {
		resolved.DSN = cfg.DSN
	}
	if cfg.MaxOpen > 0 {
		resolved.MaxOpen = cfg.MaxOpen
	}
	if cfg.BusyTimeout > 0 {
		resolved.BusyTimeout = cfg.BusyTimeout
	}
	if cfg.Embedder != nil {
		resolved.Embedder = cfg.Embedder
	}
	if cfg.Cache.Enabled || cfg.Cache.MaxSize > 0 || cfg.Cache.TTL > 0 {
		resolved.Cache = cfg.Cache
	}

	return resolved
}

func sqliteOptionsFromConfig(cfg SQLiteConfig) []SQLiteMemoryOption {
	opts := []SQLiteMemoryOption{
		WithMaxOpenConns(cfg.MaxOpen),
		WithBusyTimeout(cfg.BusyTimeout),
	}
	if cfg.Embedder != nil {
		opts = append(opts, WithEmbedder(cfg.Embedder))
	}
	if cfg.Cache.Enabled {
		opts = append(opts, WithCache(CacheConfig{
			MaxSize: cfg.Cache.MaxSize,
			TTL:     cfg.Cache.TTL,
		}))
	}
	return opts
}
