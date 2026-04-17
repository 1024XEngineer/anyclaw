package handoff

import (
	"fmt"
	"strings"
)

type Request struct {
	Task            string
	AgentNames      []string
	Reason          string
	SuccessCriteria string
	UserContext     string
}

type Plan struct {
	Goal            string
	Brief           string
	TargetAgents    []string
	Reason          string
	SuccessCriteria string
	UserContext     string
	ReturnContract  string
}

type AgentCatalog interface {
	AvailableAgentNames() []string
}

type Service struct {
	catalog AgentCatalog
}

func NewService(catalog AgentCatalog) *Service {
	return &Service{catalog: catalog}
}

func (s *Service) BuildPlan(req Request) (*Plan, error) {
	task := strings.TrimSpace(req.Task)
	if task == "" {
		return nil, fmt.Errorf("task is required")
	}

	targetAgents := normalizeNames(req.AgentNames)
	available := availableNames(s)
	if len(targetAgents) > 0 && len(available) > 0 {
		normalizedAvailable := make(map[string]string, len(available))
		for _, name := range available {
			normalizedAvailable[strings.ToLower(name)] = name
		}
		resolved := make([]string, 0, len(targetAgents))
		unknown := make([]string, 0)
		for _, name := range targetAgents {
			canonical, ok := normalizedAvailable[strings.ToLower(name)]
			if !ok {
				unknown = append(unknown, name)
				continue
			}
			resolved = append(resolved, canonical)
		}
		if len(unknown) > 0 {
			return nil, fmt.Errorf("handoff plan contains unknown target agents: %s", strings.Join(unknown, ", "))
		}
		targetAgents = normalizeNames(resolved)
	}
	if len(targetAgents) == 0 {
		switch len(available) {
		case 0:
			return nil, fmt.Errorf("handoff plan requires at least one target agent")
		case 1:
			targetAgents = available
		default:
			return nil, fmt.Errorf("handoff plan requires explicit target agents from the main agent when multiple specialists are available")
		}
	}

	return &Plan{
		Goal:            task,
		Brief:           buildBrief(task, req.Reason, req.SuccessCriteria, req.UserContext),
		TargetAgents:    targetAgents,
		Reason:          strings.TrimSpace(req.Reason),
		SuccessCriteria: strings.TrimSpace(req.SuccessCriteria),
		UserContext:     strings.TrimSpace(req.UserContext),
		ReturnContract:  "Return concrete output that the main agent can integrate into the user-facing response.",
	}, nil
}

func availableNames(s *Service) []string {
	if s == nil || s.catalog == nil {
		return nil
	}
	return normalizeNames(s.catalog.AvailableAgentNames())
}

func buildBrief(task string, reason string, successCriteria string, userContext string) string {
	lines := []string{
		"You are executing a delegated task from the main agent.",
		"",
		"Delegated task:",
		task,
	}
	if trimmed := strings.TrimSpace(reason); trimmed != "" {
		lines = append(lines, "", "Why this was delegated:", trimmed)
	}
	if trimmed := strings.TrimSpace(userContext); trimmed != "" {
		lines = append(lines, "", "Relevant user context:", trimmed)
	}
	if trimmed := strings.TrimSpace(successCriteria); trimmed != "" {
		lines = append(lines, "", "Success criteria:", trimmed)
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
