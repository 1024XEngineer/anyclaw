package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/anyclaw/anyclaw/pkg/llm"
	appRuntime "github.com/anyclaw/anyclaw/pkg/runtime"
)

// OpenAI-compatible chat completion request
type openAIChatRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	Tools       []openAITool    `json:"tools,omitempty"`
	ToolChoice  any             `json:"tool_choice,omitempty"`
	Temperature *float64        `json:"temperature,omitempty"`
	MaxTokens   *int            `json:"max_tokens,omitempty"`
	Stream      bool            `json:"stream,omitempty"`
	TopP        *float64        `json:"top_p,omitempty"`
	User        string          `json:"user,omitempty"`
}

type openAIMessage struct {
	Role       string           `json:"role"`
	Content    any              `json:"content"`
	Name       string           `json:"name,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type openAITool struct {
	Type     string            `json:"type"`
	Function openAIFunctionDef `json:"function"`
}

type openAIFunctionDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type openAIToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Index    *int               `json:"index,omitempty"`
	Function openAIFunctionCall `json:"function"`
}

type openAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// OpenAI-compatible chat completion response
type openAIChatResponse struct {
	ID                string         `json:"id"`
	Object            string         `json:"object"`
	Created           int64          `json:"created"`
	Model             string         `json:"model"`
	Choices           []openAIChoice `json:"choices"`
	Usage             openAIUsage    `json:"usage"`
	SystemFingerprint string         `json:"system_fingerprint,omitempty"`
}

type openAIChoice struct {
	Index        int            `json:"index"`
	Message      *openAIMessage `json:"message,omitempty"`
	Delta        *openAIMessage `json:"delta,omitempty"`
	FinishReason string         `json:"finish_reason"`
}

type openAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// OpenAI-compatible streaming chunk
type openAIChunk struct {
	ID                string         `json:"id"`
	Object            string         `json:"object"`
	Created           int64          `json:"created"`
	Model             string         `json:"model"`
	Choices           []openAIChoice `json:"choices"`
	SystemFingerprint string         `json:"system_fingerprint,omitempty"`
}

// handleOpenAIChatCompletions implements /v1/chat/completions
func (s *Server) handleOpenAIChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req openAIChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "Invalid request body", "invalid_request_error")
		return
	}

	if len(req.Messages) == 0 {
		writeOpenAIError(w, http.StatusBadRequest, "messages is required", "invalid_request_error")
		return
	}

	// Get or create runtime
	agentName := s.app.Config.ResolveMainAgentName()
	if req.Model != "" {
		agentName = req.Model
	}
	targetApp, err := s.runtimePool.GetOrCreate(agentName, "", "", "")
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "internal_error")
		return
	}

	// Convert OpenAI messages to llm messages
	llmMessages := convertOpenAIMessages(req.Messages)

	// Convert tools if provided
	var toolDefs []llm.ToolDefinition
	if len(req.Tools) > 0 {
		toolDefs = convertOpenAITools(req.Tools)
	}

	// Handle streaming
	if req.Stream {
		s.handleOpenAIStream(w, r, targetApp, llmMessages, toolDefs, req)
		return
	}

	// Non-streaming: use Chat method
	ctx := r.Context()
	response, err := targetApp.LLM.Chat(ctx, llmMessages, toolDefs)
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "internal_error")
		return
	}

	// Build OpenAI-compatible response
	chunkID := fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
	openAIResp := openAIChatResponse{
		ID:      chunkID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []openAIChoice{
			{
				Index:        0,
				Message:      &openAIMessage{Role: "assistant", Content: response.Content},
				FinishReason: "stop",
			},
		},
		Usage: openAIUsage{
			PromptTokens:     response.Usage.InputTokens,
			CompletionTokens: response.Usage.OutputTokens,
			TotalTokens:      response.Usage.InputTokens + response.Usage.OutputTokens,
		},
	}

	// Handle tool calls in response
	if len(response.ToolCalls) > 0 {
		var toolCalls []openAIToolCall
		for _, tc := range response.ToolCalls {
			toolCalls = append(toolCalls, openAIToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: openAIFunctionCall{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			})
		}
		openAIResp.Choices[0].Message.ToolCalls = toolCalls
		openAIResp.Choices[0].FinishReason = "tool_calls"
	}

	writeJSON(w, http.StatusOK, openAIResp)
}

// handleOpenAIStream handles streaming responses
func (s *Server) handleOpenAIStream(w http.ResponseWriter, r *http.Request, targetApp *appRuntime.App, messages []llm.Message, toolDefs []llm.ToolDefinition, req openAIChatRequest) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	chunkID := fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())

	// Send initial chunk
	writeSSEData(w, flusher, openAIChunk{
		ID:      chunkID,
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []openAIChoice{
			{Index: 0, Delta: &openAIMessage{Role: "assistant", Content: ""}},
		},
	})

	// Stream content
	var fullContent strings.Builder

	err := targetApp.LLM.StreamChat(ctx, messages, toolDefs, func(chunk string) {
		select {
		case <-ctx.Done():
			return
		default:
		}
		fullContent.WriteString(chunk)
		writeSSEData(w, flusher, openAIChunk{
			ID:      chunkID,
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Model:   req.Model,
			Choices: []openAIChoice{
				{Index: 0, Delta: &openAIMessage{Content: chunk}},
			},
		})
	})

	// Send final chunk
	finishReason := "stop"
	if err != nil {
		finishReason = "error"
	}
	finalChunk := openAIChunk{
		ID:      chunkID,
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []openAIChoice{
			{Index: 0, Delta: &openAIMessage{}, FinishReason: finishReason},
		},
	}
	if err != nil {
		finalChunk.Choices[0].FinishReason = "stop"
	}
	writeSSEData(w, flusher, finalChunk)
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func writeSSEData(w http.ResponseWriter, flusher http.Flusher, data any) {
	jsonData, _ := json.Marshal(data)
	fmt.Fprintf(w, "data: %s\n\n", jsonData)
	flusher.Flush()
}

func writeOpenAIError(w http.ResponseWriter, statusCode int, message string, errorType string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"message": message,
			"type":    errorType,
		},
	})
}

func convertOpenAIMessages(msgs []openAIMessage) []llm.Message {
	var result []llm.Message
	for _, msg := range msgs {
		m := llm.Message{
			Role:       msg.Role,
			Name:       msg.Name,
			ToolCallID: msg.ToolCallID,
		}

		// Convert content
		switch v := msg.Content.(type) {
		case string:
			m.Content = v
		case nil:
			m.Content = ""
		case []any:
			var blocks []llm.ContentBlock
			for _, part := range v {
				if p, ok := part.(map[string]any); ok {
					switch p["type"] {
					case "text":
						if text, ok := p["text"].(string); ok {
							blocks = append(blocks, llm.ContentBlock{
								Type: llm.ContentTypeText,
								Text: text,
							})
						}
					case "image_url":
						if img, ok := p["image_url"].(map[string]any); ok {
							if url, ok := img["url"].(string); ok {
								blocks = append(blocks, llm.ContentBlock{
									Type: llm.ContentTypeImageURL,
									ImageURL: &llm.ImageURLBlock{
										URL: url,
									},
								})
							}
						}
					}
				}
			}
			if len(blocks) > 0 {
				m.SetContentBlocks(blocks)
			}
		}

		// Convert tool calls
		if len(msg.ToolCalls) > 0 {
			var toolCalls []llm.ToolCall
			for _, tc := range msg.ToolCalls {
				toolCalls = append(toolCalls, llm.ToolCall{
					ID:   tc.ID,
					Type: tc.Type,
					Function: llm.FunctionCall{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				})
			}
			m.ToolCalls = toolCalls
		}

		result = append(result, m)
	}
	return result
}

func convertOpenAITools(tools []openAITool) []llm.ToolDefinition {
	var result []llm.ToolDefinition
	for _, t := range tools {
		result = append(result, llm.ToolDefinition{
			Type: "function",
			Function: llm.ToolFunctionDefinition{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				Parameters:  t.Function.Parameters,
			},
		})
	}
	return result
}

// handleOpenAIModels implements /v1/models
func (s *Server) handleOpenAIModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	type modelInfo struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Created int64  `json:"created"`
		OwnedBy string `json:"owned_by"`
	}

	var models []modelInfo
	for _, profile := range s.app.Config.Agent.Profiles {
		models = append(models, modelInfo{
			ID:      profile.Name,
			Object:  "model",
			Created: time.Now().Unix(),
			OwnedBy: "anyclaw",
		})
	}

	models = append(models, modelInfo{
		ID:      s.app.Config.LLM.Model,
		Object:  "model",
		Created: time.Now().Unix(),
		OwnedBy: "anyclaw",
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data":   models,
	})
}

// handleOpenAIResponses implements /v1/responses (OpenAI Responses API)
func (s *Server) handleOpenAIResponses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Model     string       `json:"model"`
		Input     any          `json:"input"`
		Stream    bool         `json:"stream,omitempty"`
		Tools     []openAITool `json:"tools,omitempty"`
		MaxTokens *int         `json:"max_output_tokens,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	var message string
	var messages []llm.Message

	switch v := req.Input.(type) {
	case string:
		message = v
		messages = []llm.Message{{Role: "user", Content: message}}
	case []any:
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				switch m["type"] {
				case "message":
					role := "user"
					if r, ok := m["role"].(string); ok {
						role = r
					}
					content := ""
					if c, ok := m["content"].(string); ok {
						content = c
					}
					messages = append(messages, llm.Message{Role: role, Content: content})
					if content != "" {
						message = content
					}
				case "function_call_output":
					callID, _ := m["call_id"].(string)
					output, _ := m["output"].(string)
					messages = append(messages, llm.Message{
						Role:       "tool",
						ToolCallID: callID,
						Content:    output,
					})
				}
			}
		}
	}

	if message == "" && len(messages) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "input is required"})
		return
	}

	ctx := r.Context()
	agentName := s.app.Config.ResolveMainAgentName()
	if req.Model != "" {
		agentName = req.Model
	}
	targetApp, err := s.runtimePool.GetOrCreate(agentName, "", "", "")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Convert tools if provided
	var toolDefs []llm.ToolDefinition
	if len(req.Tools) > 0 {
		toolDefs = convertOpenAITools(req.Tools)
	}

	// Handle streaming
	if req.Stream {
		s.handleOpenAIResponseStream(w, r, targetApp, messages, toolDefs, req)
		return
	}

	// Non-streaming
	response, err := targetApp.LLM.Chat(ctx, messages, toolDefs)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Build output array
	var output []map[string]any
	if len(response.ToolCalls) > 0 {
		for _, tc := range response.ToolCalls {
			output = append(output, map[string]any{
				"type":      "function_call",
				"id":        tc.ID,
				"name":      tc.Function.Name,
				"arguments": tc.Function.Arguments,
			})
		}
	}
	if response.Content != "" {
		output = append(output, map[string]any{
			"type": "message",
			"role": "assistant",
			"content": []map[string]any{
				{"type": "output_text", "text": response.Content},
			},
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":     fmt.Sprintf("resp_%d", time.Now().UnixNano()),
		"object": "response",
		"status": "completed",
		"model":  req.Model,
		"output": output,
		"usage": map[string]any{
			"input_tokens":  response.Usage.InputTokens,
			"output_tokens": response.Usage.OutputTokens,
			"total_tokens":  response.Usage.InputTokens + response.Usage.OutputTokens,
		},
	})
}

