package workflow

// WorkflowEditorSchema describes the graph JSON surface exposed to visual editors.
type WorkflowEditorSchema struct {
	Schema      string         `json:"$schema"`
	Title       string         `json:"title"`
	Type        string         `json:"type"`
	Required    []string       `json:"required,omitempty"`
	Properties  map[string]any `json:"properties"`
	Definitions map[string]any `json:"definitions,omitempty"`
}

// NodePalette groups node templates that a visual editor can offer to users.
type NodePalette struct {
	Categories []PaletteCategory `json:"categories"`
}

type PaletteCategory struct {
	Name  string        `json:"name"`
	Label string        `json:"label"`
	Nodes []PaletteNode `json:"nodes"`
}

type PaletteNode struct {
	Type        string         `json:"type"`
	Label       string         `json:"label"`
	Description string         `json:"description"`
	Defaults    map[string]any `json:"defaults"`
	Ports       NodePorts      `json:"ports"`
}

type NodePorts struct {
	Inputs  []Port `json:"inputs,omitempty"`
	Outputs []Port `json:"outputs,omitempty"`
}

type Port struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

type ConditionHelper struct {
	Name        string   `json:"name"`
	Signature   string   `json:"signature"`
	Description string   `json:"description"`
	Examples    []string `json:"examples,omitempty"`
}

// GetWorkflowJSONSchema returns a JSON-schema-compatible description of Graph.
func GetWorkflowJSONSchema() map[string]any {
	return map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"title":   "AnyClaw Workflow Graph",
		"type":    "object",
		"required": []string{
			"id",
			"name",
			"nodes",
		},
		"additionalProperties": true,
		"properties": map[string]any{
			"id":          stringProperty("Unique workflow graph ID."),
			"name":        stringProperty("Human-readable workflow name."),
			"description": stringProperty("Optional workflow description."),
			"version":     stringProperty("Optional semantic or product-specific version."),
			"author":      stringProperty("Optional workflow author."),
			"created_at":  dateTimeProperty("Creation timestamp."),
			"updated_at":  dateTimeProperty("Last update timestamp."),
			"nodes":       arrayProperty(nodeSchema(), 1, "Workflow nodes."),
			"edges":       arrayProperty(edgeSchema(), 0, "Directed links between workflow nodes."),
			"inputs":      arrayProperty(inputParamSchema(), 0, "Graph-level input parameters."),
			"outputs":     arrayProperty(outputParamSchema(), 0, "Graph-level output parameters."),
			"variables":   arrayProperty(variableSchema(), 0, "Graph-level variables."),
			"metadata":    freeformObjectProperty("Arbitrary workflow metadata."),
			"tags":        arrayProperty(stringProperty("Workflow tag."), 0, "Workflow tags."),
		},
		"definitions": map[string]any{
			"node":       nodeSchema(),
			"edge":       edgeSchema(),
			"input":      inputParamSchema(),
			"output":     outputParamSchema(),
			"variable":   variableSchema(),
			"retry":      retryPolicySchema(),
			"on_error":   errorHandlingSchema(),
			"position":   positionSchema(),
			"expression": expressionSchema(),
		},
	}
}

