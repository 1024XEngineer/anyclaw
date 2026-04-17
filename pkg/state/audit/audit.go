package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Event struct {
	Time      string         `json:"time"`
	AgentName string         `json:"agent_name,omitempty"`
	Action    string         `json:"action"`
	Input     map[string]any `json:"input,omitempty"`
	Output    string         `json:"output,omitempty"`
	Error     string         `json:"error,omitempty"`
}

type Logger struct {
	path      string
	agentName string
	mu        sync.Mutex
}

func New(path string, agentName string) *Logger {
	return &Logger{path: path, agentName: agentName}
}

func (l *Logger) LogTool(toolName string, input map[string]any, output string, err error) {
	event := Event{
		Time:      time.Now().Format(time.RFC3339),
		AgentName: l.agentName,
		Action:    toolName,
		Input:     cloneMap(input),
		Output:    output,
	}
	if err != nil {
		event.Error = err.Error()
	}
	_ = l.Append(event)
}

func (l *Logger) LogApprovalRequest(approval *Approval) error {
	return l.logApproval("approval_request", approval)
}

func (l *Logger) LogApprovalDecision(approval *Approval) error {
	return l.logApproval("approval_decision", approval)
}

func (l *Logger) LogApprovalCancelled(approvalID string) error {
	return l.logSimple("approval_cancelled", map[string]any{"approval_id": approvalID})
}

func (l *Logger) LogApprovalExpired(approvalID string) error {
	return l.logSimple("approval_expired", map[string]any{"approval_id": approvalID})
}

func (l *Logger) LogBatchApproval(batchID string, count int) error {
	return l.logSimple("batch_approval", map[string]any{"batch_id": batchID, "count": count})
}

func (l *Logger) LogSecurityAssessment(result SecurityAssessmentResult) error {
	return l.logSimple("security_assessment", map[string]any{
		"tool_name":      result.ToolName,
		"risk_level":     result.RiskLevel,
		"recommendation": result.Recommendation,
	})
}

func (l *Logger) LogToolCheck(toolName string, result ToolCheckResult) error {
	return l.logSimple("tool_check", map[string]any{
		"tool_name": toolName,
		"approved":  result.Approved,
		"reason":    result.Reason,
	})
}

func (l *Logger) LogPathCheck(path string, result PathCheckResult) error {
	return l.logSimple("path_check", map[string]any{
		"path":      path,
		"protected": result.Protected,
		"reason":    result.Reason,
	})
}

func (l *Logger) logApproval(action string, approval *Approval) error {
	event := Event{
		Time:      time.Now().Format(time.RFC3339),
		AgentName: l.agentName,
		Action:    action,
		Input: map[string]any{
			"id":         approval.ID,
			"task_id":    approval.TaskID,
			"session_id": approval.SessionID,
			"user_id":    approval.UserID,
			"scope":      approval.Scope,
			"category":   approval.Category,
			"action":     approval.Action,
			"tool_name":  approval.ToolName,
			"plugin":     approval.Plugin,
			"workflow":   approval.Workflow,
			"status":     string(approval.Status),
			"risk_level": approval.Request.RiskLevel,
		},
	}
	return l.Append(event)
}

func (l *Logger) logSimple(action string, data map[string]any) error {
	event := Event{
		Time:      time.Now().Format(time.RFC3339),
		AgentName: l.agentName,
		Action:    action,
		Input:     data,
	}
	return l.Append(event)
}

func (l *Logger) Append(event Event) error {
	if l == nil || l.path == "" {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(f, string(data)); err != nil {
		return err
	}
	return nil
}

func (l *Logger) Tail(limit int) ([]Event, error) {
	if limit <= 0 {
		limit = 20
	}
	data, err := os.ReadFile(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	lines := splitLines(string(data))
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	result := make([]Event, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		var event Event
		if err := json.Unmarshal([]byte(line), &event); err == nil {
			result = append(result, event)
		}
	}
	return result, nil
}

func cloneMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]any, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func splitLines(input string) []string {
	current := ""
	lines := []string{}
	for _, r := range input {
		if r == '\n' {
			lines = append(lines, current)
			current = ""
			continue
		}
		if r != '\r' {
			current += string(r)
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}
