package taskrunner

import (
	"fmt"
	"strings"

	agent "github.com/1024XEngineer/anyclaw/pkg/capability/agents"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

func executionStageOutput(task *state.Task, activities []agent.ToolActivity) string {
	if task != nil && task.ExecutionState != nil && task.ExecutionState.DesktopPlan != nil {
		state := task.ExecutionState.DesktopPlan
		if result := strings.TrimSpace(state.Result); result != "" {
			return result
		}
		return desktopPlanCheckpointDetail(state)
	}
	if len(activities) > 0 {
		names := make([]string, 0, len(activities))
		for _, activity := range activities {
			if name := strings.TrimSpace(activity.ToolName); name != "" {
				names = append(names, name)
			}
		}
		if len(names) > 0 {
			return "Execution completed with tools: " + strings.Join(uniqueTaskStrings(names), ", ")
		}
	}
	return "Execution completed using the current runtime."
}

func verificationStageOutput(task *state.Task, activities []agent.ToolActivity) (string, bool) {
	if task != nil && task.ExecutionState != nil && task.ExecutionState.DesktopPlan != nil {
		state := task.ExecutionState.DesktopPlan
		if desktopPlanHasExplicitVerification(state) {
			return verificationOutputFromDesktopPlan(state), true
		}
	}
	toolsUsed := observedVerificationTools(activities)
	if len(toolsUsed) > 0 {
		return "Observed verification tool calls: " + strings.Join(toolsUsed, ", "), true
	}
	return "No explicit verification tool was observed; completion relies on recorded tool outputs and the final result.", false
}

func verificationOutputFromDesktopPlan(state *state.DesktopPlanExecutionState) string {
	if state == nil {
		return ""
	}
	total := 0
	passed := 0
	for _, step := range state.Steps {
		if step.HasVerify {
			total++
		}
		if step.Verified {
			passed++
		}
	}
	if total == 0 {
		return "Desktop plan completed without explicit verification steps."
	}
	return fmt.Sprintf("Desktop plan verified %d of %d step(s).", passed, total)
}

func desktopPlanHasExplicitVerification(state *state.DesktopPlanExecutionState) bool {
	if state == nil {
		return false
	}
	for _, step := range state.Steps {
		if step.Verified {
			return true
		}
	}
	return false
}

func observedVerificationTools(activities []agent.ToolActivity) []string {
	if len(activities) == 0 {
		return nil
	}
	items := make([]string, 0, len(activities))
	for _, activity := range activities {
		switch strings.TrimSpace(strings.ToLower(activity.ToolName)) {
		case "desktop_verify_text", "desktop_wait_text", "desktop_find_text", "desktop_resolve_target", "desktop_ocr", "read_file", "browser_snapshot", "browser_screenshot":
			items = append(items, activity.ToolName)
		}
	}
	return uniqueTaskStrings(items)
}
