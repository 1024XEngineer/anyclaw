package workflow

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

type workflowActionCall struct {
	NodeID string
	Inputs map[string]any
}

type recordingWorkflowActionRunner struct {
	calls    []workflowActionCall
	handlers map[string]func(map[string]any) (map[string]any, error)
}

func (r *recordingWorkflowActionRunner) RunAction(ctx context.Context, node Node, inputs map[string]any) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.calls = append(r.calls, workflowActionCall{
		NodeID: node.ID,
		Inputs: cloneAnyMap(inputs),
	})
	if r.handlers != nil {
		if handler := r.handlers[node.ID]; handler != nil {
			return handler(inputs)
		}
	}
	return map[string]any{"ok": true, "node_id": node.ID}, nil
}

func TestWorkflowExecutorExecutesActionsConditionsAndGraphOutputs(t *testing.T) {
	graph := NewGraph("approval", "")
	graph.ID = "graph-approval"
	graph.AddInputParam("threshold", "number", "", true, nil)
	graph.AddOutputParam("approved", "boolean", "", "$approve.approved")

	scoreID := graph.AddNode(Node{
		ID:     "score",
		Type:   "action",
		Name:   "Score request",
		Plugin: "policy",
		Action: "score",
	})
	checkID := graph.AddConditionNode("Check threshold", "", "$score.score >= $threshold")
	approveID := graph.AddNode(Node{
		ID:     "approve",
		Type:   "action",
		Name:   "Approve",
		Plugin: "policy",
		Action: "approve",
		Inputs: map[string]any{"score": "$score.score"},
	})
	rejectID := graph.AddNode(Node{
		ID:     "reject",
		Type:   "action",
		Name:   "Reject",
		Plugin: "policy",
		Action: "reject",
	})
	graph.AddEdge(scoreID, checkID, "default")
	graph.AddEdge(checkID, approveID, "condition_true")
	graph.AddEdge(checkID, rejectID, "condition_false")

	runner := &recordingWorkflowActionRunner{handlers: map[string]func(map[string]any) (map[string]any, error){
		"score": func(map[string]any) (map[string]any, error) {
			return map[string]any{"score": 0.91}, nil
		},
		"approve": func(inputs map[string]any) (map[string]any, error) {
			if inputs["score"] != 0.91 {
				t.Fatalf("approve score input = %#v, want 0.91", inputs["score"])
			}
			return map[string]any{"approved": true}, nil
		},
		"reject": func(map[string]any) (map[string]any, error) {
			t.Fatal("reject branch should not run")
			return nil, nil
		},
	}}
	executor := NewWorkflowExecutor(nil, nil)
	executor.SetActionRunner(runner)

	exec, err := executor.ExecuteGraph(graph, map[string]any{"threshold": 0.7})
	if err != nil {
		t.Fatalf("ExecuteGraph: %v", err)
	}
	if exec.Status != ExecutionCompleted {
		t.Fatalf("status = %s, want completed", exec.Status)
	}
	if exec.Outputs["approved"] != true {
		t.Fatalf("graph outputs = %#v, want approved=true", exec.Outputs)
	}
	if got := callNodeIDs(runner.calls); !reflect.DeepEqual(got, []string{"score", "approve"}) {
		t.Fatalf("action call order = %v, want score then approve", got)
	}
	if exec.NodeStates[checkID].Outputs["result"] != true {
		t.Fatalf("condition result = %#v, want true", exec.NodeStates[checkID].Outputs)
	}
}

func TestWorkflowExecutorFailsActionNodesWithoutRunner(t *testing.T) {
	graph := NewGraph("missing runner", "")
	graph.AddActionNode("run", "", "policy", "score", nil)

	exec, err := NewWorkflowExecutor(nil, nil).ExecuteGraph(graph, nil)
	if err == nil {
		t.Fatal("expected missing action runner error")
	}
	if exec == nil || exec.Status != ExecutionFailed {
		t.Fatalf("execution = %#v, want failed context", exec)
	}
	if !strings.Contains(err.Error(), "action runner is not configured") {
		t.Fatalf("error = %v, want action runner message", err)
	}
}

