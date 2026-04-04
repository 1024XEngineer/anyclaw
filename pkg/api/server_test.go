package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "modernc.org/sqlite"
	_ "modernc.org/sqlite/vec"

	"github.com/anyclaw/anyclaw/pkg/index"
)

func setupTestServer(t *testing.T) (*Server, *mockEmbedder) {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	embedder := &mockEmbedder{dim: 4}
	im := index.NewIndexManager(db, embedder)
	im.Init(context.Background())

	im.Create(context.Background(), index.Config{
		Name:       "test_index",
		Dimensions: 4,
		Distance:   "cosine",
	})

	im.Index(context.Background(), "test_index", []index.IndexItem{
		{ID: 1, Vector: []float32{0.1, 0.2, 0.3, 0.4}},
		{ID: 2, Vector: []float32{0.5, 0.6, 0.7, 0.8}},
		{ID: 3, Vector: []float32{0.9, 1.0, 0.1, 0.2}},
	}, nil)

	server := NewServer(ServerConfig{
		IndexMgr: im,
		Embedder: embedder,
	})

	return server, embedder
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
	s.ServeHTTP(w, req)
	return w
}

func TestHealthEndpoint(t *testing.T) {
	s, _ := setupTestServer(t)
	w := doRequest(t, s, "GET", "/v1/health", nil)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "ok" {
		t.Errorf("expected status ok, got %s", resp["status"])
	}
}

func TestSearchEndpoint(t *testing.T) {
	s, _ := setupTestServer(t)

	reqBody := map[string]any{
		"index":  "test_index",
		"vector": []float32{0.1, 0.2, 0.3, 0.4},
		"limit":  10,
	}

	w := doRequest(t, s, "POST", "/v1/search", reqBody)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp SearchResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Count != 3 {
		t.Errorf("expected 3 results, got %d", resp.Count)
	}
	if resp.Index != "test_index" {
		t.Errorf("expected index test_index, got %s", resp.Index)
	}
	if resp.Duration == "" {
		t.Error("expected non-empty duration")
	}

	if resp.Results[0].Score < 0.99 {
		t.Errorf("expected first result score ~1.0, got %f", resp.Results[0].Score)
	}
}

func TestSearchWithThreshold(t *testing.T) {
	s, _ := setupTestServer(t)

	reqBody := map[string]any{
		"index":     "test_index",
		"vector":    []float32{0.1, 0.2, 0.3, 0.4},
		"limit":     10,
		"threshold": 0.01,
	}

	w := doRequest(t, s, "POST", "/v1/search", reqBody)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp SearchResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Count < 1 {
		t.Errorf("expected at least 1 result with threshold, got %d", resp.Count)
	}
}

