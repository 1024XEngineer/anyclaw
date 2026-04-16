# AnyClaw Runtime 模块规范分析

## 1. 文档依据

本文基于当前仓库中的真实实现整理，不以目录命名做主判断，而以实际调用链、导出接口和测试约束为依据。

- 启动装配主实现: `pkg/runtime/runtime.go`
- 委派能力实现: `pkg/runtime/delegation.go`
- 启动与目标 runtime 行为测试: `pkg/runtime/runtime_test.go`, `pkg/runtime/runtime_target_app_test.go`, `pkg/runtime/delegation_test.go`, `pkg/runtime/runtime_context_budget_test.go`
- runtime 实例池实现: `pkg/gateway/runtimes.go`
- 会话执行绑定与收尾: `pkg/gateway/gateway.go`
- 任务执行绑定: `pkg/gateway/tasks.go`
- OpenAI 兼容执行入口: `pkg/gateway/openai_api.go`
- Desktop 启动入口: `cmd/anyclaw-desktop/app.go`
- 实际执行内核: `pkg/agent/agent.go`
- 现有架构说明: `docs/ARCHITECTURE.md`

从这些源码可以确认几个当前事实：

- `pkg/runtime` 当前是运行时装配中心，不是完整的 runtime 领域边界。
- runtime 的实例生命周期管理目前不在 `pkg/runtime`，而在 `pkg/gateway/runtimes.go`。
- 真正的请求执行入口主要分散在 Gateway 会话执行、TaskManager 执行、OpenAI 兼容 API 三条链路上。
- Desktop 当前主要承担宿主启动职责，即 `Bootstrap -> gateway.New(app) -> server.Run()`，它本身不是直接调用 `Agent.Run` 的业务执行入口。

## 2. Runtime 总链路

### 2.1 启动装配链

```text
Bootstrap / NewTargetApp
  -> 加载 config
  -> 应用 provider / agent profile
  -> 初始化 secrets store / activation manager
  -> 初始化 workDir / workingDir / workspace bootstrap
  -> 初始化 memory
  -> 初始化 audit logger / security token
  -> 初始化 QMD
  -> 加载 skills
  -> 注册 builtins tools
  -> 加载 plugins
  -> 初始化 LLM client
  -> 创建 Agent
  -> 按配置创建 Orchestrator
  -> 注册 delegate_task
  -> 返回 App
```

当前 `BootPhase` 对外暴露的 phase 名称是：

- `config`
- `storage`
- `security`
- `qmd`
- `skills`
- `tools`
- `plugins`
- `llm`
- `agent`
- `orchestrator`
- `ready`

但真实触发顺序并不完全等同于上面的枚举顺序，而是：

- `config`
- `security`，这里实际表示 secrets 初始化
- `storage`
- `security`，这里实际表示 audit logger 和安全 token 初始化
- `qmd`
- `skills`
- `tools`
- `plugins`
- `llm`
- `agent`
- `orchestrator`
- `ready`

补充说明：

- `security` 当前被复用了两次，一次表示 secrets 初始化，一次表示 audit/token 初始化，语义偏宽。
- `Bootstrap` 当前同时承担配置解析、依赖构造、能力注册、Agent 创建和多 agent 编排启用，函数体已经是“总装厂”。

### 2.2 请求执行链

#### 链路 A: Gateway 会话执行

```text
HTTP / WS / Channel request
  -> Session 创建或复用
  -> RuntimePool.GetOrCreate(agent, org, project, workspace)
  -> runtime.NewTargetApp(...)
  -> targetApp.Agent.SetHistory(session.History)
  -> 注入 browser session / sandbox scope
  -> 某些 API 路径还会注入 approval hook
  -> targetApp.Agent.Run 或 RunStream
  -> Session 持久化 / 事件回写 / tool activity 记录
```

说明：

- channel 执行链会统一注入 `browser session` 和 `sandbox scope`。
- approval hook 目前不是所有 channel 路径都统一注入；较完整的审批注入已经出现在 API 会话路径和 TaskManager 路径中。

