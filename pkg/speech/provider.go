package speech

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"
)

type TTSEngine interface {
	Name() string
	Initialize(config TTSConfig) error
	Synthesize(ctx context.Context, text string, options TTSOptions) (*AudioResult, error)
	ListVoices() []Voice
	Close() error
}

type STTEngine interface {
	Name() string
	Initialize(config STTConfig) error
	Recognize(ctx context.Context, audioData []byte, options STTOptions) (*TranscriptionResult, error)
	ListModels() []STTModel
	Close() error
}

type TTSConfig struct {
	Provider    string
	APIKey      string
	Endpoint    string
	VoiceID     string
	Language    string
	SampleRate  int
	AudioFormat string
}

type TTSOptions struct {
	Voice  string
	Speed  float64
	Pitch  float64
	Volume float64
	Format string
	Engine string
}

type AudioResult struct {
	AudioData   []byte
	Format      string
	SampleRate  int
	Duration    time.Duration
	ContentType string
}

type Voice struct {
	ID       string
	Name     string
	Language string
	Gender   string
	Provider string
}

type STTConfig struct {
	Provider   string
	APIKey     string
	Endpoint   string
	Model      string
	Language   string
	SampleRate int
}

type STTOptions struct {
	Model       string
	Language    string
	Punctuation bool
	Diarization bool
}

type TranscriptionResult struct {
	Text       string
	Confidence float64
	Segments   []TranscriptionSegment
	Language   string
	Duration   time.Duration
}

type TranscriptionSegment struct {
	Text       string
	StartTime  time.Duration
	EndTime    time.Duration
	Confidence float64
}

type STTModel struct {
	ID          string
	Name        string
	Language    string
	Provider    string
	Description string
}

type SpeechProviderRegistry struct {
	ttsEngines map[string]TTSEngine
	sttEngines map[string]STTEngine
}

func NewSpeechProviderRegistry() *SpeechProviderRegistry {
	return &SpeechProviderRegistry{
		ttsEngines: make(map[string]TTSEngine),
		sttEngines: make(map[string]STTEngine),
	}
}

func (r *SpeechProviderRegistry) RegisterTTS(name string, engine TTSEngine) error {
	if _, exists := r.ttsEngines[name]; exists {
		return fmt.Errorf("TTS engine already registered: %s", name)
	}
	r.ttsEngines[name] = engine
	return nil
}

func (r *SpeechProviderRegistry) RegisterSTT(name string, engine STTEngine) error {
	if _, exists := r.sttEngines[name]; exists {
		return fmt.Errorf("STT engine already registered: %s", name)
	}
	r.sttEngines[name] = engine
	return nil
}

func (r *SpeechProviderRegistry) GetTTS(name string) (TTSEngine, bool) {
	engine, ok := r.ttsEngines[name]
	return engine, ok
}

func (r *SpeechProviderRegistry) GetSTT(name string) (STTEngine, bool) {
	engine, ok := r.sttEngines[name]
	return engine, ok
}

func (r *SpeechProviderRegistry) ListTTS() []string {
	names := make([]string, 0, len(r.ttsEngines))
	for name := range r.ttsEngines {
		names = append(names, name)
	}
	return names
}

func (r *SpeechProviderRegistry) ListSTT() []string {
	names := make([]string, 0, len(r.sttEngines))
	for name := range r.sttEngines {
		names = append(names, name)
	}
	return names
}

type OpenAITTS struct {
	config  TTSConfig
	baseURL string
	apiKey  string
	client  HTTPClient
}

type HTTPClient interface {
	Do(req interface{}) (interface{}, error)
}

func NewOpenAITTS() *OpenAITTS {
	return &OpenAITTS{}
}

func (t *OpenAITTS) Name() string { return "openai-tts" }

func (t *OpenAITTS) Initialize(config TTSConfig) error {
	t.config = config
	if config.Endpoint != "" {
		t.baseURL = config.Endpoint
	} else {
		t.baseURL = "https://api.openai.com"
	}
	t.apiKey = config.APIKey
	return nil
}

func (t *OpenAITTS) Synthesize(ctx context.Context, text string, options TTSOptions) (*AudioResult, error) {
	_ = ctx
	_ = text
	_ = options
	return &AudioResult{
		AudioData:   []byte{},
		Format:      "mp3",
		SampleRate:  24000,
		Duration:    0,
		ContentType: "audio/mp3",
	}, nil
}

func (t *OpenAITTS) ListVoices() []Voice {
	return []Voice{
		{ID: "alloy", Name: "Alloy", Language: "en", Gender: "neutral", Provider: "openai"},
		{ID: "echo", Name: "Echo", Language: "en", Gender: "male", Provider: "openai"},
		{ID: "fable", Name: "Fable", Language: "en", Gender: "neutral", Provider: "openai"},
		{ID: "onyx", Name: "Onyx", Language: "en", Gender: "male", Provider: "openai"},
		{ID: "shimmer", Name: "Shimmer", Language: "en", Gender: "female", Provider: "openai"},
	}
}

func (t *OpenAITTS) Close() error {
	return nil
}

type OpenAISTT struct {
	config  STTConfig
	baseURL string
	apiKey  string
}

func NewOpenAISTT() *OpenAISTT {
	return &OpenAISTT{}
}

func (s *OpenAISTT) Name() string { return "openai-stt" }

func (s *OpenAISTT) Initialize(config STTConfig) error {
	s.config = config
	if config.Endpoint != "" {
		s.baseURL = config.Endpoint
	} else {
		s.baseURL = "https://api.openai.com"
	}
	s.apiKey = config.APIKey
	return nil
}

func (s *OpenAISTT) Recognize(ctx context.Context, audioData []byte, options STTOptions) (*TranscriptionResult, error) {
	_ = ctx
	_ = audioData
	_ = options
	return &TranscriptionResult{
		Text:       "",
		Confidence: 0.0,
		Language:   "en",
	}, nil
}

func (s *OpenAISTT) ListModels() []STTModel {
	return []STTModel{
		{ID: "whisper-1", Name: "Whisper", Language: "multi", Provider: "openai", Description: "OpenAI Whisper model"},
	}
}

func (s *OpenAISTT) Close() error {
	return nil
}

type PluginSpeechProvider struct {
	Name_      string
	Type_      string
	Entrypoint string
	Config     map[string]any
}

func (p *PluginSpeechProvider) AsTTS() TTSEngine {
	return nil
}

func (p *PluginSpeechProvider) AsSTT() STTEngine {
	return nil
}

func RegisterPluginSpeechProvider(registry *SpeechProviderRegistry, manifest map[string]any) error {
	name, _ := manifest["name"].(string)
	speechType, _ := manifest["speech_type"].(string)

	if name == "" || speechType == "" {
		return fmt.Errorf("invalid manifest: missing name or speech_type")
	}

	if speechType == "tts" {
		return registry.RegisterTTS(name, nil)
	} else if speechType == "stt" {
		return registry.RegisterSTT(name, nil)
	}

	return fmt.Errorf("unsupported speech type: %s", speechType)
}

func AudioToBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

func Base64ToAudio(b64 string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(b64)
}
