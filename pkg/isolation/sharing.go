package isolation

import (
	"context"
	"fmt"
	"sync"
	"time"

	ctxpkg "github.com/1024XEngineer/anyclaw/pkg/runtime/context/store"
)

type ContextBridge struct {
	mu       sync.RWMutex
	manager  *ContextIsolationManager
	handlers map[string]BridgeHandler
}

type BridgeHandler func(sourceScopeID, targetScopeID string, data BridgeData) error

type BridgeData struct {
	Type      string
	Payload   any
	Metadata  map[string]string
	Timestamp time.Time
}

type ContextTransfer struct {
	ID            string
	SourceScopeID string
	TargetScopeID string
	Documents     []ctxpkg.Document
	KeyValues     map[string]any
	Namespace     string
	Status        TransferStatus
	CreatedAt     time.Time
	CompletedAt   time.Time
	Error         string
}

type TransferStatus string

const (
	TransferStatusPending   TransferStatus = "pending"
	TransferStatusApproved  TransferStatus = "approved"
	TransferStatusRejected  TransferStatus = "rejected"
	TransferStatusCompleted TransferStatus = "completed"
	TransferStatusFailed    TransferStatus = "failed"
)

func NewContextBridge(manager *ContextIsolationManager) *ContextBridge {
	return &ContextBridge{
		manager:  manager,
		handlers: make(map[string]BridgeHandler),
	}
}

func (b *ContextBridge) RegisterHandler(transferType string, handler BridgeHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[transferType] = handler
}

func (b *ContextBridge) TransferDocuments(ctx context.Context, sourceScopeID, targetScopeID string, docIDs []string, namespace string) (*ContextTransfer, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	sourceBoundary, sourceOk := b.manager.GetBoundary(sourceScopeID)
	targetBoundary, targetOk := b.manager.GetBoundary(targetScopeID)
	if !sourceOk || !targetOk {
		return nil, fmt.Errorf("source or target boundary not found")
	}

	if !b.manager.CanShareContext(sourceBoundary.Scope.AgentID, targetBoundary.Scope.AgentID, namespace) {
		return nil, fmt.Errorf("context sharing not allowed from %s to %s for namespace %s", sourceScopeID, targetScopeID, namespace)
	}

	sourceEngine, ok := b.manager.GetEngine(sourceScopeID)
	if !ok {
		return nil, fmt.Errorf("source engine not found: %s", sourceScopeID)
	}

	targetEngine, ok := b.manager.GetEngine(targetScopeID)
	if !ok {
		return nil, fmt.Errorf("target engine not found: %s", targetScopeID)
	}

	transfer := &ContextTransfer{
		ID:            fmt.Sprintf("xfer_%s_%s_%d", sourceScopeID, targetScopeID, time.Now().UnixNano()),
		SourceScopeID: sourceScopeID,
		TargetScopeID: targetScopeID,
		Namespace:     namespace,
		Status:        TransferStatusPending,
		CreatedAt:     time.Now(),
		Documents:     make([]ctxpkg.Document, 0),
		KeyValues:     make(map[string]any),
	}

	if sourceEngine.Visibility() == ContextVisibilityPrivate {
		transfer.Status = TransferStatusRejected
		transfer.Error = "source context is private"
		return transfer, nil
	}

	for _, docID := range docIDs {
		doc, err := sourceEngine.GetDocument(ctx, docID)
		if err != nil {
			continue
		}

		if ns, ok := doc.Metadata["namespace"].(string); namespace != "" && (!ok || ns != namespace) {
			continue
		}

		newDoc := *doc
		newDoc.ID = fmt.Sprintf("transferred_%s_%d", doc.ID, time.Now().UnixNano())
		newDoc.Metadata = make(map[string]any)
		for k, v := range doc.Metadata {
			newDoc.Metadata[k] = v
		}
		newDoc.Metadata["transferred_from"] = sourceScopeID
		newDoc.Metadata["transferred_at"] = time.Now().Format(time.RFC3339)

		if err := targetEngine.AddDocument(ctx, newDoc); err != nil {
			transfer.Status = TransferStatusFailed
			transfer.Error = fmt.Sprintf("failed to add document %s: %v", docID, err)
			return transfer, nil
		}

		transfer.Documents = append(transfer.Documents, newDoc)
	}

	transfer.Status = TransferStatusCompleted
	transfer.CompletedAt = time.Now()
	return transfer, nil
}

