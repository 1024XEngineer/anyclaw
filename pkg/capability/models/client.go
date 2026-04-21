package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type Config struct {
	Provider    string
	Model       string
	APIKey      string
	BaseURL     string
	Proxy       string
	MaxTokens   int
	Temperature float64
	Extra       map[string]string
}

type Client interface {
	Chat(ctx context.Context, messages []Message, tools []ToolDefinition) (*Response, error)
	StreamChat(ctx context.Context, messages []Message, tools []ToolDefinition, onChunk func(string)) error
	Name() string
}

type Message struct {
	Role          string         `json:"role"`
	Content       string         `json:"content"`
	Name          string         `json:"name,omitempty"`
	ToolCallID    string         `json:"tool_call_id,omitempty"`
	ToolCalls     []ToolCall     `json:"tool_calls,omitempty"`
	contentBlocks []ContentBlock `json:"-"`
}

type Response struct {
	Content     string
	RawResponse any
	Usage       Usage
	StopReason  string
	ToolCalls   []ToolCall
}

type ToolDefinition struct {
	Type     string                 `json:"type"`
	Function ToolFunctionDefinition `json:"function"`
}

type ToolFunctionDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ToolChoice struct {
	Type     string         `json:"type"`
	Function FunctionChoice `json:"function,omitempty"`
}

type FunctionChoice struct {
	Name string `json:"name"`
}

type client struct {
	provider    string
	model       string
	apiKey      string
	baseURL     string
	proxyURL    string
	maxTokens   int
	temperature float64
	httpClient  *http.Client
}

type configurationRequiredClient struct {
	provider string
	err      error
}

func (c *configurationRequiredClient) Chat(_ context.Context, _ []Message, _ []ToolDefinition) (*Response, error) {
	return nil, c.err
}

func (c *configurationRequiredClient) StreamChat(_ context.Context, _ []Message, _ []ToolDefinition, _ func(string)) error {
	return c.err
}

func (c *configurationRequiredClient) Name() string {
	return c.provider
}

func NormalizeProviderName(provider string) string {
	return normalizeProvider(provider)
}

func ProviderRequiresAPIKey(provider string) bool {
	switch normalizeProvider(provider) {
	case "ollama":
		return false
	default:
		return true
	}
}

func newHTTPClient(proxyURL string) *http.Client {
	transport := &http.Transport{}

	if proxyURL != "" {
		if proxyURL == "system" {
			// Use system proxy
		} else {
			transport.Proxy = func(req *http.Request) (*url.URL, error) {
				return url.Parse(proxyURL)
			}
		}
	}

	return &http.Client{Transport: transport}
}

func NewClient(cfg Config) (Client, error) {
	c := &client{
		provider:    cfg.Provider,
		model:       cfg.Model,
		apiKey:      cfg.APIKey,
		baseURL:     cfg.BaseURL,
		proxyURL:    cfg.Proxy,
		maxTokens:   cfg.MaxTokens,
		temperature: cfg.Temperature,
		httpClient:  newHTTPClient(cfg.Proxy),
	}

	if ProviderRequiresAPIKey(c.provider) && c.apiKey == "" {
		return &configurationRequiredClient{
			provider: normalizeProvider(c.provider),
			err:      fmt.Errorf("API key is required. Configure a model provider before chatting"),
		}, nil
	}

	if c.baseURL == "" {
		c.baseURL = getDefaultBaseURL(c.provider)
	}

	return c, nil
}

func getDefaultBaseURL(provider string) string {
	provider = normalizeProvider(provider)
	switch provider {
	case "openai":
		return "https://api.openai.com/v1"
	case "anthropic":
		return "https://api.anthropic.com/v1"
	case "ollama":
		return "http://localhost:11434/v1"
	case "qwen":
		return "https://dashscope.aliyuncs.com/compatible-mode/v1"
	default:
		return "https://api.openai.com/v1"
	}
}

func normalizeProvider(provider string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if strings.Contains(provider, "qwen") || strings.Contains(provider, "dashscope") || strings.Contains(provider, "alibaba") {
		return "qwen"
	}
	if strings.Contains(provider, "anthropic") || strings.Contains(provider, "claude") {
		return "anthropic"
	}
	if strings.Contains(provider, "ollama") {
		return "ollama"
	}
	if strings.Contains(provider, "compatible") {
		return "compatible"
	}
	return "openai"
}

