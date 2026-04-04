package isolation

import (
	"context"
	"fmt"
	"sync"

	ctxengine "github.com/anyclaw/anyclaw/pkg/context-engine"
)

type ContextScopeMiddleware struct {
	mu           sync.RWMutex
	manager      *ContextIsolationManager
	activeScopes map[string]*ActiveScope
	scopeStack   map[string][]string
}

type ActiveScope struct {
	ScopeID   string
	AgentID   string
	SessionID string
	EnteredAt int64
	Nested    int
}

func NewContextScopeMiddleware(manager *ContextIsolationManager) *ContextScopeMiddleware {
	return &ContextScopeMiddleware{
		manager:      manager,
		activeScopes: make(map[string]*ActiveScope),
		scopeStack:   make(map[string][]string),
	}
}

func (m *ContextScopeMiddleware) EnterScope(agentID, sessionID, taskID, namespace string, mode IsolationMode, visibility ContextVisibility) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	scope := &ContextScope{
		AgentID:   agentID,
		SessionID: sessionID,
		TaskID:    taskID,
		Namespace: namespace,
		Labels:    make(map[string]string),
	}

	boundary, err := m.manager.CreateBoundary(scope, mode, visibility)
	if err != nil {
		if existingBoundary, ok := m.manager.GetBoundary(scope.ID()); ok {
			m.scopeStack[agentID] = append(m.scopeStack[agentID], scope.ID())

			if activeScope, exists := m.activeScopes[agentID]; exists {
				activeScope.Nested++
			} else {
				m.activeScopes[agentID] = &ActiveScope{
					ScopeID:   scope.ID(),
					AgentID:   agentID,
					SessionID: sessionID,
					Nested:    1,
				}
			}

			return existingBoundary.Scope.ID(), nil
		}
		return "", fmt.Errorf("failed to create boundary: %w", err)
	}

	m.scopeStack[agentID] = append(m.scopeStack[agentID], scope.ID())

	if activeScope, exists := m.activeScopes[agentID]; exists {
		activeScope.ScopeID = scope.ID()
		activeScope.SessionID = sessionID
		activeScope.Nested++
	} else {
		m.activeScopes[agentID] = &ActiveScope{
			ScopeID:   scope.ID(),
			AgentID:   agentID,
			SessionID: sessionID,
			Nested:    1,
		}
	}

	_ = boundary
	return scope.ID(), nil
}

func (m *ContextScopeMiddleware) ExitScope(agentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	activeScope, exists := m.activeScopes[agentID]
	if !exists {
		return fmt.Errorf("no active scope for agent: %s", agentID)
	}

	if activeScope.Nested > 1 {
		activeScope.Nested--
		if len(m.scopeStack[agentID]) > 1 {
			m.scopeStack[agentID] = m.scopeStack[agentID][:len(m.scopeStack[agentID])-1]
			activeScope.ScopeID = m.scopeStack[agentID][len(m.scopeStack[agentID])-1]
		}
		return nil
	}

	if len(m.scopeStack[agentID]) > 0 {
		scopeID := m.scopeStack[agentID][len(m.scopeStack[agentID])-1]
		m.scopeStack[agentID] = m.scopeStack[agentID][:len(m.scopeStack[agentID])-1]
		delete(m.activeScopes, agentID)
		return m.manager.DeleteBoundary(scopeID)
	}

	delete(m.activeScopes, agentID)
	return nil
}

func (m *ContextScopeMiddleware) GetCurrentScope(agentID string) (*ContextScope, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	activeScope, exists := m.activeScopes[agentID]
	if !exists {
		return nil, fmt.Errorf("no active scope for agent: %s", agentID)
	}

	boundary, ok := m.manager.GetBoundary(activeScope.ScopeID)
	if !ok {
		return nil, fmt.Errorf("boundary not found for scope: %s", activeScope.ScopeID)
	}

	return boundary.Scope, nil
}

func (m *ContextScopeMiddleware) GetCurrentEngine(agentID string) (*IsolatedEngine, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	activeScope, exists := m.activeScopes[agentID]
	if !exists {
		return nil, fmt.Errorf("no active scope for agent: %s", agentID)
	}

	engine, ok := m.manager.GetEngine(activeScope.ScopeID)
	if !ok {
		return nil, fmt.Errorf("engine not found for scope: %s", activeScope.ScopeID)
	}

	return engine, nil
}

