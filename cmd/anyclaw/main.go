package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/anyclaw/anyclaw/pkg/agent"
	"github.com/anyclaw/anyclaw/pkg/agentstore"
	"github.com/anyclaw/anyclaw/pkg/audit"
	"github.com/anyclaw/anyclaw/pkg/chat"
	"github.com/anyclaw/anyclaw/pkg/config"
	"github.com/anyclaw/anyclaw/pkg/gateway"
	"github.com/anyclaw/anyclaw/pkg/llm"
	"github.com/anyclaw/anyclaw/pkg/routing"
	appRuntime "github.com/anyclaw/anyclaw/pkg/runtime"
	"github.com/anyclaw/anyclaw/pkg/skills"
	taskModule "github.com/anyclaw/anyclaw/pkg/task"
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
		return fmt.Errorf("配置加载失败: %w", err)
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
		return fmt.Errorf("启动失败: %w", err)
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

func runGatewayCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		printGatewayUsage()
		return nil
	}

	switch args[0] {
	case "run":
		return runGatewayServer(ctx, args[1:])
	case "daemon":
		return runGatewayDaemon(args[1:])
	case "status":
		return runGatewayStatus(args[1:])
	case "sessions":
		return runGatewaySessions(args[1:])
	case "events":
		return runGatewayEvents(args[1:])
	default:
		printGatewayUsage()
		return fmt.Errorf("unknown gateway command: %s", args[0])
	}
}

func runPluginCommand(args []string) error {
	if len(args) == 0 {
		printPluginUsage()
		return nil
	}
	if args[0] != "new" {
		printPluginUsage()
		return fmt.Errorf("unknown plugin command: %s", args[0])
	}
	fs := flag.NewFlagSet("plugin new", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	kind := fs.String("kind", "tool", "plugin kind: tool|ingress|channel")
	name := fs.String("name", "", "plugin name")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if strings.TrimSpace(*name) == "" {
		return fmt.Errorf("--name is required")
	}
	return scaffoldPlugin(*name, *kind)
}

func printPluginUsage() {
	fmt.Print(`AnyClaw plugin commands:

Usage:
  anyclaw plugin new --name my-plugin --kind tool
  anyclaw plugin new --name my-ingress --kind ingress
  anyclaw plugin new --name my-channel --kind channel
`)
}

func scaffoldPlugin(name string, kind string) error {
	pluginDir := filepath.Join("plugins", name)
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		return err
	}
	manifest := map[string]any{
		"name":            name,
		"version":         "1.0.0",
		"description":     "Scaffolded plugin",
		"kinds":           []string{kind},
		"enabled":         true,
		"entrypoint":      scriptNameForKind(kind),
		"exec_policy":     "manual-allow",
		"permissions":     []string{"tool:exec"},
		"timeout_seconds": 5,
		"signer":          "dev-local",
		"signature":       "sha256:replace-after-build",
	}
	switch kind {
	case "tool":
		manifest["tool"] = map[string]any{
			"name":        strings.ReplaceAll(name, "-", "_"),
			"description": "Example tool plugin",
			"input_schema": map[string]any{
				"type":       "object",
				"properties": map[string]any{"query": map[string]any{"type": "string"}},
				"required":   []string{"query"},
			},
		}
	case "ingress":
		manifest["ingress"] = map[string]any{
			"name":        name,
			"path":        "/ingress/plugins/" + name,
			"description": "Example ingress plugin",
		}
	case "channel":
		manifest["channel"] = map[string]any{
			"name":        name,
			"description": "Example channel plugin",
		}
		manifest["permissions"] = []string{"tool:exec", "net:out"}
	default:
		return fmt.Errorf("unsupported plugin kind: %s", kind)
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), data, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(pluginDir, scriptNameForKind(kind)), []byte(pluginScript(kind)), 0o644); err != nil {
		return err
	}
	printSuccess("Scaffolded %s plugin at %s", kind, pluginDir)
	printInfo("Next: implement the script, compute sha256, update plugin.json, and enable exec if needed")
	return nil
}

func scriptNameForKind(kind string) string {
	switch kind {
	case "tool":
		return "tool.py"
	case "ingress":
		return "ingress.py"
	case "channel":
		return "channel.py"
	default:
		return "plugin.py"
	}
}

func pluginScript(kind string) string {
	switch kind {
	case "tool":
		return "import json, os\ninput_data = json.loads(os.environ.get('ANYCLAW_PLUGIN_INPUT', '{}'))\nprint(f\"tool received: {input_data}\")\n"
	case "ingress":
		return "import json, os\ninput_data = json.loads(os.environ.get('ANYCLAW_PLUGIN_INPUT', '{}'))\nprint(json.dumps({'ok': True, 'received': input_data}))\n"
	case "channel":
		return "import json\nprint(json.dumps([{'source': 'example-user', 'message': 'hello from channel plugin'}]))\n"
	default:
		return "print('plugin scaffold')\n"
	}
}

func runGatewayServer(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("gateway run", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	host := fs.String("host", "", "gateway host")
	port := fs.Int("port", 0, "gateway port")
	if err := fs.Parse(args); err != nil {
		return err
	}

	app, err := appRuntime.Bootstrap(appRuntime.BootstrapOptions{
		ConfigPath: *configPath,
		Progress:   bootProgress,
	})
	if err != nil {
		return fmt.Errorf("gateway bootstrap failed: %w", err)
	}
	if *host != "" {
		app.Config.Gateway.Host = *host
	}
	if *port > 0 {
		app.Config.Gateway.Port = *port
	}

	server := gateway.New(app)
	fmt.Println(ui.Dim.Sprint(strings.Repeat("-", 50)))
	printSuccess("Gateway listening on %s", appRuntime.GatewayAddress(app.Config))
	printInfo("Health: %s/healthz", appRuntime.GatewayURL(app.Config))
	printInfo("Status: %s/status", appRuntime.GatewayURL(app.Config))
	return server.Run(ctx)
}

func runGatewayDaemon(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: anyclaw gateway daemon <start|stop>")
	}
	configPath := "anyclaw.json"
	app, err := appRuntime.Bootstrap(appRuntime.BootstrapOptions{
		ConfigPath: configPath,
		Progress:   bootProgress,
	})
	if err != nil {
		return fmt.Errorf("daemon bootstrap failed: %w", err)
	}
	app.ConfigPath = configPath

	switch args[0] {
	case "start":
		if err := gateway.StartDetached(app); err != nil {
			return err
		}
		printSuccess("Gateway daemon started")
		return nil
	case "stop":
		if err := gateway.StopDetached(app); err != nil {
			return err
		}
		printSuccess("Gateway daemon stopped")
		return nil
	default:
		return fmt.Errorf("unknown daemon command: %s", args[0])
	}
}

