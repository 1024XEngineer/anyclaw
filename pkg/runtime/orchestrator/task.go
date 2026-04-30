package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

type SubTaskStatus string

const (
	SubTaskPending   SubTaskStatus = "pending"
	SubTaskReady     SubTaskStatus = "ready"
	SubTaskRunning   SubTaskStatus = "running"
	SubTaskCompleted SubTaskStatus = "completed"
	SubTaskFailed    SubTaskStatus = "failed"
)

type SubTask struct {
	ID                   string        `json:"id"`
	Title                string        `json:"title"`
	Description          string        `json:"description"`
	AssignedAgent        string        `json:"assigned_agent"`
	Input                string        `json:"input"`
	DependsOn            []string      `json:"depends_on,omitempty"`
	Status               SubTaskStatus `json:"status"`
	Output               string        `json:"output,omitempty"`
	Error                string        `json:"error,omitempty"`
	StartedAt            *time.Time    `json:"started_at,omitempty"`
	CompletedAt          *time.Time    `json:"completed_at,omitempty"`
	Duration             time.Duration `json:"duration"`
	Index                int           `json:"index"`
	AssignmentReason     string        `json:"assignment_reason,omitempty"`
	AssignmentScore      int           `json:"assignment_score,omitempty"`
	AssignmentConfidence float64       `json:"assignment_confidence,omitempty"`
	RequiredCapabilities []string      `json:"required_capabilities,omitempty"`
}

type DecompositionPlan struct {
	Summary  string    `json:"summary"`
	SubTasks []SubTask `json:"sub_tasks"`
}

type AgentCapability struct {
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	Domain          string   `json:"domain"`
	Expertise       []string `json:"expertise"`
	Skills          []string `json:"skills"`
	Tools           []string `json:"tools,omitempty"`
	ToolCategories  []string `json:"tool_categories,omitempty"`
	PermissionLevel string   `json:"permission_level,omitempty"`
	Keywords        []string `json:"keywords,omitempty"`
}

type TaskDecomposer struct {
	llm TaskPlannerLLM
}

type TaskPlannerLLM interface {
	Chat(ctx context.Context, messages []interface{}, tools []interface{}) (*PlannerResponse, error)
	Name() string
}

type PlannerResponse struct {
	Content   string
	ToolCalls []interface{}
}

type planPayload struct {
	Summary  string     `json:"summary"`
	SubTasks []planStep `json:"sub_tasks"`
}

type planStep struct {
	Title                string   `json:"title"`
	Description          string   `json:"description"`
	AssignedAgent        string   `json:"assigned_agent"`
	DependsOn            []int    `json:"depends_on,omitempty"`
	RequiredCapabilities []string `json:"required_capabilities,omitempty"`
	Confidence           float64  `json:"confidence,omitempty"`
	Reason               string   `json:"reason,omitempty"`
}

func NewTaskDecomposer(llm TaskPlannerLLM) *TaskDecomposer {
	return &TaskDecomposer{llm: llm}
}

