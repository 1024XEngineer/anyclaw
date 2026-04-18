package openai

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	llm "github.com/1024XEngineer/anyclaw/pkg/capability/models"
	appRuntime "github.com/1024XEngineer/anyclaw/pkg/runtime"
)

type RuntimeResolver func(requestedModel string) (*appRuntime.App, string, error)
type ModelCatalog func() []string

type Handler struct {
	resolveRuntime RuntimeResolver
	listModels     ModelCatalog
}

func NewHandler(resolveRuntime RuntimeResolver, listModels ModelCatalog) *Handler {
	return &Handler{
		resolveRuntime: resolveRuntime,
		listModels:     listModels,
	}
}

type chatRequest struct {
	Model       string    `json:"model"`
	Messages    []message `json:"messages"`
	Tools       []tool    `json:"tools,omitempty"`
	ToolChoice  any       `json:"tool_choice,omitempty"`
	Temperature *float64  `json:"temperature,omitempty"`
	MaxTokens   *int      `json:"max_tokens,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
	TopP        *float64  `json:"top_p,omitempty"`
	User        string    `json:"user,omitempty"`
}

type responsesRequest struct {
	Model     string `json:"model"`
	Input     any    `json:"input"`
	Stream    bool   `json:"stream,omitempty"`
	Tools     []tool `json:"tools,omitempty"`
	MaxTokens *int   `json:"max_output_tokens,omitempty"`
}

type message struct {
	Role       string     `json:"role"`
	Content    any        `json:"content"`
	Name       string     `json:"name,omitempty"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type tool struct {
	Type     string      `json:"type"`
	Function functionDef `json:"function"`
}

type functionDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type toolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Index    *int         `json:"index,omitempty"`
	Function functionCall `json:"function"`
}

type functionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type chatResponse struct {
	ID                string   `json:"id"`
	Object            string   `json:"object"`
	Created           int64    `json:"created"`
	Model             string   `json:"model"`
	Choices           []choice `json:"choices"`
	Usage             usage    `json:"usage"`
	SystemFingerprint string   `json:"system_fingerprint,omitempty"`
}

type choice struct {
	Index        int      `json:"index"`
	Message      *message `json:"message,omitempty"`
	Delta        *message `json:"delta,omitempty"`
	FinishReason string   `json:"finish_reason"`
}

type usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type chunk struct {
	ID                string   `json:"id"`
	Object            string   `json:"object"`
	Created           int64    `json:"created"`
	Model             string   `json:"model"`
	Choices           []choice `json:"choices"`
	SystemFingerprint string   `json:"system_fingerprint,omitempty"`
}

