package qmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// QMDManager 管理 QMD 进程和集合
type QMDManager struct {
	config            QMDConfig
	workspace         string
	agentID           string
	mu                sync.RWMutex
	collections       map[string]*QMDCollection
	initialized       bool
	fallbackToBuiltin bool
	lastError         error
	lastUpdated       time.Time
	lastEmbed         time.Time
}

// NewQMDManager 创建 QMD 管理器
func NewQMDManager(config QMDConfig, workspace, agentID string) *QMDManager {
	return &QMDManager{
		config:            config,
		workspace:         workspace,
		agentID:           agentID,
		collections:       make(map[string]*QMDCollection),
		initialized:       false,
		fallbackToBuiltin: false,
	}
}

// Initialize 初始化 QMD（创建集合）
func (m *QMDManager) Initialize(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.initialized {
		return nil
	}

	// 检查 QMD 是否可用
	available, err := CheckQMDAvailable(ctx, m.config.Command, m.config.Update.CommandTimeout)
	if err != nil {
		m.lastError = err
		m.fallbackToBuiltin = true
		return fmt.Errorf("qmd not available: %w", err)
	}

	if !available {
		m.lastError = fmt.Errorf("qmd command not found or not working")
		m.fallbackToBuiltin = true
		return m.lastError
	}

	// 初始化默认集合
	if m.config.IncludeDefault {
		memoryDir := filepath.Join(m.workspace, "memory")
		if err := m.initCollection(ctx, "default", memoryDir, "**/*.md"); err != nil {
			return fmt.Errorf("failed to initialize default collection: %w", err)
		}
	}

	// 初始化用户配置的路径
	for _, pathCfg := range m.config.Paths {
		expandedPath := expandHomeDir(pathCfg.Path)
		if err := m.initCollection(ctx, pathCfg.Name, expandedPath, pathCfg.Pattern); err != nil {
			return fmt.Errorf("failed to initialize collection %s: %w", pathCfg.Name, err)
		}
	}

	// 初始化会话集合
	if m.config.Sessions.Enabled {
		sessionDir, err := FindSessionDir(m.workspace)
		if err != nil {
			// 如果找不到会话目录，使用默认路径
			sessionDir = filepath.Join(m.workspace, "sessions")
		}

		if m.config.Sessions.ExportDir != "" {
			// 先导出会话文件
			if err := ExportAllSessions(sessionDir, m.config.Sessions.ExportDir, m.config.Sessions.RetentionDays); err != nil {
				return fmt.Errorf("failed to export sessions: %w", err)
			}
			if err := m.initCollection(ctx, "sessions", m.config.Sessions.ExportDir, "*.md"); err != nil {
				return fmt.Errorf("failed to initialize sessions collection: %w", err)
			}
		}
	}

	m.initialized = true

	// 如果配置为启动时更新
	if m.config.Update.OnBoot {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), m.config.Update.UpdateTimeout)
			defer cancel()
			_ = m.Update(ctx)
		}()
	}

	return nil
}

// initCollection 初始化单个集合
func (m *QMDManager) initCollection(ctx context.Context, name, path, pattern string) error {
	// 检查路径是否存在
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("path %s does not exist", path)
	}

	// 检查集合是否已存在
	collections, err := ListCollections(ctx, m.config.Command, m.config.Update.CommandTimeout)
	if err == nil {
		for _, c := range collections {
			if c == name {
				// 集合已存在，添加到缓存
				stats, _ := GetCollectionStats(ctx, m.config.Command, name, m.config.Update.CommandTimeout)
				m.collections[name] = &QMDCollection{
					Name:       name,
					Path:       path,
					Pattern:    pattern,
					CreatedAt:  time.Now(),
					LastUpdate: time.Now(),
					DocumentCount: func() int {
						if stats != nil {
							return stats.DocumentCount
						}
						return 0
					}(),
				}
				return nil
			}
		}
	}

	// 创建新集合
	if err := CreateCollection(ctx, m.config.Command, name, path, pattern, m.config.Update.CommandTimeout); err != nil {
		return err
	}

	// 添加到缓存
	m.collections[name] = &QMDCollection{
		Name:       name,
		Path:       path,
		Pattern:    pattern,
		CreatedAt:  time.Now(),
		LastUpdate: time.Now(),
	}

	return nil
}

