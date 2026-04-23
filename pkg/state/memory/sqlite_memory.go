package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
	_ "modernc.org/sqlite/vec"
)

type SQLiteMemory struct {
	db      *sql.DB
	baseDir string
	mu      sync.RWMutex
	ctx     context.Context

	embedder   EmbeddingProvider
	dimensions int

	cache      *SearchCache
	warmupDone bool
}

type SQLiteMemoryOption func(*SQLiteMemory)

func WithEmbedder(e EmbeddingProvider) SQLiteMemoryOption {
	return func(m *SQLiteMemory) {
		m.embedder = e
		if e != nil && e.Dimension() > 0 {
			m.dimensions = e.Dimension()
		}
	}
}

func WithCache(cfg CacheConfig) SQLiteMemoryOption {
	return func(m *SQLiteMemory) {
		m.cache = NewSearchCache(cfg)
	}
}

func NewSQLiteMemory(workDir string, dsn string, opts ...SQLiteMemoryOption) (*SQLiteMemory, error) {
	if dsn == "" {
		dsn = workDir + "/memory.db"
	}
	m := &SQLiteMemory{baseDir: workDir, ctx: context.Background(), dimensions: 1536}
	for _, opt := range opts {
		opt(m)
	}
	return m, nil
}

func (m *SQLiteMemory) Init() error {
	return m.InitWithDSN("")
}

func (m *SQLiteMemory) InitWithDSN(dsn string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if dsn == "" {
		dsn = m.baseDir + "/memory.db"
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("failed to open SQLite: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)

	pragmas := []string{
		"PRAGMA busy_timeout = 30000",
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA cache_size = -64000",
		"PRAGMA foreign_keys = ON",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return fmt.Errorf("failed to set pragma %q: %w", p, err)
		}
	}

	m.db = db

	if err := m.createTablesLocked(); err != nil {
		db.Close()
		m.db = nil
		return err
	}

	return nil
}

func (m *SQLiteMemory) createTablesLocked() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS memories (
			id TEXT PRIMARY KEY,
			timestamp DATETIME NOT NULL,
			type TEXT NOT NULL,
			role TEXT,
			content TEXT NOT NULL,
			metadata TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		fmt.Sprintf(`CREATE VIRTUAL TABLE IF NOT EXISTS vec_memories USING vec0(
			memory_id TEXT PRIMARY KEY,
			embedding float[%d]
		)`, m.dimensions),
		`CREATE INDEX IF NOT EXISTS idx_memories_type ON memories(type)`,
		`CREATE INDEX IF NOT EXISTS idx_memories_timestamp ON memories(timestamp)`,
	}

	for _, q := range queries {
		if _, err := m.db.ExecContext(m.ctx, q); err != nil {
			return fmt.Errorf("failed to create table: %w", err)
		}
	}

	return nil
}

