package speech

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpenAIProviderHTTPAndHelpers(t *testing.T) {
	var payloads []map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/speech" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer openai-key" {
			t.Fatalf("unexpected auth header: %s", got)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll(request): %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("Unmarshal(request): %v", err)
		}
		payloads = append(payloads, payload)

		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("openai-audio"))
	}))
	defer server.Close()

	provider, err := NewOpenAIProvider(
		"openai-key",
		WithOpenAIBaseURL(server.URL),
		WithOpenAITimeout(time.Second),
		WithOpenAIRetries(0),
		WithOpenAIVoice("alloy"),
		WithOpenAIModel(OpenAITTS1HD),
	)
	if err != nil {
		t.Fatalf("NewOpenAIProvider: %v", err)
	}

	if provider.Name() != "openai" || provider.Type() != ProviderOpenAI {
		t.Fatalf("unexpected provider identity: %s/%s", provider.Name(), provider.Type())
	}

	voices, err := provider.ListVoices(context.Background())
	if err != nil {
		t.Fatalf("ListVoices: %v", err)
	}
	if len(voices) == 0 {
		t.Fatal("expected built-in openai voices")
	}

	voice, err := provider.GetVoice(context.Background(), "nova")
	if err != nil {
		t.Fatalf("GetVoice(nova): %v", err)
	}
	if voice.ID != "nova" {
		t.Fatalf("unexpected voice lookup result: %+v", voice)
	}
	if err := provider.ValidateVoice("invalid"); err == nil {
		t.Fatal("expected invalid voice validation to fail")
	}

	provider.SetModel(OpenAITTS1)
	provider.SetVoice("echo")

	result, err := provider.Synthesize(context.Background(), "hello", WithSpeed(1.25), WithFormat(FormatWAV))
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if string(result.Data) != "openai-audio" || result.Format != FormatWAV {
		t.Fatalf("unexpected synth result: %+v", result)
	}

	stream, err := provider.SynthesizeStream(context.Background(), "hello")
	if err != nil {
		t.Fatalf("SynthesizeStream: %v", err)
	}

	var streamed []byte
	for chunk := range stream {
		streamed = append(streamed, chunk...)
	}
	if string(streamed) != "openai-audio" {
		t.Fatalf("unexpected streamed audio: %q", streamed)
	}

	if len(payloads) < 2 {
		t.Fatalf("expected synth and stream requests, got %d", len(payloads))
	}
	if payloads[0]["model"] != string(OpenAITTS1) {
		t.Fatalf("unexpected model payload: %#v", payloads[0]["model"])
	}
	if payloads[0]["voice"] != "echo" {
		t.Fatalf("unexpected voice payload: %#v", payloads[0]["voice"])
	}
}