#### 链路 B: TaskManager 执行

```text
TaskManager.executeTask
  -> RuntimePool.GetOrCreate(...)
  -> 工作流匹配 / 审批等待 / 会话绑定
  -> targetApp.Agent.SetHistory(...)
  -> 注入 browser session / sandbox scope / approval hook
  -> app.Agent.Run(...)
  -> task/session/evidence 更新
```

#### 链路 C: OpenAI 兼容 API

```text
/v1/chat/completions
  -> RuntimePool.GetOrCreate(agentName, "", "", "")
  -> 直接调用 targetApp.LLM.Chat / StreamChat
  -> 返回 OpenAI 兼容响应
```

说明：

- 这条链路当前确实绕过了 `Agent.Run`，直接调用 `LLM.Chat` 或 `LLM.StreamChat`。
- 但它不是完全脱离 runtime 的“裸 LLM 路径”，因为前面仍然会经过 `RuntimePool.GetOrCreate(...)`。
- 当前 `RuntimePool.GetOrCreate(...)` 仍然依赖 workspace 记录存在，因此 OpenAI 兼容链路实际上还隐含依赖 Gateway 启动时的默认 workspace 准备逻辑，而不是一个完全独立、天然稳定的 runtime contract。

#### 链路 D: Desktop 启动

```text
DesktopApp.launchGateway
  -> runtime.Bootstrap(...)
  -> gateway.New(app)
  -> server.Run(...)
```

说明：

- 这是一条宿主启动链，不是直接的业务执行链。
- Desktop 的价值在于拉起和承载 Gateway，而不是自己直接承担对话执行 facade。

### 2.3 当前 runtime 设计结论

- `App` 是当前 runtime 的事实标准载体，里面聚合了 `Config`、`Agent`、`LLM`、`Memory`、`Skills`、`Tools`、`Plugins`、`Audit`、`Orchestrator`、`Delegation`、`QMD`、`Secrets` 等核心依赖。
- runtime 的“构建”和“获取”已经分裂成两层：
  - 构建在 `pkg/runtime`
  - 缓存与失效在 `pkg/gateway`
- runtime 的“执行”目前也不是单一入口：
  - 会话 / task 路径主要走 `Agent.Run` 或 `Agent.RunStream`
  - OpenAI 兼容模式直接走 `LLM.Chat` 或 `LLM.StreamChat`

## 3. 模块总览

| 模块 | 当前代码位置 | 当前事实职责 | 当前实际公开入口 |
| --- | --- | --- | --- |
| 启动装配模块 | `pkg/runtime/runtime.go` | 统一启动并组装 `App` | `Bootstrap`, `NewApp`, `NewAppFromConfig`, `LoadConfig` |
| 目标 Runtime 工厂模块 | `pkg/runtime/runtime.go` | 为指定 agent/workspace 生成隔离 runtime | `NewTargetApp` |
| 能力装配模块 | `pkg/runtime/runtime.go` | 初始化 secrets、storage、memory、QMD、skills、tools、plugins、llm | 当前没有独立公开入口 |
| Runtime 实例池模块 | `pkg/gateway/runtimes.go` | 获取、复用、淘汰、刷新 runtime 实例 | `NewRuntimePool`, `GetOrCreate`, `List`, `Refresh`, `Invalidate*` |
| 执行绑定模块 | `pkg/gateway/gateway.go`, `pkg/gateway/tasks.go`, `pkg/gateway/openai_api.go` | 把 session/task/api 请求装配成可执行 runtime 上下文 | 目前无独立公共模块，散落在 handler/task 内 |
| 运行执行模块 | `pkg/agent/agent.go`, `pkg/runtime/runtime.go` | 以 Agent 或 LLM 的形式实际执行请求 | `app.Agent.Run`, `app.Agent.RunStream`, `app.LLM.Chat`, `app.LLM.StreamChat` |
| 多 Agent 委派模块 | `pkg/runtime/delegation.go` | 对 Orchestrator 提供委派能力和工具暴露 | `delegate_task` 工具，`app.Delegation` 字段 |

