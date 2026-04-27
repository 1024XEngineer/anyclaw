package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFileGraphStoreSaveLoadListDelete(t *testing.T) {
	store, err := NewFileGraphStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileGraphStore: %v", err)
	}

	first := testStoreGraph("graph_one", "first")
	if err := store.SaveGraph(first); err != nil {
		t.Fatalf("SaveGraph first: %v", err)
	}
	time.Sleep(time.Millisecond)
	second := testStoreGraph("graph_two", "second")
	if err := store.SaveGraph(second); err != nil {
		t.Fatalf("SaveGraph second: %v", err)
	}

	loaded, err := store.LoadGraph("graph_one")
	if err != nil {
		t.Fatalf("LoadGraph: %v", err)
	}
	if loaded.ID != first.ID || loaded.Name != first.Name {
		t.Fatalf("loaded graph = %+v, want %+v", loaded, first)
	}

	graphs, err := store.ListGraphs()
	if err != nil {
		t.Fatalf("ListGraphs: %v", err)
	}
	if len(graphs) != 2 {
		t.Fatalf("graphs = %d, want 2", len(graphs))
	}
	if graphs[0].ID != "graph_two" {
		t.Fatalf("first graph = %q, want most recently saved graph_two", graphs[0].ID)
	}

	reopened, err := NewFileGraphStore(store.baseDir)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	if _, err := reopened.LoadGraph("graph_two"); err != nil {
		t.Fatalf("LoadGraph after reopen: %v", err)
	}

	if err := store.DeleteGraph("graph_one"); err != nil {
		t.Fatalf("DeleteGraph: %v", err)
	}
	if _, err := store.LoadGraph("graph_one"); err == nil {
		t.Fatal("expected deleted graph to be missing")
	}
}

func TestFileGraphStoreRejectsEscapingIDs(t *testing.T) {
	parent := t.TempDir()
	base := filepath.Join(parent, "store")
	store, err := NewFileGraphStore(base)
	if err != nil {
		t.Fatalf("NewFileGraphStore: %v", err)
	}

	graph := testStoreGraph("../escape", "escape")
	if err := store.SaveGraph(graph); err == nil {
		t.Fatal("expected graph ID traversal rejection")
	}
	if _, err := os.Stat(filepath.Join(parent, "escape.json")); !os.IsNotExist(err) {
		t.Fatalf("escape file stat err = %v, want not exist", err)
	}

	exec := NewExecutionContext("graph_one", nil)
	exec.ExecutionID = "..\\escape"
	if err := store.SaveExecution(exec); err == nil {
		t.Fatal("expected execution ID traversal rejection")
	}
}

func TestFileExecutionStoreSaveLoadListDelete(t *testing.T) {
	store, err := NewFileGraphStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileGraphStore: %v", err)
	}

	first := NewExecutionContext("graph_one", map[string]any{"name": "first"})
	first.ExecutionID = "exec_one"
	second := NewExecutionContext("graph_two", map[string]any{"name": "second"})
	second.ExecutionID = "exec_two"

	if err := store.SaveExecution(first); err != nil {
		t.Fatalf("SaveExecution first: %v", err)
	}
	if err := store.SaveExecution(second); err != nil {
		t.Fatalf("SaveExecution second: %v", err)
	}

	loaded, err := store.LoadExecution("exec_one")
	if err != nil {
		t.Fatalf("LoadExecution: %v", err)
	}
	if loaded.GraphID != "graph_one" || loaded.Inputs["name"] != "first" {
		t.Fatalf("loaded execution = %+v, want first execution", loaded)
	}

	filtered, err := store.ListExecutions("graph_two")
	if err != nil {
		t.Fatalf("ListExecutions: %v", err)
	}
	if len(filtered) != 1 || filtered[0].ExecutionID != "exec_two" {
		t.Fatalf("filtered executions = %+v, want exec_two only", filtered)
	}

	if err := store.DeleteExecution("exec_one"); err != nil {
		t.Fatalf("DeleteExecution: %v", err)
	}
	if _, err := store.LoadExecution("exec_one"); err == nil {
		t.Fatal("expected deleted execution to be missing")
	}
}

func TestCheckpointManagerRecover(t *testing.T) {
	store, err := NewFileGraphStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileGraphStore: %v", err)
	}
	manager := NewCheckpointManager(store)

	exec := NewExecutionContext("graph_one", nil)
	exec.ExecutionID = "exec_checkpoint"
	exec.Status = ExecutionFailed
	exec.Error = &ExecutionError{Code: "boom", Message: "failed"}
	if err := manager.Checkpoint(exec, "node_failed"); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}

	recovered, err := manager.Recover("exec_checkpoint")
	if err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if recovered.Status != ExecutionRunning || recovered.Error != nil {
		t.Fatalf("recovered = %+v, want running without error", recovered)
	}
	if len(recovered.Evidence) != 1 || recovered.Evidence[0].Type != "checkpoint" {
		t.Fatalf("evidence = %+v, want checkpoint evidence", recovered.Evidence)
	}

	recovered.Status = ExecutionCompleted
	if err := store.SaveExecution(recovered); err != nil {
		t.Fatalf("SaveExecution completed: %v", err)
	}
	_, err = manager.Recover("exec_checkpoint")
	if err == nil || !strings.Contains(err.Error(), "terminal state") {
		t.Fatalf("Recover completed err = %v, want terminal state error", err)
	}
}

func testStoreGraph(id, name string) *Graph {
	graph := NewGraph(name, "test graph")
	graph.ID = id
	graph.AddActionNode("run", "execute", "plugin", "action", map[string]any{"ok": true})
	return graph
}
