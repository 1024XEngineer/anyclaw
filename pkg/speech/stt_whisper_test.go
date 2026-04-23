package speech

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewWhisperProvider(t *testing.T) {
	t.Run("requires API key", func(t *testing.T) {
		_, err := NewWhisperProvider("")
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
		p, err := NewWhisperProvider("test-key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Name() != "openai-whisper" {
			t.Errorf("expected name openai-whisper, got %s", p.Name())
		}
		if p.Type() != STTProviderOpenAI {
			t.Errorf("expected type %s, got %s", STTProviderOpenAI, p.Type())
		}
		if p.baseURL != "https://api.openai.com" {
			t.Errorf("expected default baseURL, got %s", p.baseURL)
		}
		if p.model != WhisperModelV1 {
			t.Errorf("expected default model %s, got %s", WhisperModelV1, p.model)
		}
		if p.retries != 2 {
			t.Errorf("expected 2 retries, got %d", p.retries)
		}
	})

	t.Run("applies options", func(t *testing.T) {
		p, err := NewWhisperProvider("test-key",
			WithWhisperBaseURL("https://custom.api.com"),
			WithWhisperModel(WhisperModelV1),
			WithWhisperLanguage("zh"),
			WithWhisperTimeout(30*time.Second),
			WithWhisperRetries(5),
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.baseURL != "https://custom.api.com" {
			t.Errorf("expected custom baseURL, got %s", p.baseURL)
		}
		if p.language != "zh" {
			t.Errorf("expected language zh, got %s", p.language)
		}
		if p.timeout != 30*time.Second {
			t.Errorf("expected 30s timeout, got %v", p.timeout)
		}
		if p.retries != 5 {
			t.Errorf("expected 5 retries, got %d", p.retries)
		}
	})

	t.Run("rejects invalid model", func(t *testing.T) {
		_, err := NewWhisperProvider("test-key", WithWhisperModel(WhisperModel("invalid-model")))
		if err == nil {
			t.Fatal("expected error for invalid model")
		}
	})

	t.Run("trims trailing slash from baseURL", func(t *testing.T) {
		p, err := NewWhisperProvider("test-key", WithWhisperBaseURL("https://api.example.com/"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.HasSuffix(p.baseURL, "/") {
			t.Errorf("baseURL should not have trailing slash: %s", p.baseURL)
		}
	})
}

func TestWhisperProviderTranscribe(t *testing.T) {
	t.Run("rejects empty audio", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		defer server.Close()

		p, _ := NewWhisperProvider("test-key", WithWhisperBaseURL(server.URL))
		_, err := p.Transcribe(context.Background(), nil)
		if err == nil {
			t.Fatal("expected error for empty audio")
		}
	})

	t.Run("rejects audio too large", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		defer server.Close()

		p, _ := NewWhisperProvider("test-key", WithWhisperBaseURL(server.URL))
		largeAudio := make([]byte, 26*1024*1024)
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
		response := whisperResponse{
			Text:     "Hello world",
			Language: "en",
			Duration: 2.5,
			Segments: []struct {
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
			}{
				{
					ID:         0,
					Start:      0.0,
					End:        2.5,
					Text:       "Hello world",
					AvgLogProb: -0.3,
					Words: []struct {
						Word       string  `json:"word"`
						Start      float64 `json:"start"`
						End        float64 `json:"end"`
						Confidence float64 `json:"probability"`
					}{
						{Word: "Hello", Start: 0.0, End: 0.5, Confidence: 0.95},
						{Word: "world", Start: 0.6, End: 1.0, Confidence: 0.92},
					},
				},
			},
		}

		respBody, _ := json.Marshal(response)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				t.Errorf("expected POST, got %s", r.Method)
			}
			if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
				t.Error("missing Bearer token")
			}
			if !strings.Contains(r.Header.Get("Content-Type"), "multipart/form-data") {
				t.Errorf("expected multipart/form-data, got %s", r.Header.Get("Content-Type"))
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(respBody)
		}))
		defer server.Close()

		p, _ := NewWhisperProvider("test-key", WithWhisperBaseURL(server.URL))
		result, err := p.Transcribe(context.Background(), []byte("fake-audio-data"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Text != "Hello world" {
			t.Errorf("expected 'Hello world', got '%s'", result.Text)
		}
		if result.Language != "en" {
			t.Errorf("expected language 'en', got '%s'", result.Language)
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
	})

	t.Run("handles authentication error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":{"message":"Invalid API key","type":"invalid_request_error","code":"invalid_api_key"}}`))
		}))
		defer server.Close()

		p, _ := NewWhisperProvider("bad-key", WithWhisperBaseURL(server.URL), WithWhisperRetries(0))
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
			w.Write([]byte(`{"error":{"message":"Rate limit exceeded","type":"rate_limit_error","code":"rate_limit_exceeded"}}`))
		}))
		defer server.Close()

		p, _ := NewWhisperProvider("test-key", WithWhisperBaseURL(server.URL), WithWhisperRetries(0))
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

		p, _ := NewWhisperProvider("test-key", WithWhisperBaseURL(server.URL), WithWhisperRetries(1))

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := p.Transcribe(ctx, []byte("fake-audio"))
		if err == nil {
			t.Fatal("expected context cancellation error")
		}
	})

	t.Run("translation mode", func(t *testing.T) {
		var receivedURL string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedURL = r.URL.Path
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"text":"Hello world","language":"en","duration":1.0}`))
		}))
		defer server.Close()

		p, _ := NewWhisperProvider("test-key", WithWhisperBaseURL(server.URL))
		_, err := p.Transcribe(context.Background(), []byte("fake-audio"),
			WithSTTMode(ModeTranslation))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if receivedURL != "/v1/audio/translations" {
			t.Errorf("expected /v1/audio/translations, got %s", receivedURL)
		}
	})
}

