package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/anyclaw/anyclaw/pkg/agent"
	"github.com/anyclaw/anyclaw/pkg/audit"
	"github.com/anyclaw/anyclaw/pkg/config"
	"github.com/anyclaw/anyclaw/pkg/llm"
	"github.com/anyclaw/anyclaw/pkg/routing"
	appRuntime "github.com/anyclaw/anyclaw/pkg/runtime"
	"github.com/anyclaw/anyclaw/pkg/skills"
	"github.com/anyclaw/anyclaw/pkg/tools"
	"github.com/anyclaw/anyclaw/pkg/ui"
)

var version = appRuntime.Version

type RuntimeState struct {
	llmClient  *llm.ClientWrapper
	cfg        *config.Config
	agent      *agent.Agent
	skills     *skills.SkillsManager
	audit      *audit.Logger
	reader     *bufio.Reader
	configPath string
	workDir    string
	workingDir string
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Args[1:]); err != nil {
		printError("%v", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "skill":
			runSkillCommand()
			return nil
		case "skillhub":
			runSkillhubCommand()
			return nil
		case "shell":
			return runShellCommand(args[1:])
		case "gateway":
			return runGatewayCommand(ctx, args[1:])
		case "plugin":
			return runPluginCommand(args[1:])
		case "doctor":
			return runDoctorCommand(args[1:])
		case "onboard":
			return runOnboardCommand(args[1:])
		case "agent":
			return runAgentCommand(ctx, args[1:])
		case "store":
			return runStoreCommand(args[1:])
		case "task":
			return runTaskCommand(ctx, args[1:])
		}
	}

	return runRootCommand(ctx, args)
}

func runRootCommand(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("anyclaw", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)

	showVersion := fs.Bool("version", false, "show version")
	showProviders := fs.Bool("providers", false, "show available providers")
	setProvider := fs.String("provider", "", "set LLM provider")
	setModel := fs.String("model", "", "set LLM model")
	setAPIKey := fs.String("api-key", "", "set API key")
	interactive := fs.Bool("i", false, "interactive mode")
	setup := fs.Bool("setup", false, "run setup wizard")
	configPath := fs.String("config", "anyclaw.json", "path to config file")

	if err := fs.Parse(args); err != nil {
		return err
	}

	printBanner()

	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("閰嶇疆鍔犺浇澶辫触: %w", err)
	}

	if *showVersion {
		fmt.Printf("%sAnyClaw version %s%s\n", ui.Cyan.Sprint(""), version, ui.Reset.Sprint(""))
		fmt.Printf("%sFile-first memory AI agent%s\n", ui.Bold.Sprint(""), ui.Reset.Sprint(""))
		return nil
	}

	if *showProviders {
		showAvailableProviders()
		return nil
	}

	if *setProvider != "" || *setModel != "" || *setAPIKey != "" {
		if *setProvider != "" {
			cfg.LLM.Provider = *setProvider
		}
		if *setModel != "" {
			cfg.LLM.Model = *setModel
		}
		if *setAPIKey != "" {
			cfg.LLM.APIKey = *setAPIKey
		}
		if err := cfg.Save(*configPath); err != nil {
			return err
		}
		printSuccess("Config updated: %s", *configPath)
		return nil
	}

	if *setup {
		runSetupWizard(cfg)
		return cfg.Save(*configPath)
	}

	app, err := appRuntime.Bootstrap(appRuntime.BootstrapOptions{
		ConfigPath: *configPath,
		Progress:   bootProgress,
	})
	if err != nil {
		return fmt.Errorf("鍚姩澶辫触: %w", err)
	}

	state := &RuntimeState{
		llmClient:  app.LLM,
		cfg:        app.Config,
		agent:      app.Agent,
		skills:     app.Skills,
		audit:      app.Audit,
		reader:     bufio.NewReader(os.Stdin),
		configPath: *configPath,
		workDir:    app.WorkDir,
		workingDir: app.WorkingDir,
	}
	rebindBuiltins(state)

	fmt.Println(ui.Dim.Sprint(strings.Repeat("-", 50)))

	message := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if message != "" && !*interactive {
		response, err := state.agent.Run(ctx, message)
		if err != nil {
			return err
		}
		fmt.Printf("%s\n", ui.Bold.Sprint(response))
		return nil
	}

	runInteractive(ctx, state)
	return nil
}

