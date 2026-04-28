package speech

import (
	"context"
	"fmt"
	"time"
)

type STTProviderType string

const (
	STTProviderOpenAI        STTProviderType = "openai"
	STTProviderAzure         STTProviderType = "azure"
	STTProviderGoogle        STTProviderType = "google"
	STTProviderDeepgram      STTProviderType = "deepgram"
	STTProviderAssemblyAI    STTProviderType = "assemblyai"
	STTProviderWhisperCPP    STTProviderType = "whisper.cpp"
	STTProviderVosk          STTProviderType = "vosk"
	STTProviderFasterWhisper STTProviderType = "faster-whisper"
	STTProviderCustom        STTProviderType = "custom"
)

type AudioInputFormat string

const (
	InputMP3  AudioInputFormat = "mp3"
	InputWAV  AudioInputFormat = "wav"
	InputOGG  AudioInputFormat = "ogg"
	InputFLAC AudioInputFormat = "flac"
	InputPCM  AudioInputFormat = "pcm"
	InputM4A  AudioInputFormat = "m4a"
	InputMP4  AudioInputFormat = "mp4"
	InputMPEG AudioInputFormat = "mpeg"
	InputMPGA AudioInputFormat = "mpga"
	InputWEBM AudioInputFormat = "webm"
)

type STTProvider interface {
	Name() string
	Type() STTProviderType
	Transcribe(ctx context.Context, audio []byte, opts ...TranscribeOption) (*TranscriptResult, error)
	ListLanguages(ctx context.Context) ([]string, error)
}

type STTConfig struct {
	Type            STTProviderType
	APIKey          string
	CredentialsJSON string
	BaseURL         string
	Model           string
	Language        string
	SampleRate      int
	Timeout         time.Duration
}

func NewSTTProvider(cfg STTConfig) (STTProvider, error) {
	switch cfg.Type {
	case STTProviderOpenAI:
		opts := []WhisperOption{}
		if cfg.BaseURL != "" {
			opts = append(opts, WithWhisperBaseURL(cfg.BaseURL))
		}
		if cfg.Model != "" {
			opts = append(opts, WithWhisperModel(WhisperModel(cfg.Model)))
		}
		if cfg.Language != "" {
			opts = append(opts, WithWhisperLanguage(cfg.Language))
		}
		if cfg.Timeout > 0 {
			opts = append(opts, WithWhisperTimeout(cfg.Timeout))
		}
		return NewWhisperProvider(cfg.APIKey, opts...)
	case STTProviderGoogle:
		opts := []GoogleOption{}
		if cfg.CredentialsJSON != "" {
			opts = append(opts, WithGoogleCredentialsJSON(cfg.CredentialsJSON))
		}
		if cfg.BaseURL != "" {
			opts = append(opts, WithGoogleBaseURL(cfg.BaseURL))
		}
		if cfg.Language != "" {
			opts = append(opts, WithGoogleLanguageCode(cfg.Language))
		}
		if cfg.Timeout > 0 {
			opts = append(opts, WithGoogleTimeout(cfg.Timeout))
		}
		return NewGoogleProvider(cfg.APIKey, opts...)
	case STTProviderWhisperCPP:
		opts := []WhisperCPPOption{}
		if cfg.Model != "" {
			opts = append(opts, WithWhisperCPPModelPath(cfg.Model))
		}
		if cfg.Language != "" {
			opts = append(opts, WithWhisperCPPLanguage(cfg.Language))
		}
		if cfg.Timeout > 0 {
			opts = append(opts, WithWhisperCPPTimeout(cfg.Timeout))
		}
		return NewWhisperCPPProvider(opts...)
	default:
		return nil, NewSTTError(ErrProviderNotSupported, "unknown STT provider: "+string(cfg.Type))
	}
}

type TranscribeMode string

const (
	ModeTranscription TranscribeMode = "transcription"
	ModeTranslation   TranscribeMode = "translation"
)

type TranscribeOptions struct {
	Language        string
	Model           string
	Prompt          string
	Temperature     float64
	Mode            TranscribeMode
	InputFormat     AudioInputFormat
	SampleRate      int
	WordTimestamps  bool
	SpeakerLabels   bool
	MaxAlternatives int
}

type TranscribeOption func(*TranscribeOptions)

func WithSTTLanguage(lang string) TranscribeOption {
	return func(o *TranscribeOptions) {
		o.Language = lang
	}
}

func WithSTTModel(model string) TranscribeOption {
	return func(o *TranscribeOptions) {
		o.Model = model
	}
}

func WithSTTPrompt(prompt string) TranscribeOption {
	return func(o *TranscribeOptions) {
		o.Prompt = prompt
	}
}

func WithSTTTemperature(temp float64) TranscribeOption {
	return func(o *TranscribeOptions) {
		o.Temperature = temp
	}
}

func WithSTTMode(mode TranscribeMode) TranscribeOption {
	return func(o *TranscribeOptions) {
		o.Mode = mode
	}
}

func WithSTTInputFormat(format AudioInputFormat) TranscribeOption {
	return func(o *TranscribeOptions) {
		o.InputFormat = format
	}
}

func WithSTTSampleRate(rate int) TranscribeOption {
	return func(o *TranscribeOptions) {
		o.SampleRate = rate
	}
}

func WithSTTWordTimestamps(enabled bool) TranscribeOption {
	return func(o *TranscribeOptions) {
		o.WordTimestamps = enabled
	}
}

func WithSTTSpeakerLabels(enabled bool) TranscribeOption {
	return func(o *TranscribeOptions) {
		o.SpeakerLabels = enabled
	}
}

func WithSTTMaxAlternatives(n int) TranscribeOption {
	return func(o *TranscribeOptions) {
		o.MaxAlternatives = n
	}
}

type WordInfo struct {
	Word       string
	StartTime  time.Duration
	EndTime    time.Duration
	Confidence float64
}

type SegmentInfo struct {
	ID         int
	Text       string
	StartTime  time.Duration
	EndTime    time.Duration
	Confidence float64
	Speaker    string
	Words      []WordInfo
}

type TranscriptResult struct {
	Text         string
	Language     string
	Duration     time.Duration
	Confidence   float64
	Segments     []SegmentInfo
	Words        []WordInfo
	Alternatives []string
}

type STTErrorCode string

const (
	ErrProviderNotSupported STTErrorCode = "provider_not_supported"
	ErrAudioFormatInvalid   STTErrorCode = "audio_format_invalid"
	ErrTranscriptionFailed  STTErrorCode = "transcription_failed"
	ErrAudioTooLong         STTErrorCode = "audio_too_long"
	ErrAudioTooLarge        STTErrorCode = "audio_too_large"
	ErrRateLimited          STTErrorCode = "rate_limited"
	ErrAuthentication       STTErrorCode = "authentication_failed"
)

type STTError struct {
	Code    STTErrorCode
	Message string
	Err     error
}

func NewSTTError(code STTErrorCode, message string) *STTError {
	return &STTError{Code: code, Message: message}
}

func NewSTTErrorf(code STTErrorCode, format string, args ...interface{}) *STTError {
	return &STTError{Code: code, Message: fmt.Sprintf(format, args...)}
}

func (e *STTError) Error() string {
	if e.Err != nil {
		return string(e.Code) + ": " + e.Message + ": " + e.Err.Error()
	}
	return string(e.Code) + ": " + e.Message
}

func (e *STTError) Unwrap() error {
	return e.Err
}
