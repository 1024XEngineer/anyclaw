package node

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type PairingState string

const (
	PairingStateIdle    PairingState = "idle"
	PairingStateWaiting PairingState = "waiting"
	PairingStatePaired  PairingState = "paired"
	PairingStateExpired PairingState = "expired"
	PairingStateRevoked PairingState = "revoked"
)

type PairingCode struct {
	Code        string       `json:"code"`
	NodeID      string       `json:"node_id"`
	NodeName    string       `json:"node_name"`
	CreatedAt   time.Time    `json:"created_at"`
	ExpiresAt   time.Time    `json:"expires_at"`
	Permissions []string     `json:"permissions"`
	State       PairingState `json:"state"`
}

type NodePermission string

const (
	PermissionExecute  NodePermission = "execute"
	PermissionVoice    NodePermission = "voice"
	PermissionScreen   NodePermission = "screen"
	PermissionFile     NodePermission = "file"
	PermissionAdmin    NodePermission = "admin"
	PermissionReadOnly NodePermission = "read-only"
)

type Capability struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

type DeviceStatus struct {
	Online      bool      `json:"online"`
	State       string    `json:"state"`
	Battery     int       `json:"battery,omitempty"`
	NetworkType string    `json:"network_type,omitempty"`
	IPAddress   string    `json:"ip_address,omitempty"`
	Location    string    `json:"location,omitempty"`
	LastUpdate  time.Time `json:"last_update"`
}

type NodePairingManager struct {
	mu          sync.RWMutex
	pairings    map[string]*PairingCode
	activePairs map[string]*NodePair

	onPairComplete func(nodeID string, permissions []string) error
	onPairRevoke   func(nodeID string) error
}

type NodePair struct {
	NodeID      string        `json:"node_id"`
	Permissions []string      `json:"permissions"`
	PairedAt    time.Time     `json:"paired_at"`
	LastActive  time.Time     `json:"last_active"`
	ExpiresAt   time.Time     `json:"expires_at"`
	DeviceInfo  *DeviceStatus `json:"device_info,omitempty"`
}

func NewNodePairingManager() *NodePairingManager {
	return &NodePairingManager{
		pairings:    make(map[string]*PairingCode),
		activePairs: make(map[string]*NodePair),
	}
}

func (m *NodePairingManager) GeneratePairingCode(nodeID, nodeName string, ttl time.Duration, permissions []string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	codeBytes := make([]byte, 4)
	if _, err := rand.Read(codeBytes); err != nil {
		return "", fmt.Errorf("failed to generate code: %w", err)
	}
	code := hex.EncodeToString(codeBytes)

	now := time.Now()
	pairing := &PairingCode{
		Code:        code,
		NodeID:      nodeID,
		NodeName:    nodeName,
		CreatedAt:   now,
		ExpiresAt:   now.Add(ttl),
		Permissions: permissions,
		State:       PairingStateWaiting,
	}

	m.pairings[code] = pairing

	go m.expirePairing(code, ttl)

	return code, nil
}

func (m *NodePairingManager) expirePairing(code string, ttl time.Duration) {
	time.Sleep(ttl)
	m.mu.Lock()
	defer m.mu.Unlock()
	if p, ok := m.pairings[code]; ok && p.State == PairingStateWaiting {
		p.State = PairingStateExpired
	}
}

func (m *NodePairingManager) AcceptPairing(code string) (*NodePair, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	pairing, ok := m.pairings[code]
	if !ok {
		return nil, fmt.Errorf("pairing code not found")
	}

	if pairing.State != PairingStateWaiting {
		return nil, fmt.Errorf("pairing code is not valid: state=%s", pairing.State)
	}

	if time.Now().After(pairing.ExpiresAt) {
		pairing.State = PairingStateExpired
		return nil, fmt.Errorf("pairing code has expired")
	}

	pairing.State = PairingStatePaired

	pair := &NodePair{
		NodeID:      pairing.NodeID,
		Permissions: pairing.Permissions,
		PairedAt:    time.Now(),
		LastActive:  time.Now(),
		ExpiresAt:   time.Now().Add(72 * time.Hour),
	}

	m.activePairs[pairing.NodeID] = pair

	if m.onPairComplete != nil {
		m.onPairComplete(pairing.NodeID, pairing.Permissions)
	}

	return pair, nil
}

