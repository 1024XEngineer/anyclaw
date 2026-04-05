package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/anyclaw/anyclaw/pkg/agent"
	"github.com/anyclaw/anyclaw/pkg/llm"
	"github.com/anyclaw/anyclaw/pkg/memory"
	"github.com/anyclaw/anyclaw/pkg/skills"
	"github.com/anyclaw/anyclaw/pkg/tools"
)

type OrchestratorConfig struct {
	MaxConcurrentAgents int               `json:"max_concurrent_agents"`
	MaxRetries          int               `json:"max_retries"`
	Timeout             time.Duration     `json:"timeout"`
	AgentDefinitions    []AgentDefinition `json:"agent_definitions"`
	EnableDecomposition bool              `json:"enable_decomposition"`
}

type OrchestratorStatus string

const (
	StatusIdle     OrchestratorStatus = "idle"
	StatusPlanning OrchestratorStatus = "planning"
	StatusRunning  OrchestratorStatus = "running"
	StatusDone     OrchestratorStatus = "done"
	StatusError    OrchestratorStatus = "error"
)

type Orchestrator struct {
	config      OrchestratorConfig
	agentPool   *AgentPool
	decomposer  *TaskDecomposer
	queue       *TaskQueue
	lifecycle   *AgentLifecycle
	allSkills   *skills.SkillsManager
	baseTools   *tools.Registry
	memory      memory.MemoryBackend
	llm         agent.LLMCaller
	messageBus  *MessageBus
	mu          sync.Mutex
	status      OrchestratorStatus
	history     []ExecutionLog
	taskCounter int
}

type ExecutionLog struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	AgentName string    `json:"agent_name,omitempty"`
	TaskID    string    `json:"task_id,omitempty"`
}

type OrchestratorResult struct {
	Status    OrchestratorStatus `json:"status"`
	Summary   string             `json:"summary"`
	SubTasks  []SubTask          `json:"sub_tasks"`
	Stats     TaskStats          `json:"stats"`
	TotalTime time.Duration      `json:"total_time"`
}

