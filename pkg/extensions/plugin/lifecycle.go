package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// LifecycleManager 插件生命周期管理器
type LifecycleManager struct {
	plugins             map[string]*PluginLifecycle
	registry            *Registry
	capabilityIndex     *CapabilityIndex
	healthCheckInterval time.Duration
	metricsStore        MetricsStore
	trustStore          *TrustStore
}

// MetricsStore 指标存储
type MetricsStore interface {
	RecordExecution(pluginID string, success bool, duration time.Duration)
	GetMetrics(pluginID string) PluginMetrics
}

// NewLifecycleManager 创建生命周期管理器
func NewLifecycleManager(registry *Registry) *LifecycleManager {
	return &LifecycleManager{
		plugins:             make(map[string]*PluginLifecycle),
		registry:            registry,
		capabilityIndex:     NewCapabilityIndex(),
		healthCheckInterval: 5 * time.Minute,
	}
}

// SetTrustStore 设置信任存储
func (lm *LifecycleManager) SetTrustStore(ts *TrustStore) {
	lm.trustStore = ts
}

// GetCapabilityIndex 获取能力索引
func (lm *LifecycleManager) GetCapabilityIndex() *CapabilityIndex {
	return lm.capabilityIndex
}

// DiscoverPlugins 发现插件
func (lm *LifecycleManager) DiscoverPlugins(ctx context.Context, pluginDir string) ([]*ManifestV2, error) {
	var manifests []*ManifestV2

	// 扫描插件目录
	err := filepath.Walk(pluginDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 查找 plugin.json 文件
		if info.Name() == "plugin.json" {
			manifest, err := lm.LoadManifestV2(path)
			if err != nil {
				return fmt.Errorf("failed to load manifest from %s: %v", path, err)
			}

			// 验证manifest
			if err := manifest.Validate(); err != nil {
				return fmt.Errorf("manifest validation failed for %s: %v", path, err)
			}

			// 设置生命周期状态
			lifecycle := &PluginLifecycle{
				State:    PluginStateDiscovered,
				Manifest: manifest,
				LoadedAt: time.Now(),
			}
			lm.plugins[manifest.PluginID] = lifecycle

			manifests = append(manifests, manifest)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("plugin discovery failed: %v", err)
	}

	return manifests, nil
}

// LoadManifestV2 加载 Manifest V2
func (lm *LifecycleManager) LoadManifestV2(path string) (*ManifestV2, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest file: %v", err)
	}

	var manifest ManifestV2
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest JSON: %v", err)
	}

	// 设置路径信息
	manifest.sourceDir = filepath.Dir(path)
	manifest.manifestPath = path

	return &manifest, nil
}

// VerifyPlugin 验证插件
func (lm *LifecycleManager) VerifyPlugin(ctx context.Context, pluginID string) error {
	lifecycle, ok := lm.plugins[pluginID]
	if !ok {
		return fmt.Errorf("plugin %s not found", pluginID)
	}

	if lm.registry.requireTrust {
		if lifecycle.Manifest.Signature == "" {
			return fmt.Errorf("plugin %s: signature required but missing", pluginID)
		}
		if lifecycle.Manifest.Signer == "" {
			return fmt.Errorf("plugin %s: signer required but missing", pluginID)
		}
		v1Manifest := lifecycle.Manifest.ConvertToV1()
		if !lm.registry.isTrusted(*v1Manifest) {
			return fmt.Errorf("plugin %s: signer %q is not trusted", pluginID, lifecycle.Manifest.Signer)
		}
		if lifecycle.Manifest.sourceDir != "" {
			var sig Signature
			if err := json.Unmarshal([]byte(lifecycle.Manifest.Signature), &sig); err != nil {
				return fmt.Errorf("plugin %s: invalid signature format: %w", pluginID, err)
			}
			if lm.trustStore != nil {
				signer, trusted := lm.trustStore.GetSigner(sig.KeyID)
				if !trusted && !lm.trustStore.IsTrusted(sig.KeyID) {
					return fmt.Errorf("plugin %s: key %q not in trust store", pluginID, sig.KeyID)
				}
				if signer != nil && signer.TrustLevel == TrustLevelUntrusted {
					return fmt.Errorf("plugin %s: signer %q is explicitly untrusted", pluginID, signer.Name)
				}
			}
			ok, err := VerifySignature(v1Manifest, &sig, "")
			if err != nil {
				return fmt.Errorf("plugin %s: signature verification failed: %w", pluginID, err)
			}
			if !ok {
				return fmt.Errorf("plugin %s: signature verification returned false", pluginID)
			}
		}
	}

	if err := lm.CheckPluginHealth(ctx, pluginID); err != nil {
		return fmt.Errorf("plugin health check failed: %v", err)
	}

	lifecycle.State = PluginStateVerified
	return nil
}

