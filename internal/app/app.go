package app

import (
	"context"

	"anyclaw/internal/agent"
	"anyclaw/internal/channels"
	"anyclaw/internal/config"
	"anyclaw/internal/gateway"
	"anyclaw/internal/hooks"
	"anyclaw/internal/pluginruntime"
	"anyclaw/internal/providers"
	"anyclaw/internal/routing"
	"anyclaw/internal/session"
	"anyclaw/internal/tools"
	"anyclaw/pkg/sdk"
)

// App is the composed application root.
type App struct {
	Config    config.Config
	Gateway   *gateway.Service
	Router    routing.Router
	Sessions  session.SessionStore
	Providers *providers.Registry
	Tools     *tools.Registry
	Channels  *channels.Registry
	Hooks     *hooks.Registry
	Plugins   *pluginruntime.Registry
	Agent     *agent.Runner
}

// Start starts the application shell.
func (a *App) Start(ctx context.Context) error {
	for _, channel := range a.Channels.List() {
		if err := channel.Start(ctx); err != nil {
			return err
		}
	}
	return a.Gateway.Start(ctx)
}

// Shutdown stops the application shell.
func (a *App) Shutdown(ctx context.Context) error {
	for _, channel := range a.Channels.List() {
		_ = channel.Stop(ctx)
	}
	return a.Gateway.Shutdown(ctx)
}

// RegisterProvider adds a provider through the extension surface.
func (a *App) RegisterProvider(provider sdk.Provider) {
	a.Providers.Register(provider)
}

// RegisterChannel adds a channel through the extension surface.
func (a *App) RegisterChannel(channel sdk.Channel) {
	a.Channels.Register(channel)
}

// RegisterTool adds a tool through the extension surface.
func (a *App) RegisterTool(tool sdk.Tool) {
	a.Tools.Register(tool)
}

// RegisterHook adds a hook through the extension surface.
func (a *App) RegisterHook(hook sdk.Hook) {
	a.Hooks.Register(hook)
}
