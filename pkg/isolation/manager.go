package isolation

import (
	"fmt"
	"sync"
	"time"

	ctxpkg "github.com/anyclaw/anyclaw/pkg/context"
	ctxengine "github.com/anyclaw/anyclaw/pkg/context-engine"
)

var snapshotCounter uint64

type ContextIsolationManager struct {
	mu            sync.RWMutex
	boundaries    map[string]*ContextBoundary
	engines       map[string]*IsolatedEngine
	policies      []*SharedContextPolicy
	snapshots     map[string]*ContextSnapshot
	config        IsolationConfig
	kvEngines     map[string]*ctxengine.Engine
	globalKV      *ctxengine.Engine
	cleanupTicker *time.Ticker
	stopCh        chan struct{}
}

func NewContextIsolationManager(config IsolationConfig) *ContextIsolationManager {
	mgr := &ContextIsolationManager{
		boundaries: make(map[string]*ContextBoundary),
		engines:    make(map[string]*IsolatedEngine),
		policies:   make([]*SharedContextPolicy, 0),
		snapshots:  make(map[string]*ContextSnapshot),
		config:     config,
		kvEngines:  make(map[string]*ctxengine.Engine),
		globalKV: ctxengine.New(ctxengine.ContextConfig{
			MaxAge:          config.DefaultTTL,
			MaxSize:         config.MaxContextSize,
			AutoExpire:      true,
			CleanupInterval: config.CleanupInterval,
		}),
		stopCh: make(chan struct{}),
	}

	if config.CleanupInterval > 0 {
		mgr.startCleanup()
	}

	return mgr
}

func (m *ContextIsolationManager) CreateBoundary(scope *ContextScope, mode IsolationMode, visibility ContextVisibility) (*ContextBoundary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if scope == nil {
		return nil, fmt.Errorf("scope cannot be nil")
	}

	scopeID := scope.ID()
	if scopeID == "" {
		return nil, fmt.Errorf("scope must have at least one of AgentID, SessionID, or TaskID")
	}

	if _, exists := m.boundaries[scopeID]; exists {
		return nil, fmt.Errorf("boundary already exists for scope: %s", scopeID)
	}

	if mode == "" {
		mode = m.config.DefaultMode
	}
	if visibility == "" {
		visibility = m.config.DefaultVisibility
	}

	if scope.CreatedAt.IsZero() {
		scope.CreatedAt = time.Now()
	}
	if !scope.ExpiresAt.IsZero() && scope.ExpiresAt.Before(time.Now()) {
		return nil, fmt.Errorf("scope expiration must be in the future")
	}

	boundary := &ContextBoundary{
		Scope:      scope,
		Mode:       mode,
		Visibility: visibility,
		Children:   make([]*ContextBoundary, 0),
	}

	m.boundaries[scopeID] = boundary

	engine := NewIsolatedEngine(scope, boundary, m.config)
	m.engines[scopeID] = engine

	if m.config.EnableSharing {
		m.kvEngines[scopeID] = ctxengine.New(ctxengine.ContextConfig{
			MaxAge:          m.config.DefaultTTL,
			MaxSize:         m.config.MaxContextSize,
			AutoExpire:      true,
			CleanupInterval: m.config.CleanupInterval,
		})
	}

	return boundary, nil
}

func (m *ContextIsolationManager) GetBoundary(scopeID string) (*ContextBoundary, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	boundary, ok := m.boundaries[scopeID]
	return boundary, ok
}

func (m *ContextIsolationManager) GetEngine(scopeID string) (*IsolatedEngine, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	engine, ok := m.engines[scopeID]
	return engine, ok
}

func (m *ContextIsolationManager) GetKVEngine(scopeID string) (*ctxengine.Engine, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	engine, ok := m.kvEngines[scopeID]
	return engine, ok
}

func (m *ContextIsolationManager) GetGlobalKVEngine() *ctxengine.Engine {
	return m.globalKV
}

