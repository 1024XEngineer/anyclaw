# AnyClaw 远程部署指南

## 部署模式

AnyClaw 支持多种远程部署模式：

1. **本地 Gateway** - 直接在目标机器运行
2. **远程 Gateway** - 通过远程访问连接
3. **Tailscale 模式** - 通过 Tailscale 网络连接
4. **SSH Tunnel** - 通过 SSH 隧道连接

## 本地 Gateway 部署

### 快速启动

```bash
# 启动 Gateway
./anyclaw gateway start

# 后台运行
./anyclaw gateway daemon

# 检查状态
./anyclaw gateway status
```

### 配置

```json
{
  "gateway": {
    "host": "0.0.0.0",
    "port": 18789,
    "bind": "all"
  }
}
```

## 远程 Gateway 模式

### 架构

```
[客户端] <---> [远程 Gateway] <--- Tailscale/SSH ---> [节点]
```

### 配置远程 Gateway

```json
{
  "remote": {
    "mode": "gateway",
    "endpoint": "https://your-gateway.example.com",
    "token": "your-auth-token"
  }
}
```

### 启动远程连接

```bash
./anyclaw remote connect --mode gateway --endpoint https://...
```

## Tailscale 模式

### 前提条件

1. 安装 Tailscale: `curl -fsSL https://tailscale.com/install.sh`
2. 登录 Tailscale: `tailscale up`
3. 获取 Tailscale IP 地址: `tailscale ip -4`

### 配置

```json
{
  "remote": {
    "mode": "tailscale",
    "tailscale_ip": "100.x.x.x",
    "gateway_port": 18789
  }
}
```

### 启动

```bash
# 连接到 Tailscale 节点
./anyclaw remote connect --mode tailscale --ip 100.x.x.x

# 或使用节点名称
./anyclaw remote connect --mode tailscale --node my-node
```

## SSH Tunnel 模式

### 前提条件

- SSH 访问权限
- SSH 密钥或密码

### 配置

```json
{
  "remote": {
    "mode": "ssh",
    "user": "user@hostname",
    "key_path": "~/.ssh/id_rsa",
    "local_port": 18789,
    "remote_port": 18789
  }
}
```

### 启动

```bash
./anyclaw remote connect --mode ssh --user user@hostname --key ~/.ssh/id_rsa
```

## 云端部署

### Docker 部署

```dockerfile
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o anyclaw ./cmd/anyclaw

FROM alpine:latest
COPY --from=builder /app/anyclaw /usr/local/bin/
COPY anyclaw.json /app/
WORKDIR /app
EXPOSE 18789
CMD ["anyclaw", "gateway", "start"]
```

### Docker Compose

```yaml
version: '3.8'
services:
  anyclaw:
    build: .
    ports:
      - "18789:18789"
    volumes:
      - ./anyclaw.json:/app/anyclaw.json
      - ./data:/app/.anyclaw
    environment:
      - ANYCLAW_LLM_API_KEY=${LLM_API_KEY}
    restart: unless-stopped
```

### Kubernetes 部署

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: anyclaw-config
data:
  anyclaw.json: |
    {
      "llm": { ... },
      "gateway": { ... }
    }
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: anyclaw
spec:
  replicas: 1
  selector:
    matchLabels:
      app: anyclaw
  template:
    metadata:
      labels:
        app: anyclaw
    spec:
      containers:
      - name: anyclaw
        image: anyclaw:latest
        ports:
        - containerPort: 18789
        volumeMounts:
        - name: config
          mountPath: /app/anyclaw.json
          subPath: anyclaw.json
        env:
        - name: ANYCLAW_LLM_API_KEY
          valueFrom:
            secretKeyRef:
              name: anyclaw-secrets
              key: api-key
      volumes:
      - name: config
        configMap:
          name: anyclaw-config
```

## 安全配置

### 强制 HTTPS

```json
{
  "gateway": {
    "tls": {
      "cert": "/path/to/cert.pem",
      "key": "/path/to/key.pem"
    }
  }
}
```

### API Token

```json
{
  "security": {
    "api_token": "your-secure-token"
  }
}
```

### 用户认证

```json
{
  "security": {
    "users": [
      {
        "name": "admin",
        "token": "user-token",
        "role": "admin"
      }
    ]
  }
}
```

## 监控

### 日志

```bash
# 查看 Gateway 日志
./anyclaw logs --gateway

# 实时日志
./anyclaw logs --gateway --follow
```

### 指标

```bash
# 获取状态
curl http://localhost:18789/status

# 获取健康检查
curl http://localhost:18789/healthz
```

## 备份与恢复

### 备份

```bash
# 备份配置
./anyclaw config export backup.json

# 备份数据
tar -czf anyclaw-data.tar.gz .anyclaw/
```

### 恢复

```bash
# 恢复配置
./anyclaw config import backup.json

# 恢复数据
tar -xzf anyclaw-data.tar.gz
```

## 故障排查

### 连接问题

```bash
# 检查网络
./anyclaw doctor --connectivity

# 测试端点
curl http://remote-gateway:18789/healthz
```

### 认证问题

```bash
# 验证 token
./anyclaw auth verify --token your-token
```

## 下一步

- 阅读[技能系统文档](SKILLS.md)
- 阅读[安全配置指南](SECURITY.md)
- 查看[故障排查指南](TROUBLESHOOTING.md)