func runGatewayStatus(args []string) error {
	fs := flag.NewFlagSet("gateway status", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	status, err := gateway.Probe(ctx, appRuntime.GatewayURL(cfg))
	if err != nil {
		return fmt.Errorf("gateway not reachable at %s: %w", appRuntime.GatewayURL(cfg), err)
	}

	printSuccess("Gateway is %s", status.Status)
	fmt.Printf("%sAddress: %s\n", ui.Cyan.Sprint(""), status.Address)
	fmt.Printf("%sProvider: %s\n", ui.Cyan.Sprint(""), status.Provider)
	fmt.Printf("%sModel: %s\n", ui.Cyan.Sprint(""), status.Model)
	fmt.Printf("%sSessions: %d\n", ui.Cyan.Sprint(""), status.Sessions)
	fmt.Printf("%sEvents: %d\n", ui.Cyan.Sprint(""), status.Events)
	fmt.Printf("%sTools: %d\n", ui.Cyan.Sprint(""), status.Tools)
	fmt.Printf("%sSkills: %d\n", ui.Cyan.Sprint(""), status.Skills)
	return nil
}

func runGatewaySessions(args []string) error {
	fs := flag.NewFlagSet("gateway sessions", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, appRuntime.GatewayURL(cfg)+"/sessions", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gateway returned %s", resp.Status)
	}

	var sessions []struct {
		ID           string `json:"id"`
		Title        string `json:"title"`
		MessageCount int    `json:"message_count"`
		UpdatedAt    string `json:"updated_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&sessions); err != nil {
		return err
	}

	if len(sessions) == 0 {
		printInfo("No gateway sessions yet")
		return nil
	}

	printSuccess("Found %d gateway session(s)", len(sessions))
	for _, session := range sessions {
		fmt.Printf("%s%s%s\n", ui.Bold.Sprint(""), session.Title, ui.Reset.Sprint(""))
		fmt.Printf("  id=%s messages=%d updated=%s\n", session.ID, session.MessageCount, session.UpdatedAt)
	}
	return nil
}

func runGatewayEvents(args []string) error {
	fs := flag.NewFlagSet("gateway events", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	configPath := fs.String("config", "anyclaw.json", "path to config file")
	stream := fs.Bool("stream", false, "stream events over SSE")
	replay := fs.Int("replay", 10, "number of recent events to replay for stream mode")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	baseURL := appRuntime.GatewayURL(cfg)
	if *stream {
		url := fmt.Sprintf("%s/events/stream?replay=%d", baseURL, *replay)
		printInfo("Streaming events from %s", url)
		resp, err := http.Get(url)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("gateway returned %s", resp.Status)
		}
		_, err = io.Copy(os.Stdout, resp.Body)
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/events", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gateway returned %s", resp.Status)
	}

	var events []struct {
		ID        string `json:"id"`
		Type      string `json:"type"`
		SessionID string `json:"session_id"`
		Timestamp string `json:"timestamp"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		return err
	}

	if len(events) == 0 {
		printInfo("No gateway events yet")
		return nil
	}

	printSuccess("Found %d gateway event(s)", len(events))
	for _, event := range events {
		fmt.Printf("- %s session=%s at %s id=%s\n", event.Type, event.SessionID, event.Timestamp, event.ID)
	}
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

// ─── Agent Chat Command ───────────────────────────────────────────────────

func runAgentCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		printAgentUsage()
		return nil
	}

	switch args[0] {
	case "chat":
		return runAgentChat(ctx, args[1:])
	case "list":
		return runAgentList()
	case "use":
		return runAgentUse(args[1:])
	default:
		printAgentUsage()
		return fmt.Errorf("unknown agent command: %s", args[0])
	}
}

func printAgentUsage() {
	fmt.Print(`AnyClaw 智能体命令:

用法:
  anyclaw agent list                列出所有可用智能体
  anyclaw agent use <名称>          切换当前智能体
  anyclaw agent chat [智能体名称]   与智能体对话
  anyclaw agent chat --agent <名称> 与指定智能体对话

示例:
  anyclaw agent list
  anyclaw agent use Go编码专家
  anyclaw agent chat 健身教练
  anyclaw agent chat --agent 论文助手
`)
}

func runAgentList() error {
	cfg, err := config.Load("anyclaw.json")
	if err != nil {
		return fmt.Errorf("配置加载失败: %w", err)
	}

	fmt.Printf("%s\n\n", ui.Bold.Sprint("可用智能体:"))
	fmt.Printf("  %s当前: %s%s\n\n", ui.Dim.Sprint(""), cfg.Agent.Name, ui.Reset.Sprint(""))

	if len(cfg.Agent.Profiles) == 0 {
		fmt.Println("  (未配置智能体，请在 anyclaw.json 中添加 agent.profiles)")
		return nil
	}

	for _, p := range cfg.Agent.Profiles {
		status := ui.Dim.Sprint("○ 禁用")
		if p.IsEnabled() {
			status = ui.Green.Sprint("● 启用")
		}
		fmt.Printf("  %s %s\n", status, ui.Bold.Sprint(p.Name))
		if p.Description != "" {
			fmt.Printf("     %s\n", ui.Dim.Sprint(p.Description))
		}
		if p.Domain != "" {
			fmt.Printf("     领域: %s", p.Domain)
		}
		if len(p.Expertise) > 0 {
			fmt.Printf(" | 擅长: %s", strings.Join(p.Expertise, "、"))
		}
		if p.Domain != "" || len(p.Expertise) > 0 {
			fmt.Println()
		}
		fmt.Println()
	}
	return nil
}

func runAgentUse(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("用法: anyclaw agent use <智能体名称>")
	}
	name := strings.Join(args, " ")

	cfg, err := config.Load("anyclaw.json")
	if err != nil {
		return err
	}

	if !cfg.ApplyAgentProfile(name) {
		// List available names for help
		fmt.Fprintf(os.Stderr, "智能体不存在: %s\n\n可用智能体:\n", name)
		for _, p := range cfg.Agent.Profiles {
			fmt.Fprintf(os.Stderr, "  - %s\n", p.Name)
		}
		return fmt.Errorf("智能体不存在: %s", name)
	}

	if err := cfg.Save("anyclaw.json"); err != nil {
		return err
	}
	printSuccess("已切换到智能体: %s", name)
	return nil
}

