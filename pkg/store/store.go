package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ──────────────────────────────────────────────────────────────────────
// Data types
// ──────────────────────────────────────────────────────────────────────

type Assistant struct {
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	WorkingDir      string    `json:"working_dir"`
	PermissionLevel string    `json:"permission_level"`
	DefaultModel    string    `json:"default_model,omitempty"`
	Enabled         bool      `json:"enabled"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Task struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Input       string    `json:"input"`
	Assistant   string    `json:"assistant"`
	Status      string    `json:"status"` // pending, running, waiting_approval, completed, failed
	Result      string    `json:"result,omitempty"`
	Error       string    `json:"error,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	CompletedAt string    `json:"completed_at,omitempty"`
}

type AuditEvent struct {
	ID        string         `json:"id"`
	Actor     string         `json:"actor"`
	Action    string         `json:"action"`
	Target    string         `json:"target"`
	Timestamp time.Time      `json:"timestamp"`
	Meta      map[string]any `json:"meta,omitempty"`
}

// ──────────────────────────────────────────────────────────────────────
// Store
// ──────────────────────────────────────────────────────────────────────

type Store struct {
	mu     sync.RWMutex
	dir    string
	assist string // path to assistants.json
	tasks  string // path to tasks.json
	audit  string // path to audit.jsonl
}

// New creates a Store rooted at dir. It creates all necessary
// subdirectories on first use. Returns an error only if the
// directory cannot be created.
func New(dir string) (*Store, error) {
	if dir == "" {
		dir = ".anyclaw/data"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("store: create dir %q: %w", dir, err)
	}
	return &Store{
		dir:    dir,
		assist: filepath.Join(dir, "assistants.json"),
		tasks:  filepath.Join(dir, "tasks.json"),
		audit:  filepath.Join(dir, "audit.jsonl"),
	}, nil
}

func (s *Store) Dir() string { return s.dir }

// ──────────────────────────────────────────────────────────────────────
// Assistant repository
// ──────────────────────────────────────────────────────────────────────

func (s *Store) SaveAssistant(a Assistant) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	a.Name = strings.TrimSpace(a.Name)
	if a.Name == "" {
		return fmt.Errorf("store: assistant name is required")
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	a.UpdatedAt = time.Now().UTC()

	list, err := s.loadAssistantsLocked()
	if err != nil {
		return err
	}

	found := false
	for i, existing := range list {
		if strings.EqualFold(existing.Name, a.Name) {
			a.CreatedAt = existing.CreatedAt // preserve
			list[i] = a
			found = true
			break
		}
	}
	if !found {
		list = append(list, a)
	}
	return s.saveAssistantsLocked(list)
}

func (s *Store) GetAssistant(name string) (Assistant, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list, err := s.loadAssistantsLocked()
	if err != nil {
		return Assistant{}, false, err
	}
	for _, a := range list {
		if strings.EqualFold(a.Name, strings.TrimSpace(name)) {
			return a, true, nil
		}
	}
	return Assistant{}, false, nil
}

func (s *Store) ListAssistants() ([]Assistant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loadAssistantsLocked()
}

func (s *Store) DeleteAssistant(name string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	list, err := s.loadAssistantsLocked()
	if err != nil {
		return false, err
	}
	name = strings.TrimSpace(name)
	filtered := make([]Assistant, 0, len(list))
	found := false
	for _, a := range list {
		if strings.EqualFold(a.Name, name) {
			found = true
			continue
		}
		filtered = append(filtered, a)
	}
	if !found {
		return false, nil
	}
	return true, s.saveAssistantsLocked(filtered)
}

func (s *Store) loadAssistantsLocked() ([]Assistant, error) {
	return loadJSON[Assistant](s.assist)
}

func (s *Store) saveAssistantsLocked(list []Assistant) error {
	return saveJSON(s.assist, list)
}

// ──────────────────────────────────────────────────────────────────────
// Task repository
// ──────────────────────────────────────────────────────────────────────

func (s *Store) SaveTask(t Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	t.ID = strings.TrimSpace(t.ID)
	if t.ID == "" {
		return fmt.Errorf("store: task id is required")
	}
	if t.Status == "" {
		t.Status = "pending"
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now().UTC()
	}
	t.UpdatedAt = time.Now().UTC()

	list, err := s.loadTasksLocked()
	if err != nil {
		return err
	}

	found := false
	for i, existing := range list {
		if existing.ID == t.ID {
			t.CreatedAt = existing.CreatedAt
			list[i] = t
			found = true
			break
		}
	}
	if !found {
		list = append(list, t)
	}
	return s.saveTasksLocked(list)
}

func (s *Store) GetTask(id string) (Task, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list, err := s.loadTasksLocked()
	if err != nil {
		return Task{}, false, err
	}
	for _, t := range list {
		if t.ID == strings.TrimSpace(id) {
			return t, true, nil
		}
	}
	return Task{}, false, nil
}

func (s *Store) ListTasks() ([]Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loadTasksLocked()
}

func (s *Store) loadTasksLocked() ([]Task, error) {
	return loadJSON[Task](s.tasks)
}

func (s *Store) saveTasksLocked(list []Task) error {
	return saveJSON(s.tasks, list)
}

// ──────────────────────────────────────────────────────────────────────
// Audit repository (append-only JSONL)
// ──────────────────────────────────────────────────────────────────────

func (s *Store) AppendAudit(e AuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if e.ID == "" {
		e.ID = fmt.Sprintf("aud_%d", time.Now().UnixNano())
	}
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}

	f, err := os.OpenFile(s.audit, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("store: open audit log: %w", err)
	}
	defer f.Close()

	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(f, string(data))
	return err
}

func (s *Store) ListAudit(limit int) ([]AuditEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 100
	}

	data, err := os.ReadFile(s.audit)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	lines := splitLines(string(data))
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}

	result := make([]AuditEvent, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		var e AuditEvent
		if err := json.Unmarshal([]byte(line), &e); err == nil {
			result = append(result, e)
		}
	}
	return result, nil
}

// ──────────────────────────────────────────────────────────────────────
// JSON helpers
// ──────────────────────────────────────────────────────────────────────

func loadJSON[T any](path string) ([]T, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("store: read %s: %w", filepath.Base(path), err)
	}
	var list []T
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("store: parse %s: %w", filepath.Base(path), err)
	}
	return list, nil
}

func saveJSON[T any](path string, list []T) error {
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func splitLines(input string) []string {
	var lines []string
	current := ""
	for _, r := range input {
		if r == '\n' {
			lines = append(lines, current)
			current = ""
		} else if r != '\r' {
			current += string(r)
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}
