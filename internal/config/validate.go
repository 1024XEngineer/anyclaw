package config

import "fmt"

// Validate checks the minimal invariants required to boot the application.
func Validate(cfg Config) error {
	if cfg.AppName == "" {
		return fmt.Errorf("config.appName must not be empty")
	}
	if cfg.Gateway.Port <= 0 {
		return fmt.Errorf("config.gateway.port must be greater than zero")
	}
	if cfg.Agents.DefaultAgentID == "" {
		return fmt.Errorf("config.agents.defaultAgentId must not be empty")
	}
	return nil
}
