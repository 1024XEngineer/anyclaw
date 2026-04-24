package speech

import (
	"testing"
	"time"
)

func TestVADCreation(t *testing.T) {
	cfg := DefaultVADConfig()
	vad := NewVAD(cfg)

	if vad == nil {
		t.Fatal("expected VAD instance, got nil")
	}

	if vad.State() != VADStateSilence {
		t.Errorf("expected initial state to be silence, got %s", vad.State())
	}
}

func TestVADDefaultConfig(t *testing.T) {
	cfg := DefaultVADConfig()

	if cfg.SampleRate != 16000 {
		t.Errorf("expected default sample rate 16000, got %d", cfg.SampleRate)
	}
	if cfg.FrameSize != 320 {
		t.Errorf("expected default frame size 320, got %d", cfg.FrameSize)
	}
	if cfg.EnergyThreshold != 0.01 {
		t.Errorf("expected default energy threshold 0.01, got %f", cfg.EnergyThreshold)
	}
}

func TestVADProcessSilence(t *testing.T) {
	cfg := DefaultVADConfig()
	vad := NewVAD(cfg)

	silence := make([]int16, 320)
	state := vad.ProcessFrame(silence)

	if state != VADStateSilence {
		t.Errorf("expected silence state for zero samples, got %s", state)
	}
}

func TestVADProcessSpeech(t *testing.T) {
	cfg := DefaultVADConfig()
	cfg.SpeechMinFrames = 3
	vad := NewVAD(cfg)

	speech := make([]int16, 320)
	for i := range speech {
		speech[i] = 10000
	}

	for i := 0; i < 5; i++ {
		vad.ProcessFrame(speech)
	}

	state := vad.State()
	if state != VADStateSpeech {
		t.Errorf("expected speech state after consecutive speech frames, got %s", state)
	}
}

func TestVADStateTransition(t *testing.T) {
	cfg := DefaultVADConfig()
	cfg.SpeechMinFrames = 3
	cfg.HangoverFrames = 5
	vad := NewVAD(cfg)

	silence := make([]int16, 320)
	speech := make([]int16, 320)
	for i := range speech {
		speech[i] = 10000
	}

	if vad.State() != VADStateSilence {
		t.Error("expected initial silence")
	}

	for i := 0; i < 5; i++ {
		vad.ProcessFrame(speech)
	}

	if vad.State() != VADStateSpeech {
		t.Errorf("expected speech after speech frames, got %s", vad.State())
	}

	for i := 0; i < 10; i++ {
		vad.ProcessFrame(silence)
	}

	if vad.State() != VADStateSilence {
		t.Errorf("expected silence after silence frames, got %s", vad.State())
	}
}

func TestVADReset(t *testing.T) {
	cfg := DefaultVADConfig()
	cfg.SpeechMinFrames = 3
	vad := NewVAD(cfg)

	speech := make([]int16, 320)
	for i := range speech {
		speech[i] = 10000
	}

	for i := 0; i < 5; i++ {
		vad.ProcessFrame(speech)
	}

	if vad.State() != VADStateSpeech {
		t.Error("expected speech state before reset")
	}

	vad.Reset()

	if vad.State() != VADStateSilence {
		t.Errorf("expected silence after reset, got %s", vad.State())
	}
}

func TestVADListener(t *testing.T) {
	cfg := DefaultVADConfig()
	cfg.SpeechMinFrames = 3
	cfg.HangoverFrames = 5
	vad := NewVAD(cfg)

	stateChanges := make(chan VADState, 10)
	vad.RegisterListener(func(state VADState, energy float64, zcr float64) {
		stateChanges <- state
	})

	speech := make([]int16, 320)
	for i := range speech {
		speech[i] = 10000
	}

	for i := 0; i < 5; i++ {
		vad.ProcessFrame(speech)
	}

	select {
	case state := <-stateChanges:
		if state != VADStateSpeech {
			t.Errorf("expected speech state in listener, got %s", state)
		}
	default:
		t.Error("expected listener callback for speech state")
	}
}

func TestVADFloatFrame(t *testing.T) {
	cfg := DefaultVADConfig()
	vad := NewVAD(cfg)

	silence := make([]float32, 320)
	state := vad.ProcessFloatFrame(silence)

	if state != VADStateSilence {
		t.Errorf("expected silence for float silence samples, got %s", state)
	}
}

