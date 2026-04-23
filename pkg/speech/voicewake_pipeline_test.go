package speech

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/capability/agents/prompt"
)

type mockAgent struct {
	mu        sync.Mutex
	responses []string
	inputs    []string
	delay     time.Duration
	err       error
}

func (m *mockAgent) Run(ctx context.Context, userInput string) (string, error) {
	m.mu.Lock()
	m.inputs = append(m.inputs, userInput)
	resp := "default response"
	if len(m.responses) > 0 {
		resp = m.responses[0]
		m.responses = m.responses[1:]
	}
	m.mu.Unlock()

	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}

	if m.err != nil {
		return "", m.err
	}

	return resp, nil
}

func (m *mockAgent) LastInput() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.inputs) == 0 {
		return ""
	}
	return m.inputs[len(m.inputs)-1]
}

func (m *mockAgent) InputCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.inputs)
}

type mockTTSProcessor struct {
	mu        sync.Mutex
	audio     []byte
	format    AudioFormat
	duration  time.Duration
	err       error
	callCount int
}

func (m *mockTTSProcessor) Synthesize(ctx context.Context, text string, opts ...SynthesizeOption) (*AudioResult, error) {
	m.mu.Lock()
	m.callCount++
	m.mu.Unlock()

	if m.err != nil {
		return nil, m.err
	}

	return &AudioResult{
		Data:     m.audio,
		Format:   m.format,
		Duration: m.duration,
	}, nil
}

func (m *mockTTSProcessor) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

type mockAudioPlayer struct {
	mu         sync.Mutex
	isPlaying  bool
	played     [][]byte
	formats    []AudioFormat
	playDelay  time.Duration
	stopCalled bool
}

func (m *mockAudioPlayer) Play(ctx context.Context, audio []byte, format AudioFormat) error {
	m.mu.Lock()
	m.isPlaying = true
	m.played = append(m.played, append([]byte(nil), audio...))
	m.formats = append(m.formats, format)
	m.mu.Unlock()

	if m.playDelay > 0 {
		select {
		case <-time.After(m.playDelay):
		case <-ctx.Done():
			m.mu.Lock()
			m.isPlaying = false
			m.mu.Unlock()
			return ctx.Err()
		}
	}

	m.mu.Lock()
	m.isPlaying = false
	m.mu.Unlock()

	return nil
}

func (m *mockAudioPlayer) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.isPlaying = false
	m.stopCalled = true
	return nil
}

func (m *mockAudioPlayer) IsPlaying() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.isPlaying
}

func (m *mockAudioPlayer) PlayCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.played)
}

func (m *mockAudioPlayer) LastAudio() []byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.played) == 0 {
		return nil
	}
	return m.played[len(m.played)-1]
}

func TestVoiceWakeProcessorCreation(t *testing.T) {
	cfg := DefaultVoiceWakeProcessorConfig()
	processor := NewVoiceWakeProcessor(cfg)

	if processor == nil {
		t.Fatal("expected processor instance")
	}

	if processor.IsProcessing() {
		t.Error("expected not processing initially")
	}
}

func TestVoiceWakeProcessorStart(t *testing.T) {
	vw := NewVoiceWake(DefaultVoiceWakeConfig())
	agent := &mockAgent{responses: []string{"hello"}}

	cfg := DefaultVoiceWakeProcessorConfig()
	cfg.VoiceWake = vw
	cfg.Agent = agent

	processor := NewVoiceWakeProcessor(cfg)

	err := processor.Start()
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}
}

func TestVoiceWakeProcessorStartMissingAgent(t *testing.T) {
	vw := NewVoiceWake(DefaultVoiceWakeConfig())

	cfg := DefaultVoiceWakeProcessorConfig()
	cfg.VoiceWake = vw
	cfg.Agent = nil

	processor := NewVoiceWakeProcessor(cfg)

	err := processor.Start()
	if err == nil {
		t.Error("expected error for missing agent")
	}
}

