package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/1024XEngineer/anyclaw/pkg/capability/tools"
	"github.com/1024XEngineer/anyclaw/pkg/config"
	desktopexec "github.com/1024XEngineer/anyclaw/pkg/runtime/execution/desktop"
	"github.com/1024XEngineer/anyclaw/pkg/runtime/execution/verification"
)

type Manifest struct {
	Name           string             `json:"name"`
	Version        string             `json:"version"`
	Description    string             `json:"description"`
	Kinds          []string           `json:"kinds"`
	Builtin        bool               `json:"builtin"`
	Enabled        bool               `json:"enabled"`
	Entrypoint     string             `json:"entrypoint,omitempty"`
	Tool           *ToolSpec          `json:"tool,omitempty"`
	Ingress        *IngressSpec       `json:"ingress,omitempty"`
	Channel        *ChannelSpec       `json:"channel,omitempty"`
	Node           *NodeSpec          `json:"node,omitempty"`
	Surface        *SurfaceSpec       `json:"surface,omitempty"`
	Permissions    []string           `json:"permissions,omitempty"`
	ExecPolicy     string             `json:"exec_policy,omitempty"`
	TimeoutSeconds int                `json:"timeout_seconds,omitempty"`
	Signer         string             `json:"signer,omitempty"`
	Signature      string             `json:"signature,omitempty"`
	Trust          string             `json:"trust,omitempty"`
	Verified       bool               `json:"verified,omitempty"`
	CapabilityTags []string           `json:"capability_tags,omitempty"`
	RiskLevel      string             `json:"risk_level,omitempty"`
	ApprovalScope  string             `json:"approval_scope,omitempty"`
	RequiresHost   bool               `json:"requires_host,omitempty"`
	ModelProvider  *ModelProviderSpec `json:"model_provider,omitempty"`
	Speech         *SpeechSpec        `json:"speech,omitempty"`
	MCP            *MCPSpec           `json:"mcp,omitempty"`
	ContextEngine  *ContextEngineSpec `json:"context_engine,omitempty"`
	sourceDir      string
	manifestPath   string
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

type NodeSpec struct {
	Name         string           `json:"name"`
	Description  string           `json:"description"`
	Platforms    []string         `json:"platforms,omitempty"`
	Capabilities []string         `json:"capabilities,omitempty"`
	Actions      []NodeActionSpec `json:"actions,omitempty"`
}

type NodeActionSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema,omitempty"`
}

