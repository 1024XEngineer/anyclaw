package config

type Config struct {
	LLM          LLMConfig          `json:"llm"`
	Agent        AgentConfig        `json:"agent"`
	Providers    []ProviderProfile  `json:"providers,omitempty"`
	Skills       SkillsConfig       `json:"skills"`
	Memory       MemoryConfig       `json:"memory"`
	Gateway      GatewayConfig      `json:"gateway"`
	Daemon       DaemonConfig       `json:"daemon"`
	Channels     ChannelsConfig     `json:"channels"`
	Plugins      PluginsConfig      `json:"plugins"`
	Sandbox      SandboxConfig      `json:"sandbox"`
	Security     SecurityConfig     `json:"security"`
	Orchestrator OrchestratorConfig `json:"orchestrator"`
}

type LLMConfig struct {
	Provider           string             `json:"provider"`
	Model              string             `json:"model"`
	APIKey             string             `json:"api_key"`
	BaseURL            string             `json:"base_url"`
	DefaultProviderRef string             `json:"default_provider_ref,omitempty"`
	MaxTokens          int                `json:"max_tokens"`
	Temperature        float64            `json:"temperature"`
	Proxy              string             `json:"proxy"`
	Extra              map[string]string  `json:"extra"`
	Routing            ModelRoutingConfig `json:"routing"`
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
	Name                            string          `json:"name"`
	Description                     string          `json:"description"`
	WorkDir                         string          `json:"work_dir"`
	WorkingDir                      string          `json:"working_dir"`
	PermissionLevel                 string          `json:"permission_level"`
	RequireConfirmationForDangerous bool            `json:"require_confirmation_for_dangerous"`
	Skills                          []AgentSkillRef `json:"skills,omitempty"`
	Profiles                        []AgentProfile  `json:"profiles"`
	ActiveProfile                   string          `json:"active_profile"`
}

type AgentProfile struct {
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	Role            string          `json:"role,omitempty"`
	Persona         string          `json:"persona,omitempty"`
	AvatarPreset    string          `json:"avatar_preset,omitempty"`
	AvatarDataURL   string          `json:"avatar_data_url,omitempty"`
	Domain          string          `json:"domain,omitempty"`
	Expertise       []string        `json:"expertise,omitempty"`
	SystemPrompt    string          `json:"system_prompt,omitempty"`
	WorkingDir      string          `json:"working_dir"`
	PermissionLevel string          `json:"permission_level"`
	ProviderRef     string          `json:"provider_ref,omitempty"`
	DefaultModel    string          `json:"default_model,omitempty"`
	Enabled         *bool           `json:"enabled,omitempty"`
	Personality     PersonalitySpec `json:"personality,omitempty"`
	Skills          []AgentSkillRef `json:"skills,omitempty"`
}

type ProviderProfile struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Type         string            `json:"type,omitempty"`
	Provider     string            `json:"provider"`
	BaseURL      string            `json:"base_url,omitempty"`
	APIKey       string            `json:"api_key,omitempty"`
	DefaultModel string            `json:"default_model,omitempty"`
	Capabilities []string          `json:"capabilities,omitempty"`
	Enabled      *bool             `json:"enabled,omitempty"`
	Extra        map[string]string `json:"extra,omitempty"`
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
	WorkerCount          int    `json:"worker_count"`
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
	ExecutionMode  string `json:"execution_mode"`
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
	ProtectedPaths           []string       `json:"protected_paths"`
	AllowedReadPaths         []string       `json:"allowed_read_paths,omitempty"`
	AllowedWritePaths        []string       `json:"allowed_write_paths,omitempty"`
	AllowedEgressDomains     []string       `json:"allowed_egress_domains,omitempty"`
	CommandTimeoutSeconds    int            `json:"command_timeout_seconds"`
}

type OrchestratorConfig struct {
	Enabled             bool             `json:"enabled"`
	MaxConcurrentAgents int              `json:"max_concurrent_agents"`
	MaxRetries          int              `json:"max_retries"`
	TimeoutSeconds      int              `json:"timeout_seconds"`
	EnableDecomposition bool             `json:"enable_decomposition"`
	AgentNames          []string         `json:"agent_names,omitempty"`
	SubAgents           []SubAgentConfig `json:"sub_agents,omitempty"`
}

type SubAgentConfig struct {
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	Personality     string   `json:"personality,omitempty"`
	PrivateSkills   []string `json:"private_skills"`
	PermissionLevel string   `json:"permission_level"`
	WorkingDir      string   `json:"working_dir,omitempty"`

	LLMProvider    string   `json:"llm_provider,omitempty"`
	LLMModel       string   `json:"llm_model,omitempty"`
	LLMAPIKey      string   `json:"llm_api_key,omitempty"`
	LLMBaseURL     string   `json:"llm_base_url,omitempty"`
	LLMMaxTokens   *int     `json:"llm_max_tokens,omitempty"`
	LLMTemperature *float64 `json:"llm_temperature,omitempty"`
	LLMProxy       string   `json:"llm_proxy,omitempty"`
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
