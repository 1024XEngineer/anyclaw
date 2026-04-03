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
	goruntime "runtime"
	"sort"
	"strings"
	"time"
	"unicode"

	appstore "github.com/anyclaw/anyclaw/pkg/apps"
	"github.com/anyclaw/anyclaw/pkg/config"
	"github.com/anyclaw/anyclaw/pkg/tools"
	"github.com/anyclaw/anyclaw/pkg/verification"
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
	App            *AppSpec           `json:"app,omitempty"`
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

type AppSpec struct {
	Name         string                `json:"name"`
	Description  string                `json:"description"`
	Transport    string                `json:"transport,omitempty"`
	Platforms    []string              `json:"platforms,omitempty"`
	Capabilities []string              `json:"capabilities,omitempty"`
	Desktop      *appstore.DesktopSpec `json:"desktop,omitempty"`
	Actions      []AppActionSpec       `json:"actions,omitempty"`
	Workflows    []AppWorkflowSpec     `json:"workflows,omitempty"`
}

type AppActionSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Kind        string         `json:"kind,omitempty"`
	InputSchema map[string]any `json:"input_schema,omitempty"`
}

type AppWorkflowSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Action      string         `json:"action"`
	Tags        []string       `json:"tags,omitempty"`
	InputSchema map[string]any `json:"input_schema,omitempty"`
	Defaults    map[string]any `json:"defaults,omitempty"`
}

