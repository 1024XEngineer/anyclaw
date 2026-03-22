# AnyClaw - File-first Memory AI Agent

A lightweight, transparent AI Agent system focused on file-based memory and skills-as-plugins architecture.

## Philosophy

- **File-first Memory**: No opaque vector databases. Every conversation, reflection, and fact is stored in human-readable Markdown/JSON files.
- **Skills as Plugins**: Follow Anthropic's Agent Skills paradigm. Drop a folder, it's ready to use.
- **Transparent & Controllable**: All system prompt logic, tool calls, and memory operations are visible to developers.

## Features

- **File-based Memory System**: All memories stored as readable Markdown/JSON files
- **Skills Plugin System**: Load capabilities from folder structures
- **Built-in Tools**: read_file, write_file, list_directory, search_files, run_command, get_time, web_search
- **Browser Automation**: navigate pages, click/type, upload files, take screenshots, inspect HTML, export PDFs
- **Sandboxed Execution**: session/channel isolated command execution with local sandbox directories or Docker backends
- **Multi-Provider LLM Support**: OpenAI, Anthropic, Ollama, Qwen, and compatible APIs
- **Conversation History**: Automatic storage with reflection capabilities
- **Channel Integrations**: Telegram, Slack, Discord, Signal, and WhatsApp webhook
- **Session Governance**: main/group session isolation, queue modes, reply-back, presence, and typing indicators
- **Skill Platform**: installable/versioned skills with permissions, entrypoints, source metadata, and registry/catalog support

## Quick Start

```bash
# Set your API key
export OPENAI_API_KEY=sk-...

# Build
go build -o anyclaw ./cmd/anyclaw

# Run in interactive mode
./anyclaw

# Or with a single message
./anyclaw "Hello, what can you do?"
```

## Configuration

Edit `anyclaw.json`:

```json
{
  "agent": {
    "name": "AnyClaw",
    "description": "Your AI assistant"
  },
  "llm": {
    "provider": "openai",
    "model": "gpt-4o-mini",
    "api_key": "your-api-key"
  }
}
```

## Switching Providers & Models

### Command Line

```bash
# Set provider
./anyclaw --provider anthropic

# Set model
./anyclaw --model claude-sonnet-4-7

# Set API key
./anyclaw --api-key sk-ant-...

# Show available providers
./anyclaw --providers

# Show models for a provider
./anyclaw --models openai
./anyclaw --models anthropic
```

### Interactive Mode Commands

```
/provider        - Show current provider/model
/set provider    - Switch LLM provider
/set model      - Switch model
/set apikey     - Set API key
/set temperature - Set temperature
/providers      - Show available providers
/models <name>  - Show models for provider
```

### Supported Providers & Models

**OpenAI:**
- gpt-4o, gpt-4o-mini, gpt-4-turbo, gpt-4, gpt-3.5-turbo

**Anthropic:**
- claude-opus-4-5, claude-sonnet-4-7, claude-haiku-3-5

**Qwen (通义千问):**
- qwen-plus, qwen-turbo, qwen-max
- qwen2.5-72b-instruct, qwen2.5-32b-instruct, qwen2.5-14b-instruct
- qwq-32b-preview, qwen-coder-plus

**Ollama (local):**
- llama3.2, llama3.1, codellama, mistral

**OpenAI-Compatible:**
- Any API compatible with OpenAI format

## Working Directory

AnyClaw can manage files in the `working/` directory to complete your tasks:

```
working/           # Your working files
├── projects/     # Project folders
├── docs/         # Documents
├── scripts/      # Scripts
└── data/        # Data files
```

Example tasks:
```
> 请在 working 目录下创建一个 Python 项目
> 帮我整理 working 目录中的文件
> 把代码保存到 working/project.py
```

## Memory System

All memories are stored in `.anyclaw/memory/`:

```
.anyclaw/
└── memory/
    ├── conversations/    # User/assistant exchanges
    ├── reflections/      # Agent self-reflections
    ├── facts/           # User-provided facts
    └── index.json       # Memory index
```

## Skills System

Skills are loaded from the `skills/` directory:

```
skills/
├── file-operations/
│   └── skill.json
└── coder/
    └── skill.json
```

### SkillHub Store (skills.sh)

