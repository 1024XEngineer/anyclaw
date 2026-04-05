package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// FileGraphStore implements GraphStore using the filesystem.
// Each graph is stored as a JSON file in the graphs directory.
// Execution states are stored separately for checkpoint/recovery.
type FileGraphStore struct {
	baseDir   string
	mu        sync.RWMutex
	index     map[string]*graphIndexEntry
	execStore *FileExecutionStore
}

type graphIndexEntry struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Version   string    `json:"version"`
	UpdatedAt time.Time `json:"updated_at"`
	Path      string    `json:"path"`
}

// FileExecutionStore persists execution contexts for checkpoint/recovery.
type FileExecutionStore struct {
	baseDir string
	mu      sync.RWMutex
}

// NewFileGraphStore creates a new file-based graph store.
func NewFileGraphStore(baseDir string) (*FileGraphStore, error) {
	if baseDir == "" {
		baseDir = ".anyclaw/workflows"
	}

	graphsDir := filepath.Join(baseDir, "graphs")
	execDir := filepath.Join(baseDir, "executions")

	for _, dir := range []string{graphsDir, execDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	store := &FileGraphStore{
		baseDir:   baseDir,
		index:     make(map[string]*graphIndexEntry),
		execStore: &FileExecutionStore{baseDir: execDir},
	}

	if err := store.loadIndex(); err != nil {
		return nil, fmt.Errorf("failed to load index: %w", err)
	}

	return store, nil
}

func (s *FileGraphStore) graphsDir() string {
	return filepath.Join(s.baseDir, "graphs")
}

func (s *FileGraphStore) loadIndex() error {
	entries, err := os.ReadDir(s.graphsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(s.graphsDir(), entry.Name()))
		if err != nil {
			continue
		}

		var graph Graph
		if err := json.Unmarshal(data, &graph); err != nil {
			continue
		}

		s.index[graph.ID] = &graphIndexEntry{
			ID:        graph.ID,
			Name:      graph.Name,
			Version:   graph.Version,
			UpdatedAt: graph.UpdatedAt,
			Path:      filepath.Join(s.graphsDir(), entry.Name()),
		}
	}

	return nil
}

func (s *FileGraphStore) SaveGraph(graph *Graph) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	graph.UpdatedAt = time.Now().UTC()

	data, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal graph: %w", err)
	}

	filename := graph.ID + ".json"
	path := filepath.Join(s.graphsDir(), filename)

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("failed to write graph file: %w", err)
	}

	s.index[graph.ID] = &graphIndexEntry{
		ID:        graph.ID,
		Name:      graph.Name,
		Version:   graph.Version,
		UpdatedAt: graph.UpdatedAt,
		Path:      path,
	}

	return nil
}

func (s *FileGraphStore) LoadGraph(graphID string) (*Graph, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, ok := s.index[graphID]
	if !ok {
		return nil, fmt.Errorf("graph not found: %s", graphID)
	}

	data, err := os.ReadFile(entry.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to read graph file: %w", err)
	}

	var graph Graph
	if err := json.Unmarshal(data, &graph); err != nil {
		return nil, fmt.Errorf("failed to unmarshal graph: %w", err)
	}

	return &graph, nil
}

func (s *FileGraphStore) DeleteGraph(graphID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.index[graphID]
	if !ok {
		return fmt.Errorf("graph not found: %s", graphID)
	}

	if err := os.Remove(entry.Path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete graph file: %w", err)
	}

	delete(s.index, graphID)
	return nil
}

func (s *FileGraphStore) ListGraphs() ([]*Graph, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var graphs []*Graph
	for _, entry := range s.index {
		graph, err := s.loadGraphFromFile(entry.Path)
		if err != nil {
			continue
		}
		graphs = append(graphs, graph)
	}

	sort.Slice(graphs, func(i, j int) bool {
		return graphs[i].UpdatedAt.After(graphs[j].UpdatedAt)
	})

	return graphs, nil
}

func (s *FileGraphStore) loadGraphFromFile(path string) (*Graph, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var graph Graph
	if err := json.Unmarshal(data, &graph); err != nil {
		return nil, err
	}

	return &graph, nil
}

// ExecutionStore methods

func (s *FileGraphStore) SaveExecution(exec *ExecutionContext) error {
	return s.execStore.Save(exec)
}

func (s *FileGraphStore) LoadExecution(executionID string) (*ExecutionContext, error) {
	return s.execStore.Load(executionID)
}

func (s *FileGraphStore) ListExecutions(graphID string) ([]*ExecutionContext, error) {
	return s.execStore.List(graphID)
}

func (s *FileGraphStore) DeleteExecution(executionID string) error {
	return s.execStore.Delete(executionID)
}

