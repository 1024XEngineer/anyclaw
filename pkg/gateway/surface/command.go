package surface

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	gatewaycommands "github.com/1024XEngineer/anyclaw/pkg/gateway/commands"
	gatewaygovernance "github.com/1024XEngineer/anyclaw/pkg/gateway/governance"
	chatintake "github.com/1024XEngineer/anyclaw/pkg/gateway/intake/chat"
)

// ChatIngressInput is the protocol-surface view of a chat.send business message.
type ChatIngressInput struct {
	Source             string
	Request            gatewaycommands.ChatSendRequest
	RequestedAgentName string
	Meta               map[string]string
}

// DecodeHTTPChatSend parses the HTTP /chat request at the protocol surface.
func (Service) DecodeHTTPChatSend(r *http.Request) (gatewaycommands.ChatSendRequest, gatewaycommands.Request, error) {
	if r == nil {
		return gatewaycommands.ChatSendRequest{}, gatewaycommands.Request{}, fmt.Errorf("request is required")
	}
	var raw struct {
		Message   string `json:"message"`
		SessionID string `json:"session_id"`
		Title     string `json:"title"`
		Agent     string `json:"agent"`
		Assistant string `json:"assistant"`
	}
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		return gatewaycommands.ChatSendRequest{}, gatewaycommands.Request{}, fmt.Errorf("invalid request")
	}
	req := gatewaycommands.ChatSendRequest{
		Message:     strings.TrimSpace(raw.Message),
		SessionID:   strings.TrimSpace(raw.SessionID),
		Title:       strings.TrimSpace(raw.Title),
		Agent:       strings.TrimSpace(raw.Agent),
		Assistant:   strings.TrimSpace(raw.Assistant),
		OrgID:       strings.TrimSpace(r.URL.Query().Get("org")),
		ProjectID:   strings.TrimSpace(r.URL.Query().Get("project")),
		WorkspaceID: strings.TrimSpace(r.URL.Query().Get("workspace")),
	}
	return req, gatewaycommands.NewChatSendCommandRequest(req), nil
}

// DecodeWSChatSend parses a WS chat.send frame at the protocol surface.
func (Service) DecodeWSChatSend(params map[string]any) (gatewaycommands.ChatSendRequest, gatewaycommands.Request) {
	req := gatewaycommands.ChatSendRequest{
		Message:     mapString(params, "message"),
		SessionID:   mapString(params, "session_id"),
		Title:       mapString(params, "title"),
		Agent:       mapString(params, "agent"),
		Assistant:   mapString(params, "assistant"),
		OrgID:       mapString(params, "org"),
		ProjectID:   mapString(params, "project"),
		WorkspaceID: mapString(params, "workspace"),
	}
	return req, gatewaycommands.NewChatSendCommandRequest(req)
}

// DecodeHTTPV2Chat parses the HTTP /v2/chat request at the protocol surface.
func (Service) DecodeHTTPV2Chat(r *http.Request) (chatintake.ChatRequest, gatewaycommands.Request, error) {
	if r == nil {
		return chatintake.ChatRequest{}, gatewaycommands.Request{}, fmt.Errorf("request is required")
	}
	var req chatintake.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return chatintake.ChatRequest{}, gatewaycommands.Request{}, fmt.Errorf("invalid request")
	}
	req.AgentName = strings.TrimSpace(req.AgentName)
	req.Message = strings.TrimSpace(req.Message)
	req.SessionID = strings.TrimSpace(req.SessionID)
	return req, gatewaycommands.NewV2ChatCommandRequest(req), nil
}

// DecodeHTTPV2TaskCreate parses the HTTP /v2/tasks POST request at the protocol surface.
func (Service) DecodeHTTPV2TaskCreate(r *http.Request) (gatewaycommands.V2TaskCreateRequest, gatewaycommands.Request, error) {
	if r == nil {
		return gatewaycommands.V2TaskCreateRequest{}, gatewaycommands.Request{}, fmt.Errorf("request is required")
	}
	var raw struct {
		Title          string   `json:"title"`
		Input          string   `json:"input"`
		Mode           string   `json:"mode"`
		Assistant      string   `json:"assistant"`
		SessionID      string   `json:"session_id"`
		SelectedAgent  string   `json:"selected_agent"`
		SelectedAgents []string `json:"selected_agents"`
		Sync           bool     `json:"sync"`
	}
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		return gatewaycommands.V2TaskCreateRequest{}, gatewaycommands.Request{}, fmt.Errorf("invalid request")
	}
	req := gatewaycommands.V2TaskCreateRequest{
		Title:          strings.TrimSpace(raw.Title),
		Input:          strings.TrimSpace(raw.Input),
		Mode:           strings.TrimSpace(strings.ToLower(raw.Mode)),
		Assistant:      strings.TrimSpace(raw.Assistant),
		SessionID:      strings.TrimSpace(raw.SessionID),
		SelectedAgent:  strings.TrimSpace(raw.SelectedAgent),
		SelectedAgents: append([]string(nil), raw.SelectedAgents...),
		Sync:           raw.Sync,
	}
	return req, gatewaycommands.NewV2TaskCreateCommandRequest(req), nil
}

// BuildChatRawRequest translates a chat.send command into the gateway raw request contract.
func (Service) BuildChatRawRequest(input ChatIngressInput) gatewaygovernance.RawRequest {
	req := input.Request
	meta := cloneStringMap(input.Meta)
	source := strings.TrimSpace(input.Source)
	if source == "" {
		source = "api"
	}
	putIfPresent(meta, "transport", source)
	putIfPresent(meta, "title_hint", req.Title)
	putIfPresent(meta, "org", req.OrgID)
	putIfPresent(meta, "project", req.ProjectID)
	putIfPresent(meta, "workspace", req.WorkspaceID)

	return gatewaygovernance.RawRequest{
		SourceType:         source,
		EntryPoint:         "chat",
		ChannelID:          source,
		SessionID:          strings.TrimSpace(req.SessionID),
		ConversationID:     strings.TrimSpace(req.SessionID),
		Message:            req.Message,
		TitleHint:          strings.TrimSpace(req.Title),
		RequestedAgentName: strings.TrimSpace(input.RequestedAgentName),
		RequestedSessionID: strings.TrimSpace(req.SessionID),
		Metadata:           meta,
	}
}

func mapString(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	value, _ := values[key].(string)
	return strings.TrimSpace(value)
}

func putIfPresent(values map[string]string, key string, value string) {
	if values == nil {
		return
	}
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		values[key] = trimmed
	}
}