func TestSearchMissingIndex(t *testing.T) {
	s, _ := setupTestServer(t)

	reqBody := map[string]any{
		"index":  "nonexistent",
		"vector": []float32{0.1, 0.2, 0.3, 0.4},
	}

	w := doRequest(t, s, "POST", "/v1/search", reqBody)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSearchMissingVector(t *testing.T) {
	s, _ := setupTestServer(t)

	reqBody := map[string]any{
		"index": "test_index",
	}

	w := doRequest(t, s, "POST", "/v1/search", reqBody)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSearchTextEndpoint(t *testing.T) {
	s, _ := setupTestServer(t)

	reqBody := map[string]any{
		"index": "test_index",
		"text":  "search query",
		"limit": 10,
	}

	w := doRequest(t, s, "POST", "/v1/search/text", reqBody)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp SearchResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Count != 3 {
		t.Errorf("expected 3 results, got %d", resp.Count)
	}
}

func TestSearchTextMissingText(t *testing.T) {
	s, _ := setupTestServer(t)

	reqBody := map[string]any{
		"index": "test_index",
	}

	w := doRequest(t, s, "POST", "/v1/search/text", reqBody)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestEmbedEndpoint(t *testing.T) {
	s, embedder := setupTestServer(t)

	reqBody := map[string]any{
		"text": "hello world",
	}

	w := doRequest(t, s, "POST", "/v1/embed", reqBody)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp EmbedResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Dimension != 4 {
		t.Errorf("expected dimension 4, got %d", resp.Dimension)
	}
	if resp.Provider != "mock" {
		t.Errorf("expected provider mock, got %s", resp.Provider)
	}
	if len(resp.Vector) != 4 {
		t.Errorf("expected 4 vector elements, got %d", len(resp.Vector))
	}
	if embedder.callCount.Load() != 1 {
		t.Errorf("expected 1 embed call, got %d", embedder.callCount.Load())
	}
}

func TestEmbedMissingText(t *testing.T) {
	s, _ := setupTestServer(t)

	reqBody := map[string]any{}

	w := doRequest(t, s, "POST", "/v1/embed", reqBody)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestListIndexes(t *testing.T) {
	s, _ := setupTestServer(t)

	w := doRequest(t, s, "GET", "/v1/indexes", nil)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var indexes []map[string]any
	json.Unmarshal(w.Body.Bytes(), &indexes)

	if len(indexes) != 1 {
		t.Errorf("expected 1 index, got %d", len(indexes))
	}
}

func TestGetIndex(t *testing.T) {
	s, _ := setupTestServer(t)

	w := doRequest(t, s, "GET", "/v1/indexes/test_index", nil)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var info map[string]any
	json.Unmarshal(w.Body.Bytes(), &info)

	if info["name"] != "test_index" {
		t.Errorf("expected name test_index, got %v", info["name"])
	}
}

func TestGetIndexNotFound(t *testing.T) {
	s, _ := setupTestServer(t)

	w := doRequest(t, s, "GET", "/v1/indexes/nonexistent", nil)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestCreateIndex(t *testing.T) {
	s, _ := setupTestServer(t)

	reqBody := map[string]any{
		"name":       "new_index",
		"dimensions": 8,
		"distance":   "cosine",
		"metadata":   []string{"category"},
	}

	w := doRequest(t, s, "POST", "/v1/indexes", reqBody)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var info map[string]any
	json.Unmarshal(w.Body.Bytes(), &info)

	if info["name"] != "new_index" {
		t.Errorf("expected name new_index, got %v", info["name"])
	}
}

func TestCreateIndexDuplicate(t *testing.T) {
	s, _ := setupTestServer(t)

	reqBody := map[string]any{
		"name":       "test_index",
		"dimensions": 4,
	}

	w := doRequest(t, s, "POST", "/v1/indexes", reqBody)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

func TestDeleteIndex(t *testing.T) {
	s, _ := setupTestServer(t)

	w := doRequest(t, s, "DELETE", "/v1/indexes/test_index", nil)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "deleted" {
		t.Errorf("expected status deleted, got %s", resp["status"])
	}
}

func TestRebuildIndex(t *testing.T) {
	s, _ := setupTestServer(t)

	w := doRequest(t, s, "POST", "/v1/indexes/test_index/rebuild", nil)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPureSearchCosine(t *testing.T) {
	s, _ := setupTestServer(t)

	reqBody := PureSearchRequest{
		Vectors: [][]float32{
			{0.1, 0.2, 0.3, 0.4},
			{0.5, 0.6, 0.7, 0.8},
			{0.9, 1.0, 0.1, 0.2},
		},
		Query:  []float32{0.1, 0.2, 0.3, 0.4},
		Limit:  10,
		Metric: "cosine",
	}

	w := doRequest(t, s, "POST", "/v1/pure-search", reqBody)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp PureSearchResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Count != 3 {
		t.Errorf("expected 3 results, got %d", resp.Count)
	}
	if resp.Metric != "cosine" {
		t.Errorf("expected metric cosine, got %s", resp.Metric)
	}
	if resp.Results[0].Index != 0 {
		t.Errorf("expected first result index 0, got %d", resp.Results[0].Index)
	}
	if resp.Results[0].Score < 0.99 {
		t.Errorf("expected first result score ~1.0, got %f", resp.Results[0].Score)
	}
}

func TestPureSearchL2(t *testing.T) {
	s, _ := setupTestServer(t)

	reqBody := PureSearchRequest{
		Vectors: [][]float32{
			{0.0, 0.0, 0.0, 0.0},
			{3.0, 4.0, 0.0, 0.0},
		},
		Query:  []float32{0.0, 0.0, 0.0, 0.0},
		Limit:  10,
		Metric: "l2",
	}

	w := doRequest(t, s, "POST", "/v1/pure-search", reqBody)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp PureSearchResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Count != 2 {
		t.Errorf("expected 2 results, got %d", resp.Count)
	}
	if resp.Results[0].Distance > 0.01 {
		t.Errorf("expected first result distance ~0, got %f", resp.Results[0].Distance)
	}
}

func TestPureSearchThreshold(t *testing.T) {
	s, _ := setupTestServer(t)

	reqBody := PureSearchRequest{
		Vectors: [][]float32{
			{0.1, 0.2, 0.3, 0.4},
			{0.5, 0.6, 0.7, 0.8},
			{0.9, 1.0, 0.1, 0.2},
		},
		Query:     []float32{0.1, 0.2, 0.3, 0.4},
		Limit:     10,
		Threshold: 0.01,
	}

	w := doRequest(t, s, "POST", "/v1/pure-search", reqBody)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp PureSearchResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Count < 1 {
		t.Errorf("expected at least 1 result with threshold, got %d", resp.Count)
	}
}

func TestPureSearchLimit(t *testing.T) {
	s, _ := setupTestServer(t)

	reqBody := PureSearchRequest{
		Vectors: [][]float32{
			{0.1, 0.2, 0.3, 0.4},
			{0.5, 0.6, 0.7, 0.8},
			{0.9, 1.0, 0.1, 0.2},
		},
		Query: []float32{0.1, 0.2, 0.3, 0.4},
		Limit: 1,
	}

	w := doRequest(t, s, "POST", "/v1/pure-search", reqBody)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp PureSearchResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Count != 1 {
		t.Errorf("expected 1 result with limit=1, got %d", resp.Count)
	}
}

func TestEmbedBatchEndpoint(t *testing.T) {
	s, embedder := setupTestServer(t)

	reqBody := map[string]any{
		"texts":       []string{"hello", "world", "batch test"},
		"concurrency": 2,
		"chunk_size":  2,
	}

	w := doRequest(t, s, "POST", "/v1/embed/batch", reqBody)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp EmbedBatchResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Total != 3 {
		t.Errorf("expected total 3, got %d", resp.Total)
	}
	if resp.Succeeded != 3 {
		t.Errorf("expected succeeded 3, got %d", resp.Succeeded)
	}
	if resp.Failed != 0 {
		t.Errorf("expected failed 0, got %d", resp.Failed)
	}
	if len(resp.Embeddings) != 3 {
		t.Errorf("expected 3 embeddings, got %d", len(resp.Embeddings))
	}
	if embedder.callCount.Load() != 3 {
		t.Errorf("expected 3 embed calls, got %d", embedder.callCount.Load())
	}
}

func TestEmbedBatchEmptyTexts(t *testing.T) {
	s, _ := setupTestServer(t)

	reqBody := map[string]any{
		"texts": []string{},
	}

	w := doRequest(t, s, "POST", "/v1/embed/batch", reqBody)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestEmbedBatchTooMany(t *testing.T) {
	s, _ := setupTestServer(t)

	texts := make([]string, 10001)
	for i := range texts {
		texts[i] = "text"
	}

	reqBody := map[string]any{
		"texts": texts,
	}

	w := doRequest(t, s, "POST", "/v1/embed/batch", reqBody)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestEmbedBatchWithRateLimit(t *testing.T) {
	s, embedder := setupTestServer(t)

	reqBody := map[string]any{
		"texts":       []string{"a", "b", "c", "d", "e"},
		"rate_limit":  100,
		"chunk_size":  2,
		"concurrency": 1,
	}

	w := doRequest(t, s, "POST", "/v1/embed/batch", reqBody)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp EmbedBatchResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Succeeded != 5 {
		t.Errorf("expected 5 succeeded, got %d", resp.Succeeded)
	}
	if embedder.callCount.Load() != 5+5 {
		t.Logf("embed call count: %d", embedder.callCount.Load())
	}
}

func TestEmbedBatchResponseStructure(t *testing.T) {
	s, _ := setupTestServer(t)

	reqBody := map[string]any{
		"texts": []string{"test1", "test2"},
	}

	w := doRequest(t, s, "POST", "/v1/embed/batch", reqBody)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp EmbedBatchResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Provider == "" {
		t.Error("expected non-empty provider")
	}
	if resp.Duration == "" {
		t.Error("expected non-empty duration")
	}
	if resp.Errors != nil && len(resp.Errors) > 0 {
		t.Errorf("expected no errors, got %d", len(resp.Errors))
	}
}

type mockEmbedder struct {
	dim       int
	callCount atomicInt32
}

type atomicInt32 struct {
	v int32
}

func (a *atomicInt32) Add(n int32) int32 {
	a.v += n
	return a.v
}

func (a *atomicInt32) Load() int32 {
	return a.v
}

func (m *mockEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	m.callCount.Add(1)
	result := make([]float32, m.dim)
	for i := range result {
		result[i] = float32(len(text)) / float32(m.dim)
	}
	return result, nil
}

func (m *mockEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	var results [][]float32
	for _, text := range texts {
		emb, err := m.Embed(ctx, text)
		if err != nil {
			return nil, err
		}
		results = append(results, emb)
	}
	return results, nil
}

func (m *mockEmbedder) Name() string   { return "mock" }
func (m *mockEmbedder) Dimension() int { return m.dim }
