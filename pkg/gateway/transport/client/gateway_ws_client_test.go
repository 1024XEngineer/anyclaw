package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func newMockWSGateway(t *testing.T) string {
	t.Helper()
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		_ = conn.WriteJSON(openClawWSFrame{Type: "event", Event: "connect.challenge", Data: map[string]any{"nonce": "challenge-token"}})
		var connectReq openClawWSFrame
		if err := conn.ReadJSON(&connectReq); err != nil {
			return
		}
		_ = conn.WriteJSON(openClawWSFrame{Type: "res", ID: connectReq.ID, OK: true, Data: map[string]any{"connected": true}})

		for {
			var frame openClawWSFrame
			if err := conn.ReadJSON(&frame); err != nil {
				return
			}
			resp := openClawWSFrame{Type: "res", ID: frame.ID, OK: true, Data: map[string]any{}}
			switch frame.Method {
			case "chat.send":
				resp.Data = map[string]any{"response": "chat-response"}
			case "status.get":
				resp.Data = map[string]any{"ok": true, "status": "ok", "version": "1", "provider": "openai", "model": "gpt", "address": "127.0.0.1", "working_dir": ".", "work_dir": "."}
			case "sessions.list":
				resp.Data = map[string]any{"sessions": []map[string]any{{"id": "s1", "title": "Session 1"}}}
			case "events.subscribe":
				resp.Data = map[string]any{"subscribed": true}
				_ = conn.WriteJSON(openClawWSFrame{Type: "event", Event: "runtime.updated", Data: map[string]any{"value": "ok"}})
			case "tools.invoke":
				resp.Data = map[string]any{"tool": "done"}
			case "sessions.send":
				resp.Data = map[string]any{"response": "session-response"}
			case "config.get":
				resp.Data = map[string]any{"theme": "light"}
			case "config.set", "chat.abort", "ping", "device.pairing.unpair":
				resp.Data = map[string]any{"ok": true}
			case "agents.list":
				resp.Data = map[string]any{"agents": []any{"alpha", map[string]any{"name": "beta"}}}
			case "channels.list":
				resp.Data = map[string]any{"channels": []any{"general", map[string]any{"name": "random"}}}
			case "tools.list":
				resp.Data = map[string]any{"tools": []any{"grep", map[string]any{"name": "read"}}}
			case "chat.history":
				resp.Data = map[string]any{"history": []any{map[string]any{"role": "assistant"}}}
			case "methods.list":
				resp.Data = map[string]any{"methods": []any{"ping", "status.get"}}
			case "device.pairing.generate":
				resp.Data = map[string]any{"code": "abcd1234", "expires": "soon", "device": "cli", "type": "desktop"}
			case "device.pairing.validate":
				resp.Data = map[string]any{"valid": true}
			case "device.pairing.pair", "device.pairing.status", "device.pairing.renew":
				resp.Data = map[string]any{"device_id": "dev-1", "status": "paired"}
			case "device.pairing.list":
				resp.Data = map[string]any{"devices": []any{map[string]any{"device_id": "dev-1"}}}
			default:
				resp.OK = false
				resp.Error = "unknown method"
			}
			_ = conn.WriteJSON(resp)
		}
	}))
	t.Cleanup(server.Close)
	return "ws" + strings.TrimPrefix(server.URL, "http")
}

