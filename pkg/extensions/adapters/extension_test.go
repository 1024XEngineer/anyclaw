package extension

import "testing"

func TestBuiltinExtensionCatalogHasRecommendedCount(t *testing.T) {
	if got := len(builtinExtensionManifests()); got != 22 {
		t.Fatalf("expected 22 builtin extensions, got %d", got)
	}
}

func TestDiscoverIncludesBuiltinExtensionsWithoutDirectory(t *testing.T) {
	registry := NewRegistry("missing-dir")
	items, err := registry.Discover()
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(items) != 22 {
		t.Fatalf("expected 22 builtin extensions from discover, got %d", len(items))
	}
}

func TestLoadAllRegistersBuiltinExtensionsWithoutDirectory(t *testing.T) {
	registry := NewRegistry("missing-dir")
	if err := registry.LoadAll(); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if got := len(registry.List()); got != 22 {
		t.Fatalf("expected 22 loaded builtin extensions, got %d", got)
	}
	ext, ok := registry.Get("zoom")
	if !ok {
		t.Fatal("expected zoom builtin extension")
	}
	if !ext.Manifest.Builtin {
		t.Fatal("expected builtin flag on zoom manifest")
	}
}
