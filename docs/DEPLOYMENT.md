# AnyClaw 部署指南

本文档描述当前仓库已经实现的部署路径：本地 Gateway、Docker 镜像、Docker Compose 和 Kubernetes。远程访问建议通过反向代理、Tailscale 或 SSH tunnel 暴露 Gateway 端口完成，AnyClaw CLI 当前不提供独立的 `remote` 子命令。

## 本地 Gateway

### 启动

```bash
# 前台运行
./anyclaw gateway run --config anyclaw.json

# 后台守护进程
./anyclaw gateway daemon start --config anyclaw.json

# 停止守护进程
./anyclaw gateway daemon stop --config anyclaw.json

# 查看状态
./anyclaw gateway status --config anyclaw.json
```

### 最小配置

```json
{
  "gateway": {
    "host": "127.0.0.1",
    "port": 18789,
    "bind": "loopback"
  },
  "security": {
    "api_token": ""
  }
}
```

如果要让局域网或反向代理访问 Gateway，可以将 `gateway.host` 设置为 `0.0.0.0`，并配合防火墙和 `security.api_token` 使用。

## Docker 镜像

仓库根目录的 `Dockerfile` 用于运行 AnyClaw Gateway，`Dockerfile.sandbox` 用于构建隔离工具执行环境。

```bash
docker build -t anyclaw:local .
docker build -f Dockerfile.sandbox -t anyclaw-sandbox:local .
docker run --rm -p 18789:18789 \
  -v "$PWD/anyclaw.example.json:/workspace/anyclaw.json:ro" \
  -v anyclaw-data:/workspace/.anyclaw \
  anyclaw:local gateway run --config /workspace/anyclaw.json --host 0.0.0.0
```

## Docker Compose

```yaml
services:
  anyclaw:
    build: .
    command: ["gateway", "run", "--config", "/workspace/anyclaw.json", "--host", "0.0.0.0"]
    ports:
      - "18789:18789"
    volumes:
      - ./anyclaw.example.json:/workspace/anyclaw.json:ro
      - anyclaw-data:/workspace/.anyclaw
    environment:
      - ANYCLAW_LLM_API_KEY=${ANYCLAW_LLM_API_KEY}
    restart: unless-stopped

volumes:
  anyclaw-data:
```

## Kubernetes 示例

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: anyclaw-config
data:
  anyclaw.json: |
    {
      "llm": {
        "provider": "openai",
        "model": "gpt-4o-mini",
        "api_key": ""
      },
      "gateway": {
        "host": "0.0.0.0",
        "port": 18789,
        "bind": "all"
      }
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
          image: anyclaw:local
          args: ["gateway", "run", "--config", "/config/anyclaw.json", "--host", "0.0.0.0"]
          ports:
            - containerPort: 18789
          volumeMounts:
            - name: config
              mountPath: /config/anyclaw.json
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

## 监控和检查

```bash
curl http://127.0.0.1:18789/healthz
curl http://127.0.0.1:18789/status
./anyclaw status --config anyclaw.json
./anyclaw health --config anyclaw.json --verbose
```

## 备份与恢复

AnyClaw 当前没有独立的 `config export/import` CLI。备份时请保存配置文件和工作目录：

```bash
cp anyclaw.json anyclaw.backup.json
tar -czf anyclaw-data.tar.gz .anyclaw workflows
```

恢复时将这些文件放回同一位置，然后运行：

```bash
./anyclaw config validate --config anyclaw.json
./anyclaw gateway run --config anyclaw.json
```

## 远程访问建议

- 使用 HTTPS 反向代理终止 TLS，并转发到 `127.0.0.1:18789`。
- 使用 Tailscale 或 SSH tunnel 时，只暴露 Gateway 端口，不需要 AnyClaw 专用 remote CLI。
- 对公网部署时，务必设置 `security.api_token`，并限制来源网络。

## 下一步

- 阅读 [安全配置指南](SECURITY.md)
- 阅读 [技能系统文档](SKILLS.md)
- 查看 [故障排查指南](TROUBLESHOOTING.md)
