# AnyClaw Architecture

## Overview

AnyClaw is a local-first AI agent workspace written in Go, inspired by OpenClaw's architecture. It provides a complete personal AI assistant that runs on your own devices with support for 20+ messaging channels, a plugin/extension system, skills, and file-first memory.

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                        AnyClaw Architecture                      │
├─────────────────────────────────────────────────────────────────┤
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │
│  │    CLI       │  │   Gateway    │  │  Control UI  │          │
│  │  (cmd/)      │  │  (HTTP/WS)   │  │  (ui/)       │          │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘          │
│         │                 │                  │                   │
│         └─────────────────┴──────────────────┘                   │
│                           │                                      │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │                   Core Runtime (pkg/)                     │   │
│  │                                                          │   │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐ │   │
│  │  │  Agent   │  │  Tools   │  │ Channels │  │ Sessions │ │   │
│  │  │ Runtime  │  │ Registry │  │ Manager  │  │ Manager  │ │   │
│  │  └──────────┘  └──────────┘  └──────────┘  └──────────┘ │   │
│  │                                                          │   │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐ │   │
│  │  │  Plugin  │  │  Skills  │  │  Memory  │  │  Event   │ │   │
│  │  │ Registry │  │ Manager  │  │  Store   │  │   Bus    │ │   │
│  │  └──────────┘  └──────────┘  └──────────┘  └──────────┘ │   │
│  │                                                          │   │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐ │   │
│  │  │  Config  │  │  Hooks   │  │  Prompt  │  │  Routing │ │   │
│  │  │ Manager  │  │ Manager  │  │ Builder  │  │  Engine  │ │   │
│  │  └──────────┘  └──────────┘  └──────────┘  └──────────┘ │   │
│  └──────────────────────────────────────────────────────────┘   │
│                           │                                      │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │              Extensions (extensions/)                     │   │
│  │                                                          │   │
│  │  telegram  discord  slack  whatsapp  signal  irc  matrix  │   │
│  │  wechat    feishu   line   msteams   googlechat           │   │
│  └──────────────────────────────────────────────────────────┘   │
│                           │                                      │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │              Storage Layer                                │   │
│  │                                                          │   │
│  │  .anyclaw/     Memory JSON files + daily markdowns       │   │
│  │  .anyclaw/gateway/  Gateway state (sessions, tasks)       │   │
│  │  workflows/    Bootstrap files (AGENTS.md, SOUL.md...)    │   │
│  │  plugins/      Runtime-loaded plugins                     │   │
│  │  skills/       Bundled skill definitions                  │   │
│  └──────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

## Directory Structure

