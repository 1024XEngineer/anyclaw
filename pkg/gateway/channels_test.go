package gateway

import (
	"testing"

	"github.com/anyclaw/anyclaw/pkg/config"
	"github.com/anyclaw/anyclaw/pkg/plugin"
	appRuntime "github.com/anyclaw/anyclaw/pkg/runtime"
)

func TestInitChannelsRegistersAllBuiltinChannelAdapters(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Plugins.Dir = ""

	plugins, err := plugin.NewRegistry(cfg.Plugins)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	server := &Server{
		app: &appRuntime.App{
			Config:  cfg,
			Plugins: plugins,
		},
	}

	server.initChannels()

	if server.telegram == nil {
		t.Fatal("expected telegram adapter to be initialized")
	}
	if server.slack == nil {
		t.Fatal("expected slack adapter to be initialized")
	}
	if server.discord == nil {
		t.Fatal("expected discord adapter to be initialized")
	}
	if server.whatsapp == nil {
		t.Fatal("expected whatsapp adapter to be initialized")
	}
	if server.signal == nil {
		t.Fatal("expected signal adapter to be initialized")
	}

	if got := len(server.channels.Statuses()); got != 5 {
		t.Fatalf("expected 5 channel adapters, got %d", got)
	}
}
