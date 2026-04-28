package speech

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type WhisperModel string

const (
	WhisperModelV1 WhisperModel = "whisper-1"
)

var validWhisperModels = map[WhisperModel]bool{
	WhisperModelV1: true,
}

var validInputFormats = map[AudioInputFormat]bool{
	InputMP3:  true,
	InputWAV:  true,
	InputOGG:  true,
	InputFLAC: true,
	InputM4A:  true,
	InputMP4:  true,
	InputMPEG: true,
	InputMPGA: true,
	InputWEBM: true,
}

type WhisperProvider struct {
	apiKey        string
	baseURL       string
	model         WhisperModel
	language      string
	timeout       time.Duration
	retries       int
	client        *http.Client
	httpTransport *http.Transport
}

type WhisperOption func(*WhisperProvider)

func WithWhisperBaseURL(url string) WhisperOption {
	return func(p *WhisperProvider) {
		p.baseURL = strings.TrimRight(url, "/")
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

func WithWhisperHTTPTransport(transport *http.Transport) WhisperOption {
	return func(p *WhisperProvider) {
		p.httpTransport = transport
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

	if p.httpTransport != nil {
		p.client.Transport = p.httpTransport
	}
	p.client.Timeout = p.timeout

	if !validWhisperModels[p.model] {
		return nil, NewSTTErrorf(ErrProviderNotSupported, "openai: invalid whisper model: %s", p.model)
	}

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

	if err := p.validateTranscribeOptions(options); err != nil {
		return nil, err
	}

	if len(audio) == 0 {
		return nil, NewSTTError(ErrAudioFormatInvalid, "openai-whisper: audio data is empty")
	}

	const maxAudioSize = 25 * 1024 * 1024
	if len(audio) > maxAudioSize {
		return nil, NewSTTErrorf(ErrAudioTooLarge, "openai-whisper: audio exceeds 25MB limit (%d bytes)", len(audio))
	}

	if !validInputFormats[options.InputFormat] {
		return nil, NewSTTErrorf(ErrAudioFormatInvalid, "openai-whisper: unsupported input format: %s", options.InputFormat)
	}

	var lastErr error
	for attempt := 0; attempt <= p.retries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(attempt) * time.Second
			select {
			case <-ctx.Done():
				return nil, NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: context cancelled during retry: %v", ctx.Err())
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

	return nil, NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: all %d retries failed: %v", p.retries, lastErr)
}

func (p *WhisperProvider) TranscribeFile(ctx context.Context, filePath string, opts ...TranscribeOption) (*TranscriptResult, error) {
	if filePath == "" {
		return nil, NewSTTError(ErrAudioFormatInvalid, "openai-whisper: file path is empty")
	}

	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, NewSTTErrorf(ErrAudioFormatInvalid, "openai-whisper: file not found: %s", filePath)
		}
		return nil, NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: failed to stat file: %v", err)
	}

	const maxAudioSize = 25 * 1024 * 1024
	if info.Size() > maxAudioSize {
		return nil, NewSTTErrorf(ErrAudioTooLarge, "openai-whisper: file exceeds 25MB limit (%d bytes)", info.Size())
	}

	audio, err := os.ReadFile(filePath)
	if err != nil {
		return nil, NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: failed to read file: %v", err)
	}

	if len(opts) == 0 || anyInputFormatNotSet(opts) {
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(filePath)), ".")
		if ext != "" {
			formatOpts := append([]TranscribeOption{WithSTTInputFormat(AudioInputFormat(ext))}, opts...)
			return p.Transcribe(ctx, audio, formatOpts...)
		}
	}

	return p.Transcribe(ctx, audio, opts...)
}

func anyInputFormatNotSet(opts []TranscribeOption) bool {
	for _, opt := range opts {
		o := &TranscribeOptions{}
		opt(o)
		if o.InputFormat != "" {
			return false
		}
	}
	return true
}

