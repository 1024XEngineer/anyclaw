package workflow

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

type Graph struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Version     string    `json:"version,omitempty"`
	Author      string    `json:"author,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	Nodes     []Node        `json:"nodes"`
	Edges     []Edge        `json:"edges"`
	Inputs    []InputParam  `json:"inputs,omitempty"`
	Outputs   []OutputParam `json:"outputs,omitempty"`
	Variables []Variable    `json:"variables,omitempty"`

	Metadata map[string]any `json:"metadata,omitempty"`
	Tags     []string       `json:"tags,omitempty"`
}

type Node struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`

	Plugin   string            `json:"plugin,omitempty"`
	Action   string            `json:"action,omitempty"`
	Workflow string            `json:"workflow,omitempty"`
	Inputs   map[string]any    `json:"inputs,omitempty"`
	Outputs  map[string]string `json:"outputs,omitempty"`

	Condition string `json:"condition,omitempty"`
	LoopVar   string `json:"loop_var,omitempty"`
	LoopOver  string `json:"loop_over,omitempty"`

	TimeoutSec    int            `json:"timeout_sec,omitempty"`
	RetryPolicy   *RetryPolicy   `json:"retry_policy,omitempty"`
	ErrorHandling *ErrorHandling `json:"error_handling,omitempty"`
	Position      Position       `json:"position,omitempty"`
}

type Edge struct {
	ID        string `json:"id"`
	Source    string `json:"source"`
	Target    string `json:"target"`
	Type      string `json:"type"`
	Condition string `json:"condition,omitempty"`
	Label     string `json:"label,omitempty"`
}

type InputParam struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
	Default     any    `json:"default,omitempty"`
}

type OutputParam struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Source      string `json:"source,omitempty"`
}

type Variable struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	Description  string `json:"description,omitempty"`
	InitialValue any    `json:"initial_value,omitempty"`
}

type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type RetryPolicy struct {
	MaxAttempts   int     `json:"max_attempts"`
	InitialDelay  int     `json:"initial_delay"`
	MaxDelay      int     `json:"max_delay"`
	BackoffFactor float64 `json:"backoff_factor"`
}

type ErrorHandling struct {
	OnError    string `json:"on_error"`
	TargetNode string `json:"target_node,omitempty"`
	MaxRetries int    `json:"max_retries,omitempty"`
}

type ExecutionStatus string

const (
	ExecutionPending   ExecutionStatus = "pending"
	ExecutionRunning   ExecutionStatus = "running"
	ExecutionPaused    ExecutionStatus = "paused"
	ExecutionCompleted ExecutionStatus = "completed"
	ExecutionFailed    ExecutionStatus = "failed"
	ExecutionCancelled ExecutionStatus = "cancelled"
)

type NodeStatus string

const (
	NodePending   NodeStatus = "pending"
	NodeRunning   NodeStatus = "running"
	NodeCompleted NodeStatus = "completed"
	NodeFailed    NodeStatus = "failed"
	NodeSkipped   NodeStatus = "skipped"
	NodeRetrying  NodeStatus = "retrying"
)

type ExecutionContext struct {
	GraphID     string                `json:"graph_id"`
	ExecutionID string                `json:"execution_id"`
	Inputs      map[string]any        `json:"inputs"`
	Variables   map[string]any        `json:"variables"`
	Outputs     map[string]any        `json:"outputs"`
	NodeStates  map[string]*NodeState `json:"node_states"`
	CurrentNode string                `json:"current_node,omitempty"`
	Status      ExecutionStatus       `json:"status"`
	StartTime   time.Time             `json:"start_time"`
	EndTime     *time.Time            `json:"end_time,omitempty"`
	Error       *ExecutionError       `json:"error,omitempty"`
	Evidence    []Evidence            `json:"evidence,omitempty"`
}

type NodeState struct {
	NodeID    string         `json:"node_id"`
	Status    NodeStatus     `json:"status"`
	StartTime *time.Time     `json:"start_time,omitempty"`
	EndTime   *time.Time     `json:"end_time,omitempty"`
	Inputs    map[string]any `json:"inputs,omitempty"`
	Outputs   map[string]any `json:"outputs,omitempty"`
	Error     *NodeError     `json:"error,omitempty"`
	Attempts  int            `json:"attempts,omitempty"`
	Evidence  []Evidence     `json:"evidence,omitempty"`
}

type Evidence struct {
	Type      string         `json:"type"`
	Content   string         `json:"content,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
}

type ExecutionError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	NodeID  string `json:"node_id,omitempty"`
}

type NodeError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable,omitempty"`
}

type GraphStore interface {
	SaveGraph(graph *Graph) error
	LoadGraph(graphID string) (*Graph, error)
	DeleteGraph(graphID string) error
	ListGraphs() ([]*Graph, error)
}

