package gateway

import (
	"sync"
	"time"
)

type Bus struct {
	mu          sync.RWMutex
	subscribers map[chan *Event]struct{}
}

func NewBus() *Bus {
	return &Bus{subscribers: make(map[chan *Event]struct{})}
}

func (b *Bus) Subscribe(buffer int) chan *Event {
	if buffer <= 0 {
		buffer = 16
	}
	ch := make(chan *Event, buffer)
	b.mu.Lock()
	b.subscribers[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

func (b *Bus) Unsubscribe(ch chan *Event) {
	b.mu.Lock()
	if _, ok := b.subscribers[ch]; ok {
		delete(b.subscribers, ch)
		close(ch)
	}
	b.mu.Unlock()
}

func (b *Bus) Publish(event *Event) {
	if b == nil || event == nil {
		return
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subscribers {
		select {
		case ch <- cloneEvent(event):
		default:
		}
	}
}

func NewEvent(eventType string, sessionID string, payload map[string]any) *Event {
	return &Event{
		ID:        uniqueID("evt"),
		Type:      eventType,
		SessionID: sessionID,
		Timestamp: time.Now().UTC(),
		Payload:   payload,
	}
}
