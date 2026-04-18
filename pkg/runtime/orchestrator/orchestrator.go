package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	agent "github.com/1024XEngineer/anyclaw/pkg/capability/agents"
	llm "github.com/1024XEngineer/anyclaw/pkg/capability/models"
	"github.com/1024XEngineer/anyclaw/pkg/capability/skills"
	"github.com/1024XEngineer/anyclaw/pkg/capability/tools"
	"github.com/1024XEngineer/anyclaw/pkg/state/memory"
)

type OrchestratorConfig struct {
	MaxConcurrentAgents int               `json:"max_concurrent_agents"`
	MaxRetries          int               `json:"max_retries"`
	Timeout             time.Duration     `json:"timeout"`
	AgentDefinitions    []AgentDefinition `json:"agent_definitions"`
	EnableDecomposition bool              `json:"enable_decomposition"`
	DefaultWorkingDir   string            `json:"default_working_dir,omitempty"`
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
	config     OrchestratorConfig
	agentPool  *AgentPool
	decomposer *TaskDecomposer
	lifecycle  *AgentLifecycle
	allSkills  *skills.SkillsManager
	baseTools  *tools.Registry
	memory     memory.MemoryBackend
	llm        agent.LLMCaller
	messageBus *MessageBus

	mu           sync.Mutex
	taskCounter  int
	lastRun      *executionState
	lastResult   *OrchestratorResult
	initWarnings []string
}

type ExecutionLog struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	AgentName string    `json:"agent_name,omitempty"`
	TaskID    string    `json:"task_id,omitempty"`
}

type OrchestratorResult struct {
	TaskID    string             `json:"task_id"`
	Status    OrchestratorStatus `json:"status"`
	Summary   string             `json:"summary"`
	SubTasks  []SubTask          `json:"sub_tasks"`
	Stats     TaskStats          `json:"stats"`
	TotalTime time.Duration      `json:"total_time"`
	StartedAt time.Time          `json:"started_at"`
	History   []ExecutionLog     `json:"history,omitempty"`
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
		lifecycle:  NewAgentLifecycle(5, 50),
		allSkills:  allSkills,
		baseTools:  baseTools,
		memory:     mem,
		llm:        llmClient,
		messageBus: NewMessageBus(100),
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
	failures := make([]string, 0)
	registered := 0
	for _, def := range defs {
		sa, err := NewSubAgent(def, o.llm, o.allSkills, o.baseTools, o.memory)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", def.Name, err))
			continue
		}
		o.agentPool.Register(def.Name, sa)
		registered++

		ma, err := o.lifecycle.Spawn(def.Name, "", map[string]string{
			"domain": def.Domain,
			"role":   def.Persona,
			"skills": strings.Join(def.PrivateSkills, ","),
		})
		if err == nil {
			sa.SetLifecycleID(ma.ID)
		} else {
			failures = append(failures, fmt.Sprintf("%s lifecycle: %v", def.Name, err))
		}

		sa.SetMessageBus(o.messageBus)
		o.messageBus.Subscribe(def.Name)
	}
	if len(failures) > 0 {
		o.initWarnings = append([]string(nil), failures...)
	}
	if len(defs) > 0 && registered == 0 {
		return fmt.Errorf("failed to initialize all orchestrator agents: %s", strings.Join(failures, "; "))
	}
	return nil
}

func (o *Orchestrator) Run(ctx context.Context, input string) (string, error) {
	return o.RunWithAgents(ctx, input, nil)
}

func (o *Orchestrator) RunWithAgents(ctx context.Context, input string, selectedAgentNames []string) (string, error) {
	result, err := o.RunTaskResult(ctx, input, selectedAgentNames)
	if result == nil {
		return "", err
	}
	return result.Summary, err
}