func (m *ContextIsolationManager) CreateChildBoundary(parentScopeID string, childScope *ContextScope, mode IsolationMode, visibility ContextVisibility) (*ContextBoundary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	parent, exists := m.boundaries[parentScopeID]
	if !exists {
		return nil, fmt.Errorf("parent boundary not found: %s", parentScopeID)
	}

	childScopeID := childScope.ID()
	if childScopeID == "" {
		return nil, fmt.Errorf("child scope must have at least one of AgentID, SessionID, or TaskID")
	}

	if _, exists := m.boundaries[childScopeID]; exists {
		return nil, fmt.Errorf("boundary already exists for scope: %s", childScopeID)
	}

	if mode == "" {
		mode = m.config.DefaultMode
	}
	if visibility == "" {
		visibility = m.config.DefaultVisibility
	}

	if childScope.CreatedAt.IsZero() {
		childScope.CreatedAt = time.Now()
	}

	childBoundary := &ContextBoundary{
		Scope:      childScope,
		Mode:       mode,
		Visibility: visibility,
		Parent:     parent,
		Children:   make([]*ContextBoundary, 0),
	}

	m.boundaries[childScopeID] = childBoundary
	parent.Children = append(parent.Children, childBoundary)

	engine := NewIsolatedEngine(childScope, childBoundary, m.config)
	m.engines[childScopeID] = engine

	if m.config.EnableSharing {
		m.kvEngines[childScopeID] = ctxengine.New(ctxengine.ContextConfig{
			MaxAge:          m.config.DefaultTTL,
			MaxSize:         m.config.MaxContextSize,
			AutoExpire:      true,
			CleanupInterval: m.config.CleanupInterval,
		})
	}

	return childBoundary, nil
}

func (m *ContextIsolationManager) AddSharingPolicy(policy *SharedContextPolicy) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if policy.SourceAgentID == "" {
		return fmt.Errorf("source agent ID cannot be empty")
	}
	if len(policy.TargetAgentIDs) == 0 {
		return fmt.Errorf("at least one target agent ID is required")
	}

	if policy.ExpiresAt.IsZero() {
		policy.ExpiresAt = time.Now().Add(m.config.DefaultTTL)
	}

	m.policies = append(m.policies, policy)
	return nil
}

func (m *ContextIsolationManager) RemoveSharingPolicy(sourceAgentID string, targetAgentID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, policy := range m.policies {
		if policy.SourceAgentID == sourceAgentID {
			for j, target := range policy.TargetAgentIDs {
				if target == targetAgentID {
					policy.TargetAgentIDs = append(policy.TargetAgentIDs[:j], policy.TargetAgentIDs[j+1:]...)
					if len(policy.TargetAgentIDs) == 0 {
						m.policies = append(m.policies[:i], m.policies[i+1:]...)
					}
					return true
				}
			}
		}
	}
	return false
}

func (m *ContextIsolationManager) ListSharingPolicies() []*SharedContextPolicy {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*SharedContextPolicy, len(m.policies))
	copy(result, m.policies)
	return result
}

func (m *ContextIsolationManager) CanShareContext(sourceAgentID, targetAgentID, namespace string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	for _, policy := range m.policies {
		if policy.ExpiresAt.Before(now) {
			continue
		}
		if policy.SourceAgentID != sourceAgentID {
			continue
		}

		targetMatch := false
		for _, target := range policy.TargetAgentIDs {
			if target == targetAgentID {
				targetMatch = true
				break
			}
		}
		if !targetMatch {
			continue
		}

		if policy.Namespace != "" && policy.Namespace != namespace {
			continue
		}

		return true
	}
	return false
}

