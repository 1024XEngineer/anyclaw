const NAV_ITEMS = [
  { id: "workbench", label: "首页", icon: "chat", hint: "从聊天输入框开始，快速发起会话、任务与助手操作", aliases: ["workbench", "首页", "工作台", "会话", "聊天"] },
  { id: "overview", label: "控制台", icon: "layout", hint: "查看网关状态、运行概览与控制台摘要", aliases: ["overview", "控制台", "总览"] },
  { id: "tasks", label: "任务", icon: "checklist", hint: "创建、查看并追踪任务执行过程", aliases: ["tasks", "任务"] },
  { id: "approvals", label: "审批", icon: "shield", hint: "审核并处理高风险工具操作", aliases: ["approvals", "审批"] },
  { id: "agents", label: "智能体", icon: "bot", hint: "管理智能体、权限和默认配置", aliases: ["agents", "智能体", "助手"] },
  { id: "model-center", label: "模型中心", icon: "cpu", hint: "管理供应商、密钥与按智能体切换模型", aliases: ["modelcenter", "model-center", "模型中心", "供应商", "模型"] },
  { id: "resources", label: "资源", icon: "folder", hint: "管理组织、项目与工作区", aliases: ["resources", "资源", "工作区"] },
  { id: "observability", label: "观测", icon: "activity", hint: "查看事件、运行时、作业与审计轨迹", aliases: ["observability", "观测", "事件", "运行时", "审计"] },
  { id: "store", label: "商店", icon: "store", hint: "浏览并安装打包好的智能体能力", aliases: ["store", "商店"] },
  { id: "settings", label: "设置", icon: "settings", hint: "查看网关状态与访问控制设置", aliases: ["settings", "设置"] },
];

const UI_ICONS = {
  chat: `
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round">
      <path d="M7 10h10" />
      <path d="M7 14h6" />
      <path d="M5 18l-1.5 2V6.8A2.8 2.8 0 0 1 6.3 4h11.4a2.8 2.8 0 0 1 2.8 2.8v8.4a2.8 2.8 0 0 1-2.8 2.8H5z" />
    </svg>
  `,
  layout: `
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round">
      <rect x="3.5" y="4" width="17" height="16" rx="2.5" />
      <path d="M9 4v16" />
      <path d="M9 10h11.5" />
    </svg>
  `,
  checklist: `
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round">
      <path d="M9.5 7H20" />
      <path d="M9.5 12H20" />
      <path d="M9.5 17H20" />
      <path d="M4 7l1.6 1.6L8 6.2" />
      <path d="M4 12l1.6 1.6L8 11.2" />
      <path d="M4 17l1.6 1.6L8 16.2" />
    </svg>
  `,
  shield: `
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round">
      <path d="M12 3.5 5.5 6v5.5c0 4 2.6 7.6 6.5 9 3.9-1.4 6.5-5 6.5-9V6L12 3.5z" />
      <path d="m9.5 12 1.7 1.7 3.4-3.7" />
    </svg>
  `,
  bot: `
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round">
      <path d="M12 3v3" />
      <rect x="5" y="8" width="14" height="10" rx="3" />
      <path d="M8 18v2.5" />
      <path d="M16 18v2.5" />
      <path d="M8 12h.01" />
      <path d="M16 12h.01" />
      <path d="M3 11v4" />
      <path d="M21 11v4" />
    </svg>
  `,
  cpu: `
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round">
      <rect x="7" y="7" width="10" height="10" rx="2" />
      <path d="M10 1.5v3" />
      <path d="M14 1.5v3" />
      <path d="M10 19.5v3" />
      <path d="M14 19.5v3" />
      <path d="M19.5 10h3" />
      <path d="M19.5 14h3" />
      <path d="M1.5 10h3" />
      <path d="M1.5 14h3" />
    </svg>
  `,
  folder: `
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round">
      <path d="M3.5 7.5A2.5 2.5 0 0 1 6 5h4l2 2h6A2.5 2.5 0 0 1 20.5 9.5v7A2.5 2.5 0 0 1 18 19H6a2.5 2.5 0 0 1-2.5-2.5v-9z" />
    </svg>
  `,
  activity: `
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round">
      <path d="M3.5 12h4l2.4-4 3.2 8 2.2-4h5.2" />
    </svg>
  `,
  store: `
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round">
      <path d="M4 9.5 5.3 5h13.4L20 9.5" />
      <path d="M5 9.5h14v8A2.5 2.5 0 0 1 16.5 20h-9A2.5 2.5 0 0 1 5 17.5v-8z" />
      <path d="M9 13h6" />
    </svg>
  `,
  settings: `
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round">
      <circle cx="12" cy="12" r="3" />
      <path d="M19.4 15a1 1 0 0 0 .2 1.1l.1.1a2 2 0 0 1 0 2.8 2 2 0 0 1-2.8 0l-.1-.1a1 1 0 0 0-1.1-.2 1 1 0 0 0-.6.9V20a2 2 0 0 1-4 0v-.2a1 1 0 0 0-.6-.9 1 1 0 0 0-1.1.2l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1 1 0 0 0 .2-1.1 1 1 0 0 0-.9-.6H4a2 2 0 0 1 0-4h.2a1 1 0 0 0 .9-.6 1 1 0 0 0-.2-1.1l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1 1 0 0 0 1.1.2 1 1 0 0 0 .6-.9V4a2 2 0 0 1 4 0v.2a1 1 0 0 0 .6.9 1 1 0 0 0 1.1-.2l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1 1 0 0 0-.2 1.1 1 1 0 0 0 .9.6h.2a2 2 0 0 1 0 4h-.2a1 1 0 0 0-.9.6z" />
    </svg>
  `,
  pin: `
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round">
      <path d="m14 4 6 6" />
      <path d="m16.5 9.5-5.8 5.8" />
      <path d="M10 3.5 8.5 5l2.2 2.2-4.8 4.8a2 2 0 0 0 0 2.8l.3.3 4.8-4.8L13.2 13l1.5-1.5" />
      <path d="M6 18 3.5 20.5" />
    </svg>
  `,
  unpin: `
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round">
      <path d="m14 4 6 6" />
      <path d="m16.5 9.5-5.8 5.8" />
      <path d="M10 3.5 8.5 5l2.2 2.2-4.8 4.8a2 2 0 0 0 0 2.8l.3.3 4.8-4.8L13.2 13l1.5-1.5" />
      <path d="M6 18 3.5 20.5" />
      <path d="M3 3 21 21" />
    </svg>
  `,
};

const PROVIDER_PRESETS = [
  {
    id: "openai",
    name: "OpenAI",
    type: "openai",
    provider: "openai",
    defaultModel: "gpt-4o-mini",
    capabilities: ["chat", "reasoning", "vision", "tool-calling"],
  },
  {
    id: "anthropic",
    name: "Anthropic",
    type: "anthropic",
    provider: "anthropic",
    defaultModel: "claude-sonnet-4-7",
    capabilities: ["chat", "reasoning"],
  },
  {
    id: "qwen",
    name: "通义千问",
    type: "qwen",
    provider: "qwen",
    defaultModel: "qwen-plus",
    capabilities: ["chat", "reasoning"],
  },
  {
    id: "ollama",
    name: "Ollama",
    type: "ollama",
    provider: "ollama",
    baseURL: "http://127.0.0.1:11434",
    defaultModel: "llama3.2",
    capabilities: ["chat", "local"],
  },
  {
    id: "openai-compatible",
    name: "OpenAI 兼容接口",
    type: "openai-compatible",
    provider: "openai",
    defaultModel: "custom-model",
    capabilities: ["chat", "reasoning", "custom-endpoint"],
  },
];

const MODEL_CENTER_TABS = [
  { id: "bindings", label: "智能体绑定" },
  { id: "providers", label: "供应商池" },
  { id: "secrets", label: "密钥与连接" },
];

const OBSERVABILITY_TABS = [
  { id: "events", label: "事件" },
  { id: "runtimes", label: "运行时" },
  { id: "jobs", label: "作业" },
  { id: "audit", label: "审计" },
];

const APPROVAL_TABS = [
  { id: "pending", label: "待处理" },
  { id: "approved", label: "已通过" },
  { id: "rejected", label: "已拒绝" },
  { id: "all", label: "全部" },
];

const WORKBENCH_EXAMPLES = [
  {
    title: "梳理前端架构",
    prompt: "请帮我梳理当前仓库的前端架构，并指出最值得优先优化的三个问题。",
  },
  {
    title: "检查模型切换逻辑",
    prompt: "请检查当前供应商切换和按智能体绑定的实现，列出潜在风险与改进建议。",
  },
  {
    title: "规划一个新页面",
    prompt: "请基于现有控制台风格，为我规划一个更适合聊天首屏的首页布局。",
  },
  {
    title: "直接执行开发任务",
    prompt: "请帮我修改当前页面，让首页默认进入对话工作台，并保持控制台视图可访问。",
  },
];

const UI_LABELS = {
  gateway: "网关",
  provider: "供应商",
  model: "模型",
  binding: "绑定",
  health: "健康状态",
  profile: "配置状态",
  package: "扩展包",
  "default provider": "默认供应商",
  "default model": "默认模型",
  status: "状态",
};

const STATUS_TEXT = {
  ok: "正常",
  running: "运行中",
  ready: "就绪",
  idle: "空闲",
  draft: "草稿",
  reachable: "可连接",
  disabled: "已禁用",
  pending: "待处理",
  approved: "已通过",
  rejected: "已拒绝",
  completed: "已完成",
  active: "活跃",
  enabled: "启用",
  available: "可用",
  installed: "已安装",
  secured: "已保护",
  open: "开放",
  warning: "警告",
  success: "正常",
  info: "信息",
  danger: "风险",
  queued: "排队中",
  waiting: "等待中",
  waiting_approval: "等待审批",
  failed: "失败",
  invalid: "无效",
  invalid_base_url: "无效地址",
  unreachable: "不可连接",
  missing: "缺失",
  missing_key: "缺少密钥",
  global: "全局",
  global_default: "全局默认",
  unbound: "未绑定",
  unset: "未设置",
  unknown: "未知",
  configured: "已配置",
  stored: "已保存",
  "not required": "无需",
  "not stored": "未保存",
  built_in: "内置",
};

const PERMISSION_LABELS = {
  limited: "受限",
  "read-only": "只读",
  "workspace-write": "工作区可写",
  full: "完全访问",
};

const RESOURCE_KIND_LABELS = {
  org: "组织",
  project: "项目",
  workspace: "工作区",
};

const UI_TEXT = {
  "AnyClaw Console": "AnyClaw 控制台",
  "AnyClaw Web Console": "AnyClaw 本地控制台",
  "Model Center": "模型中心",
  "Local-first AI control plane": "本地优先 AI 控制台",
  "Control Plane": "控制台",
  "Loading page...": "正在加载页面...",
  "Loading AnyClaw Console...": "正在加载 AnyClaw 控制台...",
  "Connect To The Gateway": "连接到网关",
  "This gateway requires authentication. Paste an API token and the browser will keep it in local storage for future requests.": "当前网关已启用鉴权。请输入 API 令牌，浏览器会将其保存在本地，供后续请求复用。",
  "API Token": "API 令牌",
  "Bearer token": "Bearer 令牌",
  "Save And Continue": "保存并进入",
  "Provider Switching": "供应商切换",
  "Add providers in the UI, test connectivity, and switch every agent independently without editing config files.": "直接在界面中新增供应商、测试连通性，并为每个智能体独立切换模型，无需手改配置文件。",
  "Provider pool and per-agent routing": "供应商池与按智能体路由",
  "Providers are managed in the web console, tested in place, and assigned directly to each agent.": "供应商可以直接在控制台中管理、测试，并分配给每个智能体。",
  "The global default stays available, but it is no longer the only binding path.": "全局默认仍然可用，但不再是唯一入口。",
  "Launchpad": "快速入口",
  "Active workspace": "当前工作区",
  "Per-agent binding": "按智能体绑定",
  "Provider readiness": "供应商就绪度",
  "Pending approvals": "待处理审批",
  "The console exposes provider management as a first-class workflow:": "控制台将供应商管理提升为一级能力：",
  "add providers, test connections, and switch bindings for each agent independently.": "你可以直接添加供应商、测试连接，并为每个智能体独立切换绑定。",
  "Agents can inherit the global default or bind to a dedicated provider and model.": "智能体可以继承全局默认配置，也可以绑定到专属供应商和模型。",
  "Use connection tests before assigning a provider to live agents.": "在给线上智能体分配供应商前，请先完成连接测试。",
  "High-risk tool execution pauses here until an operator resolves it.": "高风险工具执行会在这里暂停，直到管理员完成处理。",
  "Recent Events": "最近事件",
  "Recent Jobs": "最近作业",
  "Recent Tools": "最近工具活动",
  "No recent events": "暂无最近事件",
  "No recent jobs": "暂无最近作业",
  "This panel will populate when the gateway produces new activity.": "网关产生新的活动后，这里会自动展示内容。",
  Providers: "供应商",
  Agents: "智能体",
  Agent: "智能体",
  Sessions: "会话",
  Runtimes: "运行时",
  "all workspaces": "全部工作区",
  "No bindings yet": "暂无绑定",
  "No providers configured": "暂无供应商配置",
  "No agent bindings available": "暂无智能体绑定",
  "No secrets configured": "暂无密钥配置",
  "No workspace selected": "未选择工作区",
  "No messages yet": "暂无消息",
  "No tasks yet": "暂无任务",
  "No organizations": "暂无组织",
  "No projects": "暂无项目",
  "No workspaces": "暂无工作区",
  "No approval selected": "未选择审批项",
  "Approval queue is empty": "审批队列为空",
  "No audit events": "暂无审计事件",
  "No background jobs": "暂无后台作业",
  "No runtimes active": "暂无运行时",
  "No agents found": "暂无智能体",
  "No browser token stored": "浏览器中尚未保存令牌",
  "No private skills configured.": "暂无私有技能配置。",
  "Open Model Center": "打开模型中心",
  "Open Workbench": "打开工作台",
  "Create providers first, then bind them to agents from Model Center.": "请先创建供应商，再在模型中心将它们绑定到智能体。",
  "Add the first provider to unlock per-agent switching and connection tests.": "先添加第一个供应商，即可启用按智能体切换与连接测试。",
  "Select a workspace before creating tasks.": "创建任务前请先选择工作区。",
  "Create or select a workspace before starting a session.": "开始会话前请先创建或选择一个工作区。",
  "Pick an assistant below to start a fresh session.": "从下方选择一个智能体，开启新的会话。",
  "Ask the selected agent to inspect code, explain a change, or take an action.": "让选中的智能体检查代码、解释变更，或直接执行操作。",
  "Send the first message to create a session transcript.": "发送第一条消息后，这里会生成会话记录。",
  "Create the first task to inspect code or execute work in the current workspace.": "创建第一个任务，用于在当前工作区检查代码或执行工作。",
  "Run a structured task against the selected workspace and agent.": "针对选中的工作区与智能体发起结构化任务。",
  "Select one or many agents, then switch provider and model in one action.": "选择一个或多个智能体后，可一次性切换供应商和模型。",
  "Every agent gets its own provider binding": "每个智能体都有独立的供应商绑定",
  "This page focuses on agent identity, permissions, and working directories.": "这个页面聚焦智能体身份、权限和工作目录。",
  "Provider selection stays one click away from every card so operators can retarget an agent without touching the global default.": "每张卡片都提供一键切换供应商的入口，管理员无需修改全局默认配置。",
  "Create a new agent profile to segment providers, permissions, or working directories.": "创建新的智能体配置，以区分供应商、权限或工作目录。",
  "OpenAI Production": "生产环境 OpenAI",
  "Leave empty to keep the current key.": "留空则保留当前密钥。",
  "Leave empty to inherit the provider default model": "留空则继承供应商默认模型",
  "What this agent is responsible for.": "填写这个智能体负责的事项。",
  "Optional agent-specific model": "可选：智能体专属模型",
  "comma-separated skill names": "使用逗号分隔多个技能名",
  "Explain who should use this role.": "说明这个角色适合什么人使用。",
  "No description provided.": "暂无说明。",
  "Agent package": "智能体包",
  "Package detail": "包详情",
  Approval: "审批项",
  "Approval Queue": "审批队列",
  "Switch Agent Provider": "切换智能体供应商",
  "Browse packaged agents and install them into this workspace": "浏览可安装的智能体包并将其加入当前工作区",
  "Store entries are packaged agent profiles with persona, domain expertise, and skills.": "商店条目是打包好的智能体配置，包含人设、领域能力与技能。",
  "Operators can inspect details before installing them into the local environment.": "安装到本地环境前，管理员可以先查看详细信息。",
  "No packaged agents were discovered in the local store source.": "当前本地商店源中没有发现可用的打包智能体。",
  "Add Provider": "添加供应商",
  "Add Role": "新增角色",
  "Add User": "新增用户",
  "Agent profile": "智能体配置",
  "Working directory": "工作目录",
  "Switch Provider": "切换供应商",
  "View Agent": "查看智能体",
  "New Session": "新建会话",
  "Task detail": "任务详情",
  "Agent Bindings": "智能体绑定",
  "Provider Pool": "供应商池",
  "Secrets and Connections": "密钥与连接",
  "Agent Binding Matrix": "智能体绑定矩阵",
  "Bulk Switch Agents": "批量切换智能体",
  "Bulk Bind": "批量绑定",
  "Page load failed": "页面加载失败",
  Retry: "重试",
  "Action failed": "操作失败",
  "Submit failed": "提交失败",
  "Unknown error": "未知错误",
  "Unauthorized": "未授权",
  "Bound Agents": "已绑定智能体",
  Address: "地址",
  "Gateway Status": "网关状态",
  "Global provider": "全局供应商",
  Endpoint: "端点",
  Auth: "认证",
  Credential: "凭证",
  "Browser API token": "浏览器 API 令牌",
  "Store is empty": "商店为空",
  "Agent Store": "智能体商店",
  Organizations: "组织",
  Projects: "项目",
  Workspaces: "工作区",
  "Add Org": "新增组织",
  "Add Project": "新增项目",
  "Add Workspace": "新增工作区",
  Author: "作者",
  Provider: "供应商",
  Model: "模型",
  Runtime: "运行时",
  Workspace: "工作区",
  Status: "状态",
  Name: "名称",
  User: "用户",
  ID: "ID",
  Description: "说明",
  Role: "角色",
  Persona: "人设",
  Path: "路径",
  Project: "项目",
  Organization: "组织",
  Permissions: "权限",
  Scopes: "作用域",
  Token: "令牌",
  Capabilities: "能力标签",
  Enabled: "启用",
  "Display type": "显示类型",
  "Runtime provider": "运行时供应商",
  "Base URL": "接口地址",
  "API key": "API 密钥",
  "API Key": "API 密钥",
  "Default model": "默认模型",
  "Model override": "模型覆盖",
  "Selected agents": "已选智能体",
  "Permission level": "权限等级",
  "Permission overrides": "权限覆盖",
  Skills: "技能",
  "Save Agent": "保存智能体",
  "Save Provider": "保存供应商",
  "Test Existing Config": "测试当前配置",
  "Apply Binding": "应用绑定",
  "Save Organization": "保存组织",
  "Save Project": "保存项目",
  "Save Workspace": "保存工作区",
  "Save User": "保存用户",
  "Save Role": "保存角色",
  Install: "安装",
  Uninstall: "卸载",
  Close: "关闭",
  Test: "测试",
  Edit: "编辑",
  Delete: "删除",
  Switch: "切换",
  Approve: "批准",
  Reject: "拒绝",
  Inspect: "查看",
  Retry: "重试",
  Cancel: "取消",
  "Select all agents": "选择全部智能体",
  "Select workspace": "选择工作区",
  "Selected agents": "已选智能体",
  Refresh: "刷新",
  "Select at least one agent.": "请至少选择一个智能体。",
  Details: "详情",
  "Browser auth token": "浏览器访问令牌",
  "Clear Browser Token": "清除浏览器令牌",
  "Access Model": "访问控制",
  "Security mode": "安全模式",
  "When the gateway is secured, operators can paste an API token into the overlay and keep it in local browser storage.": "当网关启用保护后，管理员可以在弹层中粘贴 API 令牌，并将其保存在本地浏览器中。",
  "Role strategy": "角色策略",
  "Built-in roles cover admin, operator, and viewer. Custom roles can narrow access to configuration, resources, or runtime actions.": "内置角色覆盖管理员、操作员和观察者；自定义角色则可进一步收窄配置、资源或运行时操作权限。",
  "Add an operator or viewer account when gateway auth is enabled.": "启用网关鉴权后，可以新增操作员或查看者账号。",
  "No roles loaded": "暂无角色配置",
  "Built-in roles should appear here when access control is enabled.": "启用访问控制后，内置角色会显示在这里。",
  "built-in": "内置",
  custom: "自定义",
  Users: "用户",
  Roles: "角色",
  "No users configured": "暂无用户配置",
  "Create Agent": "创建智能体",
  "Edit Agent": "编辑智能体",
  "Full platform access": "完整平台访问权限",
  "Operate sessions, runtimes, and workspace resources": "管理会话、运行时与工作区资源",
  "Read-only governance and monitoring": "只读治理与监控",
  "Global Default": "全局默认",
  "Global default": "全局默认",
  Standalone: "独立审批",
  configured: "已配置",
  "not required": "无需",
  "not stored": "未保存",
  "No role": "无角色",
  inherit: "继承默认",
  "no default model": "未设置默认模型",
  "Provider default endpoint": "使用供应商默认端点",
  "default endpoint": "默认端点",
  "Browser token saved.": "浏览器令牌已保存。",
  "Browser token cleared.": "浏览器令牌已清除。",
  "Message delivered.": "消息已发送。",
  "Task is waiting for approval.": "任务正在等待审批。",
  "Task submitted.": "任务已提交。",
  "Approval granted.": "审批已通过。",
  "Approval rejected.": "审批已拒绝。",
  "Provider saved.": "供应商已保存。",
  "Provider deleted.": "供应商已删除。",
  "Provider test finished.": "供应商连通性测试已完成。",
  "Provider not found.": "未找到供应商。",
  "Agent saved.": "智能体已保存。",
  "Agent deleted.": "智能体已删除。",
  "Resource saved.": "资源已保存。",
  "User saved.": "用户已保存。",
  "User deleted.": "用户已删除。",
  "Role saved.": "角色已保存。",
  "Role deleted.": "角色已删除。",
  "Runtime refreshed.": "运行时已刷新。",
  "No runtimes to refresh.": "当前没有可刷新的运行时。",
  "Runtime refresh batch queued.": "批量运行时刷新已加入队列。",
  "Retry queued.": "已加入重试队列。",
  "Job cancelled.": "作业已取消。",
  "Package installed.": "扩展包已安装。",
  "Package uninstalled.": "扩展包已卸载。",
  "Select one or more agents first.": "请先选择一个或多个智能体。",
  Task: "任务",
  Session: "会话",
  Payload: "请求载荷",
  Resolution: "处理结果",
  Comment: "备注",
  "Resolved by": "处理人",
  "No comment recorded.": "未记录备注。",
  "No pending or historical approvals match the current filter.": "当前筛选条件下没有待处理或历史审批。",
  "Pick an approval on the left to inspect its tool payload and resolution state.": "请从左侧选择一个审批项，查看工具载荷与处理状态。",
  "Explain why this tool action is approved or rejected.": "请说明批准或拒绝此工具操作的原因。",
  Hits: "命中",
  Builds: "构建",
  Refreshes: "刷新",
  Evictions: "淘汰",
  "Cache hits": "缓存命中",
  "Runtime creations": "运行时创建次数",
  "Manual invalidations": "手动失效次数",
  "TTL or capacity": "TTL 或容量淘汰",
  "Active Runtimes": "活跃运行时",
  "Refresh All": "全部刷新",
  "Refresh individual runtimes or queue a batch refresh for the full pool.": "可以逐个刷新运行时，也可以为整个运行时池批量排队刷新。",
  "Last Used": "最后使用",
  "Background Jobs": "后台作业",
  Job: "作业",
  Attempts: "尝试次数",
  Created: "创建时间",
  "Queued runtime refreshes and bulk moves will show up here.": "排队中的运行时刷新和批量操作会显示在这里。",
  "Audit Trail": "审计记录",
  Actor: "操作人",
  Action: "动作",
  Target: "目标",
  Timestamp: "时间",
  "Provider API keys stay masked in the UI. Browser auth tokens are stored locally only.": "供应商 API 密钥在界面中始终会被遮罩显示，浏览器令牌仅保存在本地。",
};

