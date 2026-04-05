package setup

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/anyclaw/anyclaw/pkg/channel"
	"github.com/anyclaw/anyclaw/pkg/config"
	"github.com/anyclaw/anyclaw/pkg/llm"
	"github.com/anyclaw/anyclaw/pkg/plugin"
	"github.com/anyclaw/anyclaw/pkg/skills"
	"github.com/anyclaw/anyclaw/pkg/workspace"
)

type Severity string

const (
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

type CheckResult struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Severity    Severity `json:"severity"`
	Message     string   `json:"message"`
	Detail      string   `json:"detail,omitempty"`
	Hint        string   `json:"hint,omitempty"`
	Fixable     bool     `json:"fixable"`
	FixAction   string   `json:"fix_action,omitempty"`
	FixPriority int      `json:"fix_priority,omitempty"`
}

type Report struct {
	ConfigPath string        `json:"config_path"`
	StartedAt  time.Time     `json:"started_at"`
	FinishedAt time.Time     `json:"finished_at"`
	Checks     []CheckResult `json:"checks"`
}

type DoctorOptions struct {
	CheckConnectivity bool
	CreateMissingDirs bool
	AutoFix           bool
}

func (r *Report) Add(check CheckResult) {
	if r == nil {
		return
	}
	r.Checks = append(r.Checks, check)
}

func (r *Report) ErrorCount() int {
	return countSeverity(r, SeverityError)
}

func (r *Report) WarningCount() int {
	return countSeverity(r, SeverityWarning)
}

func (r *Report) HasErrors() bool {
	return r != nil && r.ErrorCount() > 0
}

type FixResult struct {
	CheckID  string `json:"check_id"`
	Fixed    bool   `json:"fixed"`
	Action   string `json:"action"`
	Error    string `json:"error,omitempty"`
	NewValue string `json:"new_value,omitempty"`
}

func (r *Report) FixableCount() int {
	count := 0
	for _, check := range r.Checks {
		if check.Fixable && (check.Severity == SeverityError || check.Severity == SeverityWarning) {
			count++
		}
	}
	return count
}

func (r *Report) FixableChecks() []CheckResult {
	var fixes []CheckResult
	for _, check := range r.Checks {
		if check.Fixable && (check.Severity == SeverityError || check.Severity == SeverityWarning) {
			fixes = append(fixes, check)
		}
	}
	return fixes
}

func (r *Report) FixAll(configPath string) ([]FixResult, error) {
	fixes := make([]FixResult, 0)

	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	for _, check := range r.FixableChecks() {
		result := FixResult{CheckID: check.ID}

		switch check.ID {
		case "security-dm-allow-all-no-ack":
			cfg.Security.RiskAcknowledged = true
			result.Action = "set security.risk_acknowledged=true"
			result.NewValue = "true"
			result.Fixed = true

		case "security-dm-policy-permissive":
			cfg.Channels.Security.DMPolicy = "allow-list"
			result.Action = "set channels.security.dm_policy=allow-list"
			result.NewValue = "allow-list"
			result.Fixed = true

		case "security-mention-gate-disabled":
			cfg.Channels.Security.MentionGate = true
			result.Action = "set channels.security.mention_gate=true"
			result.NewValue = "true"
			result.Fixed = true

		case "security-group-policy-permissive":
			cfg.Channels.Security.GroupPolicy = "mention-only"
			result.Action = "set channels.security.group_policy=mention-only"
			result.NewValue = "mention-only"
			result.Fixed = true

		case "security-no-default-deny":
			cfg.Channels.Security.DefaultDenyDM = true
			result.Action = "set channels.security.default_deny_dm=true"
			result.NewValue = "true"
			result.Fixed = true

		case "security-risk-not-acknowledged":
			cfg.Security.RiskAcknowledged = true
			result.Action = "set security.risk_acknowledged=true"
			result.NewValue = "true"
			result.Fixed = true

		default:
			result.Action = check.FixAction
			result.Error = "manual fix required"
		}
		fixes = append(fixes, result)
	}

	if err := cfg.Save(configPath); err != nil {
		return fixes, fmt.Errorf("failed to save config: %w", err)
	}

	return fixes, nil
}