func TestVADUpdateConfig(t *testing.T) {
	cfg := DefaultVADConfig()
	vad := NewVAD(cfg)

	newCfg := VADConfig{
		EnergyThreshold: 0.05,
	}
	vad.UpdateConfig(newCfg)

	updated := vad.Config()
	if updated.EnergyThreshold != 0.05 {
		t.Errorf("expected energy threshold 0.05, got %f", updated.EnergyThreshold)
	}
}

func TestNormalizeAudio(t *testing.T) {
	samples := []int16{-32768, 0, 32767}
	normalized := NormalizeAudio(samples)

	if len(normalized) != 3 {
		t.Errorf("expected 3 samples, got %d", len(normalized))
	}

	if normalized[0] != -1.0 {
		t.Errorf("expected -1.0, got %f", normalized[0])
	}
	if normalized[1] != 0.0 {
		t.Errorf("expected 0.0, got %f", normalized[1])
	}
}

func TestFloat32ToInt16(t *testing.T) {
	samples := []float32{-1.0, 0.0, 1.0}
	converted := Float32ToInt16(samples)

	if converted[0] != -32767 {
		t.Errorf("expected -32767, got %d", converted[0])
	}
	if converted[1] != 0 {
		t.Errorf("expected 0, got %d", converted[1])
	}
	if converted[2] != 32767 {
		t.Errorf("expected 32767, got %d", converted[2])
	}
}

func TestInt16ToWAV(t *testing.T) {
	samples := []int16{0, 1000, -1000, 0}
	wav := Int16ToWAV(samples, 16000, 1)

	if len(wav) != 44+8 {
		t.Errorf("expected WAV size 52 bytes (44 header + 8 data), got %d", len(wav))
	}

	if string(wav[0:4]) != "RIFF" {
		t.Errorf("expected RIFF header, got %s", string(wav[0:4]))
	}

	if string(wav[8:12]) != "WAVE" {
		t.Errorf("expected WAVE format, got %s", string(wav[8:12]))
	}
}

func TestInt16ToWAVEmpty(t *testing.T) {
	wav := Int16ToWAV(nil, 16000, 1)
	if wav != nil {
		t.Error("expected nil for empty samples")
	}
}

func TestWakeWordDetectorCreation(t *testing.T) {
	cfg := DefaultWakeWordConfig()
	detector := NewWakeWordDetector(cfg)

	if detector == nil {
		t.Fatal("expected detector instance, got nil")
	}

	words := detector.WakeWords()
	if len(words) != 3 {
		t.Errorf("expected 3 wake words, got %d", len(words))
	}
}

func TestWakeWordExactMatch(t *testing.T) {
	cfg := WakeWordConfig{
		WakeWords: []string{"hey anyclaw"},
	}
	detector := NewWakeWordDetector(cfg)

	phrase, confidence, matched := detector.Detect("hey anyclaw")

	if !matched {
		t.Error("expected exact match for 'hey anyclaw'")
	}
	if phrase != "hey anyclaw" {
		t.Errorf("expected phrase 'hey anyclaw', got %s", phrase)
	}
	if confidence != 1.0 {
		t.Errorf("expected confidence 1.0, got %f", confidence)
	}
}

func TestWakeWordSubstringMatch(t *testing.T) {
	cfg := WakeWordConfig{
		WakeWords: []string{"hey anyclaw"},
	}
	detector := NewWakeWordDetector(cfg)

	_, confidence, matched := detector.Detect("hello hey anyclaw how are you")

	if !matched {
		t.Error("expected substring match")
	}
	if confidence < 0.8 {
		t.Errorf("expected high confidence for substring match, got %f", confidence)
	}
}

func TestWakeWordCaseInsensitive(t *testing.T) {
	cfg := WakeWordConfig{
		WakeWords: []string{"hey anyclaw"},
	}
	detector := NewWakeWordDetector(cfg)

	_, _, matched := detector.Detect("HEY ANYCLAW")

	if !matched {
		t.Error("expected case-insensitive match")
	}
}

func TestWakeWordNoMatch(t *testing.T) {
	cfg := WakeWordConfig{
		WakeWords: []string{"hey anyclaw"},
	}
	detector := NewWakeWordDetector(cfg)

	_, _, matched := detector.Detect("hello world")

	if matched {
		t.Error("expected no match for unrelated text")
	}
}