func (m *ContextIsolationManager) CreateSnapshot(agentID, sessionID, description string) (*ContextSnapshot, error) {
	if !m.config.EnableSnapshots {
		return nil, fmt.Errorf("snapshots are disabled")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.snapshots) >= m.config.MaxSnapshots {
		oldestID := ""
		oldestTime := time.Now()
		for id, snap := range m.snapshots {
			if snap.TakenAt.Before(oldestTime) {
				oldestTime = snap.TakenAt
				oldestID = id
			}
		}
		if oldestID != "" {
			delete(m.snapshots, oldestID)
		}
	}

	snapshotCounter++
	snapshotID := fmt.Sprintf("snap_%s_%d_%d", agentID, time.Now().UnixNano(), snapshotCounter)

	var docs []ctxpkg.Document
	var kvs map[string]any

	for scopeID, engine := range m.engines {
		boundary, ok := m.boundaries[scopeID]
		if !ok {
			continue
		}
		if boundary.Scope.AgentID == agentID && (sessionID == "" || boundary.Scope.SessionID == sessionID) {
			docs = engine.SnapshotDocuments()
			break
		}
	}
	for scopeID, kvEngine := range m.kvEngines {
		boundary, ok := m.boundaries[scopeID]
		if !ok {
			continue
		}
		if boundary.Scope.AgentID == agentID && (sessionID == "" || boundary.Scope.SessionID == sessionID) {
			kvs = make(map[string]any)
			for _, ctx := range kvEngine.List() {
				kvs[ctx.Key] = ctx.Value
			}
			break
		}
	}

	snapshot := &ContextSnapshot{
		ID:          snapshotID,
		AgentID:     agentID,
		SessionID:   sessionID,
		Documents:   docs,
		KeyValues:   kvs,
		TakenAt:     time.Now(),
		Description: description,
	}

	m.snapshots[snapshotID] = snapshot
	return snapshot, nil
}

func (m *ContextIsolationManager) GetSnapshot(snapshotID string) (*ContextSnapshot, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	snapshot, ok := m.snapshots[snapshotID]
	return snapshot, ok
}

func (m *ContextIsolationManager) ListSnapshots(agentID string) []*ContextSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*ContextSnapshot
	for _, snap := range m.snapshots {
		if agentID == "" || snap.AgentID == agentID {
			result = append(result, snap)
		}
	}
	return result
}

func (m *ContextIsolationManager) DeleteBoundary(scopeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	boundary, exists := m.boundaries[scopeID]
	if !exists {
		return fmt.Errorf("boundary not found: %s", scopeID)
	}

	if len(boundary.Children) > 0 {
		for _, child := range boundary.Children {
			childID := child.Scope.ID()
			delete(m.boundaries, childID)
			delete(m.engines, childID)
			delete(m.kvEngines, childID)
		}
	}

	if boundary.Parent != nil {
		parent := boundary.Parent
		for i, child := range parent.Children {
			if child.Scope.ID() == scopeID {
				parent.Children = append(parent.Children[:i], parent.Children[i+1:]...)
				break
			}
		}
	}

	delete(m.boundaries, scopeID)

	if engine, ok := m.engines[scopeID]; ok {
		engine.Close()
		delete(m.engines, scopeID)
	}
	delete(m.kvEngines, scopeID)

	return nil
}

func (m *ContextIsolationManager) ListBoundaries() []*ContextBoundary {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*ContextBoundary, 0, len(m.boundaries))
	for _, boundary := range m.boundaries {
		result = append(result, boundary)
	}
	return result
}

func (m *ContextIsolationManager) ListEngines() map[string]*IsolatedEngine {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*IsolatedEngine, len(m.engines))
	for k, v := range m.engines {
		result[k] = v
	}
	return result
}

