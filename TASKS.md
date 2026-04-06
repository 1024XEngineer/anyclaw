# AnyClaw 超级个人助手 - 开发任务书

> **项目定位**：基于 OpenClaw 架构理念，用 Go 语言打造的本地优先、可控工具、可插拔技能的超级个人 AI 助手
> **版本**：2026.3.13
> **创建日期**：2026-04-06

---

## 一、项目愿景

打造一款**个人专属的 AI 助手平台**，具备以下特质：
- **本地优先**：数据存储在本地文件中，透明可读，不依赖黑盒存储层
- **工具可控**：暴露明确的工具接口，Agent 可调用文件、Shell、浏览器、桌面操作等
- **技能可插拔**：通过 Skills 系统扩展能力，支持社区生态
- **多渠道触达**：连接微信、Telegram、Discord、Slack 等已有通讯渠道
- **多 Agent 协作**：支持角色化 Agent 编排，任务分解与协同执行
- **跨平台**：Windows / macOS / Linux 全平台支持

---

## 二、当前状态评估

### 2.1 已完成模块（完成度 ~40%）

| 模块 | 状态 | 说明 |
|------|------|------|
| CLI 命令行 | ✅ 完成 | 35 个 CLI 文件，覆盖 agent/gateway/config/skill/plugin 等 |
| Gateway | ✅ 完成 | HTTP + WebSocket 网关，端口 18789 |
| 配置系统 | ✅ 完成 | anyclaw.json，支持多 Provider 路由 |
| Agent 框架 | ✅ 完成 | 6 个 Agent Profile（Go/Python专家、健身教练、论文助手、数据分析师、翻译专家） |
| LLM 集成 | ✅ 完成 | 通义千问、OpenAI 兼容、Ollama |
| Skills 系统 | ✅ 完成 | 10 个 Skills（coder、weather、web-search、vision-agent 等） |
| Memory 系统 | ✅ 完成 | Markdown 文件优先，自动保存 |
| 安全机制 | ✅ 完成 | 危险命令拦截、路径保护、速率限制 |
| Sandbox | ✅ 基础 | 本地沙箱模式 |
| Web UI | ⚠️ 基础 | pnpm 构建，Dashboard 路由 |
| 编排器 | ⚠️ 基础 | Orchestrator 支持多 Agent，max 4 并发 |
| 插件系统 | ⚠️ 基础 | 框架存在，市场目录为空 |
| Cron | ⚠️ 基础 | 配置存在 |
| Docker | ✅ 完成 | Dockerfile + docker-compose.yml |

### 2.2 未实现/待完善模块

| 模块 | 优先级 | 当前状态 |
|------|--------|----------|
| 渠道集成（微信/Telegram/Discord 等） | P0 | 配置框架存在，全部未启用 |
| 浏览器控制（CDP） | P0 | pkg/cdp 存在，功能待实现 |
| 语音能力（STT/TTS） | P1 | 配置存在，未启用 |
| Canvas (A2UI) | P1 | pkg/canvas 存在，功能有限 |
| MCP 协议 | P1 | pkg/mcp 存在，disabled |
| 原生应用（macOS/iOS/Android） | P2 | 无 |
| Tailscale 集成 | P2 | 无 |
| Session 管理 | P1 | 基础实现 |
| 测试覆盖 | P1 | 少量测试 |
| 文档体系 | P1 | 6 个文档文件 |

---

## 三、阶段规划

### Phase 0：基础巩固（当前 → 2026.04）

**目标**：夯实核心能力，确保 CLI + Gateway + Agent 链路稳定可用

