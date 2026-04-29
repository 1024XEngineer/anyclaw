package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/config"
	gatewayserver "github.com/1024XEngineer/anyclaw/pkg/gateway"
	"github.com/1024XEngineer/anyclaw/pkg/input/cli/setup"
	appRuntime "github.com/1024XEngineer/anyclaw/pkg/runtime"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	defaultDesktopConfigName = "anyclaw.json"
	desktopLaunchTimeout     = 20 * time.Second
	desktopHealthPoll        = 250 * time.Millisecond
	desktopSnapshotTimeout   = 4 * time.Second

	desktopWindowDefaultWidth  = 1480
	desktopWindowDefaultHeight = 960
	desktopWindowMinWidth      = 1080
	desktopWindowMinHeight     = 720

	desktopPetWidth     = 320
	desktopPetHeight    = 92
	desktopPetTopOffset = 18
)

var runDesktopControlUIBuild = func(ctx context.Context, repoRoot string) error {
	node, err := exec.LookPath("node")
	if err != nil {
		return fmt.Errorf("automatic control UI build requires Node.js in PATH: %w", err)
	}

	cmd := exec.CommandContext(ctx, node, filepath.Join(repoRoot, "scripts", "ui.mjs"), "build")
	cmd.Dir = repoRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run control UI build: %w", err)
	}
	return nil
}

type LaunchResult struct {
	URL        string `json:"url,omitempty"`
	Error      string `json:"error,omitempty"`
	Attached   bool   `json:"attached"`
	BundleRoot string `json:"bundleRoot,omitempty"`
	ConfigPath string `json:"configPath,omitempty"`
}

type windowBounds struct {
	X      int
	Y      int
	Width  int
	Height int
}

type PetSnapshot struct {
	Mode         string `json:"mode"`
	State        string `json:"state"`
	Label        string `json:"label"`
	Detail       string `json:"detail"`
	DashboardURL string `json:"dashboardUrl,omitempty"`
	LastEvent    string `json:"lastEvent,omitempty"`
	UpdatedAt    string `json:"updatedAt,omitempty"`
	Error        string `json:"error,omitempty"`
}

type gatewayStatusResponse struct {
	Status struct {
		Status   string `json:"status"`
		Provider string `json:"provider"`
		Model    string `json:"model"`
	} `json:"status"`
	Typing struct {
		ActiveSessions int `json:"active_sessions"`
	} `json:"typing"`
	Approvals struct {
		Pending int `json:"pending"`
	} `json:"approvals"`
	Runtime struct {
		Active int `json:"active"`
	} `json:"runtime"`
	UpdatedAt string `json:"updated_at"`
}

type gatewayEvent struct {
	Type      string         `json:"type"`
	SessionID string         `json:"session_id"`
	Timestamp string         `json:"timestamp"`
	Payload   map[string]any `json:"payload"`
}

type gatewayApproval struct {
	ID       string         `json:"id"`
	Status   string         `json:"status"`
	ToolName string         `json:"tool_name"`
	Payload  map[string]any `json:"payload"`
}

type DesktopApp struct {
	ctx context.Context

	mu sync.Mutex

	launching bool
	waiters   []chan struct{}
	cached    LaunchResult

	gatewayCancel context.CancelFunc
	configPath    string
	bundleRoot    string
	petMode       bool
	normalBounds  windowBounds
	hasBounds     bool
}

func NewDesktopApp() *DesktopApp {
	return &DesktopApp{}
}

func (a *DesktopApp) startup(ctx context.Context) {
	a.ctx = ctx
	wailsruntime.WindowSetMinSize(ctx, desktopWindowMinWidth, desktopWindowMinHeight)
}

func (a *DesktopApp) shutdown(ctx context.Context) {
	a.stopGateway()
}

