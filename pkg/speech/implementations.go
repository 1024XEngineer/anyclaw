package speech

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

// OpenAI TTS implementation with real API calls
func (t *OpenAITTS) SynthesizeReal(ctx context.Context, text string, options TTSOptions) (*AudioResult, error) {
	if t.apiKey == "" {
		return nil, fmt.Errorf("OpenAI API key not configured")
	}

	voice := options.Voice
	if voice == "" {
		voice = "alloy"
	}

	speed := options.Speed
	if speed <= 0 {
		speed = 1.0
	}

	model := "tts-1"
	if options.Engine == "hd" {
		model = "tts-1-hd"
	}

	payload := map[string]any{
		"model":  model,
		"input":  text,
		"voice":  voice,
		"speed":  speed,
		"format": "mp3",
	}
	body, _ := json.Marshal(payload)

	url := t.baseURL + "/v1/audio/speech"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+t.apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("TTS API error (%d): %s", resp.StatusCode, string(respBody))
	}

	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return &AudioResult{
		AudioData:   audioData,
		Format:      "mp3",
		SampleRate:  24000,
		ContentType: "audio/mpeg",
	}, nil
}

// OpenAI STT implementation with real API calls
func (s *OpenAISTT) RecognizeReal(ctx context.Context, audioData []byte, options STTOptions) (*TranscriptionResult, error) {
	if s.apiKey == "" {
		return nil, fmt.Errorf("OpenAI API key not configured")
	}

	// Create multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add file
	part, err := writer.CreateFormFile("file", "audio.wav")
	if err != nil {
		return nil, err
	}
	part.Write(audioData)

	// Add model
	writer.WriteField("model", "whisper-1")

	if options.Language != "" {
		writer.WriteField("language", options.Language)
	}

	writer.Close()

	url := s.baseURL + "/v1/audio/transcriptions"
	req, err := http.NewRequestWithContext(ctx, "POST", url, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("STT API error (%d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	return &TranscriptionResult{
		Text:       result.Text,
		Confidence: 0.95,
		Language:   options.Language,
	}, nil
}

// ElevenLabs TTS implementation
type ElevenLabsTTS struct {
	config  TTSConfig
	apiKey  string
	baseURL string
}

func NewElevenLabsTTS() *ElevenLabsTTS {
	return &ElevenLabsTTS{
		baseURL: "https://api.elevenlabs.io/v1",
	}
}

func (t *ElevenLabsTTS) Name() string { return "elevenlabs-tts" }

func (t *ElevenLabsTTS) Initialize(config TTSConfig) error {
	t.config = config
	t.apiKey = config.APIKey
	if config.Endpoint != "" {
		t.baseURL = config.Endpoint
	}
	return nil
}

func (t *ElevenLabsTTS) Synthesize(ctx context.Context, text string, options TTSOptions) (*AudioResult, error) {
	if t.apiKey == "" {
		return nil, fmt.Errorf("ElevenLabs API key not configured")
	}

	voiceID := options.Voice
	if voiceID == "" {
		voiceID = "21m00Tcm4TlvDq8ikWAM" // Rachel voice
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
	body, _ := json.Marshal(payload)

	url := fmt.Sprintf("%s/text-to-speech/%s", t.baseURL, voiceID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("xi-api-key", t.apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ElevenLabs API error (%d): %s", resp.StatusCode, string(respBody))
	}

	audioData, _ := io.ReadAll(resp.Body)

	return &AudioResult{
		AudioData:   audioData,
		Format:      "mp3",
		ContentType: "audio/mpeg",
	}, nil
}

func (t *ElevenLabsTTS) ListVoices() []Voice {
	return []Voice{
		{ID: "21m00Tcm4TlvDq8ikWAM", Name: "Rachel", Language: "en", Gender: "female", Provider: "elevenlabs"},
		{ID: "EXAVITQu4vr4xnSDxMaL", Name: "Bella", Language: "en", Gender: "female", Provider: "elevenlabs"},
		{ID: "ErXwobaYiN019PkySvjV", Name: "Antoni", Language: "en", Gender: "male", Provider: "elevenlabs"},
		{ID: "VR6AewLTigWG4xSOukaG", Name: "Arnold", Language: "en", Gender: "male", Provider: "elevenlabs"},
		{ID: "pNInz6obpgDQGcFmaJgB", Name: "Adam", Language: "en", Gender: "male", Provider: "elevenlabs"},
	}
}

func (t *ElevenLabsTTS) Close() error {
	return nil
}

// Edge TTS implementation (Microsoft Edge Read Aloud)
type EdgeTTS struct {
	config  TTSConfig
	baseURL string
}

func NewEdgeTTS() *EdgeTTS {
	return &EdgeTTS{
		baseURL: "https://speech.platform.bing.com",
	}
}

func (t *EdgeTTS) Name() string { return "edge-tts" }

func (t *EdgeTTS) Initialize(config TTSConfig) error {
	t.config = config
	if config.Endpoint != "" {
		t.baseURL = config.Endpoint
	}
	return nil
}

func (t *EdgeTTS) Synthesize(ctx context.Context, text string, options TTSOptions) (*AudioResult, error) {
	// Edge TTS uses SSML
	voice := options.Voice
	if voice == "" {
		voice = "en-US-AriaNeural"
	}

	ssml := fmt.Sprintf(`<speak version="1.0" xmlns="http://www.w3.org/2001/10/synthesis" xml:lang="en-US">
		<voice name="%s">%s</voice>
	</speak>`, voice, text)

	req, err := http.NewRequestWithContext(ctx, "POST", t.baseURL+"/synthesize", bytes.NewReader([]byte(ssml)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/ssml+xml")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	audioData, _ := io.ReadAll(resp.Body)

	return &AudioResult{
		AudioData:   audioData,
		Format:      "audio-24khz-48kbitrate-mono-mp3",
		ContentType: "audio/mpeg",
	}, nil
}

func (t *EdgeTTS) ListVoices() []Voice {
	return []Voice{
		{ID: "en-US-AriaNeural", Name: "Aria", Language: "en-US", Gender: "female", Provider: "edge"},
		{ID: "en-US-GuyNeural", Name: "Guy", Language: "en-US", Gender: "male", Provider: "edge"},
		{ID: "zh-CN-XiaoxiaoNeural", Name: "Xiaoxiao", Language: "zh-CN", Gender: "female", Provider: "edge"},
		{ID: "zh-CN-YunxiNeural", Name: "Yunxi", Language: "zh-CN", Gender: "male", Provider: "edge"},
		{ID: "ja-JP-NanamiNeural", Name: "Nanami", Language: "ja-JP", Gender: "female", Provider: "edge"},
		{ID: "ko-KR-SunHiNeural", Name: "SunHi", Language: "ko-KR", Gender: "female", Provider: "edge"},
	}
}

func (t *EdgeTTS) Close() error {
	return nil
}

// SpeechManager provides high-level speech operations
type SpeechManager struct {
	registry   *SpeechProviderRegistry
	defaultTTS string
	defaultSTT string
}

func NewSpeechManager() *SpeechManager {
	registry := NewSpeechProviderRegistry()

	// Register default providers
	registry.RegisterTTS("openai", NewOpenAITTS())
	registry.RegisterTTS("elevenlabs", NewElevenLabsTTS())
	registry.RegisterTTS("edge", NewEdgeTTS())
	registry.RegisterSTT("openai", NewOpenAISTT())

	return &SpeechManager{
		registry:   registry,
		defaultTTS: "openai",
		defaultSTT: "openai",
	}
}

func (sm *SpeechManager) TextToSpeech(ctx context.Context, text string, voice string, provider string) (*AudioResult, error) {
	if provider == "" {
		provider = sm.defaultTTS
	}

	engine, ok := sm.registry.GetTTS(provider)
	if !ok {
		return nil, fmt.Errorf("TTS provider not found: %s", provider)
	}

	options := TTSOptions{
		Voice:  voice,
		Speed:  1.0,
		Engine: provider,
	}

	return engine.Synthesize(ctx, text, options)
}

func (sm *SpeechManager) SpeechToText(ctx context.Context, audioData []byte, language string, provider string) (*TranscriptionResult, error) {
	if provider == "" {
		provider = sm.defaultSTT
	}

	engine, ok := sm.registry.GetSTT(provider)
	if !ok {
		return nil, fmt.Errorf("STT provider not found: %s", provider)
	}

	options := STTOptions{
		Language: language,
	}

	return engine.Recognize(ctx, audioData, options)
}

func (sm *SpeechManager) ListTTSVoices(provider string) []Voice {
	if provider == "" {
		provider = sm.defaultTTS
	}

	engine, ok := sm.registry.GetTTS(provider)
	if !ok {
		return nil
	}
	return engine.ListVoices()
}

func (sm *SpeechManager) ListSTTModels(provider string) []STTModel {
	if provider == "" {
		provider = sm.defaultSTT
	}

	engine, ok := sm.registry.GetSTT(provider)
	if !ok {
		return nil
	}
	return engine.ListModels()
}
