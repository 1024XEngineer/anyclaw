package speech

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"
)

type DeviceType string

const (
	DeviceTypeUnknown DeviceType = "unknown"
	DeviceTypeDesktop DeviceType = "desktop"
	DeviceTypeLaptop  DeviceType = "laptop"
	DeviceTypePhone   DeviceType = "phone"
	DeviceTypeTablet  DeviceType = "tablet"
	DeviceTypeSpeaker DeviceType = "speaker"
	DeviceTypePi      DeviceType = "raspberry-pi"
)

type DeviceInfo struct {
	ID           string
	Name         string
	Type         DeviceType
	Hostname     string
	IPAddress    string
	Port         int
	Capabilities []string
	Priority     int
	LastSeen     time.Time
	Metadata     map[string]string
}

type DiscoveryEvent struct {
	Type      string
	Device    DeviceInfo
	Timestamp time.Time
}

type DiscoveryListener func(event DiscoveryEvent)

type DeviceDiscoveryConfig struct {
	DeviceID         string
	DeviceName       string
	DeviceType       DeviceType
	BindPort         int
	BroadcastPort    int
	BroadcastAddr    string
	AnnounceInterval time.Duration
	DiscoveryTimeout time.Duration
	Capabilities     []string
	Priority         int
}

func DefaultDeviceDiscoveryConfig() DeviceDiscoveryConfig {
	return DeviceDiscoveryConfig{
		BindPort:         0,
		BroadcastPort:    19876,
		BroadcastAddr:    "255.255.255.255",
		AnnounceInterval: 5 * time.Second,
		DiscoveryTimeout: 15 * time.Second,
		Capabilities:     []string{"voice-wake", "stt", "tts"},
		Priority:         50,
	}
}

type DeviceDiscovery struct {
	mu        sync.Mutex
	wg        sync.WaitGroup
	cfg       DeviceDiscoveryConfig
	devices   map[string]*DeviceInfo
	conn      *net.UDPConn
	listeners []DiscoveryListener
	isRunning bool
	doneCh    chan struct{}
	localInfo DeviceInfo
}

const (
	discoveryMsgAnnounce = "announce"
	discoveryMsgProbe    = "probe"
	discoveryMsgResponse = "response"
)

type discoveryMessage struct {
	Type     string     `json:"type"`
	Device   DeviceInfo `json:"device"`
	Sequence int64      `json:"seq"`
}

func NewDeviceDiscovery(cfg DeviceDiscoveryConfig) *DeviceDiscovery {
	if cfg.BroadcastPort == 0 {
		cfg.BroadcastPort = 19876
	}
	if cfg.BroadcastAddr == "" {
		cfg.BroadcastAddr = "255.255.255.255"
	}
	if cfg.AnnounceInterval == 0 {
		cfg.AnnounceInterval = 5 * time.Second
	}
	if cfg.DiscoveryTimeout == 0 {
		cfg.DiscoveryTimeout = 15 * time.Second
	}
	if cfg.Priority == 0 {
		cfg.Priority = 50
	}
	if len(cfg.Capabilities) == 0 {
		cfg.Capabilities = []string{"voice-wake", "stt", "tts"}
	}

	return &DeviceDiscovery{
		cfg:     cfg,
		devices: make(map[string]*DeviceInfo),
	}
}

func (d *DeviceDiscovery) Start() error {
	d.mu.Lock()

	if d.isRunning {
		d.mu.Unlock()
		return fmt.Errorf("discovery: already running")
	}

	addr := &net.UDPAddr{
		IP:   net.IPv4zero,
		Port: d.cfg.BroadcastPort,
	}

	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		d.mu.Unlock()
		return fmt.Errorf("discovery: failed to bind UDP port: %w", err)
	}

	d.conn = conn
	done := make(chan struct{})
	d.doneCh = done
	d.isRunning = true

	d.localInfo = DeviceInfo{
		ID:           d.cfg.DeviceID,
		Name:         d.cfg.DeviceName,
		Type:         d.cfg.DeviceType,
		Port:         d.cfg.BroadcastPort,
		Capabilities: d.cfg.Capabilities,
		Priority:     d.cfg.Priority,
		LastSeen:     time.Now(),
		Metadata:     make(map[string]string),
	}

	d.wg.Add(2)
	go d.listenLoop(conn, done)
	go d.announceLoop(done)

	d.mu.Unlock()

	d.broadcastMessage(discoveryMsgAnnounce)

	return nil
}

