# AnyClaw - 本地优先 AI 智能体平台

[![Github](https://img.shields.io/badge/GitHub-1024XEngineer/anyclaw-blue)](https://github.com/1024XEngineer/anyclaw)
[![Version](https://img.shields.io/badge/version-2026.3.13-green)]()
[![Go](https://img.shields.io/badge/Go-1.21+-blue)]()

一个轻量级、透明的 AI Agent 系统，专注于文件记忆和技能插件架构。

## 核心理念

- **文件优先记忆**: 不使用不透明的向量数据库。所有对话、反思和事实都存储在人类可读的 Markdown/JSON 文件中。
- **技能即插件**: 遵循 Anthropic 的 Agent Skills 范式。放入文件夹即可使用。
- **透明可控**: 所有系统提示词逻辑、工具调用和内存操作对开发者可见。

## 核心功能

### 🤖 多智能体系统

- 支持配置多个不同领域的专家智能体
- 每个智能体可独立配置 LLM 供应商和模型
- 智能体之间可以协作完成复杂任务

```bash
# 查看所有智能体
anyclaw agent list

# 切换智能体
anyclaw agent use Go编码专家

# 与特定智能体对话
anyclaw agent chat Go编码专家
```

### 📺 频道系统

- **私聊频道**: 用户与单个智能体对话
- **群聊频道**: 多个智能体协作完成任务
- **动态拉人**: 随时添加/移除智能体

```bash
# 创建私聊
anyclaw ch create --name "Go专家" --type dm --agent Go编码专家

# 创建群聊
anyclaw ch create --name "项目组" --type group --agents "Go编码专家,Python编码专家"

# 添加智能体
anyclaw ch add --channel ch_xxx --agent 数据分析师

# 交互式聊天
anyclaw ch chat
```

### 🛠️ 内置工具 (27个)

| 类别 | 工具 |
|------|------|
| **文件操作** | read_file, write_file, list_directory, search_files |
| **命令执行** | run_command |
| **网络** | web_search, fetch_url |
| **浏览器** | browser_navigate, browser_click, browser_type, browser_screenshot, browser_snapshot, browser_eval, browser_pdf, browser_upload, browser_download, browser_wait, browser_select, browser_press, browser_scroll, browser_close |
| **标签页** | browser_tab_new, browser_tab_list, browser_tab_switch, browser_tab_close |

### 🧠 文件记忆系统

所有记忆存储在 `.anyclaw/memory/`:

```
.anyclaw/
└── memory/
    ├── conversations/    # 用户/助手对话
    ├── reflections/      # 智能体自我反思
    ├── facts/           # 用户提供的事实
    └── index.json       # 记忆索引
```

### 🔌 多 LLM 供应商支持

| 供应商 | 模型示例 |
|--------|----------|
| **OpenAI** | gpt-4o, gpt-4o-mini, gpt-3.5-turbo |
| **Anthropic** | claude-sonnet-4-7, claude-haiku-3-5 |
| **Qwen (通义千问)** | qwen-plus, qwen-turbo, qwen-max |
| **Ollama** | llama3.2, llama3.1, codellama |
| **兼容API** | 任何 OpenAI 兼容的 API |

每个智能体可独立配置 LLM:

```json
{
  "orchestrator": {
    "sub_agents": [
      {
        "name": "代码专家",
        "llm_provider": "openai",
        "llm_model": "gpt-4o",
        "llm_temperature": 0.3
      },
      {
        "name": "本地助手",
        "llm_provider": "ollama",
        "llm_model": "llama3.2",
        "llm_base_url": "http://localhost:11434"
      }
    ]
  }
}
```

### 📦 技能系统

```bash
# 搜索技能
anyclaw skill search coder

# 安装技能
anyclaw skill install coder

# 列出已安装技能
anyclaw skill list

# Skillhub 商店
anyclaw skillhub search calendar
anyclaw skillhub install calendar
```

### 已安装技能

| 技能 | 功能 |
|------|------|
| coder | 代码生成和分析 |
| weather | 天气查询 |
| calendar | 日历管理 |
| file-operations | 文件操作 |
| find-skills | 技能推荐 |
| skillhub | 技能商店集成 |

## 快速开始

```bash
# 克隆仓库
git clone https://github.com/1024XEngineer/anyclaw.git
cd anyclaw

# 构建
go build -o anyclaw ./cmd/anyclaw

# 配置
cp anyclaw.json.example anyclaw.json
# 编辑 anyclaw.json，设置 LLM API Key

# 运行
./anyclaw -i
```

## 配置

编辑 `anyclaw.json`:

```json
{
  "llm": {
    "provider": "qwen",
    "model": "qwen-plus",
    "api_key": "your-api-key"
  },
  "agent": {
    "name": "个人助手",
    "profiles": [
      {
        "name": "Go编码专家",
        "domain": "Go语言开发",
        "expertise": ["Go并发编程", "微服务架构"]
      }
    ]
  },
  "orchestrator": {
    "enabled": true,
    "agent_names": ["Go编码专家", "Python编码专家"]
  }
}
```

## CLI 命令

| 命令 | 功能 |
|------|------|
| `anyclaw` | 交互模式 |
| `anyclaw agent` | 智能体管理 |
| `anyclaw skill` | 技能管理 |
| `anyclaw skillhub` | Skillhub 商店 |
| `anyclaw ch` | 频道管理 |
| `anyclaw task` | 任务执行 |
| `anyclaw group` | 群聊管理 |
| `anyclaw gateway` | 网关服务 |
| `anyclaw doctor` | 系统诊断 |
| `anyclaw shell` | 命令执行 |

### 交互模式命令

```
/exit, /quit, /q   - 退出程序
/clear             - 清除对话历史
/memory            - 查看记忆内容
/skills            - 查看可用技能
/tools             - 查看可用工具
/provider          - 显示当前提供商/模型
/providers         - 显示可用提供商
/models <名称>     - 显示提供商模型
/agents            - 显示智能体配置
/agent use <名称>  - 切换智能体
/audit             - 查看审计日志
/set provider <值> - 切换 LLM 提供商
/set model <值>    - 切换模型
/set apikey <值>   - 设置 API 密钥
/set temp <值>     - 设置温度
/help, /?          - 显示帮助
```

## 渠道集成

- Telegram Bot
- Slack
- Discord
- WhatsApp
- Signal

## 架构

```
┌────────────────────────────────────────────────────────────┐
│                      控制平面                               │
│  CLI / API / 渠道连接器                                     │
└───────────────┬────────────────────────────┬───────────────┘
                │                            │
                ▼                            ▼
┌──────────────────────────┐  ┌──────────────────────────────┐
│    智能体管理层           │  │    任务编排层                 │
│  - 智能体配置             │  │  - 会话生命周期              │
│  - 性格绑定              │  │  - 计划/执行/恢复            │
│  - 模型选择              │  │  - 队列/任务工作器           │
└───────────────┬──────────┘  └───────────────┬──────────────┘
                │                              │
                └───────────────┬──────────────┘
                                ▼
┌────────────────────────────────────────────────────────────┐
│                      运行时核心                             │
│  提示词构建 | LLM路由 | 工具注册 | 技能运行时               │
│  上下文组装 | 记忆访问 | 观察者钩子 | 安全防护              │
└───────────────┬────────────────────────────┬───────────────┘
                │                            │
                ▼                            ▼
┌──────────────────────────┐  ┌──────────────────────────────┐
│   权限与安全层            │  │    记忆与数据层              │
│  - 作用域/工作区 ACL      │  │  - 对话记录                 │
│  - 危险命令策略           │  │  - 事实/反思                │
│  - 确认检查点             │  │  - 任务记录                 │
│  - 沙箱隔离              │  │  - 智能体配置               │
└───────────────┬──────────┘  └───────────────┬──────────────┘
                │                              │
                ▼                              ▼
┌────────────────────────────────────────────────────────────┐
│                    执行与集成层                             │
│  文件工具 | 命令工具 | 浏览器工具 | Web工具 | 插件          │
│  Telegram | Slack | Discord | WhatsApp | Signal            │
└────────────────────────────────────────────────────────────┘
```

## 项目结构

```
anyclaw/
├── cmd/anyclaw/          # CLI 入口
├── pkg/
│   ├── agent/            # 智能体核心
│   ├── llm/              # LLM 客户端
│   ├── memory/           # 文件记忆
│   ├── tools/            # 工具系统
│   ├── skills/           # 技能管理
│   ├── orchestrator/     # 任务编排
│   ├── channel/          # 渠道适配
│   ├── channel2/         # 用户频道
│   ├── gateway/          # HTTP 网关
│   ├── config/           # 配置管理
│   ├── store/            # 数据存储
│   ├── audit/            # 审计日志
│   └── ...
├── skills/               # 技能目录
├── anyclaw.json          # 配置文件
└── README.md
```

## 版本

```
anyclaw version 2026.3.13
```

## 许可证

MIT License

## 链接

- [GitHub 仓库](https://github.com/1024XEngineer/anyclaw)
- [问题反馈](https://github.com/1024XEngineer/anyclaw/issues)
