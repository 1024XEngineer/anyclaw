package taskrunner

import (
	"context"
	"fmt"
	"strings"

	agent "github.com/1024XEngineer/anyclaw/pkg/capability/agents"
	"github.com/1024XEngineer/anyclaw/pkg/capability/tools"
	"github.com/1024XEngineer/anyclaw/pkg/config"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

func (m *Manager) executionMode(task *state.Task) taskExecutionMode {
	mode := taskExecutionMode{}
	if approval := m.findExecutionApproval(task.ID); approval != nil && approval.Status == "approved" {
		mode.PendingApprovalID = approval.ID
	}
	if task != nil && strings.Contains(strings.ToLower(task.PlanSummary), "inspect") {
		mode.StrictSteps = true
	}
	return mode
}

func (m *Manager) awaitApprovalsIfNeeded(task *state.Task, session *state.Session, cfg *config.Config, stepIndex int) error {
	if m.approvals == nil {
		return nil
	}
	if cfg == nil || !cfg.Agent.RequireConfirmationForDangerous {
		return nil
	}
	if stepIndex <= 0 {
		stepIndex = 2
	}
	if existing := m.findExecutionApproval(task.ID); existing != nil {
		switch existing.Status {
		case "approved":
			task.Status = "running"
			m.appendTaskEvidenceNoSave(task, state.TaskEvidence{
				Kind:      "approval_granted",
				Summary:   "Execution approval was granted.",
				StepIndex: stepIndex,
				Status:    task.Status,
				Source:    "approval",
				Data: map[string]any{
					"approval_id": existing.ID,
				},
			})
			m.setTaskRecoveryPointNoSave(task, &state.TaskRecoveryPoint{
				Kind:      "execution",
				Summary:   "Approval granted. Task execution can continue.",
				StepIndex: stepIndex,
				Status:    task.Status,
				SessionID: session.ID,
				Data: map[string]any{
					"approval_id": existing.ID,
				},
			})
			_ = m.persistTask(task)
			_ = m.setStepStatus(task.ID, stepIndex, "running", task.Input, "Approval granted. Executing planned work.", "")
			m.updateSessionPresence(session.ID, "typing", true)
			return nil
		case "rejected":
			return fmt.Errorf("task execution rejected by approver")
		case "pending":
			task.Status = "waiting_approval"
			m.appendTaskEvidenceNoSave(task, state.TaskEvidence{
				Kind:      "approval_waiting",
				Summary:   "Task is waiting for execution approval.",
				StepIndex: stepIndex,
				Status:    task.Status,
				Source:    "approval",
				Data: map[string]any{
					"approval_id": existing.ID,
					"scope":       "task_execution",
				},
			})
			m.setTaskRecoveryPointNoSave(task, &state.TaskRecoveryPoint{
				Kind:      "approval",
				Summary:   "Awaiting approval before executing the task.",
				StepIndex: stepIndex,
				Status:    task.Status,
				SessionID: session.ID,
				Data: map[string]any{
					"approval_id": existing.ID,
					"scope":       "task_execution",
				},
			})
			_ = m.persistTask(task)
			_ = m.setStepStatus(task.ID, stepIndex, "waiting_approval", task.Input, "Awaiting approval before executing planned work.", "")
			m.updateSessionPresence(session.ID, "waiting_approval", false)
			return ErrTaskWaitingApproval
		}
	}
	payload := map[string]any{
		"task_title": task.Title,
		"input":      task.Input,
		"workspace":  task.Workspace,
		"assistant":  task.Assistant,
		"scope":      "task_execution",
	}
	approval, err := m.approvals.Request(task.ID, session.ID, stepIndex, "task_execution", "execute_task", payload)
	if err != nil {
		return err
	}
	task.Status = "waiting_approval"
	m.appendTaskEvidenceNoSave(task, state.TaskEvidence{
		Kind:      "approval_waiting",
		Summary:   "Task is waiting for execution approval.",
		StepIndex: stepIndex,
		Status:    task.Status,
		Source:    "approval",
		Data: map[string]any{
			"approval_id": approval.ID,
			"scope":       "task_execution",
		},
	})
	m.setTaskRecoveryPointNoSave(task, &state.TaskRecoveryPoint{
		Kind:      "approval",
		Summary:   "Awaiting approval before executing the task.",
		StepIndex: stepIndex,
		Status:    task.Status,
		SessionID: session.ID,
		Data: map[string]any{
			"approval_id": approval.ID,
			"scope":       "task_execution",
		},
	})
	if err := m.persistTask(task); err != nil {
		return err
	}
	_ = m.setStepStatus(task.ID, stepIndex, "waiting_approval", task.Input, "Awaiting approval before executing planned work.", "")
	m.updateSessionPresence(session.ID, "waiting_approval", false)
	return ErrTaskWaitingApproval
}

func (m *Manager) toolApprovalHook(task *state.Task, session *state.Session, cfg *config.Config) agent.ToolApprovalHook {
	if m.approvals == nil || cfg == nil || !cfg.Agent.RequireConfirmationForDangerous {
		return nil
	}
	return func(ctx context.Context, tc agent.ToolCall) error {
		return m.requireToolApproval(task, session, cfg, tc.Name, tc.Args)
	}
}

func (m *Manager) protocolApprovalHook(task *state.Task, session *state.Session, cfg *config.Config) tools.ToolApprovalHook {
	if m.approvals == nil || cfg == nil || !cfg.Agent.RequireConfirmationForDangerous {
		return nil
	}
	return func(ctx context.Context, call tools.ToolApprovalCall) error {
		return m.requireToolApproval(task, session, cfg, call.Name, call.Args)
	}
}

func (m *Manager) requireToolApproval(task *state.Task, session *state.Session, cfg *config.Config, toolName string, args map[string]any) error {
	if !RequiresToolApprovalName(toolName) {
		return nil
	}
	stepIndex := firstNonZero(m.executionStageIndexes(task.ID).execute, 3)
	signature := approvalSignature(toolName, "tool_call", args)
	for _, approval := range m.store.ListTaskApprovals(task.ID) {
		if approval.Signature != signature || approval.ToolName != toolName || approval.Action != "tool_call" {
			continue
		}
		switch approval.Status {
		case "approved":
			return nil
		case "rejected":
			return fmt.Errorf("tool call rejected: %s", toolName)
		case "pending":
			task.Status = "waiting_approval"
			m.appendTaskEvidenceNoSave(task, state.TaskEvidence{
				Kind:      "approval_waiting",
				Summary:   fmt.Sprintf("Task is waiting for approval to call %s.", toolName),
				StepIndex: stepIndex,
				Status:    task.Status,
				ToolName:  toolName,
				Source:    "approval",
				Data: map[string]any{
					"approval_id": approval.ID,
					"args":        cloneAnyMap(args),
				},
			})
			m.setTaskRecoveryPointNoSave(task, &state.TaskRecoveryPoint{
				Kind:      "approval",
				Summary:   fmt.Sprintf("Awaiting approval for tool %s.", toolName),
				StepIndex: stepIndex,
				Status:    task.Status,
				SessionID: session.ID,
				ToolName:  toolName,
				Data: map[string]any{
					"approval_id": approval.ID,
					"args":        cloneAnyMap(args),
				},
			})
			_ = m.persistTask(task)
			_ = m.setStepStatus(task.ID, stepIndex, "waiting_approval", "", fmt.Sprintf("Awaiting approval for tool %s.", toolName), "")
			m.updateSessionPresence(session.ID, "waiting_approval", false)
			return ErrTaskWaitingApproval
		}
	}
	payload := map[string]any{
		"tool_name": toolName,
		"args":      args,
		"task_id":   task.ID,
		"workspace": task.Workspace,
	}
	approval, err := m.approvals.Request(task.ID, session.ID, stepIndex, toolName, "tool_call", payload)
	if err != nil {
		return err
	}
	task.Status = "waiting_approval"
	m.appendTaskEvidenceNoSave(task, state.TaskEvidence{
		Kind:      "approval_waiting",
		Summary:   fmt.Sprintf("Task is waiting for approval to call %s.", toolName),
		StepIndex: stepIndex,
		Status:    task.Status,
		ToolName:  toolName,
		Source:    "approval",
		Data: map[string]any{
			"approval_id": approval.ID,
			"args":        cloneAnyMap(args),
		},
	})
	m.setTaskRecoveryPointNoSave(task, &state.TaskRecoveryPoint{
		Kind:      "approval",
		Summary:   fmt.Sprintf("Awaiting approval for tool %s.", toolName),
		StepIndex: stepIndex,
		Status:    task.Status,
		SessionID: session.ID,
		ToolName:  toolName,
		Data: map[string]any{
			"approval_id": approval.ID,
			"args":        cloneAnyMap(args),
		},
	})
	_ = m.persistTask(task)
	_ = m.setStepStatus(task.ID, stepIndex, "waiting_approval", "", fmt.Sprintf("Awaiting approval for tool %s.", toolName), "")
	m.updateSessionPresence(session.ID, "waiting_approval", false)
	return ErrTaskWaitingApproval
}

func requiresToolApproval(tc agent.ToolCall) bool {
	return RequiresToolApprovalName(tc.Name)
}

func RequiresToolApprovalName(name string) bool {
	name = strings.TrimSpace(strings.ToLower(name))
	switch name {
	case "run_command", "write_file", "browser_upload", "desktop_open", "desktop_type", "desktop_type_human", "desktop_hotkey", "desktop_clipboard_set", "desktop_clipboard_get", "desktop_paste", "desktop_click", "desktop_screenshot", "desktop_screenshot_window", "desktop_move", "desktop_double_click", "desktop_scroll", "desktop_drag", "desktop_wait", "desktop_list_windows", "desktop_wait_window", "desktop_focus_window", "desktop_inspect_ui", "desktop_invoke_ui", "desktop_set_value_ui", "desktop_resolve_target", "desktop_activate_target", "desktop_set_target_value", "desktop_match_image", "desktop_click_image", "desktop_wait_image", "desktop_ocr", "desktop_verify_text", "desktop_find_text", "desktop_click_text", "desktop_wait_text", "desktop_plan":
		return true
	default:
		return false
	}
}

func (m *Manager) findExecutionApproval(taskID string) *state.Approval {
	approvals := m.store.ListApprovals("")
	for _, approval := range approvals {
		if approval.TaskID == taskID && approval.ToolName == "task_execution" && approval.Action == "execute_task" {
			return approval
		}
	}
	return nil
}
