# Agent 执行流程

本文档描述 Agent 处理用户请求的执行序列，重点介绍双循环机制。

## 双循环机制概览

Orchestrator 采用**双循环架构**来处理不同类型的消息：

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         Orchestrator.runLoop()                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │                    Outer Loop (外层循环)                                │ │
│  │                    职责：处理 FollowUp 后续任务                          │ │
│  │                                                                         │ │
│  │  for {                                                                  │ │
│  │      ┌──────────────────────────────────────────────────────────────┐  │ │
│  │      │              Inner Loop (内层循环)                            │  │ │
│  │      │              职责：处理工具调用链和 Steering 中断              │  │ │
│  │      │                                                               │  │ │
│  │      │  for hasMoreToolCalls || len(pendingMessages) > 0 {          │  │ │
│  │      │      // 处理消息、调用 LLM、执行工具                            │  │ │
│  │      │  }                                                           │  │ │
│  │      │                                                               │  │ │
│  │      └──────────────────────────────────────────────────────────────┘  │ │
│  │                              │                                         │ │
│  │                              ▼                                         │ │
│  │              followUpMessages := fetchFollowUpMessages()               │ │
│  │              if len(followUpMessages) > 0 {                            │ │
│  │                  continue  // 继续外层循环                              │ │
│  │              } else {                                                  │ │
│  │                  break     // 退出，任务完成                            │ │
│  │              }                                                         │ │
│  │  }                                                                     │ │
│  │                                                                         │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 循环职责对比

| 循环 | 职责 | 触发条件 | 退出条件 |
|------|------|----------|----------|
| **外层循环** | 处理 FollowUp 后续任务 | 有 FollowUp 消息 | 无 FollowUp 消息 |
| **内层循环** | 处理工具调用和 Steering | 有工具调用或待处理消息 | 无工具调用且无待处理消息 |

### 与 OpenClaw 循环架构对比

两者都采用双循环架构，但恢复逻辑的放置位置不同：

| 特性 | AnyClaw (双循环) | OpenClaw (双循环) |
|------|----------------|-------------------|
| **外层循环职责** | 处理 FollowUp 后续任务 | 处理重试、故障转移、Profile 轮换、上下文压缩 |
| **内层循环职责** | 工具调用 + Steering + 重试/故障转移 | 工具执行 (pi-agent-core 内部) |
| **迭代计数** | `iteration` 计数工具调用次数 | `runLoopIterations` 计数重试次数 (外层) |
| **重试机制** | 在内层循环的 `streamAssistantResponseWithRetry` 中处理 | 在外层循环通过 `continue` 重新调用 `runEmbeddedAttempt` |
| **故障转移** | 在 LLM 调用层处理 | 在外层循环通过 `advanceAuthProfile()` + `continue` 处理 |
| **上下文压缩** | 在 LLM 调用层处理 | 在外层循环 `overflowCompactionAttempts++` + `continue` 处理 |

**OpenClaw 双循环**：外层循环处理所有恢复逻辑（重试、故障转移、压缩），内层循环（pi-agent-core）处理工具执行。

**AnyClaw 双循环**：外层循环仅处理 FollowUp 任务链，恢复逻辑封装在内层循环的 LLM 调用层。

## 请求处理序列

