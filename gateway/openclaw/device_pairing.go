package openclaw

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// DevicePairingManager 设备配对管理器
type DevicePairingManager struct {
	mu           sync.RWMutex
	authContext  *AuthContext
	broadcastMgr *BroadcastManager
}

// NewDevicePairingManager 创建设备配对管理器
func NewDevicePairingManager(authContext *AuthContext, broadcastMgr *BroadcastManager) *DevicePairingManager {
	return &DevicePairingManager{
		authContext:  authContext,
		broadcastMgr: broadcastMgr,
	}
}

// List 列出所有设备
func (dpm *DevicePairingManager) List() []*DevicePair {
	return dpm.authContext.ListDevicePairs()
}

// Get 获取设备
func (dpm *DevicePairingManager) Get(deviceID string) (*DevicePair, error) {
	pair, ok := dpm.authContext.GetDevicePair(deviceID)
	if !ok {
		return nil, fmt.Errorf("device not paired: %s", deviceID)
	}
	return pair, nil
}

// Approve 批准设备配对
func (dpm *DevicePairingManager) Approve(requestID string, roles, scopes []string, name string) error {
	dpm.mu.Lock()
	defer dpm.mu.Unlock()

	// 获取待处理的请求
	pendingPair, ok := dpm.authContext.GetPendingDevicePair(requestID)
	if !ok {
		return fmt.Errorf("pair request not found: %s", requestID)
	}

	if time.Now().After(pendingPair.ExpiresAt) {
		return fmt.Errorf("pair request expired")
	}

	// 使用请求者 ID 作为设备 ID
	deviceID := pendingPair.RequesterID
	if name == "" {
		name = pendingPair.Name
	}

	// 创建配对
	pair := &DevicePair{
		DeviceID:   deviceID,
		Name:       name,
		Roles:      roles,
		Scopes:     scopes,
		Metadata:   make(map[string]string),
		PairedAt:   time.Now(),
		LastSeenAt: time.Now(),
	}

	dpm.authContext.AddDevicePair(pair)
	dpm.authContext.RemovePendingDevicePair(requestID)

	// 广播配对解决
	_ = dpm.broadcastMgr.BroadcastDevicePairResolved(requestID, true, "")

	return nil
}

// Reject 拒绝设备配对
func (dpm *DevicePairingManager) Reject(requestID, reason string) error {
	dpm.mu.Lock()
	defer dpm.mu.Unlock()

	_, ok := dpm.authContext.GetPendingDevicePair(requestID)
	if !ok {
		return fmt.Errorf("pair request not found: %s", requestID)
	}

	dpm.authContext.RemovePendingDevicePair(requestID)

	// 广播配对解决
	_ = dpm.broadcastMgr.BroadcastDevicePairResolved(requestID, false, reason)

	return nil
}

// Remove 移除设备配对
func (dpm *DevicePairingManager) Remove(deviceID string) error {
	dpm.mu.Lock()
	defer dpm.mu.Unlock()

	if _, ok := dpm.authContext.GetDevicePair(deviceID); !ok {
		return fmt.Errorf("device not paired: %s", deviceID)
	}

	dpm.authContext.RemoveDevicePair(deviceID)

	return nil
}

// RotateToken 轮换设备 token
func (dpm *DevicePairingManager) RotateToken(deviceID string) (string, error) {
	dpm.mu.Lock()
	defer dpm.mu.Unlock()

	pair, ok := dpm.authContext.GetDevicePair(deviceID)
	if !ok {
		return "", fmt.Errorf("device not paired: %s", deviceID)
	}

	// 生成新 token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}
	newToken := hex.EncodeToString(tokenBytes)

	// 更新配对
	pair.PublicKey = newToken
	pair.LastSeenAt = time.Now()
	dpm.authContext.AddDevicePair(pair)

	return newToken, nil
}

// RevokeToken 撤销设备 token
func (dpm *DevicePairingManager) RevokeToken(deviceID string) error {
	dpm.mu.Lock()
	defer dpm.mu.Unlock()

	if _, ok := dpm.authContext.GetDevicePair(deviceID); !ok {
		return fmt.Errorf("device not paired: %s", deviceID)
	}

	// 移除设备配对（相当于撤销 token）
	dpm.authContext.RemoveDevicePair(deviceID)

	return nil
}

// UpdateLastSeen 更新最后活跃时间
func (dpm *DevicePairingManager) UpdateLastSeen(deviceID string) error {
	dpm.mu.Lock()
	defer dpm.mu.Unlock()

	pair, ok := dpm.authContext.GetDevicePair(deviceID)
	if !ok {
		return fmt.Errorf("device not paired: %s", deviceID)
	}

	pair.LastSeenAt = time.Now()
	dpm.authContext.AddDevicePair(pair)

	return nil
}

// GetByToken 根据 token 获取设备
func (dpm *DevicePairingManager) GetByToken(token string) (*DevicePair, error) {
	dpm.mu.RLock()
	defer dpm.mu.RUnlock()

	pairs := dpm.authContext.ListDevicePairs()
	for _, pair := range pairs {
		if pair.PublicKey == token {
			return pair, nil
		}
	}

	return nil, fmt.Errorf("device not found with token")
}

