package speech

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

type VoiceWakeState string

const (
	VoiceWakeStateIdle       VoiceWakeState = "idle"
	VoiceWakeStateListening  VoiceWakeState = "listening"
	VoiceWakeStateRecording  VoiceWakeState = "recording"
	VoiceWakeStateProcessing VoiceWakeState = "processing"
	VoiceWakeStateTriggered  VoiceWakeState = "triggered"
)

type VoiceWakeEventType string

const (
	VoiceWakeEventStateChanged VoiceWakeEventType = "state_changed"
	VoiceWakeEventWakeDetected VoiceWakeEventType = "wake_detected"
	VoiceWakeEventSpeechStart  VoiceWakeEventType = "speech_start"
	VoiceWakeEventSpeechEnd    VoiceWakeEventType = "speech_end"
	VoiceWakeEventError        VoiceWakeEventType = "error"
)

type VoiceWakeEvent struct {
	Type      VoiceWakeEventType
	State     VoiceWakeState
	Timestamp time.Time
	Data      map[string]any
}

type VoiceWakeListener func(event VoiceWakeEvent)

type AudioSource interface {
	Start(ctx context.Context) error
	Stop() error
	Read(samples []int16) (int, error)
	SampleRate() int
	Channels() int
}

type VoiceWakeConfig struct {
	VADConfig        VADConfig
	WakeWordConfig   WakeWordConfig
	EngineConfig     WakeWordEngineConfig
	SampleRate       int
	Channels         int
	FrameSize        int
	MaxRecordingTime time.Duration
	CooldownTime     time.Duration
	AudioSource      AudioSource
	STTPipeline      *STTPipeline
	AutoTranscribe   bool
	WakeWordEngine   WakeWordEngineType
}

func DefaultVoiceWakeConfig() VoiceWakeConfig {
	return VoiceWakeConfig{
		VADConfig:        DefaultVADConfig(),
		WakeWordConfig:   DefaultWakeWordConfig(),
		SampleRate:       16000,
		Channels:         1,
		FrameSize:        320,
		MaxRecordingTime: 30 * time.Second,
		CooldownTime:     2 * time.Second,
		AutoTranscribe:   true,
	}
}

type VoiceWake struct {
	mu              sync.Mutex
	cfg             VoiceWakeConfig
	state           VoiceWakeState
	vad             *VAD
	wakeDetector    *WakeWordDetector
	engineRouter    *WakeWordEngineRouter
	engineAdapter   *WakeWordEngineAdapter
	listeners       []VoiceWakeListener
	audioBuffer     []int16
	recordingBuffer []int16
	isRecording     bool
	recordingStart  time.Time
	cooldownUntil   time.Time
	ctx             context.Context
	cancel          context.CancelFunc
	wg              sync.WaitGroup
	transcriber     *STTPipeline
	lastTranscript  string
	lastWakeMatch   string
	lastConfidence  float64
	lastEnergy      float64
}

func NewVoiceWake(cfg VoiceWakeConfig) *VoiceWake {
	if cfg.SampleRate == 0 {
		cfg.SampleRate = 16000
	}
	if cfg.Channels == 0 {
		cfg.Channels = 1
	}
	if cfg.FrameSize == 0 {
		cfg.FrameSize = 320
	}
	if cfg.MaxRecordingTime == 0 {
		cfg.MaxRecordingTime = 30 * time.Second
	}
	if cfg.CooldownTime == 0 {
		cfg.CooldownTime = 2 * time.Second
	}

	cfg.VADConfig.SampleRate = cfg.SampleRate
	cfg.VADConfig.FrameSize = cfg.FrameSize

	cfg.EngineConfig.SampleRate = cfg.SampleRate
	cfg.EngineConfig.FrameSize = cfg.FrameSize

	vad := NewVAD(cfg.VADConfig)
	wakeDetector := NewWakeWordDetector(cfg.WakeWordConfig)

	router := NewWakeWordEngineRouter(cfg.EngineConfig)
	adapter := NewWakeWordEngineAdapter(router, wakeDetector)

	vw := &VoiceWake{
		cfg:           cfg,
		state:         VoiceWakeStateIdle,
		vad:           vad,
		wakeDetector:  wakeDetector,
		engineRouter:  router,
		engineAdapter: adapter,
		transcriber:   cfg.STTPipeline,
	}

	vad.RegisterListener(vw.onVADStateChanged)

	return vw
}

