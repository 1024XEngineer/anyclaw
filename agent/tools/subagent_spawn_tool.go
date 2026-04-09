package tools

import (
	"context"
	"fmt"
	"strings"

	"anyclaw/config"
	"anyclaw/internal/logger"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// SubagentTypes - 分身相关类型定义（避免循环导入）

// DeliveryContext 传递上下文
type DeliveryContext struct {
	Channel   string `json:"channel,omitempty"`
	AccountID string `json:"account_id,omitempty"`
	To        string `json:"to,omitempty"`
	ThreadID  string `json:"thread_id,omitempty"`
}

// SubagentRunOutcome 分身运行结果
type SubagentRunOutcome struct {
	Status string `json:"status"` // ok, error, timeout, unknown
	Error  string `json:"error,omitempty"`
}

// SubagentRunParams 分身运行参数
type SubagentRunParams struct {
	RunID               string
	ChildSessionKey     string
	RequesterSessionKey string
	RequesterOrigin     *DeliveryContext
	RequesterDisplayKey string
	Task                string
	Cleanup             string
	Label               string
	ArchiveAfterMinutes int
}

// SubagentSystemPromptParams 系统提示词参数
type SubagentSystemPromptParams struct {
	RequesterSessionKey string
	RequesterOrigin     *DeliveryContext
	ChildSessionKey     string
	Label               string
	Task                string
}

// BuildSubagentSystemPrompt 构建分身系统提示词
func BuildSubagentSystemPrompt(params *SubagentSystemPromptParams) string {
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
		"## Rules",
		"1. **Stay focused** - Do your assigned task, nothing else",
		"2. **Complete the task** - Your final message will be automatically reported to the main agent",
		"3. **Don't initiate** - No heartbeats, no proactive actions, no side quests",
		"4. **Be ephemeral** - You may be terminated after task completion. That's fine.",
		"",
		"## Output Format",
		"When complete, your final response should include:",
		"- What you accomplished or found",
		"- Any relevant details the main agent should know",
		"- Keep it concise but informative",
		"",
		"## What You DON'T Do",
		"- NO user conversations (that's main agent's job)",
		"- NO external messages (email, tweets, etc.) unless explicitly tasked",
		"- NO cron jobs or persistent state",
		"- NO pretending to be the main agent",
	}

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

// normalizeText 规范化文本
func normalizeText(s string) string {
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

// GenerateChildSessionKey 生成子会话密钥
func GenerateChildSessionKey(agentID string) string {
	u := uuid.New()
	return fmt.Sprintf("agent:%s:subagent:%s", agentID, u.String())
}

// GenerateRunID 生成运行ID
func GenerateRunID() string {
	return uuid.New().String()
}

// End SubagentTypes

// SubagentSpawnToolParams 分身生成工具参数
type SubagentSpawnToolParams struct {
	Task              string `json:"task"`                          // 任务描述（必填）
	Label             string `json:"label,omitempty"`               // 可选标签
	AgentID           string `json:"agent_id,omitempty"`            // 目标 Agent ID
	Model             string `json:"model,omitempty"`               // 模型覆盖
	Thinking          string `json:"thinking,omitempty"`            // 思考级别
	RunTimeoutSeconds int    `json:"run_timeout_seconds,omitempty"` // 超时时间
	Cleanup           string `json:"cleanup,omitempty"`             // 清理策略
}

// SubagentSpawnResult 分身生成结果
type SubagentSpawnResult struct {
	Status            string `json:"status"` // accepted, forbidden, error
	ChildSessionKey   string `json:"child_session_key,omitempty"`
	RunID             string `json:"run_id,omitempty"`
	Error             string `json:"error,omitempty"`
	ModelApplied      bool   `json:"model_applied,omitempty"`
	Warning           string `json:"warning,omitempty"`
	ChildSystemPrompt string `json:"child_system_prompt,omitempty"` // System prompt for the child agent
}

// SubagentRegistryInterface 分身注册表接口
type SubagentRegistryInterface interface {
	RegisterRun(params *SubagentRunParams) error
}

// SubagentSpawnTool 分身生成工具
type SubagentSpawnTool struct {
	registry         SubagentRegistryInterface
	getAgentConfig   func(agentID string) *config.AgentConfig
	getDefaultConfig func() *config.AgentDefaults
	getAgentID       func(sessionKey string) string
	onSpawn          func(spawnParams *SubagentSpawnResult) error
}

// NewSubagentSpawnTool 创建分身生成工具
func NewSubagentSpawnTool(registry SubagentRegistryInterface) *SubagentSpawnTool {
	return &SubagentSpawnTool{
		registry: registry,
	}
}

// SetAgentConfigGetter 设置 Agent 配置获取器
func (t *SubagentSpawnTool) SetAgentConfigGetter(getter func(agentID string) *config.AgentConfig) {
	t.getAgentConfig = getter
}

// SetDefaultConfigGetter 设置默认配置获取器
func (t *SubagentSpawnTool) SetDefaultConfigGetter(getter func() *config.AgentDefaults) {
	t.getDefaultConfig = getter
}

// SetAgentIDGetter 设置 Agent ID 获取器
func (t *SubagentSpawnTool) SetAgentIDGetter(getter func(sessionKey string) string) {
	t.getAgentID = getter
}

// SetOnSpawn 设置分身生成回调
func (t *SubagentSpawnTool) SetOnSpawn(fn func(spawnParams *SubagentSpawnResult) error) {
	t.onSpawn = fn
}

// Name 返回工具名称
func (t *SubagentSpawnTool) Name() string {
	return "sessions_spawn"
}

// Description 返回工具描述
func (t *SubagentSpawnTool) Description() string {
	return "Spawn a background sub-agent run in an isolated session and announce the result back to the requester chat."
}

// Parameters 返回工具参数定义
func (t *SubagentSpawnTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"task": map[string]interface{}{
				"type":        "string",
				"description": "The task description for the sub-agent to complete.",
			},
			"label": map[string]interface{}{
				"type":        "string",
				"description": "Optional label for the sub-agent run.",
			},
			"agent_id": map[string]interface{}{
				"type":        "string",
				"description": "Optional target agent ID to spawn the sub-agent under.",
			},
			"model": map[string]interface{}{
				"type":        "string",
				"description": "Optional model override for the sub-agent.",
			},
			"thinking": map[string]interface{}{
				"type":        "string",
				"description": "Optional thinking level override (low, medium, high).",
			},
			"run_timeout_seconds": map[string]interface{}{
				"type":        "integer",
				"description": "Optional timeout in seconds for the sub-agent run.",
			},
			"cleanup": map[string]interface{}{
				"type":        "string",
				"description": "Cleanup strategy: 'delete' to remove immediately, 'keep' to archive after timeout.",
				"enum":        []string{"delete", "keep"},
			},
		},
		"required": []string{"task"},
	}
}

