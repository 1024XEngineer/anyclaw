package orchestrator

import "testing"

func TestIsToolAllowedForPermissionReadOnlyDesktopTools(t *testing.T) {
	if !isToolAllowedForPermission("desktop_screenshot", "read-only") {
		t.Fatal("expected desktop_screenshot to remain available for read-only agents")
	}
	for _, toolName := range []string{"desktop_list_windows", "desktop_wait_window", "desktop_inspect_ui", "desktop_resolve_target", "desktop_match_image", "desktop_wait_image", "desktop_ocr", "desktop_verify_text", "desktop_find_text", "desktop_wait_text"} {
		if !isToolAllowedForPermission(toolName, "read-only") {
			t.Fatalf("expected %s to remain available for read-only agents", toolName)
		}
	}
	for _, toolName := range []string{"desktop_open", "desktop_type", "desktop_hotkey", "desktop_click"} {
		if isToolAllowedForPermission(toolName, "read-only") {
			t.Fatalf("expected %s to be hidden from read-only agents", toolName)
		}
	}
}
