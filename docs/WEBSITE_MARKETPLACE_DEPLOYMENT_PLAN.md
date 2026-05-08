# AnyClaw 官网 + 云端市场部署上线总执行计划

本文档是 AnyClaw 官网与云端市场部署上线的执行计划。后续执行必须按轮次推进；每完成一轮更新状态，等待 owner 说“继续”后再进入下一轮。

确认时间：2026-05-07

## 执行规则

- 不擅自跳轮次。
- 不擅自扩大范围。
- 不擅自改动本计划；如发现计划不合理，先说明问题、建议、影响轮次和风险，等待确认。
- 不默认使用宝塔面板。
- 不随便新增外部依赖；新增依赖前说明理由并等待确认。
- 每轮必须有验证结果。
- 若有未完成项，必须明确列出。

## 当前代码核对结论

云端市场代码不是纯文档占位，当前仓库已经具备核心实现：

- 本地市场 API：基本完成。`/market/artifacts`、detail、versions、install、upgrade、uninstall、jobs、bindings、events、refresh 都有入口。
- 云端 registry：基本完成。`cmd/anyclaw-registry` 与 `pkg/marketregistry` 支持 catalog、resolve、download、search、publish、admin。
- install / bind / upgrade / uninstall：基本完成。有 job、receipt、checksum 校验、binding、upgrade 保留绑定、uninstall 删除 receipt/binding。
- policy / audit / event：部分完成。有 auto/ask/block、audit jsonl、events 文件；但事件名和文档理想的 EventBus/Outbox 架构不完全一致。
- agent auto supplement：基本完成但偏轻量。`pkg/capability/markettools` 提供 main-agent 市场工具；不是完整 Need Detector/Router 多包架构。
- hot reload：基本完成。`pkg/runtime/hot_reload.go` 有 RefreshScope/Coordinator，Gateway refresh/binding 使用它。
- registry hardening：部分完成。有 `database/sql` store、publisher token、quarantine、admin audit/downloads；生产级 Postgres 驱动与对象存储适配未真正落地。
- publish / quarantine / admin audit / downloads：基本完成。API 存在并有测试。
- 官网：未完成。需要新建。
- 下载页 / 一键下载：未完成。需要新建下载体验与发布产物流程。
- 生产部署编排：未完成。当前 `docker-compose.yml` 只跑 Gateway，需要 registry、website、备份、安全组、域名/IP 方案。

重要风险：

- Registry 的 admin token 若为空，当前实现会放行 admin 接口；公网部署前必须配置强 token，必要时增加生产模式保护。
- 当前工作树中市场相关文件有未跟踪和已修改状态，执行前必须持续保护已有改动，不回滚用户改动。
- 当前市场实现比设计文档更轻量，不能把文档中理想的 ports/app/events/outbox/observability 多包架构误认为已完全落地。

## 服务器信息

- 云厂商：阿里云 ECS
- 地域：中国香港
- 配置：2 核 4G
- 系统：Ubuntu 22.04 64 位
- Docker：已预装
- 系统盘：40GB ESSD Entry
- 流量：约 220GB/月
- 域名：可能暂时没有，必须支持先用服务器 IP 跑通

## 总体部署目标

第一阶段先用服务器 IP 跑通：

```text
浏览器
  -> http://SERVER_IP
  -> 官网
  -> /v1/* 反代到 anyclaw-registry
```

有域名后升级为：

```text
浏览器
  -> https://your-domain
  -> Caddy/Nginx TLS
  -> 官网静态服务
  -> /v1/* 反代到 anyclaw-registry
```

AnyClaw 本地市场读取：

```text
ANYCLAW_MARKETPLACE_ENDPOINT=http://SERVER_IP/v1 或 https://your-domain/v1
```

## 轮次状态

| 轮次 | 状态 | 说明 |
| --- | --- | --- |
| Round 0 | complete | 上线前冻结与基线核验 |
| Round 1 | complete | 部署架构定稿与环境变量规范 |
| Round 2 | complete | Docker Compose 生产编排 |
| Round 3 | complete | anyclaw-registry 生产启动 |
| Round 4 | complete | 官网信息架构与视觉方案 |
| Round 5 | complete | 官网静态实现 |
| Round 6 | complete | 官网读取 Registry 展示 agent / skill / cli |
| Round 7 | complete | 下载页和一键下载流程 |
| Round 8 | complete | agent / skill / cli 包发布流程 |
| Round 9 | complete | admin token / publisher token / 安全基线 |
| Round 10 | complete | Registry 数据备份与恢复 |
| Round 11 | complete | 服务器上线，IP 方案已跑通 |
| Round 11.5 | complete | 1Panel 可视化运维面板已安装并公网可访问 |
| Round 12 | complete | 当前线上市场修复与基线清理 |
| Round 13 | complete | 发布真实 agent / skill / cli 数据 |
| Round 13.5 | complete | Web + 桌面壳云端市场显示修复与 seed 清理 |
| Round 14 | complete | AnyClaw 客户端云端安装闭环验收 |
| Round 15 | complete | 安装后自动集成体验增强 |
| Round 16 | complete | 市场搜索、筛选、详情页产品化 |
| Round 17 | complete | 发布后台 MVP |
| Round 18 | complete | 安全体系增强 |
| Round 19 | complete | 官网市场完整产品化 |
| Round 20 | pending | 生产部署增强、HTTPS、备份、监控 |
| Round 21 | pending | 最终上线验收 |

## Round 0：上线前冻结与基线核验

目标：

- 把当前真实代码状态、部署目标、风险项冻结，避免带着错觉上线。

交付物：

- 本计划文档。
- 当前云端市场功能核对清单。
- 必须修复 / 可延后清单。
- 基线验证结果。

涉及文件/模块：

- `docs/*`
- `Dockerfile`
- `docker-compose.yml`
- `.env.example`
- `pkg/marketplace`
- `pkg/marketregistry`
- `pkg/gateway`
- `pkg/runtime`
- `pkg/capability/markettools`
- `ui/src/pages/Market`
- `ui/src/features/market`

验收标准：

- 明确哪些功能已完成、哪些只是开发版可用、哪些上线前必须补。
- 不改 `docs/MARKETPLACE_EXECUTION_PLAN.md` 的历史计划，除非 owner 单独确认。
- 基线测试命令有明确结果。

测试方式：

```powershell
go test ./pkg/marketplace ./pkg/marketplace/registry ./pkg/marketregistry ./pkg/capability/markettools ./pkg/runtime -run "Market|Registry|Install|Policy|Lifecycle|Capability|HotReload|Route"
go test ./pkg/gateway -run "Market"
corepack pnpm --dir ui exec tsc --noEmit -p tsconfig.json
corepack pnpm --dir ui test -- --run useMarketDirectory
```

风险点：

- 当前市场代码有大量未跟踪文件，需要纳入后续版本管理。
- 生产前必须处理 registry admin token 为空放行的问题。
- 前端验证需要通过 `corepack pnpm`，当前 shell 里直接 `pnpm` 不在 PATH。

## Round 1：部署架构定稿与环境变量规范

目标：

- 定稿“先用 IP 跑通，后续可平滑切域名 HTTPS”的部署拓扑。

交付物：

- 部署架构图 / 说明。
- `.env.production.example`。
- 端口规划。
- 安全组规划。
- IP 访问方案和域名访问方案。

涉及文件/模块：

- `.env.example`
- `docs/DEPLOYMENT.md`
- `docs/WEBSITE_MARKETPLACE_DEPLOYMENT_PLAN.md`
- `docker-compose.yml`

端口建议：

- `80`：官网 HTTP，或未来 Caddy/Nginx 入口。
- `443`：未来 HTTPS。
- `8791`：registry 内部端口；IP 阶段可临时开放，最终建议只由反向代理访问。
- `18789`：AnyClaw Gateway；不建议公网裸露，如必须开放必须设置 `ANYCLAW_API_TOKEN`。
- `22`：SSH；最好只允许 owner 固定 IP。

验收标准：

- 有域名方案：`https://your-domain` 可规划到官网，`https://your-domain/v1/*` 到 registry。
- 无域名方案：`http://SERVER_IP` 跑官网，registry 可通过同源路径或临时 `:8791` 跑通。
- 不使用宝塔面板。
- 不引入未确认外部依赖。

测试方式：

- 文档 review。
- `docker compose config` 在 Round 2 前不要求通过。

风险点：

- 无域名阶段不能申请常规公网 HTTPS 证书。
- 开放 registry admin 接口必须配置强 token。

Round 1 交付记录：

- 新增 `.env.production.example`，定义官网、registry、Gateway、IP/域名、token 与 marketplace endpoint 的生产变量。
- 新增 `docs/WEBSITE_MARKETPLACE_DEPLOYMENT_ARCHITECTURE.md`，固定 IP-only 与域名 HTTPS 两种部署拓扑、端口规划、安全组建议和 endpoint 写法。
- 更新 `docs/DEPLOYMENT.md`，增加官网与云端市场上线文档入口。

Round 1 验证记录：

```powershell
docker compose config
```

结果：通过。当前 `.env` 未设置 `ANYCLAW_LLM_API_KEY`，Docker Compose 输出空值警告，这是现有 compose 行为，不影响 Round 1 文档交付。

```powershell
go test ./pkg/marketplace ./pkg/marketregistry ./pkg/gateway -run "Market|Registry"
```

结果：通过。

```powershell
corepack pnpm --dir ui exec tsc --noEmit -p tsconfig.json
```

结果：通过。

Round 1 结论：

- 部署架构和环境变量规范已冻结。
- 本轮未启动服务、未新增外部依赖、未修改业务逻辑。
- 下一轮为 Round 2：Docker Compose 生产编排。必须等待 owner 说“继续”后再开始。

## Round 2：Docker Compose 生产编排

目标：

- 让服务器用 Docker Compose 一键启动 registry、官网、必要的数据卷。

交付物：

- `docker-compose.prod.yml`
- registry 数据卷。
- website 静态服务配置。
- 健康检查。
- restart policy。
- 最小 `.env.production.example`。

涉及文件/模块：

- `Dockerfile`
- `docker-compose.yml`
- `docker-compose.prod.yml`
- `deploy/`

验收标准：

- `docker compose -f docker-compose.prod.yml up -d --build` 可启动。
- `docker compose ps` 全部 healthy 或 running。
- registry 数据落在持久卷，不随容器删除。

测试方式：

```bash
docker compose -f docker-compose.prod.yml config
docker compose -f docker-compose.prod.yml up -d --build
docker compose -f docker-compose.prod.yml ps
curl http://127.0.0.1:8791/v1/health
```

风险点：

- 2 核 4G 构建镜像可能慢，必要时改为本地构建后推镜像；这属于计划变更，需确认。
- SQLite 文件和 packages 目录必须持久化。

Round 2 交付记录：

- 新增 `docker-compose.prod.yml`，定义 `registry`、`website`、`anyclaw` 三个服务。
- `registry` 使用现有 Dockerfile 产出的 `anyclaw-registry`，持久化 `/data` 到 `registry-data` volume，并强制要求 `ANYCLAW_REGISTRY_ADMIN_TOKEN`。
- `website` 复用现有镜像内的 Python 标准库 HTTP server，最初临时服务 `site/public`；Round 5 计划变更后改为服务 React/Vite 构建产物 `site/dist`。
- `anyclaw` 复用生产镜像，默认通过容器网络读取 `http://registry:8791`。
- 新增 `site/public/index.html` 作为 Round 2 占位页，只用于验证 website 容器和端口；Round 5 计划变更后将清理这批纯 HTML 文件。
- 新增 `deploy/README.md`，记录生产 compose 形态、服务器目录建议和 smoke checks。

Round 2 验证记录：

```powershell
$env:ANYCLAW_REGISTRY_ADMIN_TOKEN='round2-config-check'
docker compose -f docker-compose.prod.yml config
```

结果：通过。Compose 正确展开三服务配置。`ANYCLAW_API_TOKEN`、`ANYCLAW_LLM_API_KEY`、`ANYCLAW_REGISTRY_TOKEN` 未设置时会有空值警告；这是当前模板允许的本地 config 验证状态。公网部署时 Gateway 若可达必须设置 `ANYCLAW_API_TOKEN`。

```powershell
Remove-Item Env:ANYCLAW_REGISTRY_ADMIN_TOKEN -ErrorAction SilentlyContinue
docker compose -f docker-compose.prod.yml config
```

结果：按预期失败，提示 `ANYCLAW_REGISTRY_ADMIN_TOKEN is required`，证明生产编排不会在缺少 registry admin token 时启动。

```powershell
go test ./pkg/marketregistry ./pkg/gateway -run "Market|Registry"
```

结果：通过。

```powershell
corepack pnpm --dir ui exec tsc --noEmit -p tsconfig.json
```

结果：通过。

Round 2 结论：

- 生产 Compose 编排已就位。
- 本轮没有实际启动本机或远端容器，没有占用端口。
- 本轮没有新增外部依赖；临时 website 服务使用镜像内已有 `python3`。
- 下一轮为 Round 3：anyclaw-registry 生产启动。必须等待 owner 说“继续”后再开始。

## Round 3：anyclaw-registry 生产启动

目标：

- 先把云端市场 registry 在 ECS 上稳定跑通。

交付物：

- registry service。
- `ANYCLAW_REGISTRY_ADMIN_TOKEN`。
- 初始 seed 或正式包数据。
- admin token 创建 publisher token 的操作文档。
- registry smoke test 脚本。

涉及文件/模块：

- `cmd/anyclaw-registry`
- `pkg/marketregistry`
- `docs/MARKETPLACE_REGISTRY_DEV.md`
- `deploy/registry-smoke.sh`

验收标准：

- `GET /v1/health` 返回 ok。
- `GET /v1/artifacts` 返回 agent / skill / cli。
- `POST /v1/artifacts/{id}/resolve` 返回下载元数据。
- `GET /v1/download/...` 可下载包。
- 未带 admin token 不能访问 admin API。