```
anyclaw/
├── cmd/anyclaw/              # CLI entrypoint (multi-command)
│   ├── main.go               # Main entry, command dispatch
│   ├── agent_cli.go          # Agent subcommands
│   ├── channels_cli.go       # Channel management
│   ├── config_cli.go         # Config management
│   ├── gateway_cli.go        # Gateway start/stop/status
│   ├── gateway_http.go       # Gateway HTTP handlers
│   ├── plugin_cli.go         # Plugin management
│   ├── skill_cli.go          # Skill management
│   ├── setup_cli.go          # Setup/onboarding
│   └── ...                   # Other CLI commands
│
├── pkg/                      # Core packages (Go standard layout)
│   ├── agent/                # Agent runtime (run loop, tool calls)
│   ├── agents/               # Agent definitions
│   ├── agentstore/           # Agent store/installation
│   ├── apps/                 # App runtime, bindings, pairings
│   ├── audit/                # Audit logging
│   ├── auto-reply/           # Auto-reply pipeline
│   ├── cdp/                  # Chrome DevTools Protocol
│   ├── channel/              # Channel compatibility shim
│   ├── channels/             # Channel adapters (core)
│   ├── chat/                 # Chat handling
│   ├── clawbridge/           # Claw-code reference surface
│   ├── cliadapter/           # CLI adapter system
│   ├── clihub/               # CLI harness catalog
│   ├── config/               # Configuration system
│   ├── context/              # Context engine
│   ├── context-engine/       # Context engine abstraction
│   ├── cron/                 # Cron scheduler
│   ├── enterprise/           # Enterprise features (SSO, vector)
│   ├── event/                # Event bus
│   ├── extension/            # Extension loading & management
│   ├── gateway/              # HTTP/WebSocket gateway server
│   ├── hooks/                # Hook system (message/tool/agent)
│   ├── i18n/                 # Internationalization
│   ├── llm/                  # LLM compatibility shim
│   ├── media/                # Media handling
│   ├── memory/               # File-first memory + hybrid search
│   ├── nodes/                # Node system
│   ├── orchestrator/         # Multi-agent orchestrator
│   ├── pi/                   # Personal intelligence
│   ├── plugin/               # Plugin system
│   ├── prompt/               # System prompt builder
│   ├── providers/            # LLM provider management
│   ├── reply/                # Reply handling
│   ├── routing/              # LLM routing logic
│   ├── runtime/              # Bootstrap/runtime orchestration
│   ├── sdk/                  # SDK
│   ├── security/             # Security utilities
│   ├── session/              # Session management
│   ├── setup/                # Setup/onboarding
│   ├── skills/               # Skill loading/execution
│   ├── speech/               # Speech handling
│   ├── task/                 # Task management
│   ├── tools/                # Tool registry and builtins
│   ├── ui/                   # Terminal UI utilities
│   ├── verification/         # Verification/integration testing
│   ├── workflow/             # Workflow graph engine
│   └── workspace/            # Workspace bootstrap/rituals
│
├── extensions/               # Channel extensions (OpenClaw-style)
│   ├── telegram/             # Telegram channel extension
│   ├── discord/              # Discord channel extension
│   ├── slack/                # Slack channel extension
│   ├── whatsapp/             # WhatsApp channel extension
│   ├── signal/               # Signal channel extension
│   ├── irc/                  # IRC channel extension
│   ├── matrix/               # Matrix channel extension
│   ├── wechat/               # WeChat channel extension
│   ├── feishu/               # Feishu/Lark channel extension
│   ├── line/                 # LINE channel extension
│   ├── msteams/              # Microsoft Teams extension
│   └── googlechat/           # Google Chat extension
│
├── skills/                   # Bundled skills
│   └── web-search/           # Web search skill
│
├── plugins/                  # Plugin directory (runtime-loaded)
│
├── workflows/personal/       # Workspace bootstrap files
│   ├── AGENTS.md             # Agent definitions
│   ├── SOUL.md               # Personality/soul
│   ├── IDENTITY.md           # Identity definition
│   ├── MEMORY.md             # Memory config
│   ├── TOOLS.md              # Tool definitions
│   ├── USER.md               # User profile
│   ├── HEARTBEAT.md          # Heartbeat config
│   └── memory/               # Daily memory files
│
├── ui/                       # Web Control UI
├── docs/                     # Documentation
├── scripts/                  # Build/dev scripts
│
├── anyclaw.json              # Runtime configuration
├── go.mod / go.sum           # Go module definition
├── package.json              # Node.js UI workspace
├── Dockerfile                # Container definition
└── docker-compose.yml        # Docker compose
```

## Core Components

### 1. CLI Layer (`cmd/anyclaw/`)

Multi-command CLI with 20+ subcommands:
- **Interactive mode**: `anyclaw -i` for conversational interface
- **Gateway**: `anyclaw gateway start` for daemon mode
- **Management**: config, skills, plugins, channels, models, agents, cron, tasks
- **Diagnostics**: `doctor`, `status`, `health`

### 2. Agent Runtime (`pkg/agent/`)

The core reasoning engine with:
- **Run loop**: User input → intent preprocessor → system prompt → LLM chat → tool execution → response
- **Tool call parsing**: Native LLM tool calls + regex-based text fallback
- **Evidence-based execution**: Inspect → plan → execute → verify → adapt
- **Max tool calls**: 10 per turn to prevent infinite loops

### 3. Gateway (`pkg/gateway/`)

HTTP + WebSocket server with:
- **50+ WebSocket RPC methods**: chat, agents, sessions, tasks, tools, plugins, config, channels
- **Challenge-handshake auth**: Nonce verification before method access
- **Permission model**: RBAC with hierarchical access checks
- **State persistence**: Sessions, tasks, events, approvals saved to `.anyclaw/gateway/state.json`

