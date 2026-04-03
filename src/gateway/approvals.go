package gateway

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type approvalManager struct {
	store   *Store
	nextID  func() string
	nowFunc func() time.Time
}

func newApprovalManager(store *Store) *approvalManager {
	return &approvalManager{
		store: store,
		nextID: func() string {
			return uniqueID("approval")
		},
		nowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func (m *approvalManager) Request(taskID string, sessionID string, stepIndex int, toolName string, action string, payload map[string]any) (*Approval, error) {
	now := m.nowFunc()
	signature := approvalSignature(toolName, action, payload)
	approval := &Approval{
		ID:          m.nextID(),
		TaskID:      strings.TrimSpace(taskID),
		SessionID:   strings.TrimSpace(sessionID),
		StepIndex:   stepIndex,
		ToolName:    strings.TrimSpace(toolName),
		Action:      strings.TrimSpace(action),
		Payload:     cloneAnyMap(payload),
		Signature:   signature,
		Status:      "pending",
		RequestedAt: now,
	}
	if err := m.store.AppendApproval(approval); err != nil {
		return nil, err
	}
	return approval, nil
}

func (m *approvalManager) Resolve(id string, approved bool, actor string, comment string) (*Approval, error) {
	approval, ok := m.store.GetApproval(id)
	if !ok {
		return nil, fmt.Errorf("approval not found: %s", id)
	}
	if approval.Status != "pending" {
		return approval, nil
	}
	approval.Status = "rejected"
	if approved {
		approval.Status = "approved"
	}
	approval.ResolvedAt = m.nowFunc().Format(time.RFC3339)
	approval.ResolvedBy = strings.TrimSpace(actor)
	approval.Comment = strings.TrimSpace(comment)
	if err := m.store.UpdateApproval(approval); err != nil {
		return nil, err
	}
	return approval, nil
}

func approvalSignature(toolName string, action string, payload map[string]any) string {
	encoded, _ := json.Marshal(payload)
	return fmt.Sprintf("%s|%s|%s", strings.TrimSpace(toolName), strings.TrimSpace(action), string(encoded))
}

func cloneAnyMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	result := make(map[string]any, len(input))
	for k, v := range input {
		result[k] = v
	}
	return result
}
