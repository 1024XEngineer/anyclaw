# AnyClaw 官网信息架构与视觉方案

本文档对应 `WEBSITE_MARKETPLACE_DEPLOYMENT_PLAN.md` 的 Round 4，作为 Round 5 官网静态实现的设计规格。

## 1. 官网定位

AnyClaw 官网第一版不是纯下载页，也不是概念营销页。它要让访问者在第一屏理解三件事：

- AnyClaw 是本地优先的 AI Agent 工作台。
- AnyClaw 让 Agent 能连接真实工具、文件、浏览器、桌面和工作区。
- 云端市场把 `agent / skill / cli` 的发现、安装、绑定和自动补能接成一条链路。

一句话主张：

```text
AnyClaw 是本地优先的 AI Agent 工作台，把模型、工具、工作区和能力市场接成一个可控的执行环境。
```

## 2. 首版目标

Round 5 首版官网只做“上线可用”的静态站：

- 首页能讲清楚产品。
- 市场页有页面位置，Round 6 再接 registry 动态数据。
- 下载页能给出当前真实可行的安装方式。
- 文档入口能引导用户到 README / QUICKSTART / DEPLOYMENT。
- 不接登录，不做复杂后台，不做完整开发者平台。

## 3. 目标用户

普通用户：

- 想知道 AnyClaw 是什么、能不能帮自己完成真实任务。
- 需要下载和快速开始。

高阶用户：

- 关心本地优先、权限、安全、模型供应商、工具执行。
- 想知道如何接入 Skill / Agent / CLI。

开发者 / 发布者：

- 关心怎么发布 agent / skill / cli 包。
- 关心 registry token、publish、quarantine、downloads。

## 4. 页面结构

### 4.1 首页 `/`

首屏：

- H1：`AnyClaw`
- 副标题：`本地优先的 AI Agent 工作台`
- 支撑文案：突出“让 AI 不只聊天，而是在你的工作区里调用工具、操作文件、连接浏览器并完成任务”。
- 主按钮：`下载 AnyClaw`
- 次按钮：`浏览云端市场`
- 状态入口：`Registry online / 本地优先 / Agent + Skill + CLI`

第一屏必须有具体产品信号：

- 不做抽象渐变 hero。
- 不做纯口号。
- 使用真实 UI 截图、控制台示意或市场列表预览作为主要视觉资产。
- 如果还没有最终截图，Round 5 可用轻量 dashboard mock，但必须像真实应用界面，不做插画站。

首页内容区：

1. `一个入口，连接真实任务`
   - 本地工作区
   - 工具执行
   - Web 控制台
2. `统一能力市场`
   - Agent
   - Skill
   - CLI
   - Discover -> Install -> Bind
3. `可控，不黑盒`
   - permissions
   - risk / trust
   - audit
   - local-first
4. `快速开始`
   - build CLI
   - onboard
   - gateway

### 4.2 市场页 `/marketplace`

Round 5：

- 静态页面壳。
- 显示 agent / skill / cli 三类资源位。
- 展示 registry 连接说明。
- 显示“Round 6 将读取实时 registry 数据”的开发态说明可以保留在代码注释，不作为显眼产品文案。

Round 6：

- 读取 `/v1/artifacts`。
- 支持 kind 切换。
- 支持搜索。
- 卡片展示 name、kind、summary、version、risk、trust、permissions、source。

首版视觉结构：

```text
顶部导航
市场头部：Cloud Marketplace
类型切换：Agent / Skill / CLI
搜索框
三列或两列卡片列表
右侧或下方详情摘要
```

### 4.3 下载页 `/download`

内容：

- Windows 当前推荐方式。
- macOS / Linux 源码构建方式。
- Docker Compose 网关方式。
- checksum / release 说明预留。

按钮：

- `复制 Windows 构建命令`
- `复制 macOS/Linux 构建命令`
- `查看 Quickstart`

当前真实命令：

```powershell
git clone https://github.com/1024XEngineer/anyclaw.git
cd anyclaw
go build -o anyclaw.exe ./cmd/anyclaw
.\anyclaw.exe onboard
.\anyclaw.exe doctor
.\anyclaw.exe -i
```

```bash
git clone https://github.com/1024XEngineer/anyclaw.git
cd anyclaw
go build -o anyclaw ./cmd/anyclaw
./anyclaw onboard
./anyclaw doctor
./anyclaw -i
```

### 4.4 文档页 `/docs`

首版不复制所有文档，只做导航：

- Quickstart
- Deployment
- Security
- Marketplace Registry
- Publishing

如果静态站不能直接渲染 Markdown，Round 5 先链接到 GitHub / repo docs。

