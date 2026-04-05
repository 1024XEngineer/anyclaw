# AnyClaw 故障排查指南

## 快速诊断

### 基础检查

```bash
# 运行诊断
./anyclaw doctor

# 检查连接
./anyclaw doctor --connectivity

# 检查配置
./anyclaw config validate
```

## 常见问题

### Gateway 启动失败

**症状**: `./anyclaw gateway start` 报错

**排查步骤**:

1. 检查端口占用

```bash
# Windows
netstat -ano | findstr 18789

# Linux/Mac
lsof -i :18789
```

2. 检查配置文件

```bash
./anyclaw config validate
./anyclaw config show
```

3. 检查日志

```bash
./anyclaw logs --gateway
```

**常见原因**:

- 端口被占用 → 更改端口或停止占用进程
- 配置错误 → 修复 JSON 格式
- 权限不足 → 以管理员运行

### LLM 连接失败

**症状**: 发送消息无响应或超时

**排查步骤**:

```bash
# 测试提供商连接
./anyclaw provider test

# 检查 API Key
./anyclaw config show llm.api_key

# 验证端点
curl -s -X POST <base_url>/chat/completions \
  -H "Authorization: Bearer <api_key>" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}'
```

**常见原因**:

- API Key 错误 → 检查并更新 Key
- 网络限制 → 配置代理或 VPN
- 模型不存在 → 检查模型名称

### 渠道连接失败

**症状**: Telegram/Discord/Slack 等渠道无响应

**排查步骤**:

```bash
# 检查渠道配置
./anyclaw channels status

# 测试 Webhook
curl -X POST <webhook_url>

# 检查 Bot Token
./anyclaw config show channels.telegram.bot_token
```

**常见原因**:

- Bot Token 错误 → 重新获取 Token
- Webhook 未设置 → 配置 Webhook
- 权限不足 → 检查 Bot 权限

### 技能加载失败

**症状**: 技能无法使用或报错

**排查步骤**:

```bash
# 列出技能
./anyclaw skills list

# 查看技能详情
./anyclaw skills info <skill-name>

# 运行健康检查
./anyclaw skills health <skill-name>
```

**常见原因**:

- 缺少依赖工具 → 检查 required_tools
- 权限不足 → 检查 permissions
- 配置错误 → 检查 skill.json

### 节点无法连接

**症状**: 节点状态始终 offline

**排查步骤**:

```bash
# 检查节点状态
./anyclaw nodes list

# 测试连接
./anyclaw nodes ping <node-id>

# 检查配对状态
./anyclaw nodes pairing status
```

**常见原因**:

- 未配对 → 生成并完成配对
- 网络不通 → 检查网络连接
- Token 过期 → 重新配对

## 性能问题

### 响应缓慢

**排查**:

1. 检查系统资源

```bash
# CPU 使用
top

# 内存使用
free -h

# 磁盘使用
df -h
```

2. 检查 Gateway 性能

```bash
curl http://localhost:18789/status
```

3. 检查运行时状态

```bash
curl http://localhost:18789/runtimes
```

**解决方案**:

- 降低 max_tokens
- 减少并发会话数
- 增加 Runtime 池大小
- 启用缓存

### 内存泄漏

**症状**: 内存使用持续增长

**排查**:

```bash
# 查看进程内存
ps aux | grep anyclaw

# 定期重启
./anyclaw gateway restart
```

## 日志分析

### 日志级别

```bash
# 设置日志级别
./anyclaw config set logging.level debug

# 查看特定日志
./anyclaw logs --filter "error"
./anyclaw logs --filter "warning"
```

### 常用日志模式

| 模式 | 说明 |
|------|------|
| `authentication` | 认证相关 |
| `channel` | 渠道消息 |
| `runtime` | 执行时问题 |
| `llm` | LLM 调用 |
| `tool` | 工具执行 |

### 导出日志

```bash
# 导出到文件
./anyclaw logs export --output logs.jsonl

# 导出特定时间段
./anyclaw logs export --since "2024-01-01" --until "2024-01-02"
```

## 数据恢复

### 配置恢复

```bash
# 查看备份
./anyclaw config history

# 恢复配置
./anyclaw config restore <backup-id>

# 重置为默认
./anyclaw config reset
```

### 数据恢复

```bash
# 列出可用备份
./anyclaw backup list

# 恢复数据
./anyclaw backup restore <backup-id>

# 导出数据
./anyclaw data export --format json
```

## 网络问题

### 无法访问 Gateway

**排查**:

```bash
# 检查监听
netstat -an | grep 18789

# 测试本地连接
curl http://127.0.0.1:18789/healthz

# 测试远程连接
curl http://<ip>:18789/healthz
```

### 代理配置

```json
{
  "llm": {
    "proxy": "http://proxy:8080"
  }
}
```

### 防火墙

```bash
# Windows
netsh advfirewall firewall add rule name="AnyClaw" dir=in action=allow localport=18789

# Linux
sudo ufw allow 18789/tcp
```

## 崩溃处理

### 捕获崩溃信息

```bash
# 查看崩溃日志
./anyclaw logs --crash

# 获取堆栈跟踪
./anyclaw doctor --debug
```

### 自动重启

```bash
# 配置自动重启
./anyclaw config set daemon.auto_restart true
./anyclaw config set daemon.restart_delay 5
```

## 获取帮助

### 收集诊断信息

```bash
# 完整诊断包
./anyclaw doctor --full --output diagnostics.zip

# 包含日志
./anyclaw doctor --full --include-logs --output diagnostics.zip
```

### 社区支持

- GitHub Issues: https://github.com/anyclaw/anyclaw/issues
- Discord: https://discord.gg/anyclaw

### 报告问题

请包含以下信息：

1. 操作系统和版本
2. AnyClaw 版本
3. 完整错误信息
4. 配置文件（隐藏敏感信息）
5. 相关日志

## 下一步

- 阅读[快速开始指南](QUICKSTART.md)
- 阅读[安全配置指南](SECURITY.md)
- 阅读[部署指南](DEPLOYMENT.md)