package speech

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestNewWhisperCPPProvider(t *testing.T) {
	t.Run("requires model path", func(t *testing.T) {
		_, err := NewWhisperCPPProvider()
		if err == nil {
			t.Fatal("expected error when model path is empty")
		}
		sttErr, ok := err.(*STTError)
		if !ok {
			t.Fatalf("expected *STTError, got %T", err)
		}
		if sttErr.Code != ErrProviderNotSupported {
			t.Errorf("expected ErrProviderNotSupported, got %s", sttErr.Code)
		}
	})

	t.Run("rejects non-existent model", func(t *testing.T) {
		_, err := NewWhisperCPPProvider(WithWhisperCPPModelPath("/nonexistent/model.bin"))
		if err == nil {
			t.Fatal("expected error for non-existent model")
		}
		sttErr, ok := err.(*STTError)
		if !ok {
			t.Fatalf("expected *STTError, got %T", err)
		}
		if sttErr.Code != ErrProviderNotSupported {
			t.Errorf("expected ErrProviderNotSupported, got %s", sttErr.Code)
		}
	})

	t.Run("creates provider with defaults", func(t *testing.T) {
		tmpDir := t.TempDir()
		modelPath := filepath.Join(tmpDir, "model.bin")
		os.WriteFile(modelPath, []byte("fake-model"), 0644)

		p, err := NewWhisperCPPProvider(WithWhisperCPPModelPath(modelPath))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Name() != "whisper.cpp" {
			t.Errorf("expected name whisper.cpp, got %s", p.Name())
		}
		if p.Type() != STTProviderWhisperCPP {
			t.Errorf("expected type %s, got %s", STTProviderWhisperCPP, p.Type())
		}
		if p.threads != 4 {
			t.Errorf("expected 4 threads, got %d", p.threads)
		}
		if p.language != "auto" {
			t.Errorf("expected language auto, got %s", p.language)
		}
		if p.beamSize != 5 {
			t.Errorf("expected beam size 5, got %d", p.beamSize)
		}
		if p.timeout != 300*time.Second {
			t.Errorf("expected 300s timeout, got %v", p.timeout)
		}
	})

	t.Run("applies options", func(t *testing.T) {
		tmpDir := t.TempDir()
		modelPath := filepath.Join(tmpDir, "model.bin")
		os.WriteFile(modelPath, []byte("fake-model"), 0644)

		p, err := NewWhisperCPPProvider(
			WithWhisperCPPModelPath(modelPath),
			WithWhisperCPPBinaryPath("/custom/whisper-main"),
			WithWhisperCPPThreads(8),
			WithWhisperCPPLanguage("zh"),
			WithWhisperCPPTemperature(0.5),
			WithWhisperCPPBeamSize(3),
			WithWhisperCPPBestOf(3),
			WithWhisperCPPUseGPU(true),
			WithWhisperCPPTimeout(60*time.Second),
			WithWhisperCPPRetries(2),
			WithWhisperCPPWordTimestamps(true),
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.binaryPath != "/custom/whisper-main" {
			t.Errorf("expected custom binary path, got %s", p.binaryPath)
		}
		if p.threads != 8 {
			t.Errorf("expected 8 threads, got %d", p.threads)
		}
		if p.language != "zh" {
			t.Errorf("expected language zh, got %s", p.language)
		}
		if p.temperature != 0.5 {
			t.Errorf("expected temperature 0.5, got %f", p.temperature)
		}
		if p.beamSize != 3 {
			t.Errorf("expected beam size 3, got %d", p.beamSize)
		}
		if p.bestOf != 3 {
			t.Errorf("expected best of 3, got %d", p.bestOf)
		}
		if !p.useGPU {
			t.Error("expected useGPU to be true")
		}
		if p.timeout != 60*time.Second {
			t.Errorf("expected 60s timeout, got %v", p.timeout)
		}
		if p.retries != 2 {
			t.Errorf("expected 2 retries, got %d", p.retries)
		}
		if !p.wordTimestamps {
			t.Error("expected wordTimestamps to be true")
		}
	})
}

func TestWhisperCPPProviderTranscribe(t *testing.T) {
	t.Run("rejects empty audio", func(t *testing.T) {
		tmpDir := t.TempDir()
		modelPath := filepath.Join(tmpDir, "model.bin")
		os.WriteFile(modelPath, []byte("fake-model"), 0644)

		p, _ := NewWhisperCPPProvider(WithWhisperCPPModelPath(modelPath))
		_, err := p.Transcribe(context.Background(), nil)
		if err == nil {
			t.Fatal("expected error for empty audio")
		}
	})

	t.Run("rejects unsupported format", func(t *testing.T) {
		tmpDir := t.TempDir()
		modelPath := filepath.Join(tmpDir, "model.bin")
		os.WriteFile(modelPath, []byte("fake-model"), 0644)

		p, _ := NewWhisperCPPProvider(WithWhisperCPPModelPath(modelPath))
		_, err := p.Transcribe(context.Background(), []byte("fake-audio"),
			WithSTTInputFormat(AudioInputFormat("xyz")))
		if err == nil {
			t.Fatal("expected error for unsupported format")
		}
	})
}

func TestWhisperCPPProviderTranscribeFile(t *testing.T) {
	t.Run("rejects empty file path", func(t *testing.T) {
		tmpDir := t.TempDir()
		modelPath := filepath.Join(tmpDir, "model.bin")
		os.WriteFile(modelPath, []byte("fake-model"), 0644)

		p, _ := NewWhisperCPPProvider(WithWhisperCPPModelPath(modelPath))
		_, err := p.TranscribeFile(context.Background(), "")
		if err == nil {
			t.Fatal("expected error for empty file path")
		}
	})

	t.Run("rejects non-existent file", func(t *testing.T) {
		tmpDir := t.TempDir()
		modelPath := filepath.Join(tmpDir, "model.bin")
		os.WriteFile(modelPath, []byte("fake-model"), 0644)

		p, _ := NewWhisperCPPProvider(WithWhisperCPPModelPath(modelPath))
		_, err := p.TranscribeFile(context.Background(), "/nonexistent/file.wav")
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

	t.Run("rejects empty file", func(t *testing.T) {
		tmpDir := t.TempDir()
		modelPath := filepath.Join(tmpDir, "model.bin")
		os.WriteFile(modelPath, []byte("fake-model"), 0644)

		emptyFile := filepath.Join(tmpDir, "empty.wav")
		os.WriteFile(emptyFile, []byte{}, 0644)

		p, _ := NewWhisperCPPProvider(WithWhisperCPPModelPath(modelPath))
		_, err := p.TranscribeFile(context.Background(), emptyFile)
		if err == nil {
			t.Fatal("expected error for empty file")
		}
	})
}

func TestWhisperCPPProviderTranscribeStream(t *testing.T) {
	t.Run("rejects nil reader", func(t *testing.T) {
		tmpDir := t.TempDir()
		modelPath := filepath.Join(tmpDir, "model.bin")
		os.WriteFile(modelPath, []byte("fake-model"), 0644)

		p, _ := NewWhisperCPPProvider(WithWhisperCPPModelPath(modelPath))
		_, err := p.TranscribeStream(context.Background(), nil)
		if err == nil {
			t.Fatal("expected error for nil reader")
		}
	})
}

func TestWhisperCPPProviderBuildArgs(t *testing.T) {
	tmpDir := t.TempDir()
	modelPath := filepath.Join(tmpDir, "model.bin")
	os.WriteFile(modelPath, []byte("fake-model"), 0644)

	t.Run("default args", func(t *testing.T) {
		p, _ := NewWhisperCPPProvider(WithWhisperCPPModelPath(modelPath))
		args := p.buildArgs("input.wav", "/tmp/output", TranscribeOptions{})

		if !containsArg(args, "-m", modelPath) {
			t.Errorf("expected -m %s in args, got %v", modelPath, args)
		}
		if !containsArg(args, "-f", "input.wav") {
			t.Errorf("expected -f input.wav in args, got %v", args)
		}
		if !containsArg(args, "-oj", "") {
			t.Errorf("expected -oj in args, got %v", args)
		}
		if !containsArg(args, "-of", "/tmp/output") {
			t.Errorf("expected -of /tmp/output in args, got %v", args)
		}
		if !containsArg(args, "-t", "4") {
			t.Errorf("expected -t 4 in args, got %v", args)
		}
		if !containsArg(args, "-ng", "0") {
			t.Errorf("expected -ng 0 (CPU) in args, got %v", args)
		}
	})

	t.Run("with language", func(t *testing.T) {
		p, _ := NewWhisperCPPProvider(WithWhisperCPPModelPath(modelPath), WithWhisperCPPLanguage("zh"))
		args := p.buildArgs("input.wav", "/tmp/output", TranscribeOptions{})

		if !containsArg(args, "-l", "zh") {
			t.Errorf("expected -l zh in args, got %v", args)
		}
	})

	t.Run("with word timestamps", func(t *testing.T) {
		p, _ := NewWhisperCPPProvider(WithWhisperCPPModelPath(modelPath), WithWhisperCPPWordTimestamps(true))
		args := p.buildArgs("input.wav", "/tmp/output", TranscribeOptions{})

		if !containsArg(args, "-owts", "") {
			t.Errorf("expected -owts in args, got %v", args)
		}
	})

	t.Run("with GPU", func(t *testing.T) {
		p, _ := NewWhisperCPPProvider(WithWhisperCPPModelPath(modelPath), WithWhisperCPPUseGPU(true))
		args := p.buildArgs("input.wav", "/tmp/output", TranscribeOptions{})

		if !containsArg(args, "-ng", "1") {
			t.Errorf("expected -ng 1 (GPU) in args, got %v", args)
		}
	})

	t.Run("with temperature", func(t *testing.T) {
		p, _ := NewWhisperCPPProvider(WithWhisperCPPModelPath(modelPath), WithWhisperCPPTemperature(0.7))
		args := p.buildArgs("input.wav", "/tmp/output", TranscribeOptions{})

		if !containsArg(args, "--temperature", "0.70") {
			t.Errorf("expected --temperature 0.70 in args, got %v", args)
		}
	})

	t.Run("with beam size", func(t *testing.T) {
		p, _ := NewWhisperCPPProvider(WithWhisperCPPModelPath(modelPath), WithWhisperCPPBeamSize(3))
		args := p.buildArgs("input.wav", "/tmp/output", TranscribeOptions{})

		if !containsArg(args, "--beam-size", "3") {
			t.Errorf("expected --beam-size 3 in args, got %v", args)
		}
	})

	t.Run("auto language not added", func(t *testing.T) {
		p, _ := NewWhisperCPPProvider(WithWhisperCPPModelPath(modelPath))
		args := p.buildArgs("input.wav", "/tmp/output", TranscribeOptions{Language: "auto"})

		for i, arg := range args {
			if arg == "-l" && i+1 < len(args) && args[i+1] == "auto" {
				t.Errorf("should not add -l auto to args, got %v", args)
			}
		}
	})
}

func containsArg(args []string, flag, value string) bool {
	for i, arg := range args {
		if arg == flag {
			if value == "" {
				return true
			}
			if i+1 < len(args) && args[i+1] == value {
				return true
			}
		}
	}
	return false
}

func TestWhisperCPPProviderListLanguages(t *testing.T) {
	tmpDir := t.TempDir()
	modelPath := filepath.Join(tmpDir, "model.bin")
	os.WriteFile(modelPath, []byte("fake-model"), 0644)

	p, _ := NewWhisperCPPProvider(WithWhisperCPPModelPath(modelPath))
	langs, err := p.ListLanguages(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(langs) == 0 {
		t.Fatal("expected non-empty language list")
	}

	found := false
	for _, lang := range langs {
		if lang == "auto" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'auto' in language list")
	}

	found = false
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

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		name string
		ts   string
		want time.Duration
	}{
		{"zero", "00:00:00.000", 0},
		{"one second", "00:00:01.000", time.Second},
		{"500ms", "00:00:00.500", 500 * time.Millisecond},
		{"2.5s", "00:00:02.500", 2500 * time.Millisecond},
		{"1 minute", "00:01:00.000", time.Minute},
		{"1 hour", "01:00:00.000", time.Hour},
		{"complex", "00:01:23.456", time.Minute + 23*time.Second + 456*time.Millisecond},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseTimestamp(tt.ts)
			if got != tt.want {
				t.Errorf("parseTimestamp(%s) = %v, want %v", tt.ts, got, tt.want)
			}
		})
	}

	t.Run("invalid format", func(t *testing.T) {
		got := parseTimestamp("invalid")
		if got != 0 {
			t.Errorf("expected 0 for invalid timestamp, got %v", got)
		}
	})
}

func TestWhisperCPPProviderParseJSONOutput(t *testing.T) {
	tmpDir := t.TempDir()
	modelPath := filepath.Join(tmpDir, "model.bin")
	os.WriteFile(modelPath, []byte("fake-model"), 0644)

	t.Run("parses JSON output", func(t *testing.T) {
		jsonContent := `{
			"system_info": {
				"n_threads": 4,
				"model": "model.bin",
				"language": "en"
			},
			"transcription": [
				{
					"timestamps": [
						{
							"t0": 0.0,
							"t1": 1.5,
							"text": "Hello world",
							"words": [
								{"t0": 0.0, "t1": 0.5, "text": "Hello", "p": 0.95},
								{"t0": 0.6, "t1": 1.0, "text": "world", "p": 0.92}
							]
						}
					],
					"text": "Hello world"
				}
			]
		}`

		jsonPath := filepath.Join(tmpDir, "test_output.json")
		os.WriteFile(jsonPath, []byte(jsonContent), 0644)

		p, _ := NewWhisperCPPProvider(WithWhisperCPPModelPath(modelPath))
		result, err := p.parseJSONOutput(jsonPath, TranscribeOptions{WordTimestamps: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Text != "Hello world" {
			t.Errorf("expected 'Hello world', got '%s'", result.Text)
		}
		if result.Language != "en" {
			t.Errorf("expected language 'en', got '%s'", result.Language)
		}
		if len(result.Segments) != 1 {
			t.Fatalf("expected 1 segment, got %d", len(result.Segments))
		}
		if len(result.Words) != 2 {
			t.Fatalf("expected 2 words, got %d", len(result.Words))
		}
		if result.Words[0].Word != "Hello" {
			t.Errorf("expected first word 'Hello', got '%s'", result.Words[0].Word)
		}
		if result.Words[0].Confidence != 0.95 {
			t.Errorf("expected confidence 0.95, got %f", result.Words[0].Confidence)
		}
		if result.Words[0].StartTime != 0 {
			t.Errorf("expected start time 0, got %v", result.Words[0].StartTime)
		}
		if result.Words[0].EndTime != 500*time.Millisecond {
			t.Errorf("expected end time 500ms, got %v", result.Words[0].EndTime)
		}
	})

	t.Run("parses multi-segment output", func(t *testing.T) {
		jsonContent := `{
			"system_info": {
				"n_threads": 4,
				"model": "model.bin",
				"language": "zh"
			},
			"transcription": [
				{
					"timestamps": [{"t0": 0.0, "t1": 2.0, "text": "第一段"}],
					"text": "第一段"
				},
				{
					"timestamps": [{"t0": 2.0, "t1": 4.0, "text": "第二段"}],
					"text": "第二段"
				}
			]
		}`

		jsonPath := filepath.Join(tmpDir, "multi_output.json")
		os.WriteFile(jsonPath, []byte(jsonContent), 0644)

		p, _ := NewWhisperCPPProvider(WithWhisperCPPModelPath(modelPath))
		result, err := p.parseJSONOutput(jsonPath, TranscribeOptions{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Text != "第一段 第二段" {
			t.Errorf("expected '第一段 第二段', got '%s'", result.Text)
		}
		if len(result.Segments) != 2 {
			t.Fatalf("expected 2 segments, got %d", len(result.Segments))
		}
		if result.Language != "zh" {
			t.Errorf("expected language 'zh', got '%s'", result.Language)
		}
	})

	t.Run("parses output without word timestamps", func(t *testing.T) {
		jsonContent := `{
			"system_info": {
				"n_threads": 4,
				"model": "model.bin",
				"language": "en"
			},
			"transcription": [
				{
					"timestamps": [{"t0": 0.0, "t1": 1.0, "text": "Test"}],
					"text": "Test"
				}
			]
		}`

		jsonPath := filepath.Join(tmpDir, "no_words_output.json")
		os.WriteFile(jsonPath, []byte(jsonContent), 0644)

		p, _ := NewWhisperCPPProvider(WithWhisperCPPModelPath(modelPath))
		result, err := p.parseJSONOutput(jsonPath, TranscribeOptions{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result.Words) != 0 {
			t.Errorf("expected 0 words when WordTimestamps is false, got %d", len(result.Words))
		}
	})
}

func TestWhisperCPPProviderParseTextOutput(t *testing.T) {
	tmpDir := t.TempDir()
	modelPath := filepath.Join(tmpDir, "model.bin")
	os.WriteFile(modelPath, []byte("fake-model"), 0644)

	t.Run("parses text output with timestamps", func(t *testing.T) {
		textContent := `[00:00:00.000 --> 00:00:01.000]  Hello world
[00:00:01.000 --> 00:00:02.000]  Second segment`

		txtPath := filepath.Join(tmpDir, "output.txt")
		os.WriteFile(txtPath, []byte(textContent), 0644)

		p, _ := NewWhisperCPPProvider(WithWhisperCPPModelPath(modelPath))
		result, err := p.parseTextOutput(txtPath, TranscribeOptions{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(result.Text, "Hello world") {
			t.Errorf("expected text to contain 'Hello world', got '%s'", result.Text)
		}
		if len(result.Segments) != 2 {
			t.Fatalf("expected 2 segments, got %d", len(result.Segments))
		}
	})

	t.Run("parses plain text output", func(t *testing.T) {
		textContent := `This is plain text without timestamps.`

		txtPath := filepath.Join(tmpDir, "plain_output.txt")
		os.WriteFile(txtPath, []byte(textContent), 0644)

		p, _ := NewWhisperCPPProvider(WithWhisperCPPModelPath(modelPath))
		result, err := p.parseTextOutput(txtPath, TranscribeOptions{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Text != "This is plain text without timestamps." {
			t.Errorf("expected 'This is plain text without timestamps.', got '%s'", result.Text)
		}
	})
}

func TestWhisperCPPProviderParseStdoutOutput(t *testing.T) {
	tmpDir := t.TempDir()
	modelPath := filepath.Join(tmpDir, "model.bin")
	os.WriteFile(modelPath, []byte("fake-model"), 0644)

	t.Run("parses stdout with timestamps", func(t *testing.T) {
		stdout := `[00:00:00.000 --> 00:00:01.000]  Hello world
[00:00:01.000 --> 00:00:02.000]  Second line`

		p, _ := NewWhisperCPPProvider(WithWhisperCPPModelPath(modelPath))
		result, err := p.parseStdoutOutput(stdout, TranscribeOptions{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(result.Text, "Hello world") {
			t.Errorf("expected text to contain 'Hello world', got '%s'", result.Text)
		}
	})

	t.Run("parses plain stdout", func(t *testing.T) {
		stdout := `Plain text output`

		p, _ := NewWhisperCPPProvider(WithWhisperCPPModelPath(modelPath))
		result, err := p.parseStdoutOutput(stdout, TranscribeOptions{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Text != "Plain text output" {
			t.Errorf("expected 'Plain text output', got '%s'", result.Text)
		}
	})
}

func TestNewSTTProviderWhisperCPP(t *testing.T) {
	t.Run("creates Whisper.cpp provider", func(t *testing.T) {
		tmpDir := t.TempDir()
		modelPath := filepath.Join(tmpDir, "model.bin")
		os.WriteFile(modelPath, []byte("fake-model"), 0644)

		p, err := NewSTTProvider(STTConfig{
			Type:    STTProviderWhisperCPP,
			Model:   modelPath,
			Timeout: 60 * time.Second,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Type() != STTProviderWhisperCPP {
			t.Errorf("expected STTProviderWhisperCPP, got %s", p.Type())
		}
		if p.Name() != "whisper.cpp" {
			t.Errorf("expected name 'whisper.cpp', got %s", p.Name())
		}
	})

	t.Run("creates Whisper.cpp provider with language", func(t *testing.T) {
		tmpDir := t.TempDir()
		modelPath := filepath.Join(tmpDir, "model.bin")
		os.WriteFile(modelPath, []byte("fake-model"), 0644)

		p, err := NewSTTProvider(STTConfig{
			Type:     STTProviderWhisperCPP,
			Model:    modelPath,
			Language: "zh",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		wcpp, ok := p.(*WhisperCPPProvider)
		if !ok {
			t.Fatalf("expected *WhisperCPPProvider, got %T", p)
		}
		if wcpp.language != "zh" {
			t.Errorf("expected language zh, got %s", wcpp.language)
		}
	})
}

func TestWhisperCPPProviderWithMockedBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mocked binary tests require bash, skipping on Windows")
	}

	tmpDir := t.TempDir()
	modelPath := filepath.Join(tmpDir, "model.bin")
	os.WriteFile(modelPath, []byte("fake-model"), 0644)

	outputDir := filepath.Join(tmpDir, "output")
	os.MkdirAll(outputDir, 0755)

	mockScript := filepath.Join(tmpDir, "mock-whisper")
	scriptContent := `#!/bin/bash
OUTPUT_DIR=""
OUTPUT_BASE=""
for i in "$@"; do
  if [ "$prev" = "-of" ]; then
    OUTPUT_BASE="$i"
  fi
  prev="$i"
done

# Create JSON output
cat > "${OUTPUT_BASE}.json" << 'JSONEOF'
{
  "system_info": {
    "n_threads": 4,
    "model": "model.bin",
    "language": "en"
  },
  "transcription": [
    {
      "timestamps": [
        {"t0": 0.0, "t1": 1.5, "text": "Hello from mock", "words": [
          {"t0": 0.0, "t1": 0.5, "text": "Hello", "p": 0.95},
          {"t0": 0.6, "t1": 1.0, "text": "from", "p": 0.90},
          {"t0": 1.1, "t1": 1.5, "text": "mock", "p": 0.88}
        ]}
      ],
      "text": "Hello from mock"
    }
  ]
}
JSONEOF
`
	os.WriteFile(mockScript, []byte(scriptContent), 0755)

	p, _ := NewWhisperCPPProvider(
		WithWhisperCPPModelPath(modelPath),
		WithWhisperCPPBinaryPath(mockScript),
	)

	t.Run("successful transcription with mocked binary", func(t *testing.T) {
		result, err := p.Transcribe(context.Background(), []byte("fake-audio-data"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Text != "Hello from mock" {
			t.Errorf("expected 'Hello from mock', got '%s'", result.Text)
		}
		if result.Language != "en" {
			t.Errorf("expected language 'en', got '%s'", result.Language)
		}
		if len(result.Words) != 3 {
			t.Fatalf("expected 3 words, got %d", len(result.Words))
		}
	})

	t.Run("file transcription with mocked binary", func(t *testing.T) {
		testFile := filepath.Join(tmpDir, "test.wav")
		os.WriteFile(testFile, []byte("fake-audio-content"), 0644)

		result, err := p.TranscribeFile(context.Background(), testFile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Text != "Hello from mock" {
			t.Errorf("expected 'Hello from mock', got '%s'", result.Text)
		}
	})

	t.Run("stream transcription with mocked binary", func(t *testing.T) {
		reader := strings.NewReader("stream-audio-data")
		result, err := p.TranscribeStream(context.Background(), reader)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Text != "Hello from mock" {
			t.Errorf("expected 'Hello from mock', got '%s'", result.Text)
		}
	})
}

func TestWhisperCPPProviderBinaryNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	modelPath := filepath.Join(tmpDir, "model.bin")
	os.WriteFile(modelPath, []byte("fake-model"), 0644)

	p, _ := NewWhisperCPPProvider(
		WithWhisperCPPModelPath(modelPath),
		WithWhisperCPPBinaryPath("/nonexistent/whisper-binary"),
	)

	_, err := p.Transcribe(context.Background(), []byte("fake-audio"))
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
	sttErr, ok := err.(*STTError)
	if !ok {
		t.Fatalf("expected *STTError, got %T", err)
	}
	if sttErr.Code != ErrProviderNotSupported && sttErr.Code != ErrTranscriptionFailed {
		t.Errorf("expected ErrProviderNotSupported or ErrTranscriptionFailed, got %s", sttErr.Code)
	}
}

func TestWhisperCPPProviderTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	modelPath := filepath.Join(tmpDir, "model.bin")
	os.WriteFile(modelPath, []byte("fake-model"), 0644)

	slowScript := filepath.Join(tmpDir, "slow-whisper")
	scriptContent := `#!/bin/bash
sleep 10
`
	os.WriteFile(slowScript, []byte(scriptContent), 0755)

	p, _ := NewWhisperCPPProvider(
		WithWhisperCPPModelPath(modelPath),
		WithWhisperCPPBinaryPath(slowScript),
		WithWhisperCPPTimeout(100*time.Millisecond),
	)

	_, err := p.Transcribe(context.Background(), []byte("fake-audio"))
	if err == nil {
		t.Fatal("expected timeout error")
	}
	sttErr, ok := err.(*STTError)
	if !ok {
		t.Fatalf("expected *STTError, got %T", err)
	}
	if sttErr.Code != ErrTranscriptionFailed {
		t.Errorf("expected ErrTranscriptionFailed, got %s", sttErr.Code)
	}
}

func TestWhisperCPPProviderFailingBinary(t *testing.T) {
	tmpDir := t.TempDir()
	modelPath := filepath.Join(tmpDir, "model.bin")
	os.WriteFile(modelPath, []byte("fake-model"), 0644)

	failingScript := filepath.Join(tmpDir, "failing-whisper")
	scriptContent := `#!/bin/bash
echo "Error: model loading failed" >&2
exit 1
`
	os.WriteFile(failingScript, []byte(scriptContent), 0755)

	p, _ := NewWhisperCPPProvider(
		WithWhisperCPPModelPath(modelPath),
		WithWhisperCPPBinaryPath(failingScript),
	)

	_, err := p.Transcribe(context.Background(), []byte("fake-audio"))
	if err == nil {
		t.Fatal("expected error for failing binary")
	}
	sttErr, ok := err.(*STTError)
	if !ok {
		t.Fatalf("expected *STTError, got %T", err)
	}
	if sttErr.Code != ErrTranscriptionFailed {
		t.Errorf("expected ErrTranscriptionFailed, got %s", sttErr.Code)
	}
	if !strings.Contains(err.Error(), "stderr") {
		t.Errorf("expected stderr in error message, got: %s", err.Error())
	}
}

func TestWhisperCPPSTTManager(t *testing.T) {
	t.Run("register and use Whisper.cpp provider", func(t *testing.T) {
		m := NewSTTManager()

		tmpDir := t.TempDir()
		modelPath := filepath.Join(tmpDir, "model.bin")
		os.WriteFile(modelPath, []byte("fake-model"), 0644)

		p, _ := NewWhisperCPPProvider(WithWhisperCPPModelPath(modelPath))

		err := m.Register("whisper-cpp", p)
		if err != nil {
			t.Fatalf("failed to register provider: %v", err)
		}

		providers := m.ListProviders()
		if len(providers) != 1 {
			t.Fatalf("expected 1 provider, got %d", len(providers))
		}

		got, err := m.Get("whisper-cpp")
		if err != nil {
			t.Fatalf("failed to get provider: %v", err)
		}
		if got.Type() != STTProviderWhisperCPP {
			t.Errorf("expected STTProviderWhisperCPP, got %s", got.Type())
		}
	})
}

func TestWhisperCPPModelConstants(t *testing.T) {
	expectedModels := []WhisperCPPModel{
		WhisperCPPTiny,
		WhisperCPPBase,
		WhisperCPPSmall,
		WhisperCPPMedium,
		WhisperCPPLarge,
		WhisperCPPLargeV2,
		WhisperCPPLargeV3,
		WhisperCPPTurbo,
	}

	for _, model := range expectedModels {
		if string(model) == "" {
			t.Errorf("model constant %v has empty string value", model)
		}
	}
}

func TestWhisperCPPProviderNoOutputFiles(t *testing.T) {
	tmpDir := t.TempDir()
	modelPath := filepath.Join(tmpDir, "model.bin")
	os.WriteFile(modelPath, []byte("fake-model"), 0644)

	noopScript := filepath.Join(tmpDir, "noop-whisper")
	os.WriteFile(noopScript, []byte("#!/bin/bash\n"), 0755)

	p, _ := NewWhisperCPPProvider(
		WithWhisperCPPModelPath(modelPath),
		WithWhisperCPPBinaryPath(noopScript),
	)

	_, err := p.Transcribe(context.Background(), []byte("fake-audio"))
	if err == nil {
		t.Fatal("expected error when no output is produced")
	}
	sttErr, ok := err.(*STTError)
	if !ok {
		t.Fatalf("expected *STTError, got %T", err)
	}
	if sttErr.Code != ErrTranscriptionFailed {
		t.Errorf("expected ErrTranscriptionFailed, got %s", sttErr.Code)
	}
}

func TestWhisperCPPProviderParseJSONOutputInvalid(t *testing.T) {
	tmpDir := t.TempDir()
	modelPath := filepath.Join(tmpDir, "model.bin")
	os.WriteFile(modelPath, []byte("fake-model"), 0644)

	t.Run("invalid JSON", func(t *testing.T) {
		jsonPath := filepath.Join(tmpDir, "invalid.json")
		os.WriteFile(jsonPath, []byte("not valid json{{{"), 0644)

		p, _ := NewWhisperCPPProvider(WithWhisperCPPModelPath(modelPath))
		_, err := p.parseJSONOutput(jsonPath, TranscribeOptions{})
		if err == nil {
			t.Fatal("expected error for invalid JSON")
		}
	})

	t.Run("missing file", func(t *testing.T) {
		p, _ := NewWhisperCPPProvider(WithWhisperCPPModelPath(modelPath))
		_, err := p.parseJSONOutput("/nonexistent/file.json", TranscribeOptions{})
		if err == nil {
			t.Fatal("expected error for missing JSON file")
		}
	})
}

func TestWhisperCPPProviderParseTextOutputMissing(t *testing.T) {
	tmpDir := t.TempDir()
	modelPath := filepath.Join(tmpDir, "model.bin")
	os.WriteFile(modelPath, []byte("fake-model"), 0644)

	p, _ := NewWhisperCPPProvider(WithWhisperCPPModelPath(modelPath))
	_, err := p.parseTextOutput("/nonexistent/file.txt", TranscribeOptions{})
	if err == nil {
		t.Fatal("expected error for missing text file")
	}
}

func TestWhisperCPPProviderContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()
	modelPath := filepath.Join(tmpDir, "model.bin")
	os.WriteFile(modelPath, []byte("fake-model"), 0644)

	slowScript := filepath.Join(tmpDir, "slow-whisper")
	os.WriteFile(slowScript, []byte("#!/bin/bash\nsleep 10\n"), 0755)

	p, _ := NewWhisperCPPProvider(
		WithWhisperCPPModelPath(modelPath),
		WithWhisperCPPBinaryPath(slowScript),
		WithWhisperCPPTimeout(30*time.Second),
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := p.Transcribe(ctx, []byte("fake-audio"))
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}
