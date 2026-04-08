package gateway

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestWSClientReconnectsAfterServerDropsConnection(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	var connectionCount atomic.Int32

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("Upgrade: %v", err)
			return
		}
		go func(index int32) {
			defer conn.Close()
			serveTestGatewayConn(t, conn, func(frame openClawWSFrame) (openClawWSFrame, bool) {
				switch frame.Method {
				case "ping":
					return openClawWSFrame{
						Type: "res",
						ID:   frame.ID,
						OK:   true,
						Data: map[string]any{"pong": "ok"},
					}, false
				case "chat.send":
					reply := openClawWSFrame{
						Type: "res",
						ID:   frame.ID,
						OK:   true,
						Data: map[string]any{"response": fmt.Sprintf("conn-%d:%s", index, mapString(frame.Params, "message"))},
					}
					return reply, index == 1
				default:
					return openClawWSFrame{
						Type: "res",
						ID:   frame.ID,
						OK:   true,
						Data: map[string]any{},
					}, false
				}
			})
		}(connectionCount.Add(1))
	}))
	defer ts.Close()

	client := NewWSClient(strings.Replace(ts.URL, "http://", "ws://", 1), "")
	client.keepAliveInterval = 0
	defer client.Close()

	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	first, err := client.SendMessage(context.Background(), "first")
	if err != nil {
		t.Fatalf("SendMessage(first): %v", err)
	}
	if first != "conn-1:first" {
		t.Fatalf("unexpected first response: %q", first)
	}

	waitFor(t, time.Second, func() bool { return !client.Connected() })

	second, err := client.SendMessage(context.Background(), "second")
	if err != nil {
		t.Fatalf("SendMessage(second): %v", err)
	}
	if second != "conn-2:second" {
		t.Fatalf("unexpected second response after reconnect: %q", second)
	}
}

func TestWSClientKeepsIdleConnectionAlive(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	var pingCount atomic.Int32

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("Upgrade: %v", err)
			return
		}
		go func() {
			defer conn.Close()
			serveTestGatewayConnWithDeadline(t, conn, 80*time.Millisecond, func(frame openClawWSFrame) openClawWSFrame {
				switch frame.Method {
				case "ping":
					pingCount.Add(1)
					return openClawWSFrame{
						Type: "res",
						ID:   frame.ID,
						OK:   true,
						Data: map[string]any{"pong": "ok"},
					}
				case "chat.send":
					return openClawWSFrame{
						Type: "res",
						ID:   frame.ID,
						OK:   true,
						Data: map[string]any{"response": "still-alive"},
					}
				default:
					return openClawWSFrame{
						Type: "res",
						ID:   frame.ID,
						OK:   true,
						Data: map[string]any{},
					}
				}
			})
		}()
	}))
	defer ts.Close()

	client := NewWSClient(strings.Replace(ts.URL, "http://", "ws://", 1), "")
	client.keepAliveInterval = 20 * time.Millisecond
	defer client.Close()

	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	time.Sleep(180 * time.Millisecond)

	resp, err := client.SendMessage(context.Background(), "after-idle")
	if err != nil {
		t.Fatalf("SendMessage(after-idle): %v", err)
	}
	if resp != "still-alive" {
		t.Fatalf("unexpected response after idle: %q", resp)
	}
	if pingCount.Load() == 0 {
		t.Fatal("expected keepalive pings during idle period")
	}
}

func serveTestGatewayConn(t *testing.T, conn *websocket.Conn, handler func(frame openClawWSFrame) (openClawWSFrame, bool)) {
	t.Helper()
	if err := conn.WriteJSON(openClawWSFrame{
		Type:  "event",
		Event: "connect.challenge",
		Data:  map[string]any{"nonce": "nonce"},
	}); err != nil {
		t.Errorf("WriteJSON(challenge): %v", err)
		return
	}

	var connectFrame openClawWSFrame
	if err := conn.ReadJSON(&connectFrame); err != nil {
		t.Errorf("ReadJSON(connect): %v", err)
		return
	}
	if err := conn.WriteJSON(openClawWSFrame{
		Type: "res",
		ID:   connectFrame.ID,
		OK:   true,
		Data: map[string]any{"status": "connected"},
	}); err != nil {
		t.Errorf("WriteJSON(connect response): %v", err)
		return
	}

	for {
		var frame openClawWSFrame
		if err := conn.ReadJSON(&frame); err != nil {
			return
		}
		resp, closeAfter := handler(frame)
		if err := conn.WriteJSON(resp); err != nil {
			return
		}
		if closeAfter {
			return
		}
	}
}

func serveTestGatewayConnWithDeadline(t *testing.T, conn *websocket.Conn, deadline time.Duration, handler func(frame openClawWSFrame) openClawWSFrame) {
	t.Helper()
	if err := conn.WriteJSON(openClawWSFrame{
		Type:  "event",
		Event: "connect.challenge",
		Data:  map[string]any{"nonce": "nonce"},
	}); err != nil {
		t.Errorf("WriteJSON(challenge): %v", err)
		return
	}

	conn.SetReadDeadline(time.Now().Add(deadline))
	var connectFrame openClawWSFrame
	if err := conn.ReadJSON(&connectFrame); err != nil {
		t.Errorf("ReadJSON(connect): %v", err)
		return
	}
	if err := conn.WriteJSON(openClawWSFrame{
		Type: "res",
		ID:   connectFrame.ID,
		OK:   true,
		Data: map[string]any{"status": "connected"},
	}); err != nil {
		t.Errorf("WriteJSON(connect response): %v", err)
		return
	}

	for {
		conn.SetReadDeadline(time.Now().Add(deadline))
		var frame openClawWSFrame
		if err := conn.ReadJSON(&frame); err != nil {
			return
		}
		if err := conn.WriteJSON(handler(frame)); err != nil {
			return
		}
	}
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}
