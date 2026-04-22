package tools

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRegistryRegisterListAndUnregister(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&Tool{
		Name:        "visible",
		Description: "demo",
		Category:    ToolCategoryWeb,
		Handler: func(context.Context, map[string]any) (string, error) {
			return "ok", nil
		},
	})

	if _, ok := registry.Get("visible"); !ok {
		t.Fatal("expected registered tool to be found")
	}
	if len(registry.ListTools()) != 1 {
		t.Fatalf("expected one registered tool, got %d", len(registry.ListTools()))
	}
	if len(registry.GetToolsByCategory(ToolCategoryWeb)) != 1 {
		t.Fatal("expected category lookup to return registered tool")
	}
	if err := registry.Unregister("visible"); err != nil {
		t.Fatalf("Unregister: %v", err)
	}
	if _, ok := registry.Get("visible"); ok {
		t.Fatal("expected tool to be removed")
	}
}

func TestRegistryVisibilityAndAuthorization(t *testing.T) {
	registry := NewRegistry()
	registry.Register(&Tool{
		Name:        "main-only",
		Description: "restricted",
		Visibility:  ToolVisibilityMainAgentOnly,
		Handler: func(context.Context, map[string]any) (string, error) {
			return "secret", nil
		},
	})

	if len(registry.ListToolsForRole(true)) != 0 {
		t.Fatal("expected sub-agent listing to hide main-only tool")
	}
	if err := authorizeToolCall(WithToolCaller(context.Background(), ToolCaller{Role: ToolCallerRoleSubAgent}), &Tool{
		Name:       "main-only",
		Visibility: ToolVisibilityMainAgentOnly,
	}); err == nil {
		t.Fatal("expected sub-agent authorization to fail")
	}
	if err := authorizeToolCall(context.Background(), nil); err == nil {
		t.Fatal("expected nil tool authorization to fail")
	}
}

func TestRegistryCallAndRetry(t *testing.T) {
	registry := NewRegistry()
	attempts := 0
	registry.Register(&Tool{
		Name:       "retry",
		Retryable:  true,
		MaxRetries: 2,
		Handler: func(context.Context, map[string]any) (string, error) {
			attempts++
			if attempts < 2 {
				return "", errors.New("transient")
			}
			return "ok", nil
		},
	})

	result, err := registry.CallWithRetry(context.Background(), "retry", nil, 0)
	if err != nil {
		t.Fatalf("CallWithRetry: %v", err)
	}
	if result != "ok" || attempts != 2 {
		t.Fatalf("expected retry success after 2 attempts, got result=%q attempts=%d", result, attempts)
	}
}

func TestRegistryCacheAndDefinitions(t *testing.T) {
	registry := NewRegistry()
	calls := 0
	registry.cacheTTL = 10
	registry.Register(&Tool{
		Name:        "cached",
		Description: "cacheable",
		Category:    ToolCategoryCustom,
		CachePolicy: ToolCachePolicyDefault,
		Handler: func(context.Context, map[string]any) (string, error) {
			calls++
			return "value", nil
		},
	})

	if _, err := registry.Call(context.Background(), "cached", map[string]any{"id": 1}); err != nil {
		t.Fatalf("first cached call: %v", err)
	}
	if _, err := registry.Call(context.Background(), "cached", map[string]any{"id": 1}); err != nil {
		t.Fatalf("second cached call: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected cached tool handler to run once, got %d", calls)
	}

	jsonDefs, err := registry.GetToolDefinitionsJSON()
	if err != nil {
		t.Fatalf("GetToolDefinitionsJSON: %v", err)
	}
	if !strings.Contains(jsonDefs, `"cached"`) {
		t.Fatalf("expected tool definitions JSON to mention cached tool, got %q", jsonDefs)
	}

	registry.ClearCache()
	if _, found := registry.getFromCache(registry.generateCacheKey("cached", map[string]any{"id": 1})); found {
		t.Fatal("expected cache to be cleared")
	}

	list := registry.List()
	if len(list) != 1 || list[0].Name != "cached" {
		t.Fatalf("unexpected registry list output: %#v", list)
	}
	if len(registry.ListForRole(true)) != 1 {
		t.Fatal("expected ListForRole to return visible cached tool")
	}
}

func TestRegistryCallReturnsErrors(t *testing.T) {
	registry := NewRegistry()
	if _, err := registry.Call(context.Background(), "missing", nil); err == nil {
		t.Fatal("expected missing tool error")
	}

	registry.Register(&Tool{Name: "broken"})
	if _, err := registry.Call(context.Background(), "broken", nil); err == nil {
		t.Fatal("expected missing handler error")
	}
}
