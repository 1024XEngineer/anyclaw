package qmd

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type HealthStatus string

const (
	HealthHealthy   HealthStatus = "healthy"
	HealthDegraded  HealthStatus = "degraded"
	HealthUnhealthy HealthStatus = "unhealthy"
	HealthStarting  HealthStatus = "starting"
	HealthStopping  HealthStatus = "stopping"
)

type HealthCheck struct {
	Name      string        `json:"name"`
	Status    HealthStatus  `json:"status"`
	Message   string        `json:"message"`
	Duration  time.Duration `json:"duration"`
	Timestamp time.Time     `json:"timestamp"`
}

type HealthReport struct {
	Status    HealthStatus  `json:"status"`
	Uptime    string        `json:"uptime"`
	Checks    []HealthCheck `json:"checks"`
	Stats     Stats         `json:"stats"`
	Timestamp time.Time     `json:"timestamp"`
}

type HealthChecker struct {
	checks     []CheckFunc
	mu         sync.RWMutex
	lastReport HealthReport
}

type CheckFunc func(ctx context.Context) HealthCheck

func NewHealthChecker() *HealthChecker {
	return &HealthChecker{
		checks: make([]CheckFunc, 0),
	}
}

func (hc *HealthChecker) Register(name string, fn CheckFunc) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	hc.checks = append(hc.checks, fn)
}

func (hc *HealthChecker) Check(ctx context.Context) HealthReport {
	hc.mu.RLock()
	checks := make([]CheckFunc, len(hc.checks))
	copy(checks, hc.checks)
	hc.mu.RUnlock()

	report := HealthReport{
		Status:    HealthHealthy,
		Timestamp: time.Now(),
		Checks:    make([]HealthCheck, 0, len(checks)),
	}

	for _, fn := range checks {
		check := fn(ctx)
		report.Checks = append(report.Checks, check)

		if check.Status == HealthUnhealthy {
			report.Status = HealthUnhealthy
		} else if check.Status == HealthDegraded && report.Status != HealthUnhealthy {
			report.Status = HealthDegraded
		}
	}

	hc.mu.Lock()
	hc.lastReport = report
	hc.mu.Unlock()

	return report
}

func (hc *HealthChecker) LastReport() HealthReport {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	return hc.lastReport
}

func PingCheck(ctx context.Context, client *Client) CheckFunc {
	return func(ctx context.Context) HealthCheck {
		start := time.Now()
		err := client.Ping(ctx)
		status := HealthHealthy
		message := "ok"

		if err != nil {
			status = HealthUnhealthy
			message = err.Error()
		}

		return HealthCheck{
			Name:      "ping",
			Status:    status,
			Message:   message,
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		}
	}
}

func StoreCheck(store *Store) CheckFunc {
	return func(ctx context.Context) HealthCheck {
		start := time.Now()
		stats := store.Stats()

		status := HealthHealthy
		message := fmt.Sprintf("%d tables, %d rows", stats.TableCount, stats.TotalRows)

		if stats.TotalRows > 1000000 {
			status = HealthDegraded
			message = fmt.Sprintf("high row count: %d", stats.TotalRows)
		}

		return HealthCheck{
			Name:      "store",
			Status:    status,
			Message:   message,
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		}
	}
}

func WALCheck(store *Store, maxWALSize int64) CheckFunc {
	return func(ctx context.Context) HealthCheck {
		start := time.Now()
		walSize := store.Stats().WALEntries

		status := HealthHealthy
		message := fmt.Sprintf("WAL entries: %d", walSize)

		if walSize >= maxWALSize {
			status = HealthDegraded
			message = fmt.Sprintf("WAL size exceeds threshold: %d > %d", walSize, maxWALSize)
		}

		return HealthCheck{
			Name:      "wal",
			Status:    status,
			Message:   message,
			Duration:  time.Since(start),
			Timestamp: time.Now(),
		}
	}
}

type RestartPolicy string

const (
	RestartAlways    RestartPolicy = "always"
	RestartOnFailure RestartPolicy = "on-failure"
	RestartNever     RestartPolicy = "never"
)

type RestartConfig struct {
	Policy              RestartPolicy
	MaxRetries          int
	InitialDelay        time.Duration
	MaxDelay            time.Duration
	BackoffFactor       float64
	HealthCheckInterval time.Duration
	UnhealthyThreshold  int
	OnRestart           func(attempt int, delay time.Duration)
	OnHealthy           func()
	OnUnhealthy         func(report HealthReport)
}

func DefaultRestartConfig() RestartConfig {
	return RestartConfig{
		Policy:              RestartOnFailure,
		MaxRetries:          5,
		InitialDelay:        1 * time.Second,
		MaxDelay:            60 * time.Second,
		BackoffFactor:       2.0,
		HealthCheckInterval: 10 * time.Second,
		UnhealthyThreshold:  3,
	}
}

type ServerFactory func() (*Server, error)

type Supervisor struct {
	config         RestartConfig
	serverFactory  ServerFactory
	server         *Server
	running        atomic.Bool
	restarting     atomic.Bool
	restartCount   atomic.Int32
	unhealthyCount atomic.Int32
	mu             sync.Mutex
	stopCh         chan struct{}
	doneCh         chan struct{}
	healthChecker  *HealthChecker
	client         *Client
	serverAddr     string
}

