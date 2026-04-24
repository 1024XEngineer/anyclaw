package speech

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/capability/tools"
	transportreply "github.com/1024XEngineer/anyclaw/pkg/gateway/transport/reply"
)

type testAudioSenderImpl struct {
	id         string
	sendErr    error
	sendCalls  int
	captions   []string
	channels   []string
	recipients []string
}

func (s *testAudioSenderImpl) SendAudio(ctx context.Context, channel string, recipient string, audio *AudioResult, caption string) (string, error) {
	_ = ctx
	_ = audio
	s.sendCalls++
	s.channels = append(s.channels, channel)
	s.recipients = append(s.recipients, recipient)
	s.captions = append(s.captions, caption)
	if s.sendErr != nil {
		return "", s.sendErr
	}
	if s.id == "" {
		return "audio-id", nil
	}
	return s.id, nil
}

func (s *testAudioSenderImpl) CanSend(channel string) bool {
	return channel != ""
}

type testPipelineHook struct {
	beforeCalls int
	afterCalls  int
	sendCalls   int
	beforeErr   error
	afterErr    error
}

func (h *testPipelineHook) OnBeforeSynthesize(ctx context.Context, text string, opts *SynthesizeOptions) error {
	_, _, _ = ctx, text, opts
	h.beforeCalls++
	return h.beforeErr
}

func (h *testPipelineHook) OnAfterSynthesize(ctx context.Context, result *AudioResult) error {
	_, _ = ctx, result
	h.afterCalls++
	return h.afterErr
}

func (h *testPipelineHook) OnSendComplete(ctx context.Context, channel string, audioID string) error {
	_, _, _ = ctx, channel, audioID
	h.sendCalls++
	return nil
}

type testChannelManager struct {
	messages []*ChannelMessage
}

func (m *testChannelManager) SendMessage(ctx context.Context, channelID string, message *ChannelMessage) error {
	_ = ctx
	_ = channelID
	copyMsg := *message
	m.messages = append(m.messages, &copyMsg)
	return nil
}

type testIntegrationHook struct {
	beforeCalls   int
	afterCalls    int
	fallbackCalls int
	beforeErr     error
}

func (h *testIntegrationHook) OnBeforeTTS(ctx context.Context, req *TTSRequest) error {
	_, _ = ctx, req
	h.beforeCalls++
	return h.beforeErr
}

func (h *testIntegrationHook) OnAfterTTS(ctx context.Context, resp *TTSResponse) error {
	_, _ = ctx, resp
	h.afterCalls++
	return nil
}

func (h *testIntegrationHook) OnFallbackText(ctx context.Context, channel string, recipient string, text string) error {
	_, _, _, _ = ctx, channel, recipient, text
	h.fallbackCalls++
	return nil
}

