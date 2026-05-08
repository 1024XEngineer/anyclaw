# Capability Planner and Marketplace Supplement

This document records the current MainAgent capability planning path. It is
intentionally small and deterministic: the planner helps select tools and short
context for the current turn, while marketplace install and bind still require
the existing approval and policy path.

## Flow

1. `Agent.Run` / `RunStream` builds a `CapabilityPlan` before tool selection.
2. The plan classifies the user request as simple, local execution, complex, or
   specialized.
3. Tool selection uses the plan to expose only the relevant tools for the turn:
   local skill tools, `delegate_task`, CLIHub tools, or `market_search_artifacts`.
4. For specialized gaps, MainAgent performs a bounded pre-search with
   `market_search_artifacts` and adds a compact search summary to the turn.
5. If the user explicitly confirms a marketplace candidate in a later turn,
   MainAgent exposes `market_install_artifact` or `market_bind_artifact`.
6. Successful agent-side install or bind calls trigger runtime integration and a
   tool registry refresh so the new skill, agent, or CLI can be used in later
   turns.

## Safety Boundaries

- Do not inject the full marketplace catalog into the system prompt.
- Do not expose install or bind tools during the first missing-capability search
  turn.
- Do not auto-install high-risk capabilities. The marketplace policy result and
  tool approval flow remain authoritative.
- Do not bypass `market_install_artifact` / `market_bind_artifact` approvals.
- Do not treat arbitrary dotted strings, versions, or domains as artifact ids.
  Confirmation detection only accepts likely marketplace ids containing an
  artifact kind segment such as `agent`, `skill`, `cli`, or `plugin`.

## Runtime Integration

Marketplace tools accept optional hooks. Runtime bootstrap and refresh register
these hooks so installs done by MainAgent have the same practical effect as UI
installs:

- skills are copied or materialized into the configured skills directory and
  attached to the main profile;
- agents are added as enabled marketplace profiles;
- CLIs are added to a local CLI-Anything registry;
- `RefreshToolRegistry` reloads skills and re-registers built-in, marketplace,
  skill, and plugin tools.

The gateway still owns its HTTP job path. The runtime hook exists so direct
MainAgent tool calls do not leave behind only a receipt.

## Testing Contract

Tests should keep these behaviors stable:

- simple requests do not expose bulk tools;
- specialized gaps expose search only;
- confirmation turns expose install or bind only when a likely artifact id is in
  recent context;
- pre-search context is compact and does not include the full catalog;
- install-side runtime refresh makes new skill tools visible;
- marketplace tools remain main-agent-only.
