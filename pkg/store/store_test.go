package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sub", "dir")
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New should create dirs: %v", err)
	}
	if s.Dir() != dir {
		t.Errorf("Dir() = %q, want %q", s.Dir(), dir)
	}
}

func TestAssistantCRUD(t *testing.T) {
	s, _ := New(t.TempDir())

	// Empty list
	list, err := s.ListAssistants()
	if err != nil || len(list) != 0 {
		t.Fatalf("expected empty list, got %v %v", list, err)
	}

	// Save
	a := Assistant{Name: "dev-assistant", Description: "Developer", PermissionLevel: "limited"}
	if err := s.SaveAssistant(a); err != nil {
		t.Fatalf("SaveAssistant: %v", err)
	}

	// Get
	got, ok, err := s.GetAssistant("dev-assistant")
	if err != nil || !ok {
		t.Fatalf("GetAssistant: %v %v", err, ok)
	}
	if got.Name != "dev-assistant" {
		t.Errorf("name = %q", got.Name)
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}

	// Update
	a.Description = "Updated"
	if err := s.SaveAssistant(a); err != nil {
		t.Fatalf("SaveAssistant update: %v", err)
	}
	got, _, _ = s.GetAssistant("dev-assistant")
	if got.Description != "Updated" {
		t.Errorf("description = %q", got.Description)
	}

	// List
	list, _ = s.ListAssistants()
	if len(list) != 1 {
		t.Errorf("list len = %d", len(list))
	}

	// Delete
	deleted, err := s.DeleteAssistant("dev-assistant")
	if err != nil || !deleted {
		t.Fatalf("DeleteAssistant: %v %v", err, deleted)
	}
	_, ok, _ = s.GetAssistant("dev-assistant")
	if ok {
		t.Error("should be deleted")
	}

	// Delete non-existent
	deleted, _ = s.DeleteAssistant("nonexistent")
	if deleted {
		t.Error("should return false for non-existent")
	}
}

func TestAssistantGetNotFound(t *testing.T) {
	s, _ := New(t.TempDir())
	_, ok, err := s.GetAssistant("missing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("should not be found")
	}
}

func TestTaskCRUD(t *testing.T) {
	s, _ := New(t.TempDir())

	// Empty list
	list, err := s.ListTasks()
	if err != nil || len(list) != 0 {
		t.Fatalf("expected empty list")
	}

	// Save
	task := Task{ID: "task-1", Title: "Test task", Input: "do something", Assistant: "dev"}
	if err := s.SaveTask(task); err != nil {
		t.Fatalf("SaveTask: %v", err)
	}

	// Get
	got, ok, err := s.GetTask("task-1")
	if err != nil || !ok {
		t.Fatalf("GetTask: %v %v", err, ok)
	}
	if got.Status != "pending" {
		t.Errorf("status = %q, want pending", got.Status)
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}

	// Update
	got.Status = "completed"
	got.Result = "done"
	if err := s.SaveTask(got); err != nil {
		t.Fatalf("SaveTask update: %v", err)
	}
	got, _, _ = s.GetTask("task-1")
	if got.Status != "completed" {
		t.Errorf("status = %q", got.Status)
	}

	// List
	list, _ = s.ListTasks()
	if len(list) != 1 {
		t.Errorf("list len = %d", len(list))
	}
}

func TestTaskGetNotFound(t *testing.T) {
	s, _ := New(t.TempDir())
	_, ok, err := s.GetTask("missing")
	if err != nil || ok {
		t.Fatalf("unexpected: %v %v", err, ok)
	}
}

func TestAuditAppendAndList(t *testing.T) {
	s, _ := New(t.TempDir())

	// Empty list
	events, err := s.ListAudit(10)
	if err != nil || len(events) != 0 {
		t.Fatalf("expected empty")
	}

	// Append
	for i := 0; i < 5; i++ {
		if err := s.AppendAudit(AuditEvent{Actor: "user", Action: "test", Target: "t"}); err != nil {
			t.Fatalf("AppendAudit: %v", err)
		}
	}

	// List with limit
	events, err = s.ListAudit(3)
	if err != nil {
		t.Fatalf("ListAudit: %v", err)
	}
	if len(events) != 3 {
		t.Errorf("got %d events, want 3", len(events))
	}

	// List all
	events, _ = s.ListAudit(100)
	if len(events) != 5 {
		t.Errorf("got %d events, want 5", len(events))
	}

	// Verify fields
	if events[0].Actor != "user" {
		t.Errorf("actor = %q", events[0].Actor)
	}
	if events[0].ID == "" {
		t.Error("ID should be auto-generated")
	}
	if events[0].Timestamp.IsZero() {
		t.Error("Timestamp should be set")
	}
}

func TestAuditAppendMeta(t *testing.T) {
	s, _ := New(t.TempDir())

	s.AppendAudit(AuditEvent{
		Actor:  "admin",
		Action: "config.write",
		Target: "config",
		Meta:   map[string]any{"key": "value"},
	})

	events, _ := s.ListAudit(10)
	if len(events) != 1 {
		t.Fatal("expected 1 event")
	}
	if events[0].Meta["key"] != "value" {
		t.Errorf("meta = %v", events[0].Meta)
	}
}

func TestPersistenceAcrossInstances(t *testing.T) {
	dir := t.TempDir()

	// First instance: write data
	s1, _ := New(dir)
	s1.SaveAssistant(Assistant{Name: "persist-test"})
	s1.SaveTask(Task{ID: "t1", Title: "task"})
	s1.AppendAudit(AuditEvent{Actor: "user", Action: "test"})

	// Second instance: read data back
	s2, _ := New(dir)

	assistants, _ := s2.ListAssistants()
	if len(assistants) != 1 || assistants[0].Name != "persist-test" {
		t.Errorf("assistants not persisted: %v", assistants)
	}

	tasks, _ := s2.ListTasks()
	if len(tasks) != 1 || tasks[0].ID != "t1" {
		t.Errorf("tasks not persisted: %v", tasks)
	}

	events, _ := s2.ListAudit(10)
	if len(events) != 1 || events[0].Actor != "user" {
		t.Errorf("audit not persisted: %v", events)
	}
}

func TestFileCreatedOnFirstWrite(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir)

	// Files should not exist before any writes
	if _, err := os.Stat(s.assist); !os.IsNotExist(err) {
		t.Error("assistants.json should not exist yet")
	}

	s.SaveAssistant(Assistant{Name: "test"})

	if _, err := os.Stat(s.assist); err != nil {
		t.Error("assistants.json should exist after save")
	}
}

func TestSaveAssistantEmptyName(t *testing.T) {
	s, _ := New(t.TempDir())
	err := s.SaveAssistant(Assistant{Name: ""})
	if err == nil {
		t.Error("should reject empty name")
	}
}

func TestSaveTaskEmptyID(t *testing.T) {
	s, _ := New(t.TempDir())
	err := s.SaveTask(Task{ID: ""})
	if err == nil {
		t.Error("should reject empty id")
	}
}
