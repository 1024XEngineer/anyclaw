package speech

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type OpenAIModel string

const (
	OpenAITTS1   OpenAIModel = "tts-1"
	OpenAITTS1HD OpenAIModel = "tts-1-hd"
)

type OpenAIProvider struct {
	apiKey  string
	baseURL string
	voice   string
	model   OpenAIModel
	timeout time.Duration
	retries int
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

func WithOpenAIModel(model OpenAIModel) OpenAIOption {
	return func(p *OpenAIProvider) {
		p.model = model
	}
}

func WithOpenAITimeout(timeout time.Duration) OpenAIOption {
	return func(p *OpenAIProvider) {
		p.timeout = timeout
	}
}

func WithOpenAIRetries(retries int) OpenAIOption {
	return func(p *OpenAIProvider) {
		p.retries = retries
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
		model:   OpenAITTS1,
		timeout: 60 * time.Second,
		retries: 2,
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
		"model":  string(p.model),
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

	var lastErr error
	for attempt := 0; attempt <= p.retries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(attempt) * 500 * time.Millisecond
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("openai: context cancelled during retry: %w", ctx.Err())
			case <-time.After(backoff):
			}
		}

		result, err := p.doSynthesize(ctx, url, body, options)
		if err == nil {
			return result, nil
		}

		lastErr = err
	}

	return nil, fmt.Errorf("openai: all %d retries failed, last error: %w", p.retries, lastErr)
}

func (p *OpenAIProvider) doSynthesize(ctx context.Context, url string, body []byte, options SynthesizeOptions) (*AudioResult, error) {
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
		{ID: "alloy", Name: "Alloy", Language: "en", LanguageTag: "en-US", Gender: GenderNeutral, Provider: "openai", Description: "Balanced and versatile voice"},
		{ID: "echo", Name: "Echo", Language: "en", LanguageTag: "en-US", Gender: GenderMale, Provider: "openai", Description: "Warm and friendly voice"},
		{ID: "fable", Name: "Fable", Language: "en", LanguageTag: "en-GB", Gender: GenderNeutral, Provider: "openai", Description: "British accent, storytelling voice"},
		{ID: "onyx", Name: "Onyx", Language: "en", LanguageTag: "en-US", Gender: GenderMale, Provider: "openai", Description: "Deep and authoritative voice"},
		{ID: "nova", Name: "Nova", Language: "en", LanguageTag: "en-US", Gender: GenderFemale, Provider: "openai", Description: "Clear and professional voice"},
		{ID: "shimmer", Name: "Shimmer", Language: "en", LanguageTag: "en-US", Gender: GenderFemale, Provider: "openai", Description: "Light and cheerful voice"},
	}, nil
}

func (p *OpenAIProvider) GetVoice(ctx context.Context, voiceID string) (*Voice, error) {
	voices, err := p.ListVoices(ctx)
	if err != nil {
		return nil, err
	}

	for _, v := range voices {
		if v.ID == voiceID {
			return &v, nil
		}
	}

	return nil, fmt.Errorf("openai: voice not found: %s", voiceID)
}

func (p *OpenAIProvider) ValidateVoice(voiceID string) error {
	validVoices := map[string]bool{
		"alloy":   true,
		"echo":    true,
		"fable":   true,
		"onyx":    true,
		"nova":    true,
		"shimmer": true,
	}

	if !validVoices[voiceID] {
		return fmt.Errorf("openai: invalid voice: %s", voiceID)
	}

	return nil
}

func (p *OpenAIProvider) SynthesizeStream(ctx context.Context, text string, opts ...SynthesizeOption) (chan []byte, error) {
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
		"model":  string(p.model),
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

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("openai: API error (%d): %s", resp.StatusCode, string(respBody))
	}

	ch := make(chan []byte, 16)

	go func() {
		defer close(ch)
		defer resp.Body.Close()

		buf := make([]byte, 4096)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			n, err := resp.Body.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				ch <- chunk
			}
			if err != nil {
				if err != io.EOF {
					return
				}
				return
			}
		}
	}()

	return ch, nil
}

