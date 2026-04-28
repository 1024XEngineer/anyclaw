package workflow

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeGraphStore struct {
	graphs map[string]*Graph
	loads  []string
	err    error
}

func (s *fakeGraphStore) SaveGraph(graph *Graph) error {
	if s.graphs == nil {
		s.graphs = make(map[string]*Graph)
	}
	s.graphs[graph.ID] = graph
	return nil
}

func (s *fakeGraphStore) LoadGraph(graphID string) (*Graph, error) {
	s.loads = append(s.loads, graphID)
	if s.err != nil {
		return nil, s.err
	}
	graph, ok := s.graphs[graphID]
	if !ok {
		return nil, errors.New("not found")
	}
	return graph, nil
}

func (s *fakeGraphStore) DeleteGraph(graphID string) error {
	delete(s.graphs, graphID)
	return nil
}

func (s *fakeGraphStore) ListGraphs() ([]*Graph, error) {
	graphs := make([]*Graph, 0, len(s.graphs))
	for _, graph := range s.graphs {
		graphs = append(graphs, graph)
	}
	return graphs, nil
}

type fakeTriggerExecutor struct {
	inputs map[string]any
	graph  *Graph
	err    error
}

func (e *fakeTriggerExecutor) ExecuteGraph(graph *Graph, inputs map[string]any) (*ExecutionContext, error) {
	e.graph = graph
	e.inputs = cloneAnyMap(inputs)
	if e.err != nil {
		return nil, e.err
	}
	exec := NewExecutionContext(graph.ID, inputs)
	exec.MarkExecutionCompleted(map[string]any{"ok": true})
	return exec, nil
}

func TestAddGetListTriggersUseDefensiveCopies(t *testing.T) {
	tm := NewTriggerManager(nil, nil)
	defaults := map[string]any{"nested": map[string]any{"value": "before"}}

	if err := tm.AddTrigger(TriggerConfig{
		ID:            "manual-1",
		GraphID:       "graph-1",
		Type:          TriggerManual,
		Name:          "Manual",
		DefaultInputs: defaults,
	}); err != nil {
		t.Fatalf("AddTrigger: %v", err)
	}
	defaults["nested"].(map[string]any)["value"] = "after"

	cfg, ok := tm.GetTrigger("manual-1")
	if !ok {
		t.Fatal("expected trigger to exist")
	}
	if !cfg.Enabled {
		t.Fatal("expected trigger to be enabled by default")
	}
	if got := cfg.DefaultInputs["nested"].(map[string]any)["value"]; got != "before" {
		t.Fatalf("expected default inputs to be cloned, got %v", got)
	}

	cfg.DefaultInputs["nested"].(map[string]any)["value"] = "mutated"
	cfg, _ = tm.GetTrigger("manual-1")
	if got := cfg.DefaultInputs["nested"].(map[string]any)["value"]; got != "before" {
		t.Fatalf("GetTrigger leaked mutable state, got %v", got)
	}

	listed := tm.ListTriggers("graph-1")
	if len(listed) != 1 {
		t.Fatalf("expected one listed trigger, got %d", len(listed))
	}
	listed[0].DefaultInputs["nested"].(map[string]any)["value"] = "list-mutated"
	cfg, _ = tm.GetTrigger("manual-1")
	if got := cfg.DefaultInputs["nested"].(map[string]any)["value"]; got != "before" {
		t.Fatalf("ListTriggers leaked mutable state, got %v", got)
	}
}

