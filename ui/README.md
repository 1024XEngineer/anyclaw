# AnyClaw UI 项目说明

## 项目概述

AnyClaw UI 是 AnyClaw 项目的 Web 管理界面，采用 React + TypeScript + Tailwind CSS 构建，提供可视化的系统监控、通道管理、会话管理、定时任务等功能。

## 技术栈

- **前端框架**: React 18
- **语言**: TypeScript
- **样式**: Tailwind CSS
- **构建工具**: Vite
- **通信**: WebSocket + JSON-RPC

## 项目结构

```
ui/
├── src/
│   ├── components/          # React 组件
│   │   ├── Dashboard.tsx    # 仪表盘主页
│   │   ├── Sidebar.tsx      # 侧边导航栏
│   │   ├── Header.tsx       # 顶部标题栏
│   │   ├── ChannelList.tsx   # 通道列表
│   │   ├── SessionList.tsx   # 会话列表
│   │   ├── CronManager.tsx   # 定时任务管理
│   │   ├── ChatPanel.tsx     # 聊天面板
│   │   ├── LogsViewer.tsx    # 日志查看器
│   │   └── StatusIndicator.tsx # 状态指示器
│   ├── hooks/               # 自定义 Hooks
│   │   ├── useApi.ts         # 通用 API 调用
│   │   ├── useChannels.ts    # 通道数据获取
│   │   ├── useWebSocket.ts   # WebSocket 连接
│   │   └── useSessions.ts   # 会话数据获取
│   ├── services/            # 服务层
│   │   ├── rpc.ts            # JSON-RPC 客户端
│   │   └── websocket.ts      # WebSocket 客户端
│   ├── types/               # TypeScript 类型定义
│   │   └── index.ts
│   ├── App.tsx              # 主应用组件
│   ├── main.tsx             # 入口文件
│   └── index.css            # 全局样式
├── package.json
├── vite.config.ts
├── tailwind.config.js
└── tsconfig.json
```

## 功能介绍

### 1. 仪表盘 (Dashboard)

系统总览页面，展示以下信息：

- **统计卡片**：
  - Channels：通道数量及在线数量
  - Cron Tasks：定时任务数量及运行状态
  - WebSocket：连接状态（实时）

- **系统健康状态**：显示系统状态、上次检查时间

- **快速统计**：在线通道数等

- **通道状态概览**：显示所有已配置通道的在线状态

### 2. 通道管理 (Channels)

- 列出所有已配置的通信通道
- 显示每个通道的类型、名称、在线状态
- 支持的通道类型：Slack、Discord、飞书、企业微信、Telegram 等

### 3. 会话管理 (Sessions)

- 查看所有活跃会话
- 显示会话关联的通道、聊天 ID、创建时间
- 支持查看会话详情和清除会话

### 4. 定时任务 (Cron Jobs)

- 查看所有定时任务
- 创建新的定时任务（名称、Cron 表达式、启用状态）
- 编辑、删除、立即执行定时任务
- 查看任务执行历史记录

### 5. 聊天面板 (Chat)

- 选择通道和聊天 ID 进行即时消息发送
- 实时消息展示（用户消息、AI 回复）
- 发送消息到 Agent 并获取响应
- WebSocket 实时接收 inbound/outbound 消息

### 6. 日志查看器 (Logs)

- 查看系统日志
- 按通道筛选日志
- 显示日志级别、时间戳、消息内容

## 通信机制

### JSON-RPC

前端通过 JSON-RPC 与后端网关通信：

```typescript
// 示例：获取健康状态
const response = await fetch('/rpc', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    jsonrpc: '2.0',
    method: 'health',
    params: {},
    id: '1'
  })
});
```

主要 API 方法：
- `health` - 系统健康检查
- `channels.list` - 获取通道列表
- `channels.status` - 获取通道状态
- `send` - 发送消息
- `sessions.list` - 获取会话列表
- `cron.status` - Cron 状态
- `cron.list` - 定时任务列表
- `cron.add` - 添加定时任务
- `logs.get` - 获取日志

### WebSocket

实时通信，用于：
- 接收 inbound 消息
- 接收 outbound 消息
- 系统通知

## 运行项目

### 开发模式

```bash
cd ui
npm install
npm run dev
```

### 生产构建

```bash
npm run build
```

## 注意事项

1. **后端依赖**：UI 需要连接到运行中的 anyclaw gateway 服务（默认 `/rpc`）
2. **跨域配置**：开发环境需配置 Vite 代理解决跨域问题
3. **数据类型兼容**：部分 API 返回数据可能是字符串或对象格式，组件需兼容处理
