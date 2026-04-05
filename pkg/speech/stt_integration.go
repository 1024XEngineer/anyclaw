package speech

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

type STTIntegration struct {
	mu       sync.RWMutex
	pipeline *STTPipeline
	config   STTIntegrationConfig
	hooks    []STTIntegrationHook
}

type STTIntegrationConfig struct {
	Enabled          bool
	AutoSTT          bool
	TriggerPrefix    string
	Provider         string
	DefaultLang      string
	MaxDuration      time.Duration
	MinConfidence    float64
	Timeout          time.Duration
	Channels         map[string]bool
	ExcludeChannels  map[string]bool
	FallbackToVoice  bool
	AppendTranscript bool
}

func DefaultSTTIntegrationConfig() STTIntegrationConfig {
	return STTIntegrationConfig{
		Enabled:          true,
		AutoSTT:          true,
		TriggerPrefix:    "/transcribe",
		Provider:         "",
		DefaultLang:      "auto",
		MaxDuration:      10 * time.Minute,
		MinConfidence:    0.0,
		Timeout:          120 * time.Second,
		Channels:         map[string]bool{},
		ExcludeChannels:  map[string]bool{},
		FallbackToVoice:  false,
		AppendTranscript: true,
	}
}

type STTIntegrationHook interface {
	OnBeforeSTT(ctx context.Context, req *STTTranscribeRequest) error
	OnAfterSTT(ctx context.Context, result *TranscriptResult) error
}

func NewSTTIntegration(pipeline *STTPipeline, cfg STTIntegrationConfig) *STTIntegration {
	return &STTIntegration{
		pipeline: pipeline,
		config:   cfg,
	}
}

func (i *STTIntegration) RegisterHook(hook STTIntegrationHook) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.hooks = append(i.hooks, hook)
}

type StreamChunkHandler func(ctx context.Context, sessionID string, message string, meta map[string]string, onChunk func(chunk string) error) (string, error)

func (i *STTIntegration) WrapStreamInboundHandler(handler StreamChunkHandler) StreamChunkHandler {
	return func(ctx context.Context, sessionID string, message string, meta map[string]string, onChunk func(chunk string) error) (string, error) {
		i.mu.RLock()
		enabled := i.config.Enabled
		autoSTT := i.config.AutoSTT
		triggerPrefix := i.config.TriggerPrefix
		channels := i.config.Channels
		excludeChannels := i.config.ExcludeChannels
		i.mu.RUnlock()

		if !enabled {
			return handler(ctx, sessionID, message, meta, onChunk)
		}

		channel := ""
		if meta != nil {
			if ch, ok := meta["channel"]; ok {
				channel = ch
			}
		}

		if len(channels) > 0 && !channels[channel] {
			return handler(ctx, sessionID, message, meta, onChunk)
		}

		if excludeChannels[channel] {
			return handler(ctx, sessionID, message, meta, onChunk)
		}

		messageType := ""
		if meta != nil {
			if mt, ok := meta["message_type"]; ok {
				messageType = mt
			}
		}

		isVoiceMessage := messageType == "voice" || messageType == "audio" || messageType == "voice_note" || messageType == "audio_file"

		if !isVoiceMessage && strings.HasPrefix(message, triggerPrefix) {
			cleanText := strings.TrimSpace(message[len(triggerPrefix):])
			if cleanText == "" {
				return handler(ctx, sessionID, message, meta, onChunk)
			}
			message = cleanText
			return handler(ctx, sessionID, message, meta, onChunk)
		}

		if !isVoiceMessage && !autoSTT {
			return handler(ctx, sessionID, message, meta, onChunk)
		}

		audioURL := ""
		audioMIME := ""
		sender := ""
		if meta != nil {
			if url, ok := meta["audio_url"]; ok {
				audioURL = url
			}
			if mime, ok := meta["audio_mime"]; ok {
				audioMIME = mime
			}
			if s, ok := meta["sender"]; ok {
				sender = s
			}
		}

		if audioURL == "" && message != "" {
			audioURL = message
			message = ""
		}

		if audioURL == "" {
			if i.config.FallbackToVoice {
				meta["stt_error"] = "no audio data"
			}
			return handler(ctx, sessionID, message, meta, onChunk)
		}

		req := &STTTranscribeRequest{
			AudioURL:  audioURL,
			AudioMIME: audioMIME,
			Channel:   channel,
			Sender:    sender,
			Language:  i.config.DefaultLang,
			Provider:  i.config.Provider,
			Metadata:  map[string]any{},
		}

		result, err := i.pipeline.Transcribe(ctx, req)
		if err != nil {
			if i.config.FallbackToVoice {
				meta["stt_error"] = err.Error()
			}
			return handler(ctx, sessionID, message, meta, onChunk)
		}

		transcribedText := result.Text
		if transcribedText == "" {
			return handler(ctx, sessionID, message, meta, onChunk)
		}

		if i.config.AppendTranscript && message != "" {
			transcribedText = message + "\n\n[Transcript]: " + transcribedText
		}

		for _, hook := range i.hooks {
			_ = hook.OnAfterSTT(ctx, &TranscriptResult{Text: transcribedText})
		}

		return handler(ctx, sessionID, transcribedText, meta, onChunk)
	}
}

