package session

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// SessionStatus 会话状态
type SessionStatus string

const (
	SessionStatusActive SessionStatus = "active"
	SessionStatusIdle   SessionStatus = "idle"
	SessionStatusClosed SessionStatus = "closed"
	SessionStatusError  SessionStatus = "error"
)

// Session 会话
type Session struct {
	ID         string                 `json:"id"`
	AgentID    string                 `json:"agent_id"`
	ChannelID  string                 `json:"channel_id,omitempty"`
	UserID     string                 `json:"user_id,omitempty"`
	Status     SessionStatus          `json:"status"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	History    []Message              `json:"history,omitempty"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
	LastActive time.Time              `json:"last_active"`
}

// Message 会话消息
type Message struct {
	Role      string                 `json:"role"`
	Content   string                 `json:"content"`
	Type      string                 `json:"type,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

// SessionManager 会话管理器
type SessionManager struct {
	mu          sync.RWMutex
	sessions    map[string]*Session
	dataDir     string
	maxHistory  int
	maxSessions int
	handlers    map[string]SessionHandler
}

// SessionHandler 会话处理器
type SessionHandler func(ctx context.Context, session *Session, event SessionEvent) error

// SessionEvent 会话事件
type SessionEvent struct {
	Type      string                 `json:"type"`
	SessionID string                 `json:"session_id"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

// NewSessionManager 创建会话管理器
func NewSessionManager(dataDir string, maxHistory int) *SessionManager {
	return &SessionManager{
		sessions:    make(map[string]*Session),
		dataDir:     dataDir,
		maxHistory:  maxHistory,
		maxSessions: 1000,
		handlers:    make(map[string]SessionHandler),
	}
}

// CreateSession 创建会话
func (m *SessionManager) CreateSession(agentID string, channelID string, userID string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 检查会话数限制
	if m.maxSessions > 0 && len(m.sessions) >= m.maxSessions {
		// 清理最旧的会话
		m.cleanupOldestSessions(1)
	}

	session := &Session{
		ID:         generateSessionID(),
		AgentID:    agentID,
		ChannelID:  channelID,
		UserID:     userID,
		Status:     SessionStatusActive,
		Metadata:   make(map[string]interface{}),
		History:    make([]Message, 0),
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		LastActive: time.Now(),
	}

	m.sessions[session.ID] = session

	// 保存到磁盘
	if err := m.saveSession(session); err != nil {
		return nil, fmt.Errorf("failed to save session: %w", err)
	}

	return session, nil
}

// GetSession 获取会话
func (m *SessionManager) GetSession(sessionID string) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, exists := m.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	return session, nil
}

// GetSessionByAgent 获取代理的会话
func (m *SessionManager) GetSessionByAgent(agentID string) ([]*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var sessions []*Session
	for _, session := range m.sessions {
		if session.AgentID == agentID {
			sessions = append(sessions, session)
		}
	}

	return sessions, nil
}

// GetSessionByChannel 获取渠道的会话
func (m *SessionManager) GetSessionByChannel(channelID string) ([]*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var sessions []*Session
	for _, session := range m.sessions {
		if session.ChannelID == channelID {
			sessions = append(sessions, session)
		}
	}

	return sessions, nil
}

// GetSessionByUser 获取用户的会话
func (m *SessionManager) GetSessionByUser(userID string) ([]*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var sessions []*Session
	for _, session := range m.sessions {
		if session.UserID == userID {
			sessions = append(sessions, session)
		}
	}

	return sessions, nil
}

// UpdateSession 更新会话
func (m *SessionManager) UpdateSession(sessionID string, updates map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, exists := m.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	// 应用更新
	if status, ok := updates["status"].(SessionStatus); ok {
		session.Status = status
	}
	if metadata, ok := updates["metadata"].(map[string]interface{}); ok {
		for k, v := range metadata {
			session.Metadata[k] = v
		}
	}

	session.UpdatedAt = time.Now()
	session.LastActive = time.Now()

	// 保存到磁盘
	if err := m.saveSession(session); err != nil {
		return fmt.Errorf("failed to save session: %w", err)
	}

	return nil
}

// AddMessage 添加消息到会话
func (m *SessionManager) AddMessage(sessionID string, message Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, exists := m.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	message.Timestamp = time.Now()
	session.History = append(session.History, message)
	session.UpdatedAt = time.Now()
	session.LastActive = time.Now()

	// 限制历史长度
	if m.maxHistory > 0 && len(session.History) > m.maxHistory {
		session.History = session.History[len(session.History)-m.maxHistory:]
	}

	// 保存到磁盘
	if err := m.saveSession(session); err != nil {
		return fmt.Errorf("failed to save session: %w", err)
	}

	return nil
}

