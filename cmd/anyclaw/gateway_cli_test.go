package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/1024XEngineer/anyclaw/pkg/config"
	"github.com/1024XEngineer/anyclaw/pkg/gateway"
	appRuntime "github.com/1024XEngineer/anyclaw/pkg/runtime"
)

func TestRunAnyClawCLIRoutesGatewayCommand(t *testing.T) {
	clearModelsCLIEnv(t)

	stdout, _, err := captureCLIOutput(t, func() error {
		return runAnyClawCLI([]string{"gateway"})
	})
	if err != nil {
		t.Fatalf("runAnyClawCLI gateway: %v", err)
	}
	if !strings.Contains(stdout, "AnyClaw gateway commands:") {
		t.Fatalf("expected gateway usage output, got %q", stdout)
	}
}

func TestRunGatewayCommandUnknownSubcommandPrintsUsage(t *testing.T) {
	clearModelsCLIEnv(t)

	stdout, _, err := captureCLIOutput(t, func() error {
		return runGatewayCommand(context.Background(), []string{"unknown"})
	})
	if err == nil || !strings.Contains(err.Error(), "unknown gateway command") {
		t.Fatalf("expected unknown gateway command error, got %v", err)
	}
	if !strings.Contains(stdout, "AnyClaw gateway commands:") {
		t.Fatalf("expected gateway usage output, got %q", stdout)
	}
}

func TestRunGatewayCommandRoutesRunSubcommand(t *testing.T) {
	clearModelsCLIEnv(t)

	configPath := writeDefaultCLIConfig(t)
	controlUIRoot := filepath.Join(t.TempDir(), "control-ui")
	mustWriteGatewayFile(t, filepath.Join(controlUIRoot, "index.html"), "<html>ok</html>")
	t.Setenv("ANYCLAW_CONTROL_UI_ROOT", controlUIRoot)

	originalBootstrap := bootstrapGatewayRuntime
	originalRunner := runGatewayRuntime
	t.Cleanup(func() {
		bootstrapGatewayRuntime = originalBootstrap
		runGatewayRuntime = originalRunner
	})

	called := false
	bootstrapGatewayRuntime = func(opts appRuntime.BootstrapOptions) (*appRuntime.MainRuntime, error) {
		if opts.ConfigPath != configPath {
			t.Fatalf("expected bootstrap config path %q, got %q", configPath, opts.ConfigPath)
		}
		cfg := config.DefaultConfig()
		return &appRuntime.MainRuntime{Config: cfg, ConfigPath: opts.ConfigPath}, nil
	}
	runGatewayRuntime = func(ctx context.Context, app *appRuntime.MainRuntime) error {
		called = true
		if app.Config.Gateway.Host != "0.0.0.0" {
			t.Fatalf("expected overridden host, got %q", app.Config.Gateway.Host)
		}
		if app.Config.Gateway.Port != 19000 {
			t.Fatalf("expected overridden port, got %d", app.Config.Gateway.Port)
		}
		if app.Config.Gateway.WorkerCount != 2 {
			t.Fatalf("expected overridden workers, got %d", app.Config.Gateway.WorkerCount)
		}
		return nil
	}

	stdout, _, err := captureCLIOutput(t, func() error {
		return runGatewayCommand(context.Background(), []string{"run", "--config", configPath, "--host", "0.0.0.0", "--port", "19000", "--workers", "2"})
	})
	if err != nil {
		t.Fatalf("runGatewayCommand run: %v", err)
	}
	if !called {
		t.Fatal("expected gateway runtime runner to execute")
	}
	for _, want := range []string{
		"Gateway workers: 2",
		"Gateway listening on 0.0.0.0:19000",
		"Health: http://0.0.0.0:19000/healthz",
		"Status: http://0.0.0.0:19000/status",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected %q in output, got %q", want, stdout)
		}
	}
}