func (o *Orchestrator) AvailableAgentNames() []string {
	if o == nil || o.agentPool == nil {
		return nil
	}
	agents := o.agentPool.List()
	names := make([]string, 0, len(agents))
	for _, agent := range agents {
		if agent == nil {
			continue
		}
		name := strings.TrimSpace(agent.Name())
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

func (o *Orchestrator) RunPlan(ctx context.Context, brief string, targetAgents []string) (*OrchestratorResult, error) {
	brief = strings.TrimSpace(brief)
	if brief == "" {
		return nil, fmt.Errorf("handoff brief is required")
	}
	if len(targetAgents) == 0 {
		return nil, fmt.Errorf("handoff plan requires explicit target agents")
	}
	return o.runTaskResult(ctx, brief, targetAgents, true)
}

func (o *Orchestrator) RunTaskResult(ctx context.Context, input string, agentNames []string) (*OrchestratorResult, error) {
	return o.runTaskResult(ctx, input, agentNames, false)
}

func (o *Orchestrator) runTaskResult(ctx context.Context, input string, agentNames []string, strictSelection bool) (*OrchestratorResult, error) {
	startTime := time.Now()
	exec := newExecutionState(o.nextTaskID())
	exec.SetStatus(StatusPlanning)
	exec.Log("info", fmt.Sprintf("orchestrator starting: %s", truncateString(input, 60)), "", exec.id)

	finalize := func(summary string) *OrchestratorResult {
		result := exec.Snapshot(summary, time.Since(startTime))
		o.setLastExecution(exec, &result)
		return &result
	}

	agents, resolveErr := o.resolveAgents(agentNames, strictSelection)
	if resolveErr != nil {
		exec.SetStatus(StatusError)
		result := finalize("")
		return result, resolveErr
	}
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

	exec.Log("info", fmt.Sprintf("decomposing across %d agents", len(agents)), "", exec.id)
	if len(capabilities) == 0 {
		exec.SetStatus(StatusError)
		result := finalize("")
		return result, fmt.Errorf("no agents available for task decomposition")
	}

	plan, err := o.buildPlan(ctx, exec.id, input, capabilities)
	if err != nil {
		exec.SetStatus(StatusError)
		result := finalize("")
		return result, err
	}
	if plan == nil || len(plan.SubTasks) == 0 {
		exec.SetStatus(StatusError)
		result := finalize("")
		return result, fmt.Errorf("task decomposition produced no sub-tasks")
	}

	exec.Log("info", fmt.Sprintf("plan ready: %s (%d sub-tasks)", plan.Summary, len(plan.SubTasks)), "", exec.id)
	for _, st := range plan.SubTasks {
		exec.Log("info", fmt.Sprintf("[%d] %s -> %s deps=%v", st.Index+1, st.Title, st.AssignedAgent, st.DependsOn), st.AssignedAgent, st.ID)
	}

	exec.queue.Load(plan)
	exec.SetStatus(StatusRunning)

	var wg sync.WaitGroup
	sem := make(chan struct{}, o.config.MaxConcurrentAgents)
	var updateMu sync.Mutex

	for exec.queue.HasPending() {
		subTask := exec.queue.DequeueReady()
		if subTask == nil {
			if !exec.queue.HasPending() {
				break
			}
			time.Sleep(100 * time.Millisecond)
			continue
		}

		sa, ok := o.agentPool.Get(subTask.AssignedAgent)
		if !ok {
			exec.Log("error", fmt.Sprintf("agent %q not found for sub-task %q", subTask.AssignedAgent, subTask.Title), "", subTask.ID)
			exec.queue.UpdateResult(subTask.ID, "", fmt.Sprintf("agent %q not found", subTask.AssignedAgent), 0)
			continue
		}

		sem <- struct{}{}
		wg.Add(1)
		go func(st *SubTask, subAgent *SubAgent) {
			defer wg.Done()
			defer func() { <-sem }()

			taskInput := o.buildTaskInput(exec, st)
			exec.Log("info", fmt.Sprintf("executing %s with %s", st.Title, subAgent.Name()), subAgent.Name(), st.ID)

			lifecycleID := subAgent.LifecycleID()
			if lifecycleID != "" {
				_ = o.lifecycle.Start(lifecycleID)
			}

			subAgent.BroadcastMessage("task_started", map[string]any{
				"task_id":    st.ID,
				"title":      st.Title,
				"agent_name": subAgent.Name(),
			})

			runStart := time.Now()
			now := runStart
			st.StartedAt = &now
			output, runErr := subAgent.Run(ctx, taskInput)
			duration := time.Since(runStart)

			updateMu.Lock()
			defer updateMu.Unlock()

			if runErr != nil {
				exec.Log("error", fmt.Sprintf("agent %q failed on %q: %v", subAgent.Name(), st.Title, runErr), subAgent.Name(), st.ID)
				exec.queue.UpdateResult(st.ID, "", runErr.Error(), duration)
				if lifecycleID != "" {
					_ = o.lifecycle.Fail(lifecycleID, runErr.Error())
				}
				subAgent.BroadcastMessage("task_failed", map[string]any{
					"task_id":    st.ID,
					"title":      st.Title,
					"agent_name": subAgent.Name(),
					"error":      runErr.Error(),
				})
				return
			}

			exec.Log("info", fmt.Sprintf("completed %s in %v", st.Title, duration.Round(time.Millisecond)), subAgent.Name(), st.ID)
			exec.queue.UpdateResult(st.ID, output, "", duration)
			if lifecycleID != "" {
				_ = o.lifecycle.Complete(lifecycleID, output)
			}
			subAgent.BroadcastMessage("task_completed", map[string]any{
				"task_id":    st.ID,
				"title":      st.Title,
				"agent_name": subAgent.Name(),
				"output_len": len(output),
			})
		}(subTask, sa)
	}

	wg.Wait()

	summary := o.aggregateResults(plan.Summary, exec.queue.GetAll(), input)
	exec.SetStatus(StatusDone)

	p, r, ru, c, f := exec.queue.Stats()
	exec.Log("info", fmt.Sprintf("done in %v (pending=%d ready=%d running=%d completed=%d failed=%d)",
		time.Since(startTime).Round(time.Millisecond), p, r, ru, c, f), "", exec.id)

	result := finalize(summary)
	if f > 0 && c == 0 {
		return result, fmt.Errorf("all %d sub-tasks failed", f)
	}
	if f > 0 {
		return result, fmt.Errorf("%d/%d sub-tasks failed", f, c+f)
	}

	return result, nil
}

func (o *Orchestrator) buildPlan(ctx context.Context, taskID string, input string, capabilities []AgentCapability) (*DecompositionPlan, error) {
	if o.config.EnableDecomposition && o.decomposer != nil {
		plan, err := o.decomposer.Decompose(ctx, taskID, input, capabilities)
		if err != nil {
			return nil, fmt.Errorf("task decomposition failed: %w", err)
		}
		return plan, nil
	}
	if o.decomposer != nil {
		return o.decomposer.defaultDecompose(taskID, input, capabilities), nil
	}
	return &DecompositionPlan{
		Summary: fmt.Sprintf("Delegate the task to %s for execution", capabilities[0].Name),
		SubTasks: []SubTask{{
			ID:            fmt.Sprintf("%s_sub_0", taskID),
			Title:         "Execute task",
			Description:   input,
			AssignedAgent: capabilities[0].Name,
			Input:         input,
			Status:        SubTaskReady,
			Index:         0,
		}},
	}, nil
}

func (o *Orchestrator) resolveAgents(selectedAgentNames []string, strict bool) ([]*SubAgent, error) {
	if len(selectedAgentNames) == 0 {
		if strict {
			return nil, fmt.Errorf("no target agents were provided")
		}
		return o.agentPool.List(), nil
	}
	agents := make([]*SubAgent, 0, len(selectedAgentNames))
	missing := make([]string, 0)
	for _, name := range selectedAgentNames {
		if sa, ok := o.agentPool.Get(name); ok {
			agents = append(agents, sa)
			continue
		}
		missing = append(missing, name)
	}
	if len(agents) == 0 {
		if strict {
			if len(missing) == 0 {
				return nil, fmt.Errorf("no target agents matched the handoff plan")
			}
			return nil, fmt.Errorf("unknown target agents: %s", strings.Join(missing, ", "))
		}
		return o.agentPool.List(), nil
	}
	if strict && len(missing) > 0 {
		return nil, fmt.Errorf("unknown target agents: %s", strings.Join(missing, ", "))
	}
	return agents, nil
}

func (o *Orchestrator) nextTaskID() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.taskCounter++
	return fmt.Sprintf("orch_%d", o.taskCounter)
}

func (o *Orchestrator) setLastExecution(exec *executionState, result *OrchestratorResult) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.lastRun = exec
	o.lastResult = cloneOrchestratorResult(result)
}

