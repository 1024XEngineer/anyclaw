# AnyClaw 与 OpenClaw 七层架构、协议差异与改造建议

## 0. 文档范围

本文基于本机两个仓库的当前实现做对比：

- `D:\anyclaw\anyclaw-mvp`
- `D:\anyclaw\openclaw`

对比重点不是“谁功能更多”，而是以下四件事：

1. `anyclaw` 和 `openclaw` 在输入层、网关层、路由层、运行层、能力与集成层、状态与治理层、插件与扩展层分别有什么差异。
2. 两者的协议设计有什么差异。
3. `anyclaw` 相比 `openclaw` 的主要短板是什么。
4. 如果要把 `anyclaw` 往 `openclaw` 那种工程化程度推进，应该怎么改。

## 1. 依据文件

### 1.1 AnyClaw

- `docs/ARCHITECTURE.md`
- `docs/runtime-module-spec-analysis.md`
- `pkg/runtime/bootstrap.go`
- `pkg/runtime/app.go`
- `pkg/input/contracts.go`
- `pkg/input/plugin_contracts.go`
- `pkg/route/ingress/router.go`
- `pkg/route/handoff/service.go`
- `pkg/gateway/gateway_routes_platform.go`
- `pkg/gateway/gateway_ws_connection_loop.go`
- `pkg/gateway/gateway_ws_requests_core.go`
- `pkg/gateway/gateway_ws_methods.go`
- `pkg/api/openai/compat.go`
- `pkg/extensions/plugin/manifest_v2.go`
- `pkg/extensions/plugin/loader.go`
- `pkg/extensions/plugin/sdk/sdk.go`
- `pkg/extensions/mcp/bridge.go`
- `pkg/extensions/plugin/mcp_client.go`
- `pkg/qmd/protocol.go`
- `pkg/state/store.go`
- `pkg/state/approvals.go`
- `pkg/state/audit/audit.go`
- `pkg/state/observability/*`

### 1.2 OpenClaw

- `docs/concepts/architecture.md`
- `docs/concepts/agent-loop.md`
- `docs/concepts/session.md`
- `docs/channels/channel-routing.md`
- `docs/gateway/protocol.md`
- `docs/gateway/bridge-protocol.md`
- `docs/gateway/openai-http-api.md`
- `docs/plugins/architecture.md`
- `docs/plugins/sdk-channel-plugins.md`
- `docs/tools/acp-agents.md`
- `docs/cli/mcp.md`
- `src/gateway/server.impl.ts`
- `src/gateway/server-methods-list.ts`
- `src/gateway/protocol/schema.ts`
- `src/gateway/protocol/schema/frames.ts`
- `src/routing/resolve-route.ts`
- `src/config/sessions/store.ts`
- `src/config/zod-schema.ts`
- `src/config/types.openclaw.ts`
- `src/plugins/manifest.ts`
- `src/plugins/manifest-registry.ts`
- `src/plugin-sdk/plugin-entry.ts`
- `src/plugin-sdk/approval-runtime.ts`
- `src/channels/plugins/types.plugin.ts`
- `src/channels/plugins/types.core.ts`
- `src/flows/doctor-health.ts`
- `src/acp/*`

## 2. 先给结论

一句话总结：

- `AnyClaw` 更像一个“本地优先、Go 单体、功能正在快速聚合中的智能体工作台”。
- `OpenClaw` 更像一个“以网关协议、插件契约、会话治理、外部运行时桥接为中心的控制平面平台”。

再说得更直白一点：

- `AnyClaw` 的强项是上手快、部署轻、Go 单体直观、桌面执行和本地工具整合得很自然。
- `OpenClaw` 的强项是边界清晰、协议成熟、扩展接口稳定、治理能力强、对外生态兼容面大。
- `AnyClaw` 现在最明显的问题不是“缺功能”，而是“很多功能已经有了，但缺统一契约、缺统一协议、缺统一治理面”。

## 3. 七层逐层对比

## 3.1 输入层

### AnyClaw 当前实现