func runAgentChat(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("agent chat", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	agentName := fs.String("agent", "", "智能体名称")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// If no --agent flag, use positional arg
	if *agentName == "" && fs.NArg() > 0 {
		*agentName = strings.Join(fs.Args(), " ")
	}

	app, err := appRuntime.Bootstrap(appRuntime.BootstrapOptions{
		ConfigPath: "anyclaw.json",
		Progress:   func(ev appRuntime.BootEvent) {},
	})
	if err != nil {
		return fmt.Errorf("启动失败: %w", err)
	}

	if app.Orchestrator == nil {
		return fmt.Errorf("编排器未启用，请在 anyclaw.json 中设置 orchestrator.enabled=true")
	}

	agents := app.Orchestrator.ListAgents()
	if len(agents) == 0 {
		return fmt.Errorf("没有可用的智能体")
	}

	// If no agent specified, show selection
	if *agentName == "" {
		fmt.Printf("%s\n\n", ui.Bold.Sprint("选择智能体:"))
		for i, a := range agents {
			domain := ""
			if a.Domain != "" {
				domain = " [" + a.Domain + "]"
			}
			fmt.Printf("  %s %s%s\n", ui.Cyan.Sprint(fmt.Sprintf("%d.", i+1)), ui.Bold.Sprint(a.Name), ui.Dim.Sprint(domain))
			fmt.Printf("     %s\n\n", a.Description)
		}
		fmt.Printf("%s输入智能体编号或名称: %s", ui.Green.Sprint(""), ui.Reset.Sprint(""))
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		// Try as number
		if idx, err := strconv.Atoi(input); err == nil && idx >= 1 && idx <= len(agents) {
			*agentName = agents[idx-1].Name
		} else {
			*agentName = input
		}
	}

	// Validate agent exists
	found := false
	for _, a := range agents {
		if a.Name == *agentName {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("智能体不存在: %s", *agentName)
	}

	chatMgr := chat.NewChatManager(app.Orchestrator)
	reader := bufio.NewReader(os.Stdin)
	var sessionID string

	fmt.Println()
	printSuccess("正在与 [%s] 对话 (输入 /exit 退出, /clear 清除历史)", *agentName)
	fmt.Println(ui.Dim.Sprint(strings.Repeat("─", 50)))
	fmt.Println()

	for {
		fmt.Printf("%s%s > %s", ui.Dim.Sprint("["), ui.Bold.Sprint(*agentName), ui.Reset.Sprint(""))
		input, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}
		if input == "/exit" || input == "/quit" {
			printSuccess("再见!")
			break
		}
		if input == "/clear" {
			sessionID = ""
			printSuccess("对话已清除")
			continue
		}

		resp, err := chatMgr.Chat(ctx, chat.ChatRequest{
			AgentName: *agentName,
			SessionID: sessionID,
			Message:   input,
		})
		if err != nil {
			printError("%v", err)
			continue
		}
		sessionID = resp.SessionID
		fmt.Printf("\n%s\n\n", ui.Bold.Sprint(resp.Message.Content))
	}
	return nil
}

// ─── Store Command ────────────────────────────────────────────────────────

func runStoreCommand(args []string) error {
	if len(args) == 0 {
		printStoreUsage()
		return nil
	}

	switch args[0] {
	case "list":
		return runStoreList(args[1:])
	case "search":
		return runStoreSearch(args[1:])
	case "info":
		return runStoreInfo(args[1:])
	case "install":
		return runStoreInstall(args[1:])
	case "uninstall":
		return runStoreUninstall(args[1:])
	default:
		printStoreUsage()
		return fmt.Errorf("unknown store command: %s", args[0])
	}
}

func printStoreUsage() {
	fmt.Print(`AnyClaw ????:

??:
  anyclaw store list [??]          ??????
  anyclaw store search <???>      ????/???
  anyclaw store info <ID>            ??????
  anyclaw store install <ID>         ???????
  anyclaw store uninstall <ID>       ???????
??:
  anyclaw store list
  anyclaw store list ????
  anyclaw store search ???
  anyclaw store info xiaohongshu-auto-poster
  anyclaw store install xiaohongshu-auto-poster
`)
}
func runStoreList(args []string) error {
	sm, err := agentstore.NewStoreManager(".anyclaw", "anyclaw.json")
	if err != nil {
		return err
	}

	filter := agentstore.StoreFilter{}
	if len(args) > 0 {
		filter.Category = args[0]
	}

	packages := sm.List(filter)
	if len(packages) == 0 {
		fmt.Println("?????????????")
		return nil
	}

	fmt.Println(ui.Bold.Sprint(fmt.Sprintf("???? (%d ??????):", len(packages))))
	fmt.Println()
	for _, pkg := range packages {
		icon := pkg.Icon
		if icon == "" {
			icon = "??"
		}
		installed := ""
		if sm.IsInstalled(pkg.ID) {
			installed = ui.Green.Sprint(" [???]")
		}
		fmt.Println("  " + icon + " " + ui.Bold.Sprint(pkg.DisplayName) + installed)
		fmt.Println("     " + ui.Dim.Sprint(pkg.Description))
		fmt.Println(fmt.Sprintf("     ??: %s | ??: %.1f (%d?) | ??: %d", pkg.Category, pkg.Rating, pkg.RatingCount, pkg.Downloads))
		fmt.Println("     " + ui.Dim.Sprint(fmt.Sprintf("ID: %s", pkg.ID)))
		fmt.Println()
	}
	return nil
}
func runStoreSearch(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("??: anyclaw store search <???>")
	}
	keyword := strings.Join(args, " ")

	sm, err := agentstore.NewStoreManager(".anyclaw", "anyclaw.json")
	if err != nil {
		return err
	}

	results := sm.Search(keyword)
	if len(results) == 0 {
		fmt.Println(fmt.Sprintf("?????? %q ???????", keyword))
		return nil
	}

	fmt.Println(ui.Bold.Sprint(fmt.Sprintf("???? (%d):", len(results))))
	fmt.Println()
	for _, pkg := range results {
		icon := pkg.Icon
		if icon == "" {
			icon = "??"
		}
		installed := ""
		if sm.IsInstalled(pkg.ID) {
			installed = ui.Green.Sprint(" [???]")
		}
		fmt.Println("  " + icon + " " + ui.Bold.Sprint(pkg.DisplayName) + installed)
		fmt.Println("     " + ui.Dim.Sprint(pkg.Description))
		fmt.Println("     " + ui.Dim.Sprint(fmt.Sprintf("??: anyclaw store install %s", pkg.ID)))
		fmt.Println()
	}
	return nil
}
func runStoreInfo(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("??: anyclaw store info <ID>")
	}

	sm, err := agentstore.NewStoreManager(".anyclaw", "anyclaw.json")
	if err != nil {
		return err
	}

	pkg, err := sm.Get(args[0])
	if err != nil {
		return err
	}

	icon := pkg.Icon
	if icon == "" {
		icon = "??"
	}

	fmt.Println(icon + " " + ui.Bold.Sprint(pkg.DisplayName))
	fmt.Println()
	fmt.Println(fmt.Sprintf("  ID:          %s", pkg.ID))
	fmt.Println(fmt.Sprintf("  ??:        %s", pkg.Description))
	fmt.Println(fmt.Sprintf("  ??:        %s", pkg.Author))
	fmt.Println(fmt.Sprintf("  ??:        %s", pkg.Version))
	fmt.Println(fmt.Sprintf("  ??:        %s", pkg.Category))
	fmt.Println(fmt.Sprintf("  ??:        %s", strings.Join(pkg.Tags, ", ")))
	fmt.Println(fmt.Sprintf("  ??:        %s", pkg.Domain))
	fmt.Println(fmt.Sprintf("  ??:        %s", strings.Join(pkg.Expertise, "?")))
	fmt.Println(fmt.Sprintf("  ??:        %s", strings.Join(pkg.Skills, ", ")))
	fmt.Println(fmt.Sprintf("  ??:        %s", pkg.Permission))
	fmt.Println(fmt.Sprintf("  ??:        %.1f (%d?)", pkg.Rating, pkg.RatingCount))
	fmt.Println(fmt.Sprintf("  ???:      %d", pkg.Downloads))
	fmt.Println(fmt.Sprintf("  ??:        %s", pkg.Tone))
	fmt.Println(fmt.Sprintf("  ??:        %s", pkg.Style))
	fmt.Println()
	fmt.Println("  ?????:")
	fmt.Println("    " + ui.Dim.Sprint(pkg.SystemPrompt))
	fmt.Println()
	if sm.IsInstalled(pkg.ID) {
		fmt.Println("  " + ui.Green.Sprint("???"))
	} else {
		fmt.Println("  " + ui.Dim.Sprint(fmt.Sprintf("??: anyclaw store install %s", pkg.ID)))
	}
	return nil
}
func runStoreInstall(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("??: anyclaw store install <ID>")
	}

	sm, err := agentstore.NewStoreManager(".anyclaw", "anyclaw.json")
	if err != nil {
		return err
	}

	id := args[0]
	if sm.IsInstalled(id) {
		printInfo("????????: %s", id)
		return nil
	}

	if err := sm.Install(id); err != nil {
		return err
	}

	pkg, _ := sm.Get(id)
	if pkg != nil {
		printSuccess("????????: %s (%s)", pkg.DisplayName, id)
	} else {
		printSuccess("????????: %s", id)
	}
	return nil
}
func runStoreUninstall(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("??: anyclaw store uninstall <ID>")
	}

	sm, err := agentstore.NewStoreManager(".anyclaw", "anyclaw.json")
	if err != nil {
		return err
	}

	if err := sm.Uninstall(args[0]); err != nil {
		return err
	}
	printSuccess("????????: %s", args[0])
	return nil
}
func runTaskCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		printTaskUsage()
		return nil
	}

	switch args[0] {
	case "run":
		return runTaskRun(ctx, args[1:])
	case "list":
		return runTaskList()
	default:
		printTaskUsage()
		return fmt.Errorf("unknown task command: %s", args[0])
	}
}

func printTaskUsage() {
	fmt.Print(`AnyClaw 任务命令:

用法:
  anyclaw task run <任务描述>              单智能体执行
  anyclaw task run --multi <任务描述>      多智能体协作执行
  anyclaw task run --agent <名称> <描述>   指定智能体执行
  anyclaw task list                       列出任务

示例:
  anyclaw task run "帮我写一个Go HTTP服务器"
  anyclaw task run --agent Go编码专家 "写一个排序算法"
  anyclaw task run --multi "帮我写一篇关于AI的英文论文"
`)
}

