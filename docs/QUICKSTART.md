# AnyClaw 快速开始指南

## 安装

```bash
# 克隆仓库
git clone https://github.com/anyclaw/anyclaw.git
cd anyclaw

# 构建
go build -o anyclaw ./cmd/anyclaw

# 运行设置向导
./anyclaw --setup

# 启动交互模式
./anyclaw -i
```

## 基本使用

### 1. 配置管理

```bash
# 查看当前配置
./anyclaw config show

# 设置 LLM 提供商
./anyclaw config set llm.provider anthropic

# 设置模型
./anyclaw config set llm.model claude-sonnet-4-7

# 设置 API 密钥
./anyclaw config set llm.api_key sk-...
```

### 2. 交互模式

```bash
# 启动交互模式
./anyclaw -i

# 在交互模式中可以使用以下命令：
/help - 显示帮助
/clear - 清除历史
/memory - 显示记忆
/skills - 列出技能
/tools - 列出工具
/provider - 显示当前提供商
/models - 列出可用模型
/exit - 退出
```

### 3. Gateway 模式

```bash
# 启动 Gateway
./anyclaw gateway start

# 后台运行 Gateway
./anyclaw gateway daemon

# 查看 Gateway 状态
./anyclaw gateway status
```

### 4. 渠道管理

```bash
# 列出渠道
./anyclaw channels list

# 连接渠道
./anyclaw channels connect telegram

# 查看渠道状态
./anyclaw channels status
```

### 5. 工具使用

```bash
# 列出工具
./anyclaw tools list

# 运行工具
./anyclaw tools run read_file --path ./example.txt
```

## 配置文件

配置文件 `anyclaw.json` 示例：

```json
{
  "llm": {
    "provider": "anthropic",
    "model": "claude-sonnet-4-7",
    "temperature": 0.7,
    "max_tokens": 4096
  },
  "agent": {
    "name": "AnyClaw",
    "permission_level": "full",
    "require_confirmation_for_dangerous": true
  },
  "gateway": {
    "host": "localhost",
    "port": 18789,
    "bind": "loopback"
  },
  "channels": {
    "telegram": {
      "enabled": false,
      "bot_token": ""
    }
  },
  "security": {
    "protected_paths": ["~/.ssh", "~/.gnupg"],
    "command_timeout_seconds": 30
  }
}
```

## 环境变量

可以通过环境变量覆盖配置：

```bash
# 设置 LLM 提供商
export ANYCLAW_LLM_PROVIDER=anthropic

# 设置 API 密钥
export ANYCLAW_LLM_API_KEY=sk-...

# 设置模型
export ANYCLAW_LLM_MODEL=claude-sonnet-4-7

# 启动
./anyclaw -i
```

## 技能系统

### 创建技能

在 `skills/` 目录下创建技能目录：

```
skills/
  my-skill/
    SKILL.md
    skill.json
```

`skill.json` 示例：

```json
{
  "name": "my-skill",
  "description": "My custom skill",
  "version": "1.0.0",
  "author": "Your Name",
  "tools": ["read_file", "write_file"],
  "permissions": ["read", "write"]
}
```

`SKILL.md` 示例：

```markdown
# My Skill

This skill provides file management capabilities.

## Tools

- read_file: Read a file
- write_file: Write to a file

## Usage

Use the tools to manage files in the workspace.
```

### 使用技能

```bash
# 列出技能
./anyclaw skills list

# 启用技能
./anyclaw skills enable my-skill

# 禁用技能
./anyclaw skills disable my-skill
```

## 故障排除

### 常见问题

1. **连接失败**
   ```bash
   # 检查网络连接
   ./anyclaw doctor --connectivity
   ```

2. **配置错误**
   ```bash
   # 验证配置
   ./anyclaw config validate
   ```

3. **权限问题**
   ```bash
   # 检查权限
   ./anyclaw doctor --repair
   ```

### 日志

```bash
# 查看日志
./anyclaw logs

# 实时日志
./anyclaw logs --follow
```

## 高级用法

### 1. 多代理

```bash
# 创建代理配置文件
./anyclaw agent create my-agent

# 切换代理
./anyclaw agent use my-agent

# 列出代理
./anyclaw agent list
```

### 2. 沙箱模式

```bash
# 启用沙箱
./anyclaw config set sandbox.enabled true

# 设置执行模式
./anyclaw config set sandbox.execution_mode sandbox
```

### 3. 安全设置

```bash
# 添加受保护路径
./anyclaw config add security.protected_paths ~/.ssh

# 设置命令超时
./anyclaw config set security.command_timeout_seconds 60
```

## 下一步

1. 阅读[架构文档](ARCHITECTURE.md)了解详细设计
2. 查看[示例技能](../skills/)学习如何创建技能
3. 参考[API 文档](API.md)了解可用接口
4. 加入[社区](https://discord.gg/anyclaw)获取帮助