// LoadPlugin 加载插件
func (lm *LifecycleManager) LoadPlugin(ctx context.Context, pluginID string) error {
	lifecycle, ok := lm.plugins[pluginID]
	if !ok {
		return fmt.Errorf("plugin %s not found", pluginID)
	}

	if lifecycle.State != PluginStateVerified {
		return fmt.Errorf("plugin must be verified before loading")
	}

	if err := lm.executeHook(ctx, pluginID, HookBeforeLoad); err != nil {
		return fmt.Errorf("before_load hook failed: %v", err)
	}

	lifecycle.State = PluginStateLoaded

	if err := lm.executeHook(ctx, pluginID, HookAfterLoad); err != nil {
		return fmt.Errorf("after_load hook failed: %v", err)
	}

	return nil
}

// IndexPlugin 索引插件
func (lm *LifecycleManager) IndexPlugin(ctx context.Context, pluginID string) error {
	lifecycle, ok := lm.plugins[pluginID]
	if !ok {
		return fmt.Errorf("plugin %s not found", pluginID)
	}

	if lifecycle.State != PluginStateLoaded {
		return fmt.Errorf("plugin must be loaded before indexing")
	}

	if lifecycle.Manifest == nil {
		return fmt.Errorf("plugin manifest not loaded")
	}

	if err := lm.capabilityIndex.Index(lifecycle.Manifest); err != nil {
		return fmt.Errorf("failed to index plugin capabilities: %v", err)
	}

	lifecycle.State = PluginStateIndexed
	return nil
}

// BindPlugin 绑定插件到会话
func (lm *LifecycleManager) BindPlugin(ctx context.Context, pluginID string, sessionID string) error {
	lifecycle, ok := lm.plugins[pluginID]
	if !ok {
		return fmt.Errorf("plugin %s not found", pluginID)
	}

	if lifecycle.State != PluginStateIndexed {
		return fmt.Errorf("plugin must be indexed before binding")
	}

	if lifecycle.Sessions == nil {
		lifecycle.Sessions = make(map[string]*PluginSession)
	}

	lifecycle.Sessions[sessionID] = &PluginSession{
		SessionID: sessionID,
		BoundAt:   time.Now(),
		Context:   make(map[string]any),
	}

	lifecycle.State = PluginStateBound
	return nil
}

// ExecutePlugin 执行插件
func (lm *LifecycleManager) ExecutePlugin(ctx context.Context, pluginID string, action string, inputs map[string]any) (map[string]any, error) {
	lifecycle, ok := lm.plugins[pluginID]
	if !ok {
		return nil, fmt.Errorf("plugin %s not found", pluginID)
	}

	if lifecycle.State != PluginStateBound && lifecycle.State != PluginStateExecuting {
		return nil, fmt.Errorf("plugin must be bound before execution")
	}

	startTime := time.Now()

	if err := lm.executeHook(ctx, pluginID, HookBeforeExecute); err != nil {
		return nil, fmt.Errorf("before_execute hook failed: %v", err)
	}

	lifecycle.State = PluginStateExecuting

	outputs, err := lm.runPluginAction(ctx, pluginID, action, inputs)

	if err != nil {
		duration := time.Since(startTime)
		lifecycle.Metrics.ExecutionCount++
		lifecycle.Metrics.FailureCount++
		lifecycle.Metrics.AverageTime = time.Duration((int64(lifecycle.Metrics.AverageTime)*int64(lifecycle.Metrics.ExecutionCount-1) + int64(duration)) / int64(lifecycle.Metrics.ExecutionCount))
		lifecycle.Metrics.LastExecuted = time.Now()
		lifecycle.State = PluginStateBound

		if lm.metricsStore != nil {
			lm.metricsStore.RecordExecution(pluginID, false, duration)
		}
		return nil, &PluginExecutionError{
			PluginID:  pluginID,
			Action:    action,
			Reason:    err.Error(),
			Retryable: lifecycle.Manifest.RetryPolicy != nil,
		}
	}

	if err := lm.executeHook(ctx, pluginID, HookAfterExecute); err != nil {
		return nil, fmt.Errorf("after_execute hook failed: %v", err)
	}

	duration := time.Since(startTime)
	lifecycle.Metrics.ExecutionCount++
	lifecycle.Metrics.SuccessCount++
	lifecycle.Metrics.AverageTime = time.Duration((int64(lifecycle.Metrics.AverageTime)*int64(lifecycle.Metrics.ExecutionCount-1) + int64(duration)) / int64(lifecycle.Metrics.ExecutionCount))
	lifecycle.Metrics.LastExecuted = time.Now()
	lifecycle.LastUsed = time.Now()
	lifecycle.State = PluginStateBound

	if lm.metricsStore != nil {
		lm.metricsStore.RecordExecution(pluginID, true, duration)
	}

	return outputs, nil
}

