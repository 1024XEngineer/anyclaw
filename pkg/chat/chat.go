package chat

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/anyclaw/anyclaw/pkg/orchestrator"
)

type Message struct {
	Role      string    `json:"role"` // "user" or "assistant"
	Content   string    `json:"content"`
	AgentName string    `json:"agent_name,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

type Session struct {
	ID        string    `json:"id"`
	AgentName string    `json:"agent_name"`
	Title     string    `json:"title"`
	Messages  []Message `json:"messages"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ChatRequest struct {
	AgentName string `json:"agent_name"`
	SessionID string `json:"session_id,omitempty"`
	Message   string `json:"message"`
}

type ChatResponse struct {
	SessionID string    `json:"session_id"`
	AgentName string    `json:"agent_name"`
	Message   Message   `json:"message"`
	History   []Message `json:"history,omitempty"`
}

type ChatManager interface {
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
	GetSession(sessionID string) (*Session, error)
	ListSessions() []Session
	GetSessionHistory(sessionID string) ([]Message, error)
	DeleteSession(sessionID string) error
	ListAgents() []orchestrator.AgentInfo
}

type chatManager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	agents   map[string]*orchestrator.SubAgent
	idCount  int
}

func NewChatManager(orch *orchestrator.Orchestrator) ChatManager {
	agents := make(map[string]*orchestrator.SubAgent)
	if orch != nil {
		for _, a := range orch.ListAgents() {
			if sa, ok := orch.GetAgent(a.Name); ok {
				agents[a.Name] = sa
			}
		}
	}
	return &chatManager{
		sessions: make(map[string]*Session),
		agents:   agents,
	}
}

func (m *chatManager) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	if strings.TrimSpace(req.Message) == "" {
		return nil, fmt.Errorf("message is required")
	}

	// Find the agent
	sa, ok := m.agents[req.AgentName]
	if !ok {
		return nil, fmt.Errorf("agent not found: %s", req.AgentName)
	}

	// All session operations under lock to prevent concurrent access
	m.mu.Lock()

	var session *Session
	if req.SessionID != "" {
		session, ok = m.sessions[req.SessionID]
		if !ok {
			m.mu.Unlock()
			return nil, fmt.Errorf("session not found: %s", req.SessionID)
		}
	} else {
		m.idCount++
		sessionID := fmt.Sprintf("chat_%d_%d", time.Now().UnixNano(), m.idCount)
		session = &Session{
			ID:        sessionID,
			AgentName: req.AgentName,
			Title:     shortenMessage(req.Message),
			Messages:  make([]Message, 0),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		m.sessions[sessionID] = session
	}

	// Add user message
	userMsg := Message{
		Role:      "user",
		Content:   req.Message,
		Timestamp: time.Now(),
	}
	session.Messages = append(session.Messages, userMsg)
	session.UpdatedAt = time.Now()

	// Build conversation input while holding lock
	chatInput := m.buildConversationInput(session, req.Message)
	m.mu.Unlock()

	// Call the agent (outside lock - this can take time)
	assistantContent, err := sa.Run(ctx, chatInput)

	m.mu.Lock()

	if err != nil {
		// Remove the user message since we couldn't respond
		if len(session.Messages) > 0 {
			session.Messages = session.Messages[:len(session.Messages)-1]
		}
		m.mu.Unlock()
		return nil, fmt.Errorf("agent error: %w", err)
	}

	// Add assistant message
	assistantMsg := Message{
		Role:      "assistant",
		Content:   assistantContent,
		AgentName: req.AgentName,
		Timestamp: time.Now(),
	}
	session.Messages = append(session.Messages, assistantMsg)
	session.UpdatedAt = time.Now()

	// Copy history for response
	history := make([]Message, len(session.Messages))
	copy(history, session.Messages)
	m.mu.Unlock()

	return &ChatResponse{
		SessionID: session.ID,
		AgentName: req.AgentName,
		Message:   assistantMsg,
		History:   history,
	}, nil
}

func (m *chatManager) buildConversationInput(session *Session, currentMessage string) string {
	if len(session.Messages) <= 1 {
		// First message, no history needed
		return currentMessage
	}

	var sb strings.Builder
	sb.WriteString("以下是你们的对话历史：\n\n")

	// Include last 10 messages for context (avoid token overflow)
	startIdx := 0
	if len(session.Messages) > 11 { // current message is already appended, so -1 for it
		startIdx = len(session.Messages) - 11
	}

	for i := startIdx; i < len(session.Messages)-1; i++ { // exclude the current message
		msg := session.Messages[i]
		if msg.Role == "user" {
			sb.WriteString(fmt.Sprintf("用户: %s\n", msg.Content))
		} else {
			sb.WriteString(fmt.Sprintf("%s: %s\n", msg.AgentName, msg.Content))
		}
	}

	sb.WriteString(fmt.Sprintf("\n用户: %s\n\n请回复：", currentMessage))
	return sb.String()
}

func (m *chatManager) GetSession(sessionID string) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	session, ok := m.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	s := *session
	return &s, nil
}

func (m *chatManager) ListSessions() []Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list := make([]Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		list = append(list, *s)
	}
	return list
}

func (m *chatManager) GetSessionHistory(sessionID string) ([]Message, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	session, ok := m.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	history := make([]Message, len(session.Messages))
	copy(history, session.Messages)
	return history, nil
}

func (m *chatManager) DeleteSession(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sessions[sessionID]; !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	delete(m.sessions, sessionID)
	return nil
}

func (m *chatManager) ListAgents() []orchestrator.AgentInfo {
	infos := make([]orchestrator.AgentInfo, 0, len(m.agents))
	for _, sa := range m.agents {
		infos = append(infos, orchestrator.AgentInfo{
			Name:            sa.Name(),
			Description:     sa.Description(),
			Persona:         sa.Persona(),
			Domain:          sa.Domain(),
			Expertise:       sa.Expertise(),
			Skills:          sa.Skills(),
			PermissionLevel: sa.PermissionLevel(),
		})
	}
	return infos
}

func shortenMessage(s string) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) <= 30 {
		return s
	}
	return string(runes[:30]) + "..."
}