func TestWorkflowExecutorRetriesActionNodes(t *testing.T) {
	graph := NewGraph("retry", "")
	graph.AddNode(Node{
		ID:     "flaky",
		Type:   "action",
		Name:   "Flaky action",
		Plugin: "policy",
		Action: "flaky",
		RetryPolicy: &RetryPolicy{
			MaxAttempts: 2,
		},
	})

	attempts := 0
	runner := &recordingWorkflowActionRunner{handlers: map[string]func(map[string]any) (map[string]any, error){
		"flaky": func(map[string]any) (map[string]any, error) {
			attempts++
			if attempts == 1 {
				return nil, errors.New("temporary failure")
			}
			return map[string]any{"ok": true}, nil
		},
	}}
	executor := NewWorkflowExecutor(nil, nil)
	executor.SetActionRunner(runner)

	exec, err := executor.ExecuteGraph(graph, nil)
	if err != nil {
		t.Fatalf("ExecuteGraph: %v", err)
	}
	if exec.NodeStates["flaky"].Attempts != 2 {
		t.Fatalf("attempts = %d, want 2", exec.NodeStates["flaky"].Attempts)
	}
	if exec.NodeStates["flaky"].Status != NodeCompleted {
		t.Fatalf("state = %#v, want completed", exec.NodeStates["flaky"])
	}
}

func TestWorkflowExecutorAppliesNodeTimeoutToActionRunner(t *testing.T) {
	graph := NewGraph("timeout", "")
	graph.AddNode(Node{
		ID:         "timed",
		Type:       "action",
		Name:       "Timed",
		Plugin:     "policy",
		Action:     "run",
		TimeoutSec: 1,
	})
	executor := NewWorkflowExecutor(nil, nil)
	executor.SetActionRunner(WorkflowActionRunnerFunc(func(ctx context.Context, node Node, inputs map[string]any) (map[string]any, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("expected action context to include node timeout deadline")
		}
		return map[string]any{"ok": true}, nil
	}))

	if _, err := executor.ExecuteGraph(graph, nil); err != nil {
		t.Fatalf("ExecuteGraph: %v", err)
	}
}

func TestWorkflowExecutorMarksInterruptedContextAsCancelled(t *testing.T) {
	tests := []struct {
		name     string
		ctx      context.Context
		cancel   context.CancelFunc
		wantErr  error
		wantCode string
	}{
		{
			name:     "cancelled",
			wantErr:  context.Canceled,
			wantCode: "execution_cancelled",
		},
		{
			name:     "deadline exceeded",
			wantErr:  context.DeadlineExceeded,
			wantCode: "execution_deadline_exceeded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			graph := NewGraph("interrupted", "")
			graph.AddNode(Node{
				ID:     "run",
				Type:   "action",
				Name:   "Run",
				Plugin: "policy",
				Action: "run",
			})
			if tt.wantErr == context.Canceled {
				tt.ctx, tt.cancel = context.WithCancel(context.Background())
				tt.cancel()
			} else {
				tt.ctx, tt.cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
				defer tt.cancel()
			}

			executor := NewWorkflowExecutor(nil, nil)
			executor.SetActionRunner(&recordingWorkflowActionRunner{})

			exec, err := executor.ExecuteGraphContext(tt.ctx, graph, nil)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if exec == nil || exec.Status != ExecutionCancelled {
				t.Fatalf("execution = %#v, want cancelled status", exec)
			}
			if exec.Error == nil || exec.Error.Code != tt.wantCode {
				t.Fatalf("execution error = %#v, want %s", exec.Error, tt.wantCode)
			}
			if exec.EndTime == nil {
				t.Fatal("cancelled execution should record end time")
			}
		})
	}
}

func TestWorkflowExecutorConvertsActionPanicsToFailedExecutions(t *testing.T) {
	graph := NewGraph("panic", "")
	graph.AddNode(Node{
		ID:     "panic_node",
		Type:   "action",
		Name:   "Panic",
		Plugin: "policy",
		Action: "panic",
	})
	executor := NewWorkflowExecutor(nil, nil)
	executor.SetActionRunner(WorkflowActionRunnerFunc(func(context.Context, Node, map[string]any) (map[string]any, error) {
		panic("runner exploded")
	}))

	exec, err := executor.ExecuteGraph(graph, nil)
	if err == nil {
		t.Fatal("expected panic conversion error")
	}
	if exec == nil || exec.Status != ExecutionFailed {
		t.Fatalf("execution = %#v, want failed context", exec)
	}
	if exec.Error == nil || !strings.Contains(exec.Error.Message, "runner exploded") {
		t.Fatalf("execution error = %#v, want panic message", exec.Error)
	}
}

