package groupchat

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/anyclaw/anyclaw/pkg/orchestrator"
)

type ChatMode string

const (
	ModeCollaboration ChatMode = "collaboration"
	ModeDebate        ChatMode = "debate"
	ModeRoundRobin    ChatMode = "roundrobin"
	ModeFree          ChatMode = "free"
)

type Message struct {
	Role      string    `json:"role"`
	AgentName string    `json:"agent_name,omitempty"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

type GroupSession struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Mode      ChatMode  `json:"mode"`
	Agents    []string  `json:"agents"`
	Messages  []Message `json:"messages"`
	TurnIndex int       `json:"turn_index"`
	MaxRounds int       `json:"max_rounds"`
	CurRound  int       `json:"current_round"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type GroupChatRequest struct {
	SessionID string   `json:"session_id,omitempty"`
	Mode      ChatMode `json:"mode"`
	Agents    []string `json:"agents"`
	Message   string   `json:"message"`
	MaxRounds int      `json:"max_rounds,omitempty"`
}

type GroupChatResponse struct {
	SessionID string    `json:"session_id"`
	Messages  []Message `json:"messages"`
	Done      bool      `json:"done"`
}

type GroupChatManager interface {
	StartChat(ctx context.Context, req GroupChatRequest) (*GroupChatResponse, error)
	ContinueChat(ctx context.Context, sessionID string, message string) (*GroupChatResponse, error)
	GetSession(sessionID string) (*GroupSession, error)
	ListSessions() []GroupSession
	DeleteSession(sessionID string) error
	ListAgents() []orchestrator.AgentInfo
}

type groupChatManager struct {
	mu       sync.RWMutex
	sessions map[string]*GroupSession
	agents   map[string]*orchestrator.SubAgent
	idCount  int
}

func NewGroupChatManager(orch *orchestrator.Orchestrator) GroupChatManager {
	agents := make(map[string]*orchestrator.SubAgent)
	if orch != nil {
		for _, a := range orch.ListAgents() {
			if sa, ok := orch.GetAgent(a.Name); ok {
				agents[a.Name] = sa
			}
		}
	}
	return &groupChatManager{
		sessions: make(map[string]*GroupSession),
		agents:   agents,
	}
}

func (m *groupChatManager) StartChat(ctx context.Context, req GroupChatRequest) (*GroupChatResponse, error) {
	if len(req.Agents) < 2 {
		return nil, fmt.Errorf("至少需要选择 2 个智能体")
	}
	if req.Message == "" {
		return nil, fmt.Errorf("请提供讨论话题")
	}

	maxRounds := req.MaxRounds
	if maxRounds <= 0 {
		maxRounds = 3
	}

	for _, name := range req.Agents {
		if _, ok := m.agents[name]; !ok {
			return nil, fmt.Errorf("智能体不存在: %s", name)
		}
	}

	m.mu.Lock()
	m.idCount++
	sessionID := fmt.Sprintf("group_%d_%d", time.Now().UnixNano(), m.idCount)
	session := &GroupSession{
		ID:        sessionID,
		Title:     shortenText(req.Message, 30),
		Mode:      req.Mode,
		Agents:    req.Agents,
		Messages:  make([]Message, 0),
		TurnIndex: 0,
		MaxRounds: maxRounds,
		CurRound:  0,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	userMsg := Message{
		Role:      "user",
		Content:   req.Message,
		Timestamp: time.Now(),
	}
	session.Messages = append(session.Messages, userMsg)
	m.sessions[sessionID] = session
	m.mu.Unlock()

	response, err := m.runDiscussion(ctx, session)
	if err != nil {
		return nil, err
	}

	return response, nil
}

func (m *groupChatManager) ContinueChat(ctx context.Context, sessionID string, message string) (*GroupChatResponse, error) {
	m.mu.Lock()
	session, ok := m.sessions[sessionID]
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("会话不存在: %s", sessionID)
	}

	userMsg := Message{
		Role:      "user",
		Content:   message,
		Timestamp: time.Now(),
	}
	session.Messages = append(session.Messages, userMsg)
	session.UpdatedAt = time.Now()
	session.CurRound = 0
	m.mu.Unlock()

	response, err := m.runDiscussion(ctx, session)
	if err != nil {
		return nil, err
	}

	return response, nil
}

func (m *groupChatManager) runDiscussion(ctx context.Context, session *GroupSession) (*GroupChatResponse, error) {
	mode := session.Mode
	if mode == "" {
		mode = ModeCollaboration
	}

	switch mode {
	case ModeRoundRobin:
		return m.runRoundRobin(ctx, session)
	case ModeDebate:
		return m.runDebate(ctx, session)
	case ModeCollaboration:
		return m.runCollaboration(ctx, session)
	case ModeFree:
		return m.runFreeDiscussion(ctx, session)
	default:
		return m.runCollaboration(ctx, session)
	}
}

func (m *groupChatManager) runRoundRobin(ctx context.Context, session *GroupSession) (*GroupChatResponse, error) {
	for round := 0; round < session.MaxRounds; round++ {
		session.CurRound = round + 1

		for i, agentName := range session.Agents {
			sa := m.agents[agentName]
			input := m.buildRoundRobinInput(session, agentName, i, round)

			output, err := sa.Run(ctx, input)
			if err != nil {
				output = fmt.Sprintf("[错误: %v]", err)
			}

			msg := Message{
				Role:      "assistant",
				AgentName: agentName,
				Content:   output,
				Timestamp: time.Now(),
			}

			m.mu.Lock()
			session.Messages = append(session.Messages, msg)
			session.UpdatedAt = time.Now()
			session.TurnIndex++
			m.mu.Unlock()
		}
	}

	m.mu.RLock()
	history := make([]Message, len(session.Messages))
	copy(history, session.Messages)
	sessionID := session.ID
	m.mu.RUnlock()

	return &GroupChatResponse{
		SessionID: sessionID,
		Messages:  history,
		Done:      true,
	}, nil
}

func (m *groupChatManager) runDebate(ctx context.Context, session *GroupSession) (*GroupChatResponse, error) {
	agents := session.Agents
	if len(agents) < 2 {
		return nil, fmt.Errorf("辩论模式至少需要 2 个智能体")
	}

	side1 := agents[0]
	side2 := agents[1]

	for round := 0; round < session.MaxRounds; round++ {
		session.CurRound = round + 1

		input1 := m.buildDebateInput(session, side1, side2, round, true)
		output1, err := m.agents[side1].Run(ctx, input1)
		if err != nil {
			output1 = fmt.Sprintf("[错误: %v]", err)
		}
		msg1 := Message{
			Role:      "assistant",
			AgentName: side1,
			Content:   output1,
			Timestamp: time.Now(),
		}

		m.mu.Lock()
		session.Messages = append(session.Messages, msg1)
		session.UpdatedAt = time.Now()
		m.mu.Unlock()

		input2 := m.buildDebateInput(session, side2, side1, round, false)
		output2, err := m.agents[side2].Run(ctx, input2)
		if err != nil {
			output2 = fmt.Sprintf("[错误: %v]", err)
		}
		msg2 := Message{
			Role:      "assistant",
			AgentName: side2,
			Content:   output2,
			Timestamp: time.Now(),
		}

		m.mu.Lock()
		session.Messages = append(session.Messages, msg2)
		session.UpdatedAt = time.Now()
		m.mu.Unlock()
	}

	m.mu.RLock()
	history := make([]Message, len(session.Messages))
	copy(history, session.Messages)
	sessionID := session.ID
	m.mu.RUnlock()

	return &GroupChatResponse{
		SessionID: sessionID,
		Messages:  history,
		Done:      true,
	}, nil
}

func (m *groupChatManager) runCollaboration(ctx context.Context, session *GroupSession) (*GroupChatResponse, error) {
	for round := 0; round < session.MaxRounds; round++ {
		session.CurRound = round + 1

		for _, agentName := range session.Agents {
			sa := m.agents[agentName]
			input := m.buildCollaborationInput(session, agentName, round)

			output, err := sa.Run(ctx, input)
			if err != nil {
				output = fmt.Sprintf("[错误: %v]", err)
			}

			msg := Message{
				Role:      "assistant",
				AgentName: agentName,
				Content:   output,
				Timestamp: time.Now(),
			}

			m.mu.Lock()
			session.Messages = append(session.Messages, msg)
			session.UpdatedAt = time.Now()
			m.mu.Unlock()
		}
	}

	summary := m.buildSummary(ctx, session)

	m.mu.RLock()
	history := make([]Message, len(session.Messages))
	copy(history, session.Messages)
	sessionID := session.ID
	m.mu.RUnlock()

	if summary != "" {
		history = append(history, Message{
			Role:      "system",
			AgentName: "协调者",
			Content:   summary,
			Timestamp: time.Now(),
		})
	}

	return &GroupChatResponse{
		SessionID: sessionID,
		Messages:  history,
		Done:      true,
	}, nil
}

func (m *groupChatManager) runFreeDiscussion(ctx context.Context, session *GroupSession) (*GroupChatResponse, error) {
	for round := 0; round < session.MaxRounds; round++ {
		session.CurRound = round + 1

		for _, agentName := range session.Agents {
			sa := m.agents[agentName]
			input := m.buildFreeDiscussionInput(session, agentName, round)

			output, err := sa.Run(ctx, input)
			if err != nil {
				output = fmt.Sprintf("[错误: %v]", err)
			}

			msg := Message{
				Role:      "assistant",
				AgentName: agentName,
				Content:   output,
				Timestamp: time.Now(),
			}

			m.mu.Lock()
			session.Messages = append(session.Messages, msg)
			session.UpdatedAt = time.Now()
			m.mu.Unlock()
		}
	}

	m.mu.RLock()
	history := make([]Message, len(session.Messages))
	copy(history, session.Messages)
	sessionID := session.ID
	m.mu.RUnlock()

	return &GroupChatResponse{
		SessionID: sessionID,
		Messages:  history,
		Done:      true,
	}, nil
}

func (m *groupChatManager) buildRoundRobinInput(session *GroupSession, agentName string, agentIndex int, round int) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("你是 %s，正在参与一个轮流讨论。\n\n", agentName))

	topic := ""
	for _, msg := range session.Messages {
		if msg.Role == "user" {
			topic = msg.Content
		}
	}
	sb.WriteString(fmt.Sprintf("讨论话题: %s\n\n", topic))
	sb.WriteString(fmt.Sprintf("当前是第 %d/%d 轮讨论，你是第 %d 个发言。\n\n", round+1, session.MaxRounds, agentIndex+1))

	recent := getRecentMessages(session.Messages, agentName, 6)
	if len(recent) > 0 {
		sb.WriteString("之前的讨论内容:\n")
		for _, msg := range recent {
			if msg.Role == "user" {
				sb.WriteString(fmt.Sprintf("[话题]: %s\n", msg.Content))
			} else {
				sb.WriteString(fmt.Sprintf("[%s]: %s\n", msg.AgentName, truncateString(msg.Content, 300)))
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString("请根据你的专业知识，对这个话题发表看法。简洁明了，不超过 200 字。")
	return sb.String()
}

func (m *groupChatManager) buildDebateInput(session *GroupSession, agentName string, opponent string, round int, isFirst bool) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("你是 %s，正在与 %s 进行辩论。\n\n", agentName, opponent))

	topic := ""
	for _, msg := range session.Messages {
		if msg.Role == "user" {
			topic = msg.Content
		}
	}
	sb.WriteString(fmt.Sprintf("辩论话题: %s\n\n", topic))
	sb.WriteString(fmt.Sprintf("当前是第 %d/%d 轮辩论。\n\n", round+1, session.MaxRounds))

	if isFirst {
		sb.WriteString("你是本轮第一个发言，请阐述你的观点。\n\n")
	} else {
		sb.WriteString("请针对对手的观点进行反驳，并强化你的论点。\n\n")
	}

	opponentMessages := getAgentMessages(session.Messages, opponent)
	if len(opponentMessages) > 0 {
		lastOpponent := opponentMessages[len(opponentMessages)-1]
		sb.WriteString(fmt.Sprintf("%s 的最新观点:\n%s\n\n", opponent, truncateString(lastOpponent.Content, 400)))
	}

	sb.WriteString("请发表你的辩论观点。有理有据，简洁有力，不超过 300 字。")
	return sb.String()
}

