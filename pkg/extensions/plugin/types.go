package plugin

type PluginType string

const (
	PluginTypeTool          PluginType = "tool"
	PluginTypeChannel       PluginType = "channel"
	PluginTypeSkill         PluginType = "skill"
	PluginTypeModelProvider PluginType = "model-provider"
	PluginTypeSpeech        PluginType = "speech"
	PluginTypeMCP           PluginType = "mcp"
	PluginTypeContextEngine PluginType = "context-engine"
	PluginTypeNode          PluginType = "node"
	PluginTypeSurface       PluginType = "surface"
	PluginTypeIngress       PluginType = "ingress"
	PluginTypeWorkflowPack  PluginType = "workflow-pack"
)

type PluginSpec interface {
	GetName() string
	GetType() PluginType
}

type ModelProviderSpec struct {
	Name         string   `json:"name"`
	Provider     string   `json:"provider"`
	BaseURL      string   `json:"base_url,omitempty"`
	APIKeyEnv    string   `json:"api_key_env,omitempty"`
	Models       []string `json:"models"`
	Capabilities []string `json:"capabilities"`
}

func (s *ModelProviderSpec) GetName() string     { return s.Name }
func (s *ModelProviderSpec) GetType() PluginType { return PluginTypeModelProvider }

type SpeechSpec struct {
	Name       string `json:"name"`
	Provider   string `json:"provider"`
	STTEnabled bool   `json:"stt_enabled"`
	TTSEnabled bool   `json:"tts_enabled"`
	Language   string `json:"language,omitempty"`
	Voice      string `json:"voice,omitempty"`
	SampleRate int    `json:"sample_rate,omitempty"`
	APIKeyEnv  string `json:"api_key_env,omitempty"`
	BaseURL    string `json:"base_url,omitempty"`
}

func (s *SpeechSpec) GetName() string     { return s.Name }
func (s *SpeechSpec) GetType() PluginType { return PluginTypeSpeech }

type MCPSpec struct {
	Name         string            `json:"name"`
	Command      string            `json:"command"`
	Args         []string          `json:"args,omitempty"`
	Env          map[string]string `json:"env,omitempty"`
	Transport    string            `json:"transport"`
	Capabilities []string          `json:"capabilities"`
}

func (s *MCPSpec) GetName() string     { return s.Name }
func (s *MCPSpec) GetType() PluginType { return PluginTypeMCP }

type ContextEngineSpec struct {
	Name        string             `json:"name"`
	Provider    string             `json:"provider"`
	Type        string             `json:"type"`
	Config      map[string]any     `json:"config,omitempty"`
	VectorStore *VectorStoreConfig `json:"vector_store,omitempty"`
}

type VectorStoreConfig struct {
	Provider  string `json:"provider"`
	Dimension int    `json:"dimension"`
	IndexType string `json:"index_type"`
	Endpoint  string `json:"endpoint,omitempty"`
	APIKeyEnv string `json:"api_key_env,omitempty"`
}

func (s *ContextEngineSpec) GetName() string     { return s.Name }
func (s *ContextEngineSpec) GetType() PluginType { return PluginTypeContextEngine }

func (m *Manifest) SupportsType(t PluginType) bool {
	for _, k := range m.Kinds {
		if string(t) == k {
			return true
		}
	}
	return false
}

func (m *Manifest) GetSpec() PluginSpec {
	if m.Tool != nil {
		return m.Tool
	}
	if m.Channel != nil {
		return m.Channel
	}
	if m.ModelProvider != nil {
		return m.ModelProvider
	}
	if m.Speech != nil {
		return m.Speech
	}
	if m.MCP != nil {
		return m.MCP
	}
	if m.ContextEngine != nil {
		return m.ContextEngine
	}
	if m.Node != nil {
		return m.Node
	}
	if m.Surface != nil {
		return m.Surface
	}
	if m.Ingress != nil {
		return m.Ingress
	}
	return nil
}

func (m *Manifest) HasCapability(cap string) bool {
	for _, tag := range m.CapabilityTags {
		if tag == cap {
			return true
		}
	}
	return false
}

func (s *ToolSpec) GetName() string     { return s.Name }
func (s *ToolSpec) GetType() PluginType { return PluginTypeTool }

func (s *IngressSpec) GetName() string     { return s.Name }
func (s *IngressSpec) GetType() PluginType { return PluginTypeIngress }

func (s *ChannelSpec) GetName() string     { return s.Name }
func (s *ChannelSpec) GetType() PluginType { return PluginTypeChannel }

func (s *NodeSpec) GetName() string     { return s.Name }
func (s *NodeSpec) GetType() PluginType { return PluginTypeNode }

func (s *SurfaceSpec) GetName() string     { return s.Name }
func (s *SurfaceSpec) GetType() PluginType { return PluginTypeSurface }
