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

type WhisperModel string

const (
	WhisperModelV1    WhisperModel = "whisper-1"
	WhisperModelLarge WhisperModel = "whisper-large"
)

type WhisperProvider struct {
	apiKey   string
	baseURL  string
	model    WhisperModel
	language string
	timeout  time.Duration
	retries  int
	client   *http.Client
}

type WhisperOption func(*WhisperProvider)

func WithWhisperBaseURL(url string) WhisperOption {
	return func(p *WhisperProvider) {
		p.baseURL = url
	}
}

func WithWhisperModel(model WhisperModel) WhisperOption {
	return func(p *WhisperProvider) {
		p.model = model
	}
}

func WithWhisperLanguage(lang string) WhisperOption {
	return func(p *WhisperProvider) {
		p.language = lang
	}
}

func WithWhisperTimeout(timeout time.Duration) WhisperOption {
	return func(p *WhisperProvider) {
		p.timeout = timeout
	}
}

func WithWhisperRetries(retries int) WhisperOption {
	return func(p *WhisperProvider) {
		p.retries = retries
	}
}

func NewWhisperProvider(apiKey string, opts ...WhisperOption) (*WhisperProvider, error) {
	if apiKey == "" {
		return nil, NewSTTError(ErrAuthentication, "openai: API key is required")
	}

	p := &WhisperProvider{
		apiKey:  apiKey,
		baseURL: "https://api.openai.com",
		model:   WhisperModelV1,
		timeout: 120 * time.Second,
		retries: 2,
		client:  &http.Client{Timeout: 120 * time.Second},
	}

	for _, opt := range opts {
		opt(p)
	}

	p.client.Timeout = p.timeout

	return p, nil
}

func (p *WhisperProvider) Name() string {
	return "openai-whisper"
}

func (p *WhisperProvider) Type() STTProviderType {
	return STTProviderOpenAI
}

func (p *WhisperProvider) Transcribe(ctx context.Context, audio []byte, opts ...TranscribeOption) (*TranscriptResult, error) {
	options := TranscribeOptions{
		Model:       string(p.model),
		Language:    p.language,
		Temperature: 0,
		Mode:        ModeTranscription,
		InputFormat: InputMP3,
	}
	for _, opt := range opts {
		opt(&options)
	}

	if len(audio) == 0 {
		return nil, NewSTTError(ErrAudioFormatInvalid, "openai-whisper: audio data is empty")
	}

	if len(audio) > 25*1024*1024 {
		return nil, NewSTTError(ErrAudioTooLarge, "openai-whisper: audio exceeds 25MB limit")
	}

	var lastErr error
	for attempt := 0; attempt <= p.retries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(attempt) * time.Second
			select {
			case <-ctx.Done():
				return nil, NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: context cancelled during retry")
			case <-time.After(backoff):
			}
		}

		result, err := p.doTranscribe(ctx, audio, options)
		if err == nil {
			return result, nil
		}

		lastErr = err
	}

	return nil, NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: all %d retries failed: %v", p.retries, lastErr)
}

func (p *WhisperProvider) doTranscribe(ctx context.Context, audio []byte, options TranscribeOptions) (*TranscriptResult, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	filename := "audio." + string(options.InputFormat)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: failed to create form file: %v", err)
	}

	if _, err := part.Write(audio); err != nil {
		return nil, NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: failed to write audio data: %v", err)
	}

	if err := writer.WriteField("model", options.Model); err != nil {
		return nil, NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: failed to write model field: %v", err)
	}

	if options.Language != "" {
		if err := writer.WriteField("language", options.Language); err != nil {
			return nil, NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: failed to write language field: %v", err)
		}
	}

	if options.Prompt != "" {
		if err := writer.WriteField("prompt", options.Prompt); err != nil {
			return nil, NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: failed to write prompt field: %v", err)
		}
	}

	if options.Temperature > 0 {
		if err := writer.WriteField("temperature", fmt.Sprintf("%.2f", options.Temperature)); err != nil {
			return nil, NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: failed to write temperature field: %v", err)
		}
	}

	if options.WordTimestamps {
		if err := writer.WriteField("timestamp_granularities[]", "word"); err != nil {
			return nil, NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: failed to write timestamp_granularities field: %v", err)
		}
	}

	responseType := "verbose_json"
	if !options.WordTimestamps {
		responseType = "json"
	}
	if err := writer.WriteField("response_format", responseType); err != nil {
		return nil, NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: failed to write response_format field: %v", err)
	}

	if err := writer.Close(); err != nil {
		return nil, NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: failed to close multipart writer: %v", err)
	}

	var endpoint string
	switch options.Mode {
	case ModeTranslation:
		endpoint = "/v1/audio/translations"
	default:
		endpoint = "/v1/audio/transcriptions"
	}

	url := p.baseURL + endpoint

	req, err := http.NewRequestWithContext(ctx, "POST", url, &body)
	if err != nil {
		return nil, NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: failed to create request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			return nil, NewSTTError(ErrAuthentication, fmt.Sprintf("openai-whisper: authentication failed: %s", string(respBody)))
		case http.StatusTooManyRequests:
			return nil, NewSTTError(ErrRateLimited, fmt.Sprintf("openai-whisper: rate limited: %s", string(respBody)))
		case http.StatusBadRequest:
			return nil, NewSTTErrorf(ErrAudioFormatInvalid, "openai-whisper: invalid request: %s", string(respBody))
		default:
			return nil, NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: unexpected status %d: %s", resp.StatusCode, string(respBody))
		}
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: failed to read response: %v", err)
	}

	return p.parseResponse(respBody, options)
}

