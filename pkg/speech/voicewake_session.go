package speech

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/capability/agents/prompt"
)

type VoiceWakeSession struct {
	ID             string
	CreatedAt      time.Time
	LastActive     time.Time
	History        []prompt.Message
	Transcripts    []TranscriptEntry
	Responses      []ResponseEntry
	WakeEvents     []WakeEventEntry
	IsConversation bool
	ConversationID string
}

type TranscriptEntry struct {
	Text       string
	Confidence float64
	Language   string
	Duration   time.Duration
	Timestamp  time.Time
}

type ResponseEntry struct {
	Text      string
	Duration  time.Duration
	Timestamp time.Time
	IsSpoken  bool
}

type WakeEventEntry struct {
	Phrase     string
	Confidence float64
	Engine     string
	Timestamp  time.Time
}

type VoiceWakeSessionManager struct {
	mu          sync.RWMutex
	sessions    map[string]*VoiceWakeSession
	active      string
	maxSessions int
	maxHistory  int
}

type VoiceWakeSessionManagerConfig struct {
	MaxSessions int
	MaxHistory  int
}

func DefaultVoiceWakeSessionManagerConfig() VoiceWakeSessionManagerConfig {
	return VoiceWakeSessionManagerConfig{
		MaxSessions: 10,
		MaxHistory:  50,
	}
}

func NewVoiceWakeSessionManager(cfg VoiceWakeSessionManagerConfig) *VoiceWakeSessionManager {
	if cfg.MaxSessions <= 0 {
		cfg.MaxSessions = 10
	}
	if cfg.MaxHistory <= 0 {
		cfg.MaxHistory = 50
	}

	return &VoiceWakeSessionManager{
		sessions:    make(map[string]*VoiceWakeSession),
		maxSessions: cfg.MaxSessions,
		maxHistory:  cfg.MaxHistory,
	}
}

func (m *VoiceWakeSessionManager) CreateSession(id string) *VoiceWakeSession {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.sessions) >= m.maxSessions {
		m.evictOldest()
	}

	session := &VoiceWakeSession{
		ID:         id,
		CreatedAt:  time.Now(),
		LastActive: time.Now(),
		History:    make([]prompt.Message, 0),
	}

	m.sessions[id] = session
	m.active = id

	return session
}

func (m *VoiceWakeSessionManager) GetSession(id string) (*VoiceWakeSession, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, ok := m.sessions[id]
	return session, ok
}

func (m *VoiceWakeSessionManager) GetOrCreateSession(id string) *VoiceWakeSession {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[id]
	if ok {
		session.LastActive = time.Now()
		return session
	}

	if len(m.sessions) >= m.maxSessions {
		m.evictOldestLocked()
	}

	session = &VoiceWakeSession{
		ID:         id,
		CreatedAt:  time.Now(),
		LastActive: time.Now(),
		History:    make([]prompt.Message, 0),
	}

	m.sessions[id] = session
	m.active = id

	return session
}

func (m *VoiceWakeSessionManager) AddTranscript(sessionID string, entry TranscriptEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		return fmt.Errorf("voicewake-session: session %s not found", sessionID)
	}

	session.Transcripts = append(session.Transcripts, entry)
	session.LastActive = time.Now()

	return nil
}

func (m *VoiceWakeSessionManager) AddResponse(sessionID string, entry ResponseEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		return fmt.Errorf("voicewake-session: session %s not found", sessionID)
	}

	session.Responses = append(session.Responses, entry)
	session.LastActive = time.Now()

	return nil
}

func (m *VoiceWakeSessionManager) AddWakeEvent(sessionID string, entry WakeEventEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		return fmt.Errorf("voicewake-session: session %s not found", sessionID)
	}

	session.WakeEvents = append(session.WakeEvents, entry)
	session.LastActive = time.Now()

	return nil
}

func (m *VoiceWakeSessionManager) AddToHistory(sessionID string, msg prompt.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		return fmt.Errorf("voicewake-session: session %s not found", sessionID)
	}

	session.History = append(session.History, msg)

	if len(session.History) > m.maxHistory {
		session.History = session.History[len(session.History)-m.maxHistory:]
	}

	session.LastActive = time.Now()

	return nil
}

func (m *VoiceWakeSessionManager) GetHistory(sessionID string) ([]prompt.Message, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("voicewake-session: session %s not found", sessionID)
	}

	history := make([]prompt.Message, len(session.History))
	copy(history, session.History)

	return history, nil
}

func (m *VoiceWakeSessionManager) SetHistory(sessionID string, history []prompt.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		return fmt.Errorf("voicewake-session: session %s not found", sessionID)
	}

	session.History = history
	session.LastActive = time.Now()

	return nil
}

func (m *VoiceWakeSessionManager) SetActive(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.sessions[sessionID]; !ok {
		return fmt.Errorf("voicewake-session: session %s not found", sessionID)
	}

	m.active = sessionID
	return nil
}

func (m *VoiceWakeSessionManager) ActiveSession() *VoiceWakeSession {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, ok := m.sessions[m.active]
	if !ok {
		return nil
	}
	return session
}