func (vw *VoiceWake) RegisterListener(listener VoiceWakeListener) {
	vw.mu.Lock()
	defer vw.mu.Unlock()
	vw.listeners = append(vw.listeners, listener)
}

func (vw *VoiceWake) Start(ctx context.Context) error {
	vw.mu.Lock()
	if vw.state != VoiceWakeStateIdle {
		vw.mu.Unlock()
		return fmt.Errorf("voicewake: already in state %s", vw.state)
	}
	vw.state = VoiceWakeStateListening
	vw.mu.Unlock()

	vw.ctx, vw.cancel = context.WithCancel(ctx)

	if vw.cfg.AudioSource != nil {
		if err := vw.cfg.AudioSource.Start(vw.ctx); err != nil {
			vw.mu.Lock()
			vw.state = VoiceWakeStateIdle
			vw.mu.Unlock()
			return fmt.Errorf("voicewake: failed to start audio source: %w", err)
		}
	}

	vw.wg.Add(1)
	go vw.listenLoop()

	vw.notifyListeners(VoiceWakeEvent{
		Type:      VoiceWakeEventStateChanged,
		State:     VoiceWakeStateListening,
		Timestamp: time.Now(),
		Data:      map[string]any{"message": "Voice wake listener started"},
	})

	return nil
}

func (vw *VoiceWake) Stop() error {
	vw.mu.Lock()
	if vw.state == VoiceWakeStateIdle {
		vw.mu.Unlock()
		return nil
	}

	if vw.cancel != nil {
		vw.cancel()
	}
	vw.state = VoiceWakeStateIdle
	vw.mu.Unlock()

	if vw.cfg.AudioSource != nil {
		_ = vw.cfg.AudioSource.Stop()
	}

	vw.wg.Wait()

	vw.notifyListeners(VoiceWakeEvent{
		Type:      VoiceWakeEventStateChanged,
		State:     VoiceWakeStateIdle,
		Timestamp: time.Now(),
		Data:      map[string]any{"message": "Voice wake listener stopped"},
	})

	return nil
}

func (vw *VoiceWake) listenLoop() {
	defer vw.wg.Done()

	samples := make([]int16, vw.cfg.FrameSize)

	for {
		select {
		case <-vw.ctx.Done():
			return
		default:
		}

		var n int
		var err error

		if vw.cfg.AudioSource != nil {
			n, err = vw.cfg.AudioSource.Read(samples)
			if err != nil {
				log.Printf("voicewake: error reading audio: %v", err)
				time.Sleep(10 * time.Millisecond)
				continue
			}
		} else {
			time.Sleep(time.Duration(vw.cfg.FrameSize) * time.Second / time.Duration(vw.cfg.SampleRate))
			continue
		}

		if n == 0 {
			continue
		}

		vw.mu.Lock()
		inCooldown := time.Now().Before(vw.cooldownUntil)
		vw.mu.Unlock()

		if inCooldown {
			continue
		}

		if vw.engineAdapter != nil && vw.engineAdapter.UseEngine() {
			result, detected := vw.engineAdapter.ProcessFrame(samples[:n])
			if detected && result != nil {
				vw.mu.Lock()
				vw.lastWakeMatch = result.Keyword
				vw.lastConfidence = result.Confidence
				vw.cooldownUntil = time.Now().Add(vw.cfg.CooldownTime)
				vw.mu.Unlock()

				vw.notifyListeners(VoiceWakeEvent{
					Type:      VoiceWakeEventWakeDetected,
					State:     VoiceWakeStateTriggered,
					Timestamp: time.Now(),
					Data: map[string]any{
						"phrase":     result.Keyword,
						"confidence": result.Confidence,
						"engine":     string(result.Engine),
						"energy":     0.0,
					},
				})

				vw.mu.Lock()
				vw.setState(VoiceWakeStateTriggered)
				vw.mu.Unlock()

				time.Sleep(vw.cfg.CooldownTime)

				vw.mu.Lock()
				vw.setState(VoiceWakeStateListening)
				vw.mu.Unlock()

				continue
			}
		}

		vw.processAudio(samples[:n])
	}
}