func TestVoiceWakeProcessorStartMissingVoiceWake(t *testing.T) {
	agent := &mockAgent{}

	cfg := DefaultVoiceWakeProcessorConfig()
	cfg.Agent = agent
	cfg.VoiceWake = nil

	processor := NewVoiceWakeProcessor(cfg)

	err := processor.Start()
	if err == nil {
		t.Error("expected error for missing voicewake")
	}
}

func TestVoiceWakeProcessorHandleWakeDetected(t *testing.T) {
	vw := NewVoiceWake(DefaultVoiceWakeConfig())
	agent := &mockAgent{responses: []string{"Hello! How can I help?"}}

	cfg := DefaultVoiceWakeProcessorConfig()
	cfg.VoiceWake = vw
	cfg.Agent = agent
	cfg.AutoSpeak = false

	processor := NewVoiceWakeProcessor(cfg)
	_ = processor.Start()

	event := VoiceWakeEvent{
		Type:      VoiceWakeEventWakeDetected,
		State:     VoiceWakeStateTriggered,
		Timestamp: time.Now(),
		Data: map[string]any{
			"transcript": "hey anyclaw what is the weather",
			"phrase":     "hey anyclaw",
			"confidence": 0.95,
		},
	}

	processor.onVoiceWakeEvent(event)

	time.Sleep(50 * time.Millisecond)

	if agent.InputCount() != 1 {
		t.Errorf("expected 1 agent call, got %d", agent.InputCount())
	}

	input := agent.LastInput()
	if input != "hey anyclaw what is the weather" {
		t.Errorf("expected input 'hey anyclaw what is the weather', got %q", input)
	}

	if processor.LastResponse() != "Hello! How can I help?" {
		t.Errorf("expected response 'Hello! How can I help?', got %q", processor.LastResponse())
	}
}

func TestVoiceWakeProcessorWithTTS(t *testing.T) {
	vw := NewVoiceWake(DefaultVoiceWakeConfig())
	agent := &mockAgent{responses: []string{"The weather is sunny"}}
	tts := &mockTTSProcessor{
		audio:    []byte("fake-audio-data"),
		format:   FormatWAV,
		duration: 2 * time.Second,
	}
	player := &mockAudioPlayer{}

	cfg := DefaultVoiceWakeProcessorConfig()
	cfg.VoiceWake = vw
	cfg.Agent = agent
	cfg.TTSProcessor = tts
	cfg.AudioPlayer = player
	cfg.AutoSpeak = true

	processor := NewVoiceWakeProcessor(cfg)
	_ = processor.Start()

	event := VoiceWakeEvent{
		Type:      VoiceWakeEventWakeDetected,
		State:     VoiceWakeStateTriggered,
		Timestamp: time.Now(),
		Data: map[string]any{
			"transcript": "hey anyclaw what is the weather",
		},
	}

	processor.onVoiceWakeEvent(event)

	time.Sleep(50 * time.Millisecond)

	if tts.CallCount() != 1 {
		t.Errorf("expected 1 TTS call, got %d", tts.CallCount())
	}

	if player.PlayCount() != 1 {
		t.Errorf("expected 1 playback, got %d", player.PlayCount())
	}

	if processor.LastAudioDuration() != 2*time.Second {
		t.Errorf("expected duration 2s, got %v", processor.LastAudioDuration())
	}
}

