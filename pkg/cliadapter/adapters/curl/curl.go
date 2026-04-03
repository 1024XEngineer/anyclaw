package curl

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

type Client struct {
	curlPath string
	timeout  time.Duration
}

type Config struct {
	CurlPath string
	Timeout  time.Duration
}

func NewClient(cfg Config) *Client {
	path := cfg.CurlPath
	if path == "" {
		path = "curl"
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &Client{
		curlPath: path,
		timeout:  timeout,
	}
}

type Response struct {
	Status     string            `json:"status"`
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
	Time       int64             `json:"time_ms"`
}

func (c *Client) Run(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return "Usage: curl <url> [options]", nil
	}

	url := args[0]
	isJSON := false

	for _, arg := range args {
		if arg == "-H" {
			isJSON = true
		}
	}

	if isJSON || strings.HasPrefix(url, "http") {
		return c.request(ctx, args)
	}

	return c.run(ctx, args)
}

func (c *Client) request(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("URL required")
	}

	url := args[0]
	method := "GET"
	headers := make(map[string]string)
	data := ""

	for i := 1; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-X":
			if i+1 < len(args) {
				method = args[i+1]
				i++
			}
		case "-H":
			if i+1 < len(args) {
				header := args[i+1]
				if idx := strings.Index(header, ":"); idx > 0 {
					key := strings.TrimSpace(header[:idx])
					val := strings.TrimSpace(header[idx+1:])
					headers[key] = val
				}
				i++
			}
		case "-d", "--data":
			if i+1 < len(args) {
				data = args[i+1]
				if method == "GET" {
					method = "POST"
				}
				i++
			}
		case "-o", "--output":
			if i+1 < len(args) {
				i++
			}
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, url, strings.NewReader(data))
	if err != nil {
		return "", err
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if data != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	client := &http.Client{Timeout: c.timeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	result := fmt.Sprintf("HTTP %d %s\n\n%s", resp.StatusCode, resp.Status, string(body))
	return result, nil
}

func (c *Client) get(ctx context.Context, url string) (string, error) {
	return c.request(ctx, []string{url})
}

func (c *Client) post(ctx context.Context, url, data string) (string, error) {
	return c.request(ctx, []string{url, "-X", "POST", "-d", data})
}

func (c *Client) put(ctx context.Context, url, data string) (string, error) {
	return c.request(ctx, []string{url, "-X", "PUT", "-d", data})
}

func (c *Client) delete(ctx context.Context, url string) (string, error) {
	return c.request(ctx, []string{url, "-X", "DELETE"})
}

func (c *Client) head(ctx context.Context, url string) (string, error) {
	return c.request(ctx, []string{url, "-I"})
}

func (c *Client) run(ctx context.Context, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, c.curlPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), err
	}
	return strings.TrimSpace(string(output)), nil
}

func (c *Client) IsInstalled(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, c.curlPath, "--version")
	return cmd.Run() == nil
}