func RunDoctor(ctx context.Context, configPath string, opts DoctorOptions) (*Report, *config.Config, error) {
	report := &Report{
		ConfigPath: config.ResolveConfigPath(configPath),
		StartedAt:  time.Now(),
	}
	defer func() {
		report.FinishedAt = time.Now()
	}()

	cfg, loadErr := config.Load(configPath)
	configExists := true
	if _, err := os.Stat(configPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			configExists = false
			report.Add(CheckResult{
				ID:       "config-file",
				Title:    "Config file",
				Severity: SeverityError,
				Message:  fmt.Sprintf("Config file not found: %s", report.ConfigPath),
				Hint:     "Run `anyclaw onboard` to generate a ready-to-use config.",
			})
		} else {
			report.Add(CheckResult{
				ID:       "config-file",
				Title:    "Config file",
				Severity: SeverityError,
				Message:  fmt.Sprintf("Unable to inspect config file: %v", err),
			})
		}
	} else {
		report.Add(CheckResult{
			ID:       "config-file",
			Title:    "Config file",
			Severity: SeverityInfo,
			Message:  fmt.Sprintf("Config file loaded from %s", report.ConfigPath),
		})
	}

	if loadErr != nil {
		report.Add(CheckResult{
			ID:       "config-parse",
			Title:    "Config parsing",
			Severity: SeverityError,
			Message:  loadErr.Error(),
			Hint:     "Fix the JSON or rerun onboarding to regenerate a clean config.",
		})
		return report, nil, loadErr
	}
	if !configExists {
		return report, cfg, fmt.Errorf("config file missing")
	}

	workDir := config.ResolvePath(configPath, cfg.Agent.WorkDir)
	workingDir := config.ResolvePath(configPath, cfg.Agent.WorkingDir)
	skillsDir := config.ResolvePath(configPath, cfg.Skills.Dir)
	pluginsDir := config.ResolvePath(configPath, cfg.Plugins.Dir)
	auditLog := config.ResolvePath(configPath, cfg.Security.AuditLog)

	checkDirectory(report, "work-dir", "Work dir", workDir, opts.CreateMissingDirs)
	checkDirectory(report, "workspace", "Workspace", workingDir, opts.CreateMissingDirs)
	checkDirectory(report, "skills-dir", "Skills dir", skillsDir, opts.CreateMissingDirs)
	checkDirectory(report, "plugins-dir", "Plugins dir", pluginsDir, opts.CreateMissingDirs)
	checkFileParent(report, "audit-log", "Audit log", auditLog, opts.CreateMissingDirs)
	checkWorkspaceBootstrap(report, workingDir)

	checkProviderConfiguration(report, cfg)
	if opts.CheckConnectivity {
		checkProviderConnectivity(ctx, report, cfg)
	}
	checkSkills(report, cfg, skillsDir)
	checkPlugins(report, cfg, pluginsDir)
	checkGatewayPort(report, cfg)
	checkDesktopDependencies(report, cfg)
	checkSecurityPolicy(report, cfg)

	if report.HasErrors() {
		return report, cfg, fmt.Errorf("doctor found %d issue(s)", report.ErrorCount())
	}
	return report, cfg, nil
}

func countSeverity(report *Report, severity Severity) int {
	if report == nil {
		return 0
	}
	count := 0
	for _, check := range report.Checks {
		if check.Severity == severity {
			count++
		}
	}
	return count
}

func checkDirectory(report *Report, id string, title string, path string, create bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		report.Add(CheckResult{
			ID:       id,
			Title:    title,
			Severity: SeverityError,
			Message:  fmt.Sprintf("%s is not configured.", title),
		})
		return
	}
	var err error
	if create {
		err = os.MkdirAll(path, 0o755)
	} else {
		_, err = os.Stat(path)
	}
	if err != nil {
		report.Add(CheckResult{
			ID:       id,
			Title:    title,
			Severity: SeverityError,
			Message:  fmt.Sprintf("%s is not ready: %v", title, err),
		})
		return
	}
	if probe := filepath.Join(path, ".anyclaw-write-check"); canWritePath(probe) {
		report.Add(CheckResult{
			ID:       id,
			Title:    title,
			Severity: SeverityInfo,
			Message:  fmt.Sprintf("%s is ready: %s", title, path),
		})
		return
	}
	report.Add(CheckResult{
		ID:       id,
		Title:    title,
		Severity: SeverityError,
		Message:  fmt.Sprintf("%s is not writable: %s", title, path),
	})
}

