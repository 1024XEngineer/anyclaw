package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	llm "github.com/1024XEngineer/anyclaw/pkg/capability/models"
	"github.com/1024XEngineer/anyclaw/pkg/state/memory"
)

// MemoryBackend is an alias for memory.MemoryBackend to avoid import cycles.
type MemoryBackend = memory.MemoryBackend

// VectorMemoryBackend is an alias for memory.VectorBackend to avoid import cycles.
type VectorMemoryBackend = memory.VectorBackend

// QMDClient is the interface for QMD structured data operations.
type QMDClient interface {
	CreateTable(ctx context.Context, name string, columns []string) error
	Insert(ctx context.Context, table string, record map[string]any) error
	Get(ctx context.Context, table, id string) (map[string]any, error)
	Update(ctx context.Context, table string, record map[string]any) error
	Delete(ctx context.Context, table, id string) error
	List(ctx context.Context, table string, limit int) ([]map[string]any, error)
	Query(ctx context.Context, table, field string, value any, limit int) ([]map[string]any, error)
	ListTables(ctx context.Context) ([]TableStat, error)
	Count(ctx context.Context, table string) (int, error)
}

// TableStat represents a QMD table summary.
type TableStat struct {
	Name     string `json:"name"`
	RowCount int    `json:"row_count"`
	Columns  int    `json:"columns"`
}

// AuditLogger 审计日志接口
type AuditLogger interface {
	LogTool(toolName string, input map[string]any, output string, err error)
}

// DangerousCommandConfirmer 危险命令确认器
type DangerousCommandConfirmer func(command string) bool

// BuiltinOptions 内置工具选项
type BuiltinOptions struct {
	WorkingDir              string
	PermissionLevel         string
	ExecutionMode           string
	DangerousPatterns       []string
	ProtectedPaths          []string
	AllowedReadPaths        []string
	AllowedWritePaths       []string
	Policy                  *PolicyEngine
	CommandTimeoutSeconds   int
	ConfirmDangerousCommand DangerousCommandConfirmer
	AuditLogger             AuditLogger
	Sandbox                 *SandboxManager
	MemoryBackend           MemoryBackend
	QMDClient               QMDClient
	LLMClient               llm.Client
}

// ToolFunc 工具函数
type ToolFunc func(ctx context.Context, input map[string]any) (string, error)

// ToolCategory 工具类别
type ToolCategory string

const (
	ToolCategoryFile    ToolCategory = "file"
	ToolCategoryCommand ToolCategory = "command"
	ToolCategoryWeb     ToolCategory = "web"
	ToolCategoryChannel ToolCategory = "channel"
	ToolCategoryMemory  ToolCategory = "memory"
	ToolCategoryBrowser ToolCategory = "browser"
	ToolCategoryDesktop ToolCategory = "desktop"
	ToolCategoryCustom  ToolCategory = "custom"
)

// ToolAccessLevel 工具访问级别
type ToolAccessLevel string

// ToolVisibility controls which agent roles can see a tool.
type ToolVisibility string

// ToolCachePolicy controls whether tool outputs may be cached.
type ToolCachePolicy string

const (
	ToolAccessPublic  ToolAccessLevel = "public"
	ToolAccessOwner   ToolAccessLevel = "owner"
	ToolAccessAdmin   ToolAccessLevel = "admin"
	ToolAccessPrivate ToolAccessLevel = "private"
)

const (
	ToolVisibilityAll           ToolVisibility = "all"
	ToolVisibilityMainAgentOnly ToolVisibility = "main_agent_only"
)

const (
	ToolCachePolicyDefault ToolCachePolicy = "default"
	ToolCachePolicyNever   ToolCachePolicy = "never"
)

// Tool 工具结构
type Tool struct {
	Name        string
	Description string
	InputSchema map[string]any
	Handler     ToolFunc
	Category    ToolCategory
	AccessLevel ToolAccessLevel
	Visibility  ToolVisibility
	CachePolicy ToolCachePolicy
	Timeout     time.Duration
	Retryable   bool
	MaxRetries  int
}

// Registry 工具注册表
type Registry struct {
	mu         sync.RWMutex
	tools      map[string]*Tool
	categories map[ToolCategory][]string
	cache      map[string]interface{}
	cacheTTL   time.Duration
	cacheMu    sync.RWMutex
}