func (b *ContextBridge) TransferKeyValues(sourceScopeID, targetScopeID string, keys []string, namespace string) (*ContextTransfer, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	sourceBoundary, sourceOk := b.manager.GetBoundary(sourceScopeID)
	targetBoundary, targetOk := b.manager.GetBoundary(targetScopeID)
	if !sourceOk || !targetOk {
		return nil, fmt.Errorf("source or target boundary not found")
	}

	if !b.manager.CanShareContext(sourceBoundary.Scope.AgentID, targetBoundary.Scope.AgentID, namespace) {
		return nil, fmt.Errorf("context sharing not allowed from %s to %s for namespace %s", sourceScopeID, targetScopeID, namespace)
	}

	sourceKV, ok := b.manager.GetKVEngine(sourceScopeID)
	if !ok {
		return nil, fmt.Errorf("source KV engine not found: %s", sourceScopeID)
	}

	targetKV, ok := b.manager.GetKVEngine(targetScopeID)
	if !ok {
		return nil, fmt.Errorf("target KV engine not found: %s", targetScopeID)
	}

	transfer := &ContextTransfer{
		ID:            fmt.Sprintf("xfer_kv_%s_%s_%d", sourceScopeID, targetScopeID, time.Now().UnixNano()),
		SourceScopeID: sourceScopeID,
		TargetScopeID: targetScopeID,
		Namespace:     namespace,
		Status:        TransferStatusPending,
		CreatedAt:     time.Now(),
		KeyValues:     make(map[string]any),
	}

	if sourceBoundary.Visibility == ContextVisibilityPrivate {
		transfer.Status = TransferStatusRejected
		transfer.Error = "source context is private"
		return transfer, nil
	}

	ctx := context.Background()
	for _, key := range keys {
		value, err := sourceKV.Get(ctx, key)
		if err != nil {
			continue
		}

		newKey := fmt.Sprintf("shared_%s_%s", sourceScopeID, key)
		if err := targetKV.Set(ctx, newKey, value); err != nil {
			transfer.Status = TransferStatusFailed
			transfer.Error = fmt.Sprintf("failed to set key %s: %v", key, err)
			return transfer, nil
		}

		transfer.KeyValues[newKey] = value
	}

	transfer.Status = TransferStatusCompleted
	transfer.CompletedAt = time.Now()
	return transfer, nil
}

func (b *ContextBridge) ShareAllContext(sourceScopeID, targetScopeID string, namespace string) (*ContextTransfer, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	sourceBoundary, sourceOk := b.manager.GetBoundary(sourceScopeID)
	targetBoundary, targetOk := b.manager.GetBoundary(targetScopeID)
	if !sourceOk || !targetOk {
		return nil, fmt.Errorf("source or target boundary not found")
	}

	if !b.manager.CanShareContext(sourceBoundary.Scope.AgentID, targetBoundary.Scope.AgentID, namespace) {
		return nil, fmt.Errorf("context sharing not allowed from %s to %s for namespace %s", sourceScopeID, targetScopeID, namespace)
	}

	sourceEngine, ok := b.manager.GetEngine(sourceScopeID)
	if !ok {
		return nil, fmt.Errorf("source engine not found: %s", sourceScopeID)
	}

	targetEngine, ok := b.manager.GetEngine(targetScopeID)
	if !ok {
		return nil, fmt.Errorf("target engine not found: %s", targetScopeID)
	}

	transfer := &ContextTransfer{
		ID:            fmt.Sprintf("xfer_all_%s_%s_%d", sourceScopeID, targetScopeID, time.Now().UnixNano()),
		SourceScopeID: sourceScopeID,
		TargetScopeID: targetScopeID,
		Namespace:     namespace,
		Status:        TransferStatusPending,
		CreatedAt:     time.Now(),
		Documents:     make([]ctxpkg.Document, 0),
		KeyValues:     make(map[string]any),
	}

	if sourceBoundary.Visibility == ContextVisibilityPrivate {
		transfer.Status = TransferStatusRejected
		transfer.Error = "source context is private"
		return transfer, nil
	}

	docs := sourceEngine.SnapshotDocuments()
	ctx := context.Background()

	for _, doc := range docs {
		if namespace != "" {
			if ns, ok := doc.Metadata["namespace"].(string); !ok || ns != namespace {
				continue
			}
		}

		newDoc := doc
		newDoc.ID = fmt.Sprintf("shared_%s_%d", doc.ID, time.Now().UnixNano())
		newDoc.Metadata = make(map[string]any)
		for k, v := range doc.Metadata {
			newDoc.Metadata[k] = v
		}
		newDoc.Metadata["shared_from"] = sourceScopeID
		newDoc.Metadata["shared_at"] = time.Now().Format(time.RFC3339)

		if err := targetEngine.AddDocument(ctx, newDoc); err != nil {
			transfer.Status = TransferStatusFailed
			transfer.Error = fmt.Sprintf("failed to add document: %v", err)
			return transfer, nil
		}

		transfer.Documents = append(transfer.Documents, newDoc)
	}

	if sourceKV, ok := b.manager.GetKVEngine(sourceScopeID); ok {
		if targetKV, ok := b.manager.GetKVEngine(targetScopeID); ok {
			for _, kvCtx := range sourceKV.List() {
				if namespace != "" && kvCtx.Metadata["namespace"] != namespace {
					continue
				}
				newKey := fmt.Sprintf("shared_%s_%s", sourceScopeID, kvCtx.Key)
				targetKV.Set(ctx, newKey, kvCtx.Value)
				transfer.KeyValues[newKey] = kvCtx.Value
			}
		}
	}

	transfer.Status = TransferStatusCompleted
	transfer.CompletedAt = time.Now()
	return transfer, nil
}

