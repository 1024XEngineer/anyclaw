package runtime

import (
	"github.com/1024XEngineer/anyclaw/pkg/capability/agents/prompt"
	appstate "github.com/1024XEngineer/anyclaw/pkg/runtime/execution/desktop"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

func ToPromptMessages(history []state.HistoryMessage) []prompt.Message {
	if len(history) == 0 {
		return nil
	}
	items := make([]prompt.Message, 0, len(history))
	for _, message := range history {
		items = append(items, prompt.Message{
			Role:    message.Role,
			Content: message.Content,
		})
	}
	return items
}

func FromPromptMessages(history []prompt.Message) []state.HistoryMessage {
	if len(history) == 0 {
		return nil
	}
	items := make([]state.HistoryMessage, 0, len(history))
	for _, message := range history {
		items = append(items, state.HistoryMessage{
			Role:    message.Role,
			Content: message.Content,
		})
	}
	return items
}

func ToRuntimeDesktopPlanState(plan *state.DesktopPlanExecutionState) *appstate.DesktopPlanExecutionState {
	if plan == nil {
		return nil
	}
	steps := make([]appstate.DesktopPlanStepExecutionState, 0, len(plan.Steps))
	for _, step := range plan.Steps {
		steps = append(steps, appstate.DesktopPlanStepExecutionState{
			Index:     step.Index,
			Tool:      step.Tool,
			Label:     step.Label,
			HasVerify: step.HasVerify,
			Verified:  step.Verified,
			Status:    step.Status,
			Attempts:  step.Attempts,
			Output:    step.Output,
			Error:     step.Error,
			UpdatedAt: step.UpdatedAt,
		})
	}
	return &appstate.DesktopPlanExecutionState{
		ToolName:          plan.ToolName,
		Plugin:            plan.Plugin,
		App:               plan.App,
		Action:            plan.Action,
		Workflow:          plan.Workflow,
		Status:            plan.Status,
		Summary:           plan.Summary,
		Result:            plan.Result,
		TotalSteps:        plan.TotalSteps,
		CurrentStep:       plan.CurrentStep,
		NextStep:          plan.NextStep,
		LastCompletedStep: plan.LastCompletedStep,
		LastOutput:        plan.LastOutput,
		LastError:         plan.LastError,
		Resumed:           plan.Resumed,
		Steps:             steps,
		UpdatedAt:         plan.UpdatedAt,
	}
}

func FromRuntimeDesktopPlanState(plan *appstate.DesktopPlanExecutionState) *state.DesktopPlanExecutionState {
	if plan == nil {
		return nil
	}
	steps := make([]state.DesktopPlanStepExecutionState, 0, len(plan.Steps))
	for _, step := range plan.Steps {
		steps = append(steps, state.DesktopPlanStepExecutionState{
			Index:     step.Index,
			Tool:      step.Tool,
			Label:     step.Label,
			HasVerify: step.HasVerify,
			Verified:  step.Verified,
			Status:    step.Status,
			Attempts:  step.Attempts,
			Output:    step.Output,
			Error:     step.Error,
			UpdatedAt: step.UpdatedAt,
		})
	}
	return &state.DesktopPlanExecutionState{
		ToolName:          plan.ToolName,
		Plugin:            plan.Plugin,
		App:               plan.App,
		Action:            plan.Action,
		Workflow:          plan.Workflow,
		Status:            plan.Status,
		Summary:           plan.Summary,
		Result:            plan.Result,
		TotalSteps:        plan.TotalSteps,
		CurrentStep:       plan.CurrentStep,
		NextStep:          plan.NextStep,
		LastCompletedStep: plan.LastCompletedStep,
		LastOutput:        plan.LastOutput,
		LastError:         plan.LastError,
		Resumed:           plan.Resumed,
		Steps:             steps,
		UpdatedAt:         plan.UpdatedAt,
	}
}
