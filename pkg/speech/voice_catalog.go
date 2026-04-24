package speech

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type VoiceQuery struct {
	Provider string
	ID       string
	Language string
	Gender   VoiceGender
}

func (m *Manager) ListAllVoices(ctx context.Context) ([]Voice, error) {
	providers := m.ListProviders()
	sort.Strings(providers)

	voices := make([]Voice, 0)
	for _, name := range providers {
		items, err := m.ListVoices(ctx, name)
		if err != nil {
			return nil, err
		}
		voices = append(voices, items...)
	}

	sort.Slice(voices, func(i, j int) bool {
		if voices[i].Provider == voices[j].Provider {
			if strings.EqualFold(voices[i].LanguageTag, voices[j].LanguageTag) {
				return strings.ToLower(voices[i].Name) < strings.ToLower(voices[j].Name)
			}
			return strings.ToLower(voices[i].LanguageTag) < strings.ToLower(voices[j].LanguageTag)
		}
		return strings.ToLower(voices[i].Provider) < strings.ToLower(voices[j].Provider)
	})
	return voices, nil
}

func (m *Manager) RecommendVoice(ctx context.Context, query VoiceQuery) (*Voice, error) {
	voices, err := m.ListAllVoices(ctx)
	if err != nil {
		return nil, err
	}
	if len(voices) == 0 {
		return nil, fmt.Errorf("tts: no voices available")
	}

	var best *Voice
	bestScore := -1
	for i := range voices {
		score := scoreVoiceCandidate(voices[i], query)
		if score > bestScore {
			voice := voices[i]
			best = &voice
			bestScore = score
		}
	}
	if best == nil || bestScore < 0 {
		return nil, fmt.Errorf("tts: no voice matched query")
	}
	return best, nil
}

func scoreVoiceCandidate(voice Voice, query VoiceQuery) int {
	score := 0

	provider := strings.TrimSpace(strings.ToLower(query.Provider))
	if provider != "" {
		if strings.EqualFold(voice.Provider, provider) {
			score += 40
		} else {
			return -1
		}
	}

	voiceID := strings.TrimSpace(strings.ToLower(query.ID))
	if voiceID != "" {
		if strings.EqualFold(voice.ID, voiceID) || strings.EqualFold(voice.Name, voiceID) {
			score += 80
		} else {
			return -1
		}
	}

	queryLang := normalizeVoiceLocale(query.Language)
	voiceLang := normalizeVoiceLocale(firstNonEmptyVoiceLocale(voice.LanguageTag, voice.Language))
	if queryLang != "" {
		switch {
		case voiceLang == queryLang:
			score += 50
		case strings.HasPrefix(voiceLang, queryLang+"-"), strings.HasPrefix(queryLang, voiceLang+"-"):
			score += 30
		case strings.HasPrefix(voiceLang, queryLang), strings.HasPrefix(queryLang, voiceLang):
			score += 20
		default:
			score -= 5
		}
	}

	if query.Gender != "" {
		if voice.Gender == query.Gender {
			score += 10
		} else if voice.Gender != "" {
			score -= 3
		}
	}

	if score == 0 {
		score = 1
	}
	return score
}

func normalizeVoiceLocale(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, "_", "-")
	return value
}

func firstNonEmptyVoiceLocale(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
