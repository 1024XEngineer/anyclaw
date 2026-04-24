package embedding

import (
	"context"
	"fmt"
)

type Provider interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
	Name() string
	Dimension() int
}

type ProviderType string

const (
	ProviderOpenAI      ProviderType = "openai"
	ProviderGemini      ProviderType = "gemini"
	ProviderOllama      ProviderType = "ollama"
	ProviderVoyage      ProviderType = "voyage"
	ProviderZhipu       ProviderType = "zhipu"
	ProviderDashScope   ProviderType = "dashscope"
	ProviderBaidu       ProviderType = "baidu"
	ProviderSiliconFlow ProviderType = "siliconflow"
)

type Config struct {
	Provider  ProviderType
	APIKey    string
	SecretKey string
	BaseURL   string
	Model     string
	Dimension int
}

func NewProvider(cfg Config) (Provider, error) {
	switch cfg.Provider {
	case ProviderOpenAI:
		opts := []OpenAIOption{}
		if cfg.BaseURL != "" {
			opts = append(opts, WithOpenAIBaseURL(cfg.BaseURL))
		}
		if cfg.Model != "" {
			opts = append(opts, WithOpenAIModel(cfg.Model))
		}
		if cfg.Dimension > 0 {
			opts = append(opts, WithOpenAIDimension(cfg.Dimension))
		}
		return NewOpenAIProvider(cfg.APIKey, opts...)
	case ProviderGemini:
		opts := []GeminiOption{}
		if cfg.Model != "" {
			opts = append(opts, WithGeminiModel(cfg.Model))
		}
		return NewGeminiProvider(cfg.APIKey, opts...)
	case ProviderOllama:
		opts := []OllamaOption{}
		if cfg.BaseURL != "" {
			opts = append(opts, WithOllamaBaseURL(cfg.BaseURL))
		}
		if cfg.Model != "" {
			opts = append(opts, WithOllamaModel(cfg.Model))
		}
		return NewOllamaProvider(opts...), nil
	case ProviderVoyage:
		opts := []VoyageOption{}
		if cfg.Model != "" {
			opts = append(opts, WithVoyageModel(cfg.Model))
		}
		return NewVoyageProvider(cfg.APIKey, opts...)
	case ProviderZhipu:
		opts := []ZhipuOption{}
		if cfg.Model != "" {
			opts = append(opts, WithZhipuModel(cfg.Model))
		}
		return NewZhipuProvider(cfg.APIKey, opts...)
	case ProviderDashScope:
		opts := []DashScopeOption{}
		if cfg.Model != "" {
			opts = append(opts, WithDashScopeModel(cfg.Model))
		}
		return NewDashScopeProvider(cfg.APIKey, opts...)
	case ProviderBaidu:
		opts := []BaiduOption{}
		if cfg.Model != "" {
			opts = append(opts, WithBaiduModel(cfg.Model))
		}
		if cfg.SecretKey != "" {
			opts = append(opts, WithBaiduSecretKey(cfg.SecretKey))
		}
		return NewBaiduProvider(cfg.APIKey, opts...)
	case ProviderSiliconFlow:
		opts := []SiliconFlowOption{}
		if cfg.Model != "" {
			opts = append(opts, WithSiliconFlowModel(cfg.Model))
		}
		return NewSiliconFlowProvider(cfg.APIKey, opts...)
	default:
		return nil, fmt.Errorf("unknown embedding provider: %s", cfg.Provider)
	}
}
