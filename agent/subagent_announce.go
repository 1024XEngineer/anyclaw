package agent

import (
	"fmt"
	"time"

	"anyclaw/internal/logger"
	"go.uber.org/zap"
)

// SubagentAnnounceType 分身宣告类型
type SubagentAnnounceType string

const (
	SubagentAnnounceTypeTask SubagentAnnounceType = "subagent task"
	SubagentAnnounceTypeCron SubagentAnnounceType = "cron job"
)

// SubagentAnnounceParams 分身宣告参数
type SubagentAnnounceParams struct {
	ChildSessionKey     string
	ChildRunID          string
	RequesterSessionKey string
	RequesterOrigin     *DeliveryContext
	RequesterDisplayKey string
	Task                string
	Label               string
	StartedAt           *int64
	EndedAt             *int64
	Outcome             *SubagentRunOutcome
	Cleanup             string
	AnnounceType        SubagentAnnounceType
	TimeoutSeconds      int
}

// AnnounceCallback 宣告回调
type AnnounceCallback func(sessionKey, message string) error

// SubagentAnnouncer 分身宣告器
type SubagentAnnouncer struct {
	onAnnounce AnnounceCallback
}

// NewSubagentAnnouncer 创建分身宣告器
func NewSubagentAnnouncer(onAnnounce AnnounceCallback) *SubagentAnnouncer {
	return &SubagentAnnouncer{
		onAnnounce: onAnnounce,
	}
}

// RunAnnounceFlow 执行宣告流程
func (a *SubagentAnnouncer) RunAnnounceFlow(params *SubagentAnnounceParams) error {
	// 构建状态标签
	var statusLabel string
	if params.Outcome != nil {
		switch params.Outcome.Status {
		case "ok":
			statusLabel = "completed successfully"
		case "timeout":
			statusLabel = "timed out"
		case "error":
			statusLabel = fmt.Sprintf("failed: %s", params.Outcome.Error)
		default:
			statusLabel = "finished with unknown status"
		}
	} else {
		statusLabel = "finished with unknown status"
	}

	// 获取任务标签
	taskLabel := params.Label
	if taskLabel == "" {
		taskLabel = params.Task
	}

	// 获取分身类型
	announceType := params.AnnounceType
	if announceType == "" {
		announceType = SubagentAnnounceTypeTask
	}

	// 构建统计信息
	statsLine := a.buildStatsLine(params)

	// 构建宣告消息
	triggerMessage := fmt.Sprintf(`A %s "%s" just %s.

Findings:
%s

%s

Summarize this naturally for the user. Keep it brief (1-2 sentences). Flow it into the conversation naturally.
Do not mention technical details like tokens, stats, or that this was a %s.
You can respond with NO_REPLY if no announcement is needed (e.g., internal task with no user-facing result).`,
		announceType, taskLabel, statusLabel, params.Task, statsLine, announceType)

	// 发送宣告到主 Agent
	if err := a.onAnnounce(params.RequesterSessionKey, triggerMessage); err != nil {
		logger.Error("Failed to announce subagent result",
			zap.String("run_id", params.ChildRunID),
			zap.Error(err))
		return err
	}

	logger.Info("Subagent result announced",
		zap.String("run_id", params.ChildRunID),
		zap.String("task", taskLabel),
		zap.String("status", statusLabel))

	return nil
}

// buildStatsLine 构建统计信息行
func (a *SubagentAnnouncer) buildStatsLine(params *SubagentAnnounceParams) string {
	parts := []string{}

	// 运行时间
	if params.StartedAt != nil && params.EndedAt != nil {
		runtimeMs := *params.EndedAt - *params.StartedAt
		parts = append(parts, fmt.Sprintf("runtime %s", formatDuration(runtimeMs)))
	}

	// 会话密钥
	parts = append(parts, fmt.Sprintf("sessionKey %s", params.ChildSessionKey))

	return fmt.Sprintf("Stats: %s", joinParts(parts, " • "))
}

// formatDuration 格式化持续时间
func formatDuration(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	if ms < 60_000 {
		return fmt.Sprintf("%.1fs", float64(ms)/1000)
	}
	if ms < 3600_000 {
		min := ms / 60_000
		sec := (ms % 60_000) / 1000
		return fmt.Sprintf("%dm%ds", min, sec)
	}
	hour := ms / 3600_000
	min := (ms % 3600_000) / 60_000
	return fmt.Sprintf("%dh%dm", hour, min)
}

// joinParts 连接部分
func joinParts(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += sep + parts[i]
	}
	return result
}

