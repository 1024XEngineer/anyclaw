package main

import "testing"

func TestBuildPiSessionsPathEscapesIDs(t *testing.T) {
	got := buildPiSessionsPath("user/with?query&bits", "session/with?query&bits")
	want := "/v1/sessions/session%2Fwith%3Fquery&bits?user_id=user%2Fwith%3Fquery%26bits"
	if got != want {
		t.Fatalf("buildPiSessionsPath escaped path = %q, want %q", got, want)
	}
}

func TestBuildPiAgentsPathEscapesUserID(t *testing.T) {
	got := buildPiAgentsPath("user/with?query&bits")
	want := "/v1/agents/user%2Fwith%3Fquery&bits"
	if got != want {
		t.Fatalf("buildPiAgentsPath escaped path = %q, want %q", got, want)
	}
}