func (d *TaskDecomposer) Decompose(ctx context.Context, taskID string, input string, agents []AgentCapability) (*DecompositionPlan, error) {
	if d.llm == nil || len(agents) == 0 {
		return d.defaultDecompose(taskID, input, agents), nil
	}

	ranked := rankAgentCandidates(input, agents)
	candidateScores := selectCandidateWindow(ranked, defaultAgentCandidateLimit)
	candidates := candidatesToCapabilities(candidateScores)
	if len(candidates) == 0 {
		candidates = agents
		candidateScores = rankAgentCandidates(input, candidates)
	}

	agentDescs := make([]string, len(candidateScores))
	for i, score := range candidateScores {
		agentDescs[i] = formatAgentCandidateForPrompt(score)
	}
	agentList := strings.Join(agentDescs, "\n")

	messages := []interface{}{
		map[string]string{
			"role": "system",
			"content": `你是一个任务分解专家。你负责将用户的复杂任务拆分为多个子任务，并分配给最合适的智能体执行。

规则：
1. 每个子任务必须指定 assigned_agent，从可用智能体列表中选择
2. 子任务之间可以有依赖关系（depends_on 使用子任务索引，从0开始）
3. 前一个子任务的输出会自动传递给依赖它的子任务作为上下文
4. 每个子任务的 description 要足够详细，让智能体知道具体该做什么
5. 候选智能体已经按 skill、工具类别、权限和关键词预筛选；assigned_agent 只能从候选列表选择
6. confidence 使用 0-1；低置信度或能力冲突会被系统评分兜底覆盖
7. 返回 2-8 个子任务
8. 只返回 JSON：

{
  "summary": "任务总体描述和执行策略",
  "sub_tasks": [
    {
      "title": "子任务标题",
      "description": "详细描述，包括要做什么、输出什么格式",
      "assigned_agent": "智能体名称",
      "required_capabilities": ["需要的 skill、工具类别或专业能力"],
      "confidence": 0.85,
      "reason": "为什么这个智能体最合适",
      "depends_on": []
    }
  ]
}`,
		},
		map[string]string{
			"role":    "user",
			"content": fmt.Sprintf("任务：%s\n\n候选智能体：\n%s\n\n请将任务分解并分配给合适的智能体。", input, agentList),
		},
	}

	resp, err := d.llm.Chat(ctx, messages, nil)
	if err != nil {
		return d.defaultDecompose(taskID, input, candidates), nil
	}

	var payload planPayload
	raw := strings.TrimSpace(resp.Content)
	if raw == "" {
		return d.defaultDecompose(taskID, input, candidates), nil
	}

	jsonStr := extractJSON(raw)
	if err := json.Unmarshal([]byte(jsonStr), &payload); err != nil {
		return d.defaultDecompose(taskID, input, candidates), nil
	}

	if len(payload.SubTasks) == 0 {
		return d.defaultDecompose(taskID, input, candidates), nil
	}

	agentNames := make(map[string]bool, len(candidates))
	for _, a := range candidates {
		agentNames[a.Name] = true
	}

	subTasks := make([]SubTask, 0, len(payload.SubTasks))
	for i, step := range payload.SubTasks {
		if strings.TrimSpace(step.Title) == "" {
			continue
		}

		description := strings.TrimSpace(step.Description)
		if description == "" {
			description = strings.TrimSpace(step.Title)
		}
		requiredCaps := normalizeAssignmentCapabilities(step.RequiredCapabilities)
		assignment := chooseAgentAssignment(step.AssignedAgent, description, step.Confidence, requiredCaps, candidates)
		agentName := assignment.Name
		if !agentNames[agentName] {
			agentName = d.findBestAgent(description, candidates)
			assignment = bestAgentAssignment(description, candidates)
		}
		reason := strings.TrimSpace(step.Reason)
		if reason == "" || agentName != strings.TrimSpace(step.AssignedAgent) {
			reason = assignment.Reason
		}
		confidence := step.Confidence
		if confidence <= 0 || agentName != strings.TrimSpace(step.AssignedAgent) {
			confidence = assignment.Confidence
		}
		if len(requiredCaps) == 0 {
			requiredCaps = assignment.RequiredCaps
		}

		deps := make([]string, 0)
		for _, depIdx := range step.DependsOn {
			if depIdx >= 0 && depIdx < i {
				deps = append(deps, fmt.Sprintf("%s_sub_%d", taskID, depIdx))
			}
		}

		inputText := fmt.Sprintf("任务：%s\n子任务：%s\n要求：%s", input, step.Title, description)

		subTasks = append(subTasks, SubTask{
			ID:                   fmt.Sprintf("%s_sub_%d", taskID, i),
			Title:                step.Title,
			Description:          description,
			AssignedAgent:        agentName,
			Input:                inputText,
			DependsOn:            deps,
			Status:               SubTaskPending,
			Index:                i,
			AssignmentReason:     reason,
			AssignmentScore:      assignment.Score,
			AssignmentConfidence: confidence,
			RequiredCapabilities: requiredCaps,
		})
	}

	if len(subTasks) == 0 {
		return d.defaultDecompose(taskID, input, candidates), nil
	}

	// Mark tasks with no dependencies as ready
	for i := range subTasks {
		if len(subTasks[i].DependsOn) == 0 {
			subTasks[i].Status = SubTaskReady
		}
	}

	return &DecompositionPlan{
		Summary:  payload.Summary,
		SubTasks: subTasks,
	}, nil
}

func (d *TaskDecomposer) defaultDecompose(taskID string, input string, agents []AgentCapability) *DecompositionPlan {
	if len(agents) == 0 {
		return &DecompositionPlan{
			Summary:  "",
			SubTasks: nil,
		}
	}

	assignment := bestAgentAssignment(input, agents)
	agentName := assignment.Name
	if agentName == "" {
		agentName = agents[0].Name
	}

	return &DecompositionPlan{
		Summary: fmt.Sprintf("将任务分配给 %s 执行", agentName),
		SubTasks: []SubTask{
			{
				ID:                   fmt.Sprintf("%s_sub_0", taskID),
				Title:                "执行任务",
				Description:          input,
				AssignedAgent:        agentName,
				Input:                input,
				Status:               SubTaskReady,
				Index:                0,
				AssignmentReason:     assignment.Reason,
				AssignmentScore:      assignment.Score,
				AssignmentConfidence: assignment.Confidence,
				RequiredCapabilities: assignment.RequiredCaps,
			},
		},
	}
}

func (d *TaskDecomposer) findBestAgent(description string, agents []AgentCapability) string {
	return bestAgentAssignment(description, agents).Name
}

func normalizeAssignmentCapabilities(items []string) []string {
	normalized := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" || containsString(normalized, item) {
			continue
		}
		normalized = append(normalized, item)
	}
	return normalized
}