func TestReadPathsRedactWebhookSecrets(t *testing.T) {
	tm := NewTriggerManager(nil, nil)
	if err := tm.AddTrigger(TriggerConfig{
		ID:            "webhook-1",
		GraphID:       "graph-1",
		Type:          TriggerWebhook,
		WebhookPath:   "/hooks/private",
		WebhookSecret: "secret-token",
	}); err != nil {
		t.Fatalf("AddTrigger: %v", err)
	}
	tm.SetHookFunc(func(context.Context, string, map[string]any) (*ExecutionContext, error) {
		exec := NewExecutionContext("graph-1", nil)
		exec.MarkExecutionCompleted(nil)
		return exec, nil
	})

	cfg, ok := tm.GetTrigger("webhook-1")
	if !ok {
		t.Fatal("expected trigger to exist")
	}
	if cfg.WebhookSecret != "" {
		t.Fatalf("GetTrigger leaked webhook secret %q", cfg.WebhookSecret)
	}

	listed := tm.ListTriggers("graph-1")
	if len(listed) != 1 || listed[0].WebhookSecret != "" {
		t.Fatalf("ListTriggers leaked webhook secret: %#v", listed)
	}

	webhooks := tm.GetWebhookTriggers()
	if len(webhooks) != 1 || webhooks[0].WebhookSecret != "" {
		t.Fatalf("GetWebhookTriggers leaked webhook secret: %#v", webhooks)
	}

	if _, err := tm.HandleWebhookWithSecret(context.Background(), "/hooks/private", "secret-token", nil); err != nil {
		t.Fatalf("redaction should not break internal secret matching: %v", err)
	}
}

