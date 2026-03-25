package channel2

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/anyclaw/anyclaw/pkg/orchestrator"
)

type ChannelType string

const (
	TypeDM    ChannelType = "dm"
	TypeGroup ChannelType = "group"
)

type Message struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"`
	AgentName string    `json:"agent_name,omitempty"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

type Channel struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	Type      ChannelType `json:"type"`
	Agents    []string    `json:"agents"`
	Messages  []Message   `json:"messages"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
}

type CreateChannelRequest struct {
	Name   string      `json:"name"`
	Type   ChannelType `json:"type"`
	Agents []string    `json:"agents"`
}

type SendRequest struct {
	ChannelID string `json:"channel_id"`
	Content   string `json:"content"`
}

type ChannelManager interface {
	CreateChannel(req CreateChannelRequest) (*Channel, error)
	GetChannel(id string) (*Channel, error)
	ListChannels() []Channel
	DeleteChannel(id string) error
	AddAgent(channelID string, agentName string) error
	RemoveAgent(channelID string, agentName string) error
	SendMessage(ctx context.Context, req SendRequest) ([]Message, error)
	GetHistory(channelID string, limit int) ([]Message, error)
	ListAgents() []orchestrator.AgentInfo
}

type channelManager struct {
	mu       sync.RWMutex
	channels map[string]*Channel
	agents   map[string]*orchestrator.SubAgent
	idCount  int
	msgCount int
}

func NewChannelManager(orch *orchestrator.Orchestrator) ChannelManager {
	agents := make(map[string]*orchestrator.SubAgent)
	if orch != nil {
		for _, a := range orch.ListAgents() {
			if sa, ok := orch.GetAgent(a.Name); ok {
				agents[a.Name] = sa
			}
		}
	}
	return &channelManager{
		channels: make(map[string]*Channel),
		agents:   agents,
	}
}

func (m *channelManager) CreateChannel(req CreateChannelRequest) (*Channel, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("频道名称不能为空")
	}

	if req.Type == TypeDM {
		if len(req.Agents) != 1 {
			return nil, fmt.Errorf("私聊频道只能有一个智能体")
		}
		if _, ok := m.agents[req.Agents[0]]; !ok {
			return nil, fmt.Errorf("智能体不存在: %s", req.Agents[0])
		}
	} else {
		for _, name := range req.Agents {
			if _, ok := m.agents[name]; !ok {
				return nil, fmt.Errorf("智能体不存在: %s", name)
			}
		}
	}

	m.mu.Lock()
	m.idCount++
	channelID := fmt.Sprintf("ch_%d_%d", time.Now().UnixNano(), m.idCount)

	channel := &Channel{
		ID:        channelID,
		Name:      req.Name,
		Type:      req.Type,
		Agents:    make([]string, len(req.Agents)),
		Messages:  make([]Message, 0),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	copy(channel.Agents, req.Agents)

	m.channels[channelID] = channel
	m.mu.Unlock()

	return channel, nil
}

func (m *channelManager) GetChannel(id string) (*Channel, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ch, ok := m.channels[id]
	if !ok {
		return nil, fmt.Errorf("频道不存在: %s", id)
	}

	result := *ch
	result.Agents = make([]string, len(ch.Agents))
	copy(result.Agents, ch.Agents)
	result.Messages = make([]Message, len(ch.Messages))
	copy(result.Messages, ch.Messages)
	return &result, nil
}

func (m *channelManager) ListChannels() []Channel {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list := make([]Channel, 0, len(m.channels))
	for _, ch := range m.channels {
		list = append(list, *ch)
	}
	return list
}

func (m *channelManager) DeleteChannel(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.channels[id]; !ok {
		return fmt.Errorf("频道不存在: %s", id)
	}

	delete(m.channels, id)
	return nil
}

