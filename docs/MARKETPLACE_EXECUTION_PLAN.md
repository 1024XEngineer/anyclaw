# AnyClaw Cloud Marketplace Master Execution Plan

This document is the single execution plan for building the AnyClaw cloud marketplace. All implementation rounds must follow this plan unless the owner explicitly approves a plan change.

## 0. Execution Contract

### 0.1 Authority

This file is the working plan I will follow round by round. The product, architecture, technical specification, and module specification documents remain design references, but day-to-day implementation sequencing follows this file.

If a future implementation detail conflicts with this file, I must stop and report:

- The conflicting point.
- Why the current plan is insufficient or suboptimal.
- The proposed change.
- Impact on scope, tests, data compatibility, and later rounds.

I will only change direction after the owner confirms the adjustment.

### 0.2 Non-Negotiable Principles

- Keep install and bind separate in API, data, and UI.
- Cloud registry provides catalog, search, package metadata, and package distribution. Local AnyClaw makes final policy decisions and performs install, verification, binding, runtime refresh, and audit.
- Official marketplace artifact kinds are `agent`, `skill`, and `cli`.
- Existing `plugin` market behavior is compatibility surface, not the future primary marketplace model.
- Preserve old routes until migration is complete: `/market/search`, `/market/plugins`, `/market/plugins/*`, `/market/installed`, `/market/categories`.
- Build incrementally on existing Gateway, Runtime, Skills, Agent Profiles, CLIHub, plugin Store, and UI shell.
- Prefer the Go standard library and current project modules first. Add external dependencies only after an explicit library check.
- P1 and P2 only promise availability after runtime refresh or next turn. Same-turn hot reload belongs to the hot reload round.

### 0.3 Definition of Done

The marketplace is considered complete only when these are true:

- Users can browse local and cloud `agent / skill / cli` artifacts in one UI.
- Users can inspect artifact details, versions, permissions, risk, trust, compatibility, and dependencies.
- Users can install an artifact with visible decision, progress, verification, receipt, and failure recovery.
- Users can bind installed artifacts to supported targets.
- Users can see installed, bound, active, disabled, error, rolled_back, and quarantined states.
- Users can uninstall and, where supported, roll back.
- All install, bind, uninstall, upgrade, policy, and failure events are auditable.
- Cloud registry can serve real artifacts and package downloads.
- Agent-triggered marketplace installs are policy-controlled and auditable.

## 1. Current Code Baseline

### 1.1 Reusable Local Assets

- `pkg/marketplace/types.go`: initial unified `Artifact` model.
- `pkg/marketplace/catalog.go`: local catalog aggregation for agents, skills, plugins, and agent store packages.
- `pkg/gateway/gateway_market_artifacts_api.go`: current `GET /market/artifacts`.
- `pkg/gateway/gateway_market_api.go`: legacy plugin market routes.
- `pkg/extensions/plugin/store_impl.go`: existing plugin download, extract, verify, install, update, uninstall, and rollback patterns.
- `ui/src/pages/Market/MarketPage.tsx`: existing market shell.
- `ui/src/features/market/useMarketDirectory.ts`: current market page data hook.
- Existing local capability systems: SkillsManager, Agent Profiles, CLIHub, runtimePool.

### 1.2 Known Gaps

- No cloud registry server.
- No remote registry client for `agent / skill / cli`.
- No artifact detail/version API in the new marketplace surface.
- No durable marketplace job model.
- No unified receipt, binding, policy, audit, event, or outbox path.
- UI cloud view is still mostly placeholder.
- UI does not yet drive install, bind, progress, permissions, or error recovery.
- `plugin` still appears as an artifact kind in the initial local model and must be migrated behind compatibility handling.

## 2. Target Architecture

### 2.1 Local AnyClaw Side

```text
+---------------------------+
| UI Market Shell           |
+-------------+-------------+
              |
              v
+---------------------------+
| Gateway /market API        |
+-------------+-------------+
              |
              v
+---------------------------+
| Marketplace Application    |
| Install / Bind / Uninstall |
| Upgrade / AutoSupplement   |
+-------------+-------------+
              |
              v
+---------------------------+
| Domain Services            |
| Catalog / Registry Client  |
| Policy / Jobs / Receipts   |
| Bindings / Events / Audit  |
+-------------+-------------+
              |
              v
+---------------------------+
| Capability Ports           |
| Agent / Skill / CLI        |
| Runtime Refresh            |
+-------------+-------------+
              |
              v
+---------------------------+
| Existing AnyClaw Runtime   |
| Skills / Profiles / CLIHub |
+---------------------------+
```

