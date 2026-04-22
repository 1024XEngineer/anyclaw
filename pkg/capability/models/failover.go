package llm

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// FailoverConfig configures model failover behavior
type FailoverConfig struct {
	Enabled        bool            `json:"enabled"`
	MaxRetries     int             `json:"max_retries"`
	RetryDelay     time.Duration   `json:"retry_delay"`
	CooldownPeriod time.Duration   `json:"cooldown_period"`
	FallbackModels []FallbackModel `json:"fallback_models"`
}

// FallbackModel defines a fallback model configuration
type FallbackModel struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Priority int    `json:"priority"` // Lower = higher priority
}

// FailoverClient wraps a Client with failover support
type FailoverClient struct {
	mu          sync.RWMutex
	config      FailoverConfig
	primary     Client
	fallbacks   []Client
	cooldowns   map[string]time.Time
	currentIdx  int
	errorCounts map[string]int
	lastError   map[string]error
}

// NewFailoverClient creates a new failover client
func NewFailoverClient(primary Client, config FailoverConfig) *FailoverClient {
	if config.MaxRetries <= 0 {
		config.MaxRetries = 3
	}
	if config.RetryDelay <= 0 {
		config.RetryDelay = time.Second
	}
	if config.CooldownPeriod <= 0 {
		config.CooldownPeriod = 5 * time.Minute
	}

	return &FailoverClient{
		config:      config,
		primary:     primary,
		cooldowns:   make(map[string]time.Time),
		errorCounts: make(map[string]int),
		lastError:   make(map[string]error),
	}
}

// AddFallback adds a fallback client
func (fc *FailoverClient) AddFallback(client Client) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	fc.fallbacks = append(fc.fallbacks, client)
}

// Chat implements the Client interface with failover
func (fc *FailoverClient) Chat(ctx context.Context, messages []Message, tools []ToolDefinition) (*Response, error) {
	if !fc.config.Enabled {
		return fc.primary.Chat(ctx, messages, tools)
	}

	// Try primary first
	resp, err := fc.tryClient(ctx, fc.primary, messages, tools)
	if err == nil {
		return resp, nil
	}

	// Try fallbacks
	for _, fallback := range fc.fallbacks {
		resp, err = fc.tryClient(ctx, fallback, messages, tools)
		if err == nil {
			return resp, nil
		}
	}

	return nil, fmt.Errorf("all providers failed: %w", err)
}

// StreamChat implements streaming with failover
func (fc *FailoverClient) StreamChat(ctx context.Context, messages []Message, tools []ToolDefinition, onChunk func(string)) error {
	if !fc.config.Enabled {
		return fc.primary.StreamChat(ctx, messages, tools, onChunk)
	}

	// Try primary, but only fail over if it fails before emitting any output.
	var emitted bool
	forwardingChunk := func(chunk string) {
		if chunk == "" {
			return
		}
		emitted = true
		onChunk(chunk)
	}

	err := fc.primary.StreamChat(ctx, messages, tools, forwardingChunk)
	if err == nil {
		return nil
	}
	if emitted {
		return fmt.Errorf("stream interrupted after partial output from %s: %w", fc.primary.Name(), err)
	}

	// Try fallbacks
	for _, fallback := range fc.fallbacks {
		err = fallback.StreamChat(ctx, messages, tools, onChunk)
		if err == nil {
			return nil
		}
	}

	return fmt.Errorf("all providers failed: %w", err)
}

// Name returns the client name
func (fc *FailoverClient) Name() string {
	fc.mu.RLock()
	defer fc.mu.RUnlock()
	return fc.primary.Name()
}

