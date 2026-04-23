package speech

import (
	"sync"
	"time"
)

type WakeArbitrationConfig struct {
	ArbitrationWindow time.Duration
	MinConfidence     float64
	PreferLocal       bool
	LocalPriority     int
	ElectionMode      ElectionMode
}

type ElectionMode string

const (
	ElectionFirstResponse   ElectionMode = "first-response"
	ElectionBestSignal      ElectionMode = "best-signal"
	ElectionHighestPriority ElectionMode = "highest-priority"
)

func DefaultWakeArbitrationConfig() WakeArbitrationConfig {
	return WakeArbitrationConfig{
		ArbitrationWindow: 500 * time.Millisecond,
		MinConfidence:     0.3,
		PreferLocal:       true,
		LocalPriority:     50,
		ElectionMode:      ElectionBestSignal,
	}
}

type WakeEvent struct {
	DeviceID   string
	DeviceName string
	Phrase     string
	Confidence float64
	Energy     float64
	Timestamp  time.Time
	Engine     string
	Priority   int
}

type WakeArbitrationResult struct {
	WinnerID   string
	WinnerName string
	IsLocal    bool
	Confidence float64
	AllEvents  []WakeEvent
	DecidedAt  time.Time
}

type WakeArbitration struct {
	mu         sync.Mutex
	cfg        WakeArbitrationConfig
	localID    string
	pending    map[string][]WakeEvent
	timers     map[string]*time.Timer
	listeners  []ArbitrationListener
	suppressor *WakeSuppressor
	history    []WakeArbitrationResult
	maxHistory int
}

type ArbitrationListener func(result WakeArbitrationResult)

func NewWakeArbitration(localID string, cfg WakeArbitrationConfig) *WakeArbitration {
	if cfg.ArbitrationWindow == 0 {
		cfg.ArbitrationWindow = 500 * time.Millisecond
	}
	if cfg.MinConfidence == 0 {
		cfg.MinConfidence = 0.3
	}
	if cfg.LocalPriority == 0 {
		cfg.LocalPriority = 50
	}

	return &WakeArbitration{
		cfg:        cfg,
		localID:    localID,
		pending:    make(map[string][]WakeEvent),
		timers:     make(map[string]*time.Timer),
		maxHistory: 100,
	}
}

func (a *WakeArbitration) SetSuppressor(s *WakeSuppressor) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.suppressor = s
}

func (a *WakeArbitration) RegisterListener(listener ArbitrationListener) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.listeners = append(a.listeners, listener)
}

func (a *WakeArbitration) SubmitLocalWake(event WakeEvent) {
	event.DeviceID = a.localID
	event.Timestamp = time.Now()

	a.mu.Lock()
	defer a.mu.Unlock()

	if event.Confidence < a.cfg.MinConfidence {
		return
	}

	if a.suppressor != nil && a.suppressor.IsSuppressed() {
		return
	}

	groupID := a.groupWakeEvents(event.Phrase, event.Timestamp)

	a.pending[groupID] = append(a.pending[groupID], event)

	if _, hasTimer := a.timers[groupID]; !hasTimer {
		timer := time.AfterFunc(a.cfg.ArbitrationWindow, func() {
			a.decide(groupID)
		})
		a.timers[groupID] = timer
	}
}

func (a *WakeArbitration) SubmitRemoteWake(event WakeEvent) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if event.Confidence < a.cfg.MinConfidence {
		return
	}

	groupID := a.groupWakeEvents(event.Phrase, event.Timestamp)

	a.pending[groupID] = append(a.pending[groupID], event)

	if _, hasTimer := a.timers[groupID]; !hasTimer {
		timer := time.AfterFunc(a.cfg.ArbitrationWindow, func() {
			a.decide(groupID)
		})
		a.timers[groupID] = timer
	}
}

func (a *WakeArbitration) decide(groupID string) {
	a.mu.Lock()
	events := a.pending[groupID]
	delete(a.pending, groupID)
	delete(a.timers, groupID)
	suppressor := a.suppressor
	cfg := a.cfg
	localID := a.localID
	a.mu.Unlock()

	if len(events) == 0 {
		return
	}

	winner := a.selectWinner(events, cfg, localID)

	result := WakeArbitrationResult{
		WinnerID:   winner.DeviceID,
		WinnerName: winner.DeviceName,
		IsLocal:    winner.DeviceID == localID,
		Confidence: winner.Confidence,
		AllEvents:  events,
		DecidedAt:  time.Now(),
	}

	a.mu.Lock()
	a.history = append(a.history, result)
	if len(a.history) > a.maxHistory {
		a.history = a.history[len(a.history)-a.maxHistory:]
	}
	listeners := make([]ArbitrationListener, len(a.listeners))
	copy(listeners, a.listeners)
	a.mu.Unlock()

	if suppressor != nil && !result.IsLocal {
		suppressor.Suppress(winner.DeviceID, winner.DeviceName, cfg.ArbitrationWindow*2)
	}

	for _, listener := range listeners {
		listener(result)
	}
}

