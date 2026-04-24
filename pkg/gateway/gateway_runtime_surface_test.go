package gateway

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
)

func TestGatewayRuntimeSurface_WorkerAndDeviceHelpers(t *testing.T) {
	server := newSplitAPITestServer(t)

	var buf bytes.Buffer
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	printWorkerStatus([]int{11}, []int{18789}, 18789)
	_ = w.Close()
	os.Stdout = oldStdout
	_, _ = buf.ReadFrom(r)
	if !strings.Contains(buf.String(), "Gateway workers started") {
		t.Fatalf("unexpected worker status output: %q", buf.String())
	}

	if err := killProcess(-1); err == nil {
		t.Fatal("expected killProcess to fail for invalid pid")
	}

	server.mainRuntime.Config.Speech.TTS.Enabled = false
	server.initTTS()
	server.registerAudioSenders()
	if server.ttsManager != nil || server.ttsPipeline != nil {
		t.Fatal("expected TTS to remain uninitialized when disabled")
	}

	conn := &openClawWSConn{server: server, closed: make(chan struct{})}
	handled, err := conn.handleDeviceWSRequest(context.Background(), openClawWSFrame{ID: "1"}, "unknown.method")
	if err != nil || handled {
		t.Fatalf("unexpected device ws result: handled=%v err=%v", handled, err)
	}
}
