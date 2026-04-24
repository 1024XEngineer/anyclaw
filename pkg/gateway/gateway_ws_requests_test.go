package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gatewayauth "github.com/1024XEngineer/anyclaw/pkg/gateway/auth"
	gatewaysurface "github.com/1024XEngineer/anyclaw/pkg/gateway/surface"
	"github.com/gorilla/websocket"
)

func newWSTestConn(t *testing.T, server *Server) (*openClawWSConn, *websocket.Conn) {
	t.Helper()
	upgrader := websocket.Upgrader{}
	connCh := make(chan *websocket.Conn, 1)
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		connCh <- conn
	}))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	t.Cleanup(func() { _ = clientConn.Close() })

	serverConn := <-connCh
	t.Cleanup(func() { _ = serverConn.Close() })

	return &openClawWSConn{
		server:    server,
		conn:      serverConn,
		user:      &gatewayauth.User{Name: "tester", Role: "admin", Permissions: []string{"*"}},
		closed:    make(chan struct{}),
		challenge: "challenge-token",
		transport: gatewaysurface.TransportRef{Protocol: "ws", Path: "/ws", ClientID: "client-1"},
	}, clientConn
}

func readWSFrame(t *testing.T, conn *websocket.Conn) openClawWSFrame {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var frame openClawWSFrame
	if err := conn.ReadJSON(&frame); err != nil {
		t.Fatalf("read ws frame: %v", err)
	}
	return frame
}

func TestOpenClawWSCoreMutationAndDeviceRequests(t *testing.T) {
	server := newSplitAPITestServer(t)
	server.devicePairing.SetEnabled(true)
	providerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer providerSrv.Close()
	server.mainRuntime.Config.Providers[0].BaseURL = providerSrv.URL
	server.mainRuntime.Config.Providers[0].APIKey = "secret"

	conn, clientConn := newWSTestConn(t, server)
	ctx := gatewayauth.WithUser(context.Background(), conn.user)

	handled, err := conn.handleCoreWSRequest(ctx, openClawWSFrame{ID: "connect-1", Params: map[string]any{"challenge": "challenge-token"}}, "connect")
	if err != nil || !handled {
		t.Fatalf("connect handled=%v err=%v", handled, err)
	}
	if frame := readWSFrame(t, clientConn); !frame.OK {
		t.Fatalf("connect response not ok: %+v", frame)
	}

	for _, tc := range []struct {
		id     string
		method string
		params map[string]any
	}{
		{id: "ping-1", method: "ping"},
		{id: "methods-1", method: "methods.list"},
		{id: "status-1", method: "status.get"},
		{id: "events-1", method: "events.list", params: map[string]any{"limit": 2}},
		{id: "sub-1", method: "events.subscribe"},
		{id: "unsub-1", method: "events.unsubscribe"},
	} {
		handled, err = conn.handleCoreWSRequest(ctx, openClawWSFrame{ID: tc.id, Params: tc.params}, tc.method)
		if err != nil || !handled {
			t.Fatalf("core method %s handled=%v err=%v", tc.method, handled, err)
		}
		if frame := readWSFrame(t, clientConn); !frame.OK {
			t.Fatalf("core method %s response not ok: %+v", tc.method, frame)
		}
	}

	if err := (&openClawWSConn{}).requireConfigRead(); err == nil {
		t.Fatal("expected requireConfigRead to fail without user")
	}
	if ok, err := conn.handleCoreWSRequest(ctx, openClawWSFrame{}, "unknown"); err != nil || ok {
		t.Fatalf("unexpected unknown core method result ok=%v err=%v", ok, err)
	}

	for _, tc := range []struct {
		id     string
		method string
		params map[string]any
	}{
		{id: "prov-default", method: "providers.default", params: map[string]any{"provider_ref": "provider-1"}},
		{id: "prov-test", method: "providers.test", params: map[string]any{"provider": map[string]any{"id": "provider-1", "provider": "compatible", "base_url": providerSrv.URL, "api_key": "secret", "enabled": true}}},
		{id: "binding-update", method: "agent-bindings.update", params: map[string]any{"binding": map[string]any{"agent": "helper", "provider_ref": "provider-1", "model": "gpt-test"}}},
	} {
		handled, err = conn.handleMutationWSRequest(ctx, openClawWSFrame{ID: tc.id, Params: tc.params}, tc.method)
		if err != nil || !handled {
			t.Fatalf("mutation method %s handled=%v err=%v", tc.method, handled, err)
		}
		if frame := readWSFrame(t, clientConn); !frame.OK {
			t.Fatalf("mutation method %s response not ok: %+v", tc.method, frame)
		}
	}

	if _, err := marshalWSParam(nil, "provider", "required", "invalid"); err == nil {
		t.Fatal("expected marshalWSParam to reject nil params")
	}
	if ok, err := conn.handleMutationWSRequest(ctx, openClawWSFrame{}, "unknown.mutation"); err != nil || ok {
		t.Fatalf("unexpected unknown mutation result ok=%v err=%v", ok, err)
	}

	handled, err = conn.handleDeviceWSRequest(ctx, openClawWSFrame{ID: "gen-1", Params: map[string]any{"device_name": "cli", "device_type": "desktop"}}, "device.pairing.generate")
	if err != nil || !handled {
		t.Fatalf("generate pairing handled=%v err=%v", handled, err)
	}
	frame := readWSFrame(t, clientConn)
	if !frame.OK {
		t.Fatalf("pairing generate response not ok: %+v", frame)
	}
	data, _ := frame.Data.(map[string]any)
	code, _ := data["code"].(string)

	for _, tc := range []struct {
		id     string
		method string
		params map[string]any
	}{
		{id: "validate-1", method: "device.pairing.validate", params: map[string]any{"code": code}},
		{id: "pair-1", method: "device.pairing.pair", params: map[string]any{"code": code, "device_id": "dev-1", "device_name": "cli"}},
		{id: "list-1", method: "device.pairing.list"},
		{id: "status-2", method: "device.pairing.status"},
		{id: "renew-1", method: "device.pairing.renew", params: map[string]any{"device_id": "dev-1"}},
		{id: "unpair-1", method: "device.pairing.unpair", params: map[string]any{"device_id": "dev-1"}},
	} {
		handled, err = conn.handleDeviceWSRequest(ctx, openClawWSFrame{ID: tc.id, Params: tc.params}, tc.method)
		if err != nil || !handled {
			t.Fatalf("device method %s handled=%v err=%v", tc.method, handled, err)
		}
		if frame := readWSFrame(t, clientConn); !frame.OK {
			t.Fatalf("device method %s response not ok: %+v", tc.method, frame)
		}
	}

	if ok, err := conn.handleDeviceWSRequest(ctx, openClawWSFrame{}, "unknown.device"); err != nil || ok {
		t.Fatalf("unexpected unknown device result ok=%v err=%v", ok, err)
	}
}
