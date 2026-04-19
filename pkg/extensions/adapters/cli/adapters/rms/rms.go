package rms

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
)

type Client struct {
	apiToken string
	baseURL  string
	client   *http.Client
}

type Config struct {
	APIToken string
}

func New(cfg Config) *Client {
	token := cfg.APIToken
	if token == "" {
		token = os.Getenv("RMS_API_TOKEN")
	}
	return &Client{
		apiToken: token,
		baseURL:  "https://rms.teltonika-networks.com/api/v1",
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
	case "devices":
		return c.devices(ctx)
	case "info":
		return c.info(subArgs)
	case "status":
		return c.status(subArgs)
	case "help":
		return c.help()
	default:
		return "", fmt.Errorf("unknown command: %s", cmd)
	}
}

type Device struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	IP       string `json:"ip"`
	Model    string `json:"model"`
	Firmware string `json:"firmware"`
}

func (c *Client) devices(ctx context.Context) (string, error) {
	if c.apiToken == "" {
		return "", fmt.Errorf("RMS_API_TOKEN not configured")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/devices", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Token "+c.apiToken)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	return string(b), nil
}

func (c *Client) info(args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("info requires <device_id>")
	}
	deviceID := args[0]

	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/devices/"+deviceID, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Token "+c.apiToken)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	return string(b), nil
}

func (c *Client) status(args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("status requires <device_id>")
	}
	deviceID := args[0]

	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/devices/"+deviceID+"/status", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Token "+c.apiToken)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	return string(b), nil
}

func (c *Client) help() (string, error) {
	return `Teltonika RMS CLI adapter
Commands:
  devices                - List all devices
  info <device_id>       - Get device info
  status <device_id>    - Get device status
  help                  - Show this help
Note: Requires RMS_API_TOKEN`, nil
}
