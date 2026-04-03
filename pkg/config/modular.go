package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// ConfigValidator 配置验证器接口
type ConfigValidator interface {
	Validate(config *Config) error
}

// ConfigMigrator 配置迁移器接口
type ConfigMigrator interface {
	Migrate(config *Config, fromVersion, toVersion string) (*Config, error)
}

// ModularConfigManager 模块化配置管理器
type ModularConfigManager struct {
	mu               sync.RWMutex
	baseConfig       *Config
	modules          map[string]ConfigModule
	validators       []ConfigValidator
	migrators        []ConfigMigrator
	envOverrides     map[string]string
	runtimeOverrides map[string]interface{}
	configPath       string
	version          string
}

// ConfigModule 配置模块接口
type ConfigModule interface {
	Name() string
	Load(configPath string) error
	Save(configPath string) error
	Validate() error
	GetDefaults() interface{}
}

// NewModularConfigManager 创建模块化配置管理器
func NewModularConfigManager(configPath string) *ModularConfigManager {
	return &ModularConfigManager{
		modules:          make(map[string]ConfigModule),
		validators:       make([]ConfigValidator, 0),
		migrators:        make([]ConfigMigrator, 0),
		envOverrides:     make(map[string]string),
		runtimeOverrides: make(map[string]interface{}),
		configPath:       configPath,
		version:          "1.0.0",
	}
}

// RegisterModule 注册配置模块
func (m *ModularConfigManager) RegisterModule(module ConfigModule) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.modules[module.Name()]; exists {
		return fmt.Errorf("module %s already registered", module.Name())
	}

	m.modules[module.Name()] = module
	return nil
}

// RegisterValidator 注册配置验证器
func (m *ModularConfigManager) RegisterValidator(validator ConfigValidator) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.validators = append(m.validators, validator)
}

// RegisterMigrator 注册配置迁移器
func (m *ModularConfigManager) RegisterMigrator(migrator ConfigMigrator) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.migrators = append(m.migrators, migrator)
}

// Load 加载配置
func (m *ModularConfigManager) Load() (*Config, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 1. 读取基础配置
	config, err := m.readConfigFile(m.configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	// 2. 应用环境变量覆盖
	config = m.applyEnvOverrides(config)

	// 3. 验证配置
	if err := m.validateConfig(config); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	// 4. 应用运行时覆盖
	config = m.applyRuntimeOverrides(config)

	// 5. 加载模块
	if err := m.loadModules(config); err != nil {
		return nil, fmt.Errorf("failed to load modules: %w", err)
	}

	m.baseConfig = config
	return config, nil
}

// Save 保存配置
func (m *ModularConfigManager) Save() error {
	m.mu.RLock()
	config := m.baseConfig
	m.mu.RUnlock()

	if config == nil {
		return fmt.Errorf("no config loaded")
	}

	// 1. 保存模块
	if err := m.saveModules(); err != nil {
		return fmt.Errorf("failed to save modules: %w", err)
	}

	// 2. 保存基础配置
	if err := m.saveConfigFile(m.configPath, config); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	return nil
}

// GetConfig 获取配置
func (m *ModularConfigManager) GetConfig() *Config {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.baseConfig
}

// SetEnvOverride 设置环境变量覆盖
func (m *ModularConfigManager) SetEnvOverride(key, value string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.envOverrides[key] = value
}

// SetRuntimeOverride 设置运行时覆盖
func (m *ModularConfigManager) SetRuntimeOverride(key string, value interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.runtimeOverrides[key] = value
}

// GetModule 获取配置模块
func (m *ModularConfigManager) GetModule(name string) (ConfigModule, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	module, exists := m.modules[name]
	if !exists {
		return nil, fmt.Errorf("module %s not found", name)
	}

	return module, nil
}

// Validate 验证配置
func (m *ModularConfigManager) Validate() error {
	m.mu.RLock()
	config := m.baseConfig
	validators := m.validators
	m.mu.RUnlock()

	if config == nil {
		return fmt.Errorf("no config loaded")
	}

	var errors []string
	for _, validator := range validators {
		if err := validator.Validate(config); err != nil {
			errors = append(errors, err.Error())
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("config validation failed: %s", strings.Join(errors, "; "))
	}

	return nil
}

// Migrate 迁移配置
func (m *ModularConfigManager) Migrate(targetVersion string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.baseConfig == nil {
		return fmt.Errorf("no config loaded")
	}

	// 按版本顺序应用迁移器
	for _, migrator := range m.migrators {
		config, err := migrator.Migrate(m.baseConfig, m.version, targetVersion)
		if err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
		m.baseConfig = config
	}

	m.version = targetVersion
	return nil
}

// readConfigFile 读取配置文件
func (m *ModularConfigManager) readConfigFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return m.getDefaultConfig(), nil
		}
		return nil, err
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &config, nil
}