func (vw *VoiceWake) processAudio(samples []int16) {
	vw.mu.Lock()
	vw.audioBuffer = append(vw.audioBuffer, samples...)
	vw.mu.Unlock()

	state := vw.vad.ProcessFrame(samples)

	switch state {
	case VADStateSpeech:
		vw.mu.Lock()
		if !vw.isRecording {
			vw.isRecording = true
			vw.recordingStart = time.Now()
			vw.recordingBuffer = make([]int16, 0, vw.cfg.SampleRate*int(vw.cfg.MaxRecordingTime.Seconds()))
			vw.setState(VoiceWakeStateRecording)
		}
		vw.recordingBuffer = append(vw.recordingBuffer, samples...)
		vw.mu.Unlock()

	case VADStateSilence:
		vw.mu.Lock()
		if vw.isRecording {
			vw.isRecording = false
			recording := make([]int16, len(vw.recordingBuffer))
			copy(recording, vw.recordingBuffer)
			vw.recordingBuffer = nil
			vw.mu.Unlock()

			vw.processRecording(recording)
		} else {
			vw.mu.Unlock()
		}
	}
}

func (vw *VoiceWake) processRecording(samples []int16) {
	if len(samples) == 0 {
		return
	}

	vw.mu.Lock()
	vw.setState(VoiceWakeStateProcessing)
	vw.mu.Unlock()

	if vw.cfg.AutoTranscribe && vw.transcriber != nil {
		audioData := Int16ToWAV(samples, vw.cfg.SampleRate, vw.cfg.Channels)

		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			result, err := vw.transcriber.TranscribeDirect(ctx, audioData, WithSTTInputFormat(InputWAV))
			if err != nil {
				log.Printf("voicewake: transcription error: %v", err)
				vw.notifyListeners(VoiceWakeEvent{
					Type:      VoiceWakeEventError,
					State:     VoiceWakeStateProcessing,
					Timestamp: time.Now(),
					Data:      map[string]any{"error": err.Error()},
				})
				vw.mu.Lock()
				vw.setState(VoiceWakeStateListening)
				vw.mu.Unlock()
				return
			}

			vw.mu.Lock()
			vw.lastTranscript = result.Text
			vw.mu.Unlock()

			vw.checkWakeWord(result.Text)
		}()
	} else {
		vw.mu.Lock()
		vw.setState(VoiceWakeStateListening)
		vw.mu.Unlock()
	}
}

func (vw *VoiceWake) checkWakeWord(transcript string) {
	if transcript == "" {
		vw.mu.Lock()
		vw.setState(VoiceWakeStateListening)
		vw.mu.Unlock()
		return
	}

	phrase, confidence, matched := vw.wakeDetector.Detect(transcript)

	vw.mu.Lock()
	vw.lastTranscript = transcript
	vw.lastWakeMatch = phrase
	vw.lastConfidence = confidence
	vw.mu.Unlock()

	if matched {
		vw.mu.Lock()
		vw.setState(VoiceWakeStateTriggered)
		vw.cooldownUntil = time.Now().Add(vw.cfg.CooldownTime)
		energy := vw.lastEnergy
		vw.mu.Unlock()

		vw.notifyListeners(VoiceWakeEvent{
			Type:      VoiceWakeEventWakeDetected,
			State:     VoiceWakeStateTriggered,
			Timestamp: time.Now(),
			Data: map[string]any{
				"phrase":     phrase,
				"confidence": confidence,
				"transcript": transcript,
				"energy":     energy,
			},
		})

		time.Sleep(vw.cfg.CooldownTime)

		vw.mu.Lock()
		vw.setState(VoiceWakeStateListening)
		vw.mu.Unlock()
	} else {
		vw.mu.Lock()
		vw.setState(VoiceWakeStateListening)
		vw.mu.Unlock()
	}
}

func (vw *VoiceWake) onVADStateChanged(state VADState, energy float64, zcr float64) {
	vw.mu.Lock()
	vw.lastEnergy = energy
	vw.mu.Unlock()

	switch state {
	case VADStateSpeech:
		vw.notifyListeners(VoiceWakeEvent{
			Type:      VoiceWakeEventSpeechStart,
			State:     vw.State(),
			Timestamp: time.Now(),
			Data: map[string]any{
				"energy": energy,
				"zcr":    zcr,
			},
		})

	case VADStateSilence:
		vw.notifyListeners(VoiceWakeEvent{
			Type:      VoiceWakeEventSpeechEnd,
			State:     vw.State(),
			Timestamp: time.Now(),
			Data: map[string]any{
				"energy": energy,
				"zcr":    zcr,
			},
		})
	}
}

func (vw *VoiceWake) setState(state VoiceWakeState) {
	oldState := vw.state
	vw.state = state

	if oldState != state {
		vw.notifyListeners(VoiceWakeEvent{
			Type:      VoiceWakeEventStateChanged,
			State:     state,
			Timestamp: time.Now(),
			Data: map[string]any{
				"previous_state": oldState,
				"new_state":      state,
			},
		})
	}
}

