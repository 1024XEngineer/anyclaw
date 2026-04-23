package qmd

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

type SyncConfig struct {
	ChunkSize   int
	Concurrency int
	MaxRetries  int
	RetryDelay  time.Duration
	Timeout     time.Duration
	OnProgress  func(progress SyncProgress)
}

func DefaultSyncConfig() SyncConfig {
	return SyncConfig{
		ChunkSize:   100,
		Concurrency: 4,
		MaxRetries:  3,
		RetryDelay:  100 * time.Millisecond,
		Timeout:     5 * time.Minute,
	}
}

type SyncProgress struct {
	Total     int
	Processed int
	Succeeded int
	Failed    int
	Elapsed   time.Duration
	ETA       time.Duration
	Current   string
	Message   string
	Done      bool
}

type SyncResult struct {
	Total     int           `json:"total"`
	Processed int           `json:"processed"`
	Succeeded int           `json:"succeeded"`
	Failed    int           `json:"failed"`
	Duration  time.Duration `json:"duration"`
	Errors    []SyncError   `json:"errors,omitempty"`
}

type SyncError struct {
	Index int    `json:"index"`
	ID    string `json:"id,omitempty"`
	Error string `json:"error"`
}

type SyncManager struct {
	store  *Store
	config SyncConfig
}

func NewSyncManager(store *Store, cfg SyncConfig) *SyncManager {
	if cfg.ChunkSize <= 0 {
		cfg.ChunkSize = 100
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 4
	}
	if cfg.MaxRetries < 0 {
		cfg.MaxRetries = 3
	}
	if cfg.RetryDelay <= 0 {
		cfg.RetryDelay = 100 * time.Millisecond
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Minute
	}

	return &SyncManager{
		store:  store,
		config: cfg,
	}
}

type SyncDirection string

const (
	SyncToSQLite      SyncDirection = "to_sqlite"
	SyncFromSQLite    SyncDirection = "from_sqlite"
	SyncBidirectional SyncDirection = "bidirectional"
)

type SQLiteSyncAdapter interface {
	BatchInsert(ctx context.Context, table string, records []map[string]any) error
	BatchDelete(ctx context.Context, table string, ids []string) error
	ListAll(ctx context.Context, table string) ([]map[string]any, error)
	Tables() ([]string, error)
}

func (sm *SyncManager) SyncToSQLite(ctx context.Context, adapter SQLiteSyncAdapter) (*SyncResult, error) {
	ctx, cancel := context.WithTimeout(ctx, sm.config.Timeout)
	defer cancel()

	stats := sm.store.Stats()
	start := time.Now()
	var processed, succeeded, failed int
	var errors []SyncError
	var mu sync.Mutex

	tables := make([]string, 0, len(stats.MemoryTables))
	for _, t := range stats.MemoryTables {
		tables = append(tables, t.Name)
	}

	for _, table := range tables {
		records, err := sm.store.List(table, 0)
		if err != nil {
			continue
		}

		chunks := sm.splitRecords(records)
		for _, chunk := range chunks {
			if ctx.Err() != nil {
				return &SyncResult{
					Total:     stats.TotalRows,
					Processed: processed,
					Succeeded: succeeded,
					Failed:    failed,
					Duration:  time.Since(start),
					Errors:    errors,
				}, ctx.Err()
			}

			data := make([]map[string]any, len(chunk))
			for i, r := range chunk {
				data[i] = r.Data
				data[i]["_qmd_id"] = r.ID
				data[i]["_qmd_created_at"] = r.CreatedAt
				data[i]["_qmd_updated_at"] = r.UpdatedAt
			}

			err := sm.retry(ctx, func() error {
				return adapter.BatchInsert(ctx, table, data)
			})

			mu.Lock()
			processed += len(chunk)
			if err != nil {
				failed += len(chunk)
				errors = append(errors, SyncError{
					Index: processed,
					Error: fmt.Sprintf("table %s: %v", table, err),
				})
			} else {
				succeeded += len(chunk)
			}
			p := processed
			mu.Unlock()

			if sm.config.OnProgress != nil {
				elapsed := time.Since(start)
				rate := float64(p) / elapsed.Seconds()
				remaining := stats.TotalRows - p
				eta := time.Duration(float64(remaining)/rate) * time.Second

				sm.config.OnProgress(SyncProgress{
					Total:     stats.TotalRows,
					Processed: p,
					Succeeded: succeeded,
					Failed:    failed,
					Elapsed:   elapsed,
					ETA:       eta,
					Current:   table,
					Message:   fmt.Sprintf("synced %d/%d", p, stats.TotalRows),
				})
			}
		}
	}

	duration := time.Since(start)

	if sm.config.OnProgress != nil {
		sm.config.OnProgress(SyncProgress{
			Total:     stats.TotalRows,
			Processed: processed,
			Succeeded: succeeded,
			Failed:    failed,
			Elapsed:   duration,
			Done:      true,
			Message:   fmt.Sprintf("sync completed: %d succeeded, %d failed", succeeded, failed),
		})
	}

	return &SyncResult{
		Total:     stats.TotalRows,
		Succeeded: succeeded,
		Failed:    failed,
		Duration:  duration,
		Errors:    errors,
	}, nil
}

