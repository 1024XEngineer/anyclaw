package config

// Default returns a baseline configuration suitable for local development.
func Default() Config {
	return Config{
		AppName: "anyclaw",
		Gateway: GatewayConfig{
			Bind:            "127.0.0.1",
			Port:            18789,
			AuthToken:       "change-me",
			ProtocolVersion: 1,
		},
		Agents: AgentsConfig{
			DefaultAgentID:  "default-agent",
			DefaultProvider: "openai",
			DefaultModel:    "gpt-5.4",
			TimeoutSeconds:  120,
		},
		Providers: ProvidersBlock{Enabled: []string{"openai"}},
		Channels:  ChannelsBlock{Enabled: []string{"webchat"}},
		Plugins:   PluginsBlock{Enabled: []string{}},
	}
}
