package orchestrator

import "testing"

func TestIsToolAllowedForPermissionReadOnlyDesktopTools(t *testing.T) {
	if !isToolAllowedForPermission("desktop_screenshot", "read-only") {
		t.Fatal("expected desktop_screenshot to remain available for read-only agents")
	}
	for _, toolName := range []string{"desktop_open", "desktop_type", "desktop_hotkey", "desktop_click"} {
		if isToolAllowedForPermission(toolName, "read-only") {
			t.Fatalf("expected %s to be hidden from read-only agents", toolName)
		}
	}
}
