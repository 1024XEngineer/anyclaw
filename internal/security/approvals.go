package security

import "time"

// ApprovalRequest represents a pending human approval for a sensitive action.
type ApprovalRequest struct {
	ID          string
	RequestedBy string
	Action      string
	CreatedAt   time.Time
}
