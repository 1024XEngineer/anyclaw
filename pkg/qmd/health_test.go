package qmd

import (
	"context"
	"testing"
	"time"
)

func TestHealthCheckerBasic(t *testing.T) {
	hc := NewHealthChecker()

	hc.Register("check1", func(ctx context.Context) HealthCheck {
		return HealthCheck{
			Name:   "check1",
			Status: HealthHealthy,
		}
	})

	report := hc.Check(context.Background())

	if report.Status != HealthHealthy {
		t.Errorf("expected healthy, got %s", report.Status)
	}
	if len(report.Checks) != 1 {
		t.Errorf("expected 1 check, got %d", len(report.Checks))
	}
}

func TestHealthCheckerUnhealthy(t *testing.T) {
	hc := NewHealthChecker()

	hc.Register("bad", func(ctx context.Context) HealthCheck {
		return HealthCheck{
			Name:   "bad",
			Status: HealthUnhealthy,
		}
	})

	report := hc.Check(context.Background())

	if report.Status != HealthUnhealthy {
		t.Errorf("expected unhealthy, got %s", report.Status)
	}
}

func TestHealthCheckerDegraded(t *testing.T) {
	hc := NewHealthChecker()

	hc.Register("degraded", func(ctx context.Context) HealthCheck {
		return HealthCheck{
			Name:   "degraded",
			Status: HealthDegraded,
		}
	})

	report := hc.Check(context.Background())

	if report.Status != HealthDegraded {
		t.Errorf("expected degraded, got %s", report.Status)
	}
}

func TestHealthCheckerLastReport(t *testing.T) {
	hc := NewHealthChecker()

	hc.Register("test", func(ctx context.Context) HealthCheck {
		return HealthCheck{Name: "test", Status: HealthHealthy}
	})

	hc.Check(context.Background())

	report := hc.LastReport()
	if report.Status != HealthHealthy {
		t.Errorf("expected healthy last report, got %s", report.Status)
	}
}

func TestHealthCheckerMultipleChecks(t *testing.T) {
	hc := NewHealthChecker()

	hc.Register("good", func(ctx context.Context) HealthCheck {
		return HealthCheck{Name: "good", Status: HealthHealthy}
	})
	hc.Register("bad", func(ctx context.Context) HealthCheck {
		return HealthCheck{Name: "bad", Status: HealthUnhealthy}
	})

	report := hc.Check(context.Background())

	if report.Status != HealthUnhealthy {
		t.Errorf("expected unhealthy with one bad check, got %s", report.Status)
	}
	if len(report.Checks) != 2 {
		t.Errorf("expected 2 checks, got %d", len(report.Checks))
	}
}

func TestStoreCheck(t *testing.T) {
	store := NewStore()
	store.CreateTable("users", nil)
	store.Insert("users", &Record{ID: "u1"})

	check := StoreCheck(store)
	result := check(context.Background())

	if result.Status != HealthHealthy {
		t.Errorf("expected healthy, got %s", result.Status)
	}
	if result.Name != "store" {
		t.Errorf("expected name store, got %s", result.Name)
	}
}

func TestWALCheck(t *testing.T) {
	store := NewStore()
	store.CreateTable("t", nil)
	store.Insert("t", &Record{ID: "r1"})

	check := WALCheck(store, 1000)
	result := check(context.Background())

	if result.Status != HealthHealthy {
		t.Errorf("expected healthy, got %s", result.Status)
	}
}

func TestWALCheckDegraded(t *testing.T) {
	store := NewStore()
	store.CreateTable("t", nil)
	store.Insert("t", &Record{ID: "r1"})

	check := WALCheck(store, 0)
	result := check(context.Background())

	if result.Status != HealthDegraded {
		t.Errorf("expected degraded for WAL exceeding threshold, got %s", result.Status)
	}
}