func (a *DesktopApp) Launch() LaunchResult {
	a.mu.Lock()
	if a.cached.URL != "" {
		result := a.cached
		a.mu.Unlock()
		return result
	}
	if a.launching {
		waiter := make(chan struct{})
		a.waiters = append(a.waiters, waiter)
		a.mu.Unlock()
		<-waiter
		return a.Launch()
	}
	a.launching = true
	a.mu.Unlock()

	result := a.startDesktop()

	a.mu.Lock()
	if result.Error == "" {
		a.cached = result
	}
	a.launching = false
	for _, waiter := range a.waiters {
		close(waiter)
	}
	a.waiters = nil
	a.mu.Unlock()

	return result
}

func (a *DesktopApp) OpenInBrowser(url string) error {
	if strings.TrimSpace(url) == "" {
		return errors.New("empty url")
	}
	if a.ctx == nil {
		return errors.New("desktop runtime is not ready")
	}
	wailsruntime.BrowserOpenURL(a.ctx, url)
	return nil
}

func (a *DesktopApp) Close() {
	if a.ctx == nil {
		return
	}
	wailsruntime.Quit(a.ctx)
}

func (a *DesktopApp) EnterPetMode() error {
	if a.ctx == nil {
		return errors.New("desktop runtime is not ready")
	}

	a.mu.Lock()
	if !a.petMode {
		x, y := wailsruntime.WindowGetPosition(a.ctx)
		width, height := wailsruntime.WindowGetSize(a.ctx)
		a.normalBounds = windowBounds{
			X:      x,
			Y:      y,
			Width:  width,
			Height: height,
		}
		a.hasBounds = width > 0 && height > 0
	}
	a.petMode = true
	a.mu.Unlock()

	wailsruntime.WindowSetAlwaysOnTop(a.ctx, true)
	wailsruntime.WindowSetMinSize(a.ctx, desktopPetWidth, desktopPetHeight)
	wailsruntime.WindowSetSize(a.ctx, desktopPetWidth, desktopPetHeight)
	x, y := a.petWindowPosition()
	wailsruntime.WindowSetPosition(a.ctx, x, y)
	wailsruntime.WindowShow(a.ctx)
	wailsruntime.WindowUnminimise(a.ctx)
	return nil
}

func (a *DesktopApp) ExitPetMode() error {
	if a.ctx == nil {
		return errors.New("desktop runtime is not ready")
	}

	a.mu.Lock()
	bounds := a.normalBounds
	hasBounds := a.hasBounds
	a.petMode = false
	a.mu.Unlock()

	if !hasBounds || bounds.Width <= 0 || bounds.Height <= 0 {
		bounds = windowBounds{
			X:      120,
			Y:      96,
			Width:  desktopWindowDefaultWidth,
			Height: desktopWindowDefaultHeight,
		}
	}

	wailsruntime.WindowSetAlwaysOnTop(a.ctx, false)
	wailsruntime.WindowSetMinSize(a.ctx, desktopWindowMinWidth, desktopWindowMinHeight)
	wailsruntime.WindowSetSize(a.ctx, bounds.Width, bounds.Height)
	wailsruntime.WindowSetPosition(a.ctx, bounds.X, bounds.Y)
	wailsruntime.WindowShow(a.ctx)
	wailsruntime.WindowUnminimise(a.ctx)
	return nil
}

