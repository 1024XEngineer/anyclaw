package schedule

import (
	"path/filepath"
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