func (sm *SyncManager) SyncFromSQLite(ctx context.Context, adapter SQLiteSyncAdapter, tables []string) (*SyncResult, error) {
	ctx, cancel := context.WithTimeout(ctx, sm.config.Timeout)
	defer cancel()

	start := time.Now()
	var processed, succeeded, failed int
	var errors []SyncError
	var mu sync.Mutex

	if len(tables) == 0 {
		var err error
		tables, err = adapter.Tables()
		if err != nil {
			return nil, fmt.Errorf("qmd: list tables: %w", err)
		}
	}

	for _, table := range tables {
		if err := sm.store.CreateTable(table, nil); err != nil {
			return nil, fmt.Errorf("qmd: create table %s: %w", table, err)
		}

		rows, err := adapter.ListAll(ctx, table)
		if err != nil {
			mu.Lock()
			failed++
			errors = append(errors, SyncError{Error: fmt.Sprintf("list %s: %v", table, err)})
			mu.Unlock()
			continue
		}

		chunks := sm.splitMaps(rows)
		for _, chunk := range chunks {
			if ctx.Err() != nil {
				return &SyncResult{
					Total:     processed,
					Processed: processed,
					Succeeded: succeeded,
					Failed:    failed,
					Duration:  time.Since(start),
					Errors:    errors,
				}, ctx.Err()
			}

			for _, row := range chunk {
				id, _ := row["_qmd_id"].(string)
				if id == "" {
					id = fmt.Sprintf("sqlite_%d", time.Now().UnixNano())
				}

				delete(row, "_qmd_id")
				delete(row, "_qmd_created_at")
				delete(row, "_qmd_updated_at")

				record := &Record{
					ID:   id,
					Data: row,
				}

				err := sm.retry(ctx, func() error {
					return sm.store.Insert(table, record)
				})

				mu.Lock()
				processed++
				if err != nil {
					failed++
					errors = append(errors, SyncError{ID: id, Error: err.Error()})
				} else {
					succeeded++
				}
				p := processed
				mu.Unlock()

				if sm.config.OnProgress != nil {
					sm.config.OnProgress(SyncProgress{
						Total:     processed,
						Processed: p,
						Succeeded: succeeded,
						Failed:    failed,
						Elapsed:   time.Since(start),
						Current:   table,
						Message:   fmt.Sprintf("imported %d records", p),
					})
				}
			}
		}
	}

	duration := time.Since(start)

	if sm.config.OnProgress != nil {
		sm.config.OnProgress(SyncProgress{
			Total:     processed,
			Processed: processed,
			Succeeded: succeeded,
			Failed:    failed,
			Elapsed:   duration,
			Done:      true,
			Message:   fmt.Sprintf("import completed: %d succeeded, %d failed", succeeded, failed),
		})
	}

	return &SyncResult{
		Total:     processed,
		Succeeded: succeeded,
		Failed:    failed,
		Duration:  duration,
		Errors:    errors,
	}, nil
}