func (o *Orchestrator) buildTaskInput(exec *executionState, subTask *SubTask) string {
	var sb strings.Builder
	sb.WriteString(subTask.Input)

	depOutputs := exec.queue.GetDepOutputs(subTask.ID)
	if len(depOutputs) > 0 {
		sb.WriteString("\n\n--- dependency outputs ---\n")
		for depID, output := range depOutputs {
			for _, st := range exec.queue.GetAll() {
				if st.ID == depID {
					sb.WriteString(fmt.Sprintf("\n[%s result]:\n%s\n", st.Title, truncateString(output, 500)))
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

	if completed > 0 && failed == 0 {
		sb.WriteString("## Task Completed\n\n")
		for _, st := range subTasks {
			if st.Status == SubTaskCompleted && st.Output != "" {
				sb.WriteString(st.Output)
				sb.WriteString("\n\n")
			}
		}
		return sb.String()
	}

	sb.WriteString("## Task Execution Report\n\n")
	sb.WriteString(fmt.Sprintf("**Original request**: %s\n", originalInput))
	sb.WriteString(fmt.Sprintf("**Plan**: %s\n\n", planSummary))

	for _, st := range subTasks {
		statusIcon := "-"
		switch st.Status {
		case SubTaskCompleted:
			statusIcon = "[done]"
		case SubTaskFailed:
			statusIcon = "[failed]"
		case SubTaskRunning:
			statusIcon = "[running]"
		}

		sb.WriteString(fmt.Sprintf("### %s %s -> %s\n", statusIcon, st.Title, st.AssignedAgent))
		if st.Output != "" {
			sb.WriteString(st.Output)
			sb.WriteString("\n\n")
		}
		if st.Error != "" {
			sb.WriteString(fmt.Sprintf("error: %s\n\n", st.Error))
		}
	}

	sb.WriteString(fmt.Sprintf("**Summary**: completed %d/%d, failed %d, total time %v\n",
		completed, len(subTasks), failed, totalDuration.Round(time.Millisecond)))
	return sb.String()
}

func (o *Orchestrator) Status() OrchestratorStatus {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.lastRun == nil {
		return StatusIdle
	}
	return o.lastRun.Status()
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
	if o.lastRun == nil {
		return nil
	}
	return o.lastRun.History()
}

func (o *Orchestrator) LastResult() *OrchestratorResult {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.lastResult == nil {
		return nil
	}
	return cloneOrchestratorResult(o.lastResult)
}

func (o *Orchestrator) InitWarnings() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.initWarnings) == 0 {
		return nil
	}
	return append([]string(nil), o.initWarnings...)
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

	return &PlannerResponse{Content: resp.Content}, nil
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
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.lastRun == nil {
		return nil
	}
	return o.lastRun.queue
}

func (o *Orchestrator) Lifecycle() *AgentLifecycle {
	return o.lifecycle
}

func (o *Orchestrator) RunTask(ctx context.Context, input string, agentNames []string) (string, error) {
	return o.RunWithAgents(ctx, input, agentNames)
}
