package app

import (
	"context"

	"anyclaw/internal/agent"
	"anyclaw/internal/channels"
	"anyclaw/internal/channels/telegram"
	"anyclaw/internal/channels/webchat"
	"anyclaw/internal/config"
	"anyclaw/internal/gateway"
	"anyclaw/internal/hooks"
	"anyclaw/internal/pluginruntime"
	"anyclaw/internal/providers"
	"anyclaw/internal/providers/mock"
	"anyclaw/internal/providers/openai"
	"anyclaw/internal/routing"
	"anyclaw/internal/session"
	"anyclaw/internal/tools"
	memorytool "anyclaw/internal/tools/memory"
	shelltool "anyclaw/internal/tools/shell"
	webfetchtool "anyclaw/internal/tools/webfetch"
)

// New builds the initial application skeleton and registers starter built-ins.
func New(ctx context.Context, configPath string) (*App, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}
	if err := config.Validate(cfg); err != nil {
		return nil, err
	}

	providerRegistry := providers.NewRegistry()
	toolRegistry := tools.NewRegistry()
	channelRegistry := channels.NewRegistry()
	hookRegistry := hooks.NewRegistry()
	pluginRegistry := pluginruntime.NewRegistry()
	sessionStore := session.NewMemoryStore()

	app := &App{
		Config:    cfg,
		Gateway:   gateway.NewService(),
		Router:    routing.StaticRouter{DefaultAgentID: cfg.Agents.DefaultAgentID},
		Sessions:  sessionStore,
		Providers: providerRegistry,
		Tools:     toolRegistry,
		Channels:  channelRegistry,
		Hooks:     hookRegistry,
		Plugins:   pluginRegistry,
	}

	app.RegisterProvider(openai.New())
	app.RegisterProvider(mock.New())
	app.RegisterTool(shelltool.Tool{})
	app.RegisterTool(webfetchtool.Tool{})
	app.RegisterTool(memorytool.Tool{})
	app.RegisterChannel(webchat.New())
	app.RegisterChannel(telegram.New())

	app.Agent = agent.NewRunner(app.Sessions, app.Providers, app.Tools, app.Hooks)
	if err := app.Plugins.Boot(ctx, app); err != nil {
		return nil, err
	}
	return app, nil
}
