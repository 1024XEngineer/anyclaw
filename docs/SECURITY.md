# AnyClaw 安全配置指南

## 安全默认值

AnyClaw 默认启用以下安全措施：

```json
{
  "channels": {
    "security": {
      "dm_policy": "allow-list",
      "group_policy": "mention-only",
      "mention_gate": true,
      "default_deny_dm": true,
      "pairing_ttl_hours": 72
    }
  },
  "security": {
    "protect_events": true,
    "rate_limit_rpm": 120
  }
}
```

## 渠道安全策略

### DM 策略

| 模式 | 说明 | 适用场景 |
|------|------|----------|
| `deny-all` | 拒绝所有 DM | 高安全环境 |
| `allow-list` | 仅允许列表用户 | 推荐默认 |
| `pairing` | 需要配对设备 | 移动设备 |
| `allow-all` | 允许所有 DM | 仅测试用 |

### 群组策略

| 模式 | 说明 |
|------|------|
| `deny-all` | 拒绝所有群组消息 |
| `mention-only` | 仅响应 @mentioned |
| `allow-list` | 仅允许列表用户 |
| `allow-all` | 允许所有消息 |

### 配置示例

```json
{
  "channels": {
    "security": {
      "dm_policy": "allow-list",
      "group_policy": "mention-only",
      "mention_gate": true,
      "default_deny_dm": true,
      "allow_from": ["user-id-1", "user-id-2"]
    }
  }
}
```

## 节点配对

### 启用配对

```json
{
  "channels": {
    "security": {
      "pairing_enabled": true,
      "pairing_ttl_hours": 72
    }
  }
}
```

### 生成配对码

```bash
./anyclaw nodes pairing generate --ttl 72h
```

### 管理配对

```bash
# 列出已配对节点
./anyclaw nodes list

# 撤销配对
./anyclaw nodes revoke <node-id>
```

## 权限系统

### 权限级别

| 级别 | 说明 |
|------|------|
| `limited` | 仅基础工具，禁止危险操作 |
| `standard` | 常规操作，危险操作需确认 |
| `full` | 所有操作，危险操作需确认 |

### 配置

```json
{
  "agent": {
    "permission_level": "limited",
    "require_confirmation_for_dangerous": true
  }
}
```

## 安全审计

### 运行安全审计

```bash
./anyclaw doctor --security
```

### 审计检查项

1. **DM 策略** - 是否为 allow-list
2. **群组策略** - 是否为 mention-only
3. **Mention Gate** - 是否启用
4. **默认拒绝** - 是否启用
5. **风险确认** - 是否已确认

### 自动修复

```bash
./anyclaw doctor --auto-fix
```

## 审计日志

### 配置

```json
{
  "security": {
    "audit_log": ".anyclaw/audit/audit.jsonl"
  }
}
```

### 查看日志

```bash
# 查看审计日志
./anyclaw logs --audit

# 导出日志
./anyclaw logs export --format jsonl --output audit.jsonl
```

## 速率限制

### 配置

```json
{
  "security": {
    "rate_limit_rpm": 120
  }
}
```

### 默认值

- **请求速率**: 120 RPM
- **危险命令**: 单独限制

## 保护路径

### 默认保护路径

```json
{
  "security": {
    "protected_paths": [
      "~/.ssh",
      "~/.gnupg",
      "C:\\Users\\*\\.ssh",
      "C:\\Users\\*\\.gnupg"
    ]
  }
}
```

### 添加自定义路径

```bash
./anyclaw config add security.protected_paths /path/to/protect
```

## 危险命令模式

### 配置

```json
{
  "security": {
    "dangerous_command_patterns": [
      "rm -rf",
      "del /f",
      "format",
      "shutdown",
      "reboot"
    ]
  }
}
```

## 用户与角色

### 配置用户

```json
{
  "security": {
    "users": [
      {
        "name": "admin",
        "token": "admin-token",
        "role": "admin",
        "permissions": ["*"]
      },
      {
        "name": "operator",
        "token": "operator-token",
        "role": "operator",
        "permissions": ["read", "write", "execute"]
      }
    ],
    "roles": [
      {
        "name": "admin",
        "description": "Full access",
        "permissions": ["*"]
      },
      {
        "name": "operator",
        "description": "Operator role",
        "permissions": ["read", "write", "execute", "channels.manage"]
      },
      {
        "name": "viewer",
        "description": "Read-only access",
        "permissions": ["read", "status.read"]
      }
    ]
  }
}
```

### 权限列表

| 权限 | 说明 |
|------|------|
| `read` | 读取数据 |
| `write` | 写入数据 |
| `execute` | 执行命令 |
| `status.read` | 读取状态 |
| `channels.manage` | 管理渠道 |
| `nodes.manage` | 管理节点 |
| `config.write` | 修改配置 |
| `skills.manage` | 管理技能 |
| `*` | 所有权限 |

## API Token

### 生成 Token

```bash
# 生成随机 token
./anyclaw auth generate-token

# 指定 token
./anyclaw auth set-token your-token
```

### 使用 Token

```bash
# Header 方式
curl -H "Authorization: Bearer your-token" http://localhost:18789/status

# Query 方式
curl http://localhost:18789/status?token=your-token
```

## 故障排查

### 常见安全错误

| 错误 | 原因 | 解决方案 |
|------|------|----------|
| 401 Unauthorized | Token 错误 | 检查 token 配置 |
| 403 Forbidden | 权限不足 | 检查用户权限 |
| DM blocked | DM 策略阻止 | 添加到 allow_from |
| Command blocked | 危险命令 | 检查 patterns |

### 调试

```bash
# 详细日志
./anyclaw logs --level debug

# 安全审计
./anyclaw doctor --security --verbose
```

## 下一步

- 阅读[快速开始指南](QUICKSTART.md)
- 阅读[部署指南](DEPLOYMENT.md)
- 阅读[故障排查指南](TROUBLESHOOTING.md)