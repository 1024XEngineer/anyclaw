package speech

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	speechpb "cloud.google.com/go/speech/apiv1/speechpb"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
)

type GoogleModel string

const (
	GoogleModelLatestLong            GoogleModel = "latest_long"
	GoogleModelLatestShort           GoogleModel = "latest_short"
	GoogleModelCommandAndSearch      GoogleModel = "command_and_search"
	GoogleModelPhoneCall             GoogleModel = "phone_call"
	GoogleModelVideo                 GoogleModel = "video"
	GoogleModelDefault               GoogleModel = "default"
	GoogleModelMedicalConversational GoogleModel = "medical_conversational"
	GoogleModelMedicalDictation      GoogleModel = "medical_dictation"
)

type GoogleRecognitionConfig struct {
	Encoding                            RecognitionEncoding
	SampleRateHertz                     int32
	AudioChannelCount                   int32
	EnableSeparateRecognitionPerChannel bool
	LanguageCode                        string
	MaxAlternatives                     int32
	ProfanityFilter                     bool
	SpeechContexts                      []GoogleSpeechContext
	EnableWordTimeOffsets               bool
	EnableWordConfidence                bool
	EnableAutomaticPunctuation          bool
	EnableSpokenPunctuation             bool
	Model                               string
	UseEnhanced                         bool
}

type RecognitionEncoding string

const (
	EncodingLinear16             RecognitionEncoding = "LINEAR16"
	EncodingFLAC                 RecognitionEncoding = "FLAC"
	EncodingMULAW                RecognitionEncoding = "MULAW"
	EncodingAMR                  RecognitionEncoding = "AMR"
	EncodingAMRWB                RecognitionEncoding = "AMR_WB"
	EncodingOGGOpus              RecognitionEncoding = "OGG_OPUS"
	EncodingSpeexWithHeaderByte  RecognitionEncoding = "SPEEX_WITH_HEADER_BYTE"
	EncodingMP3                  RecognitionEncoding = "MP3"
	EncodingWEBMOpus             RecognitionEncoding = "WEBM_OPUS"
	EncodingENCODING_UNSPECIFIED RecognitionEncoding = "ENCODING_UNSPECIFIED"
)

type GoogleSpeechContext struct {
	Phrases []string
	Boost   float32
}

type GoogleProvider struct {
	apiKey          string
	credentialsJSON string
	baseURL         string
	languageCode    string
	model           GoogleModel
	useEnhanced     bool
	timeout         time.Duration
	retries         int
	client          googleRecognizeAPI
}

type GoogleOption func(*GoogleProvider)

func WithGoogleBaseURL(url string) GoogleOption {
	return func(p *GoogleProvider) {
		p.baseURL = strings.TrimRight(url, "/")
	}
}

func WithGoogleLanguageCode(code string) GoogleOption {
	return func(p *GoogleProvider) {
		p.languageCode = code
	}
}

func WithGoogleModel(model GoogleModel) GoogleOption {
	return func(p *GoogleProvider) {
		p.model = model
	}
}

func WithGoogleEnhanced(enabled bool) GoogleOption {
	return func(p *GoogleProvider) {
		p.useEnhanced = enabled
	}
}

func WithGoogleTimeout(timeout time.Duration) GoogleOption {
	return func(p *GoogleProvider) {
		p.timeout = timeout
	}
}

func WithGoogleRetries(retries int) GoogleOption {
	return func(p *GoogleProvider) {
		p.retries = retries
	}
}

func WithGoogleCredentialsJSON(credentialsJSON string) GoogleOption {
	return func(p *GoogleProvider) {
		p.credentialsJSON = credentialsJSON
	}
}

func withGoogleRecognizeClient(client googleRecognizeAPI) GoogleOption {
	return func(p *GoogleProvider) {
		p.client = client
	}
}

func NewGoogleProvider(apiKey string, opts ...GoogleOption) (*GoogleProvider, error) {
	p := &GoogleProvider{
		apiKey:       apiKey,
		baseURL:      "https://speech.googleapis.com",
		languageCode: "en-US",
		model:        GoogleModelDefault,
		timeout:      120 * time.Second,
		retries:      2,
	}

	for _, opt := range opts {
		opt(p)
	}

	if p.apiKey == "" && p.credentialsJSON == "" {
		return nil, NewSTTError(ErrAuthentication, "google: API key or credentials JSON is required")
	}

	if p.client == nil {
		client, err := newGoogleRecognizeClient(context.Background(), p.clientOptions()...)
		if err != nil {
			return nil, NewSTTErrorf(ErrAuthentication, "google-speech: failed to initialize official client: %v", err)
		}
		p.client = client
	}

	return p, nil
}

