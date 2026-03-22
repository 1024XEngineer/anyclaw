package config

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	LLM      LLMConfig      `json:"llm"`
	Agent    AgentConfig    `json:"agent"`
	Skills   SkillsConfig   `json:"skills"`
	Memory   MemoryConfig   `json:"memory"`
	Gateway  GatewayConfig  `json:"gateway"`
	Daemon   DaemonConfig   `json:"daemon"`
	Channels ChannelsConfig `json:"channels"`
	Plugins  PluginsConfig  `json:"plugins"`
	Sandbox  SandboxConfig  `json:"sandbox"`
	Security SecurityConfig `json:"security"`
}

type LLMConfig struct {
	Provider    string             `json:"provider"`
	Model       string             `json:"model"`
	APIKey      string             `json:"api_key"`
	BaseURL     string             `json:"base_url"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature float64            `json:"temperature"`
	Proxy       string             `json:"proxy"`
	Extra       map[string]string  `json:"extra"`
	Routing     ModelRoutingConfig `json:"routing"`
}

type ModelRoutingConfig struct {
	Enabled           bool     `json:"enabled"`
	ReasoningKeywords []string `json:"reasoning_keywords"`
	ReasoningProvider string   `json:"reasoning_provider"`
	ReasoningModel    string   `json:"reasoning_model"`
	FastProvider      string   `json:"fast_provider"`
	FastModel         string   `json:"fast_model"`
}

type AgentConfig struct {
	Name                            string         `json:"name"`
	Description                     string         `json:"description"`
	WorkDir                         string         `json:"work_dir"`
	WorkingDir                      string         `json:"working_dir"`
	PermissionLevel                 string         `json:"permission_level"`
	RequireConfirmationForDangerous bool           `json:"require_confirmation_for_dangerous"`
	Profiles                        []AgentProfile `json:"profiles"`
	ActiveProfile                   string         `json:"active_profile"`
}

type AgentProfile struct {
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	Role            string          `json:"role,omitempty"`
	Persona         string          `json:"persona,omitempty"`
	WorkingDir      string          `json:"working_dir"`
	PermissionLevel string          `json:"permission_level"`
	DefaultModel    string          `json:"default_model,omitempty"`
	Enabled         *bool           `json:"enabled,omitempty"`
	Personality     PersonalitySpec `json:"personality,omitempty"`
	Skills          []AgentSkillRef `json:"skills,omitempty"`
}

type PersonalitySpec struct {
	Template           string   `json:"template,omitempty"`
	Tone               string   `json:"tone,omitempty"`
	Style              string   `json:"style,omitempty"`
	GoalOrientation    string   `json:"goal_orientation,omitempty"`
	ConstraintMode     string   `json:"constraint_mode,omitempty"`
	ResponseVerbosity  string   `json:"response_verbosity,omitempty"`
	Traits             []string `json:"traits,omitempty"`
	CustomInstructions string   `json:"custom_instructions,omitempty"`
}

type AgentSkillRef struct {
	Name        string   `json:"name"`
	Enabled     bool     `json:"enabled"`
	Permissions []string `json:"permissions,omitempty"`
	Version     string   `json:"version,omitempty"`
}

type SkillsConfig struct {
	Dir      string   `json:"dir"`
	AutoLoad bool     `json:"auto_load"`
	Include  []string `json:"include"`
	Exclude  []string `json:"exclude"`
}

type MemoryConfig struct {
	Dir        string `json:"dir"`
	MaxHistory int    `json:"max_history"`
	Format     string `json:"format"`
	AutoSave   bool   `json:"auto_save"`
}

type GatewayConfig struct {
	Host                 string `json:"host"`
	Port                 int    `json:"port"`
	Bind                 string `json:"bind"`
	RuntimeMaxInstances  int    `json:"runtime_max_instances"`
	RuntimeIdleSeconds   int    `json:"runtime_idle_seconds"`
	JobWorkerCount       int    `json:"job_worker_count"`
	JobMaxAttempts       int    `json:"job_max_attempts"`
	JobRetryDelaySeconds int    `json:"job_retry_delay_seconds"`
}

type DaemonConfig struct {
	PIDFile string `json:"pid_file"`
	LogFile string `json:"log_file"`
}

type SandboxConfig struct {
	Enabled        bool   `json:"enabled"`
	Backend        string `json:"backend"`
	BaseDir        string `json:"base_dir"`
	DockerImage    string `json:"docker_image"`
	DockerNetwork  string `json:"docker_network"`
	ReusePerScope  bool   `json:"reuse_per_scope"`
	DefaultChannel string `json:"default_channel"`
}

type PluginsConfig struct {
	Dir                string   `json:"dir"`
	Enabled            []string `json:"enabled"`
	AllowExec          bool     `json:"allow_exec"`
	ExecTimeoutSeconds int      `json:"exec_timeout_seconds"`
	TrustedSigners     []string `json:"trusted_signers"`
	RequireTrust       bool     `json:"require_trust"`
}

type ChannelsConfig struct {
	Telegram TelegramChannelConfig `json:"telegram"`
	Slack    SlackChannelConfig    `json:"slack"`
	Discord  DiscordChannelConfig  `json:"discord"`
	WhatsApp WhatsAppChannelConfig `json:"whatsapp"`
	Signal   SignalChannelConfig   `json:"signal"`
	Routing  RoutingConfig         `json:"routing"`
}

type TelegramChannelConfig struct {
	Enabled   bool   `json:"enabled"`
	BotToken  string `json:"bot_token"`
	ChatID    string `json:"chat_id"`
	PollEvery int    `json:"poll_every_seconds"`
}

type SlackChannelConfig struct {
	Enabled        bool   `json:"enabled"`
	BotToken       string `json:"bot_token"`
	AppToken       string `json:"app_token"`
	DefaultChannel string `json:"default_channel"`
	PollEvery      int    `json:"poll_every_seconds"`
}

type DiscordChannelConfig struct {
	Enabled        bool   `json:"enabled"`
	BotToken       string `json:"bot_token"`
	DefaultChannel string `json:"default_channel"`
	PollEvery      int    `json:"poll_every_seconds"`
	APIBaseURL     string `json:"api_base_url"`
	GuildID        string `json:"guild_id"`
	PublicKey      string `json:"public_key"`
	UseGatewayWS   bool   `json:"use_gateway_ws"`
}

type WhatsAppChannelConfig struct {
	Enabled          bool   `json:"enabled"`
	AccessToken      string `json:"access_token"`
	PhoneNumberID    string `json:"phone_number_id"`
	VerifyToken      string `json:"verify_token"`
	DefaultRecipient string `json:"default_recipient"`
	APIVersion       string `json:"api_version"`
	AppSecret        string `json:"app_secret"`
}

type SignalChannelConfig struct {
	Enabled          bool   `json:"enabled"`
	BaseURL          string `json:"base_url"`
	Number           string `json:"number"`
	DefaultRecipient string `json:"default_recipient"`
	PollEvery        int    `json:"poll_every_seconds"`
	BearerToken      string `json:"bearer_token"`
}

type SecurityConfig struct {
	APIToken                 string         `json:"api_token"`
	PublicPaths              []string       `json:"public_paths"`
	ProtectEvents            bool           `json:"protect_events"`
	WebhookSecret            string         `json:"webhook_secret"`
	TrustedCIDRs             []string       `json:"trusted_cidrs"`
	RateLimitRPM             int            `json:"rate_limit_rpm"`
	Users                    []SecurityUser `json:"users"`
	Roles                    []SecurityRole `json:"roles"`
	AuditLog                 string         `json:"audit_log"`
	DangerousCommandPatterns []string       `json:"dangerous_command_patterns"`
	CommandTimeoutSeconds    int            `json:"command_timeout_seconds"`
}

type SecurityUser struct {
	Name                string   `json:"name"`
	Token               string   `json:"token"`
	Role                string   `json:"role"`
	Permissions         []string `json:"permissions"`
	PermissionOverrides []string `json:"permission_overrides"`
	Scopes              []string `json:"scopes"`
	Orgs                []string `json:"orgs"`
	Projects            []string `json:"projects"`
	Workspaces          []string `json:"workspaces"`
}

type SecurityRole struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

type RoutingConfig struct {
	Mode  string               `json:"mode"`
	Rules []ChannelRoutingRule `json:"rules"`
}

type ChannelRoutingRule struct {
	Channel      string `json:"channel"`
	Match        string `json:"match"`
	SessionMode  string `json:"session_mode"`
	SessionID    string `json:"session_id"`
	QueueMode    string `json:"queue_mode"`
	ReplyBack    *bool  `json:"reply_back,omitempty"`
	TitlePrefix  string `json:"title_prefix"`
	Agent        string `json:"agent"`
	Workspace    string `json:"workspace"`
	Org          string `json:"org"`
	Project      string `json:"project"`
	WorkspaceRef string `json:"workspace_ref"`
}

func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
	}

	applyEnvOverrides(cfg)
	return cfg, nil
}

func DefaultConfig() *Config {
	return &Config{
		LLM: LLMConfig{
			Provider:    "openai",
			Model:       "gpt-4o-mini",
			MaxTokens:   4096,
			Temperature: 0.7,
			Routing: ModelRoutingConfig{
				Enabled:           false,
				ReasoningKeywords: []string{"计划", "复杂", "步骤", "自动化", "脚本", "代码", "debug", "plan", "complex", "script", "code"},
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
			Host:                 "127.0.0.1",
			Port:                 18789,
			Bind:                 "loopback",
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
			CommandTimeoutSeconds:    30,
		},
	}
}

func (c *Config) FindAgentProfile(name string) (AgentProfile, bool) {
	needle := strings.TrimSpace(strings.ToLower(name))
	for _, profile := range c.Agent.Profiles {
		if strings.ToLower(strings.TrimSpace(profile.Name)) == needle {
			return profile, true
		}
	}
	return AgentProfile{}, false
}

func (p AgentProfile) IsEnabled() bool {
	return p.Enabled == nil || *p.Enabled
}

func BoolPtr(value bool) *bool {
	return &value
}

func (c *Config) ApplyAgentProfile(name string) bool {
	profile, ok := c.FindAgentProfile(name)
	if !ok {
		return false
	}
	if !profile.IsEnabled() {
		return false
	}
	if profile.Name != "" {
		c.Agent.Name = profile.Name
		c.Agent.ActiveProfile = profile.Name
	}
	if profile.Description != "" {
		c.Agent.Description = profile.Description
	}
	if profile.WorkingDir != "" {
		c.Agent.WorkingDir = profile.WorkingDir
	}
	if profile.PermissionLevel != "" {
		c.Agent.PermissionLevel = profile.PermissionLevel
	}
	if profile.DefaultModel != "" {
		c.LLM.Model = profile.DefaultModel
	}
	if strings.TrimSpace(profile.Persona) != "" {
		c.Agent.Description = strings.TrimSpace(strings.Join([]string{c.Agent.Description, "Persona: " + profile.Persona}, "\n"))
	}
	return true
}

func (c *Config) UpsertAgentProfile(profile AgentProfile) error {
	profile.Name = strings.TrimSpace(profile.Name)
	profile.Description = strings.TrimSpace(profile.Description)
	profile.Role = strings.TrimSpace(profile.Role)
	profile.Persona = strings.TrimSpace(profile.Persona)
	profile.WorkingDir = strings.TrimSpace(profile.WorkingDir)
	profile.PermissionLevel = strings.TrimSpace(profile.PermissionLevel)
	profile.DefaultModel = strings.TrimSpace(profile.DefaultModel)
	profile.Personality.Template = strings.TrimSpace(profile.Personality.Template)
	profile.Personality.Tone = strings.TrimSpace(profile.Personality.Tone)
	profile.Personality.Style = strings.TrimSpace(profile.Personality.Style)
	profile.Personality.GoalOrientation = strings.TrimSpace(profile.Personality.GoalOrientation)
	profile.Personality.ConstraintMode = strings.TrimSpace(profile.Personality.ConstraintMode)
	profile.Personality.ResponseVerbosity = strings.TrimSpace(profile.Personality.ResponseVerbosity)
	profile.Personality.CustomInstructions = strings.TrimSpace(profile.Personality.CustomInstructions)
	for i, trait := range profile.Personality.Traits {
		profile.Personality.Traits[i] = strings.TrimSpace(trait)
	}
	filteredTraits := make([]string, 0, len(profile.Personality.Traits))
	for _, trait := range profile.Personality.Traits {
		if trait != "" {
			filteredTraits = append(filteredTraits, trait)
		}
	}
	profile.Personality.Traits = filteredTraits
	filteredSkills := make([]AgentSkillRef, 0, len(profile.Skills))
	for _, skill := range profile.Skills {
		skill.Name = strings.TrimSpace(skill.Name)
		skill.Version = strings.TrimSpace(skill.Version)
		cleanPerms := make([]string, 0, len(skill.Permissions))
		for _, perm := range skill.Permissions {
			perm = strings.TrimSpace(perm)
			if perm != "" {
				cleanPerms = append(cleanPerms, perm)
			}
		}
		skill.Permissions = cleanPerms
		if skill.Name != "" {
			filteredSkills = append(filteredSkills, skill)
		}
	}
	profile.Skills = filteredSkills
	if profile.Name == "" {
		return os.ErrInvalid
	}
	for i, existing := range c.Agent.Profiles {
		if strings.EqualFold(strings.TrimSpace(existing.Name), profile.Name) {
			c.Agent.Profiles[i] = profile
			return nil
		}
	}
	c.Agent.Profiles = append(c.Agent.Profiles, profile)
	return nil
}

func (c *Config) DeleteAgentProfile(name string) bool {
	needle := strings.TrimSpace(strings.ToLower(name))
	for i, profile := range c.Agent.Profiles {
		if strings.ToLower(strings.TrimSpace(profile.Name)) != needle {
			continue
		}
		c.Agent.Profiles = append(c.Agent.Profiles[:i], c.Agent.Profiles[i+1:]...)
		if strings.EqualFold(strings.TrimSpace(c.Agent.ActiveProfile), strings.TrimSpace(profile.Name)) {
			c.Agent.ActiveProfile = ""
		}
		return true
	}
	return false
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("OPENAI_API_KEY"); v != "" {
		cfg.LLM.APIKey = v
	}
	if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" {
		cfg.LLM.APIKey = v
		cfg.LLM.Provider = "anthropic"
	}
	if v := os.Getenv("LLM_PROVIDER"); v != "" {
		cfg.LLM.Provider = v
	}
	if v := os.Getenv("LLM_MODEL"); v != "" {
		cfg.LLM.Model = v
	}
	if v := os.Getenv("LLM_BASE_URL"); v != "" {
		cfg.LLM.BaseURL = v
	}
	if v := os.Getenv("ANYCLAW_GATEWAY_HOST"); v != "" {
		cfg.Gateway.Host = v
	}
	if v := os.Getenv("ANYCLAW_GATEWAY_BIND"); v != "" {
		cfg.Gateway.Bind = v
	}
	if v := os.Getenv("ANYCLAW_GATEWAY_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil && port > 0 {
			cfg.Gateway.Port = port
		}
	}
	if v := os.Getenv("ANYCLAW_TELEGRAM_BOT_TOKEN"); v != "" {
		cfg.Channels.Telegram.BotToken = v
	}
	if v := os.Getenv("ANYCLAW_TELEGRAM_CHAT_ID"); v != "" {
		cfg.Channels.Telegram.ChatID = v
	}
	if v := os.Getenv("ANYCLAW_SLACK_BOT_TOKEN"); v != "" {
		cfg.Channels.Slack.BotToken = v
	}
	if v := os.Getenv("ANYCLAW_SLACK_APP_TOKEN"); v != "" {
		cfg.Channels.Slack.AppToken = v
	}
	if v := os.Getenv("ANYCLAW_SLACK_DEFAULT_CHANNEL"); v != "" {
		cfg.Channels.Slack.DefaultChannel = v
	}
	if v := os.Getenv("ANYCLAW_DISCORD_BOT_TOKEN"); v != "" {
		cfg.Channels.Discord.BotToken = v
	}
	if v := os.Getenv("ANYCLAW_DISCORD_DEFAULT_CHANNEL"); v != "" {
		cfg.Channels.Discord.DefaultChannel = v
	}
	if v := os.Getenv("ANYCLAW_DISCORD_API_BASE_URL"); v != "" {
		cfg.Channels.Discord.APIBaseURL = v
	}
	if v := os.Getenv("ANYCLAW_DISCORD_GUILD_ID"); v != "" {
		cfg.Channels.Discord.GuildID = v
	}
	if v := os.Getenv("ANYCLAW_DISCORD_PUBLIC_KEY"); v != "" {
		cfg.Channels.Discord.PublicKey = v
	}
	if v := os.Getenv("ANYCLAW_DISCORD_USE_GATEWAY_WS"); v != "" {
		cfg.Channels.Discord.UseGatewayWS = strings.EqualFold(v, "1") || strings.EqualFold(v, "true")
	}
	if v := os.Getenv("ANYCLAW_WHATSAPP_ACCESS_TOKEN"); v != "" {
		cfg.Channels.WhatsApp.AccessToken = v
	}
	if v := os.Getenv("ANYCLAW_WHATSAPP_PHONE_NUMBER_ID"); v != "" {
		cfg.Channels.WhatsApp.PhoneNumberID = v
	}
	if v := os.Getenv("ANYCLAW_WHATSAPP_VERIFY_TOKEN"); v != "" {
		cfg.Channels.WhatsApp.VerifyToken = v
	}
	if v := os.Getenv("ANYCLAW_WHATSAPP_APP_SECRET"); v != "" {
		cfg.Channels.WhatsApp.AppSecret = v
	}
	if v := os.Getenv("ANYCLAW_WHATSAPP_DEFAULT_RECIPIENT"); v != "" {
		cfg.Channels.WhatsApp.DefaultRecipient = v
	}
	if v := os.Getenv("ANYCLAW_SIGNAL_BASE_URL"); v != "" {
		cfg.Channels.Signal.BaseURL = v
	}
	if v := os.Getenv("ANYCLAW_SIGNAL_NUMBER"); v != "" {
		cfg.Channels.Signal.Number = v
	}
	if v := os.Getenv("ANYCLAW_SIGNAL_DEFAULT_RECIPIENT"); v != "" {
		cfg.Channels.Signal.DefaultRecipient = v
	}
	if v := os.Getenv("ANYCLAW_SIGNAL_BEARER_TOKEN"); v != "" {
		cfg.Channels.Signal.BearerToken = v
	}
	if v := os.Getenv("ANYCLAW_API_TOKEN"); v != "" {
		cfg.Security.APIToken = v
	}
	if v := os.Getenv("ANYCLAW_WEBHOOK_SECRET"); v != "" {
		cfg.Security.WebhookSecret = v
	}
	if v := os.Getenv("ANYCLAW_RATE_LIMIT_RPM"); v != "" {
		if rpm, err := strconv.Atoi(v); err == nil && rpm > 0 {
			cfg.Security.RateLimitRPM = rpm
		}
	}
	if v := os.Getenv("ANYCLAW_PLUGIN_EXEC_TIMEOUT"); v != "" {
		if sec, err := strconv.Atoi(v); err == nil && sec > 0 {
			cfg.Plugins.ExecTimeoutSeconds = sec
		}
	}
}

func (c *Config) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
