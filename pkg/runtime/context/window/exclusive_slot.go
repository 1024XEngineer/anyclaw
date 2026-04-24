package contextengine

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type SlotState string

const (
	SlotActive     SlotState = "active"
	SlotInactive   SlotState = "inactive"
	SlotPending    SlotState = "pending"
	SlotTerminated SlotState = "terminated"
)

type ExclusiveSlot struct {
	mu           sync.Mutex
	activeEngine *Engine
	activeID     string
	createdAt    time.Time
	heartbeatAt  time.Time
	maxIdle      time.Duration
	maxDuration  time.Duration
	state        SlotState
	pendingQueue []*SlotRequest
	onActivate   func(id string)
	onDeactivate func(id string)
	onTimeout    func(id string)
}

type SlotRequest struct {
	ID       string
	Priority int
	Ch       chan *SlotResult
	Timeout  time.Duration
}

type SlotResult struct {
	Engine  *Engine
	SlotID  string
	Granted bool
	Error   error
}

type SlotConfig struct {
	MaxIdle      time.Duration
	MaxDuration  time.Duration
	MaxPending   int
	OnActivate   func(id string)
	OnDeactivate func(id string)
	OnTimeout    func(id string)
}

func DefaultSlotConfig() SlotConfig {
	return SlotConfig{
		MaxIdle:     5 * time.Minute,
		MaxDuration: 30 * time.Minute,
		MaxPending:  10,
	}
}

func NewExclusiveSlot(cfg SlotConfig) *ExclusiveSlot {
	if cfg.MaxIdle <= 0 {
		cfg.MaxIdle = 5 * time.Minute
	}
	if cfg.MaxDuration <= 0 {
		cfg.MaxDuration = 30 * time.Minute
	}
	if cfg.MaxPending <= 0 {
		cfg.MaxPending = 10
	}

	s := &ExclusiveSlot{
		state:        SlotInactive,
		maxIdle:      cfg.MaxIdle,
		maxDuration:  cfg.MaxDuration,
		pendingQueue: make([]*SlotRequest, 0, cfg.MaxPending),
		onActivate:   cfg.OnActivate,
		onDeactivate: cfg.OnDeactivate,
		onTimeout:    cfg.OnTimeout,
	}

	go s.idleMonitor()
	return s
}

func (s *ExclusiveSlot) Acquire(ctx context.Context, id string, cfg ContextConfig) (*SlotResult, error) {
	s.mu.Lock()

	if s.state == SlotActive && s.activeID == id {
		s.heartbeatAt = time.Now()
		result := &SlotResult{
			Engine:  s.activeEngine,
			SlotID:  s.activeID,
			Granted: true,
		}
		s.mu.Unlock()
		return result, nil
	}

	if s.state == SlotActive {
		if len(s.pendingQueue) >= cap(s.pendingQueue) {
			s.mu.Unlock()
			return &SlotResult{Granted: false, Error: fmt.Errorf("slot queue full")}, nil
		}

		ch := make(chan *SlotResult, 1)
		req := &SlotRequest{
			ID:      id,
			Ch:      ch,
			Timeout: 30 * time.Second,
		}

		for i, p := range s.pendingQueue {
			if p.Priority < req.Priority {
				s.pendingQueue = append(s.pendingQueue, nil)
				copy(s.pendingQueue[i+1:], s.pendingQueue[i:])
				s.pendingQueue[i] = req
				goto queued
			}
		}
		s.pendingQueue = append(s.pendingQueue, req)

	queued:
		s.mu.Unlock()

		select {
		case result := <-ch:
			return result, nil
		case <-ctx.Done():
			s.removeFromQueue(id)
			return &SlotResult{Granted: false, Error: ctx.Err()}, nil
		case <-time.After(req.Timeout):
			s.removeFromQueue(id)
			return &SlotResult{Granted: false, Error: fmt.Errorf("slot acquire timeout")}, nil
		}
	}

	engine := New(cfg)
	s.activeEngine = engine
	s.activeID = id
	s.state = SlotActive
	s.createdAt = time.Now()
	s.heartbeatAt = time.Now()
	s.mu.Unlock()

	if s.onActivate != nil {
		s.onActivate(id)
	}

	return &SlotResult{
		Engine:  engine,
		SlotID:  id,
		Granted: true,
	}, nil
}

func (s *ExclusiveSlot) Release(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != SlotActive || s.activeID != id {
		return
	}

	s.deactivateLocked()
	s.grantNextLocked()
}

