package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type SQLiteMemory struct {
	db      *sql.DB
	baseDir string
	mu      sync.RWMutex
	ctx     context.Context
}

func NewSQLiteMemory(workDir string, dsn string) (*SQLiteMemory, error) {
	if dsn == "" {
		dsn = workDir + "/memory.db"
	}
	return &SQLiteMemory{baseDir: workDir, ctx: context.Background()}, nil
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
		`CREATE TABLE IF NOT EXISTS embeddings (
			memory_id TEXT PRIMARY KEY,
			embedding BLOB,
			FOREIGN KEY (memory_id) REFERENCES memories(id) ON DELETE CASCADE
		)`,
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

	return results, nil
}

func (m *SQLiteMemory) VectorSearch(queryEmbedding []float64, limit int, threshold float64) ([]VectorEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rows, err := m.db.QueryContext(m.ctx,
		`SELECT m.id, m.timestamp, m.type, m.role, m.content, m.metadata, e.embedding
		 FROM memories m
		 JOIN embeddings e ON m.id = e.memory_id
		 ORDER BY m.timestamp DESC
		 LIMIT 1000`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []VectorEntry
	for rows.Next() {
		entry, err := scanVectorRow(rows)
		if err != nil {
			continue
		}

		score := cosineSimilarity(queryEmbedding, entry.Embedding)
		if score >= threshold {
			entry.Score = score
			results = append(results, entry)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

func (m *SQLiteMemory) HybridSearch(query string, queryEmbedding []float64, limit int, vectorWeight float64) ([]HybridSearchResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ftsResults, _ := m.Search(query, limit*2)
	vectorResults, _ := m.VectorSearch(queryEmbedding, limit*2, 0.3)

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

	for _, entry := range vectorResults {
		if seen[entry.ID] {
			for i := range merged {
				if merged[i].Entry.ID == entry.ID {
					merged[i].VectorScore = entry.Score
					break
				}
			}
		} else {
			seen[entry.ID] = true
			merged = append(merged, HybridSearchResult{
				Entry:       entry,
				VectorScore: entry.Score,
			})
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

	return merged, nil
}

func (m *SQLiteMemory) StoreEmbedding(memoryID string, embedding []float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	embeddingJSON, err := json.Marshal(embedding)
	if err != nil {
		return err
	}

	_, err = m.db.ExecContext(m.ctx,
		`INSERT OR REPLACE INTO embeddings (memory_id, embedding) VALUES (?, ?)`,
		memoryID, string(embeddingJSON),
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
	m.db.ExecContext(m.ctx, `DELETE FROM embeddings WHERE memory_id = ?`, id)
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
	m.db.QueryRowContext(m.ctx, `SELECT COUNT(*) FROM embeddings`).Scan(&embeddings)
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

func scanVectorRow(row rowScanner) (VectorEntry, error) {
	var entry VectorEntry
	var tsStr, metadataJSON string
	var embeddingBlob []byte
	err := row.Scan(&entry.ID, &tsStr, &entry.Type, &entry.Role, &entry.Content, &metadataJSON, &embeddingBlob)
	if err != nil {
		return entry, err
	}

	entry.Timestamp, _ = time.Parse(time.RFC3339, tsStr)
	if metadataJSON != "" {
		json.Unmarshal([]byte(metadataJSON), &entry.Metadata)
	}
	json.Unmarshal(embeddingBlob, &entry.Embedding)

	return entry, nil
}

func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}