### 4.5 发布者页 `/publish`

首版解释：

- 什么是 agent / skill / cli 包。
- manifest 必须有 `anyclaw.artifact.json`。
- publisher token 从 admin 创建。
- publish、quarantine、downloads 的生命周期。

操作链接：

- Registry Runbook
- Marketplace Registry Dev
- 未来 Publishing 文档

## 5. 导航

桌面导航：

```text
AnyClaw | Marketplace | Download | Docs | Publish | GitHub
```

移动端：

- 简单折叠菜单。
- 不做复杂多级导航。

页脚：

- GitHub
- Docs
- Registry status
- Security note

## 6. 文案基调

中文为主，英文术语保留：

- Agent
- Skill
- CLI
- Registry
- Gateway
- Local-first

语气：

- 直接、可信、偏产品工具。
- 少抽象形容词，多说实际能力。
- 不夸大“全自动”。
- 对权限、安全、token、安装边界明确说明。

推荐短句：

- `先找能力，再安装，再绑定。`
- `安装不等于绑定，AnyClaw 会把状态讲清楚。`
- `自动补能受策略控制，不是无条件静默安装。`
- `Registry 负责分发，本地 AnyClaw 负责最终决策。`

## 7. 视觉方向

整体气质：

- 本地优先、工程可信、产品化工作台。
- 安静、清晰、可扫描。
- 避免过度营销化 hero。

布局：

- 首页首屏用左侧文案 + 右侧真实界面预览可以接受，但如果是 landing hero，主视觉不能只是抽象卡片。
- 更推荐：顶部是宽屏产品界面预览，文案叠在上方或左上方，下一屏露出市场卡片。
- 页面 section 使用全宽 band 或普通内容区，不做层层嵌套卡片。

色彩：

- 主色：深墨色 `#1f2933`。
- 背景：浅灰蓝 `#f6f8fb`。
- 强调：蓝 `#4f6f9f`、绿 `#2f855a`、警告琥珀 `#b7791f`。
- 不使用大面积紫蓝渐变。
- 不使用 beige/cream/sand/tan 作为主基调。

组件：

- 卡片圆角不超过 8px。
- 按钮带图标，优先使用现有 lucide 图标体系。
- 市场资源卡片要适合扫描：kind、risk、trust、version、permissions 要紧凑展示。
- 下载命令使用代码块，配复制按钮。

字体：

- 使用系统字体栈。
- 不随 viewport 宽度缩放字体。
- letter-spacing 保持 0。

## 8. 首页首屏内容草案

H1：

```text
AnyClaw
```

副标题：

```text
本地优先的 AI Agent 工作台
```

正文：

```text
把模型、工具、工作区和能力市场接在一起。让 AI 不只回答问题，也能在你的本地环境里调用工具、操作文件、连接浏览器，并通过 Agent / Skill / CLI 扩展完成真实任务。
```

主按钮：

```text
下载 AnyClaw
```

次按钮：

```text
浏览云端市场
```

状态条：

```text
Local-first
Agent / Skill / CLI
Policy controlled install
Registry ready
```

## 9. 市场卡片内容要求

每张卡片展示：

- name
- summary
- kind
- version
- source
- risk_level
- trust_level
- permissions 前 3 项
- hit_signals 若有

状态文案：

- `available` -> `Available`
- `installed` -> `Installed`
- `bound` -> `Bound`
- `active` -> `Active`
- `quarantined` -> `Quarantined`

## 10. 下载页内容要求

下载页必须避免制造“已有安装包”的假象。

当前如果没有 GitHub Release 安装包，就显示：

- `从源码构建`
- `Windows`
- `macOS / Linux`
- `Docker Compose Gateway`

等 Round 7 接入真实 release 产物后，再加：

- `.exe`
- `.zip`
- `.tar.gz`
- checksum

## 11. Round 5 实现边界

Round 5 必须实现：

- `/`
- `/marketplace`
- `/download`
- `/docs`
- `/publish`
- 404
- 响应式布局
- 占位市场卡片或静态 fixture
- 下载命令复制按钮

Round 5 不实现：

- 实时 registry 读取。
- publish 后台。
- 登录。
- 管理后台。
- HTTPS。
- Release 自动构建。

这些分别属于 Round 6、Round 8、Round 9、Round 12、Round 7。

## 12. 验收标准

Round 4 完成标准：

- 本文档已存在。
- 官网页面结构明确。
- 首页首屏文案明确。
- 视觉方向明确。
- Round 5 实现边界明确。
- 未新增依赖。
- 未改业务逻辑。
