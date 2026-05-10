package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestRunDefaultsToServeAndRequiresAdminToken(t *testing.T) {
	t.Setenv("ANYCLAW_REGISTRY_ADMIN_TOKEN", "")

	err := run([]string{"serve", "--data-dir", t.TempDir(), "--seed=false", "--require-admin-token=true"})
	if err == nil || !strings.Contains(err.Error(), "admin token is required") {
		t.Fatalf("expected missing admin token error, got %v", err)
	}
}

func TestRunHelpAndUnknownCommand(t *testing.T) {
	out := captureStdout(t, func() {
		if err := run([]string{"help"}); err != nil {
			t.Fatalf("help returned error: %v", err)
		}
	})
	if !strings.Contains(out, "anyclaw-registry serve") {
		t.Fatalf("help output = %q", out)
	}
	if err := run([]string{"nope"}); err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("expected unknown command error, got %v", err)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	original := os.Stdout
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = write
	defer func() { os.Stdout = original }()

	fn()
	if err := write.Close(); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, read); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}
