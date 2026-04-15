package config

// Config is the root runtime configuration for the application.
type Config struct {
	AppName   string         `yaml:"appName"`
	Gateway   GatewayConfig  `yaml:"gateway"`
	Agents    AgentsConfig   `yaml:"agents"`
	Providers ProvidersBlock `yaml:"providers"`
	Channels  ChannelsBlock  `yaml:"channels"`
	Plugins   PluginsBlock   `yaml:"plugins"`
}

// GatewayConfig stores control-plane listener settings.
type GatewayConfig struct {
	Bind            string `yaml:"bind"`
	Port            int    `yaml:"port"`
	AuthToken       string `yaml:"authToken"`
	ProtocolVersion int    `yaml:"protocolVersion"`
}

// AgentsConfig stores default agent runtime settings.
type AgentsConfig struct {
	DefaultAgentID  string `yaml:"defaultAgentId"`
	DefaultProvider string `yaml:"defaultProvider"`
	DefaultModel    string `yaml:"defaultModel"`
	TimeoutSeconds  int    `yaml:"timeoutSeconds"`
}

// ProvidersBlock lists enabled providers.
type ProvidersBlock struct {
	Enabled []string `yaml:"enabled"`
}

// ChannelsBlock lists enabled channels.
type ChannelsBlock struct {
	Enabled []string `yaml:"enabled"`
}

// PluginsBlock lists enabled plugins.
type PluginsBlock struct {
	Enabled []string `yaml:"enabled"`
}
