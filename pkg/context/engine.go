package context

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type ContextEngine interface {
	Name() string
	Type() string
	Initialize(config map[string]any) error
	AddDocument(ctx context.Context, doc Document) error
	Search(ctx context.Context, query string, options SearchOptions) ([]SearchResult, error)
	GetDocument(ctx context.Context, id string) (*Document, error)
	DeleteDocument(ctx context.Context, id string) error
	Close() error
}

type Document struct {
	ID        string         `json:"id"`
	Content   string         `json:"content"`
	Metadata  map[string]any `json:"metadata"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	Vector    []float64      `json:"vector,omitempty"`
}

type SearchOptions struct {
	TopK          int     `json:"top_k"`
	Threshold     float64 `json:"threshold"`
	Filters       map[string]any
	IncludeVector bool
}

type SearchResult struct {
	Document *Document `json:"document"`
	Score    float64   `json:"score"`
	Distance float64   `json:"distance,omitempty"`
}

type ContextEngineRegistry struct {
	engines map[string]ContextEngine
	mu      sync.RWMutex
}

func NewContextEngineRegistry() *ContextEngineRegistry {
	return &ContextEngineRegistry{
		engines: make(map[string]ContextEngine),
	}
}

func (r *ContextEngineRegistry) Register(name string, engine ContextEngine) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.engines[name]; exists {
		return fmt.Errorf("context engine already registered: %s", name)
	}
	r.engines[name] = engine
	return nil
}

func (r *ContextEngineRegistry) Get(name string) (ContextEngine, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	engine, ok := r.engines[name]
	return engine, ok
}

func (r *ContextEngineRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.engines))
	for name := range r.engines {
		names = append(names, name)
	}
	return names
}

func (r *ContextEngineRegistry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.engines[name]; !exists {
		return fmt.Errorf("context engine not found: %s", name)
	}

	engine := r.engines[name]
	if err := engine.Close(); err != nil {
		return fmt.Errorf("failed to close engine: %w", err)
	}

	delete(r.engines, name)
	return nil
}

type InMemoryContextEngine struct {
	mu         sync.RWMutex
	documents  map[string]*Document
	vectorSize int
}

func NewInMemoryContextEngine() *InMemoryContextEngine {
	return &InMemoryContextEngine{
		documents:  make(map[string]*Document),
		vectorSize: 1536,
	}
}

func (e *InMemoryContextEngine) Name() string { return "in-memory" }
func (e *InMemoryContextEngine) Type() string { return "memory" }

func (e *InMemoryContextEngine) Initialize(config map[string]any) error {
	if v, ok := config["vector_size"].(float64); ok {
		e.vectorSize = int(v)
	}
	return nil
}

func (e *InMemoryContextEngine) AddDocument(ctx context.Context, doc Document) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	doc.CreatedAt = time.Now()
	doc.UpdatedAt = time.Now()
	e.documents[doc.ID] = &doc
	return nil
}

func (e *InMemoryContextEngine) Search(ctx context.Context, query string, options SearchOptions) ([]SearchResult, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	results := make([]SearchResult, 0)
	for _, doc := range e.documents {
		score := calculateSimilarity(query, doc.Content)
		if score >= options.Threshold {
			results = append(results, SearchResult{
				Document: doc,
				Score:    score,
			})
		}
	}

	if len(results) > options.TopK {
		results = results[:options.TopK]
	}

	return results, nil
}

func (e *InMemoryContextEngine) GetDocument(ctx context.Context, id string) (*Document, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	doc, ok := e.documents[id]
	if !ok {
		return nil, fmt.Errorf("document not found: %s", id)
	}
	return doc, nil
}

func (e *InMemoryContextEngine) DeleteDocument(ctx context.Context, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, ok := e.documents[id]; !ok {
		return fmt.Errorf("document not found: %s", id)
	}
	delete(e.documents, id)
	return nil
}

func (e *InMemoryContextEngine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.documents = make(map[string]*Document)
	return nil
}

func calculateSimilarity(query, content string) float64 {
	contentWords := countWords(content)

	if contentWords == 0 {
		return 0
	}

	overlap := 0
	querySet := wordSet(query)
	for word := range wordSet(content) {
		if querySet[word] {
			overlap++
		}
	}

	return float64(overlap) / float64(contentWords)
}

func countWords(s string) int {
	count := 0
	inWord := false
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			if !inWord {
				count++
				inWord = true
			}
		} else {
			inWord = false
		}
	}
	return count
}

func wordSet(s string) map[string]bool {
	words := make(map[string]bool)
	current := []rune{}
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			current = append(current, r)
		} else {
			if len(current) > 0 {
				words[string(current)] = true
				current = nil
			}
		}
	}
	if len(current) > 0 {
		words[string(current)] = true
	}
	return words
}

type PluginContextEngine struct {
	Name_      string
	Type_      string
	Entrypoint string
	Config     map[string]any
}

func (e *PluginContextEngine) Name() string { return e.Name_ }
func (e *PluginContextEngine) Type() string { return e.Type_ }

func (e *PluginContextEngine) Initialize(config map[string]any) error {
	e.Config = config
	return nil
}

func (e *PluginContextEngine) AddDocument(ctx context.Context, doc Document) error {
	return fmt.Errorf("not implemented: plugin context engine")
}

func (e *PluginContextEngine) Search(ctx context.Context, query string, options SearchOptions) ([]SearchResult, error) {
	return nil, fmt.Errorf("not implemented: plugin context engine")
}

func (e *PluginContextEngine) GetDocument(ctx context.Context, id string) (*Document, error) {
	return nil, fmt.Errorf("not implemented: plugin context engine")
}

func (e *PluginContextEngine) DeleteDocument(ctx context.Context, id string) error {
	return fmt.Errorf("not implemented: plugin context engine")
}

func (e *PluginContextEngine) Close() error {
	return nil
}

func RegisterPluginContextEngine(registry *ContextEngineRegistry, manifest map[string]any) error {
	name, _ := manifest["name"].(string)
	engineType, _ := manifest["type"].(string)

	if name == "" || engineType == "" {
		return fmt.Errorf("invalid manifest: missing name or type")
	}

	engine := &PluginContextEngine{
		Name_:      name,
		Type_:      engineType,
		Entrypoint: "",
	}

	return registry.Register(name, engine)
}

type ContextEngineConfig struct {
	Type       string         `json:"type"`
	Provider   string         `json:"provider"`
	Dimension  int            `json:"dimension"`
	IndexType  string         `json:"index_type"`
	Endpoint   string         `json:"endpoint"`
	APIKey     string         `json:"api_key"`
	Parameters map[string]any `json:"parameters"`
}

func ParseContextEngineConfig(data []byte) (*ContextEngineConfig, error) {
	var config ContextEngineConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse context engine config: %w", err)
	}
	return &config, nil
}
