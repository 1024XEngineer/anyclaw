package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/state"
)

type RefreshScopeKind string

const (
	RefreshScopeRuntime   RefreshScopeKind = "runtime"
	RefreshScopeAgent     RefreshScopeKind = "agent"
	RefreshScopeWorkspace RefreshScopeKind = "workspace"
	RefreshScopeProject   RefreshScopeKind = "project"
	RefreshScopeSession   RefreshScopeKind = "session"
	RefreshScopeGlobal    RefreshScopeKind = "global"
)

type RefreshScope struct {
	Kind      RefreshScopeKind `json:"kind"`
	Agent     string           `json:"agent,omitempty"`
	Org       string           `json:"org,omitempty"`
	Project   string           `json:"project,omitempty"`
	Workspace string           `json:"workspace,omitempty"`
	SessionID string           `json:"session_id,omitempty"`
	Reason    string           `json:"reason,omitempty"`
	Warm      bool             `json:"warm,omitempty"`
}

type RefreshResult struct {
	Scope       RefreshScope `json:"scope"`
	Status      string       `json:"status"`
	Error       string       `json:"error,omitempty"`
	Warmed      bool         `json:"warmed,omitempty"`
	RefreshedAt string       `json:"refreshed_at"`
}

type SessionResolver interface {
	GetSession(id string) (*state.Session, bool)
}

type HotReloadCoordinator struct {
	pool     *RuntimePool
	sessions SessionResolver
}

func NewHotReloadCoordinator(pool *RuntimePool, sessions SessionResolver) *HotReloadCoordinator {
	return &HotReloadCoordinator{pool: pool, sessions: sessions}
}

func (c *HotReloadCoordinator) Refresh(ctx context.Context, scope RefreshScope) RefreshResult {
	result := RefreshResult{
		Scope:       normalizeRefreshScope(scope),
		Status:      "refreshed",
		RefreshedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if c == nil || c.pool == nil {
		return failRefreshResult(result, "runtime pool is not configured")
	}
	select {
	case <-ctx.Done():
		return failRefreshResult(result, ctx.Err().Error())
	default:
	}

	switch result.Scope.Kind {
	case RefreshScopeRuntime:
		if strings.TrimSpace(result.Scope.Workspace) == "" {
			return failRefreshResult(result, "workspace is required for runtime refresh")
		}
		c.pool.Refresh(result.Scope.Agent, result.Scope.Org, result.Scope.Project, result.Scope.Workspace)
	case RefreshScopeAgent:
		if strings.TrimSpace(result.Scope.Agent) == "" {
			return failRefreshResult(result, "agent is required for agent refresh")
		}
		c.pool.RefreshByAgent(result.Scope.Agent)
	case RefreshScopeWorkspace:
		if strings.TrimSpace(result.Scope.Workspace) == "" {
			return failRefreshResult(result, "workspace is required for workspace refresh")
		}
		c.pool.RefreshByWorkspace(result.Scope.Workspace)
	case RefreshScopeProject:
		if strings.TrimSpace(result.Scope.Project) == "" {
			return failRefreshResult(result, "project is required for project refresh")
		}
		c.pool.RefreshByProject(result.Scope.Project)
	case RefreshScopeSession:
		binding, err := c.sessionBinding(result.Scope.SessionID)
		if err != nil {
			return failRefreshResult(result, err.Error())
		}
		result.Scope.Agent = firstNonEmpty(result.Scope.Agent, binding.Agent)
		result.Scope.Org = firstNonEmpty(result.Scope.Org, binding.Org)
		result.Scope.Project = firstNonEmpty(result.Scope.Project, binding.Project)
		result.Scope.Workspace = firstNonEmpty(result.Scope.Workspace, binding.Workspace)
		if strings.TrimSpace(result.Scope.Workspace) == "" {
			return failRefreshResult(result, "workspace is required for session refresh")
		}
		c.pool.Refresh(result.Scope.Agent, result.Scope.Org, result.Scope.Project, result.Scope.Workspace)
	case RefreshScopeGlobal:
		c.pool.RefreshAll()
	default:
		return failRefreshResult(result, fmt.Sprintf("unsupported refresh scope: %s", result.Scope.Kind))
	}

	if result.Scope.Warm && result.Scope.Kind == RefreshScopeRuntime {
		if _, err := c.pool.GetOrCreate(result.Scope.Agent, result.Scope.Org, result.Scope.Project, result.Scope.Workspace); err != nil {
			return failRefreshResult(result, err.Error())
		}
		result.Warmed = true
	}
	return result
}

func (c *HotReloadCoordinator) RefreshMany(ctx context.Context, scopes []RefreshScope) []RefreshResult {
	results := make([]RefreshResult, 0, len(scopes))
	for _, scope := range scopes {
		results = append(results, c.Refresh(ctx, scope))
	}
	return results
}

func (c *HotReloadCoordinator) sessionBinding(sessionID string) (state.SessionExecutionBinding, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return state.SessionExecutionBinding{}, fmt.Errorf("session_id is required for session refresh")
	}
	if c == nil || c.sessions == nil {
		return state.SessionExecutionBinding{}, fmt.Errorf("session resolver is not configured")
	}
	session, ok := c.sessions.GetSession(sessionID)
	if !ok {
		return state.SessionExecutionBinding{}, fmt.Errorf("session not found: %s", sessionID)
	}
	return state.SessionExecutionBindingValue(session), nil
}

func normalizeRefreshScope(scope RefreshScope) RefreshScope {
	scope.Kind = RefreshScopeKind(strings.ToLower(strings.TrimSpace(string(scope.Kind))))
	if scope.Kind == "" {
		scope.Kind = RefreshScopeRuntime
	}
	scope.Agent = strings.TrimSpace(scope.Agent)
	scope.Org = strings.TrimSpace(scope.Org)
	scope.Project = strings.TrimSpace(scope.Project)
	scope.Workspace = strings.TrimSpace(scope.Workspace)
	scope.SessionID = strings.TrimSpace(scope.SessionID)
	scope.Reason = strings.TrimSpace(scope.Reason)
	return scope
}

func failRefreshResult(result RefreshResult, msg string) RefreshResult {
	result.Status = "failed"
	result.Error = msg
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
