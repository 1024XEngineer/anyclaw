package api

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"time"

	"github.com/anyclaw/anyclaw/pkg/embedding"
	"github.com/anyclaw/anyclaw/pkg/index"
	"github.com/anyclaw/anyclaw/pkg/vec"
)

type Server struct {
	mux       *http.ServeMux
	im        *index.IndexManager
	embedder  embedding.Provider
	batchProc *embedding.BatchProcessor
	addr      string
}

type ServerConfig struct {
	Addr        string
	IndexMgr    *index.IndexManager
	Embedder    embedding.Provider
	BatchConfig *embedding.BatchConfig
}

func NewServer(cfg ServerConfig) *Server {
	s := &Server{
		mux:      http.NewServeMux(),
		im:       cfg.IndexMgr,
		embedder: cfg.Embedder,
		addr:     cfg.Addr,
	}
	if s.addr == "" {
		s.addr = ":8080"
	}
	if cfg.Embedder != nil {
		mgr, _ := embedding.NewManager([]embedding.Provider{cfg.Embedder}, nil)
		batchCfg := embedding.DefaultBatchConfig()
		if cfg.BatchConfig != nil {
			batchCfg = *cfg.BatchConfig
		}
		s.batchProc = embedding.NewBatchProcessor(mgr, batchCfg)
	}
	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("POST /v1/search", s.handleSearch)
	s.mux.HandleFunc("POST /v1/search/text", s.handleSearchText)
	s.mux.HandleFunc("POST /v1/pure-search", s.handlePureSearch)
	s.mux.HandleFunc("POST /v1/embed", s.handleEmbed)
	s.mux.HandleFunc("POST /v1/embed/batch", s.handleEmbedBatch)
	s.mux.HandleFunc("GET /v1/indexes", s.handleListIndexes)
	s.mux.HandleFunc("POST /v1/indexes", s.handleCreateIndex)
	s.mux.HandleFunc("GET /v1/indexes/{name}", s.handleGetIndex)
	s.mux.HandleFunc("DELETE /v1/indexes/{name}", s.handleDeleteIndex)
	s.mux.HandleFunc("POST /v1/indexes/{name}/rebuild", s.handleRebuildIndex)
	s.mux.HandleFunc("GET /v1/health", s.handleHealth)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) Start() error {
	return http.ListenAndServe(s.addr, s)
}

func (s *Server) StartWithContext(ctx context.Context) error {
	server := &http.Server{
		Addr:    s.addr,
		Handler: s,
	}

	go func() {
		<-ctx.Done()
		server.Shutdown(context.Background())
	}()

	return server.ListenAndServe()
}

type SearchRequest struct {
	Index     string         `json:"index"`
	Vector    []float32      `json:"vector"`
	Limit     int            `json:"limit"`
	Threshold float64        `json:"threshold"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type SearchResult struct {
	ID       any            `json:"id"`
	Distance float64        `json:"distance"`
	Score    float64        `json:"score"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type SearchResponse struct {
	Results  []SearchResult `json:"results"`
	Count    int            `json:"count"`
	Index    string         `json:"index"`
	Duration string         `json:"duration"`
}

type EmbedRequest struct {
	Text string `json:"text"`
}

type EmbedResponse struct {
	Vector    []float32 `json:"vector"`
	Dimension int       `json:"dimension"`
	Provider  string    `json:"provider"`
}

type CreateIndexRequest struct {
	Name       string             `json:"name"`
	Dimensions int                `json:"dimensions"`
	Distance   vec.DistanceMetric `json:"distance"`
	Metadata   []string           `json:"metadata"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"service": "vector-search",
	})
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	var req SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Index == "" {
		writeError(w, http.StatusBadRequest, "index is required")
		return
	}
	if len(req.Vector) == 0 {
		writeError(w, http.StatusBadRequest, "vector is required")
		return
	}

	if req.Limit <= 0 {
		req.Limit = 10
	}

	start := time.Now()

	ctx := r.Context()
	results, err := s.im.Search(ctx, req.Index, req.Vector, req.Limit)
	if err != nil {
		writeErrorDetail(w, http.StatusBadRequest, "search failed", err.Error())
		return
	}

	searchResults := make([]SearchResult, 0, len(results))
	for _, r := range results {
		if req.Threshold > 0 && r.Distance > req.Threshold {
			continue
		}

		score := 1.0 - r.Distance
		if score < 0 {
			score = 0
		}

		searchResults = append(searchResults, SearchResult{
			ID:       r.ID,
			Distance: r.Distance,
			Score:    math.Round(score*10000) / 10000,
			Metadata: r.Metadata,
		})
	}

	writeJSON(w, http.StatusOK, SearchResponse{
		Results:  searchResults,
		Count:    len(searchResults),
		Index:    req.Index,
		Duration: time.Since(start).String(),
	})
}

func (s *Server) handleSearchText(w http.ResponseWriter, r *http.Request) {
	if s.embedder == nil {
		writeError(w, http.StatusBadGateway, "no embedder configured")
		return
	}

	var req struct {
		Index     string         `json:"index"`
		Text      string         `json:"text"`
		Limit     int            `json:"limit"`
		Threshold float64        `json:"threshold"`
		Metadata  map[string]any `json:"metadata,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Index == "" {
		writeError(w, http.StatusBadRequest, "index is required")
		return
	}
	if req.Text == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}

	if req.Limit <= 0 {
		req.Limit = 10
	}

	start := time.Now()

	ctx := r.Context()
	results, err := s.im.SearchByText(ctx, req.Index, req.Text, req.Limit)
	if err != nil {
		writeErrorDetail(w, http.StatusBadRequest, "search failed", err.Error())
		return
	}

	searchResults := make([]SearchResult, 0, len(results))
	for _, r := range results {
		if req.Threshold > 0 && r.Distance > req.Threshold {
			continue
		}

		score := 1.0 - r.Distance
		if score < 0 {
			score = 0
		}

		searchResults = append(searchResults, SearchResult{
			ID:       r.ID,
			Distance: r.Distance,
			Score:    math.Round(score*10000) / 10000,
			Metadata: r.Metadata,
		})
	}

	writeJSON(w, http.StatusOK, SearchResponse{
		Results:  searchResults,
		Count:    len(searchResults),
		Index:    req.Index,
		Duration: time.Since(start).String(),
	})
}

