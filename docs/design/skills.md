# anyclaw Skills 系统设计方案

> 参考 OpenClaw 的 Skills 机制，为 anyclaw 设计一套遵循 [AgentSkills](https://agentskills.io) 规范的技能系统。

## 设计理念

**Skill 是知识（Knowledge），而非直接的代码插件。**

与传统的 "Plugin = Function Call Tool" 模式不同，AgentSkills 的核心理念是 **"Prompt Injection" (提示词注入)**：
1. **加载**：系统加载 `SKILL.md`。
2. **注入**：将技能的使用说明注入到 Agent 的 System Prompt 中。
3. **执行**：LLM 阅读说明后，**主动调用现有的基础工具**（如 `run_shell`、`read_file`、`web_search`）来完成任务。

这种设计极大地降低了开发门槛：**只要你会写文档，你就能开发 Skill。**

## 核心架构

### 1. 技能定义格式 (SKILL.md)

遵循 [AgentSkills](https://agentskills.io) 规范，并兼容 OpenClaw 的元数据扩展。

```yaml
---
name: weather
description: Get current weather and forecasts via CLI.
homepage: https://wttr.in/:help

metadata:
  openclaw:
    emoji: "🌤️"
    requires:
      bins: ["curl"]           # 准入检查：仅在 PATH 中存在 curl 时加载
      anyBins: ["curl", "wget"] # 任一存在即可
      env: ["WEATHER_API_KEY"]  # 可选：要求特定环境变量
      config: ["weather.default_city"]  # 可选：要求配置项
      os: ["darwin", "linux"]   # 可选：限制操作系统
      pythonPkgs: ["requests"]  # Python 包依赖
      nodePkgs: ["axios"]       # Node.js 包依赖
    install:
      - id: curl-install
        kind: brew
        formula: curl
        bins: ["curl"]
        os: ["darwin"]
        label: "Install curl via Homebrew"
      - id: pip-install
        kind: pip
        package: requests
        os: ["darwin", "linux"]
---

# Weather Forecast

When the user asks about weather:
1. Use `run_shell` to execute: `curl wttr.in/{city}?format=3`
2. Parse the output and present it to the user
```

### 2. 技能加载器 (SkillsLoader)

```go
// SkillsLoader 技能加载器
type SkillsLoader struct {
    workspace      string
    skillsDirs     []string
    skills         map[string]*Skill
    alwaysSkills   []string
    autoInstall    bool          // 是否启用自动安装依赖
    installTimeout time.Duration // 安装超时时间
}

// Skill 技能定义
type Skill struct {
    Name        string
    Description string
    Version     string
    Author      string
    Homepage    string
    Always      bool
    Content     string       // 技能内容（Markdown）
    Metadata    struct {
        OpenClaw struct {
            Emoji    string
            Always   bool
            Requires struct {
                Bins       []string
                AnyBins    []string
                Env        []string
                Config     []string
                OS         []string
                PythonPkgs []string
                NodePkgs   []string
            }
            Install []SkillInstall
        }
    }
    MissingDeps *MissingDeps // 缺失的依赖信息
}

// MissingDeps 缺失的依赖信息
type MissingDeps struct {
    Bins       []string
    AnyBins    []string
    Env        []string
    PythonPkgs []string
    NodePkgs   []string
}

// SkillInstall 技能安装配置
type SkillInstall struct {
    ID      string   // 安装方式唯一标识
    Kind    string   // 安装方式: brew, apt, npm, pip, uv, go, node, pnpm, yarn, bun
    Formula string   // 包名 (brew, apt)
    Package string   // 包名 (npm, pip, go)
    Bins    []string // 安装后提供的可执行文件
    Label   string   // 安装说明
    OS      []string // 适用的操作系统
    Command string   // 自定义安装命令
}
```

### 3. 准入控制 (Gating)

Loader 不仅负责加载文本，还负责 **"Skill Gating" (技能准入过滤)**。

```
┌─────────────────────────────────────────────────────────────┐
│                      Skills Loader                          │
└─────────────────────────────────────────────────────────────┘
                            │
        ┌───────────────────┼───────────────────┐
        │                   │                   │
        ▼                   ▼                   ▼
   ┌─────────┐        ┌─────────┐        ┌─────────┐
   │Workspace│        │  User   │        │ Builtin │
   │ Skills  │        │ Skills  │        │ Skills  │
   └────┬────┘        └────┬────┘        └────┬────┘
        │                  │                  │
        └───────────────────┼──────────────────┘
                           │
                           ▼
                  ┌─────────────────┐
                  │  Scan & Parse   │
                  │  SKILL.md files │
                  └────────┬────────┘
                           │
                           ▼
                  ┌─────────────────┐
                  │ Dependency Check│
                  │  - bins in PATH │
                  │  - anyBins      │
                  │  - env vars set │
                  │  - OS match     │
                  │  - Python pkgs  │
                  │  - Node pkgs    │
                  └────────┬────────┘
                           │
              ┌────────────┴────────────┐
              │                         │
              ▼                         ▼
         ┌─────────┐              ┌─────────┐
         │  Valid  │              │ Invalid │
         │ Skills  │              │ (Skip)  │
         └────┬────┘              └─────────┘
              │
              ▼
     ┌─────────────────┐
     │ Apply Config    │
     │ (disabled list) │
     └────────┬────────┘
              │
              ▼
     ┌─────────────────┐
     │ Sort by Priority│
     └────────┬────────┘
              │
              ▼
     ┌─────────────────┐
     │ Inject into     │
     │ System Prompt   │
     └─────────────────┘
```

### 4. 与 Agent Loop 集成 (Context Injection)

技能通过 ContextBuilder 注入到系统提示词中。采用**两阶段注入**：

#### 第一阶段：技能摘要（所有可用技能）

```markdown
## Skills (mandatory)

Before replying: scan <available_skills> entries.
- If exactly one skill clearly applies: output a tool call `use_skill` with the skill name as parameter.
- If multiple could apply: choose the most specific one, then call `use_skill`.
- If no matching skill: use built-in tools or command tools of os.

<skill name="weather">
**Name:** weather
**Description:** Get current weather and forecasts via CLI.
**Missing Dependencies:**
  - Binary dependencies: [curl]
    You may need to install these tools first.
</skill>
```

#### 第二阶段：完整技能内容（选中的技能）

当 LLM 调用 `use_skill` 工具后，完整技能内容被注入：

```markdown
## Selected Skills (active)

<skill name="weather">
### weather
> Description: Get current weather and forecasts via CLI.

**⚠️ MISSING DEPENDENCIES - Install before using:**

**Binary dependencies:** [curl]
You may need to install these tools first.

# Weather Forecast

When the user asks about weather:
1. Use `run_shell` to execute: `curl wttr.in/{city}?format=3`
2. Parse the output and present it to the user
</skill>
```

### 5. 依赖检查与自动安装

#### 依赖类型

| 类型 | 字段 | 检查方式 |
|------|------|----------|
| 二进制 | `bins` | `exec.LookPath()` |
| 任一二进制 | `anyBins` | 任一存在即可 |
| 环境变量 | `env` | `os.Getenv()` |
| 配置项 | `config` | 配置文件检查 |
| 操作系统 | `os` | `runtime.GOOS` |
| Python 包 | `pythonPkgs` | `python3 -c "import pkg"` |
| Node.js 包 | `nodePkgs` | `npm list --global --json` |

#### 安装方式

```yaml
install:
  # Homebrew (macOS)
  - kind: brew
    formula: curl
    bins: ["curl"]
    os: ["darwin"]

  # apt (Debian/Ubuntu)
  - kind: apt
    formula: curl
    bins: ["curl"]
    os: ["linux"]

  # npm
  - kind: npm
    package: axios
    bins: ["axios"]

  # pnpm
  - kind: pnpm
    package: axios

  # yarn
  - kind: yarn
    package: axios

  # bun
  - kind: bun
    package: axios

  # pip
  - kind: pip
    package: requests

  # uv
  - kind: uv
    package: requests

  # go
  - kind: go
    package: github.com/cli/cli/cmd/gh@latest

  # 自定义命令
  - kind: command
    command: "curl -fsSL https://get.docker.com | sh"
    bins: ["docker"]
```

### 6. 技能优先级

#### 加载优先级（由高到低）

1. **Workspace Skills**: `${WORKSPACE}/skills`
2. **User Skills**: `~/.anyclaw/skills`
3. **Builtin Skills**: 随二进制分发的内置技能

当出现同名技能时，高优先级的会覆盖低优先级的。

#### 技能加载路径

```go
// 技能按以下顺序加载，同名技能后面的会覆盖前面的
skillsDirs := []string{
    filepath.Join(homeDir, ".anyclaw", "skills"),     // 用户全局目录（最低优先级）
    filepath.Join(workspace, "skills"),               // 工作区目录
    "./skills",                                       // 当前目录（最高优先级）
}
```

### 7. use_skill 工具

```go
// SkillTool 技能工具
type SkillTool struct {
    skillsLoader *agent.SkillsLoader
    context      *agent.ContextBuilder
}

func (t *SkillTool) Name() string {
    return "use_skill"
}

func (t *SkillTool) Description() string {
    return "Load a specialized skill. SKILLS HAVE HIGHEST PRIORITY - always check Skills section first."
}

func (t *SkillTool) Parameters() map[string]any {
    return map[string]any{
        "type": "object",
        "properties": map[string]any{
            "skill_name": map[string]any{
                "type":        "string",
                "description": "Name of the skill to load",
            },
        },
        "required": []string{"skill_name"},
    }
}

func (t *SkillTool) Execute(ctx context.Context, params map[string]any, onUpdate func(ToolResult)) (ToolResult, error) {
    skillName := params["skill_name"].(string)
    // 加载技能并注入到上下文
    // ...
}
```

## SKILL.md 编写最佳实践

### 结构模板

```markdown
---
name: skill-name
description: 一句话描述技能功能
homepage: https://example.com/skill-docs
metadata:
  openclaw:
    emoji: "🔧"
    requires:
      bins: ["required-command"]
      env: ["OPTIONAL_API_KEY"]
    install:
      - kind: brew
        formula: required-command
        bins: ["required-command"]
        os: ["darwin"]
---

# 技能名称

简短介绍这个技能做什么以及何时使用。

## 使用场景

当用户提到以下关键词时使用此技能：
- 关键词 1
- 关键词 2

## 执行步骤

1. **步骤一**：使用 `run_shell` 做什么
   ```bash
   command example
   ```

2. **步骤二**：根据结果做决策
   - 如果结果为 A，执行 X
   - 如果结果为 B，执行 Y

3. **步骤三**：输出最终结果

## 重要提示

> **注意**：特殊情况的说明

## 错误处理

- 如果命令失败，尝试替代方案
- 如果资源不存在，通知用户
```

### 编写原则

1. **明确具体**：给出具体的命令示例
   ```markdown
   # ❌ 不好
   使用 git 检查状态

   # ✅ 好
   使用 `run_shell` 工具执行：`git status --short`
   ```

2. **分步骤说明**：将复杂任务分解为清晰的步骤

3. **使用代码块**：所有命令都应该在代码块中

4. **说明输出格式**：告诉 LLM 期望的输出格式

5. **边界条件**：说明特殊情况如何处理

## CLI 命令

```bash
# ========== 技能管理 ==========
# 列出所有已加载的技能
anyclaw skills list

# 详细模式（显示依赖信息）
anyclaw skills list --verbose
anyclaw skills list -v

# 只列出已就绪的技能（无缺失依赖）
anyclaw skills list --eligible

# 查看技能详情
anyclaw skills info <skill-name>

# 检查技能依赖
anyclaw skills check

# 验证技能依赖
anyclaw skills validate <skill-name>

# 安装技能依赖
anyclaw skills install-deps <skill-name>

# ========== 调试 ==========
# 打印完整 System Prompt
anyclaw agent --message "测试" --thinking

# 启用详细日志
GOCRAW_LOG_LEVEL=debug anyclaw start
```

## 日志输出示例

启用详细日志可以看到技能加载过程：

```bash
GOCRAW_LOG_LEVEL=debug anyclaw start
```

输出示例：
```
[DEBUG] Loading skills from: /Users/user/.anyclaw/skills
[DEBUG] Found skill: git-helper
[DEBUG] Checking dependencies for git-helper...
[DEBUG]   - Checking binary: git ✓
[DEBUG] Skill git-helper loaded successfully
[DEBUG] Checking Python packages for weather-skill...
[WARN] Missing Python package: requests
[DEBUG] Injecting 3 skills into system prompt
[INFO] System prompt size: 2,456 tokens
```

## 自定义二进制 (Binaries)

有些技能不仅仅是 Prompt，还包含自定义脚本或二进制文件。

### 目录结构

```
skills/my-db-helper/
├── SKILL.md
└── bin/
    └── db-cli  (可执行文件)
```

### 处理逻辑

1. Loader 发现技能目录下有 `bin/` 文件夹。
2. Loader 将该 `bin/` 目录的绝对路径加入到 Agent 运行时的 `PATH` 环境变量中。
3. `SKILL.md` 中写明：
   > "Use the `db-cli` command via `run_shell` tool to interact with the database."
4. LLM 调用 `run_shell(command="db-cli status")`。
5. 系统在 PATH 中找到了 `db-cli` 并执行。

## 安全考虑

### 基础层控制

1. **Shell 工具配置**：
   ```json
   {
     "tools": {
       "shell": {
         "enabled": true,
         "denied_cmds": ["rm -rf", "dd", "mkfs", "format"]
       }
     }
   }
   ```

2. **Docker 沙箱**：
   ```json
   {
     "tools": {
       "shell": {
         "sandbox": {
           "enabled": true,
           "image": "anyclaw/sandbox:latest",
           "network": "none"
         }
       }
     }
   }
   ```

### 技能层控制

- `requires.os`: 限制操作系统
- `requires.env`: 要求环境变量
- `requires.config`: 要求配置项

## 总结

此方案回归了 AgentSkills 的本质：**Prompt Engineering at Scale**。

| 特性 | 旧方案 (Tool Wrap) | 新方案 (Prompt Injection) |
| :--- | :--- | :--- |
| **实现难度** | 高 (需解析 Markdown 自动生成 Tool) | 低 (仅需字符串拼接) |
| **灵活性** | 低 (参数被写死) | 高 (LLM 自由组合命令) |
| **兼容性** | 差 (特有协议) | 优 (兼容 OpenClaw/AgentSkills) |
| **维护性** | 差 (依赖代码绑定) | 优 (完全解耦，热更新) |
| **调试性** | 中 (需查看 Tool 定义) | 高 (直接查看 Prompt) |
| **安全性** | 独立控制层 | 依赖基础工具配置 ⚠️ |
| **热更新** | 需重新编译 | 修改文本即可 ✅ |
| **依赖管理** | 手动 | 自动检查 + 安装 ✅ |