func (a *DesktopApp) PetSnapshot() PetSnapshot {
	mode := "normal"
	a.mu.Lock()
	if a.petMode {
		mode = "pet"
	}
	configPath := a.configPath
	bundleRoot := a.bundleRoot
	a.mu.Unlock()

	if configPath == "" {
		bundleRoot = discoverBundleRoot()
		configPath = resolveDesktopConfigPath(bundleRoot)
	}

	snapshot := PetSnapshot{
		Mode:      mode,
		State:     "booting",
		Label:     "正在启动",
		Detail:    "正在连接本地网关",
		UpdatedAt: time.Now().Format(time.RFC3339),
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		snapshot.State = "error"
		snapshot.Label = "配置异常"
		snapshot.Detail = "无法读取桌面配置"
		snapshot.Error = err.Error()
		return snapshot
	}

	snapshot.DashboardURL = controlUIURL(cfg)
	baseURL := gatewayBaseURL(cfg)
	if !gatewayHealthy(baseURL) {
		snapshot.State = "offline"
		snapshot.Label = "离线"
		snapshot.Detail = "本地网关尚未连接"
		return snapshot
	}

	ctx, cancel := context.WithTimeout(context.Background(), desktopSnapshotTimeout)
	defer cancel()

	var status gatewayStatusResponse
	if err := doGatewayJSONRequest(ctx, cfg, http.MethodGet, "/status?extended=true", nil, &status); err != nil {
		snapshot.State = "offline"
		snapshot.Label = "离线"
		snapshot.Detail = "本地网关响应异常"
		snapshot.Error = err.Error()
		return snapshot
	}

	var events []gatewayEvent
	if err := doGatewayJSONRequest(ctx, cfg, http.MethodGet, "/events?limit=8", nil, &events); err != nil {
		events = nil
	}

	var approvals []gatewayApproval
	if status.Approvals.Pending > 0 {
		if err := doGatewayJSONRequest(ctx, cfg, http.MethodGet, "/approvals?status=pending", nil, &approvals); err != nil {
			approvals = nil
		}
	}

	state, label, detail, lastEvent := derivePetState(status, events, approvals)
	snapshot.State = state
	snapshot.Label = label
	snapshot.Detail = detail
	snapshot.LastEvent = lastEvent
	if status.UpdatedAt != "" {
		snapshot.UpdatedAt = status.UpdatedAt
	}
	return snapshot
}

func (a *DesktopApp) startDesktop() LaunchResult {
	bundleRoot := discoverBundleRoot()
	configPath := resolveDesktopConfigPath(bundleRoot)

	if err := ensureDesktopConfig(configPath, bundleRoot); err != nil {
		return LaunchResult{
			Error:      err.Error(),
			BundleRoot: bundleRoot,
			ConfigPath: configPath,
		}
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return LaunchResult{
			Error:      fmt.Sprintf("load config: %v", err),
			BundleRoot: bundleRoot,
			ConfigPath: configPath,
		}
	}

	if err := ensureDesktopControlUIBuilt(context.Background(), configPath, bundleRoot); err != nil {
		return LaunchResult{
			Error:      err.Error(),
			BundleRoot: bundleRoot,
			ConfigPath: configPath,
		}
	}

	baseURL := gatewayBaseURL(cfg)
	result := LaunchResult{
		Attached:   false,
		BundleRoot: bundleRoot,
		ConfigPath: configPath,
		URL:        controlUIURL(cfg),
	}

	if gatewayHealthy(baseURL) {
		result.Attached = true
		return result
	}

	app, err := appRuntime.Bootstrap(appRuntime.BootstrapOptions{
		ConfigPath: configPath,
	})
	if err != nil {
		result.Error = fmt.Sprintf("bootstrap gateway: %v", err)
		return result
	}

	// Worker mode would spawn additional GUI processes when launched from the desktop shell.
	app.Config.Gateway.WorkerCount = 0

	runCtx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	server := gatewayserver.New(app)

	go func() {
		errCh <- server.Run(runCtx)
	}()

	if err := waitForGateway(baseURL, errCh, desktopLaunchTimeout); err != nil {
		cancel()
		result.Error = fmt.Sprintf("start gateway: %v", err)
		return result
	}

	a.mu.Lock()
	a.gatewayCancel = cancel
	a.configPath = configPath
	a.bundleRoot = bundleRoot
	a.mu.Unlock()

	go func() {
		err := <-errCh
		if err == nil || errors.Is(err, context.Canceled) {
			return
		}
		fmt.Fprintf(os.Stderr, "anyclaw desktop gateway stopped: %v\n", err)
	}()

	return result
}

