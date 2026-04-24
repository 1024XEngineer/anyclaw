package llm

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type stubFailoverClient struct {
	name         string
	chatResp      *Response
	chatErrs      []error
	streamErrs    []error
	streamContent []string
	streamErrAfterChunks bool
	chatCalls     int
	streamCalls   int
}

func (s *stubFailoverClient) Chat(_ context.Context, _ []Message, _ []ToolDefinition) (*Response, error) {
	s.chatCalls++
	if len(s.chatErrs) > 0 {
		err := s.chatErrs[0]
		s.chatErrs = s.chatErrs[1:]
		return nil, err
	}
	if s.chatResp == nil {
		return &Response{Content: s.name}, nil
	}
	return s.chatResp, nil
}

func (s *stubFailoverClient) StreamChat(_ context.Context, _ []Message, _ []ToolDefinition, onChunk func(string)) error {
	s.streamCalls++
	if len(s.streamErrs) > 0 && !s.streamErrAfterChunks {
		err := s.streamErrs[0]
		s.streamErrs = s.streamErrs[1:]
		if err != nil {
			return err
		}
	}
	for _, chunk := range s.streamContent {
		onChunk(chunk)
	}
	if len(s.streamErrs) > 0 && s.streamErrAfterChunks {
		err := s.streamErrs[0]
		s.streamErrs = s.streamErrs[1:]
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *stubFailoverClient) Name() string {
	return s.name
}

type stubDiscoveryProvider struct {
	name   string
	models []ModelInfo
	err    error
}

func (s stubDiscoveryProvider) Name() string {
	return s.name
}

func (s stubDiscoveryProvider) ListModels(_ context.Context) ([]ModelInfo, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.models, nil
}

func TestNewFailoverClientAppliesDefaults(t *testing.T) {
	client := NewFailoverClient(&stubFailoverClient{name: "primary"}, FailoverConfig{})

	if client.config.MaxRetries != 3 {
		t.Fatalf("expected default max retries to be 3, got %d", client.config.MaxRetries)
	}
	if client.config.RetryDelay != time.Second {
		t.Fatalf("expected default retry delay to be 1s, got %v", client.config.RetryDelay)
	}
	if client.config.CooldownPeriod != 5*time.Minute {
		t.Fatalf("expected default cooldown to be 5m, got %v", client.config.CooldownPeriod)
	}
}

func TestFailoverChatFallsBackAfterPrimaryFailure(t *testing.T) {
	primary := &stubFailoverClient{
		name:     "primary",
		chatErrs: []error{errors.New("rate limit: 429"), errors.New("rate limit: 429")},
	}
	fallback := &stubFailoverClient{
		name:     "fallback",
		chatResp: &Response{Content: "fallback-response"},
	}

	client := NewFailoverClient(primary, FailoverConfig{
		Enabled:        true,
		MaxRetries:     1,
		RetryDelay:     time.Millisecond,
		CooldownPeriod: time.Minute,
	})
	client.AddFallback(fallback)

	resp, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	if resp.Content != "fallback-response" {
		t.Fatalf("expected fallback response, got %q", resp.Content)
	}
	if primary.chatCalls != 2 {
		t.Fatalf("expected primary to be retried twice, got %d calls", primary.chatCalls)
	}
	if fallback.chatCalls != 1 {
		t.Fatalf("expected fallback to be called once, got %d", fallback.chatCalls)
	}
}

func TestFailoverStreamChatFallsBack(t *testing.T) {
	primary := &stubFailoverClient{
		name:       "primary",
		streamErrs: []error{errors.New("connection refused")},
	}
	fallback := &stubFailoverClient{
		name:         "fallback",
		streamContent: []string{"hello", " world"},
	}

	client := NewFailoverClient(primary, FailoverConfig{Enabled: true})
	client.AddFallback(fallback)

	var chunks []string
	err := client.StreamChat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, func(chunk string) {
		chunks = append(chunks, chunk)
	})
	if err != nil {
		t.Fatalf("StreamChat returned error: %v", err)
	}
	if strings.Join(chunks, "") != "hello world" {
		t.Fatalf("unexpected streamed content: %q", strings.Join(chunks, ""))
	}
}

func TestFailoverStreamChatDoesNotFallbackAfterPartialOutput(t *testing.T) {
	primary := &stubFailoverClient{
		name:                 "primary",
		streamContent:        []string{"partial"},
		streamErrs:           []error{errors.New("upstream stream reset")},
		streamErrAfterChunks: true,
	}
	fallback := &stubFailoverClient{
		name:          "fallback",
		streamContent: []string{"replacement"},
	}

	client := NewFailoverClient(primary, FailoverConfig{Enabled: true})
	client.AddFallback(fallback)

	var chunks []string
	err := client.StreamChat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, func(chunk string) {
		chunks = append(chunks, chunk)
	})
	if err == nil {
		t.Fatal("expected partial-output stream failure to be returned")
	}
	if !strings.Contains(err.Error(), "partial output") {
		t.Fatalf("expected partial output error, got %v", err)
	}
	if strings.Join(chunks, "") != "partial" {
		t.Fatalf("expected only primary partial output, got %q", strings.Join(chunks, ""))
	}
	if fallback.streamCalls != 0 {
		t.Fatalf("expected fallback not to be called after partial output, got %d calls", fallback.streamCalls)
	}
}