AnyClaw supports the [skills.sh](https://skills.sh) ecosystem:

```bash
# Search for skills
anyclaw skill search react

# Install from skills.sh
anyclaw skill install vercel-labs/agent-skills/web-design-guidelines

# Install built-in skills
anyclaw skill install coder

# List installed skills
anyclaw skill list
```

### Built-in Skills

| Skill | Description |
|-------|-------------|
| coder | Code generation and analysis assistant |
| researcher | Web research and information gathering |
| writer | Content writing and editing assistant |
| analyst | Data analysis and visualization |
| translator | Multilingual translation assistant |

### Popular skills.sh Skills

| Skill | Installs | Description |
|-------|----------|-------------|
| vercel-react-best-practices | 225.9K | React best practices |
| web-design-guidelines | 179.8K | Web design guidelines |
| remotion-best-practices | 156.8K | Remotion animation |
| pdf | 43.0K | PDF processing |
| docx | 33.9K | Word document handling |

### Skill Definition

A skill.json example:

```json
{
  "name": "my-skill",
  "description": "What this skill does",
  "prompts": {
    "system": "Instructions for the agent when this skill is active"
  }
}
```

## Available Tools

| Tool | Description |
|------|-------------|
| read_file | Read file contents |
| write_file | Write content to a file |
| list_directory | List directory contents |
| search_files | Search for files by pattern |
| run_command | Execute shell commands |
| get_time | Get current date/time |
| web_search | Search the web using DuckDuckGo |
| fetch_url | Fetch and extract text content from a URL |
| browser_navigate | Open a page in a browser session |
| browser_click | Click a page element |
| browser_type | Type into a field |
| browser_upload | Upload a local file |
| browser_wait | Wait for page or element readiness |
| browser_select | Set/select a form value |
| browser_press | Press a keyboard key |
| browser_scroll | Scroll page or element |
| browser_download | Download a linked resource |
| browser_screenshot | Save a screenshot |
| browser_snapshot | Inspect title/URL/HTML |
| browser_eval | Run JavaScript in page context |
| browser_pdf | Export page to PDF |
| browser_tab_new | Create a new browser tab |
| browser_tab_list | List browser tabs |
| browser_tab_switch | Switch active tab |
| browser_tab_close | Close a specific tab |
| browser_close | Close browser session |

Browser session behavior:

- browser tools now default to the active chat/session id when `session_id` is omitted
- gateway and channel sessions automatically run agent browser actions inside a session-bound browser context
- this makes each AnyClaw session behave like its own browser tab/workflow unless you explicitly override `session_id`
- each browser session now supports multiple tabs/pages; most browser tools accept optional `tab_id`
- if `tab_id` is omitted, the active tab for that chat session is used automatically

Sandbox behavior:

- `run_command` can be isolated per session/channel when `sandbox.enabled` is turned on
- `sandbox.backend: local` runs commands inside per-scope filesystem sandboxes under `sandbox.base_dir`
- `sandbox.backend: docker` runs commands via `docker exec` inside per-scope containers
- gateway/channel requests automatically inject session and channel scope into tool execution

Example sandbox config:

```json
{
  "sandbox": {
    "enabled": true,
    "backend": "docker",
    "base_dir": ".anyclaw/sandboxes",
    "docker_image": "alpine:3.20",
    "docker_network": "none",
    "reuse_per_scope": true
  }
}
```

## Architecture

AnyClaw is moving from a single-agent runtime into a local-first AI agent platform. The optimized architecture keeps the existing Go runtime, file memory, gateway, channels, and tool system, then layers assistant management, permission governance, execution orchestration, and auditability on top.

### Target architecture

```text
┌──────────────────────────────────────────────────────────────────────┐
│                         AnyClaw Control Plane                       │
├──────────────────────────────────────────────────────────────────────┤
│  CLI / Open APIs / Channel Connectors                               │
└───────────────┬───────────────────────────────┬──────────────────────┘
                │                               │
                ▼                               ▼
┌──────────────────────────────┐   ┌──────────────────────────────────┐
│ Assistant Management Layer   │   │ Task & Session Orchestration     │
│ - assistant profiles         │   │ - session lifecycle              │
│ - persona + skill binding    │   │ - plan / execute / recover       │
│ - model selection            │   │ - queue / job workers            │
│ - workspace binding          │   │ - runtime pool                   │
└───────────────┬──────────────┘   └───────────────┬──────────────────┘
                │                                  │
                └───────────────┬──────────────────┘
                                ▼
┌──────────────────────────────────────────────────────────────────────┐
│                         Agent Runtime Core                          │
├──────────────────────────────────────────────────────────────────────┤
│ Prompt Builder | LLM Router | Tool Registry | Skill Runtime         │
│ Context Assembly | Memory Access | Observer Hooks | Safety Guards   │
└───────────────┬───────────────────────────────┬──────────────────────┘
                │                               │
                ▼                               ▼
┌──────────────────────────────┐   ┌──────────────────────────────────┐
│ Permission & Security Layer  │   │ Memory & Data Layer              │
│ - scope / workspace ACL      │   │ - conversations                  │
│ - dangerous command policy   │   │ - facts / reflections            │
│ - confirmation checkpoints   │   │ - task records                   │
│ - sandbox isolation          │   │ - assistant config               │
└───────────────┬──────────────┘   │ - audit/event indexes            │
                │                  └──────────────────────────────────┘
                ▼
┌──────────────────────────────────────────────────────────────────────┐
│                     Execution & Integration Layer                   │
├──────────────────────────────────────────────────────────────────────┤
│ File tools | Command tools | Browser tools | Web tools | Plugins    │
│ Telegram | Slack | Discord | WhatsApp | Signal                      │
└──────────────────────────────────────────────────────────────────────┘
```

### Layer responsibilities

- `Experience layer`: expose AnyClaw through CLI, APIs, and external channels so the same assistant can participate in chat, task execution, and operational workflows.
- `Assistant management`: treat assistants as first-class resources with name, role, persona, model, skills, workspace, and permission boundary instead of a single global agent configuration.
- `Task orchestration`: split work into session state, execution plans, queued jobs, retries, runtime allocation, and long-running task visibility.
- `Runtime core`: keep `pkg/agent`, `pkg/runtime`, `pkg/tools`, `pkg/skills`, and `pkg/routing` as the execution engine, but make them serve multiple assistants and multiple workspaces consistently.
- `Security and memory`: enforce least privilege before tool execution and persist everything needed for long-term collaboration, replay, and trust.
- `Integration layer`: isolate tool adapters and channel adapters so new capabilities can be added without coupling product logic to transport logic.

### Suggested bounded modules

The current codebase already contains the seeds of this architecture. To align it with the project report, the system should be organized around these modules:

| Module | Main responsibility | Current related packages | Recommended evolution |
|-------|----------------------|--------------------------|-----------------------|
| Assistant Center | manage assistant definitions, profiles, defaults, status | `pkg/config`, `pkg/runtime` | add persistent assistant registry and assistant-scoped config loading |
| Persona & Skills Center | persona prompts, skill binding, capability composition | `pkg/skills`, `pkg/prompt` | split built-in persona templates from installable executable skills |
| Workspace & Permission Center | working directory binding, scope checks, confirmations, sandbox policy | `pkg/tools`, `pkg/gateway/auth.go`, `pkg/gateway/state.go` | add explicit workspace ACL model and per-tool authorization policy |
| Task Orchestrator | planning, job queue, retries, approval checkpoints, runtime dispatch | `pkg/gateway/gateway.go`, `pkg/gateway/state.go` | promote jobs into a formal task pipeline with status transitions |
| Memory Center | conversation, fact, reflection, assistant memory, task records | `pkg/memory` | add assistant-scoped and workspace-scoped storage partitions |
| Audit Center | operation logs, tool trace, approvals, replay | `pkg/audit`, `pkg/gateway/state.go` | unify JSONL audit with gateway event timeline for replay views |
| Channel & API Gateway | webhook, bot channels, control APIs | `pkg/gateway`, `pkg/channel` | separate user-facing API, control-plane API, and channel ingress |
| Tool & Integration Hub | built-in tools, browser, command execution, external plugins | `pkg/tools`, `pkg/plugin` | introduce capability tags, risk levels, and approval metadata |

### End-to-end execution flow

1. The user enters AnyClaw from CLI, an API client, or an external channel.
2. The gateway resolves identity, organization, project, workspace, and target assistant.
3. Assistant Center loads persona, model strategy, skill set, memory scope, and permission profile.
4. Task Orchestrator decides whether the request is a direct response, a multi-step task, or a queued/background job.
5. Agent Runtime builds context from prompt, history, memory, and active skills.
6. LLM Router selects the appropriate model, then Runtime executes tool calls through the registry.
7. Permission Center validates tool scope, dangerous command patterns, sandbox policy, and confirmation checkpoints.
8. Results, tool traces, and audit events are persisted and streamed back to the user interface.

### Data model optimization

To support "create, configure, authorize, execute, audit, collaborate" as product primitives, the platform should gradually shift from runtime-only state to explicit domain entities:

- `Assistant`: id, name, role, persona, default model, enabled skills, permission profile, default workspace, status.
- `Workspace`: id, org/project relation, local path, allowed tools, sandbox policy, retention rules.
- `Session`: assistant binding, user/channel metadata, context window, execution state, replay pointers.
- `Task`: goal, plan, current step, priority, approval state, retry state, final result.
- `Audit Event`: actor, action, target, risk level, confirmation record, tool result, timestamp.
- `Memory Item`: conversation, fact, reflection, preference, long-term summary, task artifact.

This structure matches the project goal more closely than a single `agent + memory + tools` runtime view.

### Deployment view

```text
User / Team
    |
    v
CLI / API Clients / Channel Bots
    |
    v
Gateway API (auth, routing, session, streaming)
    |
    +--> Assistant Center
    +--> Task Orchestrator
    +--> Audit Center
    |
    v
Runtime Pool
    |
    +--> LLM Provider Adapters
    +--> Tool Registry
    +--> Skill Runtime
    +--> Sandbox Manager
    |
    v
Local Storage (.anyclaw)
    +--> memory/
    +--> gateway/state.json
    +--> audit/*.jsonl
    +--> runtimes/
    +--> sandboxes/
```

### Recommended implementation roadmap

- `Phase 1 - domain separation`: extract Assistant, Workspace, Task, and Permission Profile into stable structs and storage files instead of spreading them across config and session state.
- `Phase 2 - orchestration`: formalize planning, approvals, retries, and background jobs so complex tasks become traceable workflows rather than ad hoc session logic.
- `Phase 3 - control plane APIs`: build assistant creation, permission management, task monitoring, and audit replay around the existing gateway foundation.
- `Phase 4 - extension platform`: standardize skill metadata, tool risk labels, plugin trust, and external service adapters for marketplace-style expansion.

### Mapping to current repository

- `pkg/runtime/runtime.go`: good foundation for bootstrapping app dependencies, but currently initializes a single effective agent runtime; next step is assistant-scoped runtime factories.
- `pkg/agent/agent.go`: already covers prompt assembly, tool calling, and memory integration; next step is to separate planner/executor/observer responsibilities.
- `pkg/gateway/gateway.go`: already behaves like a control-plane entrypoint; next step is to split transport handlers from domain services.
- `pkg/gateway/state.go`: already stores sessions, jobs, orgs, projects, workspaces, and audit-like records; next step is to turn this into explicit persistent domain stores.
- `pkg/memory/memory.go`: keeps the local-first philosophy intact; next step is assistant-scoped memory and task artifact storage.
- `pkg/audit/audit.go`: good append-only audit basis; next step is richer risk, approval, and replay metadata.

In short, the optimized architecture for AnyClaw should be "control plane + orchestration plane + runtime core + security boundary + local data plane", which is much closer to the platform positioning described in your report than the current "single agent runtime" framing.

## Channels

- `telegram`: bot polling via Bot API
- `slack`: polling a default channel via Web API
- `discord`: polling a default channel via REST API
- `signal`: polling a local `signal-cli-rest-api` compatible endpoint
- `whatsapp`: Meta WhatsApp Cloud webhook receive + send replies

Production hardening notes:

- inbound channel messages now carry `message_id`, `user_id`, `username`, and `reply_target` metadata into session events
- Discord / Signal / WhatsApp use in-memory dedupe windows to avoid replaying the same inbound event repeatedly
- WhatsApp webhook POST requests can validate `X-Hub-Signature-256` with `channels.whatsapp.app_secret`
- Signal can send authenticated requests with `channels.signal.bearer_token`
- Discord stores richer transport metadata such as guild/thread style targets
- Signal now captures group and attachment metadata from inbound payloads
- WhatsApp webhook status callbacks are recorded as channel events
- Discord now supports signed interaction/slash-command webhooks at `/channels/discord/interactions`
- Discord can now use the Gateway WebSocket client for `MESSAGE_CREATE`, thread-style events, typing/presence signals, and interaction events when `discord.use_gateway_ws` is enabled

Example channel config:

```json
{
  "channels": {
    "routing": {
      "mode": "per-chat",
      "rules": [
        {
          "channel": "discord",
          "match": "support",
          "session_mode": "group",
          "queue_mode": "fifo",
          "reply_back": true,
          "title_prefix": "Discord Support"
        }
      ]
    },
    "discord": {
      "enabled": true,
      "bot_token": "discord-bot-token",
      "default_channel": "1234567890",
      "guild_id": "0987654321"
    },
    "whatsapp": {
      "enabled": true,
      "access_token": "meta-access-token",
      "phone_number_id": "1234567890",
      "verify_token": "shared-webhook-secret",
      "app_secret": "meta-app-secret"
    },
    "signal": {
      "enabled": true,
      "base_url": "http://127.0.0.1:8080",
      "number": "+15551234567",
      "bearer_token": "signal-rest-token"
    }
  }
}
```

## Skills Platform

- `skill.json` now supports platform metadata such as `version`, `permissions`, `entrypoint`, `source`, `registry`, `homepage`, and `install_command`
- built-in and remote skills can declare capability expectations before installation
- `anyclaw skill catalog [query]` shows marketplace/registry style results with version, permissions, and install hints
- `anyclaw skill info <name>` now shows installed skill metadata beyond name/version
- executable skills can provide a real filesystem `entrypoint`; AnyClaw exposes them as `skill_<name>` tools and executes them with `ANYCLAW_SKILL_*` environment variables
- executable skills support launcher auto-detection for `.py`, `.js`/`.mjs`/`.cjs`, `.sh`, and `.ps1`; direct binaries still run directly

Example skill manifest:

```json
{
  "name": "deploy-helper",
  "description": "Deployment workflow assistant",
  "version": "1.3.0",
  "registry": "skills.sh",
  "source": "https://skills.sh/deploy-helper",
  "homepage": "https://example.com/deploy-helper",
  "entrypoint": "builtin://deploy-helper",
  "permissions": ["tools:exec", "sandbox:run"],
  "install_command": "anyclaw skill install deploy-helper",
  "prompts": {
    "system": "You help users deploy and validate services safely."
  }
}
```

Executable skill runtime:

- when a skill has a non-`builtin://` `entrypoint`, it is registered as a callable tool named `skill_<name>`
- the executable receives JSON input via `ANYCLAW_SKILL_INPUT`
- additional env vars include `ANYCLAW_SKILL_NAME`, `ANYCLAW_SKILL_VERSION`, `ANYCLAW_SKILL_DIR`, `ANYCLAW_SKILL_TIMEOUT_SECONDS`, and `ANYCLAW_SKILL_PERMISSIONS`
- executable skill invocation follows the same `plugins.allow_exec` and timeout guardrails used for executable integrations
- launcher resolution order:
  - `.py` -> `python3`, then `python`
  - `.js` / `.mjs` / `.cjs` -> `node`
  - `.sh` -> `sh` on Unix, `bash` on Windows
  - `.ps1` -> `pwsh`, then `powershell`

Workspace model:

- AnyClaw now bootstraps a default local org/project/workspace from `agent.working_dir`, similar to OpenClaw's local workspace-first model
- the gateway ensures a concrete workspace resource exists instead of relying on placeholder ids like `default-org` and `default-project`
- `agent.working_dir` is normalized to an absolute path at startup so workspace identity stays stable across sessions and runtimes

Session model notes:

- `session_mode: main` keeps direct conversations isolated as primary sessions
- `session_mode: group` or `group-shared` lets multiple inbound messages land in group-scoped sessions
- `queue_mode` is stored per session so channels can coordinate FIFO-style turn handling
- `reply_back: true` marks sessions intended to echo responses back to the originating channel
- presence/typing state changes are emitted as gateway events during request handling

## Version

```
anyclaw version 2026.3.13
```
