package contextengine

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestExclusiveSlotAcquire(t *testing.T) {
	slot := NewExclusiveSlot(DefaultSlotConfig())

	ctx := context.Background()
	result, err := slot.Acquire(ctx, "engine1", ContextConfig{MaxAge: time.Hour})
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if !result.Granted {
		t.Fatal("expected slot granted")
	}
	if result.SlotID != "engine1" {
		t.Errorf("expected slot ID engine1, got %s", result.SlotID)
	}
	if result.Engine == nil {
		t.Fatal("expected non-nil engine")
	}

	if !slot.IsActive() {
		t.Error("expected slot to be active")
	}
	if slot.ActiveID() != "engine1" {
		t.Errorf("expected active ID engine1, got %s", slot.ActiveID())
	}
}

func TestExclusiveSlotExclusive(t *testing.T) {
	slot := NewExclusiveSlot(SlotConfig{
		MaxIdle:    time.Minute,
		MaxPending: 5,
	})

	ctx := context.Background()
	_, err := slot.Acquire(ctx, "engine1", ContextConfig{MaxAge: time.Hour})
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}

	done := make(chan *SlotResult, 1)
	go func() {
		result, err := slot.Acquire(ctx, "engine2", ContextConfig{MaxAge: time.Hour})
		if err != nil {
			t.Logf("second acquire error: %v", err)
		}
		done <- result
	}()

	time.Sleep(50 * time.Millisecond)

	slot.Release("engine1")

	select {
	case result := <-done:
		if !result.Granted {
			t.Fatal("expected second slot granted after release")
		}
		if result.SlotID != "engine2" {
			t.Errorf("expected slot ID engine2, got %s", result.SlotID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for second slot")
	}
}

func TestExclusiveSlotReacquire(t *testing.T) {
	slot := NewExclusiveSlot(DefaultSlotConfig())

	ctx := context.Background()
	result1, _ := slot.Acquire(ctx, "engine1", ContextConfig{MaxAge: time.Hour})
	if !result1.Granted {
		t.Fatal("expected first acquire granted")
	}

	result2, err := slot.Acquire(ctx, "engine1", ContextConfig{MaxAge: time.Hour})
	if err != nil {
		t.Fatalf("reacquire: %v", err)
	}
	if !result2.Granted {
		t.Fatal("expected reacquire granted")
	}
	if result2.Engine != result1.Engine {
		t.Error("expected same engine on reacquire")
	}
}

func TestExclusiveSlotRelease(t *testing.T) {
	slot := NewExclusiveSlot(DefaultSlotConfig())

	ctx := context.Background()
	slot.Acquire(ctx, "engine1", ContextConfig{MaxAge: time.Hour})

	slot.Release("engine1")

	if slot.IsActive() {
		t.Error("expected slot inactive after release")
	}
}

func TestExclusiveSlotForceRelease(t *testing.T) {
	slot := NewExclusiveSlot(DefaultSlotConfig())

	ctx := context.Background()
	slot.Acquire(ctx, "engine1", ContextConfig{MaxAge: time.Hour})

	err := slot.ForceRelease("engine1")
	if err != nil {
		t.Fatalf("force release: %v", err)
	}

	if slot.IsActive() {
		t.Error("expected slot inactive after force release")
	}
}

func TestExclusiveSlotForceReleaseWrongID(t *testing.T) {
	slot := NewExclusiveSlot(DefaultSlotConfig())

	ctx := context.Background()
	slot.Acquire(ctx, "engine1", ContextConfig{MaxAge: time.Hour})

	err := slot.ForceRelease("engine2")
	if err == nil {
		t.Error("expected error for wrong ID")
	}
}

func TestExclusiveSlotHeartbeat(t *testing.T) {
	slot := NewExclusiveSlot(DefaultSlotConfig())

	ctx := context.Background()
	slot.Acquire(ctx, "engine1", ContextConfig{MaxAge: time.Hour})

	err := slot.Heartbeat("engine1")
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}

	err = slot.Heartbeat("engine2")
	if err == nil {
		t.Error("expected error for wrong ID heartbeat")
	}
}