func TestRunGatewayCommandRoutesDaemonSubcommand(t *testing.T) {
	clearModelsCLIEnv(t)

	tempDir := t.TempDir()
	restoreWorkingDir(t)
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}

	configPath := filepath.Join(tempDir, "configs", "daemon.json")
	cfg := config.DefaultConfig()
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("save daemon config: %v", err)
	}

	controlUIRoot := filepath.Join(tempDir, "control-ui")
	mustWriteGatewayFile(t, filepath.Join(controlUIRoot, "index.html"), "<html>ok</html>")
	t.Setenv("ANYCLAW_CONTROL_UI_ROOT", controlUIRoot)

	originalBootstrap := bootstrapGatewayRuntime
	originalStart := startGatewayDaemon
	originalStop := stopGatewayDaemon
	t.Cleanup(func() {
		bootstrapGatewayRuntime = originalBootstrap
		startGatewayDaemon = originalStart
		stopGatewayDaemon = originalStop
	})

	bootstrapGatewayRuntime = func(opts appRuntime.BootstrapOptions) (*appRuntime.MainRuntime, error) {
		if opts.ConfigPath != configPath {
			t.Fatalf("expected daemon config path %q, got %q", configPath, opts.ConfigPath)
		}
		return &appRuntime.MainRuntime{Config: config.DefaultConfig(), ConfigPath: opts.ConfigPath}, nil
	}

	started := false
	stopped := false
	startGatewayDaemon = func(app *appRuntime.MainRuntime) error {
		started = true
		return nil
	}
	stopGatewayDaemon = func(app *appRuntime.MainRuntime) error {
		stopped = true
		return nil
	}

	stdout, _, err := captureCLIOutput(t, func() error {
		return runGatewayCommand(context.Background(), []string{"daemon", "start", "--config", configPath})
	})
	if err != nil {
		t.Fatalf("runGatewayCommand daemon start: %v", err)
	}
	if !started || !strings.Contains(stdout, "Gateway daemon started") {
		t.Fatalf("expected daemon start output, got started=%v stdout=%q", started, stdout)
	}

	stdout, _, err = captureCLIOutput(t, func() error {
		return runGatewayCommand(context.Background(), []string{"daemon", "stop", "--config", configPath})
	})
	if err != nil {
		t.Fatalf("runGatewayCommand daemon stop: %v", err)
	}
	if !stopped || !strings.Contains(stdout, "Gateway daemon stopped") {
		t.Fatalf("expected daemon stop output, got stopped=%v stdout=%q", stopped, stdout)
	}
}

func TestRunGatewayDaemonRejectsUnexpectedTrailingArgs(t *testing.T) {
	clearModelsCLIEnv(t)

	err := runGatewayDaemon([]string{"start", "extra"})
	if err == nil || !strings.Contains(err.Error(), "usage: anyclaw gateway daemon <start|stop> [--config <path>]") {
		t.Fatalf("expected daemon usage error for trailing args, got %v", err)
	}
}

func TestRunGatewayCommandRoutesStatusSubcommand(t *testing.T) {
	clearModelsCLIEnv(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(gateway.Status{Status: "running", Provider: "openai", Model: "gpt-4o-mini"})
	}))
	defer server.Close()

	configPath := writeGatewayServerConfig(t, server.URL)
	stdout, _, err := captureCLIOutput(t, func() error {
		return runGatewayCommand(context.Background(), []string{"status", "--config", configPath})
	})
	if err != nil {
		t.Fatalf("runGatewayCommand status: %v", err)
	}
	if !strings.Contains(stdout, "Gateway is running") {
		t.Fatalf("expected gateway status output, got %q", stdout)
	}
}

func TestNormalizeGatewayCommandSupportsStartAlias(t *testing.T) {
	if got := normalizeGatewayCommand("start"); got != "run" {
		t.Fatalf("expected start alias to normalize to run, got %q", got)
	}
	if got := normalizeGatewayCommand(" RUN "); got != "run" {
		t.Fatalf("expected run command to normalize to run, got %q", got)
	}
}