func runDoctorCommand(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	issues := 0
	printBanner()
	fmt.Printf("%s\n", ui.Bold.Sprint("AnyClaw doctor"))
	fmt.Println(ui.Dim.Sprint(strings.Repeat("-", 50)))

	if _, err := os.Stat(*configPath); err == nil {
		printSuccess("Config file found: %s", appRuntime.ResolveConfigPath(*configPath))
	} else {
		issues++
		printError("Config file missing: %s", appRuntime.ResolveConfigPath(*configPath))
	}

	if cfg.LLM.APIKey == "" {
		issues++
		printError("No API key configured")
	} else if strings.Contains(strings.ToLower(appRuntime.ResolveConfigPath(*configPath)), "anyclaw.json") {
		printInfo("API key is loaded; prefer environment variables over storing secrets in config")
	}

	for _, dir := range []struct {
		label string
		path  string
	}{
		{label: "Work dir", path: cfg.Agent.WorkDir},
		{label: "Working dir", path: cfg.Agent.WorkingDir},
		{label: "Skills dir", path: cfg.Skills.Dir},
	} {
		path := dir.path
		if path == "" {
			issues++
			printError("%s not configured", dir.label)
			continue
		}
		if err := os.MkdirAll(path, 0o755); err != nil {
			issues++
			printError("%s not writable: %v", dir.label, err)
		} else {
			printSuccess("%s ready: %s", dir.label, path)
		}
	}

	printInfo("Gateway address: %s", appRuntime.GatewayAddress(cfg))
	if issues == 0 {
		printSuccess("Doctor checks passed")
		return nil
	}
	return fmt.Errorf("doctor found %d issue(s)", issues)
}

