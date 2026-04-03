package gateway

import appstore "github.com/anyclaw/anyclaw/pkg/apps"

func newAppStore(configPath string) (*appstore.Store, error) {
	return appstore.NewStore(configPath)
}
