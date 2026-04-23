package qmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func setupTestServer(t *testing.T) *Server {
	t.Helper()

	s := NewServer(ServerConfig{
		HTTPAddr: ":0",
	})

	return s
}

func doRequest(t *testing.T, s *Server, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var reqBody []byte
	if body != nil {
		reqBody, _ = json.Marshal(body)
	}

	req := httptest.NewRequest(method, path, bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.newMux().ServeHTTP(w, req)
	return w
}

func TestStoreCreateTable(t *testing.T) {
	store := NewStore()

	err := store.CreateTable("users", []string{"id", "name", "email"})
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	stats := store.Stats()
	if stats.TableCount != 1 {
		t.Errorf("expected 1 table, got %d", stats.TableCount)
	}

	err = store.CreateTable("users", []string{"id"})
	if err != nil {
		t.Error("expected no error for duplicate create table")
	}
}

func TestStoreDropTable(t *testing.T) {
	store := NewStore()
	store.CreateTable("users", nil)

	err := store.DropTable("users")
	if err != nil {
		t.Fatalf("drop table: %v", err)
	}

	stats := store.Stats()
	if stats.TableCount != 0 {
		t.Errorf("expected 0 tables after drop, got %d", stats.TableCount)
	}

	err = store.DropTable("nonexistent")
	if err != ErrTableNotFound {
		t.Errorf("expected ErrTableNotFound, got %v", err)
	}
}

func TestStoreInsertAndGet(t *testing.T) {
	store := NewStore()
	store.CreateTable("users", []string{"id", "name"})

	record := &Record{
		ID:   "user1",
		Data: map[string]any{"name": "Alice", "age": 30},
	}

	err := store.Insert("users", record)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err := store.Get("users", "user1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	if got.ID != "user1" {
		t.Errorf("expected ID user1, got %s", got.ID)
	}
	if got.Data["name"] != "Alice" {
		t.Errorf("expected name Alice, got %v", got.Data["name"])
	}
}

func TestStoreInsertAutoID(t *testing.T) {
	store := NewStore()
	store.CreateTable("items", nil)

	record := &Record{
		Data: map[string]any{"name": "item"},
	}

	err := store.Insert("items", record)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	if record.ID == "" {
		t.Error("expected auto-generated ID")
	}
}

func TestStoreUpdate(t *testing.T) {
	store := NewStore()
	store.CreateTable("users", nil)

	store.Insert("users", &Record{ID: "u1", Data: map[string]any{"name": "old"}})

	err := store.Update("users", &Record{ID: "u1", Data: map[string]any{"name": "new"}})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	got, _ := store.Get("users", "u1")
	if got.Data["name"] != "new" {
		t.Errorf("expected name new, got %v", got.Data["name"])
	}
}

func TestStoreDelete(t *testing.T) {
	store := NewStore()
	store.CreateTable("users", nil)
	store.Insert("users", &Record{ID: "u1", Data: map[string]any{"name": "Alice"}})

	err := store.Delete("users", "u1")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err = store.Get("users", "u1")
	if err != ErrRecordNotFound {
		t.Errorf("expected ErrRecordNotFound after delete, got %v", err)
	}
}

func TestStoreList(t *testing.T) {
	store := NewStore()
	store.CreateTable("users", nil)

	for i := 0; i < 5; i++ {
		store.Insert("users", &Record{ID: string(rune('a' + i)), Data: map[string]any{"n": i}})
	}

	records, err := store.List("users", 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(records) != 5 {
		t.Errorf("expected 5 records, got %d", len(records))
	}

	records, err = store.List("users", 3)
	if err != nil {
		t.Fatalf("list with limit: %v", err)
	}

	if len(records) != 3 {
		t.Errorf("expected 3 records with limit, got %d", len(records))
	}
}

func TestStoreQuery(t *testing.T) {
	store := NewStore()
	store.CreateTable("users", nil)

	store.Insert("users", &Record{ID: "u1", Data: map[string]any{"role": "admin"}})
	store.Insert("users", &Record{ID: "u2", Data: map[string]any{"role": "user"}})
	store.Insert("users", &Record{ID: "u3", Data: map[string]any{"role": "admin"}})

	results, err := store.Query("users", "role", "admin", 10)
	if err != nil {
		t.Fatalf("query: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 admin users, got %d", len(results))
	}
}

func TestStoreCount(t *testing.T) {
	store := NewStore()
	store.CreateTable("users", nil)

	store.Insert("users", &Record{ID: "u1"})
	store.Insert("users", &Record{ID: "u2"})

	count, err := store.Count("users")
	if err != nil {
		t.Fatalf("count: %v", err)
	}

	if count != 2 {
		t.Errorf("expected count 2, got %d", count)
	}
}

func TestStoreStats(t *testing.T) {
	store := NewStore()
	store.CreateTable("users", []string{"id", "name"})
	store.CreateTable("items", []string{"id"})

	store.Insert("users", &Record{ID: "u1"})
	store.Insert("items", &Record{ID: "i1"})

	stats := store.Stats()

	if stats.TableCount != 2 {
		t.Errorf("expected 2 tables, got %d", stats.TableCount)
	}
	if stats.TotalRows != 2 {
		t.Errorf("expected 2 total rows, got %d", stats.TotalRows)
	}
	if stats.OpsCount < 2 {
		t.Errorf("expected at least 2 ops, got %d", stats.OpsCount)
	}
	if stats.Uptime == "" {
		t.Error("expected non-empty uptime")
	}
}

func TestStoreWAL(t *testing.T) {
	store := NewStore()
	store.CreateTable("users", nil)
	store.Insert("users", &Record{ID: "u1"})
	store.Insert("users", &Record{ID: "u2"})

	wal := store.WAL()

	if len(wal) < 3 {
		t.Errorf("expected at least 3 WAL entries, got %d", len(wal))
	}

	if wal[0].Op != "create_table" {
		t.Errorf("expected first WAL op create_table, got %s", wal[0].Op)
	}
}

func TestStoreWALSince(t *testing.T) {
	store := NewStore()
	store.CreateTable("t", nil)

	wal := store.WAL()
	firstID := wal[0].ID

	store.Insert("t", &Record{ID: "r1"})
	store.Insert("t", &Record{ID: "r2"})

	since := store.WALSince(firstID)
	if len(since) != 2 {
		t.Errorf("expected 2 WAL entries since first, got %d", len(since))
	}
}

func TestStoreTruncateWAL(t *testing.T) {
	store := NewStore()
	store.CreateTable("t", nil)
	store.Insert("t", &Record{ID: "r1"})

	store.TruncateWAL()

	wal := store.WAL()
	if len(wal) != 0 {
		t.Errorf("expected empty WAL after truncate, got %d", len(wal))
	}
}

func TestStoreClear(t *testing.T) {
	store := NewStore()
	store.CreateTable("t", nil)
	store.Insert("t", &Record{ID: "r1"})

	store.Clear()

	stats := store.Stats()
	if stats.TableCount != 0 {
		t.Errorf("expected 0 tables after clear, got %d", stats.TableCount)
	}
	if stats.TotalRows != 0 {
		t.Errorf("expected 0 rows after clear, got %d", stats.TotalRows)
	}
}

func TestHTTPCreateTable(t *testing.T) {
	s := setupTestServer(t)

	w := doRequest(t, s, "POST", "/v1/tables", map[string]any{
		"name":    "users",
		"columns": []string{"id", "name"},
	})

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHTTPInsertAndGet(t *testing.T) {
	s := setupTestServer(t)

	doRequest(t, s, "POST", "/v1/tables", map[string]any{
		"name": "users",
	})

	w := doRequest(t, s, "POST", "/v1/tables/users/records", map[string]any{
		"id":   "u1",
		"data": map[string]any{"name": "Alice"},
	})

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	w = doRequest(t, s, "GET", "/v1/tables/users/records/u1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHTTPUpdate(t *testing.T) {
	s := setupTestServer(t)

	doRequest(t, s, "POST", "/v1/tables", map[string]any{"name": "users"})
	doRequest(t, s, "POST", "/v1/tables/users/records", map[string]any{
		"id":   "u1",
		"data": map[string]any{"name": "old"},
	})

	w := doRequest(t, s, "PUT", "/v1/tables/users/records/u1", map[string]any{
		"data": map[string]any{"name": "new"},
	})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHTTPDelete(t *testing.T) {
	s := setupTestServer(t)

	doRequest(t, s, "POST", "/v1/tables", map[string]any{"name": "users"})
	doRequest(t, s, "POST", "/v1/tables/users/records", map[string]any{
		"id": "u1", "data": map[string]any{},
	})

	w := doRequest(t, s, "DELETE", "/v1/tables/users/records/u1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHTTPList(t *testing.T) {
	s := setupTestServer(t)

	doRequest(t, s, "POST", "/v1/tables", map[string]any{"name": "users"})
	doRequest(t, s, "POST", "/v1/tables/users/records", map[string]any{"id": "u1", "data": map[string]any{}})
	doRequest(t, s, "POST", "/v1/tables/users/records", map[string]any{"id": "u2", "data": map[string]any{}})

	w := doRequest(t, s, "GET", "/v1/tables/users/records?limit=10", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHTTPQuery(t *testing.T) {
	s := setupTestServer(t)

	doRequest(t, s, "POST", "/v1/tables", map[string]any{"name": "users"})
	doRequest(t, s, "POST", "/v1/tables/users/records", map[string]any{
		"id": "u1", "data": map[string]any{"role": "admin"},
	})
	doRequest(t, s, "POST", "/v1/tables/users/records", map[string]any{
		"id": "u2", "data": map[string]any{"role": "user"},
	})

	w := doRequest(t, s, "GET", "/v1/tables/users/query?field=role&value=admin&limit=10", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHTTPCount(t *testing.T) {
	s := setupTestServer(t)

	doRequest(t, s, "POST", "/v1/tables", map[string]any{"name": "users"})
	doRequest(t, s, "POST", "/v1/tables/users/records", map[string]any{"id": "u1", "data": map[string]any{}})
	doRequest(t, s, "POST", "/v1/tables/users/records", map[string]any{"id": "u2", "data": map[string]any{}})

	w := doRequest(t, s, "GET", "/v1/tables/users/count", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHTTPStats(t *testing.T) {
	s := setupTestServer(t)

	w := doRequest(t, s, "GET", "/v1/stats", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHTTPHealth(t *testing.T) {
	s := setupTestServer(t)

	w := doRequest(t, s, "GET", "/v1/health", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHTTPListTables(t *testing.T) {
	s := setupTestServer(t)

	doRequest(t, s, "POST", "/v1/tables", map[string]any{"name": "users"})
	doRequest(t, s, "POST", "/v1/tables", map[string]any{"name": "items"})

	w := doRequest(t, s, "GET", "/v1/tables", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHTTPDropTable(t *testing.T) {
	s := setupTestServer(t)

	doRequest(t, s, "POST", "/v1/tables", map[string]any{"name": "users"})

	w := doRequest(t, s, "DELETE", "/v1/tables/users", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHTTPWAL(t *testing.T) {
	s := setupTestServer(t)

	doRequest(t, s, "POST", "/v1/tables", map[string]any{"name": "users"})
	doRequest(t, s, "POST", "/v1/tables/users/records", map[string]any{"id": "u1", "data": map[string]any{}})

	w := doRequest(t, s, "GET", "/v1/wal", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHTTPTruncateWAL(t *testing.T) {
	s := setupTestServer(t)

	doRequest(t, s, "POST", "/v1/tables", map[string]any{"name": "users"})

	w := doRequest(t, s, "POST", "/v1/wal/truncate", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHTTPClear(t *testing.T) {
	s := setupTestServer(t)

	doRequest(t, s, "POST", "/v1/tables", map[string]any{"name": "users"})

	w := doRequest(t, s, "POST", "/v1/clear", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	stats := s.store.Stats()
	if stats.TableCount != 0 {
		t.Errorf("expected 0 tables after clear, got %d", stats.TableCount)
	}
}

func TestServerPersistAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	persistPath := filepath.Join(tmpDir, "qmd.wal")

	s1 := NewServer(ServerConfig{
		HTTPAddr:        ":0",
		PersistPath:     persistPath,
		PersistInterval: 100 * time.Millisecond,
	})

	s1.store.CreateTable("users", nil)
	s1.store.Insert("users", &Record{ID: "u1", Data: map[string]any{"name": "Alice"}})

	s1.persistOnce()

	s2 := NewServer(ServerConfig{
		HTTPAddr:    ":0",
		PersistPath: persistPath,
	})

	if err := s2.loadPersist(); err != nil {
		t.Fatalf("load persist: %v", err)
	}

	count, err := s2.store.Count("users")
	if err != nil {
		t.Fatalf("count: %v", err)
	}

	if count != 1 {
		t.Errorf("expected 1 record after reload, got %d", count)
	}
}

func TestServerStartStop(t *testing.T) {
	s := NewServer(ServerConfig{
		HTTPAddr: ":0",
	})

	if err := s.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

func TestServerDoubleStart(t *testing.T) {
	s := NewServer(ServerConfig{
		HTTPAddr: ":0",
	})

	s.Start()
	defer s.Shutdown(context.Background())

	err := s.Start()
	if err == nil {
		t.Error("expected error on double start")
	}
}

func TestStoreConcurrentAccess(t *testing.T) {
	store := NewStore()
	store.CreateTable("users", nil)

	var ops atomic.Int64
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := string(rune('a') + rune(n%26))
			store.Insert("users", &Record{ID: id, Data: map[string]any{"n": n}})
			store.Get("users", id)
			ops.Add(1)
		}(i)
	}

	wg.Wait()

	if ops.Load() != 100 {
		t.Errorf("expected 100 ops, got %d", ops.Load())
	}
}

func TestClientHTTP(t *testing.T) {
	s := NewServer(ServerConfig{
		HTTPAddr: ":19876",
	})

	if err := s.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer s.Shutdown(context.Background())

	time.Sleep(100 * time.Millisecond)

	client := NewClient(ClientConfig{
		Address:  "http://localhost:19876",
		Protocol: ProtocolHTTP,
		Timeout:  5 * time.Second,
	})

	ctx := context.Background()

	if err := client.Ping(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}

	if err := client.CreateTable(ctx, "users", []string{"id", "name"}); err != nil {
		t.Fatalf("create table: %v", err)
	}

	err := client.Insert(ctx, "users", &Record{
		ID:   "u1",
		Data: map[string]any{"name": "Alice", "age": 30},
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	record, err := client.Get(ctx, "users", "u1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if record.Data["name"] != "Alice" {
		t.Errorf("expected name Alice, got %v", record.Data["name"])
	}

	err = client.Update(ctx, "users", &Record{
		ID:   "u1",
		Data: map[string]any{"name": "Bob"},
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	record, _ = client.Get(ctx, "users", "u1")
	if record.Data["name"] != "Bob" {
		t.Errorf("expected name Bob after update, got %v", record.Data["name"])
	}

	records, err := client.List(ctx, "users", 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(records) != 1 {
		t.Errorf("expected 1 record, got %d", len(records))
	}

	count, err := client.Count(ctx, "users")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected count 1, got %d", count)
	}

	err = client.Delete(ctx, "users", "u1")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err = client.Get(ctx, "users", "u1")
	if err == nil {
		t.Error("expected error after delete")
	}

	tables, err := client.ListTables(ctx)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	if len(tables) != 1 {
		t.Errorf("expected 1 table, got %d", len(tables))
	}

	stats, err := client.Stats(ctx)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.TableCount != 1 {
		t.Errorf("expected 1 table in stats, got %d", stats.TableCount)
	}

	wal, err := client.WAL(ctx, "")
	if err != nil {
		t.Fatalf("wal: %v", err)
	}
	if len(wal) == 0 {
		t.Error("expected WAL entries")
	}

	if err := client.TruncateWAL(ctx); err != nil {
		t.Fatalf("truncate WAL: %v", err)
	}

	wal, err = client.WAL(ctx, "")
	if len(wal) != 0 {
		t.Errorf("expected empty WAL after truncate, got %d", len(wal))
	}

	if err := client.DropTable(ctx, "users"); err != nil {
		t.Fatalf("drop table: %v", err)
	}

	if err := client.Clear(ctx); err != nil {
		t.Fatalf("clear: %v", err)
	}
}

func TestClientBatchInsert(t *testing.T) {
	s := NewServer(ServerConfig{
		HTTPAddr: ":19877",
	})

	if err := s.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer s.Shutdown(context.Background())

	time.Sleep(100 * time.Millisecond)

	client := NewClient(ClientConfig{
		Address:  "http://localhost:19877",
		Protocol: ProtocolHTTP,
		Timeout:  5 * time.Second,
	})

	ctx := context.Background()
	client.CreateTable(ctx, "items", nil)

	records := []*Record{
		{ID: "i1", Data: map[string]any{"name": "item1"}},
		{ID: "i2", Data: map[string]any{"name": "item2"}},
		{ID: "i3", Data: map[string]any{"name": "item3"}},
	}

	if err := client.InsertBatch(ctx, "items", records); err != nil {
		t.Fatalf("batch insert: %v", err)
	}

	count, _ := client.Count(ctx, "items")
	if count != 3 {
		t.Errorf("expected 3 items after batch insert, got %d", count)
	}
}

func TestClientQuery(t *testing.T) {
	s := NewServer(ServerConfig{
		HTTPAddr: ":19878",
	})

	if err := s.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer s.Shutdown(context.Background())

	time.Sleep(100 * time.Millisecond)

	client := NewClient(ClientConfig{
		Address:  "http://localhost:19878",
		Protocol: ProtocolHTTP,
		Timeout:  5 * time.Second,
	})

	ctx := context.Background()
	client.CreateTable(ctx, "users", nil)
	client.Insert(ctx, "users", &Record{ID: "u1", Data: map[string]any{"role": "admin"}})
	client.Insert(ctx, "users", &Record{ID: "u2", Data: map[string]any{"role": "user"}})
	client.Insert(ctx, "users", &Record{ID: "u3", Data: map[string]any{"role": "admin"}})

	results, err := client.Query(ctx, "users", "role", "admin", 10)
	if err != nil {
		t.Fatalf("query: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 admin users, got %d", len(results))
	}
}

func TestClientRetry(t *testing.T) {
	client := NewClient(ClientConfig{
		Address:    "http://localhost:19999",
		Protocol:   ProtocolHTTP,
		Timeout:    100 * time.Millisecond,
		RetryCount: 2,
		RetryDelay: 10 * time.Millisecond,
	})

	ctx := context.Background()
	err := client.Ping(ctx)
	if err == nil {
		t.Error("expected error for unreachable server")
	}
}

func TestClientUnixSocket(t *testing.T) {
	if os.Getenv("GOOS") == "windows" {
		t.Skip("unix sockets not supported on Windows")
	}

	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "qmd.sock")

	s := NewServer(ServerConfig{
		UnixSocket: socketPath,
	})

	if err := s.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer s.Shutdown(context.Background())

	time.Sleep(50 * time.Millisecond)

	client := NewClient(UnixClientConfig(socketPath))

	ctx := context.Background()
	if err := client.Ping(ctx); err != nil {
		t.Fatalf("ping via unix socket: %v", err)
	}

	if err := client.CreateTable(ctx, "test", nil); err != nil {
		t.Fatalf("create table via unix socket: %v", err)
	}

	tables, err := client.ListTables(ctx)
	if err != nil {
		t.Fatalf("list tables via unix socket: %v", err)
	}
	if len(tables) != 1 {
		t.Errorf("expected 1 table via unix socket, got %d", len(tables))
	}
}

func TestProtocolMessage(t *testing.T) {
	store := NewStore()
	handler := NewProtocolHandler(store, DefaultProtocolConfig())

	msg, _ := NewRequest("req1", RequestPayload{
		Action: "create_table",
		Table:  "users",
		Params: map[string]any{"columns": []any{"id", "name"}},
	})

	resp := handler.HandleMessage(msg)
	if resp.Type != MsgResponse {
		t.Fatalf("expected response, got %s", resp.Type)
	}

	var payload ResponsePayload
	resp.UnmarshalPayload(&payload)
	if !payload.Success {
		t.Error("expected success")
	}
}

func TestProtocolInsertAndGet(t *testing.T) {
	store := NewStore()
	handler := NewProtocolHandler(store, DefaultProtocolConfig())

	handler.HandleMessage(&Message{
		Type: MsgRequest,
		Payload: mustMarshal(RequestPayload{
			Action: "create_table",
			Table:  "users",
		}),
	})

	handler.HandleMessage(&Message{
		Type: MsgRequest,
		Payload: mustMarshal(RequestPayload{
			Action: "insert",
			Table:  "users",
			ID:     "u1",
			Data:   map[string]any{"name": "Alice"},
		}),
	})

	resp := handler.HandleMessage(&Message{
		Type: MsgRequest,
		Payload: mustMarshal(RequestPayload{
			Action: "get",
			Table:  "users",
			ID:     "u1",
		}),
	})

	var payload ResponsePayload
	resp.UnmarshalPayload(&payload)
	if len(payload.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(payload.Records))
	}
	if payload.Records[0].Data["name"] != "Alice" {
		t.Errorf("expected name Alice, got %v", payload.Records[0].Data["name"])
	}
}

func TestProtocolListAndQuery(t *testing.T) {
	store := NewStore()
	handler := NewProtocolHandler(store, DefaultProtocolConfig())

	handler.HandleMessage(&Message{
		Type:    MsgRequest,
		Payload: mustMarshal(RequestPayload{Action: "create_table", Table: "users"}),
	})

	handler.HandleMessage(&Message{
		Type: MsgRequest,
		Payload: mustMarshal(RequestPayload{
			Action: "insert", Table: "users", ID: "u1",
			Data: map[string]any{"role": "admin"},
		}),
	})
	handler.HandleMessage(&Message{
		Type: MsgRequest,
		Payload: mustMarshal(RequestPayload{
			Action: "insert", Table: "users", ID: "u2",
			Data: map[string]any{"role": "user"},
		}),
	})

	resp := handler.HandleMessage(&Message{
		Type: MsgRequest,
		Payload: mustMarshal(RequestPayload{
			Action: "list", Table: "users",
			Params: map[string]any{"limit": 10},
		}),
	})

	var payload ResponsePayload
	resp.UnmarshalPayload(&payload)
	if payload.Count != 2 {
		t.Errorf("expected 2 records, got %d", payload.Count)
	}

	resp = handler.HandleMessage(&Message{
		Type: MsgRequest,
		Payload: mustMarshal(RequestPayload{
			Action: "query", Table: "users",
			Params: map[string]any{"field": "role", "value": "admin", "limit": 10},
		}),
	})

	resp.UnmarshalPayload(&payload)
	if payload.Count != 1 {
		t.Errorf("expected 1 admin, got %d", payload.Count)
	}
}

func TestProtocolStats(t *testing.T) {
	store := NewStore()
	handler := NewProtocolHandler(store, DefaultProtocolConfig())

	handler.HandleMessage(&Message{
		Type:    MsgRequest,
		Payload: mustMarshal(RequestPayload{Action: "create_table", Table: "users"}),
	})

	resp := handler.HandleMessage(&Message{
		Type:    MsgRequest,
		Payload: mustMarshal(RequestPayload{Action: "stats"}),
	})

	var payload ResponsePayload
	resp.UnmarshalPayload(&payload)
	if payload.Stats == nil {
		t.Fatal("expected stats")
	}
	if payload.Stats.TableCount != 1 {
		t.Errorf("expected 1 table, got %d", payload.Stats.TableCount)
	}
}

func TestProtocolUnknownAction(t *testing.T) {
	store := NewStore()
	handler := NewProtocolHandler(store, DefaultProtocolConfig())

	resp := handler.HandleMessage(&Message{
		Type: MsgRequest,
		Payload: mustMarshal(RequestPayload{
			Action: "unknown_action",
		}),
	})

	if resp.Type != MsgError {
		t.Errorf("expected error response, got %s", resp.Type)
	}
}

func TestProtocolHeartbeat(t *testing.T) {
	store := NewStore()
	handler := NewProtocolHandler(store, DefaultProtocolConfig())

	resp := handler.HandleMessage(NewHeartbeat())

	if resp.Type != MsgHeartbeat {
		t.Errorf("expected heartbeat response, got %s", resp.Type)
	}
}

func TestEventBus(t *testing.T) {
	eb := NewEventBus(false)

	var received int
	var mu sync.Mutex
	eb.Subscribe(EventRecordInserted, func(event *Message) {
		mu.Lock()
		received++
		mu.Unlock()
	})

	event, _ := NewEvent(EventRecordInserted, map[string]any{"table": "users"})
	eb.Publish(event)

	if received != 1 {
		t.Errorf("expected 1 event received, got %d", received)
	}
}

func TestEventBusUnsubscribe(t *testing.T) {
	eb := NewEventBus(false)

	var received int
	var mu sync.Mutex
	eb.Subscribe(EventRecordInserted, func(event *Message) {
		mu.Lock()
		received++
		mu.Unlock()
	})

	event, _ := NewEvent(EventRecordInserted, map[string]any{})
	eb.Publish(event)

	eb.Unsubscribe(EventRecordInserted)
	eb.Publish(event)

	if received != 1 {
		t.Errorf("expected 1 event after unsubscribe, got %d", received)
	}
}

func TestMessageEncodeDecode(t *testing.T) {
	msg, _ := NewRequest("test", map[string]any{"key": "value"})

	data, err := msg.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	decoded, err := DecodeMessage(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if decoded.ID != msg.ID {
		t.Errorf("expected ID %s, got %s", msg.ID, decoded.ID)
	}
	if decoded.Type != msg.Type {
		t.Errorf("expected type %s, got %s", msg.Type, decoded.Type)
	}
}

func TestMessageUnmarshalPayload(t *testing.T) {
	msg, _ := NewRequest("test", map[string]any{"name": "Alice", "age": 30})

	var data map[string]any
	if err := msg.UnmarshalPayload(&data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data["name"] != "Alice" {
		t.Errorf("expected name Alice, got %v", data["name"])
	}
}

func TestErrorMessage(t *testing.T) {
	msg := NewErrorMessage(fmt.Errorf("test error"))

	if msg.Type != MsgError {
		t.Errorf("expected error type, got %s", msg.Type)
	}
	if msg.Error != "test error" {
		t.Errorf("expected error message 'test error', got %s", msg.Error)
	}
}

func mustMarshal(v any) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}
