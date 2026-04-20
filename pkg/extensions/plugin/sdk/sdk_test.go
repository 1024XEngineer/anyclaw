package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

type stubChannel struct{}

func (stubChannel) Name() string                        { return "demo" }
func (stubChannel) Start() error                        { return nil }
func (stubChannel) Stop() error                         { return nil }
func (stubChannel) Send(msg Message) error              { _ = msg; return nil }
func (stubChannel) OnMessage(handler func(msg Message)) { _ = handler }

type stubNode struct{}

func (stubNode) Name() string      { return "desktop-node" }
func (stubNode) Platform() string  { return "windows" }
func (stubNode) Connect() error    { return nil }
func (stubNode) Disconnect() error { return nil }
func (stubNode) Invoke(action string, input json.RawMessage) (json.RawMessage, error) {
	_, _ = action, input
	return json.RawMessage(`{"ok":true}`), nil
}
func (stubNode) Capabilities() []string { return []string{"desktop-control"} }

func TestNewPluginContextInitializesCollections(t *testing.T) {
	ctx := NewPluginContext("demo", "1.0.0", "workdir", "http://127.0.0.1:8080", nil)
	api := NewPluginAPI(ctx)

	if err := api.RegisterTool(Tool{Name: "ping", Description: "Ping", Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
		_, _ = ctx, input
		return json.RawMessage(`{"ok":true}`), nil
	}}); err != nil {
		t.Fatalf("RegisterTool: %v", err)
	}
	if err := api.RegisterChannel(stubChannel{}); err != nil {
		t.Fatalf("RegisterChannel: %v", err)
	}
	if err := api.RegisterHTTPRoute(HTTPRoute{Path: "/ping", Method: http.MethodGet, Handler: func(w http.ResponseWriter, r *http.Request) {
		_, _ = w, r
	}}); err != nil {
		t.Fatalf("RegisterHTTPRoute: %v", err)
	}
	if err := api.RegisterNode(stubNode{}); err != nil {
		t.Fatalf("RegisterNode: %v", err)
	}

	if len(api.ListTools()) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(api.ListTools()))
	}
	if len(api.ListChannels()) != 1 {
		t.Fatalf("expected 1 channel, got %d", len(api.ListChannels()))
	}
	if len(api.ListHTTPRoutes()) != 1 {
		t.Fatalf("expected 1 route, got %d", len(api.ListHTTPRoutes()))
	}
	if len(api.ListNodes()) != 1 {
		t.Fatalf("expected 1 node, got %d", len(api.ListNodes()))
	}
}

func TestPluginManifestValidate(t *testing.T) {
	valid := PluginManifest{
		Name:        "demo",
		Version:     "1.0.0",
		Description: "Demo plugin",
		Kind:        []string{"tool"},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid manifest, got %v", err)
	}

	invalid := PluginManifest{}
	if err := invalid.Validate(); err == nil {
		t.Fatal("expected invalid manifest to fail validation")
	}
}