func checkFileParent(report *Report, id string, title string, path string, create bool) {
	if strings.TrimSpace(path) == "" {
		report.Add(CheckResult{
			ID:       id,
			Title:    title,
			Severity: SeverityWarning,
			Message:  fmt.Sprintf("%s is not configured.", title),
		})
		return
	}
	checkDirectory(report, id, title, filepath.Dir(path), create)
}

func checkWorkspaceBootstrap(report *Report, workingDir string) {
	if strings.TrimSpace(workingDir) == "" {
		return
	}
	files, err := workspace.LoadBootstrapFiles(workingDir, workspace.BootstrapOptions{})
	if err != nil {
		report.Add(CheckResult{
			ID:       "workspace-bootstrap",
			Title:    "Workspace bootstrap",
			Severity: SeverityWarning,
			Message:  fmt.Sprintf("Unable to inspect workspace bootstrap files: %v", err),
		})
		return
	}
	missing := make([]string, 0)
	for _, file := range files {
		if file.Missing {
			missing = append(missing, file.Name)
		}
	}
	if len(missing) == 0 {
		report.Add(CheckResult{
			ID:       "workspace-bootstrap",
			Title:    "Workspace bootstrap",
			Severity: SeverityInfo,
			Message:  "Workspace bootstrap files are present.",
		})
		return
	}
	report.Add(CheckResult{
		ID:       "workspace-bootstrap",
		Title:    "Workspace bootstrap",
		Severity: SeverityWarning,
		Message:  fmt.Sprintf("Workspace is missing bootstrap files: %s", strings.Join(missing, ", ")),
		Hint:     "Running onboarding or starting the runtime will recreate the standard workspace files.",
	})
}

func checkProviderConfiguration(report *Report, cfg *config.Config) {
	target := resolvedProviderTarget(cfg)
	if strings.TrimSpace(target.Provider) == "" {
		report.Add(CheckResult{
			ID:       "provider-config",
			Title:    "Provider config",
			Severity: SeverityError,
			Message:  "No active provider is configured.",
			Hint:     "Choose a provider during onboarding.",
		})
		return
	}
	if target.Provider == "compatible" && strings.TrimSpace(target.BaseURL) == "" {
		report.Add(CheckResult{
			ID:       "provider-base-url",
			Title:    "Provider base URL",
			Severity: SeverityError,
			Message:  "OpenAI-compatible mode requires a base_url.",
		})
		return
	}
	if baseURL := strings.TrimSpace(target.BaseURL); baseURL != "" {
		if _, err := url.ParseRequestURI(baseURL); err != nil {
			report.Add(CheckResult{
				ID:       "provider-base-url",
				Title:    "Provider base URL",
				Severity: SeverityError,
				Message:  fmt.Sprintf("Invalid provider base URL: %s", baseURL),
			})
			return
		}
	}
	if ProviderNeedsAPIKey(target.Provider) && strings.TrimSpace(target.APIKey) == "" {
		report.Add(CheckResult{
			ID:       "provider-api-key",
			Title:    "Provider API key",
			Severity: SeverityError,
			Message:  fmt.Sprintf("%s requires an API key.", ProviderLabel(target.Provider)),
			Hint:     ProviderHint(target.Provider),
		})
		return
	}
	report.Add(CheckResult{
		ID:       "provider-config",
		Title:    "Provider config",
		Severity: SeverityInfo,
		Message:  fmt.Sprintf("Active provider is %s / %s.", ProviderLabel(target.Provider), target.Model),
	})
}

