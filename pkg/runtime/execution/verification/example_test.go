package verification

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type stubVerificationContext struct {
	*DefaultContext
	windowChecks int
	windowAfter  int
}

func (s *stubVerificationContext) WindowAppears(title string) (bool, error) {
	s.windowChecks++
	return s.windowChecks >= s.windowAfter, nil
}

func TestVerificationExecutorExecuteFileExists(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("hello world"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	exec := NewVerificationExecutorWithDefaults(NewDefaultContext())
	result, err := exec.Execute(ctx, &Spec{
		Condition: &Condition{
			Type: VerificationTypeFileExists,
			Parameters: map[string]any{
				"path": path,
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.AllPassed() {
		t.Fatalf("expected file exists verification to pass, got %#v", result)
	}
}

func TestVerificationExecutorExecuteTemplate(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "template.txt")
	if err := os.WriteFile(path, []byte("template"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	exec := NewVerificationExecutorWithDefaults(NewDefaultContext())
	result, err := exec.Execute(ctx, &Spec{
		Template: "file-saved",
		Parameters: map[string]any{
			"path": path,
		},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.AllPassed() {
		t.Fatalf("expected template verification to pass, got %#v", result)
	}
}

func TestVerificationExecutorExecuteComposite(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "composite.txt")
	if err := os.WriteFile(path, []byte("hello composite"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	exec := NewVerificationExecutorWithDefaults(NewDefaultContext())
	result, err := exec.Execute(ctx, &Spec{
		Composite: &CompositeCondition{
			Op: "and",
			Conditions: []Condition{
				{
					Type: VerificationTypeFileExists,
					Parameters: map[string]any{
						"path": path,
					},
				},
				{
					Type: VerificationTypeFileContains,
					Parameters: map[string]any{
						"path":    path,
						"content": "hello",
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.AllPassed() {
		t.Fatalf("expected composite verification to pass, got %#v", result)
	}
}

func TestVerificationExecutorExecuteWithRetry(t *testing.T) {
	ctx := context.Background()
	exec := NewVerificationExecutorWithDefaults(&stubVerificationContext{
		DefaultContext: NewDefaultContext(),
		windowAfter:    3,
	})

	result, err := exec.Execute(ctx, &Spec{
		Condition: &Condition{
			Type: VerificationTypeWindowAppears,
			Parameters: map[string]any{
				"title": "Notepad",
			},
			Retry: &RetryConfig{
				MaxAttempts:   5,
				InitialDelay:  1 * time.Millisecond,
				MaxDelay:      5 * time.Millisecond,
				BackoffFactor: 1.5,
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.AllPassed() {
		t.Fatalf("expected retry verification to pass, got %#v", result)
	}
	if got := result.Results[0].Retries; got != 2 {
		t.Fatalf("expected 2 retries before success, got %d", got)
	}
}
