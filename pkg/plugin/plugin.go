package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/anyclaw/anyclaw/pkg/config"
	"github.com/anyclaw/anyclaw/pkg/tools"
)

type Manifest struct {
	Name           string       `json:"name"`
	Version        string       `json:"version"`
	Description    string       `json:"description"`
	Kinds          []string     `json:"kinds"`
	Builtin        bool         `json:"builtin"`
	Enabled        bool         `json:"enabled"`
	Entrypoint     string       `json:"entrypoint,omitempty"`
	Tool           *ToolSpec    `json:"tool,omitempty"`
	Ingress        *IngressSpec `json:"ingress,omitempty"`
	Channel        *ChannelSpec `json:"channel,omitempty"`
	Permissions    []string     `json:"permissions,omitempty"`
	ExecPolicy     string       `json:"exec_policy,omitempty"`
	TimeoutSeconds int          `json:"timeout_seconds,omitempty"`
	Signer         string       `json:"signer,omitempty"`
	Signature      string       `json:"signature,omitempty"`
	Trust          string       `json:"trust,omitempty"`
	Verified       bool         `json:"verified,omitempty"`
}

type ToolSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

type IngressSpec struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Description string `json:"description"`
}

type ChannelSpec struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type Registry struct {
	manifests      []Manifest
	allowExec      bool
	execTimeout    time.Duration
	trustedSigners map[string]bool
	requireTrust   bool
}

type IngressRunner struct {
	Manifest   Manifest
	Entrypoint string
	Timeout    time.Duration
}

type ChannelRunner struct {
	Manifest   Manifest
	Entrypoint string
	Timeout    time.Duration
}

func NewRegistry(cfg config.PluginsConfig) (*Registry, error) {
	timeout := time.Duration(cfg.ExecTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	trusted := map[string]bool{}
	for _, signer := range cfg.TrustedSigners {
		trusted[signer] = true
	}
	registry := &Registry{allowExec: cfg.AllowExec, execTimeout: timeout, trustedSigners: trusted, requireTrust: cfg.RequireTrust}
	registry.registerBuiltin(Manifest{Name: "telegram-channel", Version: "1.0.0", Description: "Telegram channel adapter", Kinds: []string{"channel"}, Builtin: true, Enabled: true})
	registry.registerBuiltin(Manifest{Name: "slack-channel", Version: "1.0.0", Description: "Slack channel adapter", Kinds: []string{"channel"}, Builtin: true, Enabled: true})
	registry.registerBuiltin(Manifest{Name: "discord-channel", Version: "1.0.0", Description: "Discord channel adapter", Kinds: []string{"channel"}, Builtin: true, Enabled: true})
	registry.registerBuiltin(Manifest{Name: "whatsapp-channel", Version: "1.0.0", Description: "WhatsApp channel adapter", Kinds: []string{"channel"}, Builtin: true, Enabled: true})
	registry.registerBuiltin(Manifest{Name: "signal-channel", Version: "1.0.0", Description: "Signal channel adapter", Kinds: []string{"channel"}, Builtin: true, Enabled: true})
	registry.registerBuiltin(Manifest{Name: "builtin-tools", Version: "1.0.0", Description: "Core file and web tools", Kinds: []string{"tools"}, Builtin: true, Enabled: true})
	if cfg.Dir != "" {
		if err := registry.loadDir(cfg.Dir); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}
	registry.verifySignatures(cfg.Dir)
	registry.applyEnabled(cfg.Enabled)
	return registry, nil
}

func (r *Registry) registerBuiltin(manifest Manifest) {
	r.manifests = append(r.manifests, manifest)
}

func (r *Registry) loadDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			manifestPath := filepath.Join(dir, entry.Name(), "plugin.json")
			data, err := os.ReadFile(manifestPath)
			if err != nil {
				continue
			}
			var manifest Manifest
			if err := json.Unmarshal(data, &manifest); err != nil {
				continue
			}
			if manifest.Name == "" {
				manifest.Name = entry.Name()
			}
			r.manifests = append(r.manifests, manifest)
		}
	}
	return nil
}

func (r *Registry) verifySignatures(baseDir string) {
	for i := range r.manifests {
		manifest := &r.manifests[i]
		if manifest.Builtin || manifest.Entrypoint == "" || strings.TrimSpace(baseDir) == "" {
			continue
		}
		entrypoint := filepath.Join(baseDir, manifest.Name, manifest.Entrypoint)
		digest, err := fileSHA256(entrypoint)
		if err != nil {
			manifest.Verified = false
			continue
		}
		manifest.Verified = strings.EqualFold(strings.TrimSpace(manifest.Signature), digest)
		if manifest.Verified {
			manifest.Trust = "verified"
		} else if manifest.Trust == "" {
			manifest.Trust = "unverified"
		}
	}
}

func (r *Registry) applyEnabled(enabled []string) {
	if len(enabled) == 0 {
		return
	}
	allowed := map[string]bool{}
	for _, name := range enabled {
		allowed[name] = true
	}
	for i := range r.manifests {
		if r.manifests[i].Builtin {
			continue
		}
		r.manifests[i].Enabled = allowed[r.manifests[i].Name]
	}
}