func TestWhisperProviderTranscribeFile(t *testing.T) {
	t.Run("rejects empty file path", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		defer server.Close()

		p, _ := NewWhisperProvider("test-key", WithWhisperBaseURL(server.URL))
		_, err := p.TranscribeFile(context.Background(), "")
		if err == nil {
			t.Fatal("expected error for empty file path")
		}
	})

	t.Run("rejects non-existent file", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		defer server.Close()

		p, _ := NewWhisperProvider("test-key", WithWhisperBaseURL(server.URL))
		_, err := p.TranscribeFile(context.Background(), "/nonexistent/file.mp3")
		if err == nil {
			t.Fatal("expected error for non-existent file")
		}
		sttErr, ok := err.(*STTError)
		if !ok {
			t.Fatalf("expected *STTError, got %T", err)
		}
		if sttErr.Code != ErrAudioFormatInvalid {
			t.Errorf("expected ErrAudioFormatInvalid, got %s", sttErr.Code)
		}
	})

	t.Run("successful file transcription", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "test.mp3")
		os.WriteFile(testFile, []byte("fake-audio-content"), 0644)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"text":"File content","language":"en","duration":1.5}`))
		}))
		defer server.Close()

		p, _ := NewWhisperProvider("test-key", WithWhisperBaseURL(server.URL))
		result, err := p.TranscribeFile(context.Background(), testFile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Text != "File content" {
			t.Errorf("expected 'File content', got '%s'", result.Text)
		}
	})
}

func TestWhisperProviderTranscribeStream(t *testing.T) {
	t.Run("rejects nil reader", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		defer server.Close()

		p, _ := NewWhisperProvider("test-key", WithWhisperBaseURL(server.URL))
		_, err := p.TranscribeStream(context.Background(), nil)
		if err == nil {
			t.Fatal("expected error for nil reader")
		}
	})

	t.Run("successful stream transcription", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"text":"Stream content","language":"en","duration":3.0}`))
		}))
		defer server.Close()

		p, _ := NewWhisperProvider("test-key", WithWhisperBaseURL(server.URL))
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

func TestWhisperProviderListLanguages(t *testing.T) {
	p, _ := NewWhisperProvider("test-key")
	langs, err := p.ListLanguages(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(langs) == 0 {
		t.Fatal("expected non-empty language list")
	}

	found := false
	for _, lang := range langs {
		if lang == "en" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'en' in language list")
	}

	found = false
	for _, lang := range langs {
		if lang == "zh" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'zh' in language list")
	}
}

func TestWhisperProviderSSE(t *testing.T) {
	t.Run("successful streaming", func(t *testing.T) {
		sseData := `data: {"text":"Hello ","language":"en"}
data: {"text":"world","language":"en"}
data: [DONE]
`
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Accept") != "text/event-stream" {
				t.Errorf("expected text/event-stream accept header, got %s", r.Header.Get("Accept"))
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.Write([]byte(sseData))
		}))
		defer server.Close()

		p, _ := NewWhisperProvider("test-key", WithWhisperBaseURL(server.URL))

		var chunks []*TranscriptResult
		err := p.TranscribeSSE(context.Background(), []byte("fake-audio"), func(chunk *TranscriptResult) {
			chunks = append(chunks, chunk)
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(chunks) != 2 {
			t.Fatalf("expected 2 chunks, got %d", len(chunks))
		}
		if chunks[0].Text != "Hello " {
			t.Errorf("expected first chunk 'Hello ', got '%s'", chunks[0].Text)
		}
		if chunks[1].Text != "Hello world" {
			t.Errorf("expected accumulated text 'Hello world', got '%s'", chunks[1].Text)
		}
	})
}

