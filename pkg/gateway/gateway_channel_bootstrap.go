package gateway

import (
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/config"
	inputlayer "github.com/1024XEngineer/anyclaw/pkg/input"
	inputchannels "github.com/1024XEngineer/anyclaw/pkg/input/channels"
	routeingress "github.com/1024XEngineer/anyclaw/pkg/route/ingress"
)

func (s *Server) initChannels() {
	s.initSTT()
	s.initTTS()

	s.ingress = routeingress.NewService(
		routeingress.NewRouter(s.mainRuntime.Config.Channels.Routing),
		routeingress.WithMainAgentNameResolver(s.mainRuntime.Config.ResolveMainAgentName),
		routeingress.WithSessionStore(ingressSessionStore{server: s, manager: s.sessions}),
	)
	if s.plugins != nil {
		s.ingressPlugins = s.plugins.IngressRunners(s.mainRuntime.Config.Plugins.Dir)
	}
	builders := map[string]func() inputlayer.Adapter{
		"telegram-channel": func() inputlayer.Adapter {
			s.telegram = inputchannels.NewTelegramAdapter(s.mainRuntime.Config.Channels.Telegram, s.appendEvent)
			return s.telegram
		},
		"slack-channel": func() inputlayer.Adapter {
			s.slack = inputchannels.NewSlackAdapter(s.mainRuntime.Config.Channels.Slack, s.appendEvent)
			return s.slack
		},
		"discord-channel": func() inputlayer.Adapter {
			s.discord = inputchannels.NewDiscordAdapter(s.mainRuntime.Config.Channels.Discord, s.appendEvent)
			return s.discord
		},
		"whatsapp-channel": func() inputlayer.Adapter {
			s.whatsapp = inputchannels.NewWhatsAppAdapter(s.mainRuntime.Config.Channels.WhatsApp, s.appendEvent)
			return s.whatsapp
		},
		"signal-channel": func() inputlayer.Adapter {
			s.signal = inputchannels.NewSignalAdapter(s.mainRuntime.Config.Channels.Signal, s.appendEvent)
			return s.signal
		},
	}
	var adapters []inputlayer.Adapter
	if s.plugins != nil {
		for _, name := range s.plugins.EnabledPluginNames() {
			if builder, ok := builders[name]; ok {
				adapters = append(adapters, builder())
			}
		}
		for _, runner := range s.plugins.ChannelRunners(s.mainRuntime.Config.Plugins.Dir) {
			adapters = append(adapters, inputlayer.NewPluginChannelAdapter(runner, s.appendEvent))
		}
	}
	if len(adapters) == 0 {
		adapters = []inputlayer.Adapter{
			builders["telegram-channel"](),
			builders["slack-channel"](),
			builders["discord-channel"](),
			builders["whatsapp-channel"](),
			builders["signal-channel"](),
		}
	}
	s.channels = inputlayer.NewManager(adapters...)
	s.initChannelAdvanced()
}

func (s *Server) initChannelAdvanced() {
	botName := s.mainRuntime.Config.Agent.Name
	if botName == "" {
		botName = "AnyClaw"
	}

	botUserID := ""
	if s.telegram != nil {
		botUserID = s.mainRuntime.Config.Channels.Telegram.BotToken
	}

	secCfg := s.mainRuntime.Config.Channels.Security
	channelPolicy := inputlayer.ChannelPolicyFromConfig(config.ChannelSecurityConfig{
		DMPolicy:         secCfg.DMPolicy,
		GroupPolicy:      secCfg.GroupPolicy,
		AllowFrom:        secCfg.AllowFrom,
		PairingEnabled:   secCfg.PairingEnabled,
		PairingTTLHours:  secCfg.PairingTTLHours,
		MentionGate:      secCfg.MentionGate,
		RiskAcknowledged: s.mainRuntime.Config.Security.RiskAcknowledged,
		DefaultDenyDM:    secCfg.DefaultDenyDM,
	})

	s.mentionGate = inputlayer.NewMentionGate(secCfg.MentionGate, botUserID, nil)
	s.groupSecurity = inputlayer.NewGroupSecurity()
	s.channelCmds = inputlayer.NewChannelCommands(botName)
	s.channelPairing = inputlayer.NewChannelPairing()
	if secCfg.PairingEnabled {
		s.channelPairing.SetEnabled(true)
	}
	for _, userID := range secCfg.AllowFrom {
		if userID = strings.TrimSpace(userID); userID != "" {
			channelPolicy.AddAllowedUser(userID)
		}
	}

	s.presenceMgr = inputlayer.NewPresenceManager(func(ch, userID string, presence inputlayer.PresenceInfo) {
		s.appendEvent("channel.presence", "", map[string]any{
			"channel": ch,
			"user_id": userID,
			"status":  presence.Status,
		})
	})
	s.contactDir = inputlayer.NewContactDirectory()

	s.channelPolicy = channelPolicy
	s.appendEvent("security.init", "", map[string]any{
		"dm_policy":         secCfg.DMPolicy,
		"group_policy":      secCfg.GroupPolicy,
		"mention_gate":      secCfg.MentionGate,
		"pairing_enabled":   secCfg.PairingEnabled,
		"allow_from":        len(secCfg.AllowFrom),
		"risk_acknowledged": s.mainRuntime.Config.Security.RiskAcknowledged,
	})
}
