package speech

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewGoogleProvider(t *testing.T) {
	t.Run("requires API key", func(t *testing.T) {
		_, err := NewGoogleProvider("")
		if err == nil {
			t.Fatal("expected error when API key is empty")
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
		p, err := NewGoogleProvider("test-key")
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
	})

	t.Run("applies options", func(t *testing.T) {
		p, err := NewGoogleProvider("test-key",
			WithGoogleBaseURL("https://custom.speech.api.com"),
			WithGoogleLanguageCode("zh-CN"),
			WithGoogleModel(GoogleModelLatestLong),
			WithGoogleEnhanced(true),
			WithGoogleTimeout(30*time.Second),
			WithGoogleRetries(5),
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
	})

	t.Run("trims trailing slash from baseURL", func(t *testing.T) {
		p, err := NewGoogleProvider("test-key", WithGoogleBaseURL("https://api.example.com/"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.HasSuffix(p.baseURL, "/") {
			t.Errorf("baseURL should not have trailing slash: %s", p.baseURL)
		}
	})
}

func TestGoogleProviderTranscribe(t *testing.T) {
	t.Run("rejects empty audio", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		defer server.Close()

		p, _ := NewGoogleProvider("test-key", WithGoogleBaseURL(server.URL))
		_, err := p.Transcribe(context.Background(), nil)
		if err == nil {
			t.Fatal("expected error for empty audio")
		}
	})

	t.Run("rejects audio too large", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		defer server.Close()

		p, _ := NewGoogleProvider("test-key", WithGoogleBaseURL(server.URL))
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

	t.Run("successful transcription", func(t *testing.T) {
		response := googleResponse{
			Results: []googleResult{
				{
					Alternatives: []googleAlternative{
						{
							Transcript: "Hello world",
							Confidence: 0.95,
							Words: []googleWordInfo{
								{Word: "Hello", Confidence: 0.96, StartTime: googleDuration{Seconds: "0", Nanos: 0}, EndTime: googleDuration{Seconds: "0", Nanos: 500000000}},
								{Word: "world", Confidence: 0.94, StartTime: googleDuration{Seconds: "0", Nanos: 600000000}, EndTime: googleDuration{Seconds: "1", Nanos: 0}},
							},
						},
					},
					LanguageCode: "en-US",
					ResultEndTime: struct {
						Seconds string `json:"seconds"`
						Nanos   int    `json:"nanos"`
					}{Seconds: "2", Nanos: 500000000},
				},
			},
		}

		respBody, _ := json.Marshal(response)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				t.Errorf("expected POST, got %s", r.Method)
			}
			if !strings.Contains(r.URL.Query().Get("key"), "test-key") {
				t.Error("missing API key in query")
			}
			if r.Header.Get("Content-Type") != "application/json" {
				t.Errorf("expected application/json, got %s", r.Header.Get("Content-Type"))
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(respBody)
		}))
		defer server.Close()

		p, _ := NewGoogleProvider("test-key", WithGoogleBaseURL(server.URL))
		result, err := p.Transcribe(context.Background(), []byte("fake-audio-data"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Text != "Hello world" {
			t.Errorf("expected 'Hello world', got '%s'", result.Text)
		}
		if result.Language != "en-US" {
			t.Errorf("expected language 'en-US', got '%s'", result.Language)
		}
		if result.Duration != 2500*time.Millisecond {
			t.Errorf("expected duration 2.5s, got %v", result.Duration)
		}
		if len(result.Segments) != 1 {
			t.Fatalf("expected 1 segment, got %d", len(result.Segments))
		}
		if len(result.Segments[0].Words) != 2 {
			t.Fatalf("expected 2 words, got %d", len(result.Segments[0].Words))
		}
		if result.Confidence != 0.95 {
			t.Errorf("expected confidence 0.95, got %f", result.Confidence)
		}
	})

	t.Run("multiple segments", func(t *testing.T) {
		response := googleResponse{
			Results: []googleResult{
				{Alternatives: []googleAlternative{{Transcript: "First segment"}}, LanguageCode: "en-US"},
				{Alternatives: []googleAlternative{{Transcript: "Second segment"}}, LanguageCode: "en-US"},
			},
		}

		respBody, _ := json.Marshal(response)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write(respBody)
		}))
		defer server.Close()

		p, _ := NewGoogleProvider("test-key", WithGoogleBaseURL(server.URL))
		result, err := p.Transcribe(context.Background(), []byte("fake-audio"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expectedText := "First segment Second segment"
		if result.Text != expectedText {
			t.Errorf("expected '%s', got '%s'", expectedText, result.Text)
		}
		if len(result.Segments) != 2 {
			t.Fatalf("expected 2 segments, got %d", len(result.Segments))
		}
	})

	t.Run("alternatives", func(t *testing.T) {
		response := googleResponse{
			Results: []googleResult{
				{
					Alternatives: []googleAlternative{
						{Transcript: "Hello world", Confidence: 0.95},
						{Transcript: "Hello word", Confidence: 0.80},
						{Transcript: "Halo world", Confidence: 0.70},
					},
					LanguageCode: "en-US",
				},
			},
		}

		respBody, _ := json.Marshal(response)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write(respBody)
		}))
		defer server.Close()

		p, _ := NewGoogleProvider("test-key", WithGoogleBaseURL(server.URL))
		result, err := p.Transcribe(context.Background(), []byte("fake-audio"),
			WithSTTMaxAlternatives(3))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result.Alternatives) != 2 {
			t.Fatalf("expected 2 alternatives, got %d", len(result.Alternatives))
		}
		if result.Alternatives[0] != "Hello word" {
			t.Errorf("expected first alternative 'Hello word', got '%s'", result.Alternatives[0])
		}
	})

	t.Run("empty results", func(t *testing.T) {
		response := googleResponse{Results: []googleResult{}}
		respBody, _ := json.Marshal(response)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write(respBody)
		}))
		defer server.Close()

		p, _ := NewGoogleProvider("test-key", WithGoogleBaseURL(server.URL))
		result, err := p.Transcribe(context.Background(), []byte("fake-audio"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Text != "" {
			t.Errorf("expected empty text, got '%s'", result.Text)
		}
	})

	t.Run("handles authentication error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":{"code":401,"message":"API key not valid. Please pass a valid API key.","status":"UNAUTHENTICATED"}}`))
		}))
		defer server.Close()

		p, _ := NewGoogleProvider("bad-key", WithGoogleBaseURL(server.URL), WithGoogleRetries(0))
		_, err := p.Transcribe(context.Background(), []byte("fake-audio"))
		if err == nil {
			t.Fatal("expected authentication error")
		}
		sttErr, ok := err.(*STTError)
		if !ok {
			t.Fatalf("expected *STTError, got %T", err)
		}
		if sttErr.Code != ErrAuthentication {
			t.Errorf("expected ErrAuthentication, got %s", sttErr.Code)
		}
	})

	t.Run("handles forbidden error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"error":{"code":403,"message":"API key expired.","status":"PERMISSION_DENIED"}}`))
		}))
		defer server.Close()

		p, _ := NewGoogleProvider("expired-key", WithGoogleBaseURL(server.URL), WithGoogleRetries(0))
		_, err := p.Transcribe(context.Background(), []byte("fake-audio"))
		if err == nil {
			t.Fatal("expected authentication error")
		}
		sttErr, ok := err.(*STTError)
		if !ok {
			t.Fatalf("expected *STTError, got %T", err)
		}
		if sttErr.Code != ErrAuthentication {
			t.Errorf("expected ErrAuthentication, got %s", sttErr.Code)
		}
	})

	t.Run("handles rate limit error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":{"code":429,"message":"Quota exceeded.","status":"RESOURCE_EXHAUSTED"}}`))
		}))
		defer server.Close()

		p, _ := NewGoogleProvider("test-key", WithGoogleBaseURL(server.URL), WithGoogleRetries(0))
		_, err := p.Transcribe(context.Background(), []byte("fake-audio"))
		if err == nil {
			t.Fatal("expected rate limit error")
		}
		sttErr, ok := err.(*STTError)
		if !ok {
			t.Fatalf("expected *STTError, got %T", err)
		}
		if sttErr.Code != ErrRateLimited {
			t.Errorf("expected ErrRateLimited, got %s", sttErr.Code)
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer server.Close()

		p, _ := NewGoogleProvider("test-key", WithGoogleBaseURL(server.URL), WithGoogleRetries(1))

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := p.Transcribe(ctx, []byte("fake-audio"))
		if err == nil {
			t.Fatal("expected context cancellation error")
		}
	})

	t.Run("uses correct URL with API key", func(t *testing.T) {
		var receivedURL string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedURL = r.URL.String()
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"results":[{"alternatives":[{"transcript":"test"}],"languageCode":"en-US"}]}`))
		}))
		defer server.Close()

		p, _ := NewGoogleProvider("my-api-key", WithGoogleBaseURL(server.URL))
		_, err := p.Transcribe(context.Background(), []byte("fake-audio"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(receivedURL, "key=my-api-key") {
			t.Errorf("expected URL to contain 'key=my-api-key', got %s", receivedURL)
		}
		if !strings.Contains(receivedURL, "/v1/speech:recognize") {
			t.Errorf("expected URL to contain '/v1/speech:recognize', got %s", receivedURL)
		}
	})

	t.Run("sends correct request body", func(t *testing.T) {
		var receivedBody googleRecognizeRequest
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewDecoder(r.Body).Decode(&receivedBody)
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"results":[{"alternatives":[{"transcript":"test"}],"languageCode":"en-US"}]}`))
		}))
		defer server.Close()

		p, _ := NewGoogleProvider("test-key", WithGoogleBaseURL(server.URL))
		_, err := p.Transcribe(context.Background(), []byte("fake-audio"),
			WithSTTLanguage("zh-CN"),
			WithSTTWordTimestamps(true),
			WithSTTMaxAlternatives(3))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if receivedBody.Config.LanguageCode != "zh-CN" {
			t.Errorf("expected language zh-CN, got %s", receivedBody.Config.LanguageCode)
		}
		if !receivedBody.Config.EnableWordTimeOffsets {
			t.Error("expected EnableWordTimeOffsets to be true")
		}
		if receivedBody.Config.MaxAlternatives != 3 {
			t.Errorf("expected maxAlternatives 3, got %d", receivedBody.Config.MaxAlternatives)
		}
		if receivedBody.Config.EnableAutomaticPunctuation != true {
			t.Error("expected EnableAutomaticPunctuation to be true")
		}
	})
}

func TestGoogleProviderTranscribeStream(t *testing.T) {
	t.Run("rejects nil reader", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		defer server.Close()

		p, _ := NewGoogleProvider("test-key", WithGoogleBaseURL(server.URL))
		_, err := p.TranscribeStream(context.Background(), nil)
		if err == nil {
			t.Fatal("expected error for nil reader")
		}
	})

	t.Run("successful stream transcription", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"results":[{"alternatives":[{"transcript":"Stream content","confidence":0.9}],"languageCode":"en-US"}]}`))
		}))
		defer server.Close()

		p, _ := NewGoogleProvider("test-key", WithGoogleBaseURL(server.URL))
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
	t.Run("returns not supported error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		defer server.Close()

		p, _ := NewGoogleProvider("test-key", WithGoogleBaseURL(server.URL))
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
	})
}

