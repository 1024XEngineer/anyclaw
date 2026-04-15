package core

import (
	"context"
	"encoding/json"
	"time"
)

//
// Protocol layer
//

// FrameType 表示网关线协议的三类基础帧。
// 它定义了控制平面通信的最外层包络类型。
type FrameType string

const (
	FrameReq   FrameType = "req"
	FrameRes   FrameType = "res"
	FrameEvent FrameType = "event"
)

// RequestFrame 表示客户端或节点发给网关的一次请求。
// 它承载 RPC 调用的方法名和参数。
type RequestFrame struct {
	Type   FrameType       `json:"type"`
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// ResponseFrame 表示网关对某个请求的响应。
// 它用于同步返回成功结果或结构化错误。
type ResponseFrame struct {
	Type    FrameType       `json:"type"`
	ID      string          `json:"id"`
	OK      bool            `json:"ok"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   *ErrorPayload   `json:"error,omitempty"`
}

// EventFrame 表示网关主动推送的事件。
// 它用于流式通知 agent 状态、presence、chat 更新等异步信息。
type EventFrame struct {
	Type         FrameType `json:"type"`
	Event        string    `json:"event"`
	Payload      any       `json:"payload,omitempty"`
	Seq          uint64    `json:"seq,omitempty"`
	StateVersion uint64    `json:"stateVersion,omitempty"`
}

// ErrorPayload 表示统一错误模型。
// 它保证网关、agent、工具等模块返回错误时可机器处理。
type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

//
// Identity and auth layer
//

// Role 表示连接到网关的主体角色。
// operator 代表控制端，node 代表能力宿主设备。
type Role string

const (
	RoleOperator Role = "operator"
	RoleNode     Role = "node"
)

// ClientInfo 描述连接端自身的信息。
// 用于识别客户端类型、平台和版本。
type ClientInfo struct {
	ID       string `json:"id"`
	Version  string `json:"version"`
	Platform string `json:"platform"`
	Mode     string `json:"mode"`
}

// DeviceIdentity 描述设备身份。
// 它用于设备指纹、签名校验和配对模型。
type DeviceIdentity struct {
	ID        string `json:"id"`
	PublicKey string `json:"publicKey"`
	Signature string `json:"signature"`
	SignedAt  int64  `json:"signedAt"`
	Nonce     string `json:"nonce"`
}

// ConnectParams 表示 connect 握手请求参数。
// 它是网关建立连接时最重要的协议对象。
type ConnectParams struct {
	MinProtocol int            `json:"minProtocol"`
	MaxProtocol int            `json:"maxProtocol"`
	Client      ClientInfo     `json:"client"`
	Role        Role           `json:"role"`
	Scopes      []string       `json:"scopes"`
	Caps        []string       `json:"caps,omitempty"`
	Commands    []string       `json:"commands,omitempty"`
	Permissions map[string]any `json:"permissions,omitempty"`
	Locale      string         `json:"locale,omitempty"`
	UserAgent   string         `json:"userAgent,omitempty"`
	Device      DeviceIdentity `json:"device"`
}

// Principal 表示通过网关鉴权后的调用主体。
// 后续 RPC、agent 调度、节点调用都会带着这个身份执行。
type Principal struct {
	Role     Role
	DeviceID string
	Scopes   []string
	Client   ClientInfo
}

//
// Gateway layer
//

// GatewayMethodHandler 表示一个网关 RPC 方法处理器。
// 它负责处理单个 method 的请求，不关心网络连接和底层 transport。
type GatewayMethodHandler interface {
	Handle(ctx context.Context, principal Principal, req RequestFrame) (any, *ErrorPayload)
}

// Gateway 是系统的控制平面入口接口。
// 它负责启动服务、注册 RPC 方法、广播事件和优雅关闭。
type Gateway interface {
	Start(ctx context.Context) error
	Shutdown(ctx context.Context) error
	RegisterMethod(method string, handler GatewayMethodHandler)
	PublishEvent(ctx context.Context, event string, payload any) error
}

//
// Routing layer
//

// RouteInput 表示一条入站消息的路由输入。
// 它包含渠道、账号、对端、群组等决定 agent/session 的关键字段。
type RouteInput struct {
	Channel       string
	AccountID     string
	PeerID        string
	PeerKind      string
	ParentPeerID  string
	GuildID       string
	TeamID        string
	MemberRoleIDs []string
}

// RouteResult 表示路由决策结果。
// 它明确这条消息应该进入哪个 agent 和哪个 session。
type RouteResult struct {
	AgentID        string
	Channel        string
	AccountID      string
	SessionKey     string
	MainSessionKey string
	MatchedBy      string
}

// Router 是路由解析器接口。
// 它把外部消息上下文映射为系统内部的 agent/session 归属。
type Router interface {
	Resolve(ctx context.Context, in RouteInput) (RouteResult, error)
}

//
// Session layer
//

// ChatMessage 表示会话中的标准消息对象。
// 它用于 session 历史、模型上下文和最终持久化。
type ChatMessage struct {
	ID        string
	Role      string
	Text      string
	CreatedAt time.Time
	Metadata  map[string]any
}

// Session 表示单个会话实例。
// 它负责追加消息和读取历史，不负责跨 session 的调度。
type Session interface {
	ID() string
	Key() string
	Append(ctx context.Context, msg ChatMessage) error
	History(ctx context.Context, limit int) ([]ChatMessage, error)
}

// SessionStore 是会话存储抽象。
// 它负责按 sessionKey 获取或创建会话，并隐藏底层 SQLite 或文件存储。
type SessionStore interface {
	GetOrCreate(ctx context.Context, sessionKey string) (Session, error)
}

// SessionLane 是单个 session 的串行执行通道。
// 它保证同一 session 上的 agent run 和工具执行不会乱序并发。
type SessionLane interface {
	Submit(ctx context.Context, fn func(context.Context) error) error
}

//
// Agent layer
//

// AgentRunRequest 表示一次 agent 执行请求。
// 它把路由结果、用户输入、模型偏好和元数据打包为一次运行单元。
type AgentRunRequest struct {
	RunID       string
	AgentID     string
	SessionKey  string
	UserMessage ChatMessage
	Model       string
	ProviderID  string
	Metadata    map[string]any
}

// AgentEventType 表示 agent 运行过程中产生的事件类别。
// lifecycle、assistant、tool 三类足以覆盖 MVP 流式语义。
type AgentEventType string

const (
	AgentLifecycle AgentEventType = "lifecycle"
	AgentAssistant AgentEventType = "assistant"
	AgentTool      AgentEventType = "tool"
)

// AgentEvent 表示一次 agent run 的流式输出事件。
// 它用于向网关和 UI 持续推送执行进展。
type AgentEvent struct {
	RunID string
	Type  AgentEventType
	Name  string
	Data  map[string]any
}

// AgentRunner 是 agent 执行引擎接口。
// 它负责完成一次完整的上下文装配、模型调用、工具调用和结果输出。
type AgentRunner interface {
	Run(ctx context.Context, req AgentRunRequest) (<-chan AgentEvent, error)
}

//
// Provider layer
//

// ModelRequest 是发给模型提供商的标准请求。
// 它统一封装消息历史、工具声明和模型选择。
type ModelRequest struct {
	Model    string
	Messages []ChatMessage
	Tools    []ToolSpec
	Metadata map[string]any
}

// ModelChunk 表示模型流式返回的一块内容。
// 它可以是文本增量、工具调用、使用量信息或完成信号。
type ModelChunk struct {
	Type  string
	Text  string
	Tool  *ToolCall
	Usage map[string]any
	Done  bool
}

// Provider 是模型提供商接口。
// 它负责把标准模型请求转换为某个厂商的真实 API 调用。
type Provider interface {
	ID() string
	ListModels(ctx context.Context) ([]string, error)
	Stream(ctx context.Context, req ModelRequest) (<-chan ModelChunk, error)
}

// ProviderRegistry 是模型提供商注册表。
// 它负责 provider 的注册、查询和枚举。
type ProviderRegistry interface {
	Register(provider Provider)
	Get(id string) (Provider, bool)
	List() []Provider
}

//
// Tool layer
//

// ToolSpec 描述一个工具的静态定义。
// 它告诉模型和运行时这个工具叫什么、做什么、参数模式是什么。
type ToolSpec struct {
	Name        string
	Description string
	Schema      map[string]any
}

// ToolCall 表示模型请求执行某个工具的一次调用。
// 它包含工具名称和结构化参数。
type ToolCall struct {
	ID   string
	Name string
	Args map[string]any
}

// ToolResult 表示工具执行结果。
// 它同时支持用户可见输出、内部元数据和错误信息。
type ToolResult struct {
	Output  any
	Error   string
	Meta    map[string]any
	Visible bool
}

// Tool 是统一工具接口。
// 它负责暴露工具声明，并处理一次具体调用。
type Tool interface {
	Name() string
	Spec() ToolSpec
	Invoke(ctx context.Context, call ToolCall) (ToolResult, error)
}

// ToolRegistry 是工具注册表。
// 它统一管理 agent 可调用的工具集合。
type ToolRegistry interface {
	Register(tool Tool)
	Get(name string) (Tool, bool)
	List() []Tool
}

//
// Channel layer
//

// InboundMessage 表示渠道侧收到的一条标准化消息。
// 所有 Telegram、WebChat、Slack 等接入层都应先归一化到这个结构。
type InboundMessage struct {
	Channel   string
	AccountID string
	PeerID    string
	PeerKind  string
	Text      string
	MessageID string
	Raw       map[string]any
}

// OutboundMessage 表示系统准备发回渠道的一条标准化消息。
// 它是统一的渠道发送模型。
type OutboundMessage struct {
	Channel   string
	AccountID string
	TargetID  string
	Text      string
	ReplyTo   string
	Metadata  map[string]any
}

// Channel 是消息渠道接口。
// 它负责启动渠道监听、接收消息并发送消息，但不直接做路由或 agent 调度。
type Channel interface {
	ID() string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Send(ctx context.Context, msg OutboundMessage) error
	SetInboundHandler(func(context.Context, InboundMessage) error)
}

// ChannelRegistry 是渠道注册表。
// 它用于集中管理所有可用消息渠道。
type ChannelRegistry interface {
	Register(ch Channel)
	Get(id string) (Channel, bool)
	List() []Channel
}

//
// Node layer
//

// NodeCommand 表示发给某个 node 的能力调用请求。
// 它统一表达 camera、screen、location、canvas 等操作。
type NodeCommand struct {
	Name string
	Args map[string]any
}

// NodeResult 表示节点能力调用结果。
// 它兼容文本结果、结构化输出和错误信息。
type NodeResult struct {
	Output any
	Error  string
}

// Node 是能力节点接口。
// 它代表一个连接到网关的设备能力宿主。
type Node interface {
	ID() string
	Caps() []string
	Invoke(ctx context.Context, cmd NodeCommand) (NodeResult, error)
}

//
// Hook layer
//

// HookPoint 表示可插入逻辑的生命周期节点。
// 这是插件和内部扩展挂载逻辑的统一坐标。
type HookPoint string

const (
	HookBeforeModelResolve HookPoint = "before_model_resolve"
	HookBeforePromptBuild  HookPoint = "before_prompt_build"
	HookBeforeToolCall     HookPoint = "before_tool_call"
	HookAfterToolCall      HookPoint = "after_tool_call"
	HookAgentEnd           HookPoint = "agent_end"
)

// HookContext 表示 hook 执行时拿到的上下文。
// 它携带当前 run、agent、session 和扩展数据。
type HookContext struct {
	RunID      string
	AgentID    string
	SessionKey string
	Data       map[string]any
}

// Hook 是生命周期扩展接口。
// 它允许在 agent 或 gateway 的关键阶段插入附加行为。
type Hook interface {
	Point() HookPoint
	Run(ctx context.Context, hc HookContext) error
}

//
// Plugin layer
//

// Plugin 是插件的顶层注册接口。
// 它用于把 provider、channel、tool、hook 等能力装配进系统。
type Plugin interface {
	ID() string
	Register(ctx context.Context, app AppContext) error
}

// AppContext 是插件注册阶段看到的应用装配上下文。
// 它向插件暴露有限的系统能力，不暴露内部实现细节。
type AppContext interface {
	Gateway() Gateway
	Router() Router
	Sessions() SessionStore
	Providers() ProviderRegistry
	Tools() ToolRegistry
	Channels() ChannelRegistry
	RegisterHook(h Hook)
}
