package taskrunner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	llm "github.com/1024XEngineer/anyclaw/pkg/capability/models"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

func (m *Manager) planTask(ctx context.Context, input string) (string, []plannedStep) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return defaultPlan(input)
	}

	if m.planner == nil {
		return defaultPlan(trimmed)
	}
	messages := []llm.Message{
		{Role: "system", Content: "You generate concise execution plans for local AI tasks. Return JSON only with fields summary and steps. Each step must contain title and kind. Use 4 to 6 steps. kinds can be analyze, inspect, execute, verify, summarize. Plans must include a verify step before summarize and should assume the runtime will observe real state before declaring success."},
		{Role: "user", Content: fmt.Sprintf("Plan this task for execution in a local assistant runtime: %s", trimmed)},
	}
	resp, err := m.planner.Chat(ctx, messages, nil)
	if err != nil {
		return defaultPlan(trimmed)
	}
	var payload struct {
		Summary string        `json:"summary"`
		Steps   []plannedStep `json:"steps"`
	}
	raw := strings.TrimSpace(resp.Content)
	if raw == "" {
		return defaultPlan(trimmed)
	}
	if err := json.Unmarshal([]byte(extractJSON(raw)), &payload); err != nil {
		return defaultPlan(trimmed)
	}
	payload.Summary = strings.TrimSpace(payload.Summary)
	if payload.Summary == "" || len(payload.Steps) == 0 {
		return defaultPlan(trimmed)
	}
	steps := make([]plannedStep, 0, len(payload.Steps))
	for _, step := range payload.Steps {
		title := strings.TrimSpace(step.Title)
		kind := normalizeStepKind(step.Kind)
		if title == "" {
			continue
		}
		steps = append(steps, plannedStep{Title: title, Kind: kind})
	}
	if len(steps) == 0 {
		return defaultPlan(trimmed)
	}
	steps = ensureRequiredPlanSteps(steps)
	return payload.Summary, steps
}

func defaultPlan(input string) (string, []plannedStep) {
	trimmed := strings.TrimSpace(input)
	summary := "Analyze the request, inspect the workspace if needed, execute the task, verify the observable outcome, and summarize the result."
	if trimmed != "" {
		summary = fmt.Sprintf("Analyze the request (%s), inspect the workspace if needed, execute the task, verify the observable outcome, and summarize the result.", state.ShortenTitle(trimmed))
	}
	return summary, []plannedStep{
		{Title: "Analyze the request", Kind: "analyze"},
		{Title: "Inspect relevant files or workspace context", Kind: "inspect"},
		{Title: "Execute the requested work", Kind: "execute"},
		{Title: "Verify the requested outcome with observable evidence", Kind: "verify"},
		{Title: "Summarize the final result", Kind: "summarize"},
	}
}

func normalizeStepKind(kind string) string {
	kind = strings.TrimSpace(strings.ToLower(kind))
	switch kind {
	case "analyze", "inspect", "execute", "verify", "summarize":
		return kind
	default:
		return "execute"
	}
}

func ensureRequiredPlanSteps(steps []plannedStep) []plannedStep {
	result := append([]plannedStep(nil), steps...)
	if !planHasKind(result, "verify") {
		verifyStep := plannedStep{Title: "Verify the requested outcome with observable evidence", Kind: "verify"}
		if len(result) > 0 && result[len(result)-1].Kind == "summarize" {
			result = append(result[:len(result)-1], append([]plannedStep{verifyStep}, result[len(result)-1:]...)...)
		} else {
			result = append(result, verifyStep)
		}
	}
	if !planHasKind(result, "summarize") {
		result = append(result, plannedStep{Title: "Summarize the final result", Kind: "summarize"})
	}
	return result
}

func planHasKind(steps []plannedStep, kind string) bool {
	kind = normalizeStepKind(kind)
	for _, step := range steps {
		if normalizeStepKind(step.Kind) == kind {
			return true
		}
	}
	return false
}

func extractJSON(input string) string {
	input = strings.TrimSpace(input)
	if strings.HasPrefix(input, "```") {
		parts := strings.Split(input, "```")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "json") {
				part = strings.TrimSpace(strings.TrimPrefix(part, "json"))
			}
			if strings.HasPrefix(part, "{") {
				return part
			}
		}
	}
	return input
}