func TestFailoverTryClientSetsCooldownAndResetsOnSuccess(t *testing.T) {
	primary := &stubFailoverClient{
		name:     "primary",
		chatErrs: []error{
			errors.New("500 upstream"),
			errors.New("500 upstream"),
			errors.New("500 upstream"),
			errors.New("500 upstream"),
		},
	}

	client := NewFailoverClient(primary, FailoverConfig{
		Enabled:        true,
		MaxRetries:     1,
		RetryDelay:     time.Millisecond,
		CooldownPeriod: 50 * time.Millisecond,
	})

	_, err := client.tryClient(context.Background(), primary, nil, nil)
	if err == nil {
		t.Fatal("expected first tryClient call to fail")
	}
	if client.errorCounts["primary"] != 1 {
		t.Fatalf("expected error count 1, got %d", client.errorCounts["primary"])
	}

	_, err = client.tryClient(context.Background(), primary, nil, nil)
	if err == nil {
		t.Fatal("expected second tryClient call to fail")
	}
	if _, ok := client.cooldowns["primary"]; !ok {
		t.Fatal("expected primary client to enter cooldown after repeated failures")
	}

	_, err = client.tryClient(context.Background(), primary, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "cooldown") {
		t.Fatalf("expected cooldown error, got %v", err)
	}

	client.cooldowns["primary"] = time.Now().Add(-time.Second)
	primary.chatErrs = nil
	primary.chatResp = &Response{Content: "recovered"}

	resp, err := client.tryClient(context.Background(), primary, nil, nil)
	if err != nil {
		t.Fatalf("expected recovered client to succeed: %v", err)
	}
	if resp.Content != "recovered" {
		t.Fatalf("expected recovered content, got %q", resp.Content)
	}
	if client.errorCounts["primary"] != 0 {
		t.Fatalf("expected error count reset to 0, got %d", client.errorCounts["primary"])
	}
	if _, ok := client.cooldowns["primary"]; ok {
		t.Fatal("expected cooldown entry to be cleared after success")
	}
}

func TestFailoverStatusAndHelpers(t *testing.T) {
	primary := &stubFailoverClient{name: "primary"}
	fallback := &stubFailoverClient{name: "fallback"}

	client := NewFailoverClient(primary, FailoverConfig{Enabled: true})
	client.AddFallback(fallback)
	client.cooldowns["primary"] = time.Now().Add(time.Minute)
	client.errorCounts["primary"] = 2

	status := client.GetStatus()
	if status["enabled"] != true {
		t.Fatalf("expected enabled status, got %v", status["enabled"])
	}
	if status["primary"] != "primary" {
		t.Fatalf("expected primary name, got %v", status["primary"])
	}

	fallbacks, ok := status["fallbacks"].([]string)
	if !ok || len(fallbacks) != 1 || fallbacks[0] != "fallback" {
		t.Fatalf("unexpected fallback list: %#v", status["fallbacks"])
	}

	cooldowns, ok := status["cooldowns"].(map[string]string)
	if !ok || cooldowns["primary"] == "" {
		t.Fatalf("expected cooldown status for primary, got %#v", status["cooldowns"])
	}

	if !isRetryableError(errors.New("rate limit 429")) {
		t.Fatal("expected rate limit error to be retryable")
	}
	if !isRetryableError(errors.New("connection refused")) {
		t.Fatal("expected connection error to be retryable")
	}
	if isRetryableError(errors.New("permission denied")) {
		t.Fatal("expected permission error to be non-retryable")
	}
	if !contains("abc123", "123") {
		t.Fatal("expected contains helper to find substring")
	}
	if contains("abc", "xyz") {
		t.Fatal("expected contains helper to return false")
	}
}

func TestModelDiscoveryDiscoversAndFindsModels(t *testing.T) {
	discovery := NewModelDiscovery()
	discovery.RegisterProvider(stubDiscoveryProvider{
		name: "openai",
		models: []ModelInfo{
			{ID: "gpt-4o", Name: "GPT-4o", Provider: "openai", Capabilities: []string{"chat", "vision"}},
		},
	})
	discovery.RegisterProvider(stubDiscoveryProvider{
		name: "broken",
		err:  errors.New("provider unavailable"),
	})

	if err := discovery.DiscoverModels(context.Background()); err != nil {
		t.Fatalf("DiscoverModels returned error: %v", err)
	}

	models := discovery.ListModels()
	if len(models) != 1 {
		t.Fatalf("expected one successful provider entry, got %d", len(models))
	}
	if got := discovery.FindModel("gpt-4o", ""); got == nil || got.Provider != "openai" {
		t.Fatalf("expected to find model by id, got %#v", got)
	}
	if got := discovery.FindModel("", "vision"); got == nil || got.Name != "GPT-4o" {
		t.Fatalf("expected to find model by capability, got %#v", got)
	}
	if got := discovery.FindModel("missing", ""); got != nil {
		t.Fatalf("expected missing model lookup to return nil, got %#v", got)
	}
}
