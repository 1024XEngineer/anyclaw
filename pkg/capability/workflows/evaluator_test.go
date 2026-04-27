package workflow

import (
	"strings"
	"testing"
)

func TestEvalConditionComparisonsAndLogic(t *testing.T) {
	vars := map[string]any{
		"name":   "anyclaw",
		"score":  42,
		"active": true,
	}

	tests := []struct {
		name string
		expr string
		want bool
	}{
		{
			name: "comparison true",
			expr: "$score >= 40",
			want: true,
		},
		{
			name: "comparison false",
			expr: "$score < 40",
			want: false,
		},
		{
			name: "and precedence",
			expr: "$active && $score == 42",
			want: true,
		},
		{
			name: "or short circuit",
			expr: "$active || $missing == true",
			want: true,
		},
		{
			name: "not expression",
			expr: "!($score < 40)",
			want: true,
		},
		{
			name: "string equality",
			expr: "$name == 'anyclaw'",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EvalCondition(tt.expr, vars)
			if err != nil {
				t.Fatalf("EvalCondition: %v", err)
			}
			if got != tt.want {
				t.Fatalf("EvalCondition(%q) = %v, want %v", tt.expr, got, tt.want)
			}
		})
	}
}

func TestEvalConditionFunctionsMembershipAndTypeChecks(t *testing.T) {
	vars := map[string]any{
		"title":  "runtime orchestration",
		"tags":   []any{"runtime", "workflow"},
		"config": map[string]any{"enabled": true},
		"empty":  "",
	}

	tests := []struct {
		name string
		expr string
		want bool
	}{
		{
			name: "contains string",
			expr: "contains($title, 'runtime')",
			want: true,
		},
		{
			name: "starts with",
			expr: "starts_with($title, 'runtime')",
			want: true,
		},
		{
			name: "ends with",
			expr: "ends_with($title, 'orchestration')",
			want: true,
		},
		{
			name: "empty",
			expr: "empty($empty)",
			want: true,
		},
		{
			name: "not empty",
			expr: "not_empty($title)",
			want: true,
		},
		{
			name: "length",
			expr: "length($tags) == 2",
			want: true,
		},
		{
			name: "in array",
			expr: "'workflow' in $tags",
			want: true,
		},
		{
			name: "not in array",
			expr: "'canvas' not_in $tags",
			want: true,
		},
		{
			name: "is array",
			expr: "is_array($tags)",
			want: true,
		},
		{
			name: "is map",
			expr: "is_map($config)",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EvalCondition(tt.expr, vars)
			if err != nil {
				t.Fatalf("EvalCondition: %v", err)
			}
			if got != tt.want {
				t.Fatalf("EvalCondition(%q) = %v, want %v", tt.expr, got, tt.want)
			}
		})
	}
}

func TestEvalConditionNodeOutputReference(t *testing.T) {
	vars := map[string]any{
		"_node_outputs:fetch": map[string]any{
			"status": "ok",
			"body": map[string]any{
				"count": 3,
			},
		},
	}

	got, err := EvalCondition("$fetch.status == 'ok' && $fetch.body.count == 3", vars)
	if err != nil {
		t.Fatalf("EvalCondition: %v", err)
	}
	if !got {
		t.Fatal("expected node output reference to evaluate true")
	}
}

func TestEvalConditionRejectsInvalidExpressions(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
	}{
		{
			name: "empty",
			expr: "",
			want: "empty condition expression",
		},
		{
			name: "function arity",
			expr: "contains('only-one-arg')",
			want: "contains() requires 2 arguments",
		},
		{
			name: "nested function arity",
			expr: "is_string(contains('only-one-arg'))",
			want: "contains() requires 2 arguments",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := EvalCondition(tt.expr, nil)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}
