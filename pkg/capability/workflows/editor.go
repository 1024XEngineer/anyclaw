package workflow

import (
	"encoding/json"
	"fmt"
	"strings"
)

// WorkflowSchema represents the JSON schema for the visual editor.
type WorkflowSchema struct {
	Schema      string           `json:"$schema"`
	Title       string           `json:"title"`
	Description string           `json:"description"`
	Type        string           `json:"type"`
	Properties  SchemaProperties `json:"properties"`
	Required    []string         `json:"required,omitempty"`
	Definitions map[string]any   `json:"definitions,omitempty"`
}

// SchemaProperties holds the JSON schema properties.
type SchemaProperties struct {
	ID          StringProperty `json:"id"`
	Name        StringProperty `json:"name"`
	Description StringProperty `json:"description"`
	Version     StringProperty `json:"version"`
	Nodes       ArrayProperty  `json:"nodes"`
	Edges       ArrayProperty  `json:"edges"`
	Inputs      ArrayProperty  `json:"inputs"`
	Outputs     ArrayProperty  `json:"outputs"`
	Variables   ArrayProperty  `json:"variables"`
	Tags        ArrayProperty  `json:"tags"`
	Metadata    ObjectProperty `json:"metadata"`
}

// StringProperty represents a string property in JSON schema.
type StringProperty struct {
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
	Format      string   `json:"format,omitempty"`
}

// ArrayProperty represents an array property in JSON schema.
type ArrayProperty struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Items       any    `json:"items,omitempty"`
	MinItems    int    `json:"minItems,omitempty"`
}

// ObjectProperty represents an object property in JSON schema.
type ObjectProperty struct {
	Type                 string `json:"type"`
	Description          string `json:"description,omitempty"`
	AdditionalProperties bool   `json:"additionalProperties"`
}

// NodeSchema defines the schema for a workflow node in the visual editor.
type NodeSchema struct {
	Type        string            `json:"type"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Properties  map[string]any    `json:"properties"`
	Required    []string          `json:"required,omitempty"`
	OneOf       []NodeVariant     `json:"oneOf,omitempty"`
	UIHints     map[string]string `json:"ui_hints,omitempty"`
}

// NodeVariant represents a variant of a node (for action/condition/loop/etc).
type NodeVariant struct {
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Properties  map[string]any `json:"properties"`
	Required    []string       `json:"required,omitempty"`
}

// EdgeSchema defines the schema for workflow edges.
type EdgeSchema struct {
	Type        string         `json:"type"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Properties  map[string]any `json:"properties"`
	Required    []string       `json:"required,omitempty"`
}

// NodePalette provides available node types for the visual editor palette.
type NodePalette struct {
	Categories []PaletteCategory `json:"categories"`
}

// PaletteCategory groups related node types.
type PaletteCategory struct {
	Name  string        `json:"name"`
	Label string        `json:"label"`
	Icon  string        `json:"icon"`
	Nodes []PaletteNode `json:"nodes"`
}

// PaletteNode describes a node available in the editor palette.
type PaletteNode struct {
	Type        string         `json:"type"`
	Label       string         `json:"label"`
	Description string         `json:"description"`
	Icon        string         `json:"icon"`
	Color       string         `json:"color"`
	Defaults    map[string]any `json:"defaults"`
	Ports       NodePorts      `json:"ports"`
}

// NodePorts describes input/output ports for a node.
type NodePorts struct {
	Inputs  []Port `json:"inputs"`
	Outputs []Port `json:"outputs"`
}

// Port describes a single input or output port.
type Port struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

