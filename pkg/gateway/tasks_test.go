package gateway

import (
	"strings"
	"testing"

	"github.com/anyclaw/anyclaw/pkg/agent"
)

func TestRequiresToolApprovalIncludesDesktopTools(t *testing.T) {
	names := []string{
		"desktop_open",
		"desktop_type",
		"desktop_hotkey",
		"desktop_click",
		"desktop_move",
		"desktop_double_click",
		"desktop_scroll",
		"desktop_drag",
		"desktop_wait",
		"desktop_list_windows",
		"desktop_wait_window",
		"desktop_focus_window",
		"desktop_inspect_ui",
		"desktop_invoke_ui",
		"desktop_set_value_ui",
		"desktop_resolve_target",
		"desktop_activate_target",
		"desktop_set_target_value",
		"desktop_screenshot",
		"desktop_match_image",
		"desktop_click_image",
		"desktop_wait_image",
		"desktop_ocr",
		"desktop_verify_text",
		"desktop_find_text",
		"desktop_click_text",
		"desktop_wait_text",
		"desktop_plan",
	}
	for _, name := range names {
		if !requiresToolApproval(agent.ToolCall{Name: name}) {
			t.Fatalf("%s should require approval", name)
		}
	}
}

func TestDefaultPlanIncludesVerificationStep(t *testing.T) {
	summary, steps := defaultPlan("ship the release")
	if !strings.Contains(summary, "verify the observable outcome") {
		t.Fatalf("expected summary to mention verification, got %q", summary)
	}
	foundVerify := false
	for _, step := range steps {
		if step.Kind == "verify" {
			foundVerify = true
			break
		}
	}
	if !foundVerify {
		t.Fatalf("expected default plan to include a verify step, got %#v", steps)
	}
}