func (p *OpenAIProvider) SetModel(model OpenAIModel) {
	p.model = model
}

func (p *OpenAIProvider) SetVoice(voice string) {
	p.voice = voice
}

type ElevenLabsModel string

const (
	ElevenLabsMultilingualV2    ElevenLabsModel = "eleven_multilingual_v2"
	ElevenLabsTurboV2           ElevenLabsModel = "eleven_turbo_v2"
	ElevenLabsTurboV25          ElevenLabsModel = "eleven_turbo_v2_5"
	ElevenLabsMultilingualSTSV1 ElevenLabsModel = "eleven_multilingual_sts_v1"
	ElevenLabsTurboSTSV1        ElevenLabsModel = "eleven_turbo_sts_v1"
)

type ElevenLabsVoiceSettings struct {
	Stability       float64
	SimilarityBoost float64
	Style           float64
	UseSpeakerBoost bool
}

type ElevenLabsProvider struct {
	apiKey        string
	baseURL       string
	voice         string
	model         ElevenLabsModel
	voiceSettings ElevenLabsVoiceSettings
	timeout       time.Duration
	retries       int
	client        *http.Client
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

func WithElevenLabsModel(model ElevenLabsModel) ElevenLabsOption {
	return func(p *ElevenLabsProvider) {
		p.model = model
	}
}

func WithElevenLabsVoiceSettings(settings ElevenLabsVoiceSettings) ElevenLabsOption {
	return func(p *ElevenLabsProvider) {
		p.voiceSettings = settings
	}
}

func WithElevenLabsTimeout(timeout time.Duration) ElevenLabsOption {
	return func(p *ElevenLabsProvider) {
		p.timeout = timeout
	}
}

func WithElevenLabsRetries(retries int) ElevenLabsOption {
	return func(p *ElevenLabsProvider) {
		p.retries = retries
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
		model:   ElevenLabsMultilingualV2,
		voiceSettings: ElevenLabsVoiceSettings{
			Stability:       0.5,
			SimilarityBoost: 0.75,
			Style:           0.0,
			UseSpeakerBoost: true,
		},
		timeout: 60 * time.Second,
		retries: 2,
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
		"model_id": string(p.model),
		"voice_settings": map[string]any{
			"stability":         p.voiceSettings.Stability,
			"similarity_boost":  p.voiceSettings.SimilarityBoost,
			"style":             p.voiceSettings.Style,
			"use_speaker_boost": p.voiceSettings.UseSpeakerBoost,
		},
	}

	if options.Format == FormatPCM {
		payload["output_format"] = "pcm_16000"
	} else if options.Format == FormatWAV {
		payload["output_format"] = "wav_16000"
	} else if options.Format == FormatOGG {
		payload["output_format"] = "ogg_16000"
	} else if options.Format == FormatFLAC {
		payload["output_format"] = "flac_16000"
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs: failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/text-to-speech/%s", p.baseURL, options.Voice)

	var lastErr error
	for attempt := 0; attempt <= p.retries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(attempt) * 500 * time.Millisecond
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("elevenlabs: context cancelled during retry: %w", ctx.Err())
			case <-time.After(backoff):
			}
		}

		result, err := p.doSynthesize(ctx, url, body, options)
		if err == nil {
			return result, nil
		}

		lastErr = err
	}

	return nil, fmt.Errorf("elevenlabs: all %d retries failed, last error: %w", p.retries, lastErr)
}

func (p *ElevenLabsProvider) doSynthesize(ctx context.Context, url string, body []byte, options SynthesizeOptions) (*AudioResult, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("elevenlabs: failed to create request: %w", err)
	}
	req.Header.Set("xi-api-key", p.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "audio/mpeg")

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

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "audio/mpeg"
	}

	return &AudioResult{
		Data:        audioData,
		Format:      options.Format,
		ContentType: contentType,
	}, nil
}