### 2.2 Cloud Registry Side

```text
cmd/anyclaw-registry
pkg/marketregistry/api
pkg/marketregistry/catalog
pkg/marketregistry/storage
pkg/marketregistry/db
pkg/marketregistry/auth
pkg/marketregistry/signing
pkg/marketregistry/admin
```

The registry starts in this repository so local client and server contracts can evolve together. It may later move to a separate service repository after protocol v1 stabilizes.

### 2.3 Package Format

Every package must include:

```text
anyclaw.artifact.json
```

The manifest must describe:

- `id`
- `kind`
- `name`
- `version`
- `summary`
- `description_md`
- `publisher`
- `permissions`
- `risk_level`
- `trust_level`
- `compatibility`
- `dependencies`
- kind-specific payload:
  - `agent`
  - `skill`
  - `cli`

Package archive formats:

- P1: `.zip` and `.tar.gz`, using Go standard library extraction.
- P2+: optional signatures.

## 3. Local API Contract

### 3.1 New Marketplace Routes

```text
GET    /market/control-plane
GET    /market/artifacts
GET    /market/artifacts/{id}
GET    /market/artifacts/{id}/versions
POST   /market/install
POST   /market/upgrade
POST   /market/uninstall
GET    /market/jobs/{id}
GET    /market/jobs
POST   /market/jobs/{id}/cancel
GET    /market/bindings
POST   /market/bindings
DELETE /market/bindings/{id}
GET    /market/events
POST   /market/refresh
```

### 3.2 Legacy Routes to Preserve

```text
GET  /market/search
GET  /market/plugins
GET  /market/plugins/*
POST /market/plugins/*
GET  /market/installed
GET  /market/categories
```

### 3.3 Binding Targets

Supported target types:

```text
main_agent
persistent_subagent
workspace
runtime_global
```

`main_agent` must be normalized by Gateway to the current workspace active main-agent profile id before persistence.

### 3.4 Artifact Status

Derived artifact status values:

```text
available
installing
installed
bound
active
disabled
error
rolled_back
quarantined
```

Status must be derived from receipts, bindings, and quarantine markers. UI must not recalculate status independently.

### 3.5 Job States

```text
pending
running
rolling_back
succeeded
failed
canceled
rolled_back
interrupted
```

Only install, upgrade, and auto supplement create jobs in P1/P2. Bind and uninstall are synchronous unless a later approved plan change says otherwise.

## 4. Cloud Registry Contract

### 4.1 Registry Server MVP Routes

```text
GET  /v1/control-plane
GET  /v1/sources
GET  /v1/artifacts
GET  /v1/artifacts/{id}
GET  /v1/artifacts/{id}/versions
POST /v1/artifacts/{id}/resolve
GET  /v1/download/{artifact_id}/{version}
POST /v1/search
```

### 4.2 Admin Routes

These are not required for first usable marketplace, but the schema must leave room for them:

```text
POST /v1/publish
POST /v1/artifacts/{id}/quarantine
POST /v1/artifacts/{id}/unquarantine
GET  /v1/admin/audit
GET  /v1/admin/downloads
```

### 4.3 Storage Plan

Development:

```text
.anyclaw-registry/
  registry.db
  packages/
  audit/
```

Production:

```text
Postgres
Object storage: S3 / R2 / OSS / COS
Nginx or Caddy TLS frontend
Optional OpenTelemetry collector
```

### 4.4 Registry Database Tables

Minimum tables:

- `artifacts`
- `artifact_versions`
- `publishers`
- `tokens`
- `downloads`
- `quarantine`
- `audit_events`

P1 may use SQLite for local development. Production deployment targets Postgres.

## 5. Round-by-Round Plan

### Round 0: Plan Freeze and Contract Cleanup

Goal: freeze the execution path before implementation expands.

