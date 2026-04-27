package workflow

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGraphAddNodesEdgesAndValidate(t *testing.T) {
	graph := NewGraph("demo", "test graph")
	readID := graph.AddActionNode("read", "read input", "files", "read", map[string]any{
		"path": "$input_file",
	})
	checkID := graph.AddConditionNode("check", "check result", "$read.success == true")
	edgeID := graph.AddEdge(readID, checkID, "")

	if readID == "" || checkID == "" || edgeID == "" {
		t.Fatalf("expected generated IDs, got read=%q check=%q edge=%q", readID, checkID, edgeID)
	}
	if readID == checkID {
		t.Fatalf("node IDs should be unique, both were %q", readID)
	}
	if graph.Edges[0].Type != "default" {
		t.Fatalf("edge type = %q, want default", graph.Edges[0].Type)
	}
	if err := graph.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	next := graph.GetNextNodes(readID)
	if len(next) != 1 || next[0] != checkID {
		t.Fatalf("next = %v, want %q", next, checkID)
	}
	previous := graph.GetPreviousNodes(checkID)
	if len(previous) != 1 || previous[0] != readID {
		t.Fatalf("previous = %v, want %q", previous, readID)
	}
	start := graph.GetStartNodes()
	if len(start) != 1 || start[0].ID != readID {
		t.Fatalf("start nodes = %+v, want %q", start, readID)
	}
}

func TestGraphAddNodeDefensiveCopy(t *testing.T) {
	graph := NewGraph("copy", "")
	inputs := map[string]any{
		"nested": map[string]any{"value": "before"},
	}
	nodeID := graph.AddNode(Node{
		Type:   "action",
		Name:   "copy",
		Plugin: "plugin",
		Action: "run",
		Inputs: inputs,
	})
	inputs["nested"].(map[string]any)["value"] = "after"

	node, ok := graph.GetNodeByID(nodeID)
	if !ok {
		t.Fatalf("node %q not found", nodeID)
	}
	nested := node.Inputs["nested"].(map[string]any)
	if nested["value"] != "before" {
		t.Fatalf("stored input = %v, want defensive copy", nested["value"])
	}
	node.Inputs["nested"].(map[string]any)["value"] = "mutated"

	node, ok = graph.GetNodeByID(nodeID)
	if !ok {
		t.Fatalf("node %q not found after mutation", nodeID)
	}
	nested = node.Inputs["nested"].(map[string]any)
	if nested["value"] != "before" {
		t.Fatalf("GetNodeByID leaked mutable state, got %v", nested["value"])
	}
}

