package state

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type ApprovalManager struct {
	store   *Store
	nextID  func() string
	nowFunc func() time.Time
}

func NewApprovalManager(store *Store) *ApprovalManager {
	return &ApprovalManager{
		store: store,
		nextID: func() string {
			return uniqueID("approval")
		},
		nowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func (m *ApprovalManager) Request(taskID string, sessionID string, stepIndex int, toolName string, action string, payload map[string]any) (*Approval, error) {
	return m.RequestWithSignature(taskID, sessionID, stepIndex, toolName, action, payload, nil)
}

func (m *ApprovalManager) RequestWithSignature(taskID string, sessionID string, stepIndex int, toolName string, action string, payload map[string]any, signaturePayload map[string]any) (*Approval, error) {
	now := m.nowFunc()
	signatureSource := payload
	if signaturePayload != nil {
		signatureSource = signaturePayload
	}
	signature := approvalSignature(toolName, action, signatureSource)
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

func (m *ApprovalManager) Resolve(id string, approved bool, actor string, comment string) (*Approval, error) {
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