测试方式：

```bash
curl http://SERVER_IP/v1/health
curl http://SERVER_IP/v1/artifacts
curl -H "Authorization: Bearer $ANYCLAW_REGISTRY_ADMIN_TOKEN" http://SERVER_IP/v1/admin/audit
```

风险点：

- 当前代码 admin token 为空会放行，所以生产 env 必须非空。
- 初期 SQLite 可以用，但需要备份；Postgres 作为后续增强，不能默认为已完成。

Round 3 交付记录：

- 新增 `docs/REGISTRY_PRODUCTION_RUNBOOK.md`，记录 registry 生产启动环境变量、Compose 启动、smoke checks、publisher token 创建和当前生产限制。
- 新增 `deploy/registry-smoke.ps1`，用于检查 registry health、artifact list/detail/versions、resolve、download、admin audit token 访问。
- 本轮未登录远端服务器；由于 owner 尚未提供 SSH 连接方式，本轮使用本机临时 registry 实例验证启动与接口行为。

Round 3 验证记录：

```powershell
go run ./cmd/anyclaw-registry serve --addr :18791 --data-dir tmp/round3-registry-smoke --admin-token round3-local-admin-token --seed=true
.\deploy\registry-smoke.ps1 -BaseUrl http://127.0.0.1:18791 -AdminToken round3-local-admin-token
```

结果：通过。验证项包括：

- `GET /v1/health`
- `GET /v1/artifacts`
- `GET /v1/artifacts/cloud.skill.release-notes`
- `GET /v1/artifacts/cloud.skill.release-notes/versions`
- `POST /v1/artifacts/cloud.skill.release-notes/resolve`
- `GET /v1/download/...`
- `GET /v1/admin/audit` 携带 admin token

说明：第一次 smoke 时 Windows PowerShell `Invoke-WebRequest` 下载检查触发空引用兼容问题，已将脚本改为 `-UseBasicParsing` 后重跑通过。`go run` 在 Windows 下曾留下临时 `anyclaw-registry.exe` 子进程，已手动清理，最终确认没有残留 `:18791` registry 进程。

```powershell
go test ./pkg/marketregistry ./pkg/marketplace/registry ./pkg/gateway -run "Registry|Market"
```

结果：通过。

```powershell
$env:ANYCLAW_REGISTRY_ADMIN_TOKEN='round3-config-check'
docker compose -f docker-compose.prod.yml config
```

结果：通过。Compose 正确展开 registry 生产命令，并把 admin token 注入 `--admin-token`。

Round 3 结论：

- Registry 生产启动手册和 smoke 工具已就位。
- 本机临时 registry 启动、读取、resolve、download、admin audit 验证通过。
- 本轮没有远端服务器操作。
- 下一轮为 Round 4：官网信息架构与视觉方案。必须等待 owner 说“继续”后再开始。

## Round 4：官网信息架构与视觉方案

目标：

- 确定官网第一版长什么样、展示什么、下载路径怎么走。

交付物：

- 官网页面结构。
- 视觉方向。
- 文案草稿。
- 下载页需求。
- 市场展示需求。

页面建议：

- 首页：AnyClaw 是什么、核心能力、市场入口、下载入口。
- Marketplace：读取 registry 展示 agent / skill / cli。
- Download：Windows / macOS / Linux 下载或源码安装。
- Docs / Quickstart：快速开始。
- Admin / Publisher Guide：发布包、token、审核流程说明。

涉及文件/模块：

- 新增官网目录，建议 `site/` 或 `website/`。
- registry API。
- docs / README / quickstart。

验收标准：

- owner 确认页面结构和首版文案方向。
- 不引入新前端框架，除非 owner 确认。

测试方式：

- 设计 review。
- 无代码或只做草图文档。

风险点：

- 官网如果做太重会拖慢上线；第一版建议静态站 + registry API 动态市场列表。

Round 4 交付记录：

- 新增 `docs/WEBSITE_INFORMATION_ARCHITECTURE.md`，明确官网定位、目标用户、页面结构、导航、文案基调、视觉方向、首页首屏草案、市场卡片要求、下载页内容要求和 Round 5 实现边界。
- 更新 `docs/DEPLOYMENT.md`，加入官网信息架构文档入口。
- Round 5 计划变更记录：原方案为纯 HTML 静态页；owner 于 2026-05-07 确认改为 React 官网，但不使用 Next.js。调整后 Round 5 使用 `site/` React + Vite 静态站，构建产物输出到 `site/dist`，仍由 Round 2 的 website 静态服务托管。影响轮次：Round 5 实现方式变化；Round 6 registry 读取将使用 React fetch/state；Round 2 Compose 的 website volume 从 `site/public` 改为 `site/dist`。风险：新增一个 workspace 包和构建步骤，但不新增 Next.js 运行时，不改变服务器部署形态。
- Round 5 实现边界已冻结：实现 React 静态官网 `/`、`/marketplace`、`/download`、`/docs`、`/publish`、404、响应式布局、占位市场卡片和下载命令复制按钮；不实现实时 registry 读取、publish 后台、登录、管理后台、HTTPS、Release 自动构建。

Round 4 验证记录：

```powershell
Get-Content -Raw -Encoding UTF8 docs\WEBSITE_INFORMATION_ARCHITECTURE.md | Measure-Object -Line -Word -Character
```

结果：通过，文档存在并可读取。

```powershell
go test ./pkg/marketregistry ./pkg/gateway -run "Market|Registry"
```

结果：通过。

```powershell
corepack pnpm --dir ui exec tsc --noEmit -p tsconfig.json
```

结果：通过。

Round 4 结论：

- 官网信息架构与视觉方案已冻结。
- 本轮未新增依赖、未修改业务逻辑、未实现正式官网页面。
- 下一轮为 Round 5：官网静态实现。必须等待 owner 说“继续”后再开始。

## Round 5：官网静态实现

目标：

- 实现能上线的官网第一版。

交付物：

- 首页。
- 市场展示页。
- 下载页。
- 基础响应式布局。
- Open Graph / favicon / SEO 基础。
- 404 页面。

涉及文件/模块：

- 新增 `site/` 或 `website/`。
- `Dockerfile` 或独立 `deploy/website.Dockerfile`。
- `deploy/nginx.conf` 或简单静态服务配置。

验收标准：

- 桌面 / 移动端可用。
- 首屏不是空壳，不是控制台 UI。
- 不依赖 AnyClaw Gateway 登录。

测试方式：

- 前端构建。
- 浏览器或 Playwright 截图检查。
- `curl http://localhost/...`

风险点：

- 如果复用现有 React 工具链，需避免把控制台路由和官网路由混在一起。
- 如果引入新的静态站工具，需要 owner 先确认依赖。

Round 5 交付记录：

- 新增 `site/` React + Vite 官网 workspace，独立于 `ui/` 控制台，避免官网路由和控制台路由混在一起。
- 实现 `/`、`/marketplace`、`/download`、`/docs`、`/publish` 和 404。
- Marketplace 页包含 agent / skill / cli 静态 fixture、类型切换和搜索；真实 registry 读取留到 Round 6。
- Download 页提供 Windows、macOS/Linux、Docker Compose Gateway 命令块和复制按钮；未伪装已有 release 安装包。
- 新增基础 SEO / Open Graph / favicon。
- 新增 `deploy/static-spa-server.py`，使用 Python 标准库服务 `site/dist`，并把 React Router 深层路径回退到 `index.html`。
- 更新 `docker-compose.prod.yml`，website 服务从 `site/dist` 托管 React 构建产物，并挂载 SPA 静态服务器脚本。
- 更新 `Dockerfile`，Docker 构建阶段纳入 `site/package.json`，保证 workspace lockfile 的 frozen install 可用。
- 更新根 `package.json`，增加 `site:dev`、`site:build`、`site:preview`。
- 清理 Round 2 遗留的纯 HTML 官网占位文件和空的 `site/public` 目录。
- Python 环境处理：确认 `D:\python\python.exe` 存在，用户级 `PATH` 已加入 `D:\python` 和 `D:\python\Scripts`。已打开的旧终端需要重开后自动生效。

Round 5 验证记录：

```powershell
corepack pnpm install --lockfile-only
corepack pnpm install
```

结果：通过。`pnpm-lock.yaml` 已包含 `site` workspace。

```powershell
corepack pnpm --dir site build
```

结果：通过。产物输出到 `site/dist`。

```powershell
$env:ANYCLAW_REGISTRY_ADMIN_TOKEN='round5-config-check'
docker compose -f docker-compose.prod.yml config
```

结果：通过。Compose 正确展开 website 的 `site/dist` 挂载与 `deploy/static-spa-server.py` 挂载。`ANYCLAW_LLM_API_KEY`、`ANYCLAW_API_TOKEN`、`ANYCLAW_REGISTRY_TOKEN` 未设置时仍有空值警告，属于当前模板行为；公网部署 Gateway 时必须设置 `ANYCLAW_API_TOKEN`。

```powershell
D:\python\python.exe deploy/static-spa-server.py --host 127.0.0.1 --port 18080 --directory site/dist
Invoke-WebRequest http://127.0.0.1:18080/
Invoke-WebRequest http://127.0.0.1:18080/marketplace
Invoke-WebRequest http://127.0.0.1:18080/download
Invoke-WebRequest http://127.0.0.1:18080/docs
Invoke-WebRequest http://127.0.0.1:18080/publish
Invoke-WebRequest http://127.0.0.1:18080/missing-page
```

结果：通过。所有路径返回 `200` 并回到 React 应用；临时 smoke 进程已关闭。

```powershell
corepack pnpm --dir ui exec tsc --noEmit -p tsconfig.json
```

结果：通过。

```powershell
go test ./pkg/marketregistry ./pkg/gateway -run "Market|Registry"
```

结果：通过。

Round 5 结论：

- React/Vite 官网第一版已实现并可构建。
- 本轮没有接入实时 registry 数据，没有做 release 自动构建，没有做登录、后台、HTTPS 或服务器部署。
- 下一轮为 Round 6：官网读取 Registry 展示 agent / skill / cli。必须等待 owner 说“继续”后再开始。

## Round 6：官网读取 Registry 展示 agent / skill / cli

目标：

- 官网 Marketplace 页面真实读取 registry，而不是写死假数据。

交付物：

- registry client for website。
- agent / skill / cli 分类展示。
- 搜索 / 筛选。
- 详情弹层或详情页。
- registry 不可达的降级提示。

涉及文件/模块：

- `site/` 或 `website/`
- `pkg/marketregistry` API 契约
- `deploy` 配置

验收标准：

- `SERVER_IP` 访问官网时能看到 registry 里的云端条目。
- 三类资源都能展示。
- registry 不可用时官网不崩溃。

测试方式：

```bash
curl http://SERVER_IP/v1/artifacts?kind=agent
```

- 浏览器 smoke test。
- 前端构建 / typecheck。

风险点：

- 浏览器跨域：IP 方案下优先同源反代 `/v1/*`，少暴露端口。

Round 6 交付记录：

- 新增 `site/src/registryClient.ts`，按真实 registry envelope 读取 `GET /v1/artifacts?limit=100`，并把 registry artifact 字段映射成官网 `MarketItem`。
- Marketplace 页从静态 fixture 改为优先读取 live registry；请求失败时显示降级提示并展示静态示例。
- Marketplace 页保留 Agent / Skill / CLI 类型筛选和搜索，搜索范围包含名称、摘要、类型、来源、publisher 和 tags。
- 新增加载态、live 状态、fallback 状态和空结果状态。
- 新增详情面板，展示 kind、version、publisher、risk/trust、compatibility、package size、permissions、tags。
- `site/vite.config.ts` 增加开发期 `/v1` proxy，默认转到 `http://127.0.0.1:8791`，可用 `ANYCLAW_SITE_REGISTRY_PROXY_TARGET` 覆盖。
- 扩展 `deploy/static-spa-server.py`：继续服务 React SPA，同时把 `/v1/*` 同源代理到 registry；支持 GET/POST/PUT/PATCH/DELETE/OPTIONS；设置 `X-Forwarded-Host` 和 `X-Forwarded-Proto`，避免后续 registry 生成内部地址。
- 更新 `docker-compose.prod.yml`，website 服务通过 `--registry-target http://registry:8791` 访问容器内 registry。
- 更新 `deploy/README.md`，补充同源 `/v1/health` 和 `/v1/artifacts` smoke checks。

Round 6 验证记录：

```powershell
corepack pnpm --dir site build
```

结果：通过。官网构建产物输出到 `site/dist`。

```powershell
$env:ANYCLAW_REGISTRY_ADMIN_TOKEN='round6-config-check'
docker compose -f docker-compose.prod.yml config
```

结果：通过。Compose 正确展开 website 的 `--registry-target http://registry:8791`。

```powershell
go build -o tmp\round6-smoke\anyclaw-registry.exe ./cmd/anyclaw-registry
tmp\round6-smoke\anyclaw-registry.exe serve --addr :18792 --data-dir tmp/round6-smoke/registry-data --admin-token round6-local-admin-token --seed=true
D:\python\python.exe deploy/static-spa-server.py --host 127.0.0.1 --port 18081 --directory site/dist --registry-target http://127.0.0.1:18792
Invoke-RestMethod http://127.0.0.1:18081/v1/health
Invoke-RestMethod http://127.0.0.1:18081/v1/artifacts?limit=100
Invoke-WebRequest http://127.0.0.1:18081/marketplace
```

结果：通过。`/v1/health` 返回 `ok`，`/v1/artifacts` 返回 3 个 seed 条目：

- `agent: cloud.agent.code-reviewer: 1.0.0`
- `skill: cloud.skill.release-notes: 1.0.0`
- `cli: cloud.cli.repo-health: 1.0.0`