func (lm *LifecycleManager) runPluginAction(ctx context.Context, pluginID string, action string, inputs map[string]any) (map[string]any, error) {
	lifecycle := lm.plugins[pluginID]
	manifest := lifecycle.Manifest

	if manifest == nil {
		return nil, fmt.Errorf("plugin manifest not loaded")
	}

	v1 := manifest.ConvertToV1()

	if v1.Entrypoint != "" {
		timeout := time.Duration(manifest.TimeoutSeconds) * time.Second
		if timeout == 0 {
			timeout = 30 * time.Second
		}
		cmdCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		cmd := exec.CommandContext(cmdCtx, v1.Entrypoint, action)
		cmd.Dir = manifest.sourceDir

		if len(inputs) > 0 {
			inputData, _ := json.Marshal(inputs)
			cmd.Stdin = bytes.NewReader(inputData)
		}

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			if stderr.Len() > 0 {
				return nil, fmt.Errorf("plugin execution failed: %s", stderr.String())
			}
			return nil, fmt.Errorf("plugin execution failed: %w", err)
		}

		var result map[string]any
		if stdout.Len() > 0 {
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
				result = map[string]any{"output": stdout.String()}
			}
		} else {
			result = map[string]any{"status": "success"}
		}
		return result, nil
	}

	if v1.MCP != nil && v1.MCP.Command != "" {
		timeout := time.Duration(manifest.TimeoutSeconds) * time.Second
		if timeout == 0 {
			timeout = 30 * time.Second
		}
		cmdCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		args := append(v1.MCP.Args, action)
		cmd := exec.CommandContext(cmdCtx, v1.MCP.Command, args...)
		cmd.Dir = manifest.sourceDir

		if len(inputs) > 0 {
			inputData, _ := json.Marshal(inputs)
			cmd.Stdin = bytes.NewReader(inputData)
		}

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			if stderr.Len() > 0 {
				return nil, fmt.Errorf("MCP execution failed: %s", stderr.String())
			}
			return nil, fmt.Errorf("MCP execution failed: %w", err)
		}

		var result map[string]any
		if stdout.Len() > 0 {
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
				result = map[string]any{"output": stdout.String()}
			}
		} else {
			result = map[string]any{"status": "success"}
		}
		return result, nil
	}

	return map[string]any{"status": "executed", "plugin": pluginID, "action": action}, nil
}

// SuspendPlugin 暂停插件
func (lm *LifecycleManager) SuspendPlugin(ctx context.Context, pluginID string) error {
	lifecycle, ok := lm.plugins[pluginID]
	if !ok {
		return fmt.Errorf("plugin %s not found", pluginID)
	}

	if lifecycle.SuspendData == nil {
		lifecycle.SuspendData = make(map[string]any)
	}

	lifecycle.SuspendData["state"] = string(lifecycle.State)
	lifecycle.SuspendData["sessions"] = lifecycle.Sessions
	lifecycle.SuspendData["suspended_at"] = time.Now().UTC().Format(time.RFC3339)

	if lifecycle.Manifest.ResumeSupport != nil && lifecycle.Manifest.ResumeSupport.Checkpointable {
		checkpointPath := filepath.Join(lifecycle.Manifest.sourceDir, ".suspend", pluginID+".json")
		os.MkdirAll(filepath.Dir(checkpointPath), 0755)
		data, _ := json.MarshalIndent(lifecycle.SuspendData, "", "  ")
		os.WriteFile(checkpointPath, data, 0644)
	}

	lifecycle.State = PluginStateSuspended
	return nil
}

