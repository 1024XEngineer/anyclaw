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
}

type SQLiteConfig struct {
	DSN         string
	MaxOpen     int
	BusyTimeout time.Duration
	Embedder    EmbeddingProvider
	Cache       SQLiteCacheConfig
}

type SQLiteCacheConfig struct {
	Enabled bool
	MaxSize int
	TTL     time.Duration
}

type BackendType string

const (
	BackendFile   BackendType = "file"
	BackendSQLite BackendType = "sqlite"
	BackendDual   BackendType = "dual"
)

func DefaultConfig(workDir string) Config {
	return Config{
		Backend: BackendSQLite,
		WorkDir: workDir,
		SQLite:  DefaultSQLiteConfig(workDir),
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
