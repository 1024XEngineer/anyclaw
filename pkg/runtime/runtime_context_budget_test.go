package runtime

import (
	"testing"

	runtimecontext "github.com/1024XEngineer/anyclaw/pkg/runtime/context"
)

func TestDeriveAgentContextTokenBudgetUsesContextFloor(t *testing.T) {
	got := runtimecontext.DeriveAgentContextTokenBudget(4096)
	if got != 16384 {
		t.Fatalf("expected 16384 context budget for llm max tokens 4096, got %d", got)
	}
}

func TestDeriveAgentContextTokenBudgetScalesForLargerCompletionBudgets(t *testing.T) {
	got := runtimecontext.DeriveAgentContextTokenBudget(12000)
	if got != 24000 {
		t.Fatalf("expected doubled context budget for larger completion cap, got %d", got)
	}
}
