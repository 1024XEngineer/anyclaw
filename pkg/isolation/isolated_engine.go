package isolation

import (
	"context"
	"fmt"
	"sync"
	"time"

	ctxpkg "github.com/1024XEngineer/anyclaw/pkg/runtime/context/store"
)

type IsolatedEngine struct {
	mu         sync.RWMutex
	scope      *ContextScope
	boundary   *ContextBoundary
	documents  map[string]*ctxpkg.Document
	config     IsolationConfig
	visibility ContextVisibility
	closed     bool
}

func NewIsolatedEngine(scope *ContextScope, boundary *ContextBoundary, config IsolationConfig) *IsolatedEngine {
	return &IsolatedEngine{
		scope:      scope,
		boundary:   boundary,
		documents:  make(map[string]*ctxpkg.Document),
		config:     config,
		visibility: boundary.Visibility,
	}
}

func (e *IsolatedEngine) Name() string {
	return fmt.Sprintf("isolated-%s", e.scope.ID())
}

func (e *IsolatedEngine) Type() string {
	return "isolated"
}

func (e *IsolatedEngine) Initialize(config map[string]any) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if v, ok := config["vector_size"].(float64); ok {
		_ = int(v)
	}
	return nil
}

func (e *IsolatedEngine) AddDocument(ctx context.Context, doc ctxpkg.Document) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return fmt.Errorf("engine is closed")
	}

	if len(e.documents) >= e.config.MaxContextSize {
		return fmt.Errorf("context size limit reached: %d", e.config.MaxContextSize)
	}

	if doc.Metadata == nil {
		doc.Metadata = make(map[string]any)
	}
	doc.Metadata["agent_id"] = e.scope.AgentID
	doc.Metadata["session_id"] = e.scope.SessionID
	doc.Metadata["task_id"] = e.scope.TaskID
	doc.Metadata["namespace"] = e.scope.Namespace
	doc.Metadata["scope_id"] = e.scope.ID()

	now := time.Now()
	doc.CreatedAt = now
	doc.UpdatedAt = now

	e.documents[doc.ID] = &doc
	return nil
}

func (e *IsolatedEngine) Search(ctx context.Context, query string, options ctxpkg.SearchOptions) ([]ctxpkg.SearchResult, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.closed {
		return nil, fmt.Errorf("engine is closed")
	}

	var results []ctxpkg.SearchResult

	for _, doc := range e.documents {
		if len(options.Filters) > 0 {
			if !matchesFilters(doc, options.Filters) {
				continue
			}
		}

		score := calculateSimilarity(query, doc.Content)
		if score >= options.Threshold {
			results = append(results, ctxpkg.SearchResult{
				Document: doc,
				Score:    score,
			})
		}
	}

	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	if len(results) > options.TopK {
		results = results[:options.TopK]
	}

	return results, nil
}

func (e *IsolatedEngine) GetDocument(ctx context.Context, id string) (*ctxpkg.Document, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if e.closed {
		return nil, fmt.Errorf("engine is closed")
	}

	doc, ok := e.documents[id]
	if !ok {
		return nil, fmt.Errorf("document not found: %s", id)
	}
	return doc, nil
}

func (e *IsolatedEngine) DeleteDocument(ctx context.Context, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return fmt.Errorf("engine is closed")
	}

	if _, ok := e.documents[id]; !ok {
		return fmt.Errorf("document not found: %s", id)
	}
	delete(e.documents, id)
	return nil
}

func (e *IsolatedEngine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.closed = true
	e.documents = make(map[string]*ctxpkg.Document)
	return nil
}

func (e *IsolatedEngine) Scope() *ContextScope {
	return e.scope
}

func (e *IsolatedEngine) Boundary() *ContextBoundary {
	return e.boundary
}

func (e *IsolatedEngine) SetVisibility(visibility ContextVisibility) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.visibility = visibility
	e.boundary.Visibility = visibility
}

func (e *IsolatedEngine) Visibility() ContextVisibility {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.visibility
}

func (e *IsolatedEngine) Clone() IsolatedContextEngine {
	e.mu.RLock()
	defer e.mu.RUnlock()

	newScope := &ContextScope{
		AgentID:   e.scope.AgentID,
		SessionID: e.scope.SessionID,
		TaskID:    e.scope.TaskID,
		Namespace: e.scope.Namespace,
		Labels:    make(map[string]string),
		CreatedAt: time.Now(),
	}
	for k, v := range e.scope.Labels {
		newScope.Labels[k] = v
	}

	newBoundary := &ContextBoundary{
		Scope:      newScope,
		Mode:       e.boundary.Mode,
		Visibility: e.visibility,
		Parent:     e.boundary,
		Children:   make([]*ContextBoundary, 0),
	}

	cloned := NewIsolatedEngine(newScope, newBoundary, e.config)
	cloned.visibility = e.visibility

	for id, doc := range e.documents {
		clonedDoc := *doc
		clonedDoc.Metadata = make(map[string]any)
		for k, v := range doc.Metadata {
			clonedDoc.Metadata[k] = v
		}
		cloned.documents[id] = &clonedDoc
	}

	return cloned
}

func (e *IsolatedEngine) SnapshotDocuments() []ctxpkg.Document {
	e.mu.RLock()
	defer e.mu.RUnlock()

	docs := make([]ctxpkg.Document, 0, len(e.documents))
	for _, doc := range e.documents {
		docs = append(docs, *doc)
	}
	return docs
}

func (e *IsolatedEngine) DocumentCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.documents)
}

func (e *IsolatedEngine) GetDocumentsByNamespace(namespace string) []ctxpkg.Document {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var docs []ctxpkg.Document
	for _, doc := range e.documents {
		if ns, ok := doc.Metadata["namespace"].(string); ok && ns == namespace {
			docs = append(docs, *doc)
		}
	}
	return docs
}

func matchesFilters(doc *ctxpkg.Document, filters map[string]any) bool {
	for key, value := range filters {
		docValue, ok := doc.Metadata[key]
		if !ok {
			return false
		}
		if docValue != value {
			return false
		}
	}
	return true
}

func calculateSimilarity(query, content string) float64 {
	queryWords := wordSet(query)
	contentWords := wordSet(content)

	if len(contentWords) == 0 {
		return 0
	}

	overlap := 0
	for word := range queryWords {
		if contentWords[word] {
			overlap++
		}
	}

	return float64(overlap) / float64(len(contentWords))
}

func wordSet(s string) map[string]bool {
	words := make(map[string]bool)
	current := []rune{}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
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
