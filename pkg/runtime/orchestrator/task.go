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
	ID            string        `json:"id"`
	Title         string        `json:"title"`
	Description   string        `json:"description"`
	AssignedAgent string        `json:"assigned_agent"`
	Input         string        `json:"input"`
	DependsOn     []string      `json:"depends_on,omitempty"`
	Status        SubTaskStatus `json:"status"`
	Output        string        `json:"output,omitempty"`
	Error         string        `json:"error,omitempty"`
	StartedAt     *time.Time    `json:"started_at,omitempty"`
	CompletedAt   *time.Time    `json:"completed_at,omitempty"`
	Duration      time.Duration `json:"duration"`
	Index         int           `json:"index"`
}

type DecompositionPlan struct {
	Summary  string    `json:"summary"`
	SubTasks []SubTask `json:"sub_tasks"`
}

type AgentCapability struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Domain      string   `json:"domain"`
	Expertise   []string `json:"expertise"`
	Skills      []string `json:"skills"`
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
	Title         string `json:"title"`
	Description   string `json:"description"`
	AssignedAgent string `json:"assigned_agent"`
	DependsOn     []int  `json:"depends_on,omitempty"`
}

func NewTaskDecomposer(llm TaskPlannerLLM) *TaskDecomposer {
	return &TaskDecomposer{llm: llm}
}

func (d *TaskDecomposer) Decompose(ctx context.Context, taskID string, input string, agents []AgentCapability) (*DecompositionPlan, error) {
	if d.llm == nil || len(agents) == 0 {
		return d.defaultDecompose(taskID, input, agents), nil
	}

	agentDescs := make([]string, len(agents))
	for i, a := range agents {
		expertise := ""
		if len(a.Expertise) > 0 {
			expertise = "，擅长：" + strings.Join(a.Expertise, "、")
		}
		agentDescs[i] = fmt.Sprintf("- %s：%s（领域：%s%s）", a.Name, a.Description, a.Domain, expertise)
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
5. 返回 2-8 个子任务
6. 只返回 JSON：

{
  "summary": "任务总体描述和执行策略",
  "sub_tasks": [
    {
      "title": "子任务标题",
      "description": "详细描述，包括要做什么、输出什么格式",
      "assigned_agent": "智能体名称",
      "depends_on": []
    }
  ]
}`,
		},
		map[string]string{
			"role":    "user",
			"content": fmt.Sprintf("任务：%s\n\n可用智能体：\n%s\n\n请将任务分解并分配给合适的智能体。", input, agentList),
		},
	}

	resp, err := d.llm.Chat(ctx, messages, nil)
	if err != nil {
		return d.defaultDecompose(taskID, input, agents), nil
	}

	var payload planPayload
	raw := strings.TrimSpace(resp.Content)
	if raw == "" {
		return d.defaultDecompose(taskID, input, agents), nil
	}

	jsonStr := extractJSON(raw)
	if err := json.Unmarshal([]byte(jsonStr), &payload); err != nil {
		return d.defaultDecompose(taskID, input, agents), nil
	}

	if len(payload.SubTasks) == 0 {
		return d.defaultDecompose(taskID, input, agents), nil
	}

	agentNames := make(map[string]bool, len(agents))
	for _, a := range agents {
		agentNames[a.Name] = true
	}

	subTasks := make([]SubTask, 0, len(payload.SubTasks))
	for i, step := range payload.SubTasks {
		if strings.TrimSpace(step.Title) == "" {
			continue
		}

		// Validate assigned agent
		agentName := step.AssignedAgent
		if !agentNames[agentName] {
			// Fallback: find best match
			agentName = d.findBestAgent(step.Description, agents)
		}

		deps := make([]string, 0)
		for _, depIdx := range step.DependsOn {
			if depIdx >= 0 && depIdx < i {
				deps = append(deps, fmt.Sprintf("%s_sub_%d", taskID, depIdx))
			}
		}

		inputText := fmt.Sprintf("任务：%s\n子任务：%s\n要求：%s", input, step.Title, step.Description)

		subTasks = append(subTasks, SubTask{
			ID:            fmt.Sprintf("%s_sub_%d", taskID, i),
			Title:         step.Title,
			Description:   step.Description,
			AssignedAgent: agentName,
			Input:         inputText,
			DependsOn:     deps,
			Status:        SubTaskPending,
			Index:         i,
		})
	}

	if len(subTasks) == 0 {
		return d.defaultDecompose(taskID, input, agents), nil
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

	// Smart default: pick the agent whose domain/expertise best matches the input
	agentName := d.findBestAgent(input, agents)
	if agentName == "" {
		agentName = agents[0].Name
	}

	return &DecompositionPlan{
		Summary: fmt.Sprintf("将任务分配给 %s 执行", agentName),
		SubTasks: []SubTask{
			{
				ID:            fmt.Sprintf("%s_sub_0", taskID),
				Title:         "执行任务",
				Description:   input,
				AssignedAgent: agentName,
				Input:         input,
				Status:        SubTaskReady,
				Index:         0,
			},
		},
	}
}

func (d *TaskDecomposer) findBestAgent(description string, agents []AgentCapability) string {
	lower := strings.ToLower(description)

	for _, a := range agents {
		if a.Domain != "" && strings.Contains(lower, strings.ToLower(a.Domain)) {
			return a.Name
		}
		for _, exp := range a.Expertise {
			if strings.Contains(lower, strings.ToLower(exp)) {
				return a.Name
			}
		}
	}

	if len(agents) > 0 {
		return agents[0].Name
	}
	return ""
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
