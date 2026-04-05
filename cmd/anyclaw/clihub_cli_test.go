package main

import (
	"reflect"
	"testing"
)

func TestSplitCLIHubExecArgs(t *testing.T) {
	flagArgs, passthrough := splitCLIHubExecArgs([]string{"shotcut", "--json=false", "--", "project", "info", "--help"})
	if !reflect.DeepEqual(flagArgs, []string{"shotcut", "--json=false"}) {
		t.Fatalf("unexpected flag args: %#v", flagArgs)
	}
	if !reflect.DeepEqual(passthrough, []string{"project", "info", "--help"}) {
		t.Fatalf("unexpected passthrough args: %#v", passthrough)
	}
}

func TestReorderFlagArgsKeepsPositionalsForCLIHubExec(t *testing.T) {
	got := reorderFlagArgs([]string{"shotcut", "--cwd", "D:\\tmp", "project", "info"}, map[string]bool{
		"--cwd": true,
	})
	want := []string{"--cwd", "D:\\tmp", "shotcut", "project", "info"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("reorderFlagArgs = %#v, want %#v", got, want)
	}
}
