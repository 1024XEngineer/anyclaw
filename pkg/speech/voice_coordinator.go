package speech

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

type VoiceWakeCoordinator struct {
	mu             sync.Mutex
	cfg            VoiceWakeCoordinatorConfig
	discovery      *DeviceDiscovery
	arbitration    *WakeArbitration
	suppressor     *WakeSuppressor
	localID        string
	isRunning      bool
	eventCh        chan CoordinatorEvent
	listeners      []CoordinatorListener
	stats          CoordinatorStats
	startTime      time.Time
	wakeConn       *net.UDPConn
	wakeListenDone chan struct{}
	wakePort       int
}

type VoiceWakeCoordinatorConfig struct {
	DeviceID          string
	DeviceName        string
	DeviceType        DeviceType
	DiscoveryConfig   DeviceDiscoveryConfig
	ArbitrationConfig WakeArbitrationConfig
	Enabled           bool
	AutoStart         bool
	BroadcastPort     int
	WakeEventPort     int
}

func DefaultVoiceWakeCoordinatorConfig() VoiceWakeCoordinatorConfig {
	return VoiceWakeCoordinatorConfig{
		DeviceID:          "device-local",
		DeviceName:        "Local Device",
		DeviceType:        DeviceTypeDesktop,
		DiscoveryConfig:   DefaultDeviceDiscoveryConfig(),
		ArbitrationConfig: DefaultWakeArbitrationConfig(),
		Enabled:           true,
		AutoStart:         false,
		BroadcastPort:     19876,
		WakeEventPort:     19877,
	}
}

type CoordinatorEventType string

const (
	CoordinatorEventDeviceDiscovered CoordinatorEventType = "device_discovered"
	CoordinatorEventDeviceLost       CoordinatorEventType = "device_lost"
	CoordinatorEventWakeSubmitted    CoordinatorEventType = "wake_submitted"
	CoordinatorEventArbitrationWon   CoordinatorEventType = "arbitration_won"
	CoordinatorEventArbitrationLost  CoordinatorEventType = "arbitration_lost"
	CoordinatorEventSuppressed       CoordinatorEventType = "suppressed"
	CoordinatorEventReleased         CoordinatorEventType = "released"
)

type CoordinatorEvent struct {
	Type      CoordinatorEventType
	Timestamp time.Time
	Data      map[string]any
}

type CoordinatorListener func(event CoordinatorEvent)

type CoordinatorStats struct {
	DevicesDiscovered int
	DevicesLost       int
	WakesSubmitted    int
	ArbitrationsWon   int
	ArbitrationsLost  int
	Suppressions      int
	Releases          int
	LastWakeTime      time.Time
	LastArbitration   time.Time
}

func NewVoiceWakeCoordinator(cfg VoiceWakeCoordinatorConfig) *VoiceWakeCoordinator {
	if cfg.DeviceID == "" {
		cfg.DeviceID = "device-local"
	}
	if cfg.DeviceName == "" {
		cfg.DeviceName = "Local Device"
	}
	if cfg.BroadcastPort == 0 {
		cfg.BroadcastPort = 19876
	}
	if cfg.WakeEventPort == 0 {
		cfg.WakeEventPort = 19877
	}

	cfg.DiscoveryConfig.DeviceID = cfg.DeviceID
	cfg.DiscoveryConfig.DeviceName = cfg.DeviceName
	cfg.DiscoveryConfig.DeviceType = cfg.DeviceType
	cfg.DiscoveryConfig.BroadcastPort = cfg.BroadcastPort

	cfg.ArbitrationConfig.LocalPriority = cfg.DiscoveryConfig.Priority

	suppressor := NewWakeSuppressor()
	arbitration := NewWakeArbitration(cfg.DeviceID, cfg.ArbitrationConfig)
	arbitration.SetSuppressor(suppressor)

	discovery := NewDeviceDiscovery(cfg.DiscoveryConfig)

	vc := &VoiceWakeCoordinator{
		cfg:         cfg,
		localID:     cfg.DeviceID,
		discovery:   discovery,
		arbitration: arbitration,
		suppressor:  suppressor,
		eventCh:     make(chan CoordinatorEvent, 100),
		wakePort:    cfg.WakeEventPort,
	}

	discovery.UpdateMetadata("wake_port", fmt.Sprintf("%d", cfg.WakeEventPort))

	return vc
}

