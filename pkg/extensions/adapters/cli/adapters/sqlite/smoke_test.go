package sqlite

import (
	"context"
	"runtime"
	"strings"
	"testing"
)

func TestRunExecutesCommand(t *testing.T) {
	client := NewClient(Config{})
	var args []string
	if runtime.GOOS == "windows" {
		client.sqlitePath = "cmd"
		args = []string{"/c", "echo ok"}
	} else {
		client.sqlitePath = "sh"
		args = []string{"-c", "printf ok"}
	}

	out, err := client.Run(context.Background(), args)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.TrimSpace(out) != "ok" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestRunReturnsUsageErrorWhenArgsMissing(t *testing.T) {
	client := NewClient(Config{})
	_, err := client.Run(context.Background(), []string{"only-db"})
	if err == nil {
		t.Fatal("expected usage error")
	}
	if !strings.Contains(err.Error(), "usage: sqlite") {
		t.Fatalf("unexpected error: %v", err)
	}
}