`/marketplace` 返回 `200`。临时 registry 和官网 smoke 进程已关闭。

```powershell
corepack pnpm --dir ui exec tsc --noEmit -p tsconfig.json
```

结果：通过。

```powershell
go test ./pkg/marketregistry ./pkg/gateway -run "Market|Registry"
```

结果：通过。

Round 6 结论：

- 官网 Marketplace 已能通过同源 `/v1/artifacts` 读取 registry 并展示 agent / skill / cli。
- IP-only 阶段不需要浏览器直连 `:8791`；website 服务会代理 `/v1/*` 到内部 registry。
- 本轮未实现下载页真实 release 产物和一键下载按钮；这属于 Round 7。
- 下一轮为 Round 7：下载页和一键下载流程。必须等待 owner 说“继续”后再开始。

## Round 7：下载页和一键下载流程

目标：

- 用户能从官网明确下载或安装 AnyClaw。

交付物：

- 下载页。
- 平台识别展示。
- Windows / macOS / Linux 安装命令。
- GitHub Release 或本地静态产物链接方案。
- “一键复制安装命令”。
- 校验和说明。

涉及文件/模块：

- `site/` 或 `website/`
- `.github/workflows`
- `scripts/build-desktop.ps1`
- release artifacts 目录或 GitHub Releases。

验收标准：

- 用户从下载页能拿到明确安装方式。
- 下载链接不是死链。
- 至少 Windows 当前路径说明清楚。

测试方式：

- `curl -I` 下载链接。
- 页面点击 smoke test。
- 构建产物 checksum 校验。

风险点：

- 真正的一键下载安装涉及签名、公证、杀软误报等，第一版可以先做“复制命令 + release 下载”。

Round 7 交付记录：

- 下载页升级为“当前可用源码构建 + Release 准备中 + checksum 说明”的完整页面，避免在尚无 GitHub Release 时放置假下载链接。
- 下载页展示 Windows、macOS/Linux、Docker Compose Gateway 三条明确安装路径，并保留复制命令按钮。
- 下载页新增计划 release assets 列表：`windows_amd64.zip`、`linux_amd64.tar.gz`、`linux_arm64.tar.gz`、`darwin_amd64.tar.gz`、`darwin_arm64.tar.gz`。
- 下载页新增安装后 checklist，强调先跑 `onboard` 和 `doctor`。
- 新增 `scripts/build-release.ps1`，用于构建 `anyclaw` 与 `anyclaw-registry`，打包 zip/tar.gz，并生成 `.sha256`。
- 新增 `.github/workflows/release.yml`，在推送 `v*` tag 时构建多平台 release assets，并用 GitHub CLI 发布 Release。
- 更新 `.gitignore`，忽略本地 release staging 目录 `.release/`。

Round 7 验证记录：

```powershell
corepack pnpm --dir site build
```

结果：通过。

```powershell
powershell -NoLogo -NoProfile -ExecutionPolicy Bypass -File scripts\build-release.ps1 -Target windows-amd64 -Version round7-smoke -OutputDir tmp\round7-release
```

结果：通过。生成：

- `tmp/round7-release/anyclaw_round7-smoke_windows_amd64.zip`
- `tmp/round7-release/anyclaw_round7-smoke_windows_amd64.zip.sha256`

```powershell
Get-FileHash -Algorithm SHA256 tmp\round7-release\anyclaw_round7-smoke_windows_amd64.zip
```

结果：通过。实际 sha256 与 `.sha256` 文件一致。

```powershell
[System.IO.Compression.ZipFile]::OpenRead(...).Entries
```

结果：通过。zip 内包含：

- `anyclaw.exe`
- `anyclaw-registry.exe`
- `README.txt`

```powershell
corepack pnpm --dir ui exec tsc --noEmit -p tsconfig.json
go test ./pkg/marketregistry ./pkg/gateway -run "Market|Registry"
```

结果：均通过。

补充说明：

- 2026-05-07 核对 GitHub Releases 页面时，`https://github.com/1024XEngineer/anyclaw/releases` 当前没有公开 release。因此下载页没有放置 `.exe/.zip/.tar.gz` 的死链。
- 本轮生成的本地 smoke release 产物已清理。

Round 7 结论：

- 下载页现在能给用户明确、真实可用的安装路径。
- Release 自动产物流程已经准备好，但正式下载链接需要等推送第一个 `v*` tag 后才会出现。
- 本轮未处理包发布流程、publisher token、admin token 安全基线；这些属于 Round 8/9。
- 下一轮为 Round 8：agent / skill / cli 包发布流程。必须等待 owner 说“继续”后再开始。

## Round 8：agent / skill / cli 包发布流程

目标：

- 形成可重复的包发布流程，不靠手工乱传。

交付物：

- artifact manifest 模板。
- 打包脚本。
- 发布脚本。
- publisher token 使用说明。
- 发布前校验清单。
- 示例 agent / skill / cli 包。

涉及文件/模块：

- `scripts/`
- `pkg/marketregistry`
- `docs/MARKETPLACE_REGISTRY_DEV.md`
- `docs/PUBLISHING.md`

验收标准：

- 能用 publisher token 发布一个 skill。
- 官网和 AnyClaw cloud market 都能看到它。
- 下载 checksum 与 registry 返回一致。

测试方式：

```bash
curl -X POST /v1/admin/tokens
curl -X POST /v1/publish
curl /v1/artifacts
```

- AnyClaw UI 读取 cloud source。

风险点：

- 当前 publish 会由 LocalStorage 生成包，不是上传真实二进制包；如果要支持上传真实包，这是新增能力，需要 owner 确认。

Round 8 交付记录：

- 新增 `docs/PUBLISHING.md`，记录当前 publish API 契约、token 流程、manifest 必填字段、registry 生成包布局、smoke checklist 和当前限制。
- 新增三个示例 artifact manifest：
  - `examples/marketplace/skill-release-notes/anyclaw.artifact.json`
  - `examples/marketplace/agent-code-reviewer/anyclaw.artifact.json`
  - `examples/marketplace/cli-repo-health/anyclaw.artifact.json`
- 新增 `scripts/registry-create-publisher-token.ps1`，用 admin token 创建 publisher token。
- 新增 `scripts/registry-publish-artifact.ps1`，用 publisher token 发布 manifest。
- 更新 `docs/MARKETPLACE_REGISTRY_DEV.md`，加入脚本化 token 创建和 publish 示例，并链接到 `docs/PUBLISHING.md`。
- 更新官网 Publish 页面，展示可复制的 publisher token 创建命令和示例 skill 发布命令。

Round 8 验证记录：

```powershell
go build -o tmp\round8-publish\anyclaw-registry.exe ./cmd/anyclaw-registry
tmp\round8-publish\anyclaw-registry.exe serve --addr :18793 --data-dir tmp/round8-publish/registry-data --admin-token round8-local-admin-token --seed=true
.\scripts\registry-create-publisher-token.ps1 -BaseUrl http://127.0.0.1:18793 -AdminToken round8-local-admin-token -PublisherId "AnyClaw Labs"
.\scripts\registry-publish-artifact.ps1 -BaseUrl http://127.0.0.1:18793 -PublisherToken <publisher-token> -Manifest examples/marketplace/skill-release-notes/anyclaw.artifact.json
```

结果：通过。发布结果：

- publisher token 创建成功：`AnyClaw Labs`
- published：`cloud.skill.example-release-notes:1.0.0`
- `GET /v1/artifacts?q=cloud.skill.example-release-notes` 返回 `total=1`
- `POST /v1/artifacts/cloud.skill.example-release-notes/resolve` 返回 checksum
- `GET /v1/download/...` 下载包成功
- 下载包 SHA256 与 resolve 返回的 `checksum_sha256` 一致
- zip 内包含 `anyclaw.artifact.json`、`README.md`、`skill/SKILL.md`
- admin audit 返回 `total=2`

```powershell
corepack pnpm --dir site build
corepack pnpm --dir ui exec tsc --noEmit -p tsconfig.json
go test ./pkg/marketregistry ./pkg/gateway -run "Market|Registry"
```

结果：均通过。

补充说明：

- 本轮没有实现“上传发布者自带真实 zip 包”。当前 registry 的 publish 行为是接收 manifest 元数据，由 `LocalStorage` 生成 package 并计算 checksum。这是当前代码真实能力，不假装已经有对象存储上传。
- 本轮 smoke 生成的临时 registry 数据已清理。

Round 8 结论：

- agent / skill / cli 的 manifest 模板和脚本化发布流程已落地。
- publisher token 创建、发布、列表可见、resolve、download、checksum、audit 闭环已通过本地验证。
- 下一轮为 Round 9：admin token / publisher token / 安全基线。必须等待 owner 说“继续”后再开始。

## Round 9：admin token / publisher token / 安全基线

目标：

- 公网部署前把权限边界收紧。

交付物：

- 强制生产环境配置 admin token 的部署检查。
- publisher token 创建 / 轮换 / 吊销方案。
- Gateway API token 配置说明。
- 安全组配置说明。
- 最小暴露端口清单。

涉及文件/模块：

- `pkg/marketregistry/server.go`
- `.env.production.example`
- `docs/SECURITY.md`
- 部署脚本。

验收标准：

- admin API 无 token 返回 401。
- publisher publish 无 token 返回 401。
- Gateway 公网访问必须有 API token 或不暴露公网。
- 安全组只开放必要端口。

测试方式：

```bash
curl /v1/admin/audit
curl -H "Authorization: Bearer $ANYCLAW_REGISTRY_ADMIN_TOKEN" /v1/admin/audit
ss -tulpn
```

风险点：

- 如果要改“admin token 为空放行”的行为，会影响开发便利性，需要用环境变量区分 dev / prod。

Round 9 交付记录：

- `anyclaw-registry serve` 新增 `--require-admin-token`，也可由 `ANYCLAW_REGISTRY_REQUIRE_ADMIN_TOKEN` 控制。
- 生产 compose 增加 `--require-admin-token=${ANYCLAW_REGISTRY_REQUIRE_ADMIN_TOKEN:-true}`；`.env.production.example` 默认 `ANYCLAW_REGISTRY_REQUIRE_ADMIN_TOKEN=true`。
- `pkg/marketregistry.NewServer` 在 `RequireAdminToken=true` 且 admin token 为空时直接返回错误，避免公网 admin API 空 token 放行。
- 新增 publisher token revocation：
  - `POST /v1/admin/tokens/{id}/revoke`
  - `Store.RevokePublisherToken`
  - `PublisherTokenRevocation`
  - audit event：`publisher_token.revoked`
- 新增 `scripts/registry-revoke-publisher-token.ps1`。
- 更新 `scripts/registry-create-publisher-token.ps1`、`scripts/registry-publish-artifact.ps1`、`scripts/registry-revoke-publisher-token.ps1`，使 `BaseUrl` 可传站点 origin、`/v1` API base 或直连 registry。
- 新增 `docs/SECURITY.md`，明确 admin token、publisher token、Gateway token、端口、安全组和 first launch rules。
- 更新 `docs/REGISTRY_PRODUCTION_RUNBOOK.md` 和 `docs/PUBLISHING.md`，加入 token revoke/rotate 说明。

Round 9 验证记录：

```powershell
go test ./pkg/marketregistry ./pkg/gateway -run "Market|Registry|Admin|Token|Publish|Quarantine"
go test ./cmd/anyclaw-registry ./pkg/marketregistry
```

结果：通过。

```powershell
corepack pnpm --dir site build
corepack pnpm --dir ui exec tsc --noEmit -p tsconfig.json
```

结果：通过。

```powershell
$env:ANYCLAW_REGISTRY_ADMIN_TOKEN='round9-config-check'
$env:ANYCLAW_REGISTRY_REQUIRE_ADMIN_TOKEN='true'
docker compose -f docker-compose.prod.yml config
```

结果：通过。Compose 正确展开 `--require-admin-token=true`。

进程级 smoke：

```powershell
anyclaw-registry serve --addr :18794 --require-admin-token=true --seed=false
```

结果：按预期启动失败，错误包含 `admin token is required`。

```powershell
anyclaw-registry serve --addr :18795 --admin-token round9-local-admin-token --require-admin-token=true --seed=true
Invoke-RestMethod http://127.0.0.1:18795/v1/admin/audit
Invoke-RestMethod -Method Post http://127.0.0.1:18795/v1/publish -Body '{}'
```

结果：无 token 访问 admin audit 返回 `401`；无 publisher token publish 返回 `401`。

```powershell
.\scripts\registry-create-publisher-token.ps1 ...
.\scripts\registry-revoke-publisher-token.ps1 ...
.\scripts\registry-publish-artifact.ps1 ... # using revoked token
```

结果：token 创建成功，revoke 成功，revoke 后再 publish 返回 `401`。

脚本 URL 兼容 smoke：

- `-BaseUrl http://127.0.0.1:<port>` 创建 token 成功。
- `-BaseUrl http://127.0.0.1:<port>/v1` revoke token 成功。

补充说明：

- 开发模式仍可不传 `--require-admin-token`，用于本地临时 registry；生产 compose 默认启用强制检查。
- Gateway 仍保留端口 `18789`，但安全文档要求首发不要在安全组公开它；若公开必须设置 `ANYCLAW_API_TOKEN`。
- 本轮 smoke 临时数据已清理。

Round 9 结论：

- registry admin API、publisher publish API 和 publisher token revoke 已形成安全基线。
- 生产 compose 已防止空 admin token 启动。
- 安全组和端口策略已文档化。
- 下一轮为 Round 10：Registry 数据备份与恢复。必须等待 owner 说“继续”后再开始。

## Round 10：Registry 数据备份与恢复

目标：

- 避免 registry 数据丢失。

交付物：

- SQLite 备份脚本。
- packages 目录备份脚本。
- 恢复脚本。
- 定时任务说明。
- 备份保留策略。

涉及文件/模块：

