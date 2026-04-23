package memory

import (
	"fmt"
	"os"
	"path/filepath"
)

func NewMemoryBackend(cfg Config) (MemoryBackend, error) {
	opts := []SQLiteMemoryOption{}
	if cfg.Embedder != nil {
		opts = append(opts, WithEmbedder(cfg.Embedder))
	}
	if cfg.Cache.Enabled {
		ccfg := CacheConfig{MaxSize: cfg.Cache.MaxSize, TTL: cfg.Cache.TTL}
		opts = append(opts, WithCache(ccfg))
	}

	switch cfg.Backend {
	case BackendFile:
		return NewFileMemory(cfg.WorkDir), nil
	case BackendSQLite:
		mem, err := NewSQLiteMemory(cfg.WorkDir, cfg.DSN, opts...)
		if err != nil {
			return nil, err
		}
		return mem, nil
	case BackendDual:
		return NewDualMemory(cfg.WorkDir, cfg.DSN, opts...)
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
