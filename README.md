# AnyClaw

AnyClaw is a local-first AI agent workspace focused on transparent files, controllable tools, and pluggable skills.

## What it does

- Keeps memory in readable files instead of opaque storage layers.
- Exposes a CLI, gateway, control UI, and canvas surface.
- Supports multiple LLM providers such as OpenAI, Anthropic, Qwen, Ollama, and OpenAI-compatible APIs.
- Lets agents call local tools for files, shell commands, browser control, desktop actions, and more.

## Quick start

```bash
git clone https://github.com/1024XEngineer/anyclaw.git
cd anyclaw
go build -o anyclaw ./cmd/anyclaw
./anyclaw --setup
./anyclaw -i
```

## Web UI workspace

AnyClaw now includes a pnpm-based UI workspace similar to OpenClaw:

```bash
pnpm ui:install
pnpm ui:dev
pnpm ui:test
pnpm ui:build
```

- Source: `ui/`
- Build output: `dist/control-ui/`
- Runtime route: gateway `/dashboard` prefers `dist/control-ui/` and falls back to embedded dashboard when missing
- Route compatibility:
  - Default: `/dashboard`
  - Legacy alias: `/control`
  - Custom: set `gateway.control_ui.base_path` (for example `/console`) and keep `/dashboard` + `/control` as compatible aliases
- Optional root override: `ANYCLAW_CONTROL_UI_ROOT=/abs/path/to/dist/control-ui`
- Optional build base path: `ANYCLAW_CONTROL_UI_BASE_PATH=/anyclaw/ pnpm ui:build`
- Configurable gateway route/root in `anyclaw.json`:

```json
{
  "gateway": {
    "control_ui": {
      "base_path": "/console",
      "root": "dist/control-ui"
    }
  }
}
```

## Common commands

```bash
anyclaw -i
anyclaw doctor
anyclaw setup
anyclaw config validate
anyclaw gateway start
anyclaw status --all
anyclaw health --verbose
anyclaw channels status
anyclaw models status
anyclaw apps list
anyclaw cron list
anyclaw skill list
anyclaw skills list
anyclaw plugins list
anyclaw agents list
anyclaw agent list
anyclaw task run "summarize this workspace"
```

OpenClaw-style aliases and command surfaces are supported for common namespaces such as `skills`, `plugins`, `agents`, `apps`, `setup`, `daemon`, `status`, `health`, `sessions`, `approvals`, `channels`, `models`, and `config`.

Interactive commands:

```text
/exit, /quit, /q
/clear
/memory
/skills
/tools
/provider
/providers
/models <provider>
/agents
/agent use <name>
/audit
/set provider <value>
/set model <value>
/set apikey <value>
/set temp <value>
/help
```

## Project layout

```text
cmd/anyclaw/     CLI entrypoint
pkg/agent/       agent runtime
pkg/config/      config loading and validation
pkg/gateway/     HTTP / websocket gateway
pkg/memory/      file-first memory
pkg/skills/      skill loading and execution
pkg/tools/       tool registry and built-ins
pkg/web/         lightweight web surfaces
skills/          bundled skills
workflows/       workspace bootstrap files
```

## Notes

- `anyclaw.json` stores runtime configuration.
- `./.anyclaw/` stores local state, memory, and runtime files.
- The web control page supports `/dashboard`, `/control`, and configured `gateway.control_ui.base_path`.
- The canvas page lives under `/canvas/`.

## 中文显示说明

在 Windows 环境下运行 AnyClaw 时，如果控制台显示中文乱码，请先设置 UTF-8 代码页：

```bash
chcp 65001
```

## Version

`2026.3.13`
