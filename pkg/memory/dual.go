package memory

import (
	"fmt"
	"sync"
	"time"
)

type DualMemory struct {
	file   *FileMemory
	sqlite *SQLiteMemory
	mu     sync.RWMutex

	syncOnWrite bool
	syncOnRead  bool
}

func NewDualMemory(workDir string, dsn string, opts ...SQLiteMemoryOption) (*DualMemory, error) {
	fileMem := NewFileMemory(workDir)

	sqliteMem, err := NewSQLiteMemory(workDir, dsn, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create SQLite memory: %w", err)
	}

	return &DualMemory{
		file:        fileMem,
		sqlite:      sqliteMem,
		syncOnWrite: true,
		syncOnRead:  false,
	}, nil
}

func (d *DualMemory) Init() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if err := d.file.Init(); err != nil {
		return fmt.Errorf("failed to init file backend: %w", err)
	}

	if err := d.sqlite.Init(); err != nil {
		return fmt.Errorf("failed to init sqlite backend: %w", err)
	}

	return nil
}

func (d *DualMemory) Add(entry MemoryEntry) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	sqliteErr := d.sqlite.Add(entry)
	if sqliteErr != nil {
		return fmt.Errorf("sqlite add failed: %w", sqliteErr)
	}

	if d.syncOnWrite {
		if err := d.file.Add(entry); err != nil {
			return fmt.Errorf("file add failed: %w", err)
		}
	}

	return nil
}

func (d *DualMemory) Get(id string) (*MemoryEntry, error) {
	if d.syncOnRead {
		d.mu.RLock()
		defer d.mu.RUnlock()

		entry, err := d.sqlite.Get(id)
		if err == nil {
			return entry, nil
		}

		return d.file.Get(id)
	}

	return d.sqlite.Get(id)
}

func (d *DualMemory) Delete(id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	sqliteErr := d.sqlite.Delete(id)
	if sqliteErr != nil && d.syncOnWrite {
		d.file.Delete(id)
	}

	return sqliteErr
}

func (d *DualMemory) List() ([]MemoryEntry, error) {
	if d.syncOnRead {
		d.mu.RLock()
		defer d.mu.RUnlock()

		entries, err := d.sqlite.List()
		if err == nil {
			return entries, nil
		}

		return d.file.List()
	}

	return d.sqlite.List()
}

func (d *DualMemory) Search(query string, limit int) ([]MemoryEntry, error) {
	if d.syncOnRead {
		d.mu.RLock()
		defer d.mu.RUnlock()

		entries, err := d.sqlite.Search(query, limit)
		if err == nil && len(entries) > 0 {
			return entries, nil
		}

		return d.file.Search(query, limit)
	}

	return d.sqlite.Search(query, limit)
}

func (d *DualMemory) GetConversationHistory(limit int) ([]MemoryEntry, error) {
	if d.syncOnRead {
		d.mu.RLock()
		defer d.mu.RUnlock()

		entries, err := d.sqlite.GetConversationHistory(limit)
		if err == nil {
			return entries, nil
		}

		return d.file.GetConversationHistory(limit)
	}

	return d.sqlite.GetConversationHistory(limit)
}

func (d *DualMemory) AddReflection(content string, metadata map[string]string) error {
	return d.Add(MemoryEntry{Type: TypeReflection, Content: content, Metadata: metadata})
}

func (d *DualMemory) AddFact(content string, metadata map[string]string) error {
	return d.Add(MemoryEntry{Type: TypeFact, Content: content, Metadata: metadata})
}

func (d *DualMemory) FormatAsMarkdown() (string, error) {
	if d.syncOnRead {
		d.mu.RLock()
		defer d.mu.RUnlock()

		md, err := d.sqlite.FormatAsMarkdown()
		if err == nil {
			return md, nil
		}

		return d.file.FormatAsMarkdown()
	}

	return d.sqlite.FormatAsMarkdown()
}

func (d *DualMemory) GetStats() (map[string]int, error) {
	stats, err := d.sqlite.GetStats()
	if err != nil {
		return d.file.GetStats()
	}
	return stats, nil
}

func (d *DualMemory) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	var sqliteErr error
	if d.sqlite != nil {
		sqliteErr = d.sqlite.Close()
	}

	return sqliteErr
}

func (d *DualMemory) VectorSearch(queryEmbedding []float64, limit int, threshold float64) ([]VectorEntry, error) {
	return d.sqlite.VectorSearch(queryEmbedding, limit, threshold)
}

func (d *DualMemory) HybridSearch(query string, queryEmbedding []float64, limit int, vectorWeight float64) ([]HybridSearchResult, error) {
	return d.sqlite.HybridSearch(query, queryEmbedding, limit, vectorWeight)
}

func (d *DualMemory) StoreEmbedding(memoryID string, embedding []float64) error {
	return d.sqlite.StoreEmbedding(memoryID, embedding)
}

func (d *DualMemory) SearchDaily(query string, limit int, dayRef string) ([]DailyMemoryMatch, error) {
	return d.file.SearchDaily(query, limit, dayRef)
}

func (d *DualMemory) GetDaily(dayRef string) (*DailyMemoryFile, error) {
	return d.file.GetDaily(dayRef)
}

func (d *DualMemory) SetDailyDir(dir string) {
	d.file.SetDailyDir(dir)
}

func (d *DualMemory) DailyDir() string {
	return d.file.DailyDir()
}

func (d *DualMemory) SetSyncOnWrite(enabled bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.syncOnWrite = enabled
}

func (d *DualMemory) SetSyncOnRead(enabled bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.syncOnRead = enabled
}

func (d *DualMemory) SyncFileToSQLite() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	entries, err := d.file.List()
	if err != nil {
		return fmt.Errorf("failed to list file entries: %w", err)
	}

	for _, entry := range entries {
		if err := d.sqlite.Add(entry); err != nil {
			return fmt.Errorf("failed to sync entry %s: %w", entry.ID, err)
		}
	}

	return nil
}

func (d *DualMemory) SyncSQLiteToFile() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	entries, err := d.sqlite.List()
	if err != nil {
		return fmt.Errorf("failed to list sqlite entries: %w", err)
	}

	for _, entry := range entries {
		if err := d.file.Add(entry); err != nil {
			return fmt.Errorf("failed to sync entry %s: %w", entry.ID, err)
		}
	}

	return nil
}

func (d *DualMemory) Warmup(queries []string, concurrency int) WarmupProgress {
	return d.sqlite.Warmup(queries, concurrency)
}

func (d *DualMemory) CacheStats() CacheStats {
	return d.sqlite.CacheStats()
}

func (d *DualMemory) StartAutoBackup(backupDir string, interval time.Duration, maxBackups int) error {
	return d.sqlite.StartAutoBackup(backupDir, interval, maxBackups)
}
