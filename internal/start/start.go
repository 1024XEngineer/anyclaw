package start

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"anyclaw/agent"
	"anyclaw/agent/tools"
	"anyclaw/bus"
	"anyclaw/channels"
	"anyclaw/config"
	"anyclaw/cron"
	"anyclaw/gateway"
	"anyclaw/internal"
	"anyclaw/internal/logger"
	"anyclaw/internal/workspace"
	"anyclaw/providers"
	"anyclaw/session"
	"go.uber.org/zap"
)

// Config holds configuration for starting the agent
type Config struct {
	LogLevel string
}

// StartAgent starts the anyclaw agent with all its components
// This is shared between `anyclaw start` and `anyclaw gateway run`
func StartAgent(cfg *Config) error {
	// 确保内置技能被复制到用户目录
	if err := internal.EnsureBuiltinSkills(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to ensure builtin skills: %v\n", err)
	}

	// 确保配置文件存在
	configCreated, err := internal.EnsureConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to ensure config: %v\n", err)
	}
	if configCreated {
		fmt.Println("Config file created at: " + internal.GetConfigPath())
		fmt.Println("Please edit the config file to set your API keys and other settings.")
		fmt.Println()
	}

	// 加载配置
	configFile, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		return err
	}

	// 初始化日志
	logLevel := cfg.LogLevel
	if logLevel == "" {
		logLevel = "info"
	}
	if err := logger.Init(logLevel, false); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		return err
	}
	defer func() { _ = logger.Sync() }()

	logger.Info("Starting anyclaw agent")

	// 验证配置
	if err := config.Validate(configFile); err != nil {
		logger.Fatal("Invalid configuration", zap.Error(err))
	}

	// 获取 workspace 目录
	workspaceDir, err := config.GetWorkspacePath(configFile)
	if err != nil {
		logger.Fatal("Failed to get workspace path", zap.Error(err))
	}

	// 创建 workspace 管理器并确保文件存在
	workspaceMgr := workspace.NewManager(workspaceDir)
	if err := workspaceMgr.Ensure(); err != nil {
		logger.Warn("Failed to ensure workspace files", zap.Error(err))
	} else {
		logger.Info("Workspace ready", zap.String("path", workspaceDir))
	}

	// 创建消息总线
	messageBus := bus.NewMessageBus(100)
	defer messageBus.Close()

	// 创建会话管理器
	homeDir, err := os.UserHomeDir()
	if err != nil {
		logger.Fatal("Failed to get home directory", zap.Error(err))
	}
	sessionDir := homeDir + "/.anyclaw/sessions"
	sessionMgr, err := session.NewManager(sessionDir)
	if err != nil {
		logger.Fatal("Failed to create session manager", zap.Error(err))
	}

	// 创建记忆存储
	memoryStore := agent.NewMemoryStore(workspaceDir)

	// 创建上下文构建器
	contextBuilder := agent.NewContextBuilder(memoryStore, workspaceDir)

	// 创建工具注册表
	toolRegistry := agent.NewToolRegistry()

	// 创建技能加载器
	// 加载顺序（后加载的同名技能会覆盖前面的）：
	// 1. ./skills/ (当前目录，最高优先级)
	// 2. ${WORKSPACE}/skills/ (工作区目录)
	// 3. ~/.anyclaw/skills/ (用户全局目录)
	anyclawDir := homeDir + "/.anyclaw"
	globalSkillsDir := anyclawDir + "/skills"
	workspaceSkillsDir := workspaceDir + "/skills"
	currentSkillsDir := "./skills"

	skillsLoader := agent.NewSkillsLoader(anyclawDir, []string{
		globalSkillsDir,    // 最先加载（最低优先级）
		workspaceSkillsDir, // 其次加载
		currentSkillsDir,   // 最后加载（最高优先级）
	})
	if err := skillsLoader.Discover(); err != nil {
		logger.Warn("Failed to discover skills", zap.Error(err))
	} else {
		skills := skillsLoader.List()
		if len(skills) > 0 {
			logger.Info("Skills loaded", zap.Int("count", len(skills)))
		}
	}

	// 注册文件系统工具
	fsTool := tools.NewFileSystemTool(configFile.Tools.FileSystem.AllowedPaths, configFile.Tools.FileSystem.DeniedPaths, workspaceDir)
	for _, tool := range fsTool.GetTools() {
		if err := toolRegistry.RegisterExisting(tool); err != nil {
			logger.Warn("Failed to register tool", zap.String("tool", tool.Name()))
		}
	}

	// 注册 use_skill 工具（用于两阶段技能加载）
	if err := toolRegistry.RegisterExisting(tools.NewUseSkillTool()); err != nil {
		logger.Warn("Failed to register use_skill tool", zap.Error(err))
	}

	// 注册 Shell 工具
	shellTool := tools.NewShellTool(
		configFile.Tools.Shell.Enabled,
		configFile.Tools.Shell.AllowedCmds,
		configFile.Tools.Shell.DeniedCmds,
		configFile.Tools.Shell.Timeout,
		configFile.Tools.Shell.WorkingDir,
		configFile.Tools.Shell.Sandbox,
	)
	for _, tool := range shellTool.GetTools() {
		if err := toolRegistry.RegisterExisting(tool); err != nil {
			logger.Warn("Failed to register tool", zap.String("tool", tool.Name()))
		}
	}

	// 注册 Web 工具
	webTool := tools.NewWebTool(
		configFile.Tools.Web.SearchAPIKey,
		configFile.Tools.Web.SearchEngine,
		configFile.Tools.Web.Timeout,
	)
	for _, tool := range webTool.GetTools() {
		if err := toolRegistry.RegisterExisting(tool); err != nil {
			logger.Warn("Failed to register tool", zap.String("tool", tool.Name()))
		}
	}

	// 注册浏览器工具（如果启用）
	if configFile.Tools.Browser.Enabled {
		browserTool := tools.NewBrowserTool(
			configFile.Tools.Browser.Headless,
			configFile.Tools.Browser.Timeout,
		)
		for _, tool := range browserTool.GetTools() {
			if err := toolRegistry.RegisterExisting(tool); err != nil {
				logger.Warn("Failed to register tool", zap.String("tool", tool.Name()))
			}
		}
		logger.Info("Browser tools registered")
	}

	// 注册 Cron 工具
	// 注意：cronTool 将在创建 cronService 后注册

	// 创建 LLM 提供商
	provider, err := providers.NewProvider(configFile)
	if err != nil {
		logger.Fatal("Failed to create LLM provider", zap.Error(err))
	}
	defer provider.Close()

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建通道管理器
	channelMgr := channels.NewManager(messageBus)
	if err := channelMgr.SetupFromConfig(configFile); err != nil {
		logger.Warn("Failed to setup channels from config", zap.Error(err))
	}

	// 创建 Cron 服务（需要在 Gateway 之前创建，因为 Handler 需要 cronService）
	cronService, err := cron.NewService(cron.DefaultCronConfig(), messageBus)
	if err != nil {
		logger.Warn("Failed to create cron service", zap.Error(err))
	}
	if cronService != nil {
		if err := cronService.Start(ctx); err != nil {
			logger.Warn("Failed to start cron service", zap.Error(err))
		}
		defer func() { _ = cronService.Stop() }()
	}

	// 注册 Cron 工具（使用已创建并启动的 cronService）
	if configFile.Tools.Cron.Enabled {
		logger.Info("Registering cron tools",
			zap.Bool("cron_service_nil", cronService == nil))
		cronTool := tools.NewCronTool(cronService)
		tools := cronTool.GetTools()
		logger.Info("CronTool.GetTools returned",
			zap.Int("count", len(tools)))
		for _, tool := range tools {
			if err := toolRegistry.RegisterExisting(tool); err != nil {
				logger.Warn("Failed to register tool", zap.String("tool", tool.Name()), zap.Error(err))
			} else {
				logger.Info("Tool registered successfully", zap.String("tool", tool.Name()))
			}
		}
		logger.Info("Cron tools registration completed")
	}

	// 创建网关服务器
	gatewayServer := gateway.NewServer(configFile, messageBus, channelMgr, sessionMgr, cronService)
	if err := gatewayServer.Start(ctx); err != nil {
		logger.Warn("Failed to start gateway server", zap.Error(err))
	}
	defer func() { _ = gatewayServer.Stop() }()

	// 创建 AgentManager
	agentManager := agent.NewAgentManager(&agent.NewAgentManagerConfig{
		Bus:            messageBus,
		Provider:       provider,
		SessionMgr:     sessionMgr,
		Tools:          toolRegistry,
		DataDir:        workspaceDir, // 使用 workspace 作为数据目录
		ContextBuilder: contextBuilder,
		SkillsLoader:   skillsLoader,
		ChannelMgr:     channelMgr,
	})

	// 从配置设置 Agent 和绑定
	if err := agentManager.SetupFromConfig(configFile, contextBuilder); err != nil {
		logger.Fatal("Failed to setup agent manager", zap.Error(err))
	}

	// 处理信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 启动通道
	if err := channelMgr.Start(ctx); err != nil {
		logger.Error("Failed to start channels", zap.Error(err))
	}
	defer func() { _ = channelMgr.Stop() }()

	// 启动出站消息分发
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("Outbound message dispatcher panicked",
					zap.Any("panic", r))
			}
		}()
		if err := channelMgr.DispatchOutbound(ctx); err != nil {
			logger.Error("Outbound message dispatcher exited with error", zap.Error(err))
		} else {
			logger.Debug("Outbound message dispatcher exited normally")
		}
	}()

	// 启动 AgentManager
	go func() {
		if err := agentManager.Start(ctx); err != nil {
			logger.Error("AgentManager error", zap.Error(err))
		}
	}()

	// 等待信号
	<-sigChan
	logger.Info("Received shutdown signal")

	// 停止 AgentManager
	if err := agentManager.Stop(); err != nil {
		logger.Error("Failed to stop agent manager", zap.Error(err))
	}

	logger.Info("anyclaw agent stopped")
	return nil
}
