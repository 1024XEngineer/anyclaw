package memory

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

// SQLiteMemory provides SQLite-based memory storage with vector search capability
type SQLiteMemory struct {
	db      *sql.DB
	baseDir string
	mu      sync.RWMutex
}

// VectorEntry represents a memory entry with optional vector embedding
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

// HybridSearchResult contains both FTS and vector search results
type HybridSearchResult struct {
	Entry       VectorEntry `json:"entry"`
	FTSScore    float64     `json:"fts_score"`
	VectorScore float64     `json:"vector_score"`
	FinalScore  float64     `json:"final_score"`
}

// NewSQLiteMemory creates a new SQLite-backed memory store
func NewSQLiteMemory(workDir string) (*SQLiteMemory, error) {
	return &SQLiteMemory{baseDir: workDir}, nil
}

// Init initializes the SQLite database
func (m *SQLiteMemory) Init() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// In-memory SQLite for now (can be extended to file-based)
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		return fmt.Errorf("failed to open SQLite: %w", err)
	}
	m.db = db

	// Create tables
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
		`CREATE TABLE IF NOT EXISTS memory_index (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			entries TEXT,
			updated DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_memories_type ON memories(type)`,
		`CREATE INDEX IF NOT EXISTS idx_memories_timestamp ON memories(timestamp)`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5(id, content, type, role)`,
	}

	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			// FTS5 might not be available, skip FTS table
			if strings.Contains(err.Error(), "fts5") || strings.Contains(err.Error(), "no such module") {
				continue
			}
			return fmt.Errorf("failed to create table: %w", err)
		}
	}

	return nil
}

// Add stores a memory entry
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

	_, err := m.db.Exec(
		`INSERT OR REPLACE INTO memories (id, timestamp, type, role, content, metadata) VALUES (?, ?, ?, ?, ?, ?)`,
		entry.ID, entry.Timestamp, entry.Type, entry.Role, entry.Content, string(metadataJSON),
	)
	if err != nil {
		return fmt.Errorf("failed to insert memory: %w", err)
	}

	// Update FTS index
	m.db.Exec(`INSERT OR REPLACE INTO memories_fts (id, content, type, role) VALUES (?, ?, ?, ?)`,
		entry.ID, entry.Content, entry.Type, entry.Role)

	return nil
}

// Get retrieves a memory entry by ID
func (m *SQLiteMemory) Get(id string) (*MemoryEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var entry MemoryEntry
	var metadataJSON string
	err := m.db.QueryRow(
		`SELECT id, timestamp, type, role, content, metadata FROM memories WHERE id = ?`, id,
	).Scan(&entry.ID, &entry.Timestamp, &entry.Type, &entry.Role, &entry.Content, &metadataJSON)
	if err != nil {
		return nil, fmt.Errorf("memory not found: %s", id)
	}

	if metadataJSON != "" {
		json.Unmarshal([]byte(metadataJSON), &entry.Metadata)
	}
	return &entry, nil
}

// Search performs hybrid search (FTS + vector similarity)
func (m *SQLiteMemory) Search(query string, limit int) ([]MemoryEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Try FTS search first
	var results []MemoryEntry
	rows, err := m.db.Query(
		`SELECT m.id, m.timestamp, m.type, m.role, m.content, m.metadata
		 FROM memories_fts fts
		 JOIN memories m ON fts.id = m.id
		 WHERE memories_fts MATCH ?
		 ORDER BY rank
		 LIMIT ?`,
		query, limit,
	)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var entry MemoryEntry
			var metadataJSON string
			if err := rows.Scan(&entry.ID, &entry.Timestamp, &entry.Type, &entry.Role, &entry.Content, &metadataJSON); err != nil {
				continue
			}
			if metadataJSON != "" {
				json.Unmarshal([]byte(metadataJSON), &entry.Metadata)
			}
			results = append(results, entry)
		}
	}

	// Fallback to substring search if FTS fails or returns no results
	if len(results) == 0 {
		queryLower := strings.ToLower(query)
		allRows, err := m.db.Query(
			`SELECT id, timestamp, type, role, content, metadata FROM memories WHERE LOWER(content) LIKE ? ORDER BY timestamp DESC LIMIT ?`,
			"%"+queryLower+"%", limit,
		)
		if err != nil {
			return nil, err
		}
		defer allRows.Close()
		for allRows.Next() {
			var entry MemoryEntry
			var metadataJSON string
			if err := allRows.Scan(&entry.ID, &entry.Timestamp, &entry.Type, &entry.Role, &entry.Content, &metadataJSON); err != nil {
				continue
			}
			if metadataJSON != "" {
				json.Unmarshal([]byte(metadataJSON), &entry.Metadata)
			}
			results = append(results, entry)
		}
	}

	return results, nil
}

// VectorSearch performs vector similarity search
func (m *SQLiteMemory) VectorSearch(queryEmbedding []float64, limit int, threshold float64) ([]VectorEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rows, err := m.db.Query(
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
		var entry VectorEntry
		var metadataJSON string
		var embeddingBlob []byte
		if err := rows.Scan(&entry.ID, &entry.Timestamp, &entry.Type, &entry.Role, &entry.Content, &metadataJSON, &embeddingBlob); err != nil {
			continue
		}
		if metadataJSON != "" {
			json.Unmarshal([]byte(metadataJSON), &entry.Metadata)
		}

		// Deserialize embedding
		var embedding []float64
		if err := json.Unmarshal(embeddingBlob, &embedding); err != nil {
			continue
		}
		entry.Embedding = embedding

		// Calculate cosine similarity
		score := cosineSimilarity(queryEmbedding, embedding)
		if score >= threshold {
			entry.Score = score
			results = append(results, entry)
		}
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// HybridSearch combines FTS and vector search with MMR re-ranking
func (m *SQLiteMemory) HybridSearch(query string, queryEmbedding []float64, limit int, vectorWeight float64) ([]HybridSearchResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Get FTS results
	ftsResults, _ := m.Search(query, limit*2)

	// Get vector results
	vectorResults, _ := m.VectorSearch(queryEmbedding, limit*2, 0.3)

	// Merge results
	seen := make(map[string]bool)
	var merged []HybridSearchResult

	// Add FTS results
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

	// Add vector results
	for _, entry := range vectorResults {
		if seen[entry.ID] {
			// Update existing entry with vector score
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

	// Calculate final scores
	textWeight := 1.0 - vectorWeight
	for i := range merged {
		merged[i].FinalScore = merged[i].FTSScore*textWeight + merged[i].VectorScore*vectorWeight
	}

	// Sort by final score
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].FinalScore > merged[j].FinalScore
	})

	if limit > 0 && len(merged) > limit {
		merged = merged[:limit]
	}

	return merged, nil
}

// StoreEmbedding stores a vector embedding for a memory entry
func (m *SQLiteMemory) StoreEmbedding(memoryID string, embedding []float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	embeddingJSON, err := json.Marshal(embedding)
	if err != nil {
		return err
	}

	_, err = m.db.Exec(
		`INSERT OR REPLACE INTO embeddings (memory_id, embedding) VALUES (?, ?)`,
		memoryID, string(embeddingJSON),
	)
	return err
}

// List returns all memory entries
func (m *SQLiteMemory) List() ([]MemoryEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rows, err := m.db.Query(`SELECT id, timestamp, type, role, content, metadata FROM memories ORDER BY timestamp DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []MemoryEntry
	for rows.Next() {
		var entry MemoryEntry
		var metadataJSON string
		if err := rows.Scan(&entry.ID, &entry.Timestamp, &entry.Type, &entry.Role, &entry.Content, &metadataJSON); err != nil {
			continue
		}
		if metadataJSON != "" {
			json.Unmarshal([]byte(metadataJSON), &entry.Metadata)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// GetConversationHistory returns conversation history
func (m *SQLiteMemory) GetConversationHistory(limit int) ([]MemoryEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	query := `SELECT id, timestamp, type, role, content, metadata FROM memories WHERE type = 'conversation' ORDER BY timestamp ASC`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := m.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []MemoryEntry
	for rows.Next() {
		var entry MemoryEntry
		var metadataJSON string
		if err := rows.Scan(&entry.ID, &entry.Timestamp, &entry.Type, &entry.Role, &entry.Content, &metadataJSON); err != nil {
			continue
		}
		if metadataJSON != "" {
			json.Unmarshal([]byte(metadataJSON), &entry.Metadata)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// AddReflection adds a reflection entry
func (m *SQLiteMemory) AddReflection(content string, metadata map[string]string) error {
	return m.Add(MemoryEntry{Type: TypeReflection, Content: content, Metadata: metadata})
}

// AddFact adds a fact entry
func (m *SQLiteMemory) AddFact(content string, metadata map[string]string) error {
	return m.Add(MemoryEntry{Type: TypeFact, Content: content, Metadata: metadata})
}

// FormatAsMarkdown formats all memories as markdown
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

// Delete removes a memory entry
func (m *SQLiteMemory) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec(`DELETE FROM memories WHERE id = ?`, id)
	if err != nil {
		return err
	}
	m.db.Exec(`DELETE FROM embeddings WHERE memory_id = ?`, id)
	m.db.Exec(`DELETE FROM memories_fts WHERE id = ?`, id)
	return nil
}

// GetStats returns memory statistics
func (m *SQLiteMemory) GetStats() (map[string]int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make(map[string]int)

	var total int
	m.db.QueryRow(`SELECT COUNT(*) FROM memories`).Scan(&total)
	stats["total"] = total

	var conversations int
	m.db.QueryRow(`SELECT COUNT(*) FROM memories WHERE type = 'conversation'`).Scan(&conversations)
	stats["conversations"] = conversations

	var reflections int
	m.db.QueryRow(`SELECT COUNT(*) FROM memories WHERE type = 'reflection'`).Scan(&reflections)
	stats["reflections"] = reflections

	var facts int
	m.db.QueryRow(`SELECT COUNT(*) FROM memories WHERE type = 'fact'`).Scan(&facts)
	stats["facts"] = facts

	var embeddings int
	m.db.QueryRow(`SELECT COUNT(*) FROM embeddings`).Scan(&embeddings)
	stats["embeddings"] = embeddings

	return stats, nil
}

// cosineSimilarity calculates cosine similarity between two vectors
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