func (sm *SyncManager) ImportJSON(ctx context.Context, table string, reader io.Reader) (*SyncResult, error) {
	ctx, cancel := context.WithTimeout(ctx, sm.config.Timeout)
	defer cancel()

	if err := sm.store.CreateTable(table, nil); err != nil {
		return nil, fmt.Errorf("qmd: create table: %w", err)
	}

	dec := json.NewDecoder(reader)

	if _, err := dec.Token(); err != nil {
		return nil, fmt.Errorf("qmd: invalid JSON: %w", err)
	}

	start := time.Now()
	var processed, succeeded, failed int
	var errors []SyncError

	for dec.More() {
		if ctx.Err() != nil {
			return &SyncResult{
				Total:     processed,
				Processed: processed,
				Succeeded: succeeded,
				Failed:    failed,
				Duration:  time.Since(start),
				Errors:    errors,
			}, ctx.Err()
		}

		var data map[string]any
		if err := dec.Decode(&data); err != nil {
			failed++
			errors = append(errors, SyncError{Index: processed, Error: err.Error()})
			processed++
			continue
		}

		id, _ := data["id"].(string)
		delete(data, "id")

		record := &Record{ID: id, Data: data}

		err := sm.retry(ctx, func() error {
			return sm.store.Insert(table, record)
		})

		processed++
		if err != nil {
			failed++
			errors = append(errors, SyncError{ID: id, Error: err.Error()})
		} else {
			succeeded++
		}

		if sm.config.OnProgress != nil {
			sm.config.OnProgress(SyncProgress{
				Total:     processed,
				Processed: processed,
				Succeeded: succeeded,
				Failed:    failed,
				Elapsed:   time.Since(start),
				Current:   table,
				Message:   fmt.Sprintf("imported %d records", processed),
			})
		}
	}

	duration := time.Since(start)

	if sm.config.OnProgress != nil {
		sm.config.OnProgress(SyncProgress{
			Total:     processed,
			Processed: processed,
			Succeeded: succeeded,
			Failed:    failed,
			Elapsed:   duration,
			Done:      true,
			Message:   fmt.Sprintf("import completed: %d succeeded, %d failed", succeeded, failed),
		})
	}

	return &SyncResult{
		Total:     processed,
		Succeeded: succeeded,
		Failed:    failed,
		Duration:  duration,
		Errors:    errors,
	}, nil
}

func (sm *SyncManager) ImportCSV(ctx context.Context, table string, reader io.Reader) (*SyncResult, error) {
	ctx, cancel := context.WithTimeout(ctx, sm.config.Timeout)
	defer cancel()

	csvReader := csv.NewReader(reader)
	headers, err := csvReader.Read()
	if err != nil {
		return nil, fmt.Errorf("qmd: read CSV headers: %w", err)
	}

	if err := sm.store.CreateTable(table, headers); err != nil {
		return nil, fmt.Errorf("qmd: create table: %w", err)
	}

	start := time.Now()
	var processed, succeeded, failed int
	var errors []SyncError

	for {
		if ctx.Err() != nil {
			return &SyncResult{
				Total:     processed,
				Processed: processed,
				Succeeded: succeeded,
				Failed:    failed,
				Duration:  time.Since(start),
				Errors:    errors,
			}, ctx.Err()
		}

		row, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			failed++
			errors = append(errors, SyncError{Index: processed, Error: err.Error()})
			processed++
			continue
		}

		data := make(map[string]any)
		for i, h := range headers {
			if i < len(row) {
				data[h] = row[i]
			}
		}

		id, _ := data["id"].(string)
		record := &Record{ID: id, Data: data}

		err = sm.retry(ctx, func() error {
			return sm.store.Insert(table, record)
		})

		processed++
		if err != nil {
			failed++
			errors = append(errors, SyncError{ID: id, Error: err.Error()})
		} else {
			succeeded++
		}

		if sm.config.OnProgress != nil {
			sm.config.OnProgress(SyncProgress{
				Total:     processed,
				Processed: processed,
				Succeeded: succeeded,
				Failed:    failed,
				Elapsed:   time.Since(start),
				Current:   table,
				Message:   fmt.Sprintf("imported %d records", processed),
			})
		}
	}

	duration := time.Since(start)

	if sm.config.OnProgress != nil {
		sm.config.OnProgress(SyncProgress{
			Total:     processed,
			Processed: processed,
			Succeeded: succeeded,
			Failed:    failed,
			Elapsed:   duration,
			Done:      true,
			Message:   fmt.Sprintf("import completed: %d succeeded, %d failed", succeeded, failed),
		})
	}

	return &SyncResult{
		Total:     processed,
		Succeeded: succeeded,
		Failed:    failed,
		Duration:  duration,
		Errors:    errors,
	}, nil
}

func (sm *SyncManager) ImportFile(ctx context.Context, table string, path string) (*SyncResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("qmd: open file: %w", err)
	}
	defer f.Close()

	if strings.HasSuffix(strings.ToLower(path), ".csv") {
		return sm.ImportCSV(ctx, table, f)
	}
	return sm.ImportJSON(ctx, table, f)
}