// NewRegistry 创建新的注册表
func NewRegistry() *Registry {
	return &Registry{
		tools:      make(map[string]*Tool),
		categories: make(map[ToolCategory][]string),
		cache:      make(map[string]interface{}),
		cacheTTL:   5 * time.Minute,
	}
}

// RegisterTool 注册工具
func (r *Registry) RegisterTool(name string, desc string, schema map[string]any, handler ToolFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.tools[name] = &Tool{
		Name:        name,
		Description: desc,
		InputSchema: schema,
		Handler:     handler,
		Category:    ToolCategoryCustom,
		AccessLevel: ToolAccessPublic,
		Visibility:  ToolVisibilityAll,
		CachePolicy: ToolCachePolicyNever,
	}
}

// Register 注册工具
func (r *Registry) Register(t *Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if t.Visibility == "" {
		t.Visibility = ToolVisibilityAll
	}
	if t.CachePolicy == "" {
		t.CachePolicy = ToolCachePolicyNever
	}
	r.tools[t.Name] = t
	if t.Category != "" {
		r.categories[t.Category] = append(r.categories[t.Category], t.Name)
	}
}

// Get 获取工具
func (r *Registry) Get(name string) (*Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	t, ok := r.tools[name]
	return t, ok
}

// ListTools 列出所有工具
func (r *Registry) ListTools() []*Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var list []*Tool
	for _, t := range r.tools {
		list = append(list, t)
	}
	return list
}

// ListToolsForRole returns tool instances visible to the given agent role.
func (r *Registry) ListToolsForRole(isSubAgent bool) []*Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var list []*Tool
	for _, t := range r.tools {
		if !toolVisibleForRole(t, isSubAgent) {
			continue
		}
		list = append(list, t)
	}
	return list
}

// Call 调用工具
func (r *Registry) Call(ctx context.Context, name string, input map[string]any) (string, error) {
	r.mu.RLock()
	t, ok := r.tools[name]
	r.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("tool not found: %s", name)
	}

	if t.Handler == nil {
		return "", fmt.Errorf("tool handler not implemented: %s", name)
	}

	if err := authorizeToolCall(ctx, t); err != nil {
		return "", err
	}

	// 检查缓存
	cacheEnabled := t.CachePolicy != ToolCachePolicyNever
	cacheKey := ""
	if cacheEnabled {
		cacheKey = r.generateCacheKey(name, input)
		if cached, found := r.getFromCache(cacheKey); found {
			if str, ok := cached.(string); ok {
				return str, nil
			}
		}
	}

	// 执行工具
	startTime := time.Now()
	result, err := t.Handler(ctx, input)
	duration := time.Since(startTime)

	if err != nil {
		return "", err
	}

	// 保存到缓存
	if cacheEnabled {
		r.saveToCache(cacheKey, result)
	}

	_ = duration // 可以用于监控

	return result, nil
}

// CallWithRetry 带重试调用工具
func (r *Registry) CallWithRetry(ctx context.Context, name string, input map[string]any, maxRetries int) (string, error) {
	r.mu.RLock()
	t, ok := r.tools[name]
	r.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("tool not found: %s", name)
	}

	if !t.Retryable {
		return r.Call(ctx, name, input)
	}

	if maxRetries <= 0 {
		maxRetries = t.MaxRetries
	}
	if maxRetries <= 0 {
		maxRetries = 3
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		result, err := r.Call(ctx, name, input)
		if err == nil {
			return result, nil
		}

		lastErr = err

		// 指数退避
		if attempt < maxRetries {
			backoff := time.Duration(1<<uint(attempt)) * 100 * time.Millisecond
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(backoff):
				continue
			}
		}
	}

	return "", fmt.Errorf("tool %s failed after %d retries: %w", name, maxRetries, lastErr)
}

// GetToolsByCategory 按类别获取工具
func (r *Registry) GetToolsByCategory(category ToolCategory) []*Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names, exists := r.categories[category]
	if !exists {
		return nil
	}

	tools := make([]*Tool, 0, len(names))
	for _, name := range names {
		if tool, exists := r.tools[name]; exists {
			tools = append(tools, tool)
		}
	}

	return tools
}

