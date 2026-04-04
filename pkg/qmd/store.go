package qmd

import (
	"sync"
	"sync/atomic"
	"time"
)

type Record struct {
	ID        string         `json:"id"`
	Table     string         `json:"table"`
	Data      map[string]any `json:"data"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

type Store struct {
	tables   map[string]*tableData
	mu       sync.RWMutex
	wal      []*WALEntry
	walMu    sync.Mutex
	walSize  atomic.Int64
	opsCount atomic.Int64
	started  time.Time
}

type tableData struct {
	rows    map[string]*Record
	index   map[string]map[string]any
	columns []string
}

type WALEntry struct {
	ID        string    `json:"id"`
	Op        string    `json:"op"`
	Table     string    `json:"table"`
	RecordID  string    `json:"record_id"`
	Data      any       `json:"data,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

type Stats struct {
	TableCount   int         `json:"table_count"`
	TotalRows    int         `json:"total_rows"`
	WALEntries   int64       `json:"wal_entries"`
	OpsCount     int64       `json:"ops_count"`
	Uptime       string      `json:"uptime"`
	MemoryTables []TableStat `json:"memory_tables"`
}

type TableStat struct {
	Name     string `json:"name"`
	RowCount int    `json:"row_count"`
	Columns  int    `json:"columns"`
}

func NewStore() *Store {
	return &Store{
		tables:  make(map[string]*tableData),
		wal:     make([]*WALEntry, 0),
		started: time.Now(),
	}
}

func (s *Store) CreateTable(table string, columns []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.tables[table]; exists {
		return nil
	}

	s.tables[table] = &tableData{
		rows:    make(map[string]*Record),
		index:   make(map[string]map[string]any),
		columns: columns,
	}

	s.appendWAL(&WALEntry{
		ID:        newWALID(),
		Op:        "create_table",
		Table:     table,
		Timestamp: time.Now(),
	})

	return nil
}

func (s *Store) DropTable(table string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.tables[table]; !exists {
		return ErrTableNotFound
	}

	delete(s.tables, table)

	s.appendWAL(&WALEntry{
		ID:        newWALID(),
		Op:        "drop_table",
		Table:     table,
		Timestamp: time.Now(),
	})

	return nil
}

func (s *Store) Insert(table string, record *Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	td, exists := s.tables[table]
	if !exists {
		return ErrTableNotFound
	}

	if record.ID == "" {
		record.ID = newRecordID()
	}
	now := time.Now()
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	record.UpdatedAt = now
	record.Table = table

	td.rows[record.ID] = record
	td.index[record.ID] = record.Data

	s.opsCount.Add(1)

	s.appendWAL(&WALEntry{
		ID:        newWALID(),
		Op:        "insert",
		Table:     table,
		RecordID:  record.ID,
		Data:      record.Data,
		Timestamp: now,
	})

	return nil
}

func (s *Store) Get(table, id string) (*Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	td, exists := s.tables[table]
	if !exists {
		return nil, ErrTableNotFound
	}

	record, exists := td.rows[id]
	if !exists {
		return nil, ErrRecordNotFound
	}

	s.opsCount.Add(1)
	return record, nil
}

func (s *Store) Update(table string, record *Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	td, exists := s.tables[table]
	if !exists {
		return ErrTableNotFound
	}

	existing, exists := td.rows[record.ID]
	if !exists {
		return ErrRecordNotFound
	}

	record.Table = table
	record.CreatedAt = existing.CreatedAt
	record.UpdatedAt = time.Now()

	td.rows[record.ID] = record
	td.index[record.ID] = record.Data

	s.opsCount.Add(1)

	s.appendWAL(&WALEntry{
		ID:        newWALID(),
		Op:        "update",
		Table:     table,
		RecordID:  record.ID,
		Data:      record.Data,
		Timestamp: time.Now(),
	})

	return nil
}

func (s *Store) Delete(table, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	td, exists := s.tables[table]
	if !exists {
		return ErrTableNotFound
	}

	if _, exists := td.rows[id]; !exists {
		return ErrRecordNotFound
	}

	delete(td.rows, id)
	delete(td.index, id)

	s.opsCount.Add(1)

	s.appendWAL(&WALEntry{
		ID:        newWALID(),
		Op:        "delete",
		Table:     table,
		RecordID:  id,
		Timestamp: time.Now(),
	})

	return nil
}

func (s *Store) List(table string, limit int) ([]*Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	td, exists := s.tables[table]
	if !exists {
		return nil, ErrTableNotFound
	}

	records := make([]*Record, 0, len(td.rows))
	for _, r := range td.rows {
		records = append(records, r)
	}

	if limit > 0 && len(records) > limit {
		records = records[:limit]
	}

	s.opsCount.Add(1)
	return records, nil
}

func (s *Store) Query(table string, field string, value any, limit int) ([]*Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	td, exists := s.tables[table]
	if !exists {
		return nil, ErrTableNotFound
	}

	var results []*Record
	for _, r := range td.rows {
		if v, ok := r.Data[field]; ok {
			if v == value {
				results = append(results, r)
				if limit > 0 && len(results) >= limit {
					break
				}
			}
		}
	}

	s.opsCount.Add(1)
	return results, nil
}

func (s *Store) Count(table string) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	td, exists := s.tables[table]
	if !exists {
		return 0, ErrTableNotFound
	}

	s.opsCount.Add(1)
	return len(td.rows), nil
}

func (s *Store) Stats() Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := Stats{
		TableCount:   len(s.tables),
		WALEntries:   s.walSize.Load(),
		OpsCount:     s.opsCount.Load(),
		Uptime:       time.Since(s.started).Round(time.Second).String(),
		MemoryTables: make([]TableStat, 0, len(s.tables)),
	}

	for name, td := range s.tables {
		stats.TotalRows += len(td.rows)
		stats.MemoryTables = append(stats.MemoryTables, TableStat{
			Name:     name,
			RowCount: len(td.rows),
			Columns:  len(td.columns),
		})
	}

	return stats
}

func (s *Store) WAL() []*WALEntry {
	s.walMu.Lock()
	defer s.walMu.Unlock()

	result := make([]*WALEntry, len(s.wal))
	copy(result, s.wal)
	return result
}

func (s *Store) WALSince(id string) []*WALEntry {
	s.walMu.Lock()
	defer s.walMu.Unlock()

	var sinceIdx int
	for i, e := range s.wal {
		if e.ID == id {
			sinceIdx = i + 1
			break
		}
	}

	if sinceIdx >= len(s.wal) {
		return nil
	}

	result := make([]*WALEntry, len(s.wal)-sinceIdx)
	copy(result, s.wal[sinceIdx:])
	return result
}

func (s *Store) TruncateWAL() {
	s.walMu.Lock()
	defer s.walMu.Unlock()

	s.wal = make([]*WALEntry, 0)
	s.walSize.Store(0)
}

func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.tables = make(map[string]*tableData)
	s.walMu.Lock()
	s.wal = make([]*WALEntry, 0)
	s.walMu.Unlock()
	s.walSize.Store(0)
	s.opsCount.Store(0)
}

func (s *Store) appendWAL(entry *WALEntry) {
	s.walMu.Lock()
	defer s.walMu.Unlock()

	s.wal = append(s.wal, entry)
	s.walSize.Add(1)

	if len(s.wal) > 100000 {
		s.wal = s.wal[50000:]
	}
}
