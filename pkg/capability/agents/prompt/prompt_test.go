package prompt

import (
	"strings"
	"testing"
)

func TestBuildSystemPromptIncludesCapabilityPlanningExecutionAndContext(t *testing.T) {
	out, err := BuildSystemPrompt("Ava", "Default description", PromptData{
		Description:  "Override description",
		SystemPrompt: "Always verify.",
		Personality:  "Direct and careful.",
		WorkingDir:   "/workspace/demo",
		CapabilityPlan: CapabilityPlanInfo{
			TaskClass:                "coding",
			Route:                    "marketplace",
			Need:                     strings.Repeat("find release-note helper ", 20),
			KindHint:                 "skill",
			TopLocalMatch:            CapabilityMatchInfo{Kind: "skill", Name: "release-notes", Score: 0.87, Reason: "matches docs"},
			Reason:                   "local tools are insufficient",
			ShouldExposeMarketSearch: true,
		},
		Tools: []ToolInfo{
			{Name: "read_file", Description: "Read files"},
			{Name: "run_command", Description: "Run commands"},
			{Name: "browser_snapshot", Description: "Observe browser"},
			{Name: "desktop_resolve_target", Description: "Resolve desktop target"},
			{Name: "market_search_artifacts", Description: "Search market"},
			{Name: "market_install_artifact", Description: "Install market item"},
		},
		CLIHub: &CLIHubInfo{
			Root:              "/cli",
			EntriesCount:      5,
			RunnableCount:     2,
			InstalledCount:    1,
			CapabilitiesCount: 4,
			Categories:        []string{"docs"},
			Runnable:          []string{"repo-health"},
			Installed:         []string{"zip"},
			SkillCommands:     []string{"skill_release_notes"},
			IntentExamples:    []string{"summarize repo"},
		},
		ClawBridge: &ClawBridgeInfo{
			Root:            "/bridge",
			CommandsCount:   3,
			ToolsCount:      4,
			SubsystemsCount: 2,
			CommandFamilies: []string{"git"},
			ToolFamilies:    []string{"desktop"},
			Subsystems:      []string{"runtime"},
		},
		WorkspaceFiles: []WorkspaceFile{{Name: "AGENTS.md", Content: "Use libraries first."}},
		AvailableSkills: []AvailableSkill{{
			Name:        "docs<skill>",
			Description: "Write & verify docs",
			Location:    "C:/skills/docs/SKILL.md",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Override description",
		"## Capability Planning",
		"market_search_artifacts",
		"market_install_artifact only after explicit user confirmation",
		"## Operating Mode",
		"## CLI Hub",
		"## Claw Bridge",
		"Working directory: /workspace/demo",
		"AGENTS.md",
		"docs&lt;skill&gt;",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("prompt missing %q:\n%s", want, out)
		}
	}
}

func TestPromptHelpersAndConversationFormatting(t *testing.T) {
	tools := []ToolInfo{
		{Name: "browser_navigate"},
		{Name: "browser_snapshot"},
		{Name: "desktop_click"},
		{Name: "desktop_wait_text"},
		{Name: "computer_action"},
		{Name: "app_mail_workflow_send"},
		{Name: "run_command"},
		{Name: "read_file"},
	}
	catalog := classifyExecutionTools(tools)
	if !catalog.HasCoreExecutionTools() || len(catalog.BrowserTools) != 2 || len(catalog.DesktopObservation) != 1 {
		t.Fatalf("unexpected catalog: %#v", catalog)
	}
	if got := formatToolNameList(tools, 3); !strings.Contains(got, "and 5 more") {
		t.Fatalf("unexpected formatted tools: %q", got)
	}
	if got := trimPromptLine("  alpha   beta gamma  ", 12); got != "alpha beta ..." {
		t.Fatalf("trimPromptLine = %q", got)
	}
	messages := BuildConversationPrompt([]Message{{Role: "user", Content: "hi"}}, "system")
	if len(messages) != 2 || messages[0]["role"] != "system" || messages[1]["content"] != "hi" {
		t.Fatalf("unexpected conversation prompt: %#v", messages)
	}
	call, err := FormatToolCall("write_file", map[string]any{"path": "a.txt"})
	if err != nil || !strings.Contains(call, "Tool: write_file") || !strings.Contains(call, "path: a.txt") {
		t.Fatalf("unexpected tool call: %q err=%v", call, err)
	}
}
