package speech

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

type AgentRunner interface {
	Run(ctx context.Context, userInput string) (string, error)
}

type TTSProcessor interface {
	Synthesize(ctx context.Context, text string, opts ...SynthesizeOption) (*AudioResult, error)
}

type AudioPlayer interface {
	Play(ctx context.Context, audio []byte, format AudioFormat) error
	Stop() error
	IsPlaying() bool
}

type VoiceWakeProcessorConfig struct {
	Agent           AgentRunner
	TTSProcessor    TTSProcessor
	AudioPlayer     AudioPlayer
	VoiceWake       *VoiceWake
	SessionID       string
	Channel         string
	Sender          string
	AutoSpeak       bool
	AutoTranscribe  bool
	MaxResponseTime time.Duration
	MaxAudioTime    time.Duration
	OnResponse      func(text string)
	OnAudioComplete func(duration time.Duration)
	OnError         func(err error)
}

func DefaultVoiceWakeProcessorConfig() VoiceWakeProcessorConfig {
	return VoiceWakeProcessorConfig{
		SessionID:       "voicewake-local",
		Channel:         "voicewake",
		Sender:          "local-mic",
		AutoSpeak:       true,
		AutoTranscribe:  true,
		MaxResponseTime: 60 * time.Second,
		MaxAudioTime:    5 * time.Minute,
	}
}

type VoiceWakeProcessor struct {
	mu                 sync.Mutex
	cfg                VoiceWakeProcessorConfig
	isProcessing       bool
	lastResponse       string
	lastAudioDur       time.Duration
	conversationCtx    context.Context
	conversationCancel context.CancelFunc
}

func NewVoiceWakeProcessor(cfg VoiceWakeProcessorConfig) *VoiceWakeProcessor {
	if cfg.SessionID == "" {
		cfg.SessionID = "voicewake-local"
	}
	if cfg.Channel == "" {
		cfg.Channel = "voicewake"
	}
	if cfg.Sender == "" {
		cfg.Sender = "local-mic"
	}
	if cfg.MaxResponseTime == 0 {
		cfg.MaxResponseTime = 60 * time.Second
	}
	if cfg.MaxAudioTime == 0 {
		cfg.MaxAudioTime = 5 * time.Minute
	}

	return &VoiceWakeProcessor{
		cfg: cfg,
	}
}

func (p *VoiceWakeProcessor) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cfg.VoiceWake == nil {
		return fmt.Errorf("voicewake-processor: VoiceWake is required")
	}
	if p.cfg.Agent == nil {
		return fmt.Errorf("voicewake-processor: Agent is required")
	}

	p.cfg.VoiceWake.RegisterListener(p.onVoiceWakeEvent)

	return nil
}

func (p *VoiceWakeProcessor) onVoiceWakeEvent(event VoiceWakeEvent) {
	switch event.Type {
	case VoiceWakeEventWakeDetected:
		p.mu.Lock()
		if p.isProcessing {
			p.mu.Unlock()
			log.Printf("voicewake-processor: already processing, skipping wake event")
			return
		}
		p.isProcessing = true
		p.mu.Unlock()
		go p.handleWakeDetected(event)

	case VoiceWakeEventError:
		if p.cfg.OnError != nil {
			if errStr, ok := event.Data["error"].(string); ok {
				p.cfg.OnError(fmt.Errorf("voicewake: %s", errStr))
			}
		}
	}
}

func (p *VoiceWakeProcessor) handleWakeDetected(event VoiceWakeEvent) {
	defer func() {
		p.mu.Lock()
		p.isProcessing = false
		p.conversationCtx = nil
		p.conversationCancel = nil
		p.mu.Unlock()
	}()

	transcript, _ := event.Data["transcript"].(string)
	if transcript == "" {
		transcript = p.cfg.VoiceWake.LastTranscript()
	}

	if transcript == "" {
		log.Printf("voicewake-processor: no transcript available")
		return
	}

	log.Printf("voicewake-processor: wake detected, transcript: %q", transcript)

	ctx, cancel := context.WithTimeout(context.Background(), p.cfg.MaxResponseTime)
	defer cancel()

	p.mu.Lock()
	p.conversationCtx = ctx
	p.conversationCancel = cancel
	p.mu.Unlock()

	response, err := p.runAgent(ctx, transcript)
	if err != nil {
		log.Printf("voicewake-processor: agent error: %v", err)
		if p.cfg.OnError != nil {
			p.cfg.OnError(err)
		}
		return
	}

	p.mu.Lock()
	p.lastResponse = response
	p.mu.Unlock()

	if p.cfg.OnResponse != nil {
		p.cfg.OnResponse(response)
	}

	if p.cfg.AutoSpeak && p.cfg.TTSProcessor != nil && p.cfg.AudioPlayer != nil {
		if err := p.speakResponse(ctx, response); err != nil {
			log.Printf("voicewake-processor: TTS error: %v", err)
			if p.cfg.OnError != nil {
				p.cfg.OnError(err)
			}
		}
	}
}

