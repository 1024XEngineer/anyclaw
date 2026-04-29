package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"testing"
	"time"
)

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

func TestRunPiSessionsUsesBoundedRequestContext(t *testing.T) {
	oldTimeout := piRequestTimeout
	piRequestTimeout = 20 * time.Millisecond
	t.Cleanup(func() {
		piRequestTimeout = oldTimeout
	})

	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() {
		releaseOnce.Do(func() {
			close(release)
		})
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return
		case <-release:
			_, _ = w.Write([]byte(`{"success":true,"data":[]}`))
		}
	}))
	defer server.Close()

	host, port := splitTestServerHostPort(t, server.URL)
	done := make(chan error, 1)
	go func() {
		done <- runPiSessions([]string{"--host", host, "--port", strconv.Itoa(port)})
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected stalled pi sessions request to fail on timeout")
		}
	case <-time.After(300 * time.Millisecond):
		releaseOnce.Do(func() {
			close(release)
		})
		<-done
		t.Fatal("expected pi sessions request to use a bounded context")
	}
}

func splitTestServerHostPort(t *testing.T, rawURL string) (string, int) {
	t.Helper()

	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	host, portString, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatalf("split test server host/port: %v", err)
	}
	port, err := strconv.Atoi(portString)
	if err != nil {
		t.Fatalf("parse test server port: %v", err)
	}
	return host, port
}
