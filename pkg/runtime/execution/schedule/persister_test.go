package schedule

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFilePersisterRoundTrip(t *testing.T) {
	persister, err := NewFilePersister(t.TempDir())
	if err != nil {
		t.Fatalf("NewFilePersister failed: %v", err)
	}

	nextRun := time.Date(2026, 4, 22, 11, 0, 0, 0, time.UTC)
	endTime := time.Date(2026, 4, 22, 10, 1, 0, 0, time.UTC)
	tasks := []*Task{{
		ID:       "task-1",
		Name:     "demo",
		Schedule: "0 * * * *",
		Command:  "echo",
		Enabled:  true,
		NextRun:  &nextRun,
	}}
	runs := []*TaskRun{{
		ID:        "run-1",
		TaskID:    "task-1",
		StartTime: time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC),
		EndTime:   &endTime,
		Status:    "success",
		Output:    "ok",
	}}

	if err := persister.SaveTasks(tasks); err != nil {
		t.Fatalf("SaveTasks failed: %v", err)
	}
	if err := persister.SaveRuns(runs); err != nil {
		t.Fatalf("SaveRuns failed: %v", err)
	}

	loadedTasks, err := persister.LoadTasks()
	if err != nil {
		t.Fatalf("LoadTasks failed: %v", err)
	}
	loadedRuns, err := persister.LoadRuns()
	if err != nil {
		t.Fatalf("LoadRuns failed: %v", err)
	}

	if len(loadedTasks) != 1 || loadedTasks[0].ID != "task-1" {
		t.Fatalf("unexpected loaded tasks: %+v", loadedTasks)
	}
	if len(loadedRuns) != 1 || loadedRuns[0].ID != "run-1" {
		t.Fatalf("unexpected loaded runs: %+v", loadedRuns)
	}

	if _, err := NewFilePersister(filepath.Join(t.TempDir(), "nested", "scheduler")); err != nil {
		t.Fatalf("NewFilePersister nested failed: %v", err)
	}
}

func TestFilePersisterMissingAndInvalidFiles(t *testing.T) {
	persister, err := NewFilePersister(t.TempDir())
	if err != nil {
		t.Fatalf("NewFilePersister failed: %v", err)
	}

	tasks, err := persister.LoadTasks()
	if err != nil {
		t.Fatalf("LoadTasks on missing file failed: %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("expected no tasks for missing file, got %+v", tasks)
	}

	invalidTasksPath := filepath.Join(persister.tasksDir, "tasks.json")
	if err := os.WriteFile(invalidTasksPath, []byte("{"), 0o644); err != nil {
		t.Fatalf("write invalid tasks json: %v", err)
	}
	if _, err := persister.LoadTasks(); err == nil {
		t.Fatal("expected invalid tasks json to fail")
	}
}

func TestReadAndWriteJSONFileEdgeCases(t *testing.T) {
	tempDir := t.TempDir()
	emptyPath := filepath.Join(tempDir, "empty.json")
	if err := os.WriteFile(emptyPath, []byte{}, 0o644); err != nil {
		t.Fatalf("write empty json file: %v", err)
	}

	var tasks []*Task
	if err := readJSONFile(emptyPath, &tasks); err != nil {
		t.Fatalf("readJSONFile on empty file failed: %v", err)
	}

	missingDirPath := filepath.Join(tempDir, "missing", "tasks.json")
	if err := writeJSONFile(missingDirPath, map[string]string{"a": "b"}); err == nil {
		t.Fatal("expected writeJSONFile to fail when parent dir is missing")
	}

	type badJSON struct {
		Ch chan int `json:"ch"`
	}
	if err := writeJSONFile(filepath.Join(tempDir, "bad.json"), badJSON{Ch: make(chan int)}); err == nil || !strings.Contains(err.Error(), "unsupported type") {
		t.Fatalf("expected marshal failure for unsupported JSON value, got %v", err)
	}
}
