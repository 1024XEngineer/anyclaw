package qmd

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSyncManagerImportJSON(t *testing.T) {
	store := NewStore()
	sm := NewSyncManager(store, DefaultSyncConfig())

	jsonData := `[{"id":"u1","name":"Alice","age":30},{"id":"u2","name":"Bob","age":25}]`
	ctx := context.Background()

	result, err := sm.ImportJSON(ctx, "users", strings.NewReader(jsonData))
	if err != nil {
		t.Fatalf("import JSON: %v", err)
	}

	if result.Succeeded != 2 {
		t.Errorf("expected 2 succeeded, got %d", result.Succeeded)
	}

	count, _ := store.Count("users")
	if count != 2 {
		t.Errorf("expected 2 records, got %d", count)
	}
}

func TestSyncManagerImportJSONInvalid(t *testing.T) {
	store := NewStore()
	sm := NewSyncManager(store, DefaultSyncConfig())

	ctx := context.Background()
	_, err := sm.ImportJSON(ctx, "users", strings.NewReader("invalid json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestSyncManagerImportCSV(t *testing.T) {
	store := NewStore()
	sm := NewSyncManager(store, DefaultSyncConfig())

	csvData := "id,name,age\nu1,Alice,30\nu2,Bob,25\n"
	ctx := context.Background()

	result, err := sm.ImportCSV(ctx, "users", strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("import CSV: %v", err)
	}

	if result.Succeeded != 2 {
		t.Errorf("expected 2 succeeded, got %d", result.Succeeded)
	}

	count, _ := store.Count("users")
	if count != 2 {
		t.Errorf("expected 2 records, got %d", count)
	}
}

func TestSyncManagerImportCSVEmpty(t *testing.T) {
	store := NewStore()
	sm := NewSyncManager(store, DefaultSyncConfig())

	ctx := context.Background()
	_, err := sm.ImportCSV(ctx, "users", strings.NewReader(""))
	if err == nil {
		t.Error("expected error for empty CSV")
	}
}

func TestSyncManagerExportJSON(t *testing.T) {
	store := NewStore()
	sm := NewSyncManager(store, DefaultSyncConfig())

	store.CreateTable("users", nil)
	store.Insert("users", &Record{ID: "u1", Data: map[string]any{"name": "Alice"}})
	store.Insert("users", &Record{ID: "u2", Data: map[string]any{"name": "Bob"}})

	var buf bytes.Buffer
	ctx := context.Background()

	if err := sm.ExportJSON(ctx, "users", &buf); err != nil {
		t.Fatalf("export JSON: %v", err)
	}

	if buf.Len() == 0 {
		t.Error("expected non-empty JSON output")
	}
}

func TestSyncManagerExportCSV(t *testing.T) {
	store := NewStore()
	sm := NewSyncManager(store, DefaultSyncConfig())

	store.CreateTable("users", nil)
	store.Insert("users", &Record{ID: "u1", Data: map[string]any{"name": "Alice"}})
	store.Insert("users", &Record{ID: "u2", Data: map[string]any{"name": "Bob"}})

	var buf bytes.Buffer
	ctx := context.Background()

	if err := sm.ExportCSV(ctx, "users", &buf); err != nil {
		t.Fatalf("export CSV: %v", err)
	}

	if buf.Len() == 0 {
		t.Error("expected non-empty CSV output")
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) < 2 {
		t.Errorf("expected at least 2 lines (header + data), got %d", len(lines))
	}
}

func TestSyncManagerExportEmpty(t *testing.T) {
	store := NewStore()
	sm := NewSyncManager(store, DefaultSyncConfig())

	store.CreateTable("users", nil)

	var buf bytes.Buffer
	ctx := context.Background()

	if err := sm.ExportCSV(ctx, "users", &buf); err != nil {
		t.Fatalf("export empty CSV: %v", err)
	}
}

func TestSyncManagerBatch(t *testing.T) {
	store := NewStore()
	sm := NewSyncManager(store, DefaultSyncConfig())

	store.CreateTable("users", nil)

	ops := []BatchOperation{
		{Table: "users", Action: "insert", ID: "u1", Data: map[string]any{"name": "Alice"}},
		{Table: "users", Action: "insert", ID: "u2", Data: map[string]any{"name": "Bob"}},
		{Table: "users", Action: "update", ID: "u1", Data: map[string]any{"name": "Alice Updated"}},
		{Table: "users", Action: "delete", ID: "u2"},
	}

	ctx := context.Background()
	result, err := sm.ExecuteBatch(ctx, ops)
	if err != nil {
		t.Fatalf("execute batch: %v", err)
	}

	if result.Succeeded != 4 {
		t.Errorf("expected 4 succeeded, got %d", result.Succeeded)
	}

	record, _ := store.Get("users", "u1")
	if record.Data["name"] != "Alice Updated" {
		t.Errorf("expected updated name, got %v", record.Data["name"])
	}

	_, err = store.Get("users", "u2")
	if err != ErrRecordNotFound {
		t.Error("expected u2 to be deleted")
	}
}

func TestSyncManagerBatchUnknownAction(t *testing.T) {
	store := NewStore()
	sm := NewSyncManager(store, DefaultSyncConfig())

	store.CreateTable("users", nil)

	ops := []BatchOperation{
		{Table: "users", Action: "unknown", ID: "u1"},
	}

	ctx := context.Background()
	result, err := sm.ExecuteBatch(ctx, ops)
	if err != nil {
		t.Fatalf("execute batch: %v", err)
	}

	if result.Failed != 1 {
		t.Errorf("expected 1 failed, got %d", result.Failed)
	}
}

func TestSyncManagerSplitRecords(t *testing.T) {
	sm := NewSyncManager(NewStore(), SyncConfig{ChunkSize: 3})

	records := []*Record{
		{ID: "1"}, {ID: "2"}, {ID: "3"}, {ID: "4"}, {ID: "5"},
	}

	chunks := sm.splitRecords(records)

	if len(chunks) != 2 {
		t.Errorf("expected 2 chunks, got %d", len(chunks))
	}
	if len(chunks[0]) != 3 {
		t.Errorf("expected first chunk size 3, got %d", len(chunks[0]))
	}
	if len(chunks[1]) != 2 {
		t.Errorf("expected second chunk size 2, got %d", len(chunks[1]))
	}
}

func TestSyncManagerSplitMaps(t *testing.T) {
	sm := NewSyncManager(NewStore(), SyncConfig{ChunkSize: 2})

	rows := []map[string]any{
		{"a": 1}, {"b": 2}, {"c": 3},
	}

	chunks := sm.splitMaps(rows)

	if len(chunks) != 2 {
		t.Errorf("expected 2 chunks, got %d", len(chunks))
	}
}

func TestSyncManagerRetry(t *testing.T) {
	store := NewStore()
	sm := NewSyncManager(store, SyncConfig{
		MaxRetries: 2,
		RetryDelay: 10 * time.Millisecond,
	})

	callCount := 0
	err := sm.retry(context.Background(), func() error {
		callCount++
		if callCount < 3 {
			return context.DeadlineExceeded
		}
		return nil
	})

	if err != nil {
		t.Errorf("expected success after retries, got %v", err)
	}
	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
}

func TestSyncManagerRetryExhausted(t *testing.T) {
	store := NewStore()
	sm := NewSyncManager(store, SyncConfig{
		MaxRetries: 1,
		RetryDelay: 10 * time.Millisecond,
	})

	err := sm.retry(context.Background(), func() error {
		return context.DeadlineExceeded
	})

	if err == nil {
		t.Error("expected error after retries exhausted")
	}
}

func TestSyncManagerImportFileJSON(t *testing.T) {
	tmpFile := t.TempDir() + "/test.json"
	f, _ := os.Create(tmpFile)
	f.WriteString(`[{"id":"r1","val":"test"}]`)
	f.Close()

	store := NewStore()
	sm := NewSyncManager(store, DefaultSyncConfig())

	ctx := context.Background()
	result, err := sm.ImportFile(ctx, "data", tmpFile)
	if err != nil {
		t.Fatalf("import file: %v", err)
	}

	if result.Succeeded != 1 {
		t.Errorf("expected 1 succeeded, got %d", result.Succeeded)
	}
}

func TestSyncManagerImportFileCSV(t *testing.T) {
	tmpFile := t.TempDir() + "/test.csv"
	f, _ := os.Create(tmpFile)
	f.WriteString("id,val\nr1,test\n")
	f.Close()

	store := NewStore()
	sm := NewSyncManager(store, DefaultSyncConfig())

	ctx := context.Background()
	result, err := sm.ImportFile(ctx, "data", tmpFile)
	if err != nil {
		t.Fatalf("import file: %v", err)
	}

	if result.Succeeded != 1 {
		t.Errorf("expected 1 succeeded, got %d", result.Succeeded)
	}
}

func TestSyncManagerImportFileNotFound(t *testing.T) {
	store := NewStore()
	sm := NewSyncManager(store, DefaultSyncConfig())

	ctx := context.Background()
	_, err := sm.ImportFile(ctx, "data", "/nonexistent/file.json")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func osCreate(path string) (*osFile, error) {
	f, err := osCreateReal(path)
	return (*osFile)(f), err
}

type osFile = os.File

func osCreateReal(path string) (*os.File, error) {
	return os.Create(path)
}

func TestSyncManagerProgressCallback(t *testing.T) {
	store := NewStore()

	var progressCount int
	sm := NewSyncManager(store, SyncConfig{
		ChunkSize: 1,
		OnProgress: func(p SyncProgress) {
			progressCount++
		},
	})

	store.CreateTable("users", nil)
	for i := 0; i < 5; i++ {
		store.Insert("users", &Record{ID: string(rune('a' + i)), Data: map[string]any{"n": i}})
	}

	ctx := context.Background()
	_, err := sm.SyncToSQLite(ctx, &mockSQLiteAdapter{})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}

	if progressCount == 0 {
		t.Error("expected progress callbacks")
	}
}

type mockSQLiteAdapter struct{}

func (m *mockSQLiteAdapter) BatchInsert(ctx context.Context, table string, records []map[string]any) error {
	return nil
}

func (m *mockSQLiteAdapter) BatchDelete(ctx context.Context, table string, ids []string) error {
	return nil
}

func (m *mockSQLiteAdapter) ListAll(ctx context.Context, table string) ([]map[string]any, error) {
	return nil, nil
}

func (m *mockSQLiteAdapter) Tables() ([]string, error) {
	return nil, nil
}
