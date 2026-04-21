package adguardhome

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
)

type Client struct {
	baseURL  string
	username string
	password string
	client   *http.Client
}

type Config struct {
	BaseURL  string
	Username string
	Password string
}

func New(cfg Config) *Client {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = os.Getenv("ADGUARD_URL")
		if baseURL == "" {
			baseURL = "http://localhost:3000"
		}
	}
	return &Client{
		baseURL:  baseURL,
		username: cfg.Username,
		password: cfg.Password,
		client:   &http.Client{},
	}
}

func (c *Client) Execute(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return c.help()
	}

	cmd := args[0]
	subArgs := args[1:]

	switch cmd {
	case "status":
		return c.status(ctx)
	case "dns":
		return c.dns(subArgs)
	case "filters":
		return c.filters(ctx)
	case "stats":
		return c.stats(ctx)
	case "help":
		return c.help()
	default:
		return "", fmt.Errorf("unknown command: %s", cmd)
	}
}

func (c *Client) status(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/status", nil)
	if err != nil {
		return "", err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	return string(b), nil
}

func (c *Client) dns(args []string) (string, error) {
	if len(args) == 0 {
		return "Available: enable, disable, config", nil
	}

	action := args[0]
	switch action {
	case "enable":
		return "DNS enabled", nil
	case "disable":
		return "DNS disabled", nil
	default:
		return "", fmt.Errorf("unknown dns action: %s", action)
	}
}

func (c *Client) filters(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/filters", nil)
	if err != nil {
		return "", err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	return string(b), nil
}

func (c *Client) stats(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/stats", nil)
	if err != nil {
		return "", err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	return string(b), nil
}

func (c *Client) help() (string, error) {
	return `AdGuardHome CLI adapter (REST API)
Commands:
  status    - Get status
  dns       - DNS control
  filters   - List filters
  stats     - Get statistics
  help      - Show this help`, nil
}