Deliverables:

- This master plan.
- Confirmed official artifact kinds: `agent / skill / cli`.
- Compatibility policy for legacy `plugin`.
- Final local API list.
- Final cloud registry MVP API list.
- Final status, job, decision, and binding target enums.

Acceptance:

- The owner approves this plan.
- Later work can be reviewed against this file.

Exit condition:

- No code implementation starts until this round is accepted.

### Round 1: Read-Only Local Marketplace API

Goal: make the local marketplace API the source of truth for the UI.

Deliverables:

- Update `pkg/marketplace` types to include full target fields needed by P1.
- Keep `plugin` behind compatibility handling.
- Extend local catalog to include local CLI artifacts from CLIHub.
- Add artifact detail shape for local artifacts.
- Add tests for local catalog filtering and status.
- Refactor UI hook to consume `/market/artifacts` instead of workspace snapshot for market entries.

Acceptance:

- `GET /market/artifacts?kind=agent&source=local` returns local agents.
- `GET /market/artifacts?kind=skill&source=local` returns local skills.
- `GET /market/artifacts?kind=cli&source=local` returns local CLI capabilities where available.
- Market UI renders from the marketplace API.
- Legacy market tests still pass.

### Round 2: Cloud Registry Server MVP

Goal: create a real registry service AnyClaw can talk to.

Deliverables:

- Add `cmd/anyclaw-registry`.
- Add `pkg/marketregistry`.
- Implement read-only registry routes.
- Implement local package storage.
- Add fixture seed command or dev seed files for one agent, one skill, and one CLI.
- Add checksum generation.
- Add Docker/dev run instructions.

Acceptance:

- Registry starts locally.
- `GET /v1/artifacts` returns seeded cloud artifacts.
- `GET /v1/artifacts/{id}` returns detail.
- `GET /v1/artifacts/{id}/versions` returns versions.
- `POST /v1/artifacts/{id}/resolve` returns download metadata.
- `GET /v1/download/...` streams the package.

### Round 3: Remote Registry Client and Cloud Read Path

Goal: show real cloud artifacts in AnyClaw.

Deliverables:

- Add `pkg/marketplace/registry` client.
- Support endpoint, token, protocol version, timeout, retry, and short TTL cache.
- Merge cloud artifacts with local installed/bound status.
- Add cloud error degradation.
- Wire UI cloud tab to real `/market/artifacts?source=cloud`.

Acceptance:

- AnyClaw can show cloud agent, skill, and CLI entries from local registry server.
- Registry unavailable shows a recoverable cloud error and preserves local view.
- Cloud artifacts include permissions, risk, trust, compatibility, score/hit signal if available.

### Round 4: Install Jobs, Pipeline, and Receipts

Goal: install cloud artifacts into local AnyClaw safely.

Deliverables:

- Add `InstallUseCase`.
- Add `JobStore`.
- Add `POST /market/install`.
- Add `GET /market/jobs/{id}` and `GET /market/jobs`.
- Add installer pipeline:
  - Resolve
  - Download
  - Verify
  - Install
  - Receipt
- Add receipt writer.
- Add rollback for verify/install/receipt failures.
- Add tests for success, checksum failure, and idempotency.

Acceptance:

- Install returns `job_id`.
- Job progress is queryable.
- Successful install writes receipt.
- Checksum mismatch rolls back and records `rolled_back`.
- Duplicate `Idempotency-Key` returns the same job.

### Round 5: Binding and Runtime Refresh

Goal: connect installed artifacts to runtime targets without confusing install and bind.

Deliverables:

- Add `BindingService`.
- Add `POST /market/bindings`.
- Add `GET /market/bindings`.
- Add `DELETE /market/bindings/{id}`.
- Normalize `main_agent` target.
- Add coarse runtime refresh through existing runtimePool methods.
- Derive artifact status from receipt + binding.

Acceptance:

- Install alone yields `installed`, not `active`.
- Binding yields `bound` or `active`.
- `main_agent` binding persists a concrete active main-agent profile id.
- Next runtime refresh/turn can see the bound capability.

### Round 6: UI Install, Bind, and Error Flows

Goal: make the user-facing marketplace complete for manual operation.

