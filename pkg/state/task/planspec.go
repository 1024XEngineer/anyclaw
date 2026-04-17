package task

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// PlanSpec 统一的计划规范
type PlanSpec struct {
	ID               string          `json:"id"`
	TaskID           string          `json:"task_id"`
	Title            string          `json:"title"`
	Description      string          `json:"description,omitempty"`
	Mode             string          `json:"mode"`       // workflow|app-action|tool-chain|planner
	RiskLevel        string          `json:"risk_level"` // low|medium|high
	RiskLabels       []string        `json:"risk_labels,omitempty"`
	PrivacyScope     string          `json:"privacy_scope,omitempty"` // public|work|personal|system
	DataScope        string          `json:"data_scope,omitempty"`    // read|write|delete|execute
	RequiresApproval bool            `json:"requires_approval"`
	ApprovalScope    string          `json:"approval_scope,omitempty"` // none|tool|action|always
	Steps            []PlanStep      `json:"steps"`
	Evidence         []PlanEvidence  `json:"evidence,omitempty"`
	Artifacts        []PlanArtifact  `json:"artifacts,omitempty"`
	RecoveryPoints   []RecoveryPoint `json:"recovery_points,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
	TokenCost        int             `json:"token_cost,omitempty"`
	Confidence       float64         `json:"confidence,omitempty"`
	Explanation      string          `json:"explanation,omitempty"`
}

type WorkflowDescriptor struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Plugin      string `json:"plugin,omitempty"`
	Action      string `json:"action,omitempty"`
	ToolName    string `json:"tool_name,omitempty"`
	Pairing     string `json:"pairing,omitempty"`
}

// PlanStep 计划步骤
type PlanStep struct {
	Index            int               `json:"index"`
	Title            string            `json:"title"`
	Description      string            `json:"description,omitempty"`
	Kind             string            `json:"kind"` // tool|workflow|app-action|verification|rollback
	Plugin           string            `json:"plugin,omitempty"`
	Workflow         string            `json:"workflow,omitempty"`
	Action           string            `json:"action,omitempty"`
	ToolName         string            `json:"tool_name,omitempty"`
	Inputs           map[string]any    `json:"inputs,omitempty"`
	ExpectedOutput   map[string]any    `json:"expected_output,omitempty"`
	Verification     *VerificationSpec `json:"verification,omitempty"`
	Rollback         *RollbackSpec     `json:"rollback,omitempty"`
	RetryPolicy      *RetryPolicy      `json:"retry_policy,omitempty"`
	TimeoutSec       int               `json:"timeout_sec,omitempty"`
	RiskLevel        string            `json:"risk_level,omitempty"`
	RequiresApproval bool              `json:"requires_approval,omitempty"`
	Status           string            `json:"status"` // pending|running|completed|failed|skipped
	StartedAt        *time.Time        `json:"started_at,omitempty"`
	CompletedAt      *time.Time        `json:"completed_at,omitempty"`
	Error            string            `json:"error,omitempty"`
	Output           map[string]any    `json:"output,omitempty"`
}

// VerificationSpec 验证规范
type VerificationSpec struct {
	Type       string         `json:"type"` // file-exists|window-appears|text-contains|clipboard|network|app-state
	Parameters map[string]any `json:"parameters"`
	TimeoutSec int            `json:"timeout_sec,omitempty"`
	RetryCount int            `json:"retry_count,omitempty"`
	OnFailure  string         `json:"on_failure,omitempty"` // retry|rollback|fail|continue
}

// RollbackSpec 回滚规范
type RollbackSpec struct {
	Steps     []RollbackStep `json:"steps,omitempty"`
	OnFailure bool           `json:"on_failure"`
	OnTimeout bool           `json:"on_timeout"`
	OnCancel  bool           `json:"on_cancel"`
}

type RollbackStep struct {
	Action string         `json:"action"`
	Inputs map[string]any `json:"inputs"`
}

// RetryPolicy 重试策略
type RetryPolicy struct {
	MaxAttempts   int     `json:"max_attempts"`
	InitialDelay  int     `json:"initial_delay"` // 毫秒
	MaxDelay      int     `json:"max_delay"`     // 毫秒
	BackoffFactor float64 `json:"backoff_factor"`
}

// PlanEvidence 计划证据
type PlanEvidence struct {
	ID        string         `json:"id"`
	StepIndex int            `json:"step_index"`
	Type      string         `json:"type"` // screenshot|log|file|network|clipboard
	Content   string         `json:"content,omitempty"`
	Path      string         `json:"path,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

// PlanArtifact 计划工件
type PlanArtifact struct {
	ID        string         `json:"id"`
	StepIndex int            `json:"step_index"`
	Name      string         `json:"name"`
	Type      string         `json:"type"` // file|url|text|data
	Path      string         `json:"path,omitempty"`
	URL       string         `json:"url,omitempty"`
	Content   string         `json:"content,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

// RecoveryPoint 恢复点
type RecoveryPoint struct {
	ID        string         `json:"id"`
	StepIndex int            `json:"step_index"`
	Type      string         `json:"type"` // checkpoint|rollback|pause
	State     map[string]any `json:"state"`
	CreatedAt time.Time      `json:"created_at"`
}

// NewPlanSpec 创建新的计划规范
func NewPlanSpec(taskID, title, description string) *PlanSpec {
	now := time.Now().UTC()
	return &PlanSpec{
		ID:               generatePlanID(),
		TaskID:           taskID,
		Title:            title,
		Description:      description,
		Mode:             "planner", // 默认模式
		RiskLevel:        "medium",  // 默认风险等级
		RequiresApproval: true,      // 默认需要审批
		Steps:            make([]PlanStep, 0),
		Evidence:         make([]PlanEvidence, 0),
		Artifacts:        make([]PlanArtifact, 0),
		RecoveryPoints:   make([]RecoveryPoint, 0),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
}

// AddStep 添加步骤
func (p *PlanSpec) AddStep(step PlanStep) {
	step.Index = len(p.Steps) + 1
	if step.Status == "" {
		step.Status = "pending"
	}
	p.Steps = append(p.Steps, step)
	p.UpdatedAt = time.Now().UTC()
}

// AddWorkflowStep 添加工作流步骤
func (p *PlanSpec) AddWorkflowStep(workflow WorkflowDescriptor, inputs map[string]any) {
	step := PlanStep{
		Title:            workflow.Name,
		Description:      workflow.Description,
		Kind:             "workflow",
		Plugin:           workflow.Plugin,
		Workflow:         workflow.Name,
		Action:           workflow.Action,
		ToolName:         workflow.ToolName,
		Inputs:           inputs,
		RiskLevel:        "low",
		RequiresApproval: false, // workflow 默认不需要审批
		Status:           "pending",
	}

	if strings.TrimSpace(workflow.Pairing) != "" {
		step.Description = fmt.Sprintf("%s (paired: %s)", step.Description, workflow.Pairing)
	}

	p.AddStep(step)
}

// AddToolStep 添加工具步骤
func (p *PlanSpec) AddToolStep(toolName string, inputs map[string]any, requiresApproval bool) {
	step := PlanStep{
		Title:            fmt.Sprintf("Execute tool: %s", toolName),
		Kind:             "tool",
		ToolName:         toolName,
		Inputs:           inputs,
		RiskLevel:        assessToolRisk(toolName),
		RequiresApproval: requiresApproval,
		Status:           "pending",
	}
	p.AddStep(step)
}

// AddVerificationStep 添加验证步骤
func (p *PlanSpec) AddVerificationStep(stepIndex int, verification VerificationSpec) {
	step := PlanStep{
		Title:            fmt.Sprintf("Verify step %d", stepIndex),
		Description:      fmt.Sprintf("Verify completion of step %d", stepIndex),
		Kind:             "verification",
		Verification:     &verification,
		RiskLevel:        "low",
		RequiresApproval: false,
		Status:           "pending",
	}
	p.AddStep(step)
}

// AddRollbackStep 添加回滚步骤
func (p *PlanSpec) AddRollbackStep(stepIndex int, rollback RollbackSpec) {
	step := PlanStep{
		Title:            fmt.Sprintf("Rollback step %d", stepIndex),
		Description:      fmt.Sprintf("Rollback changes from step %d", stepIndex),
		Kind:             "rollback",
		Rollback:         &rollback,
		RiskLevel:        "low",
		RequiresApproval: false,
		Status:           "pending",
	}
	p.AddStep(step)
}

// MarkStepStarted 标记步骤开始
func (p *PlanSpec) MarkStepStarted(stepIndex int) error {
	if stepIndex < 1 || stepIndex > len(p.Steps) {
		return fmt.Errorf("step index out of range: %d", stepIndex)
	}

	now := time.Now().UTC()
	p.Steps[stepIndex-1].StartedAt = &now
	p.Steps[stepIndex-1].Status = "running"
	p.UpdatedAt = now

	return nil
}

// MarkStepCompleted 标记步骤完成
func (p *PlanSpec) MarkStepCompleted(stepIndex int, output map[string]any) error {
	if stepIndex < 1 || stepIndex > len(p.Steps) {
		return fmt.Errorf("step index out of range: %d", stepIndex)
	}

	now := time.Now().UTC()
	p.Steps[stepIndex-1].CompletedAt = &now
	p.Steps[stepIndex-1].Status = "completed"
	p.Steps[stepIndex-1].Output = output
	p.UpdatedAt = now

	return nil
}

// MarkStepFailed 标记步骤失败
func (p *PlanSpec) MarkStepFailed(stepIndex int, err error) error {
	if stepIndex < 1 || stepIndex > len(p.Steps) {
		return fmt.Errorf("step index out of range: %d", stepIndex)
	}

	now := time.Now().UTC()
	p.Steps[stepIndex-1].CompletedAt = &now
	p.Steps[stepIndex-1].Status = "failed"
	p.Steps[stepIndex-1].Error = err.Error()
	p.UpdatedAt = now

	return nil
}

// AddEvidence 添加证据
func (p *PlanSpec) AddEvidence(stepIndex int, evidenceType, content string, data map[string]any) {
	evidence := PlanEvidence{
		ID:        generateEvidenceID(),
		StepIndex: stepIndex,
		Type:      evidenceType,
		Content:   content,
		Data:      data,
		CreatedAt: time.Now().UTC(),
	}
	p.Evidence = append(p.Evidence, evidence)
	p.UpdatedAt = time.Now().UTC()
}

// AddArtifact 添加工件
func (p *PlanSpec) AddArtifact(stepIndex int, name, artifactType, path string, data map[string]any) {
	artifact := PlanArtifact{
		ID:        generateArtifactID(),
		StepIndex: stepIndex,
		Name:      name,
		Type:      artifactType,
		Path:      path,
		Data:      data,
		CreatedAt: time.Now().UTC(),
	}
	p.Artifacts = append(p.Artifacts, artifact)
	p.UpdatedAt = time.Now().UTC()
}

// AddRecoveryPoint 添加恢复点
func (p *PlanSpec) AddRecoveryPoint(stepIndex int, pointType string, state map[string]any) {
	point := RecoveryPoint{
		ID:        generateRecoveryPointID(),
		StepIndex: stepIndex,
		Type:      pointType,
		State:     state,
		CreatedAt: time.Now().UTC(),
	}
	p.RecoveryPoints = append(p.RecoveryPoints, point)
	p.UpdatedAt = time.Now().UTC()
}

// GetCurrentStep 获取当前步骤
func (p *PlanSpec) GetCurrentStep() *PlanStep {
	for i := range p.Steps {
		if p.Steps[i].Status == "pending" {
			return &p.Steps[i]
		}
	}
	return nil
}

// GetFailedSteps 获取失败的步骤
func (p *PlanSpec) GetFailedSteps() []PlanStep {
	var failed []PlanStep
	for _, step := range p.Steps {
		if step.Status == "failed" {
			failed = append(failed, step)
		}
	}
	return failed
}

// GetCompletedSteps 获取完成的步骤
func (p *PlanSpec) GetCompletedSteps() []PlanStep {
	var completed []PlanStep
	for _, step := range p.Steps {
		if step.Status == "completed" {
			completed = append(completed, step)
		}
	}
	return completed
}

// ToJSON 转换为JSON
func (p *PlanSpec) ToJSON() ([]byte, error) {
	return json.MarshalIndent(p, "", "  ")
}

// FromJSON 从JSON解析
func FromJSON(data []byte) (*PlanSpec, error) {
	var spec PlanSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, err
	}
	return &spec, nil
}

// 辅助函数
func generatePlanID() string {
	return fmt.Sprintf("plan_%d", time.Now().UnixNano())
}

func generateEvidenceID() string {
	return fmt.Sprintf("ev_%d", time.Now().UnixNano())
}

func generateArtifactID() string {
	return fmt.Sprintf("art_%d", time.Now().UnixNano())
}

func generateRecoveryPointID() string {
	return fmt.Sprintf("rp_%d", time.Now().UnixNano())
}

func assessToolRisk(toolName string) string {
	// 危险工具列表
	dangerousTools := []string{
		"rm", "del", "format", "shutdown", "reboot",
		"chmod", "chown", "sudo", "su", "kill",
		"dd", "mkfs", "fdisk", "mount", "umount",
	}

	// 中等风险工具
	mediumRiskTools := []string{
		"write_file", "delete_file", "move_file",
		"run_command", "execute", "shell",
		"browser_navigate", "browser_click",
	}

	toolLower := strings.ToLower(toolName)

	for _, tool := range dangerousTools {
		if strings.Contains(toolLower, tool) {
			return "high"
		}
	}

	for _, tool := range mediumRiskTools {
		if strings.Contains(toolLower, tool) {
			return "medium"
		}
	}

	return "low"
}

// PlanBuilder 计划构建器
type PlanBuilder struct {
	spec *PlanSpec
}

// NewPlanBuilder 创建计划构建器
func NewPlanBuilder(taskID, title, description string) *PlanBuilder {
	return &PlanBuilder{
		spec: NewPlanSpec(taskID, title, description),
	}
}

// SetMode 设置模式
func (b *PlanBuilder) SetMode(mode string) *PlanBuilder {
	b.spec.Mode = mode
	return b
}

// SetRiskLevel 设置风险等级
func (b *PlanBuilder) SetRiskLevel(level string) *PlanBuilder {
	b.spec.RiskLevel = level
	return b
}

// AddRiskLabel 添加风险标签
func (b *PlanBuilder) AddRiskLabel(label string) *PlanBuilder {
	b.spec.RiskLabels = append(b.spec.RiskLabels, label)
	return b
}

// SetPrivacyScope 设置隐私范围
func (b *PlanBuilder) SetPrivacyScope(scope string) *PlanBuilder {
	b.spec.PrivacyScope = scope
	return b
}

// SetDataScope 设置数据范围
func (b *PlanBuilder) SetDataScope(scope string) *PlanBuilder {
	b.spec.DataScope = scope
	return b
}

// SetRequiresApproval 设置是否需要审批
func (b *PlanBuilder) SetRequiresApproval(requires bool) *PlanBuilder {
	b.spec.RequiresApproval = requires
	return b
}

// SetApprovalScope 设置审批范围
func (b *PlanBuilder) SetApprovalScope(scope string) *PlanBuilder {
	b.spec.ApprovalScope = scope
	return b
}

// AddWorkflowStep 添加工作流步骤
func (b *PlanBuilder) AddWorkflowStep(workflow WorkflowDescriptor, inputs map[string]any) *PlanBuilder {
	b.spec.AddWorkflowStep(workflow, inputs)
	return b
}

// AddToolStep 添加工具步骤
func (b *PlanBuilder) AddToolStep(toolName string, inputs map[string]any, requiresApproval bool) *PlanBuilder {
	b.spec.AddToolStep(toolName, inputs, requiresApproval)
	return b
}

// AddVerificationStep 添加验证步骤
func (b *PlanBuilder) AddVerificationStep(stepIndex int, verification VerificationSpec) *PlanBuilder {
	b.spec.AddVerificationStep(stepIndex, verification)
	return b
}

// AddRollbackStep 添加回滚步骤
func (b *PlanBuilder) AddRollbackStep(stepIndex int, rollback RollbackSpec) *PlanBuilder {
	b.spec.AddRollbackStep(stepIndex, rollback)
	return b
}

// Build 构建计划规范
func (b *PlanBuilder) Build() *PlanSpec {
	return b.spec
}
