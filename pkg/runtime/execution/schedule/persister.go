package schedule

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// FilePersister persists cron tasks and runs to JSON files.
type FilePersister struct {
	mu       sync.Mutex
	tasksDir string
	runsDir  string
}

// NewFilePersister creates a file-based persister for cron tasks and runs.
func NewFilePersister(baseDir string) (*FilePersister, error) {
	if baseDir == "" {
		baseDir = ".anyclaw/cron"
	}

	tasksDir := filepath.Join(baseDir, "tasks")
	runsDir := filepath.Join(baseDir, "runs")

	for _, dir := range []string{tasksDir, runsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return &FilePersister{
		tasksDir: tasksDir,
		runsDir:  runsDir,
	}, nil
}

func (p *FilePersister) SaveTasks(tasks []*Task) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	data, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(p.tasksDir, "tasks.json"), data, 0o644)
}

func (p *FilePersister) LoadTasks() ([]*Task, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	data, err := os.ReadFile(filepath.Join(p.tasksDir, "tasks.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var tasks []*Task
	if err := json.Unmarshal(data, &tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

func (p *FilePersister) SaveRuns(runs []*TaskRun) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	data, err := json.MarshalIndent(runs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(p.runsDir, "runs.json"), data, 0o644)
}

func (p *FilePersister) LoadRuns() ([]*TaskRun, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	data, err := os.ReadFile(filepath.Join(p.runsDir, "runs.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var runs []*TaskRun
	if err := json.Unmarshal(data, &runs); err != nil {
		return nil, err
	}
	return runs, nil
}
