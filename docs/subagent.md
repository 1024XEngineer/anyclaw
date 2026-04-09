# 分身 (Subagent) 功能文档

## 概述

anyclaw 支持两种类型的分身：

### 静态分身（Static Agents）

- **预定义的持久 Agent**，通过配置文件创建
- 每个静态分身有独立的 workspace 和角色设定
- 通过 channel 绑定关系，固定响应特定账号的消息
- 生命周期：永久运行，随系统启动和停止
- 适用于：承担特定角色的长期服务（如"代码专家"、"研究助手"等）

### 动态分身（Dynamic Subagents）

- 运行时通过 `sessions_spawn` 工具创建的临时分身
- 用于处理一次性后台任务
- 任务完成后自动清理
- 生命周期：任务执行期间存在，完成后销毁
- 适用于：并行处理、长耗时任务、一次性调研等

---

## 静态分身配置

### 完整配置示例

```json
{
  "channels": {
    "telegram": {
      "enabled": true,
      "accounts": {
        "bot_gf": {
          "enabled": true,
          "token": "your_token_here",
          "name": "小婷Bot"
        },
        "bot_coder": {
          "enabled": true,
          "token": "your_token_here",
          "name": "代码专家Bot"
        }
      }
    }
  },

  "agents": {
    "list": [
      {
        "id": "xiaoting",
        "name": "小婷",
        "default": true,
        "model": "google-antigravity/gemini-3-pro-high",
        "workspace": "~/.openclaw/workspace",
        "system_prompt": "你是小婷，一个温柔的女朋友...",
        "subagents": {
          "allow_agents": ["coder"]
        }
      },
      {
        "id": "coder",
        "name": "代码专家",
        "model": "openai-codex/gpt-5.3-codex",
        "workspace": "~/.openclaw/workspace-coder",
        "system_prompt": "你是一个专业代码审查专家，擅长性能优化和架构分析..."
      }
    ]
  },

  "bindings": [
    {
      "agent_id": "xiaoting",
      "match": { "channel": "telegram", "account_id": "bot_gf" }
    },
    {
      "agent_id": "coder",
      "match": { "channel": "telegram", "account_id": "bot_coder" }
    }
  ]
}
```

### 静态分身使用方式

```
用户 → @小婷Bot 帮我分析代码
        ↓
    路由到 xiaoting 静态分身
        ↓
    xiaoting 可能调用 sessions_spawn
    创建 coder 动态分身帮助分析
```

---

## 动态分身

### 核心特性

- **会话隔离**: 分身运行在独立的会话中（`agent:<agentId>:subagent:<uuid>`）
- **非阻塞**: `sessions_spawn` 立即返回，后台并行执行
- **自动宣告**: 任务完成后自动将结果发回主 Agent
- **工具限制**: 默认拒绝会话管理工具，防止滥用
- **跨 Agent 支持**: 允许在不同 Agent 之间创建分身
- **自动清理**: 支持自动归档和手动清理

### 静态分身 vs 动态分身对比

| 特性 | 静态分身 | 动态分身 |
|------|----------|----------|
| 创建方式 | 配置文件预先定义 | 运行时通过工具创建 |
| 生命周期 | 永久运行 | 任务完成后销毁 |
| Workspace | 独立且固定 | 临时会话 |
| 用途 | 承担特定角色的持久 Agent | 处理一次性后台任务 |
| 绑定关系 | 固定绑定到 channel | 由主 Agent 动态分配 |

---

## 动态分身配置

### 全局分身配置

```json
{
  "agents": {
    "defaults": {
      "subagents": {
        "max_concurrent": 8,            // 最大并发分身数
        "archive_after_minutes": 60,      // 自动归档时间（分钟）
        "model": "google-antigravity/gemini-3-haiku",  // 默认模型
        "thinking": "low"                // 默认思考级别
      }
    }
  }
}
```

### 单 Agent 分身配置

```json
{
  "agents": {
    "list": [
      {
        "id": "xiaoting",
        "name": "小婷",
        "subagents": {
          "allow_agents": ["*", "research", "coder"],  // 允许跨 Agent 创建
          "model": "google-antigravity/gemini-3-haiku",
          "thinking": "low",
          "deny_tools": ["gateway"]
        }
      }
    ]
  }
}
```

## sessions_spawn 工具

### 参数

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `task` | string | 是 | 任务描述 |
| `label` | string | 否 | 可选标签 |
| `agent_id` | string | 否 | 目标 Agent ID（跨 Agent 创建） |
| `model` | string | 否 | 模型覆盖 |
| `thinking` | string | 否 | 思考级别覆盖 (low/medium/high) |
| `run_timeout_seconds` | int | 否 | 超时时间（秒） |
| `cleanup` | string | 否 | 清理策略: `delete` (立即删除) 或 `keep` (自动归档) |

