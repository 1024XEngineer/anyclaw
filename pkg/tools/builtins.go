package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/anyclaw/anyclaw/pkg/web"
)

func resolvePath(path string, cwd string) string {
	if cwd == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(cwd, path)
}

var imageExtensions = map[string]bool{
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".gif":  true,
	".bmp":  true,
	".webp": true,
	".svg":  true,
	".ico":  true,
	".tiff": true,
	".tif":  true,
}

func isImageFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return imageExtensions[ext]
}

func ReadFileTool(ctx context.Context, input map[string]any) (string, error) {
	return ReadFileToolWithCwd(ctx, input, "")
}

func ReadFileToolWithPolicy(ctx context.Context, input map[string]any, cwd string, opts BuiltinOptions) (string, error) {
	path, ok := input["path"].(string)
	if !ok {
		return "", fmt.Errorf("path is required")
	}
	resolved := resolvePath(path, cwd)
	if opts.Policy != nil {
		if err := opts.Policy.CheckReadPath(resolved); err != nil {
			return "", err
		}
	} else if err := validateProtectedPath(resolved, opts.ProtectedPaths); err != nil {
		return "", err
	}
	return ReadFileToolWithCwd(ctx, input, cwd)
}

func ReadFileToolWithCwd(ctx context.Context, input map[string]any, cwd string) (string, error) {
	path, ok := input["path"].(string)
	if !ok {
		return "", fmt.Errorf("path is required")
	}

	resolvedPath := resolvePath(path, cwd)

	if isImageFile(path) || isImageFile(resolvedPath) {
		info, err := os.Stat(resolvedPath)
		if err != nil {
			return "", fmt.Errorf("failed to read file: %w", err)
		}
		return "", fmt.Errorf("无法读取图片文件 \"%s\"：当前模型不支持图片输入。请使用支持视觉的模型（如 gpt-4o、claude-opus-4-5）或将图片转为文字描述", info.Name())
	}

	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	return string(data), nil
}

func WriteFileTool(ctx context.Context, input map[string]any) (string, error) {
	return WriteFileToolWithCwd(ctx, input, "", "full")
}

func WriteFileToolWithCwd(ctx context.Context, input map[string]any, cwd string, permissionLevel string) (string, error) {
	path, ok := input["path"].(string)
	if !ok {
		return "", fmt.Errorf("path is required")
	}

	content, ok := input["content"].(string)
	if !ok {
		return "", fmt.Errorf("content is required")
	}

	path = resolvePath(path, cwd)
	if err := ensureWriteAllowed(path, cwd, permissionLevel); err != nil {
		return "", err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return fmt.Sprintf("Written to %s", path), nil
}

func WriteFileToolWithPolicy(ctx context.Context, input map[string]any, cwd string, opts BuiltinOptions) (string, error) {
	path, ok := input["path"].(string)
	if !ok {
		return "", fmt.Errorf("path is required")
	}
	resolved := resolvePath(path, cwd)
	if opts.Policy != nil {
		if err := opts.Policy.CheckWritePath(resolved); err != nil {
			return "", err
		}
	} else {
		if err := validateProtectedPath(resolved, opts.ProtectedPaths); err != nil {
			return "", err
		}
	}
	return WriteFileToolWithCwd(ctx, input, cwd, opts.PermissionLevel)
}

func ListDirectoryTool(ctx context.Context, input map[string]any) (string, error) {
	return ListDirectoryToolWithCwd(ctx, input, "")
}

func ListDirectoryToolWithPolicy(ctx context.Context, input map[string]any, cwd string, opts BuiltinOptions) (string, error) {
	path, ok := input["path"].(string)
	if !ok {
		path = cwd
	}
	resolved := resolvePath(path, cwd)
	if opts.Policy != nil {
		if err := opts.Policy.CheckReadPath(resolved); err != nil {
			return "", err
		}
	} else if err := validateProtectedPath(resolved, opts.ProtectedPaths); err != nil {
		return "", err
	}
	return ListDirectoryToolWithCwd(ctx, input, cwd)
}

func ListDirectoryToolWithCwd(ctx context.Context, input map[string]any, cwd string) (string, error) {
	path, ok := input["path"].(string)
	if !ok {
		path = cwd
	}

	path = resolvePath(path, cwd)
	entries, err := os.ReadDir(path)
	if err != nil {
		return "", fmt.Errorf("failed to read directory: %w", err)
	}

	if len(entries) == 0 {
		return "Empty directory", nil
	}

	result := make([]string, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			info = nil
		}
		if entry.IsDir() {
			result = append(result, fmt.Sprintf("d %s/", entry.Name()))
		} else if info != nil {
			result = append(result, fmt.Sprintf("- %s (%d bytes)", entry.Name(), info.Size()))
		} else {
			result = append(result, fmt.Sprintf("- %s", entry.Name()))
		}
	}

	out, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("failed to marshal result: %w", err)
	}
	return string(out), nil
}