- 入口是 `pkg/input` 和 `pkg/input/channels/*`。
- 核心抽象比较轻：`Adapter`、`StreamAdapter`、`InboundHandler`。
- 还存在一套独立的 `ChannelPlugin` 抽象：`pkg/input/plugin_contracts.go`。
- 输入安全策略、私聊策略、配对、允许名单、提及门禁等逻辑分散在 `pkg/input/security.go` 和 `pkg/input/middleware.go`。
- 渠道配置集中在 `pkg/config/types.go` 的 `ChannelsConfig`，当前主要覆盖 Telegram、Slack、Discord、WhatsApp、Signal。

### OpenClaw 当前实现

- 输入面不是单一包，而是“渠道插件 + 路由绑定 + 会话语法”三件套。
- 渠道插件总契约在 `src/channels/plugins/types.plugin.ts`。
- 渠道核心能力拆得很细：配置、配对、安全、群组、提及、出站、状态、网关、鉴权、审批、命令、生命周期、流式处理、线程、消息、目录、解析器、动作等。
- 渠道路由文档 `docs/channels/channel-routing.md` 已经把 `channel / accountId / peer / agentId / sessionKey` 的关系定义成了正式模型。
- 输入层不仅处理“收消息”，还负责线程继承、`guild/team/role` 匹配、会话绑定、审批接口面等。

### 核心区别

- `AnyClaw` 的输入层更像“若干适配器 + 一层安全包裹”。
- `OpenClaw` 的输入层更像“渠道平台”，渠道本身是一级插件能力，不只是消息入口。
- `AnyClaw` 以“消息来了怎么丢给运行时”为主。
- `OpenClaw` 以“消息来自哪里、属于哪个账号、落到哪个会话、由哪个智能体接手、在什么会话语法下回去”为主。

### AnyClaw 的缺点

- 输入模型有两套：`Adapter` 和 `ChannelPlugin`，抽象不统一。
- 渠道安全、配对、路由、消息协议、线程语义没有形成一个稳定的输入总线。
- 渠道能力还是偏“接入器”，不是“可独立治理的插件能力域”。
- 当前渠道配置粒度明显比 `OpenClaw` 粗，缺少账号、对端、线程、群组能力等类型化契约。

### 修改建议

1. 把 `pkg/input/contracts.go` 和 `pkg/input/plugin_contracts.go` 统一成一套正式的 `InboundPlugin`/`ChannelPlugin` 协议。
2. 给输入层定义统一的 `InboundEnvelope`，至少包含：
   - `channel`
   - `account_id`
   - `peer_kind`
   - `peer_id`
   - `thread_id`
   - `sender_id`
   - `message_id`
   - `reply_to`
   - `security_context`
3. 把 pairing、mention gate、allowlist、DM policy 从中间件式分散实现，收口到渠道能力契约里。
4. 把“渠道只负责收消息”的思路升级为“渠道负责会话语法 + 安全 + 出站 + 审批能力”。

## 3.2 网关层

### AnyClaw 当前实现

- 网关主入口在 `pkg/gateway`。
- HTTP 路由比较丰富：OpenAI 兼容、MCP、市场、节点、发现、Webhook。
- WS 协议有自己的帧格式：`type/id/method/event/params/data/ok/error`。
- 握手很轻：服务端发 `connect.challenge`，客户端回 `connect` 并提交 `challenge/nonce`。
- 鉴权主要基于 Bearer Token 或本地 `admin` 上下文，见 `pkg/gateway/auth/middleware.go`。
- 网关同时承担：
  - HTTP/WS 暴露
  - 运行时池生命周期
  - `session/task` 执行绑定
  - 事件流
  - 市场、MCP、节点、配对接口

### OpenClaw 当前实现

- 网关是绝对控制平面核心，见 `docs/concepts/architecture.md` 和 `docs/gateway/protocol.md`。
- 运维客户端、Web UI、CLI、Node 都通过同一条 WS 控制平面连接。
- WS 协议是类型化、版本化、模式驱动的。
- `hello-ok` 有正式结构：`protocol/server/features/snapshot/policy/auth`。
- `connect` 请求包含：
  - `minProtocol / maxProtocol`
  - `client.id/version/platform/mode`
  - `role`
  - `scopes`
  - `caps`
  - `commands`
  - `permissions`
  - `auth`
  - `device.id/publicKey/signature/nonce`
