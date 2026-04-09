package qmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// SessionFile 会话文件格式
type SessionFile struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Messages  []Message `json:"messages"`
}

// Message 消息格式
type Message struct {
	Role    string `json:"role"` // "user" | "assistant" | "system"
	Content string `json:"content"`
}

// ExportSessionToFile 导出单个会话到 Markdown
func ExportSessionToFile(session *SessionFile, exportDir string, retentionDays int) error {
	// 检查 retention
	if time.Since(session.CreatedAt) > time.Duration(retentionDays)*24*time.Hour {
		return nil
	}

	// 脱敏处理（移除敏感信息）
	content := sanitizeSessionContent(session)

	// 确保导出目录存在
	if err := os.MkdirAll(exportDir, 0755); err != nil {
		return fmt.Errorf("failed to create export directory: %w", err)
	}

	// 写入文件
	filename := filepath.Join(exportDir, session.ID+".md")
	return os.WriteFile(filename, []byte(content), 0644)
}

// sanitizeSessionContent 对会话内容进行脱敏处理
func sanitizeSessionContent(session *SessionFile) string {
	var sb strings.Builder

	// 写入元数据
	sb.WriteString(fmt.Sprintf("# Session: %s\n", session.ID))
	sb.WriteString(fmt.Sprintf("Date: %s\n\n", session.CreatedAt.Format("2006-01-02 15:04:05")))
	sb.WriteString("---\n\n")

	// 写入消息
	for _, msg := range session.Messages {
		switch msg.Role {
		case "user":
			sb.WriteString("## User\n\n")
		case "assistant":
			sb.WriteString("## Assistant\n\n")
		case "system":
			sb.WriteString("## System\n\n")
		default:
			sb.WriteString(fmt.Sprintf("## %s\n\n", msg.Role))
		}

		// 脱敏处理内容
		sanitizedContent := sanitizeText(msg.Content)
		sb.WriteString(sanitizedContent)
		sb.WriteString("\n\n")
	}

	return sb.String()
}

// sanitizeText 对文本进行脱敏处理
func sanitizeText(text string) string {
	// 移除或替换敏感信息
	text = redactAPIKeys(text)
	text = redactPasswords(text)
	text = redactEmails(text)
	text = redactPhoneNumbers(text)

	return text
}

// 敏感信息正则表达式模式
var (
	apiKeyPattern   = regexp.MustCompile(`(?i)(api[_-]?key|apikey|secret|token)["\s:=]+([a-zA-Z0-9_\-]{16,})`)
	passwordPattern = regexp.MustCompile(`(?i)(password|passwd|pwd)["\s:=]+([^\s]+)`)
	emailPattern    = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)
	phonePattern    = regexp.MustCompile(`1[3-9]\d{9}`) // 中国手机号
)

// redactAPIKeys 替换 API 密钥
func redactAPIKeys(text string) string {
	return apiKeyPattern.ReplaceAllString(text, `$1 [REDACTED_API_KEY]`)
}

// redactPasswords 替换密码
func redactPasswords(text string) string {
	return passwordPattern.ReplaceAllString(text, `$1 [REDACTED_PASSWORD]`)
}

// redactEmails 替换邮箱
func redactEmails(text string) string {
	return emailPattern.ReplaceAllString(text, "[REDACTED_EMAIL]")
}

// redactPhoneNumbers 替换手机号
func redactPhoneNumbers(text string) string {
	return phonePattern.ReplaceAllString(text, "[REDACTED_PHONE]")
}

// ExportAllSessions 导出所有会话
func ExportAllSessions(sessionDir, exportDir string, retentionDays int) error {
	// 确保导出目录存在
	if err := os.MkdirAll(exportDir, 0755); err != nil {
		return fmt.Errorf("failed to create export directory: %w", err)
	}

	// 查找所有会话文件
	files, err := os.ReadDir(sessionDir)
	if err != nil {
		return fmt.Errorf("failed to read session directory: %w", err)
	}

	exportedCount := 0
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		// 检查文件扩展名
		if !strings.HasSuffix(file.Name(), ".jsonl") {
			continue
		}

		filePath := filepath.Join(sessionDir, file.Name())
		if err := exportSessionFile(filePath, exportDir, retentionDays); err != nil {
			fmt.Printf("Warning: Failed to export %s: %v\n", file.Name(), err)
			continue
		}

		exportedCount++
	}

	return nil
}

// exportSessionFile 导出单个会话文件
func exportSessionFile(filePath, exportDir string, retentionDays int) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		var session SessionFile
		if err := json.Unmarshal([]byte(line), &session); err != nil {
			continue // Skip invalid lines
		}

		if err := ExportSessionToFile(&session, exportDir, retentionDays); err != nil {
			continue
		}
	}

	return scanner.Err()
}

// CleanOldExports 清理过期的导出文件
func CleanOldExports(exportDir string, retentionDays int) error {
	files, err := os.ReadDir(exportDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 目录不存在，不需要清理
		}
		return fmt.Errorf("failed to read export directory: %w", err)
	}

	cutoffTime := time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour)

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		info, err := file.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoffTime) {
			filePath := filepath.Join(exportDir, file.Name())
			if err := os.Remove(filePath); err != nil {
				fmt.Printf("Warning: Failed to remove old export %s: %v\n", file.Name(), err)
			}
		}
	}

	return nil
}

// FindSessionDir 查找会话目录
func FindSessionDir(workspace string) (string, error) {
	// 尝试常见位置
	possiblePaths := []string{
		filepath.Join(workspace, "sessions"),
		filepath.Join(workspace, ".anyclaw", "sessions"),
		filepath.Join(workspace, "data", "sessions"),
	}

	for _, path := range possiblePaths {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return path, nil
		}
	}

	return "", fmt.Errorf("session directory not found")
}
