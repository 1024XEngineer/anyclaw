package cron

import (
	"context"
	"fmt"
	"time"

	"github.com/anyclaw/anyclaw/pkg/agent"
	"github.com/anyclaw/anyclaw/pkg/orchestrator"
)

// AgentExecutor bridges the cron scheduler to the agent system.
// It can execute tasks against a single agent or use the orchestrator
// for multi-agent workflows.
type AgentExecutor struct {
	primaryAgent *agent.Agent
	orchestrator *orchestrator.Orchestrator
}

// NewAgentExecutor creates a new executor that routes tasks to agents.
func NewAgentExecutor(primaryAgent *agent.Agent, orch *orchestrator.Orchestrator) *AgentExecutor {
	return &AgentExecutor{
		primaryAgent: primaryAgent,
		orchestrator: orch,
	}
}

// Execute runs a command against the appropriate agent.
// The cmd field is interpreted as the task description/prompt.
// If the task specifies an agent name, that agent is used.
// If the orchestrator is available and multiple agents are configured,
// it uses the orchestrator for intelligent routing.
func (e *AgentExecutor) Execute(ctx context.Context, cmd string, input map[string]interface{}) (string, error) {
	agentName, _ := input["agent"].(string)

	if agentName != "" && e.orchestrator != nil {
		return e.executeWithOrchestrator(ctx, agentName, cmd, input)
	}

	if e.primaryAgent != nil {
		return e.executeWithAgent(ctx, e.primaryAgent, cmd, input)
	}

	return "", fmt.Errorf("no agent or orchestrator configured for cron execution")
}

func (e *AgentExecutor) executeWithAgent(ctx context.Context, ag *agent.Agent, cmd string, input map[string]interface{}) (string, error) {
	prompt := buildAgentPrompt(cmd, input)

	timeout := 5 * time.Minute
	if deadline, ok := ctx.Deadline(); ok {
		timeout = time.Until(deadline)
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, err := ag.Run(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("agent execution failed: %w", err)
	}

	return result, nil
}

func (e *AgentExecutor) executeWithOrchestrator(ctx context.Context, agentName string, cmd string, input map[string]interface{}) (string, error) {
	_, ok := e.orchestrator.GetAgent(agentName)
	if !ok {
		return "", fmt.Errorf("agent %q not found in orchestrator", agentName)
	}

	prompt := buildAgentPrompt(cmd, input)
	result, err := e.orchestrator.RunTask(ctx, prompt, []string{agentName})
	if err != nil {
		return result, fmt.Errorf("orchestrator task failed: %w", err)
	}

	return result, nil
}

// ExecuteMultiAgent runs a task using the orchestrator's multi-agent capabilities.
func (e *AgentExecutor) ExecuteMultiAgent(ctx context.Context, cmd string, agentNames []string) (string, error) {
	if e.orchestrator == nil {
		return "", fmt.Errorf("orchestrator not available for multi-agent execution")
	}

	result, err := e.orchestrator.RunTask(ctx, cmd, agentNames)
	if err != nil {
		return result, fmt.Errorf("multi-agent task failed: %w", err)
	}

	return result, nil
}

func buildAgentPrompt(cmd string, input map[string]interface{}) string {
	if len(input) == 0 {
		return cmd
	}

	prompt := cmd + "\n\n--- 任务参数 ---\n"
	for k, v := range input {
		prompt += fmt.Sprintf("- %s: %v\n", k, v)
	}
	return prompt
}