var workflowIDCounter atomic.Uint64

func NewGraph(name, description string) *Graph {
	now := time.Now().UTC()
	return &Graph{
		ID:          generateWorkflowID("graph"),
		Name:        strings.TrimSpace(name),
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
		Nodes:       make([]Node, 0),
		Edges:       make([]Edge, 0),
		Inputs:      make([]InputParam, 0),
		Outputs:     make([]OutputParam, 0),
		Variables:   make([]Variable, 0),
		Metadata:    make(map[string]any),
		Tags:        make([]string, 0),
	}
}

func (g *Graph) AddNode(node Node) string {
	if g == nil {
		return ""
	}
	node = cloneNode(node)
	if strings.TrimSpace(node.ID) == "" {
		node.ID = generateWorkflowID("node")
	}
	g.Nodes = append(g.Nodes, node)
	g.touch()
	return node.ID
}

func (g *Graph) AddActionNode(name, description, plugin, action string, inputs map[string]any) string {
	return g.AddNode(Node{
		Type:        "action",
		Name:        name,
		Description: description,
		Plugin:      plugin,
		Action:      action,
		Inputs:      cloneAnyMap(inputs),
	})
}

func (g *Graph) AddConditionNode(name, description, condition string) string {
	return g.AddNode(Node{
		Type:        "condition",
		Name:        name,
		Description: description,
		Condition:   condition,
	})
}

func (g *Graph) AddEdge(source, target, edgeType string) string {
	if g == nil {
		return ""
	}
	edge := Edge{
		ID:     generateWorkflowID("edge"),
		Source: source,
		Target: target,
		Type:   normalizeEdgeType(edgeType),
	}
	g.Edges = append(g.Edges, edge)
	g.touch()
	return edge.ID
}

func (g *Graph) AddInputParam(name, paramType, description string, required bool, defaultValue any) {
	if g == nil {
		return
	}
	g.Inputs = append(g.Inputs, InputParam{
		Name:        name,
		Type:        paramType,
		Description: description,
		Required:    required,
		Default:     cloneAny(defaultValue),
	})
	g.touch()
}

func (g *Graph) AddOutputParam(name, paramType, description, source string) {
	if g == nil {
		return
	}
	g.Outputs = append(g.Outputs, OutputParam{
		Name:        name,
		Type:        paramType,
		Description: description,
		Source:      source,
	})
	g.touch()
}

func (g *Graph) AddVariable(name, varType, description string, initialValue any) {
	if g == nil {
		return
	}
	g.Variables = append(g.Variables, Variable{
		Name:         name,
		Type:         varType,
		Description:  description,
		InitialValue: cloneAny(initialValue),
	})
	g.touch()
}

func (g *Graph) Validate() error {
	if g == nil {
		return fmt.Errorf("graph is nil")
	}
	if strings.TrimSpace(g.Name) == "" {
		return fmt.Errorf("graph name is required")
	}
	if len(g.Nodes) == 0 {
		return fmt.Errorf("graph must have at least one node")
	}

	nodeIDs := make(map[string]struct{}, len(g.Nodes))
	for _, node := range g.Nodes {
		if err := validateNode(node); err != nil {
			return err
		}
		if _, exists := nodeIDs[node.ID]; exists {
			return fmt.Errorf("duplicate node ID: %s", node.ID)
		}
		nodeIDs[node.ID] = struct{}{}
	}

	for _, edge := range g.Edges {
		if strings.TrimSpace(edge.Source) == "" {
			return fmt.Errorf("edge source is required")
		}
		if strings.TrimSpace(edge.Target) == "" {
			return fmt.Errorf("edge target is required")
		}
		if _, ok := nodeIDs[edge.Source]; !ok {
			return fmt.Errorf("edge source node not found: %s", edge.Source)
		}
		if _, ok := nodeIDs[edge.Target]; !ok {
			return fmt.Errorf("edge target node not found: %s", edge.Target)
		}
	}

	if len(g.GetStartNodes()) == 0 {
		return fmt.Errorf("graph must have at least one start node")
	}
	return nil
}

func validateNode(node Node) error {
	if strings.TrimSpace(node.ID) == "" {
		return fmt.Errorf("node ID is required")
	}
	if strings.TrimSpace(node.Type) == "" {
		return fmt.Errorf("node type is required: %s", node.ID)
	}
	if strings.TrimSpace(node.Name) == "" {
		return fmt.Errorf("node name is required: %s", node.ID)
	}

	switch node.Type {
	case "action":
		if strings.TrimSpace(node.Plugin) == "" {
			return fmt.Errorf("action node must have plugin: %s", node.ID)
		}
		if strings.TrimSpace(node.Action) == "" {
			return fmt.Errorf("action node must have action: %s", node.ID)
		}
	case "condition":
		if strings.TrimSpace(node.Condition) == "" {
			return fmt.Errorf("condition node must have condition: %s", node.ID)
		}
	case "loop":
		if strings.TrimSpace(node.LoopVar) == "" {
			return fmt.Errorf("loop node must have loop variable: %s", node.ID)
		}
		if strings.TrimSpace(node.LoopOver) == "" {
			return fmt.Errorf("loop node must have loop over expression: %s", node.ID)
		}
	case "parallel", "join":
	default:
		return fmt.Errorf("unsupported node type: %s", node.Type)
	}
	return nil
}

