package speech

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type WhisperCPPModel string

const (
	WhisperCPPTiny    WhisperCPPModel = "tiny"
	WhisperCPPBase    WhisperCPPModel = "base"
	WhisperCPPSmall   WhisperCPPModel = "small"
	WhisperCPPMedium  WhisperCPPModel = "medium"
	WhisperCPPLarge   WhisperCPPModel = "large"
	WhisperCPPLargeV2 WhisperCPPModel = "large-v2"
	WhisperCPPLargeV3 WhisperCPPModel = "large-v3"
	WhisperCPPTurbo   WhisperCPPModel = "turbo"
)

type WhisperCPPProvider struct {
	modelPath      string
	binaryPath     string
	threads        int
	language       string
	temperature    float64
	beamSize       int
	bestOf         int
	useGPU         bool
	offset         time.Duration
	duration       time.Duration
	maxTextLen     int
	wordTimestamps bool
	timeout        time.Duration
	retries        int
}

type WhisperCPPOption func(*WhisperCPPProvider)

func WithWhisperCPPModelPath(path string) WhisperCPPOption {
	return func(p *WhisperCPPProvider) {
		p.modelPath = path
	}
}

func WithWhisperCPPBinaryPath(path string) WhisperCPPOption {
	return func(p *WhisperCPPProvider) {
		p.binaryPath = path
	}
}

func WithWhisperCPPThreads(threads int) WhisperCPPOption {
	return func(p *WhisperCPPProvider) {
		p.threads = threads
	}
}

func WithWhisperCPPLanguage(lang string) WhisperCPPOption {
	return func(p *WhisperCPPProvider) {
		p.language = lang
	}
}

func WithWhisperCPPTemperature(temp float64) WhisperCPPOption {
	return func(p *WhisperCPPProvider) {
		p.temperature = temp
	}
}

func WithWhisperCPPBeamSize(size int) WhisperCPPOption {
	return func(p *WhisperCPPProvider) {
		p.beamSize = size
	}
}

func WithWhisperCPPBestOf(n int) WhisperCPPOption {
	return func(p *WhisperCPPProvider) {
		p.bestOf = n
	}
}

func WithWhisperCPPUseGPU(use bool) WhisperCPPOption {
	return func(p *WhisperCPPProvider) {
		p.useGPU = use
	}
}

func WithWhisperCPPOffset(offset time.Duration) WhisperCPPOption {
	return func(p *WhisperCPPProvider) {
		p.offset = offset
	}
}

func WithWhisperCPPDuration(d time.Duration) WhisperCPPOption {
	return func(p *WhisperCPPProvider) {
		p.duration = d
	}
}

func WithWhisperCPPMaxTextLen(n int) WhisperCPPOption {
	return func(p *WhisperCPPProvider) {
		p.maxTextLen = n
	}
}

func WithWhisperCPPWordTimestamps(enabled bool) WhisperCPPOption {
	return func(p *WhisperCPPProvider) {
		p.wordTimestamps = enabled
	}
}

func WithWhisperCPPTimeout(timeout time.Duration) WhisperCPPOption {
	return func(p *WhisperCPPProvider) {
		p.timeout = timeout
	}
}

func WithWhisperCPPRetries(retries int) WhisperCPPOption {
	return func(p *WhisperCPPProvider) {
		p.retries = retries
	}
}

func NewWhisperCPPProvider(opts ...WhisperCPPOption) (*WhisperCPPProvider, error) {
	p := &WhisperCPPProvider{
		binaryPath:  "whisper-main",
		threads:     4,
		language:    "auto",
		temperature: 0.0,
		beamSize:    5,
		bestOf:      5,
		useGPU:      false,
		timeout:     300 * time.Second,
		retries:     0,
	}

	for _, opt := range opts {
		opt(p)
	}

	if p.modelPath == "" {
		return nil, NewSTTError(ErrProviderNotSupported, "whisper.cpp: model path is required")
	}

	if _, err := os.Stat(p.modelPath); err != nil {
		if os.IsNotExist(err) {
			return nil, NewSTTErrorf(ErrProviderNotSupported, "whisper.cpp: model file not found: %s", p.modelPath)
		}
		return nil, NewSTTErrorf(ErrProviderNotSupported, "whisper.cpp: cannot access model file: %v", err)
	}

	return p, nil
}

func (p *WhisperCPPProvider) Name() string {
	return "whisper.cpp"
}

func (p *WhisperCPPProvider) Type() STTProviderType {
	return STTProviderWhisperCPP
}