| # | 任务 | 详情 | 优先级 |
|---|------|------|--------|
| 0.1 | 完善 Web UI Dashboard | 实现完整的控制面板：会话管理、Agent 切换、配置编辑、日志查看 | P0 |
| 0.2 | 增强 Session 管理 | 支持 `/new`、`/compact`、`/reset`、上下文压缩、会话持久化 | P0 |
| 0.3 | 完善 Skills 生态 | 扩充内置 Skills，实现 Skill 自动发现与安装 | P0 |
| 0.4 | 完善插件市场 | 实现插件签名验证、安装、卸载、更新流程 | P0 |
| 0.5 | 补充测试覆盖 | 为核心模块添加单元测试，目标覆盖率 > 60% | P0 |
| 0.6 | 完善文档 | 补充 API 文档、配置参考、开发指南、中文文档 | P0 |

### Phase 1：渠道接入（2026.04 → 2026.05）

**目标**：打通主流通讯渠道，实现多渠道消息收发

| # | 任务 | 详情 | 优先级 |
|---|------|------|--------|
| 1.1 | 微信渠道 | 实现微信接入（参考 OpenClaw WeChat 插件），支持消息收发、群管理 | P0 |
| 1.2 | Telegram 渠道 | 实现 Telegram Bot 接入，支持消息、图片、文件 | P0 |
| 1.3 | Discord 渠道 | 启用已配置的 Discord 模块，实现 Bot 接入 | P1 |
| 1.4 | 安全 DM 策略 | 实现 DM pairing、allowlist、mention 门控 | P0 |
| 1.5 | 群消息路由 | 实现 @mention 过滤、回复标签、分块路由 | P1 |
| 1.6 | 多渠道路由 | 实现 per-channel Agent 路由规则 | P1 |

### Phase 2：工具增强（2026.05 → 2026.06）

**目标**：完善工具链，让 Agent 具备更强的操作能力

| # | 任务 | 详情 | 优先级 |
|---|------|------|--------|
| 2.1 | 浏览器控制 | 完善 pkg/cdp，实现 Chrome CDP 控制、页面快照、元素操作、截图 | P0 |
| 2.2 | 文件工具增强 | 支持批量读写、diff 编辑、目录树操作 | P0 |
| 2.3 | Shell 工具增强 | 支持后台任务、管道、超时控制、输出流式返回 | P0 |
| 2.4 | Canvas 可视化 | 完善 pkg/canvas，实现 A2UI 渲染、Agent 驱动的可视化工作区 | P1 |
| 2.5 | Cron 定时任务 | 完善定时任务系统，支持表达式、重试、失败通知 | P1 |
| 2.6 | Webhook 接入 | 实现 Webhook 端点，支持外部事件触发 Agent | P1 |

### Phase 3：语音与多模态（2026.06 → 2026.07）

**目标**：实现语音交互和视觉能力

| # | 任务 | 详情 | 优先级 |
|---|------|------|--------|
| 3.1 | STT 语音转文字 | 接入 Whisper/通义语音，支持音频文件转文本 | P1 |
| 3.2 | TTS 文字转语音 | 接入 ElevenLabs/Edge TTS，支持语音回复 | P1 |
| 3.3 | Voice Wake 唤醒词 | 实现本地唤醒词检测（macOS/Windows） | P2 |
| 3.4 | Talk Mode 连续对话 | 实现语音连续对话模式 | P2 |
| 3.5 | 视觉 Agent | 完善 vision-agent skill，支持图片理解、OCR | P1 |

### Phase 4：高级特性（2026.07 → 2026.09）

**目标**：实现 OpenClaw 级别的高级功能

| # | 任务 | 详情 | 优先级 |
|---|------|------|--------|
| 4.1 | MCP 协议支持 | 实现 Model Context Protocol，兼容 MCP 工具生态 | P1 |
| 4.2 | Agent-to-Agent 通信 | 完善 sessions_send/sessions_spawn，支持 Agent 间协作 | P1 |
| 4.3 | 上下文工程 | 完善 pkg/context-engine，实现智能上下文管理、RAG | P1 |
| 4.4 | 向量检索 | 完善 pkg/embedding + pkg/vec，实现本地向量数据库 | P1 |
| 4.5 | 远程 Gateway | 实现 SSH 隧道 / Tailscale 远程访问 | P2 |
| 4.6 | Sandbox 增强 | 实现 Docker per-session 沙箱隔离 | P2 |