func TestElevenLabsProviderHTTPAndHelpers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("xi-api-key"); got != "eleven-key" {
			t.Fatalf("unexpected xi-api-key: %s", got)
		}

		switch {
		case r.URL.Path == "/v1/user":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"subscription":{"character_count":12,"character_limit":1000}}`))
		case strings.HasSuffix(r.URL.Path, "/stream"):
			w.Header().Set("Content-Type", "audio/mpeg")
			_, _ = w.Write([]byte("eleven-stream"))
		case strings.HasPrefix(r.URL.Path, "/v1/text-to-speech/"):
			w.Header().Set("Content-Type", "audio/ogg")
			_, _ = w.Write([]byte("eleven-audio"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	settings := ElevenLabsVoiceSettings{
		Stability:       0.8,
		SimilarityBoost: 0.9,
		Style:           0.2,
		UseSpeakerBoost: false,
	}

	provider, err := NewElevenLabsProvider(
		"eleven-key",
		WithElevenLabsBaseURL(server.URL+"/v1"),
		WithElevenLabsTimeout(time.Second),
		WithElevenLabsRetries(0),
		WithElevenLabsVoice("voice-a"),
		WithElevenLabsModel(ElevenLabsTurboV2),
		WithElevenLabsVoiceSettings(settings),
	)
	if err != nil {
		t.Fatalf("NewElevenLabsProvider: %v", err)
	}

	if provider.Name() != "elevenlabs" || provider.Type() != ProviderElevenLabs {
		t.Fatalf("unexpected provider identity: %s/%s", provider.Name(), provider.Type())
	}

	voices, err := provider.ListVoices(context.Background())
	if err != nil {
		t.Fatalf("ListVoices: %v", err)
	}
	if len(voices) == 0 {
		t.Fatal("expected built-in elevenlabs voices")
	}

	voice, err := provider.GetVoice(context.Background(), "21m00Tcm4TlvDq8ikWAM")
	if err != nil {
		t.Fatalf("GetVoice: %v", err)
	}
	if voice.ID == "" {
		t.Fatalf("unexpected voice lookup result: %+v", voice)
	}

	provider.SetModel(ElevenLabsTurboV25)
	provider.SetVoice("voice-b")
	provider.SetVoiceSettings(ElevenLabsVoiceSettings{Stability: 0.5})

	result, err := provider.Synthesize(context.Background(), "hello", WithFormat(FormatOGG))
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if string(result.Data) != "eleven-audio" || result.ContentType != "audio/ogg" {
		t.Fatalf("unexpected synth result: %+v", result)
	}

	stream, err := provider.SynthesizeStream(context.Background(), "hello")
	if err != nil {
		t.Fatalf("SynthesizeStream: %v", err)
	}
	var streamed []byte
	for chunk := range stream {
		streamed = append(streamed, chunk...)
	}
	if string(streamed) != "eleven-stream" {
		t.Fatalf("unexpected streamed audio: %q", streamed)
	}

	count, limit, err := provider.GetUsage(context.Background())
	if err != nil {
		t.Fatalf("GetUsage: %v", err)
	}
	if count != 12 || limit != 1000 {
		t.Fatalf("GetUsage() = (%d, %d), want (12, 1000)", count, limit)
	}
}

func TestEdgePiperAndCoquiProviders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/synthesize" {
			t.Fatalf("unexpected edge path: %s", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll(edge request): %v", err)
		}
		text := string(body)
		if !strings.Contains(text, `xml:lang="ja-JP"`) || !strings.Contains(text, `voice name="ja-JP-NanamiNeural"`) {
			t.Fatalf("unexpected edge SSML: %s", text)
		}
		_, _ = w.Write([]byte("edge-audio"))
	}))
	defer server.Close()

	edge, err := NewEdgeProvider(
		WithEdgeBaseURL(server.URL),
		WithEdgeVoice("ja-JP-NanamiNeural"),
		WithEdgeLanguage("ja-JP"),
		WithEdgeTimeout(time.Second),
	)
	if err != nil {
		t.Fatalf("NewEdgeProvider: %v", err)
	}
	if edge.Name() != "edge" || edge.Type() != ProviderEdge {
		t.Fatalf("unexpected edge identity: %s/%s", edge.Name(), edge.Type())
	}

	result, err := edge.Synthesize(context.Background(), "hello", WithFormat(FormatWAV))
	if err != nil {
		t.Fatalf("Edge Synthesize: %v", err)
	}
	if string(result.Data) != "edge-audio" || result.Format != FormatWAV {
		t.Fatalf("unexpected edge synth result: %+v", result)
	}

	edgeVoices, err := edge.ListVoices(context.Background())
	if err != nil {
		t.Fatalf("Edge ListVoices: %v", err)
	}
	if len(edgeVoices) == 0 {
		t.Fatal("expected edge voices")
	}

	tmpDir := t.TempDir()
	modelA := filepath.Join(tmpDir, "en_US-lessac-high.onnx")
	modelB := filepath.Join(tmpDir, "zh_CN-huayan-medium.onnx")
	for _, path := range []string{modelA, modelB} {
		if err := os.WriteFile(path, []byte("model"), 0o644); err != nil {
			t.Fatalf("WriteFile(%s): %v", path, err)
		}
		configPath := strings.TrimSuffix(path, ".onnx") + ".onnx.json"
		if err := os.WriteFile(configPath, []byte("{}"), 0o644); err != nil {
			t.Fatalf("WriteFile(%s): %v", configPath, err)
		}
	}

	piper, err := NewPiperProvider(
		WithPiperModelPath(modelA),
		WithPiperVoiceDir(tmpDir),
		WithPiperDefaultVoice("en_US-lessac-high"),
		WithPiperDefaultLanguage("en-US"),
		WithPiperUseGPU(true),
		WithPiperVolume(1.4),
		WithPiperSentenceSilence(0.3),
		WithPiperPythonCmd("python-test"),
	)
	if err != nil {
		t.Fatalf("NewPiperProvider: %v", err)
	}
	if piper.Name() != "piper" || piper.Type() != ProviderPiper {
		t.Fatalf("unexpected piper identity: %s/%s", piper.Name(), piper.Type())
	}

	piperVoices, err := piper.ListVoices(context.Background())
	if err != nil {
		t.Fatalf("Piper ListVoices: %v", err)
	}
	if len(piperVoices) == 0 {
		t.Fatal("expected piper voices")
	}

	piper.SetVoice("zh_CN-huayan-medium")
	if !strings.Contains(filepath.Base(piper.modelPath), "zh_CN-huayan-medium") {
		t.Fatalf("expected modelPath to switch, got %s", piper.modelPath)
	}
	piper.SetVoiceDir(tmpDir)

	coqui, err := NewCoquiProvider(
		WithCoquiModel(CoquiModelXTTSv2),
		WithCoquiSpeaker("speaker-a"),
		WithCoquiLanguage("fr"),
		WithCoquiDefaultLanguage("en"),
		WithCoquiUseGPU(true),
		WithCoquiPythonCmd("python-test"),
		WithCoquiVoiceDir(tmpDir),
		WithCoquiOutputFormat("wav"),
	)
	if err != nil {
		t.Fatalf("NewCoquiProvider: %v", err)
	}
	if coqui.Name() != "coqui" || coqui.Type() != ProviderType("coqui") {
		t.Fatalf("unexpected coqui identity: %s/%s", coqui.Name(), coqui.Type())
	}

	coquiVoices, err := coqui.ListVoices(context.Background())
	if err != nil {
		t.Fatalf("Coqui ListVoices (xtts): %v", err)
	}
	if len(coquiVoices) == 0 {
		t.Fatal("expected xtts voices")
	}

	coqui.SetModel(CoquiModelVits)
	coqui.SetSpeaker("speaker-b")
	coqui.SetLanguage("ja")
	coquiVoices, err = coqui.ListVoices(context.Background())
	if err != nil {
		t.Fatalf("Coqui ListVoices (vits): %v", err)
	}
	if len(coquiVoices) == 0 {
		t.Fatal("expected vits voices")
	}
}