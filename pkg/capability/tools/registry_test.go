package tools

import (
	"context"
	"fmt"
	"testing"
)

func TestRegistryDefaultToolsDoNotCache(t *testing.T) {
	registry := NewRegistry()
	calls := 0
	registry.RegisterTool("custom_counter", "counter", nil, func(ctx context.Context, input map[string]any) (string, error) {
		calls++
		return fmt.Sprintf("%d", calls), nil
	})

	first, err := registry.Call(context.Background(), "custom_counter", map[string]any{"key": "same"})
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	second, err := registry.Call(context.Background(), "custom_counter", map[string]any{"key": "same"})
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if calls != 2 || first == second {
		t.Fatalf("expected default policy to execute twice, calls=%d first=%q second=%q", calls, first, second)
	}
}

func TestRegistryCachesOnlyExplicitCachePolicy(t *testing.T) {
	registry := NewRegistry()
	calls := 0
	registry.Register(&Tool{
		Name:        "cached_counter",
		Description: "counter",
		CachePolicy: ToolCachePolicyCache,
		Handler: func(ctx context.Context, input map[string]any) (string, error) {
			calls++
			return fmt.Sprintf("%d", calls), nil
		},
	})

	first, err := registry.Call(context.Background(), "cached_counter", map[string]any{"key": "same"})
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	second, err := registry.Call(context.Background(), "cached_counter", map[string]any{"key": "same"})
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if calls != 1 || first != second {
		t.Fatalf("expected explicit cache policy to reuse result, calls=%d first=%q second=%q", calls, first, second)
	}
}

func TestRegistryInfersCategoriesForConvenienceRegistration(t *testing.T) {
	registry := NewRegistry()
	registry.RegisterTool("browser_navigate", "navigate", nil, func(ctx context.Context, input map[string]any) (string, error) {
		return "ok", nil
	})
	registry.Register(&Tool{
		Name:        "run_command",
		Description: "run",
		Handler: func(ctx context.Context, input map[string]any) (string, error) {
			return "ok", nil
		},
	})

	browserTool, ok := registry.Get("browser_navigate")
	if !ok || browserTool.Category != ToolCategoryBrowser {
		t.Fatalf("expected browser_navigate category browser, got %#v", browserTool)
	}
	commandTool, ok := registry.Get("run_command")
	if !ok || commandTool.Category != ToolCategoryCommand {
		t.Fatalf("expected run_command category command, got %#v", commandTool)
	}
	if got := registry.GetToolsByCategory(ToolCategoryBrowser); len(got) != 1 || got[0].Name != "browser_navigate" {
		t.Fatalf("expected browser category index to include browser_navigate, got %#v", got)
	}
}
