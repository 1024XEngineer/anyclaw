package task

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/anyclaw/anyclaw/pkg/orchestrator"
)

type ExecutionMode string

const (
	ModeSingle ExecutionMode = "single"
	ModeMulti  ExecutionMode = "multi"
)

type TaskState string

const (
	StatePending   TaskState = "pending"
	StateRunning   TaskState = "running"
	StateCompleted TaskState = "completed"
	StateFailed    TaskState = "failed"
	StateCancelled TaskState = "cancelled"
)

type AgentInfo = orchestrator.AgentInfo

type TaskRequest struct {
	ID             string        `json:"id"`
	Title          string        `json:"title"`
	Input          string        `json:"input"`
	Mode           ExecutionMode `json:"mode"`
	SelectedAgent  string        `json:"selected_agent,omitempty"`
	SelectedAgents []string      `json:"selected_agents,omitempty"`
	SessionID      string        `json:"session_id,omitempty"`
	Workspace      string        `json:"workspace,omitempty"`
	Priority       int           `json:"priority"`
	CreatedAt      time.Time     `json:"created_at"`
}

type TaskResponse struct {
	ID           string        `json:"id"`
	Title        string        `json:"title"`
	State        TaskState     `json:"state"`
	Mode         ExecutionMode `json:"mode"`
	Input        string        `json:"input"`
	Output       string        `json:"output,omitempty"`
	Error        string        `json:"error,omitempty"`
	AgentResults []AgentResult `json:"agent_results,omitempty"`
	CreatedAt    time.Time     `json:"created_at"`
	StartedAt    *time.Time    `json:"started_at,omitempty"`
	CompletedAt  *time.Time    `json:"completed_at,omitempty"`
	Duration     string        `json:"duration,omitempty"`
}