func (a *DesktopApp) stopGateway() {
	a.mu.Lock()
	cancel := a.gatewayCancel
	a.gatewayCancel = nil
	a.cached = LaunchResult{}
	a.configPath = ""
	a.bundleRoot = ""
	a.petMode = false
	a.mu.Unlock()

	if cancel != nil {
		cancel()
	}
}

func discoverBundleRoot() string {
	candidates := make([]string, 0, 3)

	if env := strings.TrimSpace(os.Getenv("ANYCLAW_DESKTOP_ROOT")); env != "" {
		candidates = append(candidates, env)
	}

	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, cwd)
	}

	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Dir(exe))
	}

	if root := selectDesktopBundleRoot(candidates); root != "" {
		return root
	}

	if cwd, err := os.Getwd(); err == nil {
		if abs, absErr := filepath.Abs(cwd); absErr == nil {
			return abs
		}
		return cwd
	}

	return ""
}

func resolveDesktopConfigPath(bundleRoot string) string {
	cwd := ""
	if resolved, err := os.Getwd(); err == nil {
		cwd = resolved
	}

	userConfigDir := ""
	if resolved, err := os.UserConfigDir(); err == nil {
		userConfigDir = resolved
	}

	return resolveDesktopConfigPathWith(
		bundleRoot,
		cwd,
		userConfigDir,
		strings.TrimSpace(os.Getenv("ANYCLAW_DESKTOP_CONFIG")),
	)
}

func selectDesktopBundleRoot(startPoints []string) string {
	bestPath := ""
	bestScore := -1

	for _, candidate := range expandDesktopPathCandidates(startPoints) {
		score := scoreDesktopBundleRoot(candidate)
		if score > bestScore {
			bestPath = candidate
			bestScore = score
		}
	}

	if bestScore <= 0 {
		return ""
	}

	return bestPath
}

func expandDesktopPathCandidates(startPoints []string) []string {
	seen := map[string]bool{}
	candidates := make([]string, 0, len(startPoints)*4)

	for _, point := range startPoints {
		point = strings.TrimSpace(point)
		if point == "" {
			continue
		}

		abs, err := filepath.Abs(point)
		if err != nil {
			continue
		}

		current := filepath.Clean(abs)
		for {
			if !seen[current] {
				seen[current] = true
				candidates = append(candidates, current)
			}

			parent := filepath.Dir(current)
			if parent == current {
				break
			}
			current = parent
		}
	}

	return candidates
}

func scoreDesktopBundleRoot(path string) int {
	score := 0

	if pathExists(filepath.Join(path, "dist", "control-ui")) {
		score += 4
	}
	if pathExists(filepath.Join(path, "skills")) {
		score += 3
	}
	if pathExists(filepath.Join(path, "plugins")) {
		score += 2
	}
	if pathExists(filepath.Join(path, "cmd", "anyclaw-desktop")) {
		score += 2
	}
	if pathExists(filepath.Join(path, "go.mod")) {
		score++
	}
	if pathExists(filepath.Join(path, defaultDesktopConfigName)) {
		score++
	}

	return score
}

func resolveDesktopConfigPathWith(bundleRoot string, cwd string, userConfigDir string, envConfigPath string) string {
	if raw := strings.TrimSpace(envConfigPath); raw != "" {
		return resolveAbsPath(raw)
	}

	if bundleRoot = strings.TrimSpace(bundleRoot); bundleRoot != "" {
		bundleConfig := filepath.Join(bundleRoot, defaultDesktopConfigName)
		if pathExists(bundleConfig) {
			return bundleConfig
		}
	}

	if cwd = strings.TrimSpace(cwd); cwd != "" {
		cwdConfig := filepath.Join(cwd, defaultDesktopConfigName)
		if pathExists(cwdConfig) {
			return cwdConfig
		}
	}

	if userConfigDir = strings.TrimSpace(userConfigDir); userConfigDir != "" {
		return filepath.Join(userConfigDir, "AnyClaw", defaultDesktopConfigName)
	}

	if bundleRoot != "" {
		return filepath.Join(bundleRoot, defaultDesktopConfigName)
	}

	if cwd != "" {
		return filepath.Join(cwd, defaultDesktopConfigName)
	}

	return resolveAbsPath(defaultDesktopConfigName)
}