func TestSupervisorStartStop(t *testing.T) {
	sup := NewSupervisor(func() (*Server, error) {
		return NewServer(ServerConfig{HTTPAddr: ":19879"}), nil
	}, DefaultRestartConfig())

	ctx := context.Background()
	if err := sup.Start(ctx); err != nil {
		t.Fatalf("start supervisor: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	if sup.Server() == nil {
		t.Error("expected server to be set")
	}
	if sup.Client() == nil {
		t.Error("expected client to be set")
	}

	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := sup.Stop(shutdownCtx); err != nil {
		t.Fatalf("stop supervisor: %v", err)
	}
}

func TestSupervisorDoubleStart(t *testing.T) {
	sup := NewSupervisor(func() (*Server, error) {
		return NewServer(ServerConfig{HTTPAddr: ":19880"}), nil
	}, DefaultRestartConfig())

	ctx := context.Background()
	sup.Start(ctx)
	defer sup.Stop(ctx)

	err := sup.Start(ctx)
	if err == nil {
		t.Error("expected error on double start")
	}
}

func TestSupervisorHealthCheck(t *testing.T) {
	sup := NewSupervisor(func() (*Server, error) {
		return NewServer(ServerConfig{HTTPAddr: ":19890"}), nil
	}, RestartConfig{
		HealthCheckInterval: 200 * time.Millisecond,
		UnhealthyThreshold:  100,
	})

	ctx := context.Background()
	sup.Start(ctx)
	defer sup.Stop(ctx)

	time.Sleep(500 * time.Millisecond)

	report := sup.HealthChecker().LastReport()
	if report.Status == "" {
		t.Error("expected health report")
	}
}

func TestSupervisorForceRestart(t *testing.T) {
	sup := NewSupervisor(func() (*Server, error) {
		return NewServer(ServerConfig{HTTPAddr: ":19891"}), nil
	}, DefaultRestartConfig())

	ctx := context.Background()
	sup.Start(ctx)
	defer sup.Stop(ctx)

	time.Sleep(300 * time.Millisecond)

	oldServer := sup.Server()

	if err := sup.ForceRestart(ctx); err != nil {
		t.Fatalf("force restart: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	if sup.Server() == oldServer {
		t.Error("expected new server after restart")
	}
	if sup.RestartCount() != 1 {
		t.Errorf("expected restart count 1, got %d", sup.RestartCount())
	}
}

func TestSupervisorRestartCallbacks(t *testing.T) {
	var healthyCalled bool

	sup := NewSupervisor(func() (*Server, error) {
		return NewServer(ServerConfig{HTTPAddr: ":19892"}), nil
	}, RestartConfig{
		HealthCheckInterval: 200 * time.Millisecond,
		UnhealthyThreshold:  100,
		OnRestart: func(attempt int, delay time.Duration) {
		},
		OnHealthy: func() {
			healthyCalled = true
		},
	})

	ctx := context.Background()
	sup.Start(ctx)
	defer sup.Stop(ctx)

	time.Sleep(500 * time.Millisecond)

	if !healthyCalled {
		t.Error("expected OnHealthy callback")
	}
}

func TestSupervisorUnhealthyCallback(t *testing.T) {
	var unhealthyCalled bool
	var report HealthReport

	sup := NewSupervisor(func() (*Server, error) {
		s := NewServer(ServerConfig{HTTPAddr: ":19893"})
		s.Store().Clear()
		return s, nil
	}, RestartConfig{
		Policy:              RestartNever,
		HealthCheckInterval: 200 * time.Millisecond,
		UnhealthyThreshold:  1,
		OnUnhealthy: func(r HealthReport) {
			unhealthyCalled = true
			report = r
		},
	})

	sup.healthChecker.Register("always_bad", func(ctx context.Context) HealthCheck {
		return HealthCheck{Name: "bad", Status: HealthUnhealthy, Message: "test failure"}
	})

	ctx := context.Background()
	sup.Start(ctx)
	defer sup.Stop(ctx)

	time.Sleep(500 * time.Millisecond)

	if !unhealthyCalled {
		t.Error("expected OnUnhealthy callback")
	}
	if report.Status != HealthUnhealthy {
		t.Errorf("expected unhealthy report, got %s", report.Status)
	}
}

func TestSupervisorRestartPolicyNever(t *testing.T) {
	sup := NewSupervisor(func() (*Server, error) {
		return NewServer(ServerConfig{HTTPAddr: ":19894"}), nil
	}, RestartConfig{
		Policy:              RestartNever,
		HealthCheckInterval: 200 * time.Millisecond,
		UnhealthyThreshold:  1,
	})

	sup.healthChecker.Register("always_bad", func(ctx context.Context) HealthCheck {
		return HealthCheck{Name: "bad", Status: HealthUnhealthy}
	})

	ctx := context.Background()
	sup.Start(ctx)
	defer sup.Stop(ctx)

	time.Sleep(500 * time.Millisecond)

	if sup.RestartCount() != 0 {
		t.Errorf("expected 0 restarts with RestartNever, got %d", sup.RestartCount())
	}
}

func TestSupervisorRestartPolicyOnFailureDegraded(t *testing.T) {
	sup := NewSupervisor(func() (*Server, error) {
		return NewServer(ServerConfig{HTTPAddr: ":19895"}), nil
	}, RestartConfig{
		Policy:              RestartOnFailure,
		HealthCheckInterval: 200 * time.Millisecond,
		UnhealthyThreshold:  1,
	})

	sup.healthChecker.Register("degraded", func(ctx context.Context) HealthCheck {
		return HealthCheck{Name: "degraded", Status: HealthDegraded}
	})

	ctx := context.Background()
	sup.Start(ctx)
	defer sup.Stop(ctx)

	time.Sleep(500 * time.Millisecond)

	if sup.RestartCount() != 0 {
		t.Errorf("expected 0 restarts for degraded with OnFailure policy, got %d", sup.RestartCount())
	}
}

func TestPingCheck(t *testing.T) {
	s := NewServer(ServerConfig{HTTPAddr: ":19887"})
	s.Start()
	defer s.Shutdown(context.Background())

	time.Sleep(100 * time.Millisecond)

	client := NewClient(ClientConfig{
		Address:  "http://localhost:19887",
		Protocol: ProtocolHTTP,
		Timeout:  5 * time.Second,
	})

	check := PingCheck(context.Background(), client)
	result := check(context.Background())

	if result.Status != HealthHealthy {
		t.Errorf("expected healthy ping, got %s", result.Status)
	}
	if result.Name != "ping" {
		t.Errorf("expected name ping, got %s", result.Name)
	}
}

func TestPingCheckUnhealthy(t *testing.T) {
	client := NewClient(ClientConfig{
		Address:  "http://localhost:19999",
		Protocol: ProtocolHTTP,
		Timeout:  100 * time.Millisecond,
	})

	check := PingCheck(context.Background(), client)
	result := check(context.Background())

	if result.Status != HealthUnhealthy {
		t.Errorf("expected unhealthy ping, got %s", result.Status)
	}
}

func TestHealthReportStats(t *testing.T) {
	store := NewStore()
	store.CreateTable("users", nil)

	hc := NewHealthChecker()
	hc.Register("store", StoreCheck(store))

	report := hc.Check(context.Background())

	if report.Stats.TableCount != 0 {
		t.Errorf("expected 0 tables in stats, got %d", report.Stats.TableCount)
	}
}