type TaskQueue struct {
	mu      sync.Mutex
	tasks   []*SubTask
	index   map[string]*SubTask
	ordered []*SubTask
}

func NewTaskQueue() *TaskQueue {
	return &TaskQueue{
		index: make(map[string]*SubTask),
	}
}

func (q *TaskQueue) Load(plan *DecompositionPlan) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.tasks = make([]*SubTask, len(plan.SubTasks))
	q.ordered = make([]*SubTask, len(plan.SubTasks))
	for i := range plan.SubTasks {
		task := plan.SubTasks[i] // copy
		q.tasks[i] = &task
		q.ordered[i] = &task
		q.index[task.ID] = &task
	}
}

func (q *TaskQueue) DequeueReady() *SubTask {
	q.mu.Lock()
	defer q.mu.Unlock()

	for _, task := range q.tasks {
		if task.Status != SubTaskReady {
			continue
		}

		// Check dependencies
		allDepsMet := true
		for _, depID := range task.DependsOn {
			if dep, ok := q.index[depID]; ok {
				if dep.Status != SubTaskCompleted {
					allDepsMet = false
					break
				}
			}
		}

		if allDepsMet {
			task.Status = SubTaskRunning
			return task
		}
	}
	return nil
}

func (q *TaskQueue) HasReady() bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	for _, task := range q.tasks {
		if task.Status != SubTaskReady {
			continue
		}
		allDepsMet := true
		for _, depID := range task.DependsOn {
			if dep, ok := q.index[depID]; ok {
				if dep.Status != SubTaskCompleted {
					allDepsMet = false
					break
				}
			}
		}
		if allDepsMet {
			return true
		}
	}
	return false
}

func (q *TaskQueue) HasPending() bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	for _, task := range q.tasks {
		if task.Status == SubTaskPending || task.Status == SubTaskReady || task.Status == SubTaskRunning {
			return true
		}
	}
	return false
}

func (q *TaskQueue) UpdateResult(taskID string, output string, errStr string, duration time.Duration) {
	q.mu.Lock()
	defer q.mu.Unlock()

	task, ok := q.index[taskID]
	if !ok {
		return
	}

	now := time.Now()
	task.CompletedAt = &now
	task.Duration = duration
	task.Output = output

	if errStr != "" {
		task.Status = SubTaskFailed
		task.Error = errStr
		// Mark dependent tasks as failed too
		q.markDependentsFailed(taskID)
	} else {
		task.Status = SubTaskCompleted
		// Mark dependent tasks as ready if all their deps are met
		q.activateDependents(taskID)
	}
}

func (q *TaskQueue) markDependentsFailed(failedID string) {
	for _, task := range q.tasks {
		for _, depID := range task.DependsOn {
			if depID == failedID && task.Status == SubTaskPending {
				task.Status = SubTaskFailed
				task.Error = fmt.Sprintf("dependency %s failed", failedID)
				q.markDependentsFailed(task.ID)
			}
		}
	}
}

func (q *TaskQueue) activateDependents(completedID string) {
	for _, task := range q.tasks {
		if task.Status != SubTaskPending {
			continue
		}
		for _, depID := range task.DependsOn {
			if depID == completedID {
				// Check if ALL dependencies are now met
				allMet := true
				for _, d := range task.DependsOn {
					if dep, ok := q.index[d]; ok {
						if dep.Status != SubTaskCompleted {
							allMet = false
							break
						}
					}
				}
				if allMet {
					task.Status = SubTaskReady
				}
			}
		}
	}
}

func (q *TaskQueue) GetAll() []*SubTask {
	q.mu.Lock()
	defer q.mu.Unlock()
	result := make([]*SubTask, len(q.ordered))
	copy(result, q.ordered)
	return result
}

func (q *TaskQueue) GetDepOutputs(taskID string) map[string]string {
	q.mu.Lock()
	defer q.mu.Unlock()

	task, ok := q.index[taskID]
	if !ok {
		return nil
	}

	outputs := make(map[string]string)
	for _, depID := range task.DependsOn {
		if dep, ok := q.index[depID]; ok && dep.Status == SubTaskCompleted {
			outputs[depID] = dep.Output
		}
	}
	return outputs
}

func (q *TaskQueue) Stats() (pending, ready, running, completed, failed int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, t := range q.tasks {
		switch t.Status {
		case SubTaskPending:
			pending++
		case SubTaskReady:
			ready++
		case SubTaskRunning:
			running++
		case SubTaskCompleted:
			completed++
		case SubTaskFailed:
			failed++
		}
	}
	return
}

func extractJSON(input string) string {
	input = strings.TrimSpace(input)
	if strings.HasPrefix(input, "```") {
		parts := strings.Split(input, "```")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "json") {
				part = strings.TrimSpace(strings.TrimPrefix(part, "json"))
			}
			if strings.HasPrefix(part, "{") {
				return part
			}
		}
	}
	return input
}
