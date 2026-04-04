package speech

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/anyclaw/anyclaw/pkg/reply"
)

type Integration struct {
	mu         sync.RWMutex
	pipeline   *TTSPipeline
	dispatcher *reply.Dispatcher
	channelMgr ChannelManager
	config     IntegrationConfig
	hooks      []IntegrationHook
}

type IntegrationConfig struct {
	Enabled          bool
	AutoTTS          bool
	TTSTriggerPrefix string
	VoiceProvider    string
	Voice            string
	Speed            float64
	Format           AudioFormat
	FallbackToText   bool
	MaxTextLength    int
	Timeout          time.Duration
	Channels         map[string]bool
	ExcludeChannels  map[string]bool
}

func DefaultIntegrationConfig() IntegrationConfig {
	return IntegrationConfig{
		Enabled:          true,
		AutoTTS:          false,
		TTSTriggerPrefix: "/speak",
		VoiceProvider:    "openai",
		Voice:            "",
		Speed:            1.0,
		Format:           FormatMP3,
		FallbackToText:   true,
		MaxTextLength:    5000,
		Timeout:          30 * time.Second,
		Channels:         map[string]bool{},
		ExcludeChannels:  map[string]bool{},
	}
}

type ChannelManager interface {
	SendMessage(ctx context.Context, channelID string, message *ChannelMessage) error
}

type ChannelMessage struct {
	Channel   string
	Recipient string
	Content   string
	Type      string
	Metadata  map[string]any
}

type IntegrationHook interface {
	OnBeforeTTS(ctx context.Context, req *TTSRequest) error
	OnAfterTTS(ctx context.Context, resp *TTSResponse) error
	OnFallbackText(ctx context.Context, channel string, recipient string, text string) error
}

func NewIntegration(pipeline *TTSPipeline, dispatcher *reply.Dispatcher, channelMgr ChannelManager, cfg IntegrationConfig) *Integration {
	return &Integration{
		pipeline:   pipeline,
		dispatcher: dispatcher,
		channelMgr: channelMgr,
		config:     cfg,
	}
}

func (i *Integration) RegisterHook(hook IntegrationHook) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.hooks = append(i.hooks, hook)
}

func (i *Integration) ProcessMessage(ctx context.Context, channel string, recipient string, text string, metadata map[string]any) error {
	i.mu.RLock()
	enabled := i.config.Enabled
	channels := i.config.Channels
	excludeChannels := i.config.ExcludeChannels
	triggerPrefix := i.config.TTSTriggerPrefix
	autoTTS := i.config.AutoTTS
	i.mu.RUnlock()

	if !enabled {
		return nil
	}

	if len(channels) > 0 && !channels[channel] {
		return nil
	}

	if excludeChannels[channel] {
		return nil
	}

	needsTTS := false
	cleanText := text

	if strings.HasPrefix(text, triggerPrefix) {
		needsTTS = true
		cleanText = strings.TrimSpace(text[len(triggerPrefix):])
		if cleanText == "" {
			return nil
		}
	}

	if autoTTS {
		needsTTS = true
	}

	if !needsTTS {
		return i.processTextOnly(ctx, channel, recipient, text, metadata)
	}

	return i.processWithTTS(ctx, channel, recipient, cleanText, metadata)
}

func (i *Integration) processTextOnly(ctx context.Context, channel string, recipient string, text string, metadata map[string]any) error {
	if i.dispatcher != nil {
		msg := &reply.Message{
			ID:        fmt.Sprintf("msg-%d", time.Now().UnixNano()),
			Channel:   channel,
			From:      recipient,
			Text:      text,
			Timestamp: time.Now().Unix(),
			Metadata:  metadata,
		}

		resp, err := i.dispatcher.Dispatch(ctx, msg)
		if err != nil {
			return fmt.Errorf("integration: dispatch failed: %w", err)
		}

		if resp.Text != "" {
			return i.sendTextResponse(ctx, channel, recipient, resp.Text)
		}
	}

	return nil
}

