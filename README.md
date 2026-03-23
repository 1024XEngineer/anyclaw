# AnyClaw 0.1
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8.svg)](https://golang.org/)
[![Status](https://img.shields.io/badge/status-架构脚手架-brightgreen.svg)](https://github.com/your-username/anyclaw)

AnyClaw 是一款**本地优先**的可视化 AI 智能体平台，采用 Go 语言构建。0.1 版本作为框架骨架，并非简单迁移 `anyclaw-0-2` 的现有实现细节，而是聚焦于搭建清晰的产品架构，为平台后续的规模化演进奠定基础。

## 产品定位
AnyClaw 旨在为用户提供一站式 AI 智能体管理与执行能力，核心能力包括：
- 创建并管理多个 AI 助手实例
- 自定义配置助手的人设、技能、模型及工作空间
- 在明确的安全边界内为助手授权
- 调用工具、浏览器、文件系统及命令执行真实任务
- 审计所有关键操作行为
- 与助手长期协作迭代

## 框架架构

```
├── cmd/
├── configs/
│   └── anyclaw.example.json
├── docs/
│   └── architecture.md
├── pkg/
│   ├── app/
│   │   └── app.go
│   ├── channels/
│   │   └── channels.go
│   ├── controlplane/
│   │   └── service.go
│   ├── domain/
│   │   ├── assistant/
│   │   ├── audit/
│   │   ├── memory/
│   │   ├── task/
│   │   └── workspace/
│   ├── gateway/
│   │   └── server.go
│   ├── orchestrator/
│   │   └── service.go
│   ├── runtimecore/
│   │   └── engine.go
│   ├── security/
│   │   └── service.go
│   ├── skills/
│   │   └── catalog.go
│   ├── storage/
│   │   └── local_store.go
│   └── tools/
│       └── registry.go
└── go.mod
```


## 0.1 版本核心目标
- 优先定义稳定的领域模型
- 分离控制平面与运行时核心
- 将安全与审计作为一等模块设计
- 以本地存储作为默认数据平面
- 为渠道、工具、技能预留清晰的扩展点

## 项目状态
当前为 AnyClaw 平台的**架构起点**，聚焦于搭建清晰的技术骨架，后续将逐步完善功能实现与生态扩展。

## 快速开始
### 环境要求
- Go 1.22+
- Git

### 克隆代码

```
git clone https://github.com/your-username/anyclaw.git
cd anyclaw
```

### 构建 CLI

```
go build -o anyclaw ./cmd/anyclaw
```

### 查看帮助

```
./anyclaw --help
```