func checkProviderConnectivity(ctx context.Context, report *Report, cfg *config.Config) {
	target := resolvedProviderTarget(cfg)
	if strings.TrimSpace(target.Provider) == "" || strings.TrimSpace(target.Model) == "" {
		return
	}
	if ProviderNeedsAPIKey(target.Provider) && strings.TrimSpace(target.APIKey) == "" {
		return
	}

	testCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	client, err := llm.NewClient(llm.Config{
		Provider:    target.Provider,
		Model:       target.Model,
		APIKey:      target.APIKey,
		BaseURL:     target.BaseURL,
		MaxTokens:   32,
		Temperature: 0,
	})
	if err != nil {
		report.Add(CheckResult{
			ID:       "provider-connectivity",
			Title:    "Model connectivity",
			Severity: SeverityError,
			Message:  fmt.Sprintf("Unable to initialize the provider client: %v", err),
		})
		return
	}
	_, err = client.Chat(testCtx, []llm.Message{{Role: "user", Content: "Reply with OK."}}, nil)
	if err != nil {
		report.Add(CheckResult{
			ID:       "provider-connectivity",
			Title:    "Model connectivity",
			Severity: SeverityError,
			Message:  fmt.Sprintf("Connectivity test failed for %s / %s.", ProviderLabel(target.Provider), target.Model),
			Detail:   trimDoctorDetail(err.Error()),
			Hint:     "Check the API key, model name, base URL, and outbound network access.",
		})
		return
	}
	report.Add(CheckResult{
		ID:       "provider-connectivity",
		Title:    "Model connectivity",
		Severity: SeverityInfo,
		Message:  fmt.Sprintf("Connectivity test passed for %s / %s.", ProviderLabel(target.Provider), target.Model),
	})
}

func checkSkills(report *Report, cfg *config.Config, skillsDir string) {
	manager := skills.NewSkillsManager(skillsDir)
	if err := manager.Load(); err != nil {
		report.Add(CheckResult{
			ID:       "skills-load",
			Title:    "Skills loading",
			Severity: SeverityError,
			Message:  fmt.Sprintf("Unable to load skills from %s: %v", skillsDir, err),
		})
		return
	}
	configured := configuredSkillNames(cfg)
	if len(configured) == 0 {
		report.Add(CheckResult{
			ID:       "skills-load",
			Title:    "Skills loading",
			Severity: SeverityInfo,
			Message:  fmt.Sprintf("%d skill(s) available.", len(manager.List())),
		})
		return
	}
	loaded := make(map[string]struct{}, len(manager.List()))
	for _, skill := range manager.List() {
		if skill == nil {
			continue
		}
		loaded[strings.ToLower(strings.TrimSpace(skill.Name))] = struct{}{}
	}
	missing := make([]string, 0)
	for _, name := range configured {
		if _, ok := loaded[strings.ToLower(strings.TrimSpace(name))]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		report.Add(CheckResult{
			ID:       "skills-load",
			Title:    "Skills loading",
			Severity: SeverityInfo,
			Message:  fmt.Sprintf("Configured skills are present (%d loaded).", len(manager.List())),
		})
		return
	}
	report.Add(CheckResult{
		ID:       "skills-missing",
		Title:    "Configured skills",
		Severity: SeverityError,
		Message:  fmt.Sprintf("Missing configured skills: %s", strings.Join(missing, ", ")),
		Hint:     "Install the missing skills or remove stale skill references from the agent profile.",
	})
}

func checkPlugins(report *Report, cfg *config.Config, pluginsDir string) {
	registry, err := plugin.NewRegistry(config.PluginsConfig{
		Dir:                pluginsDir,
		Enabled:            append([]string(nil), cfg.Plugins.Enabled...),
		AllowExec:          cfg.Plugins.AllowExec,
		ExecTimeoutSeconds: cfg.Plugins.ExecTimeoutSeconds,
		TrustedSigners:     append([]string(nil), cfg.Plugins.TrustedSigners...),
		RequireTrust:       cfg.Plugins.RequireTrust,
	})
	if err != nil {
		report.Add(CheckResult{
			ID:       "plugins-load",
			Title:    "Plugin loading",
			Severity: SeverityError,
			Message:  fmt.Sprintf("Unable to load plugins from %s: %v", pluginsDir, err),
		})
		return
	}
	manifests := registry.List()
	found := make(map[string]struct{}, len(manifests))
	for _, manifest := range manifests {
		found[strings.ToLower(strings.TrimSpace(manifest.Name))] = struct{}{}
	}
	missing := make([]string, 0)
	for _, name := range cfg.Plugins.Enabled {
		key := strings.ToLower(strings.TrimSpace(name))
		if key == "" {
			continue
		}
		if _, ok := found[key]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		report.Add(CheckResult{
			ID:       "plugins-load",
			Title:    "Plugin loading",
			Severity: SeverityInfo,
			Message:  fmt.Sprintf("%d plugin manifest(s) available.", len(manifests)),
		})
		return
	}
	report.Add(CheckResult{
		ID:       "plugins-missing",
		Title:    "Configured plugins",
		Severity: SeverityWarning,
		Message:  fmt.Sprintf("Configured plugins not found on disk: %s", strings.Join(missing, ", ")),
		Hint:     "Install the missing plugins or remove them from plugins.enabled.",
	})
}

