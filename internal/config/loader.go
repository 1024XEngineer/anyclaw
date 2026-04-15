package config

// Load returns the default configuration for now.
// A future implementation can merge file, env, and CLI overrides.
func Load(_ string) (Config, error) {
	cfg := Default()
	return cfg, nil
}
