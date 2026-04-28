package workflow

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// TriggerType defines the source that can start a workflow graph.
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

	CronExpr string `json:"cron_expr,omitempty"`
	Timezone string `json:"timezone,omitempty"`

	WebhookPath   string `json:"webhook_path,omitempty"`
	WebhookSecret string `json:"webhook_secret,omitempty"`

	EventSource string   `json:"event_source,omitempty"`
	EventTypes  []string `json:"event_types,omitempty"`
	EventFilter string   `json:"event_filter,omitempty"`

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
	TriggeredBy string                `json:"triggered_by"`
	Event       *WorkflowTriggerEvent `json:"event,omitempty"`
	StartedAt   time.Time             `json:"started_at"`
	EndedAt     *time.Time            `json:"ended_at,omitempty"`
	Error       string                `json:"error,omitempty"`
}

// TriggerExecutor is the minimal execution surface needed by TriggerManager.
type TriggerExecutor interface {
	ExecuteGraph(graph *Graph, inputs map[string]any) (*ExecutionContext, error)
}

// TriggerHook can replace graph loading and executor dispatch for integrations.
type TriggerHook func(ctx context.Context, triggerID string, inputs map[string]any) (*ExecutionContext, error)

// TriggerManager manages in-memory workflow triggers and run history.
type TriggerManager struct {
	mu       sync.RWMutex
	triggers map[string]TriggerConfig
	runs     []TriggerRun
	inFlight map[string]int

	executor   TriggerExecutor
	graphStore GraphStore
	hookFunc   TriggerHook
}

// NewTriggerManager creates a trigger manager.
func NewTriggerManager(executor TriggerExecutor, graphStore GraphStore) *TriggerManager {
	return &TriggerManager{
		triggers:   make(map[string]TriggerConfig),
		runs:       make([]TriggerRun, 0),
		inFlight:   make(map[string]int),
		executor:   executor,
		graphStore: graphStore,
	}
}

// SetHookFunc sets a custom execution hook used instead of graphStore/executor.
func (tm *TriggerManager) SetHookFunc(fn TriggerHook) {
	if tm == nil {
		return
	}
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.hookFunc = fn
}

