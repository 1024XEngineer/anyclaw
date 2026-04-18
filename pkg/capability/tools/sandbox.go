package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/1024XEngineer/anyclaw/pkg/config"
)

type sandboxContextKey struct{}

type SandboxScope struct {
	SessionID string
	Channel   string
}

func WithSandboxScope(ctx context.Context, scope SandboxScope) context.Context {
	return context.WithValue(ctx, sandboxContextKey{}, scope)
}

func sandboxScopeFromContext(ctx context.Context) SandboxScope {
	if ctx == nil {
		return SandboxScope{}
	}
	if scope, ok := ctx.Value(sandboxContextKey{}).(SandboxScope); ok {
		return scope
	}
	return SandboxScope{}
}

type SandboxManager struct {
	config      config.SandboxConfig
	workingDir  string
	mu          sync.Mutex
	dockerNames map[string]string
}

func NewSandboxManager(cfg config.SandboxConfig, workingDir string) *SandboxManager {
	return &SandboxManager{config: cfg, workingDir: workingDir, dockerNames: map[string]string{}}
}

func (m *SandboxManager) Enabled() bool {
	return m != nil && m.config.Enabled
}

func (m *SandboxManager) ResolveExecution(ctx context.Context, requestedCwd string) (string, func(context.Context, string) (*exec.Cmd, error), error) {
	if m == nil || !m.config.Enabled {
		cwd := strings.TrimSpace(requestedCwd)
		if cwd == "" {
			cwd = m.workingDir
		}
		return cwd, nil, nil
	}
	scope := sandboxScopeFromContext(ctx)
	key := sanitizeSandboxKey(scope)
	switch strings.ToLower(strings.TrimSpace(m.config.Backend)) {
	case "docker":
		container, err := m.ensureDockerContainer(key)
		if err != nil {
			return "", nil, err
		}
		cwd := "/workspace"
		return cwd, func(cmdCtx context.Context, command string) (*exec.Cmd, error) {
			return exec.CommandContext(cmdCtx, "docker", "exec", container, "sh", "-lc", "cd /workspace && "+command), nil
		}, nil
	case "local", "filesystem", "fs":
		root, err := m.ensureLocalSandbox(key)
		if err != nil {
			return "", nil, err
		}
		return root, nil, nil
	default:
		return "", nil, fmt.Errorf("unsupported sandbox backend: %s", m.config.Backend)
	}
}

func (m *SandboxManager) ensureLocalSandbox(key string) (string, error) {
	base := strings.TrimSpace(m.config.BaseDir)
	if base == "" {
		base = filepath.Join(m.workingDir, ".anyclaw", "sandboxes")
	}
	path := filepath.Join(base, key)
	if err := os.MkdirAll(path, 0o755); err != nil {
		return "", err
	}
	return path, nil
}

func (m *SandboxManager) ensureDockerContainer(key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.dockerNames[key]; ok {
		return existing, nil
	}
	image := strings.TrimSpace(m.config.DockerImage)
	if image == "" {
		image = "alpine:3.20"
	}
	name := "anyclaw-sbx-" + key
	cmd := exec.Command("docker", "inspect", name)
	if err := cmd.Run(); err != nil {
		args := []string{"run", "-d", "--name", name}
		if network := strings.TrimSpace(m.config.DockerNetwork); network != "" {
			args = append(args, "--network", network)
		}
		args = append(args, image, "sh", "-lc", "mkdir -p /workspace && tail -f /dev/null")
		out, runErr := exec.Command("docker", args...).CombinedOutput()
		if runErr != nil {
			return "", fmt.Errorf("failed to start docker sandbox: %w - %s", runErr, string(out))
		}
	}
	m.dockerNames[key] = name
	return name, nil
}

func sanitizeSandboxKey(scope SandboxScope) string {
	parts := []string{}
	if strings.TrimSpace(scope.Channel) != "" {
		parts = append(parts, strings.TrimSpace(scope.Channel))
	}
	if strings.TrimSpace(scope.SessionID) != "" {
		parts = append(parts, strings.TrimSpace(scope.SessionID))
	}
	if len(parts) == 0 {
		parts = append(parts, "default")
	}
	joined := strings.ToLower(strings.Join(parts, "-"))
	replacer := strings.NewReplacer("/", "-", "\\", "-", ":", "-", " ", "-")
	joined = replacer.Replace(joined)
	if len(joined) > 48 {
		joined = joined[:48]
	}
	joined = strings.Trim(joined, "-.")
	if joined == "" {
		return "default"
	}
	return joined
}
