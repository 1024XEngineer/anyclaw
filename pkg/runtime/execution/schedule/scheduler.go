package schedule

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type Task struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Schedule     string                 `json:"schedule"`
	Command      string                 `json:"command"`
	Input        map[string]interface{} `json:"input,omitempty"`
	Agent        string                 `json:"agent,omitempty"`
	Workspace    string                 `json:"workspace,omitempty"`
	Enabled      bool                   `json:"enabled"`
	Timezone     string                 `json:"timezone,omitempty"`
	MaxRetries   int                    `json:"max_retries"`
	RetryBackoff string                 `json:"retry_backoff,omitempty"` // "fixed", "exponential", "linear"
	Timeout      int                    `json:"timeout_seconds"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
	LastRun      *time.Time             `json:"last_run,omitempty"`
	NextRun      *time.Time             `json:"next_run,omitempty"`
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
	Execute(ctx context.Context, cmd string, input map[string]interface{}) (string, error)
}

type Scheduler struct {
	tasks          map[string]*Task
	taskRuns       map[string][]*TaskRun
	executor       Executor
	mu             sync.RWMutex
	running        bool
	stopCh         chan struct{}
	runHistorySize int
	cancelFuncs    map[string]context.CancelFunc
	persister      TaskPersister
}

type TaskPersister interface {
	SaveTasks(tasks []*Task) error
	LoadTasks() ([]*Task, error)
	SaveRuns(runs []*TaskRun) error
	LoadRuns() ([]*TaskRun, error)
}

func NewScheduler(executor Executor) *Scheduler {
	return &Scheduler{
		tasks:          make(map[string]*Task),
		taskRuns:       make(map[string][]*TaskRun),
		executor:       executor,
		runHistorySize: 100,
		stopCh:         make(chan struct{}),
		cancelFuncs:    make(map[string]context.CancelFunc),
	}
}

func (s *Scheduler) SetPersister(p TaskPersister) {
	s.persister = p
}

func (s *Scheduler) LoadPersisted() error {
	if s.persister == nil {
		return nil
	}
	tasks, err := s.persister.LoadTasks()
	if err != nil {
		return err
	}
	for _, t := range tasks {
		s.tasks[t.ID] = t
	}

	runs, err := s.persister.LoadRuns()
	if err != nil {
		return err
	}
	for _, r := range runs {
		s.taskRuns[r.TaskID] = append(s.taskRuns[r.TaskID], r)
	}
	return nil
}

func New() *Scheduler {
	return NewScheduler(nil)
}

func (s *Scheduler) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("scheduler already running")
	}
	s.running = true
	s.mu.Unlock()

	go s.runLoop()
	return nil
}

func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}
	s.running = false
	close(s.stopCh)
}

func (s *Scheduler) runLoop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.checkAndRunTasks()
		}
	}
}

func (s *Scheduler) checkAndRunTasks() {
	s.mu.RLock()
	var tasksToRun []*Task
	now := time.Now()

	for _, task := range s.tasks {
		if !task.Enabled {
			continue
		}

		if task.NextRun == nil {
			next := calculateNextRun(task.Schedule, now)
			task.NextRun = &next
		}

		if now.After(*task.NextRun) || now.Equal(*task.NextRun) {
			tasksToRun = append(tasksToRun, task)
		}
	}
	s.mu.RUnlock()

	for _, task := range tasksToRun {
		go s.runTask(task.ID)
	}
}

func calculateNextRun(schedule string, from time.Time) time.Time {
	expr, err := ParseCronExpr(schedule)
	if err != nil {
		return from.Add(time.Hour)
	}

	return expr.Next(from)
}

type CronExpr struct {
	Minute     int
	Hour       int
	DayOfMonth int
	Month      int
	DayOfWeek  int
}

func ParseCronExpr(expr string) (*CronExpr, error) {
	var minute, hour, dayOfMonth, month, dayOfWeek int

	_, err := fmt.Sscanf(expr, "%d %d %d %d %d",
		&minute, &hour, &dayOfMonth, &month, &dayOfWeek)

	if err != nil {
		if expr == "@hourly" {
			return &CronExpr{0, -1, -1, -1, -1}, nil
		}
		if expr == "@daily" || expr == "@midnight" {
			return &CronExpr{0, 0, -1, -1, -1}, nil
		}
		if expr == "@weekly" {
			return &CronExpr{0, 0, -1, -1, 0}, nil
		}
		if expr == "@monthly" {
			return &CronExpr{0, 0, 1, -1, -1}, nil
		}
		if expr == "@yearly" || expr == "@annually" {
			return &CronExpr{0, 0, 1, 1, -1}, nil
		}
		return nil, fmt.Errorf("invalid cron expression: %s", expr)
	}

	return &CronExpr{
		Minute:     minute,
		Hour:       hour,
		DayOfMonth: dayOfMonth,
		Month:      month,
		DayOfWeek:  dayOfWeek,
	}, nil
}

func (e *CronExpr) Next(t time.Time) time.Time {
	next := t.Add(time.Second)

	for {
		if !e.matchMonth(next) {
			next = time.Date(next.Year(), next.Month()+1, 1, 0, 0, 0, 0, next.Location())
			continue
		}

		if !e.matchDayOfMonth(next) || !e.matchDayOfWeek(next) {
			next = next.Add(24 * time.Hour)
			next = time.Date(next.Year(), next.Month(), next.Day(), e.Hour, e.Minute, 0, 0, next.Location())
			continue
		}

		if next.Hour() < e.Hour || (next.Hour() == e.Hour && next.Minute() < e.Minute) {
			next = time.Date(next.Year(), next.Month(), next.Day(), e.Hour, e.Minute, 0, 0, next.Location())
		}

		if next.After(t) {
			return next
		}

		next = next.Add(24 * time.Hour)
	}
}

func (e *CronExpr) matchMonth(t time.Time) bool {
	if e.Month == -1 {
		return true
	}
	return int(t.Month()) == e.Month
}

func (e *CronExpr) matchDayOfMonth(t time.Time) bool {
	if e.DayOfMonth == -1 {
		return true
	}
	return t.Day() == e.DayOfMonth
}

func (e *CronExpr) matchDayOfWeek(t time.Time) bool {
	if e.DayOfWeek == -1 {
		return true
	}
	return int(t.Weekday()) == e.DayOfWeek
}

func (s *Scheduler) AddTask(task *Task) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if task.ID == "" {
		task.ID = fmt.Sprintf("task-%d", time.Now().UnixNano())
	}
	if task.Name == "" {
		task.Name = task.ID
	}
	if task.MaxRetries == 0 {
		task.MaxRetries = 3
	}
	if task.Timeout == 0 {
		task.Timeout = 300
	}
	if task.CreatedAt.IsZero() {
		task.CreatedAt = time.Now()
	}
	task.UpdatedAt = time.Now()
	task.Enabled = true

	if _, err := ParseCronExpr(task.Schedule); err != nil {
		return "", fmt.Errorf("invalid schedule: %w", err)
	}

	now := time.Now()
	next := calculateNextRun(task.Schedule, now)
	task.NextRun = &next

	s.tasks[task.ID] = task

	return task.ID, nil
}

func (s *Scheduler) UpdateTask(task *Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.tasks[task.ID]
	if !ok {
		return fmt.Errorf("task not found: %s", task.ID)
	}

	if task.Schedule != existing.Schedule {
		return fmt.Errorf("cannot change schedule for existing task")
	}

	task.UpdatedAt = time.Now()
	task.CreatedAt = existing.CreatedAt
	task.NextRun = existing.NextRun
	s.tasks[task.ID] = task

	return nil
}

func (s *Scheduler) DeleteTask(taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.tasks[taskID]; !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}

	delete(s.tasks, taskID)
	return nil
}

func (s *Scheduler) GetTask(taskID string) (*Task, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tasks[taskID]
	return t, ok
}

func (s *Scheduler) ListTasks() []*Task {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tasks := make([]*Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		tasks = append(tasks, t)
	}
	return tasks
}

func (s *Scheduler) EnableTask(taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[taskID]
	if !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}

	task.Enabled = true
	task.UpdatedAt = time.Now()
	return nil
}

func (s *Scheduler) DisableTask(taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[taskID]
	if !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}

	task.Enabled = false
	task.UpdatedAt = time.Now()
	return nil
}

func (s *Scheduler) RunTaskNow(taskID string) error {
	s.mu.RLock()
	_, ok := s.tasks[taskID]
	s.mu.RUnlock()

	if !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}

	go s.runTask(taskID)
	return nil
}

func (s *Scheduler) runTask(taskID string) {
	s.mu.Lock()
	task, ok := s.tasks[taskID]
	s.mu.Unlock()

	if !ok || !task.Enabled {
		return
	}

	run := &TaskRun{
		ID:        fmt.Sprintf("run-%d", time.Now().UnixNano()),
		TaskID:    taskID,
		StartTime: time.Now(),
		Status:    "running",
	}

	s.mu.Lock()
	now := time.Now()
	task.LastRun = &now
	task.NextRun = nil
	s.taskRuns[taskID] = append(s.taskRuns[taskID], run)
	if len(s.taskRuns[taskID]) > s.runHistorySize {
		s.taskRuns[taskID] = s.taskRuns[taskID][len(s.taskRuns[taskID])-s.runHistorySize:]
	}
	s.mu.Unlock()

	timeout := time.Duration(task.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 300 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	s.mu.Lock()
	s.cancelFuncs[run.ID] = cancel
	s.mu.Unlock()

	defer func() {
		cancel()
		s.mu.Lock()
		delete(s.cancelFuncs, run.ID)
		s.mu.Unlock()
	}()

	var result string
	var err error
	actualRetries := 0

	for i := 0; i <= task.MaxRetries; i++ {
		select {
		case <-ctx.Done():
			run.Status = "cancelled"
			run.Cancelled = true
			run.Retries = actualRetries
			endTime := time.Now()
			run.EndTime = &endTime
			s.mu.Lock()
			task.UpdatedAt = time.Now()
			next := calculateNextRun(task.Schedule, time.Now())
			task.NextRun = &next
			s.mu.Unlock()
			return
		default:
		}

		if s.executor != nil {
			result, err = s.executor.Execute(ctx, task.Command, task.Input)
		} else {
			result = "No executor configured"
		}

		if err == nil {
			break
		}

		if i < task.MaxRetries {
			actualRetries++
			delay := s.calculateRetryDelay(i, task)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				run.Status = "cancelled"
				run.Cancelled = true
				run.Retries = actualRetries
				endTime := time.Now()
				run.EndTime = &endTime
				s.mu.Lock()
				task.UpdatedAt = time.Now()
				next := calculateNextRun(task.Schedule, time.Now())
				task.NextRun = &next
				s.mu.Unlock()
				return
			}
		}
	}

	endTime := time.Now()
	run.EndTime = &endTime

	if err != nil {
		run.Status = "failed"
		run.Error = err.Error()
		run.Output = result
	} else {
		run.Status = "success"
		run.Output = result
	}

	run.Retries = actualRetries

	s.mu.Lock()
	task.UpdatedAt = time.Now()
	next := calculateNextRun(task.Schedule, time.Now())
	task.NextRun = &next
	s.mu.Unlock()

	// Persist if configured
	if s.persister != nil {
		allRuns := make([]*TaskRun, 0)
		for _, runs := range s.taskRuns {
			allRuns = append(allRuns, runs...)
		}
		_ = s.persister.SaveRuns(allRuns)
	}
}

func (s *Scheduler) calculateRetryDelay(attempt int, task *Task) time.Duration {
	switch task.RetryBackoff {
	case "exponential":
		return time.Duration(1<<uint(attempt)) * time.Second
	case "linear":
		return time.Duration(attempt+1) * time.Second
	default:
		return 2 * time.Second
	}
}

func (s *Scheduler) GetTaskRuns(taskID string) []*TaskRun {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.taskRuns[taskID]
}

func (s *Scheduler) GetRunHistory(taskID string, limit int) []*TaskRun {
	s.mu.RLock()
	defer s.mu.RUnlock()

	runs := s.taskRuns[taskID]
	if limit > 0 && len(runs) > limit {
		return runs[len(runs)-limit:]
	}
	return runs
}

func (s *Scheduler) GetAllRuns(limit int) []*TaskRun {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var allRuns []*TaskRun
	for _, runs := range s.taskRuns {
		allRuns = append(allRuns, runs...)
	}

	if len(allRuns) == 0 {
		return allRuns
	}

	sortRunsByTime(allRuns)

	if limit > 0 && len(allRuns) > limit {
		return allRuns[len(allRuns)-limit:]
	}
	return allRuns
}

func sortRunsByTime(runs []*TaskRun) {
	for i := 1; i < len(runs); i++ {
		for j := i; j > 0 && runs[j].StartTime.Before(runs[j-1].StartTime); j-- {
			runs[j], runs[j-1] = runs[j-1], runs[j]
		}
	}
}

func (s *Scheduler) CancelRun(runID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, runs := range s.taskRuns {
		for _, run := range runs {
			if run.ID == runID && run.Status == "running" {
				run.Status = "cancelled"
				return nil
			}
		}
	}

	return fmt.Errorf("run not found or not running: %s", runID)
}

func (s *Scheduler) ClearHistory(taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.tasks[taskID]; !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}

	s.taskRuns[taskID] = nil
	return nil
}

func (s *Scheduler) NextRunTimes(count int) map[string]time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()

	nextTimes := make(map[string]time.Time)
	for id, task := range s.tasks {
		if task.Enabled && task.NextRun != nil {
			nextTimes[id] = *task.NextRun
		}
	}

	result := make(map[string]time.Time)
	for k, v := range nextTimes {
		result[k] = v
	}

	return result
}

func (s *Scheduler) Stats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	totalTasks := len(s.tasks)
	enabledTasks := 0
	totalRuns := 0
	failedRuns := 0
	successRuns := 0

	for _, task := range s.tasks {
		if task.Enabled {
			enabledTasks++
		}
	}

	for _, runs := range s.taskRuns {
		totalRuns += len(runs)
		for _, run := range runs {
			if run.Status == "failed" {
				failedRuns++
			} else if run.Status == "success" {
				successRuns++
			}
		}
	}

	return map[string]interface{}{
		"total_tasks":   totalTasks,
		"enabled_tasks": enabledTasks,
		"total_runs":    totalRuns,
		"success_runs":  successRuns,
		"failed_runs":   failedRuns,
	}
}

func (s *Scheduler) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

func (t *Task) Validate() error {
	if t.Schedule == "" {
		return fmt.Errorf("schedule is required")
	}

	if _, err := ParseCronExpr(t.Schedule); err != nil {
		return fmt.Errorf("invalid schedule: %w", err)
	}

	if t.Command == "" {
		return fmt.Errorf("command is required")
	}

	return nil
}

func ParseSchedule(expr string) (string, string, error) {
	specs := map[string]string{
		"@yearly":   "0 0 1 1 *",
		"@annually": "0 0 1 1 *",
		"@monthly":  "0 0 1 * *",
		"@weekly":   "0 0 * * 0",
		"@daily":    "0 0 * * *",
		"@midnight": "0 0 * * *",
		"@hourly":   "0 * * * *",
	}

	if spec, ok := specs[expr]; ok {
		return spec, "standard", nil
	}

	if _, err := ParseCronExpr(expr); err == nil {
		return expr, "standard", nil
	}

	return "", "", fmt.Errorf("invalid cron expression: %s", expr)
}

func (s *Scheduler) MarshalJSON() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return json.Marshal(map[string]interface{}{
		"tasks": s.tasks,
		"stats": s.Stats(),
	})
}