func ensureDesktopConfig(configPath string, bundleRoot string) error {
	if pathExists(configPath) {
		return nil
	}

	cfg := config.DefaultConfig()
	configDir := filepath.Dir(configPath)

	cfg.Agent.WorkDir = filepath.Join(configDir, ".anyclaw")
	cfg.Agent.WorkingDir = filepath.Join(configDir, "workflows", "default")

	if bundleRoot != "" {
		if skillsDir := filepath.Join(bundleRoot, "skills"); pathExists(skillsDir) {
			cfg.Skills.Dir = skillsDir
		}
		if pluginsDir := filepath.Join(bundleRoot, "plugins"); pathExists(pluginsDir) {
			cfg.Plugins.Dir = pluginsDir
		}
		if controlUIRoot := filepath.Join(bundleRoot, "dist", "control-ui"); pathExists(controlUIRoot) {
			cfg.Gateway.ControlUI.Root = controlUIRoot
		}
	}

	setup.EnsurePrimaryProviderProfile(cfg, cfg.LLM.Provider, cfg.LLM.Model, cfg.LLM.APIKey, cfg.LLM.BaseURL)

	return cfg.Save(configPath)
}

func ensureDesktopControlUIBuilt(ctx context.Context, configPath string, bundleRoot string) error {
	if envRoot := strings.TrimSpace(os.Getenv("ANYCLAW_CONTROL_UI_ROOT")); envRoot != "" {
		if desktopControlUIBuildExists(envRoot) {
			return nil
		}
		return fmt.Errorf("ANYCLAW_CONTROL_UI_ROOT points to a missing control UI build: %s", envRoot)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config for control UI: %w", err)
	}

	configuredRoot := config.ResolvePath(configPath, cfg.Gateway.ControlUI.Root)
	if desktopControlUIBuildExists(configuredRoot) {
		return nil
	}

	buildRoot := ""
	if strings.TrimSpace(bundleRoot) != "" {
		buildRoot = filepath.Join(bundleRoot, "dist", "control-ui")
		if desktopControlUIBuildExists(buildRoot) {
			return os.Setenv("ANYCLAW_CONTROL_UI_ROOT", buildRoot)
		}
	}

	repoRoot, ok := discoverDesktopControlUIRepoRoot(configPath, bundleRoot)
	if !ok {
		if configuredRoot != "" {
			return fmt.Errorf("configured control UI root is missing: %s", configuredRoot)
		}
		return fmt.Errorf("control UI build is missing; run `corepack pnpm -C ui build` from the repo root or set ANYCLAW_CONTROL_UI_ROOT")
	}

	buildRoot = filepath.Join(repoRoot, "dist", "control-ui")
	if desktopControlUIBuildExists(buildRoot) {
		return os.Setenv("ANYCLAW_CONTROL_UI_ROOT", buildRoot)
	}

	if err := runDesktopControlUIBuild(ctx, repoRoot); err != nil {
		return fmt.Errorf("auto-build control UI: %w", err)
	}
	if !desktopControlUIBuildExists(buildRoot) {
		return fmt.Errorf("control UI build completed but %s is still missing", buildRoot)
	}
	return os.Setenv("ANYCLAW_CONTROL_UI_ROOT", buildRoot)
}

func desktopControlUIBuildExists(root string) bool {
	root = strings.TrimSpace(root)
	if root == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(root, "index.html"))
	return err == nil && !info.IsDir()
}