- `.anyclaw-registry/registry.db`
- `.anyclaw-registry/packages/`
- `deploy/backup-registry.sh`
- `docs/DEPLOYMENT.md`

验收标准：

- 能从备份恢复 registry.db 和 packages。
- 恢复后 `/v1/artifacts`、download 正常。
- 备份文件带时间戳。

测试方式：

```bash
docker compose exec registry ...
sqlite3 .backup
tar -tzf backup.tar.gz
```

- 恢复到临时目录后 smoke test。

风险点：

- SQLite 在线备份不能直接粗暴复制热文件，优先用 SQLite backup 或短暂停服备份。
- 40GB 系统盘空间有限，要设置保留数量。

Round 10 交付记录：

- 新增 `scripts/registry-backup.ps1`，本地/Windows 备份 registry 数据目录。
- 新增 `scripts/registry-restore.ps1`，本地/Windows 从备份恢复到目标数据目录。
- 新增 `deploy/registry-backup.sh`，服务器/Linux 备份脚本。
- 新增 `deploy/registry-restore.sh`，服务器/Linux 恢复脚本。
- 新增 `docs/REGISTRY_BACKUP_RESTORE.md`，明确备份对象、在线备份注意事项、恢复 smoke、保留策略。
- 更新 `docs/REGISTRY_PRODUCTION_RUNBOOK.md`，加入备份/恢复命令。
- 更新 `deploy/README.md`，加入 registry backup 入口。

备份范围：

- `registry.db`
- `packages/`
- `audit/`
- `manifest.json`

说明：

- 如果系统有 `sqlite3`，脚本使用 SQLite `.backup`。
- 如果没有 `sqlite3`，脚本回退到文件复制；生产建议安装 `sqlite3` 或短暂停服备份。
- 必须把 `registry.db` 和 `packages/` 一起备份，避免恢复后列表可见但下载文件缺失。

Round 10 验证记录：

```powershell
go build -o tmp\round10-backup\anyclaw-registry.exe ./cmd/anyclaw-registry
anyclaw-registry serve --addr :18797 --data-dir tmp/round10-backup/source-data --admin-token round10-admin-token --require-admin-token=true --seed=true
.\scripts\registry-create-publisher-token.ps1 ...
.\scripts\registry-publish-artifact.ps1 ... examples/marketplace/skill-release-notes/anyclaw.artifact.json
.\scripts\registry-backup.ps1 -DataDir tmp/round10-backup/source-data -OutputDir tmp/round10-backup/backups -Name smoke
.\scripts\registry-restore.ps1 -BackupDir tmp/round10-backup/backups/smoke -TargetDataDir tmp/round10-backup/restore-data -Force
anyclaw-registry serve --addr :18798 --data-dir tmp/round10-backup/restore-data --admin-token round10-admin-token --require-admin-token=true --seed=false
```

结果：通过。

恢复后验证：

- `GET /v1/artifacts?q=cloud.skill.example-release-notes` 返回 `total=1`
- `POST /v1/artifacts/cloud.skill.example-release-notes/resolve` 返回 checksum
- `GET /v1/download/...` 下载成功
- 下载文件 SHA256 与 resolve 返回的 `checksum_sha256` 一致

```powershell
go test ./pkg/marketregistry ./pkg/gateway -run "Market|Registry|Admin|Token|Publish|Quarantine"
corepack pnpm --dir site build
corepack pnpm --dir ui exec tsc --noEmit -p tsconfig.json
```

结果：均通过。

补充说明：

- 本轮 smoke 临时备份和恢复目录已清理。
- 未新增数据库或对象存储依赖；Postgres/OSS/S3 仍属于后续增强。

Round 10 结论：

- Registry 首版备份与恢复流程已落地并通过 dry run。
- 恢复后的 registry 能正常 list / resolve / download，并通过 checksum 校验。
- 下一轮为 Round 11：服务器上线，IP 方案先跑通。必须等待 owner 说“继续”后再开始。

## Round 11：服务器上线，IP 方案先跑通

目标：

- 在阿里云香港 ECS 上不用域名先上线。

交付物：

- 服务器目录结构。
- `.env.production`。
- compose 启动。
- IP 访问官网。
- IP 访问 registry。
- smoke test 记录。

涉及文件/模块：

- `deploy/`
- `docker-compose.prod.yml`
- 服务器 `/opt/anyclaw/`

验收标准：

- `http://SERVER_IP` 打开官网。
- `http://SERVER_IP/v1/artifacts` 或临时 `http://SERVER_IP:8791/v1/artifacts` 可访问 registry。
- AnyClaw 本地配置 `ANYCLAW_MARKETPLACE_ENDPOINT=http://SERVER_IP/...` 后能看到 cloud artifacts。

测试方式：

```bash
docker compose ps
curl http://SERVER_IP
curl http://SERVER_IP/v1/health
curl http://SERVER_IP/v1/artifacts
```

- AnyClaw Market UI cloud tab smoke test。

风险点：

- 无域名无法做常规 HTTPS。
- 若直接开放 `8791`，安全组和 admin token 必须正确。

Round 11 预备交付记录：

- 更新 `docker-compose.prod.yml` 为 IP-only 首发安全形态：
  - 官网公开 `80:8080`
  - registry 仅 `expose: 8791`，不发布公网端口
  - Gateway 仅绑定 `127.0.0.1:18789:18789`
- 更新 `.env.production.example`，说明 registry 通过官网 `/v1/*` 同源代理访问，Gateway 默认不公网开放。
- 新增 `docs/SERVER_IP_DEPLOYMENT.md`，记录阿里云 ECS IP-only 部署步骤、安全组、服务器目录、环境变量、启动和 smoke。
- 新增 `deploy/server-ip-smoke.sh`，用于验证 `http://SERVER_IP/`、`/v1/health`、`/v1/artifacts` 和 admin 401/token 访问。
- 更新 `deploy/README.md`，加入 Round 11 IP-only 部署入口。

Round 11 本地 preflight 验证记录：

```powershell
corepack pnpm --dir site build
```

结果：通过。`site/dist` 已生成。

```powershell
$env:ANYCLAW_REGISTRY_ADMIN_TOKEN='round11-config-check'
$env:ANYCLAW_REGISTRY_REQUIRE_ADMIN_TOKEN='true'
docker compose -f docker-compose.prod.yml config
```

结果：通过。端口形态符合首发安全基线：

- `published: "80"`
- registry 使用 `expose`
- Gateway 出现 `host_ip: 127.0.0.1`
- 未出现 `published: "8791"`

```powershell
go test ./pkg/marketregistry ./pkg/gateway -run "Market|Registry|Admin|Token|Publish|Quarantine"
```

结果：通过。

Round 11 服务器部署记录：

- ECS：`47.76.186.80`
- SSH 用户：`root`
- SSH key：owner 本机 `D:\11\Pictures\useful\bin.pem`
- 服务器目录：`/opt/anyclaw`
- Docker：原服务器未安装 `docker` 命令；本轮使用 Ubuntu/阿里云 apt 源安装 `docker.io` 与 `docker-compose-v2`。
- `.env.production`：已在服务器生成，权限 `600`；`ANYCLAW_REGISTRY_ADMIN_TOKEN` 由服务器 `openssl rand -hex 32` 生成，未写入仓库。
- Compose 服务：`anyclaw-registry`、`anyclaw-website`、`anyclaw-gateway` 已启动。
- 公网端口形态：
  - website 监听 `0.0.0.0:80->8080`
  - registry 未发布公网 `8791`
  - Gateway 仅监听 `127.0.0.1:18789`

Round 11 服务器验证记录：

```powershell
corepack pnpm --dir site build
go test ./pkg/marketregistry ./pkg/gateway -run "Market|Registry|Admin|Token|Publish|Quarantine"
```

结果：通过。

```bash
docker --version
docker compose version
```

结果：通过。服务器为 Docker `29.1.3`，Docker Compose `2.40.3`。

```bash
docker compose --env-file .env.production -f docker-compose.prod.yml ps
```

结果：通过。三个容器均已启动，registry/website/gateway 均 healthy。

```bash
SERVER_ORIGIN=http://127.0.0.1 ./deploy/server-ip-smoke.sh
```

结果：通过。

- `GET http://127.0.0.1/`：200
- `GET http://127.0.0.1/v1/health`：200
- `GET http://127.0.0.1/v1/artifacts`：200
- `GET http://127.0.0.1/v1/admin/audit` 无 token：401
- `GET http://127.0.0.1/v1/admin/audit` 携带 admin token：200

Round 11 公网验收记录：

owner 已在阿里云 ECS 安全组入方向放行 `TCP 80`。复测结果：

```powershell
curl.exe --noproxy "*" -I --connect-timeout 15 http://47.76.186.80/
curl.exe --noproxy "*" -sS --connect-timeout 15 http://47.76.186.80/v1/health
curl.exe --noproxy "*" -sS --connect-timeout 15 http://47.76.186.80/v1/artifacts
```

结果：通过。

- `GET http://47.76.186.80/`：200
- `GET http://47.76.186.80/v1/health`：200，返回 registry `status=ok`
- `GET http://47.76.186.80/v1/artifacts`：200，返回 3 个 seed artifact

```bash
SERVER_ORIGIN=http://47.76.186.80 ./deploy/server-ip-smoke.sh
```

结果：通过。

- website：ok
- registry health through website proxy：ok
- registry artifacts through website proxy：ok
- admin audit without token：401

Round 11 已完成。`8791` 和 `18789` 继续保持不开放公网。

Round 11 补充修正记录：

- 首次打包上传时使用了 `--exclude=dist`，误把 `site/dist` 排除，导致公网首页显示 Python 静态服务器的目录列表。
- 已重新上传 `site/dist/index.html` 和 `site/dist/assets/*` 到服务器 `/opt/anyclaw/site/dist/`，并重启 `anyclaw-website`。
- 复测 `http://47.76.186.80/` 和 `http://47.76.186.80/marketplace` 均返回 React SPA 的 `index.html`，`http://47.76.186.80/v1/artifacts` 正常返回 registry 数据。
- 后续再次打包部署时，不能使用会匹配子目录的 `--exclude=dist`；如需排除根目录构建缓存，应改为只排除 `./dist`，并显式保留 `site/dist`。

## Round 11.5：1Panel 可视化运维面板

目标：

- 给服务器增加网页可视化管理入口，减少 owner 对命令行的依赖。

交付物：

- 1Panel 服务。
- 面板访问地址。
- 基础安全说明。
- AnyClaw 服务不受影响的验证记录。

涉及文件/模块：

- 服务器 `/opt/1panel`
- 系统服务 `1panel-core`
- 系统服务 `1panel-agent`
- `1pctl`
- 阿里云 ECS 安全组

验收标准：

- 1Panel core/agent 服务 active。
- 服务器本机可访问面板端口。
- AnyClaw 官网、registry、gateway 容器继续 healthy。
- 公网访问面板前，阿里云安全组只放行必要面板端口。

测试方式：

```bash
1pctl user-info
systemctl is-active 1panel-core
systemctl is-active 1panel-agent
curl -I http://127.0.0.1:<panel-port>/<security-entrance>
docker compose --env-file .env.production -f docker-compose.prod.yml ps
curl http://127.0.0.1/v1/health
```

风险点：

- 1Panel 是新增公网管理面板，必须使用强密码和安全入口。
- 面板端口不应长期对全网开放；建议阿里云安全组只允许 owner 当前公网 IP 访问。
- 不要在 1Panel 中随意删除 AnyClaw 容器、volume、镜像或 `/opt/anyclaw` 文件。

Round 11.5 交付记录：

- 已安装 1Panel。安装来源为 1Panel 官方在线安装包。
- 1Panel 安装器自动生成面板端口：`35302`。
- 1Panel 安装器自动生成安全入口、用户名和密码；敏感信息不写入仓库文档，可通过服务器 `1pctl user-info` 查看。
- `1panel-core` 和 `1panel-agent` 均为 active。
- 服务器本机 `curl -I http://127.0.0.1:35302/<security-entrance>` 返回 200。
- AnyClaw 服务复测通过：`anyclaw-registry`、`anyclaw-website`、`anyclaw-gateway` 继续 healthy，`GET http://127.0.0.1/v1/health` 返回 registry `status=ok`。

当前阻塞：

无。

Round 11.5 公网验收记录：

owner 已在阿里云 ECS 安全组入方向放行 `35302/tcp`。复测结果：

```powershell
curl.exe --noproxy "*" -I --connect-timeout 15 http://47.76.186.80:35302/<security-entrance>
```

结果：通过，返回 `HTTP/1.1 200 OK`。

```bash
systemctl is-active 1panel-core
systemctl is-active 1panel-agent
docker compose --env-file .env.production -f docker-compose.prod.yml ps
curl -fsS http://127.0.0.1/v1/health
```

结果：通过。

- `1panel-core`：active
- `1panel-agent`：active
- `anyclaw-registry`、`anyclaw-website`、`anyclaw-gateway`：healthy
- registry health：`status=ok`

## Round 12：当前线上市场修复与基线清理

目标：

- 先把当前线上市场修到稳定，避免“API 有数据但页面空白”的假完成状态。

交付物：

- Marketplace 页面稳定显示数据。
- `/v1/artifacts` live 状态正确或 fallback 状态清晰。
- 中文乱码清理清单。
- `.gitignore` / 部署产物边界清理。
- 提交前文件清单。

涉及文件/模块：

- `site/src/App.tsx`
- `site/src/registryClient.ts`
- `site/src/data.ts`
- `deploy/static-spa-server.py`
- `.gitignore`
- `docs/WEBSITE_MARKETPLACE_DEPLOYMENT_PLAN.md`

验收标准：