func TestGoogleProviderListLanguages(t *testing.T) {
	p, _ := NewGoogleProvider("test-key")
	langs, err := p.ListLanguages(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(langs) == 0 {
		t.Fatal("expected non-empty language list")
	}

	found := false
	for _, lang := range langs {
		if lang == "en-US" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'en-US' in language list")
	}

	found = false
	for _, lang := range langs {
		if lang == "zh-CN" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'zh-CN' in language list")
	}
}

func TestGoogleProviderEncodingMapping(t *testing.T) {
	p, _ := NewGoogleProvider("test-key")

	tests := []struct {
		format AudioInputFormat
		want   RecognitionEncoding
	}{
		{InputWAV, EncodingLinear16},
		{InputPCM, EncodingLinear16},
		{InputFLAC, EncodingFLAC},
		{InputMP3, EncodingMP3},
		{InputOGG, EncodingWEBMOpus},
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
	p, _ := NewGoogleProvider("test-key")

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

func TestGoogleProviderRetries(t *testing.T) {
	t.Run("retries on server error then succeeds", func(t *testing.T) {
		callCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			if callCount < 2 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte(`{"error":{"code":503,"message":"Service unavailable","status":"UNAVAILABLE"}}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"results":[{"alternatives":[{"transcript":"Success after retry"}],"languageCode":"en-US"}]}`))
		}))
		defer server.Close()

		p, _ := NewGoogleProvider("test-key", WithGoogleBaseURL(server.URL), WithGoogleRetries(2))
		result, err := p.Transcribe(context.Background(), []byte("fake-audio"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Text != "Success after retry" {
			t.Errorf("expected 'Success after retry', got '%s'", result.Text)
		}
		if callCount != 2 {
			t.Errorf("expected 2 calls, got %d", callCount)
		}
	})

	t.Run("does not retry on auth error", func(t *testing.T) {
		callCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":{"code":401,"message":"Invalid API key","status":"UNAUTHENTICATED"}}`))
		}))
		defer server.Close()

		p, _ := NewGoogleProvider("bad-key", WithGoogleBaseURL(server.URL), WithGoogleRetries(3))
		_, err := p.Transcribe(context.Background(), []byte("fake-audio"))
		if err == nil {
			t.Fatal("expected error")
		}
		if callCount != 1 {
			t.Errorf("expected 1 call (no retry on auth error), got %d", callCount)
		}
	})
}

