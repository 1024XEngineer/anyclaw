# AnyClaw Website And Marketplace Security

This document is the Round 9 security baseline for the first public deployment.

## Tokens

Registry admin token:

- Required in production.
- Stored only in `.env.production` or server secret storage.
- Used for admin-only APIs: publisher token creation/revocation, quarantine, audit, download stats.
- Generate with `openssl rand -hex 32`.

Publisher token:

- Created by an admin.
- Shown once by `POST /v1/admin/tokens`.
- Used by publishers for `POST /v1/publish`.
- Rotate by creating a new token, updating the publisher environment, testing publish, then revoking the old token.

Gateway API token:

- Required if the Gateway is reachable from the public internet.
- Prefer not exposing port `18789` publicly for the first launch.

## Registry Production Guard

Production compose runs:

```text
anyclaw-registry serve --require-admin-token=true
```

If `ANYCLAW_REGISTRY_ADMIN_TOKEN` is empty, registry startup fails. This prevents accidental public admin access.

## Publisher Token Scripts

Create:

```powershell
.\scripts\registry-create-publisher-token.ps1 `
  -BaseUrl http://SERVER_IP `
  -AdminToken $env:ANYCLAW_REGISTRY_ADMIN_TOKEN `
  -PublisherId "AnyClaw Labs"
```

Revoke:

```powershell
.\scripts\registry-revoke-publisher-token.ps1 `
  -BaseUrl http://SERVER_IP `
  -AdminToken $env:ANYCLAW_REGISTRY_ADMIN_TOKEN `
  -TokenId token-20260507120000.000000000
```

The scripts accept either a site origin such as `http://SERVER_IP`, a same-origin API base such as `http://SERVER_IP/v1`, or direct registry testing via `http://SERVER_IP:8791`.

## Ports And Security Group

Open to the internet:

- `80`: website and `/v1/*` same-origin registry proxy.
- `443`: later HTTPS entry after domain setup.
- `22`: SSH, preferably restricted to the owner's IP.

Do not open by default:

- `8791`: registry internal port. Open only temporarily for smoke tests.
- `18789`: AnyClaw Gateway. Keep private unless `ANYCLAW_API_TOKEN` is set and there is a concrete public-use reason.

## First Launch Rules

- Do not commit `.env.production`.
- Do not put admin token or publisher token in website code.
- The `/admin` web console must stay behind manual admin token entry. Do not persist the token in localStorage or hard-code it in frontend bundles.
- `/v1/admin/tokens` returns publisher token metadata only. Token secrets are shown only once on creation.
- Keep registry writes behind bearer tokens.
- Use same-origin `/v1/*` for public read APIs to avoid browser CORS and extra exposed ports.
- Review `GET /v1/admin/audit` after publishing and admin operations.

## Marketplace Install Policy

Round 18 adds the client-side and server-side install safety gate:

- Every install resolves metadata before download and records a `market.policy.decision` audit/event.
- High-risk artifacts are blocked by policy and are not downloaded.
- Quarantined artifacts cannot be resolved or downloaded. Registry returns `410 artifact_unavailable`.
- Marketplace installs require `user_confirmed=true` unless a low-risk verified skill is explicitly allowed by local auto-install policy.
- High-risk permissions require a separate `risk_acknowledged=true` acknowledgement. Examples include `process.exec`, `process.kill`, `desktop.control`, `browser.control`, `network.any`, `secrets.read`, and `fs.delete`.
- Client job responses include policy `reason`, `reasons`, risk/trust levels, permissions, and high-risk permissions so the UI can show the reason instead of failing silently.

## Download Integrity

- Registry resolve responses must include `checksum_sha256`.
- The installer blocks before download when checksum metadata is missing.
- The installer calculates SHA256 after download and before extraction. A mismatch blocks the install and rolls back any staged files.
- Registry download responses include `X-Checksum-SHA256`; optional signature metadata is exposed through `signature` and `X-Artifact-Signature`.
- Archives are extracted with path traversal and symlink checks before being copied into the installed artifact directory.

## Package Signature Scheme

Round 18 documents the signature contract but does not add signing or malware-scanning dependencies.

Current production-safe contract:

- `versions[].signature` is optional metadata accepted at publish time and returned by resolve/download.
- `checksum_sha256` remains the mandatory install integrity gate.
- Unsigned packages are allowed only when checksum verification passes and the local policy permits the artifact.

Proposed enforcement path for a later round:

- Use a detached Ed25519 signature over the exact package bytes identified by `checksum_sha256`.
- Store publisher public keys in registry admin-managed publisher metadata.
- Resolve responses return `signature`, `signature_algorithm`, and `publisher_key_id`.
- Clients verify checksum first, then verify the detached signature against the trusted publisher key.
- Key rotation keeps old public keys active until all still-supported versions age out.
- Signature verification failure should be a policy block before extraction.

Malware scanning and third-party signing tools may require new operational dependencies and must be separately approved before implementation.
