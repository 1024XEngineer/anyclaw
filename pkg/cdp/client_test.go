package cdp

import "testing"

func TestAllocatorFlagOverridesHeadlessFalse(t *testing.T) {
	overrides := (&CDPOptions{Headless: false}).allocatorFlagOverrides()
	got := map[string]any{}
	for _, override := range overrides {
		got[override.name] = override.value
	}

	want := map[string]any{
		"headless":        false,
		"hide-scrollbars": false,
		"mute-audio":      false,
	}

	for key, value := range want {
		if got[key] != value {
			t.Fatalf("override %q = %v, want %v", key, got[key], value)
		}
	}
}

func TestAllocatorFlagOverridesHeadlessTrue(t *testing.T) {
	overrides := (&CDPOptions{
		Headless:      true,
		DisableImages: true,
		CacheDisabled: true,
	}).allocatorFlagOverrides()

	got := map[string]any{}
	for _, override := range overrides {
		got[override.name] = override.value
	}

	if _, exists := got["headless"]; exists {
		t.Fatal("unexpected headless override when Headless is true")
	}
	if got["disable-images"] != true {
		t.Fatalf("disable-images override = %v, want true", got["disable-images"])
	}
	if got["disk-cache-size"] != 0 {
		t.Fatalf("disk-cache-size override = %v, want 0", got["disk-cache-size"])
	}
}