// GetHistory 获取会话历史
func (m *SessionManager) GetHistory(sessionID string, limit int) ([]Message, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, exists := m.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	if limit <= 0 || limit > len(session.History) {
		limit = len(session.History)
	}

	start := len(session.History) - limit
	return session.History[start:], nil
}

// ClearHistory 清除会话历史
func (m *SessionManager) ClearHistory(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, exists := m.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	session.History = make([]Message, 0)
	session.UpdatedAt = time.Now()

	// 保存到磁盘
	if err := m.saveSession(session); err != nil {
		return fmt.Errorf("failed to save session: %w", err)
	}

	return nil
}

// CloseSession 关闭会话
func (m *SessionManager) CloseSession(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, exists := m.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	session.Status = SessionStatusClosed
	session.UpdatedAt = time.Now()

	// 保存到磁盘
	if err := m.saveSession(session); err != nil {
		return fmt.Errorf("failed to save session: %w", err)
	}

	return nil
}

// DeleteSession 删除会话
func (m *SessionManager) DeleteSession(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, exists := m.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	// 从磁盘删除
	if err := m.deleteSessionFile(session); err != nil {
		return fmt.Errorf("failed to delete session file: %w", err)
	}

	delete(m.sessions, sessionID)
	return nil
}

// GetAllSessions 获取所有会话
func (m *SessionManager) GetAllSessions() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sessions := make([]*Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}

	return sessions
}

// GetActiveSessions 获取活跃会话
func (m *SessionManager) GetActiveSessions() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var sessions []*Session
	for _, session := range m.sessions {
		if session.Status == SessionStatusActive {
			sessions = append(sessions, session)
		}
	}

	return sessions
}

// RegisterHandler 注册会话处理器
func (m *SessionManager) RegisterHandler(eventType string, handler SessionHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.handlers[eventType] = handler
}

// LoadSessions 从磁盘加载会话
func (m *SessionManager) LoadSessions() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 确保目录存在
	if err := os.MkdirAll(m.dataDir, 0o755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// 读取会话文件
	files, err := filepath.Glob(filepath.Join(m.dataDir, "*.json"))
	if err != nil {
		return fmt.Errorf("failed to list session files: %w", err)
	}

	for _, file := range files {
		session, err := m.loadSessionFile(file)
		if err != nil {
			fmt.Printf("failed to load session file %s: %v\n", file, err)
			continue
		}

		m.sessions[session.ID] = session
	}

	return nil
}

// saveSession 保存会话到磁盘
func (m *SessionManager) saveSession(session *Session) error {
	filePath := filepath.Join(m.dataDir, fmt.Sprintf("%s.json", session.ID))

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write session file: %w", err)
	}

	return nil
}

// loadSessionFile 从文件加载会话
func (m *SessionManager) loadSessionFile(filePath string) (*Session, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var session Session
	if err := json.NewDecoder(bufio.NewReader(file)).Decode(&session); err != nil {
		return nil, err
	}

	return &session, nil
}

// deleteSessionFile 删除会话文件
func (m *SessionManager) deleteSessionFile(session *Session) error {
	filePath := filepath.Join(m.dataDir, fmt.Sprintf("%s.json", session.ID))
	return os.Remove(filePath)
}

// cleanupOldestSessions 清理最旧的会话
func (m *SessionManager) cleanupOldestSessions(count int) {
	// 找到最旧的会话
	type sessionWithTime struct {
		session *Session
		time    time.Time
	}

	var oldest []sessionWithTime
	for _, session := range m.sessions {
		oldest = append(oldest, sessionWithTime{session, session.LastActive})
	}

	// 按时间排序
	for i := 0; i < len(oldest)-1; i++ {
		for j := i + 1; j < len(oldest); j++ {
			if oldest[j].time.Before(oldest[i].time) {
				oldest[i], oldest[j] = oldest[j], oldest[i]
			}
		}
	}

	// 删除最旧的会话
	for i := 0; i < count && i < len(oldest); i++ {
		session := oldest[i].session
		m.deleteSessionFile(session)
		delete(m.sessions, session.ID)
	}
}

// generateSessionID 生成会话 ID
func generateSessionID() string {
	return fmt.Sprintf("session_%d", time.Now().UnixNano())
}
