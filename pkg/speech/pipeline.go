package speech

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type TTSPipeline struct {
	mu            sync.RWMutex
	manager       *Manager
	config        PipelineConfig
	hooks         []PipelineHook
	audioSenders  map[string]AudioSender
	defaultSender AudioSender
}

type PipelineConfig struct {
	Enabled         bool
	AutoTrigger     bool
	TriggerKeywords []string
	MaxTextLength   int
	DefaultProvider string
	DefaultVoice    string
	DefaultSpeed    float64
	DefaultFormat   AudioFormat
	Timeout         time.Duration
	RetryOnFail     bool
	FallbackToText  bool
}

func DefaultPipelineConfig() PipelineConfig {
	return PipelineConfig{
		Enabled:         true,
		AutoTrigger:     false,
		TriggerKeywords: []string{"/speak", "/voice", "/tts"},
		MaxTextLength:   5000,
		DefaultProvider: "openai",
		DefaultVoice:    "",
		DefaultSpeed:    1.0,
		DefaultFormat:   FormatMP3,
		Timeout:         30 * time.Second,
		RetryOnFail:     true,
		FallbackToText:  true,
	}
}

type PipelineHook interface {
	OnBeforeSynthesize(ctx context.Context, text string, opts *SynthesizeOptions) error
	OnAfterSynthesize(ctx context.Context, result *AudioResult) error
	OnSendComplete(ctx context.Context, channel string, audioID string) error
}

type AudioSender interface {
	SendAudio(ctx context.Context, channel string, recipient string, audio *AudioResult, caption string) (string, error)
	CanSend(channel string) bool
}

type TTSRequest struct {
	Text      string
	Channel   string
	Recipient string
	Provider  string
	Voice     string
	Speed     float64
	Format    AudioFormat
	Caption   string
	Metadata  map[string]any
}

type TTSResponse struct {
	AudioID   string
	Audio     *AudioResult
	Text      string
	Channel   string
	Recipient string
	Cached    bool
	Duration  time.Duration
}

func NewTTSPipeline(manager *Manager, cfg PipelineConfig) *TTSPipeline {
	if manager == nil {
		manager = NewManager()
	}

	p := &TTSPipeline{
		manager:      manager,
		config:       cfg,
		audioSenders: make(map[string]AudioSender),
	}

	if p.manager.Cache() == nil {
		p.manager.EnableCache(DefaultCacheConfig())
	}

	return p
}

func (p *TTSPipeline) RegisterAudioSender(channel string, sender AudioSender) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.audioSenders[channel] = sender
	if p.defaultSender == nil {
		p.defaultSender = sender
	}
}

func (p *TTSPipeline) RegisterHook(hook PipelineHook) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.hooks = append(p.hooks, hook)
}

func (p *TTSPipeline) SetDefaultSender(sender AudioSender) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.defaultSender = sender
}

