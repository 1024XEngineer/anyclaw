package gateway

import (
	"context"

	inputlayer "github.com/1024XEngineer/anyclaw/pkg/input"
)

func (s *Server) runChannels(ctx context.Context) {
	if s.channels == nil {
		return
	}
	handler := s.processChannelMessage
	handler = s.mentionGate.Wrap(handler)
	handler = s.groupSecurity.Wrap(handler)
	handler = s.channelPairing.Wrap(handler)
	handler = s.channelCmds.Wrap(handler)
	handler = s.contactDir.Wrap(handler)
	handler = s.presenceMgr.Wrap(handler)
	if s.sttIntegration != nil {
		handler = s.sttIntegration.WrapInboundHandler(handler)
	}
	s.channels.Run(ctx, handler)
	s.runStreamChannels(ctx)
}

func (s *Server) runStreamChannels(ctx context.Context) {
	for _, adapter := range s.getStreamAdapters() {
		if !adapter.Enabled() {
			continue
		}
		go func(sa inputlayer.StreamAdapter) {
			handler := s.processChannelMessageStream
			handler = s.mentionGate.WrapStream(handler)
			handler = s.groupSecurity.WrapStream(handler)
			handler = s.channelPairing.WrapStream(handler)
			handler = s.channelCmds.WrapStream(handler)
			handler = s.contactDir.WrapStream(handler)
			handler = s.presenceMgr.WrapStream(handler)
			if s.sttIntegration != nil {
				handler = s.sttIntegration.WrapStreamInboundHandler(handler)
			}
			_ = sa.RunStream(ctx, handler)
		}(adapter)
	}
}

func (s *Server) getStreamAdapters() []inputlayer.StreamAdapter {
	var adapters []inputlayer.StreamAdapter
	if s.telegram != nil && s.mainRuntime.Config.Channels.Telegram.StreamReply {
		adapters = append(adapters, s.telegram)
	}
	if s.discord != nil && s.mainRuntime.Config.Channels.Discord.StreamReply {
		adapters = append(adapters, s.discord)
	}
	if s.slack != nil && s.mainRuntime.Config.Channels.Slack.StreamReply {
		adapters = append(adapters, s.slack)
	}
	return adapters
}