// BuildSubagentSystemPrompt 构建分身系统提示词
func BuildSubagentSystemPrompt(params *SubagentSystemPromptParams) string {
	// 清理任务描述
	taskText := normalizeText(params.Task)
	if taskText == "" {
		taskText = "{{TASK_DESCRIPTION}}"
	}

	lines := []string{
		"# Subagent Context",
		"",
		"You are a **subagent** spawned by the main agent for a specific task.",
		"",
		"## Your Role",
		fmt.Sprintf("- You were created to handle: %s", taskText),
		"- Complete this task. That's your entire purpose.",
		"- You are NOT the main agent. Don't try to be.",
		"",
		"## Core Rules",
		"1. **Stay focused** - Do your assigned task, nothing else",
		"2. **Complete the task** - Your final message will be automatically reported to the main agent",
		"3. **Don't initiate** - No heartbeats, no proactive actions, no side quests",
		"4. **Be ephemeral** - You will be terminated after task completion. That's fine.",
		"5. **No pretending** - Never act as if you're the main agent or have broader authority",
		"",
		"## Output Format",
		"When complete, your final response should include:",
		"- What you accomplished or found",
		"- Any relevant details the main agent should know",
		"- Keep it concise but informative",
		"- No meta-commentary about being a subagent",
		"",
		"## What You DON'T Do",
		"- NO user conversations (that's main agent's job)",
		"- NO external messages (email, tweets, etc.) unless explicitly tasked",
		"- NO cron jobs or persistent state",
		"- NO spawning other subagents",
		"- NO system management tasks (gateway, config, updates)",
		"- NO claiming to be the main agent",
		"",
		"## Tool Policy",
		"- Some tools are intentionally denied (sessions_spawn, gateway, cron, etc.)",
		"- If you need a denied tool, that's a signal to ask the main agent for help",
		"- Focus on completing your task with available tools",
	}

	// 添加上下文信息
	if params.Label != "" {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("- Label: %s", params.Label))
	}
	if params.RequesterSessionKey != "" {
		lines = append(lines, fmt.Sprintf("- Requester session: %s", params.RequesterSessionKey))
	}
	if params.RequesterOrigin != nil && params.RequesterOrigin.Channel != "" {
		lines = append(lines, fmt.Sprintf("- Requester channel: %s", params.RequesterOrigin.Channel))
	}
	lines = append(lines, fmt.Sprintf("- Your session: %s", params.ChildSessionKey))
	lines = append(lines, "")

	return joinLines(lines)
}

// SubagentSystemPromptParams 系统提示词参数
type SubagentSystemPromptParams struct {
	RequesterSessionKey string
	RequesterOrigin     *DeliveryContext
	ChildSessionKey     string
	Label               string
	Task                string
}

// joinLines 连接行
func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	result := lines[0]
	for i := 1; i < len(lines); i++ {
		result += "\n" + lines[i]
	}
	return result
}

// normalizeText 规范化文本
func normalizeText(s string) string {
	// 移除多余空格
	inSpace := false
	var result []rune
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' {
			if !inSpace {
				result = append(result, ' ')
				inSpace = true
			}
		} else {
			result = append(result, r)
			inSpace = false
		}
	}
	return string(result)
}

// DefaultToolDenyList 默认拒绝的工具列表
var DefaultToolDenyList = []string{
	"sessions_spawn", // 防止嵌套创建
	"sessions_list",  // 会话管理 - 主 Agent 协调
	"sessions_history",
	"sessions_delete",
	"gateway", // 系统管理 - 分身不应操作
	"cron",    // 定时任务
}

// ResolveToolPolicy 解析工具策略
func ResolveToolPolicy(denyTools []string, allowTools []string) *ToolPolicy {
	policy := &ToolPolicy{
		Deny:  make(map[string]bool),
		Allow: make(map[string]bool),
	}

	// 先添加默认拒绝列表
	for _, tool := range DefaultToolDenyList {
		policy.Deny[tool] = true
	}

	// 添加配置的拒绝列表
	for _, tool := range denyTools {
		policy.Deny[tool] = true
	}

	// 如果有允许列表，则使用 allow-only 模式
	if len(allowTools) > 0 {
		policy.AllowOnly = true
		for _, tool := range allowTools {
			policy.Allow[tool] = true
		}
	}

	return policy
}

// ToolPolicy 工具策略
type ToolPolicy struct {
	Deny      map[string]bool
	Allow     map[string]bool
	AllowOnly bool
}

// IsToolAllowed 检查工具是否被允许
func (p *ToolPolicy) IsToolAllowed(toolName string) bool {
	// 先检查拒绝列表（优先）
	if p.Deny[toolName] {
		return false
	}

	// 如果是 allow-only 模式，检查是否在允许列表中
	if p.AllowOnly {
		return p.Allow[toolName]
	}

	// 默认允许
	return true
}

// WaitForSubagentCompletion 等待分身完成
func WaitForSubagentCompletion(runID string, timeoutSeconds int, waitFunc func(string, int) (*SubagentCompletion, error)) (*SubagentCompletion, error) {
	timeout := time.Duration(timeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}

	done := make(chan *SubagentCompletion, 1)
	errChan := make(chan error, 1)

	go func() {
		result, err := waitFunc(runID, timeoutSeconds)
		if err != nil {
			errChan <- err
			return
		}
		done <- result
	}()

	select {
	case result := <-done:
		return result, nil
	case err := <-errChan:
		return nil, err
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout waiting for subagent completion")
	}
}

// SubagentCompletion 分身完成结果
type SubagentCompletion struct {
	Status    string // ok, error, timeout
	StartedAt int64
	EndedAt   int64
	Error     string
}
