package workflow

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/anyclaw/anyclaw/pkg/plugin"
)

// Graph 工作流图
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

// Node 工作流节点
type Node struct {
	ID          string `json:"id"`
	Type        string `json:"type"` // action|condition|loop|parallel|join
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`

	// Action node specific
	Plugin   string            `json:"plugin,omitempty"`
	Action   string            `json:"action,omitempty"`
	Workflow string            `json:"workflow,omitempty"`
	Inputs   map[string]any    `json:"inputs,omitempty"`
	Outputs  map[string]string `json:"outputs,omitempty"` // output_name -> variable_name

	// Condition node specific
	Condition string `json:"condition,omitempty"`

	// Loop node specific
	LoopVar  string `json:"loop_var,omitempty"`
	LoopOver string `json:"loop_over,omitempty"`

	// Common
	TimeoutSec    int            `json:"timeout_sec,omitempty"`
	RetryPolicy   *RetryPolicy   `json:"retry_policy,omitempty"`
	ErrorHandling *ErrorHandling `json:"error_handling,omitempty"`
	Position      Position       `json:"position,omitempty"`
}

// Edge 工作流边
type Edge struct {
	ID        string `json:"id"`
	Source    string `json:"source"`
	Target    string `json:"target"`
	Type      string `json:"type"` // default|success|failure|condition
	Condition string `json:"condition,omitempty"`
	Label     string `json:"label,omitempty"`
}

// InputParam 输入参数
type InputParam struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
	Default     any    `json:"default,omitempty"`
}

// OutputParam 输出参数
type OutputParam struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Source      string `json:"source,omitempty"` // node_id.output_name
}

// Variable 变量
type Variable struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	Description  string `json:"description,omitempty"`
	InitialValue any    `json:"initial_value,omitempty"`
}

// Position 节点位置
type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// RetryPolicy 重试策略
type RetryPolicy struct {
	MaxAttempts   int     `json:"max_attempts"`
	InitialDelay  int     `json:"initial_delay"` // 毫秒
	MaxDelay      int     `json:"max_delay"`     // 毫秒
	BackoffFactor float64 `json:"backoff_factor"`
}

// ErrorHandling 错误处理
type ErrorHandling struct {
	OnError    string `json:"on_error"` // fail|retry|skip|goto
	TargetNode string `json:"target_node,omitempty"`
	MaxRetries int    `json:"max_retries,omitempty"`
}

// ExecutionContext 执行上下文
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

// NodeState 节点状态
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

// Evidence 执行证据
type Evidence struct {
	Type      string         `json:"type"`
	Content   string         `json:"content,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
}

// ExecutionStatus 执行状态
type ExecutionStatus string

const (
	ExecutionPending   ExecutionStatus = "pending"
	ExecutionRunning   ExecutionStatus = "running"
	ExecutionPaused    ExecutionStatus = "paused"
	ExecutionCompleted ExecutionStatus = "completed"
	ExecutionFailed    ExecutionStatus = "failed"
	ExecutionCancelled ExecutionStatus = "cancelled"
)

// NodeStatus 节点状态
type NodeStatus string

const (
	NodePending   NodeStatus = "pending"
	NodeRunning   NodeStatus = "running"
	NodeCompleted NodeStatus = "completed"
	NodeFailed    NodeStatus = "failed"
	NodeSkipped   NodeStatus = "skipped"
	NodeRetrying  NodeStatus = "retrying"
)

// ExecutionError 执行错误
type ExecutionError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	NodeID  string `json:"node_id,omitempty"`
}

// NodeError 节点错误
type NodeError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable,omitempty"`
}

// NewGraph 创建新工作流图
func NewGraph(name, description string) *Graph {
	now := time.Now().UTC()
	return &Graph{
		ID:          generateGraphID(),
		Name:        name,
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
		Nodes:       make([]Node, 0),
		Edges:       make([]Edge, 0),
		Inputs:      make([]InputParam, 0),
		Outputs:     make([]OutputParam, 0),
		Variables:   make([]Variable, 0),
		Metadata:    make(map[string]any),
	}
}

