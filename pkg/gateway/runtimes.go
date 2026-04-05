package gateway

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	appRuntime "github.com/anyclaw/anyclaw/pkg/runtime"
)

type RuntimeInfo struct {
	Key           string    `json:"key"`
	Agent         string    `json:"agent"`
	Org           string    `json:"org"`
	Project       string    `json:"project"`
	Workspace     string    `json:"workspace"`
	WorkspacePath string    `json:"workspace_path"`
	WorkDir       string    `json:"work_dir"`
	CreatedAt     time.Time `json:"created_at"`
	LastUsedAt    time.Time `json:"last_used_at"`
	SessionCount  int       `json:"session_count"`
	Hits          int       `json:"hits"`
	Builds        int       `json:"builds"`
	LastReason    string    `json:"last_reason,omitempty"`
}

type RuntimeMetrics struct {
	Hits      int `json:"hits"`
	Builds    int `json:"builds"`
	Evictions int `json:"evictions"`
	Refreshes int `json:"refreshes"`
}

type RuntimePool struct {
	mu           sync.Mutex
	configPath   string
	runtimes     map[string]*runtimeEntry
	store        *Store
	maxInstances int
	idleTTL      time.Duration
	metrics      RuntimeMetrics
}

type runtimeEntry struct {
	app        *appRuntime.App
	createdAt  time.Time
	lastUsedAt time.Time
	hits       int
	builds     int
	lastReason string
}

func NewRuntimePool(configPath string, store *Store, maxInstances int, idleTTL time.Duration) *RuntimePool {
	if maxInstances <= 0 {
		maxInstances = 16
	}
	if idleTTL <= 0 {
		idleTTL = 15 * time.Minute
	}
	return &RuntimePool{configPath: configPath, store: store, runtimes: make(map[string]*runtimeEntry), maxInstances: maxInstances, idleTTL: idleTTL}
}

func (p *RuntimePool) GetOrCreate(agentName string, org string, project string, workspaceID string) (*appRuntime.App, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cleanupLocked()
	workspace, ok := p.store.GetWorkspace(workspaceID)
	if !ok {
		return nil, fmt.Errorf("workspace not found: %s", workspaceID)
	}
	key := runtimeKey(agentName, org, project, workspaceID)
	if entry, ok := p.runtimes[key]; ok {
		entry.lastUsedAt = time.Now().UTC()
		entry.hits++
		entry.lastReason = "cache-hit"
		p.metrics.Hits++
		return entry.app, nil
	}
	app, err := appRuntime.NewTargetApp(p.configPath, agentName, workspace.Path)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if len(p.runtimes) >= p.maxInstances {
		p.evictOldestLocked()
	}
	p.metrics.Builds++
	p.runtimes[key] = &runtimeEntry{app: app, createdAt: now, lastUsedAt: now, builds: 1, lastReason: "created"}
	return app, nil
}

func (p *RuntimePool) List() []RuntimeInfo {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cleanupLocked()
	sessionCounts := p.sessionCounts()
	items := make([]RuntimeInfo, 0, len(p.runtimes))
	for key, entry := range p.runtimes {
		parts := sessionCounts[key]
		_ = parts
		items = append(items, RuntimeInfo{
			Key:           key,
			Agent:         entry.app.Config.Agent.Name,
			Org:           runtimePart(key, 1),
			Project:       runtimePart(key, 2),
			Workspace:     runtimePart(key, 3),
			WorkspacePath: entry.app.WorkingDir,
			WorkDir:       entry.app.WorkDir,
			CreatedAt:     entry.createdAt,
			LastUsedAt:    entry.lastUsedAt,
			SessionCount:  sessionCounts[key],
			Hits:          entry.hits,
			Builds:        entry.builds,
			LastReason:    entry.lastReason,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].LastUsedAt.After(items[j].LastUsedAt) })
	return items
}

func (p *RuntimePool) Refresh(agentName string, org string, project string, workspaceID string) {
	p.mu.Lock()
	p.metrics.Refreshes++
	p.mu.Unlock()
	p.Invalidate(agentName, org, project, workspaceID)
}

func (p *RuntimePool) Metrics() RuntimeMetrics {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.metrics
}

type PoolStatus struct {
	Pooled int `json:"pooled"`
	Active int `json:"active"`
	Idle   int `json:"idle"`
	Max    int `json:"max"`
}

func (p *RuntimePool) Status() PoolStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	return PoolStatus{
		Pooled: len(p.runtimes),
		Active: 0,
		Idle:   len(p.runtimes),
		Max:    p.maxInstances,
	}
}

func (p *RuntimePool) cleanupLocked() {
	if p.idleTTL <= 0 {
		return
	}
	now := time.Now().UTC()
	for key, entry := range p.runtimes {
		if now.Sub(entry.lastUsedAt) > p.idleTTL {
			p.metrics.Evictions++
			delete(p.runtimes, key)
		}
	}
}

func (p *RuntimePool) evictOldestLocked() {
	var oldestKey string
	var oldest time.Time
	first := true
	for key, entry := range p.runtimes {
		if first || entry.lastUsedAt.Before(oldest) {
			oldestKey = key
			oldest = entry.lastUsedAt
			first = false
		}
	}
	if oldestKey != "" {
		p.metrics.Evictions++
		delete(p.runtimes, oldestKey)
	}
}

func (p *RuntimePool) Invalidate(agentName string, org string, project string, workspaceID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.runtimes, runtimeKey(agentName, org, project, workspaceID))
}

func (p *RuntimePool) InvalidateByAgent(agentName string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for key := range p.runtimes {
		if runtimePart(key, 0) == agentName {
			delete(p.runtimes, key)
		}
	}
}

func (p *RuntimePool) InvalidateByWorkspace(workspaceID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for key := range p.runtimes {
		if runtimePart(key, 3) == workspaceID {
			delete(p.runtimes, key)
		}
	}
}

func (p *RuntimePool) InvalidateByProject(projectID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for key := range p.runtimes {
		if runtimePart(key, 2) == projectID {
			delete(p.runtimes, key)
		}
	}
}

func (p *RuntimePool) InvalidateAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	clear(p.runtimes)
}

func (p *RuntimePool) sessionCounts() map[string]int {
	counts := map[string]int{}
	if p.store == nil {
		return counts
	}
	for _, session := range p.store.ListSessions() {
		counts[runtimeKey(session.Agent, session.Org, session.Project, session.Workspace)]++
	}
	return counts
}

func runtimeKey(agentName string, parts ...string) string {
	all := append([]string{agentName}, parts...)
	return strings.Join(all, "::")
}

func runtimePart(key string, index int) string {
	parts := strings.Split(key, "::")
	if index < len(parts) {
		return parts[index]
	}
	return ""
}
