package isolation

import (
	"time"

	ctxpkg "github.com/anyclaw/anyclaw/pkg/context"
)

type IsolationMode string

const (
	IsolationModeStrict  IsolationMode = "strict"
	IsolationModeShared  IsolationMode = "shared"
	IsolationModeHybrid  IsolationMode = "hybrid"
	IsolationModeInherit IsolationMode = "inherit"
)

type ContextVisibility string

const (
	ContextVisibilityPrivate ContextVisibility = "private"
	ContextVisibilityPublic  ContextVisibility = "public"
	ContextVisibilityScoped  ContextVisibility = "scoped"
)

type ContextScope struct {
	AgentID   string
	SessionID string
	TaskID    string
	Namespace string
	Labels    map[string]string
	CreatedAt time.Time
	ExpiresAt time.Time
}

func (s *ContextScope) ID() string {
	if s.TaskID != "" {
		return s.TaskID
	}
	if s.SessionID != "" {
		return s.SessionID
	}
	return s.AgentID
}

type ContextBoundary struct {
	Scope      *ContextScope
	Mode       IsolationMode
	Visibility ContextVisibility
	Parent     *ContextBoundary
	Children   []*ContextBoundary
}

type SharedContextPolicy struct {
	SourceAgentID   string
	TargetAgentIDs  []string
	ContextKeys     []string
	Namespace       string
	ExpiresAt       time.Time
	OneWay          bool
	RequireApproval bool
}

type ContextSnapshot struct {
	ID          string
	AgentID     string
	SessionID   string
	Documents   []ctxpkg.Document
	KeyValues   map[string]any
	TakenAt     time.Time
	Description string
}

type IsolationConfig struct {
	DefaultMode       IsolationMode
	DefaultVisibility ContextVisibility
	MaxContextSize    int
	DefaultTTL        time.Duration
	EnableSharing     bool
	EnableSnapshots   bool
	MaxSnapshots      int
	CleanupInterval   time.Duration
}

func DefaultIsolationConfig() IsolationConfig {
	return IsolationConfig{
		DefaultMode:       IsolationModeStrict,
		DefaultVisibility: ContextVisibilityPrivate,
		MaxContextSize:    1000,
		DefaultTTL:        30 * time.Minute,
		EnableSharing:     true,
		EnableSnapshots:   true,
		MaxSnapshots:      50,
		CleanupInterval:   5 * time.Minute,
	}
}

type IsolatedContextEngine interface {
	ctxpkg.ContextEngine
	Scope() *ContextScope
	Boundary() *ContextBoundary
	SetVisibility(visibility ContextVisibility)
	Visibility() ContextVisibility
	Clone() IsolatedContextEngine
}
