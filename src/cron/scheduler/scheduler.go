package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type Task struct {
	ID          string
	Name        string
	Schedule    string
	IntervalSec int
	Command     string
	Enabled     bool
	LastRun     time.Time
	LastResult  string
	WorkspaceID string
	Handler     func(ctx context.Context) (string, error)
}

type TaskResult struct {
	TaskID    string
	StartTime time.Time
	EndTime   time.Time
	Output    string
	Error     error
	Success   bool
}

type Scheduler struct {
	tasks      map[string]*Task
	results    []TaskResult
	mu         sync.RWMutex
	maxResults int
	onRun      func(task *Task, output string, err error)
	running    bool
	stopChan   chan struct{}
}

func New() *Scheduler {
	return &Scheduler{
		tasks:      make(map[string]*Task),
		results:    make([]TaskResult, 0),
		maxResults: 100,
		stopChan:   make(chan struct{}),
	}
}

func (s *Scheduler) AddTask(task *Task) error {
	if task.IntervalSec <= 0 {
		return fmt.Errorf("invalid interval: %d", task.IntervalSec)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	task.ID = fmt.Sprintf("task-%d", time.Now().UnixNano())
	s.tasks[task.ID] = task
	return nil
}

func (s *Scheduler) RemoveTask(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.tasks[id]; !ok {
		return fmt.Errorf("task not found: %s", id)
	}

	delete(s.tasks, id)
	return nil
}

func (s *Scheduler) runTask(task *Task) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	start := time.Now()
	output, err := task.Handler(ctx)
	end := time.Now()

	result := TaskResult{
		TaskID:    task.ID,
		StartTime: start,
		EndTime:   end,
		Output:    output,
		Error:     err,
		Success:   err == nil,
	}

	s.mu.Lock()
	task.LastRun = end
	if err != nil {
		task.LastResult = err.Error()
	} else {
		task.LastResult = "success"
	}
	s.results = append(s.results, result)
	if len(s.results) > s.maxResults {
		s.results = s.results[len(s.results)-s.maxResults:]
	}
	s.mu.Unlock()

	if s.onRun != nil {
		s.onRun(task, output, err)
	}
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

func (s *Scheduler) GetTask(id string) *Task {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.tasks[id]
}

func (s *Scheduler) GetResults(taskID string) []TaskResult {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []TaskResult
	for _, r := range s.results {
		if r.TaskID == taskID {
			results = append(results, r)
		}
	}
	return results
}

func (s *Scheduler) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	go s.runLoop()
}

func (s *Scheduler) Stop() {
	s.mu.Lock()
	s.running = false
	s.mu.Unlock()
	close(s.stopChan)
}

func (s *Scheduler) runLoop() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case now := <-ticker.C:
			s.mu.RLock()
			for _, task := range s.tasks {
				if !task.Enabled {
					continue
				}

				interval := time.Duration(task.IntervalSec) * time.Second
				if task.LastRun.IsZero() || now.Sub(task.LastRun) >= interval {
					go s.runTask(task)
				}
			}
			s.mu.RUnlock()
		}
	}
}

func (s *Scheduler) SetOnRun(fn func(task *Task, output string, err error)) {
	s.onRun = fn
}

func (s *Scheduler) RunNow(ctx context.Context, taskID string) error {
	task := s.GetTask(taskID)
	if task == nil {
		return fmt.Errorf("task not found")
	}

	go s.runTask(task)
	return nil
}
