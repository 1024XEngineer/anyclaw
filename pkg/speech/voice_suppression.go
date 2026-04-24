package speech

import (
	"sync"
	"time"
)

type WakeSuppressor struct {
	mu             sync.Mutex
	isSuppressed   bool
	suppressedBy   string
	suppressorName string
	suppressUntil  time.Time
	duration       time.Duration
	listeners      []SuppressionListener
	history        []SuppressionEvent
	maxHistory     int
}

type SuppressionEvent struct {
	Type       string
	DeviceID   string
	DeviceName string
	Duration   time.Duration
	Timestamp  time.Time
	Remaining  time.Duration
}

type SuppressionListener func(event SuppressionEvent)

func NewWakeSuppressor() *WakeSuppressor {
	return &WakeSuppressor{
		maxHistory: 50,
	}
}

func (s *WakeSuppressor) Suppress(deviceID, deviceName string, duration time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.isSuppressed = true
	s.suppressedBy = deviceID
	s.suppressorName = deviceName
	s.suppressUntil = time.Now().Add(duration)
	s.duration = duration

	event := SuppressionEvent{
		Type:       "suppressed",
		DeviceID:   deviceID,
		DeviceName: deviceName,
		Duration:   duration,
		Timestamp:  time.Now(),
		Remaining:  duration,
	}

	s.history = append(s.history, event)
	if len(s.history) > s.maxHistory {
		s.history = s.history[len(s.history)-s.maxHistory:]
	}

	listeners := make([]SuppressionListener, len(s.listeners))
	copy(listeners, s.listeners)

	go func() {
		for _, listener := range listeners {
			listener(event)
		}
	}()
}

func (s *WakeSuppressor) IsSuppressed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isSuppressed {
		return false
	}

	if time.Now().After(s.suppressUntil) {
		s.isSuppressed = false
		s.suppressedBy = ""
		s.suppressorName = ""
		return false
	}

	return true
}

func (s *WakeSuppressor) SuppressUntil() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.suppressUntil
}

func (s *WakeSuppressor) RemainingTime() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isSuppressed {
		return 0
	}

	remaining := time.Until(s.suppressUntil)
	if remaining < 0 {
		s.isSuppressed = false
		return 0
	}

	return remaining
}

func (s *WakeSuppressor) SuppressedBy() (string, string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isSuppressed {
		return "", ""
	}

	if time.Now().After(s.suppressUntil) {
		s.isSuppressed = false
		return "", ""
	}

	return s.suppressedBy, s.suppressorName
}

func (s *WakeSuppressor) Release() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isSuppressed {
		s.isSuppressed = false

		event := SuppressionEvent{
			Type:       "released",
			DeviceID:   s.suppressedBy,
			DeviceName: s.suppressorName,
			Timestamp:  time.Now(),
			Remaining:  0,
		}

		s.history = append(s.history, event)

		listeners := make([]SuppressionListener, len(s.listeners))
		copy(listeners, s.listeners)

		go func() {
			for _, listener := range listeners {
				listener(event)
			}
		}()

		s.suppressedBy = ""
		s.suppressorName = ""
	}
}

func (s *WakeSuppressor) RegisterListener(listener SuppressionListener) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listeners = append(s.listeners, listener)
}

func (s *WakeSuppressor) History() []SuppressionEvent {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]SuppressionEvent, len(s.history))
	copy(result, s.history)
	return result
}

func (s *WakeSuppressor) IsSuppressedBy(deviceID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isSuppressed {
		return false
	}

	return s.suppressedBy == deviceID && time.Now().Before(s.suppressUntil)
}

func (s *WakeSuppressor) Status() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()

	status := map[string]any{
		"is_suppressed": s.isSuppressed,
	}

	if s.isSuppressed {
		remaining := time.Until(s.suppressUntil)
		if remaining < 0 {
			remaining = 0
			s.isSuppressed = false
		}

		status["suppressed_by"] = s.suppressedBy
		status["suppressor_name"] = s.suppressorName
		status["remaining"] = remaining
		status["duration"] = s.duration
	}

	return status
}

func (s *WakeSuppressor) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.isSuppressed = false
	s.suppressedBy = ""
	s.suppressorName = ""
	s.history = nil
}
