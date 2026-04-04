package pi

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/anyclaw/anyclaw/pkg/agent"
	"github.com/anyclaw/anyclaw/pkg/config"
	"github.com/anyclaw/anyclaw/pkg/llm"
	"github.com/anyclaw/anyclaw/pkg/memory"
	"github.com/anyclaw/anyclaw/pkg/skills"
	"github.com/anyclaw/anyclaw/pkg/tools"
)

type UserID string

type PiAgent struct {
	id       UserID
	name     string
	config   *PiAgentConfig
	agent    *agent.Agent
	memory   memory.MemoryBackend
	skills   *skills.SkillsManager
	tools    *tools.Registry
	workDir  string
	userDir  string
	mu       sync.RWMutex
	llm      *llm.ClientWrapper
	sessions map[string]*UserSession
}

type PiAgentConfig struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	MaxHistory   int    `json:"max_history"`
	EnableMemory bool   `json:"enable_memory"`
	EnableSkills bool   `json:"enable_skills"`
	PrivacyMode  bool   `json:"privacy_mode"`
	MaxToolCalls int    `json:"max_tool_calls"`
	SystemPrompt string `json:"system_prompt"`
	Model        string `json:"model"`
	Provider     string `json:"provider"`
}

type UserSession struct {
	ID        string
	UserID    UserID
	CreatedAt int64
	History   []SessionMessage
	Context   map[string]interface{}
}

type SessionMessage struct {
	Role    string   `json:"role"`
	Content string   `json:"content"`
	Tools   []string `json:"tools,omitempty"`
}

var (
	defaultConfig = PiAgentConfig{
		Name:         "Pi",
		Description:  "Personal Intelligence Agent",
		MaxHistory:   100,
		EnableMemory: true,
		EnableSkills: true,
		PrivacyMode:  true,
		MaxToolCalls: 10,
	}
	agents     = make(map[UserID]*PiAgent)
	agentsLock sync.RWMutex
)

func NewPiAgent(userID UserID, cfg PiAgentConfig, appCfg *config.Config, workDir string) (*PiAgent, error) {
	agentsLock.Lock()
	defer agentsLock.Unlock()

	if existing, ok := agents[userID]; ok {
		return existing, nil
	}

	if cfg.Name == "" {
		cfg.Name = defaultConfig.Name
	}
	if cfg.MaxHistory <= 0 {
		cfg.MaxHistory = defaultConfig.MaxHistory
	}
	if cfg.MaxToolCalls <= 0 {
		cfg.MaxToolCalls = defaultConfig.MaxToolCalls
	}

	userDir := filepath.Join(workDir, "users", string(userID))
	if err := os.MkdirAll(userDir, 0755); err != nil {
		return nil, fmt.Errorf("create user dir: %w", err)
	}

	pi := &PiAgent{
		id:       userID,
		name:     cfg.Name,
		config:   &cfg,
		workDir:  workDir,
		userDir:  userDir,
		sessions: make(map[string]*UserSession),
	}

	agentCfg := agent.Config{
		Name:        cfg.Name,
		Description: cfg.Description,
		WorkDir:     filepath.Join(userDir, ".anyclaw"),
		WorkingDir:  workDir,
	}

	pi.agent = agent.New(agentCfg)

	agents[userID] = pi
	return pi, nil
}

func GetPiAgent(userID UserID) (*PiAgent, bool) {
	agentsLock.RLock()
	defer agentsLock.RUnlock()
	ag, ok := agents[userID]
	return ag, ok
}

func DeletePiAgent(userID UserID) error {
	agentsLock.Lock()
	defer agentsLock.Unlock()
	if _, ok := agents[userID]; !ok {
		return fmt.Errorf("agent not found: %s", userID)
	}
	delete(agents, userID)
	return nil
}

func (p *PiAgent) ID() UserID {
	return p.id
}

func (p *PiAgent) Name() string {
	return p.name
}

func (p *PiAgent) Run(ctx context.Context, input string) (string, error) {
	if p.agent == nil {
		return "", fmt.Errorf("agent not initialized")
	}
	return p.agent.Run(ctx, input)
}

func (p *PiAgent) RunSession(ctx context.Context, sessionID string, input string) (string, error) {
	p.mu.Lock()
	session, ok := p.sessions[sessionID]
	if !ok {
		session = &UserSession{
			ID:      sessionID,
			UserID:  p.id,
			Context: make(map[string]interface{}),
		}
		p.sessions[sessionID] = session
	}
	p.mu.Unlock()

	response, err := p.Run(ctx, input)
	if err != nil {
		return "", err
	}

	p.mu.Lock()
	session.History = append(session.History, SessionMessage{
		Role:    "user",
		Content: input,
	})
	session.History = append(session.History, SessionMessage{
		Role:    "assistant",
		Content: response,
	})
	if len(session.History) > p.config.MaxHistory*2 {
		session.History = session.History[len(session.History)-p.config.MaxHistory*2:]
	}
	p.mu.Unlock()

	return response, nil
}

func (p *PiAgent) GetSession(sessionID string) (*UserSession, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	session, ok := p.sessions[sessionID]
	return session, ok
}

func (p *PiAgent) ListSessions() []*UserSession {
	p.mu.RLock()
	defer p.mu.RUnlock()
	sessions := make([]*UserSession, 0, len(p.sessions))
	for _, s := range p.sessions {
		sessions = append(sessions, s)
	}
	return sessions
}

func (p *PiAgent) DeleteSession(sessionID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.sessions[sessionID]; !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	delete(p.sessions, sessionID)
	return nil
}

func (p *PiAgent) GetHistory(sessionID string) []SessionMessage {
	p.mu.RLock()
	defer p.mu.RUnlock()
	session, ok := p.sessions[sessionID]
	if !ok {
		return nil
	}
	return session.History
}

func (p *PiAgent) ClearHistory(sessionID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.sessions[sessionID]; !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	p.sessions[sessionID].History = nil
	return nil
}

func (p *PiAgent) UserDir() string {
	return p.userDir
}

func (p *PiAgent) IsPrivacyMode() bool {
	return p.config.PrivacyMode
}

func ListAllPiAgents() []*PiAgent {
	agentsLock.RLock()
	defer agentsLock.RUnlock()
	list := make([]*PiAgent, 0, len(agents))
	for _, a := range agents {
		list = append(list, a)
	}
	return list
}