func (b *ContextBridge) GetTransferHistory(sourceScopeID, targetScopeID string) []*ContextTransfer {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var transfers []*ContextTransfer
	for _, engine := range b.manager.ListEngines() {
		_ = engine
	}
	return transfers
}

func (b *ContextBridge) NotifyTransfer(sourceScopeID, targetScopeID string, data BridgeData) error {
	b.mu.RLock()
	handler, ok := b.handlers[data.Type]
	b.mu.RUnlock()

	if !ok {
		return fmt.Errorf("no handler registered for transfer type: %s", data.Type)
	}

	return handler(sourceScopeID, targetScopeID, data)
}

type ContextAggregator struct {
	mu      sync.RWMutex
	manager *ContextIsolationManager
}

func NewContextAggregator(manager *ContextIsolationManager) *ContextAggregator {
	return &ContextAggregator{
		manager: manager,
	}
}

func (a *ContextAggregator) AggregateDocuments(scopeIDs []string, namespace string) ([]ctxpkg.Document, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var allDocs []ctxpkg.Document

	for _, scopeID := range scopeIDs {
		engine, ok := a.manager.GetEngine(scopeID)
		if !ok {
			continue
		}

		boundary, ok := a.manager.GetBoundary(scopeID)
		if !ok {
			continue
		}

		if boundary.Visibility == ContextVisibilityPrivate {
			continue
		}

		if boundary.Mode == IsolationModeStrict {
			continue
		}

		docs := engine.GetDocumentsByNamespace(namespace)
		allDocs = append(allDocs, docs...)

		if namespace == "" {
			docs = engine.SnapshotDocuments()
			for _, doc := range docs {
				alreadyExists := false
				for _, existing := range allDocs {
					if existing.ID == doc.ID {
						alreadyExists = true
						break
					}
				}
				if !alreadyExists {
					allDocs = append(allDocs, doc)
				}
			}
		}
	}

	return allDocs, nil
}

func (a *ContextAggregator) AggregateKeyValues(scopeIDs []string, namespace string) (map[string]any, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	aggregated := make(map[string]any)

	for _, scopeID := range scopeIDs {
		kvEngine, ok := a.manager.GetKVEngine(scopeID)
		if !ok {
			continue
		}

		boundary, ok := a.manager.GetBoundary(scopeID)
		if !ok {
			continue
		}

		if boundary.Visibility == ContextVisibilityPrivate {
			continue
		}

		if boundary.Mode == IsolationModeStrict {
			continue
		}

		for _, kvCtx := range kvEngine.List() {
			if namespace != "" && kvCtx.Metadata["namespace"] != namespace {
				continue
			}

			aggregatedKey := fmt.Sprintf("%s:%s", scopeID, kvCtx.Key)
			aggregated[aggregatedKey] = kvCtx.Value
		}
	}

	return aggregated, nil
}