// Execute 执行工具
func (t *SubagentSpawnTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	// 解析参数
	spawnParams, err := t.parseParams(params)
	if err != nil {
		result := &SubagentSpawnResult{
			Status: "error",
			Error:  err.Error(),
		}
		return t.marshalResult(result), nil
	}

	// 验证任务不为空
	if strings.TrimSpace(spawnParams.Task) == "" {
		result := &SubagentSpawnResult{
			Status: "error",
			Error:  "task is required",
		}
		return t.marshalResult(result), nil
	}

	// 规范化清理策略
	if spawnParams.Cleanup != "delete" && spawnParams.Cleanup != "keep" {
		spawnParams.Cleanup = "keep"
	}

	// Get requester session info from context
	requesterSessionKey := "main" // default
	if sk := ctx.Value("session_key"); sk != nil {
		if key, ok := sk.(string); ok {
			requesterSessionKey = key
		}
	}
	requesterAgentID := t.getAgentID(requesterSessionKey)
	if requesterAgentID == "" {
		requesterAgentID = "default"
	}

	// 确定目标 Agent ID
	targetAgentID := requesterAgentID
	if spawnParams.AgentID != "" {
		targetAgentID = spawnParams.AgentID
	}

	// 验证跨 Agent 创建权限
	if targetAgentID != requesterAgentID {
		if !t.checkCrossAgentPermission(requesterAgentID, targetAgentID) {
			result := &SubagentSpawnResult{
				Status: "forbidden",
				Error:  fmt.Sprintf("agentId %s is not allowed for sessions_spawn", targetAgentID),
			}
			return t.marshalResult(result), nil
		}
	}

	// 解析请求者来源
	requesterOrigin := &DeliveryContext{
		Channel:   "cli", // 默认值
		AccountID: "default",
	}

	// 生成子会话密钥
	childSessionKey := GenerateChildSessionKey(targetAgentID)

	// 生成运行 ID
	runID := GenerateRunID()

	// 构建分身系统提示词
	childSystemPrompt := BuildSubagentSystemPrompt(&SubagentSystemPromptParams{
		RequesterSessionKey: requesterSessionKey,
		RequesterOrigin:     requesterOrigin,
		ChildSessionKey:     childSessionKey,
		Label:               spawnParams.Label,
		Task:                spawnParams.Task,
	})
	_ = childSystemPrompt // TODO: 传递给分身实例使用

	// 获取归档时间
	archiveAfterMinutes := 60 // 默认值
	if defCfg := t.getDefaultConfig(); defCfg != nil && defCfg.Subagents != nil {
		if defCfg.Subagents.ArchiveAfterMinutes > 0 {
			archiveAfterMinutes = defCfg.Subagents.ArchiveAfterMinutes
		}
	}

	// 注册分身运行
	if err := t.registry.RegisterRun(&SubagentRunParams{
		RunID:               runID,
		ChildSessionKey:     childSessionKey,
		RequesterSessionKey: requesterSessionKey,
		RequesterOrigin:     requesterOrigin,
		RequesterDisplayKey: requesterSessionKey,
		Task:                spawnParams.Task,
		Cleanup:             spawnParams.Cleanup,
		Label:               spawnParams.Label,
		ArchiveAfterMinutes: archiveAfterMinutes,
	}); err != nil {
		result := &SubagentSpawnResult{
			Status: "error",
			Error:  fmt.Sprintf("failed to register subagent: %v", err),
		}
		return t.marshalResult(result), nil
	}

	// 调用生成回调
	if t.onSpawn != nil {
		spawnResult := &SubagentSpawnResult{
			Status:            "accepted",
			ChildSessionKey:   childSessionKey,
			RunID:             runID,
			ChildSystemPrompt: childSystemPrompt,
		}
		if err := t.onSpawn(spawnResult); err != nil {
			logger.Error("Failed to handle subagent spawn",
				zap.String("run_id", runID),
				zap.Error(err))
		}
	}

	// 构建结果
	result := &SubagentSpawnResult{
		Status:            "accepted",
		ChildSessionKey:   childSessionKey,
		RunID:             runID,
		ChildSystemPrompt: childSystemPrompt,
	}

	logger.Debug("Subagent spawned",
		zap.String("run_id", runID),
		zap.String("task", spawnParams.Task),
		zap.String("child_session_key", childSessionKey),
		zap.String("target_agent_id", targetAgentID))

	return t.marshalResult(result), nil
}

