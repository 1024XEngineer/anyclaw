package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/capability/tools"
	routehandoff "github.com/1024XEngineer/anyclaw/pkg/route/handoff"
	runtimedelegation "github.com/1024XEngineer/anyclaw/pkg/runtime/delegation"
	"github.com/1024XEngineer/anyclaw/pkg/runtime/orchestrator"
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

	brief := runtimedelegation.BuildBrief(req)
	result, selectedAgents, err := s.runDelegation(ctx, req, brief)
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
		DelegationBrief: brief,
		SelectedAgents:  selectedAgents,
		Summary:         result.Summary,
		ErrorSummary:    errorSummary,
		Stats:           result.Stats,
		SubTasks:        result.SubTasks,
	}, nil
}

func (s *DelegationService) runDelegation(ctx context.Context, req DelegationRequest, brief string) (*orchestrator.OrchestratorResult, []string, error) {
	selectedAgents := runtimedelegation.NormalizeNames(req.AgentNames)
	if len(selectedAgents) > 1 {
		result, err := s.mainRuntime.Orchestrator.RunPlan(ctx, brief, selectedAgents)
		return result, selectedAgents, err
	}

	handoffPlan := s.resolveDelegationRoute(req, selectedAgents)
	switch handoffPlan.Mode {
	case routehandoff.ModePersistentSubagent:
		target := strings.TrimSpace(handoffPlan.TargetAgentID)
		if target == "" {
			return nil, nil, fmt.Errorf("handoff route selected a persistent subagent without a target agent id")
		}
		result, err := s.mainRuntime.Orchestrator.RunPlan(ctx, brief, []string{target})
		return result, []string{target}, err
	case routehandoff.ModeTemporarySubagent:
		result, err := s.mainRuntime.Orchestrator.RunTemporaryPlan(ctx, brief, handoffPlan.TargetAgentID)
		return result, selectedAgentsFromResult(result, handoffPlan.TargetAgentID), err
	case routehandoff.ModeMain, "":
		reason := strings.TrimSpace(handoffPlan.Reason)
		if reason == "" {
			reason = "no delegation target was selected"
		}
		return nil, nil, fmt.Errorf("handoff route kept the task on the main agent: %s", reason)
	default:
		return nil, nil, fmt.Errorf("handoff route returned unsupported mode %q", handoffPlan.Mode)
	}
}

func (s *DelegationService) resolveDelegationRoute(req DelegationRequest, selectedAgents []string) routehandoff.HandoffPlan {
	entry := routehandoff.HandoffRoutingEntry{
		SessionID:      strings.TrimSpace(req.SessionID),
		UserInput:      req.Task,
		SkipDelegation: req.SkipDelegation,
	}
	if len(selectedAgents) == 1 {
		entry.PreferredSubagentID = selectedAgents[0]
	}

	router := routehandoff.NewRouter(s.mainRuntime.Orchestrator)
	handoffReq := router.Prepare(entry)
	return router.Plan(handoffReq, routehandoff.PlanOptions{
		PersistentFirst: true,
		AllowTemporary:  true,
	})
}

func selectedAgentsFromResult(result *orchestrator.OrchestratorResult, fallback string) []string {
	seen := map[string]struct{}{}
	selected := make([]string, 0)
	if result != nil {
		for _, subTask := range result.SubTasks {
			name := strings.TrimSpace(subTask.AssignedAgent)
			if name == "" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			selected = append(selected, name)
		}
	}
	if len(selected) > 0 {
		return selected
	}
	if fallback = strings.TrimSpace(fallback); fallback != "" {
		return []string{fallback}
	}
	return nil
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
				"session_id": map[string]any{
					"type":        "string",
					"description": "Optional parent session identifier for routing continuity.",
				},
				"skip_delegation": map[string]any{
					"type":        "boolean",
					"description": "When true, keep the task on the main agent and do not delegate.",
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
				SessionID:       runtimedelegation.StringFromAny(input["session_id"]),
				SkipDelegation:  input["skip_delegation"] == true,
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