// ResumePlugin 恢复插件
func (lm *LifecycleManager) ResumePlugin(ctx context.Context, pluginID string) error {
	lifecycle, ok := lm.plugins[pluginID]
	if !ok {
		return fmt.Errorf("plugin %s not found", pluginID)
	}

	if lifecycle.State != PluginStateSuspended {
		return fmt.Errorf("plugin must be suspended before resume")
	}

	if lifecycle.SuspendData != nil {
		if prevState, ok := lifecycle.SuspendData["state"].(string); ok {
			lifecycle.State = PluginState(prevState)
		}
		if sessions, ok := lifecycle.SuspendData["sessions"].(map[string]*PluginSession); ok {
			lifecycle.Sessions = sessions
		}
		delete(lifecycle.SuspendData, "suspended_at")

		if lifecycle.Manifest.ResumeSupport != nil && lifecycle.Manifest.ResumeSupport.Checkpointable {
			checkpointPath := filepath.Join(lifecycle.Manifest.sourceDir, ".suspend", pluginID+".json")
			os.Remove(checkpointPath)
		}
	}

	if lifecycle.State == "" {
		lifecycle.State = PluginStateBound
	}

	return nil
}

// UnloadPlugin 卸载插件
func (lm *LifecycleManager) UnloadPlugin(ctx context.Context, pluginID string) error {
	lifecycle, ok := lm.plugins[pluginID]
	if !ok {
		return fmt.Errorf("plugin %s not found", pluginID)
	}

	lm.capabilityIndex.Remove(pluginID)

	if err := lm.executeHook(ctx, pluginID, HookBeforeUnload); err != nil {
		return fmt.Errorf("before_unload hook failed: %v", err)
	}

	for sessionID := range lifecycle.Sessions {
		delete(lifecycle.Sessions, sessionID)
	}
	lifecycle.Sessions = nil

	if lifecycle.SuspendData != nil {
		checkpointPath := filepath.Join(lifecycle.Manifest.sourceDir, ".suspend", pluginID+".json")
		os.Remove(checkpointPath)
		lifecycle.SuspendData = nil
	}

	lifecycle.State = PluginStateUnloaded
	delete(lm.plugins, pluginID)

	return nil
}

// CheckPluginHealth 检查插件健康状态
func (lm *LifecycleManager) CheckPluginHealth(ctx context.Context, pluginID string) error {
	lifecycle, ok := lm.plugins[pluginID]
	if !ok {
		return fmt.Errorf("plugin %s not found", pluginID)
	}

	healthCheck := lifecycle.Manifest.HealthCheck
	if healthCheck == nil {
		lifecycle.Health.Status = "healthy"
		lifecycle.Health.Message = "No health check defined"
		lifecycle.Health.CheckedAt = time.Now()
		return nil
	}

	timeout := time.Duration(healthCheck.TimeoutSec) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	if healthCheck.Command != "" {
		cmdCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		result, err := executeHook(cmdCtx, healthCheck.Command, lifecycle.Manifest.sourceDir)
		if err != nil {
			lifecycle.Health.Status = "error"
			lifecycle.Health.Message = fmt.Sprintf("Health check failed: %v", err)
		} else if result == 0 {
			lifecycle.Health.Status = "healthy"
			lifecycle.Health.Message = "Health check passed"
		} else {
			lifecycle.Health.Status = "warning"
			lifecycle.Health.Message = fmt.Sprintf("Health check returned code %d", result)
		}
	} else {
		lifecycle.Health.Status = "healthy"
		lifecycle.Health.Message = "No health check command"
	}

	lifecycle.Health.CheckedAt = time.Now()

	return nil
}