// GetWorkflowJSONSchema returns the JSON schema for workflow validation.
func GetWorkflowJSONSchema() map[string]any {
	return map[string]any{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"title":   "Workflow Graph",
		"type":    "object",
		"properties": map[string]any{
			"id": map[string]any{
				"type":        "string",
				"description": "Unique identifier for the workflow",
				"pattern":     "^graph_\\d+$",
			},
			"name": map[string]any{
				"type":        "string",
				"description": "Human-readable name of the workflow",
				"minLength":   1,
			},
			"description": map[string]any{
				"type":        "string",
				"description": "Description of what the workflow does",
			},
			"version": map[string]any{
				"type":        "string",
				"description": "Semantic version of the workflow",
			},
			"nodes": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id": map[string]any{
							"type":        "string",
							"description": "Unique node identifier",
						},
						"type": map[string]any{
							"type":        "string",
							"description": "Node type",
							"enum":        []string{"action", "condition", "loop", "parallel", "join"},
						},
						"name": map[string]any{
							"type":        "string",
							"description": "Human-readable node name",
						},
						"description": map[string]any{
							"type": "string",
						},
						"plugin": map[string]any{
							"type":        "string",
							"description": "Plugin name (for action nodes)",
						},
						"action": map[string]any{
							"type":        "string",
							"description": "Action name (for action nodes)",
						},
						"workflow": map[string]any{
							"type":        "string",
							"description": "Workflow reference (for action nodes)",
						},
						"inputs": map[string]any{
							"type":                 "object",
							"description":          "Node input parameters",
							"additionalProperties": true,
						},
						"outputs": map[string]any{
							"type": "object",
							"additionalProperties": map[string]any{
								"type": "string",
							},
						},
						"condition": map[string]any{
							"type":        "string",
							"description": "Condition expression (for condition nodes)",
						},
						"loop_var": map[string]any{
							"type":        "string",
							"description": "Loop variable name (for loop nodes)",
						},
						"loop_over": map[string]any{
							"type":        "string",
							"description": "Loop collection expression (for loop nodes)",
						},
						"timeout_sec": map[string]any{
							"type":        "integer",
							"description": "Node timeout in seconds",
							"minimum":     1,
						},
						"retry_policy": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"max_attempts": map[string]any{
									"type":    "integer",
									"minimum": 1,
								},
								"initial_delay": map[string]any{
									"type":        "integer",
									"description": "Initial delay in milliseconds",
								},
								"max_delay": map[string]any{
									"type":        "integer",
									"description": "Maximum delay in milliseconds",
								},
								"backoff_factor": map[string]any{
									"type":    "number",
									"minimum": 1,
								},
							},
						},
						"error_handling": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"on_error": map[string]any{
									"type": "string",
									"enum": []string{"fail", "retry", "skip", "goto"},
								},
								"target_node": map[string]any{
									"type": "string",
								},
								"max_retries": map[string]any{
									"type": "integer",
								},
							},
						},
						"position": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"x": map[string]any{"type": "number"},
								"y": map[string]any{"type": "number"},
							},
						},
					},
					"required": []string{"id", "type", "name"},
				},
				"minItems": 1,
			},
			"edges": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id": map[string]any{
							"type": "string",
						},
						"source": map[string]any{
							"type":        "string",
							"description": "Source node ID",
						},
						"target": map[string]any{
							"type":        "string",
							"description": "Target node ID",
						},
						"type": map[string]any{
							"type":        "string",
							"description": "Edge type",
							"enum":        []string{"default", "success", "failure", "condition", "condition_true", "condition_false"},
						},
						"condition": map[string]any{
							"type":        "string",
							"description": "Edge condition expression",
						},
						"label": map[string]any{
							"type":        "string",
							"description": "Edge display label",
						},
					},
					"required": []string{"id", "source", "target", "type"},
				},
			},
			"inputs": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name":        map[string]any{"type": "string"},
						"type":        map[string]any{"type": "string", "enum": []string{"string", "number", "boolean", "array", "object"}},
						"description": map[string]any{"type": "string"},
						"required":    map[string]any{"type": "boolean"},
						"default":     map[string]any{},
					},
					"required": []string{"name", "type"},
				},
			},
			"outputs": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name":        map[string]any{"type": "string"},
						"type":        map[string]any{"type": "string"},
						"description": map[string]any{"type": "string"},
						"source":      map[string]any{"type": "string"},
					},
					"required": []string{"name", "type"},
				},
			},
			"variables": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name":          map[string]any{"type": "string"},
						"type":          map[string]any{"type": "string"},
						"description":   map[string]any{"type": "string"},
						"initial_value": map[string]any{},
					},
					"required": []string{"name", "type"},
				},
			},
			"tags": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"metadata": map[string]any{
				"type":                 "object",
				"additionalProperties": true,
			},
		},
		"required": []string{"id", "name", "nodes", "edges"},
	}
}