## 4. 模块规范建议

### 4.1 启动装配模块

责任边界：

- 只负责把配置和依赖装配成可运行的 runtime。
- 不直接承载 session、task、channel 等业务逻辑。
- 对外返回稳定的 runtime 句柄，而不是要求调用方理解所有子依赖。

建议输入结构体：

```go
type RuntimeBootstrapRequest struct {
	ConfigPath         string
	ConfigOverride     *config.Config
	TargetAgent        string
	WorkingDirOverride string
	BootObserver       BootProgress
}
```

建议输出结构体：

```go
type RuntimeBootstrapResult struct {
	Runtime    *RuntimeApp
	BootReport RuntimeBootReport
}

type RuntimeBootReport struct {
	ConfigPath string
	WorkDir    string
	WorkingDir string
	Phases     []BootEvent
}
```

建议公开函数：

```go
func BootstrapRuntime(req RuntimeBootstrapRequest) (*RuntimeBootstrapResult, error)
func LoadRuntimeConfig(configPath string) (*config.Config, error)
```

当前源码中的实际公开入口：

- `runtime.Bootstrap`
- `runtime.NewApp`
- `runtime.NewAppFromConfig`
- `runtime.LoadConfig`

实现约束：

- 启动必须保持强顺序，尤其是 config、secrets、storage、tools/plugins、llm、agent、orchestrator 的依赖关系不能随意交换。
- 对 config、storage、skills、plugins、llm 等关键阶段应 fail-fast。
- boot event 的 phase 语义应稳定，建议把当前双重含义的 `security` 拆成 `secrets` 和 `security`。
- `NewApp`、`NewAppFromConfig` 当前更像 legacy 兼容入口，外部真实调用主入口已经是 `Bootstrap`。

### 4.2 目标 Runtime 工厂模块

责任边界：

- 根据目标 agent 和 workspace 构建隔离的 runtime 实例。
- 负责应用 agent profile/provider profile，并决定 runtime 的私有 `WorkDir`。
- 不负责缓存、复用和淘汰。

建议输入结构体：

```go
type RuntimeTargetSpec struct {
	ConfigPath string
	AgentName  string
	WorkingDir string
	Org        string
	Project    string
	Workspace  string
}
```

建议输出结构体：

```go
type RuntimeTargetBuildResult struct {
	Runtime     *RuntimeApp
	RuntimeKey  string
	ProfileName string
	WorkDir     string
	WorkingDir  string
}
```

建议公开函数：

```go
func BuildTargetRuntime(spec RuntimeTargetSpec) (*RuntimeTargetBuildResult, error)
func BuildRuntimeKey(spec RuntimeTargetSpec) string
```

当前源码中的实际公开入口：

- `runtime.NewTargetApp`

实现约束：

- 当前 `WorkDir` 的隔离名基于 `agentName + workingDir` 生成，而不是基于 `agent + org + project + workspace` 全维度。
- 当前 runtime pool 的 key 与 `NewTargetApp` 的 workdir 命名维度并不完全一致，后续若不同层级复用同一路径工作区，可能出现“不同 runtime key，共享同一 workdir”的状态。
- `WorkingDirOverride` 必须优先保留，否则会把目标 workspace 覆盖回 agent profile 默认路径。

### 4.3 能力装配模块

责任边界：

- 负责组装 runtime 依赖面，不负责处理外部请求。
- 输出可注入 Agent 的能力集合。
- 内部可分为 secrets/security、storage、capabilities、model 四个子阶段。

建议输入结构体：

```go
type RuntimeDependencyRequest struct {
	Config     *config.Config
	ConfigPath string
	WorkDir    string
	WorkingDir string
}
```

建议输出结构体：

