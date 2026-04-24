package speech

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

type testSpeechProvider struct {
	name       string
	typ        ProviderType
	voices     []Voice
	synthErr   error
	synthCalls int
	lastText   string
	lastOpts   SynthesizeOptions
}

func (p *testSpeechProvider) Name() string {
	if p.name == "" {
		return "test"
	}
	return p.name
}

func (p *testSpeechProvider) Type() ProviderType {
	if p.typ == "" {
		return ProviderCustom
	}
	return p.typ
}

func (p *testSpeechProvider) Synthesize(ctx context.Context, text string, opts ...SynthesizeOption) (*AudioResult, error) {
	_ = ctx
	p.synthCalls++

	options := SynthesizeOptions{
		Voice:  "default",
		Speed:  1.0,
		Format: FormatMP3,
	}
	for _, opt := range opts {
		opt(&options)
	}

	p.lastText = text
	p.lastOpts = options
	if p.synthErr != nil {
		return nil, p.synthErr
	}

	return &AudioResult{
		Data:       []byte(text + "|" + options.Voice),
		Format:     options.Format,
		SampleRate: 24000,
	}, nil
}

func (p *testSpeechProvider) ListVoices(ctx context.Context) ([]Voice, error) {
	_ = ctx
	return append([]Voice(nil), p.voices...), nil
}

func TestManagerLifecycleAndCache(t *testing.T) {
	primary := &testSpeechProvider{
		name:   "primary",
		voices: []Voice{{ID: "primary-voice", Name: "Primary"}},
	}
	secondary := &testSpeechProvider{
		name:   "secondary",
		voices: []Voice{{ID: "secondary-voice", Name: "Secondary"}},
	}

	manager := NewManager(
		WithManagerCache(CacheConfig{MaxSize: 2, MaxBytes: 64, TTL: time.Minute}),
		WithManagerProvider("primary", primary),
		WithManagerDefault("primary"),
	)

	if manager.Cache() == nil {
		t.Fatal("expected cache to be enabled")
	}

	if got, err := manager.GetDefault(); err != nil || got != primary {
		t.Fatalf("GetDefault() = (%v, %v), want primary provider", got, err)
	}

	if err := manager.Register("", primary); err == nil {
		t.Fatal("expected empty provider name to fail")
	}
	if err := manager.Register("nil", nil); err == nil {
		t.Fatal("expected nil provider to fail")
	}
	if err := manager.Register("primary", primary); err == nil {
		t.Fatal("expected duplicate provider registration to fail")
	}
	if err := manager.Register("secondary", secondary); err != nil {
		t.Fatalf("Register secondary: %v", err)
	}
	if _, err := manager.Get("missing"); err == nil {
		t.Fatal("expected missing provider lookup to fail")
	}
	if err := manager.SetDefault("missing"); err == nil {
		t.Fatal("expected SetDefault on missing provider to fail")
	}
	if err := manager.SetDefault("secondary"); err != nil {
		t.Fatalf("SetDefault secondary: %v", err)
	}

	if got, err := manager.GetDefault(); err != nil || got != secondary {
		t.Fatalf("GetDefault() = (%v, %v), want secondary provider", got, err)
	}

	names := manager.ListProviders()
	sort.Strings(names)
	if len(names) != 2 || names[0] != "primary" || names[1] != "secondary" {
		t.Fatalf("ListProviders() = %v, want [primary secondary]", names)
	}

	result1, err := manager.Synthesize(context.Background(), "hello", "secondary", WithVoice("nova"))
	if err != nil {
		t.Fatalf("Synthesize first call: %v", err)
	}
	result2, err := manager.Synthesize(context.Background(), "hello", "secondary", WithVoice("nova"))
	if err != nil {
		t.Fatalf("Synthesize second call: %v", err)
	}

	if secondary.synthCalls != 1 {
		t.Fatalf("expected cached synthesize call count 1, got %d", secondary.synthCalls)
	}
	if string(result1.Data) != string(result2.Data) {
		t.Fatalf("cached audio mismatch: %q != %q", result1.Data, result2.Data)
	}

	voices, err := manager.ListVoices(context.Background(), "secondary")
	if err != nil {
		t.Fatalf("ListVoices: %v", err)
	}
	if len(voices) != 1 || voices[0].ID != "secondary-voice" {
		t.Fatalf("ListVoices() = %+v, want secondary voice", voices)
	}

	if err := manager.Remove("missing"); err == nil {
		t.Fatal("expected Remove on missing provider to fail")
	}
	if err := manager.Remove("secondary"); err != nil {
		t.Fatalf("Remove secondary: %v", err)
	}
	if got, err := manager.GetDefault(); err != nil || got != primary {
		t.Fatalf("GetDefault() after remove = (%v, %v), want primary provider", got, err)
	}

	manager.ClearCache()
	if size, bytes := manager.CacheStats(); size != 0 || bytes != 0 {
		t.Fatalf("CacheStats() = (%d, %d), want (0, 0)", size, bytes)
	}

	manager.DisableCache()
	if manager.Cache() != nil {
		t.Fatal("expected cache to be disabled")
	}
	if _, err := manager.Synthesize(context.Background(), "uncached", "primary"); err != nil {
		t.Fatalf("uncached synthesize: %v", err)
	}
	if primary.synthCalls != 1 {
		t.Fatalf("expected primary synth calls = 1, got %d", primary.synthCalls)
	}

	manager.EnableCache(DefaultCacheConfig())
	if manager.Cache() == nil {
		t.Fatal("expected cache to be re-enabled")
	}
}

