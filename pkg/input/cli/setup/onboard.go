package setup

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/1024XEngineer/anyclaw/pkg/config"
	"github.com/1024XEngineer/anyclaw/pkg/input/cli/consoleio"
	"github.com/1024XEngineer/anyclaw/pkg/workspace"
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
	// Don't set model default here - it will be set after user chooses provider
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

	fmt.Fprintln(output, "")
	fmt.Fprintln(output, "=== AnyClaw 设置向导 ===")
	fmt.Fprintln(output, "")
	fmt.Fprintln(output, "Step 1/8: 选择语言")
	langPrompt := "语言 [zh-CN]"
	langChoice, err := prompt(reader, output, langPrompt)
	if err != nil {
		return err
	}
	langChoice = strings.TrimSpace(langChoice)
	if langChoice == "" {
		langChoice = "zh-CN"
	}
	cfg.Agent.Lang = langChoice

	fmt.Fprintln(output, "Step 2/8: 选择 LLM 提供商")
	for idx, option := range ProviderOptions() {
		fmt.Fprintf(output, "  %d. %s (%s)\n", idx+1, option.Label, option.ID)
	}
	providerChoice, err := prompt(reader, output, fmt.Sprintf("提供商 [%s]", currentProvider))
	if err != nil {
		return err
	}
	selectedProvider := ResolveProviderChoice(providerChoice, currentProvider)
	if selectedProvider == "" {
		selectedProvider = currentProvider
	}
	sameProvider = CanonicalProvider(currentProvider) == CanonicalProvider(selectedProvider)

	availableModels := AvailableModelsForProvider(selectedProvider)
	if len(availableModels) > 0 {
		fmt.Fprintln(output, "  可用模型:")
		for _, m := range availableModels {
			fmt.Fprintf(output, "    - %s\n", m)
		}
	}

	currentModel := firstNonEmpty(cfg.LLM.Model, DefaultModelForProvider(selectedProvider))
	modelChoice, err := prompt(reader, output, fmt.Sprintf("模型 [%s]", currentModel))
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
		apiKey, err = prompt(reader, output, "API key [回车保持当前]")
		if err != nil {
			return err
		}
		apiKey = firstNonEmpty(apiKey, cfg.LLM.APIKey)
	} else {
		apiKey = ""
	}

	workspacePrompt := firstNonEmpty(cfg.Agent.WorkingDir, "workflows/default")
	workingDir, err := prompt(reader, output, fmt.Sprintf("工作区目录 [%s]", workspacePrompt))
	if err != nil {
		return err
	}

	namePrompt := firstNonEmpty(cfg.Agent.Name, "AnyClaw")
	agentName, err := prompt(reader, output, fmt.Sprintf("Agent 名称 [%s]", namePrompt))
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

	fmt.Fprintln(output, "")
	fmt.Fprintln(output, "Step 7/8: 工作偏好设置")
	fmt.Fprintln(output, "  你希望我主要帮你做什么类型的工作？")
	fmt.Fprintln(output, "  例如：编程、文档写作、日常任务、浏览器自动化等")
	workFocus, err := prompt(reader, output, "工作方向 [通用]")
	if err != nil {
		return err
	}
	cfg.Agent.WorkFocus = firstNonEmpty(workFocus, "通用")

	fmt.Fprintln(output, "  你希望我默认的行为方式是怎样的？")
	fmt.Fprintln(output, "  例如：简洁快速 / 详细解释 / 主动建议")
	behaviorStyle, err := prompt(reader, output, "行为风格 [简洁]")
	if err != nil {
		return err
	}
	cfg.Agent.BehaviorStyle = firstNonEmpty(behaviorStyle, "简洁")

	fmt.Fprintln(output, "  有什么需要遵守的约束或偏好吗？")
	fmt.Fprintln(output, "  例如：不要删除文件 / 只读模式 / 确认后执行")
	constraints, err := prompt(reader, output, "约束偏好 [无]")
	if err != nil {
		return err
	}
	cfg.Agent.Constraints = strings.TrimSpace(constraints)

	fmt.Fprintln(output, "")
	fmt.Fprintln(output, "Step 8/8: 安全设置")
	fmt.Fprintln(output, "  DM 策略: allow-list (仅允许的用户可以发私信)")
	fmt.Fprintln(output, "  群组策略: mention-only (仅 @mention 时响应)")
	fmt.Fprintln(output, "  提及门控: 启用")
	fmt.Fprintln(output, "  默认拒绝私信: 启用")
	secChoice, err := prompt(reader, output, "接受安全默认设置？[Y/n]")
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

	fmt.Fprintln(output, "")
	fmt.Fprintln(output, "风险提示")
	fmt.Fprintln(output, "  AnyClaw 可以在你的系统上执行命令。")
	fmt.Fprintln(output, "  确认后，你将对 agent 的行为负责。")
	riskChoice, err := prompt(reader, output, "确认风险？[Y/n]")
	if err != nil {
		return err
	}
	if strings.TrimSpace(strings.ToLower(riskChoice)) != "n" {
		cfg.Security.RiskAcknowledged = true
	}

	fmt.Fprintln(output, "")
	fmt.Fprintln(output, "设置完成！")

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
		UserProfile:      bootstrapUserProfile(cfg),
		WorkspaceFocus:   strings.TrimSpace(cfg.Agent.WorkFocus),
		AssistantStyle:   strings.TrimSpace(cfg.Agent.BehaviorStyle),
		Constraints:      strings.TrimSpace(cfg.Agent.Constraints),
	})
}

func bootstrapUserProfile(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}

	parts := []string{}
	if lang := strings.TrimSpace(cfg.Agent.Lang); lang != "" {
		parts = append(parts, "Default language: "+lang)
	}
	return strings.Join(parts, "; ")
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
