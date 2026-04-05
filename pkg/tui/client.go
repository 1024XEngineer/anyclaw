package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// GatewayClient communicates with the AnyClaw gateway API.
type GatewayClient struct {
	BaseURL string
	Token   string
	Client  *http.Client
}

// NewGatewayClient creates a new gateway client.
func NewGatewayClient(baseURL, token string) *GatewayClient {
	if baseURL == "" {
		baseURL = "http://127.0.0.1:18789"
	}
	return &GatewayClient{
		BaseURL: baseURL,
		Token:   token,
		Client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *GatewayClient) do(method, path string, body any) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, c.BaseURL+path, reqBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(data))
	}

	return data, nil
}

// Status represents the gateway status response.
type Status struct {
	OK        string `json:"ok"`
	Status    string `json:"status"`
	Version   string `json:"version"`
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	Address   string `json:"address"`
	StartedAt string `json:"started_at"`
	Sessions  int    `json:"sessions"`
	Events    int    `json:"events"`
	Skills    int    `json:"skills"`
	Tools     int    `json:"tools"`
}

// GetStatus fetches the gateway status.
func (c *GatewayClient) GetStatus() (*Status, error) {
	data, err := c.do("GET", "/status", nil)
	if err != nil {
		return nil, err
	}
	var status Status
	if err := json.Unmarshal(data, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

// ChatRequest is a request to send a message.
type ChatRequest struct {
	Message   string `json:"message"`
	SessionID string `json:"session_id,omitempty"`
	Agent     string `json:"agent,omitempty"`
}

// ChatResponse is the response from sending a message.
type ChatResponse struct {
	SessionID string `json:"session_id"`
	Response  string `json:"response"`
	Agent     string `json:"agent,omitempty"`
}

// SendMessage sends a message to the agent.
func (c *GatewayClient) SendMessage(req ChatRequest) (*ChatResponse, error) {
	data, err := c.do("POST", "/chat", req)
	if err != nil {
		return nil, err
	}
	var resp ChatResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Session represents a chat session.
type Session struct {
	ID            string    `json:"id"`
	Title         string    `json:"title"`
	Agent         string    `json:"agent"`
	MessageCount  int       `json:"message_count"`
	LastUserText  string    `json:"last_user_text"`
	LastAssistant string    `json:"last_assistant_text"`
	UpdatedAt     time.Time `json:"updated_at"`
	CreatedAt     time.Time `json:"created_at"`
}

// ListSessions fetches all sessions.
func (c *GatewayClient) ListSessions() ([]Session, error) {
	data, err := c.do("GET", "/sessions", nil)
	if err != nil {
		return nil, err
	}
	var sessions []Session
	if err := json.Unmarshal(data, &sessions); err != nil {
		return nil, err
	}
	return sessions, nil
}

// GetSession fetches a single session with its messages.
func (c *GatewayClient) GetSession(id string) (map[string]any, error) {
	data, err := c.do("GET", "/sessions/"+id, nil)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// CreateSession creates a new session.
func (c *GatewayClient) CreateSession(title, agent string) (string, error) {
	data, err := c.do("POST", "/sessions", map[string]string{
		"title": title,
		"agent": agent,
	})
	if err != nil {
		return "", err
	}
	var result map[string]string
	if err := json.Unmarshal(data, &result); err != nil {
		return "", err
	}
	return result["id"], nil
}

// DeleteSession deletes a session.
func (c *GatewayClient) DeleteSession(id string) error {
	_, err := c.do("DELETE", "/sessions/"+id, nil)
	return err
}

// ChannelStatus represents a channel's status.
type ChannelStatus struct {
	Name         string    `json:"name"`
	Enabled      bool      `json:"enabled"`
	Running      bool      `json:"running"`
	Healthy      bool      `json:"healthy"`
	LastError    string    `json:"last_error"`
	LastActivity time.Time `json:"last_activity"`
}

// ListChannels fetches channel statuses.
func (c *GatewayClient) ListChannels() ([]ChannelStatus, error) {
	data, err := c.do("GET", "/channels", nil)
	if err != nil {
		return nil, err
	}
	var channels []ChannelStatus
	if err := json.Unmarshal(data, &channels); err != nil {
		return nil, err
	}
	return channels, nil
}

// AgentInfo represents an agent profile.
type AgentInfo struct {
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	Persona         string   `json:"persona,omitempty"`
	Domain          string   `json:"domain,omitempty"`
	Expertise       []string `json:"expertise,omitempty"`
	Skills          []string `json:"skills,omitempty"`
	PermissionLevel string   `json:"permission_level,omitempty"`
}

// ListAgents fetches available agents.
func (c *GatewayClient) ListAgents() ([]AgentInfo, error) {
	data, err := c.do("GET", "/agents", nil)
	if err != nil {
		return nil, err
	}
	var agents []AgentInfo
	if err := json.Unmarshal(data, &agents); err != nil {
		return nil, err
	}
	return agents, nil
}

// MemoryEntry represents a memory entry.
type MemoryEntry struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// GetMemory fetches agent memory entries.
func (c *GatewayClient) GetMemory(limit int) ([]MemoryEntry, error) {
	path := "/memory"
	if limit > 0 {
		path += fmt.Sprintf("?limit=%d", limit)
	}
	data, err := c.do("GET", path, nil)
	if err != nil {
		return nil, err
	}
	var entries []MemoryEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// SkillInfo represents a loaded skill.
type SkillInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}

// ListSkills fetches loaded skills.
func (c *GatewayClient) ListSkills() ([]SkillInfo, error) {
	data, err := c.do("GET", "/skills", nil)
	if err != nil {
		return nil, err
	}
	var skills []SkillInfo
	if err := json.Unmarshal(data, &skills); err != nil {
		return nil, err
	}
	return skills, nil
}

// ControlPlaneSnapshot is the full control plane data.
type ControlPlaneSnapshot struct {
	Status       Status          `json:"status"`
	Channels     []ChannelStatus `json:"channels"`
	Runtimes     []any           `json:"runtimes"`
	RecentEvents []any           `json:"recent_events"`
	RecentTools  []any           `json:"recent_tools"`
	RecentJobs   []any           `json:"recent_jobs"`
	UpdatedAt    string          `json:"updated_at"`
}

// GetControlPlane fetches the full control plane snapshot.
func (c *GatewayClient) GetControlPlane() (*ControlPlaneSnapshot, error) {
	data, err := c.do("GET", "/control-plane", nil)
	if err != nil {
		return nil, err
	}
	var snapshot ControlPlaneSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, err
	}
	return &snapshot, nil
}

// CheckHealth checks if the gateway is reachable.
func (c *GatewayClient) CheckHealth() error {
	_, err := c.do("GET", "/healthz", nil)
	return err
}

// StreamResponse simulates streaming by returning the response in chunks.
// In a real implementation, this would use SSE or WebSocket.
func (c *GatewayClient) SendMessageStream(req ChatRequest, onChunk func(chunk string)) (*ChatResponse, error) {
	resp, err := c.SendMessage(req)
	if err != nil {
		return nil, err
	}

	if resp.Response != "" && onChunk != nil {
		words := strings.Fields(resp.Response)
		for i := 0; i < len(words); i += 3 {
			end := i + 3
			if end > len(words) {
				end = len(words)
			}
			onChunk(strings.Join(words[i:end], " ") + " ")
			time.Sleep(30 * time.Millisecond)
		}
	}

	return resp, nil
}