func executeHook(ctx context.Context, command string, workDir string) (int, error) {
	if command == "" {
		return 0, nil
	}

	var cmd *exec.Cmd
	if workDir != "" {
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
		cmd.Dir = workDir
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), fmt.Errorf("command failed (exit %d): %s", exitErr.ExitCode(), stderr.String())
		}
		return -1, fmt.Errorf("command failed: %w", err)
	}

	return 0, nil
}

type HookType string

const (
	HookBeforeLoad    HookType = "before_load"
	HookAfterLoad     HookType = "after_load"
	HookBeforeUnload  HookType = "before_unload"
	HookAfterUnload   HookType = "after_unload"
	HookBeforeExecute HookType = "before_execute"
	HookAfterExecute  HookType = "after_execute"
)

func (lm *LifecycleManager) executeHook(ctx context.Context, pluginID string, hookType HookType) error {
	lifecycle, ok := lm.plugins[pluginID]
	if !ok {
		return fmt.Errorf("plugin %s not found", pluginID)
	}

	hooks := lifecycle.Manifest.LifecycleHooks
	if hooks == nil {
		return nil
	}

	var command string
	switch hookType {
	case HookBeforeLoad:
		command = hooks.BeforeLoad
	case HookAfterLoad:
		command = hooks.AfterLoad
	case HookBeforeUnload:
		command = hooks.BeforeUnload
	case HookAfterUnload:
		command = hooks.AfterUnload
	case HookBeforeExecute:
		command = hooks.BeforeExecute
	case HookAfterExecute:
		command = hooks.AfterExecute
	}

	if command == "" {
		return nil
	}

	_, err := executeHook(ctx, command, lifecycle.Manifest.sourceDir)
	return err
}

// GetPluginState 获取插件状态
func (lm *LifecycleManager) GetPluginState(pluginID string) (PluginState, error) {
	lifecycle, ok := lm.plugins[pluginID]
	if !ok {
		return "", fmt.Errorf("plugin %s not found", pluginID)
	}

	return lifecycle.State, nil
}

// GetPluginMetrics 获取插件指标
func (lm *LifecycleManager) GetPluginMetrics(pluginID string) (PluginMetrics, error) {
	if lm.metricsStore != nil {
		return lm.metricsStore.GetMetrics(pluginID), nil
	}

	lifecycle, ok := lm.plugins[pluginID]
	if !ok {
		return PluginMetrics{}, fmt.Errorf("plugin %s not found", pluginID)
	}

	return lifecycle.Metrics, nil
}

// ListPluginsByState 按状态列出插件
func (lm *LifecycleManager) ListPluginsByState(state PluginState) []string {
	var plugins []string
	for pluginID, lifecycle := range lm.plugins {
		if lifecycle.State == state {
			plugins = append(plugins, pluginID)
		}
	}
	return plugins
}

// PluginExecutionError 插件执行错误
type PluginExecutionError struct {
	PluginID  string
	Action    string
	Reason    string
	Retryable bool
}

func (e *PluginExecutionError) Error() string {
	return fmt.Sprintf("plugin execution error: plugin=%s, action=%s, reason=%s", e.PluginID, e.Action, e.Reason)
}

// ExecuteWithRetry 带重试的执行
func (lm *LifecycleManager) ExecuteWithRetry(ctx context.Context, pluginID string, action string, inputs map[string]any, retryPolicy *RetryPolicy) (map[string]any, error) {
	if retryPolicy == nil {
		return lm.ExecutePlugin(ctx, pluginID, action, inputs)
	}

	var lastErr error
	for attempt := 1; attempt <= retryPolicy.MaxAttempts; attempt++ {
		outputs, err := lm.ExecutePlugin(ctx, pluginID, action, inputs)
		if err == nil {
			return outputs, nil
		}

		lastErr = err

		// 检查是否可重试
		if pe, ok := err.(*PluginExecutionError); ok && !pe.Retryable {
			return nil, err
		}

		// 计算延迟
		delay := retryPolicy.InitialDelay * int(retryPolicy.BackoffFactor*float64(attempt-1))
		if delay > retryPolicy.MaxDelay {
			delay = retryPolicy.MaxDelay
		}

		// 等待
		time.Sleep(time.Duration(delay) * time.Millisecond)
	}

	return nil, fmt.Errorf("plugin execution failed after %d attempts: %v", retryPolicy.MaxAttempts, lastErr)
}
