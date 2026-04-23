package taskrunner

import (
	agent "github.com/1024XEngineer/anyclaw/pkg/capability/agents"
	appruntime "github.com/1024XEngineer/anyclaw/pkg/runtime"
	appstate "github.com/1024XEngineer/anyclaw/pkg/runtime/execution/desktop"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

func (m *Manager) SetPlanner(planner Planner) {
	if m == nil {
		return
	}
	m.planner = planner
}

func (m *Manager) SetStepStatus(taskID string, index int, status string, input string, output string, stepErr string) error {
	if m == nil {
		return nil
	}
	return m.setStepStatus(taskID, index, status, input, output, stepErr)
}

func (m *Manager) DesktopPlanStateHook(task *state.Task) appstate.DesktopPlanStateHook {
	if m == nil {
		return nil
	}
	return m.desktopPlanStateHook(task)
}

func DefaultPlan(input string) (string, []PlanStep) {
	return defaultPlan(input)
}

func RequiresToolApproval(tc agent.ToolCall) bool {
	return requiresToolApproval(tc)
}

func DesktopPlanHasExplicitVerification(state *appstate.DesktopPlanExecutionState) bool {
	return desktopPlanHasExplicitVerification(appruntime.FromRuntimeDesktopPlanState(state))
}