func (i *Integration) processWithTTS(ctx context.Context, channel string, recipient string, text string, metadata map[string]any) error {
	i.mu.RLock()
	provider := i.config.VoiceProvider
	voice := i.config.Voice
	speed := i.config.Speed
	format := i.config.Format
	maxLen := i.config.MaxTextLength
	timeout := i.config.Timeout
	fallback := i.config.FallbackToText
	i.mu.RUnlock()

	if len(text) > maxLen {
		text = text[:maxLen]
	}

	if i.dispatcher != nil {
		msg := &reply.Message{
			ID:        fmt.Sprintf("tts-%d", time.Now().UnixNano()),
			Channel:   channel,
			From:      recipient,
			Text:      text,
			Timestamp: time.Now().Unix(),
			Metadata:  metadata,
		}

		resp, err := i.dispatcher.Dispatch(ctx, msg)
		if err != nil {
			if fallback {
				return i.sendTextResponse(ctx, channel, recipient, text)
			}
			return fmt.Errorf("integration: dispatch failed: %w", err)
		}

		if resp.Text == "" {
			return nil
		}

		text = resp.Text
	}

	for _, hook := range i.hooks {
		req := &TTSRequest{
			Text:      text,
			Channel:   channel,
			Recipient: recipient,
		}
		if err := hook.OnBeforeTTS(ctx, req); err != nil {
			if fallback {
				return i.sendTextResponse(ctx, channel, recipient, text)
			}
			return fmt.Errorf("integration: before hook failed: %w", err)
		}
	}

	ttsCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		ttsCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	ttsReq := &TTSRequest{
		Text:      text,
		Channel:   channel,
		Recipient: recipient,
		Provider:  provider,
		Voice:     voice,
		Speed:     speed,
		Format:    format,
		Metadata:  metadata,
	}

	ttsResp, err := i.pipeline.Process(ttsCtx, ttsReq)
	if err != nil {
		if fallback {
			return i.sendTextResponse(ctx, channel, recipient, text)
		}
		return fmt.Errorf("integration: TTS failed: %w", err)
	}

	for _, hook := range i.hooks {
		_ = hook.OnAfterTTS(ctx, ttsResp)
	}

	return nil
}

func (i *Integration) sendTextResponse(ctx context.Context, channel string, recipient string, text string) error {
	for _, hook := range i.hooks {
		if err := hook.OnFallbackText(ctx, channel, recipient, text); err != nil {
			return err
		}
	}

	if i.channelMgr != nil {
		msg := &ChannelMessage{
			Channel:   channel,
			Recipient: recipient,
			Content:   text,
			Type:      "text",
		}
		return i.channelMgr.SendMessage(ctx, channel, msg)
	}

	return nil
}

func (i *Integration) UpdateConfig(cfg IntegrationConfig) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.config = cfg
}

func (i *Integration) Config() IntegrationConfig {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.config
}

func (i *Integration) EnableAutoTTS() {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.config.AutoTTS = true
}

func (i *Integration) DisableAutoTTS() {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.config.AutoTTS = false
}

func (i *Integration) SetTriggerPrefix(prefix string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.config.TTSTriggerPrefix = prefix
}

func (i *Integration) AllowChannel(channel string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.config.Channels == nil {
		i.config.Channels = make(map[string]bool)
	}
	i.config.Channels[channel] = true
}

func (i *Integration) ExcludeChannel(channel string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.config.ExcludeChannels == nil {
		i.config.ExcludeChannels = make(map[string]bool)
	}
	i.config.ExcludeChannels[channel] = true
}

type ReplyDispatcherAdapter struct {
	integration *Integration
}

func NewReplyDispatcherAdapter(integration *Integration) *ReplyDispatcherAdapter {
	return &ReplyDispatcherAdapter{integration: integration}
}

func (a *ReplyDispatcherAdapter) OnMessage(ctx context.Context, msg *reply.Message) error {
	_ = ctx
	_ = msg
	return nil
}

func (a *ReplyDispatcherAdapter) OnResponse(ctx context.Context, resp *reply.Response) error {
	a.integration.mu.RLock()
	enabled := a.integration.config.Enabled
	autoTTS := a.integration.config.AutoTTS
	channels := a.integration.config.Channels
	excludeChannels := a.integration.config.ExcludeChannels
	a.integration.mu.RUnlock()

	if !enabled {
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

	if channel == "" || recipient == "" {
		return nil
	}

	if !autoTTS {
		return nil
	}

	if len(channels) > 0 && !channels[channel] {
		return nil
	}

	if excludeChannels[channel] {
		return nil
	}

	return a.integration.processWithTTS(ctx, channel, recipient, resp.Text, resp.Metadata)
}

func (i *Integration) WrapInboundHandler(handler InboundHandler) InboundHandler {
	return func(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error) {
		channel := ""
		recipient := ""
		if meta != nil {
			if ch, ok := meta["channel"]; ok {
				channel = ch
			}
			if rc, ok := meta["recipient"]; ok {
				recipient = rc
			}
		}

		metadata := make(map[string]any)
		for k, v := range meta {
			metadata[k] = v
		}

		if err := i.ProcessMessage(ctx, channel, recipient, message, metadata); err != nil {
			return sessionID, "", err
		}

		return handler(ctx, sessionID, message, meta)
	}
}

type InboundHandler func(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error)