// handleOpenAIResponseStream handles streaming for /v1/responses
func (s *Server) handleOpenAIResponseStream(w http.ResponseWriter, r *http.Request, targetApp *appRuntime.App, messages []llm.Message, toolDefs []llm.ToolDefinition, req struct {
	Model     string       `json:"model"`
	Input     any          `json:"input"`
	Stream    bool         `json:"stream,omitempty"`
	Tools     []openAITool `json:"tools,omitempty"`
	MaxTokens *int         `json:"max_output_tokens,omitempty"`
}) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	respID := fmt.Sprintf("resp_%d", time.Now().UnixNano())

	// Send created event
	writeSSEResponse(w, flusher, respID, "response.created", req.Model, map[string]any{})

	// Stream output
	var content strings.Builder
	err := targetApp.LLM.StreamChat(ctx, messages, toolDefs, func(chunk string) {
		select {
		case <-ctx.Done():
			return
		default:
		}
		content.WriteString(chunk)
		writeSSEResponse(w, flusher, respID, "response.output_text.delta", req.Model, map[string]any{
			"text": chunk,
		})
	})

	// Send completed event
	writeSSEResponse(w, flusher, respID, "response.completed", req.Model, map[string]any{
		"output": []map[string]any{
			{
				"type":    "message",
				"role":    "assistant",
				"content": content.String(),
			},
		},
	})

	if err != nil {
		writeSSEResponse(w, flusher, respID, "response.failed", req.Model, map[string]any{
			"error": err.Error(),
		})
	}

	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func writeSSEResponse(w http.ResponseWriter, flusher http.Flusher, respID string, eventType string, model string, data map[string]any) {
	payload := map[string]any{
		"type": eventType,
		"data": data,
	}
	if respID != "" {
		payload["response"] = map[string]any{
			"id":     respID,
			"object": "response",
			"model":  model,
		}
	}
	jsonData, _ := json.Marshal(payload)
	fmt.Fprintf(w, "data: %s\n\n", jsonData)
	flusher.Flush()
}