func (p *WhisperCPPProvider) Transcribe(ctx context.Context, audio []byte, opts ...TranscribeOption) (*TranscriptResult, error) {
	options := TranscribeOptions{
		Language:    p.language,
		InputFormat: InputMP3,
	}
	for _, opt := range opts {
		opt(&options)
	}

	if len(audio) == 0 {
		return nil, NewSTTError(ErrAudioFormatInvalid, "whisper.cpp: audio data is empty")
	}

	if !validInputFormats[options.InputFormat] {
		return nil, NewSTTErrorf(ErrAudioFormatInvalid, "whisper.cpp: unsupported input format: %s", options.InputFormat)
	}

	var lastErr error
	for attempt := 0; attempt <= p.retries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(attempt) * time.Second
			select {
			case <-ctx.Done():
				return nil, NewSTTErrorf(ErrTranscriptionFailed, "whisper.cpp: context cancelled during retry: %v", ctx.Err())
			case <-time.After(backoff):
			}
		}

		result, err := p.doTranscribe(ctx, audio, options)
		if err == nil {
			return result, nil
		}

		lastErr = err
	}

	return nil, NewSTTErrorf(ErrTranscriptionFailed, "whisper.cpp: all %d retries failed: %v", p.retries, lastErr)
}

func (p *WhisperCPPProvider) TranscribeFile(ctx context.Context, filePath string, opts ...TranscribeOption) (*TranscriptResult, error) {
	if filePath == "" {
		return nil, NewSTTError(ErrAudioFormatInvalid, "whisper.cpp: file path is empty")
	}

	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, NewSTTErrorf(ErrAudioFormatInvalid, "whisper.cpp: file not found: %s", filePath)
		}
		return nil, NewSTTErrorf(ErrTranscriptionFailed, "whisper.cpp: failed to stat file: %v", err)
	}

	if info.Size() == 0 {
		return nil, NewSTTError(ErrAudioFormatInvalid, "whisper.cpp: file is empty")
	}

	options := TranscribeOptions{
		Language:    p.language,
		InputFormat: InputWAV,
	}
	for _, opt := range opts {
		opt(&options)
	}

	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(filePath)), ".")
	if ext != "" {
		options.InputFormat = AudioInputFormat(ext)
	}

	return p.doTranscribeFile(ctx, filePath, options)
}

func (p *WhisperCPPProvider) TranscribeStream(ctx context.Context, reader io.Reader, opts ...TranscribeOption) (*TranscriptResult, error) {
	if reader == nil {
		return nil, NewSTTError(ErrAudioFormatInvalid, "whisper.cpp: reader is nil")
	}

	audio, err := io.ReadAll(reader)
	if err != nil {
		return nil, NewSTTErrorf(ErrTranscriptionFailed, "whisper.cpp: failed to read stream: %v", err)
	}

	return p.Transcribe(ctx, audio, opts...)
}

func (p *WhisperCPPProvider) doTranscribe(ctx context.Context, audio []byte, options TranscribeOptions) (*TranscriptResult, error) {
	tmpAudio, err := os.CreateTemp("", "whisper-cpp-input-*."+string(options.InputFormat))
	if err != nil {
		return nil, NewSTTErrorf(ErrTranscriptionFailed, "whisper.cpp: failed to create temp audio file: %v", err)
	}
	tmpAudioPath := tmpAudio.Name()
	tmpAudio.Close()
	defer os.Remove(tmpAudioPath)

	if err := os.WriteFile(tmpAudioPath, audio, 0644); err != nil {
		return nil, NewSTTErrorf(ErrTranscriptionFailed, "whisper.cpp: failed to write temp audio file: %v", err)
	}

	return p.doTranscribeFile(ctx, tmpAudioPath, options)
}