func (s *FileExecutionStore) execDir() string {
	return s.baseDir
}

func (s *FileExecutionStore) Save(exec *ExecutionContext) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(exec, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal execution: %w", err)
	}

	filename := exec.ExecutionID + ".json"
	path := filepath.Join(s.execDir(), filename)

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("failed to write execution file: %w", err)
	}

	return nil
}

func (s *FileExecutionStore) Load(executionID string) (*ExecutionContext, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	filename := executionID + ".json"
	path := filepath.Join(s.execDir(), filename)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("execution not found: %s", executionID)
		}
		return nil, fmt.Errorf("failed to read execution file: %w", err)
	}

	var exec ExecutionContext
	if err := json.Unmarshal(data, &exec); err != nil {
		return nil, fmt.Errorf("failed to unmarshal execution: %w", err)
	}

	return &exec, nil
}

func (s *FileExecutionStore) List(graphID string) ([]*ExecutionContext, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(s.execDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var executions []*ExecutionContext
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(s.execDir(), entry.Name()))
		if err != nil {
			continue
		}

		var exec ExecutionContext
		if err := json.Unmarshal(data, &exec); err != nil {
			continue
		}

		if graphID != "" && exec.GraphID != graphID {
			continue
		}

		executions = append(executions, &exec)
	}

	sort.Slice(executions, func(i, j int) bool {
		return executions[i].StartTime.After(executions[j].StartTime)
	})

	return executions, nil
}

func (s *FileExecutionStore) Delete(executionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	filename := executionID + ".json"
	path := filepath.Join(s.execDir(), filename)

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete execution file: %w", err)
	}

	return nil
}

// CheckpointManager handles workflow checkpoint and recovery.
type CheckpointManager struct {
	store *FileGraphStore
}

// NewCheckpointManager creates a new checkpoint manager.
func NewCheckpointManager(store *FileGraphStore) *CheckpointManager {
	return &CheckpointManager{store: store}
}

// Checkpoint saves the current execution state as a checkpoint.
func (cm *CheckpointManager) Checkpoint(exec *ExecutionContext, checkpointType string) error {
	exec.AddEvidence("checkpoint", fmt.Sprintf("checkpoint: %s", checkpointType), map[string]any{
		"type":         checkpointType,
		"current_node": exec.CurrentNode,
		"status":       string(exec.Status),
		"node_count":   len(exec.NodeStates),
	})

	return cm.store.SaveExecution(exec)
}

// Recover restores an execution from a checkpoint.
func (cm *CheckpointManager) Recover(executionID string) (*ExecutionContext, error) {
	exec, err := cm.store.LoadExecution(executionID)
	if err != nil {
		return nil, fmt.Errorf("failed to load execution: %w", err)
	}

	if exec.Status == ExecutionCompleted || exec.Status == ExecutionCancelled {
		return nil, fmt.Errorf("cannot recover from terminal state: %s", exec.Status)
	}

	exec.Status = ExecutionRunning
	exec.Error = nil

	return exec, nil
}

// ListCheckpoints returns all checkpointed executions for a graph.
func (cm *CheckpointManager) ListCheckpoints(graphID string) ([]*ExecutionContext, error) {
	return cm.store.ListExecutions(graphID)
}

// ResumeExecution resumes a paused or failed execution from the last checkpoint.
func (e *WorkflowExecutor) ResumeExecution(exec *ExecutionContext, graph *Graph) (*ExecutionContext, error) {
	if exec.Status == ExecutionCompleted || exec.Status == ExecutionCancelled {
		return nil, fmt.Errorf("cannot resume from terminal state: %s", exec.Status)
	}

	exec.Status = ExecutionRunning

	go e.resumeGraphAsync(graph, exec)

	return exec, nil
}

func (e *WorkflowExecutor) resumeGraphAsync(graph *Graph, ctx *ExecutionContext) {
	defer func() {
		if r := recover(); r != nil {
			ctx.Status = ExecutionFailed
			ctx.Error = &ExecutionError{
				Code:    "panic",
				Message: fmt.Sprintf("panic during resume: %v", r),
			}
		}
	}()

	// Find the first incomplete node
	for _, node := range graph.Nodes {
		state, exists := ctx.NodeStates[node.ID]
		if !exists || (state.Status != NodeCompleted && state.Status != NodeSkipped) {
			if err := e.executeNode(graph, ctx, &node); err != nil {
				ctx.Status = ExecutionFailed
				ctx.Error = &ExecutionError{
					Code:    "resume_failed",
					Message: err.Error(),
					NodeID:  node.ID,
				}
				return
			}
		}
	}

	ctx.MarkExecutionCompleted(ctx.Outputs)
}
