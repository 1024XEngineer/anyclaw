package embedding

import (
	"context"
	"testing"
	"time"
)

func TestCache(t *testing.T) {
	cache := NewCache(100, time.Hour)

	cache.Set("hello", []float32{0.1, 0.2, 0.3})

	val, ok := cache.Get("hello")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if len(val) != 3 || val[0] != 0.1 {
		t.Errorf("unexpected cached value: %v", val)
	}

	_, ok = cache.Get("missing")
	if ok {
		t.Error("expected cache miss")
	}
}

func TestCacheTTL(t *testing.T) {
	cache := NewCache(100, 50*time.Millisecond)

	cache.Set("expire_me", []float32{0.1, 0.2})

	_, ok := cache.Get("expire_me")
	if !ok {
		t.Fatal("expected cache hit before expiry")
	}

	time.Sleep(100 * time.Millisecond)

	_, ok = cache.Get("expire_me")
	if ok {
		t.Error("expected cache miss after TTL")
	}
}

func TestCacheEviction(t *testing.T) {
	cache := NewCache(5, time.Hour)

	for i := 0; i < 10; i++ {
		cache.Set(string(rune('a'+i)), []float32{float32(i)})
	}

	if cache.Len() > 5 {
		t.Errorf("expected cache size <= 5 after eviction, got %d", cache.Len())
	}
}

func TestCacheClear(t *testing.T) {
	cache := NewCache(100, time.Hour)
	cache.Set("a", []float32{1.0})
	cache.Set("b", []float32{2.0})

	cache.Clear()

	if cache.Len() != 0 {
		t.Errorf("expected empty cache after clear, got %d", cache.Len())
	}
}

func TestNewProvider(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "openai",
			cfg: Config{
				Provider: ProviderOpenAI,
				APIKey:   "test-key",
				Model:    "text-embedding-3-small",
			},
			wantErr: false,
		},
		{
			name: "openai no key",
			cfg: Config{
				Provider: ProviderOpenAI,
			},
			wantErr: true,
		},
		{
			name: "gemini",
			cfg: Config{
				Provider: ProviderGemini,
				APIKey:   "test-key",
			},
			wantErr: false,
		},
		{
			name: "ollama",
			cfg: Config{
				Provider: ProviderOllama,
				Model:    "nomic-embed-text",
			},
			wantErr: false,
		},
		{
			name: "voyage",
			cfg: Config{
				Provider: ProviderVoyage,
				APIKey:   "test-key",
			},
			wantErr: false,
		},
		{
			name: "zhipu",
			cfg: Config{
				Provider: ProviderZhipu,
				APIKey:   "test-key",
			},
			wantErr: false,
		},
		{
			name: "dashscope",
			cfg: Config{
				Provider: ProviderDashScope,
				APIKey:   "test-key",
			},
			wantErr: false,
		},
		{
			name: "baidu",
			cfg: Config{
				Provider:  ProviderBaidu,
				APIKey:    "test-key",
				SecretKey: "test-secret",
			},
			wantErr: false,
		},
		{
			name: "baidu no secret",
			cfg: Config{
				Provider: ProviderBaidu,
				APIKey:   "test-key",
			},
			wantErr: true,
		},
		{
			name: "siliconflow",
			cfg: Config{
				Provider: ProviderSiliconFlow,
				APIKey:   "test-key",
			},
			wantErr: false,
		},
		{
			name: "unknown",
			cfg: Config{
				Provider: "unknown",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := NewProvider(tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if p == nil {
				t.Fatal("expected non-nil provider")
			}
		})
	}
}

func TestOpenAIProvider(t *testing.T) {
	p, err := NewOpenAIProvider("test-key")
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	if p.Name() != "openai" {
		t.Errorf("expected name openai, got %s", p.Name())
	}
	if p.Dimension() != 1536 {
		t.Errorf("expected dimension 1536, got %d", p.Dimension())
	}
}

func TestOpenAIProviderOptions(t *testing.T) {
	p, err := NewOpenAIProvider("test-key",
		WithOpenAIModel("text-embedding-3-large"),
		WithOpenAIDimension(3072),
	)
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	if p.Dimension() != 3072 {
		t.Errorf("expected dimension 3072, got %d", p.Dimension())
	}
}

func TestGeminiProvider(t *testing.T) {
	p, err := NewGeminiProvider("test-key")
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	if p.Name() != "gemini" {
		t.Errorf("expected name gemini, got %s", p.Name())
	}
	if p.Dimension() != 768 {
		t.Errorf("expected dimension 768, got %d", p.Dimension())
	}
}

func TestOllamaProvider(t *testing.T) {
	p := NewOllamaProvider(
		WithOllamaBaseURL("http://localhost:11434"),
		WithOllamaModel("nomic-embed-text"),
	)

	if p.Name() != "ollama" {
		t.Errorf("expected name ollama, got %s", p.Name())
	}
	if p.Dimension() != 768 {
		t.Errorf("expected dimension 768, got %d", p.Dimension())
	}
}

func TestVoyageProvider(t *testing.T) {
	p, err := NewVoyageProvider("test-key")
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	if p.Name() != "voyage" {
		t.Errorf("expected name voyage, got %s", p.Name())
	}
	if p.Dimension() != 1024 {
		t.Errorf("expected dimension 1024, got %d", p.Dimension())
	}
}

func TestZhipuProvider(t *testing.T) {
	p, err := NewZhipuProvider("test-key")
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	if p.Name() != "zhipu" {
		t.Errorf("expected name zhipu, got %s", p.Name())
	}
	if p.Dimension() != 2048 {
		t.Errorf("expected dimension 2048, got %d", p.Dimension())
	}
}

