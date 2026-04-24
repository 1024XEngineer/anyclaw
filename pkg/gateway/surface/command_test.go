package surface

import (
	"net/http/httptest"
	"strings"
	"testing"

	gatewaycommands "github.com/1024XEngineer/anyclaw/pkg/gateway/commands"
)

func TestDecodeHTTPChatSendBuildsCommandRequest(t *testing.T) {
	r := httptest.NewRequest("POST", "/chat?org=org-1&project=project-1&workspace=workspace-1", strings.NewReader(`{
		"message": " hello ",
		"session_id": " session-1 ",
		"title": " title ",
		"agent": " helper "
	}`))

	req, commandReq, err := Service{}.DecodeHTTPChatSend(r)
	if err != nil {
		t.Fatalf("DecodeHTTPChatSend returned error: %v", err)
	}
	if req.Message != "hello" || req.SessionID != "session-1" || req.Agent != "helper" {
		t.Fatalf("unexpected decoded chat request: %+v", req)
	}
	if req.OrgID != "org-1" || req.ProjectID != "project-1" || req.WorkspaceID != "workspace-1" {
		t.Fatalf("unexpected resource hints: %+v", req)
	}
	if commandReq.Method != "chat.send" || commandReq.ResourceID != "session-1" {
		t.Fatalf("unexpected command request: %+v", commandReq)
	}
	if commandReq.Params["message"] != "hello" || commandReq.Params["has_agent"] != "true" {
		t.Fatalf("unexpected command params: %+v", commandReq.Params)
	}
}

func TestDecodeWSChatSendBuildsCommandRequest(t *testing.T) {
	req, commandReq := Service{}.DecodeWSChatSend(map[string]any{
		"message":    " hello ",
		"session_id": " session-1 ",
		"assistant":  " main ",
	})

	if req.Message != "hello" || req.SessionID != "session-1" || req.Assistant != "main" {
		t.Fatalf("unexpected decoded ws chat request: %+v", req)
	}
	if commandReq.Method != "chat.send" || commandReq.Params["has_assistant"] != "true" {
		t.Fatalf("unexpected command request: %+v", commandReq)
	}
}

func TestDecodeHTTPV2TaskCreateBuildsCommandRequest(t *testing.T) {
	r := httptest.NewRequest("POST", "/v2/tasks", strings.NewReader(`{
		"title": " T ",
		"input": " run ",
		"mode": "MAIN",
		"assistant": " helper ",
		"session_id": " session-1 ",
		"selected_agent": " agent-a ",
		"sync": true
	}`))

	req, commandReq, err := Service{}.DecodeHTTPV2TaskCreate(r)
	if err != nil {
		t.Fatalf("DecodeHTTPV2TaskCreate returned error: %v", err)
	}
	if req.Title != "T" || req.Input != "run" || req.Mode != "main" || !req.Sync {
		t.Fatalf("unexpected decoded task request: %+v", req)
	}
	if commandReq.Method != "tasks.v2.create" || commandReq.ResourceID != "session-1" {
		t.Fatalf("unexpected command request: %+v", commandReq)
	}
	if commandReq.Params["input"] != "run" || commandReq.Params["sync"] != "true" {
		t.Fatalf("unexpected command params: %+v", commandReq.Params)
	}
}

func TestBuildChatRawRequest(t *testing.T) {
	raw := Service{}.BuildChatRawRequest(ChatIngressInput{
		Source: "ws",
		Request: gatewayChatRequestForTest(
			"hello",
			"session-1",
			"Title",
			"org-1",
			"project-1",
			"workspace-1",
		),
		RequestedAgentName: "main",
	})

	if raw.SourceType != "ws" || raw.EntryPoint != "chat" || raw.ChannelID != "ws" {
		t.Fatalf("unexpected source fields: %+v", raw)
	}
	if raw.Message != "hello" || raw.TitleHint != "Title" {
		t.Fatalf("unexpected content fields: %+v", raw)
	}
	if raw.RequestedSessionID != "session-1" || raw.RequestedAgentName != "main" {
		t.Fatalf("unexpected route hints: %+v", raw)
	}
	if raw.Metadata["org"] != "org-1" || raw.Metadata["project"] != "project-1" || raw.Metadata["workspace"] != "workspace-1" {
		t.Fatalf("unexpected resource metadata: %+v", raw.Metadata)
	}
}

func gatewayChatRequestForTest(message string, sessionID string, title string, org string, project string, workspace string) gatewaycommands.ChatSendRequest {
	return gatewaycommands.ChatSendRequest{
		Message:     message,
		SessionID:   sessionID,
		Title:       title,
		OrgID:       org,
		ProjectID:   project,
		WorkspaceID: workspace,
	}
}