func (p *GoogleProvider) clientOptions() []option.ClientOption {
	opts := make([]option.ClientOption, 0, 2)
	if p.credentialsJSON != "" {
		opts = append(opts, option.WithCredentialsJSON([]byte(p.credentialsJSON)))
	} else {
		opts = append(opts, option.WithAPIKey(p.apiKey))
	}
	if p.baseURL != "" && p.baseURL != "https://speech.googleapis.com" {
		opts = append(opts, option.WithEndpoint(p.baseURL))
	}
	return opts
}

func (p *GoogleProvider) Name() string {
	return "google-speech"
}

func (p *GoogleProvider) Type() STTProviderType {
	return STTProviderGoogle
}

func (p *GoogleProvider) Transcribe(ctx context.Context, audio []byte, opts ...TranscribeOption) (*TranscriptResult, error) {
	options := TranscribeOptions{
		Language:    p.languageCode,
		Mode:        ModeTranscription,
		InputFormat: InputMP3,
	}
	for _, opt := range opts {
		opt(&options)
	}

	if len(audio) == 0 {
		return nil, NewSTTError(ErrAudioFormatInvalid, "google-speech: audio data is empty")
	}

	const maxAudioSize = 100 * 1024 * 1024
	if len(audio) > maxAudioSize {
		return nil, NewSTTErrorf(ErrAudioTooLarge, "google-speech: audio exceeds 100MB limit (%d bytes)", len(audio))
	}

	if !validInputFormats[options.InputFormat] {
		return nil, NewSTTErrorf(ErrAudioFormatInvalid, "google-speech: unsupported input format: %s", options.InputFormat)
	}

	var lastErr error
	for attempt := 0; attempt <= p.retries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(attempt) * time.Second
			select {
			case <-ctx.Done():
				return nil, NewSTTErrorf(ErrTranscriptionFailed, "google-speech: context cancelled during retry: %v", ctx.Err())
			case <-time.After(backoff):
			}
		}

		result, err := p.doTranscribe(ctx, audio, options)
		if err == nil {
			return result, nil
		}

		lastErr = err

		if sttErr, ok := err.(*STTError); ok {
			if sttErr.Code == ErrAuthentication || sttErr.Code == ErrAudioFormatInvalid || sttErr.Code == ErrAudioTooLarge || sttErr.Code == ErrRateLimited {
				return nil, err
			}
		}
	}

	return nil, NewSTTErrorf(ErrTranscriptionFailed, "google-speech: all %d retries failed: %v", p.retries, lastErr)
}

func (p *GoogleProvider) TranscribeFile(ctx context.Context, filePath string, opts ...TranscribeOption) (*TranscriptResult, error) {
	return nil, NewSTTError(ErrProviderNotSupported, "google-speech: file transcription requires GCS URI, use Transcribe with file content instead")
}

func (p *GoogleProvider) TranscribeStream(ctx context.Context, reader io.Reader, opts ...TranscribeOption) (*TranscriptResult, error) {
	if reader == nil {
		return nil, NewSTTError(ErrAudioFormatInvalid, "google-speech: reader is nil")
	}

	audio, err := io.ReadAll(reader)
	if err != nil {
		return nil, NewSTTErrorf(ErrTranscriptionFailed, "google-speech: failed to read stream: %v", err)
	}

	return p.Transcribe(ctx, audio, opts...)
}

func (p *GoogleProvider) doTranscribe(ctx context.Context, audio []byte, options TranscribeOptions) (*TranscriptResult, error) {
	req := p.buildRecognizeRequest(audio, options)

	requestCtx := ctx
	var cancel context.CancelFunc
	if _, hasDeadline := ctx.Deadline(); !hasDeadline && p.timeout > 0 {
		requestCtx, cancel = context.WithTimeout(ctx, p.timeout)
		defer cancel()
	}

	resp, err := p.client.Recognize(requestCtx, req)
	if err != nil {
		return nil, p.handleClientError(err)
	}

	return p.parseRecognizeResponse(resp, options)
}