func checkGatewayPort(report *Report, cfg *config.Config) {
	address := gatewayListenAddress(cfg)
	ln, err := net.Listen("tcp", address)
	if err != nil {
		report.Add(CheckResult{
			ID:       "gateway-port",
			Title:    "Gateway port",
			Severity: SeverityError,
			Message:  fmt.Sprintf("Gateway listen address is busy: %s", address),
			Detail:   err.Error(),
			Hint:     "Stop the conflicting process or change gateway.host / gateway.port.",
		})
		return
	}
	_ = ln.Close()
	report.Add(CheckResult{
		ID:       "gateway-port",
		Title:    "Gateway port",
		Severity: SeverityInfo,
		Message:  fmt.Sprintf("Gateway listen address is free: %s", address),
	})
}

func checkDesktopDependencies(report *Report, cfg *config.Config) {
	if runtime.GOOS != "windows" {
		if strings.EqualFold(strings.TrimSpace(cfg.Sandbox.ExecutionMode), "host-reviewed") {
			report.Add(CheckResult{
				ID:       "desktop-host",
				Title:    "Desktop host mode",
				Severity: SeverityWarning,
				Message:  "Desktop automation tools currently require Windows host mode.",
			})
		}
		return
	}

	if exe, err := findExecutable("pwsh", "powershell"); err == nil {
		report.Add(CheckResult{
			ID:       "desktop-powershell",
			Title:    "PowerShell",
			Severity: SeverityInfo,
			Message:  fmt.Sprintf("Desktop host shell is available: %s", exe),
		})
	} else {
		report.Add(CheckResult{
			ID:       "desktop-powershell",
			Title:    "PowerShell",
			Severity: SeverityError,
			Message:  "PowerShell is required for desktop automation.",
		})
	}

	if err := runPowerShellProbe(`Add-Type -AssemblyName UIAutomationClient; "ok"`); err != nil {
		report.Add(CheckResult{
			ID:       "desktop-ui-automation",
			Title:    "UI Automation",
			Severity: SeverityWarning,
			Message:  "Windows UI Automation assemblies are not ready.",
			Detail:   trimDoctorDetail(err.Error()),
		})
	} else {
		report.Add(CheckResult{
			ID:       "desktop-ui-automation",
			Title:    "UI Automation",
			Severity: SeverityInfo,
			Message:  "Windows UI Automation is available.",
		})
	}

	if strings.EqualFold(strings.TrimSpace(cfg.Sandbox.ExecutionMode), "host-reviewed") {
		if err := runPowerShellProbe(`[Environment]::UserInteractive`); err != nil {
			report.Add(CheckResult{
				ID:       "desktop-session",
				Title:    "Desktop session",
				Severity: SeverityWarning,
				Message:  "Host-reviewed mode needs an interactive Windows desktop session.",
				Detail:   trimDoctorDetail(err.Error()),
			})
		} else {
			report.Add(CheckResult{
				ID:       "desktop-session",
				Title:    "Desktop session",
				Severity: SeverityInfo,
				Message:  "Interactive Windows desktop session is available.",
			})
		}
	}

	if exe, err := exec.LookPath("tesseract"); err == nil {
		report.Add(CheckResult{
			ID:       "ocr-engine",
			Title:    "OCR engine",
			Severity: SeverityInfo,
			Message:  fmt.Sprintf("Tesseract OCR is available: %s", exe),
		})
	} else {
		report.Add(CheckResult{
			ID:       "ocr-engine",
			Title:    "OCR engine",
			Severity: SeverityWarning,
			Message:  "Tesseract OCR is not installed or not in PATH.",
			Hint:     "Install Tesseract if you plan to use desktop OCR or vision-driven app workflows.",
		})
	}
}

