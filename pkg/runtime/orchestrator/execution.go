package orchestrator

import (
	"sync"
	"time"
)

type executionState struct {
	id        string
	queue     *TaskQueue
	startedAt time.Time

	mu      sync.RWMutex
	status  OrchestratorStatus
	history []ExecutionLog
}

func newExecutionState(taskID string) *executionState {
	return &executionState{
		id:        taskID,
		queue:     NewTaskQueue(),
		startedAt: time.Now(),
		status:    StatusIdle,
		history:   make([]ExecutionLog, 0, 16),
	}
}

func (e *executionState) SetStatus(status OrchestratorStatus) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.status = status
}

func (e *executionState) Status() OrchestratorStatus {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.status
}

func (e *executionState) Log(level string, message string, agentName string, taskID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.history = append(e.history, ExecutionLog{
		Timestamp: time.Now(),
		Level:     level,
		Message:   message,
		AgentName: agentName,
		TaskID:    taskID,
	})
}

func (e *executionState) History() []ExecutionLog {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result := make([]ExecutionLog, len(e.history))
	copy(result, e.history)
	return result
}

func (e *executionState) Snapshot(summary string, totalTime time.Duration) OrchestratorResult {
	pending, ready, running, completed, failed := e.queue.Stats()
	return OrchestratorResult{
		TaskID:    e.id,
		Status:    e.Status(),
		Summary:   summary,
		SubTasks:  cloneSubTasks(e.queue.GetAll()),
		Stats:     TaskStats{Total: pending + ready + running + completed + failed, Pending: pending, Ready: ready, Running: running, Completed: completed, Failed: failed},
		TotalTime: totalTime,
		StartedAt: e.startedAt,
		History:   e.History(),
	}
}

func cloneSubTasks(items []*SubTask) []SubTask {
	if len(items) == 0 {
		return nil
	}
	result := make([]SubTask, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		copyItem := *item
		if len(item.DependsOn) > 0 {
			copyItem.DependsOn = append([]string(nil), item.DependsOn...)
		}
		result = append(result, copyItem)
	}
	return result
}

func cloneOrchestratorResult(result *OrchestratorResult) *OrchestratorResult {
	if result == nil {
		return nil
	}
	cloned := *result
	if len(result.SubTasks) > 0 {
		cloned.SubTasks = append([]SubTask(nil), result.SubTasks...)
		for i := range cloned.SubTasks {
			if len(result.SubTasks[i].DependsOn) > 0 {
				cloned.SubTasks[i].DependsOn = append([]string(nil), result.SubTasks[i].DependsOn...)
			}
		}
	}
	if len(result.History) > 0 {
		cloned.History = append([]ExecutionLog(nil), result.History...)
	}
	return &cloned
}
