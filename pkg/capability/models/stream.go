package llm

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type StreamChunk struct {
	Type   string   `json:"type"`
	Delta  Delta    `json:"delta,omitempty"`
	Choice []Choice `json:"choices,omitempty"`
}

type Delta struct {
	Content string `json:"content,omitempty"`
	Role    string `json:"role,omitempty"`
}

type Choice struct {
	Delta        Delta  `json:"delta"`
	FinishReason string `json:"finish_reason,omitempty"`
}

type OpenAIDecoder struct {
	scanner *bufio.Scanner
}

func NewDecoder(r io.Reader) *OpenAIDecoder {
	return &OpenAIDecoder{
		scanner: bufio.NewScanner(r),
	}
}

func (d *OpenAIDecoder) Decode() (*StreamChunk, error) {
	for d.scanner.Scan() {
		line := d.scanner.Text()
		line = strings.TrimPrefix(line, "data: ")

		if line == "" {
			continue
		}

		if line == "[DONE]" {
			return &StreamChunk{Type: "done"}, nil
		}

		var chunk StreamChunk
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			continue
		}

		if len(chunk.Choice) > 0 {
			chunk.Delta = chunk.Choice[0].Delta
		}

		chunk.Type = "chunk"
		return &chunk, nil
	}

	if err := d.scanner.Err(); err != nil {
		return nil, err
	}

	return &StreamChunk{Type: "done"}, nil
}

type AnthropicChunk struct {
	Type              string            `json:"type"`
	Delta             AnthropicDelta    `json:"delta,omitempty"`
	ContentBlockDelta ContentBlockDelta `json:"content_block_delta,omitempty"`
	Index             int               `json:"index,omitempty"`
}

type AnthropicDelta struct {
	Type string `json:"type,omitempty"`
	Text string `json:"text,omitempty"`
}

type ContentBlockDelta struct {
	Type string `json:"type,omitempty"`
	Text string `json:"text,omitempty"`
}

type AnthropicDecoder struct {
	scanner *bufio.Scanner
}

func NewAnthropicDecoder(r io.Reader) *AnthropicDecoder {
	return &AnthropicDecoder{
		scanner: bufio.NewScanner(r),
	}
}

func (d *AnthropicDecoder) Decode() (*AnthropicChunk, error) {
	for d.scanner.Scan() {
		line := d.scanner.Text()
		line = strings.TrimPrefix(line, "data: ")

		if line == "" {
			continue
		}

		var chunk AnthropicChunk
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			continue
		}

		switch chunk.Type {
		case "content_block_delta":
			chunk.Delta.Type = chunk.ContentBlockDelta.Type
			chunk.Delta.Text = chunk.ContentBlockDelta.Text
			return &chunk, nil
		case "message_stop":
			return &chunk, nil
		case "error":
			return nil, fmt.Errorf("anthropic error: %s", line)
		}
	}

	if err := d.scanner.Err(); err != nil {
		return nil, err
	}

	return nil, io.EOF
}