func SearchFilesTool(ctx context.Context, input map[string]any) (string, error) {
	return SearchFilesToolWithCwd(ctx, input, "")
}

func SearchFilesToolWithPolicy(ctx context.Context, input map[string]any, cwd string, opts BuiltinOptions) (string, error) {
	root, ok := input["path"].(string)
	if !ok {
		root = cwd
	}
	resolved := resolvePath(root, cwd)
	if opts.Policy != nil {
		if err := opts.Policy.CheckReadPath(resolved); err != nil {
			return "", err
		}
	} else if err := validateProtectedPath(resolved, opts.ProtectedPaths); err != nil {
		return "", err
	}
	return SearchFilesToolWithCwd(ctx, input, cwd)
}

func SearchFilesToolWithCwd(ctx context.Context, input map[string]any, cwd string) (string, error) {
	root, ok := input["path"].(string)
	if !ok {
		root = cwd
	}

	root = resolvePath(root, cwd)
	pattern, ok := input["pattern"].(string)
	if !ok {
		return "", fmt.Errorf("pattern is required")
	}

	var matches []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		name := filepath.Base(path)
		if matched, err := filepath.Match(pattern, name); err == nil && matched {
			matches = append(matches, path)
		}
		return nil
	})

	if err != nil {
		return "", fmt.Errorf("search failed: %w", err)
	}

	if len(matches) == 0 {
		return "No matches found", nil
	}

	out, err := json.Marshal(matches)
	if err != nil {
		return "", fmt.Errorf("failed to marshal results: %w", err)
	}
	return string(out), nil
}

func RunCommandTool(ctx context.Context, input map[string]any) (string, error) {
	return RunCommandToolWithPolicy(ctx, input, BuiltinOptions{})
}

func RunCommandToolWithPolicy(ctx context.Context, input map[string]any, opts BuiltinOptions) (string, error) {
	cmdStr, ok := input["command"].(string)
	if !ok {
		return "", fmt.Errorf("command is required")
	}

	cwd, _ := input["cwd"].(string)
	shellName, _ := input["shell"].(string)
	if cwd == "" {
		cwd = opts.WorkingDir
	}
	if isDangerousCommand(cmdStr, opts.DangerousPatterns) && opts.ConfirmDangerousCommand != nil {
		if !opts.ConfirmDangerousCommand(cmdStr) {
			return "", fmt.Errorf("dangerous command cancelled by user")
		}
	}
	if opts.PermissionLevel == "read-only" {
		return "", fmt.Errorf("permission denied: current agent is read-only")
	}
	if opts.Policy != nil {
		if err := opts.Policy.CheckCommandCwd(firstNonEmptyCommandCwd(cwd, opts.WorkingDir)); err != nil {
			return "", err
		}
	}
	if err := reviewCommandExecution(cmdStr, cwd, opts); err != nil {
		return "", err
	}

	if opts.CommandTimeoutSeconds > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(opts.CommandTimeoutSeconds)*time.Second)
		defer cancel()
	}

	resolvedCwd := cwd
	commandFactory := func(cmdCtx context.Context, command string) (*exec.Cmd, error) {
		return shellCommandWithShell(cmdCtx, command, shellName)
	}
	applyDir := true
	if opts.Sandbox != nil {
		sandboxCwd, factory, err := opts.Sandbox.ResolveExecution(ctx, cwd)
		if err != nil {
			return "", err
		}
		if factory != nil {
			commandFactory = factory
			applyDir = false
		}
		if sandboxCwd != "" {
			resolvedCwd = sandboxCwd
		}
	}

	cmd, err := commandFactory(ctx, cmdStr)
	if err != nil {
		return "", err
	}
	if resolvedCwd != "" && applyDir {
		cmd.Dir = resolvedCwd
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("command failed: %w - %s", err, string(output))
	}

	return string(output), nil
}

