package cli

import (
	"fmt"
	"os"

	"anyclaw/cli/commands"
	"anyclaw/config"
	"anyclaw/internal/start"
	"anyclaw/internal/workspace"
	"github.com/spf13/cobra"
)

// Version information (populated by goreleaser)
var Version = "dev"

var rootCmd = &cobra.Command{
	Use:   "anyclaw",
	Short: "Go-based AI Agent framework",
	Long:  `anyclaw is a Go language implementation of an AI Agent framework, inspired by nanobot.`,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run:   runVersion,
}

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the anyclaw agent",
	Run:   runStart,
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configuration management",
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	Run:   runConfigShow,
}

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install anyclaw workspace templates",
	Run:   runInstall,
}

// Flags for install command
var (
	installConfigPath    string
	installWorkspacePath string
)

// Flags for start command
var (
	logLevel string
)

func init() {
	// Add install command flags
	installCmd.Flags().StringVar(&installConfigPath, "config", "", "Path to config file")
	installCmd.Flags().StringVar(&installWorkspacePath, "workspace", "", "Path to workspace directory (overrides config)")

	// Add start command flags
	startCmd.Flags().StringVarP(&logLevel, "log-level", "l", "info", "Log level: debug, info, warn, error, fatal (default: info)")

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configShowCmd)
	rootCmd.AddCommand(agentsCmd)
	rootCmd.AddCommand(agentCmd)
	rootCmd.AddCommand(sessionsCmd)
	rootCmd.AddCommand(onboardCmd)

	// Register memory and logs commands from commands package
	// Note: skills command is already registered in cli/skills.go
	rootCmd.AddCommand(commands.MemoryCmd)
	rootCmd.AddCommand(commands.LogsCmd)

	// Register browser, tui, gateway, health, status commands
	rootCmd.AddCommand(commands.BrowserCommand())
	rootCmd.AddCommand(commands.TUICommand())
	rootCmd.AddCommand(commands.GatewayCommand())
	rootCmd.AddCommand(commands.HealthCommand())
	rootCmd.AddCommand(commands.StatusCommand())
	rootCmd.AddCommand(commands.ChannelsCommand())

	// Register pairing command
	rootCmd.AddCommand(pairingCmd)

	// Register approvals, cron, system commands (registered via init)
	// These commands auto-register themselves
}

// SetVersion sets the version from main package
func SetVersion(v string) {
	Version = v
	rootCmd.Version = v
}

// Execute 执行 CLI
func Execute() error {
	return rootCmd.Execute()
}

// runStart 启动 Agent
func runStart(cmd *cobra.Command, args []string) {
	if err := start.StartAgent(&start.Config{LogLevel: logLevel}); err != nil {
		os.Exit(1)
	}
}

// runConfigShow 显示配置
func runConfigShow(cmd *cobra.Command, args []string) {
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Current Configuration:")
	fmt.Printf("  Model: %s\n", cfg.Agents.Defaults.Model.Effective())
	fmt.Printf("  Max Iterations: %d\n", cfg.Agents.Defaults.MaxIterations)
	fmt.Printf("  Temperature: %.1f\n", cfg.Agents.Defaults.Temperature)
}

// runInstall 安装 anyclaw workspace 模板
func runInstall(cmd *cobra.Command, args []string) {
	// 加载配置
	cfg, err := config.Load(installConfigPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// 获取 workspace 目录
	workspaceDir := installWorkspacePath
	if workspaceDir == "" {
		workspaceDir, err = config.GetWorkspacePath(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get workspace path: %v\n", err)
			os.Exit(1)
		}
	}

	// 创建 workspace 管理器并确保文件存在
	workspaceMgr := workspace.NewManager(workspaceDir)
	if err := workspaceMgr.Ensure(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to ensure workspace: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Workspace installed successfully at: %s\n", workspaceDir)
	fmt.Println("\nWorkspace files:")
	files, err := workspaceMgr.ListFiles()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to list files: %v\n", err)
		return
	}
	for _, f := range files {
		fmt.Printf("  - %s\n", f)
	}

	memoryFiles, err := workspaceMgr.ListMemoryFiles()
	if err == nil && len(memoryFiles) > 0 {
		fmt.Println("\nMemory files:")
		for _, f := range memoryFiles {
			fmt.Printf("  - memory/%s\n", f)
		}
	}

	fmt.Println("\nYou can now customize these files to define your agent's personality and behavior.")
}

// runVersion prints version information
func runVersion(cmd *cobra.Command, args []string) {
	fmt.Printf("anyclaw %s\n", Version)
	fmt.Println("Copyright (c) 2024 smallnest")
	fmt.Println("License: MIT")
	fmt.Println("Project: anyclaw")
}
