package workflow

import (
	"encoding/json"
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

func TestEvalConditionNumericCoercion(t *testing.T) {
	vars := map[string]any{
		"small":  int32(42),
		"ratio":  float32(0.75),
		"amount": json.Number("100.5"),
	}

	tests := []struct {
		name string
		expr string
	}{
		{
			name: "int32",
			expr: "$small < 100",
		},
		{
			name: "float32",
			expr: "$ratio >= 0.5",
		},
		{
			name: "json number",
			expr: "$amount > 100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EvalCondition(tt.expr, vars)
			if err != nil {
				t.Fatalf("EvalCondition: %v", err)
			}
			if !got {
				t.Fatalf("EvalCondition(%q) = false, want true", tt.expr)
			}
		})
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
		{
			name: "bare not",
			expr: "!",
			want: "empty expression",
		},
		{
			name: "trailing logical operator",
			expr: "$active &&",
			want: "empty expression",
		},
		{
			name: "unbalanced parenthesis",
			expr: "($score < 40",
			want: "unbalanced delimiter",
		},
		{
			name: "invalid operator fragment",
			expr: "$active &&& $missing",
			want: "invalid expression syntax",
		},
		{
			name: "unterminated string",
			expr: "$name == 'anyclaw",
			want: "unterminated string literal",
		},
		{
			name: "unsupported oversized uint64",
			expr: "$huge < 100",
			want: "cannot compare numeric and non-numeric values",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vars := map[string]any{
				"huge": uint64(^uint64(0)),
			}
			_, err := EvalCondition(tt.expr, vars)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}
