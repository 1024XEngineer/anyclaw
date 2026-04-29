package gateway

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/1024XEngineer/anyclaw/pkg/extensions/plugin"
)

func (s *Server) initMarketStore() error {
	pluginDir := s.mainRuntime.Config.Plugins.Dir
	if pluginDir == "" {
		pluginDir = "plugins"
	}
	marketDir := filepath.Join(pluginDir, ".market")
	cacheDir := filepath.Join(pluginDir, ".cache")

	_ = os.MkdirAll(marketDir, 0o755)
	_ = os.MkdirAll(cacheDir, 0o755)

	sources, err := marketSources(s.mainRuntime.WorkDir)
	if err != nil {
		return fmt.Errorf("load market sources: %w", err)
	}

	trustStore := plugin.NewTrustStore()
	s.marketStore = plugin.NewStore(pluginDir, marketDir, cacheDir, sources, trustStore, s.plugins)
	return nil
}

func marketSources(workDir string) ([]plugin.PluginSource, error) {
	configured, err := plugin.LoadSources(plugin.SourcesPath(workDir))
	if err != nil {
		return nil, err
	}
	return plugin.MergeSources(plugin.DefaultSources(), configured), nil
}