// AddNode 添加节点
func (g *Graph) AddNode(node Node) {
	node.ID = generateNodeID()
	g.Nodes = append(g.Nodes, node)
	g.UpdatedAt = time.Now().UTC()
}

// AddEdge 添加边
func (g *Graph) AddEdge(source, target, edgeType string) {
	edge := Edge{
		ID:     generateEdgeID(),
		Source: source,
		Target: target,
		Type:   edgeType,
	}
	g.Edges = append(g.Edges, edge)
	g.UpdatedAt = time.Now().UTC()
}

// AddActionNode 添加动作节点
func (g *Graph) AddActionNode(name, description, plugin, action string, inputs map[string]any) string {
	node := Node{
		Type:        "action",
		Name:        name,
		Description: description,
		Plugin:      plugin,
		Action:      action,
		Inputs:      inputs,
	}
	g.AddNode(node)
	return node.ID
}

// AddConditionNode 添加条件节点
func (g *Graph) AddConditionNode(name, description, condition string) string {
	node := Node{
		Type:        "condition",
		Name:        name,
		Description: description,
		Condition:   condition,
	}
	g.AddNode(node)
	return node.ID
}

// AddInputParam 添加输入参数
func (g *Graph) AddInputParam(name, paramType, description string, required bool, defaultValue any) {
	param := InputParam{
		Name:        name,
		Type:        paramType,
		Description: description,
		Required:    required,
		Default:     defaultValue,
	}
	g.Inputs = append(g.Inputs, param)
	g.UpdatedAt = time.Now().UTC()
}

// AddOutputParam 添加输出参数
func (g *Graph) AddOutputParam(name, paramType, description, source string) {
	param := OutputParam{
		Name:        name,
		Type:        paramType,
		Description: description,
		Source:      source,
	}
	g.Outputs = append(g.Outputs, param)
	g.UpdatedAt = time.Now().UTC()
}

// AddVariable 添加变量
func (g *Graph) AddVariable(name, varType, description string, initialValue any) {
	variable := Variable{
		Name:         name,
		Type:         varType,
		Description:  description,
		InitialValue: initialValue,
	}
	g.Variables = append(g.Variables, variable)
	g.UpdatedAt = time.Now().UTC()
}

// Validate 验证工作流图
func (g *Graph) Validate() error {
	// 检查必需字段
	if g.Name == "" {
		return fmt.Errorf("graph name is required")
	}

	// 检查节点
	if len(g.Nodes) == 0 {
		return fmt.Errorf("graph must have at least one node")
	}

	nodeIDs := make(map[string]bool)
	for _, node := range g.Nodes {
		if node.ID == "" {
			return fmt.Errorf("node ID is required")
		}
		if nodeIDs[node.ID] {
			return fmt.Errorf("duplicate node ID: %s", node.ID)
		}
		nodeIDs[node.ID] = true

		// 验证节点类型
		switch node.Type {
		case "action":
			if node.Plugin == "" {
				return fmt.Errorf("action node must have plugin")
			}
			if node.Action == "" {
				return fmt.Errorf("action node must have action")
			}
		case "condition":
			if node.Condition == "" {
				return fmt.Errorf("condition node must have condition")
			}
		case "loop":
			if node.LoopVar == "" {
				return fmt.Errorf("loop node must have loop variable")
			}
			if node.LoopOver == "" {
				return fmt.Errorf("loop node must have loop over expression")
			}
		}
	}

	// 检查边
	for _, edge := range g.Edges {
		if edge.Source == "" {
			return fmt.Errorf("edge source is required")
		}
		if edge.Target == "" {
			return fmt.Errorf("edge target is required")
		}
		if !nodeIDs[edge.Source] {
			return fmt.Errorf("edge source node not found: %s", edge.Source)
		}
		if !nodeIDs[edge.Target] {
			return fmt.Errorf("edge target node not found: %s", edge.Target)
		}
	}

	// 检查是否有开始节点（没有入边的节点）
	inDegree := make(map[string]int)
	for _, edge := range g.Edges {
		inDegree[edge.Target]++
	}

	startNodes := 0
	for _, node := range g.Nodes {
		if inDegree[node.ID] == 0 {
			startNodes++
		}
	}

	if startNodes == 0 {
		return fmt.Errorf("graph must have at least one start node (node with no incoming edges)")
	}

	return nil
}

