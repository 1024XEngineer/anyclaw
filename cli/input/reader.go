package input

import (
	"fmt"
	"io"

	"github.com/ergochat/readline"
)

// ReadLine 读取一行输入（支持中文）
func ReadLine(prompt string) (string, error) {
	return ReadLineWithHistory(prompt, nil)
}

// ReadLineWithHistory 读取一行输入（支持历史记录）
// 注意：每次调用创建新的 readline 实例，适用于一次性读取
func ReadLineWithHistory(prompt string, history []string) (string, error) {
	// ergochat/readline uses simpler New() API with default config
	// For custom config, use NewFromConfig()
	rl, err := readline.New(prompt)
	if err != nil {
		return "", err
	}
	defer rl.Close()

	// 添加历史记录
	for _, h := range history {
		if h != "" {
			_ = rl.SaveToHistory(h)
		}
	}

	// 读取输入
	line, err := rl.ReadLine()
	if err != nil {
		// ergochat/readline returns io.EOF for Ctrl+C
		if err == readline.ErrInterrupt || err == io.EOF {
			return "", fmt.Errorf("interrupted")
		}
		return "", err
	}

	return line, nil
}

// NewReadline 创建持久化的 readline 实例
// 用于需要多次读取输入并保持历史记录的场景
func NewReadline(prompt string) (*readline.Instance, error) {
	// ergochat/readline's New() uses sensible defaults
	return readline.New(prompt)
}

// InitReadlineHistory 初始化 readline 实例的历史记录
// Note: ergochat/readline uses SaveToHistory instead of SaveHistory
func InitReadlineHistory(rl *readline.Instance, history []string) {
	for _, h := range history {
		if h != "" {
			_ = rl.SaveToHistory(h)
		}
	}
}