func (p *GoogleProvider) buildRecognizeRequest(audio []byte, options TranscribeOptions) *speechpb.RecognizeRequest {
	encoding := p.mapInputFormatToEncoding(options.InputFormat)

	sampleRate := int32(options.SampleRate)
	if sampleRate == 0 {
		sampleRate = p.guessSampleRate(options.InputFormat)
	}

	return &speechpb.RecognizeRequest{
		Config: &speechpb.RecognitionConfig{
			Encoding:                   p.toProtoRecognitionEncoding(encoding),
			SampleRateHertz:            sampleRate,
			LanguageCode:               options.Language,
			Model:                      string(p.model),
			UseEnhanced:                p.useEnhanced,
			MaxAlternatives:            int32(options.MaxAlternatives),
			EnableWordTimeOffsets:      options.WordTimestamps,
			EnableWordConfidence:       true,
			EnableAutomaticPunctuation: true,
		},
		Audio: &speechpb.RecognitionAudio{
			AudioSource: &speechpb.RecognitionAudio_Content{Content: audio},
		},
	}
}

func (p *GoogleProvider) mapInputFormatToEncoding(format AudioInputFormat) RecognitionEncoding {
	switch format {
	case InputWAV, InputPCM:
		return EncodingLinear16
	case InputFLAC:
		return EncodingFLAC
	case InputMP3:
		return EncodingMP3
	case InputOGG:
		return EncodingOGGOpus
	case InputWEBM:
		return EncodingWEBMOpus
	case InputM4A, InputMP4:
		return EncodingWEBMOpus
	case InputMPEG, InputMPGA:
		return EncodingMP3
	default:
		return EncodingMP3
	}
}

func (p *GoogleProvider) toProtoRecognitionEncoding(encoding RecognitionEncoding) speechpb.RecognitionConfig_AudioEncoding {
	switch encoding {
	case EncodingLinear16:
		return speechpb.RecognitionConfig_LINEAR16
	case EncodingFLAC:
		return speechpb.RecognitionConfig_FLAC
	case EncodingMULAW:
		return speechpb.RecognitionConfig_MULAW
	case EncodingAMR:
		return speechpb.RecognitionConfig_AMR
	case EncodingAMRWB:
		return speechpb.RecognitionConfig_AMR_WB
	case EncodingOGGOpus:
		return speechpb.RecognitionConfig_OGG_OPUS
	case EncodingSpeexWithHeaderByte:
		return speechpb.RecognitionConfig_SPEEX_WITH_HEADER_BYTE
	case EncodingWEBMOpus:
		return speechpb.RecognitionConfig_WEBM_OPUS
	case EncodingMP3:
		return speechpb.RecognitionConfig_MP3
	default:
		return speechpb.RecognitionConfig_ENCODING_UNSPECIFIED
	}
}

func (p *GoogleProvider) guessSampleRate(format AudioInputFormat) int32 {
	switch format {
	case InputWAV, InputPCM:
		return 16000
	case InputFLAC:
		return 16000
	case InputMP3:
		return 16000
	case InputOGG, InputWEBM:
		return 48000
	case InputM4A, InputMP4:
		return 44100
	default:
		return 16000
	}
}

func (p *GoogleProvider) handleClientError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return NewSTTErrorf(ErrTranscriptionFailed, "google-speech: request context error: %v", err)
	}

	var apiErr *googleapi.Error
	if errors.As(err, &apiErr) {
		msg := fmt.Sprintf("google-speech: API error: %s", apiErr.Message)
		switch apiErr.Code {
		case 400:
			return NewSTTError(ErrAudioFormatInvalid, msg)
		case 401, 403:
			return NewSTTError(ErrAuthentication, msg)
		case 429:
			return NewSTTError(ErrRateLimited, msg)
		default:
			return NewSTTError(ErrTranscriptionFailed, msg)
		}
	}

	switch status.Code(err) {
	case codes.InvalidArgument:
		return NewSTTError(ErrAudioFormatInvalid, "google-speech: invalid recognition request")
	case codes.Unauthenticated, codes.PermissionDenied:
		return NewSTTError(ErrAuthentication, "google-speech: authentication failed")
	case codes.ResourceExhausted:
		return NewSTTError(ErrRateLimited, "google-speech: rate limited")
	default:
		return NewSTTErrorf(ErrTranscriptionFailed, "google-speech: request failed: %v", err)
	}
}

