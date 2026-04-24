package gateway

import (
	"os"
	"path/filepath"

	"github.com/1024XEngineer/anyclaw/pkg/extensions/plugin"
)

func (s *Server) initMarketStore() {
	pluginDir := s.mainRuntime.Config.Plugins.Dir
	if pluginDir == "" {
		pluginDir = "plugins"
	}
	marketDir := filepath.Join(pluginDir, ".market")
	cacheDir := filepath.Join(pluginDir, ".cache")

	_ = os.MkdirAll(marketDir, 0o755)
	_ = os.MkdirAll(cacheDir, 0o755)

	sources := []plugin.PluginSource{
		{Name: "default", URL: "https://market.anyclaw.github.io", Type: "http"},
	}

	trustStore := plugin.NewTrustStore()
	s.marketStore = plugin.NewStore(pluginDir, marketDir, cacheDir, sources, trustStore, s.plugins)
}