func TestTTSPipelineProcessAndReplyHook(t *testing.T) {
	provider := &testSpeechProvider{name: "openai"}
	manager := NewManager(WithManagerProvider("openai", provider))

	cfg := DefaultPipelineConfig()
	cfg.DefaultProvider = "openai"
	cfg.Timeout = time.Second

	pipeline := NewTTSPipeline(manager, cfg)
	hook := &testPipelineHook{}
	sender := &testAudioSenderImpl{id: "audio-1"}
	pipeline.RegisterHook(hook)
	pipeline.RegisterAudioSender("telegram", sender)
	pipeline.SetDefaultSender(sender)

	resp, err := pipeline.Process(context.Background(), &TTSRequest{
		Text:      "hello there",
		Channel:   "telegram",
		Recipient: "user-1",
		Provider:  "openai",
		Voice:     "nova",
		Speed:     1.2,
		Format:    FormatWAV,
	})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if resp.AudioID != "audio-1" || resp.Audio == nil {
		t.Fatalf("unexpected pipeline response: %+v", resp)
	}
	if hook.beforeCalls != 1 || hook.afterCalls != 1 || hook.sendCalls != 1 {
		t.Fatalf("unexpected hook counts: before=%d after=%d send=%d", hook.beforeCalls, hook.afterCalls, hook.sendCalls)
	}
	if provider.lastOpts.Voice != "nova" || provider.lastOpts.Format != FormatWAV {
		t.Fatalf("unexpected synth options: %+v", provider.lastOpts)
	}
	if sender.sendCalls != 1 || sender.captions[0] != "hello there" {
		t.Fatalf("unexpected sender state: %+v", sender)
	}

	if !pipeline.ShouldAutoTrigger("/speak hello") {
		t.Fatal("expected trigger keyword to auto trigger")
	}
	if pipeline.ShouldAutoTrigger("plain text") {
		t.Fatal("expected plain text to not auto trigger")
	}

	replyHook := NewReplyHook(pipeline)
	if err := replyHook.OnMessage(context.Background(), &Message{ID: "msg-1"}); err != nil {
		t.Fatalf("ReplyHook OnMessage: %v", err)
	}
	if err := replyHook.OnResponse(context.Background(), &Response{
		Text: "/speak follow up",
		Metadata: map[string]any{
			"channel":   "telegram",
			"recipient": "user-2",
		},
	}); err != nil {
		t.Fatalf("ReplyHook OnResponse: %v", err)
	}
	if sender.sendCalls != 2 {
		t.Fatalf("expected reply hook to send audio, got %d sends", sender.sendCalls)
	}
}

func TestTTSPipelineFallbackAndConfigHelpers(t *testing.T) {
	brokenProvider := &testSpeechProvider{name: "broken", synthErr: errors.New("boom")}
	manager := NewManager(WithManagerProvider("broken", brokenProvider))

	cfg := DefaultPipelineConfig()
	cfg.DefaultProvider = "broken"
	cfg.FallbackToText = true
	cfg.Enabled = false

	pipeline := NewTTSPipeline(manager, cfg)
	if _, err := pipeline.Process(context.Background(), &TTSRequest{Text: "hello"}); err == nil {
		t.Fatal("expected disabled pipeline to fail")
	}

	pipeline.SetEnabled(true)
	if !pipeline.Enabled() {
		t.Fatal("expected pipeline to be enabled")
	}

	pipeline.UpdateConfig(DefaultPipelineConfig())
	updated := pipeline.Config()
	updated.DefaultProvider = "broken"
	updated.FallbackToText = true
	pipeline.UpdateConfig(updated)

	resp, err := pipeline.Process(context.Background(), &TTSRequest{Text: "hello"})
	if err != nil {
		t.Fatalf("expected fallback response, got error: %v", err)
	}
	if resp.Text != "hello" || resp.Audio != nil {
		t.Fatalf("unexpected fallback response: %+v", resp)
	}
}