func (p *WhisperProvider) TranscribeStream(ctx context.Context, reader io.Reader, opts ...TranscribeOption) (*TranscriptResult, error) {
	if reader == nil {
		return nil, NewSTTError(ErrAudioFormatInvalid, "openai-whisper: reader is nil")
	}

	audio, err := io.ReadAll(reader)
	if err != nil {
		return nil, NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: failed to read stream: %v", err)
	}

	return p.Transcribe(ctx, audio, opts...)
}

func (p *WhisperProvider) validateTranscribeOptions(options TranscribeOptions) error {
	if options.Temperature < 0 || options.Temperature > 1 {
		return NewSTTErrorf(ErrAudioFormatInvalid, "openai-whisper: temperature must be between 0 and 1, got: %f", options.Temperature)
	}

	if options.MaxAlternatives < 0 {
		return NewSTTErrorf(ErrAudioFormatInvalid, "openai-whisper: maxAlternatives cannot be negative")
	}

	if options.Model == "" {
		return NewSTTError(ErrAudioFormatInvalid, "openai-whisper: model is required")
	}

	return nil
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

	if options.MaxAlternatives > 0 {
		if err := writer.WriteField("max_alternatives", fmt.Sprintf("%d", options.MaxAlternatives)); err != nil {
			return nil, NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: failed to write max_alternatives field: %v", err)
		}
	}

	if options.WordTimestamps || options.SpeakerLabels {
		if options.WordTimestamps {
			if err := writer.WriteField("timestamp_granularities[]", "word"); err != nil {
				return nil, NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: failed to write word timestamp_granularities: %v", err)
			}
		}
		if options.SpeakerLabels {
			if err := writer.WriteField("timestamp_granularities[]", "segment"); err != nil {
				return nil, NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: failed to write segment timestamp_granularities: %v", err)
			}
		}
	}

	responseType := "verbose_json"
	if options.WordTimestamps || options.SpeakerLabels {
		responseType = "verbose_json"
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
	req.Header.Set("User-Agent", "anyclaw-stt/1.0")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, p.handleErrorResponse(resp.StatusCode, respBody)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: failed to read response: %v", err)
	}

	return p.parseResponse(respBody, options)
}

