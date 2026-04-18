package gateway

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/config"
)

func quickProviderHealth(provider config.ProviderProfile) providerHealth {
	if !provider.IsEnabled() {
		return providerHealth{Status: "disabled", Message: "Provider is disabled."}
	}
	if strings.TrimSpace(provider.Provider) == "" {
		return providerHealth{Status: "invalid", Message: "Missing runtime provider type."}
	}
	if providerRequiresAPIKey(provider.Provider) && strings.TrimSpace(provider.APIKey) == "" {
		return providerHealth{Status: "missing_key", Message: "API key required."}
	}
	if base := strings.TrimSpace(provider.BaseURL); base != "" {
		if _, err := url.ParseRequestURI(base); err != nil {
			return providerHealth{Status: "invalid_base_url", Message: "Base URL is not a valid URL."}
		}
	}
	return providerHealth{OK: true, Status: "ready", Message: "Ready to use."}
}

func activeProviderTest(ctx context.Context, provider config.ProviderProfile) providerHealth {
	initial := quickProviderHealth(provider)
	if !initial.OK && initial.Status != "ready" {
		return initial
	}
	baseURL := strings.TrimSpace(provider.BaseURL)
	providerName := strings.TrimSpace(strings.ToLower(provider.Provider))
	if baseURL == "" {
		if strings.EqualFold(providerName, "ollama") {
			baseURL = "http://127.0.0.1:11434"
		} else {
			return providerHealth{OK: true, Status: "ready", Message: "Using provider default endpoint."}
		}
	}
	parsed, err := url.ParseRequestURI(baseURL)
	if err != nil {
		return providerHealth{Status: "invalid_base_url", Message: "Base URL is not a valid URL."}
	}

	targetURL := strings.TrimRight(parsed.String(), "/")
	switch providerName {
	case "ollama":
		targetURL += "/api/tags"
	case "compatible", "qwen":
		targetURL += "/models"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return providerHealth{Status: "request_error", Message: err.Error()}
	}
	if apiKey := strings.TrimSpace(provider.APIKey); apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return providerHealth{Status: "unreachable", Message: err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return providerHealth{
			OK:         true,
			Status:     "reachable",
			Message:    fmt.Sprintf("Endpoint responded with HTTP %d.", resp.StatusCode),
			HTTPStatus: resp.StatusCode,
		}
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return providerHealth{
			Status:     "auth_error",
			Message:    fmt.Sprintf("Endpoint reached but authorization failed with HTTP %d.", resp.StatusCode),
			HTTPStatus: resp.StatusCode,
		}
	}
	if resp.StatusCode == http.StatusNotFound {
		return providerHealth{
			Status:     "endpoint_not_found",
			Message:    "Endpoint returned HTTP 404. Check whether Base URL already contains the correct API path.",
			HTTPStatus: resp.StatusCode,
		}
	}
	return providerHealth{
		Status:     "error",
		Message:    fmt.Sprintf("Endpoint responded with HTTP %d.", resp.StatusCode),
		HTTPStatus: resp.StatusCode,
	}
}