func NewSupervisor(serverFactory ServerFactory, cfg RestartConfig) *Supervisor {
	if cfg.HealthCheckInterval <= 0 {
		cfg.HealthCheckInterval = 10 * time.Second
	}
	if cfg.UnhealthyThreshold <= 0 {
		cfg.UnhealthyThreshold = 3
	}

	return &Supervisor{
		config:        cfg,
		serverFactory: serverFactory,
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
		healthChecker: NewHealthChecker(),
	}
}

func (s *Supervisor) Start(ctx context.Context) error {
	if !s.running.CompareAndSwap(false, true) {
		return fmt.Errorf("qmd: supervisor already running")
	}

	if err := s.startServer(ctx); err != nil {
		return err
	}

	go s.monitorLoop(ctx)

	return nil
}

func (s *Supervisor) Stop(ctx context.Context) error {
	if !s.running.CompareAndSwap(true, false) {
		return nil
	}

	close(s.stopCh)

	s.mu.Lock()
	srv := s.server
	s.mu.Unlock()

	if srv != nil {
		if err := srv.Shutdown(ctx); err != nil {
			return err
		}
	}

	<-s.doneCh
	return nil
}

func (s *Supervisor) Server() *Server {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.server
}

func (s *Supervisor) Client() *Client {
	return s.client
}

func (s *Supervisor) HealthChecker() *HealthChecker {
	return s.healthChecker
}

func (s *Supervisor) RestartCount() int32 {
	return s.restartCount.Load()
}

func (s *Supervisor) startServer(ctx context.Context) error {
	srv, err := s.serverFactory()
	if err != nil {
		return fmt.Errorf("qmd: create server: %w", err)
	}

	if err := srv.Start(); err != nil {
		return fmt.Errorf("qmd: start server: %w", err)
	}

	s.mu.Lock()
	s.server = srv
	s.serverAddr = srv.HTTPAddr()
	s.mu.Unlock()

	s.client = NewClient(ClientConfig{
		Address:    "http://" + s.serverAddr,
		Protocol:   ProtocolHTTP,
		Timeout:    5 * time.Second,
		RetryCount: 3,
		RetryDelay: 100 * time.Millisecond,
	})

	s.healthChecker.Register("ping", PingCheck(ctx, s.client))
	s.healthChecker.Register("store", StoreCheck(srv.Store()))
	s.healthChecker.Register("wal", WALCheck(srv.Store(), 100000))

	return nil
}

func (s *Supervisor) monitorLoop(ctx context.Context) {
	defer close(s.doneCh)
	defer s.running.Store(false)

	ticker := time.NewTicker(s.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.checkHealth(ctx)
		}
	}
}

func (s *Supervisor) checkHealth(ctx context.Context) {
	if s.restarting.Load() {
		return
	}

	report := s.healthChecker.Check(ctx)

	if report.Status == HealthHealthy {
		s.unhealthyCount.Store(0)
		if s.config.OnHealthy != nil {
			s.config.OnHealthy()
		}
		return
	}

	unhealthy := s.unhealthyCount.Add(1)
	if unhealthy >= int32(s.config.UnhealthyThreshold) {
		if s.config.OnUnhealthy != nil {
			s.config.OnUnhealthy(report)
		}

		if s.config.Policy == RestartNever {
			return
		}

		if s.config.Policy == RestartOnFailure && report.Status == HealthDegraded {
			return
		}

		go s.doRestart(ctx)
	}
}

func (s *Supervisor) doRestart(ctx context.Context) {
	if !s.restarting.CompareAndSwap(false, true) {
		return
	}
	defer s.restarting.Store(false)

	retries := s.restartCount.Load()
	if s.config.MaxRetries > 0 && retries >= int32(s.config.MaxRetries) {
		return
	}

	delay := s.config.InitialDelay
	for i := int32(0); i < retries; i++ {
		delay = time.Duration(float64(delay) * s.config.BackoffFactor)
		if delay > s.config.MaxDelay {
			delay = s.config.MaxDelay
		}
	}

	if s.config.OnRestart != nil {
		s.config.OnRestart(int(retries+1), delay)
	}

	select {
	case <-ctx.Done():
		return
	case <-time.After(delay):
	}

	s.mu.Lock()
	srv := s.server
	s.mu.Unlock()

	if srv != nil {
		srv.Shutdown(context.Background())
	}

	if err := s.startServer(ctx); err != nil {
		return
	}

	s.restartCount.Add(1)
}

func (s *Supervisor) ForceRestart(ctx context.Context) error {
	if !s.restarting.CompareAndSwap(false, true) {
		return fmt.Errorf("qmd: restart already in progress")
	}
	defer s.restarting.Store(false)

	s.mu.Lock()
	srv := s.server
	s.mu.Unlock()

	if srv != nil {
		if err := srv.Shutdown(ctx); err != nil {
			return err
		}
	}

	if err := s.startServer(ctx); err != nil {
		return err
	}

	s.restartCount.Add(1)
	return nil
}