func TestIntegrationFlows(t *testing.T) {
	provider := &testSpeechProvider{name: "openai"}
	manager := NewManager(WithManagerProvider("openai", provider))

	pipelineCfg := DefaultPipelineConfig()
	pipelineCfg.DefaultProvider = "openai"
	pipeline := NewTTSPipeline(manager, pipelineCfg)
	sender := &testAudioSenderImpl{id: "tts-id"}
	pipeline.RegisterAudioSender("telegram", sender)

	dispatcher := transportreply.NewDispatcher(tools.NewRegistry())
	dispatcher.RegisterCommand(transportreply.CommandHandler{
		Name: "echo",
		Handler: func(ctx context.Context, args map[string]string) (string, error) {
			_ = ctx
			if _, ok := args["hello"]; ok {
				return "echo:hello", nil
			}
			return "echo", nil
		},
	})

	channelMgr := &testChannelManager{}
	integration := NewIntegration(pipeline, dispatcher, channelMgr, DefaultIntegrationConfig())
	hook := &testIntegrationHook{}
	integration.RegisterHook(hook)

	if err := integration.ProcessMessage(context.Background(), "telegram", "user-1", "/echo hello", map[string]any{"k": "v"}); err != nil {
		t.Fatalf("ProcessMessage text-only: %v", err)
	}
	if len(channelMgr.messages) != 1 || channelMgr.messages[0].Content != "echo:hello" {
		t.Fatalf("unexpected channel messages: %+v", channelMgr.messages)
	}

	integration.EnableAutoTTS()
	if err := integration.ProcessMessage(context.Background(), "telegram", "user-2", "speak now", nil); err != nil {
		t.Fatalf("ProcessMessage auto TTS: %v", err)
	}
	if sender.sendCalls == 0 {
		t.Fatal("expected TTS sender to be used")
	}
	if hook.beforeCalls == 0 || hook.afterCalls == 0 {
		t.Fatalf("expected integration hooks to run, got before=%d after=%d", hook.beforeCalls, hook.afterCalls)
	}

	integration.DisableAutoTTS()
	integration.SetTriggerPrefix("/say")
	integration.AllowChannel("telegram")
	integration.ExcludeChannel("ignore-me")

	cfg := integration.Config()
	if cfg.TTSTriggerPrefix != "/say" || !cfg.Channels["telegram"] || !cfg.ExcludeChannels["ignore-me"] {
		t.Fatalf("unexpected integration config: %+v", cfg)
	}
	cfg.Enabled = true
	integration.UpdateConfig(cfg)

	adapter := NewReplyDispatcherAdapter(integration)
	if err := adapter.OnMessage(context.Background(), &transportreply.Message{ID: "msg-2"}); err != nil {
		t.Fatalf("ReplyDispatcherAdapter OnMessage: %v", err)
	}
	integration.EnableAutoTTS()
	if err := adapter.OnResponse(context.Background(), &transportreply.Response{
		Text: "adapter text",
		Metadata: map[string]any{
			"channel":   "telegram",
			"recipient": "user-3",
		},
	}); err != nil {
		t.Fatalf("ReplyDispatcherAdapter OnResponse: %v", err)
	}
	if sender.sendCalls < 2 {
		t.Fatalf("expected adapter to trigger TTS, got %d sends", sender.sendCalls)
	}
}

func TestIntegrationFallbackAndWrapInboundHandler(t *testing.T) {
	brokenProvider := &testSpeechProvider{name: "broken", synthErr: errors.New("boom")}
	manager := NewManager(WithManagerProvider("broken", brokenProvider))

	pipelineCfg := DefaultPipelineConfig()
	pipelineCfg.DefaultProvider = "broken"
	pipelineCfg.FallbackToText = false
	pipeline := NewTTSPipeline(manager, pipelineCfg)

	channelMgr := &testChannelManager{}
	cfg := DefaultIntegrationConfig()
	cfg.AutoTTS = true
	cfg.FallbackToText = true
	cfg.VoiceProvider = "broken"
	integration := NewIntegration(pipeline, nil, channelMgr, cfg)

	hook := &testIntegrationHook{beforeErr: errors.New("hook-failed")}
	integration.RegisterHook(hook)

	if err := integration.ProcessMessage(context.Background(), "telegram", "user-1", "hello", nil); err != nil {
		t.Fatalf("expected fallback path to succeed, got %v", err)
	}
	if len(channelMgr.messages) != 1 || channelMgr.messages[0].Content != "hello" {
		t.Fatalf("unexpected fallback messages: %+v", channelMgr.messages)
	}
	if hook.fallbackCalls != 1 {
		t.Fatalf("expected fallback hook to run once, got %d", hook.fallbackCalls)
	}

	wrapped := integration.WrapInboundHandler(func(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error) {
		_, _, _ = ctx, message, meta
		return sessionID + "-next", "ok", nil
	})

	nextSession, replyText, err := wrapped(context.Background(), "sess-1", "hi", map[string]string{
		"channel":   "telegram",
		"recipient": "user-2",
	})
	if err != nil {
		t.Fatalf("WrapInboundHandler: %v", err)
	}
	if nextSession != "sess-1-next" || replyText != "ok" {
		t.Fatalf("unexpected wrapped handler result: %q / %q", nextSession, replyText)
	}
}