func discoverDesktopControlUIRepoRoot(configPath string, bundleRoot string) (string, bool) {
	starts := []string{bundleRoot}
	if cwd, err := os.Getwd(); err == nil && strings.TrimSpace(cwd) != "" {
		starts = append(starts, cwd)
	}
	if resolvedConfig := config.ResolveConfigPath(configPath); strings.TrimSpace(resolvedConfig) != "" {
		starts = append(starts, filepath.Dir(resolvedConfig))
	}
	if executable, err := os.Executable(); err == nil && strings.TrimSpace(executable) != "" {
		starts = append(starts, filepath.Dir(executable))
	}

	seen := map[string]bool{}
	for _, start := range starts {
		for _, dir := range expandDesktopPathCandidates([]string{start}) {
			if seen[dir] {
				continue
			}
			seen[dir] = true
			if looksLikeDesktopControlUIRepoRoot(dir) {
				return dir, true
			}
		}
	}

	return "", false
}

func looksLikeDesktopControlUIRepoRoot(dir string) bool {
	required := []string{
		filepath.Join(dir, "package.json"),
		filepath.Join(dir, "scripts", "ui.mjs"),
		filepath.Join(dir, "ui", "package.json"),
		filepath.Join(dir, "cmd", "anyclaw"),
	}
	for _, item := range required {
		if _, err := os.Stat(item); err != nil {
			return false
		}
	}
	return true
}

func resolveAbsPath(path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(abs)
}

func pathExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func gatewayBaseURL(cfg *config.Config) string {
	host := "127.0.0.1"
	if cfg != nil {
		candidate := strings.TrimSpace(cfg.Gateway.Host)
		switch candidate {
		case "", "0.0.0.0", "::", "[::]":
		default:
			host = candidate
		}
	}

	port := 18789
	if cfg != nil && cfg.Gateway.Port > 0 {
		port = cfg.Gateway.Port
	}

	return "http://" + net.JoinHostPort(host, fmt.Sprintf("%d", port))
}

func (a *DesktopApp) petWindowPosition() (int, int) {
	if a.ctx == nil {
		return 32, desktopPetTopOffset
	}

	screenWidth := 1440
	screens, err := wailsruntime.ScreenGetAll(a.ctx)
	if err == nil {
		for _, screen := range screens {
			if screen.IsCurrent || screen.IsPrimary {
				if screen.Size.Width > 0 {
					screenWidth = screen.Size.Width
				}
				break
			}
		}
	}

	x := (screenWidth - desktopPetWidth) / 2
	if x < 24 {
		x = 24
	}
	return x, desktopPetTopOffset
}

func controlUIURL(cfg *config.Config) string {
	baseURL := gatewayBaseURL(cfg)
	basePath := "/dashboard"
	if cfg != nil {
		if candidate := strings.TrimSpace(cfg.Gateway.ControlUI.BasePath); candidate != "" {
			basePath = candidate
		}
	}
	if !strings.HasPrefix(basePath, "/") {
		basePath = "/" + basePath
	}
	return strings.TrimRight(baseURL, "/") + basePath
}

func gatewayHealthy(baseURL string) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(strings.TrimRight(baseURL, "/") + "/healthz")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func waitForGateway(baseURL string, errCh <-chan error, timeout time.Duration) error {
	if gatewayHealthy(baseURL) {
		return nil
	}

	deadline := time.Now().Add(timeout)
	for {
		select {
		case err := <-errCh:
			if err == nil || errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		default:
		}

		if gatewayHealthy(baseURL) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for %s", baseURL)
		}
		time.Sleep(desktopHealthPoll)
	}
}

