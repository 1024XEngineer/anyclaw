package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileSystemTool 文件系统工具
type FileSystemTool struct {
	allowedPaths []string
	deniedPaths  []string
	workspace    string // 工作区路径，用于配置文件更新
}

// NewFileSystemTool 创建文件系统工具
func NewFileSystemTool(allowedPaths, deniedPaths []string, workspace string) *FileSystemTool {
	return &FileSystemTool{
		allowedPaths: allowedPaths,
		deniedPaths:  deniedPaths,
		workspace:    workspace,
	}
}

// ReadFile 读取文件
func (t *FileSystemTool) ReadFile(ctx context.Context, params map[string]interface{}) (string, error) {
	path, ok := params["path"].(string)
	if !ok {
		return "", fmt.Errorf("path parameter is required")
	}

	// 检查路径权限
	if !t.isAllowed(path) {
		return "", fmt.Errorf("access to path %s is not allowed", path)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	return string(content), nil
}

// WriteFile 写入文件
func (t *FileSystemTool) WriteFile(ctx context.Context, params map[string]interface{}) (string, error) {
	path, ok := params["path"].(string)
	if !ok {
		return "", fmt.Errorf("path parameter is required")
	}

	content, ok := params["content"].(string)
	if !ok {
		return "", fmt.Errorf("content parameter is required")
	}

	// 检查路径权限
	if !t.isAllowed(path) {
		return "", fmt.Errorf("access to path %s is not allowed", path)
	}

	// 确保目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	// 写入文件
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", err
	}

	return fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), path), nil
}

// EditFile 编辑文件（精确字符串替换）
func (t *FileSystemTool) EditFile(ctx context.Context, params map[string]interface{}) (string, error) {
	path, ok := params["path"].(string)
	if !ok {
		return "", fmt.Errorf("path parameter is required")
	}

	oldStr, ok := params["old_string"].(string)
	if !ok {
		return "", fmt.Errorf("old_string parameter is required")
	}

	newStr, ok := params["new_string"].(string)
	if !ok {
		return "", fmt.Errorf("new_string parameter is required")
	}

	// 检查路径权限
	if !t.isAllowed(path) {
		return "", fmt.Errorf("access to path %s is not allowed", path)
	}

	// 读取文件内容
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	fileContent := string(content)

	// 检查旧字符串是否存在
	if !strings.Contains(fileContent, oldStr) {
		return "", fmt.Errorf("old_string not found in file. Please verify the exact text to replace.")
	}

	// 计算替换次数
	occurrences := strings.Count(fileContent, oldStr)

	// 执行替换
	newContent := strings.ReplaceAll(fileContent, oldStr, newStr)

	// 写入文件
	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return fmt.Sprintf("Successfully replaced %d occurrence(s) in %s", occurrences, path), nil
}

// ListDir 列出目录
func (t *FileSystemTool) ListDir(ctx context.Context, params map[string]interface{}) (string, error) {
	path, ok := params["path"].(string)
	if !ok {
		return "", fmt.Errorf("path parameter is required")
	}

	// 检查路径权限
	if !t.isAllowed(path) {
		return "", fmt.Errorf("access to path %s is not allowed", path)
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return "", err
	}

	var result []string
	for _, entry := range entries {
		info := ""
		if entry.IsDir() {
			info = "[DIR] "
		}
		result = append(result, info+entry.Name())
	}

	return strings.Join(result, "\n"), nil
}

// isAllowed 检查路径是否允许访问
func (t *FileSystemTool) isAllowed(path string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	// 检查拒绝列表（转换为绝对路径）
	for _, denied := range t.deniedPaths {
		absDenied, err := filepath.Abs(denied)
		if err == nil && strings.HasPrefix(absPath, absDenied) {
			return false
		}
	}

	// 如果没有允许列表，允许所有路径
	if len(t.allowedPaths) == 0 {
		return true
	}

	// 检查允许列表（转换为绝对路径）
	for _, allowed := range t.allowedPaths {
		absAllowed, err := filepath.Abs(allowed)
		if err == nil && strings.HasPrefix(absPath, absAllowed) {
			return true
		}
	}

	return false
}