func (m *ContextScopeMiddleware) GetCurrentKVEngine(agentID string) (*ctxengine.Engine, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	activeScope, exists := m.activeScopes[agentID]
	if !exists {
		return nil, fmt.Errorf("no active scope for agent: %s", agentID)
	}

	kvEngine, ok := m.manager.GetKVEngine(activeScope.ScopeID)
	if !ok {
		return nil, fmt.Errorf("KV engine not found for scope: %s", activeScope.ScopeID)
	}

	return kvEngine, nil
}

func (m *ContextScopeMiddleware) WithScope(ctx context.Context, agentID, sessionID, taskID, namespace string, mode IsolationMode, visibility ContextVisibility, fn func(scopeID string) error) error {
	scopeID, err := m.EnterScope(agentID, sessionID, taskID, namespace, mode, visibility)
	if err != nil {
		return err
	}

	defer func() {
		_ = m.ExitScope(agentID)
	}()

	return fn(scopeID)
}

func (m *ContextScopeMiddleware) GetScopeStack(agentID string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if stack, ok := m.scopeStack[agentID]; ok {
		result := make([]string, len(stack))
		copy(result, stack)
		return result
	}
	return nil
}

func (m *ContextScopeMiddleware) IsScoped(agentID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, exists := m.activeScopes[agentID]
	return exists
}

func (m *ContextScopeMiddleware) ListActiveScopes() []*ActiveScope {
	m.mu.RLock()
	defer m.mu.RUnlock()

	scopes := make([]*ActiveScope, 0, len(m.activeScopes))
	for _, scope := range m.activeScopes {
		scopes = append(scopes, scope)
	}
	return scopes
}

type ContextEnforcer struct {
	mu         sync.RWMutex
	middleware *ContextScopeMiddleware
	rules      map[string]*EnforcementRule
}

type EnforcementRule struct {
	AgentID            string
	RequiredMode       IsolationMode
	RequiredVisibility ContextVisibility
	AllowEscalation    bool
	MaxNestedDepth     int
}

func NewContextEnforcer(middleware *ContextScopeMiddleware) *ContextEnforcer {
	return &ContextEnforcer{
		middleware: middleware,
		rules:      make(map[string]*EnforcementRule),
	}
}

func (e *ContextEnforcer) AddRule(agentID string, rule *EnforcementRule) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rules[agentID] = rule
}

func (e *ContextEnforcer) RemoveRule(agentID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.rules, agentID)
}

func (e *ContextEnforcer) Enforce(agentID string) error {
	e.mu.RLock()
	rule, exists := e.rules[agentID]
	e.mu.RUnlock()

	if !exists {
		return nil
	}

	if !e.middleware.IsScoped(agentID) {
		return fmt.Errorf("agent %s must operate within a context scope", agentID)
	}

	currentScope, err := e.middleware.GetCurrentScope(agentID)
	if err != nil {
		return fmt.Errorf("failed to get current scope: %w", err)
	}

	boundary, ok := e.middleware.manager.GetBoundary(currentScope.ID())
	if !ok {
		return fmt.Errorf("boundary not found for scope: %s", currentScope.ID())
	}

	if rule.RequiredMode != "" && boundary.Mode != rule.RequiredMode {
		return fmt.Errorf("agent %s requires isolation mode %s, but current mode is %s", agentID, rule.RequiredMode, boundary.Mode)
	}

	if rule.RequiredVisibility != "" && boundary.Visibility != rule.RequiredVisibility {
		return fmt.Errorf("agent %s requires visibility %s, but current visibility is %s", agentID, rule.RequiredVisibility, boundary.Visibility)
	}

	stack := e.middleware.GetScopeStack(agentID)
	if rule.MaxNestedDepth > 0 && len(stack) > rule.MaxNestedDepth {
		return fmt.Errorf("agent %s exceeded max nested depth: %d > %d", agentID, len(stack), rule.MaxNestedDepth)
	}

	return nil
}

func (e *ContextEnforcer) EnforceWithFallback(agentID string, fallbackMode IsolationMode, fallbackVisibility ContextVisibility) error {
	err := e.Enforce(agentID)
	if err != nil {
		rule, exists := e.rules[agentID]
		if exists && rule.AllowEscalation {
			currentScope, scopeErr := e.middleware.GetCurrentScope(agentID)
			if scopeErr == nil {
				boundary, ok := e.middleware.manager.GetBoundary(currentScope.ID())
				if ok {
					boundary.Mode = fallbackMode
					boundary.Visibility = fallbackVisibility
					return nil
				}
			}
		}
		return err
	}
	return nil
}

func (e *ContextEnforcer) ListRules() map[string]*EnforcementRule {
	e.mu.RLock()
	defer e.mu.RUnlock()

	rules := make(map[string]*EnforcementRule)
	for k, v := range e.rules {
		rules[k] = v
	}
	return rules
}