// GetNodePalette returns the node palette for the visual editor.
func GetNodePalette() *NodePalette {
	return &NodePalette{
		Categories: []PaletteCategory{
			{
				Name:  "action",
				Label: "Actions",
				Icon:  "⚡",
				Nodes: []PaletteNode{
					{
						Type:        "action",
						Label:       "Plugin Action",
						Description: "Execute a plugin action",
						Icon:        "⚡",
						Color:       "#4A90D9",
						Defaults: map[string]any{
							"type":    "action",
							"plugin":  "",
							"action":  "",
							"inputs":  map[string]any{},
							"outputs": map[string]any{},
						},
						Ports: NodePorts{
							Inputs:  []Port{{ID: "in", Label: "Input", Type: "any"}},
							Outputs: []Port{{ID: "out", Label: "Output", Type: "any"}},
						},
					},
				},
			},
			{
				Name:  "control",
				Label: "Control Flow",
				Icon:  "🔀",
				Nodes: []PaletteNode{
					{
						Type:        "condition",
						Label:       "Condition",
						Description: "Branch execution based on a condition",
						Icon:        "🔀",
						Color:       "#F5A623",
						Defaults: map[string]any{
							"type":      "condition",
							"condition": "$input != ''",
						},
						Ports: NodePorts{
							Inputs:  []Port{{ID: "in", Label: "Input", Type: "any"}},
							Outputs: []Port{{ID: "true", Label: "True", Type: "any"}, {ID: "false", Label: "False", Type: "any"}},
						},
					},
					{
						Type:        "loop",
						Label:       "Loop",
						Description: "Iterate over a collection",
						Icon:        "🔄",
						Color:       "#7B68EE",
						Defaults: map[string]any{
							"type":      "loop",
							"loop_var":  "item",
							"loop_over": "$items",
						},
						Ports: NodePorts{
							Inputs:  []Port{{ID: "in", Label: "Input", Type: "any"}},
							Outputs: []Port{{ID: "body", Label: "Loop Body", Type: "any"}, {ID: "out", Label: "Output", Type: "any"}},
						},
					},
					{
						Type:        "parallel",
						Label:       "Parallel",
						Description: "Execute multiple branches in parallel",
						Icon:        "⚡",
						Color:       "#50C878",
						Defaults: map[string]any{
							"type": "parallel",
						},
						Ports: NodePorts{
							Inputs:  []Port{{ID: "in", Label: "Input", Type: "any"}},
							Outputs: []Port{{ID: "branch1", Label: "Branch 1", Type: "any"}, {ID: "branch2", Label: "Branch 2", Type: "any"}},
						},
					},
					{
						Type:        "join",
						Label:       "Join",
						Description: "Wait for all parallel branches to complete",
						Icon:        "🔗",
						Color:       "#50C878",
						Defaults: map[string]any{
							"type":   "join",
							"inputs": map[string]any{"join_mode": "all"},
						},
						Ports: NodePorts{
							Inputs:  []Port{{ID: "branch1", Label: "Branch 1", Type: "any"}, {ID: "branch2", Label: "Branch 2", Type: "any"}},
							Outputs: []Port{{ID: "out", Label: "Output", Type: "any"}},
						},
					},
				},
			},
			{
				Name:  "io",
				Label: "Input/Output",
				Icon:  "📥",
				Nodes: []PaletteNode{
					{
						Type:        "input",
						Label:       "Input",
						Description: "Workflow input parameter",
						Icon:        "📥",
						Color:       "#8B4513",
						Defaults: map[string]any{
							"type":   "action",
							"plugin": "builtin",
							"action": "input",
						},
						Ports: NodePorts{
							Inputs:  []Port{},
							Outputs: []Port{{ID: "out", Label: "Value", Type: "any"}},
						},
					},
					{
						Type:        "output",
						Label:       "Output",
						Description: "Workflow output parameter",
						Icon:        "📤",
						Color:       "#8B4513",
						Defaults: map[string]any{
							"type":   "action",
							"plugin": "builtin",
							"action": "output",
						},
						Ports: NodePorts{
							Inputs:  []Port{{ID: "in", Label: "Value", Type: "any"}},
							Outputs: []Port{},
						},
					},
				},
			},
		},
	}
}

// GetConditionFunctions returns available condition functions for the editor.
func GetConditionFunctions() []map[string]any {
	return []map[string]any{
		{"name": "empty", "description": "Check if value is empty", "args": 1, "example": "empty($value)"},
		{"name": "not_empty", "description": "Check if value is not empty", "args": 1, "example": "not_empty($value)"},
		{"name": "contains", "description": "Check if string/array contains value", "args": 2, "example": "contains($text, 'hello')"},
		{"name": "starts_with", "description": "Check if string starts with prefix", "args": 2, "example": "starts_with($text, 'http')"},
		{"name": "ends_with", "description": "Check if string ends with suffix", "args": 2, "example": "ends_with($file, '.json')"},
		{"name": "length", "description": "Get length of string/array", "args": 1, "example": "length($items)"},
		{"name": "is_string", "description": "Check if value is a string", "args": 1, "example": "is_string($value)"},
		{"name": "is_number", "description": "Check if value is a number", "args": 1, "example": "is_number($value)"},
		{"name": "is_bool", "description": "Check if value is a boolean", "args": 1, "example": "is_bool($value)"},
		{"name": "is_array", "description": "Check if value is an array", "args": 1, "example": "is_array($value)"},
		{"name": "is_map", "description": "Check if value is an object", "args": 1, "example": "is_map($value)"},
		{"name": "is_null", "description": "Check if value is null", "args": 1, "example": "is_null($value)"},
	}
}