func firstNonEmptyCommandCwd(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func shellCommand(ctx context.Context, command string) *exec.Cmd {
	cmd, err := shellCommandWithShell(ctx, command, "")
	if err != nil {
		if runtime.GOOS == "windows" {
			return exec.CommandContext(ctx, "cmd", "/C", command)
		}
		return exec.CommandContext(ctx, "sh", "-c", command)
	}
	return cmd
}

func shellCommandWithShell(ctx context.Context, command string, shellName string) (*exec.Cmd, error) {
	switch normalizeShellName(shellName) {
	case "", "auto":
		if runtime.GOOS == "windows" {
			return exec.CommandContext(ctx, "cmd", "/C", command), nil
		}
		return exec.CommandContext(ctx, "sh", "-c", command), nil
	case "cmd":
		if runtime.GOOS != "windows" {
			return nil, fmt.Errorf("shell cmd is only supported on Windows")
		}
		return exec.CommandContext(ctx, "cmd", "/C", command), nil
	case "powershell":
		exe, err := findShellExecutable("pwsh", "powershell")
		if err != nil {
			return nil, err
		}
		return exec.CommandContext(ctx, exe, "-NoProfile", "-Command", command), nil
	case "pwsh":
		exe, err := findShellExecutable("pwsh")
		if err != nil {
			return nil, err
		}
		return exec.CommandContext(ctx, exe, "-NoProfile", "-Command", command), nil
	case "sh":
		return exec.CommandContext(ctx, "sh", "-c", command), nil
	case "bash":
		return exec.CommandContext(ctx, "bash", "-lc", command), nil
	default:
		return nil, fmt.Errorf("unsupported shell: %s", shellName)
	}
}

func normalizeShellName(shellName string) string {
	return strings.ToLower(strings.TrimSpace(shellName))
}

func findShellExecutable(candidates ...string) (string, error) {
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("shell executable not found: %s", strings.Join(candidates, ", "))
}

func ensureWriteAllowed(targetPath string, workingDir string, permissionLevel string) error {
	level := strings.TrimSpace(strings.ToLower(permissionLevel))
	if level == "" || level == "limited" {
		base := workingDir
		if base == "" {
			base = "."
		}
		absBase, err := filepath.Abs(base)
		if err != nil {
			return fmt.Errorf("failed to resolve working dir: %w", err)
		}
		absTarget, err := filepath.Abs(targetPath)
		if err != nil {
			return fmt.Errorf("failed to resolve target path: %w", err)
		}
		rel, err := filepath.Rel(absBase, absTarget)
		if err != nil {
			return fmt.Errorf("failed to validate path: %w", err)
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("permission denied: limited agent can only write inside working directory")
		}
		return nil
	}
	if level == "read-only" {
		return fmt.Errorf("permission denied: current agent is read-only")
	}
	return nil
}

func isDangerousCommand(command string, patterns []string) bool {
	lower := strings.ToLower(strings.TrimSpace(command))
	for _, pattern := range patterns {
		if strings.Contains(lower, strings.ToLower(strings.TrimSpace(pattern))) {
			return true
		}
	}
	return false
}

func reviewCommandExecution(command string, cwd string, opts BuiltinOptions) error {
	mode := strings.TrimSpace(strings.ToLower(opts.ExecutionMode))
	if mode == "" {
		mode = "sandbox"
	}
	sandboxEnabled := opts.Sandbox != nil && opts.Sandbox.Enabled()
	if mode == "sandbox" && !sandboxEnabled {
		return fmt.Errorf("host execution denied: sandbox.execution_mode is sandbox and sandbox is not enabled")
	}
	if mode != "host-reviewed" || sandboxEnabled {
		return nil
	}
	if strings.TrimSpace(cwd) != "" {
		if err := validateProtectedPath(resolvePath(cwd, opts.WorkingDir), opts.ProtectedPaths); err != nil {
			return fmt.Errorf("host execution denied: %w", err)
		}
	}
	if err := validateCommandAgainstProtectedPaths(command, opts.ProtectedPaths); err != nil {
		return fmt.Errorf("host execution denied: %w", err)
	}
	return nil
}

func validateProtectedPath(targetPath string, protectedPaths []string) error {
	targetPath = strings.TrimSpace(targetPath)
	if targetPath == "" {
		return nil
	}
	targetNorm, err := normalizePathForCompare(targetPath)
	if err != nil {
		return err
	}
	for _, protected := range protectedPaths {
		protectedNorm, err := normalizePathForCompare(protected)
		if err != nil || protectedNorm == "" {
			continue
		}
		if pathWithin(targetNorm, protectedNorm) {
			return fmt.Errorf("access to protected path is denied: %s", targetPath)
		}
	}
	return nil
}

func validateCommandAgainstProtectedPaths(command string, protectedPaths []string) error {
	normalizedCommand := normalizeCommandForCompare(command)
	for _, protected := range protectedPaths {
		for _, token := range protectedPathTokens(protected) {
			if token != "" && strings.Contains(normalizedCommand, token) {
				return fmt.Errorf("command references protected path: %s", protected)
			}
		}
	}
	return nil
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

func normalizeCommandForCompare(command string) string {
	normalized := strings.ToLower(strings.TrimSpace(command))
	normalized = strings.ReplaceAll(normalized, "\\", "/")
	for strings.Contains(normalized, "//") {
		normalized = strings.ReplaceAll(normalized, "//", "/")
	}
	return normalized
}

func protectedPathTokens(path string) []string {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	tokens := map[string]bool{}
	add := func(value string) {
		value = normalizeCommandForCompare(value)
		if value != "" {
			tokens[value] = true
		}
	}
	add(path)
	base := strings.ToLower(filepath.Base(path))
	home, _ := os.UserHomeDir()
	if home != "" {
		if rel, err := filepath.Rel(home, path); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			rel = normalizeCommandForCompare(rel)
			add("~/" + rel)
			add("$home/" + rel)
			if runtime.GOOS == "windows" {
				add("%userprofile%/" + rel)
				add("%homepath%/" + rel)
			}
		}
	}
	switch base {
	case "documents", "desktop", "downloads", "pictures", "videos", "music", ".ssh", "appdata", "ntuser.dat", ".gnupg", ".config":
		add(base)
	}
	items := make([]string, 0, len(tokens))
	for token := range tokens {
		items = append(items, token)
	}
	return items
}

func GetTimeTool(ctx context.Context, input map[string]any) (string, error) {
	format, ok := input["format"].(string)
	if !ok || format == "" {
		format = time.RFC3339
	}

	now := time.Now()
	return now.Format(format), nil
}

func WebSearchTool(ctx context.Context, input map[string]any) (string, error) {
	query, ok := input["query"].(string)
	if !ok {
		return "", fmt.Errorf("query is required")
	}

	maxResults := 5
	if n, ok := input["max_results"].(float64); ok && n > 0 {
		maxResults = int(n)
	}

	results, err := web.Search(ctx, query, maxResults)
	if err != nil {
		return "", fmt.Errorf("search failed: %w", err)
	}

	if len(results) == 0 {
		return "No results found", nil
	}

	output := make([]string, len(results))
	for i, r := range results {
		output[i] = fmt.Sprintf("[%d] %s\n   %s\n   %s", i+1, r.Title, r.URL, r.Description)
	}

	return strings.Join(output, "\n\n"), nil
}

func FetchURLTool(ctx context.Context, input map[string]any) (string, error) {
	urlStr, ok := input["url"].(string)
	if !ok {
		return "", fmt.Errorf("url is required")
	}

	content, err := web.Fetch(ctx, urlStr)
	if err != nil {
		return "", fmt.Errorf("fetch failed: %w", err)
	}

	return content, nil
}
