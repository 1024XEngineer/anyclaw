package tui

import (
	"context"
	"time"

	"github.com/anyclaw/anyclaw/pkg/agent"
	"github.com/anyclaw/anyclaw/pkg/config"
	llmwrapper "github.com/anyclaw/anyclaw/pkg/llm"
	"github.com/anyclaw/anyclaw/pkg/prompt"
	"github.com/anyclaw/anyclaw/pkg/skills"
)

type StreamHandler interface {
	RunStream(ctx context.Context, message string, onChunk func(string)) error
}

type AgentAdapter struct {
	agent  *agent.Agent
	client *llmwrapper.ClientWrapper
	cfg    *config.Config
}

func NewAgentAdapter(a *agent.Agent, client *llmwrapper.ClientWrapper, cfg *config.Config) *AgentAdapter {
	return &AgentAdapter{
		agent:  a,
		client: client,
		cfg:    cfg,
	}
}

func (a *AgentAdapter) Run(ctx context.Context, message string) (string, error) {
	return a.agent.Run(ctx, message)
}

func (a *AgentAdapter) RunStream(ctx context.Context, message string, onChunk func(string)) error {
	return a.agent.RunStream(ctx, message, onChunk)
}

func (a *AgentAdapter) GetHistory() []Message {
	history := a.agent.GetHistory()
	messages := make([]Message, len(history))
	for i, h := range history {
		messages[i] = Message{
			Role:      h.Role,
			Content:   h.Content,
			Timestamp: time.Now(),
		}
	}
	return messages
}

func (a *AgentAdapter) ClearHistory() {
	a.agent.ClearHistory()
}

func (a *AgentAdapter) SetHistory(messages []Message) {
	history := make([]prompt.Message, len(messages))
	for i, m := range messages {
		history[i] = prompt.Message{
			Role:    m.Role,
			Content: m.Content,
		}
	}
	a.agent.SetHistory(history)
}

func (a *AgentAdapter) ListSkills() []SkillInfo {
	skills := a.agent.ListSkills()
	result := make([]SkillInfo, len(skills))
	for i, s := range skills {
		result[i] = SkillInfo{
			Name:        s.Name,
			Description: s.Description,
		}
	}
	return result
}

func (a *AgentAdapter) ListTools() []ToolInfo {
	tools := a.agent.ListTools()
	result := make([]ToolInfo, len(tools))
	for i, t := range tools {
		result[i] = ToolInfo{
			Name:        t.Name,
			Description: t.Description,
		}
	}
	return result
}

func (a *AgentAdapter) ShowMemory() (string, error) {
	return a.agent.ShowMemory()
}

type SkillsAdapter struct {
	skills *skills.SkillsManager
}

func NewSkillsAdapter(s *skills.SkillsManager) *SkillsAdapter {
	return &SkillsAdapter{skills: s}
}

func (s *SkillsAdapter) List() []SkillInfo {
	skills := s.skills.List()
	result := make([]SkillInfo, len(skills))
	for i, sk := range skills {
		result[i] = SkillInfo{
			Name:        sk.Name,
			Description: sk.Description,
		}
	}
	return result
}

type LLMAdapter struct {
	client *llmwrapper.ClientWrapper
	cfg    *config.Config
}

func NewLLMAdapter(client *llmwrapper.ClientWrapper, cfg *config.Config) *LLMAdapter {
	return &LLMAdapter{
		client: client,
		cfg:    cfg,
	}
}

func (l *LLMAdapter) GetProvider() string {
	return l.cfg.LLM.Provider
}

func (l *LLMAdapter) GetModel() string {
	return l.cfg.LLM.Model
}

func (l *LLMAdapter) GetTemperature() float64 {
	return l.cfg.LLM.Temperature
}

func (l *LLMAdapter) SwitchProvider(provider string) error {
	return l.client.SwitchProvider(provider)
}

func (l *LLMAdapter) SwitchModel(model string) error {
	return l.client.SwitchModel(model)
}

func (l *LLMAdapter) SetTemperature(temp float64) {
	l.client.SetTemperature(temp)
	l.cfg.LLM.Temperature = temp
}