func (r *Registry) List() []Manifest {
	items := append([]Manifest(nil), r.manifests...)
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items
}

func (r *Registry) EnabledPluginNames() []string {
	var names []string
	for _, manifest := range r.manifests {
		if manifest.Enabled {
			names = append(names, manifest.Name)
		}
	}
	return names
}

func (r *Registry) RegisterToolPlugins(registry *tools.Registry, baseDir string) {
	for _, manifest := range r.manifests {
		if !manifest.Enabled || manifest.Tool == nil || manifest.Entrypoint == "" {
			continue
		}
		if !r.canExecute(manifest) {
			continue
		}
		entrypoint := filepath.Join(baseDir, manifest.Name, manifest.Entrypoint)
		toolName := manifest.Tool.Name
		if toolName == "" {
			toolName = manifest.Name
		}
		description := manifest.Tool.Description
		if description == "" {
			description = manifest.Description
		}
		schema := manifest.Tool.InputSchema
		registry.RegisterTool(toolName, description, schema, func(ctx context.Context, input map[string]any) (string, error) {
			timeout := r.execTimeout
			if manifest.TimeoutSeconds > 0 {
				timeout = time.Duration(manifest.TimeoutSeconds) * time.Second
			}
			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			payload, err := json.Marshal(input)
			if err != nil {
				return "", err
			}
			cmd := exec.CommandContext(ctx, entrypoint)
			pluginDir := filepath.Join(baseDir, manifest.Name)
			cmd.Dir = pluginDir
			cmd.Stdin = nil
			cmd.Env = append(os.Environ(),
				"ANYCLAW_PLUGIN_INPUT="+string(payload),
				"ANYCLAW_PLUGIN_DIR="+pluginDir,
				"ANYCLAW_PLUGIN_TIMEOUT_SECONDS="+fmt.Sprintf("%d", int(timeout/time.Second)),
			)
			output, err := cmd.CombinedOutput()
			if err != nil {
				if ctx.Err() == context.DeadlineExceeded {
					return "", fmt.Errorf("plugin tool timed out after %s", timeout)
				}
				return "", fmt.Errorf("plugin tool failed: %w: %s", err, string(output))
			}
			return string(output), nil
		})
	}
}

func (r *Registry) IngressRunners(baseDir string) []IngressRunner {
	var runners []IngressRunner
	for _, manifest := range r.manifests {
		if !manifest.Enabled || manifest.Ingress == nil || manifest.Entrypoint == "" {
			continue
		}
		if !r.canExecute(manifest) {
			continue
		}
		timeout := r.execTimeout
		if manifest.TimeoutSeconds > 0 {
			timeout = time.Duration(manifest.TimeoutSeconds) * time.Second
		}
		runners = append(runners, IngressRunner{
			Manifest:   manifest,
			Entrypoint: filepath.Join(baseDir, manifest.Name, manifest.Entrypoint),
			Timeout:    timeout,
		})
	}
	return runners
}

func (r *Registry) ChannelRunners(baseDir string) []ChannelRunner {
	var runners []ChannelRunner
	for _, manifest := range r.manifests {
		if !manifest.Enabled || manifest.Channel == nil || manifest.Entrypoint == "" {
			continue
		}
		if !r.canExecute(manifest) {
			continue
		}
		timeout := r.execTimeout
		if manifest.TimeoutSeconds > 0 {
			timeout = time.Duration(manifest.TimeoutSeconds) * time.Second
		}
		runners = append(runners, ChannelRunner{
			Manifest:   manifest,
			Entrypoint: filepath.Join(baseDir, manifest.Name, manifest.Entrypoint),
			Timeout:    timeout,
		})
	}
	return runners
}

func (r *Registry) canExecute(manifest Manifest) bool {
	if manifest.Builtin {
		return true
	}
	if !r.allowExec {
		return false
	}
	if !r.isTrusted(manifest) {
		return false
	}
	policy := manifest.ExecPolicy
	if policy == "" {
		policy = "manual-allow"
	}
	if policy != "manual-allow" && policy != "trusted" {
		return false
	}
	for _, permission := range manifest.Permissions {
		switch permission {
		case "tool:exec", "fs:read", "fs:write", "net:out":
		default:
			return false
		}
	}
	return true
}

func (r *Registry) isTrusted(manifest Manifest) bool {
	if manifest.Builtin {
		return true
	}
	if !r.requireTrust {
		return true
	}
	if manifest.Signer == "" || manifest.Signature == "" {
		return false
	}
	if !r.trustedSigners[manifest.Signer] {
		return false
	}
	return manifest.Verified
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return fmt.Sprintf("sha256:%x", hasher.Sum(nil)), nil
}

func (r *Registry) Summary() (int, error) {
	if r == nil {
		return 0, fmt.Errorf("plugin registry not initialized")
	}
	return len(r.manifests), nil
}
