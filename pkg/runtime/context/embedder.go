package context

import (
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/config"
	"github.com/1024XEngineer/anyclaw/pkg/state/memory"
	"github.com/1024XEngineer/anyclaw/pkg/state/policy/secrets"
)

func ResolveEmbedder(cfg *config.Config, secretsSnap *secrets.RuntimeSnapshot) memory.EmbeddingProvider {
	if cfg == nil {
		return nil
	}
	if embedModel := strings.TrimSpace(cfg.LLM.Extra["embed_model"]); embedModel != "" {
		baseURL := cfg.LLM.BaseURL
		if v := strings.TrimSpace(cfg.LLM.Extra["embed_base_url"]); v != "" {
			baseURL = v
		}
		apiKey := resolveSecret(secretsSnap, cfg.LLM.APIKey, "llm_api_key")
		if v := strings.TrimSpace(cfg.LLM.Extra["embed_api_key"]); v != "" {
			apiKey = resolveSecret(secretsSnap, v, "embed_api_key")
		}
		switch strings.ToLower(cfg.LLM.Provider) {
		case "ollama":
			return memory.NewOllamaEmbeddingProvider(baseURL, embedModel)
		default:
			return memory.NewOpenAIEmbeddingProvider(apiKey, embedModel, baseURL)
		}
	}
	if strings.ToLower(cfg.LLM.Provider) == "ollama" {
		return memory.NewOllamaEmbeddingProvider(cfg.LLM.BaseURL, "")
	}
	apiKey := resolveSecret(secretsSnap, cfg.LLM.APIKey, "llm_api_key")
	if strings.TrimSpace(apiKey) != "" {
		return memory.NewOpenAIEmbeddingProvider(apiKey, "", cfg.LLM.BaseURL)
	}
	return nil
}

func resolveSecret(snap *secrets.RuntimeSnapshot, plaintext string, secretKey string) string {
	if snap != nil {
		if strings.Contains(plaintext, "${SECRET:") {
			resolved, err := snap.ResolveValueStrict(plaintext)
			if err == nil {
				return resolved
			}
		}
		if entry, ok := snap.Get(secretKey); ok {
			return entry.Value
		}
	}
	return plaintext
}