- `http://47.76.186.80/marketplace` 能显示 agent / skill / cli 卡片。
- `http://47.76.186.80/v1/artifacts` 返回 `total > 0`。
- Marketplace 不无限停留在骨架屏。
- 页面中文不出现明显乱码。
- 本地 `tmp/`、构建产物、密钥类文件不会进入 GitHub。

测试方式：

```powershell
corepack pnpm --dir site build
D:\python\python.exe -m py_compile deploy\static-spa-server.py
curl.exe --noproxy "*" http://47.76.186.80/v1/artifacts
```

- Chrome 桌面截图验证 `/marketplace`。
- `docker compose ps` 验证服务器容器状态。

风险点：

- 当前部分官网源码出现中文乱码，需要谨慎修复，避免扩大范围。
- 当前 registry 只有 seed 示例数据，不等于正式市场数据。

Round 12 交付记录：

- 更新总执行计划：原 Round 12/13 改为新的 Round 12-21，HTTPS 顺延到 Round 20。
- 修复 `deploy/static-spa-server.py` 的 `/v1/*` 代理响应，代理会读取完整上游 body，并返回 `Content-Length` 与 `Connection: close`，避免浏览器 fetch 长时间 pending。
- 更新 `site/src/App.tsx`：
  - Marketplace 初始显示本地示例卡片，避免页面空白。
  - registry 请求超时后进入 fallback 状态。
  - loading 且已有卡片时显示“正在刷新 Registry，先展示当前缓存/示例”，不再误导为完全空白加载。
- 更新 `.gitignore`：
  - 忽略 `tmp/`
  - 忽略 `site/dist/`
- 已重新构建并部署官网到服务器 `/opt/anyclaw/site/dist/`。

Round 12 验证记录：

```powershell
corepack pnpm --dir site build
D:\python\python.exe -m py_compile deploy\static-spa-server.py
```

结果：通过。

```powershell
curl.exe --noproxy "*" http://47.76.186.80/v1/artifacts
```

结果：通过，返回 `total=3`，当前 seed artifact 为：

- `Cloud Code Reviewer`
- `Release Notes Writer`
- `Repo Health CLI`

```bash
docker compose --env-file .env.production -f docker-compose.prod.yml ps
curl -fsS -D - http://127.0.0.1/v1/artifacts -o /tmp/round12-artifacts.json
```

结果：通过。三个 AnyClaw 容器继续运行；`/v1/artifacts` 返回 `Content-Length` 和 `Connection: close`。

浏览器截图验证：

- `http://47.76.186.80/marketplace` 已显示 3 张 agent / skill / cli 卡片。
- 页面不再无限停留在空白骨架屏。

Round 12 结论：

- 当前线上 Marketplace 可见层已修复到“不会空白”的基线状态。
- 目前显示的仍是 seed / fallback 示例数据，不是正式市场数据。
- 下一轮 Round 13 必须发布真实 agent / skill / cli 条目。

## Round 13：发布真实 agent / skill / cli 数据

目标：

- 把 seed 示例替换为真实市场条目。

交付物：

- 至少 1 个真实 agent。
- 至少 1 个真实 skill。
- 至少 1 个真实 cli。
- 正式 `anyclaw.artifact.json`。
- 发布脚本验证记录。
- 下载和 checksum 验证记录。

涉及文件/模块：

- `examples/marketplace/`
- `scripts/registry-create-publisher-token.ps1`
- `scripts/registry-publish-artifact.ps1`
- `pkg/marketregistry/`

验收标准：

- `/v1/artifacts` 显示真实条目。
- `/v1/artifacts/{id}` 正常。
- `/v1/artifacts/{id}/versions` 正常。
- `/v1/download/...` 可下载。
- checksum 一致。

测试方式：

```powershell
.\scripts\registry-create-publisher-token.ps1 ...
.\scripts\registry-publish-artifact.ps1 ...
curl.exe --noproxy "*" http://47.76.186.80/v1/artifacts
```

风险点：

- 当前 registry publish 仍是生成包，不是真实对象存储上传；OSS/S3 属于后续增强，需要单独确认。

Round 13 交付记录：

- 新增真实 marketplace manifest：
  - `examples/marketplace/agent-marketplace-operator/anyclaw.artifact.json`
  - `examples/marketplace/skill-skill-author/anyclaw.artifact.json`
  - `examples/marketplace/cli-agent-native-runner/anyclaw.artifact.json`
- 发布到线上 registry：
  - `anyclaw.agent.marketplace-operator`
  - `anyclaw.skill.skill-author`
  - `anyclaw.cli.agent-native-runner`
- 条目方向参考：
  - skill 参考 ClawHub 一类公开技能市场的目录化、可搜索、可发布产品形态。
  - cli 参考 CLI-Anything 的 agent-native CLI / skill 组织方向。
  - 只参考产品方向，没有复制第三方内容，也没有新增依赖。
- 为避免 Windows PowerShell 发布链路造成中文 `hit_signals` 编码损坏，本轮将检索关键词统一为 ASCII 英文；中文搜索体验后续需要用单独编码测试闭环处理。
- 本轮创建的临时 publisher token 已全部吊销，避免发布测试 token 长期留在服务器。

Round 13 验证记录：

```powershell
Get-Content -Raw -Encoding UTF8 examples\marketplace\*\anyclaw.artifact.json | ConvertFrom-Json
```

结果：通过。三个 manifest 均包含 `artifact.id`、`kind`、`latest_version` 和 1 个版本。

```powershell
.\scripts\registry-create-publisher-token.ps1 -BaseUrl http://47.76.186.80 ...
.\scripts\registry-publish-artifact.ps1 -BaseUrl http://47.76.186.80 ...
```

结果：通过。三类真实条目均发布到线上 registry。

```powershell
Invoke-RestMethod http://47.76.186.80/v1/artifacts?q=anyclaw
```

结果：通过。返回 `total=6`，其中 3 个为本轮新增真实条目。

```powershell
GET /v1/artifacts/{id}
GET /v1/artifacts/{id}/versions
POST /v1/artifacts/{id}/resolve
GET /v1/download/{id}/1.0.0
Get-FileHash -Algorithm SHA256
```

结果：通过。下载包 checksum 与 registry 返回值一致：

- `anyclaw.agent.marketplace-operator`：1200 bytes，SHA256 前缀 `84cc163a9649`
- `anyclaw.skill.skill-author`：1329 bytes，SHA256 前缀 `4fe75cd7ff86`
- `anyclaw.cli.agent-native-runner`：1182 bytes，SHA256 前缀 `5ab9d4dfa852`

```powershell
Invoke-RestMethod http://47.76.186.80/v1/admin/audit?limit=12 -Headers @{ Authorization = "Bearer <admin-token>" }
Invoke-RestMethod http://47.76.186.80/v1/admin/downloads -Headers @{ Authorization = "Bearer <admin-token>" }
```

结果：通过。审计中出现 `artifact.published`；下载统计中三个新条目均有下载计数。

```powershell
corepack pnpm --dir site build
Invoke-WebRequest http://47.76.186.80/marketplace
docker compose --env-file .env.production -f docker-compose.prod.yml ps
```

结果：通过。官网构建通过，`/marketplace` 返回 200，服务器 `anyclaw-registry`、`anyclaw-website`、`anyclaw-gateway` 均为 running / healthy。

Round 13 结论：

- 线上云端市场已经不再是空白，也不只是 seed 示例；已具备真实 agent / skill / cli 数据。
- 当前发布包仍由 registry 根据 metadata 生成，不是上传真实 zip 包；真实包上传、对象存储、签名策略属于后续增强，不能假装已完成。
- 下一轮先进入 Round 13.5，修复官网 / 桌面壳端显示和清理旧 seed；完成后 Round 14 再进入客户端安装闭环验收。

## Round 13.5：Web + 桌面壳云端市场显示修复与 seed 清理

目标：

- 官网 marketplace 只显示真实 registry 数据，不再被 seed / fallback 示例污染。
- 桌面壳端 Cloud tab 能显示并点开 `anyclaw.*` 真实条目。
- 线上 registry 删除 3 个旧 seed 示例。
- 生产配置关闭 seed 自动注入。

交付物：

- 桌面壳端 cloud source 详情 / 版本识别修复。
- Registry admin delete artifact 能力，或等价的安全清理能力。
- 线上删除旧 seed：
  - `cloud.agent.code-reviewer`
  - `cloud.skill.release-notes`
  - `cloud.cli.repo-health`
- 生产 `.env.production` 配置 `ANYCLAW_REGISTRY_SEED=false`。
- 官网和桌面壳 API 验证记录。

涉及文件/模块：

- `pkg/gateway/gateway_market_artifacts_api.go`
- `pkg/gateway/gateway_market_artifacts_cloud_test.go`
- `pkg/marketregistry/server.go`
- `pkg/marketregistry/store.go`
- `pkg/marketregistry/server_test.go`
- `docker-compose.prod.yml`
- `.env.production.example`
- `docs/WEBSITE_MARKETPLACE_DEPLOYMENT_PLAN.md`

验收标准：

- `GET http://47.76.186.80/v1/artifacts?limit=100` 只返回 3 个 `anyclaw.*` 真实条目。
- 官网 `http://47.76.186.80/marketplace` 显示真实 agent / skill / cli。
- 桌面壳端 `/market/artifacts?source=cloud&kind=agent|skill|cli` 能分别返回真实条目。
- `/market/artifacts/anyclaw...` 和 `/market/artifacts/anyclaw.../versions` 能正常返回。
- 生产 registry 不再自动 seed 新演示数据。

测试方式：

```powershell
go test ./pkg/marketregistry ./pkg/gateway -run "Market|Registry|Cloud|Delete"
corepack pnpm --dir site build
corepack pnpm --dir ui test -- --run useMarketDirectory
Invoke-RestMethod http://47.76.186.80/v1/artifacts?limit=100
Invoke-RestMethod http://127.0.0.1:18789/market/artifacts?source=cloud...
docker compose --env-file .env.production -f docker-compose.prod.yml ps
```

风险点：

- 物理删除 seed 会删除对应版本元数据；这是本轮明确目标，但操作必须只针对 3 个旧 seed id。
- 如果删除 API 权限处理错误，会扩大 admin 接口风险；必须用 admin token 保护，并验证无 token 401。
- 桌面壳端以前用 `cloud.` 前缀判断云端条目，本轮改为“按 Cloud 列表来源和本地不存在时走云端”，需要更新测试以覆盖 `anyclaw.*`。

Round 13.5 状态：

- complete

Round 13.5 交付记录：

- 修复桌面壳端云端详情识别：
  - 以前只把 `cloud.*` id 当作云端 artifact。
  - 现在 `source=cloud` 的详情 / 版本请求会直接走云端 registry；没有 `source=cloud` 时，如果本地目录找不到且云端 registry 已配置，也会尝试云端。
  - `anyclaw.*` 真实条目可以正常在 Cloud tab 点开详情和版本。
- 新增 registry admin 删除能力：
  - `DELETE /v1/admin/artifacts/{id}`
  - 必须携带 admin token。
  - 删除 artifact、versions、quarantine 记录，并写入 `artifact.deleted` audit。
- 删除线上旧 seed：
  - `cloud.agent.code-reviewer`
  - `cloud.skill.release-notes`
  - `cloud.cli.repo-health`
- 关闭生产 seed：
  - 服务器 `/opt/anyclaw/.env.production` 已改为 `ANYCLAW_REGISTRY_SEED=false`。
  - `.env.production.example` 默认值也改为 `false`。
- 官网 marketplace 不再使用静态 market fallback 假数据：
  - 删除 `site/src/data.ts` 中的 `marketItems` 演示数据。
  - registry 请求失败时显示错误 / 空态，不再展示静态示例冒充市场数据。
- 修复桌面壳 / 控制台 UI 入口：
  - `ui/src/features/market/useMarketDirectory.ts` 默认 source 改为 `cloud`，避免首次进入 Market 仍显示 Local / roadmap 预留信息。
  - `ui/src/features/market/MarketSidebar.tsx` 和 `ui/src/pages/Market/MarketPage.tsx` 将 Cloud 放在更优先的位置，并显示 Cloud entries 计数。
  - 2026-05-08 返工修正：`cmd/anyclaw-desktop/app.go` 的 Dashboard URL 已恢复为默认打开 `#/`，让 Wails 桌面壳启动后进入对话首页，不再直接进入云端市场。
  - 2026-05-08 返工修正：桌面壳启动时如果配置文件和环境变量都没有显式 marketplace endpoint，会为本地 Gateway 注入公开 registry endpoint；如果用户已配置 endpoint 或禁用 remote，则不覆盖用户配置。
  - 2026-05-08 返工修正：如果桌面壳发现 `127.0.0.1:18789` 已有旧 Gateway 且 Cloud Market 明确返回 `cloud registry endpoint is not configured`，新桌面壳会改用临时空闲端口启动自己的 Gateway，避免继续挂到旧的无 endpoint 进程。
  - 2026-05-08 返工修正：`ui/src/features/market/useMarketDirectory.ts` 的详情 / 版本请求会显式携带当前 `source`，Cloud 列表点开 `anyclaw.*` 条目时不再依赖“本地找不到再猜云端”的兜底逻辑。
  - 2026-05-08 返工修正：控制台 bundle 中可见的旧工作区云端预留文案已替换为当前真实三条云端市场条目说明，避免继续出现“云端 Agent Catalog / 统一连接中心 / 尚未接入”等旧占位信息。
  - 2026-05-08 返工修正：控制台 `/dashboard` index 和 assets 增加 `Cache-Control: no-store, max-age=0` / `Pragma: no-cache` / `Expires: 0`，桌面壳 Dashboard URL 增加 `?v=` cache-buster，降低浏览器和 WebView 继续使用旧 bundle 的概率。
  - `deploy/static-spa-server.py` 对 SPA index 增加 `Cache-Control: no-store`，减少浏览器继续使用旧官网页面。