// Query 执行语义搜索
func (m *QMDManager) Query(ctx context.Context, query string) ([]QMDQueryResult, error) {
	if !m.initialized {
		if err := m.Initialize(ctx); err != nil {
			return nil, err
		}
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.collections) == 0 {
		return []QMDQueryResult{}, nil
	}

	timeout := time.Duration(m.config.Limits.TimeoutMs) * time.Millisecond
	allResults := make([]QMDQueryResult, 0)

	// 查询所有集合
	for name := range m.collections {
		results, err := QueryQMD(ctx, m.config.Command, name, query, m.config.Limits.MaxResults, timeout)
		if err != nil {
			// 记录错误但继续查询其他集合
			continue
		}
		allResults = append(allResults, results...)
	}

	// 按分数排序
	sortResultsByScore(allResults)

	// 限制结果数量
	if len(allResults) > m.config.Limits.MaxResults {
		allResults = allResults[:m.config.Limits.MaxResults]
	}

	// 截断片段
	for i := range allResults {
		if len(allResults[i].Snippet) > m.config.Limits.MaxSnippetChars {
			allResults[i].Snippet = truncateSnippet(allResults[i].Snippet, m.config.Limits.MaxSnippetChars)
		}
	}

	return allResults, nil
}

// Update 更新索引
func (m *QMDManager) Update(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.initialized {
		return fmt.Errorf("QMD manager not initialized")
	}

	// 如果启用了会话导出，先导出会话
	if m.config.Sessions.Enabled && m.config.Sessions.ExportDir != "" {
		sessionDir, err := FindSessionDir(m.workspace)
		if err != nil {
			sessionDir = filepath.Join(m.workspace, "sessions")
		}
		_ = ExportAllSessions(sessionDir, m.config.Sessions.ExportDir, m.config.Sessions.RetentionDays)
	}

	// 更新所有集合
	for name := range m.collections {
		_, err := UpdateCollection(ctx, m.config.Command, name, m.config.Update.UpdateTimeout)
		if err != nil {
			m.lastError = fmt.Errorf("failed to update collection %s: %w", name, err)
			continue
		}

		if col, ok := m.collections[name]; ok {
			col.LastUpdate = time.Now()

			// 更新文档计数
			if stats, err := GetCollectionStats(ctx, m.config.Command, name, m.config.Update.CommandTimeout); err == nil {
				col.DocumentCount = stats.DocumentCount
			}
		}
	}

	m.lastUpdated = time.Now()
	return nil
}

// Embed 生成嵌入向量
func (m *QMDManager) Embed(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.initialized {
		return fmt.Errorf("QMD manager not initialized")
	}

	// 为所有集合生成嵌入向量
	for name := range m.collections {
		_, err := EmbedCollection(ctx, m.config.Command, name, m.config.Update.EmbedInterval)
		if err != nil {
			m.lastError = fmt.Errorf("failed to embed collection %s: %w", name, err)
			continue
		}
	}

	m.lastEmbed = time.Now()
	return nil
}

// GetStatus 获取状态
func (m *QMDManager) GetStatus() *QMDStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := &QMDStatus{
		Available:       m.initialized,
		Collections:     make([]string, 0, len(m.collections)),
		LastUpdated:     m.lastUpdated,
		LastEmbed:       m.lastEmbed,
		FallbackEnabled: m.fallbackToBuiltin,
		Error:           "",
	}

	if m.lastError != nil {
		status.Error = m.lastError.Error()
	}

	// 获取版本
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if version, err := GetQMDVersion(ctx, m.config.Command, 5*time.Second); err == nil {
		status.Version = version
	}

	// 添加集合名称
	for name := range m.collections {
		status.Collections = append(status.Collections, name)
	}

	// 计算总文档数
	for _, col := range m.collections {
		status.IndexedFiles++
		status.TotalDocuments += col.DocumentCount
	}

	return status
}

// Close 清理资源
func (m *QMDManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.initialized = false
	m.collections = nil
	return nil
}

// helper functions

// expandHomeDir 扩展 ~ 为用户主目录
func expandHomeDir(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// sortResultsByScore 按分数降序排序
func sortResultsByScore(results []QMDQueryResult) {
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[i].Score < results[j].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
}

// truncateSnippet 截断片段并添加省略号
func truncateSnippet(snippet string, maxLen int) string {
	if len(snippet) <= maxLen {
		return snippet
	}

	// 尝试在单词边界截断
	truncated := snippet[:maxLen-3]
	lastSpace := findLastSpace(truncated)
	if lastSpace > 0 {
		truncated = truncated[:lastSpace]
	}

	return truncated + "..."
}

// findLastSpace 查找最后一个空格的位置
func findLastSpace(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == ' ' {
			return i
		}
	}
	return -1
}
