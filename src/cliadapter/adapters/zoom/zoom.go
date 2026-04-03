package zoom

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

type Client struct {
	apiKey    string
	apiSecret string
	baseURL   string
	token     string
	client    *http.Client
}

type Config struct {
	APIKey    string
	APISecret string
	AccountID string
	Token     string
}

func New(cfg Config) *Client {
	token := cfg.Token
	if token == "" {
		token = os.Getenv("ZOOM_TOKEN")
	}
	return &Client{
		apiKey:    cfg.APIKey,
		apiSecret: cfg.APISecret,
		baseURL:   "https://api.zoom.us/v2",
		token:     token,
		client:    &http.Client{},
	}
}

func (c *Client) Execute(ctx context.Context, args []string) (string, error) {
	if len(args) == 0 {
		return c.help()
	}

	cmd := args[0]
	subArgs := args[1:]

	switch cmd {
	case "meetings":
		return c.meetings(subArgs)
	case "list":
		return c.list(subArgs)
	case "create":
		if len(subArgs) < 1 {
			return "", fmt.Errorf("create requires <topic>")
		}
		return c.createMeeting(subArgs[0], subArgs[1:])
	case "join":
		return c.join(subArgs)
	case "help":
		return c.help()
	default:
		return "", fmt.Errorf("unknown command: %s", cmd)
	}
}

type Meeting struct {
	ID        int64  `json:"id"`
	Topic     string `json:"topic"`
	StartTime string `json:"start_time"`
	Duration  int    `json:"duration"`
	JoinURL   string `json:"join_url"`
}

func (c *Client) meetings(args []string) (string, error) {
	if len(args) == 0 {
		return c.listMeetings()
	}

	action := args[0]
	switch action {
	case "list":
		return c.listMeetings()
	case "create":
		if len(args) < 2 {
			return "", fmt.Errorf("create requires <topic> [duration]")
		}
		return c.createMeeting(args[1], args[2:])
	default:
		return "", fmt.Errorf("unknown meeting action: %s", action)
	}
}

func (c *Client) listMeetings() (string, error) {
	if c.token == "" {
		return "", fmt.Errorf("ZOOM_TOKEN not configured")
	}

	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/users/me/meetings", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	return string(b), nil
}

func (c *Client) createMeeting(topic string, args []string) (string, error) {
	if c.token == "" {
		return "", fmt.Errorf("ZOOM_TOKEN not configured")
	}

	duration := 60
	if len(args) > 0 {
		fmt.Sscanf(args[0], "%d", &duration)
	}

	body := map[string]any{
		"topic":    topic,
		"type":     2,
		"duration": duration,
	}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/users/me/meetings", strings.NewReader(string(jsonBody)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	return string(b), nil
}

func (c *Client) list(args []string) (string, error) {
	return c.listMeetings()
}

func (c *Client) join(args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("join requires <meeting_id>")
	}
	meetingID := args[0]
	return fmt.Sprintf("Join at: https://zoom.us/j/%s", meetingID), nil
}

func (c *Client) help() (string, error) {
	return `Zoom CLI adapter (REST API)
Commands:
  meetings list              - List upcoming meetings
  meetings create <topic>    - Create instant meeting
  list                       - List meetings (alias)
  join <meeting_id>          - Get join URL
  help                       - Show this help
Note: Requires ZOOM_TOKEN environment variable`, nil
}