func TestExclusiveSlotStatus(t *testing.T) {
	slot := NewExclusiveSlot(DefaultSlotConfig())

	status := slot.Status()
	if status.State != SlotInactive {
		t.Errorf("expected inactive state, got %s", status.State)
	}

	ctx := context.Background()
	slot.Acquire(ctx, "engine1", ContextConfig{MaxAge: time.Hour})

	status = slot.Status()
	if status.State != SlotActive {
		t.Errorf("expected active state, got %s", status.State)
	}
	if status.ActiveID != "engine1" {
		t.Errorf("expected active ID engine1, got %s", status.ActiveID)
	}
	if status.IdleDuration < 0 {
		t.Error("expected non-negative idle duration")
	}
}

func TestExclusiveSlotTerminate(t *testing.T) {
	slot := NewExclusiveSlot(DefaultSlotConfig())

	ctx := context.Background()
	slot.Acquire(ctx, "engine1", ContextConfig{MaxAge: time.Hour})

	err := slot.Terminate("engine1")
	if err != nil {
		t.Fatalf("terminate: %v", err)
	}

	if slot.Status().State != SlotTerminated {
		t.Errorf("expected terminated state, got %s", slot.Status().State)
	}
}

func TestExclusiveSlotAcquireAfterTerminateFails(t *testing.T) {
	slot := NewExclusiveSlot(DefaultSlotConfig())

	ctx := context.Background()
	if _, err := slot.Acquire(ctx, "engine1", ContextConfig{MaxAge: time.Hour}); err != nil {
		t.Fatalf("acquire: %v", err)
	}

	if err := slot.Terminate("engine1"); err != nil {
		t.Fatalf("terminate: %v", err)
	}

	result, err := slot.Acquire(ctx, "engine2", ContextConfig{MaxAge: time.Hour})
	if err != nil {
		t.Fatalf("acquire after terminate: %v", err)
	}
	if result == nil || result.Granted {
		t.Fatal("expected terminated slot to reject new acquire")
	}
	if result.Error == nil {
		t.Fatal("expected terminated slot to report an error")
	}
	if slot.Status().State != SlotTerminated {
		t.Errorf("expected terminated state to persist, got %s", slot.Status().State)
	}
}

func TestExclusiveSlotTerminateWrongID(t *testing.T) {
	slot := NewExclusiveSlot(DefaultSlotConfig())

	ctx := context.Background()
	slot.Acquire(ctx, "engine1", ContextConfig{MaxAge: time.Hour})

	err := slot.Terminate("engine2")
	if err == nil {
		t.Error("expected error for wrong ID terminate")
	}
}

func TestExclusiveSlotQueuePriority(t *testing.T) {
	slot := NewExclusiveSlot(SlotConfig{
		MaxIdle:    time.Minute,
		MaxPending: 5,
	})

	ctx := context.Background()
	slot.Acquire(ctx, "engine1", ContextConfig{MaxAge: time.Hour})

	results := make(chan *SlotResult, 2)

	go func() {
		reqCtx, reqCancel := context.WithTimeout(ctx, 5*time.Second)
		defer reqCancel()
		r, _ := slot.Acquire(reqCtx, "low", ContextConfig{MaxAge: time.Hour})
		results <- r
	}()

	time.Sleep(100 * time.Millisecond)

	slot.mu.Lock()
	if len(slot.pendingQueue) != 1 {
		slot.mu.Unlock()
		t.Fatalf("expected 1 in queue, got %d", len(slot.pendingQueue))
	}
	slot.pendingQueue[0].Priority = -10
	slot.mu.Unlock()

	go func() {
		reqCtx, reqCancel := context.WithTimeout(ctx, 5*time.Second)
		defer reqCancel()
		r, _ := slot.Acquire(reqCtx, "high", ContextConfig{MaxAge: time.Hour})
		results <- r
	}()

	time.Sleep(100 * time.Millisecond)

	slot.mu.Lock()
	queueIDs := make([]string, len(slot.pendingQueue))
	for i, r := range slot.pendingQueue {
		queueIDs[i] = r.ID
	}
	slot.mu.Unlock()

	if len(queueIDs) != 2 {
		t.Fatalf("expected 2 in queue, got %d", len(queueIDs))
	}

	if queueIDs[0] != "high" {
		t.Errorf("expected high at front of queue, got %v", queueIDs)
	}

	slot.Release("engine1")

	first := <-results
	if first.SlotID != "high" {
		t.Errorf("expected high priority first, got %s", first.SlotID)
	}
}

