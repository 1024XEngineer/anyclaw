package speech

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type OpenAIProvider struct {
	apiKey  string
	baseURL string
	voice   string
	timeout time.Duration
	client  *http.Client
}

type OpenAIOption func(*OpenAIProvider)

func WithOpenAIBaseURL(url string) OpenAIOption {
	return func(p *OpenAIProvider) {
		p.baseURL = url
	}
}

func WithOpenAIVoice(voice string) OpenAIOption {
	return func(p *OpenAIProvider) {
		p.voice = voice
	}
}

func WithOpenAITimeout(timeout time.Duration) OpenAIOption {
	return func(p *OpenAIProvider) {
		p.timeout = timeout
	}
}

func NewOpenAIProvider(apiKey string, opts ...OpenAIOption) (*OpenAIProvider, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("openai: API key is required")
	}

	p := &OpenAIProvider{
		apiKey:  apiKey,
		baseURL: "https://api.openai.com",
		voice:   "alloy",
		timeout: 60 * time.Second,
		client:  &http.Client{Timeout: 60 * time.Second},
	}

	for _, opt := range opts {
		opt(p)
	}

	p.client.Timeout = p.timeout

	return p, nil
}

func (p *OpenAIProvider) Name() string {
	return "openai"
}

func (p *OpenAIProvider) Type() ProviderType {
	return ProviderOpenAI
}

func (p *OpenAIProvider) Synthesize(ctx context.Context, text string, opts ...SynthesizeOption) (*AudioResult, error) {
	options := SynthesizeOptions{
		Voice:      p.voice,
		Speed:      1.0,
		Format:     FormatMP3,
		SampleRate: 24000,
	}
	for _, opt := range opts {
		opt(&options)
	}

	payload := map[string]any{
		"model":  "tts-1",
		"input":  text,
		"voice":  options.Voice,
		"speed":  options.Speed,
		"format": string(options.Format),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("openai: failed to marshal request: %w", err)
	}

	url := p.baseURL + "/v1/audio/speech"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai: failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai: API error (%d): %s", resp.StatusCode, string(respBody))
	}

	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openai: failed to read response: %w", err)
	}

	return &AudioResult{
		Data:        audioData,
		Format:      options.Format,
		SampleRate:  options.SampleRate,
		ContentType: "audio/mpeg",
	}, nil
}

func (p *OpenAIProvider) ListVoices(ctx context.Context) ([]Voice, error) {
	_ = ctx
	return []Voice{
		{ID: "alloy", Name: "Alloy", Language: "en", Gender: GenderNeutral, Provider: "openai"},
		{ID: "echo", Name: "Echo", Language: "en", Gender: GenderMale, Provider: "openai"},
		{ID: "fable", Name: "Fable", Language: "en", Gender: GenderNeutral, Provider: "openai"},
		{ID: "onyx", Name: "Onyx", Language: "en", Gender: GenderMale, Provider: "openai"},
		{ID: "nova", Name: "Nova", Language: "en", Gender: GenderFemale, Provider: "openai"},
		{ID: "shimmer", Name: "Shimmer", Language: "en", Gender: GenderFemale, Provider: "openai"},
	}, nil
}

type ElevenLabsProvider struct {
	apiKey  string
	baseURL string
	voice   string
	timeout time.Duration
	client  *http.Client
}

type ElevenLabsOption func(*ElevenLabsProvider)

func WithElevenLabsBaseURL(url string) ElevenLabsOption {
	return func(p *ElevenLabsProvider) {
		p.baseURL = url
	}
}

func WithElevenLabsVoice(voice string) ElevenLabsOption {
	return func(p *ElevenLabsProvider) {
		p.voice = voice
	}
}

func WithElevenLabsTimeout(timeout time.Duration) ElevenLabsOption {
	return func(p *ElevenLabsProvider) {
		p.timeout = timeout
	}
}

func NewElevenLabsProvider(apiKey string, opts ...ElevenLabsOption) (*ElevenLabsProvider, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("elevenlabs: API key is required")
	}

	p := &ElevenLabsProvider{
		apiKey:  apiKey,
		baseURL: "https://api.elevenlabs.io/v1",
		voice:   "21m00Tcm4TlvDq8ikWAM",
		timeout: 60 * time.Second,
		client:  &http.Client{Timeout: 60 * time.Second},
	}

	for _, opt := range opts {
		opt(p)
	}

	p.client.Timeout = p.timeout

	return p, nil
}

func (p *ElevenLabsProvider) Name() string {
	return "elevenlabs"
}

func (p *ElevenLabsProvider) Type() ProviderType {
	return ProviderElevenLabs
}

func (p *ElevenLabsProvider) Synthesize(ctx context.Context, text string, opts ...SynthesizeOption) (*AudioResult, error) {
	options := SynthesizeOptions{
		Voice:  p.voice,
		Format: FormatMP3,
	}
	for _, opt := range opts {
		opt(&options)
	}

	payload := map[string]any{
		"text":     text,
		"model_id": "eleven_multilingual_v2",
		"voice_settings": map[string]any{
			"stability":         0.5,
			"similarity_boost":  0.75,
			"style":             0.0,
			"use_speaker_boost": true,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs: failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/text-to-speech/%s", p.baseURL, options.Voice)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("elevenlabs: failed to create request: %w", err)
	}
	req.Header.Set("xi-api-key", p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("elevenlabs: API error (%d): %s", resp.StatusCode, string(respBody))
	}

	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs: failed to read response: %w", err)
	}

	return &AudioResult{
		Data:        audioData,
		Format:      options.Format,
		ContentType: "audio/mpeg",
	}, nil
}