func (sm *SyncManager) ExportJSON(ctx context.Context, table string, writer io.Writer) error {
	records, err := sm.store.List(table, 0)
	if err != nil {
		return err
	}

	enc := json.NewEncoder(writer)
	enc.SetIndent("", "  ")

	if err := enc.Encode(records); err != nil {
		return fmt.Errorf("qmd: encode JSON: %w", err)
	}

	return nil
}

func (sm *SyncManager) ExportCSV(ctx context.Context, table string, writer io.Writer) error {
	records, err := sm.store.List(table, 0)
	if err != nil {
		return err
	}

	if len(records) == 0 {
		return nil
	}

	w := csv.NewWriter(writer)

	var headers []string
	for k := range records[0].Data {
		headers = append(headers, k)
	}
	headers = append([]string{"id"}, headers...)

	if err := w.Write(headers); err != nil {
		return fmt.Errorf("qmd: write CSV headers: %w", err)
	}

	for _, r := range records {
		row := []string{r.ID}
		for _, h := range headers[1:] {
			val := ""
			if v, ok := r.Data[h]; ok {
				val = fmt.Sprintf("%v", v)
			}
			row = append(row, val)
		}
		if err := w.Write(row); err != nil {
			return fmt.Errorf("qmd: write CSV row: %w", err)
		}
	}

	w.Flush()
	return w.Error()
}

func (sm *SyncManager) retry(ctx context.Context, fn func() error) error {
	var lastErr error
	for attempt := 0; attempt <= sm.config.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(sm.config.RetryDelay * time.Duration(attempt)):
			}
		}

		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}
	return lastErr
}

func (sm *SyncManager) splitRecords(records []*Record) [][]*Record {
	var chunks [][]*Record
	for i := 0; i < len(records); i += sm.config.ChunkSize {
		end := i + sm.config.ChunkSize
		if end > len(records) {
			end = len(records)
		}
		chunks = append(chunks, records[i:end])
	}
	return chunks
}

func (sm *SyncManager) splitMaps(rows []map[string]any) [][]map[string]any {
	var chunks [][]map[string]any
	for i := 0; i < len(rows); i += sm.config.ChunkSize {
		end := i + sm.config.ChunkSize
		if end > len(rows) {
			end = len(rows)
		}
		chunks = append(chunks, rows[i:end])
	}
	return chunks
}

type BatchOperation struct {
	Table  string
	Action string
	ID     string
	Data   map[string]any
}

type BatchResult struct {
	Total     int           `json:"total"`
	Succeeded int           `json:"succeeded"`
	Failed    int           `json:"failed"`
	Duration  time.Duration `json:"duration"`
	Errors    []SyncError   `json:"errors,omitempty"`
}

func (sm *SyncManager) ExecuteBatch(ctx context.Context, ops []BatchOperation) (*BatchResult, error) {
	ctx, cancel := context.WithTimeout(ctx, sm.config.Timeout)
	defer cancel()

	start := time.Now()
	var succeeded, failed int
	var errors []SyncError
	var mu sync.Mutex

	sem := make(chan struct{}, sm.config.Concurrency)
	var wg sync.WaitGroup

	for i, op := range ops {
		if ctx.Err() != nil {
			break
		}

		sem <- struct{}{}
		wg.Add(1)

		go func(idx int, o BatchOperation) {
			defer wg.Done()
			defer func() { <-sem }()

			var err error
			switch o.Action {
			case "insert":
				err = sm.retry(ctx, func() error {
					return sm.store.Insert(o.Table, &Record{ID: o.ID, Data: o.Data})
				})
			case "update":
				err = sm.retry(ctx, func() error {
					return sm.store.Update(o.Table, &Record{ID: o.ID, Data: o.Data})
				})
			case "delete":
				err = sm.retry(ctx, func() error {
					return sm.store.Delete(o.Table, o.ID)
				})
			default:
				err = fmt.Errorf("unknown action: %s", o.Action)
			}

			mu.Lock()
			if err != nil {
				failed++
				errors = append(errors, SyncError{Index: idx, ID: o.ID, Error: err.Error()})
			} else {
				succeeded++
			}
			mu.Unlock()
		}(i, op)
	}

	wg.Wait()

	return &BatchResult{
		Total:     len(ops),
		Succeeded: succeeded,
		Failed:    failed,
		Duration:  time.Since(start),
		Errors:    errors,
	}, nil
}