func TestWakeWordEmptyInput(t *testing.T) {
	cfg := WakeWordConfig{
		WakeWords: []string{"hey anyclaw"},
	}
	detector := NewWakeWordDetector(cfg)

	_, _, matched := detector.Detect("")

	if matched {
		t.Error("expected no match for empty input")
	}
}

func TestWakeWordAddRemove(t *testing.T) {
	cfg := WakeWordConfig{}
	detector := NewWakeWordDetector(cfg)

	detector.AddWakeWord(WakeWord{Phrase: "test word"})
	words := detector.WakeWords()
	if len(words) != 1 {
		t.Errorf("expected 1 wake word after add, got %d", len(words))
	}

	detector.RemoveWakeWord("test word")
	words = detector.WakeWords()
	if len(words) != 0 {
		t.Errorf("expected 0 wake words after remove, got %d", len(words))
	}
}

func TestWakeWordListener(t *testing.T) {
	cfg := WakeWordConfig{
		WakeWords: []string{"hey anyclaw"},
	}
	detector := NewWakeWordDetector(cfg)

	matched := make(chan string, 1)
	detector.RegisterListener(func(phrase string, confidence float64) {
		matched <- phrase
	})

	detector.Detect("hey anyclaw")

	select {
	case phrase := <-matched:
		if phrase != "hey anyclaw" {
			t.Errorf("expected 'hey anyclaw', got %s", phrase)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected listener callback")
	}
}

func TestWakeWordAliases(t *testing.T) {
	cfg := WakeWordConfig{}
	detector := NewWakeWordDetector(cfg)

	detector.AddWakeWord(WakeWord{
		Phrase:  "hey anyclaw",
		Aliases: []string{"hi anyclaw", "hello anyclaw"},
	})

	_, _, matched := detector.Detect("hi anyclaw")
	if !matched {
		t.Error("expected match for alias 'hi anyclaw'")
	}
}

func TestWakeWordCallback(t *testing.T) {
	cfg := WakeWordConfig{}
	detector := NewWakeWordDetector(cfg)

	called := make(chan string, 1)
	detector.AddWakeWord(WakeWord{
		Phrase: "hey anyclaw",
		Callback: func(phrase string) {
			called <- phrase
		},
	})

	detector.Detect("hey anyclaw")

	select {
	case phrase := <-called:
		if phrase != "hey anyclaw" {
			t.Errorf("expected callback with 'hey anyclaw', got %s", phrase)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected wake word callback")
	}
}

func TestWakeWordSensitivity(t *testing.T) {
	cfg := WakeWordConfig{
		WakeWords:   []string{"hey anyclaw"},
		Sensitivity: 0.5,
	}
	detector := NewWakeWordDetector(cfg)

	_, _, matched := detector.Detect("hey anycl")
	if !matched {
		t.Error("expected match with low sensitivity for close phrase")
	}

	detector.SetSensitivity(0.95)
	_, _, matched = detector.Detect("hey anycl")
	if matched {
		t.Error("expected no match with high sensitivity for close phrase")
	}
}

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		s1, s2   string
		expected int
	}{
		{"", "", 0},
		{"abc", "", 3},
		{"", "abc", 3},
		{"abc", "abc", 0},
		{"kitten", "sitting", 3},
		{"hey", "hay", 1},
	}

	for _, tt := range tests {
		dist := levenshteinDistance(tt.s1, tt.s2)
		if dist != tt.expected {
			t.Errorf("levenshtein(%q, %q) = %d, expected %d", tt.s1, tt.s2, dist, tt.expected)
		}
	}
}

