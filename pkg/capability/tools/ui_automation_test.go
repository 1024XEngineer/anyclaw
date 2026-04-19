package tools

import (
	"testing"
)

func TestResolveDesktopAutomationSelectorDefaults(t *testing.T) {
	selector, err := resolveDesktopAutomationSelector(map[string]any{
		"title": "Notepad",
		"name":  "Save",
	}, true)
	if err != nil {
		t.Fatalf("resolveDesktopAutomationSelector: %v", err)
	}
	if selector.Match != "contains" {
		t.Fatalf("expected contains match default, got %q", selector.Match)
	}
	if selector.Scope != "descendants" {
		t.Fatalf("expected descendants scope default, got %q", selector.Scope)
	}
	if selector.Index != 1 {
		t.Fatalf("expected index 1 default, got %d", selector.Index)
	}
	if selector.MaxElements != 50 {
		t.Fatalf("expected maxElements 50 default, got %d", selector.MaxElements)
	}
}

func TestResolveDesktopAutomationSelectorRequiresWindowTarget(t *testing.T) {
	_, err := resolveDesktopAutomationSelector(map[string]any{
		"name": "Save",
	}, true)
	if err == nil {
		t.Fatal("expected missing window selection to be rejected")
	}
}

func TestUnmarshalJSONObjectOrArrayHandlesArrayAndObject(t *testing.T) {
	items, err := unmarshalJSONObjectOrArray[desktopWindowInfo](`[{"title":"Notepad"},{"title":"Calculator"}]`)
	if err != nil {
		t.Fatalf("unmarshal array: %v", err)
	}
	if len(items) != 2 || items[1].Title != "Calculator" {
		t.Fatalf("unexpected array items: %#v", items)
	}

	items, err = unmarshalJSONObjectOrArray[desktopWindowInfo](`{"title":"Single"}`)
	if err != nil {
		t.Fatalf("unmarshal object: %v", err)
	}
	if len(items) != 1 || items[0].Title != "Single" {
		t.Fatalf("unexpected object items: %#v", items)
	}
}

func TestNormalizeDesktopScopeAndMatch(t *testing.T) {
	if normalizeDesktopMatchMode("exact") != "exact" {
		t.Fatal("expected exact match mode")
	}
	if normalizeDesktopMatchMode("weird") != "contains" {
		t.Fatal("expected unknown match mode to fall back to contains")
	}
	if normalizeDesktopScope("children") != "children" {
		t.Fatal("expected children scope")
	}
	if normalizeDesktopScope("weird") != "descendants" {
		t.Fatal("expected unknown scope to fall back to descendants")
	}
}
