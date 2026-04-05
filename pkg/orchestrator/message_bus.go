package orchestrator

import (
	"sync"
	"time"
)

type AgentMessage struct {
	ID        string         `json:"id"`
	From      string         `json:"from"`
	To        string         `json:"to,omitempty"`
	Broadcast bool           `json:"broadcast,omitempty"`
	Type      string         `json:"type"`
	Payload   map[string]any `json:"payload,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
	TTL       time.Duration  `json:"ttl,omitempty"`
}

type MessageBus struct {
	mu       sync.RWMutex
	channels map[string][]chan *AgentMessage
	subs     map[string][]chan *AgentMessage
	history  []*AgentMessage
	maxHist  int
}

func NewMessageBus(maxHistory int) *MessageBus {
	if maxHistory <= 0 {
		maxHistory = 100
	}
	return &MessageBus{
		channels: make(map[string][]chan *AgentMessage),
		subs:     make(map[string][]chan *AgentMessage),
		history:  make([]*AgentMessage, 0, maxHistory),
		maxHist:  maxHistory,
	}
}

func (mb *MessageBus) Subscribe(agentName string) <-chan *AgentMessage {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	ch := make(chan *AgentMessage, 32)
	mb.subs[agentName] = append(mb.subs[agentName], ch)
	return ch
}

func (mb *MessageBus) SubscribeChannel(channel string) <-chan *AgentMessage {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	ch := make(chan *AgentMessage, 32)
	mb.channels[channel] = append(mb.channels[channel], ch)
	return ch
}

func (mb *MessageBus) Unsubscribe(agentName string, ch <-chan *AgentMessage) {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	subs := mb.subs[agentName]
	for i, sub := range subs {
		if sub == ch {
			close(sub)
			mb.subs[agentName] = append(subs[:i], subs[i+1:]...)
			return
		}
	}
}

func (mb *MessageBus) Send(msg *AgentMessage) {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	if msg.ID == "" {
		msg.ID = uniqueMsgID()
	}
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now().UTC()
	}

	mb.history = append(mb.history, msg)
	if len(mb.history) > mb.maxHist {
		mb.history = mb.history[len(mb.history)-mb.maxHist:]
	}

	if msg.Broadcast {
		for agentName, subs := range mb.subs {
			if agentName == msg.From {
				continue
			}
			for _, ch := range subs {
				select {
				case ch <- msg:
				default:
				}
			}
		}
	} else if msg.To != "" {
		for _, ch := range mb.subs[msg.To] {
			select {
			case ch <- msg:
			default:
			}
		}
	}
}

func (mb *MessageBus) PublishToChannel(channel string, msg *AgentMessage) {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	if msg.ID == "" {
		msg.ID = uniqueMsgID()
	}
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now().UTC()
	}

	mb.history = append(mb.history, msg)
	if len(mb.history) > mb.maxHist {
		mb.history = mb.history[len(mb.history)-mb.maxHist:]
	}

	for _, ch := range mb.channels[channel] {
		select {
		case ch <- msg:
		default:
		}
	}
}

func (mb *MessageBus) History(limit int) []*AgentMessage {
	mb.mu.RLock()
	defer mb.mu.RUnlock()

	if limit <= 0 || limit >= len(mb.history) {
		result := make([]*AgentMessage, len(mb.history))
		copy(result, mb.history)
		return result
	}
	result := mb.history[len(mb.history)-limit:]
	out := make([]*AgentMessage, len(result))
	copy(out, result)
	return out
}

func (mb *MessageBus) HistoryForAgent(agentName string, limit int) []*AgentMessage {
	mb.mu.RLock()
	defer mb.mu.RUnlock()

	var result []*AgentMessage
	for _, msg := range mb.history {
		if msg.From == agentName || msg.To == agentName || msg.Broadcast {
			result = append(result, msg)
		}
	}
	if limit > 0 && len(result) > limit {
		result = result[len(result)-limit:]
	}
	return result
}

var msgCounter uint64

func uniqueMsgID() string {
	msgCounter++
	return "msg-" + time.Now().Format("20060102150405") + "-" + string(rune('a'+(msgCounter%26)))
}
