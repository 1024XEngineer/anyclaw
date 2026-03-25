package gateway

import (
	"testing"

	"github.com/anyclaw/anyclaw/pkg/agent"
)

func TestRequiresToolApprovalIncludesDesktopTools(t *testing.T) {
	names := []string{
		"desktop_open",
		"desktop_type",
		"desktop_hotkey",
		"desktop_click",
		"desktop_screenshot",
	}
	for _, name := range names {
		if !requiresToolApproval(agent.ToolCall{Name: name}) {
			t.Fatalf("%s should require approval", name)
		}
	}
}
