package schedule

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type Task struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	Schedule     string         `json:"schedule"`
	Command      string         `json:"command"`
	Input        map[string]any `json:"input,omitempty"`
	Agent        string         `json:"agent,omitempty"`
	Workspace    string         `json:"workspace,omitempty"`
	Enabled      bool           `json:"enabled"`
	Timezone     string         `json:"timezone,omitempty"`
	MaxRetries   int            `json:"max_retries"`
	RetryBackoff string         `json:"retry_backoff,omitempty"`
	Timeout      int            `json:"timeout_seconds"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	LastRun      *time.Time     `json:"last_run,omitempty"`
	NextRun      *time.Time     `json:"next_run,omitempty"`
}

type TaskRun struct {
	ID        string     `json:"id"`
	TaskID    string     `json:"task_id"`
	StartTime time.Time  `json:"start_time"`
	EndTime   *time.Time `json:"end_time,omitempty"`
	Status    string     `json:"status"`
	Result    string     `json:"result,omitempty"`
	Error     string     `json:"error,omitempty"`
	Output    string     `json:"output,omitempty"`
	Retries   int        `json:"retries"`
	Cancelled bool       `json:"cancelled,omitempty"`
}

type Executor interface {
	Execute(ctx context.Context, cmd string, input map[string]any) (string, error)
}

type TaskPersister interface {
	SaveTasks(tasks []*Task) error
	LoadTasks() ([]*Task, error)
	SaveRuns(runs []*TaskRun) error
	LoadRuns() ([]*TaskRun, error)
}

type Scheduler struct {
	mu             sync.RWMutex
	tasks          map[string]*Task
	taskRuns       map[string][]*TaskRun
	activeTaskRuns map[string]string
	runDone        map[string]chan struct{}
	executor       Executor
	running        bool
	stopCh         chan struct{}
	doneCh         chan struct{}
	runHistorySize int
	cancelFuncs    map[string]context.CancelFunc
	persister      TaskPersister
	lastPersistErr error
}

func New() *Scheduler {
	return NewScheduler(nil)
}

func NewScheduler(executor Executor) *Scheduler {
	return &Scheduler{
		tasks:          make(map[string]*Task),
		taskRuns:       make(map[string][]*TaskRun),
		activeTaskRuns: make(map[string]string),
		runDone:        make(map[string]chan struct{}),
		executor:       executor,
		runHistorySize: 100,
		cancelFuncs:    make(map[string]context.CancelFunc),
	}
}

func (s *Scheduler) SetPersister(p TaskPersister) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.persister = p
}

func (s *Scheduler) LoadPersisted() error {
	s.mu.RLock()
	persister := s.persister
	s.mu.RUnlock()
	if persister == nil {
		return nil
	}

	tasks, err := persister.LoadTasks()
	if err != nil {
		return err
	}
	runs, err := persister.LoadRuns()
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.tasks = make(map[string]*Task, len(tasks))
	for _, task := range tasks {
		cloned := cloneTask(task)
		if cloned == nil {
			continue
		}
		if cloned.NextRun == nil && cloned.Enabled {
			next := calculateNextRun(cloned.Schedule, time.Now().UTC(), cloned.Timezone)
			if !next.IsZero() {
				cloned.NextRun = &next
			}
		}
		s.tasks[cloned.ID] = cloned
	}

	s.taskRuns = make(map[string][]*TaskRun)
	for _, run := range runs {
		cloned := cloneTaskRun(run)
		if cloned == nil {
			continue
		}
		s.taskRuns[cloned.TaskID] = append(s.taskRuns[cloned.TaskID], cloned)
	}

	return nil
}

func (s *Scheduler) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("scheduler already running")
	}
	s.running = true
	s.stopCh = make(chan struct{})
	s.doneCh = make(chan struct{})
	stopCh := s.stopCh
	doneCh := s.doneCh
	s.mu.Unlock()

	go s.runLoop(stopCh, doneCh)
	return nil
}

func (s *Scheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	stopCh := s.stopCh
	doneCh := s.doneCh
	s.stopCh = nil
	s.doneCh = nil
	s.mu.Unlock()

	close(stopCh)
	<-doneCh
}

func (s *Scheduler) runLoop(stopCh <-chan struct{}, doneCh chan<- struct{}) {
	defer close(doneCh)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			s.checkAndRunTasks()
		}
	}
}

func (s *Scheduler) AddTask(task *Task) (string, error) {
	if task == nil {
		return "", fmt.Errorf("task is required")
	}

	cloned := cloneTask(task)
	if cloned.ID == "" {
		cloned.ID = fmt.Sprintf("task-%d", time.Now().UnixNano())
	}
	if cloned.Name == "" {
		cloned.Name = cloned.ID
	}
	if cloned.MaxRetries == 0 {
		cloned.MaxRetries = 3
	}
	if cloned.Timeout == 0 {
		cloned.Timeout = 300
	}
	if cloned.CreatedAt.IsZero() {
		cloned.CreatedAt = time.Now().UTC()
	}
	cloned.UpdatedAt = time.Now().UTC()

	if err := cloned.Validate(); err != nil {
		return "", err
	}
	if cloned.Enabled {
		next := calculateNextRun(cloned.Schedule, time.Now().UTC(), cloned.Timezone)
		if next.IsZero() {
			return "", fmt.Errorf("calculate next run for %q", cloned.Schedule)
		}
		cloned.NextRun = &next
	} else {
		cloned.NextRun = nil
	}

	s.mu.Lock()
	if _, exists := s.tasks[cloned.ID]; exists {
		s.mu.Unlock()
		return "", fmt.Errorf("task already exists: %s", cloned.ID)
	}
	s.tasks[cloned.ID] = cloned
	tasksSnapshot := cloneTasksFromMap(s.tasks)
	persister := s.persister
	s.mu.Unlock()

	if persister != nil {
		if err := persister.SaveTasks(tasksSnapshot); err != nil {
			s.recordPersistError(err)
			return cloned.ID, err
		}
		s.recordPersistError(nil)
	}
	return cloned.ID, nil
}

func (s *Scheduler) UpdateTask(task *Task) error {
	if task == nil {
		return fmt.Errorf("task is required")
	}

	s.mu.Lock()
	existing, ok := s.tasks[task.ID]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("task not found: %s", task.ID)
	}

	cloned := cloneTask(task)
	cloned.CreatedAt = existing.CreatedAt
	cloned.UpdatedAt = time.Now().UTC()
	if cloned.MaxRetries == 0 {
		cloned.MaxRetries = existing.MaxRetries
	}
	if cloned.Timeout == 0 {
		cloned.Timeout = existing.Timeout
	}
	if cloned.Schedule == "" {
		cloned.Schedule = existing.Schedule
	}
	if cloned.Command == "" {
		cloned.Command = existing.Command
	}
	if cloned.Name == "" {
		cloned.Name = existing.Name
	}
	if cloned.Timezone == "" {
		cloned.Timezone = existing.Timezone
	}
	if cloned.Input == nil {
		cloned.Input = cloneMap(existing.Input)
	}
	if err := cloned.Validate(); err != nil {
		s.mu.Unlock()
		return err
	}

	if cloned.Enabled {
		next := calculateNextRun(cloned.Schedule, time.Now().UTC(), cloned.Timezone)
		if !next.IsZero() {
			cloned.NextRun = &next
		}
	} else {
		cloned.NextRun = nil
	}
	s.tasks[cloned.ID] = cloned
	tasksSnapshot := cloneTasksFromMap(s.tasks)
	persister := s.persister
	s.mu.Unlock()

	if persister != nil {
		if err := persister.SaveTasks(tasksSnapshot); err != nil {
			s.recordPersistError(err)
			return err
		}
		s.recordPersistError(nil)
	}
	return nil
}

func (s *Scheduler) DeleteTask(taskID string) error {
	s.mu.Lock()
	if _, ok := s.tasks[taskID]; !ok {
		s.mu.Unlock()
		return fmt.Errorf("task not found: %s", taskID)
	}
	runID, active := s.activeTaskRuns[taskID]
	cancel := s.cancelFuncs[runID]
	done := s.runDone[runID]
	s.mu.Unlock()

	if active {
		if cancel != nil {
			cancel()
		}
		if done != nil {
			<-done
		}
	}

	s.mu.Lock()
	if _, ok := s.tasks[taskID]; !ok {
		s.mu.Unlock()
		return nil
	}
	delete(s.tasks, taskID)
	delete(s.taskRuns, taskID)
	delete(s.activeTaskRuns, taskID)
	tasksSnapshot := cloneTasksFromMap(s.tasks)
	runsSnapshot := cloneRunsFromMap(s.taskRuns)
	persister := s.persister
	s.mu.Unlock()

	if persister != nil {
		if err := persister.SaveTasks(tasksSnapshot); err != nil {
			s.recordPersistError(err)
			return err
		}
		if err := persister.SaveRuns(runsSnapshot); err != nil {
			s.recordPersistError(err)
			return err
		}
		s.recordPersistError(nil)
	}
	return nil
}

func (s *Scheduler) GetTask(taskID string) (*Task, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	task, ok := s.tasks[taskID]
	return cloneTask(task), ok
}

func (s *Scheduler) ListTasks() []*Task {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tasks := cloneTasksFromMap(s.tasks)
	sort.Slice(tasks, func(i int, j int) bool {
		return tasks[i].CreatedAt.Before(tasks[j].CreatedAt)
	})
	return tasks
}

func (s *Scheduler) EnableTask(taskID string) error {
	return s.setTaskEnabled(taskID, true)
}

func (s *Scheduler) DisableTask(taskID string) error {
	return s.setTaskEnabled(taskID, false)
}

func (s *Scheduler) setTaskEnabled(taskID string, enabled bool) error {
	s.mu.Lock()
	task, ok := s.tasks[taskID]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("task not found: %s", taskID)
	}
	task.Enabled = enabled
	task.UpdatedAt = time.Now().UTC()
	if enabled {
		next := calculateNextRun(task.Schedule, time.Now().UTC(), task.Timezone)
		if !next.IsZero() {
			task.NextRun = &next
		}
	} else {
		task.NextRun = nil
	}
	tasksSnapshot := cloneTasksFromMap(s.tasks)
	persister := s.persister
	s.mu.Unlock()

	if persister != nil {
		if err := persister.SaveTasks(tasksSnapshot); err != nil {
			s.recordPersistError(err)
			return err
		}
		s.recordPersistError(nil)
	}
	return nil
}

func (s *Scheduler) RunTaskNow(taskID string) error {
	s.mu.Lock()
	task, ok := s.tasks[taskID]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("task not found: %s", taskID)
	}
	if !task.Enabled {
		s.mu.Unlock()
		return fmt.Errorf("task disabled: %s", taskID)
	}
	if _, active := s.activeTaskRuns[taskID]; active {
		s.mu.Unlock()
		return fmt.Errorf("task already running: %s", taskID)
	}
	runID := fmt.Sprintf("run-%d", time.Now().UnixNano())
	s.activeTaskRuns[taskID] = runID
	s.mu.Unlock()

	go s.runTask(taskID, runID)
	return nil
}

func (s *Scheduler) checkAndRunTasks() {
	now := time.Now().UTC()
	type candidate struct {
		taskID string
		runID  string
	}

	candidates := make([]candidate, 0)
	s.mu.Lock()
	for _, task := range s.tasks {
		if !task.Enabled {
			continue
		}
		if _, active := s.activeTaskRuns[task.ID]; active {
			continue
		}
		if task.NextRun == nil {
			next := calculateNextRun(task.Schedule, now, task.Timezone)
			if !next.IsZero() {
				task.NextRun = &next
			}
		}
		if task.NextRun != nil && (now.After(*task.NextRun) || now.Equal(*task.NextRun)) {
			runID := fmt.Sprintf("run-%d", time.Now().UnixNano())
			s.activeTaskRuns[task.ID] = runID
			candidates = append(candidates, candidate{taskID: task.ID, runID: runID})
		}
	}
	s.mu.Unlock()

	for _, task := range candidates {
		go s.runTask(task.taskID, task.runID)
	}
}

func (s *Scheduler) runTask(taskID string, runID string) {
	s.mu.Lock()
	task, ok := s.tasks[taskID]
	if !ok || !task.Enabled {
		delete(s.activeTaskRuns, taskID)
		s.mu.Unlock()
		return
	}

	run := &TaskRun{
		ID:        runID,
		TaskID:    taskID,
		StartTime: time.Now().UTC(),
		Status:    "running",
	}

	now := time.Now().UTC()
	task.LastRun = &now
	task.NextRun = nil
	s.taskRuns[taskID] = append(s.taskRuns[taskID], run)
	if len(s.taskRuns[taskID]) > s.runHistorySize {
		s.taskRuns[taskID] = s.taskRuns[taskID][len(s.taskRuns[taskID])-s.runHistorySize:]
	}
	timeout := time.Duration(task.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	done := make(chan struct{})
	s.cancelFuncs[run.ID] = cancel
	s.runDone[run.ID] = done
	tasksSnapshot := cloneTasksFromMap(s.tasks)
	runsSnapshot := cloneRunsFromMap(s.taskRuns)
	persister := s.persister
	command := task.Command
	input := cloneMap(task.Input)
	maxRetries := task.MaxRetries
	retryBackoff := task.RetryBackoff
	s.mu.Unlock()

	s.persistSnapshots(persister, tasksSnapshot, runsSnapshot)

	defer func() {
		cancel()
		s.mu.Lock()
		delete(s.cancelFuncs, run.ID)
		if ch, ok := s.runDone[run.ID]; ok {
			close(ch)
			delete(s.runDone, run.ID)
		}
		delete(s.activeTaskRuns, taskID)
		s.mu.Unlock()
	}()

	var result string
	var err error
	retries := 0
	for attempt := 0; attempt <= maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			err = ctx.Err()
		default:
			if s.executor != nil {
				result, err = s.executor.Execute(ctx, command, input)
			} else {
				result = "No executor configured"
				err = nil
			}
		}

		if err == nil {
			break
		}
		if attempt == maxRetries {
			break
		}
		retries++
		select {
		case <-time.After(calculateRetryDelay(attempt, retryBackoff)):
		case <-ctx.Done():
			err = ctx.Err()
			attempt = maxRetries
		}
	}

	end := time.Now().UTC()

	s.mu.Lock()
	task = s.tasks[taskID]
	run = s.findRunLocked(taskID, runID)
	if run != nil {
		run.EndTime = &end
		run.Retries = retries
		run.Output = result
		switch {
		case err == nil:
			run.Status = "success"
		case ctx.Err() != nil:
			run.Status = "cancelled"
			run.Cancelled = true
			run.Error = ctx.Err().Error()
		default:
			run.Status = "failed"
			run.Error = err.Error()
		}
	}
	if task != nil {
		task.UpdatedAt = end
		next := calculateNextRun(task.Schedule, end, task.Timezone)
		if !next.IsZero() && task.Enabled {
			task.NextRun = &next
		} else {
			task.NextRun = nil
		}
	}
	tasksSnapshot = cloneTasksFromMap(s.tasks)
	runsSnapshot = cloneRunsFromMap(s.taskRuns)
	persister = s.persister
	s.mu.Unlock()

	s.persistSnapshots(persister, tasksSnapshot, runsSnapshot)
}

func (s *Scheduler) GetTaskRuns(taskID string) []*TaskRun {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneTaskRuns(s.taskRuns[taskID])
}

func (s *Scheduler) GetRunHistory(taskID string, limit int) []*TaskRun {
	s.mu.RLock()
	defer s.mu.RUnlock()

	runs := cloneTaskRuns(s.taskRuns[taskID])
	if limit > 0 && len(runs) > limit {
		runs = runs[len(runs)-limit:]
	}
	return runs
}

func (s *Scheduler) GetAllRuns(limit int) []*TaskRun {
	s.mu.RLock()
	defer s.mu.RUnlock()

	runs := cloneRunsFromMap(s.taskRuns)
	sort.Slice(runs, func(i int, j int) bool {
		return runs[i].StartTime.Before(runs[j].StartTime)
	})
	if limit > 0 && len(runs) > limit {
		runs = runs[len(runs)-limit:]
	}
	return runs
}

func (s *Scheduler) CancelRun(runID string) error {
	s.mu.RLock()
	cancel, ok := s.cancelFuncs[runID]
	s.mu.RUnlock()
	if !ok {
		return fmt.Errorf("run not found or not running: %s", runID)
	}
	cancel()
	return nil
}

func (s *Scheduler) ClearHistory(taskID string) error {
	s.mu.Lock()
	if _, ok := s.tasks[taskID]; !ok {
		s.mu.Unlock()
		return fmt.Errorf("task not found: %s", taskID)
	}
	s.taskRuns[taskID] = nil
	runsSnapshot := cloneRunsFromMap(s.taskRuns)
	persister := s.persister
	s.mu.Unlock()

	if persister != nil {
		if err := persister.SaveRuns(runsSnapshot); err != nil {
			s.recordPersistError(err)
			return err
		}
		s.recordPersistError(nil)
	}
	return nil
}

func (s *Scheduler) NextRunTimes(count int) map[string][]time.Time {
	s.mu.RLock()
	tasks := cloneTasksFromMap(s.tasks)
	s.mu.RUnlock()

	result := make(map[string][]time.Time, len(tasks))
	now := time.Now().UTC()
	for _, task := range tasks {
		if !task.Enabled {
			continue
		}
		times, err := nextRunTimesForTask(task, now, count)
		if err != nil {
			continue
		}
		result[task.ID] = times
	}
	return result
}

func (s *Scheduler) Stats() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	totalTasks := len(s.tasks)
	enabledTasks := 0
	totalRuns := 0
	failedRuns := 0
	successRuns := 0
	runningRuns := len(s.cancelFuncs)

	for _, task := range s.tasks {
		if task.Enabled {
			enabledTasks++
		}
	}
	for _, runs := range s.taskRuns {
		totalRuns += len(runs)
		for _, run := range runs {
			switch run.Status {
			case "failed":
				failedRuns++
			case "success":
				successRuns++
			}
		}
	}

	return map[string]any{
		"total_tasks":   totalTasks,
		"enabled_tasks": enabledTasks,
		"total_runs":    totalRuns,
		"success_runs":  successRuns,
		"failed_runs":   failedRuns,
		"running_runs":  runningRuns,
	}
}

func (s *Scheduler) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

func (s *Scheduler) LastPersistenceError() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastPersistErr
}

func (s *Scheduler) MarshalJSON() ([]byte, error) {
	s.mu.RLock()
	payload := map[string]any{
		"tasks": cloneTasksFromMap(s.tasks),
		"stats": s.statsLocked(),
	}
	s.mu.RUnlock()
	return json.Marshal(payload)
}

func (t *Task) Validate() error {
	if t == nil {
		return fmt.Errorf("task is required")
	}
	if strings.TrimSpace(t.Schedule) == "" {
		return fmt.Errorf("schedule is required")
	}
	if strings.TrimSpace(t.Command) == "" {
		return fmt.Errorf("command is required")
	}
	spec, err := ParseCronSpec(t.Schedule)
	if err != nil {
		return fmt.Errorf("invalid schedule: %w", err)
	}
	if err := spec.Validate(); err != nil {
		return fmt.Errorf("invalid schedule: %w", err)
	}
	if t.Timezone != "" {
		if _, err := time.LoadLocation(t.Timezone); err != nil {
			return fmt.Errorf("invalid timezone %q: %w", t.Timezone, err)
		}
	}
	return nil
}

func ParseSchedule(expr string) (string, string, error) {
	spec, err := ParseCronSpec(expr)
	if err != nil {
		return "", "", err
	}
	if spec.Every > 0 {
		return spec.Format(), "interval", nil
	}
	return spec.Format(), "standard", nil
}

func nextRunTimesForTask(task *Task, from time.Time, count int) ([]time.Time, error) {
	if count <= 0 {
		return []time.Time{}, nil
	}
	loc := time.UTC
	if task.Timezone != "" {
		loaded, err := time.LoadLocation(task.Timezone)
		if err != nil {
			return nil, err
		}
		loc = loaded
	}

	spec, err := ParseCronSpec(task.Schedule)
	if err != nil {
		return nil, err
	}
	current := from.In(loc)
	result := make([]time.Time, 0, count)
	for len(result) < count {
		next := spec.Next(current)
		if next.IsZero() {
			break
		}
		result = append(result, next)
		current = next
	}
	return result, nil
}

func calculateNextRun(schedule string, from time.Time, timezone string) time.Time {
	spec, err := ParseCronSpec(schedule)
	if err != nil {
		return time.Time{}
	}
	loc := from.Location()
	if timezone != "" {
		loaded, err := time.LoadLocation(timezone)
		if err == nil {
			loc = loaded
		}
	}
	return spec.Next(from.In(loc))
}

func calculateRetryDelay(attempt int, strategy string) time.Duration {
	switch strategy {
	case "exponential":
		return time.Duration(1<<uint(attempt)) * time.Second
	case "linear":
		return time.Duration(attempt+1) * time.Second
	default:
		return 2 * time.Second
	}
}

func (s *Scheduler) statsLocked() map[string]any {
	totalTasks := len(s.tasks)
	enabledTasks := 0
	totalRuns := 0
	failedRuns := 0
	successRuns := 0
	runningRuns := len(s.cancelFuncs)

	for _, task := range s.tasks {
		if task.Enabled {
			enabledTasks++
		}
	}
	for _, runs := range s.taskRuns {
		totalRuns += len(runs)
		for _, run := range runs {
			switch run.Status {
			case "failed":
				failedRuns++
			case "success":
				successRuns++
			}
		}
	}

	return map[string]any{
		"total_tasks":   totalTasks,
		"enabled_tasks": enabledTasks,
		"total_runs":    totalRuns,
		"success_runs":  successRuns,
		"failed_runs":   failedRuns,
		"running_runs":  runningRuns,
	}
}

func (s *Scheduler) persistSnapshots(persister TaskPersister, tasks []*Task, runs []*TaskRun) {
	if persister == nil {
		return
	}
	if err := persister.SaveTasks(tasks); err != nil {
		s.recordPersistError(err)
		return
	}
	if err := persister.SaveRuns(runs); err != nil {
		s.recordPersistError(err)
		return
	}
	s.recordPersistError(nil)
}

func (s *Scheduler) recordPersistError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastPersistErr = err
}

func (s *Scheduler) findRunLocked(taskID string, runID string) *TaskRun {
	for _, run := range s.taskRuns[taskID] {
		if run.ID == runID {
			return run
		}
	}
	return nil
}

func cloneTasksFromMap(tasks map[string]*Task) []*Task {
	cloned := make([]*Task, 0, len(tasks))
	for _, task := range tasks {
		cloned = append(cloned, cloneTask(task))
	}
	return cloned
}

func cloneRunsFromMap(taskRuns map[string][]*TaskRun) []*TaskRun {
	var cloned []*TaskRun
	for _, runs := range taskRuns {
		cloned = append(cloned, cloneTaskRuns(runs)...)
	}
	return cloned
}

func cloneTasks(tasks []*Task) []*Task {
	cloned := make([]*Task, 0, len(tasks))
	for _, task := range tasks {
		cloned = append(cloned, cloneTask(task))
	}
	return cloned
}

func cloneTaskRuns(runs []*TaskRun) []*TaskRun {
	cloned := make([]*TaskRun, 0, len(runs))
	for _, run := range runs {
		cloned = append(cloned, cloneTaskRun(run))
	}
	return cloned
}

func cloneTask(task *Task) *Task {
	if task == nil {
		return nil
	}
	cloned := *task
	cloned.Input = cloneMap(task.Input)
	if task.LastRun != nil {
		lastRun := *task.LastRun
		cloned.LastRun = &lastRun
	}
	if task.NextRun != nil {
		nextRun := *task.NextRun
		cloned.NextRun = &nextRun
	}
	return &cloned
}

func cloneTaskRun(run *TaskRun) *TaskRun {
	if run == nil {
		return nil
	}
	cloned := *run
	if run.EndTime != nil {
		end := *run.EndTime
		cloned.EndTime = &end
	}
	return &cloned
}

func cloneMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}
