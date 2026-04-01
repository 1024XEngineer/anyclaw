package a2ui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

const ProtocolVersion = "a2ui.v1"

type MessageType string

const (
	MsgTypeRequest    MessageType = "request"
	MsgTypeResponse   MessageType = "response"
	MsgTypeEvent      MessageType = "event"
	MsgTypeError      MessageType = "error"
	MsgTypeToolCall   MessageType = "tool_call"
	MsgTypeToolResult MessageType = "tool_result"
	MsgTypeApproval   MessageType = "approval"
	MsgTypeCanvas     MessageType = "canvas"
	MsgTypeStream     MessageType = "stream"
)

type EventType string

const (
	EventThinking     EventType = "thinking"
	EventToolStart    EventType = "tool_start"
	EventToolEnd      EventType = "tool_end"
	EventMessage      EventType = "message"
	EventComplete     EventType = "complete"
	EventError        EventType = "error"
	EventApproval     EventType = "approval_required"
	EventCanvasUpdate EventType = "canvas_update"
	EventProgress     EventType = "progress"
)

type Request struct {
	ID        string          `json:"id"`
	Type      MessageType     `json:"type"`
	Method    string          `json:"method,omitempty"`
	Params    json.RawMessage `json:"params,omitempty"`
	Timestamp int64           `json:"timestamp"`
}

type Response struct {
	ID        string      `json:"id"`
	Type      MessageType `json:"type"`
	Result    interface{} `json:"result,omitempty"`
	Error     *Error      `json:"error,omitempty"`
	Timestamp int64       `json:"timestamp"`
}

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ToolCall struct {
	ID       string                 `json:"id"`
	Name     string                 `json:"name"`
	Input    map[string]interface{} `json:"input"`
	Approved bool                   `json:"approved,omitempty"`
}

type ToolResult struct {
	ID       string `json:"id"`
	ToolName string `json:"tool_name"`
	Result   string `json:"result,omitempty"`
	Error    string `json:"error,omitempty"`
	Approved bool   `json:"approved,omitempty"`
}

type ApprovalRequest struct {
	ID        string                 `json:"id"`
	ToolName  string                 `json:"tool_name"`
	Input     map[string]interface{} `json:"input"`
	Reason    string                 `json:"reason,omitempty"`
	RiskLevel string                 `json:"risk_level,omitempty"`
	Timestamp int64                  `json:"timestamp"`
}

type CanvasUpdate struct {
	Content    string     `json:"content,omitempty"`
	Language   string     `json:"language,omitempty"`
	Operations []CanvasOp `json:"operations,omitempty"`
}

type CanvasOp struct {
	Type    string `json:"type"` // insert, delete, replace
	Start   int    `json:"start"`
	End     int    `json:"end"`
	Content string `json:"content,omitempty"`
}

type StreamEvent struct {
	Type      EventType   `json:"type"`
	Data      interface{} `json:"data,omitempty"`
	Timestamp int64       `json:"timestamp"`
}

type Session struct {
	ID        string
	UserID    string
	CreatedAt time.Time
	Context   map[string]interface{}
	History   []Message
	Mu        sync.RWMutex
}

type Message struct {
	Role     string    `json:"role"` // user, assistant, system, tool
	Content  string    `json:"content"`
	ToolCall *ToolCall `json:"tool_call,omitempty"`
}

type ProtocolHandler struct {
	mu         sync.RWMutex
	sessions   map[string]*Session
	toolRunner ToolRunner
	approver   ApprovalProvider
	canvas     CanvasStore
	EventCB    func(sessionID string, eventType string, data interface{})
}

func (p *ProtocolHandler) SendToSession(sessionID string, eventType string, data interface{}) {
	if p.EventCB != nil {
		p.EventCB(sessionID, eventType, data)
	}
}

type ToolRunner interface {
	Run(ctx context.Context, tool string, input map[string]interface{}) (string, error)
}

type ApprovalProvider interface {
	Request(ctx context.Context, req *ApprovalRequest) (bool, string)
	OnApproval(ctx context.Context, id string, approved bool, comment string) error
}

type CanvasStore interface {
	Get(sessionID string) (string, bool)
	Set(sessionID, content string)
	Update(sessionID string, ops []CanvasOp) error
}

func NewProtocolHandler() *ProtocolHandler {
	return &ProtocolHandler{
		sessions: make(map[string]*Session),
	}
}

func (p *ProtocolHandler) SetToolRunner(runner ToolRunner) {
	p.toolRunner = runner
}

func (p *ProtocolHandler) SetApprovalProvider(provider ApprovalProvider) {
	p.approver = provider
}

func (p *ProtocolHandler) SetCanvasStore(store CanvasStore) {
	p.canvas = store
}

func (p *ProtocolHandler) CreateSession(userID string) *Session {
	p.mu.Lock()
	defer p.mu.Unlock()

	session := &Session{
		ID:        generateID("sess"),
		UserID:    userID,
		CreatedAt: time.Now(),
		Context:   make(map[string]interface{}),
		History:   make([]Message, 0),
	}
	p.sessions[session.ID] = session
	return session
}