func (p *WhisperProvider) handleErrorResponse(statusCode int, body []byte) error {
	var errResp whisperErrorResponse
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error.Message != "" {
		msg := fmt.Sprintf("openai-whisper: API error: %s (type: %s, code: %s)",
			errResp.Error.Message, errResp.Error.Type, errResp.Error.Code)
		switch statusCode {
		case http.StatusUnauthorized:
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
	case http.StatusUnauthorized:
		return NewSTTError(ErrAuthentication, fmt.Sprintf("openai-whisper: authentication failed: %s", string(body)))
	case http.StatusTooManyRequests:
		return NewSTTError(ErrRateLimited, fmt.Sprintf("openai-whisper: rate limited: %s", string(body)))
	case http.StatusBadRequest:
		return NewSTTErrorf(ErrAudioFormatInvalid, "openai-whisper: invalid request: %s", string(body))
	case http.StatusServiceUnavailable:
		return NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: service unavailable: %s", string(body))
	default:
		return NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: unexpected status %d: %s", statusCode, string(body))
	}
}

type whisperResponse struct {
	Text     string  `json:"text"`
	Language string  `json:"language"`
	Duration float64 `json:"duration,omitempty"`
	Segments []struct {
		ID           int     `json:"id"`
		Seek         int     `json:"seek"`
		Start        float64 `json:"start"`
		End          float64 `json:"end"`
		Text         string  `json:"text"`
		Tokens       []int   `json:"tokens"`
		Temperature  float64 `json:"temperature"`
		AvgLogProb   float64 `json:"avg_logprob"`
		Compression  float64 `json:"compression_ratio"`
		NoSpeechProb float64 `json:"no_speech_prob"`
		Words        []struct {
			Word       string  `json:"word"`
			Start      float64 `json:"start"`
			End        float64 `json:"end"`
			Confidence float64 `json:"probability"`
		} `json:"words,omitempty"`
	} `json:"segments,omitempty"`
	LanguageProbability float64 `json:"language_probability,omitempty"`
}

type whisperErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

func (p *WhisperProvider) parseResponse(body []byte, options TranscribeOptions) (*TranscriptResult, error) {
	var resp whisperResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: failed to parse JSON response: %v", err)
	}

	result := &TranscriptResult{
		Text:       strings.TrimSpace(resp.Text),
		Language:   resp.Language,
		Duration:   time.Duration(resp.Duration * float64(time.Second)),
		Confidence: resp.LanguageProbability,
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

			if seg.AvgLogProb != 0 {
				segment.Confidence = normalizeLogProb(seg.AvgLogProb)
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

		if len(result.Segments) > 0 && result.Confidence == 0 {
			totalConfidence := 0.0
			for _, seg := range result.Segments {
				totalConfidence += seg.Confidence
			}
			result.Confidence = totalConfidence / float64(len(result.Segments))
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

func normalizeLogProb(logProb float64) float64 {
	if logProb > 0 {
		return 1.0
	}
	prob := 1.0 / (1.0 + logProb*-1)
	if prob < 0 {
		return 0
	}
	if prob > 1 {
		return 1
	}
	return prob
}

func (p *WhisperProvider) TranscribeSSE(ctx context.Context, audio []byte, onChunk func(chunk *TranscriptResult), opts ...TranscribeOption) error {
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

	if err := p.validateTranscribeOptions(options); err != nil {
		return err
	}

	if len(audio) == 0 {
		return NewSTTError(ErrAudioFormatInvalid, "openai-whisper: audio data is empty")
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	filename := "audio." + string(options.InputFormat)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: failed to create form file: %v", err)
	}

	if _, err := part.Write(audio); err != nil {
		return NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: failed to write audio data: %v", err)
	}

	if err := writer.WriteField("model", options.Model); err != nil {
		return NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: failed to write model field: %v", err)
	}

	if options.Language != "" {
		if err := writer.WriteField("language", options.Language); err != nil {
			return NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: failed to write language field: %v", err)
		}
	}

	if err := writer.WriteField("response_format", "json"); err != nil {
		return NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: failed to write response_format field: %v", err)
	}

	if err := writer.WriteField("stream", "true"); err != nil {
		return NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: failed to write stream field: %v", err)
	}

	if err := writer.Close(); err != nil {
		return NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: failed to close multipart writer: %v", err)
	}

	endpoint := "/v1/audio/transcriptions"
	if options.Mode == ModeTranslation {
		endpoint = "/v1/audio/translations"
	}

	url := p.baseURL + endpoint

	req, err := http.NewRequestWithContext(ctx, "POST", url, &body)
	if err != nil {
		return NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: failed to create request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "text/event-stream")

	resp, err := p.client.Do(req)
	if err != nil {
		return NewSTTErrorf(ErrTranscriptionFailed, "openai-whisper: streaming request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return p.handleErrorResponse(resp.StatusCode, respBody)
	}

	return p.readSSEStream(resp.Body, onChunk)
}

func (p *WhisperProvider) readSSEStream(reader io.Reader, onChunk func(chunk *TranscriptResult)) error {
	scanner := bufio.NewScanner(reader)
	scanner.Split(bufio.ScanLines)

	var currentText strings.Builder
	var detectedLanguage string

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				break
			}

			var chunk struct {
				Text     string `json:"text"`
				Language string `json:"language"`
				Done     bool   `json:"done"`
			}
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			if chunk.Text != "" {
				currentText.WriteString(chunk.Text)
			}
			if chunk.Language != "" {
				detectedLanguage = chunk.Language
			}

			onChunk(&TranscriptResult{
				Text:     currentText.String(),
				Language: detectedLanguage,
			})

			if chunk.Done {
				break
			}
		}
	}

	return scanner.Err()
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
