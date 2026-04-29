package llm

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type StreamChunk struct {
	Type         string   `json:"type"`
	Delta        Delta    `json:"delta,omitempty"`
	Choice       []Choice `json:"choices,omitempty"`
	FinishReason string   `json:"finish_reason,omitempty"`
}

type Delta struct {
	Content   string          `json:"content,omitempty"`
	Role      string          `json:"role,omitempty"`
	ToolCalls []ToolCallDelta `json:"tool_calls,omitempty"`
}

type ToolCallDelta struct {
	Index    *int              `json:"index,omitempty"`
	ID       string            `json:"id,omitempty"`
	Type     string            `json:"type,omitempty"`
	Function FunctionCallDelta `json:"function,omitempty"`
}

type FunctionCallDelta struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
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
			chunk.FinishReason = chunk.Choice[0].FinishReason
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
	Type              string                `json:"type"`
	Delta             AnthropicDelta        `json:"delta,omitempty"`
	ContentBlockDelta ContentBlockDelta     `json:"content_block_delta,omitempty"`
	ContentBlock      AnthropicContentBlock `json:"content_block,omitempty"`
	Index             int                   `json:"index,omitempty"`
	Usage             Usage                 `json:"usage,omitempty"`
}

type AnthropicDelta struct {
	Type        string `json:"type,omitempty"`
	Text        string `json:"text,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
	StopReason  string `json:"stop_reason,omitempty"`
}

type ContentBlockDelta struct {
	Type        string `json:"type,omitempty"`
	Text        string `json:"text,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
}

type AnthropicContentBlock struct {
	Type  string         `json:"type,omitempty"`
	Text  string         `json:"text,omitempty"`
	ID    string         `json:"id,omitempty"`
	Name  string         `json:"name,omitempty"`
	Input map[string]any `json:"input,omitempty"`
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

		if chunk.ContentBlockDelta.Type != "" || chunk.ContentBlockDelta.Text != "" || chunk.ContentBlockDelta.PartialJSON != "" {
			if chunk.Delta.Type == "" {
				chunk.Delta.Type = chunk.ContentBlockDelta.Type
			}
			if chunk.Delta.Text == "" {
				chunk.Delta.Text = chunk.ContentBlockDelta.Text
			}
			if chunk.Delta.PartialJSON == "" {
				chunk.Delta.PartialJSON = chunk.ContentBlockDelta.PartialJSON
			}
		}

		switch chunk.Type {
		case "content_block_start", "content_block_delta", "content_block_stop", "message_delta":
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