func TestGatewayHTTPHelpersBuildRequests(t *testing.T) {
	clearModelsCLIEnv(t)

	cfg := config.DefaultConfig()
	cfg.Gateway.Host = "127.0.0.1"
	cfg.Gateway.Port = 18789
	cfg.Security.APIToken = "secret-token"

	if got := gatewayHTTPBaseURL(cfg); got != "http://127.0.0.1:18789" {
		t.Fatalf("unexpected gateway base URL: %q", got)
	}
	if got := gatewayURL(cfg, "status"); got != "http://127.0.0.1:18789/status" {
		t.Fatalf("unexpected gateway URL: %q", got)
	}

	req, err := newGatewayRequest(context.Background(), cfg, http.MethodPost, "/status", strings.NewReader(`{"ok":true}`))
	if err != nil {
		t.Fatalf("newGatewayRequest: %v", err)
	}
	if got := req.Header.Get("Accept"); got != "application/json" {
		t.Fatalf("unexpected Accept header: %q", got)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer secret-token" {
		t.Fatalf("unexpected Authorization header: %q", got)
	}
	if got := req.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("unexpected Content-Type header: %q", got)
	}
	if got := req.URL.String(); got != "http://127.0.0.1:18789/status" {
		t.Fatalf("unexpected request URL: %q", got)
	}
}

func TestDoGatewayJSONRequestHandlesSuccessAndErrors(t *testing.T) {
	clearModelsCLIEnv(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/status":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "ok",
			})
		case "/bad":
			http.Error(w, "gateway unavailable", http.StatusServiceUnavailable)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := gatewayTestConfigFromURL(t, server.URL)

	var payload struct {
		Status string `json:"status"`
	}
	if err := doGatewayJSONRequest(context.Background(), cfg, httpMethodGet, "/status", nil, &payload); err != nil {
		t.Fatalf("doGatewayJSONRequest success: %v", err)
	}
	if payload.Status != "ok" {
		t.Fatalf("unexpected status payload: %#v", payload)
	}

	err := doGatewayJSONRequest(context.Background(), cfg, httpMethodGet, "/bad", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "503 Service Unavailable") || !strings.Contains(err.Error(), "gateway unavailable") {
		t.Fatalf("expected gateway error details, got %v", err)
	}
}

func TestRunGatewayStatusPrintsSummary(t *testing.T) {
	clearModelsCLIEnv(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(gateway.Status{
			Status:   "running",
			Address:  "127.0.0.1:18789",
			Provider: "openai",
			Model:    "gpt-4o-mini",
			Sessions: 2,
			Events:   4,
			Tools:    8,
			Skills:   3,
		})
	}))
	defer server.Close()

	configPath := writeGatewayServerConfig(t, server.URL)
	stdout, _, err := captureCLIOutput(t, func() error {
		return runGatewayStatus([]string{"--config", configPath})
	})
	if err != nil {
		t.Fatalf("runGatewayStatus: %v", err)
	}
	for _, want := range []string{
		"Gateway is running",
		"Address: 127.0.0.1:18789",
		"Provider: openai",
		"Model: gpt-4o-mini",
		"Sessions: 2",
		"Events: 4",
		"Tools: 8",
		"Skills: 3",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected %q in output, got %q", want, stdout)
		}
	}
}

func TestRunGatewaySessionsHandlesEmptyAndNonEmptyResults(t *testing.T) {
	clearModelsCLIEnv(t)

	t.Run("empty", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/sessions" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		}))
		defer server.Close()

		configPath := writeGatewayServerConfig(t, server.URL)
		stdout, _, err := captureCLIOutput(t, func() error {
			return runGatewaySessions([]string{"--config", configPath})
		})
		if err != nil {
			t.Fatalf("runGatewaySessions empty: %v", err)
		}
		if !strings.Contains(stdout, "No gateway sessions yet") {
			t.Fatalf("unexpected empty sessions output: %q", stdout)
		}
	})

	t.Run("non-empty", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/sessions" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id":            "session-1",
					"title":         "Demo Session",
					"message_count": 5,
					"updated_at":    "2026-04-24T10:00:00Z",
				},
			})
		}))
		defer server.Close()

		configPath := writeGatewayServerConfig(t, server.URL)
		stdout, _, err := captureCLIOutput(t, func() error {
			return runGatewaySessions([]string{"--config", configPath})
		})
		if err != nil {
			t.Fatalf("runGatewaySessions non-empty: %v", err)
		}
		for _, want := range []string{
			"Found 1 gateway session(s)",
			"Demo Session",
			"id=session-1 messages=5 updated=2026-04-24T10:00:00Z",
		} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("expected %q in output, got %q", want, stdout)
			}
		}
	})
}

