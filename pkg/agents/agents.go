package agents

import (
	"context"
	"sync"

	"github.com/anyclaw/anyclaw/pkg/llm"
	"github.com/anyclaw/anyclaw/pkg/memory"
	"github.com/anyclaw/anyclaw/pkg/skills"
	"github.com/anyclaw/anyclaw/pkg/tools"
)

type Agent struct {
	ID          string
	Name        string
	Description string
	Model       string
	Provider    string
	Personality string
	Memory      memory.MemoryBackend
	Skills      *skills.SkillsManager
	Tools       *tools.Registry
	LLM         llm.Client
	Config      AgentConfig
}

type AgentConfig struct {
	MaxTokens    int
	Temperature  float64
	SystemPrompt string
	ToolsEnabled bool
}

type Manager struct {
	mu     sync.RWMutex
	agents map[string]*Agent
}

func NewManager() *Manager {
	return &Manager{
		agents: make(map[string]*Agent),
	}
}

func (m *Manager) Register(agent *Agent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.agents[agent.ID] = agent
}

func (m *Manager) Get(id string) *Agent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.agents[id]
}

func (m *Manager) GetByName(name string) *Agent {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, agent := range m.agents {
		if agent.Name == name {
			return agent
		}
	}
	return nil
}

func (m *Manager) List() []*Agent {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Agent, 0, len(m.agents))
	for _, agent := range m.agents {
		result = append(result, agent)
	}
	return result
}

func (m *Manager) Remove(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.agents, id)
}

func (a *Agent) Run(ctx context.Context, input string) (string, error) {
	messages := []llm.Message{
		{Role: "system", Content: a.Config.SystemPrompt},
		{Role: "user", Content: input},
	}

	resp, err := a.LLM.Chat(ctx, messages, nil)
	if err != nil {
		return "", err
	}

	return resp.Content, nil
}

func (a *Agent) RunWithTools(ctx context.Context, input string, toolRegistry *tools.Registry) (string, []ToolCall, error) {
	messages := []llm.Message{
		{Role: "system", Content: a.Config.SystemPrompt},
		{Role: "user", Content: input},
	}

	tools := toolRegistry.List()
	var llmTools []llm.ToolDefinition
	for _, t := range tools {
		llmTools = append(llmTools, llm.ToolDefinition{
			Type: "function",
			Function: llm.ToolFunctionDefinition{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		})
	}

	resp, err := a.LLM.Chat(ctx, messages, llmTools)
	if err != nil {
		return "", nil, err
	}

	var toolCalls []ToolCall
	for _, tc := range resp.ToolCalls {
		args := make(map[string]any)
		toolCalls = append(toolCalls, ToolCall{
			ID:   tc.ID,
			Name: tc.Function.Name,
			Args: args,
		})
	}

	return resp.Content, toolCalls, nil
}

type ToolCall struct {
	ID   string
	Name string
	Args map[string]any
}

type AgentRequest struct {
	AgentID   string
	Input     string
	SessionID string
	Context   map[string]any
}

type AgentResponse struct {
	AgentID   string
	Output    string
	ToolCalls []ToolCall
	Error     error
}

func (m *Manager) Run(ctx context.Context, req *AgentRequest) *AgentResponse {
	agent := m.Get(req.AgentID)
	if agent == nil {
		return &AgentResponse{
			AgentID: req.AgentID,
			Error:   nil,
		}
	}

	output, toolCalls, err := agent.RunWithTools(ctx, req.Input, agent.Tools)
	if err != nil {
		return &AgentResponse{
			AgentID: req.AgentID,
			Error:   err,
		}
	}

	return &AgentResponse{
		AgentID:   req.AgentID,
		Output:    output,
		ToolCalls: toolCalls,
	}
}
