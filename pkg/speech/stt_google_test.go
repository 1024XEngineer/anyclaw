package speech

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	speechpb "cloud.google.com/go/speech/apiv1/speechpb"
	"google.golang.org/api/googleapi"
	"google.golang.org/protobuf/types/known/durationpb"
)

type fakeGoogleRecognizeClient struct {
	calls       int
	lastRequest *speechpb.RecognizeRequest
	recognizeFn func(context.Context, *speechpb.RecognizeRequest) (*speechpb.RecognizeResponse, error)
}

func (f *fakeGoogleRecognizeClient) Recognize(ctx context.Context, req *speechpb.RecognizeRequest) (*speechpb.RecognizeResponse, error) {
	f.calls++
	f.lastRequest = req
	if f.recognizeFn != nil {
		return f.recognizeFn(ctx, req)
	}
	return &speechpb.RecognizeResponse{}, nil
}

func (f *fakeGoogleRecognizeClient) Close() error {
	return nil
}

func TestNewGoogleProvider(t *testing.T) {
	t.Run("requires API key or credentials JSON", func(t *testing.T) {
		_, err := NewGoogleProvider("", withGoogleRecognizeClient(&fakeGoogleRecognizeClient{}))
		if err == nil {
			t.Fatal("expected error when auth config is empty")
		}
		sttErr, ok := err.(*STTError)
		if !ok {
			t.Fatalf("expected *STTError, got %T", err)
		}
		if sttErr.Code != ErrAuthentication {
			t.Errorf("expected ErrAuthentication, got %s", sttErr.Code)
		}
	})

	t.Run("creates provider with defaults", func(t *testing.T) {
		fake := &fakeGoogleRecognizeClient{}
		p, err := NewGoogleProvider("test-key", withGoogleRecognizeClient(fake))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Name() != "google-speech" {
			t.Errorf("expected name google-speech, got %s", p.Name())
		}
		if p.Type() != STTProviderGoogle {
			t.Errorf("expected type %s, got %s", STTProviderGoogle, p.Type())
		}
		if p.baseURL != "https://speech.googleapis.com" {
			t.Errorf("expected default baseURL, got %s", p.baseURL)
		}
		if p.languageCode != "en-US" {
			t.Errorf("expected default language en-US, got %s", p.languageCode)
		}
		if p.model != GoogleModelDefault {
			t.Errorf("expected default model %s, got %s", GoogleModelDefault, p.model)
		}
		if p.retries != 2 {
			t.Errorf("expected 2 retries, got %d", p.retries)
		}
		if p.client != fake {
			t.Fatal("expected injected fake client to be used")
		}
	})

	t.Run("applies options", func(t *testing.T) {
		p, err := NewGoogleProvider("test-key",
			withGoogleRecognizeClient(&fakeGoogleRecognizeClient{}),
			WithGoogleBaseURL("https://custom.speech.api.com/"),
			WithGoogleLanguageCode("zh-CN"),
			WithGoogleModel(GoogleModelLatestLong),
			WithGoogleEnhanced(true),
			WithGoogleTimeout(30*time.Second),
			WithGoogleRetries(5),
			WithGoogleCredentialsJSON(`{"type":"service_account"}`),
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.baseURL != "https://custom.speech.api.com" {
			t.Errorf("expected custom baseURL, got %s", p.baseURL)
		}
		if p.languageCode != "zh-CN" {
			t.Errorf("expected language zh-CN, got %s", p.languageCode)
		}
		if p.model != GoogleModelLatestLong {
			t.Errorf("expected model %s, got %s", GoogleModelLatestLong, p.model)
		}
		if !p.useEnhanced {
			t.Error("expected useEnhanced to be true")
		}
		if p.timeout != 30*time.Second {
			t.Errorf("expected 30s timeout, got %v", p.timeout)
		}
		if p.retries != 5 {
			t.Errorf("expected 5 retries, got %d", p.retries)
		}
		if p.credentialsJSON == "" {
			t.Error("expected credentials JSON to be stored")
		}
	})
}

func TestGoogleProviderTranscribe(t *testing.T) {
	t.Run("rejects empty audio", func(t *testing.T) {
		p, _ := NewGoogleProvider("test-key", withGoogleRecognizeClient(&fakeGoogleRecognizeClient{}))
		_, err := p.Transcribe(context.Background(), nil)
		if err == nil {
			t.Fatal("expected error for empty audio")
		}
	})

	t.Run("rejects audio too large", func(t *testing.T) {
		p, _ := NewGoogleProvider("test-key", withGoogleRecognizeClient(&fakeGoogleRecognizeClient{}))
		largeAudio := make([]byte, 101*1024*1024)
		_, err := p.Transcribe(context.Background(), largeAudio)
		if err == nil {
			t.Fatal("expected error for audio too large")
		}
		sttErr, ok := err.(*STTError)
		if !ok {
			t.Fatalf("expected *STTError, got %T", err)
		}
		if sttErr.Code != ErrAudioTooLarge {
			t.Errorf("expected ErrAudioTooLarge, got %s", sttErr.Code)
		}
	})

	t.Run("successful transcription and request mapping", func(t *testing.T) {
		fake := &fakeGoogleRecognizeClient{
			recognizeFn: func(ctx context.Context, req *speechpb.RecognizeRequest) (*speechpb.RecognizeResponse, error) {
				return &speechpb.RecognizeResponse{
					Results: []*speechpb.SpeechRecognitionResult{
						{
							Alternatives: []*speechpb.SpeechRecognitionAlternative{
								{
									Transcript: "Hello world",
									Confidence: 0.95,
									Words: []*speechpb.WordInfo{
										{
											Word:       "Hello",
											Confidence: 0.96,
											StartTime:  durationpb.New(0),
											EndTime:    durationpb.New(500 * time.Millisecond),
										},
										{
											Word:       "world",
											Confidence: 0.94,
											StartTime:  durationpb.New(600 * time.Millisecond),
											EndTime:    durationpb.New(time.Second),
										},
									},
								},
							},
							LanguageCode:  "en-US",
							ResultEndTime: durationpb.New(2500 * time.Millisecond),
						},
					},
				}, nil
			},
		}

		p, _ := NewGoogleProvider("test-key", withGoogleRecognizeClient(fake))
		result, err := p.Transcribe(context.Background(), []byte("fake-audio-data"),
			WithSTTLanguage("zh-CN"),
			WithSTTWordTimestamps(true),
			WithSTTMaxAlternatives(3),
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Text != "Hello world" {
			t.Errorf("expected 'Hello world', got '%s'", result.Text)
		}
		if result.Language != "en-US" {
			t.Errorf("expected language 'en-US', got '%s'", result.Language)
		}
		if result.Duration != time.Second {
			t.Errorf("expected duration 1s from word timestamps, got %v", result.Duration)
		}
		if len(result.Segments) != 1 {
			t.Fatalf("expected 1 segment, got %d", len(result.Segments))
		}
		if len(result.Segments[0].Words) != 2 {
			t.Fatalf("expected 2 words, got %d", len(result.Segments[0].Words))
		}
		if math.Abs(result.Confidence-0.95) > 0.0001 {
			t.Errorf("expected confidence 0.95, got %f", result.Confidence)
		}

		req := fake.lastRequest
		if req == nil {
			t.Fatal("expected request to be captured")
		}
		if req.GetConfig().GetLanguageCode() != "zh-CN" {
			t.Errorf("expected request language zh-CN, got %s", req.GetConfig().GetLanguageCode())
		}
		if !req.GetConfig().GetEnableWordTimeOffsets() {
			t.Error("expected EnableWordTimeOffsets to be true")
		}
		if req.GetConfig().GetMaxAlternatives() != 3 {
			t.Errorf("expected max alternatives 3, got %d", req.GetConfig().GetMaxAlternatives())
		}
		if req.GetConfig().GetEncoding() != speechpb.RecognitionConfig_MP3 {
			t.Errorf("expected MP3 encoding, got %v", req.GetConfig().GetEncoding())
		}
		if len(req.GetAudio().GetContent()) == 0 {
			t.Error("expected inline audio content to be populated")
		}
	})

	t.Run("multiple segments and alternatives", func(t *testing.T) {
		fake := &fakeGoogleRecognizeClient{
			recognizeFn: func(ctx context.Context, req *speechpb.RecognizeRequest) (*speechpb.RecognizeResponse, error) {
				return &speechpb.RecognizeResponse{
					Results: []*speechpb.SpeechRecognitionResult{
						{
							Alternatives: []*speechpb.SpeechRecognitionAlternative{
								{Transcript: "First segment", Confidence: 0.9},
								{Transcript: "First segments", Confidence: 0.7},
							},
							LanguageCode:  "en-US",
							ResultEndTime: durationpb.New(time.Second),
						},
						{
							Alternatives: []*speechpb.SpeechRecognitionAlternative{
								{Transcript: "Second segment", Confidence: 0.8},
								{Transcript: "Second segments", Confidence: 0.6},
							},
							LanguageCode:  "en-US",
							ResultEndTime: durationpb.New(2 * time.Second),
						},
					},
				}, nil
			},
		}

		p, _ := NewGoogleProvider("test-key", withGoogleRecognizeClient(fake))
		result, err := p.Transcribe(context.Background(), []byte("fake-audio"), WithSTTMaxAlternatives(3))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Text != "First segment Second segment" {
			t.Errorf("unexpected combined text: %s", result.Text)
		}
		if len(result.Segments) != 2 {
			t.Fatalf("expected 2 segments, got %d", len(result.Segments))
		}
		if len(result.Alternatives) != 2 {
			t.Fatalf("expected 2 alternatives, got %d", len(result.Alternatives))
		}
		if result.Duration != 2*time.Second {
			t.Errorf("expected duration 2s, got %v", result.Duration)
		}
	})

	t.Run("empty results", func(t *testing.T) {
		p, _ := NewGoogleProvider("test-key", withGoogleRecognizeClient(&fakeGoogleRecognizeClient{}))
		result, err := p.Transcribe(context.Background(), []byte("fake-audio"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Text != "" {
			t.Errorf("expected empty text, got %q", result.Text)
		}
	})

	t.Run("does not retry auth errors", func(t *testing.T) {
		fake := &fakeGoogleRecognizeClient{
			recognizeFn: func(ctx context.Context, req *speechpb.RecognizeRequest) (*speechpb.RecognizeResponse, error) {
				return nil, &googleapi.Error{Code: 401, Message: "invalid API key"}
			},
		}
		p, _ := NewGoogleProvider("bad-key", withGoogleRecognizeClient(fake), WithGoogleRetries(3))
		_, err := p.Transcribe(context.Background(), []byte("fake-audio"))
		if err == nil {
			t.Fatal("expected authentication error")
		}
		if fake.calls != 1 {
			t.Errorf("expected 1 call, got %d", fake.calls)
		}
	})

	t.Run("retries transient errors then succeeds", func(t *testing.T) {
		fake := &fakeGoogleRecognizeClient{}
		fake.recognizeFn = func(ctx context.Context, req *speechpb.RecognizeRequest) (*speechpb.RecognizeResponse, error) {
			if fake.calls == 1 {
				return nil, &googleapi.Error{Code: 503, Message: "service unavailable"}
			}
			return &speechpb.RecognizeResponse{
				Results: []*speechpb.SpeechRecognitionResult{
					{
						Alternatives:  []*speechpb.SpeechRecognitionAlternative{{Transcript: "Success after retry"}},
						LanguageCode:  "en-US",
						ResultEndTime: durationpb.New(time.Second),
					},
				},
			}, nil
		}

		p, _ := NewGoogleProvider("test-key", withGoogleRecognizeClient(fake), WithGoogleRetries(2))
		result, err := p.Transcribe(context.Background(), []byte("fake-audio"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Text != "Success after retry" {
			t.Errorf("expected 'Success after retry', got '%s'", result.Text)
		}
		if fake.calls != 2 {
			t.Errorf("expected 2 calls, got %d", fake.calls)
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		fake := &fakeGoogleRecognizeClient{
			recognizeFn: func(ctx context.Context, req *speechpb.RecognizeRequest) (*speechpb.RecognizeResponse, error) {
				return nil, context.Canceled
			},
		}
		p, _ := NewGoogleProvider("test-key", withGoogleRecognizeClient(fake), WithGoogleRetries(0))
		_, err := p.Transcribe(context.Background(), []byte("fake-audio"))
		if err == nil {
			t.Fatal("expected context cancellation error")
		}
	})
}

func TestGoogleProviderTranscribeStream(t *testing.T) {
	t.Run("rejects nil reader", func(t *testing.T) {
		p, _ := NewGoogleProvider("test-key", withGoogleRecognizeClient(&fakeGoogleRecognizeClient{}))
		_, err := p.TranscribeStream(context.Background(), nil)
		if err == nil {
			t.Fatal("expected error for nil reader")
		}
	})

	t.Run("successful stream transcription", func(t *testing.T) {
		fake := &fakeGoogleRecognizeClient{
			recognizeFn: func(ctx context.Context, req *speechpb.RecognizeRequest) (*speechpb.RecognizeResponse, error) {
				return &speechpb.RecognizeResponse{
					Results: []*speechpb.SpeechRecognitionResult{
						{
							Alternatives: []*speechpb.SpeechRecognitionAlternative{{Transcript: "Stream content", Confidence: 0.9}},
							LanguageCode: "en-US",
						},
					},
				}, nil
			},
		}
		p, _ := NewGoogleProvider("test-key", withGoogleRecognizeClient(fake))
		reader := strings.NewReader("stream-audio-data")
		result, err := p.TranscribeStream(context.Background(), reader)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Text != "Stream content" {
			t.Errorf("expected 'Stream content', got '%s'", result.Text)
		}
	})
}

func TestGoogleProviderTranscribeFile(t *testing.T) {
	p, _ := NewGoogleProvider("test-key", withGoogleRecognizeClient(&fakeGoogleRecognizeClient{}))
	_, err := p.TranscribeFile(context.Background(), "/some/file.mp3")
	if err == nil {
		t.Fatal("expected error for file transcription")
	}
	sttErr, ok := err.(*STTError)
	if !ok {
		t.Fatalf("expected *STTError, got %T", err)
	}
	if sttErr.Code != ErrProviderNotSupported {
		t.Errorf("expected ErrProviderNotSupported, got %s", sttErr.Code)
	}
}

func TestGoogleProviderListLanguages(t *testing.T) {
	p, _ := NewGoogleProvider("test-key", withGoogleRecognizeClient(&fakeGoogleRecognizeClient{}))
	langs, err := p.ListLanguages(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(langs) == 0 {
		t.Fatal("expected non-empty language list")
	}
}

func TestGoogleProviderEncodingMapping(t *testing.T) {
	p, _ := NewGoogleProvider("test-key", withGoogleRecognizeClient(&fakeGoogleRecognizeClient{}))

	tests := []struct {
		format AudioInputFormat
		want   RecognitionEncoding
	}{
		{InputWAV, EncodingLinear16},
		{InputPCM, EncodingLinear16},
		{InputFLAC, EncodingFLAC},
		{InputMP3, EncodingMP3},
		{InputOGG, EncodingOGGOpus},
		{InputWEBM, EncodingWEBMOpus},
		{InputM4A, EncodingWEBMOpus},
		{InputMP4, EncodingWEBMOpus},
		{InputMPEG, EncodingMP3},
		{InputMPGA, EncodingMP3},
	}

	for _, tt := range tests {
		t.Run(string(tt.format), func(t *testing.T) {
			got := p.mapInputFormatToEncoding(tt.format)
			if got != tt.want {
				t.Errorf("mapInputFormatToEncoding(%s) = %s, want %s", tt.format, got, tt.want)
			}
		})
	}
}

func TestGoogleProviderSampleRateGuessing(t *testing.T) {
	p, _ := NewGoogleProvider("test-key", withGoogleRecognizeClient(&fakeGoogleRecognizeClient{}))

	tests := []struct {
		format AudioInputFormat
		want   int32
	}{
		{InputWAV, 16000},
		{InputPCM, 16000},
		{InputFLAC, 16000},
		{InputMP3, 16000},
		{InputOGG, 48000},
		{InputWEBM, 48000},
		{InputM4A, 44100},
		{InputMP4, 44100},
	}

	for _, tt := range tests {
		t.Run(string(tt.format), func(t *testing.T) {
			got := p.guessSampleRate(tt.format)
			if got != tt.want {
				t.Errorf("guessSampleRate(%s) = %d, want %d", tt.format, got, tt.want)
			}
		})
	}
}

func TestParseProtoDuration(t *testing.T) {
	tests := []struct {
		name string
		d    *durationpb.Duration
		want time.Duration
	}{
		{"nil", nil, 0},
		{"zero", durationpb.New(0), 0},
		{"one second", durationpb.New(time.Second), time.Second},
		{"500ms", durationpb.New(500 * time.Millisecond), 500 * time.Millisecond},
		{"2.5s", durationpb.New(2500 * time.Millisecond), 2500 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseProtoDuration(tt.d)
			if got != tt.want {
				t.Errorf("parseProtoDuration(%v) = %v, want %v", tt.d, got, tt.want)
			}
		})
	}
}

func TestNewSTTProviderGoogle(t *testing.T) {
	p, err := NewSTTProvider(STTConfig{
		Type:    STTProviderGoogle,
		APIKey:  "test-key",
		Timeout: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Type() != STTProviderGoogle {
		t.Errorf("expected STTProviderGoogle, got %s", p.Type())
	}
	if p.Name() != "google-speech" {
		t.Errorf("expected name 'google-speech', got %s", p.Name())
	}
}

func TestGoogleSTTManager(t *testing.T) {
	m := NewSTTManager()
	p, _ := NewGoogleProvider("test-key", withGoogleRecognizeClient(&fakeGoogleRecognizeClient{}))

	err := m.Register("google", p)
	if err != nil {
		t.Fatalf("failed to register provider: %v", err)
	}

	got, err := m.Get("google")
	if err != nil {
		t.Fatalf("failed to get provider: %v", err)
	}
	if got.Type() != STTProviderGoogle {
		t.Errorf("expected STTProviderGoogle, got %s", got.Type())
	}
}

func TestGoogleProviderHandleClientError(t *testing.T) {
	p, _ := NewGoogleProvider("test-key", withGoogleRecognizeClient(&fakeGoogleRecognizeClient{}))

	tests := []struct {
		name string
		err  error
		want STTErrorCode
	}{
		{"bad request", &googleapi.Error{Code: 400, Message: "bad request"}, ErrAudioFormatInvalid},
		{"unauthorized", &googleapi.Error{Code: 401, Message: "unauthorized"}, ErrAuthentication},
		{"forbidden", &googleapi.Error{Code: 403, Message: "forbidden"}, ErrAuthentication},
		{"rate limited", &googleapi.Error{Code: 429, Message: "quota"}, ErrRateLimited},
		{"generic", errors.New("boom"), ErrTranscriptionFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := p.handleClientError(tt.err)
			sttErr, ok := err.(*STTError)
			if !ok {
				t.Fatalf("expected *STTError, got %T", err)
			}
			if sttErr.Code != tt.want {
				t.Errorf("expected %s, got %s", tt.want, sttErr.Code)
			}
		})
	}
}
