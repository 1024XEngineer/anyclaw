# AnyClaw 官网 + 云端市场部署架构

本文档对应 `WEBSITE_MARKETPLACE_DEPLOYMENT_PLAN.md` 的 Round 1，用来固定上线架构、端口、安全组、IP 与域名两种访问路径。

## 一、目标

先在阿里云香港 ECS 上用服务器 IP 跑通：

- 官网可访问。
- 官网能读取 registry 展示 agent / skill / cli。
- AnyClaw 本地市场能读取云端 registry。
- Registry admin / publish 接口受 token 保护。

后续有域名后平滑升级：

- 官网走 HTTPS。
- Registry 通过同域 `/v1/*` 暴露，减少浏览器跨域和额外端口暴露。
- Caddy 或 Nginx 终止 TLS。

## 二、服务拓扑

### 2.1 IP 先跑通

```text
Internet
  |
  | http://SERVER_IP
  v
Website container
  |
  | internal http://registry:8791/v1/*
  v
anyclaw-registry container
  |
  v
registry volume: registry.db + packages + audit
```

临时调试时可以直接开放：

```text
http://SERVER_IP:8791/v1/health
http://SERVER_IP:8791/v1/artifacts
```

但最终建议通过官网入口同源反代 `/v1/*`，不要长期暴露 `8791`。

### 2.2 有域名 + HTTPS

```text
Internet
  |
  | https://your-domain
  v
Caddy/Nginx
  |-- /        -> website:8080
  |-- /v1/*    -> registry:8791
```

这个方案的好处：

- 用户只记一个域名。
- 浏览器读取 registry 不需要 CORS。
- 只开放 80/443，registry 端口不直接暴露公网。
- HTTPS 证书由 Caddy 或 Nginx + Certbot 管理。

## 三、端口规划

| 端口 | 服务 | 是否公网开放 | 说明 |
| --- | --- | --- | --- |
| 22 | SSH | 是，建议限制来源 IP | 服务器运维入口 |
| 80 | Website / reverse proxy | 是 | IP 阶段官网 HTTP；域名阶段 HTTP 跳 HTTPS |
| 443 | reverse proxy TLS | 有域名后开放 | HTTPS |
| 8791 | anyclaw-registry | IP 调试阶段可临时开放 | 长期建议仅容器内访问或只绑定 127.0.0.1 |
| 18789 | AnyClaw Gateway | 默认不公网开放 | 如开放必须设置 `ANYCLAW_API_TOKEN` |

## 四、阿里云安全组建议

### 4.1 IP-only 首次跑通

最低开放：

- TCP 22：限制 owner 当前公网 IP 更好。
- TCP 80：`0.0.0.0/0`。
- TCP 8791：临时开放用于 registry smoke test；上线后关闭或仅允许 owner IP。

不要开放：

- TCP 18789，除非明确要公网访问 Gateway，并且已配置 `ANYCLAW_API_TOKEN`。

### 4.2 有域名 + HTTPS 后

开放：

- TCP 22：限制 owner 当前公网 IP。
- TCP 80：`0.0.0.0/0`，用于 ACME 校验和跳转 HTTPS。
- TCP 443：`0.0.0.0/0`。

关闭：

- TCP 8791。
- TCP 18789。

## 五、环境变量规范

生产部署使用 `.env.production`，模板来自仓库根目录：

```text
.env.production.example
```

关键变量：

- `ANYCLAW_PUBLIC_IP`：无域名阶段用于拼接访问地址。
- `ANYCLAW_PUBLIC_DOMAIN`：有域名后填写。
- `ANYCLAW_PUBLIC_SCHEME`：无域名时 `http`，有 HTTPS 后 `https`。
- `ANYCLAW_REGISTRY_ADMIN_TOKEN`：必须填写强随机值，公网环境不能为空。
- `ANYCLAW_MARKETPLACE_ENDPOINT`：AnyClaw Gateway 读取 registry 的 endpoint。
- `ANYCLAW_API_TOKEN`：Gateway 如果公网可达，必须填写。
- `ANYCLAW_WEBSITE_REGISTRY_BASE`：官网浏览器侧读取 registry 的 base，优先使用 `/v1`。

生成强 token 示例：

```bash
openssl rand -hex 32
```

## 六、Registry 数据路径

容器内：

```text
/data/registry.db
/data/packages/
/data/audit/
```

服务器持久化目录建议：

```text
/opt/anyclaw/registry-data/
```

后续备份 Round 10 会覆盖：

- SQLite 数据库备份。
- packages 目录打包。
- 恢复演练。
- 保留策略。

## 七、IP 与域名的 endpoint 写法

### 7.1 IP-only，直接 registry 端口

适合 Round 3 早期 smoke test：

```text
ANYCLAW_MARKETPLACE_ENDPOINT=http://SERVER_IP:8791
```

优点：最简单。

缺点：额外暴露 `8791`，浏览器从官网读取 registry 可能遇到跨域。

### 7.2 IP-only，同源反代

适合 Round 2 后：

```text
ANYCLAW_MARKETPLACE_ENDPOINT=http://SERVER_IP/v1
ANYCLAW_WEBSITE_REGISTRY_BASE=/v1
```

优点：官网和 registry 同源，不需要 CORS，不长期暴露 `8791`。

缺点：需要 website 或 reverse proxy 提供 `/v1/*` 反代。

### 7.3 域名 + HTTPS

正式推荐：

```text
ANYCLAW_MARKETPLACE_ENDPOINT=https://your-domain/v1
ANYCLAW_WEBSITE_REGISTRY_BASE=/v1
```

优点：生产体验最好，安全边界最清楚。

缺点：需要域名 DNS 和 TLS 组件。

## 八、依赖策略

Round 1 不新增运行时依赖。

后续可能需要 owner 确认的依赖：

- Caddy 或 Nginx：用于反向代理和 HTTPS。
- 静态网站服务镜像：如果不复用已有 Node/Vite 构建产物，需要确认。
- Postgres driver：只有切换 registry 到 Postgres 时才需要。
- 对象存储 SDK：只有切换 packages 到 OSS/S3/R2/COS 时才需要。

## 九、Round 1 验收

本轮完成标准：

- `.env.production.example` 已提供。
- 本部署架构文档已提供。
- 总执行计划文档已更新 Round 1 状态。
- 不启动服务、不新增外部依赖、不修改业务逻辑。
