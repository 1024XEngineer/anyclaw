package delegation

import (
	"fmt"
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/runtime/orchestrator"
)

type Request struct {
	Task            string   `json:"task"`
	AgentNames      []string `json:"agent_names,omitempty"`
	Reason          string   `json:"reason,omitempty"`
	SuccessCriteria string   `json:"success_criteria,omitempty"`
	UserContext     string   `json:"user_context,omitempty"`
	SessionID       string   `json:"session_id,omitempty"`
	SkipDelegation  bool     `json:"skip_delegation,omitempty"`
}

type Result struct {
	Status          string                 `json:"status"`
	TaskID          string                 `json:"task_id,omitempty"`
	DelegationBrief string                 `json:"delegation_brief"`
	SelectedAgents  []string               `json:"selected_agents,omitempty"`
	Summary         string                 `json:"summary"`
	ErrorSummary    string                 `json:"error_summary,omitempty"`
	Stats           orchestrator.TaskStats `json:"stats"`
	SubTasks        []orchestrator.SubTask `json:"sub_tasks,omitempty"`
}

func BuildBrief(req Request) string {
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

func NormalizeNames(items []string) []string {
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

func StatusForResult(result *orchestrator.OrchestratorResult, runErr error) string {
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

func StringFromAny(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func StringSliceFromAny(value any) []string {
	switch items := value.(type) {
	case []string:
		return NormalizeNames(items)
	case []any:
		result := make([]string, 0, len(items))
		for _, item := range items {
			name := StringFromAny(item)
			if name != "" {
				result = append(result, name)
			}
		}
		return NormalizeNames(result)
	default:
		return nil
	}
}