func TestRunGatewayEventsHandlesListAndStreamModes(t *testing.T) {
	clearModelsCLIEnv(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/events":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id":         "evt-1",
					"type":       "message.created",
					"session_id": "session-1",
					"timestamp":  "2026-04-24T11:00:00Z",
				},
			})
		case "/events/stream":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("event: message\n"))
			_, _ = w.Write([]byte("data: hello\n\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	configPath := writeGatewayServerConfig(t, server.URL)

	stdout, _, err := captureCLIOutput(t, func() error {
		return runGatewayEvents([]string{"--config", configPath})
	})
	if err != nil {
		t.Fatalf("runGatewayEvents list: %v", err)
	}
	if !strings.Contains(stdout, "Found 1 gateway event(s)") || !strings.Contains(stdout, "- message.created session=session-1 at 2026-04-24T11:00:00Z id=evt-1") {
		t.Fatalf("unexpected events list output: %q", stdout)
	}

	stdout, _, err = captureCLIOutput(t, func() error {
		return runGatewayEvents([]string{"--config", configPath, "--stream", "--replay", "2"})
	})
	if err != nil {
		t.Fatalf("runGatewayEvents stream: %v", err)
	}
	for _, want := range []string{
		"Streaming events from ",
		"event: message",
		"data: hello",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected %q in stream output, got %q", want, stdout)
		}
	}
}

func TestRunGatewayServerAndDaemonSurfaceEarlyErrors(t *testing.T) {
	clearModelsCLIEnv(t)

	if err := runGatewayDaemon(nil); err == nil || !strings.Contains(err.Error(), "usage: anyclaw gateway daemon <start|stop>") {
		t.Fatalf("expected daemon usage error, got %v", err)
	}

	configPath := writeDefaultCLIConfig(t)
	t.Setenv("ANYCLAW_CONTROL_UI_ROOT", filepath.Join(t.TempDir(), "missing-ui"))
	err := runGatewayServer(context.Background(), []string{"--config", configPath})
	if err == nil || !strings.Contains(err.Error(), "ANYCLAW_CONTROL_UI_ROOT points to a missing control UI build") {
		t.Fatalf("expected gateway server config load error, got %v", err)
	}
}

func TestEnsureGatewayControlUIBuiltUsesExistingBuild(t *testing.T) {
	repoRoot := createGatewayRepoFixture(t)
	buildRoot := filepath.Join(repoRoot, "dist", "control-ui")
	mustWriteGatewayFile(t, filepath.Join(buildRoot, "index.html"), "<html>ok</html>")
	configPath := writeGatewayConfig(t, repoRoot)
	restoreWorkingDir(t)

	originalRunner := runGatewayControlUIBuild
	defer func() { runGatewayControlUIBuild = originalRunner }()
	runGatewayControlUIBuild = func(context.Context, string) error {
		t.Fatal("did not expect control UI build runner to execute")
		return nil
	}

	t.Setenv("ANYCLAW_CONTROL_UI_ROOT", "")
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir repo root: %v", err)
	}

	if err := ensureGatewayControlUIBuilt(context.Background(), configPath); err != nil {
		t.Fatalf("expected existing build to pass, got %v", err)
	}
	if got := os.Getenv("ANYCLAW_CONTROL_UI_ROOT"); got != buildRoot {
		t.Fatalf("expected ANYCLAW_CONTROL_UI_ROOT=%q, got %q", buildRoot, got)
	}
}

