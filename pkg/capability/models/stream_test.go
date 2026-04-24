package llm

import (
	"io"
	"strings"
	"testing"
)

func TestOpenAIDecoderDecodeChunk(t *testing.T) {
	decoder := NewDecoder(strings.NewReader("data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"hello\"}}]}\n"))

	chunk, err := decoder.Decode()
	if err != nil {
		t.Fatalf("Decode returned error: %v", err)
	}
	if chunk.Type != "chunk" {
		t.Fatalf("expected chunk type, got %q", chunk.Type)
	}
	if chunk.Delta.Role != "assistant" || chunk.Delta.Content != "hello" {
		t.Fatalf("unexpected chunk delta: %#v", chunk.Delta)
	}
}

func TestOpenAIDecoderSkipsInvalidJSONAndFinishes(t *testing.T) {
	input := "data: {invalid-json}\n\ndata: [DONE]\n"
	decoder := NewDecoder(strings.NewReader(input))

	chunk, err := decoder.Decode()
	if err != nil {
		t.Fatalf("Decode returned error: %v", err)
	}
	if chunk.Type != "done" {
		t.Fatalf("expected done after invalid line and DONE marker, got %q", chunk.Type)
	}
}

func TestOpenAIDecoderReturnsDoneAtEOF(t *testing.T) {
	decoder := NewDecoder(strings.NewReader(""))

	chunk, err := decoder.Decode()
	if err != nil {
		t.Fatalf("Decode returned error: %v", err)
	}
	if chunk.Type != "done" {
		t.Fatalf("expected done at EOF, got %q", chunk.Type)
	}
}

func TestAnthropicDecoderContentBlockDelta(t *testing.T) {
	input := "data: {\"type\":\"content_block_delta\",\"content_block_delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}}\n"
	decoder := NewAnthropicDecoder(strings.NewReader(input))

	chunk, err := decoder.Decode()
	if err != nil {
		t.Fatalf("Decode returned error: %v", err)
	}
	if chunk.Type != "content_block_delta" {
		t.Fatalf("expected content_block_delta type, got %q", chunk.Type)
	}
	if chunk.Delta.Type != "text_delta" || chunk.Delta.Text != "hello" {
		t.Fatalf("unexpected anthropic delta: %#v", chunk.Delta)
	}
}

func TestAnthropicDecoderMessageStop(t *testing.T) {
	decoder := NewAnthropicDecoder(strings.NewReader("data: {\"type\":\"message_stop\"}\n"))

	chunk, err := decoder.Decode()
	if err != nil {
		t.Fatalf("Decode returned error: %v", err)
	}
	if chunk.Type != "message_stop" {
		t.Fatalf("expected message_stop, got %q", chunk.Type)
	}
}

func TestAnthropicDecoderErrorEvent(t *testing.T) {
	decoder := NewAnthropicDecoder(strings.NewReader("data: {\"type\":\"error\"}\n"))

	_, err := decoder.Decode()
	if err == nil || !strings.Contains(err.Error(), "anthropic error") {
		t.Fatalf("expected anthropic error, got %v", err)
	}
}

func TestAnthropicDecoderReturnsEOF(t *testing.T) {
	decoder := NewAnthropicDecoder(strings.NewReader(""))

	_, err := decoder.Decode()
	if err != io.EOF {
		t.Fatalf("expected io.EOF, got %v", err)
	}
}
