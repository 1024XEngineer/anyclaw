package gateway

import (
	"net/http"

	gatewaysurface "github.com/1024XEngineer/anyclaw/pkg/gateway/surface"
	"github.com/gorilla/websocket"
)

var openClawWSUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func (s *Server) handleOpenClawWS(w http.ResponseWriter, r *http.Request) {
	conn, err := openClawWSUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	client := &openClawWSConn{
		server:    s,
		conn:      conn,
		user:      UserFromContext(r.Context()),
		challenge: uniqueID("ws"),
		transport: gatewaysurface.HTTPTransport(r, ""),
		closed:    make(chan struct{}),
	}
	client.run(r.Context())
}