type whisperResponse struct {
	Text     string  `json:"text"`
	Language string  `json:"language"`
	Duration float64 `json:"duration,omitempty"`
	Segments []struct {
		ID         int     `json:"id"`
		Text       string  `json:"text"`
		Start      float64 `json:"start"`
		End        float64 `json:"end"`
		Confidence float64 `json:"avg_logprob,omitempty"`
		Words      []struct {
			Word       string  `json:"word"`
			Start      float64 `json:"start"`
			End        float64 `json:"end"`
			Confidence float64 `json:"probability"`
		} `json:"words,omitempty"`
	} `json:"segments,omitempty"`
}

func (p *WhisperProvider) parseResponse(body []byte, options TranscribeOptions) (*TranscriptResult, error) {
	var resp whisperResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: failed to parse response: %v", err)
	}

	result := &TranscriptResult{
		Text:     resp.Text,
		Language: resp.Language,
		Duration: time.Duration(resp.Duration * float64(time.Second)),
	}

	if len(resp.Segments) > 0 {
		result.Segments = make([]SegmentInfo, 0, len(resp.Segments))
		for _, seg := range resp.Segments {
			segment := SegmentInfo{
				ID:        seg.ID,
				Text:      seg.Text,
				StartTime: time.Duration(seg.Start * float64(time.Second)),
				EndTime:   time.Duration(seg.End * float64(time.Second)),
			}
			if seg.Confidence > 0 {
				segment.Confidence = seg.Confidence
			}
			if len(seg.Words) > 0 {
				segment.Words = make([]WordInfo, 0, len(seg.Words))
				for _, w := range seg.Words {
					segment.Words = append(segment.Words, WordInfo{
						Word:       w.Word,
						StartTime:  time.Duration(w.Start * float64(time.Second)),
						EndTime:    time.Duration(w.End * float64(time.Second)),
						Confidence: w.Confidence,
					})
				}
			}
			result.Segments = append(result.Segments, segment)
		}
	}

	if options.WordTimestamps && len(result.Segments) > 0 {
		words := make([]WordInfo, 0)
		for _, seg := range result.Segments {
			words = append(words, seg.Words...)
		}
		result.Words = words
	}

	return result, nil
}

func (p *WhisperProvider) ListLanguages(ctx context.Context) ([]string, error) {
	return []string{
		"af", "am", "ar", "as", "az", "ba", "be", "bg", "bn", "bo", "br", "bs", "ca", "cs", "cy", "da",
		"de", "el", "en", "es", "et", "eu", "fa", "fi", "fo", "fr", "gl", "gu", "ha", "haw", "he", "hi",
		"hr", "ht", "hu", "hy", "id", "is", "it", "ja", "jw", "ka", "kk", "km", "kn", "ko", "la", "lb",
		"ln", "lo", "lt", "lv", "mg", "mi", "mk", "ml", "mn", "mr", "ms", "mt", "my", "ne", "nl", "nn",
		"no", "oc", "pa", "pl", "ps", "pt", "ro", "ru", "sa", "sd", "si", "sk", "sl", "sn", "so", "sq",
		"sr", "su", "sv", "sw", "ta", "te", "tg", "th", "tk", "tl", "tr", "tt", "uk", "ur", "uz", "vi",
		"yi", "yo", "zh",
	}, nil
}