- 网关方法列表本身也是正式接口面，`src/gateway/server-methods-list.ts` 里有完整方法族。
- HTTP OpenAI 接口面走的是“智能体优先契约”，不是简单的提供方代理。

### 核心区别

- `AnyClaw` 的网关更像“多功能 API 服务器”。
- `OpenClaw` 的网关更像“统一控制平面总线”。
- `AnyClaw` 里网关仍然吃掉了很多运行时领域职责。
- `OpenClaw` 里网关更偏向“协议枢纽 + 编排枢纽 + 治理枢纽”。

### AnyClaw 的缺点

- 网关职责过重，和运行时生命周期、任务绑定、会话绑定耦合过深。
- WS 协议没有明确的模式协商和版本协商。
- 没有 `role + scopes + device identity + policy` 这种成熟握手能力，其中 `device identity` 对应的设备身份机制也还缺失。
- 当前 `CheckOrigin` 直接放开，协议和安全边界不够收紧。
- 网关方法集合虽然很多，但没有像 `OpenClaw` 那样形成“正式方法族 + 事件族 + 模式导出”。

### 修改建议

1. 把 `RuntimePool` 从 `pkg/gateway` 迁出，至少迁到 `pkg/runtime/pool`。
2. 给 WS 协议补正式版本字段：
   - `min_protocol`
   - `max_protocol`
   - `role`
   - `scopes`
   - `caps`
   - `device`
   - `policy`
3. 给网关协议补模式定义文件，不要只靠 Go 结构体隐式表达。
4. 重新拆分网关：
   - 协议层
   - 鉴权层
   - 会话门面层
   - 运行时绑定层
   - 事件层
   - `extensions/mcp/node` 扩展层
5. 把 OpenAI HTTP、WS、Webhook、Node、Market 统一到“控制平面协议入口”，而不是“每个处理器自己拼上下文”的方式。

## 3.3 路由层

### AnyClaw 当前实现

- 输入路由主要在 `pkg/route/ingress/router.go`。
- 模型很简单：`mode + rules + match + session_mode + session_id + queue_mode + reply_back + title_prefix`。
- 会话键的构造规则也较简单：`shared / per-message / per-chat`。
- 多智能体交接在 `pkg/route/handoff/service.go`，重点是“选择专长智能体并生成交接摘要”。
- `docs/runtime-module-spec-analysis.md` 也明确指出路由/绑定契约还未统一。

### OpenClaw 当前实现

- 路由是正式一级域：`src/routing/resolve-route.ts`。
- 路由输入包含：
  - `channel`
  - `accountId`
  - `peer`
  - `parentPeer`
  - `guildId`
  - `teamId`
  - `memberRoleIds`
- 绑定优先级非常明确：
  - peer
  - parent peer
  - guild + roles
  - guild
  - team
  - account
  - channel
  - default agent
- 会话键语法也是正式契约，例如：
  - `agent:<agentId>:main`
  - `agent:<agentId>:<channel>:group:<id>`
  - `agent:<agentId>:<channel>:channel:<id>:thread:<id>`
- `session.dmScope`、`identityLinks`、`bindings[]` 都会影响路由和隔离。

### 核心区别

- `AnyClaw` 的路由层是“规则引擎 + 会话键生成器”。
- `OpenClaw` 的路由层是“智能体绑定系统 + 会话语法系统”。
- `AnyClaw` 更偏消息路由。
- `OpenClaw` 更偏组织级会话隔离和上下文归属治理。

### AnyClaw 的缺点

- 路由规则只够覆盖简单 `channel/source` 匹配，难以承载复杂群组、线程、账号、多租户、多智能体绑定。
- 会话键语法过于薄，缺少账号、对端类型、线程继承、身份关联等维度。
- handoff 是存在的，但 `ingress route` 和 `handoff route` 之间没有统一路由契约。
- 路由溯源信息不够强，很难解释“这条消息为什么落到这个 `agent/session`”。

### 修改建议

