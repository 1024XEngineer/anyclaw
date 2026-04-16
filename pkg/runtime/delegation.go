package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anyclaw/anyclaw/pkg/orchestrator"
	"github.com/anyclaw/anyclaw/pkg/tools"
)

type DelegationRequest struct {
	Task            string   `json:"task"`
	AgentNames      []string `json:"agent_names,omitempty"`
	Reason          string   `json:"reason,omitempty"`
	SuccessCriteria string   `json:"success_criteria,omitempty"`
	UserContext     string   `json:"user_context,omitempty"`
}

type DelegationResult struct {
	Status          string                 `json:"status"`
	TaskID          string                 `json:"task_id,omitempty"`
	DelegationBrief string                 `json:"delegation_brief"`
	SelectedAgents  []string               `json:"selected_agents,omitempty"`
	Summary         string                 `json:"summary"`
	ErrorSummary    string                 `json:"error_summary,omitempty"`
	Stats           orchestrator.TaskStats `json:"stats"`
	SubTasks        []orchestrator.SubTask `json:"sub_tasks,omitempty"`
}

type DelegationService struct {
	app *App
}

func newDelegationService(app *App) *DelegationService {
	if app == nil {
		return nil
	}
	return &DelegationService{app: app}
}

func (s *DelegationService) Delegate(ctx context.Context, req DelegationRequest) (*DelegationResult, error) {
	if s == nil || s.app == nil || s.app.Orchestrator == nil {
		return nil, fmt.Errorf("delegation is unavailable: orchestrator is not enabled")
	}
	req.Task = strings.TrimSpace(req.Task)
	if req.Task == "" {
		return nil, fmt.Errorf("task is required")
	}

	brief := buildDelegationBrief(req)
	result, err := s.app.Orchestrator.RunTaskResult(ctx, brief, normalizeNames(req.AgentNames))
	if result == nil {
		if err == nil {
			err = fmt.Errorf("delegation failed without a result")
		}
		return nil, err
	}

	status := delegationStatusForResult(result, err)
	errorSummary := ""
	if err != nil {
		errorSummary = err.Error()
	}

	return &DelegationResult{
		Status:          status,
		TaskID:          result.TaskID,
		DelegationBrief: brief,
		SelectedAgents:  normalizeNames(req.AgentNames),
		Summary:         result.Summary,
		ErrorSummary:    errorSummary,
		Stats:           result.Stats,
		SubTasks:        result.SubTasks,
	}, nil
}

func registerDelegationTool(app *App) {
	if app == nil || app.Tools == nil || app.Orchestrator == nil {
		return
	}

	service := newDelegationService(app)
	app.Delegation = service

	app.Tools.Register(&tools.Tool{
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
				Task:            stringFromAny(input["task"]),
				AgentNames:      stringSliceFromAny(input["agent_names"]),
				Reason:          stringFromAny(input["reason"]),
				SuccessCriteria: stringFromAny(input["success_criteria"]),
				UserContext:     stringFromAny(input["user_context"]),
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

func delegationStatusForResult(result *orchestrator.OrchestratorResult, runErr error) string {
	if result == nil {
		if runErr != nil {
			return "failed"
		}
		return "completed"
	}
	if runErr == nil {
		return "completed"
	}
	if result.Stats.Completed > 0 {
		return "partial_failed"
	}
	return "failed"
}

func buildDelegationBrief(req DelegationRequest) string {
	lines := []string{
		"You are executing a delegated task from the main agent.",
		"",
		"Delegated task:",
		strings.TrimSpace(req.Task),
	}
	if reason := strings.TrimSpace(req.Reason); reason != "" {
		lines = append(lines, "", "Why this was delegated:", reason)
	}
	if context := strings.TrimSpace(req.UserContext); context != "" {
		lines = append(lines, "", "Relevant user context:", context)
	}
	if criteria := strings.TrimSpace(req.SuccessCriteria); criteria != "" {
		lines = append(lines, "", "Success criteria:", criteria)
	}
	lines = append(lines,
		"",
		"Work only within this delegated scope.",
		"Return concrete output that the main agent can integrate back into the user-facing answer.",
	)
	return strings.Join(lines, "\n")
}

func normalizeNames(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	result := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		name := strings.TrimSpace(item)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, name)
	}
	return result
}

func stringFromAny(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func stringSliceFromAny(value any) []string {
	switch items := value.(type) {
	case []string:
		return normalizeNames(items)
	case []any:
		result := make([]string, 0, len(items))
		for _, item := range items {
			result = append(result, stringFromAny(item))
		}
		return normalizeNames(result)
	default:
		return nil
	}
}