// saveConfigFile 保存配置文件
func (m *ModularConfigManager) saveConfigFile(path string, config *Config) error {
	// 确保目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// applyEnvOverrides 应用环境变量覆盖
func (m *ModularConfigManager) applyEnvOverrides(config *Config) *Config {
	// 应用环境变量覆盖
	for key, value := range m.envOverrides {
		switch key {
		case "ANYCLAW_LLM_PROVIDER":
			config.LLM.Provider = value
		case "ANYCLAW_LLM_MODEL":
			config.LLM.Model = value
		case "ANYCLAW_LLM_API_KEY":
			config.LLM.APIKey = value
		case "ANYCLAW_LLM_BASE_URL":
			config.LLM.BaseURL = value
		case "ANYCLAW_LLM_PROXY":
			config.LLM.Proxy = value
		case "ANYCLAW_AGENT_NAME":
			config.Agent.Name = value
		case "ANYCLAW_AGENT_WORK_DIR":
			config.Agent.WorkDir = value
		case "ANYCLAW_GATEWAY_HOST":
			config.Gateway.Host = value
		case "ANYCLAW_GATEWAY_PORT":
			// 转换为整数
			var port int
			if _, err := fmt.Sscanf(value, "%d", &port); err == nil {
				config.Gateway.Port = port
			}
		}
	}

	// 也从系统环境变量读取
	if provider := os.Getenv("ANYCLAW_LLM_PROVIDER"); provider != "" {
		config.LLM.Provider = provider
	}
	if model := os.Getenv("ANYCLAW_LLM_MODEL"); model != "" {
		config.LLM.Model = model
	}
	if apiKey := os.Getenv("ANYCLAW_LLM_API_KEY"); apiKey != "" {
		config.LLM.APIKey = apiKey
	}
	if baseURL := os.Getenv("ANYCLAW_LLM_BASE_URL"); baseURL != "" {
		config.LLM.BaseURL = baseURL
	}
	if proxy := os.Getenv("ANYCLAW_LLM_PROXY"); proxy != "" {
		config.LLM.Proxy = proxy
	}

	return config
}

// applyRuntimeOverrides 应用运行时覆盖
func (m *ModularConfigManager) applyRuntimeOverrides(config *Config) *Config {
	// 应用运行时覆盖
	for key, value := range m.runtimeOverrides {
		switch key {
		case "llm.provider":
			if v, ok := value.(string); ok {
				config.LLM.Provider = v
			}
		case "llm.model":
			if v, ok := value.(string); ok {
				config.LLM.Model = v
			}
		case "llm.temperature":
			if v, ok := value.(float64); ok {
				config.LLM.Temperature = v
			}
		case "agent.permission_level":
			if v, ok := value.(string); ok {
				config.Agent.PermissionLevel = v
			}
		}
	}

	return config
}

// validateConfig 验证配置
func (m *ModularConfigManager) validateConfig(config *Config) error {
	var errors []string

	// 基础验证
	if config.LLM.Provider == "" {
		errors = append(errors, "llm.provider is required")
	}

	if config.LLM.Model == "" {
		errors = append(errors, "llm.model is required")
	}

	// 验证温度范围
	if config.LLM.Temperature < 0 || config.LLM.Temperature > 2 {
		errors = append(errors, "llm.temperature must be between 0 and 2")
	}

	// 验证网关端口
	if config.Gateway.Port < 0 || config.Gateway.Port > 65535 {
		errors = append(errors, "gateway.port must be between 0 and 65535")
	}

	// 验证权限级别
	validPermissionLevels := map[string]bool{
		"read-only": true,
		"limited":   true,
		"full":      true,
	}
	if !validPermissionLevels[config.Agent.PermissionLevel] {
		errors = append(errors, "agent.permission_level must be one of: read-only, limited, full")
	}

	if len(errors) > 0 {
		return fmt.Errorf("config validation failed: %s", strings.Join(errors, "; "))
	}

	return nil
}

// loadModules 加载模块
func (m *ModularConfigManager) loadModules(config *Config) error {
	for name, module := range m.modules {
		// 创建模块配置目录
		moduleDir := filepath.Join(filepath.Dir(m.configPath), "modules", name)
		if err := os.MkdirAll(moduleDir, 0o755); err != nil {
			return fmt.Errorf("failed to create module directory for %s: %w", name, err)
		}

		// 加载模块
		moduleConfigPath := filepath.Join(moduleDir, "config.json")
		if err := module.Load(moduleConfigPath); err != nil {
			// 如果配置文件不存在，使用默认配置
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("failed to load module %s: %w", name, err)
		}
	}

	return nil
}

// saveModules 保存模块
func (m *ModularConfigManager) saveModules() error {
	for name, module := range m.modules {
		// 验证模块
		if err := module.Validate(); err != nil {
			return fmt.Errorf("module %s validation failed: %w", name, err)
		}

		// 保存模块
		moduleDir := filepath.Join(filepath.Dir(m.configPath), "modules", name)
		moduleConfigPath := filepath.Join(moduleDir, "config.json")
		if err := module.Save(moduleConfigPath); err != nil {
			return fmt.Errorf("failed to save module %s: %w", name, err)
		}
	}

	return nil
}

// getDefaultConfig 获取默认配置
func (m *ModularConfigManager) getDefaultConfig() *Config {
	return &Config{
		LLM: LLMConfig{
			Provider:    "openai",
			Model:       "gpt-4o-mini",
			Temperature: 0.7,
			MaxTokens:   4096,
		},
		Agent: AgentConfig{
			Name:                            "AnyClaw",
			PermissionLevel:                 "full",
			RequireConfirmationForDangerous: true,
			Profiles: []AgentProfile{
				{
					Name:            "default",
					Description:     "Default agent profile",
					PermissionLevel: "full",
				},
			},
		},
		Skills: SkillsConfig{
			Dir:      "skills",
			AutoLoad: true,
		},
		Memory: MemoryConfig{
			Dir:        "memory",
			MaxHistory: 100,
			Format:     "json",
			AutoSave:   true,
		},
		Gateway: GatewayConfig{
			Host:        "localhost",
			Port:        18789,
			Bind:        "loopback",
			WorkerCount: 4,
		},
		Channels: ChannelsConfig{
			Routing: RoutingConfig{
				Mode: "auto",
			},
		},
		Sandbox: SandboxConfig{
			Enabled:       false,
			ExecutionMode: "host-reviewed",
		},
		Security: SecurityConfig{
			ProtectedPaths: []string{
				"~/.ssh",
				"~/.gnupg",
				"~/.config",
			},
			CommandTimeoutSeconds: 30,
		},
	}
}