func runTaskRun(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("task run", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	multi := fs.Bool("multi", false, "多智能体协作模式")
	agentName := fs.String("agent", "", "指定智能体")
	if err := fs.Parse(args); err != nil {
		return err
	}

	input := strings.Join(fs.Args(), " ")
	if input == "" {
		return fmt.Errorf("请提供任务描述")
	}

	app, err := appRuntime.Bootstrap(appRuntime.BootstrapOptions{
		ConfigPath: "anyclaw.json",
		Progress:   func(ev appRuntime.BootEvent) {},
	})
	if err != nil {
		return fmt.Errorf("启动失败: %w", err)
	}

	if app.Orchestrator == nil {
		return fmt.Errorf("编排器未启用，请在 anyclaw.json 中设置 orchestrator.enabled=true")
	}

	taskMgr := taskModule.NewTaskManager(app.Orchestrator)

	mode := taskModule.ModeSingle
	if *multi {
		mode = taskModule.ModeMulti
	}

	req := taskModule.TaskRequest{
		Input:         input,
		Mode:          mode,
		SelectedAgent: *agentName,
	}

	if *multi {
		fmt.Printf("%s\n", ui.Bold.Sprint("多智能体协作模式"))
	} else if *agentName != "" {
		fmt.Printf("%s 智能体: %s\n", ui.Bold.Sprint("单智能体模式"), *agentName)
	} else {
		fmt.Printf("%s\n", ui.Bold.Sprint("单智能体模式"))
	}
	fmt.Printf("任务: %s\n\n", input)

	task, err := taskMgr.CreateTask(req)
	if err != nil {
		return err
	}

	fmt.Printf("%s 执行中...\n\n", ui.Cyan.Sprint("▸"))
	result, err := taskMgr.ExecuteTask(ctx, task.ID)
	if err != nil {
		// Still show partial results
		if result != nil && result.Output != "" {
			fmt.Printf("%s\n\n", result.Output)
		}
		printError("%v", err)
		return nil
	}

	fmt.Printf("%s\n", result.Output)
	fmt.Printf("\n%s 耗时: %s\n", ui.Dim.Sprint("⏱"), result.Duration)
	return nil
}

func runTaskList() error {
	sm, err := agentstore.NewStoreManager(".anyclaw", "anyclaw.json")
	if err != nil {
		_ = sm
	}
	fmt.Println("任务列表功能需要连接到运行中的 gateway")
	fmt.Println("请使用: anyclaw gateway run")
	return nil
}

func printGatewayUsage() {
	fmt.Print(`AnyClaw gateway commands:

Usage:
  anyclaw gateway run [--host 127.0.0.1] [--port 18789]
  anyclaw gateway daemon start
  anyclaw gateway daemon stop
  anyclaw gateway status
  anyclaw gateway sessions
  anyclaw gateway events
  anyclaw gateway events --stream
`)
}

func printBanner() {
	ui.Banner()
}

func printError(format string, args ...any) {
	fmt.Printf("%s\n", ui.Error.Sprint("✗ 错误: ")+fmt.Sprintf(format, args...))
}

func printSuccess(format string, args ...any) {
	fmt.Printf("%s\n", ui.Success.Sprint("✓ ")+fmt.Sprintf(format, args...))
}

func printInfo(format string, args ...any) {
	fmt.Printf("%s\n", ui.Warning.Sprint("ℹ ")+fmt.Sprintf(format, args...))
}

func bootProgress(ev appRuntime.BootEvent) {
	switch ev.Status {
	case "start":
		fmt.Printf("  %s %-12s %s", ui.Cyan.Sprint("▸"), ui.Dim.Sprint(string(ev.Phase)), ev.Message)
	case "ok":
		fmt.Printf("\r  %s %-12s %s %s\n", ui.Green.Sprint("✓"), ui.Cyan.Sprint(string(ev.Phase)), ev.Message, ui.Dim.Sprint(ev.Dur.Round(time.Millisecond)))
	case "warn":
		fmt.Printf("\r  %s %-12s %s %s\n", ui.Yellow.Sprint("⚠"), ui.Cyan.Sprint(string(ev.Phase)), ev.Message, ui.Dim.Sprint(ev.Dur.Round(time.Millisecond)))
	case "skip":
		fmt.Printf("\r  %s %-12s %s %s\n", ui.Dim.Sprint("○"), ui.Cyan.Sprint(string(ev.Phase)), ev.Message, ui.Dim.Sprint(ev.Dur.Round(time.Millisecond)))
	case "fail":
		errMsg := ""
		if ev.Err != nil {
			errMsg = ": " + ev.Err.Error()
		}
		fmt.Printf("\r  %s %-12s %s%s %s\n", ui.Red.Sprint("✗"), ui.Cyan.Sprint(string(ev.Phase)), ev.Message, errMsg, ui.Dim.Sprint(ev.Dur.Round(time.Millisecond)))
	}
}

func runSetupWizard(cfg *config.Config) {
	fmt.Println(ui.Dim.Sprint(strings.Repeat("─", 50)))
	fmt.Printf("%s\n\n", ui.Bold.Sprint("🚀 配置向导"))

	reader := bufio.NewReader(os.Stdin)

	fmt.Printf("%s\n\n", ui.Bold.Sprint("第 1/4 步: 选择 LLM 提供商"))
	showAvailableProviders()
	fmt.Printf("%s\n输入提供商名称 %s>%s ", ui.Cyan.Sprint(""), ui.Green.Sprint(""), ui.Reset.Sprint(""))

	provider, _ := reader.ReadString('\n')
	provider = strings.TrimSpace(strings.ToLower(provider))
	if provider == "" {
		provider = "qwen"
	}

	if provider == "ali" || provider == "alibaba" {
		provider = "qwen"
	}

	cfg.LLM.Provider = provider

	fmt.Printf("\n%s\n\n", ui.Bold.Sprint("第 2/4 步: 选择模型"))
	showModelsForProvider(provider)
	fmt.Printf("%s\n输入模型名称 %s>%s ", ui.Cyan.Sprint(""), ui.Green.Sprint(""), ui.Reset.Sprint(""))

	model, _ := reader.ReadString('\n')
	model = strings.TrimSpace(strings.ToLower(model))
	if model == "" {
		model = getDefaultModel(provider)
	}
	cfg.LLM.Model = model

	fmt.Printf("\n%s\n", ui.Bold.Sprint("第 3/5 步: 输入 API 密钥"))
	fmt.Printf("%s\n", getProviderHint(provider))
	fmt.Printf("%sAPI 密钥: %s", ui.Cyan.Sprint(""), ui.Reset.Sprint(""))

	apiKey, _ := reader.ReadString('\n')
	apiKey = strings.TrimSpace(apiKey)
	cfg.LLM.APIKey = apiKey

	fmt.Printf("\n%s", ui.Bold.Sprint("第 4/5 步: 代理设置"))
	fmt.Printf("%s (可选，直接回车跳过)%s", ui.Yellow.Sprint(""), ui.Reset.Sprint(""))
	fmt.Printf("%s\n>%s ", ui.Green.Sprint(""), ui.Reset.Sprint(""))

	proxy, _ := reader.ReadString('\n')
	proxy = strings.TrimSpace(proxy)
	cfg.LLM.Proxy = proxy

	fmt.Printf("\n%s", ui.Bold.Sprint("第 5/5 步: 智能体名称"))
	fmt.Printf("%s (默认: AnyClaw)%s", ui.Yellow.Sprint(""), ui.Reset.Sprint(""))
	fmt.Printf("%s\n>%s ", ui.Green.Sprint(""), ui.Reset.Sprint(""))

	name, _ := reader.ReadString('\n')
	name = strings.TrimSpace(name)
	if name != "" {
		cfg.Agent.Name = name
	}

	fmt.Println()
	printSuccess("配置已保存!")
	fmt.Println(ui.Dim.Sprint(strings.Repeat("─", 50)))
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
		"openai":     "获取 API 密钥: https://platform.openai.com/api-keys\n              (国内用户建议使用代理或切换到通义千问)",
		"anthropic":  "获取 API 密钥: https://console.anthropic.com/settings/keys\n              (国内用户建议使用代理或切换到通义千问)",
		"qwen":       "获取 API 密钥: https://dashscope.console.aliyun.com/apiKey\n              通义千问 - 国内可直接访问",
		"ollama":     "无需 API 密钥。请确保 Ollama 已在本地运行。\n              下载地址: https://ollama.com",
		"compatible": "请输入您的 OpenAI 兼容 API 密钥。",
	}
	if hint, ok := hints[provider]; ok {
		return hint
	}
	return "请输入您的 API 密钥。"
}

