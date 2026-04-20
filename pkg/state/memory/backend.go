package memory

import (
	"path/filepath"
	"time"
)

type VectorEntry struct {
	ID        string            `json:"id"`
	Timestamp time.Time         `json:"timestamp"`
	Type      string            `json:"type"`
	Role      string            `json:"role,omitempty"`
	Content   string            `json:"content"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Embedding []float64         `json:"embedding,omitempty"`
	Score     float64           `json:"score,omitempty"`
}

type HybridSearchResult struct {
	Entry       VectorEntry `json:"entry"`
	FTSScore    float64     `json:"fts_score"`
	VectorScore float64     `json:"vector_score"`
	FinalScore  float64     `json:"final_score"`
}

type MemoryBackend interface {
	Init() error
	Add(entry MemoryEntry) error
	Get(id string) (*MemoryEntry, error)
	Delete(id string) error
	List() ([]MemoryEntry, error)
	Search(query string, limit int) ([]MemoryEntry, error)
	GetConversationHistory(limit int) ([]MemoryEntry, error)
	AddReflection(content string, metadata map[string]string) error
	AddFact(content string, metadata map[string]string) error
	FormatAsMarkdown() (string, error)
	GetStats() (map[string]int, error)
	Close() error
}

type VectorBackend interface {
	VectorSearch(queryEmbedding []float64, limit int, threshold float64) ([]VectorEntry, error)
	HybridSearch(query string, queryEmbedding []float64, limit int, vectorWeight float64) ([]HybridSearchResult, error)
	StoreEmbedding(memoryID string, embedding []float64) error
}

type DailyBackend interface {
	SearchDaily(query string, limit int, dayRef string) ([]DailyMemoryMatch, error)
	GetDaily(dayRef string) (*DailyMemoryFile, error)
	SetDailyDir(dir string)
	DailyDir() string
}

type WarmupBackend interface {
	Warmup(queries []string, concurrency int) WarmupProgress
}

type CacheStatsBackend interface {
	CacheStats() CacheStats
}

type AutoBackupBackend interface {
	StartAutoBackup(backupDir string, interval time.Duration, maxBackups int) error
}

type Config struct {
	Backend BackendType
	WorkDir string
	SQLite  SQLiteConfig

	DSN         string
	MaxOpen     int
	BusyTimeout time.Duration
	Embedder    EmbeddingProvider
	Cache       BackendCacheConfig
	Warmup      BackendWarmupConfig
}

type BackendCacheConfig struct {
	Enabled bool
	MaxSize int
	TTL     time.Duration
}

type SQLiteCacheConfig = BackendCacheConfig

type SQLiteConfig struct {
	DSN         string
	MaxOpen     int
	BusyTimeout time.Duration
	Embedder    EmbeddingProvider
	Cache       SQLiteCacheConfig
}

type BackendWarmupConfig struct {
	Enabled bool
	Queries []string
}

type BackendType string

const (
	BackendFile   BackendType = "file"
	BackendSQLite BackendType = "sqlite"
	BackendDual   BackendType = "dual"
)

func DefaultConfig(workDir string) Config {
	sqliteCfg := DefaultSQLiteConfig(workDir)
	return Config{
		Backend:     BackendSQLite,
		WorkDir:     workDir,
		SQLite:      sqliteCfg,
		DSN:         sqliteCfg.DSN,
		MaxOpen:     sqliteCfg.MaxOpen,
		BusyTimeout: sqliteCfg.BusyTimeout,
		Cache:       sqliteCfg.Cache,
		Warmup: BackendWarmupConfig{
			Enabled: true,
			Queries: []string{
				"task",
				"project",
				"config",
				"error",
				"setup",
			},
		},
	}
}

func DefaultSQLiteConfig(workDir string) SQLiteConfig {
	return SQLiteConfig{
		DSN:         filepath.Join(workDir, "memory.db"),
		MaxOpen:     1,
		BusyTimeout: 30 * time.Second,
		Cache: SQLiteCacheConfig{
			Enabled: true,
			MaxSize: 5000,
			TTL:     5 * time.Minute,
		},
	}
}