func (m *channelManager) AddAgent(channelID string, agentName string) error {
	if _, ok := m.agents[agentName]; !ok {
		return fmt.Errorf("智能体不存在: %s", agentName)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	ch, ok := m.channels[channelID]
	if !ok {
		return fmt.Errorf("频道不存在: %s", channelID)
	}

	for _, name := range ch.Agents {
		if name == agentName {
			return fmt.Errorf("智能体已在频道中: %s", agentName)
		}
	}

	ch.Agents = append(ch.Agents, agentName)
	ch.UpdatedAt = time.Now()

	m.msgCount++
	systemMsg := Message{
		ID:        fmt.Sprintf("msg_%d_%d", time.Now().UnixNano(), m.msgCount),
		Role:      "system",
		Content:   fmt.Sprintf("%s 已加入频道", agentName),
		Timestamp: time.Now(),
	}
	ch.Messages = append(ch.Messages, systemMsg)

	return nil
}

func (m *channelManager) RemoveAgent(channelID string, agentName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ch, ok := m.channels[channelID]
	if !ok {
		return fmt.Errorf("频道不存在: %s", channelID)
	}

	if ch.Type == TypeDM {
		return fmt.Errorf("私聊频道不能移除智能体")
	}

	found := false
	newAgents := make([]string, 0, len(ch.Agents))
	for _, name := range ch.Agents {
		if name == agentName {
			found = true
		} else {
			newAgents = append(newAgents, name)
		}
	}

	if !found {
		return fmt.Errorf("智能体不在频道中: %s", agentName)
	}

	ch.Agents = newAgents
	ch.UpdatedAt = time.Now()

	m.msgCount++
	systemMsg := Message{
		ID:        fmt.Sprintf("msg_%d_%d", time.Now().UnixNano(), m.msgCount),
		Role:      "system",
		Content:   fmt.Sprintf("%s 已离开频道", agentName),
		Timestamp: time.Now(),
	}
	ch.Messages = append(ch.Messages, systemMsg)

	return nil
}

func (m *channelManager) SendMessage(ctx context.Context, req SendRequest) ([]Message, error) {
	if req.Content == "" {
		return nil, fmt.Errorf("消息内容不能为空")
	}

	m.mu.Lock()
	ch, ok := m.channels[req.ChannelID]
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("频道不存在: %s", req.ChannelID)
	}

	m.msgCount++
	userMsg := Message{
		ID:        fmt.Sprintf("msg_%d_%d", time.Now().UnixNano(), m.msgCount),
		Role:      "user",
		Content:   req.Content,
		Timestamp: time.Now(),
	}
	ch.Messages = append(ch.Messages, userMsg)
	ch.UpdatedAt = time.Now()
	m.mu.Unlock()

	var responses []Message
	var err error

	if ch.Type == TypeDM {
		responses, err = m.handleDM(ctx, ch)
	} else {
		responses, err = m.handleGroup(ctx, ch)
	}

	if err != nil {
		return nil, err
	}

	return responses, nil
}

func (m *channelManager) handleDM(ctx context.Context, ch *Channel) ([]Message, error) {
	agentName := ch.Agents[0]
	sa := m.agents[agentName]

	input := m.buildDMInput(ch, agentName)

	output, err := sa.Run(ctx, input)
	if err != nil {
		output = fmt.Sprintf("[错误: %v]", err)
	}

	m.mu.Lock()
	m.msgCount++
	msg := Message{
		ID:        fmt.Sprintf("msg_%d_%d", time.Now().UnixNano(), m.msgCount),
		Role:      "assistant",
		AgentName: agentName,
		Content:   output,
		Timestamp: time.Now(),
	}
	ch.Messages = append(ch.Messages, msg)
	ch.UpdatedAt = time.Now()
	m.mu.Unlock()

	return []Message{msg}, nil
}

func (m *channelManager) handleGroup(ctx context.Context, ch *Channel) ([]Message, error) {
	var responses []Message

	for _, agentName := range ch.Agents {
		sa := m.agents[agentName]
		input := m.buildGroupInput(ch, agentName)

		output, err := sa.Run(ctx, input)
		if err != nil {
			output = fmt.Sprintf("[错误: %v]", err)
		}

		m.mu.Lock()
		m.msgCount++
		msg := Message{
			ID:        fmt.Sprintf("msg_%d_%d", time.Now().UnixNano(), m.msgCount),
			Role:      "assistant",
			AgentName: agentName,
			Content:   output,
			Timestamp: time.Now(),
		}
		ch.Messages = append(ch.Messages, msg)
		ch.UpdatedAt = time.Now()
		m.mu.Unlock()

		responses = append(responses, msg)
	}

	return responses, nil
}