func TestAudioCacheLifecycle(t *testing.T) {
	cache := NewAudioCache(CacheConfig{MaxSize: 2, MaxBytes: 5, TTL: time.Minute})

	cache.Set("nil", nil)
	cache.Set("empty", &AudioResult{})
	if cache.Len() != 0 || cache.SizeBytes() != 0 {
		t.Fatalf("empty cache writes should be ignored, got len=%d bytes=%d", cache.Len(), cache.SizeBytes())
	}

	cache.Set("a", &AudioResult{Data: []byte("aa")})
	cache.Set("b", &AudioResult{Data: []byte("bb")})
	if cache.Len() != 2 || cache.SizeBytes() != 4 {
		t.Fatalf("after Set() len=%d bytes=%d, want len=2 bytes=4", cache.Len(), cache.SizeBytes())
	}

	if got, ok := cache.Get("a"); !ok || string(got.Data) != "aa" {
		t.Fatalf("Get(a) = (%v, %v), want audio 'aa'", got, ok)
	}

	cache.Set("oversize", &AudioResult{Data: []byte("123456")})
	if _, ok := cache.Get("oversize"); ok {
		t.Fatal("expected oversize entry to be ignored")
	}

	cache.Set("c", &AudioResult{Data: []byte("c")})
	if cache.Len() > 2 || cache.SizeBytes() > 5 {
		t.Fatalf("cache limits exceeded: len=%d bytes=%d", cache.Len(), cache.SizeBytes())
	}

	cache.items["expired"] = audioCacheItem{
		result:    &AudioResult{Data: []byte("z")},
		expiresAt: time.Now().Add(-time.Second),
		sizeBytes: 1,
	}
	cache.currentBytes++

	if got, ok := cache.Get("expired"); ok || got != nil {
		t.Fatalf("Get(expired) = (%v, %v), want (nil, false)", got, ok)
	}
	if removed := cache.Cleanup(); removed != 1 {
		t.Fatalf("Cleanup() = %d, want 1", removed)
	}

	cache.Remove("a")
	cache.Clear()
	if cache.Len() != 0 || cache.SizeBytes() != 0 {
		t.Fatalf("after Clear() len=%d bytes=%d, want 0/0", cache.Len(), cache.SizeBytes())
	}

	key1 := MakeCacheKey("hello", "provider", WithVoice("nova"), WithSpeed(1.25), WithFormat(FormatWAV), WithLanguage("en-US"))
	key2 := MakeCacheKey("hello", "provider", WithVoice("nova"), WithSpeed(1.25), WithFormat(FormatWAV), WithLanguage("en-US"))
	key3 := MakeCacheKey("hello", "provider", WithVoice("echo"))

	if key1 != key2 {
		t.Fatalf("expected identical cache keys, got %q != %q", key1, key2)
	}
	if key1 == key3 {
		t.Fatalf("expected different cache keys, got identical key %q", key1)
	}
}

