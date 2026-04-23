package schedule

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// FilePersister stores scheduler tasks and runs in JSON files.
type FilePersister struct {
	mu       sync.Mutex
	tasksDir string
	runsDir  string
}

// NewFilePersister creates the backing directories if needed.
func NewFilePersister(baseDir string) (*FilePersister, error) {
	if baseDir == "" {
		baseDir = ".anyclaw/cron"
	}

	tasksDir := filepath.Join(baseDir, "tasks")
	runsDir := filepath.Join(baseDir, "runs")
	for _, dir := range []string{tasksDir, runsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create scheduler persistence dir %s: %w", dir, err)
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
	return writeJSONFile(filepath.Join(p.tasksDir, "tasks.json"), tasks)
}

func (p *FilePersister) LoadTasks() ([]*Task, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	var tasks []*Task
	if err := readJSONFile(filepath.Join(p.tasksDir, "tasks.json"), &tasks); err != nil {
		return nil, err
	}
	return cloneTasks(tasks), nil
}

func (p *FilePersister) SaveRuns(runs []*TaskRun) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return writeJSONFile(filepath.Join(p.runsDir, "runs.json"), runs)
}

func (p *FilePersister) LoadRuns() ([]*TaskRun, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	var runs []*TaskRun
	if err := readJSONFile(filepath.Join(p.runsDir, "runs.json"), &runs); err != nil {
		return nil, err
	}
	return cloneTaskRuns(runs), nil
}

func writeJSONFile(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "scheduler-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = os.Remove(tmpPath)
	}

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return err
	}
	return nil
}

func readJSONFile(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, target)
}
