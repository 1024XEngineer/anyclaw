package speech

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"
)

type ProviderType string

const (
	ProviderOpenAI     ProviderType = "openai"
	ProviderElevenLabs ProviderType = "elevenlabs"
	ProviderEdge       ProviderType = "edge"
	ProviderAzure      ProviderType = "azure"
	ProviderGoogle     ProviderType = "google"
	ProviderAliyun     ProviderType = "aliyun"
	ProviderPiper      ProviderType = "piper"
	ProviderCoqui      ProviderType = "coqui"
	ProviderCustom     ProviderType = "custom"
)

type AudioFormat string

const (
	FormatMP3  AudioFormat = "mp3"
	FormatWAV  AudioFormat = "wav"
	FormatOGG  AudioFormat = "ogg"
	FormatFLAC AudioFormat = "flac"
	FormatPCM  AudioFormat = "pcm"
)

type VoiceGender string

const (
	GenderMale    VoiceGender = "male"
	GenderFemale  VoiceGender = "female"
	GenderNeutral VoiceGender = "neutral"
)

type Provider interface {
	Name() string
	Type() ProviderType
	Synthesize(ctx context.Context, text string, opts ...SynthesizeOption) (*AudioResult, error)
	ListVoices(ctx context.Context) ([]Voice, error)
}

type Config struct {
	Type        ProviderType
	APIKey      string
	BaseURL     string
	Voice       string
	Language    string
	SampleRate  int
	AudioFormat AudioFormat
	Timeout     time.Duration
}

func NewProvider(cfg Config) (Provider, error) {
	switch cfg.Type {
	case ProviderOpenAI:
		opts := []OpenAIOption{}
		if cfg.BaseURL != "" {
			opts = append(opts, WithOpenAIBaseURL(cfg.BaseURL))
		}
		if cfg.Voice != "" {
			opts = append(opts, WithOpenAIVoice(cfg.Voice))
		}
		if cfg.Timeout > 0 {
			opts = append(opts, WithOpenAITimeout(cfg.Timeout))
		}
		return NewOpenAIProvider(cfg.APIKey, opts...)
	case ProviderElevenLabs:
		opts := []ElevenLabsOption{}
		if cfg.BaseURL != "" {
			opts = append(opts, WithElevenLabsBaseURL(cfg.BaseURL))
		}
		if cfg.Voice != "" {
			opts = append(opts, WithElevenLabsVoice(cfg.Voice))
		}
		if cfg.Timeout > 0 {
			opts = append(opts, WithElevenLabsTimeout(cfg.Timeout))
		}
		return NewElevenLabsProvider(cfg.APIKey, opts...)
	case ProviderEdge:
		opts := []EdgeOption{}
		if cfg.BaseURL != "" {
			opts = append(opts, WithEdgeBaseURL(cfg.BaseURL))
		}
		if cfg.Voice != "" {
			opts = append(opts, WithEdgeVoice(cfg.Voice))
		}
		if cfg.Language != "" {
			opts = append(opts, WithEdgeLanguage(cfg.Language))
		}
		if cfg.Timeout > 0 {
			opts = append(opts, WithEdgeTimeout(cfg.Timeout))
		}
		return NewEdgeProvider(opts...)
	case ProviderPiper:
		opts := []PiperOption{}
		if cfg.Voice != "" {
			opts = append(opts, WithPiperDefaultVoice(cfg.Voice))
		}
		if cfg.Language != "" {
			opts = append(opts, WithPiperDefaultLanguage(cfg.Language))
		}
		return NewPiperProvider(opts...)
	case ProviderCoqui:
		opts := []CoquiOption{}
		if cfg.Voice != "" {
			opts = append(opts, WithCoquiSpeaker(cfg.Voice))
		}
		if cfg.Language != "" {
			opts = append(opts, WithCoquiDefaultLanguage(cfg.Language))
		}
		return NewCoquiProvider(opts...)
	default:
		return nil, fmt.Errorf("unknown TTS provider: %s", cfg.Type)
	}
}

type SynthesizeOptions struct {
	Voice      string
	Speed      float64
	Pitch      float64
	Volume     float64
	Format     AudioFormat
	Language   string
	SampleRate int
}

type SynthesizeOption func(*SynthesizeOptions)

func WithVoice(voice string) SynthesizeOption {
	return func(o *SynthesizeOptions) {
		o.Voice = voice
	}
}

func WithSpeed(speed float64) SynthesizeOption {
	return func(o *SynthesizeOptions) {
		o.Speed = speed
	}
}

func WithPitch(pitch float64) SynthesizeOption {
	return func(o *SynthesizeOptions) {
		o.Pitch = pitch
	}
}

func WithVolume(volume float64) SynthesizeOption {
	return func(o *SynthesizeOptions) {
		o.Volume = volume
	}
}

func WithFormat(format AudioFormat) SynthesizeOption {
	return func(o *SynthesizeOptions) {
		o.Format = format
	}
}

func WithLanguage(lang string) SynthesizeOption {
	return func(o *SynthesizeOptions) {
		o.Language = lang
	}
}

func WithSampleRate(rate int) SynthesizeOption {
	return func(o *SynthesizeOptions) {
		o.SampleRate = rate
	}
}

type AudioResult struct {
	Data        []byte
	Format      AudioFormat
	SampleRate  int
	Channels    int
	BitDepth    int
	Duration    time.Duration
	ContentType string
}

type Voice struct {
	ID          string
	Name        string
	Language    string
	LanguageTag string
	Gender      VoiceGender
	Provider    string
	Description string
}

func AudioToBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

func Base64ToAudio(b64 string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(b64)
}