func TestEnsureGatewayControlUIBuiltAutoBuildsMissingFrontend(t *testing.T) {
	repoRoot := createGatewayRepoFixture(t)
	configPath := writeGatewayConfig(t, repoRoot)
	buildRoot := filepath.Join(repoRoot, "dist", "control-ui")
	restoreWorkingDir(t)

	originalRunner := runGatewayControlUIBuild
	defer func() { runGatewayControlUIBuild = originalRunner }()

	called := false
	runGatewayControlUIBuild = func(_ context.Context, gotRepoRoot string) error {
		called = true
		if gotRepoRoot != repoRoot {
			t.Fatalf("expected repo root %q, got %q", repoRoot, gotRepoRoot)
		}
		mustWriteGatewayFile(t, filepath.Join(buildRoot, "index.html"), "<html>built</html>")
		return nil
	}

	t.Setenv("ANYCLAW_CONTROL_UI_ROOT", "")
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir repo root: %v", err)
	}

	if err := ensureGatewayControlUIBuilt(context.Background(), configPath); err != nil {
		t.Fatalf("expected auto-build to succeed, got %v", err)
	}
	if !called {
		t.Fatal("expected control UI build runner to execute")
	}
	if got := os.Getenv("ANYCLAW_CONTROL_UI_ROOT"); got != buildRoot {
		t.Fatalf("expected ANYCLAW_CONTROL_UI_ROOT=%q, got %q", buildRoot, got)
	}
}

func TestEnsureGatewayControlUIBuiltErrorsWhenRepoSourceIsMissing(t *testing.T) {
	tempDir := t.TempDir()
	configPath := writeGatewayConfig(t, tempDir)
	restoreWorkingDir(t)

	originalRunner := runGatewayControlUIBuild
	defer func() { runGatewayControlUIBuild = originalRunner }()
	runGatewayControlUIBuild = func(context.Context, string) error {
		t.Fatal("did not expect control UI build runner to execute")
		return nil
	}

	t.Setenv("ANYCLAW_CONTROL_UI_ROOT", "")
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}

	err := ensureGatewayControlUIBuilt(context.Background(), configPath)
	if err == nil {
		t.Fatal("expected missing repo source to fail")
	}
	if !strings.Contains(err.Error(), "corepack pnpm -C ui build") {
		t.Fatalf("expected build guidance, got %v", err)
	}
}

func createGatewayRepoFixture(t *testing.T) string {
	t.Helper()

	repoRoot := t.TempDir()
	mustWriteGatewayFile(t, filepath.Join(repoRoot, "package.json"), `{"name":"anyclaw-web-workspace"}`)
	mustWriteGatewayFile(t, filepath.Join(repoRoot, "scripts", "ui.mjs"), "console.log('build')")
	mustWriteGatewayFile(t, filepath.Join(repoRoot, "ui", "package.json"), `{"name":"@anyclaw/control-ui"}`)
	mustWriteGatewayFile(t, filepath.Join(repoRoot, "cmd", "anyclaw", "main.go"), "package main")
	return repoRoot
}

func writeGatewayConfig(t *testing.T, root string) string {
	t.Helper()

	cfg := config.DefaultConfig()
	configPath := filepath.Join(root, "anyclaw.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("save config: %v", err)
	}
	return configPath
}

func writeGatewayServerConfig(t *testing.T, serverURL string) string {
	t.Helper()

	cfg := gatewayTestConfigFromURL(t, serverURL)
	configPath := filepath.Join(t.TempDir(), "anyclaw.json")
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("save gateway server config: %v", err)
	}
	return configPath
}

func gatewayTestConfigFromURL(t *testing.T, rawURL string) *config.Config {
	t.Helper()

	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse URL %q: %v", rawURL, err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatalf("parse port from %q: %v", rawURL, err)
	}

	cfg := config.DefaultConfig()
	cfg.Gateway.Host = parsed.Hostname()
	cfg.Gateway.Port = port
	cfg.Security.APIToken = "secret-token"
	return cfg
}

func mustWriteGatewayFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func restoreWorkingDir(t *testing.T) {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})
}
