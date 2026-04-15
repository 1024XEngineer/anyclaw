# anyclaw

`anyclaw` is the Go foundation for a future OpenClaw-compatible system.

This repository starts with a layered skeleton that mirrors OpenClaw's major boundaries:

- `internal/core`: centralized architecture contracts and interface baseline
- `internal/protocol`: Gateway wire contract
- `internal/config`: runtime configuration
- `internal/security`: roles, scopes, identity, pairing, approvals
- `internal/gateway`: control-plane runtime
- `internal/routing`: inbound route resolution
- `internal/session`: session state and serialized execution
- `internal/agent`: agent loop runtime
- `internal/providers`: model provider registry and integrations
- `internal/tools`: tool registry and built-in tools
- `internal/channels`: messaging channel registry and adapters
- `internal/pluginruntime`: plugin manifest and registration runtime
- `pkg/sdk`: public extension contracts

The current goal is not feature-completeness yet. The goal is a stable package layout and interface baseline that can absorb OpenClaw-compatible behavior over time.