- 已重建并部署生产镜像，重新上传官网 `site/dist`。
- 已重建本机 Wails 桌面壳：`cmd/anyclaw-desktop/build/bin/anyclaw-desktop.exe`。

Round 13.5 验证记录：

```powershell
go test ./pkg/gateway -run "TestMarketArtifactsCloudUsesRegistryEndpoint|TestMarketArtifactCloudDetailAndVersions|TestMarketArtifactCloudDetailFallsBackToCloudForUnknownNonCloudPrefix|TestMarketArtifactsCloudUnavailableDegradesToEmptyList"
go test ./pkg/marketregistry -run "TestServerAdminDeleteArtifact|TestServerAdminTokenPublishQuarantineAndStats|TestServerSeededCatalogRoutes"
go test ./pkg/marketplace -run "Install|Bind|Upgrade|Uninstall|Market"
corepack pnpm --dir site build
corepack pnpm --dir ui test -- --run useMarketDirectory
D:\python\python.exe -m py_compile deploy\static-spa-server.py
go test ./cmd/anyclaw-desktop -run "ControlUI|Dashboard|Launch|URL"
.\scripts\build-desktop.ps1
```

结果：通过。

2026-05-08 返工验证记录：

```powershell
corepack pnpm --dir ui test -- src/features/market/useMarketDirectory.test.tsx src/features/workspace/useWorkspaceOverview.test.ts src/features/settings/SettingsModal.test.tsx
go test ./cmd/anyclaw-desktop -run "ControlUI|Dashboard|Launch|URL|MarketplaceEndpoint|CloudEndpointMissing"
go test ./pkg/gateway -run "TestMarketArtifactsCloudUsesRegistryEndpoint|TestMarketArtifactCloudDetailAndVersions|TestMarketArtifactCloudDetailFallsBackToCloudForUnknownNonCloudPrefix|TestMarketArtifactsCloudUnavailableDegradesToEmptyList"
corepack pnpm --dir ui build
corepack pnpm --dir site build
.\scripts\build-desktop.ps1
```

结果：通过。`dist/control-ui` 重新构建后，未再匹配到 `云端 Agent Catalog`、`统一连接中心`、`云端 Skill 和云端 Agent 目前尚未接入` 等旧可见占位文案；桌面壳产物已重建到 `cmd/anyclaw-desktop/build/bin/anyclaw-desktop.exe`。

```powershell
Invoke-RestMethod 'http://47.76.186.80/v1/artifacts?limit=100'
```

结果：通过。线上 registry 当前仍只返回 3 个真实 `anyclaw.*` 条目。

补充说明：

```powershell
go test ./pkg/marketregistry ./pkg/gateway -run "Market|Registry|Cloud|Delete"
```

结果：`pkg/marketregistry` 通过；`pkg/gateway` 中既有 `TestMarketUpgradeRefreshesExistingBindings` 在 Windows 临时路径下失败，失败点是 workspace runtime refresh 计数，不是本轮 cloud 识别 / delete 改动。已用更聚焦的本轮相关测试验证通过。

```powershell
Invoke-RestMethod http://47.76.186.80/v1/artifacts?limit=100
```

结果：通过。线上 registry 当前只返回 3 个真实条目：

- `anyclaw.agent.marketplace-operator`
- `anyclaw.skill.skill-author`
- `anyclaw.cli.agent-native-runner`

```powershell
Invoke-RestMethod http://47.76.186.80/v1/admin/audit?limit=8 -Headers @{ Authorization = "Bearer <admin-token>" }
```

结果：通过。审计中有 3 条 `artifact.deleted`：

- `cloud.agent.code-reviewer`
- `cloud.skill.release-notes`
- `cloud.cli.repo-health`

```bash
curl -fsS 'http://127.0.0.1:18789/market/artifacts?source=cloud&kind=agent&limit=100'
curl -fsS 'http://127.0.0.1:18789/market/artifacts?source=cloud&kind=skill&limit=100'
curl -fsS 'http://127.0.0.1:18789/market/artifacts?source=cloud&kind=cli&limit=100'
curl -fsS 'http://127.0.0.1:18789/market/artifacts/anyclaw.agent.marketplace-operator?source=cloud'
curl -fsS 'http://127.0.0.1:18789/market/artifacts/anyclaw.agent.marketplace-operator/versions?source=cloud'
```

结果：通过。桌面壳端 Gateway API 能分别返回真实 agent / skill / cli，`anyclaw.*` 详情和版本正常。

```bash
docker compose --env-file .env.production -f docker-compose.prod.yml ps
```

结果：通过。`anyclaw-registry`、`anyclaw-website`、`anyclaw-gateway` 均为 running / healthy。

```powershell
Invoke-WebRequest http://47.76.186.80/marketplace
```

结果：通过，公网官网 marketplace 返回 200，并带 `Cache-Control: no-store`。

```powershell
Invoke-RestMethod 'http://127.0.0.1:18789/market/artifacts?source=cloud&kind=agent&limit=100'
Invoke-RestMethod 'http://127.0.0.1:18789/market/artifacts?source=cloud&kind=skill&limit=100'
Invoke-RestMethod 'http://127.0.0.1:18789/market/artifacts?source=cloud&kind=cli&limit=100'
```

结果：通过。当前运行的本机桌面壳 Gateway `127.0.0.1:18789` 分别返回：

- `anyclaw.agent.marketplace-operator` / `Marketplace Operator`
- `anyclaw.skill.skill-author` / `Skill Author`
- `anyclaw.cli.agent-native-runner` / `Agent Native Runner`

```powershell
Invoke-WebRequest 'http://127.0.0.1:18789/dashboard?v=sanity#/market?source=cloud'
```

结果：通过。控制台 index 返回 `Cache-Control: no-store, max-age=0`、`Pragma: no-cache`、`Expires: 0`。

```text
Chrome DevTools Protocol verification:
- http://127.0.0.1:18789/dashboard?v=cdp-market3#/market?source=cloud
- document.body.innerText
```

结果：通过。真实 Chromium 渲染后的 Market DOM 中出现 `Marketplace Operator`、`Cloud Agent Directory`、`Available`；未出现 `云端预留`、`云端 Agent Catalog`、`统一连接中心`、`云端 Skill 和云端 Agent 目前尚未接入`。

```text
Chrome DevTools Protocol verification:
- http://127.0.0.1:18789/dashboard?v=cdp-chat-final#/
- document.body.innerText
```

结果：通过。真实 Chromium 渲染后的默认 Dashboard DOM 中出现 `对话`、`新对话`；未出现 `Marketplace Operator` 和旧云端预留文案，确认 web / 桌面壳默认入口回到对话首页。

```text
Chrome DevTools Protocol verification:
- http://47.76.186.80/marketplace?cdp=debug2
- document.querySelectorAll('.market-card h3')
- document.body.innerText
```

结果：通过。公网官网 `/marketplace` 真实浏览器 DOM 中 `.market-card h3` 为 `Marketplace Operator | Skill Author | Agent Native Runner`，状态条为 `Registry live: /v1/artifacts?limit=100`；页面文本未出现旧云端预留文案。

2026-05-08 中文化返工验证记录：

```powershell
corepack pnpm --dir ui test -- src/features/market/useMarketDirectory.test.tsx
corepack pnpm --dir site build
corepack pnpm --dir ui build
.\scripts\build-desktop.ps1
```

结果：通过。官网市场页、控制台市场页和桌面壳均已重新构建，桌面壳产物更新到 `cmd/anyclaw-desktop/build/bin/anyclaw-desktop.exe`。

```text
Chrome DevTools Protocol verification:
- http://47.76.186.80/marketplace?ui-cn=final
- http://127.0.0.1:18789/dashboard?v=ui-cn-final#/market?source=cloud&kind=agent
- http://127.0.0.1:18789/dashboard?v=ui-cn-chat#/
```

结果：通过。真实浏览器 DOM 验证结论：

- 官网 `/marketplace` 的 UI 文案为中文，状态条为 `Registry 已连接：/v1/artifacts?limit=100`，风险和可信度字段显示为中文；artifact 名称和描述保持 registry 原文，三张卡片为 `Marketplace Operator | Skill Author | Agent Native Runner`。
- 桌面壳 / 控制台云端市场的整体 UI 为中文，包括 `能力市场`、`云端代理目录`、`可安装`、`安装能力` 等；artifact 名称、描述、版本 changelog 保持 registry 原文，例如 `Marketplace Operator`、`Marketplace Operator is an AnyClaw...`、`Initial Marketplace Operator...`。
- 已撤销对 artifact 名称、摘要、详情和 changelog 的强制翻译；后续不能把 skill / agent / cli 包内容硬翻译为中文，只翻译 UI 外壳和通用字段标签。
- 默认入口 `/dashboard?v=<cache-buster>#/` 仍显示 `对话`、`新对话`，没有默认进入市场。
- 服务器 `anyclaw-website` 复查为 healthy。

Round 13.5 结论：

- 旧 seed 已从线上 registry 删除，生产环境也关闭了 seed 自动注入。
- 官网不再展示静态 fallback 市场假数据。
- 官网生产 `/marketplace` 和桌面壳端 Cloud 市场均已通过真实浏览器 DOM 验证：UI 外壳为中文，`anyclaw.*` 真实条目的名称和描述保持 registry 原文，且不再出现旧预留文案。
- Wails 桌面壳和控制台 `/dashboard?v=<cache-buster>#/` 默认进入对话首页，不再默认进入 Cloud Market。
- 下一轮仍为 Round 14。必须等待 owner 说“继续”后，再重新进入安装、绑定、升级、卸载闭环验收。

## Round 14：AnyClaw 客户端云端安装闭环验收

目标：

- AnyClaw 客户端真正能从云端市场浏览、安装、绑定、升级、卸载能力。

交付物：

- Cloud tab 可见真实 artifact。
- install job 成功。
- bind 成功。
- upgrade 成功。
- uninstall 成功。
- audit / event / receipt 可查。

涉及文件/模块：

- `pkg/marketplace/`
- `pkg/gateway/gateway_market_*.go`
- `ui/src/pages/Market/`
- `ui/src/features/market/`
- `pkg/runtime/hot_reload.go`

验收标准：

- 浏览 -> 安装 -> 绑定 -> 使用 -> 升级 -> 卸载 全流程跑通。
- 失败有清晰错误。
- 成功有 receipt / job / event / audit。

测试方式：

```powershell
go test ./pkg/marketplace ./pkg/gateway -run "Market|Install|Bind|Upgrade|Uninstall"
corepack pnpm --dir ui test -- --run useMarketDirectory
```

风险点：

- 本地客户端配置、Gateway token、云端 endpoint 需要统一。

Round 14 状态：

- complete

说明：此前已经做过一次 Round 14 live smoke 和客户端 endpoint 兼容修复，但 owner 在 2026-05-08 指出 Round 13.5 验收不成立，需要先返工市场显示和默认入口。owner 随后明确说“继续”，本轮已按当前代码和配置重新复验，以下记录作为 Round 14 正式验收记录。

Round 14 交付记录：

- 新增 `pkg/gateway/gateway_market_round14_live_test.go`，提供显式开启的公网 live smoke：
  - 默认跳过，不影响日常 CI。
  - 设置 `ANYCLAW_ROUND14_LIVE=1` 后，指向 `ANYCLAW_ROUND14_ENDPOINT` 或默认 `http://47.76.186.80/v1`。
  - 验证 Cloud tab 读取真实 agent / skill / cli 条目。
  - 使用 `anyclaw.skill.skill-author` 跑通 install job、receipt、bind、upgrade job、uninstall。
  - 验证 jobs、events、audit 记录存在，卸载后 binding 和 receipt 被清理。
- 修复客户端 registry endpoint 兼容性：
  - `pkg/marketplace/registry/client.go` 现在兼容 `http://SERVER_IP/v1` / `https://domain/v1` 写法，避免客户端请求变成 `/v1/v1/artifacts`。
  - 新增 `pkg/marketplace/registry/client_test.go` 覆盖带 `/v1` 的 endpoint。
- 已重建 Wails 桌面壳：
  - `cmd/anyclaw-desktop/build/bin/anyclaw-desktop.exe`

Round 14 验证记录：

2026-05-08 正式复验：

```powershell
go test ./pkg/marketplace/registry -run "Client"
```

结果：通过。覆盖 registry client 基础列表、详情、版本、resolve、缓存，以及 `Endpoint=http://host/v1` 的兼容路径。

```powershell
go test ./pkg/gateway -run "TestRound14LiveCloudInstallClosure|TestMarketArtifactsCloudUsesRegistryEndpoint|TestMarketInstallCreatesJobAndReceipt|TestMarketBindingNormalizesMainAgentAndRefreshes"
```

结果：通过。默认未设置 `ANYCLAW_ROUND14_LIVE=1` 时 live smoke 跳过，常规网关市场测试通过。

```powershell
$env:ANYCLAW_ROUND14_LIVE='1'
$env:ANYCLAW_ROUND14_ENDPOINT='http://47.76.186.80/v1'
go test ./pkg/gateway -run TestRound14LiveCloudInstallClosure -count=1 -v
```

结果：通过。公网 registry + 本地 Gateway 客户端闭环成功：
- Cloud agent / skill / cli 分类分别读取到：
  - `anyclaw.agent.marketplace-operator`
  - `anyclaw.skill.skill-author`
  - `anyclaw.cli.agent-native-runner`
- `anyclaw.skill.skill-author` 详情和版本列表可读。
- install job succeeded，并生成 cloud receipt 与 checksum。
- bind 到 `main_agent` 成功，并产生 binding event / audit。
- upgrade job succeeded，现有 binding 保持并更新到新 job receipt。
- uninstall 成功，binding 和 receipt 被清理，并产生 uninstall event / audit。

