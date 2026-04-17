package novita

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

type Config struct {
	APIKey string
}

func New(cfg Config) *Client {
	apiKey := cfg.APIKey
	if apiKey == "" {
		apiKey = "novita"
	}
	return &Client{
		apiKey:     apiKey,
		baseURL:    "https://api.novita.ai/v1",
		httpClient: &http.Client{Timeout: 5 * time.Minute},
	}
}

func (c *Client) Execute(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return c.help()
	}

	cmd := args[0]
	subArgs := args[1:]

	switch cmd {
	case "chat":
		return c.chat(ctx, subArgs)
	case "models":
		return c.models(ctx)
	case "help":
		return c.help()
	default:
		return "", fmt.Errorf("unknown command: %s", cmd)
	}
}

type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Stream      bool      `json:"stream,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatResponse struct {
	ID      string   `json:"id"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

type Choice struct {
	Message Message `json:"message"`
	Index   int     `json:"index"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func (c *Client) chat(ctx context.Context, args []string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("chat requires <model> <prompt>")
	}
	model := args[0]
	prompt := strings.Join(args[1:], " ")

	body := ChatRequest{
		Model: model,
		Messages: []Message{
			{Role: "user", Content: prompt},
		},
		MaxTokens: 1024,
	}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)

	var result ChatResponse
	if err := json.Unmarshal(b, &result); err != nil {
		return string(b), err
	}

	if len(result.Choices) > 0 {
		return result.Choices[0].Message.Content, nil
	}
	return "", fmt.Errorf("no response")
}

func (c *Client) models(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/models", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	return string(b), nil
}

func (c *Client) help() (string, error) {
	return `Novita AI CLI adapter (OpenAI-compatible API)
Models: deepseek, glm, minimax, qwen, and more
Commands:
  chat <model> <prompt>  - Send chat request
  models                  - List available models
  help                    - Show this help
Note: Requires NOVITA_API_KEY`, nil
}
