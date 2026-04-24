package speech

import (
	"context"
	"encoding/json"
	"testing"
)

type stubVoiceProvider struct {
	name   string
	voices []Voice
}

func (s *stubVoiceProvider) Name() string { return s.name }

func (s *stubVoiceProvider) Type() ProviderType { return ProviderCustom }

func (s *stubVoiceProvider) Synthesize(ctx context.Context, text string, opts ...SynthesizeOption) (*AudioResult, error) {
	_, _ = ctx, text
	payload, _ := json.Marshal(map[string]string{"ok": "true"})
	return &AudioResult{Data: payload, Format: FormatMP3}, nil
}

func (s *stubVoiceProvider) ListVoices(ctx context.Context) ([]Voice, error) {
	_ = ctx
	return append([]Voice(nil), s.voices...), nil
}

func TestListAllVoicesAggregatesProviders(t *testing.T) {
	manager := NewManager()
	if err := manager.Register("openai", &stubVoiceProvider{name: "openai", voices: []Voice{
		{ID: "alloy", Name: "Alloy", Provider: "openai", LanguageTag: "en-US"},
	}}); err != nil {
		t.Fatalf("Register openai: %v", err)
	}
	if err := manager.Register("piper", &stubVoiceProvider{name: "piper", voices: []Voice{
		{ID: "huayan", Name: "Huayan", Provider: "piper", LanguageTag: "zh-CN"},
		{ID: "tomoko", Name: "Tomoko", Provider: "piper", LanguageTag: "ja-JP"},
	}}); err != nil {
		t.Fatalf("Register piper: %v", err)
	}

	voices, err := manager.ListAllVoices(context.Background())
	if err != nil {
		t.Fatalf("ListAllVoices: %v", err)
	}
	if len(voices) != 3 {
		t.Fatalf("expected 3 voices, got %d", len(voices))
	}
}

func TestRecommendVoicePrefersLanguageAndProvider(t *testing.T) {
	manager := NewManager()
	if err := manager.Register("openai", &stubVoiceProvider{name: "openai", voices: []Voice{
		{ID: "nova", Name: "Nova", Provider: "openai", LanguageTag: "en-US", Gender: GenderFemale},
	}}); err != nil {
		t.Fatalf("Register openai: %v", err)
	}
	if err := manager.Register("piper", &stubVoiceProvider{name: "piper", voices: []Voice{
		{ID: "huayan", Name: "Huayan", Provider: "piper", LanguageTag: "zh-CN", Gender: GenderNeutral},
		{ID: "lessac", Name: "Lessac", Provider: "piper", LanguageTag: "en-US", Gender: GenderFemale},
	}}); err != nil {
		t.Fatalf("Register piper: %v", err)
	}

	voice, err := manager.RecommendVoice(context.Background(), VoiceQuery{
		Provider: "piper",
		Language: "zh-CN",
	})
	if err != nil {
		t.Fatalf("RecommendVoice: %v", err)
	}
	if voice.ID != "huayan" {
		t.Fatalf("expected huayan, got %q", voice.ID)
	}
}

func TestRecommendVoiceSupportsDirectIDLookup(t *testing.T) {
	manager := NewManager(WithManagerProvider("edge", &stubVoiceProvider{name: "edge", voices: []Voice{
		{ID: "aria", Name: "Aria", Provider: "edge", LanguageTag: "en-US", Gender: GenderFemale},
		{ID: "guy", Name: "Guy", Provider: "edge", LanguageTag: "en-US", Gender: GenderMale},
	}}))

	voice, err := manager.RecommendVoice(context.Background(), VoiceQuery{
		Provider: "edge",
		ID:       "guy",
	})
	if err != nil {
		t.Fatalf("RecommendVoice: %v", err)
	}
	if voice.Name != "Guy" {
		t.Fatalf("expected Guy, got %q", voice.Name)
	}
}
