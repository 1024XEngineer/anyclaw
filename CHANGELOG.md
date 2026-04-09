# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### Added
- `agent` command: Add `--agent` flag for specifying agent ID
- `agent` command: Add `--max-iterations` flag to limit agent loop iterations (default: 15)
- `agent` command: Add `--stream` flag for streaming output support
- `agent` command: Add validation requiring either `--agent` or `--session-id` flag
- Agent loop: Add context cancellation check to prevent hanging on timeout
- Agent loop: Add max iterations check to prevent infinite loops
- Agent: Add streaming events (`EventStreamContent`, `EventStreamThinking`, `EventStreamFinal`, `EventStreamDone`)

### Changed
- `agent` command: `--timeout` default changed from 120 to 600 seconds
- `agent` command: `--thinking` changed from boolean to string level (off|minimal|low|medium|high)
- `agent` command: `--message` now supports `-m` shorthand
- `agent` command: Updated flag descriptions to match openclaw CLI format
- Orchestrator: `streamAssistantResponse` now uses `ChatStream` for streaming providers

## 2026-02-27

### Added
- Feishu plugin: Add cron job support

### Changed
- Improve prompts

## 2026-02-26

### Added
- ACP (Agent Communication Protocol) support
- Feishu plugin: Add typing indicator
- Feishu plugin: Add image upload support
- Telegram: Add typing indicator (#19)

### Changed
- Feishu plugin: Improve markdown rendering
- Feishu plugin: Only response to its messages
- Feishu plugin: Improve logging and set log-level

### Fixed
- Fix wrong tool messages
- Fix `run_shell` name issue (#25)

## 2026-02-25

### Changed
- Improve config handling
- Improve SOUL.md template
- Refactor skills loading

### Fixed
- Fix bindings issue

## 2026-02-24

### Added
- Support feishu channel (#7)

### Changed
- Refactor agent architecture
- Improve agent performance

### Fixed
- Fix Windows issues (#23, #24)
- Merge PR from @qiangmzsx (#7)

## 2026-02-13

### Added
- Add infoflow support
- Add more logging

### Changed
- TUI and channels use the same logic
- Improve skills handling
- Improve web fetch
- Improve goreleaser configuration

### Fixed
- Fix Chinese issue in readline
- Fix readline issues
- Fix tool_call_id issues
- Fix goreleaser config

## 2026-02-12

### Added
- Add go-releaser for release v0.1.0
- Add "hi robot" feature
- Integrate qmd (#3)

### Changed
- Re-implement the agent like pi-mono (#10)
- Refactor skill package
- Improve agent/skills integration

### Fixed
- Fix agent command execution
- Fix feishu issue (#7)
- Fix logger race error (#9)
- Fix channels status command

## 2026-02-11

### Added
- Add tests
- Add history for readline
- Add `/status` command
- Support 8 types of config files
- Add find-skills feature
- Gemini image generation support

### Changed
- Improve channels
- Improve skills
- Improve QQ channel
- Set maxTokens configuration

### Fixed
- Fix go mod name
- Improve logics for tool failures (#2)
- Drop '400 input token limit is 97280' error

## 2026-02-10

### Added
- Add CLI commands

### Changed
- Improve agent loop
- Improve QQ channel

### Fixed
- Fix sending messages to QQ

## 2026-02-09

### Changed
- Re-implement smart_search with crawl4ai
- Improve browser tool usage

## 2026-02-08

### Changed
- Re-implement the browser tool

## 2026-02-06

### Added
- Initial commit
- Initial workable code
- Skills subcommands
- Clawhub subcommand for skill registry management
- Browser authentication for clawhub login
- QQ Official Bot API channel
- QQ and WeWork channel configurations
- Browser tool
- Slash commands
- Sandbox implementation

### Changed
- Change the logo

### Removed
- Remove legacy QQ WebSocket implementation
- Remove clawhub

## earlier

- Basic agent functionality
- Multi-channel support (Telegram, Discord, etc.)
- Tool system implementation
- Session management
- Memory store
