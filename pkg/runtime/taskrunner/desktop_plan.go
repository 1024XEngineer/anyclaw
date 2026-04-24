package taskrunner

import (
	"context"
	"fmt"
	"strings"

	appruntime "github.com/1024XEngineer/anyclaw/pkg/runtime"
	appstate "github.com/1024XEngineer/anyclaw/pkg/runtime/execution/desktop"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

func (m *Manager) desktopPlanStateHook(task *state.Task) appstate.DesktopPlanStateHook {
	if m == nil || task == nil {
		return nil
	}
	return func(ctx context.Context, planState appstate.DesktopPlanExecutionState) {
		freshTask, ok := m.store.GetTask(task.ID)
		if ok && freshTask != nil {
			task = freshTask
		}
		if task.ExecutionState == nil {
			task.ExecutionState = &state.TaskExecutionState{}
		}
		currentPlanState := appruntime.FromRuntimeDesktopPlanState(&planState)
		previous := task.ExecutionState.DesktopPlan
		task.ExecutionState.DesktopPlan = state.CloneDesktopPlanExecutionState(currentPlanState)
		stage := m.executionStageIndexes(task.ID)
		executeIndex := firstNonZero(stage.execute, 3)
		if shouldRecordDesktopPlanCheckpoint(previous, currentPlanState) {
			m.appendTaskEvidenceNoSave(task, state.TaskEvidence{
				Kind:      "desktop_checkpoint",
				Summary:   "Saved desktop workflow checkpoint.",
				Detail:    desktopPlanCheckpointDetail(currentPlanState),
				StepIndex: executeIndex,
				Status:    firstNonEmpty(currentPlanState.Status, task.Status),
				ToolName:  currentPlanState.ToolName,
				Source:    "desktop_plan",
				Data: map[string]any{
					"current_step":        currentPlanState.CurrentStep,
					"next_step":           currentPlanState.NextStep,
					"last_completed_step": currentPlanState.LastCompletedStep,
					"total_steps":         currentPlanState.TotalSteps,
				},
			})
		}
		switch strings.ToLower(strings.TrimSpace(currentPlanState.Status)) {
		case "pending_approval", "waiting_approval":
			_ = m.setStepStatus(task.ID, executeIndex, "waiting_approval", "", desktopPlanCheckpointDetail(currentPlanState), "")
		case "running", "resuming":
			_ = m.setStepStatus(task.ID, executeIndex, "running", "", desktopPlanCheckpointDetail(currentPlanState), "")
		case "completed":
			_ = m.setStepStatus(task.ID, executeIndex, "completed", "", executionStageOutput(task, nil), "")
			if stage.verify > 0 && desktopPlanHasExplicitVerification(currentPlanState) {
				_ = m.setStepStatus(task.ID, stage.verify, "completed", "", verificationOutputFromDesktopPlan(currentPlanState), "")
			}
		case "failed", "interrupted":
			_ = m.setStepStatus(task.ID, executeIndex, "failed", "", "", firstNonEmpty(currentPlanState.LastError, desktopPlanCheckpointDetail(currentPlanState)))
		}
		m.setTaskRecoveryPointNoSave(task, desktopPlanRecoveryPoint(task, currentPlanState))
		_ = m.persistTask(task)
	}
}

func desktopPlanRecoveryPoint(task *state.Task, planState *state.DesktopPlanExecutionState) *state.TaskRecoveryPoint {
	if planState == nil {
		return nil
	}
	return &state.TaskRecoveryPoint{
		Kind:      "desktop_plan",
		Summary:   "Resume from the saved desktop workflow checkpoint.",
		StepIndex: 3,
		Status:    firstNonEmpty(planState.Status, taskStatus(task)),
		SessionID: taskSessionID(task),
		ToolName:  planState.ToolName,
		Data: map[string]any{
			"current_step":        planState.CurrentStep,
			"next_step":           planState.NextStep,
			"last_completed_step": planState.LastCompletedStep,
			"total_steps":         planState.TotalSteps,
			"workflow":            planState.Workflow,
			"action":              planState.Action,
		},
	}
}

func shouldRecordDesktopPlanCheckpoint(previous *state.DesktopPlanExecutionState, current *state.DesktopPlanExecutionState) bool {
	if current == nil {
		return false
	}
	if previous == nil {
		return true
	}
	return previous.Status != current.Status ||
		previous.CurrentStep != current.CurrentStep ||
		previous.NextStep != current.NextStep ||
		previous.LastCompletedStep != current.LastCompletedStep ||
		previous.LastError != current.LastError ||
		previous.LastOutput != current.LastOutput
}

func desktopPlanCheckpointDetail(state *state.DesktopPlanExecutionState) string {
	if state == nil {
		return ""
	}
	parts := []string{}
	if state.ToolName != "" {
		parts = append(parts, "tool="+state.ToolName)
	}
	if state.Status != "" {
		parts = append(parts, "status="+state.Status)
	}
	if state.CurrentStep > 0 || state.TotalSteps > 0 {
		parts = append(parts, fmt.Sprintf("step=%d/%d", state.CurrentStep, state.TotalSteps))
	}
	if state.NextStep > 0 {
		parts = append(parts, fmt.Sprintf("next=%d", state.NextStep))
	}
	if state.LastCompletedStep > 0 {
		parts = append(parts, fmt.Sprintf("last_completed=%d", state.LastCompletedStep))
	}
	totalVerifies := 0
	passedVerifies := 0
	for _, step := range state.Steps {
		if step.HasVerify {
			totalVerifies++
		}
		if step.Verified {
			passedVerifies++
		}
	}
	if totalVerifies > 0 {
		parts = append(parts, fmt.Sprintf("verified=%d/%d", passedVerifies, totalVerifies))
	}
	if text := firstNonEmpty(state.Summary, state.Result, state.LastOutput, state.LastError); strings.TrimSpace(text) != "" {
		parts = append(parts, limitTaskText(text, 240))
	}
	return strings.Join(parts, " | ")
}

func recoveryToolName(task *state.Task) string {
	if task == nil || task.ExecutionState == nil || task.ExecutionState.DesktopPlan == nil {
		return ""
	}
	return task.ExecutionState.DesktopPlan.ToolName
}

func taskSessionID(task *state.Task) string {
	if task == nil {
		return ""
	}
	return task.SessionID
}

func taskStatus(task *state.Task) string {
	if task == nil {
		return ""
	}
	return task.Status
}

func limitTaskText(input string, max int) string {
	trimmed := strings.TrimSpace(input)
	if max <= 0 || len(trimmed) <= max {
		return trimmed
	}
	if max <= 3 {
		return trimmed[:max]
	}
	return trimmed[:max-3] + "..."
}