type providerTarget struct {
	Provider string
	Model    string
	BaseURL  string
	APIKey   string
}

func resolvedProviderTarget(cfg *config.Config) providerTarget {
	if cfg == nil {
		return providerTarget{}
	}
	if provider, ok := cfg.FindDefaultProviderProfile(); ok {
		return providerTarget{
			Provider: firstNonEmpty(provider.Provider, cfg.LLM.Provider),
			Model:    firstNonEmpty(provider.DefaultModel, cfg.LLM.Model),
			BaseURL:  firstNonEmpty(provider.BaseURL, cfg.LLM.BaseURL),
			APIKey:   firstNonEmpty(provider.APIKey, cfg.LLM.APIKey),
		}
	}
	return providerTarget{
		Provider: strings.TrimSpace(cfg.LLM.Provider),
		Model:    strings.TrimSpace(cfg.LLM.Model),
		BaseURL:  strings.TrimSpace(cfg.LLM.BaseURL),
		APIKey:   strings.TrimSpace(cfg.LLM.APIKey),
	}
}

func configuredSkillNames(cfg *config.Config) []string {
	if cfg == nil {
		return nil
	}
	if profile, ok := cfg.ResolveMainAgentProfile(); ok {
		return enabledSkillNames(profile.Skills)
	}
	return enabledSkillNames(cfg.Agent.Skills)
}

func enabledSkillNames(items []config.AgentSkillRef) []string {
	names := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if !item.Enabled {
			continue
		}
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		names = append(names, name)
	}
	return names
}

func gatewayListenAddress(cfg *config.Config) string {
	host := strings.TrimSpace(cfg.Gateway.Host)
	if host == "" {
		host = "127.0.0.1"
	}
	switch bind := strings.TrimSpace(strings.ToLower(cfg.Gateway.Bind)); bind {
	case "all":
		host = "0.0.0.0"
	case "loopback", "":
	default:
		host = cfg.Gateway.Bind
	}
	port := cfg.Gateway.Port
	if port <= 0 {
		port = 18789
	}
	return net.JoinHostPort(host, fmt.Sprintf("%d", port))
}

func canWritePath(path string) bool {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return false
	}
	_ = file.Close()
	_ = os.Remove(path)
	return true
}

func findExecutable(names ...string) (string, error) {
	for _, name := range names {
		path, err := exec.LookPath(name)
		if err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("not found")
}

func runPowerShellProbe(script string) error {
	exe, err := findExecutable("pwsh", "powershell")
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "-NoProfile", "-Command", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func trimDoctorDetail(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 240 {
		return value[:240] + "..."
	}
	return value
}

func checkSecurityPolicy(report *Report, cfg *config.Config) {
	policy := channel.ChannelPolicyFromConfig(cfg.Channels.Security)
	audit := channel.AuditChannelPolicy(policy)

	if audit.Passed {
		report.Add(CheckResult{
			ID:       "security-policy",
			Title:    "Security policy",
			Severity: SeverityInfo,
			Message:  fmt.Sprintf("Security policy score: %d/%d - All checks passed", audit.Score, audit.MaxScore),
			Fixable:  false,
		})
		return
	}

	report.Add(CheckResult{
		ID:       "security-policy",
		Title:    "Security policy",
		Severity: SeverityWarning,
		Message:  fmt.Sprintf("Security policy score: %d/%d - %d issue(s) found", audit.Score, audit.MaxScore, len(audit.Issues)),
	})

	for _, issue := range audit.Issues {
		fixPriority := 0
		switch issue.Severity {
		case "critical":
			fixPriority = 3
		case "warning":
			fixPriority = 2
		default:
			fixPriority = 1
		}
		report.Add(CheckResult{
			ID:          "security-" + issue.ID,
			Title:       issue.Title,
			Severity:    Severity(issue.Severity),
			Message:     issue.Message,
			Fixable:     issue.Fixable,
			FixAction:   issue.FixAction,
			FixPriority: fixPriority,
		})
	}
}
