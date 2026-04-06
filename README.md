# AnyClaw

AnyClaw is a local-first AI agent workspace focused on transparent files, controllable tools, pluggable skills, and real task execution on the local machine.

## What it does

- Runs as a CLI, gateway service, and web control surface.
- Supports multiple LLM providers including Ollama, Qwen, OpenAI-compatible APIs, OpenAI, and Anthropic.
- Exposes local tools for files, shell commands, browser automation, desktop UI automation, OCR, and screenshots.
- Integrates with CLI-Anything through `clihub`, so AnyClaw can discover and execute agent-native software harnesses when a local `CLI-Anything-0.2.0` catalog is present.
- Keeps runtime state under `./.anyclaw/` and workspace context under `workflows/`.

## Quick start

```bash
git clone https://github.com/1024XEngineer/anyclaw.git
cd anyclaw
go build -o anyclaw ./cmd/anyclaw
./anyclaw onboard --non-interactive
./anyclaw doctor --connectivity=false
./anyclaw -i
```

Windows:

```powershell
git clone https://github.com/1024XEngineer/anyclaw.git
cd anyclaw
go build -o anyclaw.exe ./cmd/anyclaw
.\anyclaw.exe onboard --non-interactive --connectivity=false
.\anyclaw.exe doctor --connectivity=false
.\anyclaw.exe -i
```

## Initialize without API keys

AnyClaw can be initialized in a no-cloud, no-API-key state.

- The current default safe bootstrap path is local `ollama`
- `doctor --connectivity=false` skips live model checks
- You can switch providers later after startup

Example:

```powershell
.\anyclaw.exe onboard --non-interactive --connectivity=false
.\anyclaw.exe doctor --connectivity=false
```

If you want local chat immediately, start Ollama separately and pull a model such as `llama3.2`.

## Reset local state

Runtime state and history are stored locally and can be cleared.

- Config: `anyclaw.json`
- Runtime state: `./.anyclaw/`
- Workspace memories: `workflows/memory/` and `workflows/**/memory.db*`

If you want a clean reset with no API keys and no records:

1. Clear API keys from `anyclaw.json`
2. Remove `./.anyclaw/`
3. Remove `workflows/memory/`
4. Remove `workflows/**/memory.db*`
5. Recreate minimal state with `anyclaw onboard --non-interactive --connectivity=false`

## CLI Hub and CLI-Anything

If a local `CLI-Anything-0.2.0` directory exists beside or inside the workspace, AnyClaw can auto-discover it and expose CLI Hub features.

Useful commands:

```bash
anyclaw clihub list --runnable
anyclaw clihub capabilities
anyclaw clihub info anygen
anyclaw clihub exec anygen -- config path
```

This gives AnyClaw access to structured harnesses for tools such as Browser, LibreOffice, Blender, GIMP, ComfyUI, AnyGen, Draw.io, Audacity, and more, depending on local dependencies.

## Browser and desktop control

AnyClaw already includes native tool families for app control:

- Browser automation: navigate, click, type, upload, download, evaluate JS, capture snapshots, export PDF
- Desktop automation on Windows: open apps, enumerate windows, inspect UI Automation trees, target controls, set values, OCR, text matching, image matching, screenshot windows
- App workflows and connector plans: `anyclaw app ...`

Notes:

- Browser tasks are generally the most reliable.
- Windows desktop automation is available now, especially for standard UI controls.
- OCR-based flows benefit from installing Tesseract.
- CLI-Anything harnesses are often more reliable than raw GUI automation when a matching harness exists.

## Web UI workspace

AnyClaw includes a pnpm-based UI workspace:

```bash
pnpm ui:install
pnpm ui:dev
pnpm ui:test
pnpm ui:build
```

- Source: `ui/`
- Build output: `dist/control-ui/`
- Runtime route: gateway `/dashboard` prefers `dist/control-ui/` and falls back when missing
- Compatible routes: `/dashboard` and `/control`
- Optional root override: `ANYCLAW_CONTROL_UI_ROOT=/abs/path/to/dist/control-ui`

## Common commands

```bash
anyclaw -i
anyclaw doctor --connectivity=false
anyclaw onboard --non-interactive --connectivity=false
anyclaw setup
anyclaw gateway start
anyclaw status --all
anyclaw health --verbose
anyclaw channels status
anyclaw models status
anyclaw clihub list --runnable
anyclaw clihub capabilities
anyclaw app list
anyclaw app workflows resolve "remove background"
anyclaw task run "summarize this workspace"
```

OpenClaw-style aliases are supported for common namespaces such as `skills`, `plugins`, `agents`, `apps`, `setup`, `daemon`, `status`, `health`, `sessions`, `approvals`, `channels`, `models`, and `config`.

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
pkg/apps/        app runtime, bindings, workflows
pkg/clihub/      CLI-Anything catalog and execution
pkg/config/      config loading and validation
pkg/gateway/     HTTP / websocket gateway
pkg/memory/      file-first memory
pkg/plugin/      plugin and app connector system
pkg/skills/      skill loading and execution
pkg/tools/       tool registry and built-ins
skills/          bundled skills
workflows/       workspace bootstrap files
```

## Notes

- `anyclaw.json` stores runtime configuration.
- `./.anyclaw/` stores local runtime state and logs.
- `workflows/` stores bootstrap context and workspace memory.
- `doctor --connectivity=false` is the easiest way to validate a fresh local setup.

## Chinese Display On Windows

If the Windows console shows garbled Chinese text while running AnyClaw, switch the terminal code page to UTF-8 first:

```bash
chcp 65001
```

## Version

`2026.3.13`