func (m *SQLiteMemory) Add(entry MemoryEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if entry.ID == "" {
		entry.ID = fmt.Sprintf("%d-%s", time.Now().UnixMilli(), randomID(8))
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	metadataJSON, _ := json.Marshal(entry.Metadata)

	_, err := m.db.ExecContext(m.ctx,
		`INSERT OR REPLACE INTO memories (id, timestamp, type, role, content, metadata) VALUES (?, ?, ?, ?, ?, ?)`,
		entry.ID, entry.Timestamp.Format(time.RFC3339), entry.Type, entry.Role, entry.Content, string(metadataJSON),
	)
	if err != nil {
		return fmt.Errorf("failed to insert memory: %w", err)
	}

	if m.embedder != nil && strings.TrimSpace(entry.Content) != "" {
		go func(id string, content string) {
			embedding, err := m.embedder.Embed(context.Background(), content)
			if err != nil || len(embedding) == 0 {
				return
			}
			m.mu.Lock()
			defer m.mu.Unlock()
			m.storeEmbeddingLocked(id, embedding)
		}(entry.ID, entry.Content)
	}

	return nil
}

func (m *SQLiteMemory) Get(id string) (*MemoryEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	row := m.db.QueryRowContext(m.ctx,
		`SELECT id, timestamp, type, role, content, metadata FROM memories WHERE id = ?`, id,
	)

	var entry MemoryEntry
	var tsStr, metadataJSON string
	err := row.Scan(&entry.ID, &tsStr, &entry.Type, &entry.Role, &entry.Content, &metadataJSON)
	if err != nil {
		return nil, fmt.Errorf("memory not found: %s", id)
	}

	entry.Timestamp, _ = time.Parse(time.RFC3339, tsStr)
	if metadataJSON != "" {
		json.Unmarshal([]byte(metadataJSON), &entry.Metadata)
	}
	return &entry, nil
}

func (m *SQLiteMemory) Search(query string, limit int) ([]MemoryEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cacheKey := fmt.Sprintf("search:%s:%d", query, limit)
	if m.cache != nil {
		if cached, ok := m.cache.Get(cacheKey); ok {
			entries := make([]MemoryEntry, len(cached))
			for i, r := range cached {
				entries[i] = r.Entry
			}
			return entries, nil
		}
	}

	queryLower := strings.ToLower(query)
	rows, err := m.db.QueryContext(m.ctx,
		`SELECT id, timestamp, type, role, content, metadata FROM memories WHERE LOWER(content) LIKE ? ORDER BY timestamp DESC LIMIT ?`,
		"%"+queryLower+"%", limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []MemoryEntry
	for rows.Next() {
		entry, err := scanMemoryRow(rows)
		if err != nil {
			continue
		}
		results = append(results, entry)
	}

	if m.cache != nil && len(results) > 0 {
		searchResults := make([]SearchResult, len(results))
		for i, e := range results {
			searchResults[i] = SearchResult{Entry: e, Score: 1.0, MatchType: "keyword"}
		}
		m.cache.Set(cacheKey, searchResults)
	}

	return results, nil
}

func (m *SQLiteMemory) VectorSearch(queryEmbedding []float64, limit int, threshold float64) ([]VectorEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 {
		limit = 10
	}

	vecJSON, _ := json.Marshal(queryEmbedding)
	rows, err := m.db.QueryContext(m.ctx, `
		SELECT m.id, m.timestamp, m.type, m.role, m.content, m.metadata,
			   1.0 - vec_distance_cosine(v.embedding, vec_f32(?)) AS score
		FROM vec_memories v
		JOIN memories m ON m.id = v.memory_id
		WHERE 1.0 - vec_distance_cosine(v.embedding, vec_f32(?)) >= ?
		ORDER BY score DESC
		LIMIT ?
	`, string(vecJSON), string(vecJSON), threshold, limit)
	if err != nil {
		return nil, fmt.Errorf("vector search failed: %w", err)
	}
	defer rows.Close()

	var results []VectorEntry
	for rows.Next() {
		entry, err := scanVectorRowWithScore(rows)
		if err != nil {
			continue
		}
		results = append(results, entry)
	}

	return results, nil
}

func (m *SQLiteMemory) HybridSearch(query string, queryEmbedding []float64, limit int, vectorWeight float64) ([]HybridSearchResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cacheKey := fmt.Sprintf("hybrid:%s:%d:%.2f", query, limit, vectorWeight)
	if m.cache != nil {
		if cached, ok := m.cache.Get(cacheKey); ok {
			results := make([]HybridSearchResult, len(cached))
			for i, r := range cached {
				results[i] = HybridSearchResult{
					Entry: VectorEntry{
						ID:        r.Entry.ID,
						Timestamp: r.Entry.Timestamp,
						Type:      r.Entry.Type,
						Role:      r.Entry.Role,
						Content:   r.Entry.Content,
						Metadata:  r.Entry.Metadata,
					},
					FTSScore:    r.KeywordScore,
					VectorScore: r.VectorScore,
					FinalScore:  r.Score,
				}
			}
			return results, nil
		}
	}

	ftsResults, _ := m.Search(query, limit*2)

	vecJSON, _ := json.Marshal(queryEmbedding)
	var vectorResults []HybridSearchResult
	rows, err := m.db.QueryContext(m.ctx, `
		SELECT m.id, m.timestamp, m.type, m.role, m.content, m.metadata,
			   1.0 - vec_distance_cosine(v.embedding, vec_f32(?)) AS score
		FROM vec_memories v
		JOIN memories m ON m.id = v.memory_id
		ORDER BY score DESC
		LIMIT ?
	`, string(vecJSON), limit*2)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			entry, err := scanVectorRowWithScore(rows)
			if err != nil {
				continue
			}
			vectorResults = append(vectorResults, HybridSearchResult{
				Entry: VectorEntry{
					ID:        entry.ID,
					Timestamp: entry.Timestamp,
					Type:      entry.Type,
					Role:      entry.Role,
					Content:   entry.Content,
					Metadata:  entry.Metadata,
				},
				VectorScore: entry.Score,
			})
		}
	}

	seen := make(map[string]bool)
	var merged []HybridSearchResult

	for _, entry := range ftsResults {
		seen[entry.ID] = true
		merged = append(merged, HybridSearchResult{
			Entry: VectorEntry{
				ID:        entry.ID,
				Timestamp: entry.Timestamp,
				Type:      entry.Type,
				Role:      entry.Role,
				Content:   entry.Content,
				Metadata:  entry.Metadata,
			},
			FTSScore: 1.0,
		})
	}

	for _, r := range vectorResults {
		if seen[r.Entry.ID] {
			for i := range merged {
				if merged[i].Entry.ID == r.Entry.ID {
					merged[i].VectorScore = r.VectorScore
					break
				}
			}
		} else {
			seen[r.Entry.ID] = true
			merged = append(merged, r)
		}
	}

	textWeight := 1.0 - vectorWeight
	for i := range merged {
		merged[i].FinalScore = merged[i].FTSScore*textWeight + merged[i].VectorScore*vectorWeight
	}

	sort.Slice(merged, func(i, j int) bool {
		return merged[i].FinalScore > merged[j].FinalScore
	})

	if limit > 0 && len(merged) > limit {
		merged = merged[:limit]
	}

	if m.cache != nil && len(merged) > 0 {
		results := make([]SearchResult, len(merged))
		for i, r := range merged {
			results[i] = SearchResult{
				Entry: MemoryEntry{
					ID:        r.Entry.ID,
					Timestamp: r.Entry.Timestamp,
					Type:      r.Entry.Type,
					Role:      r.Entry.Role,
					Content:   r.Entry.Content,
					Metadata:  r.Entry.Metadata,
				},
				Score:        r.FinalScore,
				KeywordScore: r.FTSScore,
				VectorScore:  r.VectorScore,
				MatchType:    "hybrid",
			}
		}
		m.cache.Set(cacheKey, results)
	}

	return merged, nil
}

