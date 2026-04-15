package protocol

import "encoding/json"

// FrameType distinguishes request, response, and server-push frames.
type FrameType string

const (
	FrameReq   FrameType = "req"
	FrameRes   FrameType = "res"
	FrameEvent FrameType = "event"
)

// RequestFrame is the wire envelope for an inbound RPC request.
type RequestFrame struct {
	Type   FrameType       `json:"type"`
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// ResponseFrame is the wire envelope for an RPC response.
type ResponseFrame struct {
	Type    FrameType       `json:"type"`
	ID      string          `json:"id"`
	OK      bool            `json:"ok"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   *ErrorPayload   `json:"error,omitempty"`
}

// EventFrame is the wire envelope for an asynchronous server event.
type EventFrame struct {
	Type         FrameType `json:"type"`
	Event        string    `json:"event"`
	Payload      any       `json:"payload,omitempty"`
	Seq          uint64    `json:"seq,omitempty"`
	StateVersion uint64    `json:"stateVersion,omitempty"`
}