func (p *ElevenLabsProvider) SynthesizeStream(ctx context.Context, text string, opts ...SynthesizeOption) (chan []byte, error) {
	options := SynthesizeOptions{
		Voice:  p.voice,
		Format: FormatMP3,
	}
	for _, opt := range opts {
		opt(&options)
	}

	payload := map[string]any{
		"text":     text,
		"model_id": string(p.model),
		"voice_settings": map[string]any{
			"stability":         p.voiceSettings.Stability,
			"similarity_boost":  p.voiceSettings.SimilarityBoost,
			"style":             p.voiceSettings.Style,
			"use_speaker_boost": p.voiceSettings.UseSpeakerBoost,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs: failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/text-to-speech/%s/stream", p.baseURL, options.Voice)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("elevenlabs: failed to create request: %w", err)
	}
	req.Header.Set("xi-api-key", p.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "audio/mpeg")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs: request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("elevenlabs: API error (%d): %s", resp.StatusCode, string(respBody))
	}

	ch := make(chan []byte, 16)

	go func() {
		defer close(ch)
		defer resp.Body.Close()

		buf := make([]byte, 4096)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			n, err := resp.Body.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				ch <- chunk
			}
			if err != nil {
				if err != io.EOF {
					return
				}
				return
			}
		}
	}()

	return ch, nil
}

func (p *ElevenLabsProvider) ListVoices(ctx context.Context) ([]Voice, error) {
	_ = ctx
	return []Voice{
		{ID: "21m00Tcm4TlvDq8ikWAM", Name: "Rachel", Language: "en", LanguageTag: "en-US", Gender: GenderFemale, Provider: "elevenlabs", Description: "Calm and well-balanced voice"},
		{ID: "EXAVITQu4vr4xnSDxMaL", Name: "Bella", Language: "en", LanguageTag: "en-US", Gender: GenderFemale, Provider: "elevenlabs", Description: "Soft and gentle voice"},
		{ID: "ErXwobaYiN019PkySvjV", Name: "Antoni", Language: "en", LanguageTag: "en-US", Gender: GenderMale, Provider: "elevenlabs", Description: "Well-balanced and moderately deep voice"},
		{ID: "VR6AewLTigWG4xSOukaG", Name: "Arnold", Language: "en", LanguageTag: "en-US", Gender: GenderMale, Provider: "elevenlabs", Description: "Deep and authoritative voice"},
		{ID: "pNInz6obpgDQGcFmaJgB", Name: "Adam", Language: "en", LanguageTag: "en-US", Gender: GenderMale, Provider: "elevenlabs", Description: "Deep and resonant voice"},
		{ID: "yoZ06aMxZJJ28mfd3POQ", Name: "Sam", Language: "en", LanguageTag: "en-US", Gender: GenderMale, Provider: "elevenlabs", Description: "Calm and natural voice"},
		{ID: "jBpfuIE2acCO8z3wKNLl", Name: "Gigi", Language: "en", LanguageTag: "en-US", Gender: GenderFemale, Provider: "elevenlabs", Description: "Child-like and cute voice"},
	}, nil
}

func (p *ElevenLabsProvider) GetVoice(ctx context.Context, voiceID string) (*Voice, error) {
	voices, err := p.ListVoices(ctx)
	if err != nil {
		return nil, err
	}

	for _, v := range voices {
		if v.ID == voiceID {
			return &v, nil
		}
	}

	return nil, fmt.Errorf("elevenlabs: voice not found: %s", voiceID)
}

func (p *ElevenLabsProvider) SetModel(model ElevenLabsModel) {
	p.model = model
}

func (p *ElevenLabsProvider) SetVoice(voice string) {
	p.voice = voice
}

func (p *ElevenLabsProvider) SetVoiceSettings(settings ElevenLabsVoiceSettings) {
	p.voiceSettings = settings
}