// parseParams 解析参数
func (t *SubagentSpawnTool) parseParams(params map[string]interface{}) (*SubagentSpawnToolParams, error) {
	result := &SubagentSpawnToolParams{
		Cleanup: "keep",
	}

	// 解析 task
	if val, ok := params["task"]; ok {
		if str, ok := val.(string); ok {
			result.Task = str
		}
	}

	// 解析 label
	if val, ok := params["label"]; ok {
		if str, ok := val.(string); ok {
			result.Label = str
		}
	}

	// 解析 agent_id
	if val, ok := params["agent_id"]; ok {
		if str, ok := val.(string); ok {
			result.AgentID = str
		}
	}

	// 解析 model
	if val, ok := params["model"]; ok {
		if str, ok := val.(string); ok {
			result.Model = str
		}
	}

	// 解析 thinking
	if val, ok := params["thinking"]; ok {
		if str, ok := val.(string); ok {
			result.Thinking = str
		}
	}

	// 解析 run_timeout_seconds
	if val, ok := params["run_timeout_seconds"]; ok {
		switch v := val.(type) {
		case float64:
			result.RunTimeoutSeconds = int(v)
		case int:
			result.RunTimeoutSeconds = v
		}
	}

	// 解析 cleanup
	if val, ok := params["cleanup"]; ok {
		if str, ok := val.(string); ok {
			result.Cleanup = str
		}
	}

	return result, nil
}

// marshalResult 序列化结果
func (t *SubagentSpawnTool) marshalResult(result *SubagentSpawnResult) string {
	// 简化输出
	switch result.Status {
	case "accepted":
		return fmt.Sprintf("Subagent spawned successfully. Run ID: %s, Session: %s",
			result.RunID, result.ChildSessionKey)
	case "forbidden":
		return fmt.Sprintf("Forbidden: %s", result.Error)
	case "error":
		return fmt.Sprintf("Error: %s", result.Error)
	default:
		return fmt.Sprintf("Unknown status: %s", result.Status)
	}
}

// checkCrossAgentPermission 检查跨 Agent 创建权限
func (t *SubagentSpawnTool) checkCrossAgentPermission(requesterID, targetID string) bool {
	if t.getAgentConfig == nil {
		return false
	}

	agentCfg := t.getAgentConfig(requesterID)
	if agentCfg == nil || agentCfg.Subagents == nil {
		return false
	}

	// 检查是否允许所有
	allowAgents := agentCfg.Subagents.AllowAgents
	for _, agent := range allowAgents {
		if strings.TrimSpace(agent) == "*" {
			return true
		}
	}

	// 检查是否包含目标 Agent
	for _, agent := range allowAgents {
		if strings.EqualFold(strings.TrimSpace(agent), targetID) {
			return true
		}
	}

	return false
}
