package schedule

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

type stubExecutor struct {
	mu      sync.Mutex
	output  string
	err     error
	blockCh chan struct{}
	calls   int
}

func (s *stubExecutor) Execute(ctx context.Context, cmd string, input map[string]any) (string, error) {
	s.mu.Lock()
	s.calls++
	blockCh := s.blockCh
	output := s.output
	err := s.err
	s.mu.Unlock()

	if blockCh != nil {
		select {
		case <-blockCh:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return output, err
}

func TestSchedulerAddTaskAndCopies(t *testing.T) {
	scheduler := New()
	taskID, err := scheduler.AddTask(&Task{
		Name:     "hourly",
		Schedule: "0 * * * *",
		Command:  "echo hi",
		Input:    map[string]any{"k": "v"},
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("AddTask failed: %v", err)
	}

	task, ok := scheduler.GetTask(taskID)
	if !ok {
		t.Fatalf("GetTask(%s) returned not found", taskID)
	}
	task.Input["k"] = "mutated"

	again, ok := scheduler.GetTask(taskID)
	if !ok {
		t.Fatalf("GetTask(%s) returned not found on second lookup", taskID)
	}
	if got := again.Input["k"]; got != "v" {
		t.Fatalf("expected defensive copy, got %v", got)
	}

	listed := scheduler.ListTasks()
	listed[0].Name = "changed"
	again, _ = scheduler.GetTask(taskID)
	if again.Name != "hourly" {
		t.Fatalf("expected list copy to be isolated, got %q", again.Name)
	}
}

func TestSchedulerRunTaskNowAndCancel(t *testing.T) {
	executor := &stubExecutor{blockCh: make(chan struct{})}
	scheduler := NewScheduler(executor)

	taskID, err := scheduler.AddTask(&Task{
		Name:     "blocking",
		Schedule: "@every 1m",
		Command:  "sleep",
		Timeout:  1,
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("AddTask failed: %v", err)
	}

	if err := scheduler.RunTaskNow(taskID); err != nil {
		t.Fatalf("RunTaskNow failed: %v", err)
	}

	var runID string
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		runs := scheduler.GetTaskRuns(taskID)
		if len(runs) > 0 {
			runID = runs[0].ID
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if runID == "" {
		t.Fatal("expected run to be recorded")
	}

	if err := scheduler.CancelRun(runID); err != nil {
		t.Fatalf("CancelRun failed: %v", err)
	}
	close(executor.blockCh)

	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		runs := scheduler.GetTaskRuns(taskID)
		if len(runs) > 0 && runs[0].Status == "cancelled" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("expected run to become cancelled")
}

func TestSchedulerStartStopRestart(t *testing.T) {
	scheduler := New()
	if err := scheduler.Start(); err != nil {
		t.Fatalf("first Start failed: %v", err)
	}
	scheduler.Stop()
	if err := scheduler.Start(); err != nil {
		t.Fatalf("second Start failed: %v", err)
	}
	scheduler.Stop()
}

func TestSchedulerMarshalJSON(t *testing.T) {
	scheduler := New()
	if _, err := scheduler.AddTask(&Task{
		Name:     "json",
		Schedule: "@hourly",
		Command:  "echo",
		Enabled:  true,
	}); err != nil {
		t.Fatalf("AddTask failed: %v", err)
	}

	data, err := json.Marshal(scheduler)
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty JSON payload")
	}
}

func TestSchedulerLoadPersisted(t *testing.T) {
	persister, err := NewFilePersister(t.TempDir())
	if err != nil {
		t.Fatalf("NewFilePersister failed: %v", err)
	}

	savedNext := time.Date(2026, 4, 22, 11, 0, 0, 0, time.UTC)
	if err := persister.SaveTasks([]*Task{{
		ID:       "task-1",
		Name:     "persisted",
		Schedule: "0 * * * *",
		Command:  "echo",
		Enabled:  true,
		NextRun:  &savedNext,
	}}); err != nil {
		t.Fatalf("SaveTasks failed: %v", err)
	}
	if err := persister.SaveRuns([]*TaskRun{{
		ID:        "run-1",
		TaskID:    "task-1",
		StartTime: time.Now().UTC(),
		Status:    "success",
	}}); err != nil {
		t.Fatalf("SaveRuns failed: %v", err)
	}

	scheduler := New()
	scheduler.SetPersister(persister)
	if err := scheduler.LoadPersisted(); err != nil {
		t.Fatalf("LoadPersisted failed: %v", err)
	}

	tasks := scheduler.ListTasks()
	if len(tasks) != 1 || tasks[0].ID != "task-1" {
		t.Fatalf("unexpected tasks after load: %+v", tasks)
	}
	runs := scheduler.GetTaskRuns("task-1")
	if len(runs) != 1 || runs[0].ID != "run-1" {
		t.Fatalf("unexpected runs after load: %+v", runs)
	}
}

func TestSchedulerRetryAndStats(t *testing.T) {
	executor := &stubExecutor{err: errors.New("boom")}
	scheduler := NewScheduler(executor)

	taskID, err := scheduler.AddTask(&Task{
		Name:         "retrying",
		Schedule:     "@every 1m",
		Command:      "echo",
		MaxRetries:   1,
		RetryBackoff: "linear",
		Timeout:      5,
		Enabled:      true,
	})
	if err != nil {
		t.Fatalf("AddTask failed: %v", err)
	}

	if err := scheduler.RunTaskNow(taskID); err != nil {
		t.Fatalf("RunTaskNow failed: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		runs := scheduler.GetTaskRuns(taskID)
		if len(runs) > 0 && runs[0].Status == "failed" {
			stats := scheduler.Stats()
			if stats["failed_runs"] != 1 {
				t.Fatalf("expected failed_runs=1, got %+v", stats)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("expected run to fail")
}