func (s *Server) handleEmbed(w http.ResponseWriter, r *http.Request) {
	if s.embedder == nil {
		writeError(w, http.StatusBadGateway, "no embedder configured")
		return
	}

	var req EmbedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Text == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}

	ctx := r.Context()
	vector, err := s.embedder.Embed(ctx, req.Text)
	if err != nil {
		writeErrorDetail(w, http.StatusInternalServerError, "embedding failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, EmbedResponse{
		Vector:    vector,
		Dimension: len(vector),
		Provider:  s.embedder.Name(),
	})
}

type EmbedBatchRequest struct {
	Texts       []string `json:"texts"`
	Concurrency int      `json:"concurrency"`
	ChunkSize   int      `json:"chunk_size"`
	MaxRetries  int      `json:"max_retries"`
	RateLimit   int      `json:"rate_limit"`
}

type EmbedBatchResponse struct {
	Embeddings [][]float32  `json:"embeddings"`
	Errors     []BatchError `json:"errors,omitempty"`
	Total      int          `json:"total"`
	Succeeded  int          `json:"succeeded"`
	Failed     int          `json:"failed"`
	Duration   string       `json:"duration"`
	Provider   string       `json:"provider"`
}

type BatchError struct {
	Index int    `json:"index"`
	Text  string `json:"text"`
	Error string `json:"error"`
}

func (s *Server) handleEmbedBatch(w http.ResponseWriter, r *http.Request) {
	if s.batchProc == nil {
		writeError(w, http.StatusBadGateway, "batch processing not configured")
		return
	}

	var req EmbedBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.Texts) == 0 {
		writeError(w, http.StatusBadRequest, "texts array is required")
		return
	}
	if len(req.Texts) > 10000 {
		writeError(w, http.StatusBadRequest, "maximum 10000 texts per batch")
		return
	}

	bp := s.batchProc
	if req.Concurrency > 0 || req.ChunkSize > 0 || req.MaxRetries >= 0 || req.RateLimit > 0 {
		cfg := s.batchProc.Config()
		if req.Concurrency > 0 {
			cfg.Concurrency = req.Concurrency
		}
		if req.ChunkSize > 0 {
			cfg.ChunkSize = req.ChunkSize
		}
		if req.MaxRetries >= 0 {
			cfg.MaxRetries = req.MaxRetries
		}
		if req.RateLimit > 0 {
			cfg.RateLimitPerSec = req.RateLimit
		}
		bp = embedding.NewBatchProcessor(s.batchProc.Manager(), cfg)
	}

	ctx := r.Context()
	result, err := bp.Process(ctx, req.Texts)
	if err != nil {
		writeErrorDetail(w, http.StatusInternalServerError, "batch processing failed", err.Error())
		return
	}

	batchErrors := make([]BatchError, len(result.Errors))
	for i, e := range result.Errors {
		batchErrors[i] = BatchError{
			Index: e.Index,
			Text:  e.Text,
			Error: e.Err.Error(),
		}
	}

	writeJSON(w, http.StatusOK, EmbedBatchResponse{
		Embeddings: result.Embeddings,
		Errors:     batchErrors,
		Total:      result.Total,
		Succeeded:  result.Succeeded,
		Failed:     result.Failed,
		Duration:   result.Duration.String(),
		Provider:   s.embedder.Name(),
	})
}