func TestVoiceWakeProcessorConcurrency(t *testing.T) {
	vw := NewVoiceWake(DefaultVoiceWakeConfig())
	agent := &mockAgent{responses: []string{"response1", "response2"}}

	cfg := DefaultVoiceWakeProcessorConfig()
	cfg.VoiceWake = vw
	cfg.Agent = agent
	cfg.AutoSpeak = false

	processor := NewVoiceWakeProcessor(cfg)
	_ = processor.Start()

	event1 := VoiceWakeEvent{
		Type:      VoiceWakeEventWakeDetected,
		Timestamp: time.Now(),
		Data:      map[string]any{"transcript": "first message"},
	}
	event2 := VoiceWakeEvent{
		Type:      VoiceWakeEventWakeDetected,
		Timestamp: time.Now(),
		Data:      map[string]any{"transcript": "second message"},
	}

	processor.onVoiceWakeEvent(event1)
	processor.onVoiceWakeEvent(event2)

	time.Sleep(50 * time.Millisecond)

	if agent.InputCount() != 1 {
		t.Errorf("expected only 1 agent call (second skipped due to processing), got %d", agent.InputCount())
	}
}

func TestVoiceWakeProcessorCancel(t *testing.T) {
	vw := NewVoiceWake(DefaultVoiceWakeConfig())
	agent := &mockAgent{
		responses: []string{"slow response"},
		delay:     500 * time.Millisecond,
	}

	cfg := DefaultVoiceWakeProcessorConfig()
	cfg.VoiceWake = vw
	cfg.Agent = agent
	cfg.AutoSpeak = false

	processor := NewVoiceWakeProcessor(cfg)
	_ = processor.Start()

	event := VoiceWakeEvent{
		Type:      VoiceWakeEventWakeDetected,
		Timestamp: time.Now(),
		Data:      map[string]any{"transcript": "cancel me"},
	}

	processor.onVoiceWakeEvent(event)

	time.Sleep(10 * time.Millisecond)

	if !processor.IsProcessing() {
		t.Error("expected processing to be active")
	}

	processor.CancelConversation()

	if processor.IsProcessing() {
		t.Error("expected processing to be cancelled")
	}
}