func (g *Graph) ToJSON() ([]byte, error) {
	if g == nil {
		return nil, fmt.Errorf("graph is nil")
	}
	return json.MarshalIndent(g, "", "  ")
}

func FromJSON(data []byte) (*Graph, error) {
	var graph Graph
	if err := json.Unmarshal(data, &graph); err != nil {
		return nil, err
	}
	if graph.Metadata == nil {
		graph.Metadata = make(map[string]any)
	}
	return &graph, nil
}

func (g *Graph) GetNextNodes(nodeID string) []string {
	if g == nil {
		return nil
	}
	var next []string
	for _, edge := range g.Edges {
		if edge.Source == nodeID {
			next = append(next, edge.Target)
		}
	}
	return next
}

func (g *Graph) GetPreviousNodes(nodeID string) []string {
	if g == nil {
		return nil
	}
	var previous []string
	for _, edge := range g.Edges {
		if edge.Target == nodeID {
			previous = append(previous, edge.Source)
		}
	}
	return previous
}

func (g *Graph) GetStartNodes() []Node {
	if g == nil {
		return nil
	}
	inDegree := make(map[string]int, len(g.Nodes))
	for _, edge := range g.Edges {
		inDegree[edge.Target]++
	}

	start := make([]Node, 0)
	for _, node := range g.Nodes {
		if inDegree[node.ID] == 0 {
			start = append(start, cloneNode(node))
		}
	}
	return start
}

func (g *Graph) GetNodeByID(nodeID string) (*Node, bool) {
	if g == nil {
		return nil, false
	}
	for _, node := range g.Nodes {
		if node.ID == nodeID {
			cloned := cloneNode(node)
			return &cloned, true
		}
	}
	return nil, false
}

func NewExecutionContext(graphID string, inputs map[string]any) *ExecutionContext {
	now := time.Now().UTC()
	return &ExecutionContext{
		GraphID:     graphID,
		ExecutionID: generateWorkflowID("exec"),
		Inputs:      cloneAnyMap(inputs),
		Variables:   make(map[string]any),
		Outputs:     make(map[string]any),
		NodeStates:  make(map[string]*NodeState),
		Status:      ExecutionPending,
		StartTime:   now,
		Evidence:    make([]Evidence, 0),
	}
}

func (ctx *ExecutionContext) ResolveInputs(node *Node, graph *Graph) map[string]any {
	if ctx == nil || node == nil {
		return nil
	}
	resolved := make(map[string]any, len(node.Inputs))
	for key, value := range node.Inputs {
		resolved[key] = ctx.resolveValue(value, graph)
	}
	return resolved
}

func (ctx *ExecutionContext) resolveValue(value any, graph *Graph) any {
	switch v := value.(type) {
	case string:
		if strings.HasPrefix(v, "$") {
			return ctx.resolveReference(v)
		}
		return v
	case map[string]any:
		resolved := make(map[string]any, len(v))
		for key, val := range v {
			resolved[key] = ctx.resolveValue(val, graph)
		}
		return resolved
	case []any:
		resolved := make([]any, len(v))
		for i, val := range v {
			resolved[i] = ctx.resolveValue(val, graph)
		}
		return resolved
	default:
		return value
	}
}

func (ctx *ExecutionContext) resolveReference(ref string) any {
	path := strings.TrimPrefix(strings.TrimSpace(ref), "$")
	if path == "" {
		return ref
	}
	if strings.Contains(path, ".") {
		parts := strings.SplitN(path, ".", 2)
		nodeID := parts[0]
		outputName := parts[1]
		if state, ok := ctx.NodeStates[nodeID]; ok && state.Outputs != nil {
			if value, ok := state.Outputs[outputName]; ok {
				return value
			}
			if value, ok := resolveNestedOutput(state.Outputs, outputName); ok {
				return value
			}
		}
	}
	if value, ok := ctx.Variables[path]; ok {
		return value
	}
	if value, ok := ctx.Inputs[path]; ok {
		return value
	}
	return ref
}