func TestExclusiveSlotQueueFull(t *testing.T) {
	slot := NewExclusiveSlot(SlotConfig{
		MaxIdle:    time.Minute,
		MaxPending: 1,
	})

	ctx := context.Background()
	slot.Acquire(ctx, "engine1", ContextConfig{MaxAge: time.Hour})

	go func() {
		slot.Acquire(ctx, "queued", ContextConfig{MaxAge: time.Hour})
	}()

	time.Sleep(50 * time.Millisecond)

	result, err := slot.Acquire(ctx, "overflow", ContextConfig{MaxAge: time.Hour})
	if err == nil && result.Granted {
		t.Error("expected slot queue full error")
	}
}

func TestExclusiveSlotIdleTimeout(t *testing.T) {
	var timeoutCalled bool
	var mu sync.Mutex

	slot := NewExclusiveSlot(SlotConfig{
		MaxIdle:     100 * time.Millisecond,
		MaxDuration: time.Hour,
		MaxPending:  5,
		OnTimeout: func(id string) {
			mu.Lock()
			timeoutCalled = true
			mu.Unlock()
		},
	})

	ctx := context.Background()
	slot.Acquire(ctx, "engine1", ContextConfig{MaxAge: time.Hour})

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	if !timeoutCalled {
		t.Error("expected timeout callback")
	}
	mu.Unlock()

	if slot.IsActive() {
		t.Error("expected slot inactive after timeout")
	}
}

func TestExclusiveSlotMaxDuration(t *testing.T) {
	var timeoutCalled bool
	var mu sync.Mutex

	slot := NewExclusiveSlot(SlotConfig{
		MaxIdle:     time.Hour,
		MaxDuration: 100 * time.Millisecond,
		MaxPending:  5,
		OnTimeout: func(id string) {
			mu.Lock()
			timeoutCalled = true
			mu.Unlock()
		},
	})

	ctx := context.Background()
	slot.Acquire(ctx, "engine1", ContextConfig{MaxAge: time.Hour})

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	if !timeoutCalled {
		t.Error("expected max duration timeout callback")
	}
	mu.Unlock()
}