func (p *WhisperCPPProvider) doTranscribeFile(ctx context.Context, filePath string, options TranscribeOptions) (*TranscriptResult, error) {
	outputDir, err := os.MkdirTemp("", "whisper-cpp-output-*")
	if err != nil {
		return nil, NewSTTErrorf(ErrTranscriptionFailed, "whisper.cpp: failed to create temp output dir: %v", err)
	}
	defer os.RemoveAll(outputDir)

	outputBase := filepath.Join(outputDir, "output")

	args := p.buildArgs(filePath, outputBase, options)

	binaryPath := p.binaryPath
	if _, err := exec.LookPath(binaryPath); err != nil {
		if _, err := os.Stat(binaryPath); err != nil {
			return nil, NewSTTErrorf(ErrProviderNotSupported, "whisper.cpp: binary not found at %s", binaryPath)
		}
	}

	cmdCtx := ctx
	if p.timeout > 0 {
		var cancel context.CancelFunc
		cmdCtx, cancel = context.WithTimeout(ctx, p.timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(cmdCtx, binaryPath, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if cmdCtx.Err() != nil {
			return nil, NewSTTErrorf(ErrTranscriptionFailed, "whisper.cpp: transcription timed out: %v", cmdCtx.Err())
		}
		return nil, NewSTTErrorf(ErrTranscriptionFailed, "whisper.cpp: transcription failed: %v, stderr: %s", err, stderr.String())
	}

	jsonPath := outputBase + ".json"
	if _, err := os.Stat(jsonPath); err == nil {
		return p.parseJSONOutput(jsonPath, options)
	}

	txtPath := outputBase + ".txt"
	if _, err := os.Stat(txtPath); err == nil {
		return p.parseTextOutput(txtPath, options)
	}

	if stdout.Len() > 0 {
		return p.parseStdoutOutput(stdout.String(), options)
	}

	return nil, NewSTTError(ErrTranscriptionFailed, "whisper.cpp: no output produced")
}

func (p *WhisperCPPProvider) buildArgs(inputFile, outputBase string, options TranscribeOptions) []string {
	args := []string{
		"-m", p.modelPath,
		"-f", inputFile,
		"-oj",
		"-of", outputBase,
		"-t", strconv.Itoa(p.threads),
	}

	if options.Language != "" && options.Language != "auto" {
		args = append(args, "-l", options.Language)
	} else if p.language != "" && p.language != "auto" {
		args = append(args, "-l", p.language)
	}

	if p.temperature > 0 {
		args = append(args, "--temperature", fmt.Sprintf("%.2f", p.temperature))
	}

	if p.beamSize > 0 {
		args = append(args, "--beam-size", strconv.Itoa(p.beamSize))
	}

	if p.bestOf > 0 {
		args = append(args, "--best-of", strconv.Itoa(p.bestOf))
	}

	if p.useGPU {
		args = append(args, "-ng", "1")
	} else {
		args = append(args, "-ng", "0")
	}

	if p.offset > 0 {
		args = append(args, "-o", strconv.FormatInt(int64(p.offset.Milliseconds()), 10))
	}

	if p.duration > 0 {
		args = append(args, "-d", strconv.FormatInt(int64(p.duration.Milliseconds()), 10))
	}

	if p.maxTextLen > 0 {
		args = append(args, "-ml", strconv.Itoa(p.maxTextLen))
	}

	if p.wordTimestamps || options.WordTimestamps {
		args = append(args, "-owts")
	}

	return args
}

type whisperCPPJSONOutput struct {
	SystemInfo struct {
		Threads  int    `json:"n_threads"`
		Model    string `json:"model"`
		Language string `json:"language"`
	} `json:"system_info"`
	Transcription []struct {
		Timestamps []struct {
			T0    float64 `json:"t0"`
			T1    float64 `json:"t1"`
			Text  string  `json:"text"`
			Words []struct {
				T0   float64 `json:"t0"`
				T1   float64 `json:"t1"`
				Text string  `json:"text"`
				Prob float64 `json:"p"`
			} `json:"words,omitempty"`
		} `json:"timestamps"`
		Text string `json:"text"`
	} `json:"transcription"`
}

func (p *WhisperCPPProvider) parseJSONOutput(jsonPath string, options TranscribeOptions) (*TranscriptResult, error) {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, NewSTTErrorf(ErrTranscriptionFailed, "whisper.cpp: failed to read JSON output: %v", err)
	}

	var output whisperCPPJSONOutput
	if err := json.Unmarshal(data, &output); err != nil {
		return nil, NewSTTErrorf(ErrTranscriptionFailed, "whisper.cpp: failed to parse JSON output: %v", err)
	}

	result := &TranscriptResult{
		Language: output.SystemInfo.Language,
	}

	var totalConfidence float64
	var confidenceCount int

	for i, seg := range output.Transcription {
		segment := SegmentInfo{
			ID:   i,
			Text: strings.TrimSpace(seg.Text),
		}

		for _, ts := range seg.Timestamps {
			if ts.T1 > 0 {
				segment.EndTime = time.Duration(ts.T1 * float64(time.Second))
			}
			if segment.StartTime == 0 && ts.T0 > 0 {
				segment.StartTime = time.Duration(ts.T0 * float64(time.Second))
			}

			for _, word := range ts.Words {
				wi := WordInfo{
					Word:      strings.TrimSpace(word.Text),
					StartTime: time.Duration(word.T0 * float64(time.Second)),
					EndTime:   time.Duration(word.T1 * float64(time.Second)),
				}
				if word.Prob > 0 {
					wi.Confidence = word.Prob
					totalConfidence += word.Prob
					confidenceCount++
				}
				segment.Words = append(segment.Words, wi)

				if options.WordTimestamps {
					result.Words = append(result.Words, wi)
				}
			}
		}

		result.Segments = append(result.Segments, segment)

		if result.Text == "" {
			result.Text = strings.TrimSpace(seg.Text)
		} else {
			result.Text += " " + strings.TrimSpace(seg.Text)
		}
	}

	result.Text = strings.TrimSpace(result.Text)

	if confidenceCount > 0 {
		result.Confidence = totalConfidence / float64(confidenceCount)
	}

	if len(result.Segments) > 0 {
		lastSeg := result.Segments[len(result.Segments)-1]
		if lastSeg.EndTime > 0 {
			result.Duration = lastSeg.EndTime
		}
	}

	return result, nil
}

func (p *WhisperCPPProvider) parseTextOutput(txtPath string, options TranscribeOptions) (*TranscriptResult, error) {
	data, err := os.ReadFile(txtPath)
	if err != nil {
		return nil, NewSTTErrorf(ErrTranscriptionFailed, "whisper.cpp: failed to read text output: %v", err)
	}

	text := strings.TrimSpace(string(data))

	var segments []SegmentInfo
	var currentText string
	timestampRegex := regexp.MustCompile(`\[(\d{2}:\d{2}:\d{2}\.\d{3})\s*-->\s*(\d{2}:\d{2}:\d{2}\.\d{3})\]\s*(.*)`)

	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		matches := timestampRegex.FindStringSubmatch(line)
		if matches != nil {
			if currentText != "" {
				segments = append(segments, SegmentInfo{
					ID:   len(segments),
					Text: strings.TrimSpace(currentText),
				})
			}

			startTime := parseTimestamp(matches[1])
			endTime := parseTimestamp(matches[2])
			segText := strings.TrimSpace(matches[3])

			segments = append(segments, SegmentInfo{
				ID:        len(segments),
				Text:      segText,
				StartTime: startTime,
				EndTime:   endTime,
			})
			currentText = ""
		} else {
			if currentText != "" {
				currentText += " " + line
			} else {
				currentText = line
			}
		}
	}

	if currentText != "" {
		segments = append(segments, SegmentInfo{
			ID:   len(segments),
			Text: strings.TrimSpace(currentText),
		})
	}

	fullText := strings.Join(func() []string {
		var texts []string
		for _, s := range segments {
			texts = append(texts, s.Text)
		}
		return texts
	}(), " ")

	return &TranscriptResult{
		Text:     strings.TrimSpace(fullText),
		Segments: segments,
	}, nil
}