Deliverables:

- Artifact detail panel.
- Install confirmation dialog.
- Permissions, risk, trust, compatibility, dependency display.
- Five-step install progress UI.
- Binding target selector.
- Installed/bound/active/disabled/error/rolled_back/quarantined visuals.
- Recoverable error actions.

Acceptance:

- User can install from UI.
- User can bind from UI.
- User can see why an install failed.
- UI clearly separates installed from bound.

### Round 7: Policy, Decision, and Audit

Goal: make installs safe and reviewable.

Deliverables:

- Add `DecisionPolicy`.
- Implement `auto / ask / block`.
- Agent and CLI default to Ask.
- Low-risk verified Skill may Auto only if config allows it.
- High-risk, incompatible, invalid checksum, invalid signature, and quarantined artifacts Block.
- Add EventBus.
- Add audit jsonl.
- Add notification events for UI.

Acceptance:

- Policy decisions are written to receipt and audit.
- High-risk artifact cannot be installed through normal UI.
- Auto install has visible notification and audit.
- Ask install displays full permissions before confirmation.

### Round 8: Uninstall, Upgrade, and Rollback

Goal: support lifecycle management.

Deliverables:

- Add `POST /market/uninstall`.
- Add 30-second undo path in UI where feasible.
- Add `POST /market/upgrade`.
- Add versions API integration.
- Add permissions diff display.
- Add rollback to previous version.

Acceptance:

- Uninstall removes bindings and writes receipt/audit.
- Upgrade keeps bindings on success.
- Upgrade failure rolls back.
- Permission additions are highlighted before upgrade.

### Round 9: Agent Auto Supplement

Goal: allow the main Agent to use the marketplace under policy control.

Deliverables:

- Add local capability index.
- Add capability need detector.
- Add capability router.
- Add main-agent marketplace tools:
  - `market_search_artifacts`
  - `market_install_artifact`
  - `market_bind_artifact`
- Record `installed_by=agent`.

Acceptance:

- Agent can search cloud marketplace for missing capability.
- Agent-triggered install follows policy.
- Ask path requires user confirmation.
- Auto path is audited and visible.

### Round 10: Hot Reload

Goal: make newly installed or bound capabilities available with finer refresh scope.

Deliverables:

- Add RefreshScope abstraction.
- Add HotReloadCoordinator.
- Add scoped refresh for agent/workspace/session where supported.
- Replace coarse invalidation gradually.

Acceptance:

- Refresh failure is isolated to the target scope.
- Other workspaces/sessions are not unnecessarily invalidated.
- Same-turn or near-same-turn capability availability works where supported.

### Round 11: Production Registry Hardening

Goal: prepare the registry for real deployment.

Deliverables:

- Postgres adapter.
- Object storage adapter.
- Publisher token management.
- Quarantine/admin APIs.
- Download stats.
- Optional signatures.
- Deployment docs.

Acceptance:

- Registry can run behind TLS.
- Package downloads work from object storage.
- Private token-protected source works.
- Quarantined artifacts are blocked locally.

## 6. Test Strategy

Each round must include tests proportional to risk.

Minimum required test groups:

- Unit tests for pure domain logic.
- Gateway handler tests for new routes.
- Registry client tests using `httptest`.
- Installer tests with local fixture archives.
- Job idempotency tests.
- Receipt and binding persistence tests.
- UI tests for key states where the existing UI test setup supports it.

Round 4 and later must include failure tests for:

- Registry unavailable.
- Artifact not found.
- No compatible version.
- Download failure.
- Checksum mismatch.
- Install failure.
- Receipt failure.
- Runtime refresh failure.

## 7. Security and Policy Baseline

High-risk permissions:

```text
process.exec
process.kill
desktop.control
browser.control
network.any
secrets.read
fs.delete
```

Default decisions:

- `skill + verified + low risk + compatible + user enabled auto`: Auto.
- `skill` otherwise: Ask.
- `agent`: Ask.
- `cli`: Ask.
- `high risk`: Block unless a later explicit administrator override feature is approved.
- `checksum mismatch`: Block/fail.
- `signature invalid`: Block/fail.
- `quarantined`: Block.
- `incompatible`: Block.

