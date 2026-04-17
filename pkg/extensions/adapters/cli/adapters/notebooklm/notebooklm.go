package notebooklm

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
		apiKey = os.Getenv("NOTEBOOKLM_API_KEY")
	}
	return &Client{
		apiKey:     apiKey,
		baseURL:    "https://notebooklm.google.com/api/v1",
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
	case "notebook":
		return c.notebook(ctx, subArgs)
	case "source":
		return c.source(ctx, subArgs)
	case "chat":
		return c.chat(ctx, subArgs)
	case "download":
		return c.download(ctx, subArgs)
	case "help":
		return c.help()
	default:
		return "", fmt.Errorf("unknown command: %s", cmd)
	}
}

func (c *Client) notebook(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return "Available: list, create, delete", nil
	}

	action := args[0]
	switch action {
	case "list":
		return c.listNotebooks(ctx)
	case "create":
		if len(args) < 2 {
			return "", fmt.Errorf("create requires <name>")
		}
		return c.createNotebook(ctx, args[1])
	default:
		return "", fmt.Errorf("unknown notebook action: %s", action)
	}
}

func (c *Client) listNotebooks(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/notebooks", nil)
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

func (c *Client) createNotebook(ctx context.Context, name string) (string, error) {
	body := map[string]string{"name": name}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/notebooks", bytes.NewReader(jsonBody))
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
	return string(b), nil
}

func (c *Client) source(ctx context.Context, args []string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("source add <notebook_id> <url/file>")
	}
	notebookID := args[0]
	source := args[1]

	body := map[string]string{"source": source}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/notebooks/"+notebookID+"/sources", bytes.NewReader(jsonBody))
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

func (c *Client) chat(ctx context.Context, args []string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("chat <notebook_id> <message>")
	}
	notebookID := args[0]
	message := args[1]

	body := map[string]string{"message": message}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/notebooks/"+notebookID+"/chat", bytes.NewReader(jsonBody))
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

func (c *Client) download(ctx context.Context, args []string) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("download <notebook_id> <output.zip>")
	}
	notebookID := args[0]
	output := args[1]

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/notebooks/"+notebookID+"/export", nil)
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

	return fmt.Sprintf("Downloaded to %s", output), nil
}

func (c *Client) help() (string, error) {
	return `NotebookLM CLI adapter
Commands:
  notebook list                           - List notebooks
  notebook create <name>                 - Create notebook
  source add <notebook_id> <url>        - Add source
  chat <notebook_id> <message>          - Send chat
  download <notebook_id> <output.zip>   - Export
  help                                    - Show this help
Note: Requires NOTEBOOKLM_API_KEY`, nil
}
