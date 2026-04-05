package workflow

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// TriggerType defines the type of workflow trigger.
type TriggerType string

const (
	TriggerCron    TriggerType = "cron"
	TriggerWebhook TriggerType = "webhook"
	TriggerEvent   TriggerType = "event"
	TriggerManual  TriggerType = "manual"
)

// TriggerConfig holds the configuration for a workflow trigger.
type TriggerConfig struct {
	ID          string      `json:"id"`
	GraphID     string      `json:"graph_id"`
	Type        TriggerType `json:"type"`
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Enabled     bool        `json:"enabled"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`

	// Cron trigger fields
	CronExpr string `json:"cron_expr,omitempty"`
	Timezone string `json:"timezone,omitempty"`

	// Webhook trigger fields
	WebhookPath   string `json:"webhook_path,omitempty"`
	WebhookSecret string `json:"webhook_secret,omitempty"`

	// Event trigger fields
	EventSource string   `json:"event_source,omitempty"`
	EventTypes  []string `json:"event_types,omitempty"`
	EventFilter string   `json:"event_filter,omitempty"`

	// Common
	DefaultInputs map[string]any `json:"default_inputs,omitempty"`
	MaxRuns       int            `json:"max_runs,omitempty"`
	TimeoutSec    int            `json:"timeout_sec,omitempty"`
}