func (m *groupChatManager) buildCollaborationInput(session *GroupSession, agentName string, round int) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("你是 %s，正在与其他专家协作讨论一个话题。\n\n", agentName))

	topic := ""
	for _, msg := range session.Messages {
		if msg.Role == "user" {
			topic = msg.Content
		}
	}
	sb.WriteString(fmt.Sprintf("协作话题: %s\n\n", topic))
	sb.WriteString(fmt.Sprintf("当前是第 %d/%d 轮讨论。\n\n", round+1, session.MaxRounds))

	otherAgents := []string{}
	for _, a := range session.Agents {
		if a != agentName {
			otherAgents = append(otherAgents, a)
		}
	}
	sb.WriteString(fmt.Sprintf("参与讨论的其他专家: %s\n\n", strings.Join(otherAgents, "、")))

	recent := getRecentMessages(session.Messages, agentName, 8)
	if len(recent) > 0 {
		sb.WriteString("之前的讨论:\n")
		for _, msg := range recent {
			if msg.Role == "user" {
				sb.WriteString(fmt.Sprintf("[用户]: %s\n", msg.Content))
			} else {
				sb.WriteString(fmt.Sprintf("[%s]: %s\n", msg.AgentName, truncateString(msg.Content, 300)))
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("请从你的专业角度 (%s) 发表意见。要参考其他专家的观点，提出建设性建议。简洁明了，不超过 250 字。", agentName))
	return sb.String()
}

func (m *groupChatManager) buildFreeDiscussionInput(session *GroupSession, agentName string, round int) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("你是 %s，正在参与自由讨论。\n\n", agentName))

	topic := ""
	for _, msg := range session.Messages {
		if msg.Role == "user" {
			topic = msg.Content
		}
	}
	sb.WriteString(fmt.Sprintf("讨论话题: %s\n\n", topic))

	recent := getRecentMessages(session.Messages, agentName, 6)
	if len(recent) > 0 {
		sb.WriteString("讨论记录:\n")
		for _, msg := range recent {
			if msg.Role == "user" {
				sb.WriteString(fmt.Sprintf("[用户]: %s\n", msg.Content))
			} else {
				sb.WriteString(fmt.Sprintf("[%s]: %s\n", msg.AgentName, truncateString(msg.Content, 200)))
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString("请自由发言。可以回应其他人的观点，也可以提出新想法。简洁自然，不超过 200 字。")
	return sb.String()
}

func (m *groupChatManager) buildSummary(ctx context.Context, session *GroupSession) string {
	var sb strings.Builder
	sb.WriteString("## 讨论总结\n\n")

	agentOutputs := make(map[string][]string)
	for _, msg := range session.Messages {
		if msg.Role == "assistant" && msg.AgentName != "" {
			agentOutputs[msg.AgentName] = append(agentOutputs[msg.AgentName], msg.Content)
		}
	}

	for agentName, outputs := range agentOutputs {
		sb.WriteString(fmt.Sprintf("### %s 的观点\n", agentName))
		if len(outputs) > 0 {
			last := outputs[len(outputs)-1]
			sb.WriteString(fmt.Sprintf("%s\n\n", truncateString(last, 200)))
		}
	}

	return sb.String()
}

func (m *groupChatManager) GetSession(sessionID string) (*GroupSession, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("会话不存在: %s", sessionID)
	}

	result := *session
	result.Messages = make([]Message, len(session.Messages))
	copy(result.Messages, session.Messages)
	return &result, nil
}

func (m *groupChatManager) ListSessions() []GroupSession {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list := make([]GroupSession, 0, len(m.sessions))
	for _, session := range m.sessions {
		list = append(list, *session)
	}
	return list
}

func (m *groupChatManager) DeleteSession(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.sessions[sessionID]; !ok {
		return fmt.Errorf("会话不存在: %s", sessionID)
	}

	delete(m.sessions, sessionID)
	return nil
}

func (m *groupChatManager) ListAgents() []orchestrator.AgentInfo {
	var list []orchestrator.AgentInfo
	for name := range m.agents {
		list = append(list, orchestrator.AgentInfo{Name: name})
	}
	return list
}

func getRecentMessages(messages []Message, excludeAgent string, limit int) []Message {
	var recent []Message
	for i := len(messages) - 1; i >= 0 && len(recent) < limit; i-- {
		msg := messages[i]
		if msg.Role == "assistant" && msg.AgentName == excludeAgent {
			continue
		}
		recent = append([]Message{msg}, recent...)
	}
	return recent
}

func getAgentMessages(messages []Message, agentName string) []Message {
	var result []Message
	for _, msg := range messages {
		if msg.AgentName == agentName {
			result = append(result, msg)
		}
	}
	return result
}

func truncateString(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func shortenText(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