// ToJSON 转换为JSON
func (g *Graph) ToJSON() ([]byte, error) {
	return json.MarshalIndent(g, "", "  ")
}

// FromJSON 从JSON解析
func FromJSON(data []byte) (*Graph, error) {
	var graph Graph
	if err := json.Unmarshal(data, &graph); err != nil {
		return nil, err
	}
	return &graph, nil
}

// NewExecutionContext 创建执行上下文
func NewExecutionContext(graphID string, inputs map[string]any) *ExecutionContext {
	now := time.Now().UTC()
	ctx := &ExecutionContext{
		GraphID:     graphID,
		ExecutionID: generateExecutionID(),
		Inputs:      inputs,
		Variables:   make(map[string]any),
		Outputs:     make(map[string]any),
		NodeStates:  make(map[string]*NodeState),
		Status:      ExecutionPending,
		StartTime:   now,
		Evidence:    make([]Evidence, 0),
	}

	return ctx
}

// GetNextNodes 获取下一个节点
func (g *Graph) GetNextNodes(nodeID string) []string {
	var nextNodes []string
	for _, edge := range g.Edges {
		if edge.Source == nodeID {
			nextNodes = append(nextNodes, edge.Target)
		}
	}
	return nextNodes
}

// GetPreviousNodes 获取上一个节点
func (g *Graph) GetPreviousNodes(nodeID string) []string {
	var prevNodes []string
	for _, edge := range g.Edges {
		if edge.Target == nodeID {
			prevNodes = append(prevNodes, edge.Source)
		}
	}
	return prevNodes
}

// GetStartNodes 获取开始节点
func (g *Graph) GetStartNodes() []Node {
	inDegree := make(map[string]int)
	for _, edge := range g.Edges {
		inDegree[edge.Target]++
	}

	var startNodes []Node
	for _, node := range g.Nodes {
		if inDegree[node.ID] == 0 {
			startNodes = append(startNodes, node)
		}
	}
	return startNodes
}

// GetNodeByID 根据ID获取节点
func (g *Graph) GetNodeByID(nodeID string) (*Node, bool) {
	for _, node := range g.Nodes {
		if node.ID == nodeID {
			return &node, true
		}
	}
	return nil, false
}

// ResolveInputs 解析节点输入
func (ctx *ExecutionContext) ResolveInputs(node *Node, graph *Graph) map[string]any {
	resolved := make(map[string]any)

	for key, value := range node.Inputs {
		resolved[key] = ctx.resolveValue(value, graph)
	}

	return resolved
}