```
┌───────────────┐
│ User Message   │
│ (cli/channel) │
└───────┬───────┘
        │
        ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Agent (agent.go)                              │
│                                                                 │
│  1. handleInboundMessage                                        │
│  ┌──────────────────────────────────────────────────────┐      │
│  │ - Generate session key from channel + chat ID        │      │
│  │ - Get or create session                      │      │
│  │ - Load history messages (safe, preserve tool pairs)  │      │
│  │ - Convert to AgentMessage format                     │      │
│  │ - Append current message to history                  │      │
│  └──────────────────────────────────────────────────────┘      │
│                         │                                       │
│                         ▼                                       │
│  2. Orchestrator.Run                                            │
│  ┌──────────────────────────────────────────────────────┐      │
│  │                                                       │      │
│  │  ╔═══════════════════════════════════════════════════╗│      │
│  │  ║     OUTER LOOP (处理 FollowUp)                    ║│      │
│  │  ║                                                   ║│      │
│  │  ║  ┌─────────────────────────────────────────────┐  ║│      │
│  │  ║  │   INNER LOOP (处理工具调用和 Steering)      │  ║│      │
│  │  ║  │                                             │  ║│      │
│  │  ║  │  for iteration <= maxIterations:            │  ║│      │
│  │  ║  │    │                                        │  ║│      │
│  │  ║  │    ├─ Check context cancellation            │  ║│      │
│  │  ║  │    │                                        │  ║│      │
│  │  ║  │    ├─ Inject pending Steering messages      │  ║│      │
│  │  ║  │    │                                        │  ║│      │
│  │  ║  │    ├─ Build system prompt with ContextBuild ║│      │
│  │  ║  │    │                                        │  ║│      │
│  │  ║  │    ├─ Call LLM (streamAssistantResponse)    │  ║│      │
│  │  ║  │    │                                        │  ║│      │
│  │  ║  │    ├─ Extract tool calls from response      │  ║│      │
│  │  ║  │    │                                        │  ║│      │
│  │  ║  │    ├─ If has tool calls:                    │  ║│      │
│  │  ║  │    │     executeToolCalls()                 │  ║│      │
│  │  ║  │    │     │                                  │  ║│      │
│  │  ║  │    │     ├─ Execute each tool               │  ║│      │
│  │  ║  │    │     ├─ Check Steering after each tool  │  ║│      │
│  │  ║  │    │     │   └─ If Steering: break inner    │  ║│      │
│  │  ║  │    │     └─ Add tool results to messages    │  ║│      │
│  │  ║  │    │                                        │  ║│      │
│  │  ║  │    └─ Emit EventTurnEnd                     │  ║│      │
│  │  ║  │                                             │  ║│      │
│  │  ║  │  Loop condition:                            │  ║│      │
│  │  ║  │    hasMoreToolCalls || len(pending) > 0     │  ║│      │
│  │  ║  │                                             │  ║│      │
│  │  ║  └─────────────────────────────────────────────┘  ║│      │
│  │  ║                      │                            ║│      │
│  │  ║                      ▼                            ║│      │
│  │  ║        followUpMessages := fetchFollowUpMessages()║│      │
│  │  ║        if len(followUpMessages) > 0:              ║│      │
│  │  ║            pendingMessages = followUpMessages     ║│      │
│  │  ║            continue  // Continue OUTER loop       ║│      │
│  │  ║        else:                                      ║│      │
│  │  ║            break     // Exit, task complete       ║│      │
│  │  ║                                                   ║│      │
│  │  ╚═══════════════════════════════════════════════════╝│      │
│  │                                                       │      │
│  └───────────────────────────────────────────────────────┘      │
│                         │                                       │
│                         ▼                                       │
│  3. Update Session                                              │
│  ┌──────────────────────────────────────────────────────┐      │
│  │ - Save new messages to session                        │      │
│  │ - Persist to JSONL file                               │      │
│  └──────────────────────────────────────────────────────┘      │
│                         │                                       │
│                         ▼                                       │
│  4. Publish Response                                           │
│  ┌──────────────────────────────────────────────────────┐      │
│  │ bus.PublishOutbound(OutboundMessage)                  │      │
│  └──────────────────────────────────────────────────────┘      │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
          │
          ▼
┌──────────────────┐
│ User/Channel     │
│ Receive          │
└──────────────────┘
```

## 错误处理流程

```
Tool Execution Error
        │
        ▼
┌─────────────────────────────┐
│ RetryManager                │
│ - RecordError(err)          │
│ - Classify error            │
└───────────┬─────────────────┘
            │
            ▼
    ┌───────────────┐
    Is Retryable?    │
    └───┬─────┬─────┘
        │     │
        │ No  │ Yes
        │     │
        ▼     ▼
   ┌────┴────┐
   │ Return  │ Recovery
   │ Error   │ Action
   │ to User │
   └─────────┘
             │
             ▼
   ┌──────────────────┐
   │ RecoveryAction   │
   │ - rotate_profile │
   │ - compress_ctx   │
   └────────┬─────────┘
            │
            ▼
   ┌──────────────────┐
   │ Calculate Delay  │
   │ (exponential     │
   │  backoff)        │
   └────────┬─────────┘
            │
            ▼
   ┌──────────────────┐
   │ Wait & Retry     │
   └──────────────────┘
```

