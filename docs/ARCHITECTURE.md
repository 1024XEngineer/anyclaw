# AnyClaw 架构优化文档

## 概述

本文档描述了基于 OpenClaw 架构模式对 AnyClaw 进行的优化。这些优化使 AnyClaw 更加模块化、可扩展和易于维护。

## 优化架构图

```
┌─────────────────────────────────────────────────────────┐
│                    AnyClaw 优化后架构                     │
├─────────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐     │
│  │    CLI      │  │   Gateway   │  │   Canvas    │     │
│  │   入口层    │  │   控制面    │  │    UI层     │     │
│  └─────────────┘  └─────────────┘  └─────────────┘     │
│          │               │               │              │
│          └───────────────┴───────────────┘              │
│                          │                              │
│  ┌──────────────────────────────────────────────────┐  │
│  │              Agent Runtime (核心)                  │  │
│  │  ┌────────────┐  ┌────────────┐  ┌────────────┐  │  │
│  │  │   Tools    │  │  Channels  │  │  Sessions  │  │  │
│  │  │  注册表    │  │   插件     │  │   管理     │  │  │
│  │  └────────────┘  └────────────┘  └────────────┘  │  │
│  └──────────────────────────────────────────────────┘  │
│                          │                              │
│  ┌──────────────────────────────────────────────────┐  │
│  │              基础设施层                            │  │
│  │  ┌────────────┐  ┌────────────┐  ┌────────────┐  │  │
│  │  │   Event    │  │   Config   │  │   Memory   │  │  │
│  │  │    Bus     │  │   Manager  │  │   Store    │  │  │
│  │  └────────────┘  └────────────┘  └────────────┘  │  │
│  └──────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
```

## 已实现的优化模块

### 1. 事件总线 (Event Bus)

**文件**: `pkg/event/bus.go`

**功能**:
- 发布/订阅模式
- 事件中间件支持
- 事件历史记录
- 异步事件处理

**使用示例**:
```go
// 创建事件总线
eventBus := event.NewEventBus()

// 订阅事件
eventBus.Subscribe(event.EventToolCall, func(ctx context.Context, event event.Event) error {
    fmt.Printf("Tool called: %v\n", event.Data)
    return nil
})

// 发布事件
eventBus.Publish(ctx, event.Event{
    Type:   event.EventToolCall,
    Source: "tool_registry",
    Data: map[string]interface{}{
        "tool": "read_file",
    },
    Timestamp: time.Now().Unix(),
})
```

### 2. 模块化配置系统

**文件**: `pkg/config/modular.go`

**功能**:
- 分层配置管理
- 环境变量覆盖
- 运行时覆盖
- 配置验证
- 配置迁移
- 模块化配置

**使用示例**:
```go
// 创建配置管理器
configManager := config.NewModularConfigManager("anyclaw.json")

// 注册验证器
configManager.RegisterValidator(&MyValidator{})

// 注册迁移器
configManager.RegisterMigrator(&MyMigrator{})

// 加载配置
cfg, err := configManager.Load()

// 设置环境变量覆盖
configManager.SetEnvOverride("ANYCLAW_LLM_PROVIDER", "anthropic")

// 保存配置
configManager.Save()
```

### 3. 插件化渠道系统

**文件**: `pkg/channel/plugin.go`

**功能**:
- 统一渠道接口
- 渠道插件注册
- 渠道健康检查
- 消息广播
- 事件集成

**使用示例**:
```go
// 创建渠道管理器
channelManager := channel.NewChannelManager(eventBus)

// 注册渠道插件
channelManager.RegisterPlugin(&TelegramPlugin{})

// 连接所有渠道
channelManager.ConnectAll(ctx)

// 发送消息
channelManager.SendMessage(ctx, "telegram", &channel.Message{
    Content: "Hello!",
})

// 广播消息
channelManager.BroadcastMessage(ctx, &channel.Message{
    Content: "Broadcast message",
})
```