const UI_TEXT_PATTERNS = [
  [/\bAnyClaw is ([A-Za-z _-]+)\b/g, (_, value) => `AnyClaw 当前状态：${localizeStatusValue(value)}`],
  [/\b(\d+) ready\b/g, (_, value) => `${value} 个就绪`],
  [/\b(\d+) dedicated bindings\b/g, (_, value) => `${value} 个专属绑定`],
  [/\b(\d+) total builds\b/g, (_, value) => `累计 ${value} 次构建`],
  [/\b(\d+) providers\b/g, (_, value) => `${value} 个供应商`],
  [/\b(\d+) agents\b/g, (_, value) => `${value} 个智能体`],
  [/\b(\d+) users\b/g, (_, value) => `${value} 个用户`],
  [/\b(\d+) installed\b/g, (_, value) => `已安装 ${value} 项`],
  [/\b(\d+) downloads\b/g, (_, value) => `${value} 次下载`],
  [/\b(\d+) selected\b/g, (_, value) => `已选 ${value} 项`],
  [/\b(\d+) sessions\b/g, (_, value) => `${value} 个会话`],
  [/Updated (\d+) agent bindings?\./g, (_, value) => `已更新 ${value} 个智能体绑定。`],
  [/Unknown destination: (.+)/g, (_, value) => `未知目标：${value}`],
  [/Delete provider "([^"]+)"\? Agents using it will fall back to the global default\./g, (_, value) => `确认删除供应商“${value}”？正在使用它的智能体会回退到全局默认配置。`],
  [/Delete agent "([^"]+)"\?/g, (_, value) => `确认删除智能体“${value}”？`],
  [/Delete role "([^"]+)"\?/g, (_, value) => `确认删除角色“${value}”？`],
  [/Delete user "([^"]+)"\?/g, (_, value) => `确认删除用户“${value}”？`],
  [/Assistant: (.+)/g, (_, value) => `智能体：${value}`],
  [/(org|project|workspace) deleted\./g, (_, value) => `${localizeResourceKind(value)}已删除。`],
  [/Endpoint responded with HTTP (\d+)\./g, (_, value) => `端点返回 HTTP ${value}。`],
  [/Requested /g, "发起于 "],
  [/Step (\d+)/g, (_, value) => `步骤 ${value}`],
];

const UI_TEXT_ENTRIES = Object.entries(UI_TEXT).sort((a, b) => b[0].length - a[0].length);

const state = {
  route: "workbench",
  section: "",
  token: localStorage.getItem("anyclaw_token") || "",
  selectedWorkspace: localStorage.getItem("anyclaw_workspace") || "",
  selectedSessionId: "",
  selectedWorkbenchAssistant: localStorage.getItem("anyclaw_workbench_assistant") || "",
  selectedApprovalId: "",
  selectedBindingAgents: new Set(),
  workbenchSessionsOpen: false,
  sidebarPinned: localStorage.getItem("anyclaw_sidebar_pinned") === "true",
  sidebarHover: false,
  workbenchDraft: true,
  workbenchDraftMessage: "",
  workbenchSubmitting: false,
  workbenchPendingMessage: "",
  workbenchPendingSessionId: "",
  workbenchPendingAssistant: "",
  resources: null,
  assistants: [],
  providers: [],
  bindings: [],
  storeItems: [],
  controlPlane: null,
  status: null,
  sessions: [],
  tasks: [],
  approvals: [],
  events: [],
  jobs: [],
  audit: [],
  runtimes: [],
  runtimeMetrics: null,
  users: [],
  roles: [],
};

function normalizeCommandToken(input) {
  return String(input || "")
    .trim()
    .toLowerCase()
    .replace(/[\s_-]+/g, "");
}

function localizeText(input) {
  let text = String(input ?? "");
  for (const [source, target] of UI_TEXT_ENTRIES) {
    text = text.split(source).join(target);
  }
  for (const [pattern, replacer] of UI_TEXT_PATTERNS) {
    text = text.replace(pattern, replacer);
  }
  return text;
}

function localizeHTML(input) {
  return localizeText(input);
}

function localizeLabel(label) {
  const raw = String(label || "").trim();
  if (!raw) return "";
  const key = raw.toLowerCase();
  return UI_LABELS[key] || localizeText(raw);
}

function localizeStatusValue(value) {
  const raw = String(value ?? "").trim();
  if (!raw) return "—";
  const key = raw.toLowerCase().replace(/[\s-]+/g, "_");
  return STATUS_TEXT[key] || localizeText(raw);
}

function localizePermissionLabel(value) {
  return PERMISSION_LABELS[String(value || "").trim()] || String(value || "");
}

function localizeResourceKind(kind) {
  return RESOURCE_KIND_LABELS[String(kind || "").trim()] || String(kind || "");
}

function renderUIIcon(name) {
  return UI_ICONS[name] || UI_ICONS.layout;
}

const appShellEl = document.getElementById("app");
const sidebarEl = document.getElementById("sidebar");
const headerEl = document.getElementById("header");
const pageEl = document.getElementById("page");
const modalRoot = document.getElementById("modal-root");
const toastRoot = document.getElementById("toast-root");
const authOverlay = document.getElementById("auth-overlay");

document.addEventListener("DOMContentLoaded", init);

async function init() {
  bindGlobalEvents();
  syncShellLayout();
  syncRouteFromHash();
  await refreshCoreData();
  if (state.token) {
    hideAuthOverlay();
  }
  await renderApp();
}

function bindGlobalEvents() {
  window.addEventListener("hashchange", async () => {
    syncRouteFromHash();
    await renderApp();
  });
  window.addEventListener("resize", () => {
    if (!isDesktopSidebar()) {
      state.sidebarHover = false;
    }
    syncShellLayout();
  });
  document.addEventListener("click", (event) => {
    const target = event.target.closest("[data-action]");
    if (target) {
      void handleClick(target, event);
    }
  });
  document.addEventListener("submit", (event) => {
    void handleSubmit(event);
  });
  document.addEventListener("change", handleChange);
  document.addEventListener("keydown", handleKeydown);
  sidebarEl.addEventListener("mouseenter", () => {
    if (!isDesktopSidebar() || state.sidebarPinned) return;
    state.sidebarHover = true;
    syncShellLayout();
  });
  sidebarEl.addEventListener("mouseleave", () => {
    if (!isDesktopSidebar() || state.sidebarPinned) return;
    state.sidebarHover = false;
    syncShellLayout();
  });
  sidebarEl.addEventListener("focusin", () => {
    if (!isDesktopSidebar() || state.sidebarPinned) return;
    state.sidebarHover = true;
    syncShellLayout();
  });
  sidebarEl.addEventListener("focusout", (event) => {
    if (!isDesktopSidebar() || state.sidebarPinned) return;
    if (sidebarEl.contains(event.relatedTarget)) return;
    state.sidebarHover = false;
    syncShellLayout();
  });
}

function isDesktopSidebar() {
  return window.innerWidth > 780;
}

function isSidebarExpanded() {
  return !isDesktopSidebar() || state.sidebarPinned || state.sidebarHover;
}

function syncShellLayout() {
  if (!appShellEl) return;
  const desktop = isDesktopSidebar();
  const expanded = isSidebarExpanded();
  appShellEl.classList.toggle("sidebar-collapsed", desktop && !expanded);
  appShellEl.classList.toggle("sidebar-hovered", desktop && state.sidebarHover && !state.sidebarPinned);
  appShellEl.classList.toggle("sidebar-pinned", desktop && state.sidebarPinned);
}

function handleKeydown(event) {
  if (event.key === "Escape" && modalRoot.innerHTML.trim()) {
    closeModal();
    return;
  }
  const commandInput = event.target.closest('[data-change="command"]');
  if (commandInput && event.key === "Enter") {
    event.preventDefault();
    applyCommandJump(commandInput.value);
    commandInput.value = "";
  }
}