## Steering 和 FollowUp 消息流

### 双循环中的消息注入点

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         Orchestrator Dual Loop                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ╔═══════════════════════════════════════════════════════════════════════╗  │
│  ║  OUTER LOOP                                                           ║  │
│  ║                                                                       ║  │
│  ║    ┌─────────────────────────────────────────────────────────────┐    ║  │
│  ║    │  INNER LOOP                                                  │    ║  │
│  ║    │                                                              │    ║  │
│  ║    │   ┌─────────────────────────────────────────────────────┐   │    ║  │
│  ║    │   │  Steering 注入点 A (内层循环开始前)                  │   │    ║  │
│  ║    │   │  pendingMessages = fetchSteeringMessages()          │   │    ║  │
│  ║    │   └─────────────────────────────────────────────────────┘   │    ║  │
│  ║    │                          │                                    │    ║  │
│  ║    │                          ▼                                    │    ║  │
│  ║    │   ┌─────────────────────────────────────────────────────┐   │    ║  │
│  ║    │   │  LLM 调用                                           │   │    ║  │
│  ║    │   └─────────────────────────────────────────────────────┘   │    ║  │
│  ║    │                          │                                    │    ║  │
│  ║    │                          ▼                                    │    ║  │
│  ║    │   ┌─────────────────────────────────────────────────────┐   │    ║  │
│  ║    │   │  工具执行 (Tool 1, Tool 2, ...)                     │   │    ║  │
│  ║    │   │                                                      │   │    ║  │
│  ║    │   │   ┌─────────────────────────────────────────────┐   │   │    ║  │
│  ║    │   │                                                      │   ║  │
│  ║    │   │    Tool 1 ──► Tool 2 ──► Tool 3                      │   ║  │
│  ║    │   │       │           │           │                      │   ║  │
│  ║    │   │       │           │           │                      │   ║  │
│  ║    │   │       ▼           ▼           ▼                      │   ║  │
│  ║    │   │  Steering 注入点 B (每个工具执行后)                   │   │    ║  │
│  ║    │   │  steering = fetchSteeringMessages()                  │   │    ║  │
│  ║    │   │  if steering: return results, steering               │   │    ║  │
│  ║    │   └─────────────────────────────────────────────────────┘   │    ║  │
│  ║    │                          │                                    │    ║  │
│  ║    │                          ▼                                    │    ║  │
│  ║    │              内层循环结束条件满足                              │    ║  │
│  ║    │              (无工具调用且无待处理消息)                        │    ║  │
│  ║    │                                                              │    ║  │
│  ║    └─────────────────────────────────────────────────────────────┘    ║  │
│  ║                               │                                       ║  │
│  ║                               ▼                                       ║  │
│  ║    ┌─────────────────────────────────────────────────────────────┐    ║  │
│  ║    │  FollowUp 注入点 (外层循环检查)                              │    ║  │
│  ║    │  followUpMessages = fetchFollowUpMessages()                 │    ║  │
│  ║    │  if len(followUpMessages) > 0:                              │    ║  │
│  ║    │      pendingMessages = followUpMessages                     │    ║  │
│  ║    │      continue  // 继续外层循环                               │    ║  │
│  ║    │  else:                                                      │    ║  │
│  ║    │      break     // 任务完成                                   │    ║  │
│  ║    └─────────────────────────────────────────────────────────────┘    ║  │
│  ║                                                                       ║  │
│  ╚═══════════════════════════════════════════════════════════════════════╝  │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Steering (中断式消息)