type AgentResult struct {
	AgentName string    `json:"agent_name"`
	TaskID    string    `json:"task_id,omitempty"`
	Input     string    `json:"input"`
	Output    string    `json:"output"`
	Error     string    `json:"error,omitempty"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Duration  string    `json:"duration"`
}

type TaskManager interface {
	CreateTask(req TaskRequest) (*TaskResponse, error)
	ExecuteTask(ctx context.Context, taskID string) (*TaskResponse, error)
	GetTask(taskID string) (*TaskResponse, error)
	ListTasks() []TaskResponse
	CancelTask(taskID string) error
	ListAgents() []AgentInfo
	GetAgent(name string) (*AgentInfo, error)
}

type taskManager struct {
	mu        sync.RWMutex
	tasks     map[string]*taskEntry
	orch      *orchestrator.Orchestrator
	idCounter int
}

type taskEntry struct {
	request  TaskRequest
	response TaskResponse
	cancel   context.CancelFunc
	running  bool
}

func NewTaskManager(orch *orchestrator.Orchestrator) TaskManager {
	return &taskManager{
		tasks: make(map[string]*taskEntry),
		orch:  orch,
	}
}

func (m *taskManager) CreateTask(req TaskRequest) (*TaskResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.orch == nil {
		return nil, fmt.Errorf("orchestrator not available")
	}

	m.idCounter++
	if req.ID == "" {
		req.ID = fmt.Sprintf("task_%d_%d", time.Now().UnixNano(), m.idCounter)
	}
	if req.Title == "" {
		req.Title = shortenInput(req.Input)
	}
	if req.Mode == "" {
		req.Mode = ModeSingle
	}
	req.CreatedAt = time.Now()
	if req.Mode == ModeMulti && m.orch == nil {
		return nil, fmt.Errorf("orchestrator not available for multi-agent mode")
	}

	// Validate agent selection
	if req.Mode == ModeSingle {
		if req.SelectedAgent == "" {
			req.SelectedAgent = m.findBestAgent(req.Input)
		}
		if req.SelectedAgent == "" {
			return nil, fmt.Errorf("no agent available")
		}
		if _, ok := m.orch.GetAgent(req.SelectedAgent); !ok {
			return nil, fmt.Errorf("agent not found: %s", req.SelectedAgent)
		}
	}

	resp := TaskResponse{
		ID:        req.ID,
		Title:     req.Title,
		State:     StatePending,
		Mode:      req.Mode,
		Input:     req.Input,
		CreatedAt: req.CreatedAt,
	}

	m.tasks[req.ID] = &taskEntry{
		request:  req,
		response: resp,
	}

	return &resp, nil
}

func (m *taskManager) ExecuteTask(ctx context.Context, taskID string) (*TaskResponse, error) {
	m.mu.Lock()
	entry, ok := m.tasks[taskID]
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("task not found: %s", taskID)
	}
	if entry.running {
		m.mu.Unlock()
		return &entry.response, fmt.Errorf("task %s is already running", taskID)
	}
	entry.running = true
	ctx, cancel := context.WithCancel(ctx)
	entry.cancel = cancel
	entry.response.State = StateRunning
	now := time.Now()
	entry.response.StartedAt = &now
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		entry.running = false
		m.mu.Unlock()
		if cancel != nil {
			cancel()
		}
	}()

	var err error
	if entry.request.Mode == ModeSingle {
		err = m.executeSingle(ctx, entry)
	} else {
		err = m.executeMulti(ctx, entry)
	}

	m.mu.Lock()
	endTime := time.Now()
	entry.response.CompletedAt = &endTime
	if entry.response.StartedAt != nil {
		entry.response.Duration = endTime.Sub(*entry.response.StartedAt).Round(time.Millisecond).String()
	}
	// Don't overwrite if cancelled
	if entry.response.State == StateCancelled {
		m.mu.Unlock()
		return &entry.response, fmt.Errorf("task was cancelled")
	}
	if err != nil {
		entry.response.State = StateFailed
		entry.response.Error = err.Error()
	} else {
		entry.response.State = StateCompleted
	}
	m.mu.Unlock()

	return &entry.response, nil
}

func (m *taskManager) executeSingle(ctx context.Context, entry *taskEntry) error {
	sa, ok := m.orch.GetAgent(entry.request.SelectedAgent)
	if !ok {
		return fmt.Errorf("agent %q not available", entry.request.SelectedAgent)
	}

	startTime := time.Now()
	output, err := sa.Run(ctx, entry.request.Input)
	endTime := time.Now()

	result := AgentResult{
		AgentName: sa.Name(),
		Input:     entry.request.Input,
		Output:    output,
		StartTime: startTime,
		EndTime:   endTime,
		Duration:  endTime.Sub(startTime).Round(time.Millisecond).String(),
	}
	if err != nil {
		result.Error = err.Error()
	}

	m.mu.Lock()
	entry.response.AgentResults = []AgentResult{result}
	entry.response.Output = output
	m.mu.Unlock()

	return err
}

func (m *taskManager) executeMulti(ctx context.Context, entry *taskEntry) error {
	output, err := m.orch.RunWithAgents(ctx, entry.request.Input, entry.request.SelectedAgents)

	m.mu.Lock()
	entry.response.Output = output
	m.mu.Unlock()

	return err
}

func (m *taskManager) GetTask(taskID string) (*TaskResponse, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, ok := m.tasks[taskID]
	if !ok {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}

	resp := entry.response
	return &resp, nil
}

func (m *taskManager) ListTasks() []TaskResponse {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list := make([]TaskResponse, 0, len(m.tasks))
	for _, entry := range m.tasks {
		list = append(list, entry.response)
	}
	return list
}

func (m *taskManager) CancelTask(taskID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.tasks[taskID]
	if !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}

	if entry.response.State == StateCancelled ||
		entry.response.State == StateCompleted ||
		entry.response.State == StateFailed {
		return fmt.Errorf("task %s is already in terminal state: %s", taskID, entry.response.State)
	}

	entry.response.State = StateCancelled
	now := time.Now()
	entry.response.CompletedAt = &now
	entry.running = false

	if entry.cancel != nil {
		entry.cancel()
	}

	return nil
}

func (m *taskManager) ListAgents() []AgentInfo {
	if m.orch == nil {
		return nil
	}
	return m.orch.ListAgents()
}

func (m *taskManager) GetAgent(name string) (*AgentInfo, error) {
	if m.orch == nil {
		return nil, fmt.Errorf("orchestrator not available")
	}
	sa, ok := m.orch.GetAgent(name)
	if !ok {
		return nil, fmt.Errorf("agent not found: %s", name)
	}
	info := orchestrator.AgentInfo{
		Name:            sa.Name(),
		Description:     sa.Description(),
		Persona:         sa.Persona(),
		Domain:          sa.Domain(),
		Expertise:       sa.Expertise(),
		Skills:          sa.Skills(),
		PermissionLevel: sa.PermissionLevel(),
		ExecCount:       sa.ExecCount(),
	}
	return &info, nil
}

func (m *taskManager) findBestAgent(input string) string {
	if m.orch == nil {
		return ""
	}
	lower := strings.ToLower(input)

	// Try to match by domain/expertise keywords
	agents := m.orch.ListAgents()
	for _, a := range agents {
		// Match domain
		if a.Domain != "" && strings.Contains(lower, strings.ToLower(a.Domain)) {
			return a.Name
		}
		// Match expertise
		for _, exp := range a.Expertise {
			if strings.Contains(lower, strings.ToLower(exp)) {
				return a.Name
			}
		}
	}

	// Fallback: return first agent
	if len(agents) > 0 {
		return agents[0].Name
	}
	return ""
}

func (m *taskManager) findBestAgents(input string) []string {
	if m.orch == nil {
		return nil
	}
	lower := strings.ToLower(input)
	selected := make(map[string]bool)

	agents := m.orch.ListAgents()
	for _, a := range agents {
		if a.Domain != "" && strings.Contains(lower, strings.ToLower(a.Domain)) {
			selected[a.Name] = true
		}
		for _, exp := range a.Expertise {
			if strings.Contains(lower, strings.ToLower(exp)) {
				selected[a.Name] = true
			}
		}
	}

	if len(selected) == 0 {
		// Select all agents for multi-agent mode if no match
		for _, a := range agents {
			selected[a.Name] = true
		}
	}

	result := make([]string, 0, len(selected))
	for name := range selected {
		result = append(result, name)
	}
	return result
}

func shortenInput(input string) string {
	input = strings.TrimSpace(input)
	runes := []rune(input)
	if len(runes) <= 40 {
		return input
	}
	return string(runes[:40]) + "..."
}