func (m *VoiceWakeSessionManager) ActiveSessionID() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.active
}

func (m *VoiceWakeSessionManager) DeleteSession(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.sessions[sessionID]; !ok {
		return fmt.Errorf("voicewake-session: session %s not found", sessionID)
	}

	delete(m.sessions, sessionID)

	if m.active == sessionID {
		m.active = ""
		for id := range m.sessions {
			m.active = id
			break
		}
	}

	return nil
}

func (m *VoiceWakeSessionManager) SessionCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}

func (m *VoiceWakeSessionManager) Sessions() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	return ids
}

func (m *VoiceWakeSessionManager) evictOldest() {
	var oldestID string
	var oldestTime time.Time

	for id, session := range m.sessions {
		if oldestID == "" || session.LastActive.Before(oldestTime) {
			oldestID = id
			oldestTime = session.LastActive
		}
	}

	if oldestID != "" {
		delete(m.sessions, oldestID)
	}
}

func (m *VoiceWakeSessionManager) evictOldestLocked() {
	var oldestID string
	var oldestTime time.Time

	for id, session := range m.sessions {
		if oldestID == "" || session.LastActive.Before(oldestTime) {
			oldestID = id
			oldestTime = session.LastActive
		}
	}

	if oldestID != "" {
		delete(m.sessions, oldestID)
	}
}

func (m *VoiceWakeSessionManager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.sessions = make(map[string]*VoiceWakeSession)
	m.active = ""
}

func (m *VoiceWakeSessionManager) SessionStats(sessionID string) (map[string]any, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("voicewake-session: session %s not found", sessionID)
	}

	return map[string]any{
		"id":               session.ID,
		"created_at":       session.CreatedAt,
		"last_active":      session.LastActive,
		"history_length":   len(session.History),
		"transcript_count": len(session.Transcripts),
		"response_count":   len(session.Responses),
		"wake_event_count": len(session.WakeEvents),
		"is_conversation":  session.IsConversation,
	}, nil
}

func (m *VoiceWakeSessionManager) RecentTranscripts(sessionID string, n int) ([]TranscriptEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("voicewake-session: session %s not found", sessionID)
	}

	transcripts := session.Transcripts
	if len(transcripts) > n {
		transcripts = transcripts[len(transcripts)-n:]
	}

	result := make([]TranscriptEntry, len(transcripts))
	copy(result, transcripts)

	return result, nil
}

func (m *VoiceWakeSessionManager) RecentResponses(sessionID string, n int) ([]ResponseEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("voicewake-session: session %s not found", sessionID)
	}

	responses := session.Responses
	if len(responses) > n {
		responses = responses[len(responses)-n:]
	}

	result := make([]ResponseEntry, len(responses))
	copy(result, responses)

	return result, nil
}

type VoiceWakeSessionContext struct {
	Session *VoiceWakeSession
	Cancel  context.CancelFunc
	Started time.Time
}

type VoiceWakeSessionTracker struct {
	mu       sync.Mutex
	sessions map[string]*VoiceWakeSessionContext
	active   string
	manager  *VoiceWakeSessionManager
}

func NewVoiceWakeSessionTracker(manager *VoiceWakeSessionManager) *VoiceWakeSessionTracker {
	return &VoiceWakeSessionTracker{
		sessions: make(map[string]*VoiceWakeSessionContext),
		manager:  manager,
	}
}

func (t *VoiceWakeSessionTracker) StartSession(sessionID string) (*VoiceWakeSessionContext, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if existing, ok := t.sessions[sessionID]; ok {
		return existing, nil
	}

	session := t.manager.GetOrCreateSession(sessionID)
	ctx, cancel := context.WithCancel(context.Background())
	_ = ctx

	sc := &VoiceWakeSessionContext{
		Session: session,
		Cancel:  cancel,
		Started: time.Now(),
	}

	t.sessions[sessionID] = sc
	t.active = sessionID

	return sc, nil
}

func (t *VoiceWakeSessionTracker) EndSession(sessionID string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	sc, ok := t.sessions[sessionID]
	if !ok {
		return fmt.Errorf("voicewake-tracker: session %s not found", sessionID)
	}

	if sc.Cancel != nil {
		sc.Cancel()
	}

	delete(t.sessions, sessionID)

	if t.active == sessionID {
		t.active = ""
		for id := range t.sessions {
			t.active = id
			break
		}
	}

	return nil
}

func (t *VoiceWakeSessionTracker) ActiveSession() *VoiceWakeSessionContext {
	t.mu.Lock()
	defer t.mu.Unlock()

	sc, ok := t.sessions[t.active]
	if !ok {
		return nil
	}
	return sc
}

func (t *VoiceWakeSessionTracker) ActiveSessionID() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.active
}

func (t *VoiceWakeSessionTracker) GetSession(sessionID string) (*VoiceWakeSessionContext, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	sc, ok := t.sessions[sessionID]
	return sc, ok
}

func (t *VoiceWakeSessionTracker) CancelActive() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if sc, ok := t.sessions[t.active]; ok {
		if sc.Cancel != nil {
			sc.Cancel()
		}
	}
}