```go
type RuntimeDependencySet struct {
	SecretsManager *secrets.ActivationManager
	SecretsStore   *secrets.Store
	Audit          *audit.Logger
	Memory         memory.MemoryBackend
	QMD            *qmd.Client
	Skills         *skills.SkillsManager
	Tools          *tools.Registry
	Plugins        *plugin.Registry
	LLM            *llm.ClientWrapper
}
```

建议公开函数：

```go
func BuildRuntimeDependencies(req RuntimeDependencyRequest) (*RuntimeDependencySet, error)
func BuildCapabilitySurface(req RuntimeDependencyRequest, deps *RuntimeDependencySet) error
```

当前源码中的实际公开入口：

- 没有独立公开入口
- 当前全部内嵌在 `runtime.Bootstrap` 内部阶段代码中

实现约束：

- `workingDir` 必须先绝对化并完成 `workspace.EnsureBootstrap`，否则 memory 的每日文件、技能上下文和 bootstrap ritual 都会失效。
- tools 注册必须晚于 skills 加载，因为 `sk.RegisterTools(...)` 依赖已加载的 skills。
- plugins 注册必须晚于 tools registry 和 policy engine 初始化。
- QMD 当前是“能起就接入，失败就告警降级”的软依赖，不应阻断整个 runtime。
- secrets 解析要先于 llm/security token 最终落值。

### 4.4 Runtime 实例池模块

责任边界：

- 管理 runtime 实例的获取、复用、刷新、失效和淘汰。
- 不负责 session 逻辑，也不负责真正执行请求。
- 应作为 runtime 领域服务存在，不应长期埋在 gateway handler 包内。

建议输入结构体：

```go
type RuntimeAcquireRequest struct {
	AgentName  string
	Org        string
	Project    string
	Workspace  string
	ForceBuild bool
	Reason     string
}
```

建议输出结构体：

```go
type RuntimeAcquireResult struct {
	Key        string
	Runtime    *RuntimeApp
	Reused     bool
	CreatedAt  time.Time
	LastUsedAt time.Time
}
```

建议公开函数：

```go
func (p *RuntimePool) Acquire(req RuntimeAcquireRequest) (*RuntimeAcquireResult, error)
func (p *RuntimePool) Refresh(req RuntimeAcquireRequest)
func (p *RuntimePool) List() []RuntimeInfo
func (p *RuntimePool) InvalidateByWorkspace(workspaceID string)
```

当前源码中的实际公开入口：

- `gateway.NewRuntimePool`
- `(*RuntimePool).GetOrCreate`
- `(*RuntimePool).List`
- `(*RuntimePool).Refresh`
- `(*RuntimePool).Invalidate`
- `(*RuntimePool).InvalidateByAgent`
- `(*RuntimePool).InvalidateByWorkspace`
- `(*RuntimePool).InvalidateByProject`
- `(*RuntimePool).InvalidateAll`

实现约束：

- 当前 key 维度是 `agent::org::project::workspace`。
- `GetOrCreate` 强依赖 `store.GetWorkspace(workspaceID)`，即 workspace 记录必须先存在。
- 淘汰策略目前只有 `lastUsedAt` 的简化 LRU，没有活跃引用计数，没有并发构建抑制。
- 当前 pool 位于 `pkg/gateway`，导致 runtime 生命周期管理和网关协议层耦合。

### 4.5 执行绑定模块

责任边界：

- 负责把 session/task/openai 请求绑定到具体 runtime。
- 负责注入运行上下文，包括 history、sandbox、browser session、approval hook。
- 负责执行前后状态更新，但不拥有 Agent 本身。

建议输入结构体：

```go
type RuntimeExecutionBindRequest struct {
	Source      string
	SessionID   string
	AgentName   string
	Org         string
	Project     string
	Workspace   string
	Message     string
	Streaming   bool
	History     []prompt.Message
	Meta        map[string]string
}
```

建议输出结构体：

```go
type RuntimeExecutionContext struct {
	Runtime    *RuntimeApp
	Context    context.Context
	SessionID  string
	RuntimeKey string
}
```

建议公开函数：