func (m *SQLiteMemory) StoreEmbedding(memoryID string, embedding []float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.storeEmbeddingLocked(memoryID, embedding)
}

func (m *SQLiteMemory) storeEmbeddingLocked(memoryID string, embedding []float64) error {
	vecJSON, _ := json.Marshal(embedding)
	_, err := m.db.ExecContext(m.ctx,
		`INSERT OR REPLACE INTO vec_memories (memory_id, embedding) VALUES (?, vec_f32(?))`,
		memoryID, string(vecJSON),
	)
	return err
}

func (m *SQLiteMemory) List() ([]MemoryEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rows, err := m.db.QueryContext(m.ctx, `SELECT id, timestamp, type, role, content, metadata FROM memories ORDER BY timestamp DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []MemoryEntry
	for rows.Next() {
		entry, err := scanMemoryRow(rows)
		if err != nil {
			continue
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (m *SQLiteMemory) GetConversationHistory(limit int) ([]MemoryEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var rows *sql.Rows
	var err error

	if limit > 0 {
		rows, err = m.db.QueryContext(m.ctx,
			`SELECT id, timestamp, type, role, content, metadata FROM memories WHERE type = 'conversation' ORDER BY timestamp ASC LIMIT ?`,
			limit)
	} else {
		rows, err = m.db.QueryContext(m.ctx,
			`SELECT id, timestamp, type, role, content, metadata FROM memories WHERE type = 'conversation' ORDER BY timestamp ASC`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []MemoryEntry
	for rows.Next() {
		entry, err := scanMemoryRow(rows)
		if err != nil {
			continue
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (m *SQLiteMemory) AddReflection(content string, metadata map[string]string) error {
	return m.Add(MemoryEntry{Type: TypeReflection, Content: content, Metadata: metadata})
}

func (m *SQLiteMemory) AddFact(content string, metadata map[string]string) error {
	return m.Add(MemoryEntry{Type: TypeFact, Content: content, Metadata: metadata})
}

func (m *SQLiteMemory) FormatAsMarkdown() (string, error) {
	entries, err := m.List()
	if err != nil {
		return "", err
	}

	if len(entries) == 0 {
		return "# Memory\n\n(No entries)", nil
	}

	var sb strings.Builder
	sb.WriteString("# Memory\n\n")

	for _, entry := range entries {
		sb.WriteString(fmt.Sprintf("## [%s] %s - %s\n\n%s\n\n",
			entry.Type, entry.ID, entry.Timestamp.Format("2006-01-02 15:04"), entry.Content))
	}

	return sb.String(), nil
}

func (m *SQLiteMemory) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.db.ExecContext(m.ctx, `DELETE FROM memories WHERE id = ?`, id)
	m.db.ExecContext(m.ctx, `DELETE FROM vec_memories WHERE memory_id = ?`, id)
	return nil
}

func (m *SQLiteMemory) GetStats() (map[string]int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make(map[string]int)

	var total int
	m.db.QueryRowContext(m.ctx, `SELECT COUNT(*) FROM memories`).Scan(&total)
	stats["total"] = total

	var conversations int
	m.db.QueryRowContext(m.ctx, `SELECT COUNT(*) FROM memories WHERE type = 'conversation'`).Scan(&conversations)
	stats["conversations"] = conversations

	var reflections int
	m.db.QueryRowContext(m.ctx, `SELECT COUNT(*) FROM memories WHERE type = 'reflection'`).Scan(&reflections)
	stats["reflections"] = reflections

	var facts int
	m.db.QueryRowContext(m.ctx, `SELECT COUNT(*) FROM memories WHERE type = 'fact'`).Scan(&facts)
	stats["facts"] = facts

	var embeddings int
	m.db.QueryRowContext(m.ctx, `SELECT COUNT(*) FROM vec_memories`).Scan(&embeddings)
	stats["embeddings"] = embeddings

	return stats, nil
}

func (m *SQLiteMemory) Close() error {
	m.mu.Lock()
	db := m.db
	m.db = nil
	m.mu.Unlock()

	if db == nil {
		return nil
	}
	return db.Close()
}

func (m *SQLiteMemory) Warmup(queries []string, concurrency int) WarmupProgress {
	if m.cache == nil || len(queries) == 0 {
		return WarmupProgress{Done: true}
	}
	if concurrency <= 0 {
		concurrency = 4
	}

	start := time.Now()
	total := len(queries)
	var processed int
	var failed int
	var mu sync.Mutex

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for _, query := range queries {
		sem <- struct{}{}
		wg.Add(1)

		go func(q string) {
			defer wg.Done()
			defer func() { <-sem }()

			_, err := m.Search(q, 10)

			mu.Lock()
			processed++
			if err != nil {
				failed++
			}
			mu.Unlock()
		}(query)
	}

	wg.Wait()

	return WarmupProgress{
		Total:     total,
		Processed: processed,
		Failed:    failed,
		Elapsed:   time.Since(start),
		Done:      true,
		Message:   "warmup completed",
	}
}

func (m *SQLiteMemory) CacheStats() CacheStats {
	if m.cache == nil {
		return CacheStats{}
	}
	return m.cache.Stats()
}

func (m *SQLiteMemory) StartAutoBackup(backupDir string, interval time.Duration, maxBackups int) error {
	if backupDir == "" {
		backupDir = filepath.Join(m.baseDir, "backups")
	}
	if interval <= 0 {
		interval = 1 * time.Hour
	}
	if maxBackups <= 0 {
		maxBackups = 10
	}

	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return fmt.Errorf("failed to create backup dir: %w", err)
	}

	go m.backup_loop(backupDir, interval, maxBackups)
	return nil
}

func (m *SQLiteMemory) backup_loop(backupDir string, interval time.Duration, maxBackups int) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		if err := m.performBackup(backupDir, maxBackups); err != nil {
			continue
		}
	}
}

func (m *SQLiteMemory) performBackup(backupDir string, maxBackups int) error {
	m.mu.RLock()
	db := m.db
	m.mu.RUnlock()

	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	if _, err := db.ExecContext(m.ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return fmt.Errorf("checkpoint before backup: %w", err)
	}

	timestamp := time.Now().Format("20060102_150405")
	backupPath := filepath.Join(backupDir, fmt.Sprintf("backup_%s.db", timestamp))

	srcPath := m.baseDir + "/memory.db"
	if srcPath == "" {
		return fmt.Errorf("cannot determine source database path")
	}

	srcFile, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("open source db: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(backupPath)
	if err != nil {
		return fmt.Errorf("create backup file: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("copy to backup: %w", err)
	}

	if err := dstFile.Sync(); err != nil {
		return fmt.Errorf("sync backup: %w", err)
	}

	return m.pruneBackups(backupDir, maxBackups)
}

func (m *SQLiteMemory) pruneBackups(backupDir string, maxBackups int) error {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return err
	}

	var backups []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, "backup_") && strings.HasSuffix(name, ".db") {
			backups = append(backups, name)
		}
	}

	sort.Strings(backups)

	if len(backups) <= maxBackups {
		return nil
	}

	for i := 0; i < len(backups)-maxBackups; i++ {
		os.Remove(filepath.Join(backupDir, backups[i]))
	}

	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanMemoryRow(row rowScanner) (MemoryEntry, error) {
	var entry MemoryEntry
	var tsStr, metadataJSON string
	err := row.Scan(&entry.ID, &tsStr, &entry.Type, &entry.Role, &entry.Content, &metadataJSON)
	if err != nil {
		return entry, err
	}

	entry.Timestamp, _ = time.Parse(time.RFC3339, tsStr)
	if metadataJSON != "" {
		json.Unmarshal([]byte(metadataJSON), &entry.Metadata)
	}
	return entry, nil
}

func scanVectorRowWithScore(row rowScanner) (VectorEntry, error) {
	var entry VectorEntry
	var tsStr, metadataJSON string
	var embeddingBlob []byte
	err := row.Scan(&entry.ID, &tsStr, &entry.Type, &entry.Role, &entry.Content, &metadataJSON, &entry.Score)
	if err != nil {
		return entry, err
	}

	entry.Timestamp, _ = time.Parse(time.RFC3339, tsStr)
	if metadataJSON != "" {
		json.Unmarshal([]byte(metadataJSON), &entry.Metadata)
	}

	if len(embeddingBlob) > 0 {
		json.Unmarshal(embeddingBlob, &entry.Embedding)
	}

	return entry, nil
}