func TestExclusiveSlotTimeoutCallbackCanInspectSlot(t *testing.T) {
	var slot *ExclusiveSlot
	timeoutCh := make(chan string, 1)

	slot = NewExclusiveSlot(SlotConfig{
		MaxIdle:     100 * time.Millisecond,
		MaxDuration: time.Hour,
		MaxPending:  5,
		OnTimeout: func(id string) {
			_ = slot.Status()
			timeoutCh <- id
		},
	})

	ctx := context.Background()
	if _, err := slot.Acquire(ctx, "engine1", ContextConfig{MaxAge: time.Hour}); err != nil {
		t.Fatalf("acquire: %v", err)
	}

	select {
	case id := <-timeoutCh:
		if id != "engine1" {
			t.Fatalf("expected timeout for engine1, got %s", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for timeout callback")
	}
}

func TestExclusiveSlotCallbacks(t *testing.T) {
	var activated, deactivated string
	var mu sync.Mutex

	slot := NewExclusiveSlot(SlotConfig{
		MaxIdle: time.Minute,
		OnActivate: func(id string) {
			mu.Lock()
			activated = id
			mu.Unlock()
		},
		OnDeactivate: func(id string) {
			mu.Lock()
			deactivated = id
			mu.Unlock()
		},
	})

	ctx := context.Background()
	slot.Acquire(ctx, "engine1", ContextConfig{MaxAge: time.Hour})

	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	if activated != "engine1" {
		t.Errorf("expected activated engine1, got %s", activated)
	}
	mu.Unlock()

	slot.Release("engine1")

	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	if deactivated != "engine1" {
		t.Errorf("expected deactivated engine1, got %s", deactivated)
	}
	mu.Unlock()
}

func TestExclusiveSlotQueuedAcquirePreservesConfigAndCallbacksDoNotDeadlock(t *testing.T) {
	var slot *ExclusiveSlot
	activated := make(chan string, 2)
	deactivated := make(chan string, 1)

	slot = NewExclusiveSlot(SlotConfig{
		MaxIdle:    time.Minute,
		MaxPending: 5,
		OnActivate: func(id string) {
			_ = slot.Status()
			activated <- id
		},
		OnDeactivate: func(id string) {
			_ = slot.Status()
			deactivated <- id
		},
	})

	ctx := context.Background()
	if _, err := slot.Acquire(ctx, "engine1", ContextConfig{MaxAge: time.Hour}); err != nil {
		t.Fatalf("first acquire: %v", err)
	}

	select {
	case id := <-activated:
		if id != "engine1" {
			t.Fatalf("expected engine1 activation, got %s", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for initial activation callback")
	}

	queuedCfg := ContextConfig{
		MaxAge:          2 * time.Minute,
		AutoExpire:      true,
		CleanupInterval: 10 * time.Millisecond,
	}

	resultCh := make(chan *SlotResult, 1)
	go func() {
		result, _ := slot.Acquire(ctx, "engine2", queuedCfg)
		resultCh <- result
	}()

	time.Sleep(50 * time.Millisecond)

	releaseDone := make(chan struct{})
	go func() {
		slot.Release("engine1")
		close(releaseDone)
	}()

	select {
	case <-releaseDone:
	case <-time.After(2 * time.Second):
		t.Fatal("release deadlocked")
	}

	select {
	case id := <-deactivated:
		if id != "engine1" {
			t.Fatalf("expected engine1 deactivation, got %s", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for deactivation callback")
	}

	select {
	case id := <-activated:
		if id != "engine2" {
			t.Fatalf("expected engine2 activation, got %s", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for queued activation callback")
	}

	select {
	case result := <-resultCh:
		if result == nil || !result.Granted {
			t.Fatal("expected queued acquire to be granted")
		}
		if result.Engine == nil {
			t.Fatal("expected granted slot to include engine")
		}
		if result.Engine.maxAge != queuedCfg.MaxAge {
			t.Fatalf("expected queued engine max age %v, got %v", queuedCfg.MaxAge, result.Engine.maxAge)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for queued acquire result")
	}
}

func TestExclusiveSlotEngineAccess(t *testing.T) {
	slot := NewExclusiveSlot(DefaultSlotConfig())

	ctx := context.Background()
	slot.Acquire(ctx, "engine1", ContextConfig{MaxAge: time.Hour})

	engine := slot.Engine()
	if engine == nil {
		t.Fatal("expected non-nil engine")
	}

	engine.Set(ctx, "key", "value")
	val, err := engine.Get(ctx, "key")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if val != "value" {
		t.Errorf("expected value, got %v", val)
	}

	slot.Release("engine1")

	if slot.Engine() != nil {
		t.Error("expected nil engine after release")
	}
}

func TestExclusiveSlotSetMaxIdle(t *testing.T) {
	slot := NewExclusiveSlot(DefaultSlotConfig())

	slot.SetMaxIdle(10 * time.Second)
	if slot.maxIdle != 10*time.Second {
		t.Errorf("expected max idle 10s, got %v", slot.maxIdle)
	}
}

func TestExclusiveSlotSetMaxDuration(t *testing.T) {
	slot := NewExclusiveSlot(DefaultSlotConfig())

	slot.SetMaxDuration(60 * time.Second)
	if slot.maxDuration != 60*time.Second {
		t.Errorf("expected max duration 60s, got %v", slot.maxDuration)
	}
}

func TestExclusiveSlotAcquireContextCancel(t *testing.T) {
	slot := NewExclusiveSlot(SlotConfig{
		MaxIdle:    time.Minute,
		MaxPending: 5,
	})

	ctx, cancel := context.WithCancel(context.Background())
	slot.Acquire(ctx, "engine1", ContextConfig{MaxAge: time.Hour})

	ctx2, cancel2 := context.WithCancel(context.Background())
	resultCh := make(chan *SlotResult, 1)
	go func() {
		r, _ := slot.Acquire(ctx2, "engine2", ContextConfig{MaxAge: time.Hour})
		resultCh <- r
	}()

	time.Sleep(50 * time.Millisecond)
	cancel2()

	select {
	case r := <-resultCh:
		if r.Granted {
			t.Error("expected slot not granted after context cancel")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for context cancel result")
	}

	_ = cancel
}
