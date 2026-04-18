package runtime

import (
	"context"
	"testing"

	agent "github.com/1024XEngineer/anyclaw/pkg/capability/agents"
	"github.com/1024XEngineer/anyclaw/pkg/capability/skills"
	"github.com/1024XEngineer/anyclaw/pkg/capability/tools"
	"github.com/1024XEngineer/anyclaw/pkg/state"
	"github.com/1024XEngineer/anyclaw/pkg/state/memory"
)

func TestAppExecuteReplacesHistoryThroughRuntimeBoundary(t *testing.T) {
	mem := memory.NewFileMemory(t.TempDir())
	if err := mem.Init(); err != nil {
		t.Fatalf("memory init: %v", err)
	}
	t.Cleanup(func() { mem.Close() })

	app := &App{
		Agent: agent.New(agent.Config{
			Name:   "runtime-test",
			LLM:    &testRuntimeLLM{},
			Memory: mem,
			Skills: skills.NewSkillsManager(""),
			Tools:  tools.NewRegistry(),
		}),
	}

	result, err := app.Execute(context.Background(), ExecutionRequest{
		Input:          "hello",
		History:        []state.HistoryMessage{{Role: "user", Content: "previous"}},
		ReplaceHistory: true,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == nil || result.Output != "delegated-result" {
		t.Fatalf("expected delegated-result, got %#v", result)
	}

	history := app.Agent.GetHistory()
	if len(history) < 3 {
		t.Fatalf("expected runtime execution to preserve previous history and append exchange, got %#v", history)
	}
	if history[0].Content != "previous" {
		t.Fatalf("expected first history entry to come from runtime request, got %#v", history[0])
	}
}

func TestAppStreamUsesRuntimeBoundary(t *testing.T) {
	mem := memory.NewFileMemory(t.TempDir())
	if err := mem.Init(); err != nil {
		t.Fatalf("memory init: %v", err)
	}
	t.Cleanup(func() { mem.Close() })

	app := &App{
		Agent: agent.New(agent.Config{
			Name:   "runtime-test",
			LLM:    &testRuntimeLLM{},
			Memory: mem,
			Skills: skills.NewSkillsManager(""),
			Tools:  tools.NewRegistry(),
		}),
	}

	streamed := ""
	result, err := app.Stream(context.Background(), ExecutionRequest{Input: "stream this"}, func(chunk string) {
		streamed += chunk
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if streamed != "delegated-result" {
		t.Fatalf("expected streamed delegated-result, got %q", streamed)
	}
	if result == nil || result.Output != "delegated-result" {
		t.Fatalf("expected stream result delegated-result, got %#v", result)
	}
}