// UpdateConfig 更新配置文件
func (t *FileSystemTool) UpdateConfig(ctx context.Context, params map[string]interface{}) (string, error) {
	fileType, ok := params["file"].(string)
	if !ok {
		return "", fmt.Errorf("file parameter is required (identity, agents, soul, or user)")
	}

	content, ok := params["content"].(string)
	if !ok {
		return "", fmt.Errorf("content parameter is required")
	}

	// 验证文件类型
	validFiles := map[string]string{
		"identity": "IDENTITY.md",
		"agents":   "AGENTS.md",
		"soul":     "SOUL.md",
		"user":     "USER.md",
	}

	filename, valid := validFiles[fileType]
	if !valid {
		return "", fmt.Errorf("invalid file type: %s (must be one of: identity, agents, soul, user)", fileType)
	}

	// 构建完整路径
	if t.workspace == "" {
		return "", fmt.Errorf("workspace path is not configured")
	}
	path := filepath.Join(t.workspace, filename)

	// 确保目录存在
	if err := os.MkdirAll(t.workspace, 0755); err != nil {
		return "", fmt.Errorf("failed to create workspace directory: %w", err)
	}

	// 写入文件
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write config file: %w", err)
	}

	return fmt.Sprintf("Successfully updated %s\n\nThe changes will take effect in the next conversation.", filename), nil
}

// ReadConfig 读取配置文件
func (t *FileSystemTool) ReadConfig(ctx context.Context, params map[string]interface{}) (string, error) {
	fileType, ok := params["file"].(string)
	if !ok {
		return "", fmt.Errorf("file parameter is required (identity, agents, soul, or user)")
	}

	// 验证文件类型
	validFiles := map[string]string{
		"identity": "IDENTITY.md",
		"agents":   "AGENTS.md",
		"soul":     "SOUL.md",
		"user":     "USER.md",
	}

	filename, valid := validFiles[fileType]
	if !valid {
		return "", fmt.Errorf("invalid file type: %s (must be one of: identity, agents, soul, user)", fileType)
	}

	// 构建完整路径
	if t.workspace == "" {
		return "", fmt.Errorf("workspace path is not configured")
	}
	path := filepath.Join(t.workspace, filename)

	// 读取文件
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Sprintf("Config file %s does not exist yet. Use update_config to create it.", filename), nil
		}
		return "", err
	}

	return string(content), nil
}

// GetTools 获取所有文件系统工具
func (t *FileSystemTool) GetTools() []Tool {
	tools := []Tool{
		NewBaseTool(
			"read_file",
			"Read the contents of a file",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Path to the file to read",
					},
				},
				"required": []string{"path"},
			},
			t.ReadFile,
		),
		NewBaseTool(
			"write_file",
			"Write content to a file",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Path to the file to write",
					},
					"content": map[string]interface{}{
						"type":        "string",
						"description": "Content to write to the file",
					},
				},
				"required": []string{"path", "content"},
			},
			t.WriteFile,
		),
		NewBaseTool(
			"edit_file",
			"Replace all occurrences of old_string with new_string in a file. Use for precise edits to existing files.",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Path to the file to edit",
					},
					"old_string": map[string]interface{}{
						"type":        "string",
						"description": "The exact text to be replaced (must match exactly)",
					},
					"new_string": map[string]interface{}{
						"type":        "string",
						"description": "The new text to replace old_string with",
					},
				},
				"required": []string{"path", "old_string", "new_string"},
			},
			t.EditFile,
		),
		NewBaseTool(
			"list_dir",
			"List contents of a directory",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Path to the directory",
					},
				},
				"required": []string{"path"},
			},
			t.ListDir,
		),
	}

	// 添加配置文件管理工具
	if t.workspace != "" {
		tools = append(tools,
			NewBaseTool(
				"update_config",
				"Update a configuration file (IDENTITY.md, AGENTS.md, SOUL.md, or USER.md). Changes take effect in the next conversation.",
				map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"file": map[string]interface{}{
							"type":        "string",
							"enum":        []string{"identity", "agents", "soul", "user"},
							"description": "The config file to update: identity (IDENTITY.md), agents (AGENTS.md), soul (SOUL.md), or user (USER.md)",
						},
						"content": map[string]interface{}{
							"type":        "string",
							"description": "The new content for the config file (Markdown format)",
						},
					},
					"required": []string{"file", "content"},
				},
				t.UpdateConfig,
			),
			NewBaseTool(
				"read_config",
				"Read a configuration file (IDENTITY.md, AGENTS.md, SOUL.md, or USER.md)",
				map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"file": map[string]interface{}{
							"type":        "string",
							"enum":        []string{"identity", "agents", "soul", "user"},
							"description": "The config file to read: identity (IDENTITY.md), agents (AGENTS.md), soul (SOUL.md), or user (USER.md)",
						},
					},
					"required": []string{"file"},
				},
				t.ReadConfig,
			),
		)
	}

	return tools
}