### 4. Channels (`pkg/channels/` + `extensions/`)

Multi-platform messaging with **extension architecture** (OpenClaw-style):
- **Core channels**: Telegram, Discord, Slack, WhatsApp, Signal, IRC (built-in adapters)
- **Extension channels**: Matrix, WeChat, Feishu, LINE, MS Teams, Google Chat (plugin-based)
- **Each extension**: `anyclaw.extension.json` manifest + standalone Go adapter
- **Communication**: stdin/stdout JSON protocol for external process plugins
- **Polling model**: Most channels use configurable polling intervals

### 5. Plugin System (`pkg/plugin/`)

Manifest-driven, process-isolated plugins:
- **Plugin kinds**: tool, channel, app, node, surface, ingress
- **Execution**: External processes with `ANYCLAW_PLUGIN_INPUT` env var
- **Trust system**: SHA-256 signature verification with trusted signers
- **Permission model**: `tool:exec`, `fs:read`, `fs:write`, `net:out`

### 6. Extension System (`pkg/extension/`)

OpenClaw-style extension architecture:
- **Discovery**: Scan `extensions/` directory for `anyclaw.extension.json` manifests
- **Manifest**: ID, name, version, kind, channels, entrypoint, permissions, config schema
- **Registry**: Load, enable/disable, list by kind
- **Runtime**: External process execution with stdin/stdout JSON protocol

### 7. Skills (`pkg/skills/`)

Reusable capability packages:
- **Format**: `skill.json` (metadata) + `SKILL.md` (human-readable)
- **Execution**: External processes (Python, Node.js, shell, PowerShell)
- **Tool registration**: Each skill registers as `skill_<name>` in tool registry
- **System prompts**: Skills contribute prompt fragments

### 8. Memory (`pkg/memory/`)

File-first memory with **hybrid search** (OpenClaw-style):
- **FileMemory**: JSON files organized by type (conversation, reflection, fact)
- **Daily markdown**: Conversations appended to `YYYY-MM-DD.md` files
- **Hybrid search**: Keyword (TF-IDF) + provider-backed vector similarity + temporal decay
- **MMR ranking**: Maximal Marginal Relevance for diverse results
- **Temporal decay**: Exponential decay with configurable half-life (default 7 days)

### 9. Hooks (`pkg/hooks/`)

Event-driven interceptors (OpenClaw-style):
- **Message hooks**: `message:inbound`, `message:outbound`, `message:sent`
- **Tool hooks**: `tool:call`, `tool:result`, `tool:error`
- **Agent hooks**: `agent:start`, `agent:stop`, `agent:think`, `agent:error`
- **Session hooks**: `session:create`, `session:close`, `session:message`
- **Lifecycle hooks**: `gateway:start`, `gateway:stop`, `compaction:before/after`
- **Middleware**: Composable middleware chain with timeout support

### 10. Config (`pkg/config/`)

Comprehensive configuration system:
- **14 top-level sections**: LLM, Agent, Providers, Skills, Memory, Gateway, Daemon, Channels, Plugins, Sandbox, Security, Orchestrator
- **Provider profiles**: Multiple LLM providers with capabilities
- **Agent profiles**: Named profiles with personality specs
- **Environment overrides**: `ANYCLAW_LLM_PROVIDER`, `ANYCLAW_LLM_API_KEY`, etc.
- **Validation**: Config validation with error reporting

### 11. Tools (`pkg/tools/`)

Agent action layer with 25+ files:
- **Registry**: Thread-safe tool registration with categories and access levels
- **Builtin tools**: `read_file`, `write_file`, `list_directory`, `search_files`, `run_command`, web fetch, browser control
- **Browser tools**: Chrome DevTools Protocol via `chromedp`
- **Desktop tools**: UI automation, OCR, image matching, window management
- **Policy engine**: Path-based access control and permission levels
- **Sandbox**: Execution sandbox with local and Docker backends

### 12. Routing (`pkg/routing/`)

LLM routing based on keyword matching:
- **Reasoning route**: Complex/plan/code → reasoning provider
- **Fast route**: Simple queries → fast provider
- **Configuration**: Rules in `anyclaw.json` under `llm.routing`

