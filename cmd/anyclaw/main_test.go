package main

import (
	"os"
	"strings"
	"testing"

	"github.com/anyclaw/anyclaw/pkg/consoleio"
)

func TestNormalizeRootCommandSupportsOpenClawAliases(t *testing.T) {
	tests := map[string]string{
		"skill":    "skill",
		"skills":   "skill",
		"plugin":   "plugin",
		"plugins":  "plugin",
		"agent":    "agent",
		"agents":   "agent",
		"clihub":   "clihub",
		"claw":     "claw",
		"app":      "app",
		"apps":     "app",
		"channel":  "channels",
		"session":  "sessions",
		"approval": "approvals",
		"model":    "models",
		"setup":    "onboard",
		"daemon":   "daemon",
		"cron":     "cron",
		"pi":       "pi",
	}

	for input, want := range tests {
		if got := normalizeRootCommand(input); got != want {
			t.Fatalf("normalizeRootCommand(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestReadInteractiveLineStableUsesRuntimeReader(t *testing.T) {
	originalStdin := os.Stdin
	stdinReader, stdinWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer func() {
		os.Stdin = originalStdin
		_ = stdinReader.Close()
	}()
	if _, err := stdinWriter.WriteString("from-stdin\n"); err != nil {
		t.Fatalf("stdinWriter.WriteString: %v", err)
	}
	if err := stdinWriter.Close(); err != nil {
		t.Fatalf("stdinWriter.Close: %v", err)
	}
	os.Stdin = stdinReader

	state := &RuntimeState{
		reader: consoleio.NewReader(strings.NewReader("from-state-reader\n")),
	}

	var line string
	output := captureStdout(t, func() {
		line, err = readInteractiveLineStable(state)
		if err != nil {
			t.Fatalf("readInteractiveLineStable: %v", err)
		}
	})

	if line != "from-state-reader" {
		t.Fatalf("expected input from runtime reader, got %q", line)
	}
	if output != "you > " {
		t.Fatalf("expected a single prompt marker, got %q", output)
	}
}

func TestRenderInteractiveOutputHonorsMarkdownMode(t *testing.T) {
	state := &RuntimeState{}

	rendered := renderInteractiveOutput(state, "# Title")
	if strings.Contains(rendered, "# Title") {
		t.Fatalf("expected markdown mode to transform heading markers, got %q", rendered)
	}
	if !strings.Contains(rendered, "Title") {
		t.Fatalf("expected heading content to remain, got %q", rendered)
	}

	state.rawOutput = true
	if got := renderInteractiveOutput(state, "# Title"); got != "# Title" {
		t.Fatalf("expected raw mode to preserve content, got %q", got)
	}
}

func TestHandleMarkdownCommandTogglesOutputMode(t *testing.T) {
	state := &RuntimeState{}

	output := captureStdout(t, func() {
		handleMarkdownCommand(state, "/markdown off")
	})
	if !state.rawOutput {
		t.Fatal("expected raw output mode to be enabled")
	}
	if !strings.Contains(output, "Markdown rendering disabled") {
		t.Fatalf("expected disable confirmation, got %q", output)
	}

	output = captureStdout(t, func() {
		handleMarkdownCommand(state, "/markdown on")
	})
	if state.rawOutput {
		t.Fatal("expected markdown rendering to be re-enabled")
	}
	if !strings.Contains(output, "Markdown rendering enabled") {
		t.Fatalf("expected enable confirmation, got %q", output)
	}
}