func TestDashScopeProvider(t *testing.T) {
	p, err := NewDashScopeProvider("test-key")
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	if p.Name() != "dashscope" {
		t.Errorf("expected name dashscope, got %s", p.Name())
	}
	if p.Dimension() != 1024 {
		t.Errorf("expected dimension 1024, got %d", p.Dimension())
	}
}

func TestBaiduProvider(t *testing.T) {
	p, err := NewBaiduProvider("test-key", WithBaiduSecretKey("test-secret"))
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	if p.Name() != "baidu" {
		t.Errorf("expected name baidu, got %s", p.Name())
	}
	if p.Dimension() != 384 {
		t.Errorf("expected dimension 384, got %d", p.Dimension())
	}
}

func TestBaiduProviderRequiresSecretKey(t *testing.T) {
	_, err := NewBaiduProvider("test-key")
	if err == nil {
		t.Fatal("expected error when secret key is missing")
	}
}

func TestSiliconFlowProvider(t *testing.T) {
	p, err := NewSiliconFlowProvider("test-key")
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	if p.Name() != "siliconflow" {
		t.Errorf("expected name siliconflow, got %s", p.Name())
	}
	if p.Dimension() != 1024 {
		t.Errorf("expected dimension 1024, got %d", p.Dimension())
	}
}

func TestManager(t *testing.T) {
	mock := &mockProvider{name: "mock", dim: 4}

	mgr, err := NewManager([]Provider{mock}, nil)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	ctx := context.Background()
	emb, err := mgr.Embed(ctx, "hello")
	if err != nil {
		t.Fatalf("embed failed: %v", err)
	}

	if len(emb) != 4 {
		t.Errorf("expected 4 dimensions, got %d", len(emb))
	}

	if mgr.CurrentProvider().Name() != "mock" {
		t.Errorf("expected current provider mock, got %s", mgr.CurrentProvider().Name())
	}
}

func TestManagerFallback(t *testing.T) {
	fail := &mockProvider{name: "fail", dim: 4, shouldFail: true}
	success := &mockProvider{name: "success", dim: 4}

	mgr, err := NewManager([]Provider{fail, success}, nil)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	ctx := context.Background()
	emb, err := mgr.Embed(ctx, "test")
	if err != nil {
		t.Fatalf("embed should succeed with fallback: %v", err)
	}

	if len(emb) != 4 {
		t.Errorf("expected 4 dimensions, got %d", len(emb))
	}

	if mgr.CurrentProvider().Name() != "success" {
		t.Errorf("expected fallback to success provider")
	}
}

func TestManagerUsesCurrentProviderAfterFallback(t *testing.T) {
	failCalls := 0
	successCalls := 0
	fail := &mockProvider{name: "fail", dim: 4, shouldFail: true, onEmbed: func() {
		failCalls++
	}}
	success := &mockProvider{name: "success", dim: 4, onEmbed: func() {
		successCalls++
	}}

	mgr, err := NewManager([]Provider{fail, success}, nil)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	ctx := context.Background()
	if _, err := mgr.Embed(ctx, "first"); err != nil {
		t.Fatalf("first embed should succeed with fallback: %v", err)
	}
	if _, err := mgr.Embed(ctx, "second"); err != nil {
		t.Fatalf("second embed should succeed with current provider: %v", err)
	}

	if failCalls != 1 {
		t.Fatalf("expected failed provider to be tried once, got %d", failCalls)
	}
	if successCalls != 2 {
		t.Fatalf("expected current provider to handle both successful requests, got %d", successCalls)
	}
}

func TestManagerAllFail(t *testing.T) {
	fail1 := &mockProvider{name: "fail1", dim: 4, shouldFail: true}
	fail2 := &mockProvider{name: "fail2", dim: 4, shouldFail: true}

	mgr, err := NewManager([]Provider{fail1, fail2}, nil)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	ctx := context.Background()
	_, err = mgr.Embed(ctx, "test")
	if err == nil {
		t.Error("expected error when all providers fail")
	}
}

func TestManagerCache(t *testing.T) {
	callCount := 0
	mock := &mockProvider{name: "mock", dim: 4, onEmbed: func() {
		callCount++
	}}

	mgr, _ := NewManager([]Provider{mock}, NewCache(100, time.Hour))

	ctx := context.Background()

	mgr.Embed(ctx, "cached")
	mgr.Embed(ctx, "cached")
	mgr.Embed(ctx, "cached")

	if callCount != 1 {
		t.Errorf("expected 1 provider call due to caching, got %d", callCount)
	}
}

func TestManagerProviders(t *testing.T) {
	p1 := &mockProvider{name: "p1", dim: 4}
	p2 := &mockProvider{name: "p2", dim: 8}

	mgr, _ := NewManager([]Provider{p1, p2}, nil)

	providers := mgr.Providers()
	if len(providers) != 2 {
		t.Errorf("expected 2 providers, got %d", len(providers))
	}
}

type mockProvider struct {
	name       string
	dim        int
	shouldFail bool
	onEmbed    func()
}

func (m *mockProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	if m.onEmbed != nil {
		m.onEmbed()
	}
	if m.shouldFail {
		return nil, context.Canceled
	}
	result := make([]float32, m.dim)
	for i := range result {
		result[i] = float32(len(text)) / float32(m.dim)
	}
	return result, nil
}

func (m *mockProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	var results [][]float32
	for _, text := range texts {
		emb, err := m.Embed(ctx, text)
		if err != nil {
			return nil, err
		}
		results = append(results, emb)
	}
	return results, nil
}

func (m *mockProvider) Name() string   { return m.name }
func (m *mockProvider) Dimension() int { return m.dim }