func (h *Handler) HandleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", "invalid_request_error")
		return
	}
	if len(req.Messages) == 0 {
		writeError(w, http.StatusBadRequest, "messages is required", "invalid_request_error")
		return
	}

	targetApp, resolvedModel, err := h.resolveTarget(req.Model)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "internal_error")
		return
	}

	llmMessages := convertMessages(req.Messages)

	var toolDefs []llm.ToolDefinition
	if len(req.Tools) > 0 {
		toolDefs = convertTools(req.Tools)
	}

	if req.Stream {
		h.handleChatStream(w, r, targetApp, resolvedModel, llmMessages, toolDefs)
		return
	}

	response, err := targetApp.Chat(r.Context(), llmMessages, toolDefs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "internal_error")
		return
	}

	resp := chatResponse{
		ID:      fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   resolvedModel,
		Choices: []choice{
			{
				Index:        0,
				Message:      &message{Role: "assistant", Content: response.Content},
				FinishReason: "stop",
			},
		},
		Usage: usage{
			PromptTokens:     response.Usage.InputTokens,
			CompletionTokens: response.Usage.OutputTokens,
			TotalTokens:      response.Usage.InputTokens + response.Usage.OutputTokens,
		},
	}

	if len(response.ToolCalls) > 0 {
		resp.Choices[0].FinishReason = "tool_calls"
		resp.Choices[0].Message.ToolCalls = make([]toolCall, 0, len(response.ToolCalls))
		for _, tc := range response.ToolCalls {
			resp.Choices[0].Message.ToolCalls = append(resp.Choices[0].Message.ToolCalls, toolCall{
				ID:   tc.ID,
				Type: "function",
				Function: functionCall{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			})
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) HandleModels(w http.ResponseWriter, r *http.Request) {
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

	modelNames := uniqueNames(nil)
	if h.listModels != nil {
		modelNames = uniqueNames(h.listModels())
	}

	models := make([]modelInfo, 0, len(modelNames))
	created := time.Now().Unix()
	for _, name := range modelNames {
		models = append(models, modelInfo{
			ID:      name,
			Object:  "model",
			Created: created,
			OwnedBy: "anyclaw",
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data":   models,
	})
}

func (h *Handler) HandleResponses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req responsesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	_, messages := convertResponsesInput(req.Input)
	if len(messages) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "input is required"})
		return
	}

	targetApp, resolvedModel, err := h.resolveTarget(req.Model)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	var toolDefs []llm.ToolDefinition
	if len(req.Tools) > 0 {
		toolDefs = convertTools(req.Tools)
	}

	if req.Stream {
		h.handleResponsesStream(w, r, targetApp, resolvedModel, messages, toolDefs)
		return
	}

	response, err := targetApp.Chat(r.Context(), messages, toolDefs)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	output := make([]map[string]any, 0, len(response.ToolCalls)+1)
	for _, tc := range response.ToolCalls {
		output = append(output, map[string]any{
			"type":      "function_call",
			"id":        tc.ID,
			"name":      tc.Function.Name,
			"arguments": tc.Function.Arguments,
		})
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
		"model":  resolvedModel,
		"output": output,
		"usage": map[string]any{
			"input_tokens":  response.Usage.InputTokens,
			"output_tokens": response.Usage.OutputTokens,
			"total_tokens":  response.Usage.InputTokens + response.Usage.OutputTokens,
		},
	})
}

func (h *Handler) handleChatStream(w http.ResponseWriter, r *http.Request, targetApp *appRuntime.App, resolvedModel string, messages []llm.Message, toolDefs []llm.ToolDefinition) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	chunkID := fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
	writeSSEData(w, flusher, chunk{
		ID:      chunkID,
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   resolvedModel,
		Choices: []choice{
			{Index: 0, Delta: &message{Role: "assistant", Content: ""}},
		},
	})

	err := targetApp.StreamChat(r.Context(), messages, toolDefs, func(part string) {
		select {
		case <-r.Context().Done():
			return
		default:
		}
		writeSSEData(w, flusher, chunk{
			ID:      chunkID,
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Model:   resolvedModel,
			Choices: []choice{
				{Index: 0, Delta: &message{Content: part}},
			},
		})
	})

	finishReason := "stop"
	if err != nil {
		finishReason = "error"
	}
	writeSSEData(w, flusher, chunk{
		ID:      chunkID,
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   resolvedModel,
		Choices: []choice{
			{Index: 0, Delta: &message{}, FinishReason: finishReason},
		},
	})
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func (h *Handler) handleResponsesStream(w http.ResponseWriter, r *http.Request, targetApp *appRuntime.App, resolvedModel string, messages []llm.Message, toolDefs []llm.ToolDefinition) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	respID := fmt.Sprintf("resp_%d", time.Now().UnixNano())
	writeResponseEvent(w, flusher, respID, "response.created", resolvedModel, map[string]any{})

	var content strings.Builder
	err := targetApp.StreamChat(r.Context(), messages, toolDefs, func(part string) {
		select {
		case <-r.Context().Done():
			return
		default:
		}
		content.WriteString(part)
		writeResponseEvent(w, flusher, respID, "response.output_text.delta", resolvedModel, map[string]any{
			"text": part,
		})
	})

	if err != nil {
		writeResponseEvent(w, flusher, respID, "response.failed", resolvedModel, map[string]any{
			"error": err.Error(),
		})
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
		return
	}

	writeResponseEvent(w, flusher, respID, "response.completed", resolvedModel, map[string]any{
		"output": []map[string]any{
			{
				"type": "message",
				"role": "assistant",
				"content": []map[string]any{
					{"type": "output_text", "text": content.String()},
				},
			},
		},
	})
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func (h *Handler) resolveTarget(requestedModel string) (*appRuntime.App, string, error) {
	if h.resolveRuntime == nil {
		return nil, "", fmt.Errorf("runtime resolver is not configured")
	}
	return h.resolveRuntime(requestedModel)
}

