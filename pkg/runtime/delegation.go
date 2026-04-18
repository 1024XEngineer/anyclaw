package runtime

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/1024XEngineer/anyclaw/pkg/capability/tools"
	routehandoff "github.com/1024XEngineer/anyclaw/pkg/route/handoff"
	runtimedelegation "github.com/1024XEngineer/anyclaw/pkg/runtime/delegation"
)

type DelegationRequest = runtimedelegation.Request
type DelegationResult = runtimedelegation.Result

type DelegationService struct {
	mainRuntime *MainRuntime
}

func newDelegationService(mainRuntime *MainRuntime) *DelegationService {
	if mainRuntime == nil {
		return nil
	}
	return &DelegationService{mainRuntime: mainRuntime}
}

func (s *DelegationService) Delegate(ctx context.Context, req DelegationRequest) (*DelegationResult, error) {
	if s == nil || s.mainRuntime == nil || s.mainRuntime.Orchestrator == nil {
		return nil, fmt.Errorf("delegation is unavailable: orchestrator is not enabled")
	}
	req.Task = runtimedelegation.StringFromAny(req.Task)
	if req.Task == "" {
		return nil, fmt.Errorf("task is required")
	}

	plan, err := routehandoff.NewService(s.mainRuntime.Orchestrator).BuildPlan(routehandoff.Request{
		Task:            req.Task,
		AgentNames:      req.AgentNames,
		Reason:          req.Reason,
		SuccessCriteria: req.SuccessCriteria,
		UserContext:     req.UserContext,
	})
	if err != nil {
		return nil, err
	}

	result, err := s.mainRuntime.Orchestrator.RunPlan(ctx, plan)
	if result == nil {
		if err == nil {
			err = fmt.Errorf("delegation failed without a result")
		}
		return nil, err
	}

	errorSummary := ""
	if err != nil {
		errorSummary = err.Error()
	}

	return &DelegationResult{
		Status:          runtimedelegation.StatusForResult(result, err),
		TaskID:          result.TaskID,
		DelegationBrief: plan.Brief,
		SelectedAgents:  plan.TargetAgents,
		Summary:         result.Summary,
		ErrorSummary:    errorSummary,
		Stats:           result.Stats,
		SubTasks:        result.SubTasks,
	}, nil
}

func registerDelegationTool(mainRuntime *MainRuntime) {
	if mainRuntime == nil || mainRuntime.Tools == nil || mainRuntime.Orchestrator == nil {
		return
	}

	service := newDelegationService(mainRuntime)
	mainRuntime.Delegation = service

	mainRuntime.Tools.Register(&tools.Tool{
		Name:        "delegate_task",
		Description: "Delegate a clearly-scoped sub-task to the orchestrator so specialized sub-agents can complete it.",
		Category:    tools.ToolCategoryCustom,
		AccessLevel: tools.ToolAccessPublic,
		Visibility:  tools.ToolVisibilityMainAgentOnly,
		CachePolicy: tools.ToolCachePolicyNever,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task": map[string]any{
					"type":        "string",
					"description": "The delegated sub-task the sub-agents should complete.",
				},
				"agent_names": map[string]any{
					"type":        "array",
					"description": "Optional explicit sub-agent names to use for this delegation.",
					"items":       map[string]string{"type": "string"},
				},
				"reason": map[string]any{
					"type":        "string",
					"description": "Why delegation is useful for this sub-task.",
				},
				"success_criteria": map[string]any{
					"type":        "string",
					"description": "Concrete conditions that define successful completion.",
				},
				"user_context": map[string]any{
					"type":        "string",
					"description": "Relevant user intent or context the sub-agents must preserve.",
				},
			},
			"required": []string{"task"},
		},
		Handler: func(ctx context.Context, input map[string]any) (string, error) {
			if err := tools.RequestToolApproval(ctx, "delegate_task", input); err != nil {
				return "", err
			}

			req := DelegationRequest{
				Task:            runtimedelegation.StringFromAny(input["task"]),
				AgentNames:      runtimedelegation.StringSliceFromAny(input["agent_names"]),
				Reason:          runtimedelegation.StringFromAny(input["reason"]),
				SuccessCriteria: runtimedelegation.StringFromAny(input["success_criteria"]),
				UserContext:     runtimedelegation.StringFromAny(input["user_context"]),
			}
			result, err := service.Delegate(ctx, req)
			if result == nil {
				return "", err
			}
			data, marshalErr := json.Marshal(result)
			if marshalErr != nil {
				return "", marshalErr
			}
			return string(data), nil
		},
	})
}