func (d *DeviceDiscovery) Stop() error {
	d.mu.Lock()

	if !d.isRunning {
		d.mu.Unlock()
		return nil
	}

	d.isRunning = false
	conn := d.conn
	done := d.doneCh
	d.conn = nil
	d.mu.Unlock()

	if conn != nil {
		_ = conn.Close()
	}

	if done != nil {
		close(done)
	}

	d.wg.Wait()

	d.mu.Lock()
	if d.doneCh == done {
		d.doneCh = nil
	}
	d.mu.Unlock()

	return nil
}

func (d *DeviceDiscovery) listenLoop(conn *net.UDPConn, done <-chan struct{}) {
	defer d.wg.Done()

	buf := make([]byte, 4096)

	for {
		if conn == nil {
			return
		}

		n, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-done:
				return
			default:
				continue
			}
		}

		if n == 0 {
			continue
		}

		var msg discoveryMessage
		if err := json.Unmarshal(buf[:n], &msg); err != nil {
			continue
		}

		if msg.Device.ID == d.cfg.DeviceID {
			continue
		}

		d.mu.Lock()
		existing, known := d.devices[msg.Device.ID]
		if known {
			existing.LastSeen = time.Now()
			existing.IPAddress = remoteAddr.IP.String()
		} else {
			info := msg.Device
			info.IPAddress = remoteAddr.IP.String()
			info.LastSeen = time.Now()
			d.devices[info.ID] = &info
		}
		d.mu.Unlock()

		switch msg.Type {
		case discoveryMsgAnnounce:
			d.notifyListeners(DiscoveryEvent{
				Type:      "device_discovered",
				Device:    msg.Device,
				Timestamp: time.Now(),
			})

			d.broadcastMessage(discoveryMsgResponse)

		case discoveryMsgProbe:
			d.broadcastMessage(discoveryMsgResponse)

		case discoveryMsgResponse:
			d.notifyListeners(DiscoveryEvent{
				Type:      "device_responded",
				Device:    msg.Device,
				Timestamp: time.Now(),
			})
		}
	}
}

func (d *DeviceDiscovery) announceLoop(done <-chan struct{}) {
	defer d.wg.Done()

	ticker := time.NewTicker(d.cfg.AnnounceInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d.broadcastMessage(discoveryMsgAnnounce)
			d.pruneStaleDevices()

		case <-done:
			return
		}
	}
}

func (d *DeviceDiscovery) broadcastMessage(msgType string) {
	d.mu.Lock()
	conn := d.conn
	localInfo := d.localInfo
	d.mu.Unlock()

	if conn == nil {
		return
	}

	localInfo.LastSeen = time.Now()

	msg := discoveryMessage{
		Type:     msgType,
		Device:   localInfo,
		Sequence: time.Now().UnixNano(),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	broadcastAddr := &net.UDPAddr{
		IP:   net.ParseIP(d.cfg.BroadcastAddr),
		Port: d.cfg.BroadcastPort,
	}

	conn.SetWriteDeadline(time.Now().Add(100 * time.Millisecond))
	_, _ = conn.WriteToUDP(data, broadcastAddr)
}

func (d *DeviceDiscovery) pruneStaleDevices() {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	for id, device := range d.devices {
		if now.Sub(device.LastSeen) > d.cfg.DiscoveryTimeout {
			delete(d.devices, id)
			d.notifyListeners(DiscoveryEvent{
				Type:      "device_lost",
				Device:    *device,
				Timestamp: now,
			})
		}
	}
}

func (d *DeviceDiscovery) RegisterListener(listener DiscoveryListener) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.listeners = append(d.listeners, listener)
}

