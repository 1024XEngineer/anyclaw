package schedule

import (
	"context"
	"errors"
	"testing"
	"time"
)

type stubExecutor struct {
	fn    func(ctx context.Context, cmd string, input map[string]interface{}) (string, error)
	calls int
}

func (s *stubExecutor) Execute(ctx context.Context, cmd string, input map[string]interface{}) (string, error) {
	s.calls++
	if s.fn != nil {
		return s.fn(ctx, cmd, input)
	}
	return "", nil
}

type stubPersister struct {
	loadTasksFn func() ([]*Task, error)
	loadRunsFn  func() ([]*TaskRun, error)
	savedRuns   []*TaskRun
}

func (s *stubPersister) SaveTasks(tasks []*Task) error {
	return nil
}

func (s *stubPersister) LoadTasks() ([]*Task, error) {
	if s.loadTasksFn != nil {
		return s.loadTasksFn()
	}
	return nil, nil
}

func (s *stubPersister) SaveRuns(runs []*TaskRun) error {
	s.savedRuns = append([]*TaskRun(nil), runs...)
	return nil
}

func (s *stubPersister) LoadRuns() ([]*TaskRun, error) {
	if s.loadRunsFn != nil {
		return s.loadRunsFn()
	}
	return nil, nil
}

func TestParseCronExprAliasesAndInvalid(t *testing.T) {
	tests := []struct {
		expr   string
		minute int
		hour   int
	}{
		{expr: "@hourly", minute: 0, hour: -1},
		{expr: "@daily", minute: 0, hour: 0},
		{expr: "@weekly", minute: 0, hour: 0},
		{expr: "@monthly", minute: 0, hour: 0},
	}

	for _, tt := range tests {
		parsed, err := ParseCronExpr(tt.expr)
		if err != nil {
			t.Fatalf("ParseCronExpr(%q): %v", tt.expr, err)
		}
		if parsed.Minute != tt.minute || parsed.Hour != tt.hour {
			t.Fatalf("unexpected parsed cron for %q: %+v", tt.expr, parsed)
		}
	}

	if _, err := ParseCronExpr("not-a-cron"); err == nil {
		t.Fatal("expected invalid cron expression to fail")
	}
}

func TestCronExprNextAndTaskLifecycle(t *testing.T) {
	scheduler := New()
	taskID, err := scheduler.AddTask(&Task{
		Schedule: "@daily",
		Command:  "echo hi",
	})
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}

	task, ok := scheduler.GetTask(taskID)
	if !ok {
		t.Fatalf("expected task %s to exist", taskID)
	}
	if task.Name != taskID {
		t.Fatalf("expected task name to default to id, got %q", task.Name)
	}
	if !task.Enabled {
		t.Fatal("expected added task to be enabled")
	}
	if task.MaxRetries != 3 || task.Timeout != 300 {
		t.Fatalf("expected defaults max_retries=3 timeout=300, got %+v", task)
	}
	if task.NextRun == nil {
		t.Fatal("expected next run to be scheduled")
	}

	next := calculateNextRun("@daily", time.Date(2026, 4, 23, 10, 15, 0, 0, time.UTC))
	if !next.After(time.Date(2026, 4, 23, 10, 15, 0, 0, time.UTC)) {
		t.Fatalf("expected next run to be after the source time, got %v", next)
	}

	if err := scheduler.DisableTask(taskID); err != nil {
		t.Fatalf("DisableTask: %v", err)
	}
	if err := scheduler.EnableTask(taskID); err != nil {
		t.Fatalf("EnableTask: %v", err)
	}

	updated := *task
	updated.Command = "echo updated"
	if err := scheduler.UpdateTask(&updated); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	updatedSchedule := *scheduler.tasks[taskID]
	updatedSchedule.Schedule = "@hourly"
	if err := scheduler.UpdateTask(&updatedSchedule); err == nil {
		t.Fatal("expected changing an existing schedule to fail")
	}

	nextRuns := scheduler.NextRunTimes(10)
	if _, ok := nextRuns[taskID]; !ok {
		t.Fatalf("expected next run time for task %s", taskID)
	}
	if len(scheduler.ListTasks()) != 1 {
		t.Fatalf("expected one task in scheduler, got %d", len(scheduler.ListTasks()))
	}

	if err := scheduler.ClearHistory(taskID); err != nil {
		t.Fatalf("ClearHistory: %v", err)
	}
	if err := scheduler.DeleteTask(taskID); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	if _, ok := scheduler.GetTask(taskID); ok {
		t.Fatalf("expected task %s to be deleted", taskID)
	}
}