// AddTrigger adds a trigger configuration. New triggers are enabled by default.
func (tm *TriggerManager) AddTrigger(cfg TriggerConfig) error {
	if tm == nil {
		return fmt.Errorf("trigger manager is nil")
	}

	cfg = normalizeTriggerConfig(cfg)
	if cfg.ID == "" {
		cfg.ID = generateWorkflowID("trigger")
	}
	if cfg.Type == "" {
		cfg.Type = TriggerManual
	}
	if strings.TrimSpace(cfg.Name) == "" {
		cfg.Name = cfg.ID
	}
	if cfg.CreatedAt.IsZero() {
		cfg.CreatedAt = time.Now().UTC()
	}
	cfg.UpdatedAt = time.Now().UTC()
	cfg.Enabled = true

	if err := validateTriggerConfig(cfg); err != nil {
		return err
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()
	if _, exists := tm.triggers[cfg.ID]; exists {
		return fmt.Errorf("trigger already exists: %s", cfg.ID)
	}
	tm.triggers[cfg.ID] = cfg
	return nil
}

// GetTrigger returns a defensive public copy of a trigger by ID.
func (tm *TriggerManager) GetTrigger(id string) (*TriggerConfig, bool) {
	if tm == nil {
		return nil, false
	}
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	cfg, ok := tm.triggers[id]
	if !ok {
		return nil, false
	}
	cloned := clonePublicTriggerConfig(cfg)
	return &cloned, true
}

// ListTriggers returns public trigger configurations, optionally filtered by graph ID.
func (tm *TriggerManager) ListTriggers(graphID string) []*TriggerConfig {
	if tm == nil {
		return nil
	}
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	result := make([]*TriggerConfig, 0, len(tm.triggers))
	for _, cfg := range tm.triggers {
		if graphID != "" && cfg.GraphID != graphID {
			continue
		}
		cloned := clonePublicTriggerConfig(cfg)
		result = append(result, &cloned)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

// DeleteTrigger removes a trigger.
func (tm *TriggerManager) DeleteTrigger(id string) error {
	if tm == nil {
		return fmt.Errorf("trigger manager is nil")
	}
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
	return tm.setTriggerEnabled(id, true)
}

// DisableTrigger disables a trigger.
func (tm *TriggerManager) DisableTrigger(id string) error {
	return tm.setTriggerEnabled(id, false)
}

// FireTrigger manually fires a trigger with optional inputs.
func (tm *TriggerManager) FireTrigger(ctx context.Context, triggerID string, inputs map[string]any) (*TriggerRun, error) {
	return tm.fireTrigger(ctx, triggerID, inputs, "manual", nil)
}

// HandleWebhook handles an incoming webhook payload and fires matching triggers.
func (tm *TriggerManager) HandleWebhook(ctx context.Context, path string, payload map[string]any) ([]*TriggerRun, error) {
	return tm.HandleWebhookWithSecret(ctx, path, "", payload)
}

// HandleWebhookWithSecret handles a webhook payload and enforces configured webhook secrets.
func (tm *TriggerManager) HandleWebhookWithSecret(
	ctx context.Context,
	path string,
	secret string,
	payload map[string]any,
) ([]*TriggerRun, error) {
	if tm == nil {
		return nil, fmt.Errorf("trigger manager is nil")
	}
	path = strings.TrimSpace(path)

	matching := tm.matchingTriggerIDs(func(cfg TriggerConfig) bool {
		if !cfg.Enabled || cfg.Type != TriggerWebhook || cfg.WebhookPath != path {
			return false
		}
		return webhookSecretMatches(cfg.WebhookSecret, secret)
	})
	if len(matching) == 0 {
		return nil, fmt.Errorf("no webhook trigger found for path: %s", path)
	}

	event := &WorkflowTriggerEvent{
		Source:    "webhook",
		Type:      "http_request",
		Payload:   cloneAnyMap(payload),
		Timestamp: time.Now().UTC(),
	}
	return tm.fireMatching(ctx, matching, payload, "webhook", event)
}

// HandleEvent handles an incoming event and fires matching event triggers.
func (tm *TriggerManager) HandleEvent(ctx context.Context, event *WorkflowTriggerEvent) ([]*TriggerRun, error) {
	if tm == nil {
		return nil, fmt.Errorf("trigger manager is nil")
	}
	if event == nil {
		return nil, fmt.Errorf("workflow trigger event is nil")
	}
	if event.Timestamp.IsZero() {
		event = cloneTriggerEvent(event)
		event.Timestamp = time.Now().UTC()
	}

	matching, err := tm.matchingEventTriggerIDs(event)
	if err != nil {
		return nil, err
	}
	if len(matching) == 0 {
		return nil, fmt.Errorf("no event trigger found for source=%s type=%s", event.Source, event.Type)
	}

	inputs := cloneAnyMap(event.Payload)
	if inputs == nil {
		inputs = make(map[string]any)
	}
	inputs["_event_source"] = event.Source
	inputs["_event_type"] = event.Type
	inputs["_event_timestamp"] = event.Timestamp
	return tm.fireMatching(ctx, matching, inputs, "event", event)
}

// GetRuns returns run history, optionally filtered by trigger ID.
func (tm *TriggerManager) GetRuns(triggerID string, limit int) []*TriggerRun {
	if tm == nil {
		return nil
	}
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	result := make([]*TriggerRun, 0, len(tm.runs))
	for i := len(tm.runs) - 1; i >= 0; i-- {
		run := tm.runs[i]
		if triggerID != "" && run.TriggerID != triggerID {
			continue
		}
		cloned := cloneTriggerRun(run)
		result = append(result, &cloned)
		if limit > 0 && len(result) >= limit {
			break
		}
	}
	return result
}

// GetCronTriggers returns enabled public cron triggers for scheduler integration.
func (tm *TriggerManager) GetCronTriggers() []*TriggerConfig {
	if tm == nil {
		return nil
	}
	return tm.filterTriggers(func(cfg TriggerConfig) bool {
		return cfg.Enabled && cfg.Type == TriggerCron
	})
}

// GetWebhookTriggers returns enabled public webhook triggers for HTTP registration.
func (tm *TriggerManager) GetWebhookTriggers() []*TriggerConfig {
	if tm == nil {
		return nil
	}
	return tm.filterTriggers(func(cfg TriggerConfig) bool {
		return cfg.Enabled && cfg.Type == TriggerWebhook
	})
}

// Stats returns trigger manager statistics.
func (tm *TriggerManager) Stats() map[string]any {
	if tm == nil {
		return map[string]any{
			"total_triggers": 0,
			"by_type":        map[string]int{},
			"by_status":      map[string]int{},
			"total_runs":     0,
		}
	}
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

func (tm *TriggerManager) setTriggerEnabled(id string, enabled bool) error {
	if tm == nil {
		return fmt.Errorf("trigger manager is nil")
	}
	tm.mu.Lock()
	defer tm.mu.Unlock()

	cfg, ok := tm.triggers[id]
	if !ok {
		return fmt.Errorf("trigger not found: %s", id)
	}
	cfg.Enabled = enabled
	cfg.UpdatedAt = time.Now().UTC()
	tm.triggers[id] = cfg
	return nil
}

func (tm *TriggerManager) fireTrigger(
	ctx context.Context,
	triggerID string,
	inputs map[string]any,
	triggeredBy string,
	event *WorkflowTriggerEvent,
) (*TriggerRun, error) {
	if tm == nil {
		return nil, fmt.Errorf("trigger manager is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	cfg, hook, err := tm.reserveTriggerRun(triggerID)
	if err != nil {
		return nil, err
	}
	reserved := true
	defer func() {
		if reserved {
			tm.releaseTriggerReservation(triggerID)
		}
	}()

	merged := cloneAnyMap(cfg.DefaultInputs)
	if merged == nil {
		merged = make(map[string]any)
	}
	for k, v := range inputs {
		merged[k] = cloneAny(v)
	}

	runCtx := ctx
	var cancel context.CancelFunc
	if cfg.TimeoutSec > 0 {
		runCtx, cancel = context.WithTimeout(ctx, time.Duration(cfg.TimeoutSec)*time.Second)
		defer cancel()
	}

	startedAt := time.Now().UTC()
	execCtx, execErr := tm.executeTrigger(runCtx, cfg, hook, merged)
	run := buildTriggerRun(triggerID, triggeredBy, event, startedAt, execCtx, execErr)
	tm.appendRunAndRelease(run)
	reserved = false
	return cloneTriggerRunPtr(run), execErr
}

func (tm *TriggerManager) reserveTriggerRun(triggerID string) (TriggerConfig, TriggerHook, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	cfg, ok := tm.triggers[triggerID]
	if !ok {
		return TriggerConfig{}, nil, fmt.Errorf("trigger not found: %s", triggerID)
	}
	if !cfg.Enabled {
		return TriggerConfig{}, nil, fmt.Errorf("trigger is disabled: %s", triggerID)
	}
	if cfg.MaxRuns > 0 && tm.runCountLocked(triggerID)+tm.inFlightCountLocked(triggerID) >= cfg.MaxRuns {
		return TriggerConfig{}, nil, fmt.Errorf("trigger max_runs reached: %s", triggerID)
	}
	if tm.inFlight == nil {
		tm.inFlight = make(map[string]int)
	}
	tm.inFlight[triggerID]++
	return cloneTriggerConfig(cfg), tm.hookFunc, nil
}

func (tm *TriggerManager) executeTrigger(
	ctx context.Context,
	cfg TriggerConfig,
	hook TriggerHook,
	inputs map[string]any,
) (*ExecutionContext, error) {
	if hook != nil {
		execCtx, err := hook(ctx, cfg.ID, cloneAnyMap(inputs))
		if err == nil && execCtx == nil {
			err = fmt.Errorf("workflow trigger hook returned nil execution")
		}
		return execCtx, err
	}
	if tm.graphStore == nil {
		return nil, fmt.Errorf("workflow graph store is nil")
	}
	if tm.executor == nil {
		return nil, fmt.Errorf("workflow trigger executor is nil")
	}

	graph, err := tm.graphStore.LoadGraph(cfg.GraphID)
	if err != nil {
		return nil, fmt.Errorf("load graph %q: %w", cfg.GraphID, err)
	}
	if graph == nil {
		return nil, fmt.Errorf("load graph %q: graph is nil", cfg.GraphID)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return tm.executor.ExecuteGraph(graph, cloneAnyMap(inputs))
}

func (tm *TriggerManager) runCountLocked(triggerID string) int {
	count := 0
	for _, run := range tm.runs {
		if run.TriggerID == triggerID {
			count++
		}
	}
	return count
}

func (tm *TriggerManager) inFlightCountLocked(triggerID string) int {
	if tm.inFlight == nil {
		return 0
	}
	return tm.inFlight[triggerID]
}

func (tm *TriggerManager) appendRunAndRelease(run TriggerRun) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.runs = append(tm.runs, cloneTriggerRun(run))
	tm.releaseTriggerReservationLocked(run.TriggerID)
}

func (tm *TriggerManager) releaseTriggerReservation(triggerID string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.releaseTriggerReservationLocked(triggerID)
}

func (tm *TriggerManager) releaseTriggerReservationLocked(triggerID string) {
	if tm.inFlight == nil || tm.inFlight[triggerID] == 0 {
		return
	}
	if tm.inFlight[triggerID] == 1 {
		delete(tm.inFlight, triggerID)
		return
	}
	tm.inFlight[triggerID]--
}

func (tm *TriggerManager) fireMatching(
	ctx context.Context,
	triggerIDs []string,
	inputs map[string]any,
	triggeredBy string,
	event *WorkflowTriggerEvent,
) ([]*TriggerRun, error) {
	runs := make([]*TriggerRun, 0, len(triggerIDs))
	var errs []error
	for _, id := range triggerIDs {
		run, err := tm.fireTrigger(ctx, id, inputs, triggeredBy, event)
		if run != nil {
			runs = append(runs, run)
		}
		if err != nil {
			errs = append(errs, err)
		}
	}
	return runs, errors.Join(errs...)
}

func (tm *TriggerManager) matchingTriggerIDs(match func(TriggerConfig) bool) []string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	ids := make([]string, 0)
	for _, cfg := range tm.triggers {
		if match(cfg) {
			ids = append(ids, cfg.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

func (tm *TriggerManager) matchingEventTriggerIDs(event *WorkflowTriggerEvent) ([]string, error) {
	tm.mu.RLock()
	configs := make([]TriggerConfig, 0, len(tm.triggers))
	for _, cfg := range tm.triggers {
		configs = append(configs, cloneTriggerConfig(cfg))
	}
	tm.mu.RUnlock()

	ids := make([]string, 0)
	var errs []error
	for _, cfg := range configs {
		matched, err := eventTriggerMatches(cfg, event)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if matched {
			ids = append(ids, cfg.ID)
		}
	}
	sort.Strings(ids)
	return ids, errors.Join(errs...)
}

func eventTriggerMatches(cfg TriggerConfig, event *WorkflowTriggerEvent) (bool, error) {
	if !cfg.Enabled || cfg.Type != TriggerEvent || cfg.EventSource != event.Source {
		return false, nil
	}
	if len(cfg.EventTypes) > 0 && !matchesEventType(cfg.EventTypes, event.Type) {
		return false, nil
	}
	if cfg.EventFilter == "" {
		return true, nil
	}

	ok, err := EvalCondition(cfg.EventFilter, event.Payload)
	if err != nil {
		return false, fmt.Errorf("event_filter for trigger %q: %w", cfg.ID, err)
	}
	return ok, nil
}

func (tm *TriggerManager) filterTriggers(match func(TriggerConfig) bool) []*TriggerConfig {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	result := make([]*TriggerConfig, 0)
	for _, cfg := range tm.triggers {
		if !match(cfg) {
			continue
		}
		cloned := clonePublicTriggerConfig(cfg)
		result = append(result, &cloned)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

func validateTriggerConfig(cfg TriggerConfig) error {
	if strings.TrimSpace(cfg.ID) == "" {
		return fmt.Errorf("trigger id is required")
	}
	if strings.TrimSpace(cfg.GraphID) == "" {
		return fmt.Errorf("graph_id is required")
	}
	switch cfg.Type {
	case TriggerManual:
	case TriggerCron:
		if strings.TrimSpace(cfg.CronExpr) == "" {
			return fmt.Errorf("cron_expr is required for cron triggers")
		}
	case TriggerWebhook:
		if strings.TrimSpace(cfg.WebhookPath) == "" {
			return fmt.Errorf("webhook_path is required for webhook triggers")
		}
	case TriggerEvent:
		if strings.TrimSpace(cfg.EventSource) == "" {
			return fmt.Errorf("event_source is required for event triggers")
		}
	default:
		return fmt.Errorf("unsupported trigger type: %s", cfg.Type)
	}
	return nil
}

func matchesEventType(allowed []string, eventType string) bool {
	for _, candidate := range allowed {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || candidate == eventType {
			return true
		}
	}
	return false
}

func webhookSecretMatches(expected string, provided string) bool {
	if expected == "" {
		return true
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) == 1
}

func buildTriggerRun(
	triggerID string,
	triggeredBy string,
	event *WorkflowTriggerEvent,
	startedAt time.Time,
	execCtx *ExecutionContext,
	execErr error,
) TriggerRun {
	run := TriggerRun{
		TriggerID:   triggerID,
		TriggeredBy: triggeredBy,
		Event:       cloneTriggerEvent(event),
		StartedAt:   startedAt,
	}
	if execCtx != nil {
		run.ExecutionID = execCtx.ExecutionID
		run.Status = string(execCtx.Status)
		run.StartedAt = execCtx.StartTime
		run.EndedAt = execCtx.EndTime
	}
	if run.Status == "" {
		run.Status = string(ExecutionFailed)
	}
	if execErr != nil {
		run.Status = string(ExecutionFailed)
		run.Error = execErr.Error()
		now := time.Now().UTC()
		run.EndedAt = &now
	}
	return run
}

func cloneTriggerConfig(cfg TriggerConfig) TriggerConfig {
	cfg.EventTypes = append([]string(nil), cfg.EventTypes...)
	cfg.DefaultInputs = cloneAnyMap(cfg.DefaultInputs)
	return cfg
}

func clonePublicTriggerConfig(cfg TriggerConfig) TriggerConfig {
	cfg = cloneTriggerConfig(cfg)
	cfg.WebhookSecret = ""
	return cfg
}

func normalizeTriggerConfig(cfg TriggerConfig) TriggerConfig {
	cfg = cloneTriggerConfig(cfg)
	cfg.ID = strings.TrimSpace(cfg.ID)
	cfg.GraphID = strings.TrimSpace(cfg.GraphID)
	cfg.Type = TriggerType(strings.TrimSpace(string(cfg.Type)))
	cfg.Name = strings.TrimSpace(cfg.Name)
	cfg.CronExpr = strings.TrimSpace(cfg.CronExpr)
	cfg.Timezone = strings.TrimSpace(cfg.Timezone)
	cfg.WebhookPath = strings.TrimSpace(cfg.WebhookPath)
	cfg.WebhookSecret = strings.TrimSpace(cfg.WebhookSecret)
	cfg.EventSource = strings.TrimSpace(cfg.EventSource)
	cfg.EventFilter = strings.TrimSpace(cfg.EventFilter)
	eventTypes := make([]string, 0, len(cfg.EventTypes))
	for _, eventType := range cfg.EventTypes {
		eventType = strings.TrimSpace(eventType)
		if eventType != "" {
			eventTypes = append(eventTypes, eventType)
		}
	}
	cfg.EventTypes = eventTypes
	return cfg
}

func cloneTriggerEvent(event *WorkflowTriggerEvent) *WorkflowTriggerEvent {
	if event == nil {
		return nil
	}
	cloned := *event
	cloned.Payload = cloneAnyMap(event.Payload)
	return &cloned
}

func cloneTriggerRun(run TriggerRun) TriggerRun {
	run.Event = cloneTriggerEvent(run.Event)
	if run.EndedAt != nil {
		endedAt := *run.EndedAt
		run.EndedAt = &endedAt
	}
	return run
}

func cloneTriggerRunPtr(run TriggerRun) *TriggerRun {
	cloned := cloneTriggerRun(run)
	return &cloned
}