func runInteractive(ctx context.Context, state *RuntimeState) {
	fmt.Println()
	fmt.Println(ui.Dim.Sprint(strings.Repeat("─", 50)))
	fmt.Printf("%s\n", ui.Bold.Sprint("交互模式命令:"))
	fmt.Println("  /exit, /quit, /q   - 退出程序")
	fmt.Println("  /clear             - 清除对话历史")
	fmt.Println("  /memory            - 查看记忆内容")
	fmt.Println("  /skills            - 查看可用技能")
	fmt.Println("  /tools             - 查看可用工具")
	fmt.Println("  /provider          - 显示当前提供商/模型")
	fmt.Println("  /providers         - 显示可用提供商")
	fmt.Println("  /models <名称>     - 显示提供商模型")
	fmt.Println("  /agents            - 显示 Agent 配置")
	fmt.Println("  /agent use <名称>  - 切换 Agent")
	fmt.Println("  /audit             - 查看最近审计日志")
	fmt.Println("  /set provider <值> - 切换 LLM 提供商")
	fmt.Println("  /set model <值>    - 切换模型")
	fmt.Println("  /set apikey <值>   - 设置 API 密钥")
	fmt.Println("  /set temp <值>     - 设置温度 (0.0-2.0)")
	fmt.Println("  /help, /?          - 显示帮助")
	fmt.Println(ui.Dim.Sprint(strings.Repeat("─", 50)))
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
		DangerousPatterns:     state.cfg.Security.DangerousCommandPatterns,
		CommandTimeoutSeconds: state.cfg.Security.CommandTimeoutSeconds,
		AuditLogger:           state.audit,
		Sandbox:               sandboxManager,
		ConfirmDangerousCommand: func(command string) bool {
			if !state.cfg.Agent.RequireConfirmationForDangerous {
				return true
			}
			fmt.Printf("%s检测到高风险命令:%s %s\n", ui.Warning.Sprint(""), ui.Reset.Sprint(""), command)
			fmt.Printf("%s确认执行? (y/N): ", ui.Yellow.Sprint(""))
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
		return fmt.Sprintf("[模型路由: %s -> %s/%s]", decision.Reason, state.cfg.LLM.Provider, state.cfg.LLM.Model)
	}
	return ""
}

func showAgentProfiles(state *RuntimeState) {
	fmt.Println()
	fmt.Printf("%s\n", ui.Bold.Sprint("Agent 列表:"))
	fmt.Printf("  当前 Agent: %s\n", state.cfg.Agent.Name)
	fmt.Printf("  权限级别: %s\n", state.cfg.Agent.PermissionLevel)
	if len(state.cfg.Agent.Profiles) == 0 {
		fmt.Println("  (未配置额外 Agent)")
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
		printError("读取审计日志失败: %v", err)
		return
	}
	fmt.Println()
	fmt.Printf("%s\n", ui.Bold.Sprint("最近审计日志:"))
	if len(events) == 0 {
		fmt.Println("  (暂无日志)")
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
		return fmt.Errorf("agent 不存在: %s", name)
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
		return fmt.Errorf("切换 agent 失败: %w", err)
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
		printSuccess("再见!")
		return true

	case cmd == "/help", cmd == "/?":
		fmt.Println()
		fmt.Printf("%s\n", ui.Bold.Sprint("可用命令:"))
		fmt.Println("  /exit, /quit, /q   - 退出程序")
		fmt.Println("  /clear             - 清除对话历史")
		fmt.Println("  /memory            - 查看记忆内容")
		fmt.Println("  /skills            - 查看可用技能")
		fmt.Println("  /tools             - 查看可用工具")
		fmt.Println("  /provider          - 显示当前提供商/模型")
		fmt.Println("  /providers         - 显示可用提供商")
		fmt.Println("  /models <名称>     - 显示提供商模型")
		fmt.Println("  /agents            - 显示 Agent 配置")
		fmt.Println("  /agent use <名称>  - 切换 Agent")
		fmt.Println("  /audit             - 查看最近审计日志")
		fmt.Println("  /set provider <值> - 切换 LLM 提供商")
		fmt.Println("  /set model <值>    - 切换模型")
		fmt.Println("  /set apikey <值>   - 设置 API 密钥")
		fmt.Println("  /set temp <值>     - 设置温度 (0.0-2.0)")
		return false

	case cmd == "/clear":
		state.agent.ClearHistory()
		printSuccess("对话已清除")
		return false

	case cmd == "/memory":
		mem, _ := state.agent.ShowMemory()
		fmt.Println()
		fmt.Println(ui.Dim.Sprint(strings.Repeat("─", 40)))
		fmt.Println(mem)
		fmt.Println(ui.Dim.Sprint(strings.Repeat("─", 40)))
		return false

	case cmd == "/skills":
		skills := state.agent.ListSkills()
		fmt.Println()
		fmt.Printf("%s\n", ui.Bold.Sprint("技能列表:"))
		if len(skills) == 0 {
			fmt.Printf("%s\n", ui.Yellow.Sprint("  (无)"))
		}
		for _, s := range skills {
			fmt.Printf("  %s- %s: %s\n", ui.Cyan.Sprint(""), s.Name, s.Description)
		}
		return false

	case cmd == "/tools":
		tools := state.agent.ListTools()
		fmt.Println()
		fmt.Printf("%s\n", ui.Bold.Sprint("工具列表:"))
		for _, t := range tools {
			fmt.Printf("  %s- %s: %s\n", ui.Cyan.Sprint(""), t.Name, t.Description)
		}
		return false

	case cmd == "/provider":
		fmt.Println()
		fmt.Printf("%s提供商: %s\n", ui.Cyan.Sprint(""), state.cfg.LLM.Provider)
		fmt.Printf("%s模型: %s\n", ui.Cyan.Sprint(""), state.cfg.LLM.Model)
		fmt.Printf("%s温度: %.1f\n", ui.Cyan.Sprint(""), state.cfg.LLM.Temperature)
		fmt.Printf("%s权限级别: %s\n", ui.Cyan.Sprint(""), state.cfg.Agent.PermissionLevel)
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
			printSuccess("已切换到 Agent: %s", name)
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
		fmt.Printf("%s未知命令: %s\n", ui.Error.Sprint(""), input)
		fmt.Printf("输入 %s/help%s 查看可用命令\n", ui.Cyan.Sprint(""), ui.Reset.Sprint(""))
		return false
	}
}

func handleSetCommand(state *RuntimeState, input string) {
	parts := strings.SplitN(input, " ", 3)
	if len(parts) < 3 {
		fmt.Println("用法: /set <provider|model|apikey|temp> <值>")
		fmt.Println()
		fmt.Println("示例:")
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
		printError("未知设置: %s", key)
		fmt.Println("可用设置: provider, model, apikey, temp")
	}
}

func showAvailableProviders() {
	fmt.Printf("%s\n\n", ui.Bold.Sprint("可用提供商:"))
	fmt.Printf("%s  openai%s      - %sOpenAI%s (GPT-4, GPT-3.5)\n", ui.Cyan.Sprint(""), ui.Reset.Sprint(""), ui.Green.Sprint(""), ui.Reset.Sprint(""))
	fmt.Printf("%s  anthropic%s   - %sAnthropic%s (Claude)\n", ui.Cyan.Sprint(""), ui.Reset.Sprint(""), ui.Green.Sprint(""), ui.Reset.Sprint(""))
	fmt.Printf("%s  qwen%s        - %s阿里通义千问%s (国内可用)\n", ui.Cyan.Sprint(""), ui.Reset.Sprint(""), ui.Green.Sprint(""), ui.Reset.Sprint(""))
	fmt.Printf("%s  ollama%s      - %sOllama%s (本地模型)\n", ui.Cyan.Sprint(""), ui.Reset.Sprint(""), ui.Green.Sprint(""), ui.Reset.Sprint(""))
	fmt.Printf("%s  compatible%s  - %sOpenAI 兼容 API%s\n", ui.Cyan.Sprint(""), ui.Reset.Sprint(""), ui.Green.Sprint(""), ui.Reset.Sprint(""))
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
		fmt.Printf("%s\n\n", ui.Bold.Sprint(provider+" 的模型:"))
		for _, m := range modelList {
			fmt.Printf("  %s- %s\n", ui.Cyan.Sprint(""), m)
		}
	} else {
		fmt.Printf("%s未知提供商: %s\n", ui.Error.Sprint(""), provider)
		showAvailableProviders()
	}
}

