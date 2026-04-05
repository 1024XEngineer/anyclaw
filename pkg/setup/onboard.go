package setup

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/anyclaw/anyclaw/pkg/config"
	"github.com/anyclaw/anyclaw/pkg/consoleio"
	"github.com/anyclaw/anyclaw/pkg/workspace"
)

type OnboardOptions struct {
	Interactive       bool
	CheckConnectivity bool
	Stdin             io.Reader
	Stdout            io.Writer
}

type OnboardResult struct {
	Config  *config.Config
	Report  *Report
	Created bool
}

func RunOnboarding(ctx context.Context, configPath string, opts OnboardOptions) (*OnboardResult, error) {
	if opts.Stdin == nil {
		opts.Stdin = os.Stdin
	}
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}

	created := false
	if _, err := os.Stat(configPath); errorsIsNotExist(err) {
		created = true
	}

	cfg, err := config.Load(configPath)
	if err != nil && !created {
		return nil, err
	}
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	applyOnboardingDefaults(cfg)

	if opts.Interactive {
		if err := runInteractiveOnboarding(cfg, opts.Stdin, opts.Stdout); err != nil {
			return nil, err
		}
	} else {
		EnsurePrimaryProviderProfile(cfg, cfg.LLM.Provider, cfg.LLM.Model, cfg.LLM.APIKey, cfg.LLM.BaseURL)
	}

	if err := prepareRuntimePaths(configPath, cfg); err != nil {
		return nil, err
	}
	if err := cfg.Save(configPath); err != nil {
		return nil, err
	}

	report, checkedCfg, doctorErr := RunDoctor(ctx, configPath, DoctorOptions{
		CheckConnectivity: opts.CheckConnectivity,
		CreateMissingDirs: true,
	})
	if checkedCfg != nil {
		cfg = checkedCfg
	}
	if doctorErr != nil && report == nil {
		return nil, doctorErr
	}
	return &OnboardResult{
		Config:  cfg,
		Report:  report,
		Created: created,
	}, doctorErr
}

func applyOnboardingDefaults(cfg *config.Config) {
	if cfg == nil {
		return
	}
	cfg.LLM.Provider = firstNonEmpty(CanonicalProvider(cfg.LLM.Provider), "openai")
	cfg.LLM.Model = firstNonEmpty(cfg.LLM.Model, DefaultModelForProvider(cfg.LLM.Provider))
	cfg.Agent.Name = firstNonEmpty(cfg.Agent.Name, "AnyClaw")
	cfg.Agent.WorkDir = firstNonEmpty(cfg.Agent.WorkDir, ".anyclaw")
	cfg.Agent.WorkingDir = firstNonEmpty(cfg.Agent.WorkingDir, "workflows/default")
	cfg.Agent.PermissionLevel = firstNonEmpty(cfg.Agent.PermissionLevel, "limited")
	cfg.Skills.Dir = firstNonEmpty(cfg.Skills.Dir, "skills")
	cfg.Plugins.Dir = firstNonEmpty(cfg.Plugins.Dir, "plugins")
	cfg.Security.AuditLog = firstNonEmpty(cfg.Security.AuditLog, ".anyclaw/audit/audit.jsonl")
	if cfg.Channels.Security.DMPolicy == "" {
		cfg.Channels.Security.DMPolicy = "allow-list"
	}
	if cfg.Channels.Security.GroupPolicy == "" {
		cfg.Channels.Security.GroupPolicy = "mention-only"
	}
	cfg.Channels.Security.MentionGate = true
	cfg.Channels.Security.DefaultDenyDM = true
	cfg.Channels.Security.PairingTTLHours = 72
	cfg.Security.RateLimitRPM = firstNonEmptyInt(cfg.Security.RateLimitRPM, 120)
}

func firstNonEmptyInt(vals ...int) int {
	for _, v := range vals {
		if v > 0 {
			return v
		}
	}
	return 0
}