func TestAddTriggerValidatesTypeSpecificFields(t *testing.T) {
	tm := NewTriggerManager(nil, nil)

	tests := []struct {
		name string
		cfg  TriggerConfig
		want string
	}{
		{
			name: "missing graph",
			cfg:  TriggerConfig{ID: "bad", Type: TriggerManual},
			want: "graph_id is required",
		},
		{
			name: "cron expression",
			cfg:  TriggerConfig{ID: "bad", GraphID: "graph-1", Type: TriggerCron},
			want: "cron_expr is required",
		},
		{
			name: "webhook path",
			cfg:  TriggerConfig{ID: "bad", GraphID: "graph-1", Type: TriggerWebhook},
			want: "webhook_path is required",
		},
		{
			name: "event source",
			cfg:  TriggerConfig{ID: "bad", GraphID: "graph-1", Type: TriggerEvent},
			want: "event_source is required",
		},
		{
			name: "unknown type",
			cfg:  TriggerConfig{ID: "bad", GraphID: "graph-1", Type: "unknown"},
			want: "unsupported trigger type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tm.AddTrigger(tt.cfg)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestEnableDisableAndDeleteTrigger(t *testing.T) {
	tm := NewTriggerManager(nil, nil)
	if err := tm.AddTrigger(TriggerConfig{ID: "manual-1", GraphID: "graph-1"}); err != nil {
		t.Fatalf("AddTrigger: %v", err)
	}

	if err := tm.DisableTrigger("manual-1"); err != nil {
		t.Fatalf("DisableTrigger: %v", err)
	}
	cfg, _ := tm.GetTrigger("manual-1")
	if cfg.Enabled {
		t.Fatal("expected disabled trigger")
	}

	if err := tm.EnableTrigger("manual-1"); err != nil {
		t.Fatalf("EnableTrigger: %v", err)
	}
	cfg, _ = tm.GetTrigger("manual-1")
	if !cfg.Enabled {
		t.Fatal("expected enabled trigger")
	}

	if err := tm.DeleteTrigger("manual-1"); err != nil {
		t.Fatalf("DeleteTrigger: %v", err)
	}
	if _, ok := tm.GetTrigger("manual-1"); ok {
		t.Fatal("expected trigger to be deleted")
	}
}

func TestFireTriggerUsesHookAndRecordsRun(t *testing.T) {
	tm := NewTriggerManager(nil, nil)
	if err := tm.AddTrigger(TriggerConfig{
		ID:            "manual-1",
		GraphID:       "graph-1",
		DefaultInputs: map[string]any{"mode": "default", "keep": true},
	}); err != nil {
		t.Fatalf("AddTrigger: %v", err)
	}

	tm.SetHookFunc(func(ctx context.Context, triggerID string, inputs map[string]any) (*ExecutionContext, error) {
		if triggerID != "manual-1" {
			t.Fatalf("triggerID = %q, want manual-1", triggerID)
		}
		if inputs["mode"] != "override" || inputs["keep"] != true {
			t.Fatalf("merged inputs = %#v", inputs)
		}
		exec := NewExecutionContext("graph-1", inputs)
		exec.MarkExecutionCompleted(map[string]any{"done": true})
		return exec, nil
	})

	run, err := tm.FireTrigger(context.Background(), "manual-1", map[string]any{"mode": "override"})
	if err != nil {
		t.Fatalf("FireTrigger: %v", err)
	}
	if run.TriggerID != "manual-1" || run.TriggeredBy != "manual" {
		t.Fatalf("unexpected run metadata: %#v", run)
	}
	if run.Status != string(ExecutionCompleted) {
		t.Fatalf("status = %q, want completed", run.Status)
	}

	runs := tm.GetRuns("", 1)
	if len(runs) != 1 || runs[0].ExecutionID != run.ExecutionID {
		t.Fatalf("expected recorded run, got %#v", runs)
	}
}

func TestFireTriggerUsesGraphStoreAndExecutor(t *testing.T) {
	graph := NewGraph("executor", "")
	graph.ID = "graph-1"
	graph.AddActionNode("run", "", "plugin", "action", nil)
	store := &fakeGraphStore{graphs: map[string]*Graph{"graph-1": graph}}
	executor := &fakeTriggerExecutor{}
	tm := NewTriggerManager(executor, store)

	if err := tm.AddTrigger(TriggerConfig{
		ID:            "manual-1",
		GraphID:       "graph-1",
		DefaultInputs: map[string]any{"base": "yes"},
	}); err != nil {
		t.Fatalf("AddTrigger: %v", err)
	}

	run, err := tm.FireTrigger(context.Background(), "manual-1", map[string]any{"request": "go"})
	if err != nil {
		t.Fatalf("FireTrigger: %v", err)
	}
	if run.Status != string(ExecutionCompleted) {
		t.Fatalf("status = %q, want completed", run.Status)
	}
	if len(store.loads) != 1 || store.loads[0] != "graph-1" {
		t.Fatalf("store loads = %v", store.loads)
	}
	if executor.graph == nil || executor.graph.ID != "graph-1" {
		t.Fatalf("executor graph = %#v", executor.graph)
	}
	if executor.inputs["base"] != "yes" || executor.inputs["request"] != "go" {
		t.Fatalf("executor inputs = %#v", executor.inputs)
	}
}

func TestFireTriggerRecordsFailureRun(t *testing.T) {
	tm := NewTriggerManager(nil, nil)
	if err := tm.AddTrigger(TriggerConfig{ID: "manual-1", GraphID: "graph-1"}); err != nil {
		t.Fatalf("AddTrigger: %v", err)
	}
	tm.SetHookFunc(func(context.Context, string, map[string]any) (*ExecutionContext, error) {
		return nil, errors.New("boom")
	})

	run, err := tm.FireTrigger(context.Background(), "manual-1", nil)
	if err == nil {
		t.Fatal("expected hook error")
	}
	if run == nil {
		t.Fatal("expected failed run to be returned")
	}
	if run.Status != string(ExecutionFailed) || run.Error != "boom" || run.EndedAt == nil {
		t.Fatalf("unexpected failed run: %#v", run)
	}

	runs := tm.GetRuns("manual-1", 1)
	if len(runs) != 1 || runs[0].Error != "boom" {
		t.Fatalf("expected failure in run history, got %#v", runs)
	}
}

func TestFireTriggerRejectsDisabledOrMissingExecutionBackend(t *testing.T) {
	tm := NewTriggerManager(nil, nil)
	if err := tm.AddTrigger(TriggerConfig{ID: "manual-1", GraphID: "graph-1"}); err != nil {
		t.Fatalf("AddTrigger: %v", err)
	}
	if err := tm.DisableTrigger("manual-1"); err != nil {
		t.Fatalf("DisableTrigger: %v", err)
	}

	if _, err := tm.FireTrigger(context.Background(), "manual-1", nil); err == nil {
		t.Fatal("expected disabled trigger error")
	}

	if err := tm.EnableTrigger("manual-1"); err != nil {
		t.Fatalf("EnableTrigger: %v", err)
	}
	run, err := tm.FireTrigger(context.Background(), "manual-1", nil)
	if err == nil || !strings.Contains(err.Error(), "graph store is nil") {
		t.Fatalf("expected missing graph store error, got run=%#v err=%v", run, err)
	}
	if run == nil || run.Status != string(ExecutionFailed) {
		t.Fatalf("expected failed run for missing backend, got %#v", run)
	}
}

func TestHandleWebhookFiresMatchingTriggers(t *testing.T) {
	tm := NewTriggerManager(nil, nil)
	if err := tm.AddTrigger(TriggerConfig{
		ID:            "webhook-1",
		GraphID:       "graph-1",
		Type:          TriggerWebhook,
		WebhookPath:   "/hooks/deploy",
		DefaultInputs: map[string]any{"source": "default"},
	}); err != nil {
		t.Fatalf("AddTrigger: %v", err)
	}
	tm.SetHookFunc(func(ctx context.Context, triggerID string, inputs map[string]any) (*ExecutionContext, error) {
		if inputs["source"] != "payload" {
			t.Fatalf("payload should override defaults, got %#v", inputs)
		}
		exec := NewExecutionContext("graph-1", inputs)
		exec.MarkExecutionCompleted(nil)
		return exec, nil
	})

	payload := map[string]any{"source": "payload"}
	runs, err := tm.HandleWebhook(context.Background(), "/hooks/deploy", payload)
	if err != nil {
		t.Fatalf("HandleWebhook: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected one run, got %d", len(runs))
	}
	if runs[0].TriggeredBy != "webhook" || runs[0].Event == nil || runs[0].Event.Source != "webhook" {
		t.Fatalf("unexpected webhook run: %#v", runs[0])
	}
	payload["source"] = "mutated"
	if runs[0].Event.Payload["source"] != "payload" {
		t.Fatalf("webhook event payload was not cloned: %#v", runs[0].Event.Payload)
	}
}

func TestHandleWebhookEnforcesConfiguredSecret(t *testing.T) {
	tm := NewTriggerManager(nil, nil)
	if err := tm.AddTrigger(TriggerConfig{
		ID:            "webhook-1",
		GraphID:       "graph-1",
		Type:          TriggerWebhook,
		WebhookPath:   "/hooks/private",
		WebhookSecret: "secret-token",
	}); err != nil {
		t.Fatalf("AddTrigger: %v", err)
	}
	tm.SetHookFunc(func(context.Context, string, map[string]any) (*ExecutionContext, error) {
		exec := NewExecutionContext("graph-1", nil)
		exec.MarkExecutionCompleted(nil)
		return exec, nil
	})

	if _, err := tm.HandleWebhook(context.Background(), "/hooks/private", nil); err == nil {
		t.Fatal("expected secret-protected webhook to reject missing secret")
	}
	if _, err := tm.HandleWebhookWithSecret(context.Background(), "/hooks/private", "wrong-token", nil); err == nil {
		t.Fatal("expected secret-protected webhook to reject wrong secret")
	}
	runs, err := tm.HandleWebhookWithSecret(context.Background(), "/hooks/private", "secret-token", nil)
	if err != nil {
		t.Fatalf("HandleWebhookWithSecret: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected one authenticated webhook run, got %d", len(runs))
	}
}

func TestHandleEventFiltersByTypeAndExpression(t *testing.T) {
	tm := NewTriggerManager(nil, nil)
	if err := tm.AddTrigger(TriggerConfig{
		ID:          "event-1",
		GraphID:     "graph-1",
		Type:        TriggerEvent,
		EventSource: "github",
		EventTypes:  []string{"pull_request"},
		EventFilter: "$approved == true",
	}); err != nil {
		t.Fatalf("AddTrigger: %v", err)
	}
	tm.SetHookFunc(func(ctx context.Context, triggerID string, inputs map[string]any) (*ExecutionContext, error) {
		if inputs["_event_source"] != "github" || inputs["_event_type"] != "pull_request" {
			t.Fatalf("event metadata inputs = %#v", inputs)
		}
		exec := NewExecutionContext("graph-1", inputs)
		exec.MarkExecutionCompleted(nil)
		return exec, nil
	})

	runs, err := tm.HandleEvent(context.Background(), &WorkflowTriggerEvent{
		ID:      "evt-1",
		Source:  "github",
		Type:    "pull_request",
		Payload: map[string]any{"approved": true},
	})
	if err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if len(runs) != 1 || runs[0].TriggeredBy != "event" {
		t.Fatalf("unexpected event runs: %#v", runs)
	}

	if _, err := tm.HandleEvent(context.Background(), &WorkflowTriggerEvent{
		Source:  "github",
		Type:    "pull_request",
		Payload: map[string]any{"approved": false},
	}); err == nil {
		t.Fatal("expected no matching event trigger")
	}
}

func TestHandleEventReturnsInvalidFilterError(t *testing.T) {
	tm := NewTriggerManager(nil, nil)
	if err := tm.AddTrigger(TriggerConfig{
		ID:          "event-1",
		GraphID:     "graph-1",
		Type:        TriggerEvent,
		EventSource: "github",
		EventTypes:  []string{"pull_request"},
		EventFilter: "$approved &&",
	}); err != nil {
		t.Fatalf("AddTrigger: %v", err)
	}

	_, err := tm.HandleEvent(context.Background(), &WorkflowTriggerEvent{
		Source:  "github",
		Type:    "pull_request",
		Payload: map[string]any{"approved": true},
	})
	if err == nil {
		t.Fatal("expected invalid event_filter error")
	}
	if !strings.Contains(err.Error(), `event_filter for trigger "event-1"`) {
		t.Fatalf("expected trigger-specific filter error, got %v", err)
	}
}

func TestGetRunsReturnsNewestFirstAndClones(t *testing.T) {
	tm := NewTriggerManager(nil, nil)
	if err := tm.AddTrigger(TriggerConfig{ID: "manual-1", GraphID: "graph-1"}); err != nil {
		t.Fatalf("AddTrigger: %v", err)
	}
	count := 0
	tm.SetHookFunc(func(context.Context, string, map[string]any) (*ExecutionContext, error) {
		count++
		exec := NewExecutionContext("graph-1", nil)
		exec.ExecutionID = strings.Join([]string{"exec", string(rune('0' + count))}, "-")
		exec.MarkExecutionCompleted(nil)
		return exec, nil
	})
	if _, err := tm.FireTrigger(context.Background(), "manual-1", nil); err != nil {
		t.Fatalf("FireTrigger 1: %v", err)
	}
	if _, err := tm.FireTrigger(context.Background(), "manual-1", nil); err != nil {
		t.Fatalf("FireTrigger 2: %v", err)
	}

	runs := tm.GetRuns("manual-1", 1)
	if len(runs) != 1 || runs[0].ExecutionID != "exec-2" {
		t.Fatalf("expected newest limited run, got %#v", runs)
	}
	runs[0].Status = "mutated"
	runs = tm.GetRuns("manual-1", 1)
	if runs[0].Status == "mutated" {
		t.Fatal("GetRuns leaked mutable run state")
	}
}

func TestCronWebhookHelpersAndStats(t *testing.T) {
	tm := NewTriggerManager(nil, nil)
	if err := tm.AddTrigger(TriggerConfig{
		ID:       "cron-1",
		GraphID:  "graph-1",
		Type:     TriggerCron,
		CronExpr: "*/5 * * * *",
	}); err != nil {
		t.Fatalf("AddTrigger cron: %v", err)
	}
	if err := tm.AddTrigger(TriggerConfig{
		ID:          "webhook-1",
		GraphID:     "graph-1",
		Type:        TriggerWebhook,
		WebhookPath: "/hook",
	}); err != nil {
		t.Fatalf("AddTrigger webhook: %v", err)
	}
	if err := tm.DisableTrigger("webhook-1"); err != nil {
		t.Fatalf("DisableTrigger: %v", err)
	}

	cron := tm.GetCronTriggers()
	if len(cron) != 1 || cron[0].ID != "cron-1" {
		t.Fatalf("cron triggers = %#v", cron)
	}
	webhooks := tm.GetWebhookTriggers()
	if len(webhooks) != 0 {
		t.Fatalf("expected disabled webhook to be excluded, got %#v", webhooks)
	}

	stats := tm.Stats()
	if stats["total_triggers"] != 2 {
		t.Fatalf("stats total_triggers = %#v", stats["total_triggers"])
	}
	byType := stats["by_type"].(map[string]int)
	if byType[string(TriggerCron)] != 1 || byType[string(TriggerWebhook)] != 1 {
		t.Fatalf("stats by_type = %#v", byType)
	}
	byStatus := stats["by_status"].(map[string]int)
	if byStatus["enabled"] != 1 || byStatus["disabled"] != 1 {
		t.Fatalf("stats by_status = %#v", byStatus)
	}
}

func TestFireTriggerHonorsContextCancellation(t *testing.T) {
	tm := NewTriggerManager(nil, nil)
	if err := tm.AddTrigger(TriggerConfig{ID: "manual-1", GraphID: "graph-1"}); err != nil {
		t.Fatalf("AddTrigger: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := tm.FireTrigger(ctx, "manual-1", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}

func TestFireTriggerEnforcesMaxRuns(t *testing.T) {
	tm := NewTriggerManager(nil, nil)
	if err := tm.AddTrigger(TriggerConfig{
		ID:      "manual-1",
		GraphID: "graph-1",
		MaxRuns: 1,
	}); err != nil {
		t.Fatalf("AddTrigger: %v", err)
	}
	tm.SetHookFunc(func(context.Context, string, map[string]any) (*ExecutionContext, error) {
		exec := NewExecutionContext("graph-1", nil)
		exec.MarkExecutionCompleted(nil)
		return exec, nil
	})
	if _, err := tm.FireTrigger(context.Background(), "manual-1", nil); err != nil {
		t.Fatalf("FireTrigger first: %v", err)
	}
	if _, err := tm.FireTrigger(context.Background(), "manual-1", nil); err == nil {
		t.Fatal("expected max_runs error")
	}
}

func TestFireTriggerMaxRunsCountsInFlightRuns(t *testing.T) {
	tm := NewTriggerManager(nil, nil)
	if err := tm.AddTrigger(TriggerConfig{
		ID:      "manual-1",
		GraphID: "graph-1",
		MaxRuns: 1,
	}); err != nil {
		t.Fatalf("AddTrigger: %v", err)
	}

	entered := make(chan struct{})
	release := make(chan struct{})
	var enteredOnce sync.Once
	tm.SetHookFunc(func(ctx context.Context, _ string, _ map[string]any) (*ExecutionContext, error) {
		enteredOnce.Do(func() { close(entered) })
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		exec := NewExecutionContext("graph-1", nil)
		exec.MarkExecutionCompleted(nil)
		return exec, nil
	})

	firstDone := make(chan error, 1)
	go func() {
		_, err := tm.FireTrigger(context.Background(), "manual-1", nil)
		firstDone <- err
	}()

	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("first trigger did not enter hook")
	}

	secondDone := make(chan error, 1)
	go func() {
		_, err := tm.FireTrigger(context.Background(), "manual-1", nil)
		secondDone <- err
	}()

	var secondErr error
	select {
	case secondErr = <-secondDone:
	case <-time.After(200 * time.Millisecond):
		secondErr = errors.New("second FireTrigger blocked instead of observing in-flight max_runs")
	}

	close(release)

	if err := <-firstDone; err != nil {
		t.Fatalf("first FireTrigger: %v", err)
	}
	if secondErr != nil && strings.Contains(secondErr.Error(), "blocked") {
		<-secondDone
	}
	if secondErr == nil || !strings.Contains(secondErr.Error(), "max_runs") {
		t.Fatalf("expected in-flight max_runs rejection, got %v", secondErr)
	}
}

func TestFireTriggerAppliesTimeoutContext(t *testing.T) {
	tm := NewTriggerManager(nil, nil)
	if err := tm.AddTrigger(TriggerConfig{
		ID:         "manual-1",
		GraphID:    "graph-1",
		TimeoutSec: 1,
	}); err != nil {
		t.Fatalf("AddTrigger: %v", err)
	}
	tm.SetHookFunc(func(ctx context.Context, _ string, _ map[string]any) (*ExecutionContext, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("expected timeout deadline to be applied")
		}
		exec := NewExecutionContext("graph-1", nil)
		exec.MarkExecutionCompleted(nil)
		return exec, nil
	})
	if _, err := tm.FireTrigger(context.Background(), "manual-1", nil); err != nil {
		t.Fatalf("FireTrigger: %v", err)
	}
}

func TestFireTriggerRejectsNilHookExecution(t *testing.T) {
	tm := NewTriggerManager(nil, nil)
	if err := tm.AddTrigger(TriggerConfig{ID: "manual-1", GraphID: "graph-1"}); err != nil {
		t.Fatalf("AddTrigger: %v", err)
	}
	tm.SetHookFunc(func(context.Context, string, map[string]any) (*ExecutionContext, error) {
		return nil, nil
	})
	run, err := tm.FireTrigger(nil, "manual-1", nil)
	if err == nil || !strings.Contains(err.Error(), "nil execution") {
		t.Fatalf("expected nil execution error, got run=%#v err=%v", run, err)
	}
	if run == nil || run.Status != string(ExecutionFailed) {
		t.Fatalf("expected failed run, got %#v", run)
	}
}

func TestNilTriggerManagerIsSafe(t *testing.T) {
	var tm *TriggerManager
	if err := tm.AddTrigger(TriggerConfig{}); err == nil {
		t.Fatal("expected AddTrigger nil manager error")
	}
	if _, ok := tm.GetTrigger("x"); ok {
		t.Fatal("expected no trigger from nil manager")
	}
	if got := tm.ListTriggers(""); got != nil {
		t.Fatalf("expected nil list, got %#v", got)
	}
	if stats := tm.Stats(); stats["total_triggers"] != 0 {
		t.Fatalf("unexpected nil stats: %#v", stats)
	}
	tm.SetHookFunc(nil)
}

func TestHandleEventRejectsNilEvent(t *testing.T) {
	tm := NewTriggerManager(nil, nil)
	if _, err := tm.HandleEvent(context.Background(), nil); err == nil {
		t.Fatal("expected nil event error")
	}
}

func TestAddTriggerRejectsDuplicateIDs(t *testing.T) {
	tm := NewTriggerManager(nil, nil)
	cfg := TriggerConfig{ID: "manual-1", GraphID: "graph-1"}
	if err := tm.AddTrigger(cfg); err != nil {
		t.Fatalf("AddTrigger first: %v", err)
	}
	if err := tm.AddTrigger(cfg); err == nil {
		t.Fatal("expected duplicate trigger error")
	}
}

func TestHandleWebhookReturnsNoMatchError(t *testing.T) {
	tm := NewTriggerManager(nil, nil)
	_, err := tm.HandleWebhook(context.Background(), "/missing", nil)
	if err == nil {
		t.Fatal("expected no webhook match error")
	}
}

func TestTriggerEventTimestampDefaultsWhenMissing(t *testing.T) {
	tm := NewTriggerManager(nil, nil)
	if err := tm.AddTrigger(TriggerConfig{
		ID:          "event-1",
		GraphID:     "graph-1",
		Type:        TriggerEvent,
		EventSource: "system",
		EventTypes:  []string{"*"},
	}); err != nil {
		t.Fatalf("AddTrigger: %v", err)
	}
	tm.SetHookFunc(func(context.Context, string, map[string]any) (*ExecutionContext, error) {
		exec := NewExecutionContext("graph-1", nil)
		exec.MarkExecutionCompleted(nil)
		return exec, nil
	})

	runs, err := tm.HandleEvent(context.Background(), &WorkflowTriggerEvent{
		Source: "system",
		Type:   "tick",
	})
	if err != nil {
		t.Fatalf("HandleEvent: %v", err)
	}
	if len(runs) != 1 || runs[0].Event.Timestamp.IsZero() {
		t.Fatalf("expected default event timestamp, got %#v", runs)
	}
	if time.Since(runs[0].Event.Timestamp) > time.Minute {
		t.Fatalf("default timestamp too old: %s", runs[0].Event.Timestamp)
	}
}