func TestWorkflowExecutorRunsJoinAfterAllParentsComplete(t *testing.T) {
	graph := NewGraph("join", "")
	startID := graph.AddNode(Node{ID: "start", Type: "action", Name: "Start", Plugin: "p", Action: "start"})
	fanoutID := graph.AddNode(Node{ID: "fanout", Type: "parallel", Name: "Fan out"})
	leftID := graph.AddNode(Node{ID: "left", Type: "action", Name: "Left", Plugin: "p", Action: "left"})
	rightID := graph.AddNode(Node{ID: "right", Type: "action", Name: "Right", Plugin: "p", Action: "right"})
	joinID := graph.AddNode(Node{ID: "join", Type: "join", Name: "Join"})
	finalID := graph.AddNode(Node{
		ID:     "final",
		Type:   "action",
		Name:   "Final",
		Plugin: "p",
		Action: "final",
		Inputs: map[string]any{"joined": "$join.completed_count"},
	})
	graph.AddEdge(startID, fanoutID, "default")
	graph.AddEdge(fanoutID, leftID, "branch")
	graph.AddEdge(fanoutID, rightID, "branch")
	graph.AddEdge(leftID, joinID, "default")
	graph.AddEdge(rightID, joinID, "default")
	graph.AddEdge(joinID, finalID, "default")

	runner := &recordingWorkflowActionRunner{handlers: map[string]func(map[string]any) (map[string]any, error){
		"left":  func(map[string]any) (map[string]any, error) { return map[string]any{"side": "left"}, nil },
		"right": func(map[string]any) (map[string]any, error) { return map[string]any{"side": "right"}, nil },
		"final": func(inputs map[string]any) (map[string]any, error) {
			if inputs["joined"] != 2 {
				t.Fatalf("final joined input = %#v, want 2", inputs["joined"])
			}
			return map[string]any{"done": true}, nil
		},
	}}
	executor := NewWorkflowExecutor(nil, nil)
	executor.SetActionRunner(runner)

	exec, err := executor.ExecuteGraph(graph, nil)
	if err != nil {
		t.Fatalf("ExecuteGraph: %v", err)
	}
	if exec.NodeStates[joinID].Outputs["completed_count"] != 2 {
		t.Fatalf("join outputs = %#v, want completed_count=2", exec.NodeStates[joinID].Outputs)
	}
	if exec.NodeStates[finalID].Status != NodeCompleted {
		t.Fatalf("final state = %#v, want completed", exec.NodeStates[finalID])
	}
}

func TestWorkflowExecutorLoopResolvesInputCollection(t *testing.T) {
	graph := NewGraph("loop", "")
	loopID := graph.AddNode(Node{
		ID:       "each_item",
		Type:     "loop",
		Name:     "Each item",
		LoopVar:  "item",
		LoopOver: "$items",
	})
	collectID := graph.AddNode(Node{
		ID:     "collect",
		Type:   "action",
		Name:   "Collect item",
		Plugin: "policy",
		Action: "collect",
		Inputs: map[string]any{
			"item":  "$item",
			"index": "$item_index",
			"count": "$item_count",
		},
	})
	graph.AddEdge(loopID, collectID, "default")

	runner := &recordingWorkflowActionRunner{}
	executor := NewWorkflowExecutor(nil, nil)
	executor.SetActionRunner(runner)

	exec, err := executor.ExecuteGraph(graph, map[string]any{
		"items": []any{"a", "b", "c"},
	})
	if err != nil {
		t.Fatalf("ExecuteGraph: %v", err)
	}
	outputs := exec.NodeStates["each_item"].Outputs
	if outputs["iterations"] != 3 {
		t.Fatalf("loop outputs = %#v, want 3 iterations", outputs)
	}
	if !reflect.DeepEqual(outputs["items"], []any{"a", "b", "c"}) {
		t.Fatalf("loop items = %#v, want original collection", outputs["items"])
	}
	if got := callNodeIDs(runner.calls); !reflect.DeepEqual(got, []string{"collect", "collect", "collect"}) {
		t.Fatalf("loop body calls = %v, want collect once per item", got)
	}
	for i, want := range []any{"a", "b", "c"} {
		if runner.calls[i].Inputs["item"] != want {
			t.Fatalf("call %d item = %#v, want %#v", i, runner.calls[i].Inputs["item"], want)
		}
		if runner.calls[i].Inputs["index"] != i {
			t.Fatalf("call %d index = %#v, want %d", i, runner.calls[i].Inputs["index"], i)
		}
		if runner.calls[i].Inputs["count"] != 3 {
			t.Fatalf("call %d count = %#v, want 3", i, runner.calls[i].Inputs["count"])
		}
	}
	if exec.Variables["item"] != nil || exec.Variables["item_index"] != nil {
		t.Fatalf("loop variables leaked after execution: %#v", exec.Variables)
	}
}