func (p *VoiceWakeProcessor) runAgent(ctx context.Context, transcript string) (string, error) {
	if p.cfg.Agent == nil {
		return "", fmt.Errorf("voicewake-processor: no agent configured")
	}

	return p.cfg.Agent.Run(ctx, transcript)
}

func (p *VoiceWakeProcessor) speakResponse(ctx context.Context, text string) error {
	if p.cfg.TTSProcessor == nil {
		return fmt.Errorf("voicewake-processor: no TTS processor configured")
	}
	if p.cfg.AudioPlayer == nil {
		return fmt.Errorf("voicewake-processor: no audio player configured")
	}

	if p.cfg.AudioPlayer.IsPlaying() {
		p.cfg.AudioPlayer.Stop()
	}

	ttsCtx, cancel := context.WithTimeout(ctx, p.cfg.MaxAudioTime)
	defer cancel()

	var opts []SynthesizeOption
	opts = append(opts, WithFormat(FormatWAV))

	result, err := p.cfg.TTSProcessor.Synthesize(ttsCtx, text, opts...)
	if err != nil {
		return fmt.Errorf("voicewake-processor: TTS failed: %w", err)
	}

	if len(result.Data) == 0 {
		return fmt.Errorf("voicewake-processor: TTS returned empty audio")
	}

	if err := p.cfg.AudioPlayer.Play(ttsCtx, result.Data, result.Format); err != nil {
		return fmt.Errorf("voicewake-processor: playback failed: %w", err)
	}

	p.mu.Lock()
	p.lastAudioDur = result.Duration
	p.mu.Unlock()

	if p.cfg.OnAudioComplete != nil {
		p.cfg.OnAudioComplete(result.Duration)
	}

	return nil
}

func (p *VoiceWakeProcessor) IsProcessing() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.isProcessing
}

func (p *VoiceWakeProcessor) LastResponse() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastResponse
}

func (p *VoiceWakeProcessor) LastAudioDuration() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastAudioDur
}

func (p *VoiceWakeProcessor) CancelConversation() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.conversationCancel != nil {
		p.conversationCancel()
		p.conversationCancel = nil
		p.conversationCtx = nil
	}

	if p.cfg.AudioPlayer != nil && p.cfg.AudioPlayer.IsPlaying() {
		p.cfg.AudioPlayer.Stop()
	}

	p.isProcessing = false
}

func (p *VoiceWakeProcessor) UpdateConfig(cfg VoiceWakeProcessorConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if cfg.Agent != nil {
		p.cfg.Agent = cfg.Agent
	}
	if cfg.TTSProcessor != nil {
		p.cfg.TTSProcessor = cfg.TTSProcessor
	}
	if cfg.AudioPlayer != nil {
		p.cfg.AudioPlayer = cfg.AudioPlayer
	}
	if cfg.SessionID != "" {
		p.cfg.SessionID = cfg.SessionID
	}
	if cfg.MaxResponseTime > 0 {
		p.cfg.MaxResponseTime = cfg.MaxResponseTime
	}
	if cfg.MaxAudioTime > 0 {
		p.cfg.MaxAudioTime = cfg.MaxAudioTime
	}

	p.cfg.AutoSpeak = cfg.AutoSpeak
	p.cfg.AutoTranscribe = cfg.AutoTranscribe
	p.cfg.OnResponse = cfg.OnResponse
	p.cfg.OnAudioComplete = cfg.OnAudioComplete
	p.cfg.OnError = cfg.OnError
}

func (p *VoiceWakeProcessor) Config() VoiceWakeProcessorConfig {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cfg
}

func (p *VoiceWakeProcessor) SetAgent(agent AgentRunner) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cfg.Agent = agent
}

func (p *VoiceWakeProcessor) SetTTSProcessor(processor TTSProcessor) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cfg.TTSProcessor = processor
}

func (p *VoiceWakeProcessor) SetAudioPlayer(player AudioPlayer) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cfg.AudioPlayer = player
}
