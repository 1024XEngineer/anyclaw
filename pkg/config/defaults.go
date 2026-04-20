package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func DefaultConfig() *Config {
	return &Config{
		LLM: LLMConfig{
			Provider:    "openai",
			Model:       "gpt-4o-mini",
			MaxTokens:   4096,
			Temperature: 0.7,
			Routing: ModelRoutingConfig{
				Enabled:           false,
				ReasoningKeywords: []string{"plan", "complex", "reasoning", "analysis", "debug", "script", "code", "refactor", "architecture", "design"},
			},
		},
		Agent: AgentConfig{
			Name:                            "AnyClaw",
			Description:                     "Your AI assistant with file-based memory",
			WorkDir:                         ".anyclaw",
			WorkingDir:                      "workflows",
			PermissionLevel:                 "limited",
			RequireConfirmationForDangerous: true,
		},
		Skills: SkillsConfig{
			Dir:      "skills",
			AutoLoad: true,
		},
		Memory: MemoryConfig{
			Dir:        "memory",
			MaxHistory: 100,
			Format:     "markdown",
			AutoSave:   true,
		},
		Gateway: GatewayConfig{
			Host: "127.0.0.1",
			Port: 18789,
			Bind: "loopback",
			ControlUI: GatewayControlUIConfig{
				BasePath: "/dashboard",
			},
			WorkerCount:          0,
			RuntimeMaxInstances:  16,
			RuntimeIdleSeconds:   900,
			JobWorkerCount:       2,
			JobMaxAttempts:       2,
			JobRetryDelaySeconds: 2,
		},
		Daemon: DaemonConfig{
			PIDFile: ".anyclaw/gateway.pid",
			LogFile: ".anyclaw/gateway.log",
		},
		Sandbox: SandboxConfig{
			Enabled:       false,
			ExecutionMode: "sandbox",
			Backend:       "local",
			BaseDir:       ".anyclaw/sandboxes",
			DockerImage:   "alpine:3.20",
			DockerNetwork: "none",
			ReusePerScope: true,
		},
		Plugins: PluginsConfig{
			Dir:                "plugins",
			AllowExec:          false,
			ExecTimeoutSeconds: 10,
			RequireTrust:       true,
		},
		Channels: ChannelsConfig{
			Telegram: TelegramChannelConfig{
				PollEvery: 3,
			},
			Slack: SlackChannelConfig{
				PollEvery: 3,
			},
			Discord: DiscordChannelConfig{
				PollEvery:    3,
				APIBaseURL:   "https://discord.com/api/v10",
				UseGatewayWS: true,
			},
			WhatsApp: WhatsAppChannelConfig{
				APIVersion: "v20.0",
			},
			Signal: SignalChannelConfig{
				BaseURL:   "http://127.0.0.1:8080",
				PollEvery: 3,
			},
			Routing: RoutingConfig{
				Mode: "per-chat",
			},
		},
		Security: SecurityConfig{
			PublicPaths:              []string{"/healthz"},
			ProtectEvents:            true,
			RateLimitRPM:             120,
			AuditLog:                 ".anyclaw/audit/audit.jsonl",
			DangerousCommandPatterns: []string{"rm -rf", "del /f", "format ", "mkfs", "shutdown", "reboot", "poweroff", "chmod 777", "takeown", "icacls", "git reset --hard"},
			ProtectedPaths:           defaultProtectedPaths(),
			CommandTimeoutSeconds:    30,
		},
		Orchestrator: OrchestratorConfig{
			Enabled:             true,
			MaxConcurrentAgents: 4,
			MaxRetries:          2,
			TimeoutSeconds:      300,
			EnableDecomposition: true,
			SubAgents:           nil,
		},
	}
}

func defaultProtectedPaths() []string {
	items := []string{}
	home, _ := os.UserHomeDir()
	add := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		items = append(items, filepath.Clean(path))
	}
	if runtime.GOOS == "windows" {
		add(os.Getenv("SystemRoot"))
		add(`C:\Windows`)
		add(`C:\Program Files`)
		add(`C:\Program Files (x86)`)
		add(`C:\ProgramData`)
		if home != "" {
			add(filepath.Join(home, ".ssh"))
			add(filepath.Join(home, "AppData"))
			add(filepath.Join(home, "Documents"))
			add(filepath.Join(home, "Desktop"))
			add(filepath.Join(home, "Downloads"))
			add(filepath.Join(home, "Pictures"))
			add(filepath.Join(home, "Videos"))
			add(filepath.Join(home, "Music"))
			add(filepath.Join(home, "NTUSER.DAT"))
		}
	} else {
		add("/etc")
		add("/bin")
		add("/sbin")
		add("/usr")
		add("/boot")
		add("/dev")
		add("/proc")
		add("/sys")
		add("/var/lib")
		if home != "" {
			add(filepath.Join(home, ".ssh"))
			add(filepath.Join(home, ".gnupg"))
			add(filepath.Join(home, ".config"))
			add(filepath.Join(home, "Documents"))
			add(filepath.Join(home, "Desktop"))
			add(filepath.Join(home, "Downloads"))
			add(filepath.Join(home, "Pictures"))
			add(filepath.Join(home, "Videos"))
			add(filepath.Join(home, "Music"))
		}
	}
	return items
}