func (p *TTSPipeline) Process(ctx context.Context, req *TTSRequest) (*TTSResponse, error) {
	p.mu.RLock()
	enabled := p.config.Enabled
	provider := p.config.DefaultProvider
	voice := p.config.DefaultVoice
	speed := p.config.DefaultSpeed
	format := p.config.DefaultFormat
	maxLen := p.config.MaxTextLength
	timeout := p.config.Timeout
	p.mu.RUnlock()

	if !enabled {
		return nil, fmt.Errorf("tts pipeline is disabled")
	}

	if req.Provider != "" {
		provider = req.Provider
	}
	if req.Voice != "" {
		voice = req.Voice
	}
	if req.Speed > 0 {
		speed = req.Speed
	}
	if req.Format != "" {
		format = req.Format
	}

	if len(req.Text) > maxLen {
		req.Text = req.Text[:maxLen]
	}

	opts := []SynthesizeOption{
		WithVoice(voice),
		WithSpeed(speed),
		WithFormat(format),
	}

	for _, hook := range p.hooks {
		synthOpts := SynthesizeOptions{
			Voice:  voice,
			Speed:  speed,
			Format: format,
		}
		if err := hook.OnBeforeSynthesize(ctx, req.Text, &synthOpts); err != nil {
			return nil, fmt.Errorf("tts pipeline: before hook failed: %w", err)
		}
	}

	synthCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		synthCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	audio, err := p.manager.Synthesize(synthCtx, req.Text, provider, opts...)
	if err != nil {
		if p.config.FallbackToText {
			return &TTSResponse{
				Text:    req.Text,
				Channel: req.Channel,
			}, nil
		}
		return nil, fmt.Errorf("tts pipeline: synthesis failed: %w", err)
	}

	for _, hook := range p.hooks {
		if err := hook.OnAfterSynthesize(ctx, audio); err != nil {
			return nil, fmt.Errorf("tts pipeline: after hook failed: %w", err)
		}
	}

	sender := p.resolveSender(req.Channel)
	if sender == nil {
		sender = p.defaultSender
	}

	var audioID string
	if sender != nil {
		caption := req.Caption
		if caption == "" {
			caption = req.Text
		}

		id, err := sender.SendAudio(ctx, req.Channel, req.Recipient, audio, caption)
		if err != nil {
			if p.config.FallbackToText {
				return &TTSResponse{
					Text:    req.Text,
					Channel: req.Channel,
				}, nil
			}
			return nil, fmt.Errorf("tts pipeline: failed to send audio: %w", err)
		}
		audioID = id

		for _, hook := range p.hooks {
			_ = hook.OnSendComplete(ctx, req.Channel, id)
		}
	}

	return &TTSResponse{
		AudioID:   audioID,
		Audio:     audio,
		Text:      req.Text,
		Channel:   req.Channel,
		Recipient: req.Recipient,
		Cached:    false,
	}, nil
}

func (p *TTSPipeline) ShouldAutoTrigger(text string) bool {
	p.mu.RLock()
	autoTrigger := p.config.AutoTrigger
	keywords := p.config.TriggerKeywords
	p.mu.RUnlock()

	if !autoTrigger {
		for _, kw := range keywords {
			if len(text) >= len(kw) && text[:len(kw)] == kw {
				return true
			}
		}
		return false
	}

	return true
}

func (p *TTSPipeline) resolveSender(channel string) AudioSender {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if sender, ok := p.audioSenders[channel]; ok {
		return sender
	}
	return nil
}

func (p *TTSPipeline) UpdateConfig(cfg PipelineConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.config = cfg
}

func (p *TTSPipeline) Config() PipelineConfig {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.config
}

func (p *TTSPipeline) Enabled() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.config.Enabled
}

func (p *TTSPipeline) SetEnabled(enabled bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.config.Enabled = enabled
}

type ReplyHook struct {
	pipeline *TTSPipeline
}

func NewReplyHook(pipeline *TTSPipeline) *ReplyHook {
	return &ReplyHook{pipeline: pipeline}
}

func (h *ReplyHook) OnMessage(ctx context.Context, msg *Message) error {
	_ = ctx
	_ = msg
	return nil
}

func (h *ReplyHook) OnResponse(ctx context.Context, resp *Response) error {
	if !h.pipeline.Enabled() {
		return nil
	}

	if resp.Text == "" {
		return nil
	}

	channel := ""
	recipient := ""
	if resp.Metadata != nil {
		if ch, ok := resp.Metadata["channel"].(string); ok {
			channel = ch
		}
		if rc, ok := resp.Metadata["recipient"].(string); ok {
			recipient = rc
		}
	}

	if !h.pipeline.ShouldAutoTrigger(resp.Text) {
		return nil
	}

	req := &TTSRequest{
		Text:      resp.Text,
		Channel:   channel,
		Recipient: recipient,
	}

	_, err := h.pipeline.Process(ctx, req)
	return err
}

type Message struct {
	ID        string
	Channel   string
	From      string
	Text      string
	Timestamp int64
	Metadata  map[string]any
}

type Response struct {
	MessageID string
	Text      string
	Tools     []ToolCall
	Metadata  map[string]any
}

type ToolCall struct {
	Name      string
	Arguments map[string]any
	ID        string
}