type AppWorkflowInfo struct {
	Plugin      string         `json:"plugin"`
	App         string         `json:"app"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Action      string         `json:"action"`
	ToolName    string         `json:"tool_name"`
	Tags        []string       `json:"tags,omitempty"`
	InputSchema map[string]any `json:"input_schema,omitempty"`
	Defaults    map[string]any `json:"defaults,omitempty"`
}

type AppWorkflowPairingInfo struct {
	ID          string         `json:"id,omitempty"`
	Name        string         `json:"name,omitempty"`
	Description string         `json:"description,omitempty"`
	Binding     string         `json:"binding,omitempty"`
	Triggers    []string       `json:"triggers,omitempty"`
	Defaults    map[string]any `json:"defaults,omitempty"`
}

type AppWorkflowMatch struct {
	Workflow AppWorkflowInfo         `json:"workflow"`
	Score    int                     `json:"score"`
	Reason   string                  `json:"reason,omitempty"`
	Pairing  *AppWorkflowPairingInfo `json:"pairing,omitempty"`
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

type AppRunner struct {
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

func (r *Registry) ListAppWorkflows() []AppWorkflowInfo {
	if r == nil {
		return nil
	}
	items := make([]AppWorkflowInfo, 0)
	for _, manifest := range r.manifests {
		if !manifest.Enabled || manifest.App == nil {
			continue
		}
		actionNames := map[string]bool{}
		for _, action := range manifest.App.Actions {
			if name := normalizeIdentifierToken(action.Name); name != "" {
				actionNames[name] = true
			}
		}
		appName := firstNonEmptyString(manifest.App.Name, manifest.Name)
		for _, workflow := range manifest.App.Workflows {
			if strings.TrimSpace(workflow.Name) == "" || strings.TrimSpace(workflow.Action) == "" {
				continue
			}
			if !actionNames[normalizeIdentifierToken(workflow.Action)] {
				continue
			}
			items = append(items, AppWorkflowInfo{
				Plugin:      manifest.Name,
				App:         appName,
				Name:        strings.TrimSpace(workflow.Name),
				Description: strings.TrimSpace(workflow.Description),
				Action:      strings.TrimSpace(workflow.Action),
				ToolName:    AppWorkflowToolName(manifest.Name, workflow.Name),
				Tags:        append([]string{}, workflow.Tags...),
				InputSchema: cloneMap(workflow.InputSchema),
				Defaults:    cloneMap(workflow.Defaults),
			})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Plugin == items[j].Plugin {
			return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
		}
		return strings.ToLower(items[i].Plugin) < strings.ToLower(items[j].Plugin)
	})
	return items
}

func (r *Registry) ResolveWorkflowMatches(query string, limit int) []AppWorkflowMatch {
	query = strings.TrimSpace(query)
	if r == nil || query == "" {
		return nil
	}
	workflows := r.ListAppWorkflows()
	if len(workflows) == 0 {
		return nil
	}
	if limit <= 0 {
		limit = 3
	}
	queryNorm := normalizeSearchText(query)
	queryTokens := searchTokens(queryNorm)
	matches := make([]AppWorkflowMatch, 0, len(workflows))
	for _, workflow := range workflows {
		score, reasons := scoreWorkflowMatch(workflow, queryNorm, queryTokens)
		if score < 4 {
			continue
		}
		matches = append(matches, AppWorkflowMatch{
			Workflow: workflow,
			Score:    score,
			Reason:   strings.Join(reasons, "; "),
		})
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Score == matches[j].Score {
			return matches[i].Workflow.ToolName < matches[j].Workflow.ToolName
		}
		return matches[i].Score > matches[j].Score
	})
	if len(matches) > limit {
		matches = matches[:limit]
	}
	return matches
}

func (r *Registry) ResolveWorkflowMatchesWithPairings(query string, limit int, pairings []*appstore.Pairing) []AppWorkflowMatch {
	query = strings.TrimSpace(query)
	if r == nil || query == "" {
		return nil
	}
	if limit <= 0 {
		limit = 3
	}
	baseLimit := limit * 4
	if baseLimit < 6 {
		baseLimit = 6
	}

	results := make([]AppWorkflowMatch, 0, baseLimit)
	indexByKey := map[string]int{}
	appendMatch := func(match AppWorkflowMatch) {
		key := workflowMatchKey(match)
		if idx, ok := indexByKey[key]; ok {
			if shouldReplaceWorkflowMatch(results[idx], match) {
				results[idx] = match
			}
			return
		}
		indexByKey[key] = len(results)
		results = append(results, match)
	}

	for _, match := range r.ResolveWorkflowMatches(query, baseLimit) {
		appendMatch(match)
	}

	workflows := r.ListAppWorkflows()
	if len(workflows) > 0 && len(pairings) > 0 {
		queryNorm := normalizeSearchText(query)
		queryTokens := searchTokens(queryNorm)
		for _, pairing := range pairings {
			workflow, ok := resolvePairingWorkflow(workflows, pairing)
			if !ok {
				continue
			}
			score, reasons := scorePairingWorkflowMatch(workflow, pairing, queryNorm, queryTokens)
			if score < 8 {
				continue
			}
			appendMatch(AppWorkflowMatch{
				Workflow: workflow,
				Score:    score,
				Reason:   strings.Join(uniqueStrings(reasons), "; "),
				Pairing:  pairingInfo(pairing),
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			if (results[i].Pairing != nil) != (results[j].Pairing != nil) {
				return results[i].Pairing != nil
			}
			return workflowMatchKey(results[i]) < workflowMatchKey(results[j])
		}
		return results[i].Score > results[j].Score
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results
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

func (r *Registry) AppRunners(baseDir string) []AppRunner {
	var runners []AppRunner
	for _, manifest := range r.manifests {
		if !manifest.Enabled || manifest.App == nil || manifest.Entrypoint == "" {
			continue
		}
		if !r.canExecute(manifest) {
			continue
		}
		timeout := r.execTimeout
		if manifest.TimeoutSeconds > 0 {
			timeout = time.Duration(manifest.TimeoutSeconds) * time.Second
		}
		runners = append(runners, AppRunner{
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

func (r *Registry) RegisterAppPlugins(registry *tools.Registry, baseDir string, configPath string) {
	for _, runner := range r.AppRunners(baseDir) {
		appSpec := runner.Manifest.App
		if appSpec == nil {
			continue
		}
		appName := firstNonEmptyString(appSpec.Name, runner.Manifest.Name)
		actionMap := map[string]AppActionSpec{}
		for _, action := range appSpec.Actions {
			action := action
			actionName := strings.TrimSpace(action.Name)
			if actionName == "" {
				continue
			}
			actionMap[normalizeIdentifierToken(actionName)] = action
			registerAppTool(registry, runner, configPath, appName, action, nil)
		}
		for _, workflow := range appSpec.Workflows {
			workflow := workflow
			workflowName := strings.TrimSpace(workflow.Name)
			if workflowName == "" {
				continue
			}
			action, ok := actionMap[normalizeIdentifierToken(workflow.Action)]
			if !ok {
				continue
			}
			registerAppTool(registry, runner, configPath, appName, action, &workflow)
		}
	}
}

func AppActionToolName(pluginName string, actionName string) string {
	pluginName = normalizeIdentifierToken(pluginName)
	actionName = normalizeIdentifierToken(actionName)
	if pluginName == "" {
		pluginName = "plugin"
	}
	if actionName == "" {
		actionName = "action"
	}
	return "app_" + pluginName + "_" + actionName
}

func AppWorkflowToolName(pluginName string, workflowName string) string {
	pluginName = normalizeIdentifierToken(pluginName)
	workflowName = normalizeIdentifierToken(workflowName)
	if pluginName == "" {
		pluginName = "plugin"
	}
	if workflowName == "" {
		workflowName = "workflow"
	}
	return "app_" + pluginName + "_workflow_" + workflowName
}

func registerAppTool(registry *tools.Registry, runner AppRunner, configPath string, appName string, action AppActionSpec, workflow *AppWorkflowSpec) {
	if registry == nil {
		return
	}
	actionName := strings.TrimSpace(action.Name)
	if actionName == "" {
		return
	}
	toolName := AppActionToolName(runner.Manifest.Name, actionName)
	description := firstNonEmptyString(action.Description, fmt.Sprintf("Run %s on app connector %s", actionName, appName))
	schema := cloneMap(action.InputSchema)
	if workflow != nil {
		toolName = AppWorkflowToolName(runner.Manifest.Name, workflow.Name)
		description = firstNonEmptyString(workflow.Description, description)
		if workflow.InputSchema != nil {
			schema = cloneMap(workflow.InputSchema)
		}
	}
	if schema == nil {
		schema = map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}
	}
	entrypoint := runner.Entrypoint
	timeout := runner.Timeout
	manifestName := runner.Manifest.Name
	registry.RegisterTool(toolName, description, schema, func(ctx context.Context, input map[string]any) (string, error) {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		mergedInput := map[string]any{}
		if workflow != nil {
			mergedInput = mergeMaps(mergedInput, cloneMap(workflow.Defaults))
		}
		var store *appstore.Store
		var err error
		if configPath != "" {
			store, err = appstore.NewStore(configPath)
			if err != nil {
				return "", err
			}
		}
		pairingRef := firstNonEmptyString(stringInput(input, "pairing"), stringInput(input, "pairing_id"), stringInput(input, "pairing_name"))
		var pairing *appstore.Pairing
		if store != nil && workflow != nil && pairingRef != "" {
			pairing, err = store.ResolvePairing(manifestName, pairingRef)
			if err != nil {
				return "", err
			}
			if pairing != nil && !pairingTargetsWorkflow(pairing, manifestName, *workflow) {
				return "", fmt.Errorf("app pairing %s does not target workflow %s", pairingRef, workflow.Name)
			}
			if pairing != nil {
				mergedInput = mergeMaps(mergedInput, cloneMap(pairing.Defaults))
			}
		}
		mergedInput = mergeMaps(mergedInput, cloneMap(input))
		bindingRef := firstNonEmptyString(
			stringInput(mergedInput, "binding"),
			stringInput(mergedInput, "binding_id"),
			stringInput(mergedInput, "binding_name"),
		)
		if bindingRef == "" && pairing != nil {
			bindingRef = strings.TrimSpace(pairing.Binding)
		}
		var binding *appstore.Binding
		if store != nil {
			binding, err = store.Resolve(manifestName, bindingRef)
			if err != nil {
				return "", err
			}
		}
		payload, err := json.Marshal(map[string]any{
			"plugin":   manifestName,
			"app":      appName,
			"action":   actionName,
			"workflow": workflowPayload(workflow),
			"pairing":  pairingPayload(pairing),
			"input":    mergedInput,
			"binding":  bindingPayload(binding),
			"protocol": buildProtocolContext(runner.Manifest, action, registry),
		})
		if err != nil {
			return "", err
		}
		cmd, err := pluginCommandContext(ctx, entrypoint)
		if err != nil {
			return "", err
		}
		pluginDir := filepath.Dir(entrypoint)
		cmd.Dir = pluginDir
		cmd.Stdin = nil
		cmd.Env = append(os.Environ(),
			"ANYCLAW_PLUGIN_MODE=app-action",
			"ANYCLAW_PLUGIN_INPUT="+string(payload),
			"ANYCLAW_PLUGIN_DIR="+pluginDir,
			"ANYCLAW_PLUGIN_TIMEOUT_SECONDS="+fmt.Sprintf("%d", int(timeout/time.Second)),
		)
		cmd.Env = append(cmd.Env, appstore.ResolveBindingEnvs(binding)...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return "", fmt.Errorf("app plugin timed out after %s", timeout)
			}
			return "", fmt.Errorf("app plugin failed: %w: %s", err, string(output))
		}
		if handled, ok, err := ExecuteProtocolOutput(ctx, registry, ProtocolExecutionMeta{
			ToolName: toolName,
			Plugin:   manifestName,
			App:      appName,
			Action:   actionName,
			Workflow: workflowName(workflow),
			Binding:  bindingPayload(binding),
			Input:    cloneMap(mergedInput),
		}, output); ok {
			return handled, err
		}
		return strings.TrimSpace(string(output)), nil
	})
}

func workflowMatchKey(match AppWorkflowMatch) string {
	if match.Pairing != nil && strings.TrimSpace(match.Pairing.ID) != "" {
		return match.Workflow.ToolName + "#" + strings.TrimSpace(match.Pairing.ID)
	}
	return match.Workflow.ToolName
}

func shouldReplaceWorkflowMatch(current AppWorkflowMatch, next AppWorkflowMatch) bool {
	if next.Score != current.Score {
		return next.Score > current.Score
	}
	if current.Pairing == nil && next.Pairing != nil {
		return true
	}
	return false
}

func resolvePairingWorkflow(workflows []AppWorkflowInfo, pairing *appstore.Pairing) (AppWorkflowInfo, bool) {
	if pairing == nil {
		return AppWorkflowInfo{}, false
	}
	appNorm := normalizeIdentifierToken(pairing.App)
	workflowRef := normalizeIdentifierToken(pairing.Workflow)
	var single AppWorkflowInfo
	count := 0
	for _, workflow := range workflows {
		if appNorm != "" && !pairingMatchesWorkflowApp(workflow, appNorm) {
			continue
		}
		count++
		if workflowRef != "" && pairingMatchesWorkflowRef(workflow, workflowRef) {
			return workflow, true
		}
		single = workflow
	}
	if workflowRef == "" && count == 1 {
		return single, true
	}
	return AppWorkflowInfo{}, false
}

func pairingMatchesWorkflowApp(workflow AppWorkflowInfo, appNorm string) bool {
	return normalizeIdentifierToken(workflow.Plugin) == appNorm || normalizeIdentifierToken(workflow.App) == appNorm
}

func pairingMatchesWorkflowRef(workflow AppWorkflowInfo, ref string) bool {
	return normalizeIdentifierToken(workflow.Name) == ref ||
		normalizeIdentifierToken(workflow.ToolName) == ref ||
		normalizeIdentifierToken(workflow.Action) == ref
}

func scorePairingWorkflowMatch(workflow AppWorkflowInfo, pairing *appstore.Pairing, queryNorm string, queryTokens []string) (int, []string) {
	workflowScore, workflowReasons := scoreWorkflowMatch(workflow, queryNorm, queryTokens)
	pairingScore, pairingReasons := scorePairingMatch(pairing, queryNorm, queryTokens)
	score := workflowScore + pairingScore
	if pairingScore > 0 {
		score += 10
	}
	return score, append(pairingReasons, workflowReasons...)
}

func scorePairingMatch(pairing *appstore.Pairing, queryNorm string, queryTokens []string) (int, []string) {
	if pairing == nil {
		return 0, nil
	}
	score := 0
	reasons := make([]string, 0, 4)
	nameNorm := normalizeSearchText(pairing.Name)
	descNorm := normalizeSearchText(pairing.Description)
	appNorm := normalizeSearchText(pairing.App + " " + pairing.Workflow + " " + pairing.Binding)
	triggerNorms := make([]string, 0, len(pairing.Triggers))
	for _, trigger := range pairing.Triggers {
		triggerNorm := normalizeSearchText(trigger)
		if triggerNorm != "" {
			triggerNorms = append(triggerNorms, triggerNorm)
		}
	}

	if nameNorm != "" && (strings.Contains(queryNorm, nameNorm) || strings.Contains(nameNorm, queryNorm)) {
		score += 18
		reasons = append(reasons, "pairing name matched")
	}
	if descNorm != "" && strings.Contains(descNorm, queryNorm) {
		score += 10
		reasons = append(reasons, "pairing description matched")
	}
	for _, triggerNorm := range triggerNorms {
		if strings.Contains(queryNorm, triggerNorm) || strings.Contains(triggerNorm, queryNorm) {
			score += 16
			reasons = append(reasons, "pairing trigger matched")
		}
	}

	nameTokens := tokenSet(nameNorm)
	descTokens := tokenSet(descNorm)
	appTokens := tokenSet(appNorm)
	triggerTokens := tokenSet(strings.Join(triggerNorms, " "))
	for _, token := range queryTokens {
		switch {
		case triggerTokens[token]:
			score += 6
		case nameTokens[token]:
			score += 5
		case descTokens[token]:
			score += 3
		case appTokens[token]:
			score += 2
		}
	}
	if score > 0 && len(reasons) == 0 {
		reasons = append(reasons, "pairing keyword overlap")
	}
	return score, reasons
}

func pairingInfo(pairing *appstore.Pairing) *AppWorkflowPairingInfo {
	if pairing == nil {
		return nil
	}
	return &AppWorkflowPairingInfo{
		ID:          strings.TrimSpace(pairing.ID),
		Name:        strings.TrimSpace(pairing.Name),
		Description: strings.TrimSpace(pairing.Description),
		Binding:     strings.TrimSpace(pairing.Binding),
		Triggers:    append([]string{}, pairing.Triggers...),
		Defaults:    cloneMap(pairing.Defaults),
	}
}

func pairingPayload(pairing *appstore.Pairing) map[string]any {
	if pairing == nil {
		return nil
	}
	return map[string]any{
		"id":          strings.TrimSpace(pairing.ID),
		"name":        strings.TrimSpace(pairing.Name),
		"description": strings.TrimSpace(pairing.Description),
		"app":         strings.TrimSpace(pairing.App),
		"workflow":    strings.TrimSpace(pairing.Workflow),
		"binding":     strings.TrimSpace(pairing.Binding),
		"triggers":    append([]string{}, pairing.Triggers...),
		"defaults":    cloneMap(pairing.Defaults),
		"metadata":    cloneStringMap(pairing.Metadata),
	}
}

func pairingTargetsWorkflow(pairing *appstore.Pairing, pluginName string, workflow AppWorkflowSpec) bool {
	if pairing == nil {
		return false
	}
	if normalizeIdentifierToken(pairing.App) != normalizeIdentifierToken(pluginName) {
		return false
	}
	ref := normalizeIdentifierToken(pairing.Workflow)
	if ref == "" {
		return false
	}
	return ref == normalizeIdentifierToken(workflow.Name) ||
		ref == normalizeIdentifierToken(AppWorkflowToolName(pluginName, workflow.Name)) ||
		ref == normalizeIdentifierToken(workflow.Action)
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

func stringInput(input map[string]any, key string) string {
	if input == nil {
		return ""
	}
	value, _ := input[key].(string)
	return strings.TrimSpace(value)
}

func bindingPayload(binding *appstore.Binding) map[string]any {
	if binding == nil {
		return nil
	}
	return map[string]any{
		"id":          binding.ID,
		"app":         binding.App,
		"name":        binding.Name,
		"description": binding.Description,
		"org":         binding.Org,
		"project":     binding.Project,
		"workspace":   binding.Workspace,
		"target":      binding.Target,
		"metadata":    binding.Metadata,
	}
}

func workflowPayload(workflow *AppWorkflowSpec) map[string]any {
	if workflow == nil {
		return nil
	}
	return map[string]any{
		"name":         strings.TrimSpace(workflow.Name),
		"description":  strings.TrimSpace(workflow.Description),
		"action":       strings.TrimSpace(workflow.Action),
		"tags":         append([]string{}, workflow.Tags...),
		"input_schema": cloneMap(workflow.InputSchema),
		"defaults":     cloneMap(workflow.Defaults),
	}
}

func workflowName(workflow *AppWorkflowSpec) string {
	if workflow == nil {
		return ""
	}
	return strings.TrimSpace(workflow.Name)
}

func buildProtocolContext(manifest Manifest, action AppActionSpec, registry *tools.Registry) appstore.ProtocolContext {
	transport := ""
	platforms := []string{}
	capabilities := []string{}
	if manifest.App != nil {
		transport = firstNonEmptyString(manifest.App.Transport, defaultTransport(manifest.App))
		platforms = append([]string{}, manifest.App.Platforms...)
		capabilities = append([]string{}, manifest.App.Capabilities...)
	}
	ctx := appstore.ProtocolContext{
		Version:      appstore.DesktopProtocolVersion,
		Transport:    transport,
		Platforms:    platforms,
		Capabilities: capabilities,
		Host: appstore.HostContext{
			OS:             goruntime.GOOS,
			AvailableTools: listToolsByPrefix(registry, "desktop_"),
		},
		Action: appstore.ActionContext{
			Name: action.Name,
			Kind: action.Kind,
		},
	}
	if manifest.App != nil && manifest.App.Desktop != nil {
		ctx.Desktop = &appstore.DesktopContext{
			LaunchCommand:        manifest.App.Desktop.LaunchCommand,
			WindowTitle:          manifest.App.Desktop.WindowTitle,
			WindowClass:          manifest.App.Desktop.WindowClass,
			FocusStrategy:        manifest.App.Desktop.FocusStrategy,
			DetectionHints:       append([]string{}, manifest.App.Desktop.DetectionHints...),
			RequiresHostReviewed: manifest.App.Desktop.RequiresHostReviewed,
		}
	}
	return ctx
}

func defaultTransport(spec *AppSpec) string {
	if spec == nil {
		return ""
	}
	if spec.Desktop != nil {
		return "desktop"
	}
	return ""
}

func listToolsByPrefix(registry *tools.Registry, prefix string) []string {
	if registry == nil {
		return nil
	}
	items := registry.List()
	matches := make([]string, 0, len(items))
	for _, item := range items {
		if strings.HasPrefix(item.Name, prefix) {
			matches = append(matches, item.Name)
		}
	}
	sort.Strings(matches)
	return matches
}

func ExecuteProtocolOutput(ctx context.Context, registry *tools.Registry, meta ProtocolExecutionMeta, output []byte) (string, bool, error) {
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return "", false, nil
	}
	var plan appstore.DesktopPlan
	if err := json.Unmarshal([]byte(trimmed), &plan); err != nil {
		return "", false, nil
	}
	if strings.TrimSpace(plan.Protocol) != appstore.DesktopProtocolVersion {
		return "", false, nil
	}
	state := buildDesktopPlanExecutionState(meta, plan, appstore.DesktopPlanResumeStateFromContext(ctx))
	state.Status = "pending_approval"
	state.UpdatedAt = appstore.TimestampNowRFC3339()
	appstore.ReportDesktopPlanState(ctx, state)
	if err := requestProtocolApproval(ctx, meta, plan); err != nil {
		state.Status = desktopPlanStatusFromError(err)
		state.LastError = strings.TrimSpace(err.Error())
		state.UpdatedAt = appstore.TimestampNowRFC3339()
		appstore.ReportDesktopPlanState(ctx, state)
		return "", true, err
	}
	result, err := executeDesktopPlan(ctx, registry, plan, &state)
	return result, true, err
}

func requestProtocolApproval(ctx context.Context, meta ProtocolExecutionMeta, plan appstore.DesktopPlan) error {
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

func desktopPlanStepsPayload(steps []appstore.DesktopPlanStep) []map[string]any {
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

func scoreWorkflowMatch(workflow AppWorkflowInfo, queryNorm string, queryTokens []string) (int, []string) {
	score := 0
	reasons := make([]string, 0, 4)
	nameNorm := normalizeSearchText(workflow.Name)
	descNorm := normalizeSearchText(workflow.Description)
	actionNorm := normalizeSearchText(workflow.Action)
	appNorm := normalizeSearchText(workflow.App + " " + workflow.Plugin)
	tagNorms := make([]string, 0, len(workflow.Tags))
	for _, tag := range workflow.Tags {
		tagNorm := normalizeSearchText(tag)
		if tagNorm != "" {
			tagNorms = append(tagNorms, tagNorm)
		}
	}

	if nameNorm != "" && (strings.Contains(queryNorm, nameNorm) || strings.Contains(nameNorm, queryNorm)) {
		score += 18
		reasons = append(reasons, "workflow name matched")
	}
	if descNorm != "" && strings.Contains(descNorm, queryNorm) {
		score += 10
		reasons = append(reasons, "description matched")
	}
	for _, tagNorm := range tagNorms {
		if strings.Contains(queryNorm, tagNorm) || strings.Contains(tagNorm, queryNorm) {
			score += 12
			reasons = append(reasons, "tag matched")
		}
	}

	nameTokens := tokenSet(nameNorm)
	descTokens := tokenSet(descNorm)
	actionTokens := tokenSet(actionNorm)
	appTokens := tokenSet(appNorm)
	tagTokens := tokenSet(strings.Join(tagNorms, " "))
	for _, token := range queryTokens {
		switch {
		case nameTokens[token]:
			score += 5
		case tagTokens[token]:
			score += 4
		case descTokens[token]:
			score += 3
		case appTokens[token]:
			score += 2
		case actionTokens[token]:
			score += 1
		}
	}
	if score > 0 && len(reasons) == 0 {
		reasons = append(reasons, "keyword overlap")
	}
	return score, reasons
}

func normalizeSearchText(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	var b strings.Builder
	lastSpace := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastSpace = false
			continue
		}
		if !lastSpace {
			b.WriteByte(' ')
			lastSpace = true
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func searchTokens(value string) []string {
	value = normalizeSearchText(value)
	if value == "" {
		return nil
	}
	fields := strings.Fields(value)
	tokens := make([]string, 0, len(fields))
	seen := map[string]bool{}
	for _, field := range fields {
		if len([]rune(field)) <= 1 {
			continue
		}
		if !seen[field] {
			seen[field] = true
			tokens = append(tokens, field)
		}
	}
	return tokens
}

func tokenSet(value string) map[string]bool {
	items := searchTokens(value)
	set := make(map[string]bool, len(items))
	for _, item := range items {
		set[item] = true
	}
	return set
}

type desktopPlanStepResult struct {
	Output    string
	Attempts  int
	Continued bool
	Verified  bool
}

func executeDesktopPlan(ctx context.Context, registry *tools.Registry, plan appstore.DesktopPlan, state *appstore.DesktopPlanExecutionState) (string, error) {
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
		state.UpdatedAt = appstore.TimestampNowRFC3339()
		appstore.ReportDesktopPlanState(ctx, *state)
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
			appstore.ReportDesktopPlanState(ctx, *state)
		}
		stepResult, err := executeDesktopPlanStep(ctx, registry, step, idx+1, &executions)
		if err != nil {
			if state != nil {
				markDesktopPlanStepFailed(state, idx+1, stepResult.Attempts, err)
				appstore.ReportDesktopPlanState(ctx, *state)
			}
			return "", err
		}
		output := strings.TrimSpace(stepResult.Output)
		if output != "" {
			results = append(results, output)
		}
		if state != nil {
			markDesktopPlanStepCompleted(state, idx+1, stepResult, output)
			appstore.ReportDesktopPlanState(ctx, *state)
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
		state.UpdatedAt = appstore.TimestampNowRFC3339()
		appstore.ReportDesktopPlanState(ctx, *state)
	}
	if summary == "" {
		return strings.Join(results, "\n"), nil
	}
	if len(results) == 0 {
		return summary, nil
	}
	return strings.TrimSpace(summary + "\n" + strings.Join(results, "\n")), nil
}

func executeDesktopPlanStep(ctx context.Context, registry *tools.Registry, step appstore.DesktopPlanStep, index int, executions *int) (desktopPlanStepResult, error) {
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

func executeDesktopRecovery(ctx context.Context, registry *tools.Registry, steps []appstore.DesktopPlanStep, index int, executions *int) (string, error) {
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

func runDesktopPlanCheck(ctx context.Context, registry *tools.Registry, check *appstore.DesktopPlanCheck, index int, executions *int) error {
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

func resolveDesktopPlanStepCall(step appstore.DesktopPlanStep) (string, map[string]any, error) {
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

func resolveDesktopPlanCheckCall(check appstore.DesktopPlanCheck) (string, map[string]any, error) {
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

func formatDesktopStepOutput(step appstore.DesktopPlanStep, output string, attempt int) string {
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

func summarizeDesktopStepFailure(step appstore.DesktopPlanStep, index int, err error, recovery string) string {
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

func buildDesktopPlanExecutionState(meta ProtocolExecutionMeta, plan appstore.DesktopPlan, resume *appstore.DesktopPlanExecutionState) appstore.DesktopPlanExecutionState {
	now := appstore.TimestampNowRFC3339()
	state := appstore.DesktopPlanExecutionState{
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
		cloned := appstore.CloneDesktopPlanExecutionState(resume)
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

func buildDesktopPlanStepStates(steps []appstore.DesktopPlanStep, existing []appstore.DesktopPlanStepExecutionState) []appstore.DesktopPlanStepExecutionState {
	items := make([]appstore.DesktopPlanStepExecutionState, 0, len(steps))
	for idx, step := range steps {
		toolName, _, _ := resolveDesktopPlanStepCall(step)
		item := appstore.DesktopPlanStepExecutionState{
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

func canResumeDesktopPlan(meta ProtocolExecutionMeta, plan appstore.DesktopPlan, resume *appstore.DesktopPlanExecutionState) bool {
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

func desktopPlanStartIndex(state *appstore.DesktopPlanExecutionState, totalSteps int) int {
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

func markDesktopPlanStepRunning(state *appstore.DesktopPlanExecutionState, index int, step appstore.DesktopPlanStep) {
	if state == nil {
		return
	}
	now := appstore.TimestampNowRFC3339()
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

func markDesktopPlanStepCompleted(state *appstore.DesktopPlanExecutionState, index int, result desktopPlanStepResult, output string) {
	if state == nil {
		return
	}
	now := appstore.TimestampNowRFC3339()
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

func markDesktopPlanStepFailed(state *appstore.DesktopPlanExecutionState, index int, attempts int, err error) {
	if state == nil {
		return
	}
	now := appstore.TimestampNowRFC3339()
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

func desktopPlanStepStateAt(state *appstore.DesktopPlanExecutionState, index int) *appstore.DesktopPlanStepExecutionState {
	if state == nil || index <= 0 {
		return nil
	}
	for i := range state.Steps {
		if state.Steps[i].Index == index {
			return &state.Steps[i]
		}
	}
	state.Steps = append(state.Steps, appstore.DesktopPlanStepExecutionState{Index: index})
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
