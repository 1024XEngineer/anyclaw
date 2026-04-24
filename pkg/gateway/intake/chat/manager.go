package chat

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// AgentInfo is the gateway-visible snapshot of an executable chat agent.
type AgentInfo struct {
	Name            string
	Description     string
	Persona         string
	Domain          string
	Expertise       []string
	Skills          []string
	PermissionLevel string
}

// AgentRunner is the minimal execution surface the chat intake needs.
type AgentRunner interface {
	Run(ctx context.Context, input string) (string, error)
	Name() string
	Description() string
	Persona() string
	Domain() string
	Expertise() []string
	Skills() []string
	PermissionLevel() string
}

// AgentCatalog keeps the gateway chat runtime decoupled from orchestrator internals.
type AgentCatalog interface {
	ListAgents() []AgentInfo
	GetAgent(name string) (AgentRunner, bool)
}

// ChatManager owns the lightweight /v2/chat session runtime.
type ChatManager interface {
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
	GetSession(sessionID string) (*Session, error)
	ListSessions() []Session
	GetSessionHistory(sessionID string) ([]Message, error)
	DeleteSession(sessionID string) error
	ListAgents() []AgentInfo
}

type chatManager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	agents   map[string]AgentRunner
	idCount  int
}

// NewChatManager creates a chat manager from the minimal runtime agent catalog.
func NewChatManager(catalog AgentCatalog) ChatManager {
	agents := make(map[string]AgentRunner)
	if catalog != nil {
		for _, a := range catalog.ListAgents() {
			if runner, ok := catalog.GetAgent(a.Name); ok {
				agents[a.Name] = runner
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

	agentName := strings.TrimSpace(req.AgentName)
	runner, ok := m.agents[agentName]
	if !ok {
		return nil, fmt.Errorf("agent not found: %s", agentName)
	}

	m.mu.Lock()

	var session *Session
	if strings.TrimSpace(req.SessionID) != "" {
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
			AgentName: agentName,
			Title:     shortenMessage(req.Message),
			Messages:  make([]Message, 0),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		m.sessions[sessionID] = session
	}

	userMsg := Message{
		Role:      "user",
		Content:   req.Message,
		Timestamp: time.Now(),
	}
	session.Messages = append(session.Messages, userMsg)
	session.UpdatedAt = time.Now()

	chatInput := m.buildConversationInput(session, req.Message)
	m.mu.Unlock()

	assistantContent, err := runner.Run(ctx, chatInput)

	m.mu.Lock()
	defer m.mu.Unlock()

	if err != nil {
		if len(session.Messages) > 0 {
			session.Messages = session.Messages[:len(session.Messages)-1]
		}
		return nil, fmt.Errorf("agent error: %w", err)
	}

	assistantMsg := Message{
		Role:      "assistant",
		Content:   assistantContent,
		AgentName: agentName,
		Timestamp: time.Now(),
	}
	session.Messages = append(session.Messages, assistantMsg)
	session.UpdatedAt = time.Now()

	history := make([]Message, len(session.Messages))
	copy(history, session.Messages)

	return &ChatResponse{
		SessionID: session.ID,
		AgentName: agentName,
		Message:   assistantMsg,
		History:   history,
	}, nil
}

func (m *chatManager) buildConversationInput(session *Session, currentMessage string) string {
	if len(session.Messages) <= 1 {
		return currentMessage
	}

	var sb strings.Builder
	sb.WriteString("Conversation history:\n\n")

	startIdx := 0
	if len(session.Messages) > 11 {
		startIdx = len(session.Messages) - 11
	}

	for i := startIdx; i < len(session.Messages)-1; i++ {
		msg := session.Messages[i]
		if msg.Role == "user" {
			sb.WriteString(fmt.Sprintf("User: %s\n", msg.Content))
		} else {
			sb.WriteString(fmt.Sprintf("%s: %s\n", msg.AgentName, msg.Content))
		}
	}

	sb.WriteString(fmt.Sprintf("\nUser: %s\n\nReply:", currentMessage))
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

func (m *chatManager) ListAgents() []AgentInfo {
	infos := make([]AgentInfo, 0, len(m.agents))
	for _, runner := range m.agents {
		infos = append(infos, AgentInfo{
			Name:            runner.Name(),
			Description:     runner.Description(),
			Persona:         runner.Persona(),
			Domain:          runner.Domain(),
			Expertise:       runner.Expertise(),
			Skills:          runner.Skills(),
			PermissionLevel: runner.PermissionLevel(),
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
