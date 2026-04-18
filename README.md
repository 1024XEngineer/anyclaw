# AnyClaw

<div align="center">

本地优先的 AI Agent 工作台，兼顾 CLI、网关服务与 Web 控制台。  
让 AI 不只会聊天，还能真正调用工具、操作文件、连接浏览器，并在你的工作区里完成任务。

<p>
  <img alt="Go" src="https://img.shields.io/badge/Go-1.25%2B-00ADD8?style=flat-square&logo=go&logoColor=white" />
  <img alt="React" src="https://img.shields.io/badge/Web%20UI-React%2019-20232A?style=flat-square&logo=react&logoColor=61DAFB" />
  <img alt="Vite" src="https://img.shields.io/badge/Build-Vite%207-646CFF?style=flat-square&logo=vite&logoColor=white" />
  <img alt="Local First" src="https://img.shields.io/badge/Mode-Local%20First-111827?style=flat-square" />
</p>

</div>

## 项目简介

AnyClaw 是一个面向真实任务执行的 AI Agent 系统。

它不是单纯的聊天壳，而是一个可以在本地工作区中运行的可控执行环境，支持：

- 用命令行直接与 Agent 对话
- 通过网关服务提供 Web 控制台与会话能力
- 接入多种大模型供应商
- 调用文件、Shell、浏览器、桌面自动化等工具
- 通过 Skill / Agent / Channel 扩展能力

如果你想要一个“能落地执行”的本地 AI 工作台，而不是只停留在问答层，AnyClaw 就是为这种场景设计的。

## 核心能力

| 能力 | 说明 |
| --- | --- |
| 多模型接入 | 支持 OpenAI 兼容接口、OpenAI、Anthropic、Qwen、Ollama 等供应商 |
| 本地优先 | 运行状态、工作区上下文、历史数据默认保存在本地 |
| 工具执行 | 支持文件读写、Shell 命令、浏览器自动化、截图、OCR、桌面控制 |
| Web 控制台 | 提供新的控制台界面，用于聊天、市场、渠道、设置等操作入口 |
| 可扩展架构 | 支持 Skill、Agent、插件与 CLI Hub 扩展 |
| 工作区记忆 | 通过 `workflows/` 和本地状态目录管理上下文与运行轨迹 |

## 适用场景

- 想在本地搭建可控的 AI Agent 工作台
- 想把模型能力和真实工具调用接在一起
- 想做 Skill、Agent、Channel 等能力扩展
- 想通过 Web 控制台管理模型、会话和工作区

## 快速开始

### 1. 环境要求

- Go `1.25+`
- Node.js `20+`
- `corepack` / `pnpm`

### 2. 克隆并编译

macOS / Linux:

```bash
git clone https://github.com/1024XEngineer/anyclaw.git
cd anyclaw
go build -o anyclaw ./cmd/anyclaw
```

Windows:

```powershell
git clone https://github.com/1024XEngineer/anyclaw.git
cd anyclaw
go build -o anyclaw.exe ./cmd/anyclaw
```

说明：Windows 下不要把上面的输出文件名写成 `anyclaw`。
如果执行 `go build -o anyclaw ./cmd/anyclaw`，Go 会生成一个没有 `.exe` 后缀的文件，
后续再执行 `.\anyclaw.exe` 时就会提示找不到程序。

### 3. 首次初始化

macOS / Linux:

```bash
./anyclaw onboard
./anyclaw doctor
./anyclaw -i
```

Windows:

```powershell
.\anyclaw.exe onboard
.\anyclaw.exe doctor
.\anyclaw.exe -i
```

## 首次使用说明

AnyClaw 现在默认不会直接把用户带到本地模型。

首次运行时，更推荐你先配置自己的模型供应商信息：

- `Provider`
- `Base URL`
- `API Key`
- 默认模型名称

也就是说，仓库里的 `anyclaw.json` 只是一个安全的示例起点，不是可以直接聊天的私人配置。

如果你更想走纯本地路线，也可以在初始化或设置页中切换到 `Ollama`。

## 启动 Web 控制台

```bash
corepack enable
pnpm ui:install
pnpm ui:build
go run ./cmd/anyclaw gateway start
```

启动后打开：

```text
http://127.0.0.1:18789/dashboard
```

开发 UI 时可以使用：

```bash
pnpm ui:dev
```

常用脚本：

```bash
pnpm ui:install
pnpm ui:dev
pnpm ui:test
pnpm ui:build
pnpm ui:preview
```

## 常用命令

```bash
anyclaw -i
anyclaw onboard
anyclaw doctor
anyclaw gateway start
anyclaw status --all
anyclaw health --verbose
anyclaw channels status
anyclaw models status
anyclaw clihub list --runnable
anyclaw clihub capabilities
anyclaw app list
anyclaw task run "summarize this workspace"
```

交互模式下常见命令：

```text
/help
/clear
/memory
/skills
/tools
/providers
/models <provider>
/agents
/agent use <name>
/set provider <value>
/set model <value>
/set apikey <value>
```

## 控制台与扩展能力

AnyClaw 当前已经具备以下扩展基础：

- `Skill`：扩展任务能力与工具编排
- `Agent`：面向不同角色或任务类型的代理能力
- `Channel`：对接微信、飞书等外部渠道
- `CLI Hub`：发现并调用本地 CLI-Anything 能力目录

如果本地存在 `CLI-Anything-0.2.0` 目录，AnyClaw 可以自动发现并暴露可执行能力，例如：

- Browser
- LibreOffice
- Blender
- GIMP
- ComfyUI
- AnyGen
- Draw.io
- Audacity

示例命令：

```bash
anyclaw clihub list --runnable
anyclaw clihub info anygen
anyclaw clihub exec anygen -- config path
```

## 项目结构

```text
cmd/anyclaw/     CLI 入口
pkg/agent/       Agent 运行时
pkg/apps/        应用运行时与工作流
pkg/clihub/      CLI Hub 与本地能力目录接入
pkg/config/      配置加载与校验
pkg/gateway/     HTTP / WebSocket 网关
pkg/memory/      本地记忆系统
pkg/plugin/      插件与连接器系统
pkg/skills/      Skill 加载与执行
pkg/tools/       内置工具注册表
ui/              Web 控制台源码
dist/control-ui/ Web 控制台构建产物
workflows/       工作区上下文与引导文件
```

## 文档导航

- [快速开始](./docs/QUICKSTART.md)
- [架构说明](./docs/ARCHITECTURE.md)
- [部署说明](./docs/DEPLOYMENT.md)
- [Skill 文档](./docs/SKILLS.md)
- [安全说明](./docs/SECURITY.md)
- [故障排查](./docs/TROUBLESHOOTING.md)

## 注意事项

- `anyclaw.json` 是仓库内可提交的起始配置，请不要把私人密钥直接提交到仓库
- 本地运行状态默认保存在 `./.anyclaw/`
- 工作区相关上下文与记忆保存在 `workflows/`
- 如果你修改了 UI 源码，建议重新执行 `pnpm ui:build` 再启动网关

Windows 终端如果出现中文乱码，可以先执行：

```bash
chcp 65001
```

## 一句话总结

AnyClaw 适合想把“大模型能力 + 本地工具执行 + Web 控制台管理”真正组合到一起的人。  
如果你希望 AI 不只是回答问题，而是能在你的工作区里实际做事，这个项目就是围绕这个目标搭起来的。
