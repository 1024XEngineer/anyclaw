package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type SessionStore struct {
	BaseDir string
}

type Session struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Messages  []Message `json:"messages"`
	Provider  string    `json:"provider,omitempty"`
	Model     string    `json:"model,omitempty"`
}

func NewSessionStore(workDir string) *SessionStore {
	sessionDir := filepath.Join(workDir, ".anyclaw", "sessions")
	return &SessionStore{BaseDir: sessionDir}
}

func (s *SessionStore) Init() error {
	if err := os.MkdirAll(s.BaseDir, 0o755); err != nil {
		return fmt.Errorf("failed to create sessions directory: %w", err)
	}
	return nil
}

func (s *SessionStore) SaveSession(session Session) error {
	session.UpdatedAt = time.Now()

	sessionFile := filepath.Join(s.BaseDir, session.ID+".json")
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	if err := os.WriteFile(sessionFile, data, 0o644); err != nil {
		return fmt.Errorf("failed to write session file: %w", err)
	}

	return nil
}

func (s *SessionStore) LoadSession(sessionID string) (*Session, error) {
	sessionFile := filepath.Join(s.BaseDir, sessionID+".json")

	data, err := os.ReadFile(sessionFile)
	if err != nil {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("failed to parse session: %w", err)
	}

	return &session, nil
}

func (s *SessionStore) ListSessions() ([]Session, error) {
	entries, err := os.ReadDir(s.BaseDir)
	if err != nil {
		return nil, err
	}

	var sessions []Session
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		sessionID := entry.Name()[:len(entry.Name())-5]
		session, err := s.LoadSession(sessionID)
		if err != nil {
			continue
		}
		sessions = append(sessions, *session)
	}

	return sessions, nil
}

func (s *SessionStore) DeleteSession(sessionID string) error {
	sessionFile := filepath.Join(s.BaseDir, sessionID+".json")
	return os.Remove(sessionFile)
}

func (s *SessionStore) CreateSession() Session {
	return Session{
		ID:        fmt.Sprintf("session-%d", time.Now().UnixMilli()),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Messages:  []Message{},
	}
}
