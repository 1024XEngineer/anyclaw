package gateway

import (
	"context"
	"strings"
)

func (s *Server) processChannelMessage(ctx context.Context, sessionID string, message string, meta map[string]string) (string, string, error) {
	source := strings.TrimSpace(meta["channel"])
	if source == "" {
		source = "telegram"
	}
	response, session, err := s.runOrCreateChannelSession(ctx, source, sessionID, message, meta)
	if err != nil {
		return "", "", err
	}

	if s.ttsIntegration != nil && response != "" {
		recipient := firstNonEmpty(strings.TrimSpace(meta["chat_id"]), strings.TrimSpace(session.ReplyTarget), strings.TrimSpace(meta["reply_target"]), strings.TrimSpace(meta["user_id"]), sessionID)
		metadata := make(map[string]any, len(meta))
		for k, v := range meta {
			metadata[k] = v
		}
		if err := s.ttsIntegration.ProcessMessage(ctx, source, recipient, response, metadata); err != nil {
			s.appendEvent("tts.process.error", sessionID, map[string]any{"error": err.Error(), "channel": source})
		}
	}

	return session.ID, response, nil
}

func (s *Server) processChannelMessageStream(ctx context.Context, sessionID string, message string, meta map[string]string, onChunk func(chunk string) error) (string, error) {
	source := strings.TrimSpace(meta["channel"])
	if source == "" {
		source = "telegram"
	}
	_, session, err := s.runOrCreateChannelSessionStream(ctx, source, sessionID, message, meta, onChunk)
	if err != nil {
		return "", err
	}

	if s.ttsIntegration != nil {
		recipient := firstNonEmpty(strings.TrimSpace(meta["chat_id"]), strings.TrimSpace(session.ReplyTarget), strings.TrimSpace(meta["reply_target"]), strings.TrimSpace(meta["user_id"]), sessionID)
		metadata := make(map[string]any, len(meta))
		for k, v := range meta {
			metadata[k] = v
		}
		if err := s.ttsIntegration.ProcessMessage(ctx, source, recipient, session.ID, metadata); err != nil {
			s.appendEvent("tts.process.error", sessionID, map[string]any{"error": err.Error(), "channel": source})
		}
	}

	return session.ID, nil
}