1. 把 `RoutingConfig` 从字符串规则升级为类型化绑定：
   - `match.channel`
   - `match.account`
   - `match.peer.kind`
   - `match.peer.id`
   - `match.team`
   - `match.guild`
   - `match.roles`
2. 给会话键定义正式语法，而不是在各处拼字符串。
3. 在路由结果里保留 `matched_by`、`route_source`、`binding_id` 等可审计字段。
4. 把 `ingress route`、`channel session route`、`handoff plan route` 合并成统一路由域。

## 3.4 运行层

### AnyClaw 当前实现

- `pkg/runtime/bootstrap.go` 是总装厂。
- 启动顺序是：
  - config
  - secrets/security
  - storage/workspace
  - memory
  - audit/security token
  - QMD
  - skills
  - tools
  - plugins
  - llm
  - agent
  - orchestrator
- `MainRuntime`/`App` 聚合很多依赖：Config、Agent、LLM、Memory、Skills、Tools、Plugins、Audit、Orchestrator、Delegation、QMD、Secrets。
- 实际执行入口却分散：
  - `session/task` 主要走 `Agent.Run`
  - OpenAI 兼容走 `LLM.Chat`/`StreamChat` 的模型路径
- `docs/runtime-module-spec-analysis.md` 已明确指出：
  - 运行时构建和运行时生命周期不在同一域
  - 运行时绑定逻辑分散
  - OpenAI 兼容 API 和运行时契约存在错位

### OpenClaw 当前实现

- 运行层不是一个“总装厂结构体”，而是多个稳定域协作：
  - 网关接单
  - 会话存储绑定
  - `agent` / `agent.wait`
  - 嵌入式执行器
  - 队列通道
  - 工具流
  - 生命周期事件流
  - 对话转录持久化
- `docs/concepts/agent-loop.md` 已把智能体执行循环定义成正式运行契约。
- 支持 OpenClaw 原生子智能体运行时。
- 同时支持 ACP 运行时，把外部宿主执行器作为正式运行时类型接入。
- 运行时不是单一“结构体聚合”，而是“执行循环契约 + 会话契约 + 运行时后端契约”。

### 核心区别

- `AnyClaw` 的运行层偏“启动出来一个 App，然后由不同入口直接调不同字段”。
- `OpenClaw` 的运行层偏“所有 run 都遵守一个正式执行循环契约，再由不同运行时后端执行”。

### AnyClaw 的缺点

- `Bootstrap` 过重。
- `App` 暴露过多原始依赖字段，难以形成稳定门面。
- `Run` 和 `RunStream` 语义对齐还不够强。
- `session/task/openai` 三条链路没有统一执行绑定契约。
- 运行时池在网关域内，导致协议层和运行时生命周期耦合。

### 修改建议

1. 以 `docs/runtime-module-spec-analysis.md` 的建议为起点，正式拆成四层：
   - 运行时引导
   - 运行时池
   - 执行绑定
   - 运行时调用门面
2. 禁止上层长期直接拿 `app.Agent`、`app.LLM` 做业务调用。
3. 让 `OpenAI compat` 也走统一执行门面，明确它是：
   - 完整智能体运行
   - 还是仅模型运行时
4. 给运行层补正式事件：
   - `run.accepted`
   - `run.started`
   - `assistant.delta`
   - `tool.started`
   - `tool.finished`
   - `run.completed`
   - `run.failed`

## 3.5 能力与集成层

### AnyClaw 当前实现

- 能力面很丰富：`capability/models`、`capability/tools`、`capability/skills`、`clihub`、`vision`、`media`、`canvas`、`speech`、`qmd`。
- MCP 通过 `pkg/extensions/mcp` 和 `pkg/extensions/plugin/mcp_client.go` 接入。
- QMD 有自己的一套 `request/response/event/heartbeat` 协议。
- CLI 中枢让本地 CLI 能力自动注册到工具层。
- 工具层、技能层、插件层、MCP 桥接层都能“加能力”，但这几条线并未统一为一个能力契约。

### OpenClaw 当前实现

