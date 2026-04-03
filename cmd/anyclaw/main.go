package main

import (
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
	"github.com/anyclaw/anyclaw/pkg/consoleio"
	"github.com/anyclaw/anyclaw/pkg/llm"
	"github.com/anyclaw/anyclaw/pkg/routing"
	appRuntime "github.com/anyclaw/anyclaw/pkg/runtime"
	"github.com/anyclaw/anyclaw/pkg/setup"
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
	reader     *consoleio.Reader
	configPath string
	workDir    string
	workingDir string
}

func main() {
	configureConsoleUTF8()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Args[1:]); err != nil {
		printError("%v", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) > 0 {
		switch normalizeRootCommand(args[0]) {
		case "skill":
			runSkillCommand()
			return nil
		case "skillhub":
			runSkillhubCommand()
			return nil
		case "clawhub":
			runClawhubCommand()
			return nil
		case "clihub":
			return runCLIHubCommand(args[1:])
		case "shell":
			return runShellCommand(args[1:])
		case "gateway":
			return runGatewayCommand(ctx, args[1:])
		case "daemon":
			return runGatewayCommand(ctx, append([]string{"daemon"}, args[1:]...))
		case "config":
			return runConfigCommand(args[1:])
		case "plugin":
			return runPluginCommand(args[1:])
		case "channels":
			return runChannelsCommand(args[1:])
		case "doctor":
			return runDoctorCommand(args[1:])
		case "claw":
			return runClawCommand(args[1:])
		case "app":
			return runAppCommand(args[1:])
		case "cron":
			return runCronCommand(ctx, args[1:])
		case "models":
			return runModelsCommand(args[1:])
		case "status":
			return runStatusCommand(args[1:])
		case "health":
			return runHealthCommand(args[1:])
		case "sessions":
			return runSessionsCommand(args[1:])
		case "approvals":
			return runApprovalsCommand(args[1:])
		case "onboard":
			return runOnboardCommand(args[1:])
		case "pi":
			return runPiCommand(ctx, args[1:])
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

func normalizeRootCommand(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "skills":
		return "skill"
	case "plugins":
		return "plugin"
	case "agents":
		return "agent"
	case "apps":
		return "app"
	case "channel":
		return "channels"
	case "session":
		return "sessions"
	case "approval":
		return "approvals"
	case "model":
		return "models"
	case "setup":
		return "onboard"
	case "daemon":
		return "daemon"
	default:
		return strings.ToLower(strings.TrimSpace(name))
	}
}

func runRootCommand(ctx context.Context, args []string) error {
	return runRootCommandStable(ctx, args)
}

func runDoctorCommand(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	repair := fs.Bool("repair", false, "create missing directories while checking")
	connectivity := fs.Bool("connectivity", true, "run a live model connectivity check")
	if err := fs.Parse(args); err != nil {
		return err
	}

	printBanner()
	fmt.Printf("%s\n", ui.Bold.Sprint("AnyClaw doctor"))
	fmt.Println(ui.Dim.Sprint(strings.Repeat("-", 50)))
	report, _, err := setup.RunDoctor(context.Background(), *configPath, setup.DoctorOptions{
		CheckConnectivity: *connectivity,
		CreateMissingDirs: *repair,
	})
	printDoctorReport(report)
	if report != nil {
		printInfo("Summary: %d error(s), %d warning(s)", report.ErrorCount(), report.WarningCount())
	}
	return err
}

func runOnboardCommand(args []string) error {
	fs := flag.NewFlagSet("onboard", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	nonInteractive := fs.Bool("non-interactive", false, "write defaults without prompting")
	connectivity := fs.Bool("connectivity", true, "run a live model connectivity check after saving")
	if err := fs.Parse(args); err != nil {
		return err
	}

	printBanner()
	result, err := setup.RunOnboarding(context.Background(), *configPath, setup.OnboardOptions{
		Interactive:       !*nonInteractive && terminalInteractive(),
		CheckConnectivity: *connectivity,
		Stdin:             os.Stdin,
		Stdout:            os.Stdout,
	})
	if result != nil {
		printDoctorReport(result.Report)
		printSuccess("Onboarding wrote: %s", appRuntime.ResolveConfigPath(*configPath))
	}
	return err
}

func printBanner() {
	ui.Banner(version)
}

func printError(format string, args ...any) {
	fmt.Printf("%s\n", ui.Error.Sprint("x Error: ")+fmt.Sprintf(format, args...))
}

func printSuccess(format string, args ...any) {
	fmt.Printf("%s\n", ui.Success.Sprint("+ ")+fmt.Sprintf(format, args...))
}

func printInfo(format string, args ...any) {
	fmt.Printf("%s\n", ui.Info.Sprint("i ")+fmt.Sprintf(format, args...))
}

func bootProgress(ev appRuntime.BootEvent) {
	clear := ""
	if terminalInteractive() {
		clear = "\r" + strings.Repeat(" ", 512) + "\r"
	}
	switch ev.Status {
	case "start":
		fmt.Printf("%s  %s %-12s %s", clear, ui.Cyan.Sprint("..."), ui.Dim.Sprint(string(ev.Phase)), ev.Message)
	case "ok":
		fmt.Printf("%s  %s %-12s %s %s\n", clear, ui.Green.Sprint("OK"), ui.Cyan.Sprint(string(ev.Phase)), ev.Message, ui.Dim.Sprint(ev.Dur.Round(time.Millisecond)))
	case "warn":
		fmt.Printf("%s  %s %-12s %s %s\n", clear, ui.Yellow.Sprint("WARN"), ui.Cyan.Sprint(string(ev.Phase)), ev.Message, ui.Dim.Sprint(ev.Dur.Round(time.Millisecond)))
	case "skip":
		fmt.Printf("%s  %s %-12s %s %s\n", clear, ui.Dim.Sprint("SKIP"), ui.Cyan.Sprint(string(ev.Phase)), ev.Message, ui.Dim.Sprint(ev.Dur.Round(time.Millisecond)))
	case "fail":
		errMsg := ""
		if ev.Err != nil {
			errMsg = ": " + ev.Err.Error()
		}
		fmt.Printf("%s  %s %-12s %s%s %s\n", clear, ui.Red.Sprint("FAIL"), ui.Cyan.Sprint(string(ev.Phase)), ev.Message, errMsg, ui.Dim.Sprint(ev.Dur.Round(time.Millisecond)))
	}
}

func runSetupWizard(cfg *config.Config) {
	runSetupWizardStable(cfg)
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
	runInteractiveStable(ctx, state)
}

func rebindBuiltins(state *RuntimeState) {
	rebindBuiltinsStable(state)
}

func applyLLMRoute(state *RuntimeState, input string) string {
	return applyLLMRouteStable(state, input)
}

func showAgentProfiles(state *RuntimeState) {
	showAgentProfilesStable(state)
}

func showAuditLog(state *RuntimeState) {
	showAuditLogStable(state)
}

func switchAgentProfile(state *RuntimeState, name string) error {
	return switchAgentProfileStable(state, name)
}

func handleCommand(ctx context.Context, state *RuntimeState, input string) bool {
	return handleCommandStable(ctx, state, input)
}

func handleSetCommand(state *RuntimeState, input string) {
	handleSetCommandStable(state, input)
}

func showAvailableProviders() {
	showAvailableProvidersStable()
}

func showModelsForProvider(provider string) {
	showModelsForProviderStable(provider)
}

func runRootCommandStable(ctx context.Context, args []string) error {
	rootFS := flag.NewFlagSet("anyclaw", flag.ContinueOnError)
	rootFS.SetOutput(os.Stdout)

	versionFlag := rootFS.Bool("version", false, "show version")
	providersFlag := rootFS.Bool("providers", false, "show available providers")
	setProviderFlag := rootFS.String("provider", "", "set LLM provider")
	setModelFlag := rootFS.String("model", "", "set LLM model")
	setAPIKeyFlag := rootFS.String("api-key", "", "set API key")
	interactiveFlag := rootFS.Bool("i", false, "interactive mode")
	setupFlag := rootFS.Bool("setup", false, "run setup wizard")
	configPathFlag := rootFS.String("config", "anyclaw.json", "path to config file")

	if err := rootFS.Parse(args); err != nil {
		return err
	}

	printBanner()

	if *versionFlag {
		fmt.Printf("%sAnyClaw version %s%s\n", ui.Cyan.Sprint(""), version, ui.Reset.Sprint(""))
		fmt.Printf("%sFile-first AI agent workspace%s\n", ui.Bold.Sprint(""), ui.Reset.Sprint(""))
		return nil
	}

	if *providersFlag {
		showAvailableProvidersStable()
		return nil
	}

	if *setupFlag {
		result, err := setup.RunOnboarding(ctx, *configPathFlag, setup.OnboardOptions{
			Interactive:       terminalInteractive(),
			CheckConnectivity: true,
			Stdin:             os.Stdin,
			Stdout:            os.Stdout,
		})
		if result != nil {
			printDoctorReport(result.Report)
		}
		return err
	}

	if err := ensureConfigOnboarded(ctx, *configPathFlag, true); err != nil {
		return err
	}

	cfg, err := config.Load(*configPathFlag)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if *setProviderFlag != "" || *setModelFlag != "" || *setAPIKeyFlag != "" {
		if *setProviderFlag != "" {
			cfg.LLM.Provider = *setProviderFlag
		}
		if *setModelFlag != "" {
			cfg.LLM.Model = *setModelFlag
		}
		if *setAPIKeyFlag != "" {
			cfg.LLM.APIKey = *setAPIKeyFlag
		}
		if err := cfg.Save(*configPathFlag); err != nil {
			return err
		}
		printSuccess("Config updated: %s", *configPathFlag)
		return nil
	}

	app, err := appRuntime.Bootstrap(appRuntime.BootstrapOptions{
		ConfigPath: *configPathFlag,
		Progress:   bootProgress,
	})
	if err != nil {
		return fmt.Errorf("failed to bootstrap runtime: %w", err)
	}

	state := &RuntimeState{
		llmClient:  app.LLM,
		cfg:        app.Config,
		agent:      app.Agent,
		skills:     app.Skills,
		audit:      app.Audit,
		reader:     consoleio.NewReader(os.Stdin),
		configPath: *configPathFlag,
		workDir:    app.WorkDir,
		workingDir: app.WorkingDir,
	}
	rebindBuiltinsStable(state)

	fmt.Println(ui.Dim.Sprint(strings.Repeat("-", 50)))

	messageText := strings.TrimSpace(strings.Join(rootFS.Args(), " "))
	if messageText != "" && !*interactiveFlag {
		routeLabel := applyLLMRouteStable(state, messageText)
		responseText, err := state.agent.Run(ctx, messageText)
		if err != nil {
			return err
		}
		if routeLabel != "" {
			fmt.Printf("%s%s%s\n", ui.Dim.Sprint(""), routeLabel, ui.Reset.Sprint(""))
		}
		fmt.Printf("%s\n", ui.Bold.Sprint(responseText))
		return nil
	}

	runInteractiveStable(ctx, state)
	return nil
}

func runSetupWizardStable(cfg *config.Config) {
	fmt.Println(ui.Dim.Sprint(strings.Repeat("-", 50)))
	fmt.Printf("%s\n\n", ui.Bold.Sprint("Setup Wizard"))

	wizardReader := consoleio.NewReader(os.Stdin)

	fmt.Printf("%s\n\n", ui.Bold.Sprint("Step 1/5: Choose provider"))
	showAvailableProvidersStable()
	fmt.Printf("%s\nProvider > %s", ui.Cyan.Sprint(""), ui.Reset.Sprint(""))

	selectedProvider, _ := wizardReader.ReadString('\n')
	selectedProvider = strings.TrimSpace(strings.ToLower(selectedProvider))
	if selectedProvider == "" {
		selectedProvider = "qwen"
	}
	if selectedProvider == "ali" || selectedProvider == "alibaba" {
		selectedProvider = "qwen"
	}
	cfg.LLM.Provider = selectedProvider

	fmt.Printf("\n%s\n\n", ui.Bold.Sprint("Step 2/5: Choose model"))
	showModelsForProviderStable(selectedProvider)
	fmt.Printf("%s\nModel > %s", ui.Cyan.Sprint(""), ui.Reset.Sprint(""))

	selectedModel, _ := wizardReader.ReadString('\n')
	selectedModel = strings.TrimSpace(strings.ToLower(selectedModel))
	if selectedModel == "" {
		selectedModel = getDefaultModel(selectedProvider)
	}
	cfg.LLM.Model = selectedModel

	fmt.Printf("\n%s\n", ui.Bold.Sprint("Step 3/5: API key"))
	fmt.Printf("%s\n", getProviderHint(selectedProvider))
	fmt.Printf("%sAPI key: %s", ui.Cyan.Sprint(""), ui.Reset.Sprint(""))

	enteredAPIKey, _ := wizardReader.ReadString('\n')
	cfg.LLM.APIKey = strings.TrimSpace(enteredAPIKey)

	fmt.Printf("\n%s", ui.Bold.Sprint("Step 4/5: Proxy"))
	fmt.Printf("%s (optional, press Enter to skip)%s", ui.Yellow.Sprint(""), ui.Reset.Sprint(""))
	fmt.Printf("%s\n> %s", ui.Green.Sprint(""), ui.Reset.Sprint(""))

	enteredProxy, _ := wizardReader.ReadString('\n')
	cfg.LLM.Proxy = strings.TrimSpace(enteredProxy)

	fmt.Printf("\n%s", ui.Bold.Sprint("Step 5/5: Agent name"))
	fmt.Printf("%s (default: AnyClaw)%s", ui.Yellow.Sprint(""), ui.Reset.Sprint(""))
	fmt.Printf("%s\n> %s", ui.Green.Sprint(""), ui.Reset.Sprint(""))

	agentName, _ := wizardReader.ReadString('\n')
	agentName = strings.TrimSpace(agentName)
	if agentName != "" {
		cfg.Agent.Name = agentName
	}

	fmt.Println()
	printSuccess("Config saved")
	fmt.Println(ui.Dim.Sprint(strings.Repeat("-", 50)))
}

func runInteractiveStable(ctx context.Context, state *RuntimeState) {
	fmt.Println()
	fmt.Println(ui.Dim.Sprint(strings.Repeat("-", 50)))
	fmt.Printf("%s\n", ui.Bold.Sprint("Interactive mode"))
	fmt.Printf("  Agent: %s\n", state.cfg.Agent.Name)
	fmt.Printf("  LLM:   %s / %s\n", state.cfg.LLM.Provider, state.cfg.LLM.Model)
	if state.workingDir != "" {
		fmt.Printf("  Dir:   %s\n", state.workingDir)
	}
	fmt.Println("  Type /help to list commands.")
	fmt.Println(ui.Dim.Sprint(strings.Repeat("-", 50)))
	printInteractiveHelp()
	fmt.Println(ui.Dim.Sprint(strings.Repeat("-", 50)))
	fmt.Println()

	for {
		line, err := readInteractiveLineStable(state)
		if err != nil {
			break
		}
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "/") {
			if handleCommandStable(ctx, state, line) {
				break
			}
			continue
		}

		fmt.Println()
		routeLabel := applyLLMRouteStable(state, line)
		answer, err := state.agent.Run(ctx, line)
		if err != nil {
			printError("%v", err)
			continue
		}
		if routeLabel != "" {
			fmt.Printf("%s%s%s\n", ui.Dim.Sprint(""), routeLabel, ui.Reset.Sprint(""))
		}
		fmt.Printf("%s\n\n", ui.Bold.Sprint(answer))
	}
}

func readInteractiveLineStable(state *RuntimeState) (string, error) {
	fmt.Print("> ")
	line, err := state.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func rebindBuiltinsStable(state *RuntimeState) {
	if state == nil || state.agent == nil {
		return
	}

	toolRegistry := tools.NewRegistry()
	sandboxMgr := tools.NewSandboxManager(state.cfg.Sandbox, state.workingDir)
	tools.RegisterBuiltins(toolRegistry, tools.BuiltinOptions{
		WorkingDir:            state.workingDir,
		PermissionLevel:       state.cfg.Agent.PermissionLevel,
		ExecutionMode:         state.cfg.Sandbox.ExecutionMode,
		DangerousPatterns:     state.cfg.Security.DangerousCommandPatterns,
		ProtectedPaths:        state.cfg.Security.ProtectedPaths,
		CommandTimeoutSeconds: state.cfg.Security.CommandTimeoutSeconds,
		AuditLogger:           state.audit,
		Sandbox:               sandboxMgr,
		ConfirmDangerousCommand: func(command string) bool {
			if !state.cfg.Agent.RequireConfirmationForDangerous {
				return true
			}
			fmt.Printf("%sDangerous command detected:%s %s\n", ui.Warning.Sprint(""), ui.Reset.Sprint(""), command)
			fmt.Printf("%sContinue? (y/N): ", ui.Yellow.Sprint(""))
			confirmText, _ := state.reader.ReadString('\n')
			return strings.EqualFold(strings.TrimSpace(confirmText), "y")
		},
	})
	state.skills.RegisterTools(toolRegistry, skills.ExecutionOptions{
		AllowExec:          state.cfg.Plugins.AllowExec,
		ExecTimeoutSeconds: state.cfg.Plugins.ExecTimeoutSeconds,
	})
	state.agent.SetTools(toolRegistry)
}

func applyLLMRouteStable(state *RuntimeState, input string) string {
	routeDecision := routing.DecideLLM(state.cfg.LLM, input)
	providerChanged := strings.TrimSpace(routeDecision.Provider) != "" && routeDecision.Provider != state.cfg.LLM.Provider
	modelChanged := strings.TrimSpace(routeDecision.Model) != "" && routeDecision.Model != state.cfg.LLM.Model

	if providerChanged {
		if err := state.llmClient.SwitchProvider(routeDecision.Provider); err == nil {
			state.cfg.LLM.Provider = routeDecision.Provider
		}
	}
	if modelChanged {
		if err := state.llmClient.SwitchModel(routeDecision.Model); err == nil {
			state.cfg.LLM.Model = routeDecision.Model
		}
	}
	if providerChanged || modelChanged {
		return fmt.Sprintf("[route: %s -> %s/%s]", routeDecision.Reason, state.cfg.LLM.Provider, state.cfg.LLM.Model)
	}
	return ""
}

func showAgentProfilesStable(state *RuntimeState) {
	fmt.Println()
	fmt.Printf("%s\n", ui.Bold.Sprint("Agent profiles"))
	fmt.Printf("  Active agent: %s\n", state.cfg.Agent.Name)
	fmt.Printf("  Permission:   %s\n", state.cfg.Agent.PermissionLevel)
	if len(state.cfg.Agent.Profiles) == 0 {
		fmt.Println("  (no saved profiles)")
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

func showAuditLogStable(state *RuntimeState) {
	entries, err := state.audit.Tail(10)
	if err != nil {
		printError("failed to read audit log: %v", err)
		return
	}

	fmt.Println()
	fmt.Printf("%s\n", ui.Bold.Sprint("Recent audit log"))
	if len(entries) == 0 {
		fmt.Println("  (no events yet)")
		return
	}
	for _, event := range entries {
		line := fmt.Sprintf("  %s | %s | %s", event.Time, event.AgentName, event.Action)
		if event.Error != "" {
			line += " | error=" + event.Error
		}
		fmt.Println(line)
	}
}

func switchAgentProfileStable(state *RuntimeState, name string) error {
	if !state.cfg.ApplyAgentProfile(name) {
		return fmt.Errorf("agent not found: %s", name)
	}
	if err := state.cfg.Save(state.configPath); err != nil {
		return err
	}

	reloadedApp, err := appRuntime.Bootstrap(appRuntime.BootstrapOptions{
		ConfigPath: state.configPath,
		Progress: func(ev appRuntime.BootEvent) {
			if ev.Status == "fail" {
				printError("%s: %v", ev.Message, ev.Err)
			}
		},
	})
	if err != nil {
		return fmt.Errorf("failed to reload agent: %w", err)
	}

	historyCopy := state.agent.GetHistory()
	state.agent = reloadedApp.Agent
	state.agent.SetHistory(historyCopy)
	state.llmClient = reloadedApp.LLM
	state.audit = reloadedApp.Audit
	state.workDir = reloadedApp.WorkDir
	state.workingDir = reloadedApp.WorkingDir
	state.cfg = reloadedApp.Config
	rebindBuiltinsStable(state)
	return nil
}

func handleCommandStable(ctx context.Context, state *RuntimeState, input string) bool {
	commandText := strings.ToLower(strings.TrimSpace(input))

	switch {
	case commandText == "/exit", commandText == "/quit", commandText == "/q":
		fmt.Println()
		printSuccess("Bye")
		return true

	case commandText == "/help", commandText == "/?":
		fmt.Println()
		printInteractiveHelp()
		return false

	case commandText == "/clear":
		state.agent.ClearHistory()
		printSuccess("Chat history cleared")
		return false

	case commandText == "/memory":
		mem, _ := state.agent.ShowMemory()
		fmt.Println()
		fmt.Println(ui.Dim.Sprint(strings.Repeat("-", 40)))
		fmt.Println(mem)
		fmt.Println(ui.Dim.Sprint(strings.Repeat("-", 40)))
		return false

	case commandText == "/skills":
		loadedSkills := state.agent.ListSkills()
		fmt.Println()
		fmt.Printf("%s\n", ui.Bold.Sprint("Skills"))
		if len(loadedSkills) == 0 {
			fmt.Println(ui.Yellow.Sprint("  (no skills loaded)"))
			return false
		}
		for _, skill := range loadedSkills {
			fmt.Printf("  - %s: %s\n", skill.Name, skill.Description)
		}
		return false

	case commandText == "/tools":
		registeredTools := state.agent.ListTools()
		fmt.Println()
		fmt.Printf("%s\n", ui.Bold.Sprint("Tools"))
		for _, tool := range registeredTools {
			fmt.Printf("  - %s: %s\n", tool.Name, tool.Description)
		}
		return false

	case commandText == "/provider":
		fmt.Println()
		fmt.Printf("Provider:   %s\n", state.cfg.LLM.Provider)
		fmt.Printf("Model:      %s\n", state.cfg.LLM.Model)
		fmt.Printf("Temp:       %.1f\n", state.cfg.LLM.Temperature)
		fmt.Printf("Permission: %s\n", state.cfg.Agent.PermissionLevel)
		return false

	case commandText == "/agents":
		showAgentProfilesStable(state)
		return false

	case commandText == "/audit":
		showAuditLogStable(state)
		return false

	case strings.HasPrefix(commandText, "/agent use "):
		targetName := strings.TrimSpace(strings.TrimPrefix(input, "/agent use "))
		if err := switchAgentProfileStable(state, targetName); err != nil {
			printError("%v", err)
		} else {
			printSuccess("Switched to agent: %s", targetName)
		}
		return false

	case commandText == "/providers":
		fmt.Println()
		showAvailableProvidersStable()
		return false

	case strings.HasPrefix(commandText, "/models"):
		parts := strings.Fields(input)
		providerName := state.cfg.LLM.Provider
		if len(parts) >= 2 {
			providerName = parts[1]
		}
		fmt.Println()
		showModelsForProviderStable(providerName)
		return false

	case strings.HasPrefix(commandText, "/set"):
		fmt.Println()
		handleSetCommandStable(state, input)
		return false

	default:
		fmt.Printf("%sUnknown command:%s %s\n", ui.Error.Sprint(""), ui.Reset.Sprint(""), input)
		fmt.Printf("Type %s/help%s to see the available commands.\n", ui.Cyan.Sprint(""), ui.Reset.Sprint(""))
		return false
	}
}

func handleSetCommandStable(state *RuntimeState, input string) {
	parts := strings.SplitN(input, " ", 3)
	if len(parts) < 3 {
		fmt.Println("Usage: /set <provider|model|apikey|api-key|temp> <value>")
		fmt.Println()
		fmt.Println("Examples:")
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
		if err := state.llmClient.SwitchProvider(value); err != nil {
			printError("Failed to switch provider: %v", err)
			return
		}
		state.cfg.LLM.Provider = value
		saveRuntimeConfig(state)
		printSuccess("Provider set to: %s", value)

	case "model":
		if err := state.llmClient.SwitchModel(value); err != nil {
			printError("Failed to switch model: %v", err)
			return
		}
		state.cfg.LLM.Model = value
		saveRuntimeConfig(state)
		printSuccess("Model set to: %s", value)

	case "apikey", "api-key":
		if err := state.llmClient.SetAPIKey(value); err != nil {
			printError("Failed to set API key: %v", err)
			return
		}
		state.cfg.LLM.APIKey = value
		saveRuntimeConfig(state)
		printSuccess("API key updated!")

	case "temp", "temperature":
		tempValue, err := strconv.ParseFloat(value, 64)
		if err != nil {
			printError("Invalid temperature value (0.0-2.0)")
			return
		}
		if tempValue < 0 || tempValue > 2 {
			printError("Temperature must be between 0.0 and 2.0")
			return
		}
		state.cfg.LLM.Temperature = tempValue
		state.llmClient.SetTemperature(tempValue)
		saveRuntimeConfig(state)
		printSuccess("Temperature set to: %.1f", tempValue)

	default:
		printError("Unknown setting: %s", key)
		fmt.Println("Available settings: provider, model, apikey, api-key, temp")
	}
}

func showAvailableProvidersStable() {
	fmt.Printf("%s\n\n", ui.Bold.Sprint("Available providers"))
	fmt.Printf("  %sopenai%s      - OpenAI (GPT-4, GPT-3.5)\n", ui.Cyan.Sprint(""), ui.Reset.Sprint(""))
	fmt.Printf("  %santhropic%s   - Anthropic (Claude)\n", ui.Cyan.Sprint(""), ui.Reset.Sprint(""))
	fmt.Printf("  %sqwen%s        - Qwen (DashScope / Tongyi)\n", ui.Cyan.Sprint(""), ui.Reset.Sprint(""))
	fmt.Printf("  %sollama%s      - Ollama (local models)\n", ui.Cyan.Sprint(""), ui.Reset.Sprint(""))
	fmt.Printf("  %scompatible%s  - OpenAI-compatible API\n", ui.Cyan.Sprint(""), ui.Reset.Sprint(""))
	fmt.Println()
}

func showModelsForProviderStable(provider string) {
	modelsByProvider := map[string][]string{
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
			"(use your provider's model names)",
		},
	}

	name := strings.ToLower(provider)
	modelList, ok := modelsByProvider[name]
	if !ok {
		fmt.Printf("%sUnknown provider:%s %s\n", ui.Error.Sprint(""), ui.Reset.Sprint(""), provider)
		showAvailableProvidersStable()
		return
	}

	fmt.Printf("%s\n\n", ui.Bold.Sprint(name+" models"))
	for _, model := range modelList {
		fmt.Printf("  - %s\n", model)
	}
}

func printInteractiveHelp() {
	fmt.Printf("%s\n", ui.Bold.Sprint("Commands:"))
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
}

func saveRuntimeConfig(state *RuntimeState) {
	if err := state.cfg.Save(state.configPath); err != nil {
		printError("Failed to save config: %v", err)
	}
}
