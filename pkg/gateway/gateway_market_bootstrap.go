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

	trustStore := plugin.NewTrustStore()
	s.marketStore = plugin.NewStore(pluginDir, marketDir, cacheDir, marketSources(s.mainRuntime.WorkDir), trustStore, s.plugins)
}

func marketSources(workDir string) []plugin.PluginSource {
	sources, _ := loadConfiguredMarketSources(workDir)
	return sources
}

func loadConfiguredMarketSources(workDir string) ([]plugin.PluginSource, error) {
	configured, err := plugin.LoadSources(plugin.SourcesPath(workDir))
	if err != nil {
		return plugin.DefaultSources(), err
	}
	return plugin.MergeSources(plugin.DefaultSources(), configured), nil
}