func TestParseGoogleDuration(t *testing.T) {
	tests := []struct {
		name string
		d    googleDuration
		want time.Duration
	}{
		{"zero", googleDuration{Seconds: "0", Nanos: 0}, 0},
		{"one second", googleDuration{Seconds: "1", Nanos: 0}, time.Second},
		{"500ms", googleDuration{Seconds: "0", Nanos: 500000000}, 500 * time.Millisecond},
		{"2.5s", googleDuration{Seconds: "2", Nanos: 500000000}, 2500 * time.Millisecond},
		{"1.234s", googleDuration{Seconds: "1", Nanos: 234000000}, time.Second + 234*time.Millisecond},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseGoogleDuration(tt.d)
			if got != tt.want {
				t.Errorf("parseGoogleDuration(%v) = %v, want %v", tt.d, got, tt.want)
			}
		})
	}
}

func TestNewSTTProviderGoogle(t *testing.T) {
	t.Run("creates Google provider", func(t *testing.T) {
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
	})

	t.Run("creates Google provider with language", func(t *testing.T) {
		p, err := NewSTTProvider(STTConfig{
			Type:     STTProviderGoogle,
			APIKey:   "test-key",
			Language: "zh-CN",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		gp, ok := p.(*GoogleProvider)
		if !ok {
			t.Fatalf("expected *GoogleProvider, got %T", p)
		}
		if gp.languageCode != "zh-CN" {
			t.Errorf("expected language zh-CN, got %s", gp.languageCode)
		}
	})
}

func TestGoogleSTTManager(t *testing.T) {
	t.Run("register and use Google provider", func(t *testing.T) {
		m := NewSTTManager()
		p, _ := NewGoogleProvider("test-key")

		err := m.Register("google", p)
		if err != nil {
			t.Fatalf("failed to register provider: %v", err)
		}

		providers := m.ListProviders()
		if len(providers) != 1 {
			t.Fatalf("expected 1 provider, got %d", len(providers))
		}

		got, err := m.Get("google")
		if err != nil {
			t.Fatalf("failed to get provider: %v", err)
		}
		if got.Type() != STTProviderGoogle {
			t.Errorf("expected STTProviderGoogle, got %s", got.Type())
		}
	})
}
