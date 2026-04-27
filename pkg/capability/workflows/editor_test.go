package workflow

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGetWorkflowJSONSchemaShape(t *testing.T) {
	schema := GetWorkflowJSONSchema()
	if schema["title"] != "AnyClaw Workflow Graph" {
		t.Fatalf("schema title = %v, want workflow title", schema["title"])
	}
	if _, err := json.Marshal(schema); err != nil {
		t.Fatalf("schema should marshal as JSON: %v", err)
	}

	required := schema["required"].([]string)
	if !containsString(required, "id") || !containsString(required, "name") || !containsString(required, "nodes") {
		t.Fatalf("required = %v, want id/name/nodes", required)
	}

	props := schema["properties"].(map[string]any)
	nodes := props["nodes"].(map[string]any)
	if nodes["minItems"] != 1 {
		t.Fatalf("nodes minItems = %v, want 1", nodes["minItems"])
	}

	nodeProps := nodes["items"].(map[string]any)["properties"].(map[string]any)
	if _, ok := nodeProps["workflow"]; ok {
		t.Fatal("node schema should not expose workflow until sub-workflow execution is supported")
	}
	nodeType := nodeProps["type"].(map[string]any)
	enum := nodeType["enum"].([]string)
	for _, want := range []string{"action", "condition", "loop", "parallel", "join"} {
		if !containsString(enum, want) {
			t.Fatalf("node enum = %v, missing %q", enum, want)
		}
	}
	assertSchemaType(t, nodeProps["timeout_sec"], "integer")
	assertSchemaType(t, nodeProps["position"].(map[string]any)["properties"].(map[string]any)["x"], "number")

	retryProps := nodeProps["retry_policy"].(map[string]any)["properties"].(map[string]any)
	assertSchemaType(t, retryProps["max_attempts"], "integer")
	assertSchemaType(t, retryProps["initial_delay"], "integer")
	assertSchemaType(t, retryProps["max_delay"], "integer")
	assertSchemaType(t, retryProps["backoff_factor"], "number")

	errorProps := nodeProps["error_handling"].(map[string]any)["properties"].(map[string]any)
	assertSchemaType(t, errorProps["max_retries"], "integer")

	allOf := nodes["items"].(map[string]any)["allOf"].([]map[string]any)
	assertNodeTypeRequired(t, allOf, "action", []string{"plugin", "action"})
	assertNodeTypeRequired(t, allOf, "condition", []string{"condition"})
	assertNodeTypeRequired(t, allOf, "loop", []string{"loop_var", "loop_over"})
}

func TestNodePaletteTemplatesMatchSupportedNodeTypes(t *testing.T) {
	palette := GetNodePalette()
	if len(palette.Categories) == 0 {
		t.Fatal("expected node palette categories")
	}

	seen := map[string]bool{}
	for _, category := range palette.Categories {
		if category.Name == "" || category.Label == "" {
			t.Fatalf("category = %+v, want name and label", category)
		}
		for _, paletteNode := range category.Nodes {
			seen[paletteNode.Type] = true
			graph := NewGraph("palette "+paletteNode.Type, "")
			graph.AddNode(nodeFromPalette(paletteNode))
			if err := graph.Validate(); err != nil {
				t.Fatalf("palette node %q should validate: %v", paletteNode.Type, err)
			}
			if len(paletteNode.Ports.Inputs) == 0 && paletteNode.Type != "join" {
				t.Fatalf("palette node %q should expose input ports", paletteNode.Type)
			}
			if len(paletteNode.Ports.Outputs) == 0 {
				t.Fatalf("palette node %q should expose output ports", paletteNode.Type)
			}
		}
	}

	for _, want := range []string{"action", "condition", "loop", "parallel", "join"} {
		if !seen[want] {
			t.Fatalf("palette missing node type %q", want)
		}
	}
}

func TestEditorMetadataReturnsFreshValues(t *testing.T) {
	palette := GetNodePalette()
	palette.Categories[0].Nodes[0].Defaults["name"] = "mutated"
	nextPalette := GetNodePalette()
	if nextPalette.Categories[0].Nodes[0].Defaults["name"] == "mutated" {
		t.Fatal("GetNodePalette leaked mutable defaults across calls")
	}

	schema := GetWorkflowJSONSchema()
	props := schema["properties"].(map[string]any)
	props["name"] = "mutated"
	nextSchema := GetWorkflowJSONSchema()
	nextProps := nextSchema["properties"].(map[string]any)
	if nextProps["name"] == "mutated" {
		t.Fatal("GetWorkflowJSONSchema leaked mutable properties across calls")
	}
}

func TestConditionHelpersCoverEvaluatorSurface(t *testing.T) {
	helpers := GetConditionHelpers()
	names := make(map[string]bool, len(helpers))
	for _, helper := range helpers {
		if helper.Name == "" || helper.Signature == "" || helper.Description == "" {
			t.Fatalf("helper = %+v, want name/signature/description", helper)
		}
		if helper.Name == "is_null" && strings.Contains(helper.Description, "missing") {
			t.Fatalf("is_null description = %q, should not claim missing variables are null", helper.Description)
		}
		names[helper.Name] = true
	}

	for _, want := range []string{
		"contains",
		"starts_with",
		"ends_with",
		"empty",
		"not_empty",
		"length",
		"is_null",
		"is_string",
		"is_number",
		"is_bool",
		"is_array",
		"is_map",
	} {
		if !names[want] {
			t.Fatalf("condition helpers missing %q", want)
		}
	}
}

func nodeFromPalette(paletteNode PaletteNode) Node {
	defaults := paletteNode.Defaults
	node := Node{
		ID:          "node_" + paletteNode.Type,
		Type:        stringDefault(defaults["type"], paletteNode.Type),
		Name:        stringDefault(defaults["name"], paletteNode.Label),
		Description: paletteNode.Description,
		Plugin:      stringDefault(defaults["plugin"], ""),
		Action:      stringDefault(defaults["action"], ""),
		Condition:   stringDefault(defaults["condition"], ""),
		LoopVar:     stringDefault(defaults["loop_var"], ""),
		LoopOver:    stringDefault(defaults["loop_over"], ""),
	}
	if inputs, ok := defaults["inputs"].(map[string]any); ok {
		node.Inputs = inputs
	}
	return node
}

func stringDefault(value any, fallback string) string {
	if text, ok := value.(string); ok && text != "" {
		return text
	}
	return fallback
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func assertNodeTypeRequired(t *testing.T, rules []map[string]any, nodeType string, required []string) {
	t.Helper()
	for _, rule := range rules {
		ifSchema := rule["if"].(map[string]any)
		props := ifSchema["properties"].(map[string]any)
		typeSchema := props["type"].(map[string]any)
		if typeSchema["const"] != nodeType {
			continue
		}
		thenSchema := rule["then"].(map[string]any)
		got := thenSchema["required"].([]string)
		for _, want := range required {
			if !containsString(got, want) {
				t.Fatalf("%s required = %v, missing %q", nodeType, got, want)
			}
		}
		return
	}
	t.Fatalf("missing conditional required rule for %q", nodeType)
}

func assertSchemaType(t *testing.T, schema any, want string) {
	t.Helper()
	got, ok := schema.(map[string]any)["type"].(string)
	if !ok {
		t.Fatalf("schema = %#v, missing string type", schema)
	}
	if got != want {
		t.Fatalf("schema type = %q, want %q", got, want)
	}
}