func TestVoiceWakeProcessorCallbacks(t *testing.T) {
	vw := NewVoiceWake(DefaultVoiceWakeConfig())
	agent := &mockAgent{responses: []string{"response"}}
	tts := &mockTTSProcessor{
		audio:    []byte("audio"),
		format:   FormatWAV,
		duration: 1 * time.Second,
	}
	player := &mockAudioPlayer{}

	responseCalled := make(chan string, 1)
	audioCalled := make(chan time.Duration, 1)

	cfg := DefaultVoiceWakeProcessorConfig()
	cfg.VoiceWake = vw
	cfg.Agent = agent
	cfg.TTSProcessor = tts
	cfg.AudioPlayer = player
	cfg.AutoSpeak = true
	cfg.OnResponse = func(text string) {
		responseCalled <- text
	}
	cfg.OnAudioComplete = func(dur time.Duration) {
		audioCalled <- dur
	}

	processor := NewVoiceWakeProcessor(cfg)
	_ = processor.Start()

	event := VoiceWakeEvent{
		Type:      VoiceWakeEventWakeDetected,
		Timestamp: time.Now(),
		Data:      map[string]any{"transcript": "test"},
	}

	processor.onVoiceWakeEvent(event)

	select {
	case resp := <-responseCalled:
		if resp != "response" {
			t.Errorf("expected response 'response', got %q", resp)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected OnResponse callback")
	}

	select {
	case dur := <-audioCalled:
		if dur != 1*time.Second {
			t.Errorf("expected duration 1s, got %v", dur)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected OnAudioComplete callback")
	}
}

func TestVoiceWakeProcessorUpdateConfig(t *testing.T) {
	vw := NewVoiceWake(DefaultVoiceWakeConfig())
	agent := &mockAgent{}

	cfg := DefaultVoiceWakeProcessorConfig()
	cfg.VoiceWake = vw
	cfg.Agent = agent

	processor := NewVoiceWakeProcessor(cfg)

	newAgent := &mockAgent{}
	processor.SetAgent(newAgent)

	if processor.cfg.Agent != newAgent {
		t.Error("expected agent to be updated")
	}
}

func TestLocalAudioPlayerCreation(t *testing.T) {
	cfg := DefaultLocalAudioPlayerConfig()
	player := NewLocalAudioPlayer(cfg)

	if player == nil {
		t.Fatal("expected player instance")
	}

	if player.IsPlaying() {
		t.Error("expected not playing initially")
	}
}

func TestLocalAudioPlayerAvailablePlayers(t *testing.T) {
	cfg := DefaultLocalAudioPlayerConfig()
	player := NewLocalAudioPlayer(cfg)

	players := player.AvailablePlayers()
	// At minimum, should return empty or available players
	if players == nil {
		t.Error("expected non-nil player list")
	}
}

func TestLocalAudioPlayerSetPlayer(t *testing.T) {
	cfg := DefaultLocalAudioPlayerConfig()
	player := NewLocalAudioPlayer(cfg)

	player.SetPlayer("test-player")
	if player.Player() != "test-player" {
		t.Errorf("expected player 'test-player', got %s", player.Player())
	}
}

func TestLocalAudioPlayerVolume(t *testing.T) {
	cfg := DefaultLocalAudioPlayerConfig()
	player := NewLocalAudioPlayer(cfg)

	player.SetVolume(0.5)
	if player.Volume() != 0.5 {
		t.Errorf("expected volume 0.5, got %f", player.Volume())
	}

	player.SetVolume(3.0)
	if player.Volume() != 0.5 {
		t.Error("expected volume unchanged after invalid set")
	}
}

func TestBufferAudioPlayer(t *testing.T) {
	player := NewBufferAudioPlayer()

	if player.IsPlaying() {
		t.Error("expected not playing initially")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	go func() {
		time.Sleep(10 * time.Millisecond)
		player.Stop()
	}()

	err := player.Play(ctx, []byte("audio"), FormatWAV)
	if err != nil {
		t.Errorf("play failed: %v", err)
	}

	buf := player.Buffer()
	if string(buf) != "audio" {
		t.Errorf("expected buffer 'audio', got %s", string(buf))
	}

	if player.Format() != FormatWAV {
		t.Errorf("expected format WAV, got %s", player.Format())
	}
}

func TestMultiAudioPlayer(t *testing.T) {
	player1 := &mockAudioPlayer{}
	player2 := &mockAudioPlayer{}

	multi := NewMultiAudioPlayer(player1, player2)

	ctx := context.Background()
	err := multi.Play(ctx, []byte("test"), FormatMP3)
	if err != nil {
		t.Errorf("play failed: %v", err)
	}

	if player1.PlayCount() != 1 {
		t.Error("expected first player to be used")
	}

	err = multi.SetPlayer(1)
	if err != nil {
		t.Errorf("set player failed: %v", err)
	}

	err = multi.Play(ctx, []byte("test2"), FormatMP3)
	if err != nil {
		t.Errorf("play failed: %v", err)
	}

	if player2.PlayCount() != 1 {
		t.Error("expected second player to be used after switch")
	}
}

func TestNopAudioPlayer(t *testing.T) {
	player := NopAudioPlayer{}

	ctx := context.Background()
	err := player.Play(ctx, []byte("test"), FormatWAV)
	if err != nil {
		t.Errorf("nop play failed: %v", err)
	}

	if player.IsPlaying() {
		t.Error("nop player should never be playing")
	}

	err = player.Stop()
	if err != nil {
		t.Errorf("nop stop failed: %v", err)
	}
}

func TestVoiceWakeSessionManagerCreation(t *testing.T) {
	cfg := DefaultVoiceWakeSessionManagerConfig()
	mgr := NewVoiceWakeSessionManager(cfg)

	if mgr == nil {
		t.Fatal("expected session manager")
	}

	if mgr.SessionCount() != 0 {
		t.Errorf("expected 0 sessions, got %d", mgr.SessionCount())
	}
}

func TestVoiceWakeSessionManagerCreateAndGet(t *testing.T) {
	cfg := DefaultVoiceWakeSessionManagerConfig()
	mgr := NewVoiceWakeSessionManager(cfg)

	session := mgr.CreateSession("test-session")
	if session == nil {
		t.Fatal("expected session")
	}

	if session.ID != "test-session" {
		t.Errorf("expected ID 'test-session', got %s", session.ID)
	}

	if mgr.SessionCount() != 1 {
		t.Errorf("expected 1 session, got %d", mgr.SessionCount())
	}

	retrieved, ok := mgr.GetSession("test-session")
	if !ok {
		t.Error("expected to find session")
	}
	if retrieved.ID != "test-session" {
		t.Errorf("expected ID 'test-session', got %s", retrieved.ID)
	}

	_, ok = mgr.GetSession("nonexistent")
	if ok {
		t.Error("expected not to find nonexistent session")
	}
}

func TestVoiceWakeSessionManagerGetOrCreate(t *testing.T) {
	cfg := DefaultVoiceWakeSessionManagerConfig()
	mgr := NewVoiceWakeSessionManager(cfg)

	session := mgr.GetOrCreateSession("new-session")
	if session == nil {
		t.Fatal("expected session")
	}

	if mgr.SessionCount() != 1 {
		t.Errorf("expected 1 session, got %d", mgr.SessionCount())
	}

	same := mgr.GetOrCreateSession("new-session")
	if same.ID != session.ID {
		t.Error("expected same session")
	}

	if mgr.SessionCount() != 1 {
		t.Errorf("expected still 1 session, got %d", mgr.SessionCount())
	}
}

func TestVoiceWakeSessionManagerAddTranscript(t *testing.T) {
	cfg := DefaultVoiceWakeSessionManagerConfig()
	mgr := NewVoiceWakeSessionManager(cfg)
	mgr.CreateSession("test")

	entry := TranscriptEntry{
		Text:       "hello world",
		Confidence: 0.9,
		Language:   "en",
		Duration:   3 * time.Second,
		Timestamp:  time.Now(),
	}

	err := mgr.AddTranscript("test", entry)
	if err != nil {
		t.Errorf("add transcript failed: %v", err)
	}

	transcripts, err := mgr.RecentTranscripts("test", 10)
	if err != nil {
		t.Errorf("recent transcripts failed: %v", err)
	}

	if len(transcripts) != 1 {
		t.Errorf("expected 1 transcript, got %d", len(transcripts))
	}
	if transcripts[0].Text != "hello world" {
		t.Errorf("expected text 'hello world', got %s", transcripts[0].Text)
	}
}

func TestVoiceWakeSessionManagerAddResponse(t *testing.T) {
	cfg := DefaultVoiceWakeSessionManagerConfig()
	mgr := NewVoiceWakeSessionManager(cfg)
	mgr.CreateSession("test")

	entry := ResponseEntry{
		Text:      "Hi there!",
		Duration:  2 * time.Second,
		Timestamp: time.Now(),
		IsSpoken:  true,
	}

	err := mgr.AddResponse("test", entry)
	if err != nil {
		t.Errorf("add response failed: %v", err)
	}

	responses, err := mgr.RecentResponses("test", 10)
	if err != nil {
		t.Errorf("recent responses failed: %v", err)
	}

	if len(responses) != 1 {
		t.Errorf("expected 1 response, got %d", len(responses))
	}
}

func TestVoiceWakeSessionManagerAddWakeEvent(t *testing.T) {
	cfg := DefaultVoiceWakeSessionManagerConfig()
	mgr := NewVoiceWakeSessionManager(cfg)
	mgr.CreateSession("test")

	entry := WakeEventEntry{
		Phrase:     "hey anyclaw",
		Confidence: 0.95,
		Engine:     "builtin",
		Timestamp:  time.Now(),
	}

	err := mgr.AddWakeEvent("test", entry)
	if err != nil {
		t.Errorf("add wake event failed: %v", err)
	}

	session, _ := mgr.GetSession("test")
	if len(session.WakeEvents) != 1 {
		t.Errorf("expected 1 wake event, got %d", len(session.WakeEvents))
	}
}

func TestVoiceWakeSessionManagerHistory(t *testing.T) {
	cfg := DefaultVoiceWakeSessionManagerConfig()
	mgr := NewVoiceWakeSessionManager(cfg)
	mgr.CreateSession("test")

	err := mgr.AddToHistory("test", prompt.Message{Role: "user", Content: "hello"})
	if err != nil {
		t.Errorf("add history failed: %v", err)
	}

	err = mgr.AddToHistory("test", prompt.Message{Role: "assistant", Content: "hi"})
	if err != nil {
		t.Errorf("add history failed: %v", err)
	}

	history, err := mgr.GetHistory("test")
	if err != nil {
		t.Errorf("get history failed: %v", err)
	}

	if len(history) != 2 {
		t.Errorf("expected 2 history entries, got %d", len(history))
	}
}

func TestVoiceWakeSessionManagerDelete(t *testing.T) {
	cfg := DefaultVoiceWakeSessionManagerConfig()
	mgr := NewVoiceWakeSessionManager(cfg)
	mgr.CreateSession("s1")
	mgr.CreateSession("s2")

	err := mgr.DeleteSession("s1")
	if err != nil {
		t.Errorf("delete session failed: %v", err)
	}

	if mgr.SessionCount() != 1 {
		t.Errorf("expected 1 session after delete, got %d", mgr.SessionCount())
	}
}

func TestVoiceWakeSessionManagerActive(t *testing.T) {
	cfg := DefaultVoiceWakeSessionManagerConfig()
	mgr := NewVoiceWakeSessionManager(cfg)
	mgr.CreateSession("s1")
	mgr.CreateSession("s2")

	if mgr.ActiveSessionID() != "s2" {
		t.Errorf("expected active session 's2', got %s", mgr.ActiveSessionID())
	}

	err := mgr.SetActive("s1")
	if err != nil {
		t.Errorf("set active failed: %v", err)
	}

	if mgr.ActiveSessionID() != "s1" {
		t.Errorf("expected active session 's1', got %s", mgr.ActiveSessionID())
	}
}

func TestVoiceWakeSessionManagerEviction(t *testing.T) {
	cfg := VoiceWakeSessionManagerConfig{MaxSessions: 2, MaxHistory: 50}
	mgr := NewVoiceWakeSessionManager(cfg)

	mgr.CreateSession("s1")
	time.Sleep(1 * time.Millisecond)
	mgr.CreateSession("s2")
	time.Sleep(1 * time.Millisecond)
	mgr.CreateSession("s3")

	if mgr.SessionCount() > 2 {
		t.Errorf("expected max 2 sessions, got %d", mgr.SessionCount())
	}
}

func TestVoiceWakeSessionManagerStats(t *testing.T) {
	cfg := DefaultVoiceWakeSessionManagerConfig()
	mgr := NewVoiceWakeSessionManager(cfg)
	mgr.CreateSession("test")

	_ = mgr.AddTranscript("test", TranscriptEntry{Text: "hi", Timestamp: time.Now()})
	_ = mgr.AddResponse("test", ResponseEntry{Text: "hello", Timestamp: time.Now()})
	_ = mgr.AddWakeEvent("test", WakeEventEntry{Phrase: "hey", Timestamp: time.Now()})

	stats, err := mgr.SessionStats("test")
	if err != nil {
		t.Errorf("session stats failed: %v", err)
	}

	if stats["transcript_count"] != 1 {
		t.Errorf("expected 1 transcript, got %v", stats["transcript_count"])
	}
	if stats["response_count"] != 1 {
		t.Errorf("expected 1 response, got %v", stats["response_count"])
	}
	if stats["wake_event_count"] != 1 {
		t.Errorf("expected 1 wake event, got %v", stats["wake_event_count"])
	}
}

func TestVoiceWakeSessionTracker(t *testing.T) {
	mgr := NewVoiceWakeSessionManager(DefaultVoiceWakeSessionManagerConfig())
	tracker := NewVoiceWakeSessionTracker(mgr)

	sc, err := tracker.StartSession("test")
	if err != nil {
		t.Fatalf("start session failed: %v", err)
	}

	if sc.Session.ID != "test" {
		t.Errorf("expected session ID 'test', got %s", sc.Session.ID)
	}

	if tracker.ActiveSessionID() != "test" {
		t.Errorf("expected active session 'test', got %s", tracker.ActiveSessionID())
	}

	err = tracker.EndSession("test")
	if err != nil {
		t.Errorf("end session failed: %v", err)
	}

	if tracker.ActiveSession() != nil {
		t.Error("expected no active session after end")
	}
}

func TestVoiceWakeIntegrationCreation(t *testing.T) {
	cfg := DefaultVoiceWakeIntegrationConfig()
	integration := NewVoiceWakeIntegration(cfg)

	if integration == nil {
		t.Fatal("expected integration instance")
	}

	if integration.IsRunning() {
		t.Error("expected not running initially")
	}
}

func TestVoiceWakeIntegrationInit(t *testing.T) {
	cfg := DefaultVoiceWakeIntegrationConfig()
	integration := NewVoiceWakeIntegration(cfg)

	agent := &mockAgent{}
	tts := &mockTTSProcessor{}

	err := integration.Init(agent, tts)
	if err != nil {
		t.Fatalf("init failed: %v", err)
	}

	if integration.VoiceWake() == nil {
		t.Error("expected VoiceWake after init")
	}
	if integration.Processor() == nil {
		t.Error("expected Processor after init")
	}
	if integration.SessionManager() == nil {
		t.Error("expected SessionManager after init")
	}
	if integration.AudioPlayer() == nil {
		t.Error("expected AudioPlayer after init")
	}
}

func TestVoiceWakeIntegrationStartStop(t *testing.T) {
	cfg := DefaultVoiceWakeIntegrationConfig()
	integration := NewVoiceWakeIntegration(cfg)

	agent := &mockAgent{}
	tts := &mockTTSProcessor{}

	_ = integration.Init(agent, tts)

	ctx := context.Background()
	err := integration.Start(ctx)
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}

	if !integration.IsRunning() {
		t.Error("expected running after start")
	}

	err = integration.Stop()
	if err != nil {
		t.Fatalf("stop failed: %v", err)
	}

	if integration.IsRunning() {
		t.Error("expected not running after stop")
	}
}

func TestVoiceWakeIntegrationStats(t *testing.T) {
	cfg := DefaultVoiceWakeIntegrationConfig()
	integration := NewVoiceWakeIntegration(cfg)

	agent := &mockAgent{}
	tts := &mockTTSProcessor{}

	_ = integration.Init(agent, tts)

	stats := integration.Stats()
	if stats.WakeCount != 0 {
		t.Errorf("expected 0 wake count, got %d", stats.WakeCount)
	}
}

func TestVoiceWakeIntegrationSetters(t *testing.T) {
	cfg := DefaultVoiceWakeIntegrationConfig()
	integration := NewVoiceWakeIntegration(cfg)

	agent := &mockAgent{}
	tts := &mockTTSProcessor{}

	_ = integration.Init(agent, tts)

	newAgent := &mockAgent{}
	integration.SetAgent(newAgent)

	integration.SetAutoSpeak(false)
	if integration.processor.cfg.AutoSpeak {
		t.Error("expected auto-speak disabled")
	}

	integration.SetSensitivity(0.8)
	if integration.voiceWake.WakeDetector().Sensitivity() != 0.8 {
		t.Errorf("expected sensitivity 0.8, got %f", integration.voiceWake.WakeDetector().Sensitivity())
	}
}

func TestVoiceWakeIntegrationCancel(t *testing.T) {
	cfg := DefaultVoiceWakeIntegrationConfig()
	integration := NewVoiceWakeIntegration(cfg)

	agent := &mockAgent{}
	tts := &mockTTSProcessor{}

	_ = integration.Init(agent, tts)

	integration.CancelConversation()
	if integration.IsProcessing() {
		t.Error("expected not processing after cancel")
	}
}

func TestVoiceWakeIntegrationHistory(t *testing.T) {
	cfg := DefaultVoiceWakeIntegrationConfig()
	integration := NewVoiceWakeIntegration(cfg)

	agent := &mockAgent{}
	tts := &mockTTSProcessor{}

	_ = integration.Init(agent, tts)

	_ = integration.AddToHistory("user", "hello")
	_ = integration.AddToHistory("assistant", "hi")

	history, err := integration.GetHistory()
	if err != nil {
		t.Errorf("get history failed: %v", err)
	}

	if len(history) != 2 {
		t.Errorf("expected 2 history entries, got %d", len(history))
	}
}

func TestVoiceWakeIntegrationNoAgentError(t *testing.T) {
	vw := NewVoiceWake(DefaultVoiceWakeConfig())

	cfg := DefaultVoiceWakeProcessorConfig()
	cfg.VoiceWake = vw
	cfg.Agent = nil

	processor := NewVoiceWakeProcessor(cfg)
	_ = processor.Start()

	event := VoiceWakeEvent{
		Type:      VoiceWakeEventWakeDetected,
		Timestamp: time.Now(),
		Data:      map[string]any{"transcript": "test"},
	}

	processor.onVoiceWakeEvent(event)

	time.Sleep(50 * time.Millisecond)

	if processor.LastResponse() != "" {
		t.Error("expected empty response when no agent")
	}
}

func TestVoiceWakeProcessorEmptyTranscript(t *testing.T) {
	vw := NewVoiceWake(DefaultVoiceWakeConfig())
	agent := &mockAgent{}

	cfg := DefaultVoiceWakeProcessorConfig()
	cfg.VoiceWake = vw
	cfg.Agent = agent
	cfg.AutoSpeak = false

	processor := NewVoiceWakeProcessor(cfg)
	_ = processor.Start()

	event := VoiceWakeEvent{
		Type:      VoiceWakeEventWakeDetected,
		Timestamp: time.Now(),
		Data:      map[string]any{"transcript": ""},
	}

	processor.onVoiceWakeEvent(event)

	time.Sleep(50 * time.Millisecond)

	if agent.InputCount() != 0 {
		t.Errorf("expected no agent call for empty transcript, got %d", agent.InputCount())
	}
}

func TestVoiceWakeProcessorErrorCallback(t *testing.T) {
	vw := NewVoiceWake(DefaultVoiceWakeConfig())
	agent := &mockAgent{
		responses: []string{"ok"},
		delay:     10 * time.Millisecond,
	}
	tts := &mockTTSProcessor{
		err: errors.New("TTS failed"),
	}
	player := &mockAudioPlayer{}

	errCh := make(chan error, 1)

	cfg := DefaultVoiceWakeProcessorConfig()
	cfg.VoiceWake = vw
	cfg.Agent = agent
	cfg.TTSProcessor = tts
	cfg.AudioPlayer = player
	cfg.AutoSpeak = true
	cfg.OnError = func(err error) {
		errCh <- err
	}

	processor := NewVoiceWakeProcessor(cfg)
	_ = processor.Start()

	event := VoiceWakeEvent{
		Type:      VoiceWakeEventWakeDetected,
		Timestamp: time.Now(),
		Data:      map[string]any{"transcript": "test"},
	}

	processor.onVoiceWakeEvent(event)

	select {
	case err := <-errCh:
		if err == nil {
			t.Error("expected error in callback")
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("expected OnError callback")
	}
}