func writeSSEData(w http.ResponseWriter, flusher http.Flusher, data any) {
	payload, _ := json.Marshal(data)
	fmt.Fprintf(w, "data: %s\n\n", payload)
	flusher.Flush()
}

func writeResponseEvent(w http.ResponseWriter, flusher http.Flusher, responseID string, eventType string, model string, data map[string]any) {
	payload := map[string]any{
		"type": eventType,
		"data": data,
	}
	if responseID != "" {
		payload["response"] = map[string]any{
			"id":     responseID,
			"object": "response",
			"model":  model,
		}
	}
	writeSSEData(w, flusher, payload)
}

func writeError(w http.ResponseWriter, statusCode int, message string, errorType string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"message": message,
			"type":    errorType,
		},
	})
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}

func convertMessages(msgs []message) []llm.Message {
	result := make([]llm.Message, 0, len(msgs))
	for _, msg := range msgs {
		out := llm.Message{
			Role:       msg.Role,
			Name:       msg.Name,
			ToolCallID: msg.ToolCallID,
		}

		switch value := msg.Content.(type) {
		case string:
			out.Content = value
		case nil:
			out.Content = ""
		case []any:
			blocks := make([]llm.ContentBlock, 0, len(value))
			for _, part := range value {
				item, ok := part.(map[string]any)
				if !ok {
					continue
				}
				switch item["type"] {
				case "text":
					text, _ := item["text"].(string)
					if text == "" {
						continue
					}
					blocks = append(blocks, llm.ContentBlock{
						Type: llm.ContentTypeText,
						Text: text,
					})
				case "image_url":
					imageURL, _ := item["image_url"].(map[string]any)
					url, _ := imageURL["url"].(string)
					if url == "" {
						continue
					}
					blocks = append(blocks, llm.ContentBlock{
						Type: llm.ContentTypeImageURL,
						ImageURL: &llm.ImageURLBlock{
							URL: url,
						},
					})
				}
			}
			if len(blocks) > 0 {
				out.SetContentBlocks(blocks)
			}
		}

		if len(msg.ToolCalls) > 0 {
			out.ToolCalls = make([]llm.ToolCall, 0, len(msg.ToolCalls))
			for _, tc := range msg.ToolCalls {
				out.ToolCalls = append(out.ToolCalls, llm.ToolCall{
					ID:   tc.ID,
					Type: tc.Type,
					Function: llm.FunctionCall{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				})
			}
		}

		result = append(result, out)
	}
	return result
}

func convertTools(tools []tool) []llm.ToolDefinition {
	result := make([]llm.ToolDefinition, 0, len(tools))
	for _, item := range tools {
		result = append(result, llm.ToolDefinition{
			Type: "function",
			Function: llm.ToolFunctionDefinition{
				Name:        item.Function.Name,
				Description: item.Function.Description,
				Parameters:  item.Function.Parameters,
			},
		})
	}
	return result
}

func convertResponsesInput(input any) (string, []llm.Message) {
	var lastMessage string
	messages := make([]llm.Message, 0)

	switch value := input.(type) {
	case string:
		lastMessage = value
		messages = append(messages, llm.Message{Role: "user", Content: value})
	case []any:
		for _, item := range value {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			switch entry["type"] {
			case "message":
				role := "user"
				if rawRole, ok := entry["role"].(string); ok && strings.TrimSpace(rawRole) != "" {
					role = rawRole
				}
				content := ""
				switch rawContent := entry["content"].(type) {
				case string:
					content = rawContent
				case []any:
					for _, part := range rawContent {
						block, ok := part.(map[string]any)
						if !ok {
							continue
						}
						if block["type"] == "input_text" || block["type"] == "output_text" {
							if text, ok := block["text"].(string); ok && text != "" {
								content += text
							}
						}
					}
				}
				if content == "" {
					continue
				}
				messages = append(messages, llm.Message{Role: role, Content: content})
				lastMessage = content
			case "function_call_output":
				callID, _ := entry["call_id"].(string)
				output, _ := entry["output"].(string)
				messages = append(messages, llm.Message{
					Role:       "tool",
					ToolCallID: callID,
					Content:    output,
				})
			}
		}
	}

	return lastMessage, messages
}

func uniqueNames(names []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, name)
	}
	return result
}