- 能力层是能力驱动的。
- 插件能力在 `docs/plugins/architecture.md` 已被正式枚举：
  - 文本推理
  - 语音
  - 实时转录
  - 实时语音
  - 媒体理解
  - 图像生成
  - 音乐生成
  - 视频生成
  - 网页抓取
  - 网页搜索
  - 渠道/消息
- 渠道插件、提供方插件、工具/服务插件都通过稳定 SDK 接口面暴露。
- ACP 和 MCP 不是孤立功能，而是运行时/集成协议的一部分。

### 核心区别

- `AnyClaw` 的能力很多，但来源分散。
- `OpenClaw` 的能力多，而且来源被统一编排进能力注册表。

### AnyClaw 的缺点

- 工具、技能、插件、MCP、CLI 中枢都在扩展能力，但没有一个统一能力分类体系。
- “能力是谁拥有”的问题不清楚，经常是核心层、插件、技能都能做。
- QMD、MCP、工具注册表、插件注册表之间缺少统一的注册与消费语义。

### 修改建议

1. 先定义 AnyClaw 能力分类体系，例如：
   - `provider`
   - `tool`
   - `channel`
   - `memory`
   - `node`
   - `workflow`
   - `integration`
2. 所有新能力统一通过能力注册表进入运行时，而不是分散在多个注册入口。
3. 把 `skill` 定位成“提示词/工作流增强”，把 `plugin` 定位成“运行时能力拥有者”。
4. 把 QMD 从“当前引导过程中的特殊子系统”改成“正式的 memory/integration 能力”。

## 3.6 状态与治理层

### AnyClaw 当前实现

- 全局状态主要收在 `pkg/state/store.go` 的 `state.json`。
- `sessions`、`events`、`tools`、`tasks`、`approvals`、`audit`、`orgs`、`projects`、`workspaces`、`jobs` 都能落进去。
- 审批在 `pkg/state/approvals.go`。
- 审计在 `pkg/state/audit/audit.go`，是 JSONL 追加日志。
- 可观测在 `pkg/state/observability/*`，自带 `metrics/gauge/histogram`。
- `secrets` 有 activation/store，但治理链路还偏轻。
- 网关鉴权主要是 `token/user/role/permissions` 组合。

### OpenClaw 当前实现

- 配置有完整类型化接口面：`src/config/types.openclaw.ts`。
- 配置校验有完整 zod 模式：`src/config/zod-schema.ts`。
- 会话存储是独立子域：`src/config/sessions/store.ts`。
- 对话转录、会话维护、磁盘预算、锁、缓存、迁移都是正式组成部分。
- `doctor` 是正式治理流程，不是简单的诊断命令。
- 审批也是正式子域：执行审批、插件审批、原生审批能力。
- 鉴权模式比 AnyClaw 明显成熟：
  - none
  - token
  - password
  - trusted-proxy
  - tailscale identity
  - device token
  - bootstrap token

### 核心区别

- `AnyClaw` 的治理更像“系统能力的伴生模块”。
- `OpenClaw` 的治理更像“平台第一能力”。
- `AnyClaw` 有状态和审计。
- `OpenClaw` 则把配置、会话、审批、安全、doctor、迁移都协议化、制度化了。

### AnyClaw 的缺点

- `state.json` 粒度偏粗，多个领域状态混在一起。
- 配置模式约束强度不够，不像 OpenClaw 那样是模式优先。
- 缺少 `doctor`、迁移、兼容性审计这类平台级修复入口。
- 审批、插件审批、原生审批还没有细分成正式治理域。
- 安全治理还偏“有配置项”，没有形成“持续治理机制”。

### 修改建议

1. 拆状态域：
   - `sessions`
   - `tasks`
   - `approvals`
   - `audit`
   - `catalog/install`
   - `runtime health`
2. 把配置升级为模式优先，至少做到：
   - load
   - validate
   - normalize
   - migrate
   - audit
3. 增加 `anyclaw doctor` 和 `anyclaw security audit` 双入口。
4. `secrets`、`approvals`、插件信任、会话维护都要进入治理闭环。

## 3.7 插件与扩展层

### AnyClaw 当前实现