func TestWorkflowExecutorContinuesAfterLoopJoin(t *testing.T) {
	graph := NewGraph("loop join", "")
	loopID := graph.AddNode(Node{
		ID:       "loop",
		Type:     "loop",
		Name:     "Loop",
		LoopVar:  "item",
		LoopOver: "$items",
	})
	bodyID := graph.AddNode(Node{
		ID:     "body",
		Type:   "action",
		Name:   "Body",
		Plugin: "policy",
		Action: "body",
		Inputs: map[string]any{"item": "$item"},
	})
	joinID := graph.AddNode(Node{
		ID:   "join",
		Type: "join",
		Name: "Join",
	})
	finalID := graph.AddNode(Node{
		ID:     "final",
		Type:   "action",
		Name:   "Final",
		Plugin: "policy",
		Action: "final",
		Inputs: map[string]any{"iterations": "$loop.iterations"},
	})
	graph.AddEdge(loopID, bodyID, "each")
	graph.AddEdge(loopID, joinID, "default")
	graph.AddEdge(joinID, finalID, "default")

	runner := &recordingWorkflowActionRunner{}
	executor := NewWorkflowExecutor(nil, nil)
	executor.SetActionRunner(runner)

	exec, err := executor.ExecuteGraph(graph, map[string]any{"items": []any{"x", "y"}})
	if err != nil {
		t.Fatalf("ExecuteGraph: %v", err)
	}
	if got := callNodeIDs(runner.calls); !reflect.DeepEqual(got, []string{"body", "body", "final"}) {
		t.Fatalf("calls = %v, want loop body twice then final", got)
	}
	if runner.calls[2].Inputs["iterations"] != 2 {
		t.Fatalf("final iterations input = %#v, want 2", runner.calls[2].Inputs["iterations"])
	}
	if exec.NodeStates[bodyID] != nil {
		t.Fatalf("loop body state should not be retained as final singleton state: %#v", exec.NodeStates[bodyID])
	}
	if exec.NodeStates[joinID].Status != NodeCompleted {
		t.Fatalf("join state = %#v, want completed", exec.NodeStates[joinID])
	}
}

func TestWorkflowExecutorExecutesStoredGraphByID(t *testing.T) {
	graph := NewGraph("stored", "")
	graph.ID = "graph-stored"
	graph.AddNode(Node{
		ID:     "run",
		Type:   "action",
		Name:   "Run",
		Plugin: "policy",
		Action: "run",
		Inputs: map[string]any{"request": "$request"},
	})
	store := &fakeGraphStore{graphs: map[string]*Graph{graph.ID: graph}}
	runner := &recordingWorkflowActionRunner{}
	executor := NewWorkflowExecutor(nil, store)
	executor.SetActionRunner(runner)

	exec, err := executor.ExecuteStoredGraph(context.Background(), graph.ID, map[string]any{"request": "ok"})
	if err != nil {
		t.Fatalf("ExecuteStoredGraph: %v", err)
	}
	if exec.GraphID != graph.ID || exec.Status != ExecutionCompleted {
		t.Fatalf("execution = %#v, want completed stored graph", exec)
	}
	if len(store.loads) != 1 || store.loads[0] != graph.ID {
		t.Fatalf("store loads = %v, want %q", store.loads, graph.ID)
	}
	if len(runner.calls) != 1 || runner.calls[0].Inputs["request"] != "ok" {
		t.Fatalf("runner calls = %#v, want request input", runner.calls)
	}
}

func callNodeIDs(calls []workflowActionCall) []string {
	ids := make([]string, 0, len(calls))
	for _, call := range calls {
		ids = append(ids, call.NodeID)
	}
	return ids
}

func TestWorkflowExecutorReturnsGraphValidationErrors(t *testing.T) {
	graph := NewGraph("invalid", "")
	graph.Nodes = nil

	exec, err := NewWorkflowExecutor(nil, nil).ExecuteGraph(graph, nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if exec != nil {
		t.Fatalf("execution = %#v, want nil for validation failure", exec)
	}
	if !strings.Contains(fmt.Sprint(err), "graph validation failed") {
		t.Fatalf("error = %v, want graph validation wrapper", err)
	}
}
