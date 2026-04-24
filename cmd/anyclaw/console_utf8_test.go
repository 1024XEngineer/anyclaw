package main

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestConfigureConsoleUTF8(t *testing.T) {
	configureConsoleUTF8()
}

func TestPrintConsoleUTF8WarningUsesStderr(t *testing.T) {
	originalWriter := consoleUTF8WarningWriter
	t.Cleanup(func() {
		consoleUTF8WarningWriter = originalWriter
	})

	stdout, stderr, err := captureCLIOutput(t, func() error {
		consoleUTF8WarningWriter = os.Stderr
		printConsoleUTF8Warning("SetConsoleCP failed: %v", errors.New("boom"))
		return nil
	})
	if err != nil {
		t.Fatalf("printConsoleUTF8Warning: %v", err)
	}
	if strings.Contains(stdout, "SetConsoleCP failed") {
		t.Fatalf("expected warning to stay out of stdout, got %q", stdout)
	}
	if !strings.Contains(stderr, "Warning: SetConsoleCP failed: boom") {
		t.Fatalf("expected warning on stderr, got %q", stderr)
	}
}
