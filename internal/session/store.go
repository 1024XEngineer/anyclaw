package session

import (
	"context"
	"sync"
	"time"
)

// ChatMessage is the normalized message record stored in a session.
type ChatMessage struct {
	ID        string
	Role      string
	Text      string
	CreatedAt time.Time
	Metadata  map[string]any
}

// Session is the storage-facing contract for one conversation lane.
type Session interface {
	ID() string
	Key() string
	Append(ctx context.Context, msg ChatMessage) error
	History(ctx context.Context, limit int) ([]ChatMessage, error)
}

// SessionStore looks up or creates sessions by session key.
type SessionStore interface {
	GetOrCreate(ctx context.Context, sessionKey string) (Session, error)
}

// MemoryStore is a lightweight in-memory session store for early development.
type MemoryStore struct {
	mu       sync.Mutex
	sessions map[string]*MemorySession
}

// NewMemoryStore creates an empty session store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{sessions: make(map[string]*MemorySession)}
}

// GetOrCreate returns an existing session or creates a new one.
func (s *MemoryStore) GetOrCreate(_ context.Context, sessionKey string) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.sessions[sessionKey]; ok {
		return existing, nil
	}
	created := &MemorySession{id: sessionKey, key: sessionKey}
	s.sessions[sessionKey] = created
	return created, nil
}

// MemorySession is an in-memory session implementation.
type MemorySession struct {
	mu       sync.Mutex
	id       string
	key      string
	messages []ChatMessage
}

// ID returns the session identifier.
func (s *MemorySession) ID() string { return s.id }

// Key returns the canonical session key.
func (s *MemorySession) Key() string { return s.key }

// Append adds a message to the session transcript.
func (s *MemorySession) Append(_ context.Context, msg ChatMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, msg)
	return nil
}

// History returns up to limit most recent messages, or all when limit <= 0.
func (s *MemorySession) History(_ context.Context, limit int) ([]ChatMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 || limit >= len(s.messages) {
		out := make([]ChatMessage, len(s.messages))
		copy(out, s.messages)
		return out, nil
	}
	start := len(s.messages) - limit
	out := make([]ChatMessage, len(s.messages[start:]))
	copy(out, s.messages[start:])
	return out, nil
}
