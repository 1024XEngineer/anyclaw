package taskrunner

import (
	"fmt"
	"strings"

	agent "github.com/1024XEngineer/anyclaw/pkg/capability/agents"
	"github.com/1024XEngineer/anyclaw/pkg/state"
)

func (m *Manager) updateSessionPresence(sessionID string, presence string, typing bool) {
	if m.sessions == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	_, _ = m.sessions.SetPresence(sessionID, presence, typing)
}

func (m *Manager) persistTask(task *state.Task) error {
	if task == nil {
		return nil
	}
	task.LastUpdatedAt = m.nowFunc()
	return m.store.UpdateTask(task)
}

func (m *Manager) appendTaskEvidenceNoSave(task *state.Task, evidence state.TaskEvidence) {
	if task == nil {
		return
	}
	evidence.Kind = strings.TrimSpace(evidence.Kind)
	if evidence.Kind == "" {
		evidence.Kind = "note"
	}
	evidence.Summary = limitTaskText(evidence.Summary, 240)
	evidence.Detail = limitTaskText(evidence.Detail, 1200)
	if strings.TrimSpace(evidence.ID) == "" {
		evidence.ID = m.nextID("evidence")
	}
	if evidence.CreatedAt.IsZero() {
		evidence.CreatedAt = m.nowFunc()
	}
	evidence.Data = cloneAnyMap(evidence.Data)
	task.Evidence = append(task.Evidence, &evidence)
	if len(task.Evidence) > taskEvidenceLimit {
		task.Evidence = task.Evidence[len(task.Evidence)-taskEvidenceLimit:]
	}
}

func (m *Manager) setTaskRecoveryPointNoSave(task *state.Task, point *state.TaskRecoveryPoint) {
	if task == nil {
		return
	}
	if point == nil {
		task.RecoveryPoint = nil
		return
	}
	clone := state.CloneTaskRecoveryPoint(point)
	clone.Kind = strings.TrimSpace(clone.Kind)
	if clone.Kind == "" {
		clone.Kind = "task"
	}
	clone.Summary = limitTaskText(clone.Summary, 240)
	if clone.UpdatedAt.IsZero() {
		clone.UpdatedAt = m.nowFunc()
	}
	task.RecoveryPoint = clone
}

func (m *Manager) appendTaskArtifactNoSave(task *state.Task, artifact state.TaskArtifact) {
	if task == nil {
		return
	}
	artifact.Kind = strings.TrimSpace(artifact.Kind)
	if artifact.Kind == "" {
		artifact.Kind = "file"
	}
	artifact.Label = limitTaskText(strings.TrimSpace(artifact.Label), 160)
	artifact.Path = strings.TrimSpace(artifact.Path)
	artifact.Description = limitTaskText(strings.TrimSpace(artifact.Description), 320)
	if strings.TrimSpace(artifact.ID) == "" {
		artifact.ID = m.nextID("artifact")
	}
	if artifact.CreatedAt.IsZero() {
		artifact.CreatedAt = m.nowFunc()
	}
	artifact.Meta = cloneAnyMap(artifact.Meta)
	for _, existing := range task.Artifacts {
		if existing == nil {
			continue
		}
		if existing.Kind == artifact.Kind && existing.ToolName == artifact.ToolName && existing.Path == artifact.Path && existing.Label == artifact.Label {
			return
		}
	}
	task.Artifacts = append(task.Artifacts, &artifact)
	if len(task.Artifacts) > taskArtifactLimit {
		task.Artifacts = task.Artifacts[len(task.Artifacts)-taskArtifactLimit:]
	}
}

func (m *Manager) recordTaskToolActivitiesNoSave(task *state.Task, activities []agent.ToolActivity) {
	if task == nil {
		return
	}
	for _, activity := range activities {
		status := "completed"
		summary := fmt.Sprintf("Tool %s executed.", activity.ToolName)
		detail := limitTaskText(activity.Result, 800)
		if strings.TrimSpace(activity.Error) != "" {
			status = "failed"
			summary = fmt.Sprintf("Tool %s failed.", activity.ToolName)
			detail = limitTaskText(activity.Error, 800)
		}
		m.appendTaskEvidenceNoSave(task, state.TaskEvidence{
			Kind:      "tool_activity",
			Summary:   summary,
			Detail:    detail,
			StepIndex: 3,
			Status:    status,
			ToolName:  activity.ToolName,
			Source:    "agent",
			Data: map[string]any{
				"args": cloneAnyMap(activity.Args),
			},
		})
		for _, artifact := range inferTaskArtifacts(activity) {
			m.appendTaskArtifactNoSave(task, artifact)
		}
	}
}

func inferTaskArtifacts(activity agent.ToolActivity) []state.TaskArtifact {
	items := make([]state.TaskArtifact, 0)
	for key, value := range activity.Args {
		text, ok := value.(string)
		if !ok {
			continue
		}
		text = strings.TrimSpace(text)
		if text == "" || !looksLikeArtifactKey(key) {
			continue
		}
		items = append(items, state.TaskArtifact{
			Kind:        inferArtifactKind(activity.ToolName, key, text),
			Label:       fmt.Sprintf("%s:%s", activity.ToolName, key),
			Path:        text,
			ToolName:    activity.ToolName,
			Description: "Observed from tool arguments during task execution.",
			Meta: map[string]any{
				"arg": key,
			},
		})
	}
	return items
}

func looksLikeArtifactKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	return strings.Contains(key, "path") ||
		strings.Contains(key, "file") ||
		strings.Contains(key, "output") ||
		strings.Contains(key, "download") ||
		strings.Contains(key, "save") ||
		strings.Contains(key, "export") ||
		strings.Contains(key, "destination") ||
		strings.Contains(key, "screenshot")
}

func inferArtifactKind(toolName string, key string, value string) string {
	lowerTool := strings.ToLower(strings.TrimSpace(toolName))
	lowerKey := strings.ToLower(strings.TrimSpace(key))
	lowerValue := strings.ToLower(strings.TrimSpace(value))
	switch {
	case strings.Contains(lowerTool, "screenshot") || strings.Contains(lowerKey, "screenshot"):
		return "screenshot"
	case strings.Contains(lowerTool, "pdf") || strings.HasSuffix(lowerValue, ".pdf"):
		return "pdf"
	case strings.Contains(lowerTool, "download") || strings.Contains(lowerKey, "download"):
		return "download"
	default:
		return "file"
	}
}
