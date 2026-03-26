package gateway

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/anyclaw/anyclaw/pkg/tools"
)

func (s *Server) resolveAssistantNames(names []string) ([]string, error) {
	resolved := make([]string, 0, len(names))
	seen := make(map[string]bool)
	for _, name := range names {
		resolvedName, err := s.resolveAssistantName(name)
		if err != nil {
			return nil, err
		}
		if resolvedName == "" || seen[resolvedName] {
			continue
		}
		seen[resolvedName] = true
		resolved = append(resolved, resolvedName)
	}
	return resolved, nil
}

func sessionAgentNames(session *Session) []string {
	if session == nil {
		return nil
	}
	participants := normalizeParticipants(session.Agent, session.Participants)
	if len(participants) == 0 && strings.TrimSpace(session.Agent) != "" {
		return []string{strings.TrimSpace(session.Agent)}
	}
	return participants
}

func (s *Server) runGroupSessionMessage(ctx context.Context, session *Session, userMessage string) (string, *Session, error) {
	agents := sessionAgentNames(session)
	if len(agents) == 0 {
		return "", nil, fmt.Errorf("group channel has no agents")
	}

	newMessages := []SessionMessage{
		{
			ID:        uniqueID("msg"),
			Role:      "user",
			Content:   userMessage,
			Kind:      "message",
			CreatedAt: time.Now().UTC(),
		},
	}
	summaryParts := make([]string, 0, len(agents))

	for _, agentName := range agents {
		targetApp, err := s.runtimePool.GetOrCreate(agentName, session.Org, session.Project, session.Workspace)
		if err != nil {
			return "", nil, err
		}
		targetApp.Agent.SetHistory(nil)

		execCtx := tools.WithBrowserSession(ctx, session.ID)
		execCtx = tools.WithSandboxScope(execCtx, tools.SandboxScope{SessionID: session.ID, Channel: "channel"})
		response, err := targetApp.Agent.Run(execCtx, buildCollaborativeSessionInput(session, append([]SessionMessage(nil), newMessages...), agentName))
		if err != nil {
			response = fmt.Sprintf("执行失败: %v", err)
		}

		newMessages = append(newMessages, SessionMessage{
			ID:        uniqueID("msg"),
			Role:      "assistant",
			Agent:     agentName,
			Content:   response,
			Kind:      "message",
			CreatedAt: time.Now().UTC(),
		})
		s.recordSessionToolActivities(session, targetApp.Agent.GetLastToolActivities())
		summaryParts = append(summaryParts, fmt.Sprintf("%s:\n%s", agentName, response))
	}

	updatedSession, err := s.sessions.AddMessages(session.ID, newMessages)
	if err != nil {
		return "", nil, err
	}
	return strings.Join(summaryParts, "\n\n"), updatedSession, nil
}

func buildCollaborativeSessionInput(session *Session, newMessages []SessionMessage, agentName string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("你是频道成员 %s，正在与其他智能体协作回复用户。\n\n", agentName))

	otherAgents := make([]string, 0)
	for _, name := range sessionAgentNames(session) {
		if name != agentName {
			otherAgents = append(otherAgents, name)
		}
	}
	if len(otherAgents) > 0 {
		sb.WriteString(fmt.Sprintf("同频道其他智能体: %s\n\n", strings.Join(otherAgents, "、")))
	}

	transcript := append([]SessionMessage(nil), session.Messages...)
	transcript = append(transcript, newMessages...)
	if len(transcript) > 12 {
		transcript = transcript[len(transcript)-12:]
	}
	sb.WriteString("频道最近消息:\n")
	for _, message := range transcript {
		switch strings.TrimSpace(strings.ToLower(message.Role)) {
		case "user":
			sb.WriteString(fmt.Sprintf("[用户] %s\n", message.Content))
		case "assistant":
			name := strings.TrimSpace(message.Agent)
			if name == "" {
				name = "智能体"
			}
			sb.WriteString(fmt.Sprintf("[%s] %s\n", name, message.Content))
		case "system":
			sb.WriteString(fmt.Sprintf("[系统] %s\n", message.Content))
		}
	}

	sb.WriteString("\n回复要求:\n")
	sb.WriteString("1. 先理解用户目标，再结合前面智能体的结论补充或协作推进。\n")
	sb.WriteString("2. 如果前面的智能体已经覆盖了某部分，就补充你的专业判断，不要机械重复。\n")
	sb.WriteString("3. 如果用户实际上是在发布任务，请直接把它当成你们要协作完成的任务来回应。\n")
	sb.WriteString("4. 回复简洁明确，优先给出可执行结论。\n")
	sb.WriteString("5. 使用中文回复。\n")
	return sb.String()
}