func (m *channelManager) buildDMInput(ch *Channel, agentName string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("你是 %s，正在与用户私聊。\n\n", agentName))

	recent := getRecentMessages(ch.Messages, 6)
	if len(recent) > 0 {
		sb.WriteString("对话记录:\n")
		for _, msg := range recent {
			if msg.Role == "user" {
				sb.WriteString(fmt.Sprintf("[用户]: %s\n", msg.Content))
			} else if msg.Role == "assistant" {
				sb.WriteString(fmt.Sprintf("[%s]: %s\n", msg.AgentName, truncateString(msg.Content, 300)))
			}
		}
		sb.WriteString("\n")
	}

	lastUserMsg := getLastUserMessage(ch.Messages)
	if lastUserMsg != nil {
		sb.WriteString(fmt.Sprintf("用户最新消息: %s\n\n", lastUserMsg.Content))
	}

	sb.WriteString("请回复用户的消息。用中文回复。")
	return sb.String()
}

func (m *channelManager) buildGroupInput(ch *Channel, agentName string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("你是 %s，正在一个团队群聊中与其他专家协作完成用户任务。\n\n", agentName))

	otherAgents := []string{}
	for _, a := range ch.Agents {
		if a != agentName {
			otherAgents = append(otherAgents, a)
		}
	}
	if len(otherAgents) > 0 {
		sb.WriteString(fmt.Sprintf("团队成员: %s\n\n", strings.Join(otherAgents, "、")))
	}

	recent := getRecentMessages(ch.Messages, 10)
	if len(recent) > 0 {
		sb.WriteString("对话记录:\n")
		for _, msg := range recent {
			switch msg.Role {
			case "user":
				sb.WriteString(fmt.Sprintf("[用户]: %s\n", msg.Content))
			case "system":
				sb.WriteString(fmt.Sprintf("[系统]: %s\n", msg.Content))
			case "assistant":
				sb.WriteString(fmt.Sprintf("[%s]: %s\n", msg.AgentName, truncateString(msg.Content, 250)))
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("请从你的专业角度 (%s) 发表意见。\n", agentName))
	sb.WriteString("- 如果用户提出了任务，请分析任务并提供你的专业建议\n")
	sb.WriteString("- 可以参考其他专家的观点，提出建设性意见\n")
	sb.WriteString("- 如果任务涉及你的专业领域，请主动承担相关工作\n")
	sb.WriteString("- 简洁明了，不超过 250 字\n")
	sb.WriteString("- 用中文回复")
	return sb.String()
}

func (m *channelManager) GetHistory(channelID string, limit int) ([]Message, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ch, ok := m.channels[channelID]
	if !ok {
		return nil, fmt.Errorf("频道不存在: %s", channelID)
	}

	if limit <= 0 || limit > len(ch.Messages) {
		limit = len(ch.Messages)
	}

	start := len(ch.Messages) - limit
	result := make([]Message, limit)
	copy(result, ch.Messages[start:])
	return result, nil
}

func (m *channelManager) ListAgents() []orchestrator.AgentInfo {
	var list []orchestrator.AgentInfo
	for name := range m.agents {
		list = append(list, orchestrator.AgentInfo{Name: name})
	}
	return list
}

func getRecentMessages(messages []Message, limit int) []Message {
	if limit <= 0 || limit > len(messages) {
		limit = len(messages)
	}
	start := len(messages) - limit
	result := make([]Message, limit)
	copy(result, messages[start:])
	return result
}

func getLastUserMessage(messages []Message) *Message {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return &messages[i]
		}
	}
	return nil
}

func truncateString(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