## Initialization Flow

```
main()
  └── run()
       └── switch command
            ├── interactive: runRootCommand()
            │    ├── ensureConfigOnboarded()
            │    ├── config.Load()
            │    ├── appRuntime.Bootstrap()
            │    │    ├── Phase 1: Config
            │    │    ├── Phase 2: Storage
            │    │    ├── Phase 3: Security
            │    │    ├── Phase 4: Skills
            │    │    ├── Phase 5: Tools
            │    │    ├── Phase 6: Plugins
            │    │    ├── Phase 7: LLM
            │    │    └── Phase 8: Agent
            │    ├── rebindBuiltins()
            │    └── runInteractive()
            │
            └── gateway: gatewayCLI.Start()
                 ├── appRuntime.Bootstrap()
                 ├── Gateway server init
                 ├── Channel adapters start
                 └── WebSocket + HTTP listeners
```

## Design Patterns

| Pattern | Where Used |
|---------|-----------|
| Phase-Based Bootstrap | `runtime.Bootstrap()` - 9 ordered phases |
| Publish-Subscribe | `event.EventBus` - decoupled component communication |
| Registry | `tools.Registry`, `plugin.Registry`, `extension.Registry` |
| Plugin Architecture | Manifest-driven, process-isolated plugins |
| Extension Architecture | OpenClaw-style `extensions/` with manifests |
| Strategy | Config validators/migrators, LLM providers |
| File-First Storage | Memory as JSON files + daily markdown |
| Evidence-Based Execution | Agent inspect→execute→verify loops |
| Hierarchical Resources | Org → Project → Workspace |
| Hook System | Message/tool/agent lifecycle interceptors |
| Middleware Chain | Hooks with composable middleware |
| Hybrid Search | Keyword + vector + temporal decay + MMR |

## Differences from OpenClaw

| Aspect | AnyClaw (Go) | OpenClaw (TypeScript) |
|--------|-------------|----------------------|
| Language | Go (compiled, single binary) | TypeScript (Node.js runtime) |
| Module System | Go packages (`pkg/`) | pnpm workspaces (packages, extensions) |
| Plugin Loading | External process (stdin/stdout JSON) | jiti runtime transpilation |
| Distribution | Single static binary | npm package |
| Concurrency | Goroutines + channels | Async/await + event emitters |
| Memory | File-first JSON + markdown | SQLite + sqlite-vec + LanceDB |
| Build | `go build` | `tsdown` bundler + Vite |
| Channels | Built-in + extension processes | Extension packages (44+) |
| UI | Embedded dashboard + Lit SPA | Lit SPA served by gateway |
| Native Apps | Not yet | Android, iOS, macOS apps |

## Supported Channels

| Channel | Status | Type |
|---------|--------|------|
| Telegram | ✅ Built-in | Polling |
| Discord | ✅ Built-in | Polling + Webhook |
| Slack | ✅ Built-in | Polling |
| WhatsApp | ✅ Built-in | Webhook |
| Signal | ✅ Built-in | Polling (signal-cli) |
| IRC | ✅ Built-in | Persistent connection |
| Matrix | 📦 Extension | Polling |
| WeChat | 📦 Extension | Webhook |
| Feishu/Lark | 📦 Extension | Webhook |
| LINE | 📦 Extension | Webhook |
| MS Teams | 📦 Extension | Webhook |
| Google Chat | 📦 Extension | Webhook |

## Getting Started

```bash
# Build
go build -o anyclaw ./cmd/anyclaw

# Setup
./anyclaw --setup

# Interactive mode
./anyclaw -i

# Gateway mode
./anyclaw gateway start
```

## Configuration

Runtime configuration is stored in `anyclaw.json`:

```json
{
  "llm": {
    "provider": "openai",
    "model": "gpt-4",
    "api_key": "sk-..."
  },
  "channels": {
    "telegram": {
      "enabled": true,
      "bot_token": "...",
      "poll_every": 3
    }
  },
  "memory": {
    "backend": "file"
  },
  "gateway": {
    "port": 18789
  }
}
```

## Version

`2026.3.13`