func resolveNestedOutput(outputs map[string]any, path string) (any, bool) {
	current := any(outputs)
	for _, part := range strings.Split(path, ".") {
		currentMap, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = currentMap[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func (ctx *ExecutionContext) AddEvidence(evidenceType, content string, data map[string]any) {
	if ctx == nil {
		return
	}
	ctx.Evidence = append(ctx.Evidence, Evidence{
		Type:      evidenceType,
		Content:   content,
		Data:      cloneAnyMap(data),
		Timestamp: time.Now().UTC(),
	})
}

func (ctx *ExecutionContext) MarkNodeStarted(nodeID string, inputs map[string]any) {
	if ctx == nil {
		return
	}
	now := time.Now().UTC()
	state, ok := ctx.NodeStates[nodeID]
	if !ok {
		state = &NodeState{
			NodeID:   nodeID,
			Evidence: make([]Evidence, 0),
		}
		ctx.NodeStates[nodeID] = state
	}
	if state.Attempts == 0 {
		state.Attempts = 1
	}
	state.Status = NodeRunning
	state.StartTime = &now
	state.EndTime = nil
	state.Inputs = cloneAnyMap(inputs)
	state.Error = nil
	ctx.CurrentNode = nodeID
	ctx.Status = ExecutionRunning
}

func (ctx *ExecutionContext) MarkNodeCompleted(nodeID string, outputs map[string]any) {
	if ctx == nil {
		return
	}
	now := time.Now().UTC()
	state, ok := ctx.NodeStates[nodeID]
	if !ok {
		state = &NodeState{NodeID: nodeID}
		ctx.NodeStates[nodeID] = state
	}
	state.Status = NodeCompleted
	state.EndTime = &now
	state.Outputs = cloneAnyMap(outputs)
}

func (ctx *ExecutionContext) MarkNodeFailed(nodeID string, err *NodeError) {
	if ctx == nil {
		return
	}
	now := time.Now().UTC()
	state, ok := ctx.NodeStates[nodeID]
	if !ok {
		state = &NodeState{NodeID: nodeID}
		ctx.NodeStates[nodeID] = state
	}
	state.Status = NodeFailed
	state.EndTime = &now
	state.Error = err
	ctx.Status = ExecutionFailed
	ctx.Error = &ExecutionError{
		Code:    errCode(err),
		Message: errMessage(err),
		NodeID:  nodeID,
	}
}

func (ctx *ExecutionContext) MarkNodeRetrying(nodeID string) {
	if ctx == nil {
		return
	}
	if state, ok := ctx.NodeStates[nodeID]; ok {
		state.Status = NodeRetrying
		state.Attempts++
	}
}

func (ctx *ExecutionContext) MarkExecutionCompleted(outputs map[string]any) {
	if ctx == nil {
		return
	}
	now := time.Now().UTC()
	ctx.Status = ExecutionCompleted
	ctx.EndTime = &now
	ctx.Outputs = cloneAnyMap(outputs)
}

func (ctx *ExecutionContext) IsCompleted() bool {
	if ctx == nil {
		return false
	}
	return ctx.Status == ExecutionCompleted ||
		ctx.Status == ExecutionFailed ||
		ctx.Status == ExecutionCancelled
}

func (g *Graph) touch() {
	g.UpdatedAt = time.Now().UTC()
}

func normalizeEdgeType(edgeType string) string {
	edgeType = strings.TrimSpace(edgeType)
	if edgeType == "" {
		return "default"
	}
	return edgeType
}

func generateWorkflowID(prefix string) string {
	return fmt.Sprintf("%s_%d_%d", prefix, time.Now().UTC().UnixNano(), workflowIDCounter.Add(1))
}

func errCode(err *NodeError) string {
	if err == nil || strings.TrimSpace(err.Code) == "" {
		return "execution_failed"
	}
	return err.Code
}

func errMessage(err *NodeError) string {
	if err == nil || strings.TrimSpace(err.Message) == "" {
		return "node failed"
	}
	return err.Message
}

func cloneNode(node Node) Node {
	node.Inputs = cloneAnyMap(node.Inputs)
	if node.Outputs != nil {
		outputs := make(map[string]string, len(node.Outputs))
		for key, value := range node.Outputs {
			outputs[key] = value
		}
		node.Outputs = outputs
	}
	if node.RetryPolicy != nil {
		policy := *node.RetryPolicy
		node.RetryPolicy = &policy
	}
	if node.ErrorHandling != nil {
		handling := *node.ErrorHandling
		node.ErrorHandling = &handling
	}
	return node
}

func cloneAnyMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = cloneAny(value)
	}
	return output
}

func cloneAny(input any) any {
	switch v := input.(type) {
	case map[string]any:
		return cloneAnyMap(v)
	case []any:
		output := make([]any, len(v))
		for i, value := range v {
			output[i] = cloneAny(value)
		}
		return output
	default:
		return input
	}
}