```
用户发送紧急消息
        │
        ▼
┌─────────────────────────────┐
│ agent.Steer(msg)            │
│ state.Steer(msg)            │
│ → 加入 steeringMessages 队列│
└───────────┬─────────────────┘
            │
            ▼
┌─────────────────────────────┐
│ Orchestrator 在下一次迭代   │
│ fetchSteeringMessages()     │
│ → 获取并清空队列            │
└───────────┬─────────────────┘
            │
            ▼
┌─────────────────────────────┐
│ 注入到当前对话              │
│ 立即影响 Agent 行为         │
└─────────────────────────────┘
```

### FollowUp (后续消息)

```
用户发送后续任务
        │
        ▼
┌─────────────────────────────┐
│ agent.FollowUp(msg)         │
│ state.FollowUp(msg)         │
│ → 加入 followUpMessages 队列│
└───────────┬─────────────────┘
            │
            ▼
┌─────────────────────────────┐
│ Orchestrator 当前任务完成后 │
│ fetchFollowUpMessages()     │
│ → 获取并清空队列            │
└───────────┬─────────────────┘
            │
            ▼
┌─────────────────────────────┐
│ 继续执行外层循环            │
│ 处理后续任务                │
└─────────────────────────────┘
```

## 技能加载流程

```
┌──────────────────┐
│ Agent Starts     │
└───────┬──────────┘
        │
        ▼
┌─────────────────────────────────────────────────────────────┐
│ SkillsLoader.Discover()                                      │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  1. 扫描技能目录（按优先级顺序）                              │
│     for _, dir := range skillsDirs:                         │
│         discoverInDir(dir)                                   │
│                                                              │
│  2. 解析每个 SKILL.md                                        │
│     for each skillDir:                                       │
│         - Read SKILL.md                                      │
│         - Parse YAML frontmatter                             │
│         - Extract metadata                                   │
│                                                              │
│  3. 检查阻塞性需求                                            │
│     checkBlockingRequirements():                             │
│       - OS 兼容性检查                                        │
│       - Always 技能跳过检查                                  │
│                                                              │
│  4. 计算缺失依赖                                              │
│     getMissingDeps():                                        │
│       - Check bins: exec.LookPath()                          │
│       - Check anyBins: 任一存在即可                          │
│       - Check env: os.Getenv()                               │
│       - Check Python packages                                │
│       - Check Node.js packages                               │
│                                                              │
│  5. 存储到 skills map                                         │
│     skills[name] = &skill                                    │
│                                                              │
└─────────────────────────────────────────────────────────────┘
        │
        ▼
┌─────────────────────────────────────────────────────────────┐
│ ContextBuilder.BuildSystemPrompt()                           │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  第一阶段：技能摘要                                           │
│  ┌─────────────────────────────────────────────────────────┐│
│  │ buildSkillsPrompt():                                     ││
│  │   for each skill:                                        ││
│  │     - Name, Description                                  ││
│  │     - Missing Dependencies (with install commands)       ││
│  │                                                          ││
│  │ 输出示例:                                                 ││
│  │ <skill name="weather">                                   ││
│  │ **Name:** weather                                        ││
│  │ **Description:** Get weather info                        ││
│  │ **Missing Dependencies:**                                ││
│  │   - Binary dependencies: [curl]                          ││
│  │ </skill>                                                 ││
│  └─────────────────────────────────────────────────────────┘│
│                                                              │
│  第二阶段：完整技能内容（use_skill 调用后）                    │
│  ┌─────────────────────────────────────────────────────────┐│
│  │ buildSelectedSkills():                                   ││
│  │   for each loadedSkill:                                  ││
│  │     - Full skill content from SKILL.md                   ││
│  │     - Detailed install instructions                      ││
│  │                                                          ││
│  │ 输出示例:                                                 ││
│  │ <skill name="weather">                                   ││
│  │ ### weather                                              ││
│  │ > Description: Get weather info                          ││
│  │                                                          ││
│  │ # Weather Forecast                                       ││
│  │ When the user asks about weather:                        ││
│  │ 1. Use run_shell to execute: curl wttr.in/...           ││
│  │ </skill>                                                 ││
│  └─────────────────────────────────────────────────────────┘│
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

## SubAgent 生成流程

```
┌──────────────────────────────┐
│ Main Agent                   │
│ (decides to delegate)        │
└──────────┬───────────────────┘
           │
           │ Tool call: spawn_subagent
           ▼
