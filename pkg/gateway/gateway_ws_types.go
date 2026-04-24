package gateway

import (
	"sync"
	"time"

	gatewaysurface "github.com/1024XEngineer/anyclaw/pkg/gateway/surface"
	"github.com/1024XEngineer/anyclaw/pkg/state"
	"github.com/gorilla/websocket"
)

type openClawWSFrame struct {
	Type   string         `json:"type"`
	ID     string         `json:"id,omitempty"`
	Method string         `json:"method,omitempty"`
	Event  string         `json:"event,omitempty"`
	Params map[string]any `json:"params,omitempty"`
	Data   any            `json:"data,omitempty"`
	OK     bool           `json:"ok,omitempty"`
	Error  string         `json:"error,omitempty"`
}

type openClawWSConn struct {
	server      *Server
	conn        *websocket.Conn
	user        *AuthUser
	writeMu     sync.Mutex
	connected   bool
	connMu      sync.RWMutex
	challenge   string
	transport   gatewaysurface.TransportRef
	eventStream chan *state.Event
	closed      chan struct{}
	closeOnce   sync.Once
	connectedAt time.Time
}