```go
func BindChannelExecution(ctx context.Context, req RuntimeExecutionBindRequest) (*RuntimeExecutionContext, error)
func BindTaskExecution(ctx context.Context, req RuntimeExecutionBindRequest) (*RuntimeExecutionContext, error)
func BindModelExecution(ctx context.Context, req RuntimeExecutionBindRequest) (*RuntimeExecutionContext, error)
func FinalizeExecution(result RuntimeExecutionResult) error
```

当前源码中的实际公开入口：

- `Server.prepareChannelExecution`
- `Server.finalizeChannelExecution`
- `TaskManager.executeTask` 内部直接获取 runtime 并拼装执行上下文
- `handleOpenAIChatCompletions` 内部直接获取 runtime

实现约束：

- history 必须在执行前写入 `targetApp.Agent.SetHistory(...)`。
- browser session 和 sandbox scope 已经是会话型执行链的稳定上下文组成。
- approval hook 当前只在部分路径中完整注入，未来若要统一审批与可观测性，需要先统一执行绑定 contract。
- channel、task、openai 现在各自拼装上下文，已经出现 contract 分叉。
- 当前 OpenAI 兼容 API 绕过了 Agent 级 runtime 执行绑定，直接走 LLM，语义上属于“模型代理模式”而非“完整 runtime 模式”。

### 4.6 运行执行模块

责任边界：

- 把用户输入转成模型调用、工具调用和最终文本输出。
- 不负责 runtime 构建和实例缓存。
- 对上层暴露统一执行 facade，而不是暴露 `App.Agent`、`App.LLM` 的原始字段。

建议输入结构体：

```go
type RuntimeInvokeRequest struct {
	Input     string
	Streaming bool
	Context   context.Context
}
```

建议输出结构体：

```go
type RuntimeInvokeResult struct {
	Content        string
	ToolActivities []agent.ToolActivity
	FinishReason   string
}
```

建议公开函数：

```go
func (rt *RuntimeApp) Invoke(ctx context.Context, req RuntimeInvokeRequest) (*RuntimeInvokeResult, error)
func (rt *RuntimeApp) InvokeModel(ctx context.Context, messages []llm.Message, tools []llm.ToolDefinition) (*llm.Response, error)
```

当前源码中的实际公开入口：

- `app.Agent.Run`
- `app.Agent.RunStream`
- `app.LLM.Chat`
- `app.LLM.StreamChat`

实现约束：

- `Run` 和 `RunStream` 应保持结果语义一致，至少在 history、memory、tool activity 记录层面尽量一致。
- 当前 `RunStream` 的普通路径直接流式调用 LLM，但没有像 `Run` 一样追加 assistant history 和 conversation memory，这会导致流式与非流式行为不完全对齐。
- 若继续允许外部直接访问 `App.Agent` 和 `App.LLM`，则 runtime 层难以形成稳定 facade。

### 4.7 多 Agent 委派模块

责任边界：

- 负责把主 agent 的局部任务委托给 orchestrator。
- 既要提供服务接口，也要提供工具暴露接口。
- 不应把“注册工具”和“执行委派”强耦合在一个内部函数里。

建议输入结构体：

```go
type RuntimeDelegationRequest struct {
	Task            string
	AgentNames      []string
	Reason          string
	SuccessCriteria string
	UserContext     string
}
```

建议输出结构体：

```go
type RuntimeDelegationResult struct {
	Status          string
	TaskID          string
	DelegationBrief string
	SelectedAgents  []string
	Summary         string
	ErrorSummary    string
	Stats           orchestrator.TaskStats
	SubTasks        []orchestrator.SubTask
}
```

建议公开函数：

```go
func NewDelegationService(app *RuntimeApp) *DelegationService
func (s *DelegationService) Delegate(ctx context.Context, req RuntimeDelegationRequest) (*RuntimeDelegationResult, error)
func RegisterDelegationTool(registry *tools.Registry, service *DelegationService)
```

当前源码中的实际公开入口：

