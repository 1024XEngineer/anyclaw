package gateway

import (
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/speech"
)

func (s *Server) initSTT() {
	sttCfg := s.mainRuntime.Config.Speech.STT
	if !sttCfg.Enabled {
		return
	}

	s.sttManager = speech.NewSTTManager()

	if sttCfg.Provider != "" && sttCfg.APIKey != "" {
		providerType := speech.STTProviderType(sttCfg.Provider)
		sttProviderCfg := speech.STTConfig{
			Type:     providerType,
			APIKey:   sttCfg.APIKey,
			BaseURL:  sttCfg.BaseURL,
			Model:    sttCfg.Model,
			Language: sttCfg.DefaultLang,
			Timeout:  time.Duration(sttCfg.TimeoutSec) * time.Second,
		}
		if sttCfg.TimeoutSec <= 0 {
			sttProviderCfg.Timeout = 120 * time.Second
		}

		provider, err := speech.NewSTTProvider(sttProviderCfg)
		if err != nil {
			s.appendEvent("stt.init.error", "", map[string]any{"error": err.Error(), "provider": sttCfg.Provider})
			return
		}

		if err := s.sttManager.Register(sttCfg.Provider, provider); err != nil {
			s.appendEvent("stt.init.error", "", map[string]any{"error": err.Error(), "provider": sttCfg.Provider})
			return
		}
	}

	pipelineCfg := speech.STTPipelineConfig{
		Provider:      sttCfg.Provider,
		DefaultLang:   sttCfg.DefaultLang,
		AutoDetect:    sttCfg.DefaultLang == "auto",
		MaxDuration:   time.Duration(sttCfg.MaxDurationSec) * time.Second,
		MinConfidence: sttCfg.MinConfidence,
		Timeout:       time.Duration(sttCfg.TimeoutSec) * time.Second,
	}
	if sttCfg.MaxDurationSec <= 0 {
		pipelineCfg.MaxDuration = 10 * time.Minute
	}
	if sttCfg.TimeoutSec <= 0 {
		pipelineCfg.Timeout = 120 * time.Second
	}

	s.sttPipeline = speech.NewSTTPipeline(s.sttManager, pipelineCfg)

	integrationCfg := speech.STTIntegrationConfig{
		Enabled:          sttCfg.Enabled,
		AutoSTT:          sttCfg.AutoSTT,
		TriggerPrefix:    sttCfg.TriggerPrefix,
		Provider:         sttCfg.Provider,
		DefaultLang:      sttCfg.DefaultLang,
		MaxDuration:      pipelineCfg.MaxDuration,
		MinConfidence:    sttCfg.MinConfidence,
		Timeout:          pipelineCfg.Timeout,
		Channels:         sttCfg.Channels,
		ExcludeChannels:  sttCfg.ExcludeChannels,
		FallbackToVoice:  sttCfg.FallbackToVoice,
		AppendTranscript: sttCfg.AppendTranscript,
	}
	if integrationCfg.TriggerPrefix == "" {
		integrationCfg.TriggerPrefix = "/transcribe"
	}

	s.sttIntegration = speech.NewSTTIntegration(s.sttPipeline, integrationCfg)

	s.appendEvent("stt.init.ok", "", map[string]any{
		"provider": sttCfg.Provider,
		"auto_stt": sttCfg.AutoSTT,
		"language": sttCfg.DefaultLang,
		"channels": len(sttCfg.Channels),
		"excluded": len(sttCfg.ExcludeChannels),
	})
}