func TestProviderHelpersAndFactory(t *testing.T) {
	opts := SynthesizeOptions{}
	WithVoice("voice-a")(&opts)
	WithSpeed(1.5)(&opts)
	WithPitch(0.2)(&opts)
	WithVolume(0.8)(&opts)
	WithFormat(FormatWAV)(&opts)
	WithLanguage("ja-JP")(&opts)
	WithSampleRate(44100)(&opts)

	if opts.Voice != "voice-a" || opts.Speed != 1.5 || opts.Pitch != 0.2 || opts.Volume != 0.8 || opts.Format != FormatWAV || opts.Language != "ja-JP" || opts.SampleRate != 44100 {
		t.Fatalf("unexpected synth options: %+v", opts)
	}

	audio := []byte("audio-data")
	encoded := AudioToBase64(audio)
	decoded, err := Base64ToAudio(encoded)
	if err != nil {
		t.Fatalf("Base64ToAudio(valid): %v", err)
	}
	if string(decoded) != string(audio) {
		t.Fatalf("decoded audio mismatch: %q != %q", decoded, audio)
	}
	if _, err := Base64ToAudio("%%%"); err == nil {
		t.Fatal("expected invalid base64 to fail")
	}

	if _, err := NewProvider(Config{Type: ProviderOpenAI}); err == nil {
		t.Fatal("expected openai provider without API key to fail")
	}
	if _, err := NewProvider(Config{Type: ProviderElevenLabs}); err == nil {
		t.Fatal("expected elevenlabs provider without API key to fail")
	}

	edgeProvider, err := NewProvider(Config{
		Type:     ProviderEdge,
		BaseURL:  "http://edge.local",
		Voice:    "edge-voice",
		Language: "zh-CN",
		Timeout:  2 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewProvider(edge): %v", err)
	}
	edge := edgeProvider.(*EdgeProvider)
	if edge.baseURL != "http://edge.local" || edge.voice != "edge-voice" || edge.language != "zh-CN" || edge.client.Timeout != 2*time.Second {
		t.Fatalf("unexpected edge provider config: %+v", edge)
	}

	tmpDir := t.TempDir()
	modelPath := filepath.Join(tmpDir, "test-model.onnx")
	configPath := strings.TrimSuffix(modelPath, ".onnx") + ".onnx.json"
	if err := os.WriteFile(modelPath, []byte("model"), 0o644); err != nil {
		t.Fatalf("WriteFile(model): %v", err)
	}
	if err := os.WriteFile(configPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("WriteFile(config): %v", err)
	}

	piperProvider, err := NewProvider(Config{
		Type:     ProviderPiper,
		Voice:    modelPath,
		Language: "ja-JP",
	})
	if err != nil {
		t.Fatalf("NewProvider(piper): %v", err)
	}
	piper := piperProvider.(*PiperProvider)
	if piper.modelPath != modelPath || piper.configFile != configPath || piper.defaultLang != "ja-JP" {
		t.Fatalf("unexpected piper provider config: %+v", piper)
	}

	coquiProvider, err := NewProvider(Config{
		Type:     ProviderCoqui,
		Voice:    "speaker-a",
		Language: "fr",
	})
	if err != nil {
		t.Fatalf("NewProvider(coqui): %v", err)
	}
	coqui := coquiProvider.(*CoquiProvider)
	if coqui.speakerName != "speaker-a" || coqui.defaultLang != "fr" {
		t.Fatalf("unexpected coqui provider config: %+v", coqui)
	}

	if _, err := NewProvider(Config{Type: ProviderCustom}); err == nil {
		t.Fatal("expected unknown provider type to fail")
	}
}