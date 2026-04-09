package qmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"time"
)

// RunQMDCommand 执行 QMD 命令
func RunQMDCommand(ctx context.Context, cmd *exec.Cmd, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd = exec.CommandContext(ctx, cmd.Path, cmd.Args[1:]...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("qmd command timed out after %v", timeout)
		}
		return nil, fmt.Errorf("qmd command failed: %w, output: %s", err, string(output))
	}

	return output, nil
}

// QueryQMD 执行 qmd query 命令
func QueryQMD(ctx context.Context, qmdPath, collection, query string, limit int, timeout time.Duration) ([]QMDQueryResult, error) {
	cmd := exec.Command(qmdPath, "query", "--json", "--limit", strconv.Itoa(limit), collection, query)
	output, err := RunQMDCommand(ctx, cmd, timeout)
	if err != nil {
		return nil, err
	}

	var results []QMDQueryResult
	if err := json.Unmarshal(output, &results); err != nil {
		return nil, fmt.Errorf("failed to parse qmd output: %w", err)
	}

	return results, nil
}

// CreateCollection 创建 QMD 集合
func CreateCollection(ctx context.Context, qmdPath, name, path, pattern string, timeout time.Duration) error {
	args := []string{"collection", "create", "--name", name}
	if path != "" {
		args = append(args, "--path", path)
	}
	if pattern != "" {
		args = append(args, "--pattern", pattern)
	}

	cmd := exec.Command(qmdPath, args...)
	_, err := RunQMDCommand(ctx, cmd, timeout)
	return err
}

// UpdateCollection 更新集合索引
func UpdateCollection(ctx context.Context, qmdPath, name string, timeout time.Duration) ([]byte, error) {
	cmd := exec.Command(qmdPath, "update", name)
	return RunQMDCommand(ctx, cmd, timeout)
}

// EmbedCollection 为集合生成嵌入向量
func EmbedCollection(ctx context.Context, qmdPath, name string, timeout time.Duration) ([]byte, error) {
	cmd := exec.Command(qmdPath, "embed", name)
	return RunQMDCommand(ctx, cmd, timeout)
}

// ListCollections 列出所有集合
func ListCollections(ctx context.Context, qmdPath string, timeout time.Duration) ([]string, error) {
	cmd := exec.Command(qmdPath, "collection", "list", "--json")
	output, err := RunQMDCommand(ctx, cmd, timeout)
	if err != nil {
		return nil, err
	}

	var collections []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(output, &collections); err != nil {
		return nil, fmt.Errorf("failed to parse collections: %w", err)
	}

	names := make([]string, len(collections))
	for i, c := range collections {
		names[i] = c.Name
	}

	return names, nil
}

// GetQMDVersion 获取 QMD 版本
func GetQMDVersion(ctx context.Context, qmdPath string, timeout time.Duration) (string, error) {
	cmd := exec.Command(qmdPath, "--version")
	output, err := RunQMDCommand(ctx, cmd, timeout)
	if err != nil {
		return "", err
	}

	// Remove trailing newline
	version := string(output)
	if len(version) > 0 && version[len(version)-1] == '\n' {
		version = version[:len(version)-1]
	}

	return version, nil
}

// GetCollectionStats 获取集合统计信息
type CollectionStats struct {
	Name           string `json:"name"`
	DocumentCount  int    `json:"document_count"`
	EmbeddingCount int    `json:"embedding_count"`
	LastUpdate     string `json:"last_update"`
}

func GetCollectionStats(ctx context.Context, qmdPath, name string, timeout time.Duration) (*CollectionStats, error) {
	cmd := exec.Command(qmdPath, "collection", "stats", name, "--json")
	output, err := RunQMDCommand(ctx, cmd, timeout)
	if err != nil {
		return nil, err
	}

	var stats CollectionStats
	if err := json.Unmarshal(output, &stats); err != nil {
		return nil, fmt.Errorf("failed to parse collection stats: %w", err)
	}

	return &stats, nil
}

// CheckQMDAvailable 检查 QMD 是否可用
func CheckQMDAvailable(ctx context.Context, qmdPath string, timeout time.Duration) (bool, error) {
	// First check if command exists
	_, err := exec.LookPath(qmdPath)
	if err != nil {
		return false, fmt.Errorf("qmd command not found: %w", err)
	}

	// Try to run --version to verify it works
	version, err := GetQMDVersion(ctx, qmdPath, timeout)
	if err != nil {
		return false, err
	}

	return len(version) > 0, nil
}