type TaskStats struct {
	Total     int `json:"total"`
	Pending   int `json:"pending"`
	Ready     int `json:"ready"`
	Running   int `json:"running"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
}

type OrchestratorOption func(*Orchestrator)

func WithDecomposer(d *TaskDecomposer) OrchestratorOption {
	return func(o *Orchestrator) {
		o.decomposer = d
	}
}

func NewOrchestrator(cfg OrchestratorConfig, llmClient agent.LLMCaller, allSkills *skills.SkillsManager, baseTools *tools.Registry, mem memory.MemoryBackend, opts ...OrchestratorOption) (*Orchestrator, error) {
	if cfg.MaxConcurrentAgents <= 0 {
		cfg.MaxConcurrentAgents = 4
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 2
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Minute
	}

	o := &Orchestrator{
		config:     cfg,
		agentPool:  NewAgentPool(),
		queue:      NewTaskQueue(),
		lifecycle:  NewAgentLifecycle(5, 50),
		allSkills:  allSkills,
		baseTools:  baseTools,
		memory:     mem,
		llm:        llmClient,
		messageBus: NewMessageBus(100),
		status:     StatusIdle,
		history:    make([]ExecutionLog, 0),
	}

	for _, opt := range opts {
		opt(o)
	}

	if o.decomposer == nil && llmClient != nil {
		o.decomposer = NewTaskDecomposer(&plannerAdapter{client: llmClient})
	}

	if err := o.initAgents(cfg.AgentDefinitions); err != nil {
		return nil, fmt.Errorf("failed to initialize agents: %w", err)
	}

	return o, nil
}

func (o *Orchestrator) initAgents(defs []AgentDefinition) error {
	for _, def := range defs {
		sa, err := NewSubAgent(def, o.llm, o.allSkills, o.baseTools, o.memory)
		if err != nil {
			o.log("warn", fmt.Sprintf("Failed to create agent %q: %v", def.Name, err), "", "")
			continue
		}
		o.agentPool.Register(def.Name, sa)

		ma, err := o.lifecycle.Spawn(def.Name, "", map[string]string{
			"domain": def.Domain,
			"role":   def.Persona,
			"skills": strings.Join(def.PrivateSkills, ","),
		})
		if err == nil {
			sa.SetLifecycleID(ma.ID)
		}

		sa.SetMessageBus(o.messageBus)

		o.messageBus.Subscribe(def.Name)

		o.log("info", fmt.Sprintf("Registered agent %q [domain=%s]", def.Name, def.Domain), def.Name, "")
	}
	return nil
}

func (o *Orchestrator) Run(ctx context.Context, input string) (string, error) {
	return o.RunWithAgents(ctx, input, nil)
}

func (o *Orchestrator) RunWithAgents(ctx context.Context, input string, selectedAgentNames []string) (string, error) {
	startTime := time.Now()

	o.mu.Lock()
	o.status = StatusPlanning
	o.taskCounter++
	taskID := fmt.Sprintf("orch_%d", o.taskCounter)
	o.history = o.history[:0]
	o.mu.Unlock()

	o.log("info", fmt.Sprintf("Orchestrator starting for task: %s", truncateString(input, 60)), "", taskID)

	// Determine which agents to use
	var agents []*SubAgent
	if len(selectedAgentNames) > 0 {
		for _, name := range selectedAgentNames {
			if sa, ok := o.agentPool.Get(name); ok {
				agents = append(agents, sa)
			}
		}
	}
	if len(agents) == 0 {
		agents = o.agentPool.List()
	}

	// Build agent capabilities for decomposer
	capabilities := make([]AgentCapability, len(agents))
	for i, sa := range agents {
		capabilities[i] = AgentCapability{
			Name:        sa.Name(),
			Description: sa.Description(),
			Domain:      sa.Domain(),
			Expertise:   sa.Expertise(),
			Skills:      sa.Skills(),
		}
	}

	// Phase 1: Decompose task
	o.log("info", fmt.Sprintf("Decomposing task among %d agents...", len(agents)), "", taskID)

	if len(capabilities) == 0 {
		o.mu.Lock()
		o.status = StatusError
		o.mu.Unlock()
		return "", fmt.Errorf("no agents available for task decomposition")
	}

	var plan *DecompositionPlan
	var err error
	if o.config.EnableDecomposition && o.decomposer != nil {
		plan, err = o.decomposer.Decompose(ctx, taskID, input, capabilities)
	} else if o.decomposer != nil {
		plan = o.decomposer.defaultDecompose(taskID, input, capabilities)
	} else {
		// No decomposer available (no LLM client) - create minimal plan
		plan = &DecompositionPlan{
			Summary: fmt.Sprintf("将任务分配给 %s 执行", capabilities[0].Name),
			SubTasks: []SubTask{{
				ID:            fmt.Sprintf("%s_sub_0", taskID),
				Title:         "执行任务",
				Description:   input,
				AssignedAgent: capabilities[0].Name,
				Input:         input,
				Status:        SubTaskReady,
				Index:         0,
			}},
		}
	}
	if err != nil {
		o.mu.Lock()
		o.status = StatusError
		o.mu.Unlock()
		return "", fmt.Errorf("task decomposition failed: %w", err)
	}

	if plan == nil || len(plan.SubTasks) == 0 {
		o.mu.Lock()
		o.status = StatusError
		o.mu.Unlock()
		return "", fmt.Errorf("task decomposition produced no sub-tasks")
	}

	o.log("info", fmt.Sprintf("Plan: %s (%d sub-tasks)", plan.Summary, len(plan.SubTasks)), "", taskID)
	for _, st := range plan.SubTasks {
		o.log("info", fmt.Sprintf("  [%d] %s → %s (deps: %v)", st.Index+1, st.Title, st.AssignedAgent, st.DependsOn), st.AssignedAgent, st.ID)
	}

	// Phase 2: Load plan into queue
	o.queue.Load(plan)

	o.mu.Lock()
	o.status = StatusRunning
	o.mu.Unlock()

	// Phase 3: Execute sub-tasks concurrently with dependency resolution
	var wg sync.WaitGroup
	sem := make(chan struct{}, o.config.MaxConcurrentAgents)
	var execMu sync.Mutex

	for o.queue.HasPending() {
		subTask := o.queue.DequeueReady()
		if subTask == nil {
			if !o.queue.HasPending() {
				break
			}
			time.Sleep(100 * time.Millisecond)
			continue
		}

		sa, ok := o.agentPool.Get(subTask.AssignedAgent)
		if !ok {
			o.log("error", fmt.Sprintf("Agent %q not found for sub-task %q", subTask.AssignedAgent, subTask.Title), "", subTask.ID)
			o.queue.UpdateResult(subTask.ID, "", fmt.Sprintf("agent %q not found", subTask.AssignedAgent), 0)
			continue
		}

		sem <- struct{}{}
		wg.Add(1)
		go func(st *SubTask, agent *SubAgent) {
			defer wg.Done()
			defer func() { <-sem }()

			taskInput := o.buildTaskInput(st, input)

			o.log("info", fmt.Sprintf("Executing: %s (agent: %s)", st.Title, agent.Name()), agent.Name(), st.ID)

			lifecycleID := agent.LifecycleID()
			if lifecycleID != "" {
				_ = o.lifecycle.Start(lifecycleID)
			}

			agent.BroadcastMessage("task_started", map[string]any{
				"task_id":    st.ID,
				"title":      st.Title,
				"agent_name": agent.Name(),
			})

			runStart := time.Now()
			now := runStart
			st.StartedAt = &now
			output, runErr := agent.Run(ctx, taskInput)
			duration := time.Since(runStart)

			execMu.Lock()
			if runErr != nil {
				o.log("error", fmt.Sprintf("Agent %q failed on %q: %v", agent.Name(), st.Title, runErr), agent.Name(), st.ID)
				o.queue.UpdateResult(st.ID, "", runErr.Error(), duration)
				if lifecycleID != "" {
					_ = o.lifecycle.Fail(lifecycleID, runErr.Error())
				}
				agent.BroadcastMessage("task_failed", map[string]any{
					"task_id":    st.ID,
					"title":      st.Title,
					"agent_name": agent.Name(),
					"error":      runErr.Error(),
				})
			} else {
				o.log("info", fmt.Sprintf("Completed: %s (in %v)", st.Title, duration.Round(time.Millisecond)), agent.Name(), st.ID)
				o.queue.UpdateResult(st.ID, output, "", duration)
				if lifecycleID != "" {
					_ = o.lifecycle.Complete(lifecycleID, output)
				}
				agent.BroadcastMessage("task_completed", map[string]any{
					"task_id":    st.ID,
					"title":      st.Title,
					"agent_name": agent.Name(),
					"output_len": len(output),
				})
			}
			execMu.Unlock()
		}(subTask, sa)
	}

	wg.Wait()

	// Phase 4: Aggregate results
	allSubTasks := o.queue.GetAll()
	summary := o.aggregateResults(plan.Summary, allSubTasks, input)

	o.mu.Lock()
	o.status = StatusDone
	totalTime := time.Since(startTime)
	o.mu.Unlock()

	p, r, ru, c, f := o.queue.Stats()
	o.log("info", fmt.Sprintf("Done in %v (pending=%d ready=%d running=%d completed=%d failed=%d)",
		totalTime.Round(time.Millisecond), p, r, ru, c, f), "", taskID)

	if f > 0 && c == 0 {
		return summary, fmt.Errorf("all %d sub-tasks failed", f)
	}
	if f > 0 {
		return summary, fmt.Errorf("%d/%d sub-tasks failed", f, c+f)
	}

	return summary, nil
}

func (o *Orchestrator) buildTaskInput(subTask *SubTask, originalInput string) string {
	var sb strings.Builder
	sb.WriteString(subTask.Input)

	// Add dependency outputs as context
	depOutputs := o.queue.GetDepOutputs(subTask.ID)
	if len(depOutputs) > 0 {
		sb.WriteString("\n\n--- 前置任务的输出结果（请基于这些信息继续工作）---\n")
		for depID, output := range depOutputs {
			// Find the sub-task title
			for _, st := range o.queue.GetAll() {
				if st.ID == depID {
					sb.WriteString(fmt.Sprintf("\n[%s 的结果]:\n%s\n", st.Title, truncateString(output, 500)))
					break
				}
			}
		}
	}

	return sb.String()
}

func (o *Orchestrator) aggregateResults(planSummary string, subTasks []*SubTask, originalInput string) string {
	var sb strings.Builder

	completed := 0
	failed := 0
	totalDuration := time.Duration(0)

	for _, st := range subTasks {
		if st.Status == SubTaskCompleted {
			completed++
		} else if st.Status == SubTaskFailed {
			failed++
		}
		totalDuration += st.Duration
	}

	// If all completed, combine the outputs
	if completed > 0 && failed == 0 {
		sb.WriteString("## 任务完成\n\n")
		for _, st := range subTasks {
			if st.Status == SubTaskCompleted && st.Output != "" {
				sb.WriteString(st.Output)
				sb.WriteString("\n\n")
			}
		}
	} else {
		// Partial or failed results
		sb.WriteString(fmt.Sprintf("## 任务执行报告\n\n"))
		sb.WriteString(fmt.Sprintf("**原始需求**: %s\n", originalInput))
		sb.WriteString(fmt.Sprintf("**执行计划**: %s\n\n", planSummary))

		for _, st := range subTasks {
			statusIcon := "⏳"
			switch st.Status {
			case SubTaskCompleted:
				statusIcon = "✅"
			case SubTaskFailed:
				statusIcon = "❌"
			case SubTaskRunning:
				statusIcon = "🔄"
			}

			sb.WriteString(fmt.Sprintf("### %s %s (→ %s)\n", statusIcon, st.Title, st.AssignedAgent))
			if st.Output != "" {
				sb.WriteString(fmt.Sprintf("%s\n\n", st.Output))
			}
			if st.Error != "" {
				sb.WriteString(fmt.Sprintf("错误: %s\n\n", st.Error))
			}
		}

		sb.WriteString(fmt.Sprintf("\n**汇总**: 完成 %d/%d，失败 %d，总耗时 %v\n",
			completed, len(subTasks), failed, totalDuration.Round(time.Millisecond)))
	}

	return sb.String()
}

func (o *Orchestrator) log(level string, message string, agentName string, taskID string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.history = append(o.history, ExecutionLog{
		Timestamp: time.Now(),
		Level:     level,
		Message:   message,
		AgentName: agentName,
		TaskID:    taskID,
	})
}

func (o *Orchestrator) Status() OrchestratorStatus {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.status
}

func (o *Orchestrator) AgentCount() int {
	return o.agentPool.Size()
}

func (o *Orchestrator) GetAgent(name string) (*SubAgent, bool) {
	return o.agentPool.Get(name)
}

func (o *Orchestrator) FindAgentForSkills(skills []string) *SubAgent {
	return o.agentPool.FindAgentForSkills(skills)
}

func (o *Orchestrator) ListAgents() []AgentInfo {
	return o.agentPool.ListInfos()
}

func (o *Orchestrator) History() []ExecutionLog {
	o.mu.Lock()
	defer o.mu.Unlock()
	result := make([]ExecutionLog, len(o.history))
	copy(result, o.history)
	return result
}

func truncateString(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

type plannerAdapter struct {
	client agent.LLMCaller
}

func (p *plannerAdapter) Chat(ctx context.Context, messages []interface{}, tools []interface{}) (*PlannerResponse, error) {
	llmMessages := make([]llm.Message, 0, len(messages))
	for _, m := range messages {
		if msgMap, ok := m.(map[string]string); ok {
			llmMessages = append(llmMessages, llm.Message{
				Role:    msgMap["role"],
				Content: msgMap["content"],
			})
		}
	}

	resp, err := p.client.Chat(ctx, llmMessages, nil)
	if err != nil {
		return nil, err
	}

	return &PlannerResponse{
		Content: resp.Content,
	}, nil
}

func (p *plannerAdapter) Name() string {
	return p.client.Name()
}

func (o *Orchestrator) AgentPool() *AgentPool {
	return o.agentPool
}

func (o *Orchestrator) MessageBus() *MessageBus {
	return o.messageBus
}

func (o *Orchestrator) Queue() *TaskQueue {
	return o.queue
}

func (o *Orchestrator) Lifecycle() *AgentLifecycle {
	return o.lifecycle
}

func (o *Orchestrator) RunTask(ctx context.Context, input string, agentNames []string) (string, error) {
	return o.RunWithAgents(ctx, input, agentNames)
}
