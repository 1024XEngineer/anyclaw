package gateway

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

type PairingCode struct {
	Code       string    `json:"code"`
	CreatedAt  time.Time `json:"created_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	DeviceID   string    `json:"device_id,omitempty"`
	DeviceName string    `json:"device_name,omitempty"`
	DeviceType string    `json:"device_type,omitempty"`
	Used       bool      `json:"used"`
	UsedAt     time.Time `json:"used_at,omitempty"`
	Token      string    `json:"token,omitempty"`
}

type DevicePairing struct {
	mu        sync.RWMutex
	codes     map[string]*PairingCode
	pairings  map[string]*DevicePairingInfo
	ttl       time.Duration
	maxActive int
	allowList []string
	enabled   bool
}

type DevicePairingInfo struct {
	DeviceID    string    `json:"device_id"`
	DeviceName  string    `json:"device_name"`
	DeviceType  string    `json:"device_type"`
	PairedAt    time.Time `json:"paired_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	Token       string    `json:"token"`
	Permissions []string  `json:"permissions"`
	LastSeen    time.Time `json:"last_seen"`
	Status      string    `json:"status"`
}

func NewDevicePairing(ttlHours int) *DevicePairing {
	ttl := time.Hour * time.Duration(ttlHours)
	if ttl == 0 {
		ttl = 24 * time.Hour
	}
	return &DevicePairing{
		codes:     make(map[string]*PairingCode),
		pairings:  make(map[string]*DevicePairingInfo),
		ttl:       ttl,
		maxActive: 10,
		enabled:   true,
	}
}

func (dp *DevicePairing) SetEnabled(enabled bool) {
	dp.mu.Lock()
	defer dp.mu.Unlock()
	dp.enabled = enabled
}

func (dp *DevicePairing) IsEnabled() bool {
	dp.mu.RLock()
	defer dp.mu.RUnlock()
	return dp.enabled
}

func (dp *DevicePairing) SetAllowList(allowed []string) {
	dp.mu.Lock()
	defer dp.mu.Unlock()
	dp.allowList = allowed
}

func (dp *DevicePairing) GeneratePairingCode(deviceName, deviceType string) (*PairingCode, error) {
	dp.mu.Lock()
	defer dp.mu.Unlock()

	if !dp.enabled {
		return nil, fmt.Errorf("pairing is disabled")
	}

	if len(dp.allowList) > 0 {
		allowed := false
		for _, d := range dp.allowList {
			if d == deviceType || d == deviceName {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, fmt.Errorf("device type not allowed: %s", deviceType)
		}
	}

	if len(dp.pairings) >= dp.maxActive {
		return nil, fmt.Errorf("maximum paired devices reached: %d", dp.maxActive)
	}

	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return nil, fmt.Errorf("failed to generate random bytes: %w", err)
	}

	hash := sha256.Sum256(bytes)
	code := hex.EncodeToString(hash[:])[:8]

	codeObj := &PairingCode{
		Code:       code,
		CreatedAt:  time.Now(),
		ExpiresAt:  time.Now().Add(dp.ttl),
		DeviceName: deviceName,
		DeviceType: deviceType,
		Used:       false,
	}

	dp.codes[code] = codeObj
	return codeObj, nil
}

func (dp *DevicePairing) ValidatePairingCode(code string) (*PairingCode, error) {
	dp.mu.RLock()
	codeObj, ok := dp.codes[code]
	dp.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("invalid pairing code")
	}

	if codeObj.Used {
		return nil, fmt.Errorf("pairing code already used")
	}

	if time.Now().After(codeObj.ExpiresAt) {
		return nil, fmt.Errorf("pairing code expired")
	}

	return codeObj, nil
}

func (dp *DevicePairing) CompletePairing(code string, deviceID string) (*DevicePairingInfo, error) {
	dp.mu.Lock()
	defer dp.mu.Unlock()

	codeObj, ok := dp.codes[code]
	if !ok {
		return nil, fmt.Errorf("invalid pairing code")
	}

	if codeObj.Used {
		return nil, fmt.Errorf("pairing code already used")
	}

	if time.Now().After(codeObj.ExpiresAt) {
		return nil, fmt.Errorf("pairing code expired")
	}

	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}
	token := base64.StdEncoding.EncodeToString(tokenBytes)

	pairingInfo := &DevicePairingInfo{
		DeviceID:    deviceID,
		DeviceName:  codeObj.DeviceName,
		DeviceType:  codeObj.DeviceType,
		PairedAt:    time.Now(),
		ExpiresAt:   time.Now().Add(dp.ttl),
		Token:       token,
		Permissions: []string{"chat.send", "chat.history", "status.read"},
		LastSeen:    time.Now(),
		Status:      "paired",
	}

	codeObj.Used = true
	codeObj.UsedAt = time.Now()
	codeObj.DeviceID = deviceID
	codeObj.Token = token

	dp.pairings[deviceID] = pairingInfo
	return pairingInfo, nil
}

func (dp *DevicePairing) GetPairing(deviceID string) (*DevicePairingInfo, bool) {
	dp.mu.RLock()
	defer dp.mu.RUnlock()
	p, ok := dp.pairings[deviceID]
	return p, ok
}

func (dp *DevicePairing) ValidateToken(deviceID, token string) bool {
	dp.mu.RLock()
	defer dp.mu.RUnlock()

	p, ok := dp.pairings[deviceID]
	if !ok {
		return false
	}

	if p.Status == "unpaired" {
		return false
	}

	if time.Now().After(p.ExpiresAt) {
		return false
	}

	return p.Token == token
}

func (dp *DevicePairing) Unpair(deviceID string) error {
	dp.mu.Lock()
	defer dp.mu.Unlock()

	if _, ok := dp.pairings[deviceID]; !ok {
		return fmt.Errorf("device not paired: %s", deviceID)
	}

	delete(dp.pairings, deviceID)
	return nil
}