func (a *ContextAggregator) CrossScopeSearch(scopeIDs []string, query string, namespace string, topK int) ([]ctxpkg.SearchResult, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var allResults []ctxpkg.SearchResult
	ctx := context.Background()

	for _, scopeID := range scopeIDs {
		engine, ok := a.manager.GetEngine(scopeID)
		if !ok {
			continue
		}

		boundary, ok := a.manager.GetBoundary(scopeID)
		if !ok {
			continue
		}

		if boundary.Visibility == ContextVisibilityPrivate {
			continue
		}

		results, err := engine.Search(ctx, query, ctxpkg.SearchOptions{
			TopK:      topK,
			Threshold: 0.1,
			Filters:   map[string]any{"namespace": namespace},
		})
		if err != nil {
			continue
		}

		allResults = append(allResults, results...)
	}

	for i := 0; i < len(allResults)-1; i++ {
		for j := i + 1; j < len(allResults); j++ {
			if allResults[j].Score > allResults[i].Score {
				allResults[i], allResults[j] = allResults[j], allResults[i]
			}
		}
	}

	if len(allResults) > topK {
		allResults = allResults[:topK]
	}

	return allResults, nil
}

type ContextSync struct {
	mu      sync.RWMutex
	manager *ContextIsolationManager
	syncing map[string]bool
}

func NewContextSync(manager *ContextIsolationManager) *ContextSync {
	return &ContextSync{
		manager: manager,
		syncing: make(map[string]bool),
	}
}

func (s *ContextSync) SyncContext(sourceScopeID, targetScopeID string, namespace string) error {
	s.mu.Lock()
	syncKey := fmt.Sprintf("%s:%s:%s", sourceScopeID, targetScopeID, namespace)
	if s.syncing[syncKey] {
		s.mu.Unlock()
		return fmt.Errorf("sync already in progress for %s", syncKey)
	}
	s.syncing[syncKey] = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.syncing, syncKey)
		s.mu.Unlock()
	}()

	sourceBoundary, sourceOk := s.manager.GetBoundary(sourceScopeID)
	targetBoundary, targetOk := s.manager.GetBoundary(targetScopeID)
	if !sourceOk || !targetOk {
		return fmt.Errorf("source or target boundary not found")
	}

	if !s.manager.CanShareContext(sourceBoundary.Scope.AgentID, targetBoundary.Scope.AgentID, namespace) {
		return fmt.Errorf("context sharing not allowed from %s to %s for namespace %s", sourceScopeID, targetScopeID, namespace)
	}

	sourceEngine, ok := s.manager.GetEngine(sourceScopeID)
	if !ok {
		return fmt.Errorf("source engine not found: %s", sourceScopeID)
	}

	targetEngine, ok := s.manager.GetEngine(targetScopeID)
	if !ok {
		return fmt.Errorf("target engine not found: %s", targetScopeID)
	}

	docs := sourceEngine.SnapshotDocuments()
	ctx := context.Background()

	for _, doc := range docs {
		if namespace != "" {
			if ns, ok := doc.Metadata["namespace"].(string); !ok || ns != namespace {
				continue
			}
		}

		if _, err := targetEngine.GetDocument(ctx, doc.ID); err == nil {
			continue
		}

		newDoc := doc
		newDoc.Metadata = make(map[string]any)
		for k, v := range doc.Metadata {
			newDoc.Metadata[k] = v
		}
		newDoc.Metadata["synced_from"] = sourceScopeID
		newDoc.Metadata["synced_at"] = time.Now().Format(time.RFC3339)

		targetEngine.AddDocument(ctx, newDoc)
	}

	if sourceKV, ok := s.manager.GetKVEngine(sourceScopeID); ok {
		if targetKV, ok := s.manager.GetKVEngine(targetScopeID); ok {
			for _, kvCtx := range sourceKV.List() {
				if namespace != "" && kvCtx.Metadata["namespace"] != namespace {
					continue
				}

				_, err := targetKV.Get(ctx, kvCtx.Key)
				if err == nil {
					continue
				}

				targetKV.Set(ctx, kvCtx.Key, kvCtx.Value)
			}
		}
	}

	return nil
}

func (s *ContextSync) IsSyncing(sourceScopeID, targetScopeID, namespace string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	syncKey := fmt.Sprintf("%s:%s:%s", sourceScopeID, targetScopeID, namespace)
	return s.syncing[syncKey]
}

func (s *ContextSync) SyncAllPairs(scopeIDs []string, namespace string) map[string]error {
	results := make(map[string]error)

	for i, sourceID := range scopeIDs {
		for j, targetID := range scopeIDs {
			if i == j {
				continue
			}

			pairKey := fmt.Sprintf("%s->%s", sourceID, targetID)
			if err := s.SyncContext(sourceID, targetID, namespace); err != nil {
				results[pairKey] = err
			} else {
				results[pairKey] = nil
			}
		}
	}

	return results
}