func TestWSClientEndToEnd(t *testing.T) {
	client := NewWSClient(newMockWSGateway(t), "token")
	client.keepAliveInterval = 0

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer client.Close()
	if !client.Connected() {
		t.Fatal("expected client to be connected")
	}

	var eventMu sync.Mutex
	var events []string
	if err := client.SubscribeEvents(ctx, "runtime.updated", func(event *Event) {
		eventMu.Lock()
		defer eventMu.Unlock()
		events = append(events, event.Type)
	}); err != nil {
		t.Fatalf("SubscribeEvents: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	if resp, err := client.SendMessage(ctx, "hello"); err != nil || resp != "chat-response" {
		t.Fatalf("SendMessage = %q, %v", resp, err)
	}
	if _, err := client.GetStatus(ctx); err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if sessions, err := client.ListSessions(ctx); err != nil || len(sessions) != 1 {
		t.Fatalf("ListSessions = %v, %v", sessions, err)
	}
	if out, err := client.InvokeTool(ctx, "grep", map[string]any{"q": "x"}); err != nil || out == nil {
		t.Fatalf("InvokeTool = %v, %v", out, err)
	}
	if resp, err := client.SendChatMessage(ctx, "s1", "hi"); err != nil || resp != "session-response" {
		t.Fatalf("SendChatMessage = %q, %v", resp, err)
	}
	if cfg, err := client.GetConfig(ctx); err != nil || cfg["theme"] != "light" {
		t.Fatalf("GetConfig = %v, %v", cfg, err)
	}
	if err := client.SetConfig(ctx, "theme", "dark"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if agents, err := client.ListAgents(ctx); err != nil || len(agents) != 2 {
		t.Fatalf("ListAgents = %v, %v", agents, err)
	}
	if channels, err := client.ListChannels(ctx); err != nil || len(channels) != 2 {
		t.Fatalf("ListChannels = %v, %v", channels, err)
	}
	if tools, err := client.ListTools(ctx); err != nil || len(tools) != 2 {
		t.Fatalf("ListTools = %v, %v", tools, err)
	}
	if err := client.AbortChat(ctx); err != nil {
		t.Fatalf("AbortChat: %v", err)
	}
	if history, err := client.GetChatHistory(ctx, "s1"); err != nil || len(history) != 1 {
		t.Fatalf("GetChatHistory = %v, %v", history, err)
	}
	if err := client.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if methods, err := client.ListMethods(ctx); err != nil || len(methods) != 2 {
		t.Fatalf("ListMethods = %v, %v", methods, err)
	}

	if pairing, err := client.GeneratePairingCode(ctx, "cli", "desktop"); err != nil || pairing.Code == "" {
		t.Fatalf("GeneratePairingCode = %v, %v", pairing, err)
	}
	if valid, err := client.ValidatePairingCode(ctx, "abcd1234"); err != nil || !valid {
		t.Fatalf("ValidatePairingCode = %v, %v", valid, err)
	}
	if result, err := client.CompletePairing(ctx, "abcd1234", "dev-1", "cli"); err != nil || result["status"] != "paired" {
		t.Fatalf("CompletePairing = %v, %v", result, err)
	}
	if devices, err := client.ListPairedDevices(ctx); err != nil || len(devices) != 1 {
		t.Fatalf("ListPairedDevices = %v, %v", devices, err)
	}
	if status, err := client.GetPairingStatus(ctx); err != nil || status["status"] != "paired" {
		t.Fatalf("GetPairingStatus = %v, %v", status, err)
	}
	if renewed, err := client.RenewPairing(ctx, "dev-1"); err != nil || renewed["status"] != "paired" {
		t.Fatalf("RenewPairing = %v, %v", renewed, err)
	}
	if err := client.UnpairDevice(ctx, "dev-1"); err != nil {
		t.Fatalf("UnpairDevice: %v", err)
	}

	eventMu.Lock()
	defer eventMu.Unlock()
	if len(events) == 0 || events[0] != "runtime.updated" {
		t.Fatalf("expected subscription event, got %v", events)
	}
}

func TestGatewayClientReadLoopCallbacks(t *testing.T) {
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		frames := []map[string]any{
			{"type": "message", "data": map[string]any{"content": "hello"}},
			{"type": "tool_call", "data": map[string]any{"name": "grep"}},
			{"type": "presence", "data": map[string]any{"status": "online"}},
			{"type": "typing", "data": map[string]any{"typing": true}},
		}
		for _, frame := range frames {
			data, _ := json.Marshal(frame)
			_ = conn.WriteMessage(websocket.TextMessage, data)
		}
		time.Sleep(50 * time.Millisecond)
	}))
	defer server.Close()

	addr := strings.TrimPrefix(server.URL, "http://")
	client, err := NewGateway(WithGatewayAddr(addr))
	if err != nil {
		t.Fatalf("NewGateway: %v", err)
	}

	client.OnMessage(func(IncomingMessage) {})
	client.OnToolCall(func(ToolCall) {})
	client.OnPresence(func(Presence) {})
	client.OnTyping(func(TypingIndicator) {})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer client.Disconnect()
	time.Sleep(100 * time.Millisecond)
}