- `delegate_task` 工具
- `app.Delegation` 字段在 runtime 内部装配后可间接使用

实现约束：

- 委派必须依赖 orchestrator 已启用，否则只能返回 unavailable。
- `delegate_task` 当前只对 main agent 可见，这是正确约束，应保留。
- 工具 handler 当前强制走 `tools.RequestToolApproval(...)`，后续 facade 设计也应保留审批钩子。
- 当前没有导出的 `NewDelegationService` 和 `RegisterDelegationTool`，导致委派能力更像 bootstrap 内部副作用。

## 5. 当前源码与建议规范的差距

### 5.1 事实上的大一统装配器

`runtime.Bootstrap` 当前过重，已经同时承担：

- 配置读取
- profile 解析
- 目录初始化
- secrets 与安全注入
- memory / QMD 初始化
- skills / tools / plugins 注册
- LLM / Agent / Orchestrator 创建

建议把它收口为 facade，把依赖装配拆成独立 builder。

### 5.2 App 是“原始依赖包”，不是稳定 runtime 契约

当前外部普遍直接访问：

- `app.Config`
- `app.Agent`
- `app.LLM`
- `app.WorkDir`
- `app.WorkingDir`

这会让 runtime 很难做演进。建议逐步把外部访问收敛到 `RuntimeApp` facade 方法。

### 5.3 runtime 生命周期管理不在 runtime 域

`RuntimePool` 目前在 `pkg/gateway`，但它管理的是 runtime 实例生命周期，而不是协议层逻辑。建议中期迁移到 `pkg/runtime/pool` 或等价子域。

### 5.4 执行绑定逻辑散落

当前 channel、task、openai 三条链路各自拼装执行上下文，已经出现 contract 分叉：

- channel / task 主要走完整 agent runtime
- openai 兼容接口直接走 LLM
- approval hook 的注入粒度也还没有完全统一

后续若要做统一可观测性、统一审批、统一 tool activity，就需要先统一执行绑定层。

### 5.5 OpenAI 兼容 API 与 runtime pool 契约存在错位

当前 OpenAI 兼容接口通过：

```text
GetOrCreate(agentName, "", "", "")
```

获取 runtime，但 `RuntimePool.GetOrCreate(...)` 仍然要求 `workspaceID` 对应的 workspace 记录存在。现在这条链路依赖 Gateway 启动时预创建默认 workspace 才能工作，契约并不自洽。

如果后续要继续保留这条入口，至少有两个方向：

- 明确为 OpenAI 模式补一层默认 workspace 解析
- 或者把 OpenAI 模式改造成不经过 workspace 绑定的独立 model runtime

### 5.6 流式与非流式执行语义不完全一致

`Agent.Run` 和 `Agent.RunStream` 当前在历史写回和 memory 落盘上存在差异。这个差异如果不在 runtime facade 层统一，会持续扩散到 Gateway、Desktop 和外部 API。

## 6. 建议的 issue 结论

可以把这次讨论收敛成下面这几个明确动作：

1. 把 `runtime.Bootstrap` 收口为 facade，并拆出独立的依赖装配 builder。
2. 为 runtime 定义稳定 facade，逐步减少外部对 `App.Agent`、`App.LLM`、`App.Config` 的直接访问。
3. 把 `RuntimePool` 从 `pkg/gateway` 迁移到 runtime 领域下。
4. 统一 channel、task、openai 三条链路的执行绑定 contract。
5. 明确 OpenAI 兼容 API 的 workspace/runtime 契约，消除当前错位。
6. 对齐 `Run` 与 `RunStream` 的 history / memory / tool activity 语义。

## 7. 一句话结论

AnyClaw 当前已经形成了 runtime 相关的事实模块，但边界尚未收口：`pkg/runtime` 负责构建、`pkg/gateway` 负责生命周期和部分执行绑定、`pkg/agent` 负责最终执行。下一步最值得做的不是继续堆功能，而是先把 runtime 的构建、获取、绑定、执行四层 contract 正式化。