func (i *STTIntegration) WrapInboundHandler(handler InboundHandler) InboundHandler {
	return func(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error) {
		i.mu.RLock()
		enabled := i.config.Enabled
		autoSTT := i.config.AutoSTT
		triggerPrefix := i.config.TriggerPrefix
		channels := i.config.Channels
		excludeChannels := i.config.ExcludeChannels
		i.mu.RUnlock()

		if !enabled {
			return handler(ctx, sessionID, message, meta)
		}

		channel := ""
		if meta != nil {
			if ch, ok := meta["channel"]; ok {
				channel = ch
			}
		}

		if len(channels) > 0 && !channels[channel] {
			return handler(ctx, sessionID, message, meta)
		}

		if excludeChannels[channel] {
			return handler(ctx, sessionID, message, meta)
		}

		messageType := ""
		if meta != nil {
			if mt, ok := meta["message_type"]; ok {
				messageType = mt
			}
		}

		isVoiceMessage := messageType == "voice" || messageType == "audio" || messageType == "voice_note" || messageType == "audio_file"

		if !isVoiceMessage && !strings.HasPrefix(message, triggerPrefix) && !autoSTT {
			return handler(ctx, sessionID, message, meta)
		}

		if !isVoiceMessage && strings.HasPrefix(message, triggerPrefix) {
			cleanText := strings.TrimSpace(message[len(triggerPrefix):])
			if cleanText == "" {
				return handler(ctx, sessionID, message, meta)
			}
			message = cleanText
			return handler(ctx, sessionID, message, meta)
		}

		if !isVoiceMessage {
			return handler(ctx, sessionID, message, meta)
		}

		audioURL := ""
		audioMIME := ""
		sender := ""
		if meta != nil {
			if url, ok := meta["audio_url"]; ok {
				audioURL = url
			}
			if mime, ok := meta["audio_mime"]; ok {
				audioMIME = mime
			}
			if s, ok := meta["sender"]; ok {
				sender = s
			}
		}

		if audioURL == "" && message != "" {
			audioURL = message
			message = ""
		}

		if audioURL == "" {
			if i.config.FallbackToVoice {
				meta["stt_error"] = "no audio data"
			}
			return handler(ctx, sessionID, message, meta)
		}

		req := &STTTranscribeRequest{
			AudioURL:  audioURL,
			AudioMIME: audioMIME,
			Channel:   channel,
			Sender:    sender,
			Language:  i.config.DefaultLang,
			Provider:  i.config.Provider,
			Metadata:  map[string]any{},
		}

		for k, v := range meta {
			req.Metadata[k] = v
		}

		for _, hook := range i.hooks {
			if err := hook.OnBeforeSTT(ctx, req); err != nil {
				if i.config.FallbackToVoice {
					meta["stt_error"] = err.Error()
					return handler(ctx, sessionID, message, meta)
				}
				return sessionID, "", fmt.Errorf("stt-integration: before hook failed: %w", err)
			}
		}

		result, err := i.pipeline.Transcribe(ctx, req)
		if err != nil {
			if i.config.FallbackToVoice {
				meta["stt_error"] = err.Error()
				return handler(ctx, sessionID, message, meta)
			}
			return sessionID, "", fmt.Errorf("stt-integration: transcription failed: %w", err)
		}

		for _, hook := range i.hooks {
			if err := hook.OnAfterSTT(ctx, result); err != nil {
				return sessionID, "", fmt.Errorf("stt-integration: after hook failed: %w", err)
			}
		}

		transcribedText := result.Text
		if transcribedText == "" {
			if i.config.FallbackToVoice {
				return handler(ctx, sessionID, message, meta)
			}
			return sessionID, "", fmt.Errorf("stt-integration: empty transcription result")
		}

		meta["stt_language"] = result.Language
		meta["stt_confidence"] = fmt.Sprintf("%.2f", result.Confidence)
		meta["stt_duration"] = result.Duration.String()
		meta["message_type"] = "text"

		if i.config.AppendTranscript && message != "" {
			transcribedText = message + " " + transcribedText
		}

		return handler(ctx, sessionID, transcribedText, meta)
	}
}

