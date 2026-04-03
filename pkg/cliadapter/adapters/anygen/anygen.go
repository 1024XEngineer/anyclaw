package anygen

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
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
		apiKey = os.Getenv("ANYGEN_API_KEY")
	}
	return &Client{
		apiKey:     apiKey,
		baseURL:    "https://api.anygen.com/v1",
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
	case "docs":
		return c.docs(ctx, subArgs)
	case "slides":
		return c.slides(ctx, subArgs)
	case "website":
		return c.website(ctx, subArgs)
	case "help":
		return c.help()
	default:
		return "", fmt.Errorf("unknown command: %s", cmd)
	}
}

func (c *Client) docs(ctx context.Context, args []string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("docs requires <topic> <output.md>")
	}
	topic := args[0]
	output := args[1]

	body := map[string]string{"topic": topic, "type": "docs"}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/generate", bytes.NewReader(jsonBody))
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
	os.WriteFile(output, b, 0644)

	return fmt.Sprintf("Generated docs: %s", output), nil
}

func (c *Client) slides(ctx context.Context, args []string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("slides requires <topic> <output.pptx>")
	}
	topic := args[0]
	output := args[1]

	body := map[string]string{"topic": topic, "type": "slides"}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/generate", bytes.NewReader(jsonBody))
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
	os.WriteFile(output, b, 0644)

	return fmt.Sprintf("Generated slides: %s", output), nil
}

func (c *Client) website(ctx context.Context, args []string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("website requires <topic> <output-dir>")
	}
	topic := args[0]
	outputDir := args[1]

	body := map[string]string{"topic": topic, "type": "website"}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/generate", bytes.NewReader(jsonBody))
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
	os.MkdirAll(outputDir, 0755)
	os.WriteFile(outputDir+"/index.html", b, 0644)

	return fmt.Sprintf("Generated website: %s", outputDir), nil
}

func (c *Client) help() (string, error) {
	return `AnyGen CLI adapter
Commands:
  docs <topic> <output.md>      - Generate docs
  slides <topic> <output.pptx>   - Generate slides
  website <topic> <output-dir>  - Generate website
  help                          - Show this help
Note: Requires ANYGEN_API_KEY`, nil
}
