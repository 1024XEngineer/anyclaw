package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	mdnsAddr      = "224.0.0.251:5353"
	mdnsPort      = 5353
	serviceType   = "_anyclaw._tcp.local."
	announceEvery = 4 * time.Second
	staleTimeout  = 15 * time.Second
)

type Instance struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Version  string            `json:"version"`
	Host     string            `json:"host"`
	Port     int               `json:"port"`
	Address  string            `json:"address"`
	URL      string            `json:"url"`
	Caps     []string          `json:"capabilities"`
	LastSeen time.Time         `json:"last_seen"`
	IsSelf   bool              `json:"is_self"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type Config struct {
	ServiceName      string
	ServicePort      int
	InstanceID       string
	Version          string
	Capabilities     []string
	Metadata         map[string]string
	AnnounceInterval time.Duration
}

type Service struct {
	mu         sync.RWMutex
	config     Config
	conn       *net.UDPConn
	instances  map[string]*Instance
	self       *Instance
	onDiscover func(instance *Instance)
	stopCh     chan struct{}
}

func NewService(cfg Config) *Service {
	if cfg.AnnounceInterval <= 0 {
		cfg.AnnounceInterval = announceEvery
	}
	s := &Service{
		config:    cfg,
		instances: make(map[string]*Instance),
		stopCh:    make(chan struct{}),
	}
	s.self = &Instance{
		ID:       cfg.InstanceID,
		Name:     cfg.ServiceName,
		Version:  cfg.Version,
		Port:     cfg.ServicePort,
		Caps:     cfg.Capabilities,
		Metadata: cfg.Metadata,
		IsSelf:   true,
		LastSeen: time.Now().UTC(),
	}
	return s
}

func (s *Service) OnDiscover(fn func(instance *Instance)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onDiscover = fn
}

func (s *Service) Start(ctx context.Context) error {
	mcastAddr, err := net.ResolveUDPAddr("udp4", mdnsAddr)
	if err != nil {
		return fmt.Errorf("resolve multicast addr: %w", err)
	}

	conn, err := net.ListenMulticastUDP("udp4", nil, mcastAddr)
	if err != nil {
		bindAddr, _ := net.ResolveUDPAddr("udp4", fmt.Sprintf(":%d", mdnsPort))
		conn, err = net.ListenUDP("udp4", bindAddr)
		if err != nil {
			return fmt.Errorf("listen udp: %w", err)
		}
	}
	s.conn = conn

	s.updateSelfAddress()

	go s.announceLoop(ctx)
	go s.listenLoop(ctx)
	go s.pruneLoop(ctx)

	return nil
}

func (s *Service) Stop() {
	close(s.stopCh)
	if s.conn != nil {
		s.conn.Close()
	}
}

func (s *Service) Instances() []*Instance {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Instance, 0, len(s.instances)+1)
	if s.self != nil {
		result = append(result, s.self)
	}
	for _, inst := range s.instances {
		result = append(result, inst)
	}
	return result
}

func (s *Service) Self() *Instance {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.self
}

func (s *Service) announceLoop(ctx context.Context) {
	s.announce()

	ticker := time.NewTicker(s.config.AnnounceInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.announce()
		}
	}
}

func (s *Service) announce() {
	s.updateSelfAddress()

	msg := mdnsMessage{
		Type: "announce",
		Service: mdnsService{
			Name:     s.config.ServiceName,
			Type:     serviceType,
			Host:     s.self.Address,
			Port:     s.config.ServicePort,
			ID:       s.config.InstanceID,
			Version:  s.config.Version,
			Caps:     s.config.Capabilities,
			Metadata: s.config.Metadata,
		},
	}

	data, _ := json.Marshal(msg)
	s.sendBroadcast(data)
}

func (s *Service) listenLoop(ctx context.Context) {
	buf := make([]byte, 4096)
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		default:
		}

		s.conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, _, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			continue
		}

		var msg mdnsMessage
		if err := json.Unmarshal(buf[:n], &msg); err != nil {
			continue
		}

		if msg.Type == "announce" || msg.Type == "response" {
			s.handleAnnounce(&msg)
		} else if msg.Type == "query" {
			s.handleQuery(&msg)
		}
	}
}

func (s *Service) handleAnnounce(msg *mdnsMessage) {
	if msg.Service.ID == s.config.InstanceID {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	inst, exists := s.instances[msg.Service.ID]
	if !exists {
		inst = &Instance{
			ID:       msg.Service.ID,
			Name:     msg.Service.Name,
			Version:  msg.Service.Version,
			Host:     msg.Service.Host,
			Port:     msg.Service.Port,
			Address:  msg.Service.Host,
			URL:      fmt.Sprintf("http://%s:%d", msg.Service.Host, msg.Service.Port),
			Caps:     msg.Service.Caps,
			Metadata: msg.Service.Metadata,
			IsSelf:   false,
		}
		s.instances[msg.Service.ID] = inst

		if s.onDiscover != nil {
			go s.onDiscover(inst)
		}
	}

	inst.LastSeen = time.Now().UTC()
	inst.Name = msg.Service.Name
	inst.Version = msg.Service.Version
	inst.Host = msg.Service.Host
	inst.Port = msg.Service.Port
	inst.Address = msg.Service.Host
	inst.URL = fmt.Sprintf("http://%s:%d", msg.Service.Host, msg.Service.Port)
	inst.Caps = msg.Service.Caps
	inst.Metadata = msg.Service.Metadata
}

func (s *Service) handleQuery(msg *mdnsMessage) {
	if msg.Query != serviceType && msg.Query != "" {
		return
	}

	s.updateSelfAddress()

	resp := mdnsMessage{
		Type: "response",
		Service: mdnsService{
			Name:     s.config.ServiceName,
			Type:     serviceType,
			Host:     s.self.Address,
			Port:     s.config.ServicePort,
			ID:       s.config.InstanceID,
			Version:  s.config.Version,
			Caps:     s.config.Capabilities,
			Metadata: s.config.Metadata,
		},
	}

	data, _ := json.Marshal(resp)
	s.sendBroadcast(data)
}

func (s *Service) pruneLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.pruneStale()
		}
	}
}

func (s *Service) pruneStale() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	for id, inst := range s.instances {
		if now.Sub(inst.LastSeen) > staleTimeout {
			delete(s.instances, id)
		}
	}
}

func (s *Service) sendBroadcast(data []byte) {
	if s.conn == nil {
		return
	}
	addr, _ := net.ResolveUDPAddr("udp4", mdnsAddr)
	s.conn.WriteToUDP(data, addr)
}

func (s *Service) updateSelfAddress() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.self.Address = getLocalIP()
	s.self.URL = fmt.Sprintf("http://%s:%d", s.self.Address, s.config.ServicePort)
	s.self.LastSeen = time.Now().UTC()
}

func (s *Service) SendQuery() {
	msg := mdnsMessage{
		Type:  "query",
		Query: serviceType,
	}
	data, _ := json.Marshal(msg)
	s.sendBroadcast(data)
}

type mdnsMessage struct {
	Type    string      `json:"t"`
	Service mdnsService `json:"s,omitempty"`
	Query   string      `json:"q,omitempty"`
}

type mdnsService struct {
	Name     string            `json:"n"`
	Type     string            `json:"t"`
	Host     string            `json:"h"`
	Port     int               `json:"p"`
	ID       string            `json:"i"`
	Version  string            `json:"v"`
	Caps     []string          `json:"c"`
	Metadata map[string]string `json:"m,omitempty"`
}

func getLocalIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "127.0.0.1"
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				if ipnet.IP.To4() != nil {
					return ipnet.IP.String()
				}
			}
		}
	}

	hostname, _ := os.Hostname()
	addrs, _ := net.LookupHost(hostname)
	for _, addr := range addrs {
		if ip := net.ParseIP(addr); ip != nil && ip.To4() != nil && !ip.IsLoopback() {
			return ip.String()
		}
	}

	return "127.0.0.1"
}

func GetLocalIP() string {
	return getLocalIP()
}

func GetLocalIPs() []string {
	var ips []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return ips
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				if ipnet.IP.To4() != nil {
					ips = append(ips, ipnet.IP.String())
				}
			}
		}
	}
	return ips
}

func IsLocalhost(host string) bool {
	return host == "127.0.0.1" || host == "localhost" || strings.HasPrefix(host, "127.")
}