func (p *WhisperCPPProvider) parseStdoutOutput(stdout string, options TranscribeOptions) (*TranscriptResult, error) {
	lines := strings.Split(stdout, "\n")
	var textLines []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.Contains(line, "]") {
			idx := strings.Index(line, "]")
			if idx+1 < len(line) {
				content := strings.TrimSpace(line[idx+1:])
				if content != "" {
					textLines = append(textLines, content)
				}
			}
		} else {
			textLines = append(textLines, line)
		}
	}

	return &TranscriptResult{
		Text: strings.TrimSpace(strings.Join(textLines, " ")),
	}, nil
}

func parseTimestamp(ts string) time.Duration {
	parts := strings.Split(ts, ":")
	if len(parts) != 3 {
		return 0
	}

	hours, _ := strconv.Atoi(parts[0])
	minutes, _ := strconv.Atoi(parts[1])

	secParts := strings.Split(parts[2], ".")
	seconds, _ := strconv.Atoi(secParts[0])
	millis := 0
	if len(secParts) > 1 {
		millis, _ = strconv.Atoi(secParts[1])
		for millis > 0 && millis < 100 {
			millis *= 10
		}
	}

	return time.Duration(hours)*time.Hour +
		time.Duration(minutes)*time.Minute +
		time.Duration(seconds)*time.Second +
		time.Duration(millis)*time.Millisecond
}

func (p *WhisperCPPProvider) ListLanguages(ctx context.Context) ([]string, error) {
	return []string{
		"auto", "af", "am", "ar", "as", "az", "ba", "be", "bg", "bn", "bo", "br", "bs", "ca", "cs", "cy", "da",
		"de", "el", "en", "es", "et", "eu", "fa", "fi", "fo", "fr", "gl", "gu", "ha", "haw", "he", "hi",
		"hr", "ht", "hu", "hy", "id", "is", "it", "ja", "jw", "ka", "kk", "km", "kn", "ko", "la", "lb",
		"ln", "lo", "lt", "lv", "mg", "mi", "mk", "ml", "mn", "mr", "ms", "mt", "my", "ne", "nl", "nn",
		"no", "oc", "pa", "pl", "ps", "pt", "ro", "ru", "sa", "sd", "si", "sk", "sl", "sn", "so", "sq",
		"sr", "su", "sv", "sw", "ta", "te", "tg", "th", "tk", "tl", "tr", "tt", "uk", "ur", "uz", "vi",
		"yi", "yo", "zh",
	}, nil
}
