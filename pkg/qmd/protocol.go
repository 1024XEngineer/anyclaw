package qmd

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type MessageType string

const (
	MsgRequest   MessageType = "request"
	MsgResponse  MessageType = "response"
	MsgEvent     MessageType = "event"
	MsgHeartbeat MessageType = "heartbeat"
	MsgError     MessageType = "error"
)

type EventTopic string

const (
	EventRecordInserted EventTopic = "record.inserted"
	EventRecordUpdated  EventTopic = "record.updated"
	EventRecordDeleted  EventTopic = "record.deleted"
	EventTableCreated   EventTopic = "table.created"
	EventTableDropped   EventTopic = "table.dropped"
	EventWALFlushed     EventTopic = "wal.flushed"
	EventError          EventTopic = "error"
)

type Message struct {
	ID        string          `json:"id"`
	Type      MessageType     `json:"type"`
	Topic     EventTopic      `json:"topic,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Error     string          `json:"error,omitempty"`
	Timestamp time.Time       `json:"timestamp"`
}

func NewRequest(id string, payload any) (*Message, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("qmd: marshal payload: %w", err)
	}
	return &Message{
		ID:        id,
		Type:      MsgRequest,
		Payload:   data,
		Timestamp: time.Now(),
	}, nil
}

func NewResponse(id string, payload any) (*Message, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("qmd: marshal response: %w", err)
	}
	return &Message{
		ID:        id,
		Type:      MsgResponse,
		Payload:   data,
		Timestamp: time.Now(),
	}, nil
}

func NewEvent(topic EventTopic, payload any) (*Message, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("qmd: marshal event: %w", err)
	}
	return &Message{
		ID:        newWALID(),
		Type:      MsgEvent,
		Topic:     topic,
		Payload:   data,
		Timestamp: time.Now(),
	}, nil
}

func NewErrorMessage(err error) *Message {
	return &Message{
		ID:        newWALID(),
		Type:      MsgError,
		Error:     err.Error(),
		Timestamp: time.Now(),
	}
}

func NewHeartbeat() *Message {
	return &Message{
		ID:        newWALID(),
		Type:      MsgHeartbeat,
		Timestamp: time.Now(),
	}
}

func (m *Message) UnmarshalPayload(v any) error {
	if m.Payload == nil {
		return fmt.Errorf("qmd: empty payload")
	}
	return json.Unmarshal(m.Payload, v)
}

func (m *Message) Encode() ([]byte, error) {
	return json.Marshal(m)
}

func DecodeMessage(data []byte) (*Message, error) {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("qmd: decode message: %w", err)
	}
	return &msg, nil
}

type EventBus struct {
	mu       sync.RWMutex
	handlers map[EventTopic][]EventHandler
	dropSlow bool
}

type EventHandler func(event *Message)

func NewEventBus(dropSlow bool) *EventBus {
	return &EventBus{
		handlers: make(map[EventTopic][]EventHandler),
		dropSlow: dropSlow,
	}
}

func (eb *EventBus) Subscribe(topic EventTopic, handler EventHandler) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	eb.handlers[topic] = append(eb.handlers[topic], handler)
}

func (eb *EventBus) SubscribeAll(handler EventHandler) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	for topic := range eb.handlers {
		eb.handlers[topic] = append(eb.handlers[topic], handler)
	}
	eb.handlers["*"] = append(eb.handlers["*"], handler)
}

func (eb *EventBus) Publish(event *Message) {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	if handlers, ok := eb.handlers[event.Topic]; ok {
		for _, h := range handlers {
			if eb.dropSlow {
				go h(event)
			} else {
				h(event)
			}
		}
	}

	if handlers, ok := eb.handlers["*"]; ok {
		for _, h := range handlers {
			if eb.dropSlow {
				go h(event)
			} else {
				h(event)
			}
		}
	}
}

func (eb *EventBus) Unsubscribe(topic EventTopic) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	delete(eb.handlers, topic)
}

type RequestPayload struct {
	Action string         `json:"action"`
	Table  string         `json:"table,omitempty"`
	ID     string         `json:"id,omitempty"`
	Data   map[string]any `json:"data,omitempty"`
	Params map[string]any `json:"params,omitempty"`
}

type ResponsePayload struct {
	Success bool           `json:"success"`
	Data    map[string]any `json:"data,omitempty"`
	Records []*Record      `json:"records,omitempty"`
	Count   int            `json:"count,omitempty"`
	Stats   *Stats         `json:"stats,omitempty"`
}

type ProtocolConfig struct {
	HeartbeatInterval time.Duration
	HeartbeatTimeout  time.Duration
	MaxPending        int
	DropSlowEvents    bool
}

func DefaultProtocolConfig() ProtocolConfig {
	return ProtocolConfig{
		HeartbeatInterval: 10 * time.Second,
		HeartbeatTimeout:  30 * time.Second,
		MaxPending:        1000,
		DropSlowEvents:    true,
	}
}

type ProtocolHandler struct {
	config   ProtocolConfig
	store    *Store
	eventBus *EventBus
	pending  map[string]chan *Message
	mu       sync.RWMutex
}

func NewProtocolHandler(store *Store, cfg ProtocolConfig) *ProtocolHandler {
	if cfg.HeartbeatInterval <= 0 {
		cfg.HeartbeatInterval = 10 * time.Second
	}
	if cfg.HeartbeatTimeout <= 0 {
		cfg.HeartbeatTimeout = 30 * time.Second
	}
	if cfg.MaxPending <= 0 {
		cfg.MaxPending = 1000
	}

	return &ProtocolHandler{
		config:   cfg,
		store:    store,
		eventBus: NewEventBus(cfg.DropSlowEvents),
		pending:  make(map[string]chan *Message),
	}
}

func (ph *ProtocolHandler) HandleMessage(msg *Message) *Message {
	switch msg.Type {
	case MsgRequest:
		return ph.handleRequest(msg)
	case MsgHeartbeat:
		return NewHeartbeat()
	default:
		return NewErrorMessage(fmt.Errorf("unknown message type: %s", msg.Type))
	}
}

func (ph *ProtocolHandler) handleRequest(msg *Message) *Message {
	var req RequestPayload
	if err := msg.UnmarshalPayload(&req); err != nil {
		return NewErrorMessage(fmt.Errorf("invalid request: %w", err))
	}

	var resp *Message
	var err error

	switch req.Action {
	case "create_table":
		resp, err = ph.actionCreateTable(req)
	case "drop_table":
		resp, err = ph.actionDropTable(req)
	case "insert":
		resp, err = ph.actionInsert(req)
	case "get":
		resp, err = ph.actionGet(req)
	case "update":
		resp, err = ph.actionUpdate(req)
	case "delete":
		resp, err = ph.actionDelete(req)
	case "list":
		resp, err = ph.actionList(req)
	case "query":
		resp, err = ph.actionQuery(req)
	case "count":
		resp, err = ph.actionCount(req)
	case "stats":
		resp, err = ph.actionStats(req)
	default:
		return NewErrorMessage(fmt.Errorf("unknown action: %s", req.Action))
	}

	if err != nil {
		return NewErrorMessage(err)
	}

	return resp
}

func (ph *ProtocolHandler) actionCreateTable(req RequestPayload) (*Message, error) {
	var columns []string
	if cols, ok := req.Params["columns"]; ok {
		if arr, ok := cols.([]any); ok {
			for _, c := range arr {
				if s, ok := c.(string); ok {
					columns = append(columns, s)
				}
			}
		}
	}

	if err := ph.store.CreateTable(req.Table, columns); err != nil {
		return nil, err
	}

	event, _ := NewEvent(EventTableCreated, map[string]any{"table": req.Table})
	ph.eventBus.Publish(event)

	return NewResponse("", ResponsePayload{Success: true})
}

func (ph *ProtocolHandler) actionDropTable(req RequestPayload) (*Message, error) {
	if err := ph.store.DropTable(req.Table); err != nil {
		return nil, err
	}

	event, _ := NewEvent(EventTableDropped, map[string]any{"table": req.Table})
	ph.eventBus.Publish(event)

	return NewResponse("", ResponsePayload{Success: true})
}

func (ph *ProtocolHandler) actionInsert(req RequestPayload) (*Message, error) {
	record := &Record{
		ID:   req.ID,
		Data: req.Data,
	}

	if err := ph.store.Insert(req.Table, record); err != nil {
		return nil, err
	}

	event, _ := NewEvent(EventRecordInserted, map[string]any{
		"table": req.Table,
		"id":    req.ID,
	})
	ph.eventBus.Publish(event)

	return NewResponse("", ResponsePayload{Success: true})
}

func (ph *ProtocolHandler) actionGet(req RequestPayload) (*Message, error) {
	record, err := ph.store.Get(req.Table, req.ID)
	if err != nil {
		return nil, err
	}

	return NewResponse("", ResponsePayload{
		Success: true,
		Records: []*Record{record},
	})
}

func (ph *ProtocolHandler) actionUpdate(req RequestPayload) (*Message, error) {
	record := &Record{
		ID:   req.ID,
		Data: req.Data,
	}

	if err := ph.store.Update(req.Table, record); err != nil {
		return nil, err
	}

	event, _ := NewEvent(EventRecordUpdated, map[string]any{
		"table": req.Table,
		"id":    req.ID,
	})
	ph.eventBus.Publish(event)

	return NewResponse("", ResponsePayload{Success: true})
}

func (ph *ProtocolHandler) actionDelete(req RequestPayload) (*Message, error) {
	if err := ph.store.Delete(req.Table, req.ID); err != nil {
		return nil, err
	}

	event, _ := NewEvent(EventRecordDeleted, map[string]any{
		"table": req.Table,
		"id":    req.ID,
	})
	ph.eventBus.Publish(event)

	return NewResponse("", ResponsePayload{Success: true})
}

func (ph *ProtocolHandler) actionList(req RequestPayload) (*Message, error) {
	limit := 100
	if l, ok := req.Params["limit"]; ok {
		switch v := l.(type) {
		case float64:
			limit = int(v)
		case int:
			limit = v
		}
	}

	records, err := ph.store.List(req.Table, limit)
	if err != nil {
		return nil, err
	}

	return NewResponse("", ResponsePayload{
		Success: true,
		Records: records,
		Count:   len(records),
	})
}

func (ph *ProtocolHandler) actionQuery(req RequestPayload) (*Message, error) {
	field, _ := req.Params["field"].(string)
	value := req.Params["value"]
	limit := 100
	if l, ok := req.Params["limit"]; ok {
		switch v := l.(type) {
		case float64:
			limit = int(v)
		case int:
			limit = v
		}
	}

	records, err := ph.store.Query(req.Table, field, value, limit)
	if err != nil {
		return nil, err
	}

	return NewResponse("", ResponsePayload{
		Success: true,
		Records: records,
		Count:   len(records),
	})
}

func (ph *ProtocolHandler) actionCount(req RequestPayload) (*Message, error) {
	count, err := ph.store.Count(req.Table)
	if err != nil {
		return nil, err
	}

	return NewResponse("", ResponsePayload{
		Success: true,
		Count:   count,
	})
}

func (ph *ProtocolHandler) actionStats(req RequestPayload) (*Message, error) {
	stats := ph.store.Stats()
	return NewResponse("", ResponsePayload{
		Success: true,
		Stats:   &stats,
	})
}

func (ph *ProtocolHandler) EventBus() *EventBus {
	return ph.eventBus
}