// CreatePendingRequest 创建待处理的配对请求
func (dpm *DevicePairingManager) CreatePendingRequest(deviceID, name string, metadata map[string]interface{}) (*PendingPair, error) {
	dpm.mu.Lock()
	defer dpm.mu.Unlock()

	requestID, err := GeneratePairID()
	if err != nil {
		return nil, err
	}

	pending := &PendingPair{
		ID:          requestID,
		RequesterID: deviceID,
		Name:        name,
		RequestedAt: time.Now(),
		ExpiresAt:   time.Now().Add(5 * time.Minute),
		Metadata:    metadata,
	}

	dpm.authContext.AddPendingDevicePair(pending)

	// 广播配对请求
	_ = dpm.broadcastMgr.BroadcastDevicePairRequested(requestID, deviceID, metadata)

	return pending, nil
}

// GetPendingRequests 获取待处理的配对请求
func (dpm *DevicePairingManager) GetPendingRequests() []*PendingPair {
	return dpm.authContext.ListPendingDevicePairs()
}

// GetPendingRequest 获取待处理的配对请求
func (dpm *DevicePairingManager) GetPendingRequest(requestID string) (*PendingPair, error) {
	pending, ok := dpm.authContext.GetPendingDevicePair(requestID)
	if !ok {
		return nil, fmt.Errorf("pair request not found: %s", requestID)
	}
	return pending, nil
}

// RegisterDevicePairingMethods 注册设备配对方法
func RegisterDevicePairingMethods(mh *MessageHandler, dpm *DevicePairingManager) {
	// device.pair.list
	mh.Register("device.pair.list", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		devices := dpm.List()
		return map[string]interface{}{
			"devices": devices,
			"count":   len(devices),
		}, nil
	})

	// device.pair.approve
	mh.Register("device.pair.approve", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			RequestID string   `json:"requestId"`
			Roles     []string `json:"roles,omitempty"`
			Scopes    []string `json:"scopes,omitempty"`
			Name      string   `json:"name,omitempty"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		if params.RequestID == "" {
			return nil, NewErrorInfo(ErrorInvalidParams, "requestId is required")
		}

		if len(params.Roles) == 0 {
			params.Roles = []string{RoleNode}
		}

		if len(params.Scopes) == 0 {
			params.Scopes = []string{ScopeRead, ScopeWrite}
		}

		if err := dpm.Approve(params.RequestID, params.Roles, params.Scopes, params.Name); err != nil {
			return nil, NewErrorInfo(ErrorInternalError, err.Error())
		}

		return map[string]interface{}{
			"status":    "approved",
			"requestId": params.RequestID,
		}, nil
	})

	// device.pair.reject
	mh.Register("device.pair.reject", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			RequestID string `json:"requestId"`
			Reason    string `json:"reason,omitempty"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		if params.RequestID == "" {
			return nil, NewErrorInfo(ErrorInvalidParams, "requestId is required")
		}

		if err := dpm.Reject(params.RequestID, params.Reason); err != nil {
			return nil, NewErrorInfo(ErrorInternalError, err.Error())
		}

		return map[string]interface{}{
			"status":    "rejected",
			"requestId": params.RequestID,
		}, nil
	})

	// device.pair.remove
	mh.Register("device.pair.remove", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			DeviceID string `json:"deviceId"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		if params.DeviceID == "" {
			return nil, NewErrorInfo(ErrorInvalidParams, "deviceId is required")
		}

		if err := dpm.Remove(params.DeviceID); err != nil {
			return nil, NewErrorInfo(ErrorInternalError, err.Error())
		}

		return map[string]interface{}{
			"status":   "removed",
			"deviceId": params.DeviceID,
		}, nil
	})

	// device.token.rotate
	mh.Register("device.token.rotate", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			DeviceID string `json:"deviceId"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		if params.DeviceID == "" {
			return nil, NewErrorInfo(ErrorInvalidParams, "deviceId is required")
		}

		newToken, err := dpm.RotateToken(params.DeviceID)
		if err != nil {
			return nil, NewErrorInfo(ErrorInternalError, err.Error())
		}

		return map[string]interface{}{
			"status":   "rotated",
			"deviceId": params.DeviceID,
			"token":    newToken,
		}, nil
	})

	// device.token.revoke
	mh.Register("device.token.revoke", func(conn *Connection, req *Request) (interface{}, *ErrorInfo) {
		var params struct {
			DeviceID string `json:"deviceId"`
		}
		if err := parseParams(req.Params, &params); err != nil {
			return nil, NewErrorInfo(ErrorInvalidParams, err.Error())
		}

		if params.DeviceID == "" {
			return nil, NewErrorInfo(ErrorInvalidParams, "deviceId is required")
		}

		if err := dpm.RevokeToken(params.DeviceID); err != nil {
			return nil, NewErrorInfo(ErrorInternalError, err.Error())
		}

		return map[string]interface{}{
			"status":   "revoked",
			"deviceId": params.DeviceID,
		}, nil
	})
}
