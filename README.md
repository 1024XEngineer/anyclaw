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
- The web control page lives under `/control/`.
- The canvas page lives under `/canvas/`.

## Version

`2026.3.13`