func (a *WakeArbitration) selectWinner(events []WakeEvent, cfg WakeArbitrationConfig, localID string) WakeEvent {
	if len(events) == 1 {
		return events[0]
	}

	switch cfg.ElectionMode {
	case ElectionFirstResponse:
		return a.electionFirstResponse(events)

	case ElectionBestSignal:
		return a.electionBestSignal(events, cfg, localID)

	case ElectionHighestPriority:
		return a.electionHighestPriority(events, cfg, localID)

	default:
		return a.electionBestSignal(events, cfg, localID)
	}
}

func (a *WakeArbitration) electionFirstResponse(events []WakeEvent) WakeEvent {
	best := events[0]
	for _, e := range events[1:] {
		if e.Timestamp.Before(best.Timestamp) {
			best = e
		}
	}
	return best
}

func (a *WakeArbitration) electionBestSignal(events []WakeEvent, cfg WakeArbitrationConfig, localID string) WakeEvent {
	best := events[0]
	bestScore := a.signalScore(events[0], cfg, localID)

	for _, e := range events[1:] {
		score := a.signalScore(e, cfg, localID)
		if score > bestScore {
			best = e
			bestScore = score
		}
	}

	return best
}

func (a *WakeArbitration) signalScore(event WakeEvent, cfg WakeArbitrationConfig, localID string) float64 {
	score := event.Confidence * 0.6

	if event.Energy > 0 {
		energyNorm := event.Energy
		if energyNorm > 1.0 {
			energyNorm = 1.0
		}
		score += energyNorm * 0.3
	}

	if event.DeviceID == localID && cfg.PreferLocal {
		score += 0.1
	}

	priorityNorm := float64(event.Priority) / 100.0
	score += priorityNorm * 0.1

	return score
}

func (a *WakeArbitration) electionHighestPriority(events []WakeEvent, cfg WakeArbitrationConfig, localID string) WakeEvent {
	best := events[0]
	bestPriority := a.priorityScore(events[0], cfg, localID)

	for _, e := range events[1:] {
		priority := a.priorityScore(e, cfg, localID)
		if priority > bestPriority {
			best = e
			bestPriority = priority
		}
	}

	return best
}

func (a *WakeArbitration) priorityScore(event WakeEvent, cfg WakeArbitrationConfig, localID string) int {
	score := event.Priority

	if event.DeviceID == localID && cfg.PreferLocal {
		score += 10
	}

	score += int(event.Confidence * 50)

	return score
}

func (a *WakeArbitration) groupWakeEvents(phrase string, ts time.Time) string {
	return phrase
}

func (a *WakeArbitration) History() []WakeArbitrationResult {
	a.mu.Lock()
	defer a.mu.Unlock()

	result := make([]WakeArbitrationResult, len(a.history))
	copy(result, a.history)
	return result
}

func (a *WakeArbitration) PendingCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()

	count := 0
	for _, events := range a.pending {
		count += len(events)
	}
	return count
}

func (a *WakeArbitration) Clear() {
	a.mu.Lock()
	defer a.mu.Unlock()

	for _, timer := range a.timers {
		timer.Stop()
	}

	a.pending = make(map[string][]WakeEvent)
	a.timers = make(map[string]*time.Timer)
}

func (a *WakeArbitration) SetConfig(cfg WakeArbitrationConfig) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if cfg.ArbitrationWindow > 0 {
		a.cfg.ArbitrationWindow = cfg.ArbitrationWindow
	}
	if cfg.MinConfidence > 0 {
		a.cfg.MinConfidence = cfg.MinConfidence
	}
	a.cfg.PreferLocal = cfg.PreferLocal
	a.cfg.LocalPriority = cfg.LocalPriority
	a.cfg.ElectionMode = cfg.ElectionMode
}

func (a *WakeArbitration) Config() WakeArbitrationConfig {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg
}