- 插件侧至少有：
  - `pkg/extensions/plugin/manifest_v2.go`
  - `pkg/extensions/plugin/loader.go`
  - `pkg/extensions/plugin/sdk/sdk.go`
- 加载器非常宽，支持 `json`、`go`、`python`、`node`、`rust`、`binary`、`wasm`、`mcp`、`openclaw`、`claude`、`cursor` 等格式。
- `manifest V2` 覆盖工具、`ingress`、`channel`、`node`、`surface`、`workflow_pack`、`risk`、`approval`、`health`、`lifecycle hooks` 等字段。
- 插件 SDK 相对简单，本质是：
  - 注册工具
  - 注册渠道
  - 注册事件处理器
  - 注册 HTTP 路由
  - 注册节点
- 同时仓库里还有 `skills/`、`plugins/`、`extensions/` 三套扩展形态。

### OpenClaw 当前实现

- 扩展层是整个系统的正式一级架构。
- `openclaw.plugin.json` 是规范清单。
- 清单注册表会做：
  - 发现
  - 启用
  - 校验
  - 兼容性检查
  - 契约归属判定
  - 安装/运行提示
- 插件 SDK 有明确入口点，例如 `src/plugin-sdk/plugin-entry.ts`。
- 插件不是简单注册工具，而是注册能力、服务、命令、安全审计收集器、运行时辅助器。
- 文档上已经明确区分：
  - 清单与发现
  - 启用与校验
  - 运行时加载
  - 接口消费

### 核心区别

- `AnyClaw` 的插件层很灵活，但更像“格式兼容层 + 执行挂载层”。
- `OpenClaw` 的插件层是“平台契约层”。

### AnyClaw 的缺点

- 加载器支持的格式很多，但稳定公共契约反而更弱。
- `plugin / extension / skill` 三套概念还没有完全统一。
- SDK 还是偏注册式，没有充分体现能力归属、配置契约、doctor 契约、审批契约。
- `manifest` 虽然字段很多，但“发现/校验/运行时/接口面”还没有被分成清晰阶段。

### 修改建议

1. 先确定唯一规范清单，建议统一成 `anyclaw.plugin.json`。
2. 继续保留多格式加载器，但把它们视为“兼容入口”，不要当主契约。
3. 正式区分四阶段：
   - 发现
   - 启用
   - 校验
   - 运行时加载
4. 把 `skills` 和 `extensions` 收口到插件平台之下：
   - `skill = 提示词/工作流制品`
   - `extension/plugin = 运行时能力制品`
5. 提供稳定 SDK 子路径，而不是把所有能力都堆在一个大 `PluginAPI` 上。

## 4. 协议差异

## 4.1 网关 WebSocket 协议

### AnyClaw

- 自定义 WS 帧：`req/res/event`
- 服务端先发 `connect.challenge`
- 客户端回 `connect`
- `connect` 主要校验 `challenge/nonce`
- 返回内容主要是：
  - `status`
  - `protocol`
  - `connected_at`
  - `user`
  - `methods`

### OpenClaw

- 同样是 WS，但已经正式做了模式化和版本化。
- `connect` 明确协商协议版本。
- 区分 `operator` 与 `node`。
- 支持 `scopes/caps/commands/permissions`。
- 支持设备身份和签名挑战。
- `hello-ok` 还包含：
  - 功能发现
  - 快照
  - 策略
  - 可选设备令牌

### 本质差异

- `AnyClaw` 的 WS 协议更像“够用的内部协议”。
- `OpenClaw` 的 WS 协议更像“对外稳定控制平面协议”。

## 4.2 HTTP / OpenAI 兼容协议

### AnyClaw

- 开箱暴露：
  - `POST /v1/chat/completions`
  - `GET /v1/models`
  - `POST /v1/responses`
- 兼容面更像提供方/模型门面。
- 当前实现走 `targetApp.Chat`，本质是模型路径，不是完整智能体执行循环。

### OpenClaw

- OpenAI HTTP 接口面默认关闭，需要显式开启。
- 除 `chat/completions` 外，还支持：
  - `GET /v1/models/{id}`
  - `POST /v1/embeddings`
  - `POST /v1/responses`
