package node

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Device represents a connected device or discovered worker endpoint.
type Device struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Type         string            `json:"type"`
	Capabilities []string          `json:"capabilities"`
	Status       string            `json:"status"`
	ConnectedAt  time.Time         `json:"connected_at"`
	LastSeen     time.Time         `json:"last_seen"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// DeviceManager tracks lightweight device and node inventory for gateway control APIs.
type DeviceManager struct {
	mu      sync.RWMutex
	devices map[string]*Device
}

func NewDeviceManager() *DeviceManager {
	return &DeviceManager{
		devices: make(map[string]*Device),
	}
}

func (m *DeviceManager) Register(device *Device) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if device.ID == "" {
		device.ID = fmt.Sprintf("node-%d", time.Now().UnixNano())
	}
	now := time.Now()
	device.ConnectedAt = now
	device.LastSeen = now
	device.Status = "online"

	m.devices[device.ID] = device
	return nil
}

func (m *DeviceManager) Unregister(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if device, ok := m.devices[id]; ok {
		device.Status = "offline"
	}
	return nil
}

func (m *DeviceManager) Get(id string) (*Device, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	device, ok := m.devices[id]
	return device, ok
}

func (m *DeviceManager) List() []*Device {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list := make([]*Device, 0, len(m.devices))
	for _, device := range m.devices {
		list = append(list, device)
	}
	return list
}

func (m *DeviceManager) Invoke(ctx context.Context, nodeID string, action string, params map[string]any) (map[string]any, error) {
	m.mu.RLock()
	device, ok := m.devices[nodeID]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("node not found: %s", nodeID)
	}
	if device.Status != "online" {
		return nil, fmt.Errorf("node is offline: %s", nodeID)
	}

	m.mu.Lock()
	device.LastSeen = time.Now()
	m.mu.Unlock()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	switch action {
	case "system.run":
		return map[string]any{"status": "executed", "action": action, "params": params}, nil
	case "system.notify":
		return map[string]any{"status": "notified", "action": action, "params": params}, nil
	case "camera.snap":
		return map[string]any{"status": "captured", "action": action}, nil
	case "location.get":
		return map[string]any{"status": "retrieved", "action": action, "lat": 0, "lng": 0}, nil
	case "screen.record":
		return map[string]any{"status": "recording", "action": action}, nil
	default:
		return nil, fmt.Errorf("unsupported action: %s", action)
	}
}

func (m *DeviceManager) GetCapabilities(nodeID string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	device, ok := m.devices[nodeID]
	if !ok {
		return nil, fmt.Errorf("node not found: %s", nodeID)
	}
	return device.Capabilities, nil
}

func (m *DeviceManager) Health() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()

	online := 0
	offline := 0
	for _, device := range m.devices {
		if device.Status == "online" {
			online++
		} else {
			offline++
		}
	}

	return map[string]any{
		"total":   len(m.devices),
		"online":  online,
		"offline": offline,
	}
}