func runSkillCommand() {
	if len(os.Args) < 2 {
		printSkillUsage()
		return
	}

	args := os.Args[2:]
	if len(args) == 0 {
		printSkillUsage()
		return
	}

	switch args[0] {
	case "search":
		query := ""
		if len(args) > 1 {
			query = strings.Join(args[1:], " ")
		}
		searchSkillsFromHub(query)
	case "install":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "用法: anyclaw skill install <名称>\n")
			os.Exit(1)
		}
		installSkillFromHub(args[1])
	case "list":
		listInstalledSkills()
	case "info":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "用法: anyclaw skill info <名称>\n")
			os.Exit(1)
		}
		showSkillInfo(args[1])
	case "catalog", "market", "registry":
		query := ""
		if len(args) > 1 {
			query = strings.Join(args[1:], " ")
		}
		showSkillCatalog(query)
	case "create":
		createNewSkill()
	default:
		fmt.Fprintf(os.Stderr, "未知技能命令: %s\n", args[0])
		printSkillUsage()
		os.Exit(1)
	}
}

func printSkillUsage() {
	fmt.Print(`AnyClaw 技能命令:

用法:
  anyclaw skill search <关键词>   搜索技能
  anyclaw skill install <名称>    安装技能
  anyclaw skill create            交互式创建技能
  anyclaw skill list              列出已安装的技能
  anyclaw skill info <名称>       显示技能信息
  anyclaw skill catalog [关键词]  查看技能市场/注册表

示例:
  anyclaw skill search coder
  anyclaw skill install coder
  anyclaw skill list
`)
}

func runSkillhubCommand() {
	if len(os.Args) < 2 {
		printSkillhubUsage()
		return
	}

	args := os.Args[2:]
	if len(args) == 0 {
		printSkillhubUsage()
		return
	}

	switch args[0] {
	case "search":
		query := ""
		if len(args) > 1 {
			query = strings.Join(args[1:], " ")
		}
		searchSkillhubFromCLI(query)
	case "install":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "用法: anyclaw skillhub install <名称>\n")
			os.Exit(1)
		}
		installSkillhubFromCLI(args[1])
	case "list":
		listSkillhubSkills()
	case "check":
		checkSkillhubCLI()
	default:
		fmt.Fprintf(os.Stderr, "未知 Skillhub 命令: %s\n", args[0])
		printSkillhubUsage()
		os.Exit(1)
	}
}

func printSkillhubUsage() {
	fmt.Print(`AnyClaw Skillhub 商店:

用法:
  anyclaw skillhub search <关键词>   搜索 Skillhub 技能
  anyclaw skillhub install <名称>    安装 Skillhub 技能
  anyclaw skillhub list              列出已安装的技能
  anyclaw skillhub check             查看 Skillhub 状态

示例:
  anyclaw skillhub search calendar
  anyclaw skillhub install calendar
  anyclaw skillhub list
`)
}

func searchSkillhubFromCLI(query string) {
	fmt.Printf("%s在 Skillhub 搜索: %s\n", ui.Cyan.Sprint("❯"), query)
	fmt.Println(ui.Dim.Sprint(strings.Repeat("─", 50)))

	ctx := context.Background()
	results, err := skills.SearchSkillhub(ctx, query, 10)
	if err != nil {
		printError("搜索失败: %v", err)
		return
	}

	if len(results) == 0 {
		printInfo("没有找到相关技能")
		return
	}

	fmt.Printf("%s 找到 %d 个技能\n\n", ui.Bold.Sprint("✓"), len(results))

	for i, r := range results {
		fullName := r.FullName
		if fullName == "" {
			fullName = r.Name
		}
		desc := r.Description
		if desc == "" {
			desc = "无描述"
		}

		fmt.Printf("%s %s\n", ui.Bold.Sprint(fmt.Sprintf("%d.", i+1)), ui.Green.Sprint(fullName))
		fmt.Printf("   %s\n", desc)
		if r.Category != "" {
			fmt.Printf("   分类: %s\n", r.Category)
		}
		fmt.Printf("   %s\n\n", ui.Dim.Sprint(fmt.Sprintf("安装: anyclaw skillhub install %s", r.Name)))
	}

	fmt.Println(ui.Dim.Sprint(strings.Repeat("─", 50)))
}

func installSkillhubFromCLI(skillName string) {
	fmt.Printf("%s正在安装 Skillhub 技能: %s\n", ui.Cyan.Sprint("❯"), skillName)

	ctx := context.Background()
	skillsDir := "skills"
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		printError("创建技能目录失败: %v", err)
		return
	}

	if err := skills.InstallSkillhubSkill(ctx, skillName, skillsDir); err != nil {
		printError("安装失败: %v", err)
		return
	}

	printSuccess("技能安装成功: %s", skillName)
	printInfo("请重启 AnyClaw 以加载新技能")
}

func listSkillhubSkills() {
	entries, err := os.ReadDir("skills")
	if err != nil {
		printInfo("没有已安装的技能")
		return
	}

	var skillList []string
	for _, entry := range entries {
		if entry.IsDir() {
			skillJSON := filepath.Join("skills", entry.Name(), "skill.json")
			if _, err := os.Stat(skillJSON); err == nil {
				skillList = append(skillList, entry.Name())
			}
		}
	}

	if len(skillList) == 0 {
		printInfo("没有已安装的技能")
		return
	}

	fmt.Printf("%s\n\n", ui.Bold.Sprint("已安装的技能:"))
	for _, skill := range skillList {
		fmt.Printf("  %s %s\n", ui.Green.Sprint("●"), skill)
	}
}

func checkSkillhubCLI() {
	printSuccess("Skillhub 已集成到 AnyClaw")
	printInfo("使用 'anyclaw skillhub search <关键词>' 搜索技能")
	printInfo("使用 'anyclaw skillhub install <名称>' 安装技能")
	printInfo("使用 'anyclaw skillhub list' 列出已安装技能")
}

func searchSkillsFromHub(query string) {
	fmt.Printf("%s在 skills.sh 搜索: %s\n", ui.Cyan.Sprint("❯"), query)
	fmt.Println(ui.Dim.Sprint(strings.Repeat("─", 50)))

	ctx := context.Background()
	results, err := skills.SearchSkills(ctx, query, 10)
	if err != nil || len(results) == 0 {
		showBuiltinSkillsHelp()
		return
	}

	fmt.Printf("%s 找到 %d 个技能\n\n", ui.Bold.Sprint("✓"), len(results))

	for i, r := range results {
		installs := formatInstalls(r.Installs)
		fullName := r.FullName
		if fullName == "" {
			fullName = r.Name
		}
		desc := r.Description
		if desc == "" {
			desc = "无描述"
		}

		qualityBadge := getQualityBadge(r.Installs, r.Stars)

		fmt.Printf("%s %s\n", ui.Bold.Sprint(fmt.Sprintf("%d.", i+1)), ui.Green.Sprint(fullName))
		fmt.Printf("   %s\n", desc)
		fmt.Printf("   %s  %s  %s\n",
			ui.Yellow.Sprint("📥"+installs),
			ui.Yellow.Sprint("⭐"+strconv.Itoa(r.Stars)),
			qualityBadge,
		)
		fmt.Printf("   %s\n\n", ui.Dim.Sprint(fmt.Sprintf("安装: anyclaw skill install %s", r.Name)))
	}

	fmt.Println(ui.Dim.Sprint(strings.Repeat("─", 50)))
	fmt.Printf("%s 更多技能: %s\n", ui.Info.Sprint("ℹ"), ui.Cyan.Sprint("https://skills.sh/"))
}