说明：当前线上三个真实 artifact 都只有 `1.0.0` 一个版本，所以 Round 14 的 upgrade 验证证明了客户端 upgrade 通路可用、可保留 binding、可写 job/event/audit；尚不能证明真实多版本从 `1.0.0` 升到 `2.0.0`。多版本发布与产品化升级体验需要后续发布真实新版本时再验收。

```powershell
go test ./pkg/marketplace ./pkg/gateway -run "Market|Install|Bind|Upgrade|Uninstall"
```

结果：通过。`pkg/marketplace` 与 `pkg/gateway` 的市场浏览、安装、绑定、升级、卸载相关测试通过。

```powershell
corepack pnpm --dir ui test -- --run useMarketDirectory
```

结果：通过。Vitest 显示 `10 passed` test files、`40 passed` tests，其中 `useMarketDirectory.test.tsx` 通过 8 个测试。

```powershell
$env:ANYCLAW_ROUND14_LIVE='1'
$env:ANYCLAW_ROUND14_ENDPOINT='http://47.76.186.80/v1'
go test ./pkg/gateway -run TestRound14LiveCloudInstallClosure -count=1 -v
```

结果：通过。公网 registry + 本地临时 Gateway/store 跑通 Round 14 live closure：

- Cloud agent / skill / cli 分类分别读取到：
  - `anyclaw.agent.marketplace-operator`
  - `anyclaw.skill.skill-author`
  - `anyclaw.cli.agent-native-runner`
- `anyclaw.skill.skill-author` 详情和版本列表可读。
- install job `succeeded`，生成 cloud receipt 与 checksum。
- bind 到 `main_agent` 成功，并产生 binding event / audit。
- upgrade job `succeeded`，现有 binding 保持并更新到新 job receipt。
- uninstall 成功，binding 和 receipt 被清理，并产生 uninstall event / audit。

```powershell
Invoke-RestMethod 'http://127.0.0.1:18789/market/artifacts?source=cloud&kind=agent&limit=100'
Invoke-RestMethod 'http://127.0.0.1:18789/market/artifacts?source=cloud&kind=skill&limit=100'
Invoke-RestMethod 'http://127.0.0.1:18789/market/artifacts?source=cloud&kind=cli&limit=100'
Invoke-RestMethod 'http://127.0.0.1:18789/market/artifacts/anyclaw.skill.skill-author?source=cloud'
Invoke-RestMethod 'http://127.0.0.1:18789/market/artifacts/anyclaw.skill.skill-author/versions?source=cloud'
```

结果：通过。当前运行桌面 Gateway `127.0.0.1:18789` 可读取三类真实 cloud 条目；`anyclaw.skill.skill-author` 详情与 `1.0.0` 版本列表可读。

```powershell
# 当前桌面 Gateway 实际闭环
POST /market/install
GET  /market/jobs/{job_id}
POST /market/bindings
POST /market/upgrade
POST /market/uninstall
GET  /market/events?limit=30
GET  /market/bindings
```

结果：通过。当前桌面 Gateway 使用 `anyclaw.skill.skill-author` 跑通实际闭环：

- 安装前 cloud 状态为 `available`。
- install job `succeeded`，receipt 为 `anyclaw.skill.skill-author@1.0.0`，checksum 存在。
- bind 到 `main_agent` 成功，binding 状态为 `enabled`。
- upgrade job `succeeded`，receipt 仍为 `anyclaw.skill.skill-author@1.0.0`。
- 绑定后 cloud overlay 状态为 `active`。
- uninstall 成功，移除 binding `binding-20260507173145.545520500`。
- 卸载后 cloud overlay 状态回到 `available`，`anyclaw.skill.skill-author` binding 数量为 0。
- `/market/events` 可查到 `market.install.succeeded`、`market.binding.created`、`market.upgrade.started`、`market.uninstall.succeeded`。
- 本地 marketplace audit `marketplace.jsonl` 可查到 `anyclaw.skill.skill-author` 的 install / binding / upgrade / uninstall 记录。
- 卸载后 `anyclaw.skill.skill-author@1.0.0` receipt 文件不存在，证明 receipt 已清理。

```powershell
Invoke-RestMethod 'http://47.76.186.80/v1/artifacts?limit=100'
```

结果：通过。线上 registry 当前仍只返回 3 个真实 `anyclaw.*` 条目。

```powershell
.\scripts\build-desktop.ps1
```

结果：通过。Wails 桌面壳已重建到 `cmd/anyclaw-desktop/build/bin/anyclaw-desktop.exe`。

Round 14 结论：

- complete。
- AnyClaw 客户端云端市场已通过“浏览 -> 安装 -> 绑定 -> 升级通路 -> 卸载”闭环验收。
- 成功路径有 job、receipt、event、audit 记录；卸载后 binding 与 skill receipt 被清理。
- 当前线上三个真实 artifact 都只有 `1.0.0` 一个版本，所以本轮 upgrade 验证证明客户端 upgrade 通路可用、可保留 binding、可写 job/event/audit；尚不能证明真实多版本从 `1.0.0` 升到 `2.0.0`。多版本升级体验需要后续发布真实新版本时再验收。
- 本轮没有进入 Round 15。必须等待 owner 再次说“继续”后，才能进入 Round 15。

## Round 15：安装后自动集成体验增强

目标：

- 安装后能力真的被 AnyClaw runtime 识别和使用。

交付物：

- skill 自动进入 skill catalog。
- agent 自动注册 profile。
- cli 自动注册命令入口。
- binding 后 hot reload 生效。
- uninstall 后清理干净。

涉及文件/模块：

- `pkg/marketplace/`
- `pkg/runtime/`
- `pkg/gateway/gateway_skills_*.go`
- `pkg/gateway/gateway_control_plane_agents_*.go`

验收标准：

- 安装 skill 后可被 agent 使用。
- 安装 agent 后 UI 可见。
- 安装 cli 后可执行或可绑定。
- 卸载后不残留。

测试方式：

```powershell
go test ./pkg/marketplace ./pkg/runtime ./pkg/gateway -run "Market|HotReload|Skill|Agent|CLI"
```

风险点：

- 不同 artifact 类型安装语义不同，需要严格定义目录和 manifest。

Round 15 状态：

- complete

Round 15 交付记录：

- 新增 Gateway marketplace integration 层：
  - install / upgrade job 成功后读取 receipt 与包内 `anyclaw.artifact.json`。
  - skill 自动写入本机 `skills` catalog，并挂到 main agent profile。
  - agent 自动写入 AnyClaw agent profile，`/agents` 可见。
  - cli 自动写入本机 `CLI-Anything/registry.json`，并生成本地命令入口，local CLI market 显示 `installed` / `enabled`。
  - 生成 integration receipt，uninstall 时只按 receipt 清理本轮创建的 skill / agent / cli 资源。
  - install / upgrade / uninstall 后继续触发现有 hot reload 路径。
- marketplace receipt 增加 `description` 字段，安装时从 manifest summary / description 保留 registry 原文，不强制翻译 artifact 名称或说明。
- 对 registry 生成的 `skill/SKILL.md` 包补写规范 `skill.json`，避免 runtime catalog 退化为目录名 `anyclaw-skill-skill-author`。
- 重建桌面壳：
  - `cmd/anyclaw-desktop/build/bin/anyclaw-desktop.exe`

Round 15 验证记录：

2026-05-08 正式验收：

```powershell
go test ./pkg/gateway -run Round15 -count=1 -v
```

结果：通过。`TestRound15MarketInstallIntegratesSkillAgentAndCLI` 验证 skill / agent / cli 安装后分别进入 `/skills`、`/agents`、local CLI market；`TestRound15MarketUninstallCleansIntegratedResources` 验证卸载后 skill 目录、main profile skill ref、integration receipt 被清理。

```powershell
go test ./pkg/marketplace ./pkg/runtime ./pkg/gateway -run "Market|HotReload|Skill|Agent|CLI"
```

结果：通过。Round 15 指定测试命令通过，覆盖 marketplace、runtime hot reload、gateway skill / agent / CLI 相关路径。

```powershell
.\scripts\build-desktop.ps1
```

结果：通过。已先停止占用 `cmd/anyclaw-desktop/build/bin/anyclaw-desktop.exe` 的旧桌面进程，再完成 Wails 桌面壳重建并启动新版 Gateway。

当前桌面 Gateway `http://127.0.0.1:18789` 真实 API 验证：

- `anyclaw.skill.skill-author`：
  - cloud list 可见。
  - install job `succeeded`。
  - `/skills` 出现 `Skill Author`，不再出现目录名 `anyclaw-skill-skill-author`。
  - bind 到 `main_agent` 成功。
  - `market.integration.succeeded` event 可查。
  - uninstall 后 `/skills` 中 `Skill Author` 数量为 0。
- `anyclaw.agent.marketplace-operator`：
  - install job `succeeded`。
  - `/agents` 出现 `Marketplace Operator`。
  - uninstall 后 `/agents` 中 `Marketplace Operator` 数量为 0。
- `anyclaw.cli.agent-native-runner`：
  - install job `succeeded`。
  - local CLI market 出现 `agent-native-runner` / `Agent Native Runner`。
  - 状态为 `installed`，`enabled=True`，entrypoint 指向本地生成的 `.cmd` 文件。
  - uninstall 后 local CLI market 中该条目数量为 0。

Round 15 结论：

- complete。
- 已安装能力现在不只是有 receipt / binding，而是能进入 AnyClaw runtime 使用的本地 catalog：skill 可被 main agent profile 引用，agent profile 可见，cli 可作为本地命令入口显示为 installed/enabled。
- 卸载会按 integration receipt 清理本轮创建资源，避免残留。
- 本轮没有进入 Round 16。必须等待 owner 再次说“继续”后，才能进入 Round 16。

## Round 16：市场搜索、筛选、详情页产品化

目标：

- 市场从“列表”变成“可发现能力”。

交付物：

- 风险筛选。
- 信任筛选。
- 标签筛选。
- 权限筛选。
- 发布者筛选。
- 兼容性筛选。
- 详情页。
- 版本页。
- 排序规则。

涉及文件/模块：

- `pkg/marketregistry/server.go`
- `pkg/marketregistry/store.go`
- `pkg/gateway/gateway_market_artifacts_api.go`
- `ui/src/pages/Market/`
- `site/src/`

验收标准：

- `q/kind/risk/trust/tag/publisher` 可组合筛选。
- 详情页展示权限、版本、下载、兼容性。
- 空状态和错误状态清晰。

测试方式：

```powershell
go test ./pkg/marketregistry ./pkg/gateway -run "Search|Market|Registry"
corepack pnpm --dir site build
corepack pnpm --dir ui exec tsc --noEmit -p tsconfig.json
```

风险点：

- 不先上 embedding；先把关键词 + 标签 + score 做扎实。embedding 属于新增依赖，需要单独确认。

Round 16 状态：

- complete

Round 16 交付记录：

- Registry `SearchFilter` 扩展 `tag`、`permission`、`publisher`、`os`、`arch`、`sort`，`GET /v1/artifacts` 和 `POST /v1/search` 共用同一套组合筛选。
- Registry 排序支持默认 score、`updated`、`name`；默认仍为 score desc + updated desc，未引入 embedding 或新增外部依赖。
- Gateway `/market/artifacts` 解析并透传 `q/kind/risk/trust/tag/permission/publisher/os/arch/sort` 到云端 registry，本地 catalog 也支持同样过滤语义。
- 桌面市场增加风险、可信度、标签、权限、发布者、系统、架构、排序控件；筛选状态进入 URL，可刷新/分享；artifact 名称、描述、标签、权限保持 registry 原文，不强制翻译。
- 官网 `/marketplace` 改为用 registry 服务端筛选参数加载列表，增加同样的筛选控件；详情面板会读取 `/v1/artifacts/{id}/versions` 展示版本、changelog、权限差异和包大小。
- 继续保持 Web / 桌面默认入口为对话，不把首页强制改成市场。
- 重建桌面壳：`cmd/anyclaw-desktop/build/bin/anyclaw-desktop.exe`

Round 16 验证记录：

2026-05-08 正式验收：

```powershell
go test ./pkg/marketregistry ./pkg/gateway -run "Search|Market|Registry"
```

结果：通过。覆盖 registry 组合筛选与 gateway cloud market 参数透传；新增 `TestServerSearchFiltersCombine` 验证 `q/kind/risk/trust/tag/publisher/permission/os/arch/sort` 组合命中与任一条件不匹配时返回空列表。

```powershell
corepack pnpm --dir site build
corepack pnpm --dir ui exec tsc --noEmit -p tsconfig.json
corepack pnpm --dir ui test -- --run useMarketDirectory
.\scripts\build-desktop.ps1
```

结果：全部通过。桌面 hook 测试新增组合筛选 URL 验证；官网 production build 通过；Wails 桌面壳已重建。

真实 API 验证：

```powershell
Invoke-RestMethod 'http://47.76.186.80/v1/artifacts?kind=skill&q=skill&risk=low&trust=verified&publisher=AnyClaw%20Labs&permission=fs.read&os=windows&arch=amd64&sort=name&limit=100'
Invoke-RestMethod 'http://127.0.0.1:18789/market/artifacts?source=cloud&kind=skill&q=skill&risk=low&trust=verified&publisher=AnyClaw%20Labs&permission=fs.read&os=windows&arch=amd64&sort=name&limit=100'
Invoke-RestMethod 'http://47.76.186.80/v1/artifacts/anyclaw.skill.skill-author/versions'
```

结果：生产 registry 和本机 Gateway 均返回 `anyclaw.skill.skill-author`，版本接口返回 `1.0.0`、changelog、compatibility、permissions_diff、size_bytes 和 checksum。

Round 16 结论：

- complete。
- 市场列表现在支持可组合发现筛选，详情和版本元数据可见，空态/错误态沿用现有 UI 并在无匹配结果时明确提示。
- 本轮没有进入 Round 17。必须等待 owner 再次说“继续”后，才能进入 Round 17。

## Round 17：发布后台 MVP