func (c *VoiceWakeCoordinator) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.cfg.Enabled {
		return nil
	}

	if c.isRunning {
		return fmt.Errorf("coordinator: already running")
	}

	if err := c.discovery.Start(); err != nil {
		return fmt.Errorf("coordinator: failed to start discovery: %w", err)
	}

	c.discovery.RegisterListener(c.onDiscoveryEvent)
	c.arbitration.RegisterListener(c.onArbitrationResult)
	c.suppressor.RegisterListener(c.onSuppressionEvent)

	conn, done, err := c.startWakeEventListener()
	if err != nil {
		c.discovery.Stop()
		return fmt.Errorf("coordinator: failed to start wake event listener: %w", err)
	}

	c.isRunning = true
	c.startTime = time.Now()

	go c.wakeEventListenLoop(conn, done)
	go c.eventLoop(ctx)

	log.Printf("coordinator: started (device: %s, discovery port: %d, wake port: %d)", c.localID, c.cfg.BroadcastPort, c.wakePort)

	return nil
}

func (c *VoiceWakeCoordinator) Stop() error {
	c.mu.Lock()

	if !c.isRunning {
		c.mu.Unlock()
		return nil
	}

	c.isRunning = false
	discovery := c.discovery
	arbitration := c.arbitration
	c.mu.Unlock()

	if discovery != nil {
		if err := discovery.Stop(); err != nil {
			log.Printf("coordinator: error stopping discovery: %v", err)
		}
	}

	c.stopWakeEventListener()

	if arbitration != nil {
		arbitration.Clear()
	}

	log.Printf("coordinator: stopped")

	return nil
}

func (c *VoiceWakeCoordinator) eventLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-c.eventCh:
			if !ok {
				return
			}

			c.mu.Lock()
			listeners := make([]CoordinatorListener, len(c.listeners))
			copy(listeners, c.listeners)
			c.mu.Unlock()

			for _, listener := range listeners {
				listener(event)
			}
		}
	}
}

func (c *VoiceWakeCoordinator) onDiscoveryEvent(event DiscoveryEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch event.Type {
	case "device_discovered":
		c.stats.DevicesDiscovered++

	case "device_lost":
		c.stats.DevicesLost++
	}

	c.emitEvent(CoordinatorEvent{
		Type:      CoordinatorEventType(event.Type),
		Timestamp: event.Timestamp,
		Data: map[string]any{
			"device_id":   event.Device.ID,
			"device_name": event.Device.Name,
			"device_type": event.Device.Type,
			"ip_address":  event.Device.IPAddress,
		},
	})
}

func (c *VoiceWakeCoordinator) onArbitrationResult(result WakeArbitrationResult) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.stats.LastArbitration = result.DecidedAt

	if result.IsLocal {
		c.stats.ArbitrationsWon++

		c.emitEvent(CoordinatorEvent{
			Type:      CoordinatorEventArbitrationWon,
			Timestamp: result.DecidedAt,
			Data: map[string]any{
				"winner_id":   result.WinnerID,
				"winner_name": result.WinnerName,
				"confidence":  result.Confidence,
				"event_count": len(result.AllEvents),
			},
		})
	} else {
		c.stats.ArbitrationsLost++

		c.emitEvent(CoordinatorEvent{
			Type:      CoordinatorEventArbitrationLost,
			Timestamp: result.DecidedAt,
			Data: map[string]any{
				"winner_id":   result.WinnerID,
				"winner_name": result.WinnerName,
				"confidence":  result.Confidence,
			},
		})
	}
}

func (c *VoiceWakeCoordinator) onSuppressionEvent(event SuppressionEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch event.Type {
	case "suppressed":
		c.stats.Suppressions++

		c.emitEvent(CoordinatorEvent{
			Type:      CoordinatorEventSuppressed,
			Timestamp: event.Timestamp,
			Data: map[string]any{
				"device_id":   event.DeviceID,
				"device_name": event.DeviceName,
				"duration":    event.Duration,
				"remaining":   event.Remaining,
			},
		})

	case "released":
		c.stats.Releases++

		c.emitEvent(CoordinatorEvent{
			Type:      CoordinatorEventReleased,
			Timestamp: event.Timestamp,
			Data: map[string]any{
				"device_id":   event.DeviceID,
				"device_name": event.DeviceName,
			},
		})
	}
}