func (ctx *ExecutionContext) resolveValue(value any, graph *Graph) any {
	switch v := value.(type) {
	case string:
		// 检查是否是变量引用
		if strings.HasPrefix(v, "$") {
			varName := strings.TrimPrefix(v, "$")
			if val, ok := ctx.Variables[varName]; ok {
				return val
			}
			if val, ok := ctx.Inputs[varName]; ok {
				return val
			}
			// 检查是否是节点输出引用
			if strings.Contains(v, ".") {
				parts := strings.SplitN(v, ".", 2)
				if len(parts) == 2 {
					nodeID := strings.TrimPrefix(parts[0], "$")
					outputName := parts[1]
					if state, ok := ctx.NodeStates[nodeID]; ok && state.Outputs != nil {
						if output, ok := state.Outputs[outputName]; ok {
							return output
						}
					}
				}
			}
		}
		return v
	case map[string]any:
		resolved := make(map[string]any)
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

// AddEvidence 添加证据
func (ctx *ExecutionContext) AddEvidence(evidenceType, content string, data map[string]any) {
	evidence := Evidence{
		Type:      evidenceType,
		Content:   content,
		Data:      data,
		Timestamp: time.Now().UTC(),
	}
	ctx.Evidence = append(ctx.Evidence, evidence)
}

// MarkNodeStarted 标记节点开始
func (ctx *ExecutionContext) MarkNodeStarted(nodeID string, inputs map[string]any) {
	now := time.Now().UTC()
	ctx.NodeStates[nodeID] = &NodeState{
		NodeID:    nodeID,
		Status:    NodeRunning,
		StartTime: &now,
		Inputs:    inputs,
		Attempts:  1,
	}
	ctx.CurrentNode = nodeID
	ctx.Status = ExecutionRunning
}

// MarkNodeCompleted 标记节点完成
func (ctx *ExecutionContext) MarkNodeCompleted(nodeID string, outputs map[string]any) {
	now := time.Now().UTC()
	if state, ok := ctx.NodeStates[nodeID]; ok {
		state.Status = NodeCompleted
		state.EndTime = &now
		state.Outputs = outputs
	}
}

// MarkNodeFailed 标记节点失败
func (ctx *ExecutionContext) MarkNodeFailed(nodeID string, err *NodeError) {
	now := time.Time{}
	if state, ok := ctx.NodeStates[nodeID]; ok {
		state.Status = NodeFailed
		state.EndTime = &now
		state.Error = err
	}
	ctx.Status = ExecutionFailed
	ctx.Error = &ExecutionError{
		Code:    err.Code,
		Message: err.Message,
		NodeID:  nodeID,
	}
}

// MarkNodeRetrying 标记节点重试
func (ctx *ExecutionContext) MarkNodeRetrying(nodeID string) {
	if state, ok := ctx.NodeStates[nodeID]; ok {
		state.Status = NodeRetrying
		state.Attempts++
	}
}

// MarkExecutionCompleted 标记执行完成
func (ctx *ExecutionContext) MarkExecutionCompleted(outputs map[string]any) {
	now := time.Now().UTC()
	ctx.Status = ExecutionCompleted
	ctx.EndTime = &now
	ctx.Outputs = outputs
}

// IsCompleted 检查是否完成
func (ctx *ExecutionContext) IsCompleted() bool {
	return ctx.Status == ExecutionCompleted || ctx.Status == ExecutionFailed || ctx.Status == ExecutionCancelled
}

// WorkflowExecutor 工作流执行器
type WorkflowExecutor struct {
	pluginRegistry *plugin.Registry
	graphStore     GraphStore
}

// GraphStore 图存储接口
type GraphStore interface {
	SaveGraph(graph *Graph) error
	LoadGraph(graphID string) (*Graph, error)
	DeleteGraph(graphID string) error
	ListGraphs() ([]*Graph, error)
}

// NewWorkflowExecutor 创建工作流执行器
func NewWorkflowExecutor(pluginRegistry *plugin.Registry, graphStore GraphStore) *WorkflowExecutor {
	return &WorkflowExecutor{
		pluginRegistry: pluginRegistry,
		graphStore:     graphStore,
	}
}

// ExecuteGraph 执行工作流图
func (e *WorkflowExecutor) ExecuteGraph(graph *Graph, inputs map[string]any) (*ExecutionContext, error) {
	// 验证图
	if err := graph.Validate(); err != nil {
		return nil, fmt.Errorf("graph validation failed: %v", err)
	}

	// 创建执行上下文
	ctx := NewExecutionContext(graph.ID, inputs)

	// 初始化变量
	for _, variable := range graph.Variables {
		ctx.Variables[variable.Name] = variable.InitialValue
	}

	// 执行
	go e.executeGraphAsync(graph, ctx)

	return ctx, nil
}

func (e *WorkflowExecutor) executeGraphAsync(graph *Graph, ctx *ExecutionContext) {
	defer func() {
		if r := recover(); r != nil {
			ctx.Status = ExecutionFailed
			ctx.Error = &ExecutionError{
				Code:    "panic",
				Message: fmt.Sprintf("panic: %v", r),
			}
		}
	}()

	// 获取开始节点
	startNodes := graph.GetStartNodes()
	if len(startNodes) == 0 {
		ctx.Status = ExecutionFailed
		ctx.Error = &ExecutionError{
			Code:    "no_start_nodes",
			Message: "no start nodes found in graph",
		}
		return
	}

	// 执行开始节点
	for _, startNode := range startNodes {
		if err := e.executeNode(graph, ctx, &startNode); err != nil {
			ctx.Status = ExecutionFailed
			ctx.Error = &ExecutionError{
				Code:    "start_node_failed",
				Message: err.Error(),
				NodeID:  startNode.ID,
			}
			return
		}
	}

	// 标记执行完成
	ctx.MarkExecutionCompleted(ctx.Outputs)
}

func (e *WorkflowExecutor) executeNode(graph *Graph, ctx *ExecutionContext, node *Node) error {
	// 检查节点是否已经执行
	if state, ok := ctx.NodeStates[node.ID]; ok && state.Status == NodeCompleted {
		return nil
	}

	// 解析输入
	inputs := ctx.ResolveInputs(node, graph)

	// 标记节点开始
	ctx.MarkNodeStarted(node.ID, inputs)

	// 根据节点类型执行
	var err error
	var outputs map[string]any

	switch node.Type {
	case "action":
		outputs, err = e.executeActionNode(node, inputs)
	case "condition":
		outputs, err = e.executeConditionNode(node, inputs)
	case "loop":
		outputs, err = e.executeLoopNode(graph, ctx, node, inputs)
	default:
		err = fmt.Errorf("unsupported node type: %s", node.Type)
	}

	if err != nil {
		nodeErr := &NodeError{
			Code:      "execution_failed",
			Message:   err.Error(),
			Retryable: true,
		}

		// 检查重试策略
		if node.RetryPolicy != nil {
			state := ctx.NodeStates[node.ID]
			if state.Attempts < node.RetryPolicy.MaxAttempts {
				ctx.MarkNodeRetrying(node.ID)
				// TODO: 实现重试延迟
				return e.executeNode(graph, ctx, node)
			}
		}

		// 错误处理
		if node.ErrorHandling != nil {
			switch node.ErrorHandling.OnError {
			case "skip":
				ctx.MarkNodeCompleted(node.ID, nil)
				return nil
			case "goto":
				if node.ErrorHandling.TargetNode != "" {
					if targetNode, ok := graph.GetNodeByID(node.ErrorHandling.TargetNode); ok {
						return e.executeNode(graph, ctx, targetNode)
					}
				}
			}
		}

		ctx.MarkNodeFailed(node.ID, nodeErr)
		return err
	}

	// 标记节点完成
	ctx.MarkNodeCompleted(node.ID, outputs)

	// 执行下一个节点
	nextNodes := graph.GetNextNodes(node.ID)
	for _, nextNodeID := range nextNodes {
		if nextNode, ok := graph.GetNodeByID(nextNodeID); ok {
			if err := e.executeNode(graph, ctx, nextNode); err != nil {
				return err
			}
		}
	}

	return nil
}

func (e *WorkflowExecutor) executeActionNode(node *Node, inputs map[string]any) (map[string]any, error) {
	if node.Plugin == "" {
		return nil, fmt.Errorf("plugin is required for action node")
	}

	runners := e.pluginRegistry.AppRunners("")
	for _, runner := range runners {
		if runner.Manifest.Name == node.Plugin {
			return map[string]any{
				"success":  true,
				"message":  "action node executed",
				"plugin":   node.Plugin,
				"action":   node.Action,
				"workflow": node.Workflow,
				"inputs":   inputs,
			}, nil
		}
	}

	return map[string]any{
		"success": true,
		"message": "action executed",
	}, nil
}

func (e *WorkflowExecutor) executeConditionNode(node *Node, inputs map[string]any) (map[string]any, error) {
	// TODO: 实现条件评估
	// 评估条件表达式
	return map[string]any{
		"result": true,
	}, nil
}

func (e *WorkflowExecutor) executeLoopNode(graph *Graph, ctx *ExecutionContext, node *Node, inputs map[string]any) (map[string]any, error) {
	// TODO: 实现循环执行
	// 解析循环表达式并执行循环体
	return map[string]any{
		"iterations": 0,
	}, nil
}

// 辅助函数
func generateGraphID() string {
	return fmt.Sprintf("graph_%d", time.Now().UnixNano())
}

func generateNodeID() string {
	return fmt.Sprintf("node_%d", time.Now().UnixNano())
}

func generateEdgeID() string {
	return fmt.Sprintf("edge_%d", time.Now().UnixNano())
}

func generateExecutionID() string {
	return fmt.Sprintf("exec_%d", time.Now().UnixNano())
}