func runOnboardCommand(args []string) error {
	fs := flag.NewFlagSet("onboard", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	printBanner()
	runSetupWizard(cfg)
	if err := cfg.Save(*configPath); err != nil {
		return err
	}
	printSuccess("Onboarding complete: %s", appRuntime.ResolveConfigPath(*configPath))
	return nil
}

func printBanner() {
	ui.Banner()
}

func printError(format string, args ...any) {
	fmt.Printf("%s\n", ui.Error.Sprint("鉁?閿欒: ")+fmt.Sprintf(format, args...))
}

func printSuccess(format string, args ...any) {
	fmt.Printf("%s\n", ui.Success.Sprint("鉁?")+fmt.Sprintf(format, args...))
}

func printInfo(format string, args ...any) {
	fmt.Printf("%s\n", ui.Warning.Sprint("鈩?")+fmt.Sprintf(format, args...))
}

func bootProgress(ev appRuntime.BootEvent) {
	switch ev.Status {
	case "start":
		fmt.Printf("  %s %-12s %s", ui.Cyan.Sprint("..."), ui.Dim.Sprint(string(ev.Phase)), ev.Message)
	case "ok":
		fmt.Printf("\r  %s %-12s %s %s\n", ui.Green.Sprint("OK"), ui.Cyan.Sprint(string(ev.Phase)), ev.Message, ui.Dim.Sprint(ev.Dur.Round(time.Millisecond)))
	case "warn":
		fmt.Printf("\r  %s %-12s %s %s\n", ui.Yellow.Sprint("WARN"), ui.Cyan.Sprint(string(ev.Phase)), ev.Message, ui.Dim.Sprint(ev.Dur.Round(time.Millisecond)))
	case "skip":
		fmt.Printf("\r  %s %-12s %s %s\n", ui.Dim.Sprint("SKIP"), ui.Cyan.Sprint(string(ev.Phase)), ev.Message, ui.Dim.Sprint(ev.Dur.Round(time.Millisecond)))
	case "fail":
		errMsg := ""
		if ev.Err != nil {
			errMsg = ": " + ev.Err.Error()
		}
		fmt.Printf("\r  %s %-12s %s%s %s\n", ui.Red.Sprint("FAIL"), ui.Cyan.Sprint(string(ev.Phase)), ev.Message, errMsg, ui.Dim.Sprint(ev.Dur.Round(time.Millisecond)))
	}
}

func runSetupWizard(cfg *config.Config) {
	fmt.Println(ui.Dim.Sprint(strings.Repeat("-", 50)))
	fmt.Printf("%s\n\n", ui.Bold.Sprint("Setup Wizard"))

	reader := bufio.NewReader(os.Stdin)

	fmt.Printf("%s\n\n", ui.Bold.Sprint("Step 1/5: Choose provider"))
	showAvailableProviders()
	fmt.Printf("%s\nProvider > %s", ui.Cyan.Sprint(""), ui.Reset.Sprint(""))

	provider, _ := reader.ReadString('\n')
	provider = strings.TrimSpace(strings.ToLower(provider))
	if provider == "" {
		provider = "qwen"
	}

	if provider == "ali" || provider == "alibaba" {
		provider = "qwen"
	}

	cfg.LLM.Provider = provider

	fmt.Printf("\n%s\n\n", ui.Bold.Sprint("Step 2/5: Choose model"))
	showModelsForProvider(provider)
	fmt.Printf("%s\nModel > %s", ui.Cyan.Sprint(""), ui.Reset.Sprint(""))

	model, _ := reader.ReadString('\n')
	model = strings.TrimSpace(strings.ToLower(model))
	if model == "" {
		model = getDefaultModel(provider)
	}
	cfg.LLM.Model = model

	fmt.Printf("\n%s\n", ui.Bold.Sprint("Step 3/5: API key"))
	fmt.Printf("%s\n", getProviderHint(provider))
	fmt.Printf("%sAPI key: %s", ui.Cyan.Sprint(""), ui.Reset.Sprint(""))

	apiKey, _ := reader.ReadString('\n')
	apiKey = strings.TrimSpace(apiKey)
	cfg.LLM.APIKey = apiKey

	fmt.Printf("\n%s", ui.Bold.Sprint("Step 4/5: Proxy"))
	fmt.Printf("%s (optional, press Enter to skip)%s", ui.Yellow.Sprint(""), ui.Reset.Sprint(""))
	fmt.Printf("%s\n> %s", ui.Green.Sprint(""), ui.Reset.Sprint(""))

	proxy, _ := reader.ReadString('\n')
	proxy = strings.TrimSpace(proxy)
	cfg.LLM.Proxy = proxy

	fmt.Printf("\n%s", ui.Bold.Sprint("Step 5/5: Agent name"))
	fmt.Printf("%s (榛樿: AnyClaw)%s", ui.Yellow.Sprint(""), ui.Reset.Sprint(""))
	fmt.Printf("%s\n> %s", ui.Green.Sprint(""), ui.Reset.Sprint(""))

	name, _ := reader.ReadString('\n')
	name = strings.TrimSpace(name)
	if name != "" {
		cfg.Agent.Name = name
	}

	fmt.Println()
	printSuccess("Config saved")
	fmt.Println(ui.Dim.Sprint(strings.Repeat("-", 50)))
}

func getDefaultModel(provider string) string {
	defaults := map[string]string{
		"openai":    "gpt-4o-mini",
		"anthropic": "claude-sonnet-4-7",
		"qwen":      "qwen-plus",
		"ollama":    "llama3.2",
	}
	if model, ok := defaults[provider]; ok {
		return model
	}
	return "gpt-4o-mini"
}

func getProviderHint(provider string) string {
	hints := map[string]string{
		"openai":     "Get API key: https://platform.openai.com/api-keys",
		"anthropic":  "Get API key: https://console.anthropic.com/settings/keys",
		"qwen":       "Get API key: https://dashscope.console.aliyun.com/apiKey",
		"ollama":     "No API key needed. Ensure Ollama is running locally: https://ollama.com",
		"compatible": "Enter your OpenAI-compatible API key.",
	}
	if hint, ok := hints[provider]; ok {
		return hint
	}
	return "Enter your API key."
}

func runInteractive(ctx context.Context, state *RuntimeState) {
	fmt.Println()
	fmt.Println(ui.Dim.Sprint(strings.Repeat("-", 50)))
	fmt.Printf("%s\n", ui.Bold.Sprint("Interactive commands:"))
	fmt.Println("  /exit, /quit, /q   - exit")
	fmt.Println("  /clear             - clear chat history")
	fmt.Println("  /memory            - show memory")
	fmt.Println("  /skills            - list skills")
	fmt.Println("  /tools             - list tools")
	fmt.Println("  /provider          - current provider/model")
	fmt.Println("  /providers         - available providers")
	fmt.Println("  /models <name>     - models for provider")
	fmt.Println("  /agents            - show agent profiles")
	fmt.Println("  /agent use <name>  - switch active agent")
	fmt.Println("  /audit             - recent audit log")
	fmt.Println("  /set provider <v>  - set provider")
	fmt.Println("  /set model <v>     - set model")
	fmt.Println("  /set apikey <v>    - set API key")
	fmt.Println("  /set temp <v>      - set temperature (0.0-2.0)")
	fmt.Println("  /help, /?          - help")
	fmt.Println(ui.Dim.Sprint(strings.Repeat("-", 50)))
	fmt.Println()

	for {
		fmt.Printf("%s", ui.Prompt())
		input, err := state.reader.ReadString('\n')
		if err != nil {
			break
		}
		input = strings.TrimSpace(input)

		if input == "" {
			continue
		}

		if strings.HasPrefix(input, "/") {
			if handleCommand(ctx, state, input) {
				break
			}
			continue
		}

		fmt.Println()
		routeInfo := applyLLMRoute(state, input)
		response, err := state.agent.Run(ctx, input)
		if err != nil {
			printError("%v", err)
			continue
		}
		if routeInfo != "" {
			fmt.Printf("%s%s%s\n", ui.Dim.Sprint(""), routeInfo, ui.Reset.Sprint(""))
		}
		fmt.Printf("%s\n\n", ui.Bold.Sprint(response))
	}
}

func rebindBuiltins(state *RuntimeState) {
	if state == nil || state.agent == nil {
		return
	}
	registry := tools.NewRegistry()
	sandboxManager := tools.NewSandboxManager(state.cfg.Sandbox, state.workingDir)
	tools.RegisterBuiltins(registry, tools.BuiltinOptions{
		WorkingDir:            state.workingDir,
		PermissionLevel:       state.cfg.Agent.PermissionLevel,
		ExecutionMode:         state.cfg.Sandbox.ExecutionMode,
		DangerousPatterns:     state.cfg.Security.DangerousCommandPatterns,
		ProtectedPaths:        state.cfg.Security.ProtectedPaths,
		CommandTimeoutSeconds: state.cfg.Security.CommandTimeoutSeconds,
		AuditLogger:           state.audit,
		Sandbox:               sandboxManager,
		ConfirmDangerousCommand: func(command string) bool {
			if !state.cfg.Agent.RequireConfirmationForDangerous {
				return true
			}
			fmt.Printf("%s妫€娴嬪埌楂橀闄╁懡浠?%s %s\n", ui.Warning.Sprint(""), ui.Reset.Sprint(""), command)
			fmt.Printf("%s纭鎵ц? (y/N): ", ui.Yellow.Sprint(""))
			confirm, _ := state.reader.ReadString('\n')
			return strings.EqualFold(strings.TrimSpace(confirm), "y")
		},
	})
	state.skills.RegisterTools(registry, skills.ExecutionOptions{AllowExec: state.cfg.Plugins.AllowExec, ExecTimeoutSeconds: state.cfg.Plugins.ExecTimeoutSeconds})
	state.agent.SetTools(registry)
}

func applyLLMRoute(state *RuntimeState, input string) string {
	decision := routing.DecideLLM(state.cfg.LLM, input)
	providerChanged := strings.TrimSpace(decision.Provider) != "" && decision.Provider != state.cfg.LLM.Provider
	modelChanged := strings.TrimSpace(decision.Model) != "" && decision.Model != state.cfg.LLM.Model
	if providerChanged {
		if err := state.llmClient.SwitchProvider(decision.Provider); err == nil {
			state.cfg.LLM.Provider = decision.Provider
		}
	}
	if modelChanged {
		if err := state.llmClient.SwitchModel(decision.Model); err == nil {
			state.cfg.LLM.Model = decision.Model
		}
	}
	if providerChanged || modelChanged {
		return fmt.Sprintf("[妯″瀷璺敱: %s -> %s/%s]", decision.Reason, state.cfg.LLM.Provider, state.cfg.LLM.Model)
	}
	return ""
}

func showAgentProfiles(state *RuntimeState) {
	fmt.Println()
	fmt.Printf("%s\n", ui.Bold.Sprint("Agent 鍒楄〃:"))
	fmt.Printf("  褰撳墠 Agent: %s\n", state.cfg.Agent.Name)
	fmt.Printf("  鏉冮檺绾у埆: %s\n", state.cfg.Agent.PermissionLevel)
	if len(state.cfg.Agent.Profiles) == 0 {
		fmt.Println("  (鏈厤缃澶?Agent)")
		return
	}
	for _, profile := range state.cfg.Agent.Profiles {
		marker := " "
		if strings.EqualFold(profile.Name, state.cfg.Agent.ActiveProfile) || strings.EqualFold(profile.Name, state.cfg.Agent.Name) {
			marker = "*"
		}
		fmt.Printf("  %s %s - %s [%s]\n", marker, profile.Name, profile.Description, profile.PermissionLevel)
	}
}

func showAuditLog(state *RuntimeState) {
	events, err := state.audit.Tail(10)
	if err != nil {
		printError("璇诲彇瀹¤鏃ュ織澶辫触: %v", err)
		return
	}
	fmt.Println()
	fmt.Printf("%s\n", ui.Bold.Sprint("鏈€杩戝璁℃棩蹇?"))
	if len(events) == 0 {
		fmt.Println("  (鏆傛棤鏃ュ織)")
		return
	}
	for _, event := range events {
		line := fmt.Sprintf("  %s | %s | %s", event.Time, event.AgentName, event.Action)
		if event.Error != "" {
			line += " | error=" + event.Error
		}
		fmt.Println(line)
	}
}

func switchAgentProfile(state *RuntimeState, name string) error {
	if !state.cfg.ApplyAgentProfile(name) {
		return fmt.Errorf("agent 涓嶅瓨鍦? %s", name)
	}
	if err := state.cfg.Save(state.configPath); err != nil {
		return err
	}
	app, err := appRuntime.Bootstrap(appRuntime.BootstrapOptions{
		ConfigPath: state.configPath,
		Progress: func(ev appRuntime.BootEvent) {
			if ev.Status == "fail" {
				printError("%s: %v", ev.Message, ev.Err)
			}
		},
	})
	if err != nil {
		return fmt.Errorf("鍒囨崲 agent 澶辫触: %w", err)
	}
	history := state.agent.GetHistory()
	state.agent = app.Agent
	state.agent.SetHistory(history)
	state.llmClient = app.LLM
	state.audit = app.Audit
	state.workDir = app.WorkDir
	state.workingDir = app.WorkingDir
	state.cfg = app.Config
	rebindBuiltins(state)
	return nil
}

func handleCommand(ctx context.Context, state *RuntimeState, input string) bool {
	cmd := strings.ToLower(strings.TrimSpace(input))

	switch {
	case cmd == "/exit", cmd == "/quit", cmd == "/q":
		fmt.Println()
		printSuccess("鍐嶈!")
		return true

	case cmd == "/help", cmd == "/?":
		fmt.Println()
		fmt.Printf("%s\n", ui.Bold.Sprint("Available commands:"))
		fmt.Println("  /exit, /quit, /q   - exit")
		fmt.Println("  /clear             - clear chat history")
		fmt.Println("  /memory            - show memory")
		fmt.Println("  /skills            - list skills")
		fmt.Println("  /tools             - list tools")
		fmt.Println("  /provider          - current provider/model")
		fmt.Println("  /providers         - available providers")
		fmt.Println("  /models <name>     - models for provider")
		fmt.Println("  /agents            - show agent profiles")
		fmt.Println("  /agent use <name>  - switch active agent")
		fmt.Println("  /audit             - recent audit log")
		fmt.Println("  /set provider <v>  - set provider")
		fmt.Println("  /set model <v>     - set model")
		fmt.Println("  /set apikey <v>    - set API key")
		fmt.Println("  /set temp <v>      - set temperature (0.0-2.0)")
		return false

	case cmd == "/clear":
		state.agent.ClearHistory()
		printSuccess("Chat history cleared")
		return false

	case cmd == "/memory":
		mem, _ := state.agent.ShowMemory()
		fmt.Println()
		fmt.Println(ui.Dim.Sprint(strings.Repeat("鈹€", 40)))
		fmt.Println(mem)
		fmt.Println(ui.Dim.Sprint(strings.Repeat("鈹€", 40)))
		return false

	case cmd == "/skills":
		skills := state.agent.ListSkills()
		fmt.Println()
		fmt.Printf("%s\n", ui.Bold.Sprint("鎶€鑳藉垪琛?"))
		if len(skills) == 0 {
			fmt.Printf("%s\n", ui.Yellow.Sprint("  (鏃?"))
		}
		for _, s := range skills {
			fmt.Printf("  %s- %s: %s\n", ui.Cyan.Sprint(""), s.Name, s.Description)
		}
		return false

	case cmd == "/tools":
		tools := state.agent.ListTools()
		fmt.Println()
		fmt.Printf("%s\n", ui.Bold.Sprint("宸ュ叿鍒楄〃:"))
		for _, t := range tools {
			fmt.Printf("  %s- %s: %s\n", ui.Cyan.Sprint(""), t.Name, t.Description)
		}
		return false

	case cmd == "/provider":
		fmt.Println()
		fmt.Printf("%s鎻愪緵鍟? %s\n", ui.Cyan.Sprint(""), state.cfg.LLM.Provider)
		fmt.Printf("%s妯″瀷: %s\n", ui.Cyan.Sprint(""), state.cfg.LLM.Model)
		fmt.Printf("%s娓╁害: %.1f\n", ui.Cyan.Sprint(""), state.cfg.LLM.Temperature)
		fmt.Printf("%s鏉冮檺绾у埆: %s\n", ui.Cyan.Sprint(""), state.cfg.Agent.PermissionLevel)
		return false

	case cmd == "/agents":
		showAgentProfiles(state)
		return false

	case cmd == "/audit":
		showAuditLog(state)
		return false

	case strings.HasPrefix(cmd, "/agent use "):
		name := strings.TrimSpace(strings.TrimPrefix(input, "/agent use "))
		if err := switchAgentProfile(state, name); err != nil {
			printError("%v", err)
		} else {
			printSuccess("宸插垏鎹㈠埌 Agent: %s", name)
		}
		return false

	case cmd == "/providers":
		fmt.Println()
		showAvailableProviders()
		return false

	case strings.HasPrefix(cmd, "/models"):
		parts := strings.Split(input, " ")
		provider := state.cfg.LLM.Provider
		if len(parts) >= 2 {
			provider = parts[1]
		}
		fmt.Println()
		showModelsForProvider(provider)
		return false

	case strings.HasPrefix(cmd, "/set"):
		fmt.Println()
		handleSetCommand(state, input)
		return false

	default:
		fmt.Printf("%s鏈煡鍛戒护: %s\n", ui.Error.Sprint(""), input)
		fmt.Printf("杈撳叆 %s/help%s 鏌ョ湅鍙敤鍛戒护\n", ui.Cyan.Sprint(""), ui.Reset.Sprint(""))
		return false
	}
}

func handleSetCommand(state *RuntimeState, input string) {
	parts := strings.SplitN(input, " ", 3)
	if len(parts) < 3 {
		fmt.Println("鐢ㄦ硶: /set <provider|model|apikey|temp> <鍊?")
		fmt.Println()
		fmt.Println("绀轰緥:")
		fmt.Println("  /set provider anthropic")
		fmt.Println("  /set model gpt-4o")
		fmt.Println("  /set apikey sk-...")
		fmt.Println("  /set temp 0.7")
		return
	}

	key := strings.ToLower(parts[1])
	value := strings.TrimSpace(parts[2])

	switch key {
	case "provider":
		state.cfg.LLM.Provider = value
		if err := state.llmClient.SwitchProvider(value); err != nil {
			printError("Failed to switch provider: %v", err)
		} else {
			state.cfg.Save(state.configPath)
			printSuccess("Provider set to: %s", value)
		}

	case "model":
		state.cfg.LLM.Model = value
		if err := state.llmClient.SwitchModel(value); err != nil {
			printError("Failed to switch model: %v", err)
		} else {
			state.cfg.Save(state.configPath)
			printSuccess("Model set to: %s", value)
		}

	case "apikey":
		state.cfg.LLM.APIKey = value
		if err := state.llmClient.SetAPIKey(value); err != nil {
			printError("Failed to set API key: %v", err)
		} else {
			state.cfg.Save(state.configPath)
			printSuccess("API key updated!")
		}

	case "temp", "temperature":
		if temp, err := strconv.ParseFloat(value, 64); err == nil {
			state.cfg.LLM.Temperature = temp
			state.llmClient.SetTemperature(temp)
			state.cfg.Save(state.configPath)
			printSuccess("Temperature set to: %.1f", temp)
		} else {
			printError("Invalid temperature value (0.0-2.0)")
		}

	default:
		printError("鏈煡璁剧疆: %s", key)
		fmt.Println("鍙敤璁剧疆: provider, model, apikey, temp")
	}
}

func showAvailableProviders() {
	fmt.Printf("%s\n\n", ui.Bold.Sprint("鍙敤鎻愪緵鍟?"))
	fmt.Printf("%s  openai%s      - %sOpenAI%s (GPT-4, GPT-3.5)\n", ui.Cyan.Sprint(""), ui.Reset.Sprint(""), ui.Green.Sprint(""), ui.Reset.Sprint(""))
	fmt.Printf("%s  anthropic%s   - %sAnthropic%s (Claude)\n", ui.Cyan.Sprint(""), ui.Reset.Sprint(""), ui.Green.Sprint(""), ui.Reset.Sprint(""))
	fmt.Printf("%s  qwen%s        - %s闃块噷閫氫箟鍗冮棶%s (鍥藉唴鍙敤)\n", ui.Cyan.Sprint(""), ui.Reset.Sprint(""), ui.Green.Sprint(""), ui.Reset.Sprint(""))
	fmt.Printf("%s  ollama%s      - %sOllama%s (鏈湴妯″瀷)\n", ui.Cyan.Sprint(""), ui.Reset.Sprint(""), ui.Green.Sprint(""), ui.Reset.Sprint(""))
	fmt.Printf("%s  compatible%s  - %sOpenAI 鍏煎 API%s\n", ui.Cyan.Sprint(""), ui.Reset.Sprint(""), ui.Green.Sprint(""), ui.Reset.Sprint(""))
	fmt.Println()
}

func showModelsForProvider(provider string) {
	models := map[string][]string{
		"openai": {
			"gpt-4o", "gpt-4o-mini", "gpt-4-turbo", "gpt-4", "gpt-3.5-turbo",
		},
		"anthropic": {
			"claude-opus-4-5", "claude-sonnet-4-7", "claude-haiku-3-5",
		},
		"qwen": {
			"qwen-plus", "qwen-turbo", "qwen-max", "qwen2.5-72b-instruct",
			"qwen2.5-14b-instruct", "qwq-32b-preview", "qwen-coder-plus",
		},
		"ollama": {
			"llama3.2", "llama3.1", "codellama", "mistral", "qwen2.5",
		},
		"compatible": {
			"(use your provider's models)",
		},
	}

	provider = strings.ToLower(provider)
	if modelList, ok := models[provider]; ok {
		fmt.Printf("%s\n\n", ui.Bold.Sprint(provider+" 鐨勬ā鍨?"))
		for _, m := range modelList {
			fmt.Printf("  %s- %s\n", ui.Cyan.Sprint(""), m)
		}
	} else {
		fmt.Printf("%s鏈煡鎻愪緵鍟? %s\n", ui.Error.Sprint(""), provider)
		showAvailableProviders()
	}
}