### 返回值

```json
{
  "status": "accepted",           // accepted, forbidden, error
  "child_session_key": "agent:xiaoting:subagent:xxx",
  "run_id": "xxx-xxx-xxx"
}
```

## 使用示例

### 重要：用户实际使用方式

**用户不需要手动调用工具！** `sessions_spawn` 是主 Agent 可用的一个工具，主 Agent 会根据你的自然语言请求，**自主决定**是否创建分身。

### 用户输入示例（自然语言）

```
用户: 帮我分析一下这个项目的代码架构
用户: 让代码专家来审查这段代码
用户: 调研一下最新的 AI 技术趋势
用户: 搜索并整理关于某个主题的资料
```

### 主 Agent 的决策流程

```
用户输入 → 主 Agent LLM 分析需求
              ↓
       是否需要并行/长时间任务？
              ↓
    是 → 调用 sessions_spawn 工具
              ↓
      后台创建分身执行任务
              ↓
   分身完成后自动宣告结果
              ↓
       主 Agent 自然呈现给用户
```

### 典型场景对比

| 用户输入 | 主 Agent 行为 |
|----------|-------------|
| "帮我分析这个项目的代码架构" | 创建分身执行分析（耗时任务） |
| "写一个简单的排序算法" | 直接完成（简单任务，不创建分身） |
| "调研最新的 AI 技术" | 创建分身进行调研（长时间任务） |
| "让代码专家审查这段代码" | 跨 Agent 创建分身 |
| "帮我读一下 readme 文件" | 直接完成（简单任务） |

### 查看可用工具

使用 `/tools` 命令可以查看主 Agent 可用的所有工具，`sessions_spawn` 就在其中。

---

### sessions_spawn 工具调用示例

以下示例仅供**开发调试**参考，展示主 Agent 可能如何调用该工具：

#### 基本调用

```
sessions_spawn 调用示例：
task: "分析这个项目的代码架构，找出所有模块和依赖关系"
label: "code_analysis"
cleanup: "delete"
```

#### 跨 Agent 调用

```
sessions_spawn 跨 Agent 调用示例：
task: "审查这段代码的性能问题"
agent_id: "coder"    // 在代码专家 Agent 中执行
label: "code_review"
cleanup: "keep"       // 保留会话供后续查看
```

## 分身系统提示词

分身会自动获得以下系统提示词：

```
# Subagent Context

You are a **subagent** spawned by the main agent for a specific task.

## Your Role
- You were created to handle: {task}
- Complete this task. That's your entire purpose.
- You are NOT the main agent. Don't try to be.

## Rules
1. **Stay focused** - Do your assigned task, nothing else
2. **Complete the task** - Your final message will be automatically reported to the main agent
3. **Don't initiate** - No heartbeats, no proactive actions, no side quests
4. **Be ephemeral** - You may be terminated after task completion. That's fine.

## What You DON'T Do
- NO user conversations (that's main agent's job)
- NO external messages unless explicitly tasked
- NO cron jobs or persistent state
- NO pretending to be the main agent
```

## 工具策略

### 默认拒绝工具

- `sessions_spawn` - 防止嵌套创建
- `sessions_list`, `sessions_history`, `sessions_delete` - 会话管理
- `gateway`, `cron` - 系统管理

### 配置工具策略

```json
{
  "subagents": {
    "deny_tools": ["sessions_spawn", "cron"],
    "allow_tools": ["read", "write", "exec"]  // allow-only 模式
  }
}
```

## 会话密钥格式

- 主 Agent: `agent:<agentId>:<chatId>`
- 分身: `agent:<agentId>:subagent:<uuid>`

## 架构组件

| 组件 | 文件 | 说明 |
|------|------|------|
| 配置结构 | `config/schema.go` | 分身配置定义 |
| 分身注册表 | `agent/subagent_registry.go` | 管理分身运行状态 |
| 分身宣告器 | `agent/subagent_announce.go` | 处理结果宣告 |
| 生成工具 | `agent/tools/subagent_spawn_tool.go` | sessions_spawn 工具 |
| 管理器 | `agent/manager.go` | 集成分身功能 |

## 工作流程

```
主 Agent
   ↓
调用 sessions_spawn(task)
   ↓
立即返回 run_id, child_session_key
   ↓
后台执行分身任务...
   ↓
分身完成
   ↓
自动宣告结果回主 Agent
   ↓
主 Agent 自然地呈现给用户
```

## 注意事项

1. **禁止嵌套**: 分身不能创建分身
2. **权限控制**: 跨 Agent 创建需要配置 `allow_agents`
3. **成本控制**: 可以为分身配置更便宜的模型
4. **清理策略**: `delete` 立即删除，`keep` 自动归档（默认60分钟）