func (m *NodePairingManager) RevokePairing(nodeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, ok := m.activePairs[nodeID]
	if !ok {
		return fmt.Errorf("node not paired")
	}

	delete(m.activePairs, nodeID)

	if m.onPairRevoke != nil {
		m.onPairRevoke(nodeID)
	}

	return nil
}

func (m *NodePairingManager) GetPair(nodeID string) (*NodePair, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	pair, ok := m.activePairs[nodeID]
	return pair, ok
}

func (m *NodePairingManager) ListPairedNodes() []*NodePair {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*NodePair, 0, len(m.activePairs))
	for _, pair := range m.activePairs {
		result = append(result, pair)
	}
	return result
}

func (m *NodePairingManager) CheckPermission(nodeID string, permission string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	pair, ok := m.activePairs[nodeID]
	if !ok {
		return false
	}

	for _, p := range pair.Permissions {
		if p == string(permission) || p == "admin" {
			return true
		}
	}
	return false
}

func (m *NodePairingManager) RefreshActivity(nodeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	pair, ok := m.activePairs[nodeID]
	if !ok {
		return fmt.Errorf("node not paired")
	}

	pair.LastActive = time.Now()
	return nil
}

func (m *NodePairingManager) OnPairComplete(handler func(nodeID string, permissions []string) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onPairComplete = handler
}

func (m *NodePairingManager) OnPairRevoke(handler func(nodeID string) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onPairRevoke = handler
}

type CapabilityRegistry struct {
	mu           sync.RWMutex
	capabilities map[string]map[string]*Capability
}

func NewCapabilityRegistry() *CapabilityRegistry {
	return &CapabilityRegistry{
		capabilities: make(map[string]map[string]*Capability),
	}
}

func (cr *CapabilityRegistry) Register(nodeID string, caps []Capability) {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	if cr.capabilities[nodeID] == nil {
		cr.capabilities[nodeID] = make(map[string]*Capability)
	}

	for _, cap := range caps {
		capCopy := cap
		cr.capabilities[nodeID][cap.Name] = &capCopy
	}
}

func (cr *CapabilityRegistry) GetCapabilities(nodeID string) []Capability {
	cr.mu.RLock()
	defer cr.mu.RUnlock()

	nodeCaps, ok := cr.capabilities[nodeID]
	if !ok {
		return nil
	}

	result := make([]Capability, 0, len(nodeCaps))
	for _, cap := range nodeCaps {
		result = append(result, *cap)
	}
	return result
}

func (cr *CapabilityRegistry) HasCapability(nodeID, capability string) bool {
	cr.mu.RLock()
	defer cr.mu.RUnlock()

	nodeCaps, ok := cr.capabilities[nodeID]
	if !ok {
		return false
	}

	_, exists := nodeCaps[capability]
	return exists
}

func (cr *CapabilityRegistry) Unregister(nodeID string) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	delete(cr.capabilities, nodeID)
}

type DeviceStatusManager struct {
	mu       sync.RWMutex
	statuses map[string]*DeviceStatus
}

func NewDeviceStatusManager() *DeviceStatusManager {
	return &DeviceStatusManager{
		statuses: make(map[string]*DeviceStatus),
	}
}

func (dm *DeviceStatusManager) UpdateStatus(nodeID string, status *DeviceStatus) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	status.LastUpdate = time.Now()
	dm.statuses[nodeID] = status
}

func (dm *DeviceStatusManager) GetStatus(nodeID string) (*DeviceStatus, bool) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	status, ok := dm.statuses[nodeID]
	return status, ok
}

func (dm *DeviceStatusManager) ListStatuses() []*DeviceStatus {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	result := make([]*DeviceStatus, 0, len(dm.statuses))
	for _, status := range dm.statuses {
		result = append(result, status)
	}
	return result
}

func (dm *DeviceStatusManager) PruneOffline(maxAge time.Duration) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	now := time.Now()
	for nodeID, status := range dm.statuses {
		if !status.Online && now.Sub(status.LastUpdate) > maxAge {
			delete(dm.statuses, nodeID)
		}
	}
}