func (m *ContextIsolationManager) SharedSearch(scopeID string, query string, namespace string) ([]SharedSearchResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	boundary, exists := m.boundaries[scopeID]
	if !exists {
		return nil, fmt.Errorf("boundary not found: %s", scopeID)
	}

	targetAgentID := boundary.Scope.AgentID

	var results []SharedSearchResult

	if boundary.Mode == IsolationModeShared || boundary.Mode == IsolationModeHybrid {
		for sourceScopeID, kvEngine := range m.kvEngines {
			if sourceScopeID == scopeID {
				continue
			}

			sourceBoundary, exists := m.boundaries[sourceScopeID]
			if !exists {
				continue
			}

			sourceAgentID := sourceBoundary.Scope.AgentID

			if !m.canShareContextLocked(sourceAgentID, targetAgentID, namespace) {
				continue
			}

			if sourceBoundary.Visibility == ContextVisibilityPrivate {
				continue
			}

			for _, ctx := range kvEngine.List() {
				if namespace != "" && ctx.Metadata["namespace"] != namespace {
					continue
				}
				results = append(results, SharedSearchResult{
					SourceScopeID: sourceScopeID,
					Key:           ctx.Key,
					Value:         ctx.Value,
					Metadata:      ctx.Metadata,
				})
			}
		}
	}

	return results, nil
}

func (m *ContextIsolationManager) canShareContextLocked(sourceID, targetID, namespace string) bool {
	now := time.Now()
	for _, policy := range m.policies {
		if policy.ExpiresAt.Before(now) {
			continue
		}
		if policy.SourceAgentID != sourceID {
			continue
		}

		targetMatch := false
		for _, target := range policy.TargetAgentIDs {
			if target == targetID {
				targetMatch = true
				break
			}
		}
		if !targetMatch {
			continue
		}

		if policy.Namespace != "" && policy.Namespace != namespace {
			continue
		}

		return true
	}
	return false
}

func (m *ContextIsolationManager) startCleanup() {
	m.cleanupTicker = time.NewTicker(m.config.CleanupInterval)
	go func() {
		for {
			select {
			case <-m.cleanupTicker.C:
				m.cleanup()
			case <-m.stopCh:
				return
			}
		}
	}()
}

func (m *ContextIsolationManager) cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	expired := make([]string, 0)

	for scopeID, boundary := range m.boundaries {
		if !boundary.Scope.ExpiresAt.IsZero() && boundary.Scope.ExpiresAt.Before(now) {
			expired = append(expired, scopeID)
		}
	}

	for _, scopeID := range expired {
		boundary := m.boundaries[scopeID]
		if len(boundary.Children) > 0 {
			for _, child := range boundary.Children {
				childID := child.Scope.ID()
				delete(m.boundaries, childID)
				delete(m.engines, childID)
				delete(m.kvEngines, childID)
			}
		}
		delete(m.boundaries, scopeID)
		if engine, ok := m.engines[scopeID]; ok {
			engine.Close()
			delete(m.engines, scopeID)
		}
		delete(m.kvEngines, scopeID)
	}

	expiredPolicies := make([]int, 0)
	for i, policy := range m.policies {
		if policy.ExpiresAt.Before(now) {
			expiredPolicies = append(expiredPolicies, i)
		}
	}
	for i := len(expiredPolicies) - 1; i >= 0; i-- {
		m.policies = append(m.policies[:expiredPolicies[i]], m.policies[expiredPolicies[i]+1:]...)
	}

	expiredSnapshots := make([]string, 0)
	for id, snap := range m.snapshots {
		if snap.TakenAt.Add(m.config.DefaultTTL * 2).Before(now) {
			expiredSnapshots = append(expiredSnapshots, id)
		}
	}
	for _, id := range expiredSnapshots {
		delete(m.snapshots, id)
	}
}

func (m *ContextIsolationManager) Close() error {
	if m.cleanupTicker != nil {
		m.cleanupTicker.Stop()
	}
	close(m.stopCh)

	m.mu.Lock()
	defer m.mu.Unlock()

	for scopeID, engine := range m.engines {
		engine.Close()
		delete(m.engines, scopeID)
	}

	m.boundaries = make(map[string]*ContextBoundary)
	m.kvEngines = make(map[string]*ctxengine.Engine)
	m.policies = make([]*SharedContextPolicy, 0)
	m.snapshots = make(map[string]*ContextSnapshot)

	return nil
}

type SharedSearchResult struct {
	SourceScopeID string
	Key           string
	Value         any
	Metadata      map[string]string
}