type SurfaceSpec struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Path         string   `json:"path,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
}

type Registry struct {
	manifests      []Manifest
	allowExec      bool
	execTimeout    time.Duration
	trustedSigners map[string]bool
	requireTrust   bool
	policy         *tools.PolicyEngine
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

type ProtocolExecutionMeta struct {
	ToolName string
	Plugin   string
	App      string
	Action   string
	Workflow string
	Binding  map[string]any
	Input    map[string]any
}

const maxDesktopPlanExecutions = 60

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

func (r *Registry) SetPolicyEngine(policy *tools.PolicyEngine) {
	if r == nil {
		return
	}
	r.policy = policy
}

func (r *Registry) loadDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifest, ok := loadPluginManifest(filepath.Join(dir, entry.Name()), entry.Name())
		if !ok {
			continue
		}
		r.manifests = append(r.manifests, manifest)
	}
	return nil
}

var pluginManifestCandidates = []string{
	"openclaw.plugin.json",
	"plugin.json",
	".codex-plugin/plugin.json",
	".claude-plugin/plugin.json",
	".cursor-plugin/plugin.json",
}

func loadPluginManifest(pluginDir string, fallbackName string) (Manifest, bool) {
	for _, relPath := range pluginManifestCandidates {
		manifestPath := filepath.Join(pluginDir, filepath.FromSlash(relPath))
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			continue
		}
		var manifest Manifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			continue
		}
		if manifest.Name == "" {
			manifest.Name = fallbackName
		}
		manifest.sourceDir = pluginDir
		manifest.manifestPath = manifestPath
		return manifest, true
	}
	return Manifest{}, false
}

func (r *Registry) verifySignatures(baseDir string) {
	for i := range r.manifests {
		manifest := &r.manifests[i]
		if manifest.Builtin || manifest.Entrypoint == "" || strings.TrimSpace(baseDir) == "" {
			continue
		}
		entrypoint := resolveEntrypoint(baseDir, *manifest)
		digest, err := fileSHA256(entrypoint)
		if err != nil {
			manifest.Verified = false
			continue
		}
		manifest.Verified = signatureMatchesDigest(manifest.Signature, digest)
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
		entrypoint := resolveEntrypoint(baseDir, manifest)
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
			cmd, err := pluginCommandContext(ctx, entrypoint)
			if err != nil {
				return "", err
			}
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
			Entrypoint: resolveEntrypoint(baseDir, manifest),
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
			Entrypoint: resolveEntrypoint(baseDir, manifest),
			Timeout:    timeout,
		})
	}
	return runners
}

type SurfaceRunner struct {
	Manifest   Manifest
	Entrypoint string
	Timeout    time.Duration
}

func (r *Registry) SurfaceRunners(baseDir string) []SurfaceRunner {
	var runners []SurfaceRunner
	for _, manifest := range r.manifests {
		if !manifest.Enabled || manifest.Surface == nil || manifest.Entrypoint == "" {
			continue
		}
		if !r.canExecute(manifest) {
			continue
		}
		timeout := r.execTimeout
		if manifest.TimeoutSeconds > 0 {
			timeout = time.Duration(manifest.TimeoutSeconds) * time.Second
		}
		runners = append(runners, SurfaceRunner{
			Manifest:   manifest,
			Entrypoint: resolveEntrypoint(baseDir, manifest),
			Timeout:    timeout,
		})
	}
	return runners
}

func resolveEntrypoint(baseDir string, manifest Manifest) string {
	entrypoint := strings.TrimSpace(manifest.Entrypoint)
	if entrypoint == "" {
		return ""
	}
	candidates := uniqueNonEmptyPaths(
		filepath.Join(filepath.Dir(manifest.manifestPath), entrypoint),
		filepath.Join(manifest.sourceDir, entrypoint),
		filepath.Join(baseDir, manifest.Name, entrypoint),
	)
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	if len(candidates) > 0 {
		return candidates[0]
	}
	return entrypoint
}

func uniqueNonEmptyPaths(values ...string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		cleaned := filepath.Clean(value)
		if _, ok := seen[cleaned]; ok {
			continue
		}
		seen[cleaned] = struct{}{}
		result = append(result, cleaned)
	}
	return result
}

func uniqueStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := map[string]bool{}
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		key := strings.ToLower(item)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, item)
	}
	return result
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
	if r.policy != nil {
		if err := r.policy.ValidatePluginPermissions(manifest.Name, manifest.Permissions); err != nil {
			return false
		}
	}
	return true
}

func pluginCommandContext(ctx context.Context, entrypoint string) (*exec.Cmd, error) {
	ext := strings.ToLower(strings.TrimSpace(filepath.Ext(entrypoint)))
	switch ext {
	case ".py":
		for _, candidate := range []struct {
			name string
			args []string
		}{
			{name: "py", args: []string{"-3", entrypoint}},
			{name: "python", args: []string{entrypoint}},
			{name: "python3", args: []string{entrypoint}},
		} {
			if path, err := exec.LookPath(candidate.name); err == nil {
				return exec.CommandContext(ctx, path, candidate.args...), nil
			}
		}
		return nil, fmt.Errorf("python interpreter not found for plugin entrypoint: %s", entrypoint)
	case ".ps1":
		if path, err := exec.LookPath("powershell"); err == nil {
			return exec.CommandContext(ctx, path, "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", entrypoint), nil
		}
		return nil, fmt.Errorf("powershell not found for plugin entrypoint: %s", entrypoint)
	default:
		return exec.CommandContext(ctx, entrypoint), nil
	}
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
	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}

func signatureMatchesDigest(signature string, digest string) bool {
	signature = strings.TrimSpace(strings.ToLower(signature))
	digest = strings.TrimSpace(strings.ToLower(digest))
	if signature == "" || digest == "" {
		return false
	}
	return strings.TrimPrefix(signature, "sha256:") == strings.TrimPrefix(digest, "sha256:")
}

func (r *Registry) Summary() (int, error) {
	if r == nil {
		return 0, fmt.Errorf("plugin registry not initialized")
	}
	return len(r.manifests), nil
}

func normalizeIdentifierToken(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	var b strings.Builder
	lastUnderscore := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func ExecuteProtocolOutput(ctx context.Context, registry *tools.Registry, meta ProtocolExecutionMeta, output []byte) (string, bool, error) {
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return "", false, nil
	}
	var plan desktopexec.DesktopPlan
	if err := json.Unmarshal([]byte(trimmed), &plan); err != nil {
		return "", false, nil
	}
	if strings.TrimSpace(plan.Protocol) != desktopexec.DesktopProtocolVersion {
		return "", false, nil
	}
	state := buildDesktopPlanExecutionState(meta, plan, desktopexec.DesktopPlanResumeStateFromContext(ctx))
	state.Status = "pending_approval"
	state.UpdatedAt = desktopexec.TimestampNowRFC3339()
	desktopexec.ReportDesktopPlanState(ctx, state)
	if err := requestProtocolApproval(ctx, meta, plan); err != nil {
		state.Status = desktopPlanStatusFromError(err)
		state.LastError = strings.TrimSpace(err.Error())
		state.UpdatedAt = desktopexec.TimestampNowRFC3339()
		desktopexec.ReportDesktopPlanState(ctx, state)
		return "", true, err
	}
	result, err := executeDesktopPlan(ctx, registry, plan, &state)
	return result, true, err
}

func requestProtocolApproval(ctx context.Context, meta ProtocolExecutionMeta, plan desktopexec.DesktopPlan) error {
	payload := map[string]any{
		"tool_name": meta.ToolName,
		"plugin":    meta.Plugin,
		"app":       meta.App,
		"action":    meta.Action,
		"workflow":  meta.Workflow,
		"binding":   cloneMap(meta.Binding),
		"input":     cloneMap(meta.Input),
		"protocol":  plan.Protocol,
		"summary":   strings.TrimSpace(plan.Summary),
		"result":    strings.TrimSpace(plan.Result),
		"steps":     desktopPlanStepsPayload(plan.Steps),
	}
	return tools.RequestToolApproval(ctx, "desktop_plan", payload)
}

func desktopPlanStepsPayload(steps []desktopexec.DesktopPlanStep) []map[string]any {
	items := make([]map[string]any, 0, len(steps))
	for _, step := range steps {
		toolName, resolvedInput, _ := resolveDesktopPlanStepCall(step)
		item := map[string]any{
			"tool":              toolName,
			"label":             strings.TrimSpace(step.Label),
			"target":            cloneMap(step.Target),
			"action":            strings.TrimSpace(step.Action),
			"input":             resolvedInput,
			"retry":             step.Retry,
			"retry_delay_ms":    step.RetryDelayMS,
			"wait_after_ms":     step.WaitAfterMS,
			"continue_on_error": step.ContinueOnError,
		}
		if step.Value != nil {
			item["value"] = *step.Value
		}
		if step.Append != nil {
			item["append"] = *step.Append
		}
		if step.Submit != nil {
			item["submit"] = *step.Submit
		}
		if step.Verify != nil {
			verifyTool, verifyInput, _ := resolveDesktopPlanCheckCall(*step.Verify)
			item["verify"] = map[string]any{
				"tool":           verifyTool,
				"target":         cloneMap(step.Verify.Target),
				"input":          verifyInput,
				"retry":          step.Verify.Retry,
				"retry_delay_ms": step.Verify.RetryDelayMS,
			}
		}
		if len(step.OnFailure) > 0 {
			item["on_failure"] = desktopPlanStepsPayload(step.OnFailure)
		}
		items = append(items, item)
	}
	return items
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}

func cloneStringMap(items map[string]string) map[string]string {
	if len(items) == 0 {
		return map[string]string{}
	}
	cloned := make(map[string]string, len(items))
	for key, value := range items {
		cloned[key] = value
	}
	return cloned
}

func mergeMaps(base map[string]any, overlay map[string]any) map[string]any {
	if base == nil && overlay == nil {
		return nil
	}
	merged := cloneMap(base)
	if merged == nil {
		merged = map[string]any{}
	}
	for key, value := range overlay {
		merged[key] = value
	}
	return merged
}

type desktopPlanStepResult struct {
	Output    string
	Attempts  int
	Continued bool
	Verified  bool
}

func executeDesktopPlan(ctx context.Context, registry *tools.Registry, plan desktopexec.DesktopPlan, state *desktopexec.DesktopPlanExecutionState) (string, error) {
	if registry == nil {
		return "", errors.New("tool registry not available")
	}
	if len(plan.Steps) > 20 {
		return "", fmt.Errorf("desktop plan exceeds 20 steps")
	}
	startIndex := 0
	if state != nil {
		startIndex = desktopPlanStartIndex(state, len(plan.Steps))
		state.TotalSteps = len(plan.Steps)
		if state.NextStep == 0 {
			state.NextStep = startIndex + 1
		}
		if startIndex > 0 {
			state.Status = "resuming"
			state.Resumed = true
		} else {
			state.Status = "running"
		}
		state.CurrentStep = 0
		state.LastError = ""
		state.UpdatedAt = desktopexec.TimestampNowRFC3339()
		desktopexec.ReportDesktopPlanState(ctx, *state)
	}
	results := make([]string, 0, len(plan.Steps))
	if state != nil && startIndex > 0 {
		for i := 0; i < startIndex && i < len(state.Steps); i++ {
			if output := strings.TrimSpace(state.Steps[i].Output); output != "" {
				results = append(results, output)
			}
		}
	}
	executions := 0
	for idx := startIndex; idx < len(plan.Steps); idx++ {
		step := plan.Steps[idx]
		if state != nil {
			markDesktopPlanStepRunning(state, idx+1, step)
			desktopexec.ReportDesktopPlanState(ctx, *state)
		}
		stepResult, err := executeDesktopPlanStep(ctx, registry, step, idx+1, &executions)
		if err != nil {
			if state != nil {
				markDesktopPlanStepFailed(state, idx+1, stepResult.Attempts, err)
				desktopexec.ReportDesktopPlanState(ctx, *state)
			}
			return "", err
		}
		output := strings.TrimSpace(stepResult.Output)
		if output != "" {
			results = append(results, output)
		}
		if state != nil {
			markDesktopPlanStepCompleted(state, idx+1, stepResult, output)
			desktopexec.ReportDesktopPlanState(ctx, *state)
		}
	}
	summary := firstNonEmptyString(plan.Result, plan.Summary)
	if summary == "" && len(results) == 0 {
		summary = "Desktop plan executed."
	}
	finalResult := summary
	if summary == "" {
		finalResult = strings.Join(results, "\n")
	} else if len(results) > 0 {
		finalResult = strings.TrimSpace(summary + "\n" + strings.Join(results, "\n"))
	}
	if state != nil {
		state.Status = "completed"
		state.Result = finalResult
		state.CurrentStep = 0
		state.NextStep = len(plan.Steps) + 1
		state.LastError = ""
		state.UpdatedAt = desktopexec.TimestampNowRFC3339()
		desktopexec.ReportDesktopPlanState(ctx, *state)
	}
	if summary == "" {
		return strings.Join(results, "\n"), nil
	}
	if len(results) == 0 {
		return summary, nil
	}
	return strings.TrimSpace(summary + "\n" + strings.Join(results, "\n")), nil
}

func executeDesktopPlanStep(ctx context.Context, registry *tools.Registry, step desktopexec.DesktopPlanStep, index int, executions *int) (desktopPlanStepResult, error) {
	toolName, toolInput, err := resolveDesktopPlanStepCall(step)
	if err != nil {
		return desktopPlanStepResult{}, fmt.Errorf("desktop plan step %d is invalid: %w", index, err)
	}
	if !strings.HasPrefix(toolName, "desktop_") {
		return desktopPlanStepResult{}, fmt.Errorf("desktop plan step %d uses unsupported tool: %s", index, toolName)
	}
	attempts := step.Retry + 1
	if attempts <= 0 {
		attempts = 1
	}
	delay := time.Duration(step.RetryDelayMS) * time.Millisecond
	var lastErr error
	result := desktopPlanStepResult{}
	for attempt := 1; attempt <= attempts; attempt++ {
		result.Attempts = attempt
		if err := incrementDesktopExecutionBudget(executions); err != nil {
			return result, err
		}
		currentOutput, err := registry.Call(ctx, toolName, toolInput)
		if err == nil && step.Verify != nil {
			err = runDesktopPlanCheck(ctx, registry, step.Verify, index, executions)
			if err == nil {
				result.Verified = true
			}
		}
		if err == nil {
			result.Output = formatDesktopStepOutput(step, strings.TrimSpace(currentOutput), attempt)
			if step.WaitAfterMS > 0 {
				if err := sleepWithContext(ctx, time.Duration(step.WaitAfterMS)*time.Millisecond); err != nil {
					return result, err
				}
			}
			return result, nil
		}
		lastErr = err
		if attempt < attempts && delay > 0 {
			if err := sleepWithContext(ctx, delay); err != nil {
				return result, err
			}
		}
	}

	recoveryOutput, recoveryErr := executeDesktopRecovery(ctx, registry, step.OnFailure, index, executions)
	if recoveryErr != nil {
		return result, fmt.Errorf("desktop plan step %d failed: %w", index, recoveryErr)
	}
	if step.ContinueOnError {
		result.Output = summarizeDesktopStepFailure(step, index, lastErr, recoveryOutput)
		result.Continued = true
		return result, nil
	}
	if recoveryOutput != "" {
		return result, fmt.Errorf("desktop plan step %d failed: %w (%s)", index, lastErr, recoveryOutput)
	}
	return result, fmt.Errorf("desktop plan step %d failed: %w", index, lastErr)
}

func executeDesktopRecovery(ctx context.Context, registry *tools.Registry, steps []desktopexec.DesktopPlanStep, index int, executions *int) (string, error) {
	if len(steps) == 0 {
		return "", nil
	}
	parts := make([]string, 0, len(steps))
	for recoveryIdx, recovery := range steps {
		result, err := executeDesktopPlanStep(ctx, registry, recovery, index*100+recoveryIdx+1, executions)
		if err != nil {
			return "", fmt.Errorf("recovery step %d failed: %w", recoveryIdx+1, err)
		}
		output := result.Output
		if strings.TrimSpace(output) != "" {
			parts = append(parts, strings.TrimSpace(output))
		}
	}
	return strings.Join(parts, "\n"), nil
}

func runDesktopPlanCheck(ctx context.Context, registry *tools.Registry, check *desktopexec.DesktopPlanCheck, index int, executions *int) error {
	if check == nil {
		return nil
	}
	toolName, toolInput, err := resolveDesktopPlanCheckCall(*check)
	if err != nil {
		return fmt.Errorf("desktop plan step %d verification is invalid: %w", index, err)
	}
	if !strings.HasPrefix(toolName, "desktop_") {
		return fmt.Errorf("desktop plan step %d verification uses unsupported tool: %s", index, toolName)
	}
	attempts := check.Retry + 1
	if attempts <= 0 {
		attempts = 1
	}
	delay := time.Duration(check.RetryDelayMS) * time.Millisecond

	if shouldUseRawDesktopPlanCheck(toolName) {
		var lastErr error
		for attempt := 1; attempt <= attempts; attempt++ {
			if err := incrementDesktopExecutionBudget(executions); err != nil {
				return err
			}
			if _, err := registry.Call(ctx, toolName, toolInput); err == nil {
				return nil
			} else {
				lastErr = err
			}
			if attempt < attempts && delay > 0 {
				if err := sleepWithContext(ctx, delay); err != nil {
					return err
				}
			}
		}
		if lastErr == nil {
			lastErr = fmt.Errorf("verification failed")
		}
		return fmt.Errorf("verification failed: %w", lastErr)
	}

	execFn := func(ctx context.Context, tool string, input map[string]any) (string, error) {
		if err := incrementDesktopExecutionBudget(executions); err != nil {
			return "", err
		}
		result, err := registry.Call(ctx, tool, input)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%v", result), nil
	}
	ie := verification.NewIntegrationExecutor(execFn)
	normalized := *check
	normalized.Tool = toolName
	normalized.Input = toolInput
	normalized.Target = nil
	vResult, err := ie.ExecuteFromDesktopPlan(ctx, &normalized)
	if err != nil {
		return fmt.Errorf("desktop plan step %d verification error: %w", index, err)
	}
	if vResult.AllPassed() {
		return nil
	}
	var lastErr error
	for _, r := range vResult.Results {
		if !r.Passed {
			lastErr = fmt.Errorf("%s: %s", r.Type, r.Message)
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("verification failed")
	}
	return fmt.Errorf("verification failed: %w", lastErr)
}

func resolveDesktopPlanStepCall(step desktopexec.DesktopPlanStep) (string, map[string]any, error) {
	toolName := strings.TrimSpace(step.Tool)
	action := strings.TrimSpace(step.Action)
	input := mergeMaps(step.Target, step.Input)
	if action != "" && !strings.EqualFold(action, "wait") {
		input = mergeMaps(input, map[string]any{"action": action})
	}
	if step.Value != nil {
		input = mergeMaps(input, map[string]any{"value": *step.Value})
	}
	if step.Append != nil {
		input = mergeMaps(input, map[string]any{"append": *step.Append})
	}
	if step.Submit != nil {
		input = mergeMaps(input, map[string]any{"submit": *step.Submit})
	}
	if toolName == "" {
		switch {
		case step.Value != nil || step.Append != nil || step.Submit != nil:
			toolName = "desktop_set_target_value"
		case strings.EqualFold(action, "wait"):
			toolName = "desktop_resolve_target"
			input = mergeMaps(input, map[string]any{"require_found": true})
		case len(step.Target) > 0:
			toolName = "desktop_activate_target"
		default:
			return "", nil, fmt.Errorf("tool or target is required")
		}
	}
	return toolName, input, nil
}

func resolveDesktopPlanCheckCall(check desktopexec.DesktopPlanCheck) (string, map[string]any, error) {
	toolName := strings.TrimSpace(check.Tool)
	input := mergeMaps(check.Target, check.Input)
	if toolName == "" {
		if len(check.Target) == 0 {
			return "", nil, fmt.Errorf("tool or target is required")
		}
		toolName = "desktop_resolve_target"
		input = mergeMaps(input, map[string]any{"require_found": true})
	}
	return toolName, input, nil
}

func incrementDesktopExecutionBudget(executions *int) error {
	if executions == nil {
		return nil
	}
	*executions = *executions + 1
	if *executions > maxDesktopPlanExecutions {
		return fmt.Errorf("desktop plan exceeds %d tool executions", maxDesktopPlanExecutions)
	}
	return nil
}

func formatDesktopStepOutput(step desktopexec.DesktopPlanStep, output string, attempt int) string {
	prefix := strings.TrimSpace(step.Label)
	if prefix == "" {
		if toolName, _, err := resolveDesktopPlanStepCall(step); err == nil {
			prefix = toolName
		}
	}
	if output == "" {
		output = "ok"
	}
	if attempt > 1 {
		output = fmt.Sprintf("%s (attempt %d)", output, attempt)
	}
	if prefix == "" {
		return output
	}
	return prefix + ": " + output
}

func summarizeDesktopStepFailure(step desktopexec.DesktopPlanStep, index int, err error, recovery string) string {
	label := strings.TrimSpace(step.Label)
	if label == "" {
		label = fmt.Sprintf("step %d", index)
	}
	message := label + " failed"
	if err != nil {
		message += ": " + strings.TrimSpace(err.Error())
	}
	if strings.TrimSpace(recovery) != "" {
		message += "\nRecovery: " + strings.TrimSpace(recovery)
	}
	return message
}

func buildDesktopPlanExecutionState(meta ProtocolExecutionMeta, plan desktopexec.DesktopPlan, resume *desktopexec.DesktopPlanExecutionState) desktopexec.DesktopPlanExecutionState {
	now := desktopexec.TimestampNowRFC3339()
	state := desktopexec.DesktopPlanExecutionState{
		ToolName:   strings.TrimSpace(meta.ToolName),
		Plugin:     strings.TrimSpace(meta.Plugin),
		App:        strings.TrimSpace(meta.App),
		Action:     strings.TrimSpace(meta.Action),
		Workflow:   strings.TrimSpace(meta.Workflow),
		Summary:    strings.TrimSpace(plan.Summary),
		TotalSteps: len(plan.Steps),
		NextStep:   1,
		UpdatedAt:  now,
		Steps:      buildDesktopPlanStepStates(plan.Steps, nil),
	}
	if canResumeDesktopPlan(meta, plan, resume) {
		cloned := desktopexec.CloneDesktopPlanExecutionState(resume)
		if cloned != nil {
			cloned.ToolName = strings.TrimSpace(meta.ToolName)
			cloned.Plugin = strings.TrimSpace(meta.Plugin)
			cloned.App = strings.TrimSpace(meta.App)
			cloned.Action = strings.TrimSpace(meta.Action)
			cloned.Workflow = strings.TrimSpace(meta.Workflow)
			cloned.Summary = firstNonEmptyString(strings.TrimSpace(plan.Summary), cloned.Summary)
			cloned.TotalSteps = len(plan.Steps)
			cloned.Steps = buildDesktopPlanStepStates(plan.Steps, cloned.Steps)
			if cloned.NextStep <= 0 {
				cloned.NextStep = cloned.LastCompletedStep + 1
			}
			if cloned.NextStep <= 0 {
				cloned.NextStep = 1
			}
			if cloned.NextStep > len(plan.Steps)+1 {
				cloned.NextStep = len(plan.Steps) + 1
			}
			cloned.Resumed = cloned.NextStep > 1 && cloned.NextStep <= len(plan.Steps)
			cloned.UpdatedAt = now
			return *cloned
		}
	}
	return state
}

func buildDesktopPlanStepStates(steps []desktopexec.DesktopPlanStep, existing []desktopexec.DesktopPlanStepExecutionState) []desktopexec.DesktopPlanStepExecutionState {
	items := make([]desktopexec.DesktopPlanStepExecutionState, 0, len(steps))
	for idx, step := range steps {
		toolName, _, _ := resolveDesktopPlanStepCall(step)
		item := desktopexec.DesktopPlanStepExecutionState{
			Index:     idx + 1,
			Tool:      toolName,
			Label:     strings.TrimSpace(step.Label),
			HasVerify: step.Verify != nil,
		}
		for _, current := range existing {
			if current.Index != idx+1 {
				continue
			}
			item.HasVerify = step.Verify != nil
			item.Verified = current.Verified
			item.Status = current.Status
			item.Attempts = current.Attempts
			item.Output = current.Output
			item.Error = current.Error
			item.UpdatedAt = current.UpdatedAt
			break
		}
		items = append(items, item)
	}
	return items
}

func canResumeDesktopPlan(meta ProtocolExecutionMeta, plan desktopexec.DesktopPlan, resume *desktopexec.DesktopPlanExecutionState) bool {
	if resume == nil || len(plan.Steps) == 0 {
		return false
	}
	if strings.TrimSpace(resume.ToolName) != strings.TrimSpace(meta.ToolName) {
		return false
	}
	if strings.TrimSpace(meta.Plugin) != "" && strings.TrimSpace(resume.Plugin) != strings.TrimSpace(meta.Plugin) {
		return false
	}
	if strings.TrimSpace(meta.Action) != "" && strings.TrimSpace(resume.Action) != strings.TrimSpace(meta.Action) {
		return false
	}
	if strings.TrimSpace(meta.Workflow) != "" && strings.TrimSpace(resume.Workflow) != strings.TrimSpace(meta.Workflow) {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(resume.Status), "completed") {
		return false
	}
	nextStep := resume.NextStep
	if nextStep <= 0 {
		nextStep = resume.LastCompletedStep + 1
	}
	return nextStep > 1 && nextStep <= len(plan.Steps)
}

func desktopPlanStartIndex(state *desktopexec.DesktopPlanExecutionState, totalSteps int) int {
	if state == nil || totalSteps <= 0 {
		return 0
	}
	nextStep := state.NextStep
	if nextStep <= 0 {
		nextStep = state.LastCompletedStep + 1
	}
	if nextStep <= 1 {
		return 0
	}
	if nextStep > totalSteps {
		return totalSteps
	}
	return nextStep - 1
}

func markDesktopPlanStepRunning(state *desktopexec.DesktopPlanExecutionState, index int, step desktopexec.DesktopPlanStep) {
	if state == nil {
		return
	}
	now := desktopexec.TimestampNowRFC3339()
	state.Status = "running"
	if state.Resumed && index > 1 {
		state.Status = "resuming"
	}
	state.CurrentStep = index
	state.NextStep = index
	state.UpdatedAt = now
	stepState := desktopPlanStepStateAt(state, index)
	stepState.Tool = strings.TrimSpace(step.Tool)
	stepState.Label = strings.TrimSpace(step.Label)
	stepState.Status = "running"
	stepState.Error = ""
	stepState.UpdatedAt = now
}

func markDesktopPlanStepCompleted(state *desktopexec.DesktopPlanExecutionState, index int, result desktopPlanStepResult, output string) {
	if state == nil {
		return
	}
	now := desktopexec.TimestampNowRFC3339()
	stepState := desktopPlanStepStateAt(state, index)
	stepState.Attempts = result.Attempts
	stepState.Verified = result.Verified
	stepState.Output = output
	stepState.Error = ""
	stepState.UpdatedAt = now
	if result.Continued {
		stepState.Status = "continued"
	} else {
		stepState.Status = "completed"
	}
	state.LastCompletedStep = index
	state.CurrentStep = 0
	state.NextStep = index + 1
	state.LastOutput = output
	state.LastError = ""
	state.UpdatedAt = now
	state.Status = "running"
}

func markDesktopPlanStepFailed(state *desktopexec.DesktopPlanExecutionState, index int, attempts int, err error) {
	if state == nil {
		return
	}
	now := desktopexec.TimestampNowRFC3339()
	stepState := desktopPlanStepStateAt(state, index)
	stepState.Attempts = attempts
	stepState.Error = strings.TrimSpace(err.Error())
	stepState.UpdatedAt = now
	stepState.Status = desktopPlanStatusFromError(err)
	state.Status = desktopPlanStatusFromError(err)
	state.CurrentStep = index
	state.NextStep = index
	state.LastError = strings.TrimSpace(err.Error())
	state.UpdatedAt = now
}

func desktopPlanStepStateAt(state *desktopexec.DesktopPlanExecutionState, index int) *desktopexec.DesktopPlanStepExecutionState {
	if state == nil || index <= 0 {
		return nil
	}
	for i := range state.Steps {
		if state.Steps[i].Index == index {
			return &state.Steps[i]
		}
	}
	state.Steps = append(state.Steps, desktopexec.DesktopPlanStepExecutionState{Index: index})
	return &state.Steps[len(state.Steps)-1]
}

func shouldUseRawDesktopPlanCheck(toolName string) bool {
	switch strings.TrimSpace(strings.ToLower(toolName)) {
	case "desktop_verify_text", "desktop_wait_text", "desktop_resolve_target", "desktop_find_text":
		return true
	default:
		return false
	}
}

func desktopPlanStatusFromError(err error) string {
	if err == nil {
		return "completed"
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "interrupted"
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(message, "waiting approval"), strings.Contains(message, "awaiting approval"):
		return "waiting_approval"
	default:
		return "failed"
	}
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