func (c *client) Name() string {
	return c.provider
}

func (c *client) Chat(ctx context.Context, messages []Message, tools []ToolDefinition) (*Response, error) {
	provider := normalizeProvider(c.provider)
	switch provider {
	case "openai", "ollama", "compatible", "qwen":
		return c.chatOpenAICompatible(ctx, messages, tools)
	case "anthropic":
		return c.chatAnthropic(ctx, messages, tools)
	default:
		return c.chatOpenAICompatible(ctx, messages, tools)
	}
}

func (c *client) StreamChat(ctx context.Context, messages []Message, tools []ToolDefinition, onChunk func(string)) error {
	provider := normalizeProvider(c.provider)
	switch provider {
	case "openai", "ollama", "compatible", "qwen":
		return c.streamOpenAICompatible(ctx, messages, tools, onChunk)
	case "anthropic":
		return c.streamAnthropic(ctx, messages, tools, onChunk)
	default:
		return c.streamOpenAICompatible(ctx, messages, tools, onChunk)
	}
}

func (c *client) chatOpenAICompatible(ctx context.Context, messages []Message, tools []ToolDefinition) (*Response, error) {
	url := fmt.Sprintf("%s/chat/completions", c.baseURL)

	serializedMessages := serializeMessagesOpenAI(messages)

	payload := map[string]any{
		"model":       c.model,
		"messages":    serializedMessages,
		"max_tokens":  c.maxTokens,
		"temperature": c.temperature,
	}
	if len(tools) > 0 {
		payload["tools"] = tools
		payload["tool_choice"] = "auto"
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: %s - %s", resp.Status, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content   json.RawMessage `json:"content"`
				ToolCalls []ToolCall      `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("no response from API")
	}

	content := extractOpenAICompatibleContent(result.Choices[0].Message.Content)
	if strings.TrimSpace(content) == "" && len(result.Choices[0].Message.ToolCalls) == 0 {
		return nil, fmt.Errorf("empty response content from API; provider/model may be incompatible with chat/completions")
	}

	return &Response{
		Content:     content,
		ToolCalls:   result.Choices[0].Message.ToolCalls,
		RawResponse: result,
		Usage: Usage{
			InputTokens:  result.Usage.PromptTokens,
			OutputTokens: result.Usage.CompletionTokens,
		},
		StopReason: result.Choices[0].FinishReason,
	}, nil
}

func (c *client) streamOpenAICompatible(ctx context.Context, messages []Message, tools []ToolDefinition, onChunk func(string)) error {
	url := fmt.Sprintf("%s/chat/completions", c.baseURL)

	serializedMessages := serializeMessagesOpenAI(messages)

	payload := map[string]any{
		"model":       c.model,
		"messages":    serializedMessages,
		"max_tokens":  c.maxTokens,
		"temperature": c.temperature,
		"stream":      true,
	}
	if len(tools) > 0 {
		payload["tools"] = tools
		payload["tool_choice"] = "auto"
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	decoder := NewDecoder(resp.Body)
	for {
		data, err := decoder.Decode()
		if err != nil {
			break
		}
		if data.Type == "chunk" && data.Delta.Content != "" {
			onChunk(data.Delta.Content)
		}
		if data.Type == "done" {
			break
		}
	}

	return nil
}

func extractOpenAICompatibleContent(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return ""
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}

	var blocks []map[string]any
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}

	var parts []string
	for _, block := range blocks {
		if s, ok := block["text"].(string); ok && strings.TrimSpace(s) != "" {
			parts = append(parts, s)
			continue
		}
		if textObj, ok := block["text"].(map[string]any); ok {
			if s, ok := textObj["value"].(string); ok && strings.TrimSpace(s) != "" {
				parts = append(parts, s)
			}
		}
	}

	return strings.Join(parts, "")
}

func (c *client) streamAnthropic(ctx context.Context, messages []Message, tools []ToolDefinition, onChunk func(string)) error {
	url := "https://api.anthropic.com/v1/messages"

	filteredMessages, systemPrompt := serializeMessagesAnthropic(messages)

	payload := map[string]any{
		"model":       c.model,
		"messages":    filteredMessages,
		"max_tokens":  c.maxTokens,
		"temperature": c.temperature,
		"stream":      true,
	}
	if systemPrompt != "" {
		payload["system"] = systemPrompt
	}
	if len(tools) > 0 {
		payloadTools := make([]map[string]any, 0, len(tools))
		for _, tool := range tools {
			payloadTools = append(payloadTools, map[string]any{
				"name":         tool.Function.Name,
				"description":  tool.Function.Description,
				"input_schema": tool.Function.Parameters,
			})
		}
		payload["tools"] = payloadTools
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	decoder := NewAnthropicDecoder(resp.Body)
	for {
		data, err := decoder.Decode()
		if err != nil {
			break
		}
		if data.Type == "content_block_delta" && data.Delta.Type == "text_delta" && data.Delta.Text != "" {
			onChunk(data.Delta.Text)
		}
		if data.Type == "message_stop" {
			break
		}
	}

	return nil
}

func (c *client) chatAnthropic(ctx context.Context, messages []Message, tools []ToolDefinition) (*Response, error) {
	url := "https://api.anthropic.com/v1/messages"

	filteredMessages, systemPrompt := serializeMessagesAnthropic(messages)

	payload := map[string]any{
		"model":       c.model,
		"messages":    filteredMessages,
		"max_tokens":  c.maxTokens,
		"temperature": c.temperature,
	}
	if systemPrompt != "" {
		payload["system"] = systemPrompt
	}
	if len(tools) > 0 {
		payloadTools := make([]map[string]any, 0, len(tools))
		for _, tool := range tools {
			payloadTools = append(payloadTools, map[string]any{
				"name":         tool.Function.Name,
				"description":  tool.Function.Description,
				"input_schema": tool.Function.Parameters,
			})
		}
		payload["tools"] = payloadTools
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: %s - %s", resp.Status, string(respBody))
	}

	var result struct {
		Content []struct {
			Type  string         `json:"type"`
			Text  string         `json:"text,omitempty"`
			ID    string         `json:"id,omitempty"`
			Name  string         `json:"name,omitempty"`
			Input map[string]any `json:"input,omitempty"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
		Usage      struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	content := ""
	toolCalls := []ToolCall{}
	for _, block := range result.Content {
		if block.Type == "text" {
			content += block.Text
		}
		if block.Type == "tool_use" && block.Name != "" {
			args, _ := json.Marshal(block.Input)
			toolCalls = append(toolCalls, ToolCall{ID: block.ID, Type: "function", Function: FunctionCall{Name: block.Name, Arguments: string(args)}})
		}
	}

	return &Response{
		Content:     strings.TrimSpace(content),
		ToolCalls:   toolCalls,
		RawResponse: result,
		Usage: Usage{
			InputTokens:  result.Usage.InputTokens,
			OutputTokens: result.Usage.OutputTokens,
		},
		StopReason: result.StopReason,
	}, nil
}

type ClientWrapper struct {
	client      Client
	provider    string
	model       string
	apiKey      string
	baseURL     string
	proxyURL    string
	maxTokens   int
	temperature float64
}

func NewClientWrapper(cfg Config) (*ClientWrapper, error) {
	maxTokens := cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	c := &ClientWrapper{
		provider:    normalizeProvider(cfg.Provider),
		model:       cfg.Model,
		apiKey:      cfg.APIKey,
		baseURL:     cfg.BaseURL,
		proxyURL:    cfg.Proxy,
		maxTokens:   maxTokens,
		temperature: cfg.Temperature,
	}

	if c.baseURL == "" {
		c.baseURL = getDefaultBaseURL(cfg.Provider)
	}

	if err := c.initClient(); err != nil {
		return nil, err
	}

	return c, nil
}

func (w *ClientWrapper) initClient() error {
	client, err := NewClient(Config{
		Provider:    w.provider,
		Model:       w.model,
		APIKey:      w.apiKey,
		BaseURL:     w.baseURL,
		Proxy:       w.proxyURL,
		MaxTokens:   w.maxTokens,
		Temperature: w.temperature,
	})
	if err != nil {
		return err
	}
	w.client = client
	return nil
}

func (w *ClientWrapper) Chat(ctx context.Context, messages []Message, tools []ToolDefinition) (*Response, error) {
	return w.client.Chat(ctx, messages, tools)
}

func (w *ClientWrapper) StreamChat(ctx context.Context, messages []Message, tools []ToolDefinition, onChunk func(string)) error {
	return w.client.StreamChat(ctx, messages, tools, onChunk)
}

func (w *ClientWrapper) Name() string {
	return w.provider
}

func (w *ClientWrapper) SwitchProvider(provider string) error {
	w.provider = normalizeProvider(provider)
	if w.baseURL == "" {
		w.baseURL = getDefaultBaseURL(provider)
	}
	return w.initClient()
}

func (w *ClientWrapper) SwitchModel(model string) error {
	w.model = model
	return w.initClient()
}

func (w *ClientWrapper) SetAPIKey(apiKey string) error {
	w.apiKey = apiKey
	return w.initClient()
}

func (w *ClientWrapper) SetTemperature(temp float64) {
	w.temperature = temp
}

func (w *ClientWrapper) SetBaseURL(url string) {
	w.baseURL = url
}

func serializeMessagesOpenAI(messages []Message) []map[string]any {
	result := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		if msg.contentBlocks != nil && len(msg.contentBlocks) > 0 {
			blocks := make([]map[string]any, 0, len(msg.contentBlocks))
			for _, b := range msg.contentBlocks {
				block := serializeContentBlockOpenAI(b)
				if block != nil {
					blocks = append(blocks, block)
				}
			}
			if len(blocks) == 0 {
				blocks = append(blocks, map[string]any{"type": "text", "text": msg.Content})
			}
			entry := map[string]any{
				"role":    msg.Role,
				"content": blocks,
			}
			if msg.Name != "" {
				entry["name"] = msg.Name
			}
			if msg.ToolCallID != "" {
				entry["tool_call_id"] = msg.ToolCallID
			}
			if len(msg.ToolCalls) > 0 {
				entry["tool_calls"] = msg.ToolCalls
			}
			result = append(result, entry)
		} else {
			entry := map[string]any{
				"role":    msg.Role,
				"content": msg.Content,
			}
			if msg.Name != "" {
				entry["name"] = msg.Name
			}
			if msg.ToolCallID != "" {
				entry["tool_call_id"] = msg.ToolCallID
			}
			if len(msg.ToolCalls) > 0 {
				entry["tool_calls"] = msg.ToolCalls
			}
			result = append(result, entry)
		}
	}
	return result
}

