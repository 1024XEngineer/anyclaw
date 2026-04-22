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
	started chan struct{}
	calls   int
}

func (s *stubExecutor) Execute(ctx context.Context, cmd string, input map[string]any) (string, error) {
	s.mu.Lock()
	s.calls++
	blockCh := s.blockCh
	output := s.output
	err := s.err
	started := s.started
	s.mu.Unlock()

	if started != nil {
		select {
		case <-started:
		default:
			close(started)
		}
	}

	if blockCh != nil {
		select {
		case <-blockCh:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return output, err
}

type failingPersister struct {
	saveTasksErr error
	saveRunsErr  error
}

func (p *failingPersister) SaveTasks(tasks []*Task) error  { return p.saveTasksErr }
func (p *failingPersister) LoadTasks() ([]*Task, error)    { return nil, nil }
func (p *failingPersister) SaveRuns(runs []*TaskRun) error { return p.saveRunsErr }
func (p *failingPersister) LoadRuns() ([]*TaskRun, error)  { return nil, nil }

type loadFailingPersister struct {
	loadTasksErr error
	loadRunsErr  error
}

func (p *loadFailingPersister) SaveTasks(tasks []*Task) error  { return nil }
func (p *loadFailingPersister) LoadTasks() ([]*Task, error)    { return nil, p.loadTasksErr }
func (p *loadFailingPersister) SaveRuns(runs []*TaskRun) error { return nil }
func (p *loadFailingPersister) LoadRuns() ([]*TaskRun, error)  { return nil, p.loadRunsErr }

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

func TestSchedulerUpdateTaskCanDisableTask(t *testing.T) {
	scheduler := New()
	taskID, err := scheduler.AddTask(&Task{
		Name:     "toggle",
		Schedule: "@hourly",
		Command:  "echo",
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("AddTask failed: %v", err)
	}

	if err := scheduler.UpdateTask(&Task{
		ID:       taskID,
		Name:     "toggle",
		Schedule: "@hourly",
		Command:  "echo",
		Enabled:  false,
	}); err != nil {
		t.Fatalf("UpdateTask failed: %v", err)
	}

	task, ok := scheduler.GetTask(taskID)
	if !ok {
		t.Fatalf("GetTask(%s) returned not found", taskID)
	}
	if task.Enabled {
		t.Fatal("expected task to be disabled after update")
	}
	if task.NextRun != nil {
		t.Fatalf("expected disabled task to have nil next run, got %v", *task.NextRun)
	}
}

func TestSchedulerDeleteTaskCancelsActiveRun(t *testing.T) {
	executor := &stubExecutor{
		blockCh: make(chan struct{}),
		started: make(chan struct{}),
	}
	scheduler := NewScheduler(executor)

	taskID, err := scheduler.AddTask(&Task{
		Name:     "delete-running",
		Schedule: "@every 1m",
		Command:  "sleep",
		Timeout:  5,
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("AddTask failed: %v", err)
	}
	if err := scheduler.RunTaskNow(taskID); err != nil {
		t.Fatalf("RunTaskNow failed: %v", err)
	}

	select {
	case <-executor.started:
	case <-time.After(2 * time.Second):
		t.Fatal("expected executor to start")
	}

	if err := scheduler.DeleteTask(taskID); err != nil {
		t.Fatalf("DeleteTask failed: %v", err)
	}

	if _, ok := scheduler.GetTask(taskID); ok {
		t.Fatal("expected task to be deleted")
	}
	if runs := scheduler.GetTaskRuns(taskID); len(runs) != 0 {
		t.Fatalf("expected deleted task runs to be cleared, got %+v", runs)
	}
}

func TestSchedulerRecordsPersistenceErrorFromRunSnapshots(t *testing.T) {
	persister := &failingPersister{saveRunsErr: errors.New("disk full")}
	scheduler := NewScheduler(nil)
	scheduler.SetPersister(persister)

	taskID, err := scheduler.AddTask(&Task{
		Name:     "persist-error",
		Schedule: "@every 1m",
		Command:  "echo",
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("AddTask failed: %v", err)
	}
	if err := scheduler.RunTaskNow(taskID); err != nil {
		t.Fatalf("RunTaskNow failed: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		runs := scheduler.GetTaskRuns(taskID)
		if len(runs) > 0 && runs[0].Status == "success" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := scheduler.LastPersistenceError(); err == nil || err.Error() != "disk full" {
		t.Fatalf("expected last persistence error to be recorded, got %v", err)
	}
}

func TestSchedulerUtilityPathsAndErrors(t *testing.T) {
	if _, _, err := ParseSchedule("bad cron"); err == nil {
		t.Fatal("expected invalid ParseSchedule input to fail")
	}

	schedule, kind, err := ParseSchedule("@every 30s")
	if err != nil {
		t.Fatalf("ParseSchedule(@every) failed: %v", err)
	}
	if kind != "interval" || schedule != "@every 30s" {
		t.Fatalf("unexpected interval schedule parse: schedule=%q kind=%q", schedule, kind)
	}

	schedule, kind, err = ParseSchedule("@hourly")
	if err != nil {
		t.Fatalf("ParseSchedule(@hourly) failed: %v", err)
	}
	if kind != "standard" || schedule != "0 * * * *" {
		t.Fatalf("unexpected standard schedule parse: schedule=%q kind=%q", schedule, kind)
	}

	task := &Task{Name: "bad-zone", Schedule: "@hourly", Command: "echo", Enabled: true, Timezone: "No/SuchZone"}
	if _, err := nextRunTimesForTask(task, time.Now().UTC(), 1); err == nil {
		t.Fatal("expected invalid timezone to fail")
	}

	if delay := calculateRetryDelay(2, "exponential"); delay != 4*time.Second {
		t.Fatalf("unexpected exponential retry delay: %s", delay)
	}
	if delay := calculateRetryDelay(2, "linear"); delay != 3*time.Second {
		t.Fatalf("unexpected linear retry delay: %s", delay)
	}
	if delay := calculateRetryDelay(2, "fixed"); delay != 2*time.Second {
		t.Fatalf("unexpected default retry delay: %s", delay)
	}
}

func TestSchedulerEnableDisableClearHistoryAndCancelErrors(t *testing.T) {
	scheduler := New()
	taskID, err := scheduler.AddTask(&Task{
		Name:     "managed",
		Schedule: "@hourly",
		Command:  "echo",
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("AddTask failed: %v", err)
	}

	if err := scheduler.DisableTask(taskID); err != nil {
		t.Fatalf("DisableTask failed: %v", err)
	}
	task, _ := scheduler.GetTask(taskID)
	if task.Enabled || task.NextRun != nil {
		t.Fatalf("expected disabled task with nil next run, got %+v", task)
	}

	if err := scheduler.EnableTask(taskID); err != nil {
		t.Fatalf("EnableTask failed: %v", err)
	}
	task, _ = scheduler.GetTask(taskID)
	if !task.Enabled || task.NextRun == nil {
		t.Fatalf("expected enabled task with next run, got %+v", task)
	}

	scheduler.mu.Lock()
	scheduler.taskRuns[taskID] = []*TaskRun{{ID: "run-1", TaskID: taskID, Status: "success", StartTime: time.Now().UTC()}}
	scheduler.mu.Unlock()
	if err := scheduler.ClearHistory(taskID); err != nil {
		t.Fatalf("ClearHistory failed: %v", err)
	}
	if runs := scheduler.GetTaskRuns(taskID); len(runs) != 0 {
		t.Fatalf("expected cleared history, got %+v", runs)
	}

	if err := scheduler.CancelRun("missing"); err == nil {
		t.Fatal("expected missing run cancel to fail")
	}
	if err := scheduler.ClearHistory("missing-task"); err == nil {
		t.Fatal("expected missing task history clear to fail")
	}
}

func TestSchedulerPersistenceErrorsOnTaskOperations(t *testing.T) {
	scheduler := New()
	scheduler.SetPersister(&failingPersister{saveTasksErr: errors.New("cannot save tasks")})

	if _, err := scheduler.AddTask(&Task{
		Name:     "persist-add",
		Schedule: "@hourly",
		Command:  "echo",
		Enabled:  true,
	}); err == nil {
		t.Fatal("expected AddTask persistence failure")
	}
	if err := scheduler.LastPersistenceError(); err == nil || err.Error() != "cannot save tasks" {
		t.Fatalf("expected add persistence error to be recorded, got %v", err)
	}
}

func TestSchedulerCheckAndRunTasksAndQueryHelpers(t *testing.T) {
	executor := &stubExecutor{started: make(chan struct{}), blockCh: make(chan struct{})}
	scheduler := NewScheduler(executor)

	taskID, err := scheduler.AddTask(&Task{
		Name:     "due-now",
		Schedule: "@every 1m",
		Command:  "echo",
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("AddTask failed: %v", err)
	}

	scheduler.mu.Lock()
	past := time.Now().UTC().Add(-time.Second)
	scheduler.tasks[taskID].NextRun = &past
	scheduler.mu.Unlock()

	scheduler.checkAndRunTasks()

	select {
	case <-executor.started:
	case <-time.After(2 * time.Second):
		t.Fatal("expected scheduled task to start running")
	}

	if runs := scheduler.GetAllRuns(1); len(runs) != 1 || runs[0].Status != "running" {
		t.Fatalf("expected one running task in GetAllRuns, got %+v", runs)
	}
	if runs := scheduler.GetRunHistory(taskID, 1); len(runs) != 1 || runs[0].TaskID != taskID {
		t.Fatalf("expected one run in GetRunHistory, got %+v", runs)
	}

	nextRuns := scheduler.NextRunTimes(2)
	if len(nextRuns[taskID]) != 2 {
		t.Fatalf("expected next run times for enabled task, got %+v", nextRuns)
	}

	close(executor.blockCh)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		runs := scheduler.GetTaskRuns(taskID)
		if len(runs) == 1 && runs[0].Status == "success" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("expected scheduled run to finish successfully")
}

func TestSchedulerRunningStateTaskValidationAndLoadEdges(t *testing.T) {
	scheduler := New()
	if scheduler.IsRunning() {
		t.Fatal("expected new scheduler to be stopped")
	}
	if err := scheduler.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if !scheduler.IsRunning() {
		t.Fatal("expected scheduler to report running after Start")
	}
	scheduler.Stop()
	if scheduler.IsRunning() {
		t.Fatal("expected scheduler to report stopped after Stop")
	}

	if err := (&Task{}).Validate(); err == nil {
		t.Fatal("expected empty task validation to fail")
	}
	if err := (&Task{Schedule: "@hourly"}).Validate(); err == nil {
		t.Fatal("expected missing command validation to fail")
	}
	if err := (&Task{Schedule: "@hourly", Command: "echo", Timezone: "No/SuchZone"}).Validate(); err == nil {
		t.Fatal("expected invalid timezone validation to fail")
	}

	disabledTimes := scheduler.NextRunTimes(2)
	if len(disabledTimes) != 0 {
		t.Fatalf("expected empty next run map for empty scheduler, got %+v", disabledTimes)
	}

	taskID, err := scheduler.AddTask(&Task{
		Name:     "history",
		Schedule: "@hourly",
		Command:  "echo",
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("AddTask failed: %v", err)
	}

	scheduler.mu.Lock()
	run1 := &TaskRun{ID: "run-1", TaskID: taskID, Status: "success", StartTime: time.Now().UTC().Add(-2 * time.Hour)}
	run2 := &TaskRun{ID: "run-2", TaskID: taskID, Status: "failed", StartTime: time.Now().UTC().Add(-time.Hour)}
	scheduler.taskRuns[taskID] = []*TaskRun{run1, run2}
	scheduler.mu.Unlock()

	if got := scheduler.GetRunHistory(taskID, 1); len(got) != 1 || got[0].ID != "run-2" {
		t.Fatalf("expected limited run history to return newest run, got %+v", got)
	}
	if got := scheduler.GetAllRuns(1); len(got) != 1 || got[0].ID != "run-2" {
		t.Fatalf("expected limited GetAllRuns to return newest run, got %+v", got)
	}

	persister := &failingPersister{saveRunsErr: errors.New("cannot save runs")}
	scheduler.SetPersister(persister)
	if err := scheduler.ClearHistory(taskID); err == nil {
		t.Fatal("expected ClearHistory to surface persistence failure")
	}
	if err := scheduler.LastPersistenceError(); err == nil || err.Error() != "cannot save runs" {
		t.Fatalf("expected save-runs failure to be recorded, got %v", err)
	}

	scheduler = New()
	if err := scheduler.LoadPersisted(); err != nil {
		t.Fatalf("LoadPersisted without persister failed: %v", err)
	}

	loader := New()
	loader.SetPersister(&loadFailingPersister{loadTasksErr: errors.New("load tasks failed")})
	if err := loader.LoadPersisted(); err == nil {
		t.Fatal("expected LoadPersisted to fail on task load error")
	}
}
