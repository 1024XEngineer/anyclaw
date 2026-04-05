package remote

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

type TunnelMode string

const (
	TunnelModeTailscale TunnelMode = "tailscale"
	TunnelModeSSH       TunnelMode = "ssh"
	TunnelModeGateway   TunnelMode = "gateway"
)

type TunnelConfig struct {
	Mode         TunnelMode
	LocalAddr    string
	RemoteAddr   string
	TailscaleKey string
	SSHUser      string
	SSHKeyPath   string
	SSHPassword  string
	GatewayURL   string
	GatewayToken string
}

type TunnelStatus string

const (
	TunnelStatusIdle       TunnelStatus = "idle"
	TunnelStatusConnecting TunnelStatus = "connecting"
	TunnelStatusConnected  TunnelStatus = "connected"
	TunnelStatusError      TunnelStatus = "error"
)

type Tunnel interface {
	Start(ctx context.Context) error
	Stop() error
	Status() TunnelStatus
	Endpoint() string
}

type RemoteGateway struct {
	mu       sync.RWMutex
	config   TunnelConfig
	status   TunnelStatus
	endpoint string
	listener net.Listener
	client   *http.Client
	conns    map[string]net.Conn
}

func NewRemoteGateway(cfg TunnelConfig) *RemoteGateway {
	return &RemoteGateway{
		config: cfg,
		status: TunnelStatusIdle,
		conns:  make(map[string]net.Conn),
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout: 10 * time.Second,
				}).DialContext,
			},
		},
	}
}

func (r *RemoteGateway) Start(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.status == TunnelStatusConnected {
		return nil
	}

	r.status = TunnelStatusConnecting

	switch r.config.Mode {
	case TunnelModeTailscale:
		return r.startTailscale(ctx)
	case TunnelModeSSH:
		return r.startSSH(ctx)
	case TunnelModeGateway:
		return r.startGateway(ctx)
	default:
		return fmt.Errorf("unsupported tunnel mode: %s", r.config.Mode)
	}
}

func (r *RemoteGateway) startTailscale(ctx context.Context) error {
	r.status = TunnelStatusConnected
	r.endpoint = fmt.Sprintf("tcp://%s", r.config.LocalAddr)
	return nil
}

func (r *RemoteGateway) startSSH(ctx context.Context) error {
	r.status = TunnelStatusConnected
	r.endpoint = fmt.Sprintf("tcp://%s", r.config.LocalAddr)
	return nil
}

func (r *RemoteGateway) startGateway(ctx context.Context) error {
	if r.config.GatewayURL == "" {
		return fmt.Errorf("gateway url is required")
	}

	r.status = TunnelStatusConnected
	r.endpoint = r.config.GatewayURL
	return nil
}

func (r *RemoteGateway) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.listener != nil {
		r.listener.Close()
	}

	for _, conn := range r.conns {
		conn.Close()
	}
	r.conns = make(map[string]net.Conn)

	r.status = TunnelStatusIdle
	r.endpoint = ""
	return nil
}

func (r *RemoteGateway) Status() TunnelStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.status
}

func (r *RemoteGateway) Endpoint() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.endpoint
}

type RemoteNode struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Type         string            `json:"type"`
	State        string            `json:"state"`
	Endpoint     string            `json:"endpoint"`
	Capabilities []string          `json:"capabilities"`
	Metadata     map[string]string `json:"metadata"`
	ConnectedAt  time.Time         `json:"connected_at"`
	LastSeen     time.Time         `json:"last_seen"`
}

type NodeRegistry struct {
	mu    sync.RWMutex
	nodes map[string]*RemoteNode
}

func NewNodeRegistry() *NodeRegistry {
	return &NodeRegistry{
		nodes: make(map[string]*RemoteNode),
	}
}

func (nr *NodeRegistry) Register(node *RemoteNode) {
	nr.mu.Lock()
	defer nr.mu.Unlock()
	node.ConnectedAt = time.Now()
	node.LastSeen = time.Now()
	nr.nodes[node.ID] = node
}

func (nr *NodeRegistry) Unregister(nodeID string) {
	nr.mu.Lock()
	defer nr.mu.Unlock()
	delete(nr.nodes, nodeID)
}

func (nr *NodeRegistry) Get(nodeID string) (*RemoteNode, bool) {
	nr.mu.RLock()
	defer nr.mu.RUnlock()
	node, ok := nr.nodes[nodeID]
	return node, ok
}

func (nr *NodeRegistry) List() []*RemoteNode {
	nr.mu.RLock()
	defer nr.mu.RUnlock()
	result := make([]*RemoteNode, 0, len(nr.nodes))
	for _, node := range nr.nodes {
		result = append(result, node)
	}
	return result
}

func (nr *NodeRegistry) UpdateState(nodeID string, state string) {
	nr.mu.Lock()
	defer nr.mu.Unlock()
	if node, ok := nr.nodes[nodeID]; ok {
		node.State = state
		node.LastSeen = time.Now()
	}
}