func runInteractiveOnboarding(cfg *config.Config, input io.Reader, output io.Writer) error {
	reader := consoleio.NewReader(input)
	currentProvider := firstNonEmpty(cfg.LLM.Provider, "openai")
	sameProvider := false

	fmt.Fprintln(output, "Step 1/7: Choose provider")
	for idx, option := range ProviderOptions() {
		fmt.Fprintf(output, "  %d. %s (%s)\n", idx+1, option.Label, option.ID)
	}
	providerChoice, err := prompt(reader, output, fmt.Sprintf("Provider [%s]", currentProvider))
	if err != nil {
		return err
	}
	selectedProvider := ResolveProviderChoice(providerChoice, currentProvider)
	if selectedProvider == "" {
		selectedProvider = currentProvider
	}
	sameProvider = CanonicalProvider(currentProvider) == CanonicalProvider(selectedProvider)

	currentModel := firstNonEmpty(cfg.LLM.Model, DefaultModelForProvider(selectedProvider))
	modelChoice, err := prompt(reader, output, fmt.Sprintf("Model [%s]", currentModel))
	if err != nil {
		return err
	}
	selectedModel := firstNonEmpty(modelChoice, currentModel, DefaultModelForProvider(selectedProvider))

	baseURL := strings.TrimSpace(cfg.LLM.BaseURL)
	if !sameProvider {
		baseURL = ""
	}
	if selectedProvider == "compatible" {
		baseURLPrompt := firstNonEmpty(baseURL, "https://api.example.com/v1")
		baseURL, err = prompt(reader, output, fmt.Sprintf("Base URL [%s]", baseURLPrompt))
		if err != nil {
			return err
		}
		baseURL = firstNonEmpty(baseURL, baseURLPrompt)
	}

	apiKey := strings.TrimSpace(cfg.LLM.APIKey)
	if !sameProvider {
		apiKey = ""
	}
	if ProviderNeedsAPIKey(selectedProvider) {
		fmt.Fprintf(output, "%s\n", ProviderHint(selectedProvider))
		apiKey, err = prompt(reader, output, "API key [press Enter to keep current]")
		if err != nil {
			return err
		}
		apiKey = firstNonEmpty(apiKey, cfg.LLM.APIKey)
	} else {
		apiKey = ""
	}

	workspacePrompt := firstNonEmpty(cfg.Agent.WorkingDir, "workflows/default")
	workingDir, err := prompt(reader, output, fmt.Sprintf("Workspace [%s]", workspacePrompt))
	if err != nil {
		return err
	}

	namePrompt := firstNonEmpty(cfg.Agent.Name, "AnyClaw")
	agentName, err := prompt(reader, output, fmt.Sprintf("Agent name [%s]", namePrompt))
	if err != nil {
		return err
	}

	cfg.Agent.Name = firstNonEmpty(agentName, namePrompt)
	cfg.Agent.WorkingDir = firstNonEmpty(workingDir, workspacePrompt)
	cfg.LLM.Provider = selectedProvider
	cfg.LLM.Model = selectedModel
	cfg.LLM.APIKey = strings.TrimSpace(apiKey)
	if selectedProvider == "compatible" {
		cfg.LLM.BaseURL = strings.TrimSpace(baseURL)
	} else {
		cfg.LLM.BaseURL = DefaultBaseURLForProvider(selectedProvider)
	}
	if !ProviderNeedsAPIKey(selectedProvider) {
		cfg.LLM.APIKey = ""
	}
	EnsurePrimaryProviderProfile(cfg, selectedProvider, selectedModel, cfg.LLM.APIKey, cfg.LLM.BaseURL)

	fmt.Fprintln(output)
	fmt.Fprintln(output, "Step 6/7: Channel security defaults")
	fmt.Fprintln(output, "  DM policy: allow-list (only allowed users can DM)")
	fmt.Fprintln(output, "  Group policy: mention-only (bot responds only when @mentioned)")
	fmt.Fprintln(output, "  Mention gate: enabled")
	fmt.Fprintln(output, "  Default deny DM: enabled")
	secChoice, err := prompt(reader, output, "Accept security defaults? [Y/n]")
	if err != nil {
		return err
	}
	if strings.TrimSpace(strings.ToLower(secChoice)) != "n" {
		cfg.Channels.Security.DMPolicy = "allow-list"
		cfg.Channels.Security.GroupPolicy = "mention-only"
		cfg.Channels.Security.MentionGate = true
		cfg.Channels.Security.DefaultDenyDM = true
		cfg.Channels.Security.PairingTTLHours = 72
		cfg.Security.RiskAcknowledged = true
	}

	fmt.Fprintln(output)
	fmt.Fprintln(output, "Step 7/7: Risk acknowledgement")
	fmt.Fprintln(output, "  AnyClaw can execute commands on your system.")
	fmt.Fprintln(output, "  By acknowledging, you accept responsibility for agent actions.")
	riskChoice, err := prompt(reader, output, "Acknowledge risks? [Y/n]")
	if err != nil {
		return err
	}
	if strings.TrimSpace(strings.ToLower(riskChoice)) != "n" {
		cfg.Security.RiskAcknowledged = true
	}

	return nil
}

func prepareRuntimePaths(configPath string, cfg *config.Config) error {
	workDir := config.ResolvePath(configPath, cfg.Agent.WorkDir)
	workingDir := config.ResolvePath(configPath, cfg.Agent.WorkingDir)
	skillsDir := config.ResolvePath(configPath, cfg.Skills.Dir)
	pluginsDir := config.ResolvePath(configPath, cfg.Plugins.Dir)

	for _, path := range []string{workDir, workingDir, skillsDir, pluginsDir, filepath.Dir(config.ResolvePath(configPath, cfg.Security.AuditLog))} {
		if strings.TrimSpace(path) == "" {
			continue
		}
		if err := os.MkdirAll(path, 0o755); err != nil {
			return err
		}
	}
	return workspace.EnsureBootstrap(workingDir, workspace.BootstrapOptions{
		AgentName:        cfg.Agent.Name,
		AgentDescription: cfg.Agent.Description,
	})
}

func prompt(reader *consoleio.Reader, output io.Writer, label string) (string, error) {
	fmt.Fprintf(output, "%s: ", label)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func errorsIsNotExist(err error) bool {
	return err != nil && os.IsNotExist(err)
}