- `model` 字段不是“提供方模型 ID”，而是“智能体目标”。
- 这是典型的智能体优先契约。

### 本质差异

- `AnyClaw` 更像“兼容 OpenAI 客户端”。
- `OpenClaw` 更像“把 OpenAI 协议映射到自己的智能体控制平面”。

## 4.3 路由与会话协议

### AnyClaw

- 会话模式主要是字符串：
  - `shared`
  - `per-message`
  - `per-chat`
- 路由规则主要靠 `channel + match`。
- 会话键语法比较松散。

### OpenClaw

- 路由/绑定有正式优先级。
- 会话键语法是正式契约。
- 还有私聊隔离、`identityLinks`、线程继承、会话绑定。

### 本质差异

- `AnyClaw` 是“规则路由”。
- `OpenClaw` 是“绑定路由 + 会话隔离协议”。

## 4.4 MCP 协议

### AnyClaw

- 支持 MCP 客户端。
- 通过 `pkg/extensions/plugin/mcp_client.go` 用 JSON-RPC 2.0 和 MCP 服务器通信。
- 启动后把外部 MCP 工具桥接到本地工具注册表，命名为 `mcp__<server>__<tool>`。
- 网关还暴露 `/mcp/*` HTTP 接口用于列举和调用。

### OpenClaw

- 同时把 MCP 当成服务端和客户端两种能力。
- `openclaw mcp serve` 可以把 OpenClaw 会话暴露给外部 MCP 客户端。
- `mcp.servers` 配置支持：
  - `stdio`
  - `sse`
  - `streamable-http`
- 等于说 `OpenClaw` 已经把 MCP 做成平台级桥接协议。

### 本质差异

- `AnyClaw` 的 MCP 更偏“把外部工具接进来”。
- `OpenClaw` 的 MCP 更偏“双向桥接总线”。

## 4.5 ACP 协议

### AnyClaw

- 当前没有与 `OpenClaw ACP` 对等的正式 ACP 运行时协议层。
- 多智能体协作主要还是内部编排器 + `handoff plan`。

### OpenClaw

- ACP 是正式运行时类型。
- 可以把 Codex、Claude Code、Gemini CLI 等外部宿主执行器作为 ACP 会话运行。
- ACP 会话有自己独立的会话键语法、控制命令、后台任务跟踪和会话绑定。

### 本质差异

- `AnyClaw` 的协作偏内部编排。
- `OpenClaw` 的协作已经扩展到“外部智能体运行时的协议接入”。

## 4.6 插件协议

### AnyClaw

- 清单字段很多，但 SDK 以注册工具、渠道、HTTP 路由为主。
- 多格式加载器说明兼容性思路很强，但公共协议边界还不够硬。

### OpenClaw

- `openclaw.plugin.json` + 清单注册表 + 插件 SDK 入口点构成稳定三件套。
- 能力归属、配置契约、审批运行时、渠道运行时都有正式接口面。

### 本质差异

- `AnyClaw` 的插件协议偏“开放装配”。
- `OpenClaw` 的插件协议偏“稳定契约”。

## 4.7 状态治理协议

### AnyClaw

- 状态落盘、审计、审批都存在，但大多还是内部数据结构。
- 没有形成很强的配置迁移、兼容性审计、`doctor` 协议。

### OpenClaw

- 配置模式、会话存储、`doctor`、审批、`trusted-proxy` 鉴权、设备令牌、原生审批都是正式治理契约。

### 本质差异

- `AnyClaw` 的治理偏“实现层能力”。
- `OpenClaw` 的治理偏“产品级制度层能力”。

## 5. AnyClaw 对比 OpenClaw 的主要缺点

这里的“缺点”不是否定 `AnyClaw`，而是站在工程化、平台化、协议化角度说它目前还不够成熟的地方。

### 5.1 边界不够硬

- 运行时、网关、会话、任务之间的边界还在互相穿透。
- `docs/runtime-module-spec-analysis.md` 已经明确承认这一点。

### 5.2 协议成熟度不够

- 网关 WS 没有像 `OpenClaw` 一样成为类型化、版本化契约。
- OpenAI 兼容层也没有完全升格成智能体优先契约。