// GetConditionOperators returns available comparison operators.
func GetConditionOperators() []map[string]any {
	return []map[string]any{
		{"operator": "==", "description": "Equal to", "example": "$status == 'success'"},
		{"operator": "!=", "description": "Not equal to", "example": "$status != 'failed'"},
		{"operator": ">", "description": "Greater than", "example": "$count > 0"},
		{"operator": ">=", "description": "Greater than or equal", "example": "$count >= 1"},
		{"operator": "<", "description": "Less than", "example": "$count < 10"},
		{"operator": "<=", "description": "Less than or equal", "example": "$count <= 100"},
		{"operator": "&&", "description": "Logical AND", "example": "$a > 0 && $b < 10"},
		{"operator": "||", "description": "Logical OR", "example": "$a == 'x' || $b == 'y'"},
		{"operator": "!", "description": "Logical NOT", "example": "!$disabled"},
		{"operator": "in", "description": "Membership check", "example": "$status in ['pending', 'running']"},
		{"operator": "not_in", "description": "Not in collection", "example": "$status not_in ['failed', 'cancelled']"},
	}
}

// GetEdgeTypes returns available edge types for the editor.
func GetEdgeTypes() []map[string]any {
	return []map[string]any{
		{"type": "default", "label": "Default", "description": "Default flow, always followed", "color": "#999"},
		{"type": "success", "label": "On Success", "description": "Followed when source node succeeds", "color": "#50C878"},
		{"type": "failure", "label": "On Failure", "description": "Followed when source node fails", "color": "#E74C3C"},
		{"type": "condition", "label": "Conditional", "description": "Followed when edge condition is true", "color": "#F5A623"},
		{"type": "condition_true", "label": "Condition True", "description": "Followed when condition result is true", "color": "#50C878"},
		{"type": "condition_false", "label": "Condition False", "description": "Followed when condition result is false", "color": "#E74C3C"},
	}
}

// GetVariableTypes returns available variable types for the editor.
func GetVariableTypes() []map[string]any {
	return []map[string]any{
		{"type": "string", "label": "String", "description": "Text value"},
		{"type": "number", "label": "Number", "description": "Numeric value"},
		{"type": "boolean", "label": "Boolean", "description": "True/false value"},
		{"type": "array", "label": "Array", "description": "List of values"},
		{"type": "object", "label": "Object", "description": "Key-value map"},
	}
}

// ValidateWorkflowJSON validates a workflow JSON against the schema rules.
func ValidateWorkflowJSON(data []byte) ([]string, error) {
	var graph Graph
	if err := json.Unmarshal(data, &graph); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	var warnings []string

	if err := graph.Validate(); err != nil {
		return warnings, err
	}

	// Check for orphan nodes (no incoming or outgoing edges)
	hasIncoming := make(map[string]bool)
	hasOutgoing := make(map[string]bool)
	for _, edge := range graph.Edges {
		hasOutgoing[edge.Source] = true
		hasIncoming[edge.Target] = true
	}

	for _, node := range graph.Nodes {
		if !hasIncoming[node.ID] && !hasOutgoing[node.ID] {
			warnings = append(warnings, fmt.Sprintf("node %q (%s) has no connections", node.Name, node.ID))
		}
	}

	// Check for unused variables
	definedVars := make(map[string]bool)
	for _, v := range graph.Variables {
		definedVars[v.Name] = true
	}

	jsonStr := string(data)
	for varName := range definedVars {
		if !strings.Contains(jsonStr, "$"+varName) {
			warnings = append(warnings, fmt.Sprintf("variable %q is defined but never used", varName))
		}
	}

	return warnings, nil
}

// GenerateWorkflowTemplate generates a workflow template from a description.
func GenerateWorkflowTemplate(name, description string, nodeTypes []string) *Graph {
	graph := NewGraph(name, description)

	var lastNodeID string
	for i, nodeType := range nodeTypes {
		var nodeID string
		switch nodeType {
		case "action":
			nodeID = graph.AddActionNode(
				fmt.Sprintf("step_%d", i+1),
				fmt.Sprintf("Action step %d", i+1),
				"builtin",
				"execute",
				map[string]any{},
			)
		case "condition":
			nodeID = graph.AddConditionNode(
				fmt.Sprintf("check_%d", i+1),
				fmt.Sprintf("Condition check %d", i+1),
				"$input != ''",
			)
		case "loop":
			node := Node{
				Type:     "loop",
				Name:     fmt.Sprintf("loop_%d", i+1),
				LoopVar:  "item",
				LoopOver: "$items",
			}
			graph.AddNode(node)
			nodeID = graph.Nodes[len(graph.Nodes)-1].ID
		}

		if lastNodeID != "" && nodeID != "" {
			graph.AddEdge(lastNodeID, nodeID, "default")
		}
		lastNodeID = nodeID
	}

	return graph
}