func (c *VoiceWakeCoordinator) emitEvent(event CoordinatorEvent) {
	select {
	case c.eventCh <- event:
	default:
	}
}

func (c *VoiceWakeCoordinator) SubmitWake(phrase string, confidence float64, energy float64, engine string) bool {
	c.mu.Lock()
	suppressor := c.suppressor
	localID := c.localID
	deviceName := c.cfg.DeviceName
	priority := c.cfg.DiscoveryConfig.Priority
	c.mu.Unlock()

	if suppressor.IsSuppressed() {
		return false
	}

	event := WakeEvent{
		DeviceID:   localID,
		DeviceName: deviceName,
		Phrase:     phrase,
		Confidence: confidence,
		Energy:     energy,
		Engine:     engine,
		Priority:   priority,
	}

	c.mu.Lock()
	c.stats.WakesSubmitted++
	c.stats.LastWakeTime = time.Now()
	c.mu.Unlock()

	c.arbitration.SubmitLocalWake(event)

	c.broadcastWakeEvent(event)

	return true
}

func (c *VoiceWakeCoordinator) broadcastWakeEvent(event WakeEvent) {
	c.mu.Lock()
	devices := c.discovery.GetDevices()
	c.mu.Unlock()

	for _, device := range devices {
		if device.ID == c.localID {
			continue
		}

		c.sendWakeToPeer(device, event)
	}
}

func (c *VoiceWakeCoordinator) ReceiveRemoteWake(event WakeEvent) {
	c.mu.Lock()
	suppressor := c.suppressor
	c.mu.Unlock()

	if suppressor.IsSuppressedBy(event.DeviceID) {
		return
	}

	c.arbitration.SubmitRemoteWake(event)
}

func (c *VoiceWakeCoordinator) IsSuppressed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.suppressor.IsSuppressed()
}

func (c *VoiceWakeCoordinator) SuppressionRemaining() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.suppressor.RemainingTime()
}

func (c *VoiceWakeCoordinator) RegisterListener(listener CoordinatorListener) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.listeners = append(c.listeners, listener)
}

func (c *VoiceWakeCoordinator) GetDevices() []DeviceInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.discovery.GetDevices()
}

func (c *VoiceWakeCoordinator) GetDeviceCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.discovery.GetDeviceCount()
}

func (c *VoiceWakeCoordinator) LocalDevice() DeviceInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.discovery.LocalDevice()
}

func (c *VoiceWakeCoordinator) Stats() CoordinatorStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stats
}

func (c *VoiceWakeCoordinator) IsRunning() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.isRunning
}

func (c *VoiceWakeCoordinator) Arbitration() *WakeArbitration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.arbitration
}

func (c *VoiceWakeCoordinator) Suppressor() *WakeSuppressor {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.suppressor
}

func (c *VoiceWakeCoordinator) Discovery() *DeviceDiscovery {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.discovery
}

func (c *VoiceWakeCoordinator) SetPriority(priority int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cfg.DiscoveryConfig.Priority = priority
	c.cfg.ArbitrationConfig.LocalPriority = priority

	if c.discovery != nil {
		c.discovery.SetPriority(priority)
	}
}

func (c *VoiceWakeCoordinator) SetElectionMode(mode ElectionMode) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cfg.ArbitrationConfig.ElectionMode = mode
	if c.arbitration != nil {
		c.arbitration.SetConfig(c.cfg.ArbitrationConfig)
	}
}

func (c *VoiceWakeCoordinator) SetPreferLocal(prefer bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cfg.ArbitrationConfig.PreferLocal = prefer
	if c.arbitration != nil {
		c.arbitration.SetConfig(c.cfg.ArbitrationConfig)
	}
}

func (c *VoiceWakeCoordinator) SetArbitrationWindow(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cfg.ArbitrationConfig.ArbitrationWindow = d
	if c.arbitration != nil {
		c.arbitration.SetConfig(c.cfg.ArbitrationConfig)
	}
}

func (c *VoiceWakeCoordinator) ProbeDevices() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.discovery != nil {
		c.discovery.Probe()
	}
}

