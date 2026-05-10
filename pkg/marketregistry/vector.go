package marketregistry

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/embedding"
)

type embeddingProvider interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
	Name() string
	Dimension() int
}

type VectorConfig struct {
	Enabled         bool
	FailOpen        bool
	Provider        string
	Model           string
	QueryModel      string
	APIKey          string
	BaseURL         string
	SecretKey       string
	QueryTimeout    time.Duration
	WorkerPoll      time.Duration
	MaxJobAttempts  int
	HybridTopK      int
	HybridCandidate int
	QueryCacheTTL   time.Duration
	QueryCacheSize  int
}

type vectorRuntime struct {
	cfg           VectorConfig
	artifactEmbed embeddingProvider
	queryEmbed    embeddingProvider
	queryCache    *embedding.Cache
	metrics       *registryMetrics
}

func normalizeVectorConfig(cfg VectorConfig) VectorConfig {
	if cfg.QueryTimeout <= 0 {
		cfg.QueryTimeout = 12 * time.Second
	}
	if cfg.WorkerPoll <= 0 {
		cfg.WorkerPoll = 5 * time.Second
	}
	if cfg.MaxJobAttempts <= 0 {
		cfg.MaxJobAttempts = 3
	}
	if cfg.HybridTopK <= 0 {
		cfg.HybridTopK = 25
	}
	if cfg.HybridCandidate <= 0 {
		cfg.HybridCandidate = 200
	}
	if cfg.QueryCacheTTL <= 0 {
		cfg.QueryCacheTTL = 10 * time.Minute
	}
	if cfg.QueryCacheSize <= 0 {
		cfg.QueryCacheSize = 512
	}
	cfg.Provider = strings.TrimSpace(strings.ToLower(cfg.Provider))
	cfg.Model = strings.TrimSpace(cfg.Model)
	cfg.QueryModel = strings.TrimSpace(cfg.QueryModel)
	if cfg.QueryModel == "" {
		cfg.QueryModel = cfg.Model
	}
	return cfg
}

func newVectorRuntime(cfg VectorConfig) (*vectorRuntime, error) {
	cfg = normalizeVectorConfig(cfg)
	if !cfg.Enabled {
		return &vectorRuntime{cfg: cfg}, nil
	}
	if cfg.Provider == "" || cfg.Model == "" || cfg.QueryModel == "" || strings.TrimSpace(cfg.APIKey) == "" {
		if cfg.FailOpen {
			return &vectorRuntime{cfg: cfg}, nil
		}
		return nil, fmt.Errorf("vector search is enabled but embedding provider/model/api key is incomplete")
	}

	artifactProvider, err := embedding.NewProvider(embedding.Config{
		Provider:  embedding.ProviderType(cfg.Provider),
		APIKey:    cfg.APIKey,
		SecretKey: cfg.SecretKey,
		BaseURL:   cfg.BaseURL,
		Model:     cfg.Model,
	})
	if err != nil {
		if cfg.FailOpen {
			return &vectorRuntime{cfg: cfg}, nil
		}
		return nil, err
	}
	queryProvider, err := embedding.NewProvider(embedding.Config{
		Provider:  embedding.ProviderType(cfg.Provider),
		APIKey:    cfg.APIKey,
		SecretKey: cfg.SecretKey,
		BaseURL:   cfg.BaseURL,
		Model:     cfg.QueryModel,
	})
	if err != nil {
		if cfg.FailOpen {
			return &vectorRuntime{cfg: cfg}, nil
		}
		return nil, err
	}
	return &vectorRuntime{
		cfg:           cfg,
		artifactEmbed: artifactProvider,
		queryEmbed:    queryProvider,
		queryCache:    embedding.NewCache(cfg.QueryCacheSize, cfg.QueryCacheTTL),
	}, nil
}

func newVectorRuntimeWithProviders(cfg VectorConfig, artifactProvider, queryProvider embeddingProvider) (*vectorRuntime, error) {
	cfg = normalizeVectorConfig(cfg)
	if artifactProvider == nil {
		artifactProvider = queryProvider
	}
	if queryProvider == nil {
		queryProvider = artifactProvider
	}
	if !cfg.Enabled || artifactProvider == nil || queryProvider == nil {
		return &vectorRuntime{cfg: cfg}, nil
	}
	return &vectorRuntime{
		cfg:           cfg,
		artifactEmbed: artifactProvider,
		queryEmbed:    queryProvider,
		queryCache:    embedding.NewCache(cfg.QueryCacheSize, cfg.QueryCacheTTL),
	}, nil
}

func (r *vectorRuntime) enabled() bool {
	return r != nil && r.cfg.Enabled && r.artifactEmbed != nil && r.queryEmbed != nil
}

func (r *vectorRuntime) artifactModel() string {
	if r == nil {
		return ""
	}
	return r.cfg.Model
}

func (r *vectorRuntime) queryModel() string {
	if r == nil {
		return ""
	}
	return r.cfg.QueryModel
}
