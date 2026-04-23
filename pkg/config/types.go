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
	Speech       SpeechConfig       `json:"speech"`
	MCP          MCPConfig          `json:"mcp"`
}

type MCPConfig struct {
	Enabled bool              `json:"enabled"`
	Servers []MCPServerConfig `json:"servers"`
}

type MCPServerConfig struct {
	Name        string            `json:"name"`
	Command     string            `json:"command"`
	Args        []string          `json:"args,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	Transport   string            `json:"transport"`
	Enabled     bool              `json:"enabled"`
	AutoConnect bool              `json:"auto_connect"`
	TimeoutSec  int               `json:"timeout_seconds"`
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
	Lang                            string          `json:"lang,omitempty"`
	WorkFocus                       string          `json:"work_focus,omitempty"`
	BehaviorStyle                   string          `json:"behavior_style,omitempty"`
	Constraints                     string          `json:"constraints,omitempty"`
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
	Host                 string                 `json:"host"`
	Port                 int                    `json:"port"`
	Bind                 string                 `json:"bind"`
	ControlUI            GatewayControlUIConfig `json:"control_ui"`
	WorkerCount          int                    `json:"worker_count"`
	RuntimeMaxInstances  int                    `json:"runtime_max_instances"`
	RuntimeIdleSeconds   int                    `json:"runtime_idle_seconds"`
	JobWorkerCount       int                    `json:"job_worker_count"`
	JobMaxAttempts       int                    `json:"job_max_attempts"`
	JobRetryDelaySeconds int                    `json:"job_retry_delay_seconds"`
	CanvasMaxVersions    int                    `json:"canvas_max_versions"`
}

type GatewayControlUIConfig struct {
	BasePath string `json:"base_path"`
	Root     string `json:"root"`
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
	Security ChannelSecurityConfig `json:"security"`
}

type ChannelSecurityConfig struct {
	DMPolicy         string   `json:"dm_policy"`
	GroupPolicy      string   `json:"group_policy"`
	AllowFrom        []string `json:"allow_from"`
	PairingEnabled   bool     `json:"pairing_enabled"`
	PairingTTLHours  int      `json:"pairing_ttl_hours"`
	MentionGate      bool     `json:"mention_gate"`
	RiskAcknowledged bool     `json:"risk_acknowledged"`
	DefaultDenyDM    bool     `json:"default_deny_dm"`

	pairingEnabledSet bool
	mentionGateSet    bool
	defaultDenyDMSet  bool
}

func (c ChannelSecurityConfig) PairingEnabledSet() bool {
	return c.pairingEnabledSet
}

func (c ChannelSecurityConfig) MentionGateSet() bool {
	return c.mentionGateSet
}

func (c ChannelSecurityConfig) DefaultDenyDMSet() bool {
	return c.defaultDenyDMSet
}

type TelegramChannelConfig struct {
	Enabled        bool   `json:"enabled"`
	BotToken       string `json:"bot_token"`
	ChatID         string `json:"chat_id"`
	PollEvery      int    `json:"poll_every_seconds"`
	StreamReply    bool   `json:"stream_reply"`
	StreamInterval int    `json:"stream_interval_ms"`
}

type SlackChannelConfig struct {
	Enabled        bool   `json:"enabled"`
	BotToken       string `json:"bot_token"`
	AppToken       string `json:"app_token"`
	DefaultChannel string `json:"default_channel"`
	PollEvery      int    `json:"poll_every_seconds"`
	StreamReply    bool   `json:"stream_reply"`
	StreamInterval int    `json:"stream_interval_ms"`
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
	StreamReply    bool   `json:"stream_reply"`
	StreamInterval int    `json:"stream_interval_ms"`
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
	RiskAcknowledged         bool           `json:"risk_acknowledged"`
	SecurityAudit            []string       `json:"security_audit,omitempty"`
	PairingEnabled           bool           `json:"pairing_enabled"`
	PairingTTLHours          int            `json:"pairing_ttl_hours"`
	PairingMaxDevices        int            `json:"pairing_max_devices"`
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
	Role            string   `json:"role,omitempty"`
	ParentRef       string   `json:"parent_ref,omitempty"`
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
	Channel     string `json:"channel"`
	Match       string `json:"match"`
	SessionMode string `json:"session_mode"`
	SessionID   string `json:"session_id"`
	QueueMode   string `json:"queue_mode"`
	ReplyBack   *bool  `json:"reply_back,omitempty"`
	TitlePrefix string `json:"title_prefix"`
	// Deprecated: ingress routing no longer selects agents directly.
	Agent string `json:"agent,omitempty"`
	// Deprecated: ingress routing no longer selects workspaces directly.
	Workspace string `json:"workspace,omitempty"`
	// Deprecated: ingress routing no longer selects orgs directly.
	Org string `json:"org,omitempty"`
	// Deprecated: ingress routing no longer selects projects directly.
	Project string `json:"project,omitempty"`
	// Deprecated: ingress routing no longer selects workspace refs directly.
	WorkspaceRef string `json:"workspace_ref,omitempty"`
}

type SpeechConfig struct {
	STT STTConfigSection `json:"stt"`
	TTS TTSConfigSection `json:"tts"`
}

type STTConfigSection struct {
	Enabled          bool            `json:"enabled"`
	AutoSTT          bool            `json:"auto_stt"`
	Provider         string          `json:"provider"`
	Model            string          `json:"model"`
	APIKey           string          `json:"api_key"`
	BaseURL          string          `json:"base_url"`
	DefaultLang      string          `json:"default_lang"`
	MaxDurationSec   int             `json:"max_duration_seconds"`
	MinConfidence    float64         `json:"min_confidence"`
	TimeoutSec       int             `json:"timeout_seconds"`
	Channels         map[string]bool `json:"channels"`
	ExcludeChannels  map[string]bool `json:"exclude_channels"`
	FallbackToVoice  bool            `json:"fallback_to_voice"`
	AppendTranscript bool            `json:"append_transcript"`
	TriggerPrefix    string          `json:"trigger_prefix"`
}

type TTSConfigSection struct {
	Enabled         bool            `json:"enabled"`
	AutoTTS         bool            `json:"auto_tts"`
	Provider        string          `json:"provider"`
	APIKey          string          `json:"api_key"`
	BaseURL         string          `json:"base_url"`
	Voice           string          `json:"voice"`
	Speed           float64         `json:"speed"`
	Format          string          `json:"format"`
	TimeoutSec      int             `json:"timeout_seconds"`
	Channels        map[string]bool `json:"channels"`
	ExcludeChannels map[string]bool `json:"exclude_channels"`
	FallbackToText  bool            `json:"fallback_to_text"`
	TriggerPrefix   string          `json:"trigger_prefix"`
}
