# AnyClaw 安全配置指南

AnyClaw 的安全边界由配置、Gateway 鉴权、渠道策略、工具权限、沙箱和审计日志共同组成。

## 默认安全姿态

```json
{
  "agent": {
    "permission_level": "limited",
    "require_confirmation_for_dangerous": true
  },
  "sandbox": {
    "enabled": false,
    "execution_mode": "sandbox"
  },
  "channels": {
    "security": {
      "dm_policy": "allow-list",
      "group_policy": "mention-only",
      "mention_gate": true,
      "default_deny_dm": true
    }
  },
  "security": {
    "protect_events": true,
    "rate_limit_rpm": 120
  }
}
```

注意：如果 `sandbox.execution_mode` 为 `sandbox`，但 `sandbox.enabled=false`，host shell 执行会被拒绝。需要本机审阅执行时，将 `sandbox.execution_mode` 设置为 `host-reviewed`。

## Gateway 鉴权

对非本地或公网部署，建议设置 `security.api_token`：

```json
{
  "security": {
    "api_token": "replace-with-a-strong-token"
  }
}
```

请求时使用：

```bash
curl -H "Authorization: Bearer replace-with-a-strong-token" http://127.0.0.1:18789/status
```

## 用户和角色

```json
{
  "security": {
    "users": [
      {
        "name": "admin",
        "token": "replace-with-user-token",
        "role": "admin",
        "permissions": ["*"]
      }
    ],
    "roles": [
      {
        "name": "admin",
        "description": "Full access",
        "permissions": ["*"]
      },
      {
        "name": "viewer",
        "description": "Read-only access",
        "permissions": ["status.read", "sessions.read", "tasks.read"]
      }
    ]
  }
}
```

## 渠道安全策略

| 配置 | 建议值 | 说明 |
| --- | --- | --- |
| `dm_policy` | `allow-list` | 只允许明确配置的私聊来源 |
| `group_policy` | `mention-only` | 群聊中仅响应 mention |
| `mention_gate` | `true` | 强制群聊 mention gate |
| `default_deny_dm` | `true` | 默认拒绝未授权私聊 |
| `pairing_enabled` | `false` 或按需开启 | 移动端/设备接入时开启 |

配对命令：

```bash
./anyclaw pairing generate --config anyclaw.json --name "Phone" --type mobile
./anyclaw pairing list --config anyclaw.json
./anyclaw pairing unpair --config anyclaw.json --device <device_id>
```

## 工具权限

Agent 权限级别：

| 级别 | 说明 |
| --- | --- |
| `read-only` | 禁止写入和命令执行 |
| `limited` | 限制写入范围，危险操作需审查 |
| `full` | 允许更广泛操作，仍受安全策略约束 |

危险命令模式示例：

```json
{
  "security": {
    "dangerous_command_patterns": [
      "rm -rf",
      "del /f",
      "format ",
      "mkfs",
      "shutdown",
      "reboot",
      "poweroff",
      "chmod 777",
      "takeown",
      "icacls",
      "git reset --hard"
    ]
  }
}
```

保护路径和允许路径：

```json
{
  "security": {
    "protected_paths": ["~/.ssh", "~/.gnupg"],
    "allowed_read_paths": ["workflows"],
    "allowed_write_paths": ["workflows"]
  }
}
```

## 沙箱

本地文件系统沙箱：

```json
{
  "sandbox": {
    "enabled": true,
    "execution_mode": "sandbox",
    "backend": "local",
    "base_dir": ".anyclaw/sandboxes"
  }
}
```

Docker 沙箱需要本机 Docker 可用，并会由运行时通过 Docker CLI 创建执行容器：

```json
{
  "sandbox": {
    "enabled": true,
    "execution_mode": "sandbox",
    "backend": "docker",
    "docker_image": "anyclaw-sandbox:local",
    "docker_network": "none"
  }
}
```

## 审计日志

默认位置：

```json
{
  "security": {
    "audit_log": ".anyclaw/audit/audit.jsonl"
  }
}
```

查看：

```bash
tail -n 50 .anyclaw/audit/audit.jsonl
```

## 配置检查

```bash
./anyclaw config validate --config anyclaw.json
./anyclaw doctor --config anyclaw.json
./anyclaw health --config anyclaw.json --verbose
```

## 上线前检查清单

- 设置 `security.api_token` 或放在可信内网。
- 不把真实 API key 写入公开仓库。
- 公网访问时使用 HTTPS 反向代理。
- 保持 `channels.security.default_deny_dm=true`。
- 对 shell 执行启用沙箱，或明确使用 `host-reviewed` 并限制保护路径。
- 定期检查 `.anyclaw/audit/audit.jsonl`。

## 下一步

- 阅读 [部署指南](DEPLOYMENT.md)
- 阅读 [故障排查指南](TROUBLESHOOTING.md)