// GetNodePalette returns editor-ready templates for supported node types.
func GetNodePalette() NodePalette {
	return NodePalette{
		Categories: []PaletteCategory{
			{
				Name:  "control",
				Label: "Control Flow",
				Nodes: []PaletteNode{
					{
						Type:        "condition",
						Label:       "Condition",
						Description: "Branch execution by evaluating a condition expression.",
						Defaults: map[string]any{
							"type":      "condition",
							"name":      "Condition",
							"condition": "$input == true",
						},
						Ports: NodePorts{
							Inputs: []Port{{ID: "in", Label: "In", Type: "flow"}},
							Outputs: []Port{
								{ID: "true", Label: "True", Type: "flow"},
								{ID: "false", Label: "False", Type: "flow"},
							},
						},
					},
					{
						Type:        "loop",
						Label:       "Loop",
						Description: "Iterate over an array value and execute downstream nodes per item.",
						Defaults: map[string]any{
							"type":      "loop",
							"name":      "Loop",
							"loop_var":  "item",
							"loop_over": "$items",
						},
						Ports: NodePorts{
							Inputs:  []Port{{ID: "in", Label: "In", Type: "flow"}},
							Outputs: []Port{{ID: "each", Label: "Each", Type: "flow"}},
						},
					},
					{
						Type:        "parallel",
						Label:       "Parallel",
						Description: "Fan out execution to multiple branches.",
						Defaults: map[string]any{
							"type": "parallel",
							"name": "Parallel",
						},
						Ports: NodePorts{
							Inputs:  []Port{{ID: "in", Label: "In", Type: "flow"}},
							Outputs: []Port{{ID: "branch", Label: "Branch", Type: "flow"}},
						},
					},
					{
						Type:        "join",
						Label:       "Join",
						Description: "Join multiple branches back into one path.",
						Defaults: map[string]any{
							"type": "join",
							"name": "Join",
						},
						Ports: NodePorts{
							Inputs:  []Port{{ID: "branches", Label: "Branches", Type: "flow"}},
							Outputs: []Port{{ID: "out", Label: "Out", Type: "flow"}},
						},
					},
				},
			},
			{
				Name:  "actions",
				Label: "Actions",
				Nodes: []PaletteNode{
					{
						Type:        "action",
						Label:       "Action",
						Description: "Invoke a plugin action with resolved inputs.",
						Defaults: map[string]any{
							"type":   "action",
							"name":   "Action",
							"plugin": "plugin-name",
							"action": "action-name",
							"inputs": map[string]any{},
						},
						Ports: NodePorts{
							Inputs:  []Port{{ID: "in", Label: "In", Type: "flow"}},
							Outputs: []Port{{ID: "out", Label: "Out", Type: "flow"}},
						},
					},
				},
			},
		},
	}
}

// GetConditionHelpers returns the condition helpers supported by EvalCondition.
func GetConditionHelpers() []ConditionHelper {
	return []ConditionHelper{
		{
			Name:        "contains",
			Signature:   "contains(value, needle)",
			Description: "Checks whether a string, array, or map contains a value.",
			Examples:    []string{`contains($title, "urgent")`, `contains($tags, "todo")`},
		},
		{
			Name:        "starts_with",
			Signature:   "starts_with(value, prefix)",
			Description: "Checks whether a string starts with a prefix.",
			Examples:    []string{`starts_with($branch, "release/")`},
		},
		{
			Name:        "ends_with",
			Signature:   "ends_with(value, suffix)",
			Description: "Checks whether a string ends with a suffix.",
			Examples:    []string{`ends_with($file, ".md")`},
		},
		{
			Name:        "empty",
			Signature:   "empty(value)",
			Description: "Checks whether a value is empty.",
			Examples:    []string{`empty($assignee)`},
		},
		{
			Name:        "not_empty",
			Signature:   "not_empty(value)",
			Description: "Checks whether a value is not empty.",
			Examples:    []string{`not_empty($summary)`},
		},
		{
			Name:        "length",
			Signature:   "length(value)",
			Description: "Returns the length of a string, array, or map.",
			Examples:    []string{`length($items) > 0`},
		},
		{
			Name:        "is_null",
			Signature:   "is_null(value)",
			Description: "Checks whether a value is explicitly null.",
			Examples:    []string{`is_null($error)`},
		},
		{
			Name:        "is_string",
			Signature:   "is_string(value)",
			Description: "Checks whether a value is a string.",
		},
		{
			Name:        "is_number",
			Signature:   "is_number(value)",
			Description: "Checks whether a value is numeric.",
		},
		{
			Name:        "is_bool",
			Signature:   "is_bool(value)",
			Description: "Checks whether a value is boolean.",
		},
		{
			Name:        "is_array",
			Signature:   "is_array(value)",
			Description: "Checks whether a value is an array.",
		},
		{
			Name:        "is_map",
			Signature:   "is_map(value)",
			Description: "Checks whether a value is an object/map.",
		},
	}
}

func nodeSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": true,
		"required":             []string{"id", "type", "name"},
		"allOf": []map[string]any{
			nodeTypeRequiredSchema("action", []string{"plugin", "action"}),
			nodeTypeRequiredSchema("condition", []string{"condition"}),
			nodeTypeRequiredSchema("loop", []string{"loop_var", "loop_over"}),
		},
		"properties": map[string]any{
			"id":             stringProperty("Unique node ID."),
			"type":           enumStringProperty("Node type.", []string{"action", "condition", "loop", "parallel", "join"}),
			"name":           stringProperty("Human-readable node name."),
			"description":    stringProperty("Optional node description."),
			"plugin":         stringProperty("Plugin name for action nodes."),
			"action":         stringProperty("Plugin action name for action nodes."),
			"inputs":         freeformObjectProperty("Node input values."),
			"outputs":        freeformStringMapProperty("Node output name mappings."),
			"condition":      expressionSchema(),
			"loop_var":       stringProperty("Variable name used by loop nodes."),
			"loop_over":      expressionSchema(),
			"timeout_sec":    integerProperty("Node timeout in seconds."),
			"retry_policy":   retryPolicySchema(),
			"error_handling": errorHandlingSchema(),
			"position":       positionSchema(),
		},
	}
}

