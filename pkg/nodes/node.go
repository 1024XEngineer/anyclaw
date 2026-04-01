package nodes

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type NodeType string

const (
	NodeTypeMacOS   NodeType = "macos"
	NodeTypeIOS     NodeType = "ios"
	NodeTypeAndroid NodeType = "android"
	NodeTypeLinux   NodeType = "linux"
	NodeTypeWindows NodeType = "windows"
)

type NodeState string

const (
	NodeStateConnecting NodeState = "connecting"
	NodeStateOnline     NodeState = "online"
	NodeStateOffline    NodeState = "offline"
	NodeStateBusy       NodeState = "busy"
	NodeStateError      NodeState = "error"
)

type Node struct {
	ID           string
	Name         string
	Type         NodeType
	State        NodeState
	Platform     string
	Version      string
	Capabilities []string
	ConnectedAt  time.Time
	LastSeen     time.Time
	IPAddress    string
	wsConn       *websocket.Conn

	mu      sync.RWMutex
	actions map[string]*NodeAction
}

type NodeAction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

type NodeManager struct {
	nodes map[string]*Node
	mu    sync.RWMutex

	onNodeConnect    func(node *Node)
	onNodeDisconnect func(node *Node)
	onNodeMessage    func(node *Node, msg []byte)
}

func NewNodeManager() *NodeManager {
	return &NodeManager{
		nodes: make(map[string]*Node),
	}
}

func (nm *NodeManager) Register(node *Node) {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	node.State = NodeStateOnline
	node.ConnectedAt = time.Now()
	nm.nodes[node.ID] = node

	if nm.onNodeConnect != nil {
		nm.onNodeConnect(node)
	}
}

func (nm *NodeManager) Unregister(nodeID string) {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if node, ok := nm.nodes[nodeID]; ok {
		node.State = NodeStateOffline
		if nm.onNodeDisconnect != nil {
			nm.onNodeDisconnect(node)
		}
		delete(nm.nodes, nodeID)
	}
}

func (nm *NodeManager) Get(nodeID string) (*Node, bool) {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	node, ok := nm.nodes[nodeID]
	return node, ok
}

func (nm *NodeManager) List() []*Node {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	result := make([]*Node, 0, len(nm.nodes))
	for _, node := range nm.nodes {
		result = append(result, node)
	}
	return result
}

func (nm *NodeManager) ListByType(nodeType NodeType) []*Node {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	result := make([]*Node, 0)
	for _, node := range nm.nodes {
		if node.Type == nodeType {
			result = append(result, node)
		}
	}
	return result
}

func (nm *NodeManager) Invoke(ctx context.Context, nodeID string, action string, input json.RawMessage) (json.RawMessage, error) {
	nm.mu.RLock()
	node, ok := nm.nodes[nodeID]
	nm.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("node not found: %s", nodeID)
	}

	if node.State != NodeStateOnline {
		return nil, fmt.Errorf("node is not online: %s", nodeID)
	}

	node.mu.RLock()
	actionExists := false
	for a := range node.actions {
		if a == action {
			actionExists = true
			break
		}
	}
	node.mu.RUnlock()

	if !actionExists {
		return nil, fmt.Errorf("action not found: %s", action)
	}

	req := map[string]any{
		"type":   "node_action",
		"action": action,
		"input":  input,
	}

	data, _ := json.Marshal(req)
	err := node.wsConn.WriteMessage(websocket.TextMessage, data)
	if err != nil {
		return nil, fmt.Errorf("failed to send action: %w", err)
	}

	return json.Marshal(map[string]any{
		"invoked": true,
		"action":  action,
	})
}

func (nm *NodeManager) OnNodeConnect(handler func(node *Node)) {
	nm.onNodeConnect = handler
}

func (nm *NodeManager) OnNodeDisconnect(handler func(node *Node)) {
	nm.onNodeDisconnect = handler
}

func (nm *NodeManager) OnNodeMessage(handler func(node *Node, msg []byte)) {
	nm.onNodeMessage = handler
}

func (n *Node) RegisterAction(action NodeAction) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.actions == nil {
		n.actions = make(map[string]*NodeAction)
	}
	n.actions[action.Name] = &action
}

func (n *Node) HasCapability(cap string) bool {
	n.mu.RLock()
	defer n.mu.RUnlock()

	for _, c := range n.Capabilities {
		if c == cap {
			return true
		}
	}
	return false
}

func (n *Node) UpdateState(state NodeState) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.State = state
	n.LastSeen = time.Now()
}

type NodeInfo struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	State        string   `json:"state"`
	Platform     string   `json:"platform"`
	Version      string   `json:"version"`
	Capabilities []string `json:"capabilities"`
	LastSeen     int64    `json:"last_seen"`
}

func (n *Node) Info() NodeInfo {
	n.mu.RLock()
	defer n.mu.RUnlock()

	return NodeInfo{
		ID:           n.ID,
		Name:         n.Name,
		Type:         string(n.Type),
		State:        string(n.State),
		Platform:     n.Platform,
		Version:      n.Version,
		Capabilities: n.Capabilities,
		LastSeen:     n.LastSeen.Unix(),
	}
}