func doGatewayJSONRequest(ctx context.Context, cfg *config.Config, method string, path string, requestBody any, responseBody any) error {
	var body io.Reader
	if requestBody != nil {
		data, err := json.Marshal(requestBody)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}

	path = strings.TrimSpace(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(gatewayBaseURL(cfg), "/")+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if cfg != nil {
		if token := strings.TrimSpace(cfg.Security.APIToken); token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}

	resp, err := (&http.Client{Timeout: desktopSnapshotTimeout}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(resp.Body)
		if len(payload) == 0 {
			return fmt.Errorf("gateway returned %s", resp.Status)
		}
		return fmt.Errorf("gateway returned %s: %s", resp.Status, strings.TrimSpace(string(payload)))
	}

	if responseBody == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}

	return json.NewDecoder(resp.Body).Decode(responseBody)
}

func derivePetState(status gatewayStatusResponse, events []gatewayEvent, approvals []gatewayApproval) (string, string, string, string) {
	lastType := ""
	lastAt := time.Time{}
	if len(events) > 0 {
		last := events[len(events)-1]
		lastType = strings.TrimSpace(last.Type)
		if parsed, err := time.Parse(time.RFC3339, last.Timestamp); err == nil {
			lastAt = parsed
		}
	}

	switch {
	case status.Approvals.Pending > 0:
		detail := "有操作正在等待你批准"
		if summary := summarizePendingApproval(approvals); summary != "" {
			detail = summary
		}
		return "waiting", "等待确认", detail, lastType
	case status.Runtime.Active > 0:
		if lastType == "tool.activity" || strings.HasPrefix(lastType, "task.") {
			return "executing", "正在执行", "桌宠正在调用工具处理任务", lastType
		}
		return "thinking", "正在思考", "桌宠正在整理上下文并生成回复", lastType
	case status.Typing.ActiveSessions > 0:
		return "thinking", "正在思考", "桌宠正在继续当前对话", lastType
	case recentEvent(lastType, lastAt, 8*time.Second, "chat.failed", "tts.process.error", "stt.init.error", "tts.init.error"):
		return "error", "出了点问题", "最近一次执行出现异常", lastType
	case recentEvent(lastType, lastAt, 5*time.Second, "chat.completed", "task.completed", "approval.resolved"):
		return "complete", "刚刚完成", "上一轮动作已经结束", lastType
	default:
		detail := "网关在线，随时可以继续工作"
		if strings.TrimSpace(status.Status.Model) != "" {
			detail = fmt.Sprintf("%s · %s", status.Status.Provider, status.Status.Model)
		}
		return "online", "在线", detail, lastType
	}
}

func summarizePendingApproval(approvals []gatewayApproval) string {
	for _, approval := range approvals {
		if !strings.EqualFold(strings.TrimSpace(approval.Status), "pending") {
			continue
		}
		if summary := summarizeApprovalPayload(approval.Payload); summary != "" {
			return "等待批准：" + summary
		}
		if toolName := strings.TrimSpace(approval.ToolName); toolName != "" {
			return "等待批准：" + toolName
		}
	}
	return ""
}

func summarizeApprovalPayload(payload map[string]any) string {
	if len(payload) == 0 {
		return ""
	}
	if message, ok := payload["message"].(string); ok && strings.TrimSpace(message) != "" {
		return strings.TrimSpace(message)
	}
	if title, ok := payload["title"].(string); ok && strings.TrimSpace(title) != "" {
		return strings.TrimSpace(title)
	}
	if args, ok := payload["args"].(map[string]any); ok {
		for _, key := range []string{"command", "target", "url", "path", "text"} {
			if summary := approvalValueSummary(args[key]); summary != "" {
				return summary
			}
		}
	}
	for _, key := range []string{"command", "target", "url", "path", "text"} {
		if summary := approvalValueSummary(payload[key]); summary != "" {
			return summary
		}
	}
	return ""
}

func approvalValueSummary(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return ""
	}
}

func recentEvent(lastType string, at time.Time, within time.Duration, kinds ...string) bool {
	if lastType == "" || at.IsZero() {
		return false
	}
	if time.Since(at) > within {
		return false
	}
	for _, kind := range kinds {
		if strings.EqualFold(lastType, kind) {
			return true
		}
	}
	return false
}