目标：

- 不靠命令行也能运营市场。

交付物：

- admin 登录保护。
- artifact 列表。
- 发布表单。
- 版本管理。
- quarantine / unquarantine。
- publisher token 管理。
- audit / downloads 页面。

涉及文件/模块：

- `pkg/marketregistry/`
- `site/src/` 或独立 admin 页面。
- `docs/PUBLISHING.md`
- `docs/SECURITY.md`

验收标准：

- 管理员能在网页发布 / 隔离 / 查看审计。
- 发布者不能拿 admin token。
- 无 token 访问 admin API 返回 401。

测试方式：

```powershell
go test ./pkg/marketregistry -run "Admin|Token|Publish|Quarantine"
```

风险点：

- 这是新增管理面，会增加安全风险。是否放在官网里还是单独 admin 路径，需要 owner 确认。

Round 17 状态：

- complete

Round 17 交付记录：

- 新增官网 `/admin` 发布后台 MVP，未挂公开导航：
  - 手动输入 admin token 后进入后台。
  - admin token 仅保存在当前 React state，不写入 localStorage，不写入源码或文档。
  - artifact 列表与版本查看。
  - 发布表单，可提交 agent / skill / cli artifact 元数据与版本 changelog。
  - quarantine / unquarantine / delete 管理操作。
  - publisher token 创建、列表、吊销。
  - audit / downloads 查看。
- Registry 新增 `GET /v1/admin/tokens`：
  - 只返回 token metadata：`id`、`publisher_id`、`created_at`、`revoked_at`。
  - 不返回 publisher token 明文；token 仍只在创建响应中一次性展示。
- 更新 `docs/PUBLISHING.md` 和 `docs/SECURITY.md`，补充 `/admin` 安全边界、publisher token 一次性展示和 metadata-only token list。

Round 17 验证记录：

2026-05-08 正式验收：

```powershell
go test ./pkg/marketregistry -run "Admin|Token|Publish|Quarantine"
go test ./pkg/marketregistry ./pkg/gateway -run "Admin|Token|Publish|Quarantine|Market|Registry"
corepack pnpm --dir site exec tsc --noEmit -p tsconfig.json
corepack pnpm --dir site build
```

结果：全部通过。

本地真实 admin API smoke：

```powershell
go build -o $env:TEMP\anyclaw-round17-admin-smoke\anyclaw-registry.exe ./cmd/anyclaw-registry
# 启动临时 registry，使用临时 admin token 和 require-admin-token=true
# 验证 /v1/admin/tokens 无 token 401
# 创建 publisher token
# GET /v1/admin/tokens 只返回 metadata，不泄露 token 明文
# 用 publisher token 发布 round17.skill.admin-smoke
# quarantine 后 resolve 返回 410
# unquarantine 恢复
# admin audit / downloads 可访问
```

结果：

- unauth admin tokens status：`401`
- token metadata list：`token_list_leaks_secret=false`
- published：`round17.skill.admin-smoke`
- resolve after quarantine：`410`
- audit_count：`4`
- downloads_count：`0`

生产部署验证：

```powershell
corepack pnpm --dir site build
scp site/dist ...
ssh root@47.76.186.80 'cd /opt/anyclaw && docker compose --env-file .env.production -f docker-compose.prod.yml up -d --build registry website'
curl http://47.76.186.80/
curl http://47.76.186.80/marketplace
curl http://47.76.186.80/admin
curl http://47.76.186.80/v1/health
curl http://47.76.186.80/v1/artifacts?limit=100
curl http://47.76.186.80/v1/admin/tokens
```

结果：

- `/`、`/marketplace`、`/admin`、`/v1/health`、`/v1/artifacts?limit=100` 均返回 `200`。
- `/v1/admin/tokens` 无 token 返回 `401`。
- 服务器 `docker compose ps` 显示 `anyclaw-registry`、`anyclaw-website`、`anyclaw-gateway` 均 healthy。
- 使用服务器 `.env.production` 中的 admin token 做 metadata smoke：`token_count=4`、`token_list_leaks_secret=false`、`audit_count=5`。验证过程中未打印 token 明文。

Round 17 结论：

- complete。
- 发布后台 MVP 已可网页发布、隔离、查看审计、查看下载统计、管理 publisher token。
- 发布者仍只需要 publisher token，不需要 admin token。
- 本轮没有进入 Round 18。必须等待 owner 再次说“继续”后，才能进入 Round 18。

## Round 18：安全体系增强

目标：

- 上线前把包安全边界补强。

交付物：

- 安装前风险解释。
- 权限 diff。
- 高危权限二次确认。
- artifact 下架提示。
- 下载链路校验强化。
- 包签名方案文档。

涉及文件/模块：

- `pkg/marketplace/`
- `pkg/marketregistry/`
- `pkg/gateway/`
- `docs/SECURITY.md`

验收标准：

- 高风险包不能静默安装。
- checksum 必须校验。
- quarantine 后不能 resolve/download。
- 客户端能看到风险原因。

测试方式：

```powershell
go test ./pkg/marketplace ./pkg/marketregistry ./pkg/gateway -run "Policy|Audit|Quarantine|Checksum"
```

风险点：

- 包签名、恶意扫描可能需要新增工具或依赖，执行前必须单独确认。

Round 18 状态：

- 状态：complete
- 完成时间：2026-05-08

Round 18 交付记录：

- 安装策略会在 resolve 后、download 前输出 `PolicyDecision`，包含 `reason`、`reasons`、风险/可信度、权限和高危权限。
- 高风险 artifact 仍直接 block，不会下载。
- 高危权限新增独立二次确认字段 `risk_acknowledged`，普通 `user_confirmed` 不再足以静默安装高危权限包。
- checksum 缺失会在下载前 block；checksum mismatch 会在安装前 rollback。
- registry quarantine 后 resolve 和 download 均返回 `410 artifact_unavailable`，客户端可看到失败原因。
- 桌面市场安装确认框展示风险、可信度、权限、权限 diff、高危权限；安装 job 展示策略原因和 checksum。
- `market_install_artifact` agent tool 同步支持 `risk_acknowledged`。
- 包签名方案已补充到 `docs/SECURITY.md`；本轮未新增签名工具、恶意扫描工具或外部依赖。

Round 18 验证记录：

```powershell
go test ./pkg/marketplace ./pkg/marketregistry ./pkg/gateway -run "Policy|Audit|Quarantine|Checksum"
```

结果：通过。

```powershell
corepack pnpm --dir ui test -- --run useMarketDirectory
```

结果：通过，10 个测试文件、41 个测试通过。

```powershell
corepack pnpm --dir ui exec tsc --noEmit -p tsconfig.json
```

结果：通过。

```powershell
go test ./pkg/capability/markettools
```

结果：通过。

Round 18 结论：

- 安全体系增强已完成本轮计划验收。
- 本轮没有进入 Round 19。必须等待 owner 再次说“继续”后，才能进入 Round 19。

## Round 19：官网市场完整产品化

目标：

- 官网不只是展示列表，而是完整市场门面。

交付物：

- 市场首页。
- artifact 详情页。
- 安装指引。
- 下载页完善。
- FAQ。
- 更新日志入口。
- 真实数据展示。
- 移动端适配。

涉及文件/模块：

- `site/src/`
- `site/src/registryClient.ts`
- `site/src/styles.css`

验收标准：

- 普通用户能看懂 AnyClaw。
- 能找到 agent / skill / cli。
- 能看到详情和安装方式。
- 移动端不乱。

测试方式：

```powershell
corepack pnpm --dir site build
```

- Chrome desktop/mobile 截图。
- `curl /marketplace /download /docs`。

风险点：

- 官网不要做成管理后台，公开页面和 admin 功能要分清。

Round 19 状态：

- 状态：complete
- 完成时间：2026-05-08

Round 19 交付记录：

- `/marketplace` 从真实 Registry 读取并展示公开市场首页，包含 agent / skill / cli 统计、筛选、搜索、安装说明和真实条目。
- 新增公开详情路由 `/marketplace/:artifactId`，展示 artifact 原始名称/描述、版本、权限、标签、兼容性、风险/可信度和客户端安装指引。
- 下载页补充 release asset 说明、源码构建命令、checksum 校验命令和安装 checklist。
- 文档页补充 FAQ 和更新日志入口。
- 移动端标题、筛选区、市场列表和详情页做了响应式检查，避免横向溢出。
- 修复 `deploy/static-spa-server.py` 的 SPA fallback：带点号的 artifact id 详情路由（例如 `/marketplace/anyclaw.skill.skill-author`）不再被误判为静态文件扩展名。
- 官网继续保持公开门面，不把 `/admin` 链接进公开导航，不在公开页面保存 token 或执行管理操作。

Round 19 验证记录：

```powershell
corepack pnpm --dir site build
```

结果：通过。

```powershell
# Vite dev proxy 指向生产 registry，用于验证真实数据
corepack pnpm --dir site dev -- --host 127.0.0.1 --port 5173
```

结果：通过。`/v1/artifacts?limit=100` 返回真实 3 个条目：

- `anyclaw.agent.marketplace-operator`
- `anyclaw.skill.skill-author`
- `anyclaw.cli.agent-native-runner`

```powershell
# Chrome headless screenshots
chrome --headless=new --window-size=1440,1000 --screenshot=.tmp-round19-market-desktop.png http://127.0.0.1:5173/marketplace
chrome --headless=new --window-size=390,900 --screenshot=.tmp-round19-market-mobile-3.png http://127.0.0.1:5173/marketplace
chrome --headless=new --window-size=1440,1100 --screenshot=.tmp-round19-detail-desktop.png http://127.0.0.1:5173/marketplace/anyclaw.skill.skill-author
```

结果：通过。截图确认市场首页和详情页非空，桌面/移动可读，真实条目可见。

```powershell
curl http://127.0.0.1:5173/marketplace
curl http://127.0.0.1:5173/download
curl http://127.0.0.1:5173/docs
curl http://127.0.0.1:5173/marketplace/anyclaw.skill.skill-author
```

结果：通过，均返回 200。

```powershell
ssh root@47.76.186.80 "cd /opt/anyclaw && docker compose --env-file .env.production -f docker-compose.prod.yml ps"
curl http://47.76.186.80/marketplace
curl http://47.76.186.80/download
curl http://47.76.186.80/docs
curl http://47.76.186.80/marketplace/anyclaw.skill.skill-author
curl http://47.76.186.80/v1/artifacts?limit=100
```

结果：通过。生产 `anyclaw-registry`、`anyclaw-website`、`anyclaw-gateway` 均 healthy；公开路由和 Registry API 均返回 200；生产详情页 Chrome headless 截图 `.tmp-round19-prod-detail.png` 生成成功。

Round 19 结论：

- 官网市场完整产品化已完成本轮计划验收。
- 本轮没有进入 Round 20。必须等待 owner 再次说“继续”后，才能进入 Round 20。

## Round 20：生产部署增强、HTTPS、备份、监控

目标：

- 从 IP 测试环境升级到生产可用。

交付物：

- 域名 DNS。
- Caddy 或 Nginx。
- HTTPS。
- HTTP -> HTTPS。
- 定时备份。
- 恢复演练。
- 安全组最终收紧。
- 1Panel 访问限制。

涉及文件/模块：

- `docker-compose.prod.yml`
- `deploy/Caddyfile` 或 `deploy/nginx.conf`
- `deploy/registry-backup.sh`
- `docs/REGISTRY_BACKUP_RESTORE.md`
- `docs/SECURITY.md`

验收标准：

- `https://domain` 可访问。
- `https://domain/v1/artifacts` 可访问。
- 证书自动续期。
- 备份可恢复。
- `8791` / `18789` 不公网开放。

测试方式：

```bash
curl -I https://domain
curl https://domain/v1/health
docker compose ps
```

风险点：

- 需要域名。Caddy/Nginx 是新增部署依赖，执行前要 owner 确认。

## Round 21：最终上线验收

目标：

- 确认云端市场完整可用。

交付物：

- 上线验收报告。
- 真实 artifact 清单。
- 发布流程报告。
- 客户端闭环报告。
- 安全检查报告。
- 备份恢复报告。
- 未完成项列表。

验收标准：

- 官网可访问。
- 市场有真实数据。
- 客户端可安装 / 绑定 / 升级 / 卸载。
- 发布后台可管理。
- admin API 安全。
- quarantine 有效。
- 备份可恢复。
- HTTPS 可用。

测试方式：

```powershell
go test ./pkg/marketplace ./pkg/marketregistry ./pkg/gateway -run Market
corepack pnpm --dir site build
corepack pnpm --dir ui exec tsc --noEmit -p tsconfig.json
curl https://domain/v1/artifacts
docker compose ps
```

风险点：

- 如果某项只是 MVP，要在验收报告里明确，不假装商业级完成。

## Round 0 验证记录

执行时间：2026-05-07

验证命令与结果：

```powershell
go test ./pkg/marketplace ./pkg/marketplace/registry ./pkg/marketregistry ./pkg/capability/markettools ./pkg/runtime -run "Market|Registry|Install|Policy|Lifecycle|Capability|HotReload|Route"
```

结果：通过。

```powershell
go test ./pkg/gateway -run "Market"
```

结果：通过。

```powershell
corepack pnpm --dir ui exec tsc --noEmit -p tsconfig.json
```

结果：通过。

```powershell
corepack pnpm --dir ui test -- --run useMarketDirectory
```

结果：通过。Vitest 显示 `10 passed` test files、`39 passed` tests，其中 `src/features/market/useMarketDirectory.test.tsx` 通过 7 个测试。

Round 0 结论：

- 新的官网 + 云端市场部署上线总计划已落盘。
- 当前云端市场功能核对结论已写入本文档。
- 基线测试通过。
- 下一轮为 Round 1：部署架构定稿与环境变量规范。必须等待 owner 说“继续”后再开始。