┌─────────────────────────────────────┐
│ SpawnTool.Execute()                  │
│ 1. Validate subagent name            │
│ 2. Create subagent context           │
│ 3. Generate unique task ID           │
│ 4. Create task message               │
└───────────┬─────────────────────────┘
            │
            ▼
┌─────────────────────────────────────┐
│ SubAgentManager                      │
│ - Create isolated agent instance     │
│ - Use PromptModeMinimal              │
│ - Run in separate goroutine          │
└───────────┬─────────────────────────┘
            │
            ▼
┌─────────────────────────────────────┐
│ New Agent Instance                   │
│ - Own session/context                │
│ - Runs independently                 │
│ - Has limited tool set               │
│ - Reports back when done             │
└───────────┬─────────────────────────┘
            │
            ▼ (completion announcement)
┌─────────────────────────────────────┐
│ Original Session                     │
│ Receives compiled result             │
│ via SubAgentAnnounce                 │
└─────────────────────────────────────┘
```

## 事件流

```
Orchestrator Events
        │
        ├── EventAgentStart
        │       └── Agent 开始运行
        │
        ├── EventTurnStart
        │       └── 新一轮对话开始
        │
        ├── EventMessageStart
        │       └── 消息开始生成
        │
        ├── EventStreamContent (多次)
        │       └── 流式内容块
        │
        ├── EventStreamThinking (多次)
        │       └── 思考过程内容
        │
        ├── EventStreamFinal (多次)
        │       └── 最终输出内容
        │
        ├── EventStreamDone
        │       └── 流式输出完成
        │
        ├── EventMessageEnd
        │       └── 消息生成完成
        │
        ├── EventToolExecutionStart
        │       └── 工具执行开始
        │       │   - ToolID
        │       │   - ToolName
        │       │   - Arguments
        │
        ├── EventToolExecutionUpdate (多次)
        │       └── 工具执行更新
        │
        ├── EventToolExecutionEnd
        │       └── 工具执行完成
        │           - Result
        │           - Error (if any)
        │
        ├── EventTurnEnd
        │       └── 本轮对话结束
        │
        └── EventAgentEnd
                └── Agent 运行结束
                    - FinalMessages