func (c *VoiceWakeCoordinator) startWakeEventListener() (*net.UDPConn, chan struct{}, error) {
	port := c.cfg.WakeEventPort
	if port == 0 {
		port = 19877
	}

	addr := &net.UDPAddr{
		IP:   net.IPv4zero,
		Port: port,
	}

	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to bind wake event port %d: %w", port, err)
	}

	c.wakeConn = conn
	c.wakePort = port
	done := make(chan struct{})
	c.wakeListenDone = done

	return conn, done, nil
}

func (c *VoiceWakeCoordinator) stopWakeEventListener() {
	c.mu.Lock()
	conn := c.wakeConn
	done := c.wakeListenDone
	c.mu.Unlock()

	if conn != nil {
		_ = conn.Close()
	}

	if done != nil {
		<-done
	}

	c.mu.Lock()
	if c.wakeConn == conn {
		c.wakeConn = nil
	}
	if c.wakeListenDone == done {
		c.wakeListenDone = nil
	}
	c.mu.Unlock()
}

func (c *VoiceWakeCoordinator) wakeEventListenLoop(conn *net.UDPConn, done chan struct{}) {
	defer close(done)

	buf := make([]byte, 8192)

	for {
		c.mu.Lock()
		isRunning := c.isRunning
		c.mu.Unlock()

		if !isRunning || conn == nil {
			return
		}

		n, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			return
		}

		if n == 0 {
			continue
		}

		var msg wakeEventMessage
		if err := json.Unmarshal(buf[:n], &msg); err != nil {
			continue
		}

		if msg.Type != "wake_event" {
			continue
		}

		if msg.DeviceID == c.localID {
			continue
		}

		remoteEvent := WakeEvent{
			DeviceID:   msg.DeviceID,
			DeviceName: msg.DeviceName,
			Phrase:     msg.Phrase,
			Confidence: msg.Confidence,
			Energy:     msg.Energy,
			Timestamp:  msg.Timestamp,
			Engine:     msg.Engine,
			Priority:   msg.Priority,
		}

		log.Printf("coordinator: received remote wake from %s (%s) phrase=%q confidence=%.2f",
			remoteEvent.DeviceName, remoteAddr.IP.String(), remoteEvent.Phrase, remoteEvent.Confidence)

		c.ReceiveRemoteWake(remoteEvent)
	}
}

type wakeEventMessage struct {
	Type       string    `json:"type"`
	DeviceID   string    `json:"device_id"`
	DeviceName string    `json:"device_name"`
	Phrase     string    `json:"phrase"`
	Confidence float64   `json:"confidence"`
	Energy     float64   `json:"energy"`
	Timestamp  time.Time `json:"timestamp"`
	Engine     string    `json:"engine"`
	Priority   int       `json:"priority"`
}

func (c *VoiceWakeCoordinator) sendWakeToPeer(device DeviceInfo, event WakeEvent) {
	c.mu.Lock()
	conn := c.wakeConn
	c.mu.Unlock()

	if conn == nil {
		return
	}

	msg := wakeEventMessage{
		Type:       "wake_event",
		DeviceID:   event.DeviceID,
		DeviceName: event.DeviceName,
		Phrase:     event.Phrase,
		Confidence: event.Confidence,
		Energy:     event.Energy,
		Timestamp:  event.Timestamp,
		Engine:     event.Engine,
		Priority:   event.Priority,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("coordinator: failed to marshal wake event: %v", err)
		return
	}

	targetPort := device.Metadata["wake_port"]
	if targetPort == "" {
		targetPort = "19877"
	}

	port := 19877
	if p, ok := device.Metadata["wake_port"]; ok {
		fmt.Sscanf(p, "%d", &port)
	}

	targetAddr := &net.UDPAddr{
		IP:   net.ParseIP(device.IPAddress),
		Port: port,
	}

	conn.SetWriteDeadline(time.Now().Add(200 * time.Millisecond))
	_, err = conn.WriteToUDP(data, targetAddr)
	if err != nil {
		log.Printf("coordinator: failed to send wake to peer %s (%s): %v", device.Name, device.IPAddress, err)
		return
	}

	log.Printf("coordinator: sent wake event to peer %s (%s)", device.Name, device.IPAddress)
}