// WorkflowTriggerEvent represents an event that can trigger a workflow.
type WorkflowTriggerEvent struct {
	ID        string         `json:"id"`
	Source    string         `json:"source"`
	Type      string         `json:"type"`
	Payload   map[string]any `json:"payload,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
}

// TriggerRun tracks a single trigger execution.
type TriggerRun struct {
	TriggerID   string                `json:"trigger_id"`
	ExecutionID string                `json:"execution_id"`
	Status      string                `json:"status"`
	TriggeredBy string                `json:"triggered_by"` // cron, webhook, event, manual
	Event       *WorkflowTriggerEvent `json:"event,omitempty"`
	StartedAt   time.Time             `json:"started_at"`
	EndedAt     *time.Time            `json:"ended_at,omitempty"`
	Error       string                `json:"error,omitempty"`
}

// TriggerManager manages workflow triggers.
type TriggerManager struct {
	mu         sync.RWMutex
	triggers   map[string]*TriggerConfig
	runs       []*TriggerRun
	executor   *WorkflowExecutor
	graphStore *FileGraphStore
	hookFunc   func(ctx context.Context, triggerID string, inputs map[string]any) (*ExecutionContext, error)
}

// NewTriggerManager creates a new trigger manager.
func NewTriggerManager(executor *WorkflowExecutor, graphStore *FileGraphStore) *TriggerManager {
	return &TriggerManager{
		triggers:   make(map[string]*TriggerConfig),
		runs:       make([]*TriggerRun, 0),
		executor:   executor,
		graphStore: graphStore,
	}
}

// SetHookFunc sets a custom hook function for trigger execution.
func (tm *TriggerManager) SetHookFunc(fn func(ctx context.Context, triggerID string, inputs map[string]any) (*ExecutionContext, error)) {
	tm.hookFunc = fn
}

// AddTrigger adds a new trigger configuration.
func (tm *TriggerManager) AddTrigger(cfg TriggerConfig) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if cfg.ID == "" {
		cfg.ID = fmt.Sprintf("trigger_%d", time.Now().UnixNano())
	}
	cfg.Enabled = true
	cfg.CreatedAt = time.Now().UTC()
	cfg.UpdatedAt = time.Now().UTC()

	if cfg.GraphID == "" {
		return fmt.Errorf("graph_id is required")
	}

	switch cfg.Type {
	case TriggerCron:
		if cfg.CronExpr == "" {
			return fmt.Errorf("cron_expr is required for cron triggers")
		}
	case TriggerWebhook:
		if cfg.WebhookPath == "" {
			return fmt.Errorf("webhook_path is required for webhook triggers")
		}
	case TriggerEvent:
		if cfg.EventSource == "" {
			return fmt.Errorf("event_source is required for event triggers")
		}
	}

	tm.triggers[cfg.ID] = &cfg
	return nil
}

// GetTrigger returns a trigger by ID.
func (tm *TriggerManager) GetTrigger(id string) (*TriggerConfig, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	cfg, ok := tm.triggers[id]
	return cfg, ok
}

// ListTriggers returns all triggers, optionally filtered by graph ID.
func (tm *TriggerManager) ListTriggers(graphID string) []*TriggerConfig {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	var result []*TriggerConfig
	for _, cfg := range tm.triggers {
		if graphID != "" && cfg.GraphID != graphID {
			continue
		}
		result = append(result, cfg)
	}
	return result
}

// DeleteTrigger removes a trigger.
func (tm *TriggerManager) DeleteTrigger(id string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if _, ok := tm.triggers[id]; !ok {
		return fmt.Errorf("trigger not found: %s", id)
	}

	delete(tm.triggers, id)
	return nil
}

// EnableTrigger enables a trigger.
func (tm *TriggerManager) EnableTrigger(id string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	cfg, ok := tm.triggers[id]
	if !ok {
		return fmt.Errorf("trigger not found: %s", id)
	}

	cfg.Enabled = true
	cfg.UpdatedAt = time.Now().UTC()
	return nil
}

// DisableTrigger disables a trigger.
func (tm *TriggerManager) DisableTrigger(id string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	cfg, ok := tm.triggers[id]
	if !ok {
		return fmt.Errorf("trigger not found: %s", id)
	}

	cfg.Enabled = false
	cfg.UpdatedAt = time.Now().UTC()
	return nil
}

// FireTrigger manually fires a trigger with optional inputs.
func (tm *TriggerManager) FireTrigger(ctx context.Context, triggerID string, inputs map[string]any) (*TriggerRun, error) {
	tm.mu.Lock()
	cfg, ok := tm.triggers[triggerID]
	if !ok {
		tm.mu.Unlock()
		return nil, fmt.Errorf("trigger not found: %s", triggerID)
	}

	if !cfg.Enabled {
		tm.mu.Unlock()
		return nil, fmt.Errorf("trigger is disabled: %s", triggerID)
	}
	tm.mu.Unlock()

	// Merge default inputs with provided inputs
	merged := make(map[string]any)
	for k, v := range cfg.DefaultInputs {
		merged[k] = v
	}
	for k, v := range inputs {
		merged[k] = v
	}

	// Load graph
	graph, err := tm.graphStore.LoadGraph(cfg.GraphID)
	if err != nil {
		return nil, fmt.Errorf("failed to load graph: %w", err)
	}

	// Execute
	var exec *ExecutionContext
	if tm.hookFunc != nil {
		exec, err = tm.hookFunc(ctx, triggerID, merged)
	} else {
		exec, err = tm.executor.ExecuteGraph(graph, merged)
	}

	run := &TriggerRun{
		TriggerID:   triggerID,
		ExecutionID: exec.ExecutionID,
		Status:      string(exec.Status),
		TriggeredBy: "manual",
		StartedAt:   exec.StartTime,
	}

	if err != nil {
		run.Status = "failed"
		run.Error = err.Error()
		now := time.Now().UTC()
		run.EndedAt = &now
	}

	tm.mu.Lock()
	tm.runs = append(tm.runs, run)
	tm.mu.Unlock()

	return run, nil
}

// HandleWebhook handles an incoming webhook request and fires matching triggers.
func (tm *TriggerManager) HandleWebhook(ctx context.Context, path string, payload map[string]any) ([]*TriggerRun, error) {
	tm.mu.RLock()
	var matching []*TriggerConfig
	for _, cfg := range tm.triggers {
		if !cfg.Enabled {
			continue
		}
		if cfg.Type == TriggerWebhook && cfg.WebhookPath == path {
			matching = append(matching, cfg)
		}
	}
	tm.mu.RUnlock()

	if len(matching) == 0 {
		return nil, fmt.Errorf("no webhook trigger found for path: %s", path)
	}

	var runs []*TriggerRun
	for _, cfg := range matching {
		inputs := make(map[string]any)
		for k, v := range cfg.DefaultInputs {
			inputs[k] = v
		}
		for k, v := range payload {
			inputs[k] = v
		}

		run, err := tm.FireTrigger(ctx, cfg.ID, inputs)
		if err != nil {
			continue
		}
		run.TriggeredBy = "webhook"
		run.Event = &WorkflowTriggerEvent{
			Source:    "webhook",
			Type:      "http_request",
			Payload:   payload,
			Timestamp: time.Now().UTC(),
		}
		runs = append(runs, run)
	}

	return runs, nil
}

// HandleEvent handles an incoming event and fires matching triggers.
func (tm *TriggerManager) HandleEvent(ctx context.Context, event *WorkflowTriggerEvent) ([]*TriggerRun, error) {
	tm.mu.RLock()
	var matching []*TriggerConfig
	for _, cfg := range tm.triggers {
		if !cfg.Enabled {
			continue
		}
		if cfg.Type != TriggerEvent {
			continue
		}
		if cfg.EventSource != event.Source {
			continue
		}
		if len(cfg.EventTypes) > 0 {
			matched := false
			for _, et := range cfg.EventTypes {
				if et == event.Type || et == "*" {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		// Check event filter
		if cfg.EventFilter != "" {
			result, err := EvalCondition(cfg.EventFilter, event.Payload)
			if err != nil || !result {
				continue
			}
		}
		matching = append(matching, cfg)
	}
	tm.mu.RUnlock()

	var runs []*TriggerRun
	for _, cfg := range matching {
		inputs := make(map[string]any)
		for k, v := range cfg.DefaultInputs {
			inputs[k] = v
		}
		for k, v := range event.Payload {
			inputs[k] = v
		}
		inputs["_event_source"] = event.Source
		inputs["_event_type"] = event.Type
		inputs["_event_timestamp"] = event.Timestamp

		run, err := tm.FireTrigger(ctx, cfg.ID, inputs)
		if err != nil {
			continue
		}
		run.TriggeredBy = "event"
		run.Event = event
		runs = append(runs, run)
	}

	return runs, nil
}

// GetRuns returns trigger runs, optionally filtered by trigger ID.
func (tm *TriggerManager) GetRuns(triggerID string, limit int) []*TriggerRun {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	var result []*TriggerRun
	for _, run := range tm.runs {
		if triggerID != "" && run.TriggerID != triggerID {
			continue
		}
		result = append(result, run)
		if limit > 0 && len(result) >= limit {
			break
		}
	}
	return result
}

// GetCronTriggers returns all cron triggers for scheduling integration.
func (tm *TriggerManager) GetCronTriggers() []*TriggerConfig {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	var result []*TriggerConfig
	for _, cfg := range tm.triggers {
		if cfg.Enabled && cfg.Type == TriggerCron {
			result = append(result, cfg)
		}
	}
	return result
}

// GetWebhookTriggers returns all webhook triggers for HTTP handler registration.
func (tm *TriggerManager) GetWebhookTriggers() []*TriggerConfig {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	var result []*TriggerConfig
	for _, cfg := range tm.triggers {
		if cfg.Enabled && cfg.Type == TriggerWebhook {
			result = append(result, cfg)
		}
	}
	return result
}

// Stats returns trigger manager statistics.
func (tm *TriggerManager) Stats() map[string]any {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	byType := make(map[string]int)
	byStatus := make(map[string]int)
	for _, cfg := range tm.triggers {
		byType[string(cfg.Type)]++
		if cfg.Enabled {
			byStatus["enabled"]++
		} else {
			byStatus["disabled"]++
		}
	}

	return map[string]any{
		"total_triggers": len(tm.triggers),
		"by_type":        byType,
		"by_status":      byStatus,
		"total_runs":     len(tm.runs),
	}
}