func getQualityBadge(installs int64, stars int) string {
	if installs >= 100000 || stars >= 1000 {
		return ui.Bold.Sprint(ui.Green.Sprint(" ★ 精品"))
	}
	if installs >= 10000 || stars >= 500 {
		return ui.Bold.Sprint(ui.Cyan.Sprint(" ★ 热门"))
	}
	if installs >= 1000 || stars >= 100 {
		return ui.Green.Sprint("推荐")
	}
	return ui.Yellow.Sprint("一般")
}

func showBuiltinSkillsHelp() {
	fmt.Printf("%s 没有找到相关技能\n\n", ui.Warning.Sprint("⚠"))
	fmt.Printf("%s\n", ui.Bold.Sprint("📦 内置技能:"))
	fmt.Println(ui.Dim.Sprint(strings.Repeat("─", 40)))

	skillNames := []struct {
		name        string
		description string
		examples    string
	}{
		{"coder", "代码生成和分析", "写代码、调试、解释代码"},
		{"writer", "内容写作和编辑", "写文章、修改文案、润色"},
		{"researcher", "网络搜索和调研", "搜索信息、研究主题"},
		{"analyst", "数据分析和可视化", "分析数据、生成图表"},
		{"translator", "多语言翻译", "中英互译、文档翻译"},
		{"find-skills", "技能推荐助手", "帮你找到合适的技能"},
	}

	for _, s := range skillNames {
		fmt.Printf("  %s %s\n", ui.Green.Sprint(s.name), ui.Dim.Sprint("-"+s.description))
		fmt.Printf("     %s\n", ui.Dim.Sprint(s.examples))
	}

	fmt.Println(ui.Dim.Sprint(strings.Repeat("─", 40)))
	fmt.Printf("\n%s %s\n", ui.Info.Sprint("💡"), ui.Dim.Sprint("安装内置技能:"))
	fmt.Printf("  %s\n\n", ui.Cyan.Sprint("  anyclaw skill install <名称>"))
}

func formatInstalls(n int64) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	}
	return strconv.FormatInt(n, 10)
}

func installSkillFromHub(name string) {
	skillsDir := "skills"
	if envDir := os.Getenv("ANYCLAW_SKILLS_DIR"); envDir != "" {
		skillsDir = envDir
	}

	if skillContent, ok := skills.BuiltinSkills[name]; ok {
		installBuiltinSkill(name, skillContent, skillsDir)
		return
	}

	parts := strings.Split(name, "/")
	if len(parts) == 3 {
		owner, repo, skillName := parts[0], parts[1], parts[2]
		fmt.Printf("正在从 skills.sh 安装 %s/%s/%s...\n", owner, repo, skillName)
		ctx := context.Background()
		if err := skills.InstallSkillFromGitHub(ctx, owner, repo, skillName, skillsDir); err != nil {
			fmt.Fprintf(os.Stderr, "安装失败: %v\n", err)
			fmt.Println("\n可用的内置技能:")
			for name := range skills.BuiltinSkills {
				fmt.Printf("  - %s\n", name)
			}
			os.Exit(1)
		}
		fmt.Printf("%s成功从 skills.sh 安装 %s!%s\n", ui.Green.Sprint(""), name, ui.Reset.Sprint(""))
		return
	}

	fmt.Fprintf(os.Stderr, "未找到技能: %s\n", name)
	fmt.Println("\n可用的内置技能:")
	for name := range skills.BuiltinSkills {
		fmt.Printf("  - %s\n", name)
	}
	fmt.Println("\n或从 skills.sh 安装:")
	fmt.Printf("  %sanyclaw skill install owner/repo/skill-name%s\n", ui.Cyan.Sprint(""), ui.Reset.Sprint(""))
	fmt.Println("  示例: anyclaw skill install vercel-labs/agent-skills/react-best-practices")
	os.Exit(1)
}

func installBuiltinSkill(name, content, skillsDir string) {
	skillPath := filepath.Join(skillsDir, name)
	if err := os.MkdirAll(skillPath, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create skill directory: %v\n", err)
		os.Exit(1)
	}

	filePath := filepath.Join(skillPath, "skill.json")
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write skill file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("%sSuccessfully installed skill: %s%s\n", ui.Green.Sprint(""), name, ui.Reset.Sprint(""))
}

func listInstalledSkills() {
	skillsDir := "skills"
	if envDir := os.Getenv("ANYCLAW_SKILLS_DIR"); envDir != "" {
		skillsDir = envDir
	}

	manager := skills.NewSkillsManager(skillsDir)
	if err := manager.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "加载技能失败: %v\n", err)
		os.Exit(1)
	}

	list := manager.List()
	if len(list) == 0 {
		fmt.Println("未安装任何技能。")
		fmt.Println("\n安装技能命令:")
		fmt.Printf("  %sanyclaw skill install <名称>%s\n", ui.Cyan.Sprint(""), ui.Reset.Sprint(""))
		return
	}

	fmt.Printf("%s\n\n", ui.Bold.Sprint(fmt.Sprintf("已安装的技能 (%d):", len(list))))
	for _, s := range list {
		fmt.Printf("  %s\n", ui.Green.Sprint(fmt.Sprintf("%-15s v%s", s.Name, s.Version)))
		fmt.Printf("  %s\n\n", s.Description)
		if len(s.Permissions) > 0 {
			fmt.Printf("    permissions: %s\n", strings.Join(s.Permissions, ", "))
		}
		if s.Entrypoint != "" || s.Registry != "" {
			fmt.Printf("    entrypoint: %s  registry: %s\n\n", s.Entrypoint, s.Registry)
		}
	}
}

func showSkillInfo(name string) {
	skillsDir := "skills"
	if envDir := os.Getenv("ANYCLAW_SKILLS_DIR"); envDir != "" {
		skillsDir = envDir
	}
	manager := skills.NewSkillsManager(skillsDir)
	_ = manager.Load()
	if skill, ok := manager.Get(name); ok {
		fmt.Printf("名称:        %s\n", skill.Name)
		fmt.Printf("版本:        %s\n", skill.Version)
		fmt.Printf("描述:        %s\n", skill.Description)
		fmt.Printf("来源:        %s\n", skill.Source)
		fmt.Printf("注册表:      %s\n", skill.Registry)
		fmt.Printf("入口:        %s\n", skill.Entrypoint)
		if len(skill.Permissions) > 0 {
			fmt.Printf("权限:        %s\n", strings.Join(skill.Permissions, ", "))
		}
		if skill.InstallCommand != "" {
			fmt.Printf("安装命令:    %s\n", skill.InstallCommand)
		}
		return
	}
	fmt.Fprintf(os.Stderr, "未找到技能: %s\n", name)
	os.Exit(1)
}

func showSkillCatalog(query string) {
	ctx := context.Background()
	entries, err := skills.SearchCatalog(ctx, query, 20)
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载技能市场失败: %v\n", err)
		os.Exit(1)
	}
	if len(entries) == 0 {
		fmt.Println("未找到任何技能。")
		return
	}
	fmt.Printf("%s\n\n", ui.Bold.Sprint("技能市场 / Registry"))
	for _, entry := range entries {
		name := entry.FullName
		if name == "" {
			name = entry.Name
		}
		fmt.Printf("- %s  v%s\n", name, entry.Version)
		fmt.Printf("  %s\n", entry.Description)
		if len(entry.Permissions) > 0 {
			fmt.Printf("  permissions: %s\n", strings.Join(entry.Permissions, ", "))
		}
		if entry.Entrypoint != "" {
			fmt.Printf("  entrypoint: %s\n", entry.Entrypoint)
		}
		if entry.InstallHint != "" {
			fmt.Printf("  install: %s\n", entry.InstallHint)
		}
		fmt.Println()
	}
}