// tryClient attempts to use a client with retry and cooldown
func (fc *FailoverClient) tryClient(ctx context.Context, client Client, messages []Message, tools []ToolDefinition) (*Response, error) {
	clientName := client.Name()

	// Check cooldown
	fc.mu.RLock()
	cooldown, inCooldown := fc.cooldowns[clientName]
	fc.mu.RUnlock()

	if inCooldown && time.Now().Before(cooldown) {
		return nil, fmt.Errorf("client %s is in cooldown until %v", clientName, cooldown)
	}

	// Try with retries
	var lastErr error
	for i := 0; i <= fc.config.MaxRetries; i++ {
		resp, err := client.Chat(ctx, messages, tools)
		if err == nil {
			// Reset error count on success
			fc.mu.Lock()
			fc.errorCounts[clientName] = 0
			delete(fc.cooldowns, clientName)
			fc.mu.Unlock()
			return resp, nil
		}

		lastErr = err

		// Check if error is retryable
		if !isRetryableError(err) {
			break
		}

		// Wait before retry
		if i < fc.config.MaxRetries {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(fc.config.RetryDelay * time.Duration(i+1)):
			}
		}
	}

	// Set cooldown
	fc.mu.Lock()
	fc.errorCounts[clientName]++
	fc.lastError[clientName] = lastErr
	if fc.errorCounts[clientName] >= 2 {
		fc.cooldowns[clientName] = time.Now().Add(fc.config.CooldownPeriod)
	}
	fc.mu.Unlock()

	return nil, lastErr
}

// isRetryableError checks if an error is retryable
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	// Rate limit errors
	if contains(errStr, "rate limit") || contains(errStr, "429") {
		return true
	}
	// Timeout errors
	if contains(errStr, "timeout") || contains(errStr, "deadline") {
		return true
	}
	// Server errors
	if contains(errStr, "500") || contains(errStr, "502") || contains(errStr, "503") {
		return true
	}
	// Connection errors
	if contains(errStr, "connection") || contains(errStr, "refused") {
		return true
	}
	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsImpl(s, substr))
}

func containsImpl(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// GetStatus returns failover status
func (fc *FailoverClient) GetStatus() map[string]any {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	status := map[string]any{
		"enabled": fc.config.Enabled,
		"primary": fc.primary.Name(),
	}

	fallbackNames := make([]string, len(fc.fallbacks))
	for i, f := range fc.fallbacks {
		fallbackNames[i] = f.Name()
	}
	status["fallbacks"] = fallbackNames

	cooldowns := make(map[string]string)
	for name, until := range fc.cooldowns {
		if time.Now().Before(until) {
			cooldowns[name] = until.Format(time.RFC3339)
		}
	}
	status["cooldowns"] = cooldowns
	status["error_counts"] = fc.errorCounts

	return status
}

// ModelDiscovery discovers available models from providers
type ModelDiscovery struct {
	mu        sync.RWMutex
	models    map[string][]ModelInfo
	providers []Provider
}

// ModelInfo represents information about a model
type ModelInfo struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Provider     string   `json:"provider"`
	MaxTokens    int      `json:"max_tokens"`
	Capabilities []string `json:"capabilities"` // chat, completion, vision, tools, reasoning
}

// Provider represents an LLM provider for discovery
type Provider interface {
	Name() string
	ListModels(ctx context.Context) ([]ModelInfo, error)
}

// NewModelDiscovery creates a new model discovery
func NewModelDiscovery() *ModelDiscovery {
	return &ModelDiscovery{
		models: make(map[string][]ModelInfo),
	}
}

// RegisterProvider registers a provider for discovery
func (md *ModelDiscovery) RegisterProvider(provider Provider) {
	md.mu.Lock()
	defer md.mu.Unlock()
	md.providers = append(md.providers, provider)
}

// DiscoverModels discovers models from all providers
func (md *ModelDiscovery) DiscoverModels(ctx context.Context) error {
	md.mu.Lock()
	defer md.mu.Unlock()

	for _, provider := range md.providers {
		models, err := provider.ListModels(ctx)
		if err != nil {
			continue
		}
		md.models[provider.Name()] = models
	}

	return nil
}

// ListModels returns all discovered models
func (md *ModelDiscovery) ListModels() map[string][]ModelInfo {
	md.mu.RLock()
	defer md.mu.RUnlock()

	result := make(map[string][]ModelInfo)
	for k, v := range md.models {
		result[k] = v
	}
	return result
}

// FindModel finds a model by name or capability
func (md *ModelDiscovery) FindModel(name string, capability string) *ModelInfo {
	md.mu.RLock()
	defer md.mu.RUnlock()

	for _, models := range md.models {
		for _, model := range models {
			if model.ID == name || model.Name == name {
				return &model
			}
			if capability != "" {
				for _, cap := range model.Capabilities {
					if cap == capability {
						return &model
					}
				}
			}
		}
	}
	return nil
}
