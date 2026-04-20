package audit

import (
	"time"
)

// AuditLogger 审计日志接口
type AuditLogger interface {
	LogTool(toolName string, input map[string]any, output string, err error)
	LogApprovalRequest(approval *Approval) error
	LogApprovalDecision(approval *Approval) error
	LogApprovalCancelled(approvalID string) error
	LogApprovalExpired(approvalID string) error
	LogBatchApproval(batchID string, count int) error
	LogSecurityAssessment(result SecurityAssessmentResult) error
	LogToolCheck(toolName string, result ToolCheckResult) error
	LogPathCheck(path string, result PathCheckResult) error
}

// Approval 审批记录 (简化版本，避免循环依赖)
type Approval struct {
	ID        string           `json:"id"`
	TaskID    string           `json:"task_id"`
	SessionID string           `json:"session_id"`
	UserID    string           `json:"user_id"`
	Scope     string           `json:"scope"`
	Category  string           `json:"category"`
	Action    string           `json:"action"`
	ToolName  string           `json:"tool_name,omitempty"`
	Plugin    string           `json:"plugin,omitempty"`
	Workflow  string           `json:"workflow,omitempty"`
	Request   ApprovalRequest  `json:"request"`
	Decision  ApprovalDecision `json:"decision"`
	Status    ApprovalStatus   `json:"status"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
	ExpiresAt time.Time        `json:"expires_at"`
	Metadata  map[string]any   `json:"metadata,omitempty"`
}

// ApprovalRequest 审批请求
type ApprovalRequest struct {
	Title       string         `json:"title"`
	Description string         `json:"description"`
	RiskLevel   string         `json:"risk_level"`
	RiskLabels  []string       `json:"risk_labels,omitempty"`
	Payload     map[string]any `json:"payload"`
	Evidence    []Evidence     `json:"evidence,omitempty"`
	Urgency     string         `json:"urgency"`
}

type Evidence struct {
	Type      string         `json:"type"`
	Content   string         `json:"content,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
}

// ApprovalDecision 审批决定
type ApprovalDecision struct {
	Decision   string    `json:"decision"`
	DecidedBy  string    `json:"decided_by"`
	DecidedAt  time.Time `json:"decided_at"`
	Notes      string    `json:"notes,omitempty"`
	Conditions []string  `json:"conditions,omitempty"`
}

// ApprovalStatus 审批状态
type ApprovalStatus string

const (
	ApprovalStatusPending   ApprovalStatus = "pending"
	ApprovalStatusApproved  ApprovalStatus = "approved"
	ApprovalStatusRejected  ApprovalStatus = "rejected"
	ApprovalStatusExpired   ApprovalStatus = "expired"
	ApprovalStatusCancelled ApprovalStatus = "cancelled"
)

// SecurityAssessmentResult 安全检查结果
type SecurityAssessmentResult struct {
	ToolName       string    `json:"tool_name"`
	RiskLevel      string    `json:"risk_level"`
	Recommendation string    `json:"recommendation"`
	Timestamp      time.Time `json:"timestamp"`
}

// ToolCheckResult 工具检查结果
type ToolCheckResult struct {
	ToolName  string    `json:"tool_name"`
	Approved  bool      `json:"approved"`
	Reason    string    `json:"reason"`
	Timestamp time.Time `json:"timestamp"`
}

// PathCheckResult 路径检查结果
type PathCheckResult struct {
	Path      string    `json:"path"`
	Protected bool      `json:"protected"`
	Reason    string    `json:"reason"`
	Timestamp time.Time `json:"timestamp"`
}