func TestNormalizeLogProb(t *testing.T) {
	tests := []struct {
		name    string
		logProb float64
		want    float64
	}{
		{"zero", 0, 1.0},
		{"positive", 0.5, 1.0},
		{"negative small", -0.1, 0.9090909090909091},
		{"negative large", -5.0, 0.16666666666666666},
		{"negative very large", -100.0, 0.009900990099009901},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeLogProb(tt.logProb)
			if got < 0 || got > 1 {
				t.Errorf("normalizeLogProb(%f) = %f, should be in [0,1]", tt.logProb, got)
			}
		})
	}
}

func TestSTTError(t *testing.T) {
	t.Run("error message", func(t *testing.T) {
		err := NewSTTError(ErrAuthentication, "test message")
		if err.Error() != "authentication_failed: test message" {
			t.Errorf("unexpected error message: %s", err.Error())
		}
	})

	t.Run("error with format", func(t *testing.T) {
		err := NewSTTErrorf(ErrTranscriptionFailed, "retry %d failed", 3)
		if err.Error() != "transcription_failed: retry 3 failed" {
			t.Errorf("unexpected error message: %s", err.Error())
		}
	})

	t.Run("error with wrapped error", func(t *testing.T) {
		inner := context.DeadlineExceeded
		err := &STTError{
			Code:    ErrTranscriptionFailed,
			Message: "timeout",
			Err:     inner,
		}
		if !strings.Contains(err.Error(), "context deadline exceeded") {
			t.Errorf("expected wrapped error in message: %s", err.Error())
		}
		if err.Unwrap() != inner {
			t.Error("Unwrap() did not return wrapped error")
		}
	})
}

func TestNewSTTProvider(t *testing.T) {
	t.Run("creates OpenAI provider", func(t *testing.T) {
		p, err := NewSTTProvider(STTConfig{
			Type:    STTProviderOpenAI,
			APIKey:  "test-key",
			Model:   "whisper-1",
			Timeout: 30 * time.Second,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Type() != STTProviderOpenAI {
			t.Errorf("expected STTProviderOpenAI, got %s", p.Type())
		}
	})

	t.Run("rejects unknown provider", func(t *testing.T) {
		_, err := NewSTTProvider(STTConfig{
			Type:   STTProviderType("unknown"),
			APIKey: "test-key",
		})
		if err == nil {
			t.Fatal("expected error for unknown provider")
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

func TestWhisperProviderRetries(t *testing.T) {
	t.Run("retries on server error then succeeds", func(t *testing.T) {
		callCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			if callCount < 2 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"text":"Success after retry","language":"en","duration":1.0}`))
		}))
		defer server.Close()

		p, _ := NewWhisperProvider("test-key", WithWhisperBaseURL(server.URL), WithWhisperRetries(2))
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
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":{"message":"Invalid key","type":"auth_error","code":"bad_key"}}`))
		}))
		defer server.Close()

		p, _ := NewWhisperProvider("bad-key", WithWhisperBaseURL(server.URL), WithWhisperRetries(3))
		_, err := p.Transcribe(context.Background(), []byte("fake-audio"))
		if err == nil {
			t.Fatal("expected error")
		}
		if callCount != 1 {
			t.Errorf("expected 1 call (no retry on auth error), got %d", callCount)
		}
	})
}

func TestWhisperProviderValidation(t *testing.T) {
	t.Run("rejects invalid temperature", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		defer server.Close()

		p, _ := NewWhisperProvider("test-key", WithWhisperBaseURL(server.URL))
		_, err := p.Transcribe(context.Background(), []byte("fake-audio"),
			WithSTTTemperature(1.5))
		if err == nil {
			t.Fatal("expected error for invalid temperature")
		}
	})

	t.Run("rejects negative max alternatives", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		defer server.Close()

		p, _ := NewWhisperProvider("test-key", WithWhisperBaseURL(server.URL))
		_, err := p.Transcribe(context.Background(), []byte("fake-audio"),
			WithSTTMaxAlternatives(-1))
		if err == nil {
			t.Fatal("expected error for negative max alternatives")
		}
	})

	t.Run("rejects unsupported input format", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		defer server.Close()

		p, _ := NewWhisperProvider("test-key", WithWhisperBaseURL(server.URL))
		_, err := p.Transcribe(context.Background(), []byte("fake-audio"),
			WithSTTInputFormat(AudioInputFormat("xyz")))
		if err == nil {
			t.Fatal("expected error for unsupported format")
		}
	})
}