func (s *ExclusiveSlot) ForceRelease(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != SlotActive {
		return fmt.Errorf("no active slot")
	}
	if s.activeID != id {
		return fmt.Errorf("slot %s is not active (active: %s)", id, s.activeID)
	}

	s.deactivateLocked()
	s.grantNextLocked()
	return nil
}

func (s *ExclusiveSlot) Heartbeat(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != SlotActive || s.activeID != id {
		return fmt.Errorf("slot %s is not active", id)
	}

	s.heartbeatAt = time.Now()
	return nil
}

func (s *ExclusiveSlot) Status() SlotStatus {
	s.mu.Lock()
	defer s.mu.Unlock()

	status := SlotStatus{
		State:       s.state,
		ActiveID:    s.activeID,
		Pending:     len(s.pendingQueue),
		CreatedAt:   s.createdAt,
		HeartbeatAt: s.heartbeatAt,
	}

	if s.state == SlotActive {
		status.IdleDuration = time.Since(s.heartbeatAt)
		status.ActiveDuration = time.Since(s.createdAt)
	}

	return status
}

func (s *ExclusiveSlot) IsActive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state == SlotActive
}

func (s *ExclusiveSlot) ActiveID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activeID
}

func (s *ExclusiveSlot) Engine() *Engine {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activeEngine
}

func (s *ExclusiveSlot) Terminate(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != SlotActive || s.activeID != id {
		return fmt.Errorf("slot %s is not active", id)
	}

	s.state = SlotTerminated
	s.activeEngine = nil
	s.activeID = ""

	if s.onDeactivate != nil {
		s.onDeactivate(id)
	}

	s.drainQueueLocked()
	return nil
}

func (s *ExclusiveSlot) SetMaxIdle(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.maxIdle = d
}

func (s *ExclusiveSlot) SetMaxDuration(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.maxDuration = d
}

func (s *ExclusiveSlot) removeFromQueue(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, req := range s.pendingQueue {
		if req.ID == id {
			s.pendingQueue = append(s.pendingQueue[:i], s.pendingQueue[i+1:]...)
			return
		}
	}
}

func (s *ExclusiveSlot) deactivateLocked() {
	id := s.activeID
	s.state = SlotInactive
	s.activeEngine = nil
	s.activeID = ""

	if s.onDeactivate != nil {
		s.onDeactivate(id)
	}
}

func (s *ExclusiveSlot) grantNextLocked() {
	if len(s.pendingQueue) == 0 {
		return
	}

	req := s.pendingQueue[0]
	s.pendingQueue = s.pendingQueue[1:]

	engine := New(ContextConfig{
		MaxAge:     30 * time.Minute,
		AutoExpire: true,
	})

	s.activeEngine = engine
	s.activeID = req.ID
	s.state = SlotActive
	s.createdAt = time.Now()
	s.heartbeatAt = time.Now()

	select {
	case req.Ch <- &SlotResult{
		Engine:  engine,
		SlotID:  req.ID,
		Granted: true,
	}:
	default:
	}

	if s.onActivate != nil {
		s.onActivate(req.ID)
	}
}

func (s *ExclusiveSlot) drainQueueLocked() {
	for _, req := range s.pendingQueue {
		select {
		case req.Ch <- &SlotResult{
			Granted: false,
			Error:   fmt.Errorf("slot terminated"),
		}:
		default:
		}
	}
	s.pendingQueue = s.pendingQueue[:0]
}

func (s *ExclusiveSlot) idleMonitor() {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.Lock()
		if s.state != SlotActive {
			s.mu.Unlock()
			continue
		}

		idleDuration := time.Since(s.heartbeatAt)
		activeDuration := time.Since(s.createdAt)

		if idleDuration > s.maxIdle || activeDuration > s.maxDuration {
			id := s.activeID
			s.deactivateLocked()

			if s.onTimeout != nil {
				s.onTimeout(id)
			}

			s.grantNextLocked()
		}

		s.mu.Unlock()
	}
}

type SlotStatus struct {
	State          SlotState     `json:"state"`
	ActiveID       string        `json:"active_id"`
	Pending        int           `json:"pending"`
	CreatedAt      time.Time     `json:"created_at"`
	HeartbeatAt    time.Time     `json:"heartbeat_at"`
	IdleDuration   time.Duration `json:"idle_duration"`
	ActiveDuration time.Duration `json:"active_duration"`
}
