package speech

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

type STTPipeline struct {
	mu            sync.RWMutex
	manager       *STTManager
	provider      string
	defaultLang   string
	autoDetect    bool
	maxDuration   time.Duration
	minConfidence float64
	timeout       time.Duration
	hooks         []STTPipelineHook
	httpClient    *http.Client
}

type STTPipelineConfig struct {
	Provider      string
	DefaultLang   string
	AutoDetect    bool
	MaxDuration   time.Duration
	MinConfidence float64
	Timeout       time.Duration
}

func DefaultSTTPipelineConfig() STTPipelineConfig {
	return STTPipelineConfig{
		Provider:      "",
		DefaultLang:   "auto",
		AutoDetect:    true,
		MaxDuration:   10 * time.Minute,
		MinConfidence: 0.0,
		Timeout:       120 * time.Second,
	}
}

type STTPipelineHook interface {
	OnBeforeTranscribe(ctx context.Context, req *STTTranscribeRequest) error
	OnAfterTranscribe(ctx context.Context, result *TranscriptResult) error
	OnTranscriptionError(ctx context.Context, err error, audioSource string)
}

type STTTranscribeRequest struct {
	AudioData []byte
	AudioURL  string
	AudioMIME string
	Channel   string
	Sender    string
	Language  string
	Provider  string
	Metadata  map[string]any
}

func NewSTTPipeline(manager *STTManager, cfg STTPipelineConfig) *STTPipeline {
	return &STTPipeline{
		manager:       manager,
		provider:      cfg.Provider,
		defaultLang:   cfg.DefaultLang,
		autoDetect:    cfg.AutoDetect,
		maxDuration:   cfg.MaxDuration,
		minConfidence: cfg.MinConfidence,
		timeout:       cfg.Timeout,
		httpClient:    &http.Client{Timeout: cfg.Timeout},
	}
}

func (p *STTPipeline) RegisterHook(hook STTPipelineHook) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.hooks = append(p.hooks, hook)
}

func (p *STTPipeline) Transcribe(ctx context.Context, req *STTTranscribeRequest) (*TranscriptResult, error) {
	p.mu.RLock()
	provider := p.provider
	defaultLang := p.defaultLang
	minConfidence := p.minConfidence
	timeout := p.timeout
	hooks := make([]STTPipelineHook, len(p.hooks))
	copy(hooks, p.hooks)
	p.mu.RUnlock()

	if req.AudioData == nil && req.AudioURL == "" {
		return nil, NewSTTError(ErrAudioFormatInvalid, "stt-pipeline: no audio data or URL provided")
	}

	audioData := req.AudioData
	if audioData == nil && req.AudioURL != "" {
		var err error
		audioData, err = p.downloadAudio(ctx, req.AudioURL)
		if err != nil {
			for _, hook := range hooks {
				hook.OnTranscriptionError(ctx, err, req.AudioURL)
			}
			return nil, NewSTTErrorf(ErrTranscriptionFailed, "stt-pipeline: failed to download audio: %v", err)
		}
	}

	if len(audioData) == 0 {
		return nil, NewSTTError(ErrAudioFormatInvalid, "stt-pipeline: downloaded audio data is empty")
	}

	for _, hook := range hooks {
		if err := hook.OnBeforeTranscribe(ctx, req); err != nil {
			return nil, fmt.Errorf("stt-pipeline: before transcribe hook failed: %w", err)
		}
	}

	transcribeCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		transcribeCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	var opts []TranscribeOption

	lang := req.Language
	if lang == "" {
		lang = defaultLang
	}
	if lang != "" && lang != "auto" {
		opts = append(opts, WithSTTLanguage(lang))
	}

	if req.AudioMIME != "" {
		format := p.mimeToFormat(req.AudioMIME)
		if format != "" {
			opts = append(opts, WithSTTInputFormat(format))
		}
	}

	var result *TranscriptResult
	var err error

	if provider != "" {
		result, err = p.manager.Transcribe(transcribeCtx, audioData, provider, opts...)
	} else {
		result, err = p.manager.Transcribe(transcribeCtx, audioData, "", opts...)
	}

	if err != nil {
		audioSource := req.AudioURL
		if audioSource == "" {
			audioSource = fmt.Sprintf("%d bytes", len(audioData))
		}
		for _, hook := range hooks {
			hook.OnTranscriptionError(ctx, err, audioSource)
		}
		return nil, fmt.Errorf("stt-pipeline: transcription failed: %w", err)
	}

	if minConfidence > 0 && result.Confidence > 0 && result.Confidence < minConfidence {
		return nil, NewSTTErrorf(ErrTranscriptionFailed, "stt-pipeline: confidence %.2f below threshold %.2f", result.Confidence, minConfidence)
	}

	for _, hook := range hooks {
		if err := hook.OnAfterTranscribe(ctx, result); err != nil {
			return nil, fmt.Errorf("stt-pipeline: after transcribe hook failed: %w", err)
		}
	}

	return result, nil
}

func (p *STTPipeline) TranscribeDirect(ctx context.Context, audio []byte, opts ...TranscribeOption) (*TranscriptResult, error) {
	req := &STTTranscribeRequest{
		AudioData: audio,
	}
	return p.Transcribe(ctx, req)
}

func (p *STTPipeline) downloadAudio(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("stt-pipeline: failed to create request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("stt-pipeline: failed to download audio: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("stt-pipeline: download failed with status %d", resp.StatusCode)
	}

	const maxAudioSize = 100 * 1024 * 1024
	if resp.ContentLength > maxAudioSize {
		return nil, fmt.Errorf("stt-pipeline: audio too large: %d bytes", resp.ContentLength)
	}

	audioData := make([]byte, 0, 4096)
	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if len(audioData)+n > maxAudioSize {
				return nil, fmt.Errorf("stt-pipeline: audio exceeds size limit during download")
			}
			audioData = append(audioData, buf[:n]...)
		}
		if err != nil {
			break
		}
	}

	return audioData, nil
}

func (p *STTPipeline) mimeToFormat(mime string) AudioInputFormat {
	mime = strings.ToLower(strings.TrimSpace(mime))
	switch {
	case strings.Contains(mime, "mp3"), strings.Contains(mime, "mpeg"):
		return InputMP3
	case strings.Contains(mime, "wav"):
		return InputWAV
	case strings.Contains(mime, "ogg"):
		return InputOGG
	case strings.Contains(mime, "flac"):
		return InputFLAC
	case strings.Contains(mime, "m4a"):
		return InputM4A
	case strings.Contains(mime, "mp4"):
		return InputMP4
	case strings.Contains(mime, "webm"):
		return InputWEBM
	case strings.Contains(mime, "opus"):
		return InputOGG
	default:
		return ""
	}
}

func (p *STTPipeline) UpdateConfig(cfg STTPipelineConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.provider = cfg.Provider
	p.defaultLang = cfg.DefaultLang
	p.autoDetect = cfg.AutoDetect
	p.maxDuration = cfg.MaxDuration
	p.minConfidence = cfg.MinConfidence
	p.timeout = cfg.Timeout
	p.httpClient.Timeout = cfg.Timeout
}

func (p *STTPipeline) Config() STTPipelineConfig {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return STTPipelineConfig{
		Provider:      p.provider,
		DefaultLang:   p.defaultLang,
		AutoDetect:    p.autoDetect,
		MaxDuration:   p.maxDuration,
		MinConfidence: p.minConfidence,
		Timeout:       p.timeout,
	}
}