func (p *ElevenLabsProvider) GetUsage(ctx context.Context) (int, int, error) {
	_ = ctx
	url := p.baseURL + "/user"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("elevenlabs: failed to create request: %w", err)
	}
	req.Header.Set("xi-api-key", p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return 0, 0, fmt.Errorf("elevenlabs: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return 0, 0, fmt.Errorf("elevenlabs: API error (%d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Subscription struct {
			CharacterCount int `json:"character_count"`
			CharacterLimit int `json:"character_limit"`
		} `json:"subscription"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, 0, fmt.Errorf("elevenlabs: failed to decode response: %w", err)
	}

	return result.Subscription.CharacterCount, result.Subscription.CharacterLimit, nil
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

type PiperQuality string

const (
	PiperQualityXLow   PiperQuality = "x_low"
	PiperQualityLow    PiperQuality = "low"
	PiperQualityMedium PiperQuality = "medium"
	PiperQualityHigh   PiperQuality = "high"
)

type PiperProvider struct {
	modelPath       string
	modelFile       string
	configFile      string
	voiceDir        string
	pythonCmd       string
	useGPU          bool
	volume          float64
	sentenceSilence float64
	defaultVoice    string
	defaultLang     string
	sampleRate      int
}

type PiperOption func(*PiperProvider)

func WithPiperModelPath(path string) PiperOption {
	return func(p *PiperProvider) {
		p.modelPath = path
	}
}

func WithPiperVoiceDir(dir string) PiperOption {
	return func(p *PiperProvider) {
		p.voiceDir = dir
	}
}

func WithPiperDefaultVoice(voice string) PiperOption {
	return func(p *PiperProvider) {
		p.defaultVoice = voice
	}
}

func WithPiperDefaultLanguage(lang string) PiperOption {
	return func(p *PiperProvider) {
		p.defaultLang = lang
	}
}

func WithPiperUseGPU(use bool) PiperOption {
	return func(p *PiperProvider) {
		p.useGPU = use
	}
}

func WithPiperVolume(vol float64) PiperOption {
	return func(p *PiperProvider) {
		p.volume = vol
	}
}

func WithPiperSentenceSilence(seconds float64) PiperOption {
	return func(p *PiperProvider) {
		p.sentenceSilence = seconds
	}
}

func WithPiperPythonCmd(cmd string) PiperOption {
	return func(p *PiperProvider) {
		p.pythonCmd = cmd
	}
}

func NewPiperProvider(opts ...PiperOption) (*PiperProvider, error) {
	p := &PiperProvider{
		voiceDir:        "",
		pythonCmd:       "python3",
		useGPU:          false,
		volume:          1.0,
		sentenceSilence: 0.0,
		defaultVoice:    "en_US-lessac-medium",
		defaultLang:     "en-US",
		sampleRate:      22050,
	}

	for _, opt := range opts {
		opt(p)
	}

	if p.voiceDir == "" {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			p.voiceDir = filepath.Join(homeDir, ".local", "share", "piper-voices")
		} else {
			p.voiceDir = filepath.Join("data", "piper-voices")
		}
	}

	if err := p.findModel(); err != nil {
		return nil, fmt.Errorf("piper: %w", err)
	}

	return p, nil
}

func (p *PiperProvider) findModel() error {
	voice := p.defaultVoice
	if !strings.HasSuffix(voice, ".onnx") {
		voice = voice + ".onnx"
	}

	modelPath := filepath.Join(p.voiceDir, voice)
	if _, err := os.Stat(modelPath); err == nil {
		p.modelPath = modelPath
		p.modelFile = voice
		configFile := strings.TrimSuffix(modelPath, ".onnx") + ".onnx.json"
		if _, err := os.Stat(configFile); err == nil {
			p.configFile = configFile
		}
		return nil
	}

	if _, err := os.Stat(p.defaultVoice); err == nil {
		p.modelPath = p.defaultVoice
		p.modelFile = filepath.Base(p.defaultVoice)
		configFile := strings.TrimSuffix(p.defaultVoice, ".onnx") + ".onnx.json"
		if _, err := os.Stat(configFile); err == nil {
			p.configFile = configFile
		}
		return nil
	}

	return fmt.Errorf("model not found: %s (searched in %s)", p.defaultVoice, p.voiceDir)
}

func (p *PiperProvider) Name() string {
	return "piper"
}

func (p *PiperProvider) Type() ProviderType {
	return ProviderType("piper")
}

func (p *PiperProvider) Synthesize(ctx context.Context, text string, opts ...SynthesizeOption) (*AudioResult, error) {
	options := SynthesizeOptions{
		Voice:      p.defaultVoice,
		Format:     FormatWAV,
		SampleRate: p.sampleRate,
		Volume:     p.volume,
	}
	for _, opt := range opts {
		opt(&options)
	}

	modelPath := p.modelPath
	if options.Voice != "" && options.Voice != p.defaultVoice {
		voiceFile := options.Voice
		if !strings.HasSuffix(voiceFile, ".onnx") {
			voiceFile = voiceFile + ".onnx"
		}

		candidate := filepath.Join(p.voiceDir, voiceFile)
		if _, err := os.Stat(candidate); err == nil {
			modelPath = candidate
		} else if _, err := os.Stat(options.Voice); err == nil {
			modelPath = options.Voice
		}
	}

	tmpFile, err := os.CreateTemp("", "piper-*.wav")
	if err != nil {
		return nil, fmt.Errorf("piper: failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	args := []string{"-m", "piper", "-m", modelPath, "-f", tmpPath}

	if p.useGPU {
		args = append(args, "--cuda")
	}

	if options.Volume > 0 && options.Volume != 1.0 {
		args = append(args, "--volume", fmt.Sprintf("%.2f", options.Volume))
	} else if p.volume > 0 && p.volume != 1.0 {
		args = append(args, "--volume", fmt.Sprintf("%.2f", p.volume))
	}

	if p.sentenceSilence > 0 {
		args = append(args, "--sentence-silence", fmt.Sprintf("%.2f", p.sentenceSilence))
	}

	args = append(args, "--")
	args = append(args, text)

	cmd := exec.CommandContext(ctx, p.pythonCmd, args...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("piper: synthesis failed: %w, stderr: %s", err, stderr.String())
	}

	audioData, err := os.ReadFile(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("piper: failed to read output file: %w", err)
	}

	if len(audioData) == 0 {
		return nil, fmt.Errorf("piper: synthesis produced no audio data")
	}

	sampleRate := options.SampleRate
	if sampleRate == 0 {
		sampleRate = p.sampleRate
	}

	return &AudioResult{
		Data:        audioData,
		Format:      FormatWAV,
		SampleRate:  sampleRate,
		Channels:    1,
		BitDepth:    16,
		ContentType: "audio/wav",
	}, nil
}

func (p *PiperProvider) ListVoices(ctx context.Context) ([]Voice, error) {
	_ = ctx

	var voices []Voice

	entries, err := os.ReadDir(p.voiceDir)
	if err != nil {
		return voices, nil
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".onnx") {
			continue
		}

		voiceID := strings.TrimSuffix(name, ".onnx")
		parts := strings.SplitN(voiceID, "-", 3)
		if len(parts) < 2 {
			continue
		}

		langCode := parts[0] + "_" + parts[1]
		voiceName := parts[len(parts)-1]

		quality := PiperQualityMedium
		if strings.Contains(voiceID, "x_low") {
			quality = PiperQualityXLow
		} else if strings.Contains(voiceID, "_low") {
			quality = PiperQualityLow
		} else if strings.Contains(voiceID, "_high") {
			quality = PiperQualityHigh
		}

		voices = append(voices, Voice{
			ID:          voiceID,
			Name:        voiceName,
			Language:    langCode,
			LanguageTag: strings.Replace(langCode, "_", "-", 1),
			Gender:      GenderNeutral,
			Provider:    "piper",
			Description: fmt.Sprintf("Piper %s quality voice", quality),
		})
	}

	if len(voices) == 0 {
		voices = []Voice{
			{ID: "en_US-lessac-medium", Name: "Lessac", Language: "en_US", LanguageTag: "en-US", Gender: GenderFemale, Provider: "piper", Description: "Default English voice"},
			{ID: "zh_CN-huayan-medium", Name: "Huayan", Language: "zh_CN", LanguageTag: "zh-CN", Gender: GenderNeutral, Provider: "piper", Description: "Chinese voice"},
			{ID: "ja_JP-tomoko-medium", Name: "Tomoko", Language: "ja_JP", LanguageTag: "ja-JP", Gender: GenderFemale, Provider: "piper", Description: "Japanese voice"},
		}
	}

	return voices, nil
}

func (p *PiperProvider) SetVoice(voice string) {
	p.defaultVoice = voice
	if err := p.findModel(); err == nil {
		p.defaultVoice = voice
	}
}

func (p *PiperProvider) SetVoiceDir(dir string) {
	p.voiceDir = dir
}

type CoquiModel string

const (
	CoquiModelTacotron2    CoquiModel = "tacotron2"
	CoquiModelVits         CoquiModel = "vits"
	CoquiModelXTTSv2       CoquiModel = "xtts_v2"
	CoquiModelYourTTS      CoquiModel = "your_tts"
	CoquiModelGlowTTS      CoquiModel = "glow_tts"
	CoquiModelSpeedySpeech CoquiModel = "speedy_speech"
)

type CoquiProvider struct {
	modelName    CoquiModel
	speakerName  string
	language     string
	useGPU       bool
	pythonCmd    string
	voiceDir     string
	defaultLang  string
	outputFormat string
}

type CoquiOption func(*CoquiProvider)

func WithCoquiModel(model CoquiModel) CoquiOption {
	return func(p *CoquiProvider) {
		p.modelName = model
	}
}

func WithCoquiSpeaker(speaker string) CoquiOption {
	return func(p *CoquiProvider) {
		p.speakerName = speaker
	}
}

func WithCoquiLanguage(lang string) CoquiOption {
	return func(p *CoquiProvider) {
		p.language = lang
	}
}

func WithCoquiDefaultLanguage(lang string) CoquiOption {
	return func(p *CoquiProvider) {
		p.defaultLang = lang
	}
}

func WithCoquiUseGPU(use bool) CoquiOption {
	return func(p *CoquiProvider) {
		p.useGPU = use
	}
}

func WithCoquiPythonCmd(cmd string) CoquiOption {
	return func(p *CoquiProvider) {
		p.pythonCmd = cmd
	}
}

func WithCoquiVoiceDir(dir string) CoquiOption {
	return func(p *CoquiProvider) {
		p.voiceDir = dir
	}
}

func WithCoquiOutputFormat(format string) CoquiOption {
	return func(p *CoquiProvider) {
		p.outputFormat = format
	}
}

func NewCoquiProvider(opts ...CoquiOption) (*CoquiProvider, error) {
	p := &CoquiProvider{
		modelName:    CoquiModelVits,
		speakerName:  "",
		language:     "",
		useGPU:       false,
		pythonCmd:    "python3",
		voiceDir:     "",
		defaultLang:  "en",
		outputFormat: "wav",
	}

	for _, opt := range opts {
		opt(p)
	}

	if p.voiceDir == "" {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			p.voiceDir = filepath.Join(homeDir, ".local", "share", "tts")
		} else {
			p.voiceDir = filepath.Join("data", "tts")
		}
	}

	return p, nil
}

func (p *CoquiProvider) Name() string {
	return "coqui"
}

func (p *CoquiProvider) Type() ProviderType {
	return ProviderType("coqui")
}

func (p *CoquiProvider) Synthesize(ctx context.Context, text string, opts ...SynthesizeOption) (*AudioResult, error) {
	options := SynthesizeOptions{
		Voice:    p.speakerName,
		Format:   FormatWAV,
		Language: p.defaultLang,
	}
	for _, opt := range opts {
		opt(&options)
	}

	tmpFile, err := os.CreateTemp("", "coqui-*.wav")
	if err != nil {
		return nil, fmt.Errorf("coqui: failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	modelStr := string(p.modelName)
	if p.modelName == CoquiModelXTTSv2 {
		modelStr = "tts_models/multilingual/multi-dataset/xtts_v2"
	} else if p.modelName == CoquiModelVits {
		modelStr = "tts_models/" + options.Language + "/vits"
	}

	args := []string{"-m", "TTS", "--model_name", modelStr, "--text", text, "--out_path", tmpPath}

	if options.Voice != "" {
		args = append(args, "--speaker_idx", options.Voice)
	}

	if options.Language != "" {
		args = append(args, "--language_idx", options.Language)
	}

	if p.useGPU {
		args = append(args, "--use_cuda", "true")
	}

	if p.voiceDir != "" {
		args = append(args, "--config_path", filepath.Join(p.voiceDir, "config.json"))
	}

	cmd := exec.CommandContext(ctx, p.pythonCmd, args...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("coqui: synthesis failed: %w, stderr: %s", err, stderr.String())
	}

	audioData, err := os.ReadFile(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("coqui: failed to read output file: %w", err)
	}

	if len(audioData) == 0 {
		return nil, fmt.Errorf("coqui: synthesis produced no audio data")
	}

	return &AudioResult{
		Data:        audioData,
		Format:      FormatWAV,
		SampleRate:  22050,
		Channels:    1,
		BitDepth:    16,
		ContentType: "audio/wav",
	}, nil
}

func (p *CoquiProvider) ListVoices(ctx context.Context) ([]Voice, error) {
	_ = ctx

	switch p.modelName {
	case CoquiModelXTTSv2:
		return []Voice{
			{ID: "Ana Florence", Name: "Ana Florence", Language: "en", LanguageTag: "en", Gender: GenderFemale, Provider: "coqui", Description: "XTTS v2 reference voice"},
			{ID: "Claribel Dervla", Name: "Claribel Dervla", Language: "en", LanguageTag: "en", Gender: GenderFemale, Provider: "coqui", Description: "XTTS v2 reference voice"},
			{ID: "Daisy Studious", Name: "Daisy Studious", Language: "en", LanguageTag: "en", Gender: GenderFemale, Provider: "coqui", Description: "XTTS v2 reference voice"},
			{ID: "Gracie Wise", Name: "Gracie Wise", Language: "en", LanguageTag: "en", Gender: GenderFemale, Provider: "coqui", Description: "XTTS v2 reference voice"},
			{ID: "Tammie Ema", Name: "Tammie Ema", Language: "en", LanguageTag: "en", Gender: GenderFemale, Provider: "coqui", Description: "XTTS v2 reference voice"},
			{ID: "Alison Dietlinde", Name: "Alison Dietlinde", Language: "en", LanguageTag: "en", Gender: GenderFemale, Provider: "coqui", Description: "XTTS v2 reference voice"},
		}, nil
	case CoquiModelVits:
		return []Voice{
			{ID: "vits-en", Name: "VITS English", Language: "en", LanguageTag: "en-US", Gender: GenderNeutral, Provider: "coqui", Description: "VITS English model"},
			{ID: "vits-zh", Name: "VITS Chinese", Language: "zh", LanguageTag: "zh-CN", Gender: GenderNeutral, Provider: "coqui", Description: "VITS Chinese model"},
			{ID: "vits-ja", Name: "VITS Japanese", Language: "ja", LanguageTag: "ja-JP", Gender: GenderNeutral, Provider: "coqui", Description: "VITS Japanese model"},
		}, nil
	default:
		return []Voice{
			{ID: string(p.modelName), Name: string(p.modelName), Language: p.defaultLang, LanguageTag: p.defaultLang, Gender: GenderNeutral, Provider: "coqui", Description: fmt.Sprintf("Coqui %s model", p.modelName)},
		}, nil
	}
}

func (p *CoquiProvider) SetModel(model CoquiModel) {
	p.modelName = model
}

func (p *CoquiProvider) SetSpeaker(speaker string) {
	p.speakerName = speaker
}

func (p *CoquiProvider) SetLanguage(lang string) {
	p.language = lang
}

func detectPythonCmd() string {
	if runtime.GOOS == "windows" {
		if _, err := exec.LookPath("python"); err == nil {
			return "python"
		}
		if _, err := exec.LookPath("python3"); err == nil {
			return "python3"
		}
		return "python"
	}

	if _, err := exec.LookPath("python3"); err == nil {
		return "python3"
	}
	if _, err := exec.LookPath("python"); err == nil {
		return "python"
	}
	return "python3"
}