func createNewSkill() {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println()
	fmt.Println(ui.Dim.Sprint(strings.Repeat("─", 50)))
	fmt.Printf("%s  %s\n", ui.Bold.Sprint("🎨"), ui.Bold.Sprint("创建新技能"))
	fmt.Printf("%s\n\n", ui.Dim.Sprint("引导你完成技能创建"))
	fmt.Println(ui.Dim.Sprint(strings.Repeat("─", 50)))

	var name, description, version, systemPrompt string
	var commands []map[string]string

	fmt.Printf("\n%s 基础信息\n", ui.Bold.Sprint("📝 第一步"))
	fmt.Println(ui.Dim.Sprint(strings.Repeat("─", 30)))

	fmt.Printf("%s技能名称%s (%s必填%s)\n", ui.Cyan.Sprint(""), ui.Reset.Sprint(""), ui.Red.Sprint("*"), ui.Reset.Sprint(""))
	fmt.Printf("%s使用字母、数字、下划线，如: my-tool, code_helper%s\n", ui.Dim.Sprint(""), ui.Reset.Sprint(""))
	fmt.Printf("%s>%s ", ui.Green.Sprint(""), ui.Reset.Sprint(""))
	nameInput, _ := reader.ReadString('\n')
	name = strings.TrimSpace(nameInput)
	if name == "" {
		printError("技能名称不能为空")
		os.Exit(1)
	}
	if strings.Contains(name, " ") {
		printError("技能名称不能包含空格")
		os.Exit(1)
	}
	if matched, _ := filepath.Match("*[^a-zA-Z0-9_-]*", name); matched && name != "" {
		printError("技能名称只能包含字母、数字、下划线和连字符")
		os.Exit(1)
	}

	fmt.Printf("\n%s技能描述%s (%s必填%s)\n", ui.Cyan.Sprint(""), ui.Reset.Sprint(""), ui.Red.Sprint("*"), ui.Reset.Sprint(""))
	fmt.Printf("%s简短描述这个技能的作用%s\n", ui.Dim.Sprint(""), ui.Reset.Sprint(""))
	fmt.Printf("%s>%s ", ui.Green.Sprint(""), ui.Reset.Sprint(""))
	descInput, _ := reader.ReadString('\n')
	description = strings.TrimSpace(descInput)
	if description == "" {
		printError("技能描述不能为空")
		os.Exit(1)
	}

	fmt.Printf("\n%s版本号%s (%s默认: %s%s)\n", ui.Cyan.Sprint(""), ui.Reset.Sprint(""), ui.Dim.Sprint(""), ui.Green.Sprint("1.0.0"), ui.Reset.Sprint(""))
	fmt.Printf("%s>%s ", ui.Green.Sprint(""), ui.Reset.Sprint(""))
	verInput, _ := reader.ReadString('\n')
	version = strings.TrimSpace(verInput)
	if version == "" {
		version = "1.0.0"
	}

	fmt.Printf("\n%s 系统提示词\n", ui.Bold.Sprint("🧠 第二步"))
	fmt.Println(ui.Dim.Sprint(strings.Repeat("─", 30)))
	fmt.Printf("%s定义 AI 的角色和行为规则（可选）%s\n\n", ui.Dim.Sprint(""), ui.Reset.Sprint(""))
	fmt.Printf("%s  1. 角色定位：如 \"你是一个专业的 Python 开发者\"\n", ui.Dim.Sprint(""))
	fmt.Printf("  2. 能力范围：如 \"擅长编写高效的算法\"\n")
	fmt.Printf("  3. 工作原则：如 \"优先代码可读性\"\n\n")
	fmt.Printf("%s直接回车跳过，或输入提示词:%s\n", ui.Info.Sprint("💡"), ui.Reset.Sprint(""))
	fmt.Printf("%s>%s ", ui.Green.Sprint(""), ui.Reset.Sprint(""))

	lines := []string{}
	for {
		line, _ := reader.ReadString('\n')
		line = strings.TrimSuffix(line, "\n")
		if line == "" && len(lines) > 0 {
			break
		}
		if line == "" && len(lines) == 0 {
			fmt.Println()
			break
		}
		lines = append(lines, line)
	}
	systemPrompt = strings.Join(lines, "\n")
	systemPrompt = strings.TrimSpace(systemPrompt)

	fmt.Printf("\n%s 命令配置\n", ui.Bold.Sprint("⚡ 第三步"))
	fmt.Println(ui.Dim.Sprint(strings.Repeat("─", 30)))
	fmt.Printf("%s命令用于在对话中触发技能（可选）%s\n\n", ui.Dim.Sprint(""), ui.Reset.Sprint(""))

	for {
		fmt.Printf("%s命令名称%s (%s输入空结束%s)\n", ui.Yellow.Sprint(""), ui.Reset.Sprint(""), ui.Dim.Sprint("回车"), ui.Reset.Sprint(""))
		fmt.Printf("%s>%s ", ui.Green.Sprint(""), ui.Reset.Sprint(""))
		cmdName, _ := reader.ReadString('\n')
		cmdName = strings.TrimSpace(cmdName)
		if cmdName == "" {
			break
		}

		fmt.Printf("%s命令描述%s\n", ui.Yellow.Sprint(""), ui.Reset.Sprint(""))
		fmt.Printf("%s>%s ", ui.Green.Sprint(""), ui.Reset.Sprint(""))
		cmdDesc, _ := reader.ReadString('\n')
		cmdDesc = strings.TrimSpace(cmdDesc)

		fmt.Printf("%s触发关键词%s (用户输入包含此词时激活)\n", ui.Yellow.Sprint(""), ui.Reset.Sprint(""))
		fmt.Printf("%s>%s ", ui.Green.Sprint(""), ui.Reset.Sprint(""))
		cmdPattern, _ := reader.ReadString('\n')
		cmdPattern = strings.TrimSpace(cmdPattern)

		commands = append(commands, map[string]string{
			"name":        cmdName,
			"description": cmdDesc,
			"pattern":     cmdPattern,
		})
		fmt.Printf("\n%s 已添加命令: %s\n", ui.Success.Sprint("✓"), cmdName)
		fmt.Println()
	}

	skill := map[string]any{
		"name":        name,
		"description": description,
		"version":     version,
		"commands":    commands,
		"prompts":     map[string]string{},
	}

	if systemPrompt != "" {
		skill["prompts"] = map[string]string{"system": systemPrompt}
	}

	data, err := json.MarshalIndent(skill, "", "  ")
	if err != nil {
		printError("生成技能文件失败: %v", err)
		os.Exit(1)
	}

	skillsDir := "skills"
	if envDir := os.Getenv("ANYCLAW_SKILLS_DIR"); envDir != "" {
		skillsDir = envDir
	}

	skillPath := filepath.Join(skillsDir, name)
	if err := os.MkdirAll(skillPath, 0755); err != nil {
		printError("创建技能目录失败: %v", err)
		os.Exit(1)
	}

	filePath := filepath.Join(skillPath, "skill.json")

	if _, err := os.Stat(filePath); err == nil {
		fmt.Printf("\n%s 技能已存在: %s\n", ui.Warning.Sprint("⚠"), name)
		fmt.Printf("%s是否覆盖? (y/N): ", ui.Yellow.Sprint(""))
		confirm, _ := reader.ReadString('\n')
		if strings.ToLower(strings.TrimSpace(confirm)) != "y" {
			fmt.Println("已取消")
			return
		}
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		printError("写入技能文件失败: %v", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println(ui.Dim.Sprint(strings.Repeat("─", 50)))
	fmt.Printf("%s 技能创建成功!\n\n", ui.Success.Sprint("✓"))
	fmt.Printf("  %s  %s %s\n", ui.Cyan.Sprint("📦"), ui.Bold.Sprint(name), ui.Dim.Sprint("v"+version))
	fmt.Printf("  %s  %s\n", ui.Cyan.Sprint("📝"), description)
	if len(commands) > 0 {
		fmt.Printf("  %s  %d 个命令\n", ui.Cyan.Sprint("⚡"), len(commands))
	}
	fmt.Printf("\n  %s %s\n", ui.Cyan.Sprint("📂"), filePath)
	fmt.Println()
	fmt.Printf("%s 使用 %s 重新加载后生效\n", ui.Info.Sprint("💡"), ui.Bold.Sprint("anyclaw"))
	fmt.Println(ui.Dim.Sprint(strings.Repeat("─", 50)))
}