func (dp *DevicePairing) ListPaired() []*DevicePairingInfo {
	dp.mu.RLock()
	defer dp.mu.RUnlock()

	var list []*DevicePairingInfo
	for _, p := range dp.pairings {
		list = append(list, p)
	}
	return list
}

func (dp *DevicePairing) RenewPairing(deviceID string) (*DevicePairingInfo, error) {
	dp.mu.Lock()
	defer dp.mu.Unlock()

	p, ok := dp.pairings[deviceID]
	if !ok {
		return nil, fmt.Errorf("device not paired: %s", deviceID)
	}

	p.ExpiresAt = time.Now().Add(dp.ttl)
	p.LastSeen = time.Now()
	return p, nil
}

func (dp *DevicePairing) UpdateLastSeen(deviceID string) error {
	dp.mu.Lock()
	defer dp.mu.Unlock()

	p, ok := dp.pairings[deviceID]
	if !ok {
		return fmt.Errorf("device not paired: %s", deviceID)
	}

	p.LastSeen = time.Now()
	return nil
}

func (dp *DevicePairing) GetStatus() map[string]any {
	dp.mu.RLock()
	defer dp.mu.RUnlock()

	active := 0
	expired := 0
	for _, p := range dp.pairings {
		if time.Now().Before(p.ExpiresAt) {
			active++
		} else {
			expired++
		}
	}

	activeCodes := 0
	for _, c := range dp.codes {
		if !c.Used && time.Now().Before(c.ExpiresAt) {
			activeCodes++
		}
	}

	return map[string]any{
		"enabled":     dp.enabled,
		"max_devices": dp.maxActive,
		"paired":      len(dp.pairings),
		"active":      active,
		"expired":     expired,
		"codes":       activeCodes,
	}
}

func (dp *DevicePairing) Cleanup() {
	dp.mu.Lock()
	defer dp.mu.Unlock()

	for code, c := range dp.codes {
		if c.Used || time.Now().After(c.ExpiresAt) {
			delete(dp.codes, code)
		}
	}

	for id, p := range dp.pairings {
		if time.Now().After(p.ExpiresAt) {
			delete(dp.pairings, id)
		}
	}
}

type PairingRequest struct {
	Action      string   `json:"action"` // "generate", "validate", "pair", "unpair", "list", "status"
	Code        string   `json:"code,omitempty"`
	DeviceID    string   `json:"device_id,omitempty"`
	DeviceName  string   `json:"device_name,omitempty"`
	DeviceType  string   `json:"device_type,omitempty"`
	Token       string   `json:"token,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
	TTLHours    int      `json:"ttl_hours,omitempty"`
}

type PairingResponse struct {
	OK      bool                 `json:"ok"`
	Code    string               `json:"code,omitempty"`
	Device  *DevicePairingInfo   `json:"device,omitempty"`
	Devices []*DevicePairingInfo `json:"devices,omitempty"`
	Status  map[string]any       `json:"status,omitempty"`
	Error   string               `json:"error,omitempty"`
}

func (dp *DevicePairing) HandleRequest(ctx context.Context, req PairingRequest) (*PairingResponse, error) {
	switch req.Action {
	case "generate":
		code, err := dp.GeneratePairingCode(req.DeviceName, req.DeviceType)
		if err != nil {
			return &PairingResponse{OK: false, Error: err.Error()}, nil
		}
		return &PairingResponse{
			OK:   true,
			Code: code.Code,
			Status: map[string]any{
				"expires_at": code.ExpiresAt.Format(time.RFC3339),
			},
		}, nil

	case "validate":
		code, err := dp.ValidatePairingCode(req.Code)
		if err != nil {
			return &PairingResponse{OK: false, Error: err.Error()}, nil
		}
		return &PairingResponse{
			OK: true,
			Status: map[string]any{
				"valid":       true,
				"device_name": code.DeviceName,
				"device_type": code.DeviceType,
				"expires_at":  code.ExpiresAt.Format(time.RFC3339),
			},
		}, nil

	case "pair":
		if req.DeviceID == "" || req.Token == "" {
			return &PairingResponse{OK: false, Error: "device_id and token required"}, nil
		}
		validated, err := dp.ValidatePairingCode(req.Code)
		if err != nil {
			return &PairingResponse{OK: false, Error: err.Error()}, nil
		}
		if validated.DeviceName != req.DeviceName {
			validated.DeviceName = req.DeviceName
		}
		pairing, err := dp.CompletePairing(req.Code, req.DeviceID)
		if err != nil {
			return &PairingResponse{OK: false, Error: err.Error()}, nil
		}
		return &PairingResponse{OK: true, Device: pairing}, nil

	case "unpair":
		if req.DeviceID == "" {
			return &PairingResponse{OK: false, Error: "device_id required"}, nil
		}
		if err := dp.Unpair(req.DeviceID); err != nil {
			return &PairingResponse{OK: false, Error: err.Error()}, nil
		}
		return &PairingResponse{OK: true}, nil

	case "list":
		devices := dp.ListPaired()
		return &PairingResponse{OK: true, Devices: devices}, nil

	case "status":
		return &PairingResponse{OK: true, Status: dp.GetStatus()}, nil

	case "renew":
		if req.DeviceID == "" {
			return &PairingResponse{OK: false, Error: "device_id required"}, nil
		}
		pairing, err := dp.RenewPairing(req.DeviceID)
		if err != nil {
			return &PairingResponse{OK: false, Error: err.Error()}, nil
		}
		return &PairingResponse{OK: true, Device: pairing}, nil

	default:
		return nil, fmt.Errorf("unknown action: %s", req.Action)
	}
}
