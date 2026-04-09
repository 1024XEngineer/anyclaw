# AnyClaw CLI 命令速查表

AnyClaw 是一个 Go 语言实现的 AI Agent 框架，提供完整的命令行工具来管理 agents、channels、gateway 和系统功能。

## 目录

- [基本命令](#基本命令)
- [Agent 管理](#agent-管理)
- [Channel 管理](#channel-管理)
- [Gateway 管理](#gateway-管理)
- [Cron 定时任务](#cron-定时任务)
- [Browser 自动化](#browser-自动化)
- [System 控制](#system-控制)
- [Memory 管理](#memory-管理)
- [Sessions 管理](#sessions-管理)
- [Skills 管理](#skills-管理)
- [Approvals 审批](#approvals-审批)
- [Logs 日志](#logs-日志)
- [Health & Status](#health--status)

---

## 基本命令

```bash
# 显示帮助
anyclaw --help
anyclaw [command] --help

# 启动 anyclaw agent 服务（后台运行）
anyclaw start

# 交互式终端 UI
anyclaw tui

# 单次执行
anyclaw agent --message "你好"

# 配置管理
anyclaw config show
anyclaw config validate
```

---

## Agent 管理

### 运行单个 Agent 交互

```bash
# 基本用法
anyclaw agent --message "你好"

# 指定 channel
anyclaw agent --message "测试" --channel telegram

# 指定 session
anyclaw agent --message "继续" --session-id abc123

# 本地模式（不连接 channels）
anyclaw agent --message "本地测试" --local

# 显示思考过程
anyclaw agent --message "解释这段代码" --thinking

# 设置超时（秒）
anyclaw agent --message "长任务" --timeout 300

# JSON 输出
anyclaw agent --message "测试" --json
```

### 管理 Isolated Agents

```bash
# 列出所有 agents
anyclaw agents list

# 添加新 agent（交互式）
anyclaw agents add

# 添加新 agent（非交互式）
anyclaw agents add my-agent --workspace /path/to/workspace --model claude-3-5-sonnet-20241022

# 删除 agent
anyclaw agents delete my-agent

# 删除 agent（强制）
anyclaw agents delete my-agent --force
```

---

## Channel 管理

```bash
# 列出所有 channels
anyclaw channels list

# 检查 channel 状态
anyclaw channels status

# 添加 channel
anyclaw channels add --channel telegram --account mybot --name "My Bot" --token $TELEGRAM_BOT_TOKEN

# 添加 Discord channel
anyclaw channels add --channel discord --account work --name "Work Bot" --token $DISCORD_BOT_TOKEN

# 删除 channel
anyclaw channels remove --channel discord --account work

# 删除 channel（包括配置）
anyclaw channels remove --channel discord --account work --delete

# 登录到 channel
anyclaw channels login --channel whatsapp

# 从 channel 登出
anyclaw channels logout --channel whatsapp

# 查看 channel 日志
anyclaw channels logs --channel telegram --lines 100

# Channel 状态探测
anyclaw channels status --probe
```

### 微信通道管理

```bash
# 微信扫码登录
anyclaw channels weixin login [account-id]

# 查看微信登录状态
anyclaw channels weixin status [account-id]

# 微信登出
anyclaw channels weixin logout [account-id]
```

---

## Gateway 管理

### Gateway 服务管理

```bash
# 运行 WebSocket Gateway
anyclaw gateway run

# 自定义端口运行
anyclaw gateway run --port 8080

# 绑定地址
anyclaw gateway run --bind 0.0.0.0 --port 28789

# 使用 Tailscale
anyclaw gateway run --tailscale

# 开发模式
anyclaw gateway run --dev
```

### Gateway 系统服务

```bash
# 安装为系统服务
anyclaw gateway install

# 安装服务（自定义端口）
anyclaw gateway install --port 8080

# 启动服务
anyclaw gateway start

# 停止服务
anyclaw gateway stop

# 重启服务
anyclaw gateway restart

# 卸载服务
anyclaw gateway uninstall
```

### Gateway 状态检查

```bash
# 查看 gateway 状态
anyclaw gateway status

# 深度检查
anyclaw gateway status --deep

# 探测连接性
anyclaw gateway probe

# 健康检查
anyclaw gateway health

# RPC 调用
anyclaw gateway call config.get
anyclaw gateway call skills.list --params '{"limit": 10}'
```

---

## Cron 定时任务

```bash
# 查看调度器状态
anyclaw cron status

# JSON 格式输出
anyclaw cron status --json

# 列出所有任务
anyclaw cron list

# 列出所有任务（包括禁用的）
anyclaw cron list --all

# JSON 格式输出
anyclaw cron list --json
```

### 添加定时任务

```bash
# 添加任务（交互式）
anyclaw cron add

# 定时执行（每天 14:30）
anyclaw cron add --name "Daily Report" --at "14:30" --message "生成日报"

# 间隔执行（每小时）
anyclaw cron add --name "Hourly Check" --every "1h" --system-event "health_check"

# 使用 cron 表达式
anyclaw cron add --name "Weekly Backup" --cron "0 2 * * 0" --message "执行备份"

# 每天早上 9 点（工作日）
anyclaw cron add --name "Morning Briefing" --cron "0 9 * * 1-5" --message "早报"
```

### 编辑定时任务

```bash
# 编辑任务名称
anyclaw cron edit job-1234567890 --name "New Name"

# 修改调度时间
anyclaw cron edit job-1234567890 --at "10:00"

# 修改为间隔执行
anyclaw cron edit job-1234567890 --every "2h"

# 修改为 cron 表达式
anyclaw cron edit job-1234567890 --cron "0 */6 * * *"

# 修改消息
anyclaw cron edit job-1234567890 --message "更新后的消息"

# 启用任务
anyclaw cron edit job-1234567890 --enable

# 禁用任务
anyclaw cron edit job-1234567890 --disable

# 组合修改
anyclaw cron edit job-1234567890 --name "Updated" --at "10:00" --enable
```

### 管理定时任务

```bash
# 立即运行任务
anyclaw cron run job-1234567890

# 强制运行（即使禁用）
anyclaw cron run job-1234567890 --force

# 查看运行历史
anyclaw cron runs --id job-1234567890

# 查看最近 20 次运行
anyclaw cron runs --id job-1234567890 --limit 20

# 启用任务
anyclaw cron enable job-1234567890

# 禁用任务
anyclaw cron disable job-1234567890

# 删除任务
anyclaw cron rm job-1234567890
```

---

## Browser 自动化

### Browser 管理

```bash
# 查看浏览器状态
anyclaw browser status

# 启动浏览器
anyclaw browser start

# 停止浏览器
anyclaw browser stop

# 重置浏览器配置
anyclaw browser reset-profile

# 列出所有标签页
anyclaw browser tabs

# 列出所有标签页（新方法）
anyclaw browser focus --list

# 切换到指定标签
anyclaw browser focus <targetId>
```

### Browser 操作

```bash
# 打开 URL
anyclaw browser open https://example.com

# 导航到 URL
anyclaw browser navigate https://example.com

# 截图
anyclaw browser screenshot

# 截图（指定标签）
anyclaw browser screenshot <targetId>

# 页面快照（HTML + 截图）
anyclaw browser snapshot

# 调整视口大小
anyclaw browser resize 1920 1080
```

### Browser 交互

```bash
# 点击元素
anyclaw browser click "#submit-button"

# 输入文本
anyclaw browser type "#username" "myuser"

# 按键
anyclaw browser press "Enter"
anyclaw browser press "Escape"
anyclaw browser press "Tab"

# 悬停
anyclaw browser hover "#menu-item"

# 选择下拉选项
anyclaw browser select "#country" "China"

# 上传文件
anyclaw browser upload "#file-input" /path/to/file.pdf

# 填充表单字段
anyclaw browser fill "#email" "user@example.com"

# 处理对话框
anyclaw browser dialog accept
anyclaw browser dialog dismiss
anyclaw browser dialog accept "提示文本"

# 等待元素
anyclaw browser wait "#loaded-element"
anyclaw browser wait "#element" 30

# 评估 JavaScript
anyclaw browser evaluate "document.title"

# 获取控制台日志
anyclaw browser console
anyclaw browser console --errors-only
anyclaw browser console --warnings-only
anyclaw browser console --info-only
anyclaw browser console --max=50

# 保存为 PDF
anyclaw browser pdf
anyclaw browser pdf output.pdf

# 关闭标签
anyclaw browser close
anyclaw browser close <targetId>

# 管理配置文件
anyclaw browser profiles
```

---

## System 控制

```bash
# 发送系统事件
anyclaw system event --text "系统重启"

# 指定事件模式
anyclaw system event --text "测试" --mode test

# 心跳控制
anyclaw system heartbeat last
anyclaw system heartbeat enable
anyclaw system heartbeat disable

# 列出在线连接
anyclaw system presence
```

---

## Memory 管理

```bash
# 查看内存状态
anyclaw memory status

# 重新索引内存文件
anyclaw memory index

# 语义搜索
anyclaw memory search "如何配置 API"

# 搜索并限制结果
anyclaw memory search "配置" --limit 5
```

---

## Sessions 管理

```bash
# 列出所有会话
anyclaw sessions list

# 详细输出
anyclaw sessions list --verbose

# JSON 输出
anyclaw sessions list --json

# 只显示活动会话
anyclaw sessions list --active

# 指定存储目录
anyclaw sessions list --store /path/to/sessions
```

---

## Skills 管理

```bash
# 列出所有技能
anyclaw skills list

# 只列出已就绪的技能（无缺失依赖）
anyclaw skills list --eligible

# 详细输出
anyclaw skills list -v

# 查看技能详情
anyclaw skills info <skill-name>

# 检查技能状态
anyclaw skills check

# 安装技能依赖
anyclaw skills install-deps <skill-name>

# 验证技能依赖
anyclaw skills validate <skill-name>

# 测试技能
anyclaw skills test <skill-name> --prompt "测试提示"
```

### Skills 配置

```bash
# 技能配置管理
anyclaw skills config list <skill-name>
anyclaw skills config get <skill-name> <key>
anyclaw skills config set <skill-name> <key> <value>
anyclaw skills config unset <skill-name> <key>
```

---

## Approvals 审批

```bash
# 获取审批设置
anyclaw approvals get

# 设置审批行为
anyclaw approvals set auto
anyclaw approvals set manual
anyclaw approvals set prompt

# 允许列表管理
anyclaw approvals allowlist add web_search
anyclaw approvals allowlist add browser
anyclaw approvals allowlist add shell_command

# 移除允许列表项
anyclaw approvals allowlist remove web_search

# 查看允许列表
anyclaw approvals get
```

---

## Logs 日志

```bash
# 查看日志（默认 100 行）
anyclaw logs

# 实时跟踪日志
anyclaw logs -f

# 指定行数
anyclaw logs -n 500

# 指定日志文件
anyclaw logs -f /var/log/anyclaw/gateway.log

# JSON 输出
anyclaw logs --json

# 禁用颜色
anyclaw logs --no-color

# 纯文本输出
anyclaw logs --plain
```

---

## Health & Status

```bash
# 健康检查
anyclaw health

# JSON 输出
anyclaw health --json

# 设置超时
anyclaw health --timeout 10

# 详细输出
anyclaw health -v
```

### 状态查看

```bash
# 基本状态
anyclaw status

# 所有会话
anyclaw status --all

# 深度扫描
anyclaw status --deep

# 显示资源使用
anyclaw status --usage

# JSON 输出
anyclaw status --json

# 调试输出
anyclaw status --debug

# 详细输出
anyclaw status -v
```

---

## 高级用法

### 环境变量

```bash
# 设置 API Key
export ANTHROPIC_API_KEY="your-key"
export OPENAI_API_KEY="your-key"

# 设置 Gateway URL
export GOCRAW_GATEWAY_URL="ws://localhost:28789"
export GOCRAW_GATEWAY_TOKEN="your-token"

# 设置日志级别
export GOCRAW_LOG_LEVEL="debug"

# 技能自动安装依赖
export GOCRAW_SKILL_AUTO_INSTALL="true"

# Node.js 包管理器选择
export GOCRAW_NODE_MANAGER="pnpm"
```

### 配置文件

```bash
# 默认配置位置
~/.anyclaw/config.json  # 用户全局目录（最高优先级）
./config.json          # 当前目录

# 工作区
~/.anyclaw/workspace/

# 会话存储
~/.anyclaw/sessions/

# 日志目录
~/.anyclaw/logs/

# 技能目录
~/.anyclaw/skills/     # 用户全局技能
./skills/             # 当前目录技能（最高优先级）
```

### 组合命令

```bash
# 启动 agent 并指定模型
anyclaw start --model claude-3-5-sonnet-20241022

# 添加 agent 并绑定 channel
anyclaw agents add myagent --workspace /path --bind telegram --bind discord

# 安装 gateway 服务并自定义端口
anyclaw gateway install --port 8080 && anyclaw gateway start

# Cron 任务组合
anyclaw cron add --name "Daily" --at "09:00" --message "日报" && anyclaw cron enable $(anyclaw cron list --json | jq -r '.[0].id')
```

---

## 故障排查

```bash
# 检查配置
anyclaw config show

# 检查 gateway 连接
anyclaw gateway probe

# 深度健康检查
anyclaw gateway status --deep

# 查看 channel 日志
anyclaw channels logs --channel all --lines 200

# 检查技能依赖
anyclaw skills check

# 验证配置
anyclaw config validate

# 查看详细日志
GOCRAW_LOG_LEVEL=debug anyclaw start
```

---

## 参考资源

- 项目文档：`README.md`
- 问题反馈：使用你自己的 issue 跟踪系统
- [OpenClaw 文档](https://docs.openclaw.ai)
- [AgentSkills 规范](https://agentskills.io)
