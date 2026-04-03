package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ManagedAgent represents a managed sub-agent with lifecycle tracking
type ManagedAgent struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Status      string            `json:"status"`
	Depth       int               `json:"depth"`
	ParentID    string            `json:"parent_id,omitempty"`
	MaxDepth    int               `json:"max_depth"`
	CreatedAt   time.Time         `json:"created_at"`
	StartedAt   *time.Time        `json:"started_at,omitempty"`
	CompletedAt *time.Time        `json:"completed_at,omitempty"`
	Result      string            `json:"result,omitempty"`
	Error       string            `json:"error,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// AgentLifecycle manages the lifecycle of sub-agents
type AgentLifecycle struct {
	mu       sync.RWMutex
	agents   map[string]*ManagedAgent
	children map[string][]string
	maxDepth int
	maxTotal int
}

// NewAgentLifecycle creates a new agent lifecycle manager
func NewAgentLifecycle(maxDepth, maxTotal int) *AgentLifecycle {
	if maxDepth <= 0 {
		maxDepth = 5
	}
	if maxTotal <= 0 {
		maxTotal = 50
	}
	return &AgentLifecycle{
		agents:   make(map[string]*ManagedAgent),
		children: make(map[string][]string),
		maxDepth: maxDepth,
		maxTotal: maxTotal,
	}
}

// Spawn creates a new sub-agent
func (al *AgentLifecycle) Spawn(name string, parentID string, metadata map[string]string) (*ManagedAgent, error) {
	al.mu.Lock()
	defer al.mu.Unlock()

	if len(al.agents) >= al.maxTotal {
		return nil, fmt.Errorf("maximum sub-agent limit reached (%d)", al.maxTotal)
	}

	depth := 0
	if parentID != "" {
		parent, ok := al.agents[parentID]
		if !ok {
			return nil, fmt.Errorf("parent agent not found: %s", parentID)
		}
		depth = parent.Depth + 1
		if depth > al.maxDepth {
			return nil, fmt.Errorf("maximum spawn depth exceeded (%d)", al.maxDepth)
		}
	}

	agent := &ManagedAgent{
		ID:        fmt.Sprintf("sa-%d", time.Now().UnixNano()),
		Name:      name,
		Status:    "idle",
		Depth:     depth,
		ParentID:  parentID,
		MaxDepth:  al.maxDepth,
		CreatedAt: time.Now(),
		Metadata:  metadata,
	}

	al.agents[agent.ID] = agent
	if parentID != "" {
		al.children[parentID] = append(al.children[parentID], agent.ID)
	}
	return agent, nil
}

// Get retrieves a sub-agent by ID
func (al *AgentLifecycle) Get(id string) (*ManagedAgent, bool) {
	al.mu.RLock()
	defer al.mu.RUnlock()
	agent, ok := al.agents[id]
	return agent, ok
}

// List returns all sub-agents
func (al *AgentLifecycle) List() []*ManagedAgent {
	al.mu.RLock()
	defer al.mu.RUnlock()
	var list []*ManagedAgent
	for _, a := range al.agents {
		list = append(list, a)
	}
	return list
}

// ListByParent returns child agents of a parent
func (al *AgentLifecycle) ListByParent(parentID string) []*ManagedAgent {
	al.mu.RLock()
	defer al.mu.RUnlock()
	childIDs := al.children[parentID]
	var list []*ManagedAgent
	for _, id := range childIDs {
		if agent, ok := al.agents[id]; ok {
			list = append(list, agent)
		}
	}
	return list
}

// Start marks a sub-agent as running
func (al *AgentLifecycle) Start(id string) error {
	al.mu.Lock()
	defer al.mu.Unlock()
	agent, ok := al.agents[id]
	if !ok {
		return fmt.Errorf("agent not found: %s", id)
	}
	if agent.Status != "idle" {
		return fmt.Errorf("agent is not idle: %s (status: %s)", id, agent.Status)
	}
	agent.Status = "running"
	now := time.Now()
	agent.StartedAt = &now
	return nil
}

// Complete marks a sub-agent as completed
func (al *AgentLifecycle) Complete(id string, result string) error {
	al.mu.Lock()
	defer al.mu.Unlock()
	agent, ok := al.agents[id]
	if !ok {
		return fmt.Errorf("agent not found: %s", id)
	}
	agent.Status = "completed"
	agent.Result = result
	now := time.Now()
	agent.CompletedAt = &now
	return nil
}

// Fail marks a sub-agent as failed
func (al *AgentLifecycle) Fail(id string, errMsg string) error {
	al.mu.Lock()
	defer al.mu.Unlock()
	agent, ok := al.agents[id]
	if !ok {
		return fmt.Errorf("agent not found: %s", id)
	}
	agent.Status = "failed"
	agent.Error = errMsg
	now := time.Now()
	agent.CompletedAt = &now
	return nil
}

// Cancel marks a sub-agent as cancelled
func (al *AgentLifecycle) Cancel(id string) error {
	al.mu.Lock()
	defer al.mu.Unlock()
	agent, ok := al.agents[id]
	if !ok {
		return fmt.Errorf("agent not found: %s", id)
	}
	if agent.Status == "completed" || agent.Status == "failed" {
		return fmt.Errorf("agent already finished: %s", id)
	}
	agent.Status = "cancelled"
	now := time.Now()
	agent.CompletedAt = &now
	return nil
}

// Steer sends a message to a running sub-agent
func (al *AgentLifecycle) Steer(id string, message string) error {
	al.mu.RLock()
	defer al.mu.RUnlock()
	agent, ok := al.agents[id]
	if !ok {
		return fmt.Errorf("agent not found: %s", id)
	}
	if agent.Status != "running" {
		return fmt.Errorf("agent is not running: %s (status: %s)", id, agent.Status)
	}
	if agent.Metadata == nil {
		agent.Metadata = make(map[string]string)
	}
	agent.Metadata["steer_message"] = message
	return nil
}

// GetTree returns the agent tree starting from a root
func (al *AgentLifecycle) GetTree(rootID string) map[string]any {
	al.mu.RLock()
	defer al.mu.RUnlock()
	agent, ok := al.agents[rootID]
	if !ok {
		return nil
	}
	tree := map[string]any{
		"agent":    agent,
		"children": []map[string]any{},
	}
	childIDs := al.children[rootID]
	for _, childID := range childIDs {
		childTree := al.GetTree(childID)
		if childTree != nil {
			tree["children"] = append(tree["children"].([]map[string]any), childTree)
		}
	}
	return tree
}

// Cleanup removes completed/failed/cancelled agents
func (al *AgentLifecycle) Cleanup() int {
	al.mu.Lock()
	defer al.mu.Unlock()
	removed := 0
	for id, agent := range al.agents {
		if agent.Status == "completed" || agent.Status == "failed" || agent.Status == "cancelled" {
			delete(al.agents, id)
			delete(al.children, id)
			removed++
		}
	}
	for parentID, childIDs := range al.children {
		var active []string
		for _, id := range childIDs {
			if _, ok := al.agents[id]; ok {
				active = append(active, id)
			}
		}
		if len(active) == 0 {
			delete(al.children, parentID)
		} else {
			al.children[parentID] = active
		}
	}
	return removed
}

// Stats returns registry statistics
func (al *AgentLifecycle) Stats() map[string]any {
	al.mu.RLock()
	defer al.mu.RUnlock()
	stats := map[string]any{
		"total":    len(al.agents),
		"max":      al.maxTotal,
		"maxDepth": al.maxDepth,
	}
	byStatus := make(map[string]int)
	for _, agent := range al.agents {
		byStatus[agent.Status]++
	}
	stats["by_status"] = byStatus
	return stats
}

// Announce sends a notification to all child agents
func (al *AgentLifecycle) Announce(parentID string, message string) error {
	al.mu.RLock()
	defer al.mu.RUnlock()
	childIDs := al.children[parentID]
	for _, id := range childIDs {
		if agent, ok := al.agents[id]; ok && agent.Status == "running" {
			if agent.Metadata == nil {
				agent.Metadata = make(map[string]string)
			}
			agent.Metadata["announce"] = message
		}
	}
	return nil
}

// WaitForCompletion waits for a sub-agent to complete
func (al *AgentLifecycle) WaitForCompletion(ctx context.Context, id string, timeout time.Duration) (*ManagedAgent, error) {
	deadline := time.After(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline:
			return nil, fmt.Errorf("timeout waiting for agent %s", id)
		case <-ticker.C:
			agent, ok := al.Get(id)
			if !ok {
				return nil, fmt.Errorf("agent not found: %s", id)
			}
			if agent.Status == "completed" || agent.Status == "failed" || agent.Status == "cancelled" {
				return agent, nil
			}
		}
	}
}
