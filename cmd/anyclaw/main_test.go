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
	if output != "> " {
		t.Fatalf("expected a single prompt marker, got %q", output)
	}
}
