package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdvanceBootstrapRitualCompletesAndRemovesBootstrapFile(t *testing.T) {
	dir := t.TempDir()
	if err := EnsureBootstrap(dir, BootstrapOptions{
		AgentName:        "assistant",
		AgentDescription: "Execution helper",
	}); err != nil {
		t.Fatalf("EnsureBootstrap: %v", err)
	}

	result, err := AdvanceBootstrapRitual(dir, "please help me", BootstrapRitualOptions{
		AgentName:        "assistant",
		AgentDescription: "Execution helper",
	})
	if err != nil {
		t.Fatalf("AdvanceBootstrapRitual(start): %v", err)
	}
	if !strings.Contains(result.Response, "Question 1/4") {
		t.Fatalf("expected first question, got %q", result.Response)
	}

	answers := []string{
		"Call me Alex and default to Chinese.",
		"Help with local coding work.",
		"Be concise and proactive.",
		"Avoid destructive commands without confirmation.",
	}
	for _, answer := range answers {
		result, err = AdvanceBootstrapRitual(dir, answer, BootstrapRitualOptions{
			AgentName:        "assistant",
			AgentDescription: "Execution helper",
		})
		if err != nil {
			t.Fatalf("AdvanceBootstrapRitual(answer): %v", err)
		}
	}

	if !result.Completed {
		t.Fatalf("expected ritual completion, got %#v", result)
	}
	if _, err := os.Stat(filepath.Join(dir, "BOOTSTRAP.md")); !os.IsNotExist(err) {
		t.Fatalf("expected BOOTSTRAP.md to be removed, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".anyclaw-bootstrap-state.json")); !os.IsNotExist(err) {
		t.Fatalf("expected bootstrap state to be removed, stat err=%v", err)
	}

	userData, err := os.ReadFile(filepath.Join(dir, "USER.md"))
	if err != nil {
		t.Fatalf("ReadFile(USER.md): %v", err)
	}
	if !strings.Contains(string(userData), "Call me Alex and default to Chinese.") {
		t.Fatalf("expected USER.md to include bootstrap answer, got %q", string(userData))
	}
	if !strings.Contains(string(userData), "<!-- anyclaw:bootstrap:start -->") {
		t.Fatalf("expected managed bootstrap block, got %q", string(userData))
	}
}

func TestAdvanceBootstrapRitualUsesConfiguredIntro(t *testing.T) {
	dir := t.TempDir()
	if err := EnsureBootstrap(dir, BootstrapOptions{
		AgentName:        "HelperBot",
		AgentDescription: "a focused workspace copilot",
	}); err != nil {
		t.Fatalf("EnsureBootstrap: %v", err)
	}

	result, err := AdvanceBootstrapRitual(dir, "", BootstrapRitualOptions{
		AgentName:        "HelperBot",
		AgentDescription: "a focused workspace copilot",
	})
	if err != nil {
		t.Fatalf("AdvanceBootstrapRitual(start): %v", err)
	}

	if !strings.Contains(result.Response, "Hello. I am HelperBot, a focused workspace copilot.") {
		t.Fatalf("expected configured intro, got %q", result.Response)
	}
	if strings.Contains(result.Response, "Hello. I am AnyClaw") {
		t.Fatalf("expected intro to avoid default agent name, got %q", result.Response)
	}
}
