package input

import (
	"context"
	"testing"
	"time"
)

func TestBaseAdapterSetRunningMarksHealthyWithoutError(t *testing.T) {
	adapter := NewBaseAdapter("demo", true)

	adapter.SetRunning(true)

	status := adapter.Status()
	if !status.Running {
		t.Fatalf("expected adapter to be running")
	}
	if !status.Healthy {
		t.Fatalf("expected adapter to be healthy after entering running state")
	}
}

func TestBaseAdapterSetRunningDoesNotClearExistingError(t *testing.T) {
	adapter := NewBaseAdapter("demo", true)
	adapter.SetError(assertErr("boom"))

	adapter.SetRunning(true)

	status := adapter.Status()
	if status.Healthy {
		t.Fatalf("expected adapter with last error to remain unhealthy")
	}
	if status.LastError != "boom" {
		t.Fatalf("expected last error to be preserved, got %q", status.LastError)
	}
}

func TestManagerRunWaitsForEnabledAdaptersToExit(t *testing.T) {
	started := make(chan struct{})
	exited := make(chan struct{})
	adapter := &managerTestAdapter{
		name:    "demo",
		enabled: true,
		run: func(ctx context.Context, _ InboundHandler) error {
			close(started)
			<-ctx.Done()
			close(exited)
			return nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		NewManager(adapter).Run(ctx, nil)
		close(done)
	}()

	waitForSignal(t, started, "adapter start")
	select {
	case <-done:
		t.Fatalf("expected manager to keep running until context cancellation")
	default:
	}

	cancel()
	waitForSignal(t, exited, "adapter exit")
	waitForSignal(t, done, "manager exit")
}

func TestManagerRunSkipsDisabledAdapters(t *testing.T) {
	called := make(chan struct{}, 1)
	adapter := &managerTestAdapter{
		name:    "demo",
		enabled: false,
		run: func(ctx context.Context, _ InboundHandler) error {
			called <- struct{}{}
			return nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		NewManager(adapter).Run(ctx, nil)
		close(done)
	}()

	waitForSignal(t, done, "manager exit without enabled adapters")
	select {
	case <-called:
		t.Fatalf("expected disabled adapter to be skipped")
	default:
	}
}

type assertErr string

func (e assertErr) Error() string { return string(e) }

type managerTestAdapter struct {
	name    string
	enabled bool
	run     func(ctx context.Context, handle InboundHandler) error
}

func (a *managerTestAdapter) Name() string  { return a.name }
func (a *managerTestAdapter) Enabled() bool { return a.enabled }

func (a *managerTestAdapter) Run(ctx context.Context, handle InboundHandler) error {
	if a.run == nil {
		return nil
	}
	return a.run(ctx, handle)
}

func (a *managerTestAdapter) Status() Status {
	return Status{Name: a.name, Enabled: a.enabled}
}

func waitForSignal(t *testing.T, ch <-chan struct{}, label string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", label)
	}
}