```

## 消息转换

### Agent 消息 → Provider 消息

```go
// convertToProviderMessages
func convertToProviderMessages(messages []AgentMessage) []providers.Message {
    result := []providers.Message{}

    for _, msg := range messages {
        // 跳过 system 消息（已单独处理）
        if msg.Role == RoleSystem {
            continue
        }

        // 跳过孤立的 tool 消息
        if msg.Role == RoleToolResult {
            if !hasValidToolCallID(msg) {
                continue
            }
        }

        providerMsg := providers.Message{
            Role: string(msg.Role),
        }

        // 提取内容
        for _, block := range msg.Content {
            switch b := block.(type) {
            case TextContent:
                providerMsg.Content += b.Text
            case ImageContent:
                providerMsg.Images = append(providerMsg.Images, b.Data)
            }
        }

        // 处理 tool calls
        if msg.Role == RoleAssistant {
            providerMsg.ToolCalls = extractToolCalls(msg)
        }

        // 处理 tool result
        if msg.Role == RoleToolResult {
            providerMsg.ToolCallID = msg.Metadata["tool_call_id"]
            providerMsg.ToolName = msg.Metadata["tool_name"]
        }

        result = append(result, providerMsg)
    }

    return result
}
```

### Provider 响应 → Agent 消息

```go
// convertFromProviderResponse
func convertFromProviderResponse(response *providers.Response) AgentMessage {
    content := []ContentBlock{TextContent{Text: response.Content}}

    // 处理 tool calls
    for _, tc := range response.ToolCalls {
        content = append(content, ToolCallContent{
            ID:        tc.ID,
            Name:      tc.Name,
            Arguments: tc.Params,
        })
    }

    return AgentMessage{
        Role:      RoleAssistant,
        Content:   content,
        Timestamp: time.Now().UnixMilli(),
        Metadata:  map[string]any{"stop_reason": response.FinishReason},
    }
}
```

## 上下文压缩

当达到 token 限制时，Orchestrator 会压缩消息历史：

```go
// compressMessages
func (o *Orchestrator) compressMessages(messages []AgentMessage) []AgentMessage {
    if len(messages) <= 4 {
        return messages
    }

    // 保留最近 4 条消息
    keepRecent := 4
    summary := o.createMessageSummary(messages[:len(messages)-keepRecent])

    // 创建压缩后的消息历史
    compressed := []AgentMessage{
        {
            Role:    RoleSystem,
            Content: []ContentBlock{TextContent{Text: summary}},
        },
    }
    compressed = append(compressed, messages[len(messages)-keepRecent:]...)

    return compressed
}
```

输出示例：
```
[CONTEXT SUMMARY]
Previous conversation history has been compressed.
Messages: 5 user, 5 assistant, 12 tool results.
Last user message: Create a Python script that processes CSV files...
[END SUMMARY]
```

## 与 OpenClaw 循环架构对比

### 架构差异

两者都采用双循环架构，但恢复逻辑的放置位置不同：

| 特性 | AnyClaw (双循环) | OpenClaw (双循环) |
|------|----------------|-------------------|
| **外层循环职责** | 处理 FollowUp 后续任务 | 处理重试、故障转移、Profile 轮换、上下文压缩 |
| **内层循环职责** | 工具调用 + Steering 中断 + 重试/故障转移 | 工具执行 (pi-agent-core 内部) |
| **迭代计数** | `iteration` 计数工具调用次数 | `runLoopIterations` 计数重试次数 (外层) |

### OpenClaw 外层循环核心

```typescript
// OpenClaw: 外层循环 - 处理重试、故障转移、压缩
while (true) {
  if (runLoopIterations >= MAX_RUN_LOOP_ITERATIONS) {
    return error;
  }
  runLoopIterations += 1;

  // 内层循环通过 runEmbeddedAttempt 调用 pi-agent-core
  const attempt = await runEmbeddedAttempt({ ... });

  if (promptError) {
    // 故障转移
    if (await advanceAuthProfile()) {
      continue; // 继续外层循环
    }
    throw promptError;
  }

  if (contextOverflowError) {
    // 上下文压缩
    const compactResult = await contextEngine.compact({ ... });
    if (compactResult.compacted) {
      continue; // 继续外层循环
    }
    // 工具结果截断...
  }

  if (shouldRotate) {
    // Profile 轮换
    const rotated = await advanceAuthProfile();
    if (rotated) {
      continue; // 继续外层循环
    }
  }

  return result;
}
```

### AnyClaw 双循环核心

```go
// AnyClaw: 外层循环 - 仅处理 FollowUp
for {
    hasMoreToolCalls := true

    // 内层循环 - 包含工具执行和重试/故障转移
    for hasMoreToolCalls || len(pendingMessages) > 0 {
        iteration++
        if iteration > maxIterations { ... }

        // 处理 Steering
        // 调用 LLM (重试机制封装在此函数内)
        assistantMsg, err := o.streamAssistantResponseWithRetry(ctx, state, retryManager)
        // 执行工具
        results, steering := o.executeToolCalls(ctx, toolCalls, state)
        // 检查 Steering 中断
    }

    // 检查 FollowUp
    followUpMessages := o.fetchFollowUpMessages()
    if len(followUpMessages) > 0 {
        continue
    }
    break
}
```

### 关键差异

| 方面 | AnyClaw | OpenClaw |
|------|--------|----------|
| **外层循环职责** | 仅 FollowUp 消息处理 | 重试、故障转移、Profile 轮换、压缩 |
| **内层循环职责** | 工具执行 + 重试/故障转移 | 工具执行 (pi-agent-core) |
| **架构优势** | 重试逻辑封装在 LLM 调用层，职责清晰 | 恢复逻辑集中在外层，整体流程清晰 |
| **适用场景** | 工具调用密集型任务 | 重试/故障转移频繁场景 |