func nodeTypeRequiredSchema(nodeType string, required []string) map[string]any {
	return map[string]any{
		"if": map[string]any{
			"properties": map[string]any{
				"type": map[string]any{
					"const": nodeType,
				},
			},
			"required": []string{"type"},
		},
		"then": map[string]any{
			"required": append([]string(nil), required...),
		},
	}
}

func edgeSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": true,
		"required":             []string{"id", "source", "target", "type"},
		"properties": map[string]any{
			"id":        stringProperty("Unique edge ID."),
			"source":    stringProperty("Source node ID."),
			"target":    stringProperty("Target node ID."),
			"type":      stringProperty("Edge type, such as default, condition_true, or condition_false."),
			"condition": expressionSchema(),
			"label":     stringProperty("Optional edge label."),
		},
	}
}

func inputParamSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": true,
		"required":             []string{"name", "type"},
		"properties": map[string]any{
			"name":        stringProperty("Input name."),
			"type":        stringProperty("Input type."),
			"description": stringProperty("Input description."),
			"required":    booleanProperty("Whether the input is required."),
			"default":     map[string]any{"description": "Default input value."},
		},
	}
}

func outputParamSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": true,
		"required":             []string{"name", "type"},
		"properties": map[string]any{
			"name":        stringProperty("Output name."),
			"type":        stringProperty("Output type."),
			"description": stringProperty("Output description."),
			"source":      stringProperty("Source expression or node output reference."),
		},
	}
}

func variableSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": true,
		"required":             []string{"name", "type"},
		"properties": map[string]any{
			"name":          stringProperty("Variable name."),
			"type":          stringProperty("Variable type."),
			"description":   stringProperty("Variable description."),
			"initial_value": map[string]any{"description": "Initial variable value."},
		},
	}
}

func retryPolicySchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"max_attempts":   integerProperty("Maximum retry attempts."),
			"initial_delay":  integerProperty("Initial retry delay."),
			"max_delay":      integerProperty("Maximum retry delay."),
			"backoff_factor": numberProperty("Retry backoff multiplier."),
		},
	}
}

func errorHandlingSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": true,
		"properties": map[string]any{
			"on_error":    stringProperty("Error strategy."),
			"target_node": stringProperty("Target node for error routing."),
			"max_retries": integerProperty("Maximum handler retries."),
		},
	}
}

func positionSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"x": numberProperty("Horizontal editor position."),
			"y": numberProperty("Vertical editor position."),
		},
	}
}

func expressionSchema() map[string]any {
	return stringProperty("Workflow expression or condition.")
}

func stringProperty(description string) map[string]any {
	return map[string]any{
		"type":        "string",
		"description": description,
	}
}

func enumStringProperty(description string, values []string) map[string]any {
	return map[string]any{
		"type":        "string",
		"description": description,
		"enum":        append([]string(nil), values...),
	}
}

func dateTimeProperty(description string) map[string]any {
	schema := stringProperty(description)
	schema["format"] = "date-time"
	return schema
}

func numberProperty(description string) map[string]any {
	return map[string]any{
		"type":        "number",
		"description": description,
	}
}

func integerProperty(description string) map[string]any {
	return map[string]any{
		"type":        "integer",
		"description": description,
	}
}

func booleanProperty(description string) map[string]any {
	return map[string]any{
		"type":        "boolean",
		"description": description,
	}
}

func arrayProperty(items any, minItems int, description string) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": description,
		"items":       items,
		"minItems":    minItems,
	}
}

func freeformObjectProperty(description string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"description":          description,
		"additionalProperties": true,
	}
}

func freeformStringMapProperty(description string) map[string]any {
	return map[string]any{
		"type":        "object",
		"description": description,
		"additionalProperties": map[string]any{
			"type": "string",
		},
	}
}