func (p *ElevenLabsProvider) ListVoices(ctx context.Context) ([]Voice, error) {
	_ = ctx
	return []Voice{
		{ID: "21m00Tcm4TlvDq8ikWAM", Name: "Rachel", Language: "en", Gender: GenderFemale, Provider: "elevenlabs"},
		{ID: "EXAVITQu4vr4xnSDxMaL", Name: "Bella", Language: "en", Gender: GenderFemale, Provider: "elevenlabs"},
		{ID: "ErXwobaYiN019PkySvjV", Name: "Antoni", Language: "en", Gender: GenderMale, Provider: "elevenlabs"},
		{ID: "VR6AewLTigWG4xSOukaG", Name: "Arnold", Language: "en", Gender: GenderMale, Provider: "elevenlabs"},
		{ID: "pNInz6obpgDQGcFmaJgB", Name: "Adam", Language: "en", Gender: GenderMale, Provider: "elevenlabs"},
	}, nil
}

type EdgeProvider struct {
	baseURL  string
	voice    string
	language string
	timeout  time.Duration
	client   *http.Client
}

type EdgeOption func(*EdgeProvider)

func WithEdgeBaseURL(url string) EdgeOption {
	return func(p *EdgeProvider) {
		p.baseURL = url
	}
}

func WithEdgeVoice(voice string) EdgeOption {
	return func(p *EdgeProvider) {
		p.voice = voice
	}
}

func WithEdgeLanguage(lang string) EdgeOption {
	return func(p *EdgeProvider) {
		p.language = lang
	}
}

func WithEdgeTimeout(timeout time.Duration) EdgeOption {
	return func(p *EdgeProvider) {
		p.timeout = timeout
	}
}

func NewEdgeProvider(opts ...EdgeOption) (*EdgeProvider, error) {
	p := &EdgeProvider{
		baseURL:  "https://speech.platform.bing.com",
		voice:    "en-US-AriaNeural",
		language: "en-US",
		timeout:  30 * time.Second,
		client:   &http.Client{Timeout: 30 * time.Second},
	}

	for _, opt := range opts {
		opt(p)
	}

	p.client.Timeout = p.timeout

	return p, nil
}

func (p *EdgeProvider) Name() string {
	return "edge"
}

func (p *EdgeProvider) Type() ProviderType {
	return ProviderEdge
}

func (p *EdgeProvider) Synthesize(ctx context.Context, text string, opts ...SynthesizeOption) (*AudioResult, error) {
	options := SynthesizeOptions{
		Voice:    p.voice,
		Language: p.language,
		Format:   FormatMP3,
	}
	for _, opt := range opts {
		opt(&options)
	}

	if options.Voice == "" {
		options.Voice = p.voice
	}

	ssml := fmt.Sprintf(`<speak version="1.0" xmlns="http://www.w3.org/2001/10/synthesis" xml:lang="%s">
		<voice name="%s">%s</voice>
	</speak>`, options.Language, options.Voice, text)

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/synthesize", bytes.NewReader([]byte(ssml)))
	if err != nil {
		return nil, fmt.Errorf("edge: failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/ssml+xml")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("edge: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("edge: API error (%d): %s", resp.StatusCode, string(respBody))
	}

	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("edge: failed to read response: %w", err)
	}

	return &AudioResult{
		Data:        audioData,
		Format:      options.Format,
		ContentType: "audio/mpeg",
	}, nil
}

func (p *EdgeProvider) ListVoices(ctx context.Context) ([]Voice, error) {
	_ = ctx
	return []Voice{
		{ID: "en-US-AriaNeural", Name: "Aria", Language: "en-US", LanguageTag: "en-US", Gender: GenderFemale, Provider: "edge"},
		{ID: "en-US-GuyNeural", Name: "Guy", Language: "en-US", LanguageTag: "en-US", Gender: GenderMale, Provider: "edge"},
		{ID: "zh-CN-XiaoxiaoNeural", Name: "Xiaoxiao", Language: "zh-CN", LanguageTag: "zh-CN", Gender: GenderFemale, Provider: "edge"},
		{ID: "zh-CN-YunxiNeural", Name: "Yunxi", Language: "zh-CN", LanguageTag: "zh-CN", Gender: GenderMale, Provider: "edge"},
		{ID: "ja-JP-NanamiNeural", Name: "Nanami", Language: "ja-JP", LanguageTag: "ja-JP", Gender: GenderFemale, Provider: "edge"},
		{ID: "ko-KR-SunHiNeural", Name: "SunHi", Language: "ko-KR", LanguageTag: "ko-KR", Gender: GenderFemale, Provider: "edge"},
	}, nil
}