### Phase 5：生态与社区（2026.09 → 2026.12）

**目标**：构建生态，开放社区

| # | 任务 | 详情 | 优先级 |
|---|------|------|--------|
| 5.1 | Skills 注册表 | 建立在线 Skills 市场，支持搜索与安装 | P2 |
| 5.2 | 插件 SDK | 完善 pkg/sdk，提供 Go/Python 插件开发框架 | P2 |
| 5.3 | 桌面应用 | 开发 macOS/Windows 菜单栏应用 | P2 |
| 5.4 | 移动端 Node | 开发 iOS/Android 节点应用 | P3 |
| 5.5 | 开源社区 | 完善贡献指南、CI/CD、Issue 模板 | P2 |

---

## 四、技术架构

### 4.1 目录结构

```
cmd/anyclaw/          CLI 入口（35 个文件）
pkg/
  agent/              Agent 运行时
  agents/             多 Agent 管理
  gateway/            HTTP/WS 网关
  config/             配置加载与验证
  memory/             文件优先记忆系统
  skills/             Skill 加载与执行
  tools/              工具注册与内置工具
  cdp/                浏览器控制（CDP）
  canvas/             Canvas/A2UI 渲染
  channel/            渠道抽象层
  channels/           具体渠道实现
  cron/               定时任务
  mcp/                MCP 协议
  speech/             语音 STT/TTS
  orchestrator/       多 Agent 编排
  security/           安全机制
  session/            会话管理
  plugin/             插件系统
  vision/             视觉能力
  embedding/          向量嵌入
  vec/                向量检索
  context-engine/     上下文工程
  web/                Web 界面
  ui/                 UI 后端
  ...（共 66 个子包）
skills/               内置 Skills（10 个）
workflows/            Agent 工作区（7 个）
ui/                   Web UI 前端
plugins/              插件目录
docs/                 文档
```

### 4.2 技术栈

| 层面 | 技术 |
|------|------|
| 后端 | Go 1.22+ |
| 前端 | TypeScript + pnpm |
| 数据库 | SQLite (pkg/sqlite) |
| 通信 | HTTP + WebSocket |
| 容器 | Docker |
| LLM | 通义千问 / OpenAI 兼容 / Ollama |

---

## 五、关键指标（KPI）

| 指标 | Phase 0 | Phase 1 | Phase 2 | Phase 3 | Phase 4 |
|------|---------|---------|---------|---------|---------|
| 渠道数量 | 1 (CLI) | 3+ | 3+ | 3+ | 5+ |
| Skills 数量 | 15+ | 20+ | 25+ | 30+ | 40+ |
| 测试覆盖率 | 60% | 65% | 70% | 75% | 80% |
| 文档完整度 | 80% | 85% | 90% | 95% | 100% |
| 对标 OpenClaw | 40% | 55% | 65% | 75% | 85% |

---

## 六、风险与应对

| 风险 | 影响 | 应对策略 |
|------|------|----------|
| 微信渠道被封 | 高 | 使用企业微信 API 或合规第三方库 |
| CDP 浏览器兼容 | 中 | 支持 Chrome/Chromium 多版本 |
| 多 Provider 兼容性 | 中 | 统一抽象层，充分测试 |
| 性能瓶颈 | 中 | Go 语言优势，注意并发控制 |
| 安全风险 | 高 | 严格权限控制，沙箱隔离 |

---

## 七、下一步行动（本周）

1. **完善 Web UI Dashboard** - 实现会话管理、Agent 切换、配置编辑
2. **增强 Session 管理** - 实现上下文压缩、会话持久化
3. **扩充 Skills** - 新增 5 个常用 Skills（git-helper、file-organizer、email-assistant 等）
4. **补充文档** - 完善中文开发指南和配置参考
5. **启动微信渠道调研** - 评估可行的微信接入方案

---

*任务书由 AnyClaw 自动生成，将根据项目进展持续更新。*