func (s *Server) handleListIndexes(w http.ResponseWriter, r *http.Request) {
	indexes := s.im.List()
	writeJSON(w, http.StatusOK, indexes)
}

func (s *Server) handleCreateIndex(w http.ResponseWriter, r *http.Request) {
	var req CreateIndexRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Dimensions <= 0 {
		writeError(w, http.StatusBadRequest, "dimensions must be positive")
		return
	}

	if req.Distance == "" {
		req.Distance = vec.DistanceCosine
	}

	ctx := r.Context()
	info, err := s.im.Create(ctx, index.Config{
		Name:       req.Name,
		Dimensions: req.Dimensions,
		Distance:   req.Distance,
		Metadata:   req.Metadata,
	})
	if err != nil {
		writeErrorDetail(w, http.StatusConflict, "create index failed", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, info)
}

func (s *Server) handleGetIndex(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "index name is required")
		return
	}

	info, err := s.im.Get(name)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, info)
}

func (s *Server) handleDeleteIndex(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "index name is required")
		return
	}

	ctx := r.Context()
	if err := s.im.Delete(ctx, name); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "index": name})
}

func (s *Server) handleRebuildIndex(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "index name is required")
		return
	}

	ctx := r.Context()
	result, err := s.im.Rebuild(ctx, name, nil)
	if err != nil {
		writeErrorDetail(w, http.StatusBadRequest, "rebuild failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) HandleIndex(ctx context.Context, indexName string, items []index.IndexItem, progress index.ProgressFunc) (*index.IndexResult, error) {
	return s.im.Index(ctx, indexName, items, progress)
}

func (s *Server) RemoveVectors(ctx context.Context, indexName string, ids []any) (int, error) {
	return s.im.RemoveVectors(ctx, indexName, ids)
}

func (s *Server) CosineSimilarity(a, b []float32) float64 {
	return vec.CosineSimilarity(a, b)
}

func (s *Server) CosineDistance(a, b []float32) float64 {
	return vec.CosineDistance(a, b)
}

type scoredEntry struct {
	index    int
	distance float64
	score    float64
}

type PureSearchRequest struct {
	Vectors   [][]float32 `json:"vectors"`
	Query     []float32   `json:"query"`
	Limit     int         `json:"limit"`
	TopK      int         `json:"top_k"`
	Threshold float64     `json:"threshold"`
	Metric    string      `json:"metric"`
}

type PureSearchResult struct {
	Index    int     `json:"index"`
	Distance float64 `json:"distance"`
	Score    float64 `json:"score"`
}

type PureSearchResponse struct {
	Results  []PureSearchResult `json:"results"`
	Count    int                `json:"count"`
	Metric   string             `json:"metric"`
	Duration string             `json:"duration"`
}

func (s *Server) handlePureSearch(w http.ResponseWriter, r *http.Request) {
	var req PureSearchRequest
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if len(req.Query) == 0 {
		writeError(w, http.StatusBadRequest, "query vector is required")
		return
	}

	vectors := req.Vectors
	if len(vectors) == 0 {
		writeError(w, http.StatusBadRequest, "vectors are required")
		return
	}

	if req.Limit <= 0 {
		req.Limit = 10
	}
	if req.TopK > 0 && req.TopK < req.Limit {
		req.Limit = req.TopK
	}

	metric := req.Metric
	if metric == "" {
		metric = "cosine"
	}

	start := time.Now()

	type scored struct {
		index    int
		distance float64
		score    float64
	}

	var scoredResults []scoredEntry
	for i, v := range vectors {
		var distance float64
		switch metric {
		case "cosine":
			distance = vec.CosineDistance(req.Query, v)
		case "l2", "euclidean":
			distance = vec.L2Distance(req.Query, v)
		default:
			distance = vec.CosineDistance(req.Query, v)
		}

		score := 1.0 - distance
		if score < 0 {
			score = 0
		}

		if req.Threshold > 0 && distance > req.Threshold {
			continue
		}

		scoredResults = append(scoredResults, scoredEntry{
			index:    i,
			distance: distance,
			score:    math.Round(score*10000) / 10000,
		})
	}

	sortByScore(scoredResults)

	if len(scoredResults) > req.Limit {
		scoredResults = scoredResults[:req.Limit]
	}

	results := make([]PureSearchResult, len(scoredResults))
	for i, sr := range scoredResults {
		results[i] = PureSearchResult{
			Index:    sr.index,
			Distance: sr.distance,
			Score:    sr.score,
		}
	}

	writeJSON(w, http.StatusOK, PureSearchResponse{
		Results:  results,
		Count:    len(results),
		Metric:   metric,
		Duration: time.Since(start).String(),
	})
}

func sortByScore(results []scoredEntry) {
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].score > results[i].score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
}