func TestNormalizeText(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Hello World", "hello world"},
		{"HEY ANYCLAW!", "hey anyclaw"},
		{"  spaces  ", "spaces"},
		{"Hey, AnyClaw!", "hey anyclaw"},
	}

	for _, tt := range tests {
		result := normalizeText(tt.input)
		if result != tt.expected {
			t.Errorf("normalizeText(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

func TestMatchPhrase(t *testing.T) {
	tests := []struct {
		input   string
		phrase  string
		minConf float64
	}{
		{"hey anyclaw", "hey anyclaw", 1.0},
		{"hello hey anyclaw how are you", "hey anyclaw", 0.8},
		{"hey anycl", "hey anyclaw", 0.7},
	}

	for _, tt := range tests {
		conf := matchPhrase(tt.input, tt.phrase)
		if conf < tt.minConf {
			t.Errorf("matchPhrase(%q, %q) = %f, expected >= %f", tt.input, tt.phrase, conf, tt.minConf)
		}
	}
}

func TestVoiceWakeCreation(t *testing.T) {
	cfg := DefaultVoiceWakeConfig()
	vw := NewVoiceWake(cfg)

	if vw == nil {
		t.Fatal("expected VoiceWake instance, got nil")
	}

	if vw.State() != VoiceWakeStateIdle {
		t.Errorf("expected idle state, got %s", vw.State())
	}
}

func TestVoiceWakeDefaultConfig(t *testing.T) {
	cfg := DefaultVoiceWakeConfig()

	if cfg.SampleRate != 16000 {
		t.Errorf("expected sample rate 16000, got %d", cfg.SampleRate)
	}
	if cfg.Channels != 1 {
		t.Errorf("expected 1 channel, got %d", cfg.Channels)
	}
	if cfg.AutoTranscribe != true {
		t.Error("expected auto-transcribe to be enabled by default")
	}
}

func TestVoiceWakeStartStop(t *testing.T) {
	cfg := DefaultVoiceWakeConfig()
	vw := NewVoiceWake(cfg)

	ctx := t.Context()
	err := vw.Start(ctx)
	if err != nil {
		t.Errorf("expected successful start, got error: %v", err)
	}

	if vw.State() != VoiceWakeStateListening {
		t.Errorf("expected listening state after start, got %s", vw.State())
	}

	err = vw.Stop()
	if err != nil {
		t.Errorf("expected successful stop, got error: %v", err)
	}

	if vw.State() != VoiceWakeStateIdle {
		t.Errorf("expected idle state after stop, got %s", vw.State())
	}
}

func TestVoiceWakeDoubleStart(t *testing.T) {
	cfg := DefaultVoiceWakeConfig()
	vw := NewVoiceWake(cfg)

	ctx := t.Context()
	_ = vw.Start(ctx)

	err := vw.Start(ctx)
	if err == nil {
		t.Error("expected error on double start")
	}

	_ = vw.Stop()
}

func TestVoiceWakeListener(t *testing.T) {
	cfg := DefaultVoiceWakeConfig()
	vw := NewVoiceWake(cfg)

	events := make(chan VoiceWakeEvent, 10)
	vw.RegisterListener(func(event VoiceWakeEvent) {
		events <- event
	})

	ctx := t.Context()
	_ = vw.Start(ctx)

	select {
	case event := <-events:
		if event.Type != VoiceWakeEventStateChanged {
			t.Errorf("expected state_changed event, got %s", event.Type)
		}
		if event.State != VoiceWakeStateListening {
			t.Errorf("expected listening state, got %s", event.State)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected state change event on start")
	}

	_ = vw.Stop()
}

func TestVoiceWakeComponents(t *testing.T) {
	cfg := DefaultVoiceWakeConfig()
	vw := NewVoiceWake(cfg)

	vad := vw.VAD()
	if vad == nil {
		t.Error("expected VAD instance")
	}

	detector := vw.WakeDetector()
	if detector == nil {
		t.Error("expected WakeWordDetector instance")
	}
}

func TestVoiceWakeUpdateConfig(t *testing.T) {
	cfg := DefaultVoiceWakeConfig()
	vw := NewVoiceWake(cfg)

	newCfg := VoiceWakeConfig{
		SampleRate: 44100,
		Channels:   2,
	}
	vw.UpdateConfig(newCfg)

	updated := vw.Config()
	if updated.SampleRate != 44100 {
		t.Errorf("expected sample rate 44100, got %d", updated.SampleRate)
	}
	if updated.Channels != 2 {
		t.Errorf("expected 2 channels, got %d", updated.Channels)
	}
}

func TestVoiceWakeSetTranscriber(t *testing.T) {
	cfg := DefaultVoiceWakeConfig()
	vw := NewVoiceWake(cfg)

	manager := NewSTTManager()
	pipeline := NewSTTPipeline(manager, DefaultSTTPipelineConfig())

	vw.SetTranscriber(pipeline)

	if vw.transcriber != pipeline {
		t.Error("expected transcriber to be set")
	}
}

func TestVoiceWakeLastTranscript(t *testing.T) {
	cfg := DefaultVoiceWakeConfig()
	vw := NewVoiceWake(cfg)

	transcript := vw.LastTranscript()
	if transcript != "" {
		t.Errorf("expected empty transcript initially, got %s", transcript)
	}
}

func TestVoiceWakeLastWakeMatch(t *testing.T) {
	cfg := DefaultVoiceWakeConfig()
	vw := NewVoiceWake(cfg)

	phrase, confidence := vw.LastWakeMatch()
	if phrase != "" {
		t.Errorf("expected empty phrase initially, got %s", phrase)
	}
	if confidence != 0 {
		t.Errorf("expected zero confidence initially, got %f", confidence)
	}
}

func TestVoiceWakeEngineRouter(t *testing.T) {
	cfg := DefaultVoiceWakeConfig()
	vw := NewVoiceWake(cfg)

	router := vw.EngineRouter()
	if router == nil {
		t.Fatal("expected engine router")
	}

	engines := router.Engines()
	if len(engines) != 0 {
		t.Errorf("expected no engines initially, got %d", len(engines))
	}
}

func TestVoiceWakeRegisterMockEngine(t *testing.T) {
	cfg := DefaultVoiceWakeConfig()
	vw := NewVoiceWake(cfg)

	router := vw.EngineRouter()
	if router == nil {
		t.Fatal("expected engine router")
	}

	router.RegisterFactory("mock", func(cfg WakeWordEngineConfig) (WakeWordEngine, error) {
		return NewMockWakeWordEngine("test-word", 5), nil
	})

	engineCfg := WakeWordEngineConfig{
		Type:     "mock",
		Keywords: []string{"test-word"},
	}
	err := router.CreateEngine("mock", engineCfg)
	if err != nil {
		t.Fatalf("failed to create mock engine: %v", err)
	}

	engines := router.Engines()
	if len(engines) != 1 {
		t.Errorf("expected 1 engine, got %d", len(engines))
	}

	if router.ActiveEngine() == "" {
		t.Error("expected active engine after creation")
	}
}

func TestVoiceWakeEngineDetection(t *testing.T) {
	cfg := DefaultVoiceWakeConfig()
	vw := NewVoiceWake(cfg)

	router := vw.EngineRouter()
	router.RegisterFactory("mock", func(cfg WakeWordEngineConfig) (WakeWordEngine, error) {
		return NewMockWakeWordEngine("test-word", 3), nil
	})

	_ = router.CreateEngine("mock", WakeWordEngineConfig{
		Type:     "mock",
		Keywords: []string{"test-word"},
	})

	silence := make([]int16, 320)
	for i := 0; i < 2; i++ {
		result, detected := router.ProcessFrame(silence)
		if detected {
			t.Errorf("expected no detection on frame %d", i)
		}
		_ = result
	}

	result, detected := router.ProcessFrame(silence)
	if !detected {
		t.Error("expected detection on frame 3")
	}
	if result.Keyword != "test-word" {
		t.Errorf("expected keyword 'test-word', got %s", result.Keyword)
	}
	if result.Engine != "mock" {
		t.Errorf("expected engine 'mock', got %s", result.Engine)
	}
}

func TestVoiceWakeEngineAdapter(t *testing.T) {
	cfg := DefaultVoiceWakeConfig()
	vw := NewVoiceWake(cfg)

	adapter := vw.EngineAdapter()
	if adapter == nil {
		t.Fatal("expected engine adapter")
	}

	if adapter.UseEngine() {
		t.Error("expected engine not in use by default")
	}

	adapter.SetUseEngine(true)
	if !adapter.UseEngine() {
		t.Error("expected engine in use after SetUseEngine(true)")
	}

	adapter.SetUseEngine(false)
	if adapter.UseEngine() {
		t.Error("expected engine not in use after SetUseEngine(false)")
	}
}

func TestVoiceWakeEngineAdapterTranscript(t *testing.T) {
	cfg := DefaultVoiceWakeConfig()
	vw := NewVoiceWake(cfg)

	adapter := vw.EngineAdapter()
	phrase, confidence, matched := adapter.DetectTranscript("hey anyclaw")
	if !matched {
		t.Error("expected transcript match with builtin detector")
	}
	if phrase != "hey anyclaw" {
		t.Errorf("expected 'hey anyclaw', got %s", phrase)
	}
	if confidence != 1.0 {
		t.Errorf("expected confidence 1.0, got %f", confidence)
	}
}

func TestVoiceWakeSetActiveEngine(t *testing.T) {
	cfg := DefaultVoiceWakeConfig()
	vw := NewVoiceWake(cfg)

	router := vw.EngineRouter()
	router.RegisterFactory("mock1", func(cfg WakeWordEngineConfig) (WakeWordEngine, error) {
		return NewMockWakeWordEngine("word1", 5), nil
	})
	router.RegisterFactory("mock2", func(cfg WakeWordEngineConfig) (WakeWordEngine, error) {
		return NewMockWakeWordEngine("word2", 10), nil
	})

	_ = router.CreateEngine("mock1", WakeWordEngineConfig{Keywords: []string{"word1"}})
	_ = router.CreateEngine("mock2", WakeWordEngineConfig{Keywords: []string{"word2"}})

	engines := router.Engines()
	if len(engines) != 2 {
		t.Errorf("expected 2 engines, got %d", len(engines))
	}

	err := router.SetActive("mock-word2")
	if err != nil {
		t.Errorf("failed to set active engine: %v", err)
	}

	if router.ActiveEngine() != "mock-word2" {
		t.Errorf("expected active engine 'mock-word2', got %s", router.ActiveEngine())
	}

	err = router.SetActive("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent engine")
	}
}

func TestVoiceWakeEngineRouterReset(t *testing.T) {
	cfg := DefaultVoiceWakeConfig()
	vw := NewVoiceWake(cfg)

	router := vw.EngineRouter()
	router.RegisterFactory("mock", func(cfg WakeWordEngineConfig) (WakeWordEngine, error) {
		return NewMockWakeWordEngine("test", 3), nil
	})

	_ = router.CreateEngine("mock", WakeWordEngineConfig{Keywords: []string{"test"}})

	err := router.Reset()
	if err != nil {
		t.Errorf("failed to reset router: %v", err)
	}
}

func TestVoiceWakeEngineRouterClose(t *testing.T) {
	cfg := DefaultVoiceWakeConfig()
	vw := NewVoiceWake(cfg)

	router := vw.EngineRouter()
	router.RegisterFactory("mock", func(cfg WakeWordEngineConfig) (WakeWordEngine, error) {
		return NewMockWakeWordEngine("test", 3), nil
	})

	_ = router.CreateEngine("mock", WakeWordEngineConfig{Keywords: []string{"test"}})

	err := router.Close()
	if err != nil {
		t.Errorf("failed to close router: %v", err)
	}

	engines := router.Engines()
	if len(engines) != 0 {
		t.Errorf("expected no engines after close, got %d", len(engines))
	}
}

func TestVoiceWakeUseWakeWordEngine(t *testing.T) {
	cfg := DefaultVoiceWakeConfig()
	vw := NewVoiceWake(cfg)

	vw.UseWakeWordEngine(true)
	if !vw.IsUsingWakeWordEngine() {
		t.Error("expected engine to be in use")
	}

	vw.UseWakeWordEngine(false)
	if vw.IsUsingWakeWordEngine() {
		t.Error("expected engine not in use")
	}
}

func TestVoiceWakeAvailableEngines(t *testing.T) {
	cfg := DefaultVoiceWakeConfig()
	vw := NewVoiceWake(cfg)

	engines := vw.AvailableEngines()
	if len(engines) != 0 {
		t.Errorf("expected no engines initially, got %d", len(engines))
	}

	router := vw.EngineRouter()
	router.RegisterFactory("mock", func(cfg WakeWordEngineConfig) (WakeWordEngine, error) {
		return NewMockWakeWordEngine("test", 3), nil
	})

	_ = router.CreateEngine("mock", WakeWordEngineConfig{Keywords: []string{"test"}})

	engines = vw.AvailableEngines()
	if len(engines) != 1 {
		t.Errorf("expected 1 engine, got %d", len(engines))
	}
}

func TestVoiceWakeRegisterEngine(t *testing.T) {
	cfg := DefaultVoiceWakeConfig()
	vw := NewVoiceWake(cfg)

	router := vw.EngineRouter()
	router.RegisterFactory("mock", func(cfg WakeWordEngineConfig) (WakeWordEngine, error) {
		return NewMockWakeWordEngine("anycalw", 3), nil
	})

	err := vw.RegisterEngine("mock", WakeWordEngineConfig{
		Keywords: []string{"anycalw"},
	})
	if err != nil {
		t.Fatalf("failed to register engine: %v", err)
	}

	if !vw.IsUsingWakeWordEngine() {
		t.Error("expected engine to be in use after registration")
	}
}

func TestVoiceWakeClose(t *testing.T) {
	cfg := DefaultVoiceWakeConfig()
	vw := NewVoiceWake(cfg)

	router := vw.EngineRouter()
	router.RegisterFactory("mock", func(cfg WakeWordEngineConfig) (WakeWordEngine, error) {
		return NewMockWakeWordEngine("test", 3), nil
	})

	_ = router.CreateEngine("mock", WakeWordEngineConfig{Keywords: []string{"test"}})

	ctx := t.Context()
	_ = vw.Start(ctx)

	err := vw.Close()
	if err != nil {
		t.Errorf("failed to close voicewake: %v", err)
	}

	if vw.State() != VoiceWakeStateIdle {
		t.Errorf("expected idle state after close, got %s", vw.State())
	}
}

func TestMockWakeWordEngine(t *testing.T) {
	engine := NewMockWakeWordEngine("hello", 5)

	if engine.Name() != "mock-hello" {
		t.Errorf("expected name 'mock-hello', got %s", engine.Name())
	}
	if engine.Type() != "mock" {
		t.Errorf("expected type 'mock', got %s", engine.Type())
	}

	if err := engine.Init(); err != nil {
		t.Errorf("init failed: %v", err)
	}

	samples := make([]int16, 320)
	for i := 0; i < 4; i++ {
		result, detected := engine.ProcessFrame(samples)
		if detected {
			t.Errorf("expected no detection on frame %d", i)
		}
		_ = result
	}

	result, detected := engine.ProcessFrame(samples)
	if !detected {
		t.Error("expected detection on frame 5")
	}
	if result.Keyword != "hello" {
		t.Errorf("expected keyword 'hello', got %s", result.Keyword)
	}
	if result.Confidence != 0.95 {
		t.Errorf("expected confidence 0.95, got %f", result.Confidence)
	}

	if err := engine.Reset(); err != nil {
		t.Errorf("reset failed: %v", err)
	}

	result, detected = engine.ProcessFrame(samples)
	if detected {
		t.Error("expected no detection after reset")
	}
	_ = result

	if err := engine.Close(); err != nil {
		t.Errorf("close failed: %v", err)
	}
}

func TestWakeWordEngineConfig(t *testing.T) {
	cfg := WakeWordEngineConfig{
		Type:         EnginePorcupine,
		Keywords:     []string{"alexa"},
		AccessKey:    "test-key",
		Sensitivity:  0.7,
		SampleRate:   16000,
		FrameSize:    512,
		LibraryPath:  "/usr/lib/porcupine",
		ModelPath:    "/usr/lib/porcupine/model.pv",
		ResourcePath: "/usr/lib/porcupine/common.res",
		Extra:        map[string]string{"debug": "true"},
	}

	if cfg.Type != EnginePorcupine {
		t.Errorf("expected type porcupine, got %s", cfg.Type)
	}
	if len(cfg.Keywords) != 1 {
		t.Errorf("expected 1 keyword, got %d", len(cfg.Keywords))
	}
	if cfg.Sensitivity != 0.7 {
		t.Errorf("expected sensitivity 0.7, got %f", cfg.Sensitivity)
	}
}

func TestWakeWordDetectionResult(t *testing.T) {
	result := WakeWordDetectionResult{
		Keyword:    "hey assistant",
		Confidence: 0.85,
		Engine:     EnginePorcupine,
	}

	if result.Keyword != "hey assistant" {
		t.Errorf("expected keyword 'hey assistant', got %s", result.Keyword)
	}
	if result.Confidence != 0.85 {
		t.Errorf("expected confidence 0.85, got %f", result.Confidence)
	}
	if result.Engine != EnginePorcupine {
		t.Errorf("expected engine porcupine, got %s", result.Engine)
	}
}
