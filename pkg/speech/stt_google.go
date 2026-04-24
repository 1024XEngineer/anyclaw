package speech

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
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
	client          *http.Client
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

func NewGoogleProvider(apiKey string, opts ...GoogleOption) (*GoogleProvider, error) {
	if apiKey == "" {
		return nil, NewSTTError(ErrAuthentication, "google: API key is required")
	}

	p := &GoogleProvider{
		apiKey:       apiKey,
		baseURL:      "https://speech.googleapis.com",
		languageCode: "en-US",
		model:        GoogleModelDefault,
		timeout:      120 * time.Second,
		retries:      2,
		client:       &http.Client{Timeout: 120 * time.Second},
	}

	for _, opt := range opts {
		opt(p)
	}

	p.client.Timeout = p.timeout

	return p, nil
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
	encoding := p.mapInputFormatToEncoding(options.InputFormat)

	sampleRate := int32(options.SampleRate)
	if sampleRate == 0 {
		sampleRate = p.guessSampleRate(options.InputFormat)
	}

	reqBody := googleRecognizeRequest{
		Config: googleRecognitionConfigRequest{
			Encoding:                   string(encoding),
			SampleRateHertz:            sampleRate,
			LanguageCode:               options.Language,
			Model:                      string(p.model),
			UseEnhanced:                p.useEnhanced,
			MaxAlternatives:            int32(options.MaxAlternatives),
			EnableWordTimeOffsets:      options.WordTimestamps,
			EnableWordConfidence:       true,
			EnableAutomaticPunctuation: true,
		},
		Audio: googleAudioRequest{
			Content: base64.StdEncoding.EncodeToString(audio),
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, NewSTTErrorf(ErrTranscriptionFailed, "google-speech: failed to marshal request: %v", err)
	}

	url := fmt.Sprintf("%s/v1/speech:recognize?key=%s", p.baseURL, p.apiKey)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, NewSTTErrorf(ErrTranscriptionFailed, "google-speech: failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "anyclaw-stt/1.0")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, NewSTTErrorf(ErrTranscriptionFailed, "google-speech: request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, p.handleErrorResponse(resp.StatusCode, respBody)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, NewSTTErrorf(ErrTranscriptionFailed, "google-speech: failed to read response: %v", err)
	}

	return p.parseResponse(respBody, options)
}

func (p *GoogleProvider) mapInputFormatToEncoding(format AudioInputFormat) RecognitionEncoding {
	switch format {
	case InputWAV, InputPCM:
		return EncodingLinear16
	case InputFLAC:
		return EncodingFLAC
	case InputMP3:
		return EncodingMP3
	case InputOGG, InputWEBM:
		return EncodingWEBMOpus
	case InputM4A, InputMP4:
		return EncodingWEBMOpus
	case InputMPEG, InputMPGA:
		return EncodingMP3
	default:
		return EncodingMP3
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

func (p *GoogleProvider) handleErrorResponse(statusCode int, body []byte) error {
	var errResp googleErrorResponse
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error.Message != "" {
		msg := fmt.Sprintf("google-speech: API error: %s (status: %s)", errResp.Error.Message, errResp.Error.Status)
		switch statusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return NewSTTError(ErrAuthentication, msg)
		case http.StatusTooManyRequests:
			return NewSTTError(ErrRateLimited, msg)
		case http.StatusBadRequest:
			return NewSTTError(ErrAudioFormatInvalid, msg)
		default:
			return NewSTTError(ErrTranscriptionFailed, msg)
		}
	}

	switch statusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return NewSTTError(ErrAuthentication, fmt.Sprintf("google-speech: authentication failed: %s", string(body)))
	case http.StatusTooManyRequests:
		return NewSTTError(ErrRateLimited, fmt.Sprintf("google-speech: rate limited: %s", string(body)))
	case http.StatusBadRequest:
		return NewSTTError(ErrAudioFormatInvalid, fmt.Sprintf("google-speech: invalid request: %s", string(body)))
	case http.StatusServiceUnavailable:
		return NewSTTError(ErrTranscriptionFailed, fmt.Sprintf("google-speech: service unavailable: %s", string(body)))
	default:
		return NewSTTErrorf(ErrTranscriptionFailed, "google-speech: unexpected status %d: %s", statusCode, string(body))
	}
}

type googleRecognizeRequest struct {
	Config googleRecognitionConfigRequest `json:"config"`
	Audio  googleAudioRequest             `json:"audio"`
}

type googleRecognitionConfigRequest struct {
	Encoding                   string `json:"encoding"`
	SampleRateHertz            int32  `json:"sampleRateHertz"`
	LanguageCode               string `json:"languageCode"`
	Model                      string `json:"model,omitempty"`
	UseEnhanced                bool   `json:"useEnhanced,omitempty"`
	MaxAlternatives            int32  `json:"maxAlternatives,omitempty"`
	EnableWordTimeOffsets      bool   `json:"enableWordTimeOffsets,omitempty"`
	EnableWordConfidence       bool   `json:"enableWordConfidence,omitempty"`
	EnableAutomaticPunctuation bool   `json:"enableAutomaticPunctuation,omitempty"`
	EnableSpokenPunctuation    bool   `json:"enableSpokenPunctuation,omitempty"`
}

type googleAudioRequest struct {
	Content string `json:"content"`
}

type googleResponse struct {
	Results []googleResult `json:"results"`
}

type googleResult struct {
	Alternatives  []googleAlternative `json:"alternatives"`
	LanguageCode  string              `json:"languageCode"`
	ResultEndTime struct {
		Seconds string `json:"seconds"`
		Nanos   int    `json:"nanos"`
	} `json:"resultEndTime"`
}

type googleAlternative struct {
	Transcript string           `json:"transcript"`
	Confidence float64          `json:"confidence"`
	Words      []googleWordInfo `json:"words"`
}

type googleWordInfo struct {
	StartTime  googleDuration `json:"startTime"`
	EndTime    googleDuration `json:"endTime"`
	Word       string         `json:"word"`
	Confidence float64        `json:"confidence"`
}

type googleDuration struct {
	Seconds string `json:"seconds"`
	Nanos   int    `json:"nanos"`
}

type googleErrorResponse struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

func (p *GoogleProvider) parseResponse(body []byte, options TranscribeOptions) (*TranscriptResult, error) {
	var resp googleResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, NewSTTErrorf(ErrTranscriptionFailed, "google-speech: failed to parse JSON response: %v", err)
	}

	if len(resp.Results) == 0 {
		return &TranscriptResult{
			Text:     "",
			Language: options.Language,
		}, nil
	}

	result := &TranscriptResult{}
	var totalConfidence float64
	var confidenceCount int

	for i, res := range resp.Results {
		if len(res.Alternatives) == 0 {
			continue
		}

		primary := res.Alternatives[0]

		segment := SegmentInfo{
			ID:   i,
			Text: primary.Transcript,
		}

		if primary.Confidence > 0 {
			segment.Confidence = primary.Confidence
			totalConfidence += primary.Confidence
			confidenceCount++
		}

		if len(primary.Words) > 0 {
			segment.Words = make([]WordInfo, 0, len(primary.Words))
			for _, w := range primary.Words {
				segment.Words = append(segment.Words, WordInfo{
					Word:       w.Word,
					StartTime:  parseGoogleDuration(w.StartTime),
					EndTime:    parseGoogleDuration(w.EndTime),
					Confidence: w.Confidence,
				})
			}
		}

		if options.WordTimestamps && len(segment.Words) > 0 {
			result.Words = append(result.Words, segment.Words...)
		}

		result.Segments = append(result.Segments, segment)

		if i == 0 {
			result.Text = primary.Transcript
			result.Language = res.LanguageCode
		} else {
			result.Text += " " + primary.Transcript
		}

		if options.MaxAlternatives > 1 && len(res.Alternatives) > 1 {
			for _, alt := range res.Alternatives[1:] {
				result.Alternatives = append(result.Alternatives, alt.Transcript)
			}
		}
	}

	if confidenceCount > 0 {
		result.Confidence = totalConfidence / float64(confidenceCount)
	}

	if len(resp.Results) > 0 {
		endTime := resp.Results[len(resp.Results)-1].ResultEndTime
		result.Duration = parseGoogleDuration(googleDuration{
			Seconds: endTime.Seconds,
			Nanos:   endTime.Nanos,
		})
	}

	return result, nil
}

func parseGoogleDuration(d googleDuration) time.Duration {
	var seconds int64
	if d.Seconds != "" {
		fmt.Sscanf(d.Seconds, "%d", &seconds)
	}
	return time.Duration(seconds)*time.Second + time.Duration(d.Nanos)*time.Nanosecond
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