func (p *ProtocolHandler) GetSession(sessionID string) (*Session, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	s, ok := p.sessions[sessionID]
	return s, ok
}

func (p *ProtocolHandler) DeleteSession(sessionID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.sessions[sessionID]; !ok {
		return fmt.Errorf("session not found")
	}
	delete(p.sessions, sessionID)
	return nil
}

func (p *ProtocolHandler) HandleRequest(ctx context.Context, req *Request) (*Response, error) {
	switch req.Method {
	case "chat":
		return p.handleChat(ctx, req)
	case "stream":
		return p.handleStream(ctx, req)
	case "tool":
		return p.handleTool(ctx, req)
	case "canvas":
		return p.handleCanvas(ctx, req)
	case "approval":
		return p.handleApproval(ctx, req)
	case "session":
		return p.handleSession(ctx, req)
	default:
		return &Response{
			ID:    req.ID,
			Type:  MsgTypeError,
			Error: &Error{Code: "unknown_method", Message: fmt.Sprintf("unknown method: %s", req.Method)},
		}, nil
	}
}

func (p *ProtocolHandler) handleChat(ctx context.Context, req *Request) (*Response, error) {
	var params struct {
		Message   string `json:"message"`
		SessionID string `json:"session_id,omitempty"`
		Stream    bool   `json:"stream,omitempty"`
	}

	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errorResponse(req.ID, "invalid_params", err.Error()), nil
	}

	session := p.GetOrCreateSession(params.SessionID)

	session.Mu.Lock()
	session.History = append(session.History, Message{
		Role:    "user",
		Content: params.Message,
	})
	session.Mu.Unlock()

	if p.toolRunner == nil {
		return errorResponse(req.ID, "no_tool_runner", "tool runner not configured"), nil
	}

	var toolResults []string
	toolCalls := extractToolCalls(params.Message)

	for _, tc := range toolCalls {
		if p.approver != nil {
			approvalReq := &ApprovalRequest{
				ID:        generateID("approval"),
				ToolName:  tc.Name,
				Input:     tc.Input,
				Timestamp: time.Now().Unix(),
			}

			approved, comment := p.approver.Request(ctx, approvalReq)
			if !approved {
				tc.Approved = false
				session.History = append(session.History, Message{
					Role:    "tool",
					Content: fmt.Sprintf("Tool %s denied: %s", tc.Name, comment),
				})
				continue
			}
			tc.Approved = true
		}

		result, err := p.toolRunner.Run(ctx, tc.Name, tc.Input)
		if err != nil {
			session.History = append(session.History, Message{
				Role:    "tool",
				Content: fmt.Sprintf("Error: %v", err),
			})
			continue
		}
		toolResults = append(toolResults, result)
		session.History = append(session.History, Message{
			Role:    "tool",
			Content: result,
		})
	}

	response := strings.Join(toolResults, "\n\n")
	session.History = append(session.History, Message{
		Role:    "assistant",
		Content: response,
	})

	return &Response{
		ID:   req.ID,
		Type: MsgTypeResponse,
		Result: map[string]interface{}{
			"response":   response,
			"session_id": session.ID,
			"tools_used": len(toolCalls),
		},
	}, nil
}

func (p *ProtocolHandler) handleTool(ctx context.Context, req *Request) (*Response, error) {
	var params struct {
		Tool  string                 `json:"tool"`
		Input map[string]interface{} `json:"input"`
	}

	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errorResponse(req.ID, "invalid_params", err.Error()), nil
	}

	if p.toolRunner == nil {
		return errorResponse(req.ID, "no_tool_runner", "tool runner not configured"), nil
	}

	result, err := p.toolRunner.Run(ctx, params.Tool, params.Input)
	if err != nil {
		return errorResponse(req.ID, "tool_error", err.Error()), nil
	}

	return &Response{
		ID:   req.ID,
		Type: MsgTypeResponse,
		Result: map[string]interface{}{
			"result": result,
		},
	}, nil
}

func (p *ProtocolHandler) handleCanvas(ctx context.Context, req *Request) (*Response, error) {
	var params struct {
		SessionID string     `json:"session_id"`
		Operation string     `json:"operation"` // get, set, update
		Content   string     `json:"content,omitempty"`
		Ops       []CanvasOp `json:"ops,omitempty"`
	}

	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errorResponse(req.ID, "invalid_params", err.Error()), nil
	}

	if p.canvas == nil {
		return errorResponse(req.ID, "no_canvas", "canvas not configured"), nil
	}

	switch params.Operation {
	case "get":
		content, _ := p.canvas.Get(params.SessionID)
		return &Response{
			ID:   req.ID,
			Type: MsgTypeResponse,
			Result: map[string]interface{}{
				"content": content,
			},
		}, nil

	case "set":
		p.canvas.Set(params.SessionID, params.Content)
		return &Response{
			ID:   req.ID,
			Type: MsgTypeResponse,
			Result: map[string]interface{}{
				"status": "saved",
			},
		}, nil

	case "update":
		if err := p.canvas.Update(params.SessionID, params.Ops); err != nil {
			return errorResponse(req.ID, "update_error", err.Error()), nil
		}
		content, _ := p.canvas.Get(params.SessionID)
		return &Response{
			ID:   req.ID,
			Type: MsgTypeResponse,
			Result: map[string]interface{}{
				"content": content,
			},
		}, nil

	default:
		return errorResponse(req.ID, "invalid_operation", "unknown operation"), nil
	}
}

