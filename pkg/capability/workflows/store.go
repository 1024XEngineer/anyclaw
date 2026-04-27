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

// FileGraphStore persists workflow graphs and execution checkpoints on disk.
type FileGraphStore struct {
	baseDir   string
	graphsDir string
	mu        sync.RWMutex
	index     map[string]graphIndexEntry
	execStore *FileExecutionStore
}

type graphIndexEntry struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Version   string    `json:"version,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
	Path      string    `json:"path"`
}

// FileExecutionStore persists execution contexts for checkpoint/recovery.
type FileExecutionStore struct {
	baseDir string
	mu      sync.RWMutex
}

// NewFileGraphStore creates a filesystem-backed workflow graph store.
func NewFileGraphStore(baseDir string) (*FileGraphStore, error) {
	if strings.TrimSpace(baseDir) == "" {
		baseDir = filepath.Join(".anyclaw", "workflows")
	}

	graphsDir := filepath.Join(baseDir, "graphs")
	execDir := filepath.Join(baseDir, "executions")
	for _, dir := range []string{graphsDir, execDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create workflow store directory %s: %w", dir, err)
		}
	}

	store := &FileGraphStore{
		baseDir:   baseDir,
		graphsDir: graphsDir,
		index:     make(map[string]graphIndexEntry),
		execStore: &FileExecutionStore{baseDir: execDir},
	}
	if err := store.loadIndex(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *FileGraphStore) loadIndex() error {
	entries, err := os.ReadDir(s.graphsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read workflow graph directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(s.graphsDir, entry.Name())
		graph, err := loadGraphFile(path)
		if err != nil || graph.ID == "" {
			continue
		}
		if _, err := safeStoreID(graph.ID); err != nil {
			continue
		}
		s.index[graph.ID] = graphIndexEntry{
			ID:        graph.ID,
			Name:      graph.Name,
			Version:   graph.Version,
			UpdatedAt: graph.UpdatedAt,
			Path:      path,
		}
	}
	return nil
}

// SaveGraph writes a graph snapshot to disk and updates the in-memory index.
func (s *FileGraphStore) SaveGraph(graph *Graph) error {
	if s == nil {
		return fmt.Errorf("graph store is nil")
	}
	if graph == nil {
		return fmt.Errorf("graph is nil")
	}

	path, err := s.graphPath(graph.ID)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	graph.UpdatedAt = time.Now().UTC()
	if err := writeJSONFile(path, graph); err != nil {
		return fmt.Errorf("write graph %s: %w", graph.ID, err)
	}

	s.index[graph.ID] = graphIndexEntry{
		ID:        graph.ID,
		Name:      graph.Name,
		Version:   graph.Version,
		UpdatedAt: graph.UpdatedAt,
		Path:      path,
	}
	return nil
}

// LoadGraph reads a graph by ID.
func (s *FileGraphStore) LoadGraph(graphID string) (*Graph, error) {
	if s == nil {
		return nil, fmt.Errorf("graph store is nil")
	}
	if _, err := safeStoreID(graphID); err != nil {
		return nil, err
	}

	s.mu.RLock()
	entry, ok := s.index[graphID]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("graph not found: %s", graphID)
	}

	graph, err := loadGraphFile(entry.Path)
	if err != nil {
		return nil, fmt.Errorf("load graph %s: %w", graphID, err)
	}
	return graph, nil
}

// DeleteGraph removes a graph from disk and the index.
func (s *FileGraphStore) DeleteGraph(graphID string) error {
	if s == nil {
		return fmt.Errorf("graph store is nil")
	}
	if _, err := safeStoreID(graphID); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.index[graphID]
	if !ok {
		return fmt.Errorf("graph not found: %s", graphID)
	}
	if err := os.Remove(entry.Path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete graph %s: %w", graphID, err)
	}
	delete(s.index, graphID)
	return nil
}

// ListGraphs returns graphs sorted by most recent update first.
func (s *FileGraphStore) ListGraphs() ([]*Graph, error) {
	if s == nil {
		return nil, fmt.Errorf("graph store is nil")
	}

	s.mu.RLock()
	entries := make([]graphIndexEntry, 0, len(s.index))
	for _, entry := range s.index {
		entries = append(entries, entry)
	}
	s.mu.RUnlock()

	graphs := make([]*Graph, 0, len(entries))
	for _, entry := range entries {
		graph, err := loadGraphFile(entry.Path)
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

// SaveExecution persists an execution context.
func (s *FileGraphStore) SaveExecution(exec *ExecutionContext) error {
	if s == nil {
		return fmt.Errorf("graph store is nil")
	}
	return s.execStore.Save(exec)
}

// LoadExecution reads an execution context by ID.
func (s *FileGraphStore) LoadExecution(executionID string) (*ExecutionContext, error) {
	if s == nil {
		return nil, fmt.Errorf("graph store is nil")
	}
	return s.execStore.Load(executionID)
}

// ListExecutions returns persisted executions, optionally filtered by graph ID.
func (s *FileGraphStore) ListExecutions(graphID string) ([]*ExecutionContext, error) {
	if s == nil {
		return nil, fmt.Errorf("graph store is nil")
	}
	return s.execStore.List(graphID)
}

// DeleteExecution removes a persisted execution.
func (s *FileGraphStore) DeleteExecution(executionID string) error {
	if s == nil {
		return fmt.Errorf("graph store is nil")
	}
	return s.execStore.Delete(executionID)
}

func (s *FileGraphStore) graphPath(graphID string) (string, error) {
	filename, err := safeStoreID(graphID)
	if err != nil {
		return "", err
	}
	return filepath.Join(s.graphsDir, filename+".json"), nil
}

// Save writes an execution context to disk.
func (s *FileExecutionStore) Save(exec *ExecutionContext) error {
	if s == nil {
		return fmt.Errorf("execution store is nil")
	}
	if exec == nil {
		return fmt.Errorf("execution context is nil")
	}
	path, err := s.executionPath(exec.ExecutionID)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := writeJSONFile(path, exec); err != nil {
		return fmt.Errorf("write execution %s: %w", exec.ExecutionID, err)
	}
	return nil
}

// Load reads an execution context by ID.
func (s *FileExecutionStore) Load(executionID string) (*ExecutionContext, error) {
	if s == nil {
		return nil, fmt.Errorf("execution store is nil")
	}
	path, err := s.executionPath(executionID)
	if err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("execution not found: %s", executionID)
		}
		return nil, fmt.Errorf("read execution %s: %w", executionID, err)
	}

	var exec ExecutionContext
	if err := json.Unmarshal(data, &exec); err != nil {
		return nil, fmt.Errorf("decode execution %s: %w", executionID, err)
	}
	return &exec, nil
}

// List returns executions sorted by start time descending.
func (s *FileExecutionStore) List(graphID string) ([]*ExecutionContext, error) {
	if s == nil {
		return nil, fmt.Errorf("execution store is nil")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read execution directory: %w", err)
	}

	executions := make([]*ExecutionContext, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.baseDir, entry.Name()))
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

// Delete removes an execution context from disk.
func (s *FileExecutionStore) Delete(executionID string) error {
	if s == nil {
		return fmt.Errorf("execution store is nil")
	}
	path, err := s.executionPath(executionID)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete execution %s: %w", executionID, err)
	}
	return nil
}

func (s *FileExecutionStore) executionPath(executionID string) (string, error) {
	filename, err := safeStoreID(executionID)
	if err != nil {
		return "", err
	}
	return filepath.Join(s.baseDir, filename+".json"), nil
}

// CheckpointManager handles workflow checkpoint and recovery persistence.
type CheckpointManager struct {
	store *FileGraphStore
}

// NewCheckpointManager creates a checkpoint manager for a graph store.
func NewCheckpointManager(store *FileGraphStore) *CheckpointManager {
	return &CheckpointManager{store: store}
}

// Checkpoint saves the current execution state with checkpoint evidence.
func (cm *CheckpointManager) Checkpoint(exec *ExecutionContext, checkpointType string) error {
	if cm == nil || cm.store == nil {
		return fmt.Errorf("checkpoint manager has no store")
	}
	if exec == nil {
		return fmt.Errorf("execution context is nil")
	}
	exec.AddEvidence("checkpoint", fmt.Sprintf("checkpoint: %s", checkpointType), map[string]any{
		"type":         checkpointType,
		"current_node": exec.CurrentNode,
		"status":       string(exec.Status),
		"node_count":   len(exec.NodeStates),
	})
	return cm.store.SaveExecution(exec)
}

// Recover loads a checkpoint and moves non-terminal executions back to running.
func (cm *CheckpointManager) Recover(executionID string) (*ExecutionContext, error) {
	if cm == nil || cm.store == nil {
		return nil, fmt.Errorf("checkpoint manager has no store")
	}
	exec, err := cm.store.LoadExecution(executionID)
	if err != nil {
		return nil, err
	}
	if exec.Status == ExecutionCompleted || exec.Status == ExecutionCancelled {
		return nil, fmt.Errorf("cannot recover from terminal state: %s", exec.Status)
	}
	exec.Status = ExecutionRunning
	exec.Error = nil
	return exec, nil
}

// ListCheckpoints returns all persisted checkpoints for a graph.
func (cm *CheckpointManager) ListCheckpoints(graphID string) ([]*ExecutionContext, error) {
	if cm == nil || cm.store == nil {
		return nil, fmt.Errorf("checkpoint manager has no store")
	}
	return cm.store.ListExecutions(graphID)
}

func safeStoreID(id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", fmt.Errorf("store ID is required")
	}
	if id == "." || id == ".." || filepath.IsAbs(id) || strings.ContainsAny(id, `/\`) {
		return "", fmt.Errorf("invalid store ID: %q", id)
	}
	if filepath.Clean(id) != id || filepath.Base(id) != id {
		return "", fmt.Errorf("invalid store ID: %q", id)
	}
	for _, r := range id {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.' {
			continue
		}
		return "", fmt.Errorf("invalid store ID: %q", id)
	}
	return id, nil
}

func loadGraphFile(path string) (*Graph, error) {
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

func writeJSONFile(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
			_ = os.Remove(tmp)
			return err
		}
		if renameErr := os.Rename(tmp, path); renameErr != nil {
			_ = os.Remove(tmp)
			return renameErr
		}
	}
	return nil
}