## 8. Deployment Plan

### 8.1 Local Development

```powershell
go run ./cmd/anyclaw-registry serve --db sqlite --storage local
go run ./cmd/anyclaw gateway start
```

### 8.2 Production

```text
Caddy/Nginx TLS
  -> anyclaw-registry
Postgres
Object storage
Optional OpenTelemetry Collector
```

### 8.3 Configuration

Local AnyClaw config target:

```json
{
  "marketplace": {
    "registry_endpoint": "https://registry.anyclaw.example.com",
    "registry_token": "${ANYCLAW_REGISTRY_TOKEN}",
    "protocol_version": "1.0",
    "auto_install_skill": false,
    "cache_ttl_seconds": 60,
    "download_timeout_seconds": 300,
    "request_timeout_seconds": 30
  }
}
```

Environment overrides:

```text
ANYCLAW_MARKETPLACE_ENDPOINT
ANYCLAW_REGISTRY_TOKEN
ANYCLAW_MARKETPLACE_DISABLE_REMOTE
ANYCLAW_OTLP_ENDPOINT
```

## 9. Change Control

I must not silently change this plan.

Allowed without approval:

- Small naming improvements that do not change API, data, or behavior.
- Internal helper extraction.
- Test-only fixture changes.

Requires owner confirmation:

- Changing round order.
- Adding a new required external dependency.
- Changing artifact package format.
- Changing API response fields.
- Changing status or job state semantics.
- Replacing SQLite/Postgres/object storage assumptions.
- Making bind or uninstall asynchronous before planned.
- Adding same-turn hot reload before Round 10.

## 10. Implementation Status

Current execution status:

- Round 0: complete. The owner approved this plan.
- Round 1: complete. The local read-only marketplace API and UI read path are in place.
- Round 2: complete. The cloud registry MVP exists under `cmd/anyclaw-registry` and `pkg/marketregistry`.
- Round 3: complete. AnyClaw can read cloud artifacts from the registry through `/market/artifacts?source=cloud`.
- Round 4: complete. Cloud artifacts can be installed through marketplace jobs with download, checksum verification, local receipt writing, rollback on failure, and idempotency.
- Round 5: complete. Installed artifacts can be bound to marketplace targets, artifact status is derived from receipts and bindings, and binding/refresh invalidates the runtime pool.
- Round 6: complete. The UI can inspect artifact details, confirm installs, show five-step install progress and recoverable errors, and bind installed artifacts to supported targets while keeping install and bind separate.
- Round 7: complete. Marketplace install decisions now use `auto / ask / block`, high-risk and incompatible artifacts are blocked before download, low-risk verified skills may auto-install only when configured, decisions are written to receipts and marketplace audit jsonl, and install/bind notification events are queryable through `/market/events`.
- Round 8: complete. Marketplace lifecycle APIs now support uninstall and upgrade, uninstall removes bindings and receipts with audit/events, upgrade keeps bindings on success, upgrade verification failures preserve previous receipts and bindings, and the UI exposes uninstall, upgrade, undo-window notice, and permission diff display.
- Round 9: complete. Main Agent now has policy-controlled marketplace tools for searching, installing, and binding artifacts; local capability routing can prefer installed capabilities or recommend cloud installs; ask decisions require explicit confirmation, agent installs record `installed_by=agent`, and auto/ask/bind paths are audited and tested.
- Round 10: complete. Marketplace refresh now uses a `RefreshScope` abstraction and `HotReloadCoordinator` to refresh runtime, agent, workspace, project, session, or global scopes; binding and upgrade flows refresh only affected scopes, session refresh resolves the session execution binding, failure is isolated per scope, and optional runtime warming is available for explicit runtime refreshes.
- Round 11: complete. The registry now has configurable `database/sql` store wiring, a package storage adapter seam, publisher token creation and validation, publish route support, quarantine/unquarantine admin APIs, audit and download stats APIs, optional artifact signatures in resolve/download responses, admin bearer-token protection, and deployment documentation for TLS, Postgres-style builds, and durable object storage adapters.
- Next action: Marketplace execution plan complete; remaining work should be tracked as hardening bugs, deployment-specific adapters, or approved post-plan enhancements.