func serializeContentBlockOpenAI(b ContentBlock) map[string]any {
	switch b.Type {
	case ContentTypeText:
		return map[string]any{"type": "text", "text": b.Text}
	case ContentTypeImageURL:
		img := map[string]any{"url": b.ImageURL.URL}
		if b.ImageURL.Detail != "" {
			img["detail"] = b.ImageURL.Detail
		}
		return map[string]any{"type": "image_url", "image_url": img}
	case ContentTypeImage:
		dataURL := fmt.Sprintf("data:%s;base64,%s", b.Image.Source.MediaType, b.Image.Source.Data)
		img := map[string]any{"url": dataURL}
		return map[string]any{"type": "image_url", "image_url": img}
	default:
		return nil
	}
}

func serializeMessagesAnthropic(messages []Message) ([]map[string]any, string) {
	systemPrompt := ""
	result := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		if msg.Role == "system" {
			systemPrompt += msg.Content + "\n"
			continue
		}

		if msg.contentBlocks != nil && len(msg.contentBlocks) > 0 {
			blocks := make([]map[string]any, 0, len(msg.contentBlocks))
			for _, b := range msg.contentBlocks {
				block := serializeContentBlockAnthropic(b)
				if block != nil {
					blocks = append(blocks, block)
				}
			}
			if len(blocks) == 0 && msg.Content != "" {
				blocks = append(blocks, map[string]any{"type": "text", "text": msg.Content})
			}
			entry := map[string]any{
				"role":    msg.Role,
				"content": blocks,
			}
			result = append(result, entry)
		} else {
			entry := map[string]any{
				"role":    msg.Role,
				"content": msg.Content,
			}
			result = append(result, entry)
		}
	}
	return result, strings.TrimSpace(systemPrompt)
}

func serializeContentBlockAnthropic(b ContentBlock) map[string]any {
	switch b.Type {
	case ContentTypeText:
		return map[string]any{"type": "text", "text": b.Text}
	case ContentTypeImageURL:
		return map[string]any{"type": "image", "source": map[string]any{
			"type": "url",
			"url":  b.ImageURL.URL,
		}}
	case ContentTypeImage:
		return map[string]any{"type": "image", "source": map[string]any{
			"type":       "base64",
			"media_type": b.Image.Source.MediaType,
			"data":       b.Image.Source.Data,
		}}
	default:
		return nil
	}
}