func (vw *VoiceWake) State() VoiceWakeState {
	vw.mu.Lock()
	defer vw.mu.Unlock()
	return vw.state
}

func (vw *VoiceWake) notifyListeners(event VoiceWakeEvent) {
	vw.mu.Lock()
	listeners := make([]VoiceWakeListener, len(vw.listeners))
	copy(listeners, vw.listeners)
	vw.mu.Unlock()

	for _, listener := range listeners {
		listener(event)
	}
}

func (vw *VoiceWake) LastTranscript() string {
	vw.mu.Lock()
	defer vw.mu.Unlock()
	return vw.lastTranscript
}

func (vw *VoiceWake) LastWakeMatch() (string, float64) {
	vw.mu.Lock()
	defer vw.mu.Unlock()
	return vw.lastWakeMatch, vw.lastConfidence
}

func (vw *VoiceWake) VAD() *VAD {
	return vw.vad
}

func (vw *VoiceWake) WakeDetector() *WakeWordDetector {
	return vw.wakeDetector
}

func (vw *VoiceWake) UpdateConfig(cfg VoiceWakeConfig) {
	vw.mu.Lock()
	defer vw.mu.Unlock()

	if cfg.SampleRate > 0 {
		vw.cfg.SampleRate = cfg.SampleRate
	}
	if cfg.Channels > 0 {
		vw.cfg.Channels = cfg.Channels
	}
	if cfg.FrameSize > 0 {
		vw.cfg.FrameSize = cfg.FrameSize
	}
	if cfg.MaxRecordingTime > 0 {
		vw.cfg.MaxRecordingTime = cfg.MaxRecordingTime
	}
	if cfg.CooldownTime > 0 {
		vw.cfg.CooldownTime = cfg.CooldownTime
	}

	vw.cfg.AutoTranscribe = cfg.AutoTranscribe
}

func (vw *VoiceWake) Config() VoiceWakeConfig {
	vw.mu.Lock()
	defer vw.mu.Unlock()
	return vw.cfg
}

func (vw *VoiceWake) SetTranscriber(pipeline *STTPipeline) {
	vw.mu.Lock()
	defer vw.mu.Unlock()
	vw.transcriber = pipeline
}

func (vw *VoiceWake) RegisterEngine(engineType WakeWordEngineType, cfg WakeWordEngineConfig) error {
	vw.mu.Lock()
	router := vw.engineRouter
	vw.mu.Unlock()

	if router == nil {
		return fmt.Errorf("voicewake: no engine router available")
	}

	if err := router.CreateEngine(engineType, cfg); err != nil {
		return err
	}

	vw.engineAdapter.SetUseEngine(true)
	return nil
}

func (vw *VoiceWake) SetActiveEngine(name string) error {
	vw.mu.Lock()
	router := vw.engineRouter
	vw.mu.Unlock()

	if router == nil {
		return fmt.Errorf("voicewake: no engine router available")
	}

	return router.SetActive(name)
}

func (vw *VoiceWake) UseWakeWordEngine(use bool) {
	vw.mu.Lock()
	defer vw.mu.Unlock()
	if vw.engineAdapter != nil {
		vw.engineAdapter.SetUseEngine(use)
	}
}

func (vw *VoiceWake) IsUsingWakeWordEngine() bool {
	vw.mu.Lock()
	defer vw.mu.Unlock()
	if vw.engineAdapter == nil {
		return false
	}
	return vw.engineAdapter.UseEngine()
}

func (vw *VoiceWake) AvailableEngines() []string {
	vw.mu.Lock()
	defer vw.mu.Unlock()
	if vw.engineRouter == nil {
		return nil
	}
	return vw.engineRouter.Engines()
}

func (vw *VoiceWake) ActiveEngine() string {
	vw.mu.Lock()
	defer vw.mu.Unlock()
	if vw.engineRouter == nil {
		return ""
	}
	return vw.engineRouter.ActiveEngine()
}

func (vw *VoiceWake) EngineRouter() *WakeWordEngineRouter {
	return vw.engineRouter
}

func (vw *VoiceWake) EngineAdapter() *WakeWordEngineAdapter {
	return vw.engineAdapter
}

func (vw *VoiceWake) Close() error {
	if err := vw.Stop(); err != nil {
		return err
	}

	vw.mu.Lock()
	router := vw.engineRouter
	vw.mu.Unlock()

	if router != nil {
		return router.Close()
	}
	return nil
}
