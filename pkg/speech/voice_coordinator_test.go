package speech

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestWakeArbitration_FirstResponse(t *testing.T) {
	cfg := DefaultWakeArbitrationConfig()
	cfg.ElectionMode = ElectionFirstResponse

	arb := NewWakeArbitration("device-1", cfg)

	done := make(chan WakeArbitrationResult, 1)
	arb.RegisterListener(func(result WakeArbitrationResult) {
		done <- result
	})

	arb.SubmitLocalWake(WakeEvent{
		DeviceID:   "device-1",
		DeviceName: "Device 1",
		Phrase:     "hey assistant",
		Confidence: 0.8,
		Energy:     0.6,
		Priority:   50,
	})

	time.Sleep(50 * time.Millisecond)

	arb.SubmitRemoteWake(WakeEvent{
		DeviceID:   "device-2",
		DeviceName: "Device 2",
		Phrase:     "hey assistant",
		Confidence: 0.9,
		Energy:     0.8,
		Priority:   60,
		Timestamp:  time.Now().Add(10 * time.Millisecond),
	})

	select {
	case result := <-done:
		if result.WinnerID != "device-1" {
			t.Errorf("expected device-1 to win (first response), got %s", result.WinnerID)
		}
		if !result.IsLocal {
			t.Error("expected local device to win")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for arbitration result")
	}
}

func TestWakeArbitration_BestSignal(t *testing.T) {
	cfg := DefaultWakeArbitrationConfig()
	cfg.ElectionMode = ElectionBestSignal

	arb := NewWakeArbitration("device-1", cfg)

	done := make(chan WakeArbitrationResult, 1)
	arb.RegisterListener(func(result WakeArbitrationResult) {
		done <- result
	})

	arb.SubmitLocalWake(WakeEvent{
		DeviceID:   "device-1",
		DeviceName: "Device 1",
		Phrase:     "hey assistant",
		Confidence: 0.5,
		Energy:     0.3,
		Priority:   50,
	})

	time.Sleep(50 * time.Millisecond)

	arb.SubmitRemoteWake(WakeEvent{
		DeviceID:   "device-2",
		DeviceName: "Device 2",
		Phrase:     "hey assistant",
		Confidence: 0.95,
		Energy:     0.9,
		Priority:   70,
		Timestamp:  time.Now().Add(10 * time.Millisecond),
	})

	select {
	case result := <-done:
		if result.WinnerID != "device-2" {
			t.Errorf("expected device-2 to win (best signal), got %s", result.WinnerID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for arbitration result")
	}
}

func TestWakeArbitration_HighestPriority(t *testing.T) {
	cfg := DefaultWakeArbitrationConfig()
	cfg.ElectionMode = ElectionHighestPriority

	arb := NewWakeArbitration("device-1", cfg)

	done := make(chan WakeArbitrationResult, 1)
	arb.RegisterListener(func(result WakeArbitrationResult) {
		done <- result
	})

	arb.SubmitLocalWake(WakeEvent{
		DeviceID:   "device-1",
		DeviceName: "Device 1",
		Phrase:     "hey assistant",
		Confidence: 0.8,
		Priority:   30,
	})

	time.Sleep(50 * time.Millisecond)

	arb.SubmitRemoteWake(WakeEvent{
		DeviceID:   "device-2",
		DeviceName: "Device 2",
		Phrase:     "hey assistant",
		Confidence: 0.7,
		Priority:   90,
		Timestamp:  time.Now().Add(10 * time.Millisecond),
	})

	select {
	case result := <-done:
		if result.WinnerID != "device-2" {
			t.Errorf("expected device-2 to win (highest priority), got %s", result.WinnerID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for arbitration result")
	}
}

func TestWakeArbitration_Suppression(t *testing.T) {
	cfg := DefaultWakeArbitrationConfig()
	cfg.ArbitrationWindow = 100 * time.Millisecond

	arb := NewWakeArbitration("device-1", cfg)
	suppressor := NewWakeSuppressor()
	arb.SetSuppressor(suppressor)

	done := make(chan WakeArbitrationResult, 1)
	arb.RegisterListener(func(result WakeArbitrationResult) {
		done <- result
	})

	arb.SubmitLocalWake(WakeEvent{
		DeviceID:   "device-1",
		DeviceName: "Device 1",
		Phrase:     "hey assistant",
		Confidence: 0.5,
		Priority:   30,
	})

	time.Sleep(50 * time.Millisecond)

	arb.SubmitRemoteWake(WakeEvent{
		DeviceID:   "device-2",
		DeviceName: "Device 2",
		Phrase:     "hey assistant",
		Confidence: 0.9,
		Priority:   80,
		Timestamp:  time.Now().Add(10 * time.Millisecond),
	})

	select {
	case result := <-done:
		if result.WinnerID != "device-2" {
			t.Errorf("expected device-2 to win, got %s", result.WinnerID)
		}

		time.Sleep(10 * time.Millisecond)

		if !suppressor.IsSuppressed() {
			t.Error("expected suppressor to be active after losing arbitration")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for arbitration result")
	}
}

func TestWakeArbitration_MinConfidence(t *testing.T) {
	cfg := DefaultWakeArbitrationConfig()
	cfg.MinConfidence = 0.5

	arb := NewWakeArbitration("device-1", cfg)

	done := make(chan WakeArbitrationResult, 1)
	arb.RegisterListener(func(result WakeArbitrationResult) {
		done <- result
	})

	arb.SubmitLocalWake(WakeEvent{
		DeviceID:   "device-1",
		DeviceName: "Device 1",
		Phrase:     "hey assistant",
		Confidence: 0.3,
		Priority:   50,
	})

	select {
	case <-done:
		t.Error("should not have triggered arbitration for low confidence wake")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestWakeSuppressor_Basic(t *testing.T) {
	s := NewWakeSuppressor()

	if s.IsSuppressed() {
		t.Error("should not be suppressed initially")
	}

	s.Suppress("device-2", "Device 2", 500*time.Millisecond)

	if !s.IsSuppressed() {
		t.Error("should be suppressed after Suppress() call")
	}

	byID, byName := s.SuppressedBy()
	if byID != "device-2" {
		t.Errorf("expected suppressed by device-2, got %s", byID)
	}
	if byName != "Device 2" {
		t.Errorf("expected suppressed by Device 2, got %s", byName)
	}

	time.Sleep(600 * time.Millisecond)

	if s.IsSuppressed() {
		t.Error("should not be suppressed after duration expired")
	}
}

func TestWakeSuppressor_IsSuppressedBy(t *testing.T) {
	s := NewWakeSuppressor()

	s.Suppress("device-2", "Device 2", 500*time.Millisecond)

	if !s.IsSuppressedBy("device-2") {
		t.Error("should be suppressed by device-2")
	}

	if s.IsSuppressedBy("device-3") {
		t.Error("should not be suppressed by device-3")
	}
}

func TestWakeSuppressor_Release(t *testing.T) {
	s := NewWakeSuppressor()

	done := make(chan SuppressionEvent, 2)
	s.RegisterListener(func(event SuppressionEvent) {
		done <- event
	})

	s.Suppress("device-2", "Device 2", 10*time.Second)

	select {
	case event := <-done:
		if event.Type != "suppressed" {
			t.Errorf("expected suppressed event, got %s", event.Type)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for suppression event")
	}

	s.Release()

	select {
	case event := <-done:
		if event.Type != "released" {
			t.Errorf("expected released event, got %s", event.Type)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for release event")
	}

	if s.IsSuppressed() {
		t.Error("should not be suppressed after manual release")
	}
}

func TestWakeSuppressor_History(t *testing.T) {
	s := NewWakeSuppressor()

	s.Suppress("device-2", "Device 2", 100*time.Millisecond)
	s.Release()
	s.Suppress("device-3", "Device 3", 100*time.Millisecond)

	history := s.History()
	if len(history) != 3 {
		t.Errorf("expected 3 history entries, got %d", len(history))
	}
}

func TestVoiceWakeCoordinator_StartStop(t *testing.T) {
	cfg := DefaultVoiceWakeCoordinatorConfig()
	cfg.DeviceID = "test-device"
	cfg.DeviceName = "Test Device"
	cfg.Enabled = true
	cfg.BroadcastPort = 19900
	cfg.WakeEventPort = 19901

	coord := NewVoiceWakeCoordinator(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := coord.Start(ctx); err != nil {
		t.Fatalf("failed to start coordinator: %v", err)
	}

	if !coord.IsRunning() {
		t.Error("coordinator should be running")
	}

	if err := coord.Stop(); err != nil {
		t.Fatalf("failed to stop coordinator: %v", err)
	}

	if coord.IsRunning() {
		t.Error("coordinator should not be running after stop")
	}
}

func TestVoiceWakeCoordinator_StopClearsWakeListenerState(t *testing.T) {
	cfg := DefaultVoiceWakeCoordinatorConfig()
	cfg.DeviceID = "test-device"
	cfg.DeviceName = "Test Device"
	cfg.Enabled = true
	cfg.BroadcastPort = 19920
	cfg.WakeEventPort = 19921

	coord := NewVoiceWakeCoordinator(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := coord.Start(ctx); err != nil {
		t.Fatalf("failed to start coordinator: %v", err)
	}

	if coord.wakeConn == nil || coord.wakeListenDone == nil {
		t.Fatal("expected wake listener state to be initialized after start")
	}

	if err := coord.Stop(); err != nil {
		t.Fatalf("failed to stop coordinator: %v", err)
	}

	if coord.wakeConn != nil {
		t.Error("expected wake connection to be cleared after stop")
	}
	if coord.wakeListenDone != nil {
		t.Error("expected wake listener done channel to be cleared after stop")
	}

	if err := coord.Stop(); err != nil {
		t.Fatalf("second stop should be a no-op: %v", err)
	}
}

func TestVoiceWakeCoordinator_StartKeepsWakeListenerRunning(t *testing.T) {
	cfg := DefaultVoiceWakeCoordinatorConfig()
	cfg.DeviceID = "test-device"
	cfg.DeviceName = "Test Device"
	cfg.Enabled = true
	cfg.BroadcastPort = 19922
	cfg.WakeEventPort = 19923

	coord := NewVoiceWakeCoordinator(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := coord.Start(ctx); err != nil {
		t.Fatalf("failed to start coordinator: %v", err)
	}
	defer coord.Stop()

	if coord.wakeListenDone == nil {
		t.Fatal("expected wake listener done channel after start")
	}

	time.Sleep(50 * time.Millisecond)

	select {
	case <-coord.wakeListenDone:
		t.Fatal("wake listener exited unexpectedly while coordinator is running")
	default:
	}
}

func TestVoiceWakeCoordinator_SubmitWake(t *testing.T) {
	cfg := DefaultVoiceWakeCoordinatorConfig()
	cfg.DeviceID = "test-device"
	cfg.DeviceName = "Test Device"
	cfg.Enabled = true
	cfg.BroadcastPort = 19902
	cfg.WakeEventPort = 19903
	cfg.ArbitrationConfig.ArbitrationWindow = 100 * time.Millisecond

	coord := NewVoiceWakeCoordinator(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := coord.Start(ctx); err != nil {
		t.Fatalf("failed to start coordinator: %v", err)
	}
	defer coord.Stop()

	allowed := coord.SubmitWake("hey assistant", 0.8, 0.6, "builtin")
	if !allowed {
		t.Error("expected wake submission to be allowed")
	}

	stats := coord.Stats()
	if stats.WakesSubmitted != 1 {
		t.Errorf("expected 1 wake submitted, got %d", stats.WakesSubmitted)
	}
}

func TestVoiceWakeCoordinator_SuppressionBlocksWake(t *testing.T) {
	cfg := DefaultVoiceWakeCoordinatorConfig()
	cfg.DeviceID = "test-device"
	cfg.DeviceName = "Test Device"
	cfg.Enabled = true
	cfg.BroadcastPort = 19904
	cfg.WakeEventPort = 19905

	coord := NewVoiceWakeCoordinator(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := coord.Start(ctx); err != nil {
		t.Fatalf("failed to start coordinator: %v", err)
	}
	defer coord.Stop()

	coord.Suppressor().Suppress("device-2", "Device 2", 5*time.Second)

	allowed := coord.SubmitWake("hey assistant", 0.8, 0.6, "builtin")
	if allowed {
		t.Error("expected wake submission to be blocked when suppressed")
	}
}

func TestVoiceWakeCoordinator_ReceiveRemoteWake(t *testing.T) {
	cfg := DefaultVoiceWakeCoordinatorConfig()
	cfg.DeviceID = "test-device"
	cfg.DeviceName = "Test Device"
	cfg.Enabled = true
	cfg.BroadcastPort = 19906
	cfg.WakeEventPort = 19907
	cfg.ArbitrationConfig.ArbitrationWindow = 100 * time.Millisecond

	coord := NewVoiceWakeCoordinator(cfg)

	done := make(chan WakeArbitrationResult, 1)
	coord.Arbitration().RegisterListener(func(result WakeArbitrationResult) {
		done <- result
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := coord.Start(ctx); err != nil {
		t.Fatalf("failed to start coordinator: %v", err)
	}
	defer coord.Stop()

	coord.ReceiveRemoteWake(WakeEvent{
		DeviceID:   "device-2",
		DeviceName: "Device 2",
		Phrase:     "hey assistant",
		Confidence: 0.9,
		Energy:     0.7,
		Priority:   60,
	})

	select {
	case result := <-done:
		if result.WinnerID != "device-2" {
			t.Errorf("expected device-2 to win, got %s", result.WinnerID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for arbitration result")
	}
}

func TestVoiceWakeCoordinator_EventListeners(t *testing.T) {
	cfg := DefaultVoiceWakeCoordinatorConfig()
	cfg.DeviceID = "test-device"
	cfg.DeviceName = "Test Device"
	cfg.Enabled = true
	cfg.BroadcastPort = 19908
	cfg.WakeEventPort = 19909

	coord := NewVoiceWakeCoordinator(cfg)

	events := make(chan CoordinatorEvent, 10)
	coord.RegisterListener(func(event CoordinatorEvent) {
		events <- event
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := coord.Start(ctx); err != nil {
		t.Fatalf("failed to start coordinator: %v", err)
	}
	defer coord.Stop()

	coord.SubmitWake("hey assistant", 0.8, 0.6, "builtin")

	select {
	case event := <-events:
		if event.Type != CoordinatorEventWakeSubmitted {
			t.Errorf("expected wake_submitted event, got %s", event.Type)
		}
	case <-time.After(500 * time.Millisecond):
	}
}

func TestVoiceWakeCoordinator_ArbitrationResult(t *testing.T) {
	cfg := DefaultVoiceWakeCoordinatorConfig()
	cfg.DeviceID = "test-device"
	cfg.DeviceName = "Test Device"
	cfg.Enabled = true
	cfg.BroadcastPort = 19910
	cfg.WakeEventPort = 19911
	cfg.ArbitrationConfig.ArbitrationWindow = 100 * time.Millisecond

	coord := NewVoiceWakeCoordinator(cfg)

	var mu sync.Mutex
	var won bool
	coord.RegisterListener(func(event CoordinatorEvent) {
		mu.Lock()
		defer mu.Unlock()
		if event.Type == CoordinatorEventArbitrationWon {
			won = true
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := coord.Start(ctx); err != nil {
		t.Fatalf("failed to start coordinator: %v", err)
	}
	defer coord.Stop()

	coord.SubmitWake("hey assistant", 0.9, 0.8, "builtin")

	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	if !won {
		t.Error("expected arbitration won event for local device with high confidence")
	}
	mu.Unlock()
}

func TestVoiceWakeCoordinator_Configuration(t *testing.T) {
	cfg := DefaultVoiceWakeCoordinatorConfig()
	cfg.DeviceID = "test-device"
	cfg.DeviceName = "Test Device"
	cfg.Enabled = true
	cfg.BroadcastPort = 19912
	cfg.WakeEventPort = 19913

	coord := NewVoiceWakeCoordinator(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := coord.Start(ctx); err != nil {
		t.Fatalf("failed to start coordinator: %v", err)
	}
	defer coord.Stop()

	coord.SetPriority(75)
	coord.SetElectionMode(ElectionHighestPriority)
	coord.SetPreferLocal(false)
	coord.SetArbitrationWindow(300 * time.Millisecond)

	arbCfg := coord.Arbitration().Config()
	if arbCfg.ElectionMode != ElectionHighestPriority {
		t.Errorf("expected election mode highest-priority, got %s", arbCfg.ElectionMode)
	}
	if arbCfg.PreferLocal {
		t.Error("expected prefer local to be false")
	}
	if arbCfg.ArbitrationWindow != 300*time.Millisecond {
		t.Errorf("expected 300ms arbitration window, got %v", arbCfg.ArbitrationWindow)
	}
}

func TestVoiceWakeCoordinator_LocalDevice(t *testing.T) {
	cfg := DefaultVoiceWakeCoordinatorConfig()
	cfg.DeviceID = "my-device"
	cfg.DeviceName = "My Device"
	cfg.DeviceType = DeviceTypeSpeaker
	cfg.Enabled = true
	cfg.BroadcastPort = 19914
	cfg.WakeEventPort = 19915

	coord := NewVoiceWakeCoordinator(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := coord.Start(ctx); err != nil {
		t.Fatalf("failed to start coordinator: %v", err)
	}
	defer coord.Stop()

	local := coord.LocalDevice()
	if local.ID != "my-device" {
		t.Errorf("expected device ID my-device, got %s", local.ID)
	}
	if local.Name != "My Device" {
		t.Errorf("expected device name My Device, got %s", local.Name)
	}
	if local.Type != DeviceTypeSpeaker {
		t.Errorf("expected device type speaker, got %s", local.Type)
	}
}

func TestWakeArbitration_Clear(t *testing.T) {
	cfg := DefaultWakeArbitrationConfig()
	cfg.ArbitrationWindow = 5 * time.Second

	arb := NewWakeArbitration("device-1", cfg)

	arb.SubmitLocalWake(WakeEvent{
		DeviceID:   "device-1",
		DeviceName: "Device 1",
		Phrase:     "hey assistant",
		Confidence: 0.8,
		Priority:   50,
	})

	if arb.PendingCount() == 0 {
		t.Error("expected pending count > 0")
	}

	arb.Clear()

	if arb.PendingCount() != 0 {
		t.Errorf("expected pending count 0 after clear, got %d", arb.PendingCount())
	}
}

func TestWakeArbitration_History(t *testing.T) {
	cfg := DefaultWakeArbitrationConfig()
	cfg.ArbitrationWindow = 50 * time.Millisecond

	arb := NewWakeArbitration("device-1", cfg)

	done := make(chan WakeArbitrationResult, 5)
	arb.RegisterListener(func(result WakeArbitrationResult) {
		done <- result
	})

	for i := 0; i < 3; i++ {
		arb.SubmitLocalWake(WakeEvent{
			DeviceID:   "device-1",
			DeviceName: "Device 1",
			Phrase:     "hey assistant",
			Confidence: 0.8,
			Priority:   50,
		})
		time.Sleep(100 * time.Millisecond)
	}

	history := arb.History()
	if len(history) != 3 {
		t.Errorf("expected 3 history entries, got %d", len(history))
	}
}

func TestWakeArbitration_DifferentPhrases(t *testing.T) {
	cfg := DefaultWakeArbitrationConfig()
	cfg.ArbitrationWindow = 100 * time.Millisecond

	arb := NewWakeArbitration("device-1", cfg)

	results := make(chan WakeArbitrationResult, 2)
	arb.RegisterListener(func(result WakeArbitrationResult) {
		results <- result
	})

	arb.SubmitLocalWake(WakeEvent{
		DeviceID:   "device-1",
		DeviceName: "Device 1",
		Phrase:     "hey assistant",
		Confidence: 0.8,
		Priority:   50,
	})

	arb.SubmitLocalWake(WakeEvent{
		DeviceID:   "device-1",
		DeviceName: "Device 1",
		Phrase:     "ok computer",
		Confidence: 0.7,
		Priority:   50,
	})

	count := 0
	for i := 0; i < 2; i++ {
		select {
		case <-results:
			count++
		case <-time.After(500 * time.Millisecond):
		}
	}

	if count != 2 {
		t.Errorf("expected 2 separate arbitration results for different phrases, got %d", count)
	}
}

func TestVoiceWakeCoordinator_Disabled(t *testing.T) {
	cfg := DefaultVoiceWakeCoordinatorConfig()
	cfg.Enabled = false

	coord := NewVoiceWakeCoordinator(cfg)

	ctx := context.Background()

	if err := coord.Start(ctx); err != nil {
		t.Fatalf("start should return nil when disabled: %v", err)
	}

	if coord.IsRunning() {
		t.Error("coordinator should not be running when disabled")
	}
}

func TestVoiceWakeCoordinator_DoubleStart(t *testing.T) {
	cfg := DefaultVoiceWakeCoordinatorConfig()
	cfg.DeviceID = "test-device"
	cfg.Enabled = true
	cfg.BroadcastPort = 19916
	cfg.WakeEventPort = 19917

	coord := NewVoiceWakeCoordinator(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := coord.Start(ctx); err != nil {
		t.Fatalf("first start failed: %v", err)
	}

	if err := coord.Start(ctx); err == nil {
		t.Error("second start should fail with already running error")
	}

	coord.Stop()
}

func TestVoiceWakeCoordinator_Stats(t *testing.T) {
	cfg := DefaultVoiceWakeCoordinatorConfig()
	cfg.DeviceID = "test-device"
	cfg.DeviceName = "Test Device"
	cfg.Enabled = true
	cfg.BroadcastPort = 19918
	cfg.WakeEventPort = 19919

	coord := NewVoiceWakeCoordinator(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := coord.Start(ctx); err != nil {
		t.Fatalf("failed to start coordinator: %v", err)
	}
	defer coord.Stop()

	coord.SubmitWake("hey assistant", 0.8, 0.6, "builtin")
	coord.SubmitWake("hey assistant", 0.7, 0.5, "builtin")

	stats := coord.Stats()
	if stats.WakesSubmitted != 2 {
		t.Errorf("expected 2 wakes submitted, got %d", stats.WakesSubmitted)
	}
}