func (p *ProtocolHandler) handleApproval(ctx context.Context, req *Request) (*Response, error) {
	var params struct {
		ApprovalID string `json:"approval_id"`
		Approved   bool   `json:"approved"`
		Comment    string `json:"comment,omitempty"`
	}

	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errorResponse(req.ID, "invalid_params", err.Error()), nil
	}

	if p.approver == nil {
		return errorResponse(req.ID, "no_approver", "approval not configured"), nil
	}

	if err := p.approver.OnApproval(ctx, params.ApprovalID, params.Approved, params.Comment); err != nil {
		return errorResponse(req.ID, "approval_error", err.Error()), nil
	}

	return &Response{
		ID:   req.ID,
		Type: MsgTypeResponse,
		Result: map[string]interface{}{
			"status": "processed",
		},
	}, nil
}

func (p *ProtocolHandler) handleSession(ctx context.Context, req *Request) (*Response, error) {
	var params struct {
		Operation string `json:"operation"` // create, get, delete, history
		SessionID string `json:"session_id,omitempty"`
		UserID    string `json:"user_id,omitempty"`
	}

	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errorResponse(req.ID, "invalid_params", err.Error()), nil
	}

	switch params.Operation {
	case "create":
		session := p.CreateSession(params.UserID)
		return &Response{
			ID:   req.ID,
			Type: MsgTypeResponse,
			Result: map[string]interface{}{
				"session_id": session.ID,
			},
		}, nil

	case "get":
		session, ok := p.GetSession(params.SessionID)
		if !ok {
			return errorResponse(req.ID, "not_found", "session not found"), nil
		}
		return &Response{
			ID:   req.ID,
			Type: MsgTypeResponse,
			Result: map[string]interface{}{
				"session": session,
			},
		}, nil

	case "delete":
		if err := p.DeleteSession(params.SessionID); err != nil {
			return errorResponse(req.ID, "delete_error", err.Error()), nil
		}
		return &Response{
			ID:   req.ID,
			Type: MsgTypeResponse,
			Result: map[string]interface{}{
				"status": "deleted",
			},
		}, nil

	case "history":
		session, ok := p.GetSession(params.SessionID)
		if !ok {
			return errorResponse(req.ID, "not_found", "session not found"), nil
		}
		session.Mu.RLock()
		history := make([]Message, len(session.History))
		copy(history, session.History)
		session.Mu.RUnlock()
		return &Response{
			ID:   req.ID,
			Type: MsgTypeResponse,
			Result: map[string]interface{}{
				"history": history,
			},
		}, nil

	default:
		return errorResponse(req.ID, "invalid_operation", "unknown operation"), nil
	}
}

func (p *ProtocolHandler) handleStream(ctx context.Context, req *Request) (*Response, error) {
	return &Response{
		ID:   req.ID,
		Type: MsgTypeResponse,
		Result: map[string]interface{}{
			"stream_url": fmt.Sprintf("/api/a2ui/stream/%s", generateID("stream")),
		},
	}, nil
}

func (p *ProtocolHandler) GetOrCreateSession(sessionID string) *Session {
	if sessionID == "" {
		return p.CreateSession("default")
	}
	session, ok := p.GetSession(sessionID)
	if !ok {
		return p.CreateSession("default")
	}
	return session
}

func errorResponse(id, code, message string) *Response {
	return &Response{
		ID:   id,
		Type: MsgTypeError,
		Error: &Error{
			Code:    code,
			Message: message,
		},
	}
}

func generateID(prefix string) string {
	return fmt.Sprintf("%s-%d-%s", prefix, time.Now().UnixNano(), randString(8))
}

func randString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[time.Now().UnixNano()%int64(len(letters))]
	}
	return string(b)
}

func extractToolCalls(message string) []*ToolCall {
	var calls []*ToolCall
	lines := strings.Split(message, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "```json") {
			var tc ToolCall
			if err := json.Unmarshal([]byte(line[7:]), &tc); err == nil {
				if tc.Name != "" {
					calls = append(calls, &tc)
				}
			}
		}
	}

	return calls
}

func NewStreamEvent(eventType EventType, data interface{}) *StreamEvent {
	return &StreamEvent{
		Type:      eventType,
		Data:      data,
		Timestamp: time.Now().Unix(),
	}
}

func (e *StreamEvent) JSON() ([]byte, error) {
	return json.Marshal(e)
}