func TestSchedulerRunTaskSuccessAndFailure(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		persister := &stubPersister{}
		scheduler := NewScheduler(nil)
		scheduler.SetPersister(persister)

		taskID, err := scheduler.AddTask(&Task{
			ID:         "task-success",
			Schedule:   "@hourly",
			Command:    "noop",
			MaxRetries: 0,
			Timeout:    1,
		})
		if err != nil {
			t.Fatalf("AddTask: %v", err)
		}
		scheduler.tasks[taskID].MaxRetries = 0

		scheduler.runTask(taskID)

		runs := scheduler.GetTaskRuns(taskID)
		if len(runs) != 1 {
			t.Fatalf("expected one run, got %d", len(runs))
		}
		if runs[0].Status != "success" || runs[0].Output != "No executor configured" {
			t.Fatalf("unexpected success run: %+v", runs[0])
		}
		if runs[0].EndTime == nil {
			t.Fatal("expected end time to be recorded")
		}
		if len(persister.savedRuns) != 1 {
			t.Fatalf("expected persister to save runs, got %d", len(persister.savedRuns))
		}
		if got := scheduler.GetRunHistory(taskID, 1); len(got) != 1 {
			t.Fatalf("expected run history entry, got %d", len(got))
		}
	})

	t.Run("failure", func(t *testing.T) {
		executor := &stubExecutor{
			fn: func(ctx context.Context, cmd string, input map[string]interface{}) (string, error) {
				return "partial output", errors.New("boom")
			},
		}
		scheduler := NewScheduler(executor)
		taskID, err := scheduler.AddTask(&Task{
			ID:         "task-failure",
			Schedule:   "@hourly",
			Command:    "explode",
			MaxRetries: 0,
			Timeout:    1,
		})
		if err != nil {
			t.Fatalf("AddTask: %v", err)
		}
		scheduler.tasks[taskID].MaxRetries = 0

		scheduler.runTask(taskID)

		runs := scheduler.GetTaskRuns(taskID)
		if len(runs) != 1 {
			t.Fatalf("expected one run, got %d", len(runs))
		}
		if runs[0].Status != "failed" || runs[0].Error != "boom" || runs[0].Output != "partial output" {
			t.Fatalf("unexpected failed run: %+v", runs[0])
		}
		if executor.calls != 1 {
			t.Fatalf("expected executor to be called once, got %d", executor.calls)
		}
		if got := scheduler.GetAllRuns(10); len(got) != 1 {
			t.Fatalf("expected one aggregated run, got %d", len(got))
		}
	})
}

func TestSchedulerLoadPersistedAndRetryHelpers(t *testing.T) {
	when := time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)
	persister := &stubPersister{
		loadTasksFn: func() ([]*Task, error) {
			return []*Task{{ID: "task-1", Name: "loaded", Schedule: "@daily", Enabled: true, NextRun: &when}}, nil
		},
		loadRunsFn: func() ([]*TaskRun, error) {
			return []*TaskRun{{ID: "run-1", TaskID: "task-1", StartTime: when, Status: "success"}}, nil
		},
	}
	scheduler := New()
	scheduler.SetPersister(persister)

	if err := scheduler.LoadPersisted(); err != nil {
		t.Fatalf("LoadPersisted: %v", err)
	}
	if len(scheduler.ListTasks()) != 1 {
		t.Fatalf("expected one loaded task, got %d", len(scheduler.ListTasks()))
	}
	if len(scheduler.GetTaskRuns("task-1")) != 1 {
		t.Fatalf("expected one loaded run, got %d", len(scheduler.GetTaskRuns("task-1")))
	}
	if delay := scheduler.calculateRetryDelay(0, &Task{RetryBackoff: "exponential"}); delay != time.Second {
		t.Fatalf("expected exponential retry delay 1s, got %v", delay)
	}
	if delay := scheduler.calculateRetryDelay(2, &Task{RetryBackoff: "linear"}); delay != 3*time.Second {
		t.Fatalf("expected linear retry delay 3s, got %v", delay)
	}
	if delay := scheduler.calculateRetryDelay(5, &Task{}); delay != 2*time.Second {
		t.Fatalf("expected fixed retry delay 2s, got %v", delay)
	}
}

func TestSchedulerStartAndStop(t *testing.T) {
	scheduler := New()

	if err := scheduler.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := scheduler.Start(); err == nil {
		t.Fatal("expected second start to fail")
	}
	if !scheduler.running {
		t.Fatal("expected scheduler to be marked running")
	}
	scheduler.Stop()
	if scheduler.running {
		t.Fatal("expected scheduler to stop running")
	}
	scheduler.Stop()
}