func (d *DeviceDiscovery) notifyListeners(event DiscoveryEvent) {
	d.mu.Lock()
	listeners := make([]DiscoveryListener, len(d.listeners))
	copy(listeners, d.listeners)
	d.mu.Unlock()

	for _, listener := range listeners {
		listener(event)
	}
}

func (d *DeviceDiscovery) GetDevice(id string) (*DeviceInfo, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	device, ok := d.devices[id]
	if !ok {
		return nil, false
	}

	return device, true
}

func (d *DeviceDiscovery) GetDevices() []DeviceInfo {
	d.mu.Lock()
	defer d.mu.Unlock()

	devices := make([]DeviceInfo, 0, len(d.devices))
	for _, device := range d.devices {
		devices = append(devices, *device)
	}

	return devices
}

func (d *DeviceDiscovery) GetDeviceCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.devices)
}

func (d *DeviceDiscovery) LocalDevice() DeviceInfo {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.localInfo
}

func (d *DeviceDiscovery) HasCapability(deviceID, capability string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	device, ok := d.devices[deviceID]
	if !ok {
		return false
	}

	for _, cap := range device.Capabilities {
		if cap == capability {
			return true
		}
	}

	return false
}

func (d *DeviceDiscovery) GetDevicesWithCapability(capability string) []DeviceInfo {
	d.mu.Lock()
	defer d.mu.Unlock()

	var result []DeviceInfo
	for _, device := range d.devices {
		for _, cap := range device.Capabilities {
			if cap == capability {
				result = append(result, *device)
				break
			}
		}
	}

	return result
}

func (d *DeviceDiscovery) Probe() {
	d.broadcastMessage(discoveryMsgProbe)
}

func (d *DeviceDiscovery) SetPriority(priority int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.localInfo.Priority = priority
	d.cfg.Priority = priority
}

func (d *DeviceDiscovery) SetDeviceName(name string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.localInfo.Name = name
	d.cfg.DeviceName = name
}

func (d *DeviceDiscovery) UpdateMetadata(key, value string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.localInfo.Metadata == nil {
		d.localInfo.Metadata = make(map[string]string)
	}
	d.localInfo.Metadata[key] = value
}

type MockDeviceDiscovery struct {
	mu        sync.Mutex
	devices   map[string]*DeviceInfo
	local     DeviceInfo
	listeners []DiscoveryListener
}

func NewMockDeviceDiscovery(deviceID, deviceName string, priority int) *MockDeviceDiscovery {
	return &MockDeviceDiscovery{
		devices: make(map[string]*DeviceInfo),
		local: DeviceInfo{
			ID:           deviceID,
			Name:         deviceName,
			Type:         DeviceTypeDesktop,
			Priority:     priority,
			Capabilities: []string{"voice-wake", "stt", "tts"},
			LastSeen:     time.Now(),
		},
	}
}

func (m *MockDeviceDiscovery) AddDevice(info DeviceInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()

	info.LastSeen = time.Now()
	m.devices[info.ID] = &info

	for _, listener := range m.listeners {
		listener(DiscoveryEvent{
			Type:      "device_discovered",
			Device:    info,
			Timestamp: time.Now(),
		})
	}
}

func (m *MockDeviceDiscovery) RemoveDevice(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if device, ok := m.devices[id]; ok {
		delete(m.devices, id)
		for _, listener := range m.listeners {
			listener(DiscoveryEvent{
				Type:      "device_lost",
				Device:    *device,
				Timestamp: time.Now(),
			})
		}
	}
}

func (m *MockDeviceDiscovery) GetDevices() []DeviceInfo {
	m.mu.Lock()
	defer m.mu.Unlock()

	devices := make([]DeviceInfo, 0, len(m.devices))
	for _, device := range m.devices {
		devices = append(devices, *device)
	}
	return devices
}

func (m *MockDeviceDiscovery) GetDeviceCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.devices)
}

func (m *MockDeviceDiscovery) LocalDevice() DeviceInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.local
}

func (m *MockDeviceDiscovery) RegisterListener(listener DiscoveryListener) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listeners = append(m.listeners, listener)
}