func TestGraphValidateRejectsInvalidGraphs(t *testing.T) {
	tests := []struct {
		name  string
		graph *Graph
		want  string
	}{
		{
			name:  "nil",
			graph: nil,
			want:  "graph is nil",
		},
		{
			name:  "missing name",
			graph: &Graph{Nodes: []Node{{ID: "n1", Type: "join", Name: "join"}}},
			want:  "graph name is required",
		},
		{
			name:  "no nodes",
			graph: NewGraph("empty", ""),
			want:  "at least one node",
		},
		{
			name: "duplicate node",
			graph: &Graph{
				Name: "duplicate",
				Nodes: []Node{
					{ID: "n1", Type: "join", Name: "join"},
					{ID: "n1", Type: "join", Name: "join again"},
				},
			},
			want: "duplicate node ID",
		},
		{
			name: "missing action plugin",
			graph: &Graph{
				Name:  "bad action",
				Nodes: []Node{{ID: "n1", Type: "action", Name: "run", Action: "go"}},
			},
			want: "action node must have plugin",
		},
		{
			name: "missing edge target",
			graph: &Graph{
				Name:  "bad edge",
				Nodes: []Node{{ID: "n1", Type: "join", Name: "join"}},
				Edges: []Edge{{ID: "e1", Source: "n1", Target: "missing"}},
			},
			want: "edge target node not found",
		},
		{
			name: "no start node",
			graph: &Graph{
				Name: "cycle",
				Nodes: []Node{
					{ID: "n1", Type: "join", Name: "one"},
					{ID: "n2", Type: "join", Name: "two"},
				},
				Edges: []Edge{
					{ID: "e1", Source: "n1", Target: "n2"},
					{ID: "e2", Source: "n2", Target: "n1"},
				},
			},
			want: "at least one start node",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.graph.Validate()
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestGraphJSONRoundTrip(t *testing.T) {
	graph := NewGraph("roundtrip", "json")
	graph.Version = "1.0.0"
	graph.Tags = []string{"workflow", "test"}
	graph.Metadata["owner"] = "anyclaw"
	graph.AddInputParam("path", "string", "input path", true, nil)
	graph.AddVariable("result", "object", "result object", map[string]any{"ok": true})
	graph.AddActionNode("run", "execute", "plugin", "action", map[string]any{"path": "$path"})

	data, err := graph.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON: %v", err)
	}
	if !json.Valid(data) {
		t.Fatalf("invalid json: %s", string(data))
	}

	loaded, err := FromJSON(data)
	if err != nil {
		t.Fatalf("FromJSON: %v", err)
	}
	if loaded.ID != graph.ID || loaded.Name != graph.Name || loaded.Metadata["owner"] != "anyclaw" {
		t.Fatalf("loaded graph = %+v, want matching graph", loaded)
	}
	if err := loaded.Validate(); err != nil {
		t.Fatalf("loaded Validate: %v", err)
	}
}

func TestExecutionContextResolveInputs(t *testing.T) {
	ctx := NewExecutionContext("graph-1", map[string]any{
		"input_file": "data.csv",
	})
	ctx.Variables["limit"] = 10
	ctx.NodeStates["fetch"] = &NodeState{
		NodeID: "fetch",
		Outputs: map[string]any{
			"success": true,
		},
	}
	node := &Node{
		Inputs: map[string]any{
			"path":    "$input_file",
			"limit":   "$limit",
			"ok":      "$fetch.success",
			"missing": "$missing",
			"nested": map[string]any{
				"again": "$input_file",
			},
			"list": []any{"$limit", "literal"},
		},
	}

	resolved := ctx.ResolveInputs(node, nil)
	if resolved["path"] != "data.csv" {
		t.Fatalf("path = %v, want data.csv", resolved["path"])
	}
	if resolved["limit"] != 10 {
		t.Fatalf("limit = %v, want 10", resolved["limit"])
	}
	if resolved["ok"] != true {
		t.Fatalf("ok = %v, want true", resolved["ok"])
	}
	if resolved["missing"] != "$missing" {
		t.Fatalf("missing = %v, want unresolved reference", resolved["missing"])
	}
	nested := resolved["nested"].(map[string]any)
	if nested["again"] != "data.csv" {
		t.Fatalf("nested again = %v, want data.csv", nested["again"])
	}
	list := resolved["list"].([]any)
	if list[0] != 10 || list[1] != "literal" {
		t.Fatalf("list = %v, want resolved values", list)
	}
}

func TestExecutionContextStateTransitions(t *testing.T) {
	ctx := NewExecutionContext("graph-1", map[string]any{"x": "y"})
	ctx.Inputs["x"] = "mutated"

	if ctx.Status != ExecutionPending || ctx.ExecutionID == "" {
		t.Fatalf("initial context = %+v", ctx)
	}
	if ctx.Inputs["x"] != "mutated" {
		t.Fatal("expected local context input to be mutable")
	}

	ctx.MarkNodeStarted("n1", map[string]any{"in": "value"})
	if ctx.Status != ExecutionRunning || ctx.CurrentNode != "n1" {
		t.Fatalf("started context = %+v", ctx)
	}
	if ctx.NodeStates["n1"].Attempts != 1 {
		t.Fatalf("attempts = %d, want 1", ctx.NodeStates["n1"].Attempts)
	}

	ctx.MarkNodeRetrying("n1")
	if ctx.NodeStates["n1"].Status != NodeRetrying || ctx.NodeStates["n1"].Attempts != 2 {
		t.Fatalf("retrying state = %+v", ctx.NodeStates["n1"])
	}

	ctx.MarkNodeCompleted("n1", map[string]any{"out": "done"})
	if ctx.NodeStates["n1"].Status != NodeCompleted || ctx.NodeStates["n1"].EndTime == nil {
		t.Fatalf("completed state = %+v", ctx.NodeStates["n1"])
	}

	ctx.AddEvidence("checkpoint", "saved", map[string]any{"node": "n1"})
	if len(ctx.Evidence) != 1 || ctx.Evidence[0].Data["node"] != "n1" {
		t.Fatalf("evidence = %+v, want checkpoint", ctx.Evidence)
	}

	ctx.MarkExecutionCompleted(map[string]any{"success": true})
	if !ctx.IsCompleted() || ctx.Status != ExecutionCompleted || ctx.EndTime == nil {
		t.Fatalf("completed context = %+v", ctx)
	}
}

func TestExecutionContextMarkNodeFailed(t *testing.T) {
	ctx := NewExecutionContext("graph-1", nil)
	ctx.MarkNodeFailed("n1", &NodeError{
		Code:    "boom",
		Message: "failed",
	})

	if ctx.Status != ExecutionFailed || !ctx.IsCompleted() {
		t.Fatalf("failed context = %+v", ctx)
	}
	if ctx.Error == nil || ctx.Error.Code != "boom" || ctx.Error.NodeID != "n1" {
		t.Fatalf("execution error = %+v, want boom on n1", ctx.Error)
	}
	if ctx.NodeStates["n1"].Status != NodeFailed || ctx.NodeStates["n1"].EndTime == nil {
		t.Fatalf("node state = %+v, want failed", ctx.NodeStates["n1"])
	}
}