### 5.3 路由模型过于简化

- 对单机 demo 足够。
- 对多渠道、多账号、多线程、多智能体、多租户场景明显不够。

### 5.4 扩展面过宽但不够稳定

- 加载器支持很多格式，这是优点。
- 但“支持很多格式”不等于“有稳定插件平台”。

### 5.5 运行时入口不统一

- `session/task/openai` 三条执行链不是一个契约。
- 这会导致可观测性、审批、历史、记忆很难完全对齐。

### 5.6 治理能力偏轻

- 有 `state`、`audit`、`approval`。
- 但缺模式优先配置、doctor、兼容性迁移、插件治理这种平台级治理面。

### 5.7 对外生态协议不够完整

- MCP 已经接入，但更多是客户端桥接。
- 缺 ACP 这种正式外部智能体运行时协议层。

## 6. AnyClaw 应该怎么改

## 6.1 第一阶段：先收口契约，不先追新功能

目标：让系统从“能跑”变成“边界清楚”。

### 必做项

1. 运行时门面化
   - 收口 `Bootstrap`
   - 收口 `App`
   - 把执行绑定独立出来
2. 网关协议化
   - 给 WS 补模式定义
   - 给 `connect/hello` 补版本和策略
   - 补 `role/scopes/device identity` 等握手要素
3. 路由类型化
   - 用绑定取代当前粗粒度匹配规则
   - 正式定义会话键语法
4. 插件规范化
   - 统一 `anyclaw.plugin.json`
   - 继续保留多加载器，但降级为兼容层

## 6.2 第二阶段：把“能力来源”统一成能力注册表

目标：把 `skill`、`tool`、`plugin`、MCP、CLI 中枢统一成一张能力图。

### 必做项

1. 定义能力类型。
2. 所有能力注册都进入统一注册表。
3. 运行时只消费注册表，不直接感知每个来源。
4. 新功能优先通过能力扩展，不再往核心处理器里硬塞。

## 6.3 第三阶段：把状态治理正式化

目标：让 `AnyClaw` 从“本地智能体工具”升级为“可运维平台”。

### 必做项

1. 拆分状态存储。
2. 增加模式优先配置。
3. 增加 `doctor`、`security audit`、`compat migration`。
4. 审批体系拆为：
   - `exec approval`
   - `plugin approval`
   - `session approval`
5. `secrets` 和插件信任形成统一治理面。

## 6.4 第四阶段：补外部生态协议

目标：不只是内部多智能体，而是正式连接外部智能体运行时。

### 优先顺序

1. 先把网关 WS 做稳。
2. 再把 MCP `server/client` 双向桥打通。
3. 如果要做 Codex/Claude Code 这类宿主集成，再考虑引入 ACP 风格运行时。

## 7. 推荐的落地路线图

## P0：立刻做

- 把 `RuntimePool` 从 `pkg/gateway` 搬出去。
- 给 WS 握手补版本、`role`、`scopes`、`policy`。
- 给路由增加类型化绑定和会话键语法。
- 把 OpenAI 兼容层明确成智能体运行时还是模型运行时。

## P1：一个迭代内做

- 统一 `plugin/extension/skill` 术语和归属。
- 统一能力注册表。
- 拆分状态存储。
- 上 `doctor` 和配置迁移。

## P2：中期做

- 把 Node、MCP、外部智能体运行时做成正式桥接协议。
- 把审批、安全审计、插件信任做成平台治理面。

## 8. 最后的判断

如果把两者放在同一条演进坐标上：

- `AnyClaw` 现在更像一个“功能已经很多、产品形态已经出现，但平台契约还没完全成型”的系统。
- `OpenClaw` 则已经走到了“控制平面先行、插件契约先行、治理先行”的阶段。

所以 `AnyClaw` 真正该补的不是更多零散功能，而是三件基础设施：

1. 正式协议
2. 正式边界
3. 正式治理

只要这三件补起来，`AnyClaw` 现有的 Go 单体、本地优先、桌面执行、QMD、本地工具链整合这些优势，反而会变得更清晰，也更容易形成自己的差异化路线。