// Unregister 注销工具
func (r *Registry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	tool, exists := r.tools[name]
	if !exists {
		return fmt.Errorf("tool %s not found", name)
	}

	// 从类别中移除
	if tool.Category != "" {
		if names, exists := r.categories[tool.Category]; exists {
			for i, n := range names {
				if n == name {
					r.categories[tool.Category] = append(names[:i], names[i+1:]...)
					break
				}
			}
		}
	}

	delete(r.tools, name)
	return nil
}

// ClearCache 清除缓存
func (r *Registry) ClearCache() {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()

	r.cache = make(map[string]interface{})
}

// GetToolDefinitions 获取工具定义
func (r *Registry) GetToolDefinitions() []map[string]any {
	return r.GetToolDefinitionsForRole(false)
}

// GetToolDefinitionsJSON 获取工具定义的 JSON
func (r *Registry) GetToolDefinitionsForRole(isSubAgent bool) []map[string]any {
	r.mu.RLock()
	defer r.mu.RUnlock()

	defs := make([]map[string]any, 0, len(r.tools))
	for _, tool := range r.tools {
		if !toolVisibleForRole(tool, isSubAgent) {
			continue
		}
		defs = append(defs, map[string]any{
			"name":         tool.Name,
			"description":  tool.Description,
			"category":     string(tool.Category),
			"access_level": string(tool.AccessLevel),
			"visibility":   string(tool.Visibility),
			"cache_policy": string(tool.CachePolicy),
			"input_schema": tool.InputSchema,
		})
	}

	return defs
}

func (r *Registry) GetToolDefinitionsJSON() (string, error) {
	defs := r.GetToolDefinitions()

	data, err := json.MarshalIndent(defs, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal tool definitions: %w", err)
	}

	return string(data), nil
}

// ToolInfo 工具信息
type ToolInfo struct {
	Name        string
	Description string
	InputSchema map[string]any
	Visibility  ToolVisibility
	CachePolicy ToolCachePolicy
}

// List 列出工具信息
func (r *Registry) List() []ToolInfo {
	return r.ListForRole(false)
}

// generateCacheKey 生成缓存键
func (r *Registry) ListForRole(isSubAgent bool) []ToolInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var list []ToolInfo
	for _, t := range r.tools {
		if !toolVisibleForRole(t, isSubAgent) {
			continue
		}
		list = append(list, ToolInfo{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
			Visibility:  t.Visibility,
			CachePolicy: t.CachePolicy,
		})
	}
	return list
}

func toolVisibleForRole(tool *Tool, isSubAgent bool) bool {
	if tool == nil {
		return false
	}
	if tool.Visibility == "" || !isSubAgent {
		return true
	}
	return tool.Visibility != ToolVisibilityMainAgentOnly
}

func authorizeToolCall(ctx context.Context, tool *Tool) error {
	if tool == nil {
		return fmt.Errorf("tool is nil")
	}
	caller := ToolCallerFromContext(ctx)
	if toolVisibleForCaller(tool, caller.Role) {
		return nil
	}
	role := string(caller.Role)
	if role == "" {
		role = "unknown"
	}
	return fmt.Errorf("tool %s is not available for caller role %s", tool.Name, role)
}

func toolVisibleForCaller(tool *Tool, role ToolCallerRole) bool {
	switch role {
	case ToolCallerRoleSubAgent:
		return toolVisibleForRole(tool, true)
	case ToolCallerRoleMainAgent, ToolCallerRoleSystem:
		return true
	default:
		return tool.Visibility != ToolVisibilityMainAgentOnly
	}
}

func (r *Registry) generateCacheKey(toolName string, input map[string]any) string {
	data, _ := json.Marshal(input)
	return fmt.Sprintf("%s:%x", toolName, data)
}

// getFromCache 从缓存获取
func (r *Registry) getFromCache(key string) (interface{}, bool) {
	r.cacheMu.RLock()
	defer r.cacheMu.RUnlock()

	item, exists := r.cache[key]
	return item, exists
}

// saveToCache 保存到缓存
func (r *Registry) saveToCache(key string, value interface{}) {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()

	r.cache[key] = value

	// 定期清理缓存
	go func() {
		time.Sleep(r.cacheTTL)
		r.cacheMu.Lock()
		delete(r.cache, key)
		r.cacheMu.Unlock()
	}()
}
