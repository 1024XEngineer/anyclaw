package main

import "testing"

func TestNormalizeGatewayCommandSupportsStartAlias(t *testing.T) {
	if got := normalizeGatewayCommand("start"); got != "run" {
		t.Fatalf("expected start alias to normalize to run, got %q", got)
	}
	if got := normalizeGatewayCommand(" RUN "); got != "run" {
		t.Fatalf("expected run command to normalize to run, got %q", got)
	}
}
