package config

import "time"

// AuthMode defines the credential type
type AuthMode string

const (
	AuthModeAPIKey AuthMode = "api_key"
	AuthModeOAuth  AuthMode = "oauth"
	AuthModeToken  AuthMode = "token"
)

// AuthProfileConfig defines an auth profile
type AuthProfileConfig struct {
	Provider string   `mapstructure:"provider" json:"provider"`
	Mode     AuthMode `mapstructure:"mode" json:"mode"`
	Email    string   `mapstructure:"email" json:"email,omitempty"`
}

// AuthCooldownConfig defines cooldown configuration for auth failures
type AuthCooldownConfig struct {
	// Default billing backoff (hours). Default: 5.
	BillingBackoffHours int `mapstructure:"billing_backoff_hours" json:"billing_backoff_hours"`
	// Optional per-provider billing backoff (hours).
	BillingBackoffHoursByProvider map[string]int `mapstructure:"billing_backoff_hours_by_provider" json:"billing_backoff_hours_by_provider"`
	// Billing backoff cap (hours). Default: 24.
	BillingMaxHours int `mapstructure:"billing_max_hours" json:"billing_max_hours"`
	// Failure window for backoff counters (hours). If no failures occur within
	// this window, counters reset. Default: 24.
	FailureWindowHours int `mapstructure:"failure_window_hours" json:"failure_window_hours"`
}

// AuthConfig defines auth configuration
type AuthConfig struct {
	Profiles  map[string]AuthProfileConfig `mapstructure:"profiles" json:"profiles"`
	Order     map[string][]string          `mapstructure:"order" json:"order"`
	Cooldowns *AuthCooldownConfig          `mapstructure:"cooldowns" json:"cooldowns"`
}

// DefaultAuthConfig returns default auth configuration
func DefaultAuthConfig() AuthConfig {
	return AuthConfig{
		Profiles: make(map[string]AuthProfileConfig),
		Order:    make(map[string][]string),
		Cooldowns: &AuthCooldownConfig{
			BillingBackoffHours:           5,
			BillingBackoffHoursByProvider: make(map[string]int),
			BillingMaxHours:               24,
			FailureWindowHours:            24,
		},
	}
}

// GetProfilesForProvider returns all profile IDs for a given provider
func (a *AuthConfig) GetProfilesForProvider(provider string) []string {
	var profiles []string
	for id, profile := range a.Profiles {
		if profile.Provider == provider {
			profiles = append(profiles, id)
		}
	}
	return profiles
}

// GetOrderedProfilesForProvider returns profile IDs for a provider in configured order
func (a *AuthConfig) GetOrderedProfilesForProvider(provider string) []string {
	if order, ok := a.Order[provider]; ok && len(order) > 0 {
		return order
	}
	return a.GetProfilesForProvider(provider)
}

// GetCooldown returns the cooldown duration for a provider
func (a *AuthConfig) GetCooldown(provider string) time.Duration {
	if a.Cooldowns == nil {
		return 5 * time.Hour
	}

	hours := a.Cooldowns.BillingBackoffHours
	if providerHours, ok := a.Cooldowns.BillingBackoffHoursByProvider[provider]; ok {
		hours = providerHours
	}

	// Cap to max hours
	if maxHours := a.Cooldowns.BillingMaxHours; maxHours > 0 && hours > maxHours {
		hours = maxHours
	}

	return time.Duration(hours) * time.Hour
}