### 4. 改进的工具注册表

**文件**: `pkg/tools/registry.go`

**功能**:
- 工具分类
- 访问级别控制
- 工具缓存
- 带重试执行
- 工具定义导出

**使用示例**:
```go
// 创建注册表
registry := tools.NewRegistry()

// 注册工具
registry.Register(&tools.Tool{
    Name:        "read_file",
    Description: "Read a file",
    Category:    tools.ToolCategoryFile,
    AccessLevel: tools.ToolAccessPublic,
    Handler:     ReadFileHandler,
})

// 按类别获取工具
fileTools := registry.GetToolsByCategory(tools.ToolCategoryFile)

// 带重试执行
result, err := registry.CallWithRetry(ctx, "read_file", input, 3)
```

### 5. Gateway 控制面

**文件**: `pkg/gateway/control.go`

**功能**:
- WebSocket 服务器
- HTTP API
- 客户端管理
- 消息广播
- 健康检查

**使用示例**:
```go
// 创建 Gateway
gateway := gateway.NewGateway(gateway.GatewayConfig{
    Host: "localhost",
    Port: 18789,
})

// 注册消息处理器
gateway.RegisterHandler("agent.send", handleAgentSend)

// 启动 Gateway
go gateway.Start(ctx)

// 广播消息
gateway.Broadcast(&gateway.Message{
    Type: "notification",
    Result: map[string]interface{}{
        "message": "Hello!",
    },
})
```

### 6. 会话管理系统

**文件**: `pkg/session/manager.go`

**功能**:
- 会话创建/关闭
- 会话历史管理
- 会话持久化
- 会话查询
- 历史限制

**使用示例**:
```go
// 创建会话管理器
sessionManager := session.NewSessionManager("data/sessions", 100)

// 创建会话
session, err := sessionManager.CreateSession("agent1", "telegram", "user1")

// 添加消息
sessionManager.AddMessage(session.ID, session.Message{
    Role:    "user",
    Content: "Hello!",
})

// 获取历史
history, err := sessionManager.GetHistory(session.ID, 10)
```

## 设计模式

### 1. 发布/订阅模式

事件总线使用发布/订阅模式实现组件间的松耦合通信。

### 2. 工具注册表模式

工具注册表使用注册表模式管理所有可用工具。

### 3. 插件模式

渠道系统使用插件模式支持动态加载渠道。

### 4. 策略模式

配置系统使用策略模式支持不同的验证和迁移策略。

### 5. 单例模式

会话管理器使用单例模式确保全局状态一致性。

## 性能优化

### 1. 缓存

工具注册表和配置管理器都实现了缓存机制，减少重复计算。

### 2. 异步处理

事件总线支持异步事件处理，不阻塞主流程。

### 3. 连接池

Gateway 使用连接池管理 WebSocket 连接。

### 4. 历史限制

会话管理器和事件总线都实现了历史限制，防止内存溢出。

## 安全性

### 1. 访问控制

工具注册表支持访问级别控制（public, owner, admin, private）。

### 2. 输入验证

所有工具都实现了输入验证。

### 3. 配置验证

配置管理器在加载时验证配置。

### 4. 安全的 WebSocket

Gateway 支持 TLS 和跨域检查。

## 测试

每个模块都应该有对应的测试文件。测试应该包括：

1. 单元测试
2. 集成测试
3. 性能测试
4. 安全测试

## 后续优化

1. **添加更多渠道插件**: 实现更多渠道的插件
2. **实现 Canvas UI**: 添加可视化界面
3. **添加监控**: 集成监控系统
4. **实现分布式**: 支持分布式部署
5. **添加机器学习**: 集成 ML 模型

## 结论

通过这些优化，AnyClaw 变得更加模块化、可扩展和易于维护。新的架构借鉴了 OpenClaw 的设计模式，同时保持了 Go 语言的简洁性和性能优势。