func (i *STTIntegration) ProcessVoiceMessage(ctx context.Context, channel string, sender string, audioURL string, audioMIME string, metadata map[string]string) (*TranscriptResult, error) {
	i.mu.RLock()
	provider := i.config.Provider
	defaultLang := i.config.DefaultLang
	enabled := i.config.Enabled
	i.mu.RUnlock()

	if !enabled {
		return nil, fmt.Errorf("stt-integration: integration is disabled")
	}

	req := &STTTranscribeRequest{
		AudioURL:  audioURL,
		AudioMIME: audioMIME,
		Channel:   channel,
		Sender:    sender,
		Language:  defaultLang,
		Provider:  provider,
		Metadata:  make(map[string]any),
	}

	for k, v := range metadata {
		req.Metadata[k] = v
	}

	for _, hook := range i.hooks {
		if err := hook.OnBeforeSTT(ctx, req); err != nil {
			return nil, fmt.Errorf("stt-integration: before hook failed: %w", err)
		}
	}

	result, err := i.pipeline.Transcribe(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("stt-integration: transcription failed: %w", err)
	}

	for _, hook := range i.hooks {
		if err := hook.OnAfterSTT(ctx, result); err != nil {
			return nil, fmt.Errorf("stt-integration: after hook failed: %w", err)
		}
	}

	return result, nil
}

func (i *STTIntegration) ProcessVoiceData(ctx context.Context, channel string, sender string, audioData []byte, audioMIME string, metadata map[string]string) (*TranscriptResult, error) {
	i.mu.RLock()
	provider := i.config.Provider
	defaultLang := i.config.DefaultLang
	enabled := i.config.Enabled
	i.mu.RUnlock()

	if !enabled {
		return nil, fmt.Errorf("stt-integration: integration is disabled")
	}

	req := &STTTranscribeRequest{
		AudioData: audioData,
		AudioMIME: audioMIME,
		Channel:   channel,
		Sender:    sender,
		Language:  defaultLang,
		Provider:  provider,
		Metadata:  make(map[string]any),
	}

	for k, v := range metadata {
		req.Metadata[k] = v
	}

	for _, hook := range i.hooks {
		if err := hook.OnBeforeSTT(ctx, req); err != nil {
			return nil, fmt.Errorf("stt-integration: before hook failed: %w", err)
		}
	}

	result, err := i.pipeline.Transcribe(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("stt-integration: transcription failed: %w", err)
	}

	for _, hook := range i.hooks {
		if err := hook.OnAfterSTT(ctx, result); err != nil {
			return nil, fmt.Errorf("stt-integration: after hook failed: %w", err)
		}
	}

	return result, nil
}

func (i *STTIntegration) UpdateConfig(cfg STTIntegrationConfig) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.config = cfg
}

func (i *STTIntegration) Config() STTIntegrationConfig {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.config
}

func (i *STTIntegration) EnableAutoSTT() {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.config.AutoSTT = true
}

func (i *STTIntegration) DisableAutoSTT() {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.config.AutoSTT = false
}

func (i *STTIntegration) SetTriggerPrefix(prefix string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.config.TriggerPrefix = prefix
}

func (i *STTIntegration) AllowChannel(channel string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.config.Channels == nil {
		i.config.Channels = make(map[string]bool)
	}
	i.config.Channels[channel] = true
}

func (i *STTIntegration) ExcludeChannel(channel string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.config.ExcludeChannels == nil {
		i.config.ExcludeChannels = make(map[string]bool)
	}
	i.config.ExcludeChannels[channel] = true
}

type STTTranscriptResult struct {
	Text       string  `json:"text"`
	Language   string  `json:"language"`
	Duration   string  `json:"duration"`
	Confidence float64 `json:"confidence"`
	Words      []Word  `json:"words,omitempty"`
}

type Word struct {
	Text       string  `json:"text"`
	StartTime  string  `json:"start_time"`
	EndTime    string  `json:"end_time"`
	Confidence float64 `json:"confidence"`
}

func TranscriptToJSON(result *TranscriptResult) STTTranscriptResult {
	out := STTTranscriptResult{
		Text:       result.Text,
		Language:   result.Language,
		Duration:   result.Duration.String(),
		Confidence: result.Confidence,
	}

	if len(result.Words) > 0 {
		out.Words = make([]Word, 0, len(result.Words))
		for _, w := range result.Words {
			out.Words = append(out.Words, Word{
				Text:       w.Word,
				StartTime:  w.StartTime.String(),
				EndTime:    w.EndTime.String(),
				Confidence: w.Confidence,
			})
		}
	}

	return out
}

func TranscriptToJSONString(result *TranscriptResult) (string, error) {
	out := TranscriptToJSON(result)
	data, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("stt-integration: failed to marshal transcript: %w", err)
	}
	return string(data), nil
}
