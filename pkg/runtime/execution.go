package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/anyclaw/anyclaw/pkg/agent"
	appstate "github.com/anyclaw/anyclaw/pkg/apps"
	"github.com/anyclaw/anyclaw/pkg/prompt"
	"github.com/anyclaw/anyclaw/pkg/tools"
)

type ExecutionRequest struct {
	Input                  string
	History                []prompt.Message
	ReplaceHistory         bool
	SessionID              string
	Channel                string
	AgentApprovalHook      agent.ToolApprovalHook
	ProtocolApprovalHook   tools.ToolApprovalHook
	DesktopPlanResumeState *appstate.DesktopPlanExecutionState
	DesktopPlanStateHook   appstate.DesktopPlanStateHook
}

type ExecutionResult struct {
	Output         string
	ToolActivities []agent.ToolActivity
}

func (a *App) Execute(ctx context.Context, req ExecutionRequest) (*ExecutionResult, error) {
	if a == nil || a.Agent == nil {
		return nil, fmt.Errorf("runtime execution is unavailable: agent is not initialized")
	}
	if req.ReplaceHistory {
		a.Agent.SetHistory(req.History)
	}

	execCtx := prepareExecutionContext(ctx, req)
	output, err := a.Agent.Run(execCtx, req.Input)
	result := &ExecutionResult{
		Output:         output,
		ToolActivities: a.Agent.GetLastToolActivities(),
	}
	if err != nil {
		return result, err
	}
	return result, nil
}

func (a *App) Stream(ctx context.Context, req ExecutionRequest, onChunk func(string)) (*ExecutionResult, error) {
	if a == nil || a.Agent == nil {
		return nil, fmt.Errorf("runtime execution is unavailable: agent is not initialized")
	}
	if req.ReplaceHistory {
		a.Agent.SetHistory(req.History)
	}

	execCtx := prepareExecutionContext(ctx, req)
	var out strings.Builder
	err := a.Agent.RunStream(execCtx, req.Input, func(chunk string) {
		out.WriteString(chunk)
		if onChunk != nil {
			onChunk(chunk)
		}
	})
	result := &ExecutionResult{
		Output:         out.String(),
		ToolActivities: a.Agent.GetLastToolActivities(),
	}
	if err != nil {
		return result, err
	}
	return result, nil
}

func (a *App) Run(ctx context.Context, userInput string) (string, error) {
	result, err := a.Execute(ctx, ExecutionRequest{Input: userInput})
	if result == nil {
		return "", err
	}
	return result.Output, err
}

func (a *App) RunStream(ctx context.Context, userInput string, onChunk func(string)) error {
	_, err := a.Stream(ctx, ExecutionRequest{Input: userInput}, onChunk)
	return err
}

func prepareExecutionContext(ctx context.Context, req ExecutionRequest) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(req.SessionID) != "" {
		ctx = tools.WithBrowserSession(ctx, req.SessionID)
		ctx = tools.WithSandboxScope(ctx, tools.SandboxScope{
			SessionID: req.SessionID,
			Channel:   strings.TrimSpace(req.Channel),
		})
	}
	if req.AgentApprovalHook != nil {
		ctx = agent.WithToolApprovalHook(ctx, req.AgentApprovalHook)
	}
	if req.ProtocolApprovalHook != nil {
		ctx = tools.WithToolApprovalHook(ctx, req.ProtocolApprovalHook)
	}
	if req.DesktopPlanResumeState != nil {
		ctx = appstate.WithDesktopPlanResumeState(ctx, req.DesktopPlanResumeState)
	}
	if req.DesktopPlanStateHook != nil {
		ctx = appstate.WithDesktopPlanStateHook(ctx, req.DesktopPlanStateHook)
	}
	return ctx
}
