package tools

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

func resolvePath(path string, cwd string) string {
	if cwd == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(cwd, path)
}

func normalizePathForCompare(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", nil
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path %q: %w", path, err)
	}
	cleaned := filepath.Clean(absPath)
	if runtime.GOOS == "windows" {
		cleaned = strings.ToLower(cleaned)
	}
	return cleaned, nil
}

func pathWithin(target string, base string) bool {
	if target == base {
		return true
	}
	return strings.HasPrefix(target, base+string(filepath.Separator))
}