func (p *GoogleProvider) parseRecognizeResponse(resp *speechpb.RecognizeResponse, options TranscribeOptions) (*TranscriptResult, error) {
	results := resp.GetResults()
	if len(results) == 0 {
		return &TranscriptResult{
			Text:     "",
			Language: options.Language,
		}, nil
	}

	result := &TranscriptResult{}
	var totalConfidence float64
	var confidenceCount int
	var lastEnd time.Duration

	for i, res := range results {
		if len(res.GetAlternatives()) == 0 {
			continue
		}

		primary := res.GetAlternatives()[0]
		segment := SegmentInfo{
			ID:   i,
			Text: primary.GetTranscript(),
		}

		if confidence := primary.GetConfidence(); confidence > 0 {
			segment.Confidence = float64(confidence)
			totalConfidence += segment.Confidence
			confidenceCount++
		}

		if len(primary.GetWords()) > 0 {
			segment.Words = make([]WordInfo, 0, len(primary.GetWords()))
			for _, word := range primary.GetWords() {
				wordInfo := WordInfo{
					Word:       word.GetWord(),
					StartTime:  parseProtoDuration(word.GetStartTime()),
					EndTime:    parseProtoDuration(word.GetEndTime()),
					Confidence: float64(word.GetConfidence()),
				}
				segment.Words = append(segment.Words, wordInfo)
			}
			segment.StartTime = segment.Words[0].StartTime
			segment.EndTime = segment.Words[len(segment.Words)-1].EndTime
		} else {
			segment.EndTime = parseProtoDuration(res.GetResultEndTime())
		}

		if options.WordTimestamps && len(segment.Words) > 0 {
			result.Words = append(result.Words, segment.Words...)
		}

		result.Segments = append(result.Segments, segment)

		if i == 0 {
			result.Text = primary.GetTranscript()
			if lang := res.GetLanguageCode(); lang != "" {
				result.Language = lang
			}
		} else {
			result.Text += " " + primary.GetTranscript()
		}

		if options.MaxAlternatives > 1 && len(res.GetAlternatives()) > 1 {
			for _, alt := range res.GetAlternatives()[1:] {
				result.Alternatives = append(result.Alternatives, alt.GetTranscript())
			}
		}

		if segment.EndTime > lastEnd {
			lastEnd = segment.EndTime
		}
	}

	if result.Language == "" {
		result.Language = options.Language
	}

	if confidenceCount > 0 {
		result.Confidence = totalConfidence / float64(confidenceCount)
	}

	if lastEnd > 0 {
		result.Duration = lastEnd
	} else {
		result.Duration = parseProtoDuration(resp.GetTotalBilledTime())
	}

	return result, nil
}

func parseProtoDuration(d *durationpb.Duration) time.Duration {
	if d == nil {
		return 0
	}
	return d.AsDuration()
}

func (p *GoogleProvider) ListLanguages(ctx context.Context) ([]string, error) {
	return []string{
		"af-ZA", "am-ET", "hy-AM", "az-AZ", "id-ID", "ms-MY", "bn-BD", "bn-IN", "ca-ES", "cs-CZ",
		"da-DK", "de-DE", "en-AU", "en-CA", "en-GH", "en-GB", "en-IN", "en-IE", "en-KE", "en-NZ",
		"en-NG", "en-PH", "en-SG", "en-ZA", "en-TZ", "en-US", "es-AR", "es-BO", "es-CL", "es-CO",
		"es-CR", "es-EC", "es-SV", "es-ES", "es-US", "es-GT", "es-HN", "es-MX", "es-NI", "es-PA",
		"es-PY", "es-PE", "es-PR", "es-DO", "es-UY", "es-VE", "eu-ES", "fil-PH", "fr-CA", "fr-FR",
		"gl-ES", "ka-GE", "gu-IN", "hr-HR", "zu-ZA", "is-IS", "it-IT", "jv-ID", "kn-IN", "km-KH",
		"lo-LA", "lv-LV", "lt-LT", "hu-HU", "ml-IN", "mr-IN", "nl-NL", "ne-NP", "nb-NO", "pl-PL",
		"pt-BR", "pt-PT", "ro-RO", "si-LK", "sk-SK", "sl-SI", "sr-RS", "fi-FI", "sv-SE", "ta-IN",
		"ta-SG", "ta-LK", "ta-MY", "te-IN", "vi-VN", "tr-TR", "ur-IN", "ur-PK", "el-GR", "bg-BG",
		"ru-RU", "sr-RS", "uk-UA", "he-IL", "ar-AE", "ar-BH", "ar-DZ", "ar-EG", "ar-IQ", "ar-JO",
		"ar-KW", "ar-LB", "ar-LY", "ar-MA", "ar-OM", "ar-QA", "ar-SA", "ar-PS", "ar-SY", "ar-TN",
		"ar-YE", "fa-IR", "hi-IN", "th-TH", "ko-KR", "zh-TW", "ja-JP", "zh", "zh-CN", "zh-HK",
		"yue-Hant-HK",
	}, nil
}
