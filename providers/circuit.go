package providers

import (
	"sync"
	"time"
)

// CircuitState 断路器状态
type CircuitState int

const (
	// CircuitStateClosed 断路器关闭（正常）
	CircuitStateClosed CircuitState = iota
	// CircuitStateOpen 断路器打开（故障）
	CircuitStateOpen
	// CircuitStateHalfOpen 半开（尝试恢复）
	CircuitStateHalfOpen
)

func (s CircuitState) String() string {
	switch s {
	case CircuitStateClosed:
		return "closed"
	case CircuitStateOpen:
		return "open"
	case CircuitStateHalfOpen:
		return "half_open"
	default:
		return "unknown"
	}
}

// CircuitBreaker 断路器
type CircuitBreaker struct {
	// 失败阈值
	failureThreshold int
	// 超时时间（打开后多久进入半开状态）
	timeout time.Duration
	// 重置时间（半开后多久关闭）
	resetTimeout time.Duration

	// 当前状态
	state CircuitState
	// 失败计数
	failures int
	// 上次状态变更时间
	lastStateChange time.Time
	// 成功计数（半开状态使用）
	successCount int

	mu sync.RWMutex
}

// NewCircuitBreaker 创建断路器
func NewCircuitBreaker(failureThreshold int, timeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		failureThreshold: failureThreshold,
		timeout:          timeout,
		resetTimeout:     30 * time.Second,
		state:            CircuitStateClosed,
		lastStateChange:  time.Now(),
	}
}

// IsOpen 检查断路器是否打开
func (cb *CircuitBreaker) IsOpen() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return cb.state == CircuitStateOpen
}

// RecordSuccess 记录成功
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()

	switch cb.state {
	case CircuitStateClosed:
		// 关闭状态，重置失败计数
		cb.failures = 0
	case CircuitStateHalfOpen:
		// 半开状态，增加成功计数
		cb.successCount++
		if cb.successCount >= 3 {
			// 连续成功，关闭断路器
			cb.setState(CircuitStateClosed, now)
			cb.failures = 0
			cb.successCount = 0
		}
	case CircuitStateOpen:
		// 打开状态，尝试进入半开
		if now.Sub(cb.lastStateChange) > cb.timeout {
			cb.setState(CircuitStateHalfOpen, now)
			cb.successCount = 0
		}
	}
}

// RecordFailure 记录失败
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()

	switch cb.state {
	case CircuitStateClosed:
		// 关闭状态，增加失败计数
		cb.failures++
		if cb.failures >= cb.failureThreshold {
			// 达到阈值，打开断路器
			cb.setState(CircuitStateOpen, now)
		}
	case CircuitStateHalfOpen:
		// 半开状态失败，重新打开
		cb.setState(CircuitStateOpen, now)
		cb.successCount = 0
	case CircuitStateOpen:
		// 已经打开，保持状态
		// 更新最后状态变更时间
		cb.lastStateChange = now
	}
}

// setState 设置状态
func (cb *CircuitBreaker) setState(state CircuitState, when time.Time) {
	cb.state = state
	cb.lastStateChange = when

	// 状态变更时的处理
	if state == CircuitStateClosed {
		cb.failures = 0
		cb.successCount = 0
	}
}

// GetState 获取当前状态
func (cb *CircuitBreaker) GetState() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// GetStateInfo 获取状态信息
func (cb *CircuitBreaker) GetStateInfo() map[string]interface{} {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	now := time.Now()
	timeSinceChange := now.Sub(cb.lastStateChange)

	return map[string]interface{}{
		"state":             cb.state.String(),
		"failures":          cb.failures,
		"success_count":     cb.successCount,
		"last_state_change": cb.lastStateChange,
		"time_since_change": timeSinceChange.String(),
		"is_open":           cb.state == CircuitStateOpen,
	}
}

// Reset 重置断路器
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = CircuitStateClosed
	cb.failures = 0
	cb.successCount = 0
	cb.lastStateChange = time.Now()
}

// AllowRequest 检查是否允许请求
func (cb *CircuitBreaker) AllowRequest() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	now := time.Now()

	switch cb.state {
	case CircuitStateClosed:
		return true
	case CircuitStateOpen:
		// 检查是否可以进入半开状态
		return now.Sub(cb.lastStateChange) > cb.timeout
	case CircuitStateHalfOpen:
		return true
	default:
		return false
	}
}
