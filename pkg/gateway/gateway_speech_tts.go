package gateway

import (
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/speech"
)

func (s *Server) initTTS() {
	ttsCfg := s.mainRuntime.Config.Speech.TTS
	if !ttsCfg.Enabled {
		return
	}

	s.ttsManager = speech.NewManager()

	if ttsCfg.Provider != "" {
		providerType := speech.ProviderType(ttsCfg.Provider)
		ttsProviderCfg := speech.Config{
			Type:        providerType,
			APIKey:      ttsCfg.APIKey,
			BaseURL:     ttsCfg.BaseURL,
			Voice:       ttsCfg.Voice,
			AudioFormat: speech.AudioFormat(ttsCfg.Format),
			Timeout:     time.Duration(ttsCfg.TimeoutSec) * time.Second,
		}
		if ttsProviderCfg.AudioFormat == "" {
			ttsProviderCfg.AudioFormat = speech.FormatMP3
		}
		if ttsCfg.TimeoutSec <= 0 {
			ttsProviderCfg.Timeout = 30 * time.Second
		}

		provider, err := speech.NewProvider(ttsProviderCfg)
		if err != nil {
			s.appendEvent("tts.init.error", "", map[string]any{"error": err.Error(), "provider": ttsCfg.Provider})
			return
		}

		if err := s.ttsManager.Register(ttsCfg.Provider, provider); err != nil {
			s.appendEvent("tts.init.error", "", map[string]any{"error": err.Error(), "provider": ttsCfg.Provider})
			return
		}
	}

	pipelineCfg := speech.PipelineConfig{
		Enabled:         ttsCfg.Enabled,
		AutoTrigger:     ttsCfg.AutoTTS,
		TriggerKeywords: []string{ttsCfg.TriggerPrefix},
		DefaultProvider: ttsCfg.Provider,
		DefaultVoice:    ttsCfg.Voice,
		DefaultSpeed:    ttsCfg.Speed,
		DefaultFormat:   speech.AudioFormat(ttsCfg.Format),
		FallbackToText:  ttsCfg.FallbackToText,
		Timeout:         time.Duration(ttsCfg.TimeoutSec) * time.Second,
	}
	if pipelineCfg.TriggerKeywords[0] == "" {
		pipelineCfg.TriggerKeywords = []string{"/speak", "/voice", "/tts"}
	}
	if pipelineCfg.DefaultFormat == "" {
		pipelineCfg.DefaultFormat = speech.FormatMP3
	}
	if pipelineCfg.DefaultSpeed <= 0 {
		pipelineCfg.DefaultSpeed = 1.0
	}
	if pipelineCfg.Timeout <= 0 {
		pipelineCfg.Timeout = 30 * time.Second
	}

	s.ttsPipeline = speech.NewTTSPipeline(s.ttsManager, pipelineCfg)

	s.registerAudioSenders()

	integrationCfg := speech.IntegrationConfig{
		Enabled:          ttsCfg.Enabled,
		AutoTTS:          ttsCfg.AutoTTS,
		TTSTriggerPrefix: ttsCfg.TriggerPrefix,
		VoiceProvider:    ttsCfg.Provider,
		Voice:            ttsCfg.Voice,
		Speed:            ttsCfg.Speed,
		Format:           speech.AudioFormat(ttsCfg.Format),
		FallbackToText:   ttsCfg.FallbackToText,
		Timeout:          pipelineCfg.Timeout,
		Channels:         ttsCfg.Channels,
		ExcludeChannels:  ttsCfg.ExcludeChannels,
	}
	if integrationCfg.TTSTriggerPrefix == "" {
		integrationCfg.TTSTriggerPrefix = "/speak"
	}
	if integrationCfg.Format == "" {
		integrationCfg.Format = speech.FormatMP3
	}
	if integrationCfg.Speed <= 0 {
		integrationCfg.Speed = 1.0
	}

	s.ttsIntegration = speech.NewIntegration(s.ttsPipeline, nil, nil, integrationCfg)

	s.appendEvent("tts.init.ok", "", map[string]any{
		"provider": ttsCfg.Provider,
		"auto_tts": ttsCfg.AutoTTS,
		"voice":    ttsCfg.Voice,
		"channels": len(ttsCfg.Channels),
		"excluded": len(ttsCfg.ExcludeChannels),
	})
}

func (s *Server) registerAudioSenders() {
	if s.ttsPipeline == nil {
		return
	}

	chCfg := s.mainRuntime.Config.Channels

	if chCfg.Telegram.Enabled && chCfg.Telegram.BotToken != "" {
		s.ttsPipeline.RegisterAudioSender("telegram", speech.NewTelegramAudioSender(chCfg.Telegram.BotToken))
	}
	if chCfg.Discord.Enabled && chCfg.Discord.BotToken != "" {
		s.ttsPipeline.RegisterAudioSender("discord", speech.NewDiscordAudioSender(chCfg.Discord.BotToken))
	}
	if chCfg.Slack.Enabled && chCfg.Slack.BotToken != "" {
		s.ttsPipeline.RegisterAudioSender("slack", speech.NewSlackAudioSender(chCfg.Slack.BotToken))
	}
	if chCfg.WhatsApp.Enabled && chCfg.WhatsApp.PhoneNumberID != "" && chCfg.WhatsApp.AccessToken != "" {
		s.ttsPipeline.RegisterAudioSender("whatsapp", speech.NewWhatsAppAudioSender(chCfg.WhatsApp.PhoneNumberID, chCfg.WhatsApp.AccessToken))
	}
	if chCfg.Signal.Enabled && chCfg.Signal.Number != "" {
		s.ttsPipeline.RegisterAudioSender("signal", speech.NewSignalAudioSender(chCfg.Signal.Number, ""))
	}
}