function syncRouteFromHash() {
  const raw = (window.location.hash || "#workbench").replace(/^#/, "");
  const [route, section] = raw.split("/");
  const known = NAV_ITEMS.some((item) => item.id === route);
  state.route = known ? route : "workbench";
  state.section = section || defaultSectionForRoute(state.route);
}

function defaultSectionForRoute(route) {
  if (route === "model-center") return "bindings";
  if (route === "observability") return "events";
  if (route === "approvals") return "pending";
  return "";
}

function setRoute(route, section = "") {
  const next = `#${route}${section ? `/${section}` : ""}`;
  if (window.location.hash === next) {
    syncRouteFromHash();
    void renderApp();
    return;
  }
  window.location.hash = next;
}

function applyCommandJump(input) {
  const normalized = normalizeCommandToken(input);
  if (!normalized) return;
  const direct = NAV_ITEMS.find(
    (item) => [item.id, item.label, ...(item.aliases || [])].some((value) => normalizeCommandToken(value) === normalized),
  );
  if (direct) {
    setRoute(direct.id);
    return;
  }
  if (["model", "provider", "模型", "供应商"].some((token) => normalized.includes(normalizeCommandToken(token)))) return setRoute("model-center");
  if (["agent", "智能体", "助手"].some((token) => normalized.includes(normalizeCommandToken(token)))) return setRoute("agents");
  if (["task", "任务"].some((token) => normalized.includes(normalizeCommandToken(token)))) return setRoute("tasks");
  if (["approval", "审批"].some((token) => normalized.includes(normalizeCommandToken(token)))) return setRoute("approvals");
  if (["workbench", "chat", "session", "工作台", "会话", "聊天"].some((token) => normalized.includes(normalizeCommandToken(token)))) return setRoute("workbench");
  if (["event", "runtime", "audit", "job", "事件", "运行时", "审计", "作业"].some((token) => normalized.includes(normalizeCommandToken(token)))) return setRoute("observability");
  if (["resource", "workspace", "资源", "工作区"].some((token) => normalized.includes(normalizeCommandToken(token)))) return setRoute("resources");
  if (["store", "商店"].some((token) => normalized.includes(normalizeCommandToken(token)))) return setRoute("store");
  if (["setting", "user", "role", "设置", "用户", "角色"].some((token) => normalized.includes(normalizeCommandToken(token)))) return setRoute("settings");
  showToast(`Unknown destination: ${input}`, "warning");
}

async function apiFetch(url, options = {}) {
  const headers = new Headers(options.headers || {});
  const isFormData = options.body instanceof FormData;
  if (!headers.has("Content-Type") && options.body && !isFormData && typeof options.body !== "string") {
    headers.set("Content-Type", "application/json");
  }
  if (state.token) {
    headers.set("Authorization", `Bearer ${state.token}`);
  }
  const response = await fetch(url, {
    ...options,
    headers,
    body:
      options.body && !isFormData && typeof options.body !== "string"
        ? JSON.stringify(options.body)
        : options.body,
  });
  if (response.status === 401) {
    showAuthOverlay();
    throw new Error("未授权，请先填写访问令牌。");
  }
  const raw = await response.text();
  const contentType = response.headers.get("content-type") || "";
  let payload = null;
  if (raw) {
    payload = contentType.includes("application/json") ? JSON.parse(raw) : raw;
  }
  if (!response.ok) {
    const message = typeof payload === "string" ? payload : payload?.error || `${response.status} ${response.statusText}`;
    throw new Error(message);
  }
  return payload;
}

async function safeFetch(url, options = {}, fallback = null) {
  try {
    return await apiFetch(url, options);
  } catch (error) {
    console.error(error);
    return fallback;
  }
}

async function refreshCoreData() {
  const [resources, assistants, providers, bindings, controlPlane, status] = await Promise.all([
    safeFetch("/resources", {}, null),
    safeFetch("/assistants", {}, []),
    safeFetch("/providers", {}, []),
    safeFetch("/agent-bindings", {}, []),
    safeFetch("/control-plane", {}, null),
    safeFetch("/status", {}, null),
  ]);
  state.resources = resources;
  state.assistants = assistants || [];
  state.providers = providers || [];
  state.bindings = bindings || [];
  state.controlPlane = controlPlane;
  state.status = status || controlPlane?.status || null;

  const workspaces = state.resources?.workspaces || [];
  const workspaceExists = workspaces.some((workspace) => workspace.id === state.selectedWorkspace);
  if ((!state.selectedWorkspace || !workspaceExists) && workspaces.length) {
    state.selectedWorkspace = workspaces[0].id;
    localStorage.setItem("anyclaw_workspace", state.selectedWorkspace);
  }

  const validAgents = new Set(state.bindings.map((binding) => binding.name));
  state.selectedBindingAgents = new Set(
    [...state.selectedBindingAgents].filter((name) => validAgents.has(name)),
  );

  const enabledAssistants = state.assistants.filter((assistant) => assistant.enabled !== false);
  const selectedAssistantValid = enabledAssistants.some((assistant) => assistant.name === state.selectedWorkbenchAssistant);
  if (!selectedAssistantValid) {
    state.selectedWorkbenchAssistant = enabledAssistants[0]?.name || "";
    localStorage.setItem("anyclaw_workbench_assistant", state.selectedWorkbenchAssistant);
  }
}

async function renderApp() {
  headerEl.className = state.route === "workbench" ? "topbar topbar-workbench" : "topbar";
  pageEl.className = state.route === "workbench" ? "page-shell page-shell-workbench" : "page-shell";
  renderSidebar();
  syncShellLayout();
  renderHeader();
  await renderPage();
}

function renderSidebar() {
  const counts = {
    "model-center": state.providers.length || 0,
    approvals: state.approvals.filter((item) => item.status === "pending").length || 0,
  };
  const toggleLabel = state.sidebarPinned ? "切换为悬停展开" : "固定左侧导航";
  const toggleIcon = state.sidebarPinned ? "unpin" : "pin";
  sidebarEl.innerHTML = localizeHTML(`
    <div class="sidebar-head">
      <div class="brand">
        <div class="brand-mark">A</div>
        <div class="brand-copy sidebar-text-block">
          <h1>AnyClaw 控制台</h1>
          <p>本地优先 AI 工作台</p>
        </div>
      </div>
      <button
        type="button"
        class="sidebar-toggle ${state.sidebarPinned ? "is-active" : ""}"
        data-action="toggle-sidebar-pin"
        aria-pressed="${state.sidebarPinned ? "true" : "false"}"
        aria-label="${toggleLabel}"
        title="${toggleLabel}"
      >
        <span class="sidebar-toggle-icon" aria-hidden="true">${renderUIIcon(toggleIcon)}</span>
      </button>
    </div>
    <div class="nav-group">
      <div class="nav-label sidebar-text">主要导航</div>
      ${NAV_ITEMS.map((item) => {
    const active = item.id === state.route;
    return `
          <button class="nav-item ${active ? "is-active" : ""}" data-action="navigate" data-route="${item.id}">
            <span class="nav-item-main">
              <span class="nav-icon" aria-hidden="true">${renderUIIcon(item.icon)}</span>
              <span class="nav-item-label sidebar-text">${escapeHTML(item.label)}</span>
            </span>
            ${counts[item.id] ? `<span class="nav-pill sidebar-text">${counts[item.id]}</span>` : ""}
          </button>
        `;
  }).join("")}
    </div>
  `);
  const toggleButton = sidebarEl.querySelector('[data-action="toggle-sidebar-pin"]');
  if (toggleButton) {
    toggleButton.addEventListener("click", (event) => {
      event.preventDefault();
      event.stopPropagation();
      void handleClick(toggleButton, event);
    });
  }
}

function renderHeader() {
  if (state.route === "workbench") {
    headerEl.style.display = "none";
    return;
  }
  headerEl.style.display = ""; // 其他页面恢复显示
  const gatewayStatus = state.status || state.controlPlane?.status || {};
  const workspaceOptions = (state.resources?.workspaces || []).map((workspace) => {
    const selected = workspace.id === state.selectedWorkspace ? "selected" : "";
    return `<option value="${escapeHTML(workspace.id)}" ${selected}>${escapeHTML(workspace.name)} / ${escapeHTML(workspace.path)}</option>`;
  });
  const routeMeta = NAV_ITEMS.find((item) => item.id === state.route) || NAV_ITEMS[0];
  headerEl.innerHTML = localizeHTML(`
    <div class="topbar-main">
      <div class="eyebrow">AnyClaw 本地控制台</div>
      <div class="topbar-title">
        <h2>${escapeHTML(routeMeta.label)}</h2>
        <p>${escapeHTML(routeMeta.hint)}</p>
      </div>
    </div>
    <div class="topbar-tools">
      <label class="workspace-picker topbar-workspace-picker">
        <select class="select" data-change="workspace" aria-label="Select workspace">
          ${workspaceOptions.length ? workspaceOptions.join("") : '<option value="">无工作区</option>'}
        </select>
      </label>
      <label class="command-bar topbar-command-bar">
        <span>跳转</span>
        <input type="text" data-change="command" placeholder="输入页面名，如模型中心、任务、智能体..." />
      </label>
      <div class="topbar-status">
        ${statusChip(gatewayStatus.status || "unknown", "gateway")}
        ${statusChip(gatewayStatus.provider || "unbound", "provider")}
        ${statusChip(gatewayStatus.model || "unset", "model", true)}
      </div>
    </div>
  `);
}

async function renderPage() {
  pageEl.innerHTML = localizeHTML('<section class="loading-state"><div class="spinner-ring"></div><p>正在加载页面...</p></section>');
  try {
    switch (state.route) {
      case "overview":
        await renderOverview();
        break;
      case "workbench":
        await renderWorkbench();
        break;
      case "tasks":
        await renderTasks();
        break;
      case "approvals":
        await renderApprovals();
        break;
      case "agents":
        await renderAgents();
        break;
      case "model-center":
        await renderModelCenter();
        break;
      case "resources":
        await renderResources();
        break;
      case "observability":
        await renderObservability();
        break;
      case "store":
        await renderStore();
        break;
      case "settings":
        await renderSettings();
        break;
      default:
        await renderOverview();
        break;
    }
  } catch (error) {
    pageEl.innerHTML = localizeHTML(`
      <section class="error-state">
        <h3>页面加载失败</h3>
        <p>${escapeHTML(localizeText(error.message || "Unknown error"))}</p>
        <div class="inline-actions">
          <button class="btn btn-primary" data-action="rerender">重试</button>
        </div>
      </section>
    `);
  }
}

function currentWorkspace() {
  return (state.resources?.workspaces || []).find((workspace) => workspace.id === state.selectedWorkspace) || null;
}

function currentProject() {
  const workspace = currentWorkspace();
  if (!workspace) return null;
  return (state.resources?.projects || []).find((project) => project.id === workspace.project_id) || null;
}

function currentOrg() {
  const project = currentProject();
  if (!project) return null;
  return (state.resources?.orgs || []).find((org) => org.id === project.org_id) || null;
}

function currentWorkspaceQuery() {
  const workspace = currentWorkspace();
  const project = currentProject();
  const org = currentOrg();
  const params = new URLSearchParams();
  if (workspace?.id) params.set("workspace", workspace.id);
  if (project?.id) params.set("project", project.id);
  if (org?.id) params.set("org", org.id);
  return params;
}

function withWorkspaceQuery(path) {
  const query = currentWorkspaceQuery().toString();
  return query ? `${path}?${query}` : path;
}

function enabledAssistants() {
  return state.assistants.filter((assistant) => assistant.enabled !== false);
}

function setWorkbenchAssistant(name) {
  const next = String(name || "").trim();
  state.selectedWorkbenchAssistant = next;
  localStorage.setItem("anyclaw_workbench_assistant", next);
}

function setWorkbenchSessionsOpen(open) {
  state.workbenchSessionsOpen = Boolean(open);
}

function resolveWorkbenchSession(sessions) {
  if (!sessions.length) {
    state.selectedSessionId = "";
    return null;
  }
  if (state.workbenchDraft) {
    return null;
  }
  const resolved = resolveSelectedSession(sessions);
  return resolved || null;
}

function resolveWorkbenchAssistant(selectedSession) {
  const assistants = enabledAssistants();
  const fallback = assistants[0]?.name || "";
  const preferred = String(state.selectedWorkbenchAssistant || sessionPrimaryAssistant(selectedSession) || fallback).trim();
  if (preferred && preferred !== state.selectedWorkbenchAssistant) {
    setWorkbenchAssistant(preferred);
  }
  return preferred;
}

function resolveSelectedSession(sessions) {
  if (!sessions.length) return null;
  if (state.selectedSessionId) {
    const hit = sessions.find((session) => session.id === state.selectedSessionId);
    if (hit) return hit;
  }
  state.selectedSessionId = sessions[0].id;
  return sessions[0];
}

function resolveSelectedApproval(items) {
  if (!items.length) return null;
  if (state.selectedApprovalId) {
    const hit = items.find((item) => item.id === state.selectedApprovalId);
    if (hit) return hit;
  }
  state.selectedApprovalId = items[0].id;
  return items[0];
}

function renderKeyValueList(title, items, mapper) {
  return `
    <section class="panel">
      <div class="section-header">
        <h3>${escapeHTML(localizeText(title))}</h3>
        <span class="provider-chip">${items?.length || 0}</span>
      </div>
      ${items?.length ? `
        <div class="stack">
          ${items.map((item) => {
    const view = mapper(item);
    return `
              <article class="event-item">
                <div class="event-header">
                  <span class="event-title">${escapeHTML(localizeText(view.title))}</span>
                  <span class="muted">${escapeHTML(localizeText(view.meta))}</span>
                </div>
                <div class="muted mono">${escapeHTML(localizeText(view.subtitle || ""))}</div>
              </article>
            `;
  }).join("")}
        </div>
      ` : emptyState(`No ${title.toLowerCase()}`, "This panel will populate when the gateway produces new activity.")}
    </section>
  `;
}

function renderSectionTabs(route, tabs, currentSection) {
  return `
    <div class="tab-row">
      ${tabs.map((tab) => `
        <button class="tab-btn ${tab.id === currentSection ? "is-active" : ""}" data-action="navigate-section" data-route="${route}" data-section="${tab.id}">
          ${escapeHTML(tab.label)}
        </button>
      `).join("")}
    </div>
  `;
}

function renderAssistantOptions(selectedName) {
  const options = state.assistants
    .filter((assistant) => assistant.enabled !== false)
    .map((assistant) => {
      const selected = assistant.name === selectedName ? "selected" : "";
      return `<option value="${escapeHTML(assistant.name)}" ${selected}>${escapeHTML(assistant.name)}</option>`;
    });
  return options.length ? options.join("") : '<option value="">暂无智能体</option>';
}

function renderProviderOptions(selected) {
  return `
    <option value="">继承全局默认</option>
    ${state.providers.map((provider) => `
      <option value="${escapeHTML(provider.id)}" ${provider.id === selected ? "selected" : ""}>${escapeHTML(provider.name)} / ${escapeHTML(provider.provider)}</option>
    `).join("")}
  `;
}

function renderPermissionOptions(selected) {
  return ["limited", "workspace-write", "full"]
    .map((value) => `<option value="${value}" ${value === selected ? "selected" : ""}>${localizePermissionLabel(value)}</option>`)
    .join("");
}

function findBinding(name) {
  return state.bindings.find((binding) => binding.name === name) || null;
}

function findAssistant(name) {
  return state.assistants.find((assistant) => assistant.name === name) || null;
}

function findProvider(ref) {
  const needle = String(ref || "").trim().toLowerCase();
  return state.providers.find((provider) => provider.id?.toLowerCase() === needle || provider.name?.toLowerCase() === needle) || null;
}

function messageRole(message) {
  const raw = String(message?.role ?? message?.Role ?? "").trim().toLowerCase();
  return raw === "assistant" || raw === "system" ? "assistant" : "user";
}

function messageContent(message) {
  if (message == null) return "";
  return message.content ?? message.Content ?? "";
}

function sessionParticipants(session) {
  const participants = Array.isArray(session?.participants) ? session.participants.filter(Boolean) : [];
  if (participants.length) return participants;
  return session?.agent ? [session.agent] : [];
}

function sessionPrimaryAssistant(session) {
  return sessionParticipants(session)[0] || session?.agent || "";
}

function sessionMemberSummary(session) {
  const participants = sessionParticipants(session);
  if (!participants.length) return "未指定智能体";
  if (participants.length === 1) return participants[0];
  if (participants.length === 2) return participants.join("、");
  return `${participants[0]} 等 ${participants.length} 个 Agent`;
}

function sessionKindLabel(session) {
  return (session?.is_group || sessionParticipants(session).length > 1) ? "群聊频道" : "私聊频道";
}

function sessionMessages(session) {
  if (Array.isArray(session?.messages) && session.messages.length) return session.messages;
  return Array.isArray(session?.history) ? session.history : [];
}

function clearWorkbenchPendingState() {
  state.workbenchSubmitting = false;
  state.workbenchPendingMessage = "";
  state.workbenchPendingSessionId = "";
  state.workbenchPendingAssistant = "";
}

function resolveOrgName(id) {
  return (state.resources?.orgs || []).find((org) => org.id === id)?.name || id || "";
}

function resolveWorkspaceName(id) {
  return (state.resources?.workspaces || []).find((workspace) => workspace.id === id)?.name || id || "";
}

function findResource(kind, id) {
  if (!id) return null;
  if (kind === "org") return (state.resources?.orgs || []).find((item) => item.id === id) || null;
  if (kind === "project") return (state.resources?.projects || []).find((item) => item.id === id) || null;
  if (kind === "workspace") return (state.resources?.workspaces || []).find((item) => item.id === id) || null;
  return null;
}

function escapeHTML(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

function formatTime(value) {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return String(value);
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
}

function formatMultiline(value) {
  const text =
    typeof value === "string"
      ? value
      : value == null
        ? ""
        : typeof value === "object"
          ? JSON.stringify(value, null, 2)
          : String(value);
  return escapeHTML(text).replace(/\n/g, "<br />");
}

function formatMessageInline(value) {
  return escapeHTML(value)
    .replace(/`([^`\n]+)`/g, '<code class="message-inline-code">$1</code>')
    .replace(/\n/g, "<br />");
}

function formatMessageBody(value) {
  const text =
    typeof value === "string"
      ? value
      : value == null
        ? ""
        : typeof value === "object"
          ? JSON.stringify(value, null, 2)
          : String(value);
  if (!text.includes("```")) return formatMessageInline(text);
  const pattern = /```([\w-]*)\n?([\s\S]*?)```/g;
  let html = "";
  let lastIndex = 0;
  let match;
  while ((match = pattern.exec(text)) !== null) {
    html += formatMessageInline(text.slice(lastIndex, match.index));
    const language = String(match[1] || "").trim();
    const code = String(match[2] || "").replace(/\n$/, "");
    html += `
      <pre class="message-code-block"><code>${escapeHTML(code)}</code>${language ? `<span class="message-code-lang">${escapeHTML(language)}</span>` : ""}</pre>
    `;
    lastIndex = pattern.lastIndex;
  }
  html += formatMessageInline(text.slice(lastIndex));
  return html;
}

function renderThinkingDots() {
  return `
    <div class="thinking-indicator" aria-live="polite">
      <span class="thinking-dot"></span>
      <span class="thinking-dot"></span>
      <span class="thinking-dot"></span>
      <span class="thinking-text">正在思考...</span>
    </div>
  `;
}

function renderWorkbenchMessage(message, options = {}) {
  const role = options.role || messageRole(message);
  const pending = Boolean(options.pending);
  const thinking = Boolean(options.thinking);
  const author = role === "assistant"
    ? (options.assistantName || "智能体")
    : "你";
  const stateLabel = thinking
    ? "处理中"
    : pending
      ? "发送中"
      : "";
  return `
    <article class="message ${role} ${pending ? "is-pending" : ""} ${thinking ? "is-thinking" : ""}">
      <div class="message-head">
        <div class="message-meta-row">
          <span class="message-author">${escapeHTML(author)}</span>
          ${stateLabel ? `<span class="message-state">${escapeHTML(stateLabel)}</span>` : ""}
        </div>
      </div>
      <div class="message-body">${thinking ? renderThinkingDots() : formatMessageBody(options.content ?? messageContent(message))}</div>
    </article>
  `;
}

function renderChannelMessage(message, options = {}) {
  const role = options.role || messageRole(message);
  const pending = Boolean(options.pending);
  const thinking = Boolean(options.thinking);
  const rawRole = String(message?.role ?? message?.Role ?? "").trim().toLowerCase();
  const author = rawRole === "system"
    ? "系统"
    : role === "assistant"
      ? (message?.agent || message?.Agent || options.assistantName || "智能体")
      : "你";
  const stateLabel = thinking
    ? "处理中"
    : pending
      ? "发送中"
      : "";
  return `
    <article class="message ${role} ${pending ? "is-pending" : ""} ${thinking ? "is-thinking" : ""}">
      <div class="message-head">
        <div class="message-meta-row">
          <span class="message-author">${escapeHTML(author)}</span>
          ${stateLabel ? `<span class="message-state">${escapeHTML(stateLabel)}</span>` : ""}
        </div>
      </div>
      <div class="message-body">${thinking ? renderThinkingDots() : formatMessageBody(options.content ?? messageContent(message))}</div>
    </article>
  `;
}

function statusTone(status) {
  const value = String(status || "").toLowerCase();
  if (!value) return "info";
  if (value === "success") return "success";
  if (value === "warning") return "warning";
  if (value === "danger") return "danger";
  if (value === "info") return "info";
  if (["ready", "ok", "reachable", "enabled", "approved", "completed", "installed", "running", "active"].some((token) => value.includes(token))) return "success";
  if (["warning", "pending", "queued", "waiting", "draft", "missing", "disabled"].some((token) => value.includes(token))) return "warning";
  if (["danger", "error", "failed", "rejected", "forbidden", "unreachable", "invalid"].some((token) => value.includes(token))) return "danger";
  return "info";
}

function statusChip(value, label, compact = false) {
  const tone = statusTone(value);
  const localizedValue = localizeStatusValue(value || label || "—");
  const localizedLabel = localizeLabel(label || "status");
  const text = compact ? escapeHTML(localizedValue) : `${escapeHTML(localizedLabel)}: ${escapeHTML(localizedValue)}`;
  return `<span class="status-chip is-${tone}">${text}</span>`;
}

function metricCard(label, value, meta, tone = "info") {
  return `
    <article class="metric-card metric-card-${tone}">
      <p class="metric-label">${escapeHTML(localizeText(label))}</p>
      <div class="metric-value">${escapeHTML(String(value))}</div>
      <div class="metric-meta">
        <span class="muted">${escapeHTML(localizeText(meta || ""))}</span>
        ${statusChip(tone, tone, true)}
      </div>
    </article>
  `;
}

function emptyState(title, body, actionLabel = "", action = "") {
  return `
    <div class="empty-state">
      <h3>${escapeHTML(localizeText(title))}</h3>
      <p>${escapeHTML(localizeText(body))}</p>
      ${actionLabel && action ? `<div class="inline-actions"><button class="btn btn-primary" data-action="${escapeHTML(action)}">${escapeHTML(localizeText(actionLabel))}</button></div>` : ""}
    </div>
  `;
}

function renderJSON(value) {
  return `<pre class="code-block mono">${escapeHTML(JSON.stringify(value, null, 2))}</pre>`;
}

function maskToken(value) {
  const token = String(value || "").trim();
  if (!token) return "";
  if (token.length <= 8) return "*".repeat(token.length);
  return `${token.slice(0, 4)}${"*".repeat(Math.max(0, token.length - 8))}${token.slice(-4)}`;
}

function parseCSV(input) {
  return String(input || "")
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
}

async function renderOverview() {
  const [sessions, tasks, approvals, runtimes] = await Promise.all([
    safeFetch(`/sessions?workspace=${encodeURIComponent(state.selectedWorkspace)}`, {}, []),
    safeFetch(`/tasks?workspace=${encodeURIComponent(state.selectedWorkspace)}`, {}, []),
    safeFetch("/approvals?status=pending", {}, []),
    safeFetch("/runtimes", {}, []),
  ]);

  state.sessions = sessions || [];
  state.tasks = tasks || [];
  state.approvals = approvals || [];
  state.runtimes = runtimes || [];

  const workspace = currentWorkspace();
  const readyProviders = state.providers.filter((provider) => provider.health?.ok).length;
  const boundAgents = state.bindings.filter((binding) => binding.provider_ref).length;
  const recentEvents = state.controlPlane?.recent_events || [];
  const recentJobs = state.controlPlane?.recent_jobs || [];

  pageEl.innerHTML = localizeHTML(`
    <div class="page-grid">
      <div class="hero-grid">
        <section class="hero-card">
          <div class="section-header">
            <div>
              <div class="eyebrow">Control Plane</div>
              <h3>AnyClaw is ${escapeHTML((state.status?.status || "running").toLowerCase())}</h3>
            </div>
            ${statusChip(state.status?.status || "running", "gateway")}
          </div>
          <p>
            The console exposes provider management as a first-class workflow:
            add providers, test connections, and switch bindings for each agent independently.
          </p>
          <div class="inline-actions">
            <button class="btn btn-primary" data-action="navigate-section" data-route="model-center" data-section="bindings">Open Model Center</button>
            <button class="btn btn-secondary" data-action="open-provider-modal">Add Provider</button>
            <button class="btn btn-ghost" data-action="navigate" data-route="workbench">Open Workbench</button>
          </div>
          <div class="subtle-divider"></div>
          <div class="stack-sm">
            <div class="muted">Active workspace</div>
            <div class="mono">${escapeHTML(workspace?.path || "No workspace selected")}</div>
            <div class="badge-row">
              ${statusChip(state.status?.provider || "unset", "default provider")}
              ${statusChip(state.status?.model || "unset", "default model", true)}
            </div>
          </div>
        </section>

        <section class="panel">
          <div class="section-header">
            <h3>Launchpad</h3>
            <span class="provider-chip">${state.assistants.length} agents</span>
          </div>
          <div class="stack">
            <article class="list-row">
              <div class="event-header">
                <span class="event-title">Per-agent binding</span>
                <span class="muted">${boundAgents}/${state.bindings.length}</span>
              </div>
              <p class="muted">Agents can inherit the global default or bind to a dedicated provider and model.</p>
            </article>
            <article class="list-row">
              <div class="event-header">
                <span class="event-title">Provider readiness</span>
                <span class="muted">${readyProviders}/${state.providers.length}</span>
              </div>
              <p class="muted">Use connection tests before assigning a provider to live agents.</p>
            </article>
            <article class="list-row">
              <div class="event-header">
                <span class="event-title">Pending approvals</span>
                <span class="muted">${state.approvals.length}</span>
              </div>
              <p class="muted">High-risk tool execution pauses here until an operator resolves it.</p>
            </article>
          </div>
        </section>
      </div>

      <div class="stats-grid">
        ${metricCard("Providers", state.providers.length, `${readyProviders} ready`, "info")}
        ${metricCard("Agents", state.assistants.length, `${boundAgents} dedicated bindings`, "success")}
        ${metricCard("Sessions", state.sessions.length, workspace ? workspace.name : "all workspaces", "warning")}
        ${metricCard("Runtimes", state.runtimes.length, `${state.controlPlane?.runtime_metrics?.builds || 0} total builds`, "info")}
      </div>

      <div class="two-col">
        ${renderKeyValueList("Recent Events", recentEvents.slice(-6).reverse(), (event) => ({
    title: event.type,
    meta: formatTime(event.timestamp),
    subtitle: JSON.stringify(event.payload || {}),
  }))}
        ${renderKeyValueList("Recent Jobs", recentJobs.slice(-6).reverse(), (job) => ({
    title: `${job.kind} · ${job.status}`,
    meta: formatTime(job.created_at || job.createdAt),
    subtitle: job.summary || job.id,
  }))}
      </div>

      <div class="card-grid">
        ${state.bindings.slice(0, 3).map(renderBindingSpotlightCard).join("") || `
          <section class="panel">
            ${emptyState("No bindings yet", "Create providers first, then bind them to agents from Model Center.")}
          </section>
        `}
      </div>
    </div>
  `);
}

function renderBindingSpotlightCard(binding) {
  return `
    <article class="provider-card">
      <div class="section-header">
        <div>
          <h3>${escapeHTML(binding.name)}</h3>
          <p class="muted">${escapeHTML(binding.description || binding.role || "Agent profile")}</p>
        </div>
        ${statusChip(binding.health?.status || "unknown", "binding")}
      </div>
      <div class="stack-sm">
        <div class="badge-row">
          <span class="provider-chip">${escapeHTML(binding.provider_name || "Global default")}</span>
          <span class="mono-chip">${escapeHTML(binding.model || "inherit")}</span>
        </div>
        <div class="muted">Working directory</div>
        <div class="mono">${escapeHTML(binding.working_dir || "-")}</div>
      </div>
      <div class="inline-actions">
        <button class="btn btn-secondary" data-action="open-binding-modal" data-agent="${escapeHTML(binding.name)}">Switch Provider</button>
        <button class="btn btn-ghost" data-action="navigate" data-route="agents">View Agent</button>
      </div>
    </article>
  `;
}

function renderWorkbenchAssistantStrip(selectedName) {
  const assistants = enabledAssistants();
  if (!assistants.length) {
    return `
      <button type="button" class="agent-switch-chip is-empty" data-action="open-agent-modal">
        创建第一个智能体
      </button>
    `;
  }
  return assistants.map((assistant) => {
    const binding = findBinding(assistant.name);
    const active = assistant.name === selectedName;
    const providerName = binding?.provider_name || "全局默认";
    const modelName = binding?.model || state.status?.model || "继承默认";
    return `
      <div class="agent-switch-chip ${active ? "is-active" : ""}">
        <button
          type="button"
          class="agent-switch-chip-main"
          data-action="set-workbench-assistant"
          data-agent="${escapeHTML(assistant.name)}"
          aria-pressed="${active ? "true" : "false"}"
        >
          <span class="agent-switch-name">${escapeHTML(assistant.name)}</span>
          <span class="agent-switch-meta">${escapeHTML(providerName)} · ${escapeHTML(modelName)}</span>
        </button>
        <button
          type="button"
          class="agent-switch-quick"
          data-action="open-binding-modal"
          data-agent="${escapeHTML(assistant.name)}"
          title="切换 ${escapeHTML(assistant.name)} 的供应商和模型"
        >
          切换
        </button>
      </div>
    `;
  }).join("");
}

function renderWorkbenchAgentButton(assistantName) {
  const label = assistantName || "选择智能体";
  return `
    <button
      type="button"
      class="btn btn-ghost workbench-agent-trigger"
      data-action="open-workbench-agent-picker"
      aria-label="切换智能体"
      ${state.workbenchSubmitting ? "disabled" : ""}
    >
      <span class="workbench-agent-trigger-label">智能体</span>
      <strong>${escapeHTML(label)}</strong>
      <span class="workbench-agent-trigger-caret" aria-hidden="true">▾</span>
    </button>
  `;
}

function renderWorkbenchComposer(options) {
  const {
    assistantName = "",
    sessionId = "",
    hero = false,
  } = options || {};
  const submitting = state.workbenchSubmitting;
  const composerClass = hero
    ? "composer workbench-composer workbench-composer-home"
    : "composer workbench-composer workbench-composer-docked";
  const textareaClass = hero ? "textarea textarea-home" : "textarea textarea-chat";
  const textareaValue = escapeHTML(state.workbenchDraftMessage || "");
  const actionLabel = submitting ? "发送中..." : hero ? "开始对话" : "发送";
  const secondaryButton = sessionId
    ? `<button type="button" class="btn btn-ghost" data-action="new-session" ${submitting ? "disabled" : ""}>新对话</button>`
    : `<button type="button" class="btn btn-ghost" data-action="navigate" data-route="tasks" ${submitting ? "disabled" : ""}>改用任务模式</button>`;
  const placeholder = hero ? "输入你的目标、问题或任务..." : "继续输入消息...";
  const statusText = submitting
    ? sessionId
      ? "智能体正在回复..."
      : "正在创建新对话..."
    : "";
  const formClass = `${composerClass}${submitting ? " is-submitting" : ""}`;

  return `
    <form class="${formClass}" data-form="chat" aria-busy="${submitting ? "true" : "false"}">
      ${sessionId ? `<input type="hidden" name="session_id" value="${escapeHTML(sessionId)}" />` : ""}
      <input type="hidden" name="assistant" value="${escapeHTML(assistantName)}" />
      <label class="field field-composer">
        <span class="sr-only">${hero ? "输入你的目标、问题或任务" : "输入消息"}</span>
        <textarea class="${textareaClass}" name="message" placeholder="${placeholder}" required ${submitting ? "disabled" : ""}>${textareaValue}</textarea>
      </label>
      <div class="composer-actions">
        <div class="badge-row">
          ${secondaryButton}
          ${statusText ? `<span class="composer-status" aria-live="polite">${escapeHTML(statusText)}</span>` : ""}
        </div>
        <button type="submit" class="btn btn-primary" ${submitting ? "disabled" : ""}>${actionLabel}</button>
      </div>
    </form>
  `;
}

function renderWorkbenchSessionsDrawer(sessions, selectedSession, sessionCount) {
  if (!state.workbenchSessionsOpen) return "";
  return `
    <div class="workbench-session-layer is-open">
      <button
        type="button"
        class="workbench-session-backdrop"
        data-action="toggle-workbench-sessions"
        aria-label="关闭历史会话"
      ></button>
      <aside class="panel workbench-session-drawer">
        <div class="section-header workbench-session-head">
          <div>
            <h3>历史会话</h3>
            <p class="muted">${sessionCount ? `共 ${sessionCount} 个会话` : "还没有历史会话"}</p>
          </div>
          <div class="inline-actions workbench-session-actions">
            <button class="btn btn-ghost" data-action="new-session">新对话</button>
          </div>
        </div>
        ${sessions.length
          ? `
            <div class="stack session-list">
              ${sessions.map((session) => `
                <button class="session-item ${session.id === selectedSession?.id ? "is-active" : ""}" data-action="select-session" data-id="${escapeHTML(session.id)}">
                  <div class="event-header">
                    <span class="event-title">${escapeHTML(session.title || "未命名会话")}</span>
                    <span class="muted">${escapeHTML(session.agent || "未指定智能体")}</span>
                  </div>
                  <div class="muted session-preview">${escapeHTML(session.last_user_text || session.last_assistant_text || "暂无消息")}</div>
                  <div class="badge-row">
                    ${statusChip(session.presence || "idle", "状态", true)}
                    <span class="provider-chip">${session.message_count || 0} 条消息</span>
                  </div>
                </button>
              `).join("")}
            </div>
          `
          : `
            <div class="workbench-empty-note">
              <h4>还没有历史会话</h4>
              <p>从主输入区开始，发送第一条消息后会自动生成会话。</p>
            </div>
          `}
      </aside>
    </div>
  `;
}

async function renderApprovals() {
  const status = state.section && state.section !== "all" ? state.section : "";
  const url = status ? `/approvals?status=${encodeURIComponent(status)}` : "/approvals";
  state.approvals = (await safeFetch(url, {}, [])) || [];
  const selected = resolveSelectedApproval(state.approvals);

  pageEl.innerHTML = localizeHTML(`
    <div class="page-grid">
      ${renderSectionTabs("approvals", APPROVAL_TABS, state.section || "pending")}
      <div class="approval-grid">
        <section class="panel">
          <div class="section-header">
            <h3>Approval Queue</h3>
            <span class="provider-chip">${state.approvals.length}</span>
          </div>
          ${state.approvals.length
      ? `
            <div class="stack">
              ${state.approvals.map((approval) => `
                <button class="approval-item ${approval.id === selected?.id ? "is-active" : ""}" data-action="select-approval" data-id="${escapeHTML(approval.id)}">
                  <div class="event-header">
                    <span class="event-title">${escapeHTML(approval.tool_name || approval.action || approval.id)}</span>
                    <span class="muted">${formatTime(approval.requested_at)}</span>
                  </div>
                  <div class="badge-row">
                    ${statusChip(approval.status || "pending", approval.status || "pending", true)}
                    ${approval.task_id ? `<span class="provider-chip">${escapeHTML(approval.task_id)}</span>` : ""}
                  </div>
                </button>
              `).join("")}
            </div>
          `
      : emptyState("Approval queue is empty", "No pending or historical approvals match the current filter.")
    }
        </section>

        <section class="panel">
          ${selected
      ? `
            <div class="section-header">
              <div>
                <h3>${escapeHTML(selected.tool_name || selected.action || "Approval")}</h3>
                <p class="muted">${escapeHTML(selected.signature || selected.id)}</p>
              </div>
              ${statusChip(selected.status || "pending", "approval")}
            </div>
            <div class="stack">
              <article class="list-row">
                <div class="event-header">
                  <span class="event-title">Task</span>
                  <span class="muted">${escapeHTML(selected.task_id || "Standalone")}</span>
                </div>
                <div class="muted">Requested ${formatTime(selected.requested_at)}</div>
              </article>
              <article class="list-row">
                <div class="event-header">
                  <span class="event-title">Session</span>
                  <span class="muted">${escapeHTML(selected.session_id || "-")}</span>
                </div>
                <div class="muted">Step ${escapeHTML(String(selected.step_index ?? 0))}</div>
              </article>
              <div>
                <div class="muted">Payload</div>
                ${renderJSON(selected.payload || {})}
              </div>
            </div>
          `
      : emptyState("No approval selected", "Pick an approval on the left to inspect its tool payload and resolution state.")
    }
        </section>

        <section class="panel">
          <div class="section-header">
            <h3>Resolution</h3>
            ${statusChip(selected?.status || "idle", "state")}
          </div>
          ${selected?.status === "pending"
      ? `
            <form class="stack" data-form="approval-resolve">
              <input type="hidden" name="id" value="${escapeHTML(selected.id)}" />
              <label class="field">
                <span>Comment</span>
                <textarea class="textarea" name="comment" placeholder="Explain why this tool action is approved or rejected."></textarea>
              </label>
              <div class="inline-actions">
                <button type="submit" class="btn btn-primary" name="approved" value="true">Approve</button>
                <button type="submit" class="btn btn-danger" name="approved" value="false">Reject</button>
              </div>
            </form>
          `
      : `
            <div class="stack">
              <article class="list-row">
                <div class="event-header">
                  <span class="event-title">Resolved by</span>
                  <span class="muted">${escapeHTML(selected?.resolved_by || "-")}</span>
                </div>
                <div class="muted">${formatTime(selected?.resolved_at)}</div>
              </article>
              <article class="list-row">
                <div class="event-header">
                  <span class="event-title">Comment</span>
                </div>
                <div class="muted">${escapeHTML(selected?.comment || "No comment recorded.")}</div>
              </article>
            </div>
          `
    }
        </section>
      </div>
    </div>
  `);
}

async function renderAgents() {
  const cards = state.assistants.map((assistant) => {
    const binding = findBinding(assistant.name);
    const skillNames = (assistant.skills || []).map((skill) => skill.name).filter(Boolean);
    return `
      <article class="agent-card">
        <div class="section-header">
          <div>
            <h3>${escapeHTML(assistant.name)}</h3>
            <p class="muted">${escapeHTML(assistant.description || assistant.role || "Assistant profile")}</p>
          </div>
          ${assistant.active ? statusChip("active", "profile") : statusChip(assistant.enabled ? "enabled" : "disabled", "profile")}
        </div>
        <div class="stack-sm">
          <div class="badge-row">
            <span class="provider-chip">${escapeHTML(binding?.provider_name || assistant.provider_name || "Global default")}</span>
            <span class="mono-chip">${escapeHTML(binding?.model || assistant.default_model || state.status?.model || "inherit")}</span>
            <span class="tag">${escapeHTML(localizePermissionLabel(assistant.permission_level || "limited"))}</span>
          </div>
          <div class="muted">Working directory</div>
          <div class="mono">${escapeHTML(assistant.working_dir || "-")}</div>
          ${skillNames.length
        ? `<div class="badge-row">${skillNames.map((skill) => `<span class="tag">${escapeHTML(skill)}</span>`).join("")}</div>`
        : '<div class="muted">No private skills configured.</div>'
      }
        </div>
        <div class="inline-actions">
          <button class="btn btn-primary" data-action="open-binding-modal" data-agent="${escapeHTML(assistant.name)}">Switch Provider</button>
          <button class="btn btn-secondary" data-action="open-agent-modal" data-name="${escapeHTML(assistant.name)}">Edit Agent</button>
          <button class="btn btn-danger" data-action="delete-agent" data-name="${escapeHTML(assistant.name)}">Delete</button>
        </div>
      </article>
    `;
  }).join("");

  pageEl.innerHTML = localizeHTML(`
    <div class="page-grid">
      <section class="hero-card">
        <div class="section-header">
          <div>
            <div class="eyebrow">Agents</div>
            <h3>Every agent gets its own provider binding</h3>
          </div>
          <button class="btn btn-primary" data-action="open-agent-modal">Create Agent</button>
        </div>
        <p>
          This page focuses on agent identity, permissions, and working directories.
          Provider selection stays one click away from every card so operators can retarget an agent without touching the global default.
        </p>
      </section>
      <div class="card-grid">
        ${cards || `<section class="panel">${emptyState("No agents found", "Create a new agent profile to segment providers, permissions, or working directories.")}</section>`}
      </div>
    </div>
  `);
}

async function renderModelCenter() {
  const content =
    state.section === "providers"
      ? renderProviderPool()
      : state.section === "secrets"
        ? renderSecretsView()
        : renderBindingMatrix();

  pageEl.innerHTML = localizeHTML(`
    <div class="page-grid">
      <section class="hero-card">
        <div class="section-header">
          <div>
            <div class="eyebrow">Model Center</div>
            <h3>Provider pool and per-agent routing</h3>
          </div>
          <div class="inline-actions">
            <button class="btn btn-primary" data-action="open-provider-modal">Add Provider</button>
            <button class="btn btn-secondary" data-action="open-bulk-binding-modal">Bulk Switch Agents</button>
          </div>
        </div>
        <p>
          Providers are managed in the web console, tested in place, and assigned directly to each agent.
          The global default stays available, but it is no longer the only binding path.
        </p>
      </section>
      ${renderSectionTabs("model-center", MODEL_CENTER_TABS, state.section || "bindings")}
      ${content}
    </div>
  `);
}

function renderBindingMatrix() {
  return `
    <section class="panel">
      <div class="section-header">
        <div>
          <h3>Agent Binding Matrix</h3>
          <p class="muted">Select one or many agents, then switch provider and model in one action.</p>
        </div>
        <div class="inline-actions">
          ${state.selectedBindingAgents.size ? `<span class="provider-chip">${state.selectedBindingAgents.size} selected</span>` : ""}
          <button class="btn btn-secondary" data-action="open-bulk-binding-modal" ${state.selectedBindingAgents.size ? "" : "disabled"}>Bulk Bind</button>
        </div>
      </div>
      ${state.bindings.length
      ? `
        <div class="table-wrap">
          <table>
            <thead>
              <tr>
                <th><input type="checkbox" data-change="binding-all" aria-label="Select all agents" /></th>
                <th>Agent</th>
                <th>Provider</th>
                <th>Model</th>
                <th>Runtime</th>
                <th>Status</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              ${state.bindings.map((binding) => {
        const selected = state.selectedBindingAgents.has(binding.name) ? "checked" : "";
        return `
                  <tr>
                    <td><input type="checkbox" data-change="binding-pick" data-agent="${escapeHTML(binding.name)}" ${selected} aria-label="Select ${escapeHTML(binding.name)}" /></td>
                    <td>
                      <div class="event-title">${escapeHTML(binding.name)}</div>
                      <div class="muted">${escapeHTML(binding.working_dir || "-")}</div>
                    </td>
                    <td>${escapeHTML(binding.provider_name || "Global default")}</td>
                    <td><span class="mono">${escapeHTML(binding.model || state.status?.model || "inherit")}</span></td>
                    <td>${escapeHTML(binding.provider || state.status?.provider || "unset")}</td>
                    <td>${statusChip(binding.health?.status || "unknown", binding.health?.status || "unknown", true)}</td>
                    <td><button class="btn btn-ghost" data-action="open-binding-modal" data-agent="${escapeHTML(binding.name)}">Switch</button></td>
                  </tr>
                `;
      }).join("")}
            </tbody>
          </table>
        </div>
      `
      : emptyState("No agent bindings available", "Create agents first, then assign dedicated providers or models from this matrix.")
    }
    </section>
  `;
}

function renderProviderPool() {
  if (!state.providers.length) {
    return `
      <section class="panel">
        ${emptyState("No providers configured", "Add the first provider to unlock per-agent switching and connection tests.", "Add Provider", "open-provider-modal")}
      </section>
    `;
  }

  return `
    <div class="card-grid">
      ${state.providers.map((provider) => `
        <article class="provider-card">
          <div class="section-header">
            <div>
              <h3>${escapeHTML(provider.name)}</h3>
              <p class="muted">${escapeHTML(provider.type || provider.provider || "provider")}</p>
            </div>
            ${statusChip(provider.health?.status || "unknown", "health")}
          </div>
          <div class="stack-sm">
            <div class="badge-row">
              <span class="provider-chip">${escapeHTML(provider.provider)}</span>
              <span class="mono-chip">${escapeHTML(provider.default_model || "no default model")}</span>
            </div>
            <div class="muted">Endpoint</div>
            <div class="mono">${escapeHTML(provider.base_url || "Provider default endpoint")}</div>
            <div class="muted">Auth</div>
            <div class="mono">${escapeHTML(provider.api_key_preview || (provider.has_api_key ? "configured" : "not required"))}</div>
            ${provider.capabilities?.length ? `<div class="badge-row">${provider.capabilities.map((capability) => `<span class="tag">${escapeHTML(capability)}</span>`).join("")}</div>` : ""}
          </div>
          <div class="inline-actions">
            <button class="btn btn-primary" data-action="test-provider" data-id="${escapeHTML(provider.id)}">Test</button>
            <button class="btn btn-secondary" data-action="open-provider-modal" data-id="${escapeHTML(provider.id)}">Edit</button>
            <button class="btn btn-danger" data-action="delete-provider" data-id="${escapeHTML(provider.id)}">Delete</button>
          </div>
        </article>
      `).join("")}
    </div>
  `;
}

function renderSecretsView() {
  return `
    <section class="panel">
      <div class="section-header">
        <div>
          <h3>Secrets and Connections</h3>
          <p class="muted">Provider API keys stay masked in the UI. Browser auth tokens are stored locally only.</p>
        </div>
        <span class="provider-chip">${state.providers.length} providers</span>
      </div>
      ${state.providers.length
      ? `
        <div class="table-wrap">
          <table>
            <thead>
              <tr>
                <th>Provider</th>
                <th>Endpoint</th>
                <th>Credential</th>
                <th>Bound Agents</th>
                <th>Status</th>
              </tr>
            </thead>
            <tbody>
              ${state.providers.map((provider) => `
                <tr>
                  <td>
                    <div class="event-title">${escapeHTML(provider.name)}</div>
                    <div class="muted">${escapeHTML(provider.provider)}</div>
                  </td>
                  <td class="mono">${escapeHTML(provider.base_url || "default endpoint")}</td>
                  <td class="mono">${escapeHTML(provider.api_key_preview || (provider.has_api_key ? "configured" : "not required"))}</td>
                  <td>${escapeHTML(String(provider.bound_agent_count || 0))}</td>
                  <td>${statusChip(provider.health?.status || "unknown", provider.health?.status || "unknown", true)}</td>
                </tr>
              `).join("")}
            </tbody>
          </table>
        </div>
      `
      : emptyState("No secrets configured", "Add a provider first to populate this connection inventory.")
    }
      <div class="subtle-divider"></div>
      <div class="stack">
        <article class="list-row">
          <div class="event-header">
            <span class="event-title">Browser API token</span>
            <span class="muted">${state.token ? "configured" : "not stored"}</span>
          </div>
          <div class="mono">${escapeHTML(maskToken(state.token) || "No browser token stored")}</div>
        </article>
      </div>
    </section>
  `;
}

async function renderResources() {
  const sessions = (await safeFetch("/sessions", {}, [])) || [];
  state.sessions = sessions;

  const projectCounts = new Map();
  const workspaceCounts = new Map();
  for (const session of sessions) {
    projectCounts.set(session.project, (projectCounts.get(session.project) || 0) + 1);
    workspaceCounts.set(session.workspace, (workspaceCounts.get(session.workspace) || 0) + 1);
  }

  const orgs = state.resources?.orgs || [];
  const projects = state.resources?.projects || [];
  const workspaces = state.resources?.workspaces || [];

  pageEl.innerHTML = localizeHTML(`
    <div class="page-grid">
      <div class="three-col">
        <section class="panel">
          <div class="section-header">
            <h3>Organizations</h3>
            <button class="btn btn-ghost" data-action="open-resource-modal" data-kind="org">Add Org</button>
          </div>
          ${orgs.length
      ? `<div class="stack">
                ${orgs.map((org) => `
                  <article class="list-row">
                    <div class="event-header">
                      <span class="event-title">${escapeHTML(org.name)}</span>
                      <span class="muted">${escapeHTML(org.id)}</span>
                    </div>
                    <div class="inline-actions">
                      <button class="btn btn-ghost" data-action="open-resource-modal" data-kind="org" data-id="${escapeHTML(org.id)}">Edit</button>
                      <button class="btn btn-danger" data-action="delete-resource" data-kind="org" data-id="${escapeHTML(org.id)}">Delete</button>
                    </div>
                  </article>
                `).join("")}
              </div>`
      : emptyState("No organizations", "Create an org to start structuring workspaces.")
    }
        </section>

        <section class="panel">
          <div class="section-header">
            <h3>Projects</h3>
            <button class="btn btn-ghost" data-action="open-resource-modal" data-kind="project">Add Project</button>
          </div>
          ${projects.length
      ? `<div class="stack">
                ${projects.map((project) => `
                  <article class="list-row">
                    <div class="event-header">
                      <span class="event-title">${escapeHTML(project.name)}</span>
                      <span class="muted">${projectCounts.get(project.id) || 0} sessions</span>
                    </div>
                    <div class="muted">${escapeHTML(resolveOrgName(project.org_id))}</div>
                    <div class="inline-actions">
                      <button class="btn btn-ghost" data-action="open-resource-modal" data-kind="project" data-id="${escapeHTML(project.id)}">Edit</button>
                      <button class="btn btn-danger" data-action="delete-resource" data-kind="project" data-id="${escapeHTML(project.id)}">Delete</button>
                    </div>
                  </article>
                `).join("")}
              </div>`
      : emptyState("No projects", "Add a project to group related workspaces.")
    }
        </section>

        <section class="panel">
          <div class="section-header">
            <h3>Workspaces</h3>
            <button class="btn btn-ghost" data-action="open-resource-modal" data-kind="workspace">Add Workspace</button>
          </div>
          ${workspaces.length
      ? `<div class="stack">
                ${workspaces.map((workspace) => `
                  <article class="list-row">
                    <div class="event-header">
                      <span class="event-title">${escapeHTML(workspace.name)}</span>
                      <span class="muted">${workspaceCounts.get(workspace.id) || 0} sessions</span>
                    </div>
                    <div class="mono">${escapeHTML(workspace.path)}</div>
                    <div class="inline-actions">
                      <button class="btn btn-ghost" data-action="open-resource-modal" data-kind="workspace" data-id="${escapeHTML(workspace.id)}">Edit</button>
                      <button class="btn btn-danger" data-action="delete-resource" data-kind="workspace" data-id="${escapeHTML(workspace.id)}">Delete</button>
                    </div>
                  </article>
                `).join("")}
              </div>`
      : emptyState("No workspaces", "Create the first workspace to anchor sessions, tasks, and runtimes.")
    }
        </section>
      </div>
    </div>
  `);
}

async function renderObservability() {
  const section = state.section || "events";

  if (section === "events") {
    state.events = (await safeFetch("/events", {}, [])) || [];
  } else if (section === "runtimes") {
    const [runtimes, metrics] = await Promise.all([
      safeFetch("/runtimes", {}, []),
      safeFetch("/runtimes/metrics", {}, null),
    ]);
    state.runtimes = runtimes || [];
    state.runtimeMetrics = metrics;
  } else if (section === "jobs") {
    state.jobs = (await safeFetch("/jobs", {}, [])) || [];
  } else if (section === "audit") {
    state.audit = (await safeFetch("/audit", {}, [])) || [];
  }

  let content = "";
  if (section === "events") {
    content = `
      <div class="two-col">
        ${renderKeyValueList("Event Stream", state.events.slice().reverse(), (event) => ({
      title: event.type,
      meta: formatTime(event.timestamp),
      subtitle: JSON.stringify(event.payload || {}),
    }))}
        ${renderKeyValueList("Recent Tools", (state.controlPlane?.recent_tools || []).slice().reverse(), (tool) => ({
      title: `${tool.tool_name} · ${tool.agent || "agent"}`,
      meta: formatTime(tool.timestamp),
      subtitle: tool.error || tool.result || JSON.stringify(tool.args || {}),
    }))}
      </div>
    `;
  } else if (section === "runtimes") {
    content = `
      <div class="stats-grid">
        ${metricCard("Hits", state.runtimeMetrics?.hits || 0, "Cache hits", "success")}
        ${metricCard("Builds", state.runtimeMetrics?.builds || 0, "Runtime creations", "info")}
        ${metricCard("Refreshes", state.runtimeMetrics?.refreshes || 0, "Manual invalidations", "warning")}
        ${metricCard("Evictions", state.runtimeMetrics?.evictions || 0, "TTL or capacity", "danger")}
      </div>
      <section class="panel">
        <div class="section-header">
          <div>
            <h3>Active Runtimes</h3>
            <p class="muted">Refresh individual runtimes or queue a batch refresh for the full pool.</p>
          </div>
          <button class="btn btn-secondary" data-action="refresh-all-runtimes">Refresh All</button>
        </div>
        ${state.runtimes.length
        ? `
          <div class="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Agent</th>
                  <th>Workspace</th>
                  <th>Hits</th>
                  <th>Last Used</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                ${state.runtimes.map((runtime) => `
                  <tr>
                    <td>
                      <div class="event-title">${escapeHTML(runtime.agent)}</div>
                      <div class="muted">${escapeHTML(runtime.key)}</div>
                    </td>
                    <td>
                      <div>${escapeHTML(resolveWorkspaceName(runtime.workspace) || runtime.workspace)}</div>
                      <div class="mono">${escapeHTML(runtime.workspace_path || runtime.work_dir || "-")}</div>
                    </td>
                    <td>${escapeHTML(String(runtime.hits || 0))}</td>
                    <td>${formatTime(runtime.last_used_at)}</td>
                    <td><button class="btn btn-ghost" data-action="refresh-runtime" data-agent="${escapeHTML(runtime.agent)}" data-org="${escapeHTML(runtime.org)}" data-project="${escapeHTML(runtime.project)}" data-workspace="${escapeHTML(runtime.workspace)}">Refresh</button></td>
                  </tr>
                `).join("")}
              </tbody>
            </table>
          </div>
        `
        : emptyState("No runtimes active", "Runtime entries appear after sessions or tasks warm a workspace-specific app instance.")
      }
      </section>
    `;
  } else if (section === "jobs") {
    content = `
      <section class="panel">
        <div class="section-header">
          <h3>Background Jobs</h3>
          <span class="provider-chip">${state.jobs.length}</span>
        </div>
        ${state.jobs.length
        ? `
          <div class="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Job</th>
                  <th>Status</th>
                  <th>Attempts</th>
                  <th>Created</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                ${state.jobs.slice().reverse().map((job) => `
                  <tr>
                    <td>
                      <div class="event-title">${escapeHTML(job.summary || job.kind)}</div>
                      <div class="muted">${escapeHTML(job.kind)}</div>
                    </td>
                    <td>${statusChip(job.status || "unknown", job.status || "unknown", true)}</td>
                    <td>${escapeHTML(String(job.attempts || 0))}/${escapeHTML(String(job.max_attempts || 0))}</td>
                    <td>${formatTime(job.created_at || job.createdAt)}</td>
                    <td>
                      <div class="inline-actions">
                        ${job.retriable ? `<button class="btn btn-ghost" data-action="retry-job" data-id="${escapeHTML(job.id)}">Retry</button>` : ""}
                        ${job.cancellable ? `<button class="btn btn-danger" data-action="cancel-job" data-id="${escapeHTML(job.id)}">Cancel</button>` : ""}
                        <button class="btn btn-ghost" data-action="open-job-detail" data-id="${escapeHTML(job.id)}">Inspect</button>
                      </div>
                    </td>
                  </tr>
                `).join("")}
              </tbody>
            </table>
          </div>
        `
        : emptyState("No background jobs", "Queued runtime refreshes and bulk moves will show up here.")
      }
      </section>
    `;
  } else if (section === "audit") {
    content = `
      <section class="panel">
        <div class="section-header">
          <h3>Audit Trail</h3>
          <span class="provider-chip">${state.audit.length}</span>
        </div>
        ${state.audit.length
        ? `
          <div class="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Actor</th>
                  <th>Action</th>
                  <th>Target</th>
                  <th>Timestamp</th>
                </tr>
              </thead>
              <tbody>
                ${state.audit.slice().reverse().map((item) => `
                  <tr>
                    <td>
                      <div class="event-title">${escapeHTML(item.actor || "anonymous")}</div>
                      <div class="muted">${escapeHTML(item.role || "-")}</div>
                    </td>
                    <td>${escapeHTML(item.action || "-")}</td>
                    <td>${escapeHTML(item.target || "-")}</td>
                    <td>${formatTime(item.timestamp)}</td>
                  </tr>
                `).join("")}
              </tbody>
            </table>
          </div>
        `
        : emptyState("No audit events", "Configuration and operator actions will populate this ledger.")
      }
      </section>
    `;
  }

  pageEl.innerHTML = localizeHTML(`
    <div class="page-grid">
      ${renderSectionTabs("observability", OBSERVABILITY_TABS, section)}
      ${content}
    </div>
  `);
}

async function renderStore() {
  state.storeItems = (await safeFetch("/v2/store", {}, [])) || [];
  const installedCount = state.storeItems.filter((item) => item.installed_at).length;

  pageEl.innerHTML = localizeHTML(`
    <div class="page-grid">
      <section class="hero-card">
        <div class="section-header">
          <div>
            <div class="eyebrow">Agent Store</div>
            <h3>Browse packaged agents and install them into this workspace</h3>
          </div>
          <span class="provider-chip">${installedCount} installed</span>
        </div>
        <p>
          Store entries are packaged agent profiles with persona, domain expertise, and skills.
          Operators can inspect details before installing them into the local environment.
        </p>
      </section>

      <div class="card-grid">
        ${state.storeItems.length
      ? state.storeItems.map((item) => `
                <article class="provider-card">
                  <div class="section-header">
                    <div>
                      <h3>${escapeHTML(item.display_name || item.name || item.id)}</h3>
                      <p class="muted">${escapeHTML(item.category || "package")}</p>
                    </div>
                    ${item.installed_at ? statusChip("installed", "package") : statusChip("available", "package")}
                  </div>
                  <div class="stack-sm">
                    <div class="muted">${escapeHTML(item.description || "No description provided.")}</div>
                    <div class="badge-row">
                      <span class="provider-chip">${escapeHTML(item.author || "unknown author")}</span>
                      <span class="tag">${escapeHTML(item.version || "v1")}</span>
                      <span class="tag">${escapeHTML(String(item.downloads || 0))} downloads</span>
                    </div>
                    ${item.tags?.length ? `<div class="badge-row">${item.tags.map((tag) => `<span class="tag">${escapeHTML(tag)}</span>`).join("")}</div>` : ""}
                  </div>
                  <div class="inline-actions">
                    <button class="btn btn-secondary" data-action="open-store-detail" data-id="${escapeHTML(item.id)}">Details</button>
                    ${item.installed_at
          ? `<button class="btn btn-danger" data-action="uninstall-store" data-id="${escapeHTML(item.id)}">Uninstall</button>`
          : `<button class="btn btn-primary" data-action="install-store" data-id="${escapeHTML(item.id)}">Install</button>`
        }
                  </div>
                </article>
              `).join("")
      : `<section class="panel">${emptyState("Store is empty", "No packaged agents were discovered in the local store source.")}</section>`
    }
      </div>
    </div>
  `);
}

async function renderSettings() {
  const [status, users, roles] = await Promise.all([
    safeFetch("/status", {}, state.status),
    safeFetch("/auth/users", {}, []),
    safeFetch("/auth/roles", {}, []),
  ]);

  state.status = status || state.status;
  state.users = users || [];
  state.roles = roles || [];

  pageEl.innerHTML = localizeHTML(`
    <div class="page-grid">
      <div class="two-col">
        <section class="panel">
          <div class="section-header">
            <h3>Gateway Status</h3>
            ${statusChip(state.status?.status || "unknown", "gateway")}
          </div>
          <div class="stack">
            <article class="list-row">
              <div class="event-header">
                <span class="event-title">Address</span>
                <span class="muted">${escapeHTML(state.status?.version || "-")}</span>
              </div>
              <div class="mono">${escapeHTML(state.status?.address || "-")}</div>
            </article>
            <article class="list-row">
              <div class="event-header">
                <span class="event-title">Global provider</span>
                <span class="muted">${escapeHTML(state.status?.provider || "-")}</span>
              </div>
              <div class="mono">${escapeHTML(state.status?.model || "-")}</div>
            </article>
            <article class="list-row">
              <div class="event-header">
                <span class="event-title">Browser auth token</span>
                <span class="muted">${localizeStatusValue(state.token ? "stored" : "not stored")}</span>
              </div>
              <div class="mono">${escapeHTML(maskToken(state.token) || "No browser token stored")}</div>
            </article>
          </div>
          <div class="inline-actions">
            <button class="btn btn-ghost" data-action="clear-browser-token">Clear Browser Token</button>
          </div>
        </section>

        <section class="panel">
          <div class="section-header">
            <h3>Access Model</h3>
            <span class="provider-chip">${escapeHTML(String(state.status?.users || 0))} users</span>
          </div>
          <div class="stack">
            <article class="list-row">
              <div class="event-header">
                <span class="event-title">Security mode</span>
                <span class="muted">${localizeStatusValue(state.status?.secured ? "secured" : "open")}</span>
              </div>
              <div class="muted">When the gateway is secured, operators can paste an API token into the overlay and keep it in local browser storage.</div>
            </article>
            <article class="list-row">
              <div class="event-header">
                <span class="event-title">Role strategy</span>
              </div>
              <div class="muted">Built-in roles cover admin, operator, and viewer. Custom roles can narrow access to configuration, resources, or runtime actions.</div>
            </article>
          </div>
        </section>
      </div>

      <section class="panel">
        <div class="section-header">
          <h3>Users</h3>
          <button class="btn btn-primary" data-action="open-user-modal">Add User</button>
        </div>
        ${state.users.length
      ? `
          <div class="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>User</th>
                  <th>Role</th>
                  <th>Permissions</th>
                  <th>Scopes</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                ${state.users.map((user) => `
                  <tr>
                    <td>
                      <div class="event-title">${escapeHTML(user.name)}</div>
                      <div class="muted">${escapeHTML((user.workspaces || []).join(", ") || "all workspaces")}</div>
                    </td>
                    <td>${escapeHTML(user.role || "-")}</td>
                    <td>${escapeHTML((user.permissions || []).join(", ") || "-")}</td>
                    <td>${escapeHTML((user.scopes || []).join(", ") || "-")}</td>
                    <td><button class="btn btn-danger" data-action="delete-user" data-name="${escapeHTML(user.name)}">Delete</button></td>
                  </tr>
                `).join("")}
              </tbody>
            </table>
          </div>
        `
      : emptyState("No users configured", "Add an operator or viewer account when gateway auth is enabled.")
    }
      </section>

      <section class="panel">
        <div class="section-header">
          <h3>Roles</h3>
          <button class="btn btn-primary" data-action="open-role-modal">Add Role</button>
        </div>
        ${state.roles.length
      ? `
          <div class="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Role</th>
                  <th>Description</th>
                  <th>Permissions</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                ${state.roles.map((role) => `
                  <tr>
                    <td>
                      <div class="event-title">${escapeHTML(role.name)}</div>
                      <div class="muted">${role.custom ? "custom" : "built-in"}</div>
                    </td>
                    <td>${escapeHTML(role.description || "-")}</td>
                    <td>${escapeHTML((role.permissions || []).join(", ") || "-")}</td>
                    <td>
                      <div class="inline-actions">
                        <button class="btn btn-ghost" data-action="open-role-modal" data-name="${escapeHTML(role.name)}">Edit</button>
                        ${role.custom ? `<button class="btn btn-danger" data-action="delete-role" data-name="${escapeHTML(role.name)}">Delete</button>` : ""}
                      </div>
                    </td>
                  </tr>
                `).join("")}
              </tbody>
            </table>
          </div>
        `
      : emptyState("No roles loaded", "Built-in roles should appear here when access control is enabled.")
    }
      </section>
    </div>
  `);
}

async function handleClick(target, event) {
  const action = target.dataset.action;

  if (action === "modal-backdrop" && event.target === target) {
    closeModal();
    return;
  }

  try {
    switch (action) {
      case "navigate":
        setRoute(target.dataset.route || "overview");
        break;
      case "navigate-section":
        setRoute(target.dataset.route || state.route, target.dataset.section || "");
        break;
      case "rerender":
        await refreshCoreData();
        await renderApp();
        break;
      case "new-session":
        state.workbenchDraft = true;
        state.selectedSessionId = "";
        state.workbenchDraftMessage = "";
        setWorkbenchSessionsOpen(false);
        await renderWorkbench();
        break;
      case "fill-prompt":
        fillWorkbenchPrompt(target.dataset.prompt || "");
        break;
      case "set-workbench-assistant":
        state.workbenchDraftMessage =
          pageEl.querySelector('form[data-form="chat"] textarea[name="message"]')?.value || state.workbenchDraftMessage;
        setWorkbenchAssistant(target.dataset.agent || "");
        closeModal();
        await renderWorkbench();
        break;
      case "open-workbench-agent-picker":
        openWorkbenchAgentPicker();
        break;
      case "select-session":
        state.workbenchDraft = false;
        state.selectedSessionId = target.dataset.id || "";
        state.workbenchDraftMessage = "";
        setWorkbenchSessionsOpen(false);
        if (state.selectedSessionId) {
          const selected = state.sessions.find((session) => session.id === state.selectedSessionId);
          if (selected?.agent) {
            setWorkbenchAssistant(selected.agent);
          }
        }
        await renderWorkbench();
        break;
      case "toggle-sidebar-pin":
        state.sidebarPinned = !state.sidebarPinned;
        state.sidebarHover = false;
        localStorage.setItem("anyclaw_sidebar_pinned", String(state.sidebarPinned));
        renderSidebar();
        syncShellLayout();
        break;
      case "toggle-workbench-sessions":
        setWorkbenchSessionsOpen(!state.workbenchSessionsOpen);
        await renderWorkbench();
        break;
      case "open-channel":
        state.workbenchDraft = false;
        state.selectedSessionId = target.dataset.id || "";
        setWorkbenchSessionsOpen(false);
        setRoute("workbench");
        await renderApp();
        break;
      case "select-approval":
        state.selectedApprovalId = target.dataset.id || "";
        await renderApprovals();
        break;
      case "open-provider-modal":
        openProviderModal(target.dataset.id || "", { bindAgent: target.dataset.bindAgent || "" });
        break;
      case "apply-provider-preset":
        applyProviderPreset(target.dataset.preset || "");
        break;
      case "test-provider":
        await testProvider(target.dataset.id || "");
        break;
      case "delete-provider":
        await deleteProvider(target.dataset.id || "");
        break;
      case "open-binding-modal":
        openBindingModal([target.dataset.agent || ""]);
        break;
      case "open-bulk-binding-modal":
        openBulkBindingModal();
        break;
      case "open-agent-modal":
        openAgentModal(target.dataset.name || "");
        break;
      case "delete-agent":
        await deleteAgent(target.dataset.name || "");
        break;
      case "open-task-detail":
      case "open-channel-detail":
        await openChannelDetail(target.dataset.id || "");
        break;
      case "open-store-detail":
        await openStoreDetail(target.dataset.id || "");
        break;
      case "install-store":
        await installStore(target.dataset.id || "");
        break;
      case "uninstall-store":
        await uninstallStore(target.dataset.id || "");
        break;
      case "open-resource-modal":
        openResourceModal(target.dataset.kind || "", target.dataset.id || "");
        break;
      case "delete-resource":
        await deleteResource(target.dataset.kind || "", target.dataset.id || "");
        break;
      case "refresh-runtime":
        await refreshRuntime(target.dataset);
        break;
      case "refresh-all-runtimes":
        await refreshAllRuntimes();
        break;
      case "retry-job":
        await retryJob(target.dataset.id || "");
        break;
      case "cancel-job":
        await cancelJob(target.dataset.id || "");
        break;
      case "open-job-detail":
        await openJobDetail(target.dataset.id || "");
        break;
      case "open-user-modal":
        openUserModal();
        break;
      case "delete-user":
        await deleteUser(target.dataset.name || "");
        break;
      case "open-role-modal":
        openRoleModal(target.dataset.name || "");
        break;
      case "delete-role":
        await deleteRole(target.dataset.name || "");
        break;
      case "clear-browser-token":
        localStorage.removeItem("anyclaw_token");
        state.token = "";
        showToast("Browser token cleared.", "success");
        showAuthOverlay();
        await renderSettings();
        break;
      case "close-modal":
        closeModal();
        break;
      default:
        break;
    }
  } catch (error) {
    console.error(error);
    showToast(error.message || "Action failed", "danger");
  }
}

async function handleSubmit(event) {
  event.preventDefault();
  const form = event.target;
  const formType = form.dataset.form;
  const formData = new FormData(form);

  try {
    switch (formType) {
      case "auth": {
        state.token = String(formData.get("token") || "").trim();
        localStorage.setItem("anyclaw_token", state.token);
        hideAuthOverlay();
        await refreshCoreData();
        await renderApp();
        showToast("Browser token saved.", "success");
        break;
      }
      case "chat": {
        const message = String(formData.get("message") || "").trim();
        if (!message) throw new Error("Message is required.");
        const sessionId = String(formData.get("session_id") || "").trim();
        const assistant = String(formData.get("assistant") || "").trim();
        if (assistant) {
          setWorkbenchAssistant(assistant);
        }
        const title = String(formData.get("title") || "").trim();
        state.workbenchSubmitting = true;
        state.workbenchPendingMessage = message;
        state.workbenchPendingSessionId = sessionId;
        state.workbenchPendingAssistant = assistant || state.selectedWorkbenchAssistant || "";
        state.workbenchDraftMessage = "";
        if (sessionId) {
          state.workbenchDraft = false;
        }
        await renderWorkbench();
        try {
          const url = sessionId ? "/chat" : withWorkspaceQuery("/chat");
          const payload = await apiFetch(url, {
            method: "POST",
            body: { message, session_id: sessionId, assistant, title },
          });
          state.selectedSessionId = payload?.session?.id || state.selectedSessionId;
          state.workbenchDraft = false;
          clearWorkbenchPendingState();
          await refreshCoreData();
          await renderWorkbench();
          showToast("Message delivered.", "success");
        } catch (error) {
          state.workbenchDraft = !sessionId;
          state.workbenchDraftMessage = message;
          clearWorkbenchPendingState();
          await renderWorkbench();
          throw error;
        }
        break;
      }
      case "channel": {
        const assistant = String(formData.get("assistant") || "").trim();
        const type = String(formData.get("channel_type") || "dm").trim().toLowerCase();
        const participants = type === "group"
          ? Array.from(new Set([assistant, ...formData.getAll("participants").map((value) => String(value || "").trim()).filter(Boolean)]))
          : [assistant];
        if (!assistant) throw new Error("请选择默认智能体。");
        if (type === "group" && participants.length < 2) {
          throw new Error("群聊频道至少需要两个智能体。");
        }
        const payload = await apiFetch(withWorkspaceQuery("/sessions"), {
          method: "POST",
          body: {
            title: String(formData.get("title") || "").trim(),
            assistant,
            participants,
            is_group: type === "group",
            session_mode: type === "group" ? "channel-group" : "channel-dm",
            queue_mode: "fifo",
          },
        });
        state.selectedSessionId = payload?.id || "";
        state.workbenchDraft = false;
        await refreshCoreData();
        setRoute("workbench");
        await renderApp();
        showToast(type === "group" ? "群聊频道已创建。" : "私聊频道已创建。", "success");
        break;
      }
      case "task": {
        const input = String(formData.get("input") || "").trim();
        if (!input) throw new Error("请输入任务内容。");
        const payload = await apiFetch(withWorkspaceQuery("/tasks"), {
          method: "POST",
          body: {
            title: String(formData.get("title") || "").trim(),
            input,
            assistant: String(formData.get("assistant") || "").trim(),
          },
        });
        await refreshCoreData();
        await renderTasks();
        form.reset();
        showToast(payload?.status === "waiting_approval" ? "Task is waiting for approval." : "Task submitted.", "success");
        if (payload?.task?.id) {
          await openTaskDetail(payload.task.id);
        }
        break;
      }
      case "approval-resolve": {
        const id = String(formData.get("id") || "").trim();
        const approved = event.submitter?.value === "true";
        await apiFetch(`/approvals/${encodeURIComponent(id)}/resolve`, {
          method: "POST",
          body: { approved, comment: String(formData.get("comment") || "").trim() },
        });
        await refreshCoreData();
        await renderApprovals();
        showToast(approved ? "Approval granted." : "Approval rejected.", approved ? "success" : "warning");
        break;
      }
      case "provider": {
        const bindAgent = String(formData.get("bind_agent") || "").trim();
        const savedProvider = await apiFetch("/providers", {
          method: "POST",
          body: {
            id: String(formData.get("id") || "").trim(),
            name: String(formData.get("name") || "").trim(),
            type: String(formData.get("type") || "").trim(),
            provider: String(formData.get("provider") || "").trim(),
            base_url: String(formData.get("base_url") || "").trim(),
            api_key: String(formData.get("api_key") || "").trim(),
            default_model: String(formData.get("default_model") || "").trim(),
            capabilities: parseCSV(String(formData.get("capabilities") || "")),
            enabled: formData.get("enabled") === "on",
          },
        });
        await refreshCoreData();
        closeModal();
        if (event.submitter?.value === "save-bind" && bindAgent && savedProvider?.id) {
          openBindingModal([bindAgent], { providerRef: savedProvider.id, model: savedProvider.default_model || "" });
        } else {
          await renderApp();
        }
        showToast("Provider saved.", "success");
        break;
      }
      case "binding": {
        const agents = parseCSV(String(formData.get("agents") || ""));
        if (!agents.length) throw new Error("Select at least one agent.");
        await apiFetch("/agent-bindings", {
          method: "POST",
          body: {
            agents,
            provider_ref: String(formData.get("provider_ref") || "").trim(),
            model: String(formData.get("model") || "").trim(),
          },
        });
        state.selectedBindingAgents = new Set();
        await refreshCoreData();
        closeModal();
        await renderModelCenter();
        showToast(`Updated ${agents.length} agent binding${agents.length === 1 ? "" : "s"}.`, "success");
        break;
      }
      case "agent": {
        const skills = parseCSV(String(formData.get("skills") || "")).map((name) => ({ name, enabled: true }));
        await apiFetch("/assistants", {
          method: "POST",
          body: {
            name: String(formData.get("name") || "").trim(),
            description: String(formData.get("description") || "").trim(),
            role: String(formData.get("role") || "").trim(),
            persona: String(formData.get("persona") || "").trim(),
            working_dir: String(formData.get("working_dir") || "").trim(),
            permission_level: String(formData.get("permission_level") || "").trim(),
            provider_ref: String(formData.get("provider_ref") || "").trim(),
            default_model: String(formData.get("default_model") || "").trim(),
            enabled: formData.get("enabled") === "on",
            skills,
          },
        });
        await refreshCoreData();
        closeModal();
        await renderAgents();
        showToast("Agent saved.", "success");
        break;
      }
      case "resource": {
        const kind = String(formData.get("kind") || "").trim();
        let body = {};
        if (kind === "org") {
          body = {
            org: {
              id: String(formData.get("id") || "").trim(),
              name: String(formData.get("name") || "").trim(),
            },
          };
        } else if (kind === "project") {
          body = {
            project: {
              id: String(formData.get("id") || "").trim(),
              name: String(formData.get("name") || "").trim(),
              org_id: String(formData.get("org_id") || "").trim(),
            },
          };
        } else if (kind === "workspace") {
          body = {
            workspace: {
              id: String(formData.get("id") || "").trim(),
              name: String(formData.get("name") || "").trim(),
              project_id: String(formData.get("project_id") || "").trim(),
              path: String(formData.get("path") || "").trim(),
            },
          };
        }
        await apiFetch("/resources", { method: "POST", body });
        await refreshCoreData();
        closeModal();
        await renderResources();
        showToast("Resource saved.", "success");
        break;
      }
      case "user": {
        await apiFetch("/auth/users", {
          method: "POST",
          body: {
            name: String(formData.get("name") || "").trim(),
            token: String(formData.get("token") || "").trim(),
            role: String(formData.get("role") || "").trim(),
            permission_overrides: parseCSV(String(formData.get("permission_overrides") || "")),
            scopes: parseCSV(String(formData.get("scopes") || "")),
            orgs: parseCSV(String(formData.get("orgs") || "")),
            projects: parseCSV(String(formData.get("projects") || "")),
            workspaces: parseCSV(String(formData.get("workspaces") || "")),
          },
        });
        closeModal();
        await renderSettings();
        showToast("User saved.", "success");
        break;
      }
      case "role": {
        await apiFetch("/auth/roles", {
          method: "POST",
          body: {
            name: String(formData.get("name") || "").trim(),
            description: String(formData.get("description") || "").trim(),
            permissions: parseCSV(String(formData.get("permissions") || "")),
          },
        });
        closeModal();
        await renderSettings();
        showToast("Role saved.", "success");
        break;
      }
      default:
        break;
    }
  } catch (error) {
    console.error(error);
    showToast(error.message || "Submit failed", "danger");
  }
}

function handleChange(event) {
  const type = event.target.dataset.change;
  if (type === "workspace") {
    state.selectedWorkspace = event.target.value;
    state.selectedSessionId = "";
    setWorkbenchSessionsOpen(false);
    state.workbenchDraft = true;
    state.workbenchDraftMessage = "";
    clearWorkbenchPendingState();
    localStorage.setItem("anyclaw_workspace", state.selectedWorkspace);
    void renderApp();
    return;
  }
  if (type === "workbench-assistant") {
    state.workbenchDraftMessage =
      pageEl.querySelector('form[data-form="chat"] textarea[name="message"]')?.value || state.workbenchDraftMessage;
    setWorkbenchAssistant(event.target.value);
    if (state.route === "workbench") {
      void renderWorkbench();
    }
    return;
  }
  if (type === "binding-pick") {
    const agent = event.target.dataset.agent || "";
    if (!agent) return;
    if (event.target.checked) state.selectedBindingAgents.add(agent);
    else state.selectedBindingAgents.delete(agent);
    if (state.route === "model-center" && state.section === "bindings") {
      void renderModelCenter();
    }
    return;
  }
  if (type === "binding-all") {
    const shouldSelect = event.target.checked;
    state.selectedBindingAgents = shouldSelect ? new Set(state.bindings.map((binding) => binding.name)) : new Set();
    if (state.route === "model-center" && state.section === "bindings") {
      void renderModelCenter();
    }
  }
}

function fillWorkbenchPrompt(prompt) {
  state.workbenchDraft = true;
  state.selectedSessionId = "";
  state.workbenchDraftMessage = String(prompt || "");
  const textarea =
    pageEl.querySelector('.composer-hero textarea[name="message"]') ||
    pageEl.querySelector('form[data-form="chat"] textarea[name="message"]');
  if (!textarea) return;
  textarea.value = state.workbenchDraftMessage;
  textarea.focus();
  textarea.setSelectionRange(textarea.value.length, textarea.value.length);
}

async function openChannelDetail(id) {
  if (!id) return;
  const channel = await apiFetch(`/sessions/${encodeURIComponent(id)}`);
  const messages = sessionMessages(channel);
  openModal({
    title: channel.title || id,
    description: `${sessionKindLabel(channel)} · ${sessionMemberSummary(channel)}`,
    body: `
      <div class="stack">
        <article class="list-row">
          <div class="event-header">
            <span class="event-title">频道状态</span>
            ${statusChip(channel.presence || "idle", channel.presence || "idle", true)}
          </div>
          <div class="muted">${escapeHTML(channel.last_user_text || channel.last_assistant_text || "暂无消息")}</div>
        </article>
        <div>
          <div class="muted">频道消息</div>
          ${messages.length
            ? `<div class="stack">${messages.slice(-8).map((message) => renderChannelMessage(message, {
              assistantName: sessionPrimaryAssistant(channel),
            })).join("")}</div>`
            : emptyState("暂无消息", "这个频道还没有任何聊天或任务记录。")}
        </div>
        <div>
          <div class="muted">频道数据</div>
          ${renderJSON(channel)}
        </div>
      </div>
    `,
  });
}

async function openTaskDetail(id) {
  if (!id) return;
  const details = await apiFetch(`/tasks/${encodeURIComponent(id)}`);
  const task = details?.task || {};
  openModal({
    title: task.title || id,
    description: task.assistant ? `智能体：${task.assistant}` : "任务详情",
    body: `
      <div class="stack">
        <article class="list-row">
          <div class="event-header">
            <span class="event-title">状态</span>
            ${statusChip(task.status || "unknown", task.status || "unknown", true)}
          </div>
          <div class="muted">${escapeHTML(task.result || task.error || task.input || "")}</div>
        </article>
        <div>
          <div class="muted">任务数据</div>
          ${renderJSON(task)}
        </div>
        <div>
          <div class="muted">执行步骤</div>
          ${details?.steps?.length
        ? `<div class="stack">${details.steps.map((step) => `
                    <article class="list-row">
                      <div class="event-header">
                        <span class="event-title">${escapeHTML(`${step.index + 1}. ${step.title}`)}</span>
                        ${statusChip(step.status || "unknown", step.status || "unknown", true)}
                      </div>
                      <div class="muted">${escapeHTML(step.output || step.error || step.input || "")}</div>
                    </article>
                  `).join("")}</div>`
        : emptyState("暂无步骤", "这个任务没有保存逐步执行记录。")
      }
        </div>
        <div>
          <div class="muted">审批记录</div>
          ${renderJSON(details?.approvals || [])}
        </div>
      </div>
    `,
  });
};

function openProviderModal(providerId = "", options = {}) {
  const provider = providerId ? findProvider(providerId) : null;
  const bindAgent = String(options.bindAgent || "").trim();
  openModal({
    title: provider ? `编辑 ${provider.name}` : "添加供应商",
    description: "直接在控制台中保存供应商凭证和默认模型，无需手动修改配置文件。",
    body: `
      <div class="stack">
        ${bindAgent ? `<article class="list-row"><div class="event-header"><span class="event-title">保存后绑定</span><span class="muted">${escapeHTML(bindAgent)}</span></div><div class="muted">你可以保存供应商后，直接为当前智能体打开绑定面板。</div></article>` : ""}
        <div class="inline-actions">
          ${PROVIDER_PRESETS.map((preset) => `
            <button type="button" class="btn btn-ghost" data-action="apply-provider-preset" data-preset="${preset.id}">
              ${escapeHTML(preset.name)}
            </button>
          `).join("")}
        </div>
        <form class="stack" data-form="provider">
          <input type="hidden" name="bind_agent" value="${escapeHTML(bindAgent)}" />
          <label class="field">
            <span>名称</span>
            <input class="input" name="name" type="text" value="${escapeHTML(provider?.name || "")}" placeholder="OpenAI Production" required />
          </label>
          <label class="field">
            <span>ID</span>
            <input class="input" name="id" type="text" value="${escapeHTML(provider?.id || "")}" placeholder="openai-prod" />
          </label>
          <label class="field">
            <span>显示类型</span>
            <input class="input" name="type" type="text" value="${escapeHTML(provider?.type || "")}" placeholder="openai / anthropic / ollama" />
          </label>
          <label class="field">
            <span>运行时供应商</span>
            <input class="input" name="provider" type="text" value="${escapeHTML(provider?.provider || "")}" placeholder="openai" required />
          </label>
          <label class="field">
            <span>Base URL</span>
            <input class="input" name="base_url" type="url" value="${escapeHTML(provider?.base_url || "")}" placeholder="https://api.openai.com/v1" />
          </label>
          <label class="field">
            <span>API Key</span>
            <input class="input" name="api_key" type="password" value="" placeholder="${escapeHTML(provider?.has_api_key ? "Leave empty to keep the current key." : "sk-...")}" />
          </label>
          <label class="field">
            <span>默认模型</span>
            <input class="input" name="default_model" type="text" value="${escapeHTML(provider?.default_model || "")}" placeholder="gpt-4o-mini" />
          </label>
          <label class="field">
            <span>能力标签</span>
            <input class="input" name="capabilities" type="text" value="${escapeHTML((provider?.capabilities || []).join(", "))}" placeholder="chat, reasoning, vision" />
          </label>
          <label class="field">
            <span><input type="checkbox" name="enabled" ${provider?.enabled === false ? "" : "checked"} /> 启用</span>
          </label>
          <div class="inline-actions">
            <button type="submit" class="btn btn-primary" value="save" name="intent">保存供应商</button>
            ${bindAgent && !provider ? `<button type="submit" class="btn btn-secondary" value="save-bind" name="intent">保存并绑定当前智能体</button>` : ""}
            ${provider ? `<button type="button" class="btn btn-secondary" data-action="test-provider" data-id="${escapeHTML(provider.id)}">Test Existing Config</button>` : ""}
          </div>
        </form>
      </div>
    `,
  });
}

function openWorkbenchAgentPicker() {
  const selectedName = resolveWorkbenchAssistant(resolveWorkbenchSession(state.sessions));
  openModal({
    title: "切换智能体",
    description: "选择当前对话要交给哪个智能体处理，也可以直接调整它的供应商和模型。",
    compact: true,
    body: `
      <div class="stack">
        <div class="agent-picker-list">
          ${renderWorkbenchAssistantStrip(selectedName)}
        </div>
        <div class="inline-actions">
          <button type="button" class="btn btn-ghost" data-action="open-agent-modal">创建智能体</button>
          ${selectedName ? `<button type="button" class="btn btn-secondary" data-action="open-binding-modal" data-agent="${escapeHTML(selectedName)}">配置当前智能体</button>` : ""}
          ${selectedName ? `<button type="button" class="btn btn-ghost" data-action="open-provider-modal" data-bind-agent="${escapeHTML(selectedName)}">添加供应商</button>` : ""}
        </div>
      </div>
    `,
  });
}

function applyProviderPreset(presetId) {
  const preset = PROVIDER_PRESETS.find((item) => item.id === presetId);
  const form = modalRoot.querySelector('form[data-form="provider"]');
  if (!preset || !form) return;
  form.elements.name.value = form.elements.name.value || preset.name;
  form.elements.id.value = form.elements.id.value || preset.id;
  form.elements.type.value = preset.type || "";
  form.elements.provider.value = preset.provider || "";
  form.elements.base_url.value = preset.baseURL || "";
  form.elements.default_model.value = preset.defaultModel || "";
  form.elements.capabilities.value = (preset.capabilities || []).join(", ");
}

function openBindingModal(agentNames, defaults = {}) {
  const agents = agentNames.filter(Boolean);
  const binding = agents.length === 1 ? findBinding(agents[0]) : null;
  const providerRef = defaults.providerRef ?? binding?.provider_ref ?? "";
  const modelValue = defaults.model ?? binding?.model ?? "";
  openModal({
    title: agents.length === 1 ? `切换 ${agents[0]}` : "批量切换智能体",
    description: "为每个智能体指定供应商或模型；如果留空，则继承全局默认配置。",
    compact: true,
    body: `
      <form class="stack" data-form="binding">
        <input type="hidden" name="agents" value="${escapeHTML(agents.join(", "))}" />
        <article class="list-row">
          <div class="event-header">
            <span class="event-title">Selected agents</span>
            <span class="muted">${agents.length}</span>
          </div>
          <div class="mono">${escapeHTML(agents.join(", "))}</div>
        </article>
        <label class="field">
          <span>供应商</span>
          <select class="select" name="provider_ref">
            ${renderProviderOptions(providerRef)}
          </select>
        </label>
        <label class="field">
          <span>模型覆盖</span>
          <input class="input" name="model" type="text" value="${escapeHTML(modelValue)}" placeholder="Leave empty to inherit the provider default model" />
        </label>
        <div class="inline-actions">
          <button type="submit" class="btn btn-primary">Apply Binding</button>
        </div>
      </form>
    `,
  });
}

function openBulkBindingModal() {
  const agents = [...state.selectedBindingAgents];
  if (!agents.length) {
    showToast("Select one or more agents first.", "warning");
    return;
  }
  openBindingModal(agents);
}

function openAgentModal(name = "") {
  const assistant = name ? findAssistant(name) : null;
  openModal({
    title: assistant ? `编辑 ${assistant.name}` : "创建智能体",
    description: "设置工作目录、权限等级，以及可选的默认供应商配置。",
    body: `
      <form class="stack" data-form="agent">
        <label class="field">
          <span>Name</span>
          <input class="input" name="name" type="text" value="${escapeHTML(assistant?.name || "")}" placeholder="frontend-reviewer" ${assistant ? "readonly" : ""} required />
        </label>
        <label class="field">
          <span>Description</span>
          <textarea class="textarea" name="description" placeholder="What this agent is responsible for.">${escapeHTML(assistant?.description || "")}</textarea>
        </label>
        <label class="field">
          <span>Role</span>
          <input class="input" name="role" type="text" value="${escapeHTML(assistant?.role || "")}" placeholder="reviewer / coder / researcher" />
        </label>
        <label class="field">
          <span>Persona</span>
          <input class="input" name="persona" type="text" value="${escapeHTML(assistant?.persona || "")}" placeholder="Calm reviewer, aggressive debugger, etc." />
        </label>
        <label class="field">
          <span>Working directory</span>
          <input class="input" name="working_dir" type="text" value="${escapeHTML(assistant?.working_dir || "")}" placeholder="pkg/gateway" />
        </label>
        <label class="field">
          <span>Permission level</span>
          <select class="select" name="permission_level">
            ${renderPermissionOptions(assistant?.permission_level || "limited")}
          </select>
        </label>
        <label class="field">
          <span>Provider</span>
          <select class="select" name="provider_ref">
            ${renderProviderOptions(assistant?.provider_ref || "")}
          </select>
        </label>
        <label class="field">
          <span>Default model</span>
          <input class="input" name="default_model" type="text" value="${escapeHTML(assistant?.default_model || "")}" placeholder="Optional agent-specific model" />
        </label>
        <label class="field">
          <span>Skills</span>
          <input class="input" name="skills" type="text" value="${escapeHTML((assistant?.skills || []).map((skill) => skill.name).join(", "))}" placeholder="comma-separated skill names" />
        </label>
        <label class="field">
          <span><input type="checkbox" name="enabled" ${assistant?.enabled === false ? "" : "checked"} /> Enabled</span>
        </label>
        <div class="inline-actions">
          <button type="submit" class="btn btn-primary">Save Agent</button>
        </div>
      </form>
    `,
  });
}

function openResourceModal(kind, id = "") {
  const resource = findResource(kind, id);
  let body = "";

  if (kind === "org") {
    body = `
      <form class="stack" data-form="resource">
        <input type="hidden" name="kind" value="org" />
        <label class="field">
          <span>ID</span>
          <input class="input" name="id" type="text" value="${escapeHTML(resource?.id || "")}" placeholder="org-local" required />
        </label>
        <label class="field">
          <span>Name</span>
          <input class="input" name="name" type="text" value="${escapeHTML(resource?.name || "")}" placeholder="Local Org" required />
        </label>
        <div class="inline-actions">
          <button type="submit" class="btn btn-primary">Save Organization</button>
        </div>
      </form>
    `;
  } else if (kind === "project") {
    body = `
      <form class="stack" data-form="resource">
        <input type="hidden" name="kind" value="project" />
        <label class="field">
          <span>ID</span>
          <input class="input" name="id" type="text" value="${escapeHTML(resource?.id || "")}" placeholder="project-local" required />
        </label>
        <label class="field">
          <span>Name</span>
          <input class="input" name="name" type="text" value="${escapeHTML(resource?.name || "")}" placeholder="Local Project" required />
        </label>
        <label class="field">
          <span>Organization</span>
          <select class="select" name="org_id">
            ${(state.resources?.orgs || []).map((org) => `<option value="${escapeHTML(org.id)}" ${org.id === resource?.org_id ? "selected" : ""}>${escapeHTML(org.name)}</option>`).join("")}
          </select>
        </label>
        <div class="inline-actions">
          <button type="submit" class="btn btn-primary">Save Project</button>
        </div>
      </form>
    `;
  } else if (kind === "workspace") {
    body = `
      <form class="stack" data-form="resource">
        <input type="hidden" name="kind" value="workspace" />
        <label class="field">
          <span>ID</span>
          <input class="input" name="id" type="text" value="${escapeHTML(resource?.id || "")}" placeholder="workspace-default" required />
        </label>
        <label class="field">
          <span>Name</span>
          <input class="input" name="name" type="text" value="${escapeHTML(resource?.name || "")}" placeholder="Main Workspace" required />
        </label>
        <label class="field">
          <span>Project</span>
          <select class="select" name="project_id">
            ${(state.resources?.projects || []).map((project) => `<option value="${escapeHTML(project.id)}" ${project.id === resource?.project_id ? "selected" : ""}>${escapeHTML(project.name)}</option>`).join("")}
          </select>
        </label>
        <label class="field">
          <span>Path</span>
          <input class="input" name="path" type="text" value="${escapeHTML(resource?.path || "")}" placeholder="D:\\repo" required />
        </label>
        <div class="inline-actions">
          <button type="submit" class="btn btn-primary">Save Workspace</button>
        </div>
      </form>
    `;
  }

  openModal({
    title: `${resource ? "编辑" : "新增"}${localizeResourceKind(kind)}`,
    description: "资源层级决定了会话路由与运行时隔离边界。",
    compact: true,
    body,
  });
}

function openUserModal() {
  openModal({
    title: "新增用户",
    description: "由于读取接口不会返回历史密钥，因此创建用户时需要直接填写访问令牌。",
    body: `
      <form class="stack" data-form="user">
        <label class="field"><span>Name</span><input class="input" name="name" type="text" placeholder="operator-alice" required /></label>
        <label class="field"><span>Token</span><input class="input" name="token" type="password" placeholder="Bearer token" required /></label>
        <label class="field">
          <span>Role</span>
          <select class="select" name="role">
            <option value="">No role</option>
            ${state.roles.map((role) => `<option value="${escapeHTML(role.name)}">${escapeHTML(role.name)}</option>`).join("")}
          </select>
        </label>
        <label class="field"><span>Permission overrides</span><input class="input" name="permission_overrides" type="text" placeholder="config.write, tasks.write" /></label>
        <label class="field"><span>Scopes</span><input class="input" name="scopes" type="text" placeholder="workspace-default" /></label>
        <label class="field"><span>Organizations</span><input class="input" name="orgs" type="text" placeholder="org-local" /></label>
        <label class="field"><span>Projects</span><input class="input" name="projects" type="text" placeholder="project-local" /></label>
        <label class="field"><span>Workspaces</span><input class="input" name="workspaces" type="text" placeholder="workspace-default" /></label>
        <div class="inline-actions"><button type="submit" class="btn btn-primary">Save User</button></div>
      </form>
    `,
  });
}

function openRoleModal(name = "") {
  const role = state.roles.find((item) => item.name === name) || null;
  openModal({
    title: role ? `编辑 ${role.name}` : "新增角色",
    description: "自定义角色可以进一步收窄管理员或查看者的权限边界。",
    body: `
      <form class="stack" data-form="role">
        <label class="field"><span>Name</span><input class="input" name="name" type="text" value="${escapeHTML(role?.name || "")}" placeholder="release-manager" ${role ? "readonly" : ""} required /></label>
        <label class="field"><span>Description</span><textarea class="textarea" name="description" placeholder="Explain who should use this role.">${escapeHTML(role?.description || "")}</textarea></label>
        <label class="field"><span>Permissions</span><input class="input" name="permissions" type="text" value="${escapeHTML((role?.permissions || []).join(", "))}" placeholder="status.read, config.read" /></label>
        <div class="inline-actions"><button type="submit" class="btn btn-primary">Save Role</button></div>
      </form>
    `,
  });
}

async function deleteProvider(id) {
  if (!id) return;
  if (!window.confirm(`确认删除供应商“${id}”？正在使用它的智能体会回退到全局默认配置。`)) return;
  await apiFetch(`/providers?id=${encodeURIComponent(id)}`, { method: "DELETE" });
  await refreshCoreData();
  await renderApp();
  showToast("Provider deleted.", "success");
}

async function testProvider(id) {
  if (!id) return;
  const provider = findProvider(id);
  if (!provider) throw new Error("Provider not found.");
  const result = await apiFetch("/providers/test", {
    method: "POST",
    body: {
      id: provider.id,
      name: provider.name,
      type: provider.type,
      provider: provider.provider,
      base_url: provider.base_url,
      default_model: provider.default_model,
      enabled: provider.enabled,
    },
  });
  showToast(result?.message || result?.status || "Provider test finished.", result?.ok ? "success" : "warning");
}

async function deleteAgent(name) {
  if (!name) return;
  if (!window.confirm(`确认删除智能体“${name}”？`)) return;
  await apiFetch(`/assistants?name=${encodeURIComponent(name)}`, { method: "DELETE" });
  await refreshCoreData();
  await renderAgents();
  showToast("Agent deleted.", "success");
}

async function refreshRuntime(dataset) {
  await apiFetch("/runtimes/refresh", {
    method: "POST",
    body: {
      agent: dataset.agent || "",
      org: dataset.org || "",
      project: dataset.project || "",
      workspace: dataset.workspace || "",
    },
  });
  await renderObservability();
  showToast("Runtime refreshed.", "success");
}

async function refreshAllRuntimes() {
  if (!state.runtimes.length) {
    showToast("No runtimes to refresh.", "warning");
    return;
  }
  await apiFetch("/runtimes/refresh-batch", {
    method: "POST",
    body: {
      items: state.runtimes.map((runtime) => ({
        agent: runtime.agent,
        org: runtime.org,
        project: runtime.project,
        workspace: runtime.workspace,
      })),
    },
  });
  showToast("Runtime refresh batch queued.", "success");
  await renderObservability();
}

async function retryJob(id) {
  await apiFetch("/jobs/retry", { method: "POST", body: { job_id: id } });
  await renderObservability();
  showToast("Retry queued.", "success");
}

async function cancelJob(id) {
  await apiFetch("/jobs/cancel", { method: "POST", body: { job_id: id } });
  await renderObservability();
  showToast("Job cancelled.", "warning");
}

async function openStoreDetail(id) {
  if (!id) return;
  const item = await apiFetch(`/v2/store/${encodeURIComponent(id)}`);
  openModal({
    title: item.display_name || item.name || id,
    description: item.category || "Agent package",
    body: `
      <div class="stack">
        <article class="list-row">
          <div class="event-header">
            <span class="event-title">Author</span>
            <span class="muted">${escapeHTML(item.version || "v1")}</span>
          </div>
          <div class="muted">${escapeHTML(item.author || "unknown")}</div>
        </article>
        <div class="muted">${escapeHTML(item.description || "No description provided.")}</div>
        <div class="badge-row">
          ${(item.tags || []).map((tag) => `<span class="tag">${escapeHTML(tag)}</span>`).join("")}
        </div>
        <div>
          <div class="muted">Package detail</div>
          ${renderJSON(item)}
        </div>
        <div class="inline-actions">
          ${item.installed_at
        ? `<button class="btn btn-danger" data-action="uninstall-store" data-id="${escapeHTML(item.id)}">Uninstall</button>`
        : `<button class="btn btn-primary" data-action="install-store" data-id="${escapeHTML(item.id)}">Install</button>`
      }
        </div>
      </div>
    `,
  });
}

async function installStore(id) {
  await apiFetch(`/v2/store/${encodeURIComponent(id)}/install`, { method: "POST" });
  state.storeItems = (await safeFetch("/v2/store", {}, [])) || [];
  closeModal();
  await renderStore();
  showToast("Package installed.", "success");
}

async function uninstallStore(id) {
  await apiFetch(`/v2/store/${encodeURIComponent(id)}/uninstall`, { method: "POST" });
  state.storeItems = (await safeFetch("/v2/store", {}, [])) || [];
  closeModal();
  await renderStore();
  showToast("Package uninstalled.", "warning");
}

async function deleteResource(kind, id) {
  if (!kind || !id) return;
  if (!window.confirm(`确认删除${localizeResourceKind(kind)}“${id}”？`)) return;
  await apiFetch(`/resources?kind=${encodeURIComponent(kind)}&id=${encodeURIComponent(id)}`, { method: "DELETE" });
  await refreshCoreData();
  await renderResources();
  showToast(`${kind} deleted.`, "success");
}

async function deleteUser(name) {
  if (!window.confirm(`确认删除用户“${name}”？`)) return;
  await apiFetch(`/auth/users?name=${encodeURIComponent(name)}`, { method: "DELETE" });
  await renderSettings();
  showToast("User deleted.", "success");
}

async function deleteRole(name) {
  if (!window.confirm(`确认删除角色“${name}”？`)) return;
  await apiFetch(`/auth/roles?name=${encodeURIComponent(name)}`, { method: "DELETE" });
  await renderSettings();
  showToast("Role deleted.", "success");
}

function openModal({ title, description = "", body = "", compact = false }) {
  modalRoot.innerHTML = localizeHTML(`
    <div class="modal-overlay" data-action="modal-backdrop">
      <div class="modal-panel ${compact ? "is-compact" : ""}" role="dialog" aria-modal="true" aria-label="${escapeHTML(localizeText(title))}">
        <div class="modal-header">
          <div class="section-header">
            <div>
              <h3>${escapeHTML(localizeText(title))}</h3>
              ${description ? `<p class="muted">${escapeHTML(localizeText(description))}</p>` : ""}
            </div>
            <button class="btn btn-ghost" data-action="close-modal">Close</button>
          </div>
        </div>
        <div class="modal-body">${localizeHTML(body)}</div>
      </div>
    </div>
  `);
}

function closeModal() {
  modalRoot.innerHTML = "";
}

function showToast(message, tone = "info") {
  const toast = document.createElement("div");
  toast.className = `toast is-${tone}`;
  toast.textContent = localizeText(message);
  toastRoot.appendChild(toast);
  window.setTimeout(() => {
    toast.remove();
  }, 4000);
}

function showAuthOverlay() {
  authOverlay.classList.remove("hidden");
  const input = authOverlay.querySelector('input[name="token"]');
  if (input) input.focus();
}

function hideAuthOverlay() {
  authOverlay.classList.add("hidden");
}

async function renderWorkbench() {
  const workspace = currentWorkspace();
  if (!workspace) {
    pageEl.innerHTML = localizeHTML(emptyState("No workspace selected", "Create or select a workspace before starting a session."));
    return;
  }

  state.sessions = (await safeFetch(`/sessions?workspace=${encodeURIComponent(workspace.id)}`, {}, [])) || [];
  const selectedSession = resolveWorkbenchSession(state.sessions);
  const activeAssistantName = resolveWorkbenchAssistant(selectedSession);
  const sessionCount = state.sessions.length;
  const selectedMessages = sessionMessages(selectedSession);
  const pendingAssistantName = state.workbenchPendingAssistant || activeAssistantName || "智能体";
  const optimisticMessages = [...selectedMessages];
  const optimisticOnSession =
    state.workbenchSubmitting &&
    selectedSession &&
    state.workbenchPendingMessage &&
    state.workbenchPendingSessionId === selectedSession.id;

  if (optimisticOnSession) {
    optimisticMessages.push({ role: "user", content: state.workbenchPendingMessage, pending: true });
    optimisticMessages.push({ role: "assistant", content: "", pending: true, thinking: true });
  }

  const pendingOnDraft = state.workbenchSubmitting && !state.workbenchPendingSessionId && state.workbenchPendingMessage;
  const workspaceOptions = (state.resources?.workspaces || []).map((ws) => {
    const selected = ws.id === state.selectedWorkspace ? "selected" : "";
    return `<option value="${escapeHTML(ws.id)}" ${selected}>${escapeHTML(ws.name)}</option>`;
  });

  pageEl.innerHTML = localizeHTML(`
    <div class="workbench-layout">
      ${renderWorkbenchSessionsDrawer(state.sessions, selectedSession, sessionCount)}
      <section class="panel workbench-chat-surface ${selectedSession ? "has-session" : "is-draft"}">
        <div class="workbench-toolbar">
          <div class="workbench-toolbar-left">
            <button
              type="button"
              class="btn btn-ghost btn-inline workbench-toolbar-toggle"
              data-action="toggle-workbench-sessions"
              title="历史会话"
              aria-label="切换会话列表"
            >
              <span class="workbench-toolbar-toggle-icon">≡</span>
              ${sessionCount ? `<span class="workbench-toolbar-toggle-count">${sessionCount}</span>` : ""}
            </button>
            <label class="workspace-picker workspace-picker-compact workbench-toolbar-workspace">
              <select class="select" data-change="workspace" aria-label="选择工作区">
                ${workspaceOptions.length ? workspaceOptions.join("") : '<option value="">无工作区</option>'}
              </select>
            </label>
          </div>
          <div class="workbench-toolbar-right">
            ${renderWorkbenchAgentButton(activeAssistantName)}
          </div>
        </div>
        ${selectedSession
      ? `
          <div class="workbench-chat-topline">
            <div class="workbench-chat-copy">
              <div class="eyebrow">当前会话</div>
              <h2>${escapeHTML(selectedSession.title || "未命名会话")}</h2>
            </div>
            <div class="badge-row workbench-chat-badges">
              <span class="provider-chip">${escapeHTML(activeAssistantName || "未指定智能体")}</span>
              ${statusChip(selectedSession.presence || "idle", "状态", true)}
            </div>
          </div>
          ${optimisticMessages.length
        ? `
          <div class="message-thread workbench-thread">
            ${optimisticMessages.map((message) => renderChannelMessage(message, {
          assistantName: pendingAssistantName,
          pending: Boolean(message?.pending),
          thinking: Boolean(message?.thinking),
        })).join("")}
          </div>
        `
        : `
          <div class="workbench-chat-empty">
            <h3>这个会话还没有消息</h3>
            <p class="muted">从下方输入框开始，让 ${escapeHTML(activeAssistantName || "当前智能体")} 接手这个新会话。</p>
          </div>
        `
      }
          ${renderWorkbenchComposer({
        assistantName: activeAssistantName,
        sessionId: selectedSession.id,
      })}
        `
      : `
          <div class="workbench-home">
            <div class="workbench-home-shell">
              <div class="workbench-home-copy">
                <div class="eyebrow workbench-home-eyebrow">AnyClaw 工作台</div>
                <h1>今天准备推进什么工作？</h1>
                <p class="muted">直接输入目标、问题或下一步动作，系统会把当前工作区和所选智能体作为上下文，一次接住你的意图。</p>
              </div>
              <div class="workbench-home-statusbar" aria-label="当前上下文">
                <article class="workbench-home-status">
                  <span class="workbench-home-status-icon" aria-hidden="true">${renderUIIcon("folder")}</span>
                  <div class="workbench-home-status-copy">
                    <span>当前工作区</span>
                    <strong>${escapeHTML(workspace.name || "未命名工作区")}</strong>
                  </div>
                </article>
                <article class="workbench-home-status">
                  <span class="workbench-home-status-icon" aria-hidden="true">${renderUIIcon("bot")}</span>
                  <div class="workbench-home-status-copy">
                    <span>当前智能体</span>
                    <strong>${escapeHTML(activeAssistantName || "点击右上角选择智能体")}</strong>
                  </div>
                </article>
                <article class="workbench-home-status">
                  <span class="workbench-home-status-icon" aria-hidden="true">${renderUIIcon("cpu")}</span>
                  <div class="workbench-home-status-copy">
                    <span>模型路由</span>
                    <strong>按智能体独立切换供应商与模型</strong>
                  </div>
                </article>
              </div>
              ${pendingOnDraft
        ? `
                <div class="workbench-home-pending" aria-live="polite">
                  <div class="workbench-home-pending-label">正在创建对话</div>
                  <div class="workbench-home-pending-text">${escapeHTML(state.workbenchPendingMessage)}</div>
                </div>
              `
        : ""}
            </div>
            <div class="workbench-home-composer-wrap">
              ${renderWorkbenchComposer({
        assistantName: activeAssistantName,
        hero: true,
      })}
            </div>
            <div class="workbench-home-quick-actions">
              ${WORKBENCH_EXAMPLES.slice(0, 3).map((item) => `
                <button type="button" class="suggestion-chip" data-action="fill-prompt" data-prompt="${escapeHTML(item.prompt)}" ${state.workbenchSubmitting ? "disabled" : ""}>
                  ${escapeHTML(item.title)}
                </button>
              `).join("")}
            </div>
          </div>
        `
    }
      </section>
    </div>
  `);

  pageEl.querySelectorAll(
    ".workbench-toolbar-toggle, .workbench-session-backdrop, .workbench-session-drawer [data-action='select-session'], .workbench-session-drawer [data-action='new-session']",
  ).forEach((button) => {
    button.addEventListener("click", (event) => {
      event.preventDefault();
      event.stopPropagation();
      void handleClick(button, event);
    });
  });
}

async function renderTasks() {
  const workspace = currentWorkspace();
  if (!workspace) {
    pageEl.innerHTML = localizeHTML(emptyState("No workspace selected", "Select a workspace before creating tasks."));
    return;
  }

  state.tasks = (await safeFetch(`/tasks?workspace=${encodeURIComponent(workspace.id)}`, {}, [])) || [];
  const completedCount = state.tasks.filter((task) => task.status === "completed").length;
  const waitingCount = state.tasks.filter((task) => task.status === "waiting_approval").length;

  pageEl.innerHTML = localizeHTML(`
    <div class="page-grid tasks-page">
      <div class="stats-grid tasks-stats-grid">
        ${metricCard("任务总数", state.tasks.length, workspace.name, "info")}
        ${metricCard("已完成", completedCount, "执行成功", "success")}
        ${metricCard("待审批", waitingCount, "需要人工确认", "warning")}
        ${metricCard("失败", state.tasks.filter((task) => task.status === "failed").length, "需要排查", "danger")}
      </div>

      <div class="two-col tasks-layout">
        <section class="panel tasks-panel tasks-form-panel">
          <div class="section-header section-header-tight">
            <div>
              <h3>创建任务</h3>
              <p class="muted">适合需要结构化推进的工作。调研、修改、执行和回归检查，都可以从这里直接发起。</p>
            </div>
          </div>
          <form class="stack tasks-form" data-form="task">
            <label class="field">
              <span>任务标题</span>
              <input class="input" type="text" name="title" placeholder="例如：检查供应商切换逻辑" />
            </label>
            <label class="field">
              <span>执行智能体</span>
              <select class="select" name="assistant">
                ${renderAssistantOptions(state.assistants[0]?.name || "")}
              </select>
            </label>
            <label class="field">
              <span>任务内容</span>
              <textarea class="textarea" name="input" placeholder="描述目标、问题或预期结果，例如要排查什么、修改什么、做到什么程度。" required></textarea>
            </label>
            <div class="inline-actions">
              <button type="submit" class="btn btn-primary">运行任务</button>
              <button type="button" class="btn btn-ghost" data-action="navigate" data-route="workbench">进入对话工作台</button>
            </div>
          </form>
        </section>

        <section class="panel tasks-panel tasks-list-panel">
          <div class="section-header section-header-tight">
            <div>
              <h3>最近任务</h3>
              <p class="muted">集中查看最近执行记录、结果摘要与状态变化。</p>
            </div>
            <span class="provider-chip">${state.tasks.length}</span>
          </div>
          ${state.tasks.length
      ? `
            <div class="table-wrap tasks-table-wrap">
              <table class="tasks-table">
                <thead>
                  <tr>
                    <th>任务</th>
                    <th>智能体</th>
                    <th>状态</th>
                    <th>更新时间</th>
                    <th></th>
                  </tr>
                </thead>
                <tbody>
                  ${state.tasks.map((task) => `
                    <tr>
                      <td>
                        <div class="event-title">${escapeHTML(task.title || task.id)}</div>
                        <div class="muted">${escapeHTML(task.result || task.error || task.input || "")}</div>
                      </td>
                      <td>${escapeHTML(task.assistant || "-")}</td>
                      <td>${statusChip(task.status || "unknown", task.status || "unknown", true)}</td>
                      <td>${formatTime(task.last_updated_at || task.completed_at || task.created_at)}</td>
                      <td><button class="btn btn-ghost" data-action="open-task-detail" data-id="${escapeHTML(task.id)}">详情</button></td>
                    </tr>
                  `).join("")}
                </tbody>
              </table>
            </div>
          `
      : emptyState("暂无任务", "创建第一个任务，用于在当前工作区检查代码、执行修改或推进工作。")
    }
        </section>
      </div>
    </div>
  `);
}

async function openJobDetail(id) {
  if (!id) return;
  const job = await apiFetch(`/jobs/${encodeURIComponent(id)}`);
  openModal({
    title: job.summary || job.kind || id,
    description: job.kind || "后台作业",
    body: `
      <div class="stack">
        <article class="list-row">
          <div class="event-header">
            <span class="event-title">状态</span>
            ${statusChip(job.status || "unknown", job.status || "unknown", true)}
          </div>
          <div class="muted">${escapeHTML(job.error || "暂无错误记录。")}</div>
        </article>
        <div>
          <div class="muted">载荷</div>
          ${renderJSON(job.payload || {})}
        </div>
        <div>
          <div class="muted">详情</div>
          ${renderJSON(job.details || {})}
        </div>
      </div>
    `,
  });
}

const taskNavItem = NAV_ITEMS.find((item) => item.id === "tasks");
if (taskNavItem) {
  taskNavItem.label = "频道";
  taskNavItem.hint = "创建私聊频道、群聊频道，并在同一个频道里持续推进聊天和任务";
  taskNavItem.aliases = ["tasks", "频道", "channels", "channel", "群聊", "私聊"];
}

function renderWorkbenchComposer(options) {
  const {
    assistantName = "",
    sessionId = "",
    hero = false,
  } = options || {};
  const submitting = state.workbenchSubmitting;
  const composerClass = hero
    ? "composer workbench-composer workbench-composer-home"
    : "composer workbench-composer workbench-composer-docked";
  const textareaClass = hero ? "textarea textarea-home" : "textarea textarea-chat";
  const textareaValue = escapeHTML(state.workbenchDraftMessage || "");
  const actionLabel = submitting ? "发送中..." : hero ? "开始频道对话" : "发送消息";
  const secondaryButton = sessionId
    ? `<button type="button" class="btn btn-ghost" data-action="new-session" ${submitting ? "disabled" : ""}>新频道</button>`
    : `<button type="button" class="btn btn-ghost" data-action="navigate" data-route="tasks" ${submitting ? "disabled" : ""}>管理频道</button>`;
  const placeholder = hero ? "输入你的目标、问题，或者想在频道里推进的任务..." : "继续在这个频道里补充消息或任务...";
  const statusText = submitting
    ? sessionId
      ? "频道成员正在协作回复..."
      : "正在创建频道并发送首条消息..."
    : "";
  const formClass = `${composerClass}${submitting ? " is-submitting" : ""}`;

  return `
    <form class="${formClass}" data-form="chat" aria-busy="${submitting ? "true" : "false"}">
      ${sessionId ? `<input type="hidden" name="session_id" value="${escapeHTML(sessionId)}" />` : ""}
      <input type="hidden" name="assistant" value="${escapeHTML(assistantName)}" />
      <label class="field field-composer">
        <span class="sr-only">${hero ? "输入你的目标、问题或频道任务" : "输入频道消息"}</span>
        <textarea class="${textareaClass}" name="message" placeholder="${placeholder}" required ${submitting ? "disabled" : ""}>${textareaValue}</textarea>
      </label>
      <div class="composer-actions">
        <div class="badge-row">
          ${secondaryButton}
          ${statusText ? `<span class="composer-status" aria-live="polite">${escapeHTML(statusText)}</span>` : ""}
        </div>
        <button type="submit" class="btn btn-primary" ${submitting ? "disabled" : ""}>${actionLabel}</button>
      </div>
    </form>
  `;
}

function renderWorkbenchSessionsDrawer(sessions, selectedSession, sessionCount) {
  if (!state.workbenchSessionsOpen) return "";
  return `
    <div class="workbench-session-layer is-open">
      <button
        type="button"
        class="workbench-session-backdrop"
        data-action="toggle-workbench-sessions"
        aria-label="关闭频道列表"
      ></button>
      <aside class="panel workbench-session-drawer">
        <div class="section-header workbench-session-head">
          <div>
            <h3>频道列表</h3>
            <p class="muted">${sessionCount ? `共 ${sessionCount} 个频道` : "还没有历史频道"}</p>
          </div>
          <div class="inline-actions workbench-session-actions">
            <button class="btn btn-ghost" data-action="new-session">新频道</button>
          </div>
        </div>
        ${sessions.length
          ? `
            <div class="stack session-list">
              ${sessions.map((session) => `
                <button class="session-item ${session.id === selectedSession?.id ? "is-active" : ""}" data-action="select-session" data-id="${escapeHTML(session.id)}">
                  <div class="event-header">
                    <span class="event-title">${escapeHTML(session.title || "未命名频道")}</span>
                    <span class="muted">${escapeHTML(sessionKindLabel(session))}</span>
                  </div>
                  <div class="muted session-preview">${escapeHTML(session.last_user_text || session.last_assistant_text || "暂无消息")}</div>
                  <div class="badge-row">
                    ${statusChip(session.presence || "idle", session.presence || "idle", true)}
                    <span class="provider-chip">${escapeHTML(sessionMemberSummary(session))}</span>
                  </div>
                </button>
              `).join("")}
            </div>
          `
          : `
            <div class="workbench-empty-note">
              <h4>还没有频道</h4>
              <p>从主输入区开始，发送第一条消息后会自动生成私聊频道；你也可以去频道页创建群聊频道。</p>
            </div>
          `}
      </aside>
    </div>
  `;
}

async function renderTasks() {
  const workspace = currentWorkspace();
  if (!workspace) {
    pageEl.innerHTML = localizeHTML(emptyState("No workspace selected", "Select a workspace before creating channels."));
    return;
  }

  state.sessions = (await safeFetch(`/sessions?workspace=${encodeURIComponent(workspace.id)}`, {}, [])) || [];
  const channels = state.sessions;
  const dmCount = channels.filter((session) => !session.is_group && sessionParticipants(session).length <= 1).length;
  const groupCount = channels.filter((session) => session.is_group || sessionParticipants(session).length > 1).length;
  const assistantOptions = enabledAssistants().map((assistant) => `<option value="${escapeHTML(assistant.name)}">${escapeHTML(assistant.name)}</option>`).join("");

  pageEl.innerHTML = localizeHTML(`
    <div class="page-grid tasks-page">
      <div class="stats-grid tasks-stats-grid">
        ${metricCard("频道总数", channels.length, workspace.name, "info")}
        ${metricCard("私聊频道", dmCount, "用户与单个 Agent", "success")}
        ${metricCard("群聊频道", groupCount, "多个 Agent 协作", "warning")}
        ${metricCard("活跃频道", channels.filter((session) => (session.message_count || 0) > 0).length, "已有消息记录", "info")}
      </div>

      <div class="two-col tasks-layout">
        <section class="panel tasks-panel tasks-form-panel">
          <div class="section-header section-header-tight">
            <div>
              <h3>创建频道</h3>
              <p class="muted">把聊天和任务统一放进频道里。私聊频道适合单个 Agent 跟进，群聊频道适合多个 Agent 共享上下文协作完成任务。</p>
            </div>
          </div>
          <form class="stack tasks-form" data-form="channel">
            <label class="field">
              <span>频道名称</span>
              <input class="input" type="text" name="title" placeholder="例如：接口联调群、前端私聊、Bug 排查频道" />
            </label>
            <label class="field">
              <span>频道类型</span>
              <select class="select" name="channel_type">
                <option value="dm">私聊频道</option>
                <option value="group">群聊频道</option>
              </select>
            </label>
            <label class="field">
              <span>默认智能体</span>
              <select class="select" name="assistant">
                ${renderAssistantOptions(state.assistants[0]?.name || "")}
              </select>
            </label>
            <label class="field">
              <span>群聊成员</span>
              <select class="select" name="participants" multiple size="6">
                ${assistantOptions}
              </select>
              <div class="muted">群聊时可额外选择多个 Agent。默认智能体会自动加入成员列表。</div>
            </label>
            <div class="inline-actions">
              <button type="submit" class="btn btn-primary">创建频道</button>
              <button type="button" class="btn btn-ghost" data-action="navigate" data-route="workbench">进入工作台</button>
            </div>
          </form>
        </section>

        <section class="panel tasks-panel tasks-list-panel">
          <div class="section-header section-header-tight">
            <div>
              <h3>最近频道</h3>
              <p class="muted">选择已有频道继续聊天、补充任务，或者直接查看频道详情。</p>
            </div>
            <span class="provider-chip">${channels.length}</span>
          </div>
          ${channels.length
            ? `
              <div class="table-wrap tasks-table-wrap">
                <table class="tasks-table">
                  <thead>
                    <tr>
                      <th>频道</th>
                      <th>类型</th>
                      <th>成员</th>
                      <th>更新</th>
                      <th></th>
                    </tr>
                  </thead>
                  <tbody>
                    ${channels.map((session) => `
                      <tr>
                        <td>
                          <div class="event-title">${escapeHTML(session.title || session.id)}</div>
                          <div class="muted">${escapeHTML(session.last_user_text || session.last_assistant_text || "暂无消息")}</div>
                        </td>
                        <td>${statusChip(sessionKindLabel(session), sessionKindLabel(session), true)}</td>
                        <td>${escapeHTML(sessionMemberSummary(session))}</td>
                        <td>${formatTime(session.updated_at || session.created_at)}</td>
                        <td>
                          <div class="inline-actions">
                            <button class="btn btn-ghost" data-action="open-channel" data-id="${escapeHTML(session.id)}">进入</button>
                            <button class="btn btn-ghost" data-action="open-channel-detail" data-id="${escapeHTML(session.id)}">详情</button>
                          </div>
                        </td>
                      </tr>
                    `).join("")}
                  </tbody>
                </table>
              </div>
            `
            : emptyState("暂无频道", "创建第一个私聊或群聊频道，然后直接在频道里聊天、发任务、协作推进。")}
        </section>
      </div>
    </div>
  `);
